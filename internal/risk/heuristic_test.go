package risk

import (
	"context"
	"testing"
)

func TestHeuristicClassifier(t *testing.T) {
	classifier := NewHeuristicClassifier()
	ctx := context.Background()

	t.Run("Drop table is critical and non-reversible", func(t *testing.T) {
		input := ClassificationInput{
			File:   "001_drop_table.sql",
			SQL:    "DROP TABLE legacy_users;",
			Engine: "postgres",
			Operations: []DDLOperation{
				{Type: "DROP_TABLE", TargetTable: "legacy_users", RawSQL: "DROP TABLE legacy_users;"},
			},
		}

		risk, err := classifier.Classify(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if risk.Level != RiskCritical {
			t.Errorf("expected CRITICAL, got %v", risk.Level)
		}
		if !risk.Destructive {
			t.Errorf("expected destructive to be true")
		}
		if risk.Reversible {
			t.Errorf("expected reversible to be false")
		}
		if !risk.ExclusiveLock {
			t.Errorf("expected ExclusiveLock to be true")
		}
	})

	t.Run("Drop column referencing FK is critical and touches FKs", func(t *testing.T) {
		input := ClassificationInput{
			File:   "002_drop_col.sql",
			SQL:    "ALTER TABLE users DROP COLUMN email;",
			Engine: "postgres",
			TableMetadata: &TableMetadata{
				TableName: "users",
				InboundFKs: []ForeignKeyRef{
					{ConstraintName: "fk_orders_user_email", FromTable: "orders", FromColumn: "user_email", ToTable: "users", ToColumn: "email"},
				},
			},
			Operations: []DDLOperation{
				{Type: "DROP_COLUMN", TargetTable: "users", TargetColumn: "email", RawSQL: "ALTER TABLE users DROP COLUMN email;"},
			},
		}

		risk, err := classifier.Classify(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if risk.Level != RiskCritical {
			t.Errorf("expected CRITICAL due to FK reference, got %v", risk.Level)
		}
		if !risk.TouchesFKs {
			t.Errorf("expected TouchesFKs to be true")
		}
	})

	t.Run("Add column with NOT NULL without default is high risk", func(t *testing.T) {
		input := ClassificationInput{
			File:   "003_add_col.sql",
			SQL:    "ALTER TABLE users ADD COLUMN age INT NOT NULL;",
			Engine: "postgres",
			Operations: []DDLOperation{
				{Type: "ADD_COLUMN", TargetTable: "users", TargetColumn: "age", RawSQL: "ALTER TABLE users ADD COLUMN age INT NOT NULL;"},
			},
		}

		risk, err := classifier.Classify(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if risk.Level != RiskHigh {
			t.Errorf("expected HIGH for NOT NULL column without default, got %v", risk.Level)
		}
	})

	t.Run("Create table is low risk and reversible", func(t *testing.T) {
		input := ClassificationInput{
			File:   "004_create_table.sql",
			SQL:    "CREATE TABLE logs (id SERIAL PRIMARY KEY, msg TEXT);",
			Engine: "postgres",
			Operations: []DDLOperation{
				{Type: "CREATE_TABLE", TargetTable: "logs", RawSQL: "CREATE TABLE logs (id SERIAL PRIMARY KEY, msg TEXT);"},
			},
		}

		risk, err := classifier.Classify(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if risk.Level != RiskLow {
			t.Errorf("expected LOW for CREATE TABLE, got %v", risk.Level)
		}
		if !risk.Reversible {
			t.Errorf("expected CREATE TABLE to be reversible")
		}
	})
}
