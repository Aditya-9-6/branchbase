package risk

import (
	"testing"
)

func TestStripCommentsAndTransactions(t *testing.T) {
	sql := `
		-- This is a comment
		/* Multi-line
		   comment */
		ALTER TABLE users DROP COLUMN email;
		-- Another comment
	`
	cleaned := StripCommentsAndTransactions(sql)
	expected := "ALTER TABLE users DROP COLUMN email;"
	if cleaned != expected {
		t.Errorf("StripCommentsAndTransactions failed: got %q, expected %q", cleaned, expected)
	}
}

func TestSplitStatements(t *testing.T) {
	sql := `
		BEGIN;
		CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT);
		INSERT INTO users (name) VALUES ('hello; world');
		ALTER TABLE users ADD COLUMN age INT;
		COMMIT;
	`
	stmts := SplitStatements(sql)
	if len(stmts) != 3 {
		t.Fatalf("expected 3 executable statements (excluding BEGIN/COMMIT), got %d: %v", len(stmts), stmts)
	}
}

func TestDDLParserOperations(t *testing.T) {
	parser := NewDDLParser("postgres")

	tests := []struct {
		sql          string
		expectedType string
		targetTable  string
		targetColumn string
	}{
		{
			sql:          "DROP TABLE IF EXISTS legacy_users;",
			expectedType: "DROP_TABLE",
			targetTable:  "legacy_users",
		},
		{
			sql:          "TRUNCATE TABLE sessions;",
			expectedType: "TRUNCATE",
			targetTable:  "sessions",
		},
		{
			sql:          `ALTER TABLE "public"."users" DROP COLUMN "email";`,
			expectedType: "DROP_COLUMN",
			targetTable:  "users",
			targetColumn: "email",
		},
		{
			sql:          "ALTER TABLE orders ALTER COLUMN amount TYPE NUMERIC(12,2);",
			expectedType: "ALTER_TYPE",
			targetTable:  "orders",
			targetColumn: "amount",
		},
		{
			sql:          "CREATE INDEX CONCURRENTLY idx_users_email ON users (email);",
			expectedType: "ADD_INDEX",
			targetTable:  "users",
		},
		{
			sql:          "ALTER TABLE orders ADD CONSTRAINT fk_orders_user FOREIGN KEY (user_id) REFERENCES users(id);",
			expectedType: "ADD_CONSTRAINT",
			targetTable:  "orders",
		},
		{
			sql:          "CREATE TABLE customers (id INT);",
			expectedType: "CREATE_TABLE",
			targetTable:  "customers",
		},
		{
			sql:          "ALTER TABLE users ADD COLUMN phone VARCHAR(20);",
			expectedType: "ADD_COLUMN",
			targetTable:  "users",
			targetColumn: "phone",
		},
	}

	for _, tt := range tests {
		ops, err := parser.ParseDiff(tt.sql)
		if err != nil {
			t.Fatalf("ParseDiff failed for %q: %v", tt.sql, err)
		}
		if len(ops) != 1 {
			t.Fatalf("expected 1 operation for %q, got %d", tt.sql, len(ops))
		}
		op := ops[0]
		if op.Type != tt.expectedType {
			t.Errorf("SQL %q -> Type got %q, expected %q", tt.sql, op.Type, tt.expectedType)
		}
		if tt.targetTable != "" && op.TargetTable != tt.targetTable {
			t.Errorf("SQL %q -> Table got %q, expected %q", tt.sql, op.TargetTable, tt.targetTable)
		}
		if tt.targetColumn != "" && op.TargetColumn != tt.targetColumn {
			t.Errorf("SQL %q -> Column got %q, expected %q", tt.sql, op.TargetColumn, tt.targetColumn)
		}
	}
}
