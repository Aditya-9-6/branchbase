package risk

import (
	"testing"
)

func TestParseRiskLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected RiskLevel
	}{
		{"LOW", RiskLow},
		{"low", RiskLow},
		{"MEDIUM", RiskMedium},
		{"med", RiskMedium},
		{"HIGH", RiskHigh},
		{"CRITICAL", RiskCritical},
		{"crit", RiskCritical},
		{"invalid", RiskUnknown},
	}

	for _, tt := range tests {
		res := ParseRiskLevel(tt.input)
		if res != tt.expected {
			t.Errorf("ParseRiskLevel(%q) = %v, expected %v", tt.input, res, tt.expected)
		}
	}
}

func TestRiskLevelEscalation(t *testing.T) {
	if RiskLow.Escalate() != RiskMedium {
		t.Errorf("expected LOW to escalate to MEDIUM, got %v", RiskLow.Escalate())
	}
	if RiskMedium.Escalate() != RiskHigh {
		t.Errorf("expected MEDIUM to escalate to HIGH, got %v", RiskMedium.Escalate())
	}
	if RiskHigh.Escalate() != RiskCritical {
		t.Errorf("expected HIGH to escalate to CRITICAL, got %v", RiskHigh.Escalate())
	}
	if RiskCritical.Escalate() != RiskCritical {
		t.Errorf("expected CRITICAL to remain CRITICAL, got %v", RiskCritical.Escalate())
	}
}

func TestMigrationRiskSummary(t *testing.T) {
	risk := MigrationRisk{
		File:          "migrations/001_drop.sql",
		Level:         RiskCritical,
		Confidence:    0.95,
		Category:      CategorySchemaChange,
		Flags:         []string{"DROP COLUMN", "CASCADE"},
		ExclusiveLock: true,
	}

	sum := risk.Summary()
	if sum == "" {
		t.Errorf("expected non-empty summary")
	}
}
