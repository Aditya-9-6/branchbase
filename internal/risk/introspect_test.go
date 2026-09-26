package risk

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPostgresIntrospector(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	introspector := NewPostgresIntrospector(db)

	// Mock column query
	mock.ExpectQuery(`SELECT column_name FROM information_schema\.columns WHERE table_name = \$1`).
		WithArgs("users").
		WillReturnRows(sqlmock.NewRows([]string{"column_name"}).
			AddRow("id").
			AddRow("name").
			AddRow("email").
			AddRow("created_at"))

	// Mock inbound FK query
	mock.ExpectQuery(`SELECT tc\.constraint_name.*FROM information_schema\.table_constraints.*WHERE tc\.constraint_type = 'FOREIGN KEY' AND ccu\.table_name = \$1`).
		WithArgs("users").
		WillReturnRows(sqlmock.NewRows([]string{"constraint_name", "from_table", "from_column", "to_table", "to_column"}).
			AddRow("fk_orders_user", "orders", "user_id", "users", "id").
			AddRow("fk_logs_user", "audit_logs", "actor_id", "users", "id"))

	// Mock outbound FK query
	mock.ExpectQuery(`SELECT tc\.constraint_name.*FROM information_schema\.table_constraints.*WHERE tc\.constraint_type = 'FOREIGN KEY' AND tc\.table_name = \$1`).
		WithArgs("users").
		WillReturnRows(sqlmock.NewRows([]string{"constraint_name", "from_table", "from_column", "to_table", "to_column"}))

	ctx := context.Background()
	meta, err := introspector.IntrospectTable(ctx, "users")
	if err != nil {
		t.Fatalf("IntrospectTable failed: %v", err)
	}

	if len(meta.Columns) != 4 {
		t.Errorf("expected 4 columns, got %d", len(meta.Columns))
	}
	if len(meta.InboundFKs) != 2 {
		t.Errorf("expected 2 inbound FKs, got %d", len(meta.InboundFKs))
	}

	sig := FormatSchemaSignature(meta)
	if sig == "" {
		t.Errorf("expected non-empty schema signature")
	}

	promptCtx := FormatPromptContext(meta)
	if promptCtx == "" {
		t.Errorf("expected non-empty prompt context")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

func TestMockIntrospector(t *testing.T) {
	mock := NewMockIntrospector()
	mock.Tables["users"] = &TableMetadata{
		TableName: "users",
		Columns:   []string{"id", "email"},
		InboundFKs: []ForeignKeyRef{
			{ConstraintName: "fk_orders", FromTable: "orders", FromColumn: "user_id", ToTable: "users", ToColumn: "id"},
		},
	}

	ctx := context.Background()
	meta, err := mock.IntrospectTable(ctx, "users")
	if err != nil {
		t.Fatalf("mock introspect failed: %v", err)
	}
	if len(meta.InboundFKs) != 1 {
		t.Errorf("expected 1 inbound FK, got %d", len(meta.InboundFKs))
	}
}
