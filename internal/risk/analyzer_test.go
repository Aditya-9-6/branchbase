package risk

import (
	"context"
	"testing"
)

func TestRiskAnalyzer(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	mockIntrospect := NewMockIntrospector()
	mockIntrospect.Tables["users"] = &TableMetadata{
		TableName: "users",
		Columns:   []string{"id", "email", "name"},
		InboundFKs: []ForeignKeyRef{
			{ConstraintName: "fk_orders_user", FromTable: "orders", FromColumn: "user_email", ToTable: "users", ToColumn: "email"},
		},
	}

	cache, err := NewRiskCache(tmpDir)
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	analyzer, err := NewRiskAnalyzer(AnalyzeOptions{
		RepoRoot:     tmpDir,
		Engine:       "postgres",
		Offline:      true,
		Classifier:   NewHeuristicClassifier(),
		Introspector: mockIntrospect,
		Cache:        cache,
	})
	if err != nil {
		t.Fatalf("failed to create analyzer: %v", err)
	}

	// 1. Analyze safe migration
	safeSQL := "CREATE TABLE products (id SERIAL PRIMARY KEY, title TEXT);"
	riskSafe, err := analyzer.AnalyzeSQL(ctx, "001_create_products.sql", safeSQL)
	if err != nil {
		t.Fatalf("failed to analyze safe SQL: %v", err)
	}
	if riskSafe.Level != RiskLow {
		t.Errorf("expected LOW for CREATE TABLE, got %v", riskSafe.Level)
	}

	// 2. Analyze destructive FK-referencing migration
	critSQL := "ALTER TABLE users DROP COLUMN email;"
	riskCrit, err := analyzer.AnalyzeSQL(ctx, "002_drop_email.sql", critSQL)
	if err != nil {
		t.Fatalf("failed to analyze critical SQL: %v", err)
	}
	if riskCrit.Level != RiskCritical {
		t.Errorf("expected CRITICAL for dropping FK referenced column, got %v", riskCrit.Level)
	}
	if !riskCrit.TouchesFKs {
		t.Errorf("expected TouchesFKs to be true")
	}

	// 3. Test caching behavior: second call should retrieve from cache
	riskCached, err := analyzer.AnalyzeSQL(ctx, "002_drop_email.sql", critSQL)
	if err != nil {
		t.Fatalf("failed to analyze cached SQL: %v", err)
	}
	if riskCached.Source != "cache" {
		t.Errorf("expected source 'cache', got %s", riskCached.Source)
	}

	// 4. Test aggregated report evaluation
	report := analyzer.EvaluateReport([]MigrationRisk{*riskSafe, *riskCrit}, "feature/drop-email")
	if report.OverallLevel != RiskCritical {
		t.Errorf("expected overall level CRITICAL, got %s", report.OverallLevel)
	}
	if report.OverallAction != ActionBlock || !report.Blocked {
		t.Errorf("expected overall action BLOCK, got %s (blocked=%v)", report.OverallAction, report.Blocked)
	}

	// 5. Test pretty formatting
	pretty := FormatPrettyTerminal(report)
	if len(pretty) == 0 {
		t.Errorf("expected non-empty pretty terminal report")
	}

	// 6. Test github annotations formatting
	annotations := FormatGitHubAnnotations(report)
	if len(annotations) == 0 {
		t.Errorf("expected non-empty github annotations")
	}
}
