package test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/branchbase/branchbase/internal/risk"
)

func TestRiskGateIntegration(t *testing.T) {
	ctx := context.Background()
	fixturesDir := filepath.Join("fixtures", "migrations")

	mockIntrospect := risk.NewMockIntrospector()
	mockIntrospect.Tables["users"] = &risk.TableMetadata{
		TableName: "users",
		Columns:   []string{"id", "email", "name", "created_at"},
		InboundFKs: []risk.ForeignKeyRef{
			{ConstraintName: "fk_orders_user", FromTable: "orders", FromColumn: "user_email", ToTable: "users", ToColumn: "email"},
		},
	}

	analyzer, err := risk.NewRiskAnalyzer(risk.AnalyzeOptions{
		RepoRoot:     ".",
		Engine:       "postgres",
		Offline:      true,
		Classifier:   risk.NewHeuristicClassifier(),
		Introspector: mockIntrospect,
	})
	if err != nil {
		t.Fatalf("failed to create analyzer: %v", err)
	}

	t.Run("001_safe_add_column.sql is LOW risk", func(t *testing.T) {
		path := filepath.Join(fixturesDir, "001_safe_add_column.sql")
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read fixture %s: %v", path, err)
		}

		res, err := analyzer.AnalyzeSQL(ctx, path, string(content))
		if err != nil {
			t.Fatalf("AnalyzeSQL failed: %v", err)
		}

		if res.Level != risk.RiskLow {
			t.Errorf("expected LOW risk, got %v", res.Level)
		}
		if res.Destructive {
			t.Errorf("expected non-destructive")
		}
	})

	t.Run("002_drop_referenced_fk.sql is CRITICAL risk due to inbound FK", func(t *testing.T) {
		path := filepath.Join(fixturesDir, "002_drop_referenced_fk.sql")
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read fixture %s: %v", path, err)
		}

		res, err := analyzer.AnalyzeSQL(ctx, path, string(content))
		if err != nil {
			t.Fatalf("AnalyzeSQL failed: %v", err)
		}

		if res.Level != risk.RiskCritical {
			t.Errorf("expected CRITICAL risk, got %v", res.Level)
		}
		if !res.TouchesFKs {
			t.Errorf("expected TouchesFKs to be true")
		}
		if !res.Destructive {
			t.Errorf("expected destructive=true")
		}
	})

	t.Run("003_drop_table_critical.sql is CRITICAL risk and blocks policy", func(t *testing.T) {
		path := filepath.Join(fixturesDir, "003_drop_table_critical.sql")
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read fixture %s: %v", path, err)
		}

		res, err := analyzer.AnalyzeSQL(ctx, path, string(content))
		if err != nil {
			t.Fatalf("AnalyzeSQL failed: %v", err)
		}

		if res.Level != risk.RiskCritical {
			t.Errorf("expected CRITICAL risk, got %v", res.Level)
		}

		report := analyzer.EvaluateReport([]risk.MigrationRisk{*res}, "feature/drop-legacy")
		if !report.Blocked {
			t.Errorf("expected report.Blocked to be true")
		}
	})

	t.Run("004_mysql_change_col.sql classifies ALTER_TYPE", func(t *testing.T) {
		path := filepath.Join(fixturesDir, "004_mysql_change_col.sql")
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read fixture %s: %v", path, err)
		}

		res, err := analyzer.AnalyzeSQL(ctx, path, string(content))
		if err != nil {
			t.Fatalf("AnalyzeSQL failed: %v", err)
		}

		if res.Level != risk.RiskHigh {
			t.Errorf("expected HIGH risk for column type change, got %v", res.Level)
		}
	})
}
