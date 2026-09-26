package risk

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// PolicyDecision represents the evaluated action for a migration assessment.
type PolicyDecision struct {
	Action      PolicyAction `json:"action"`
	EffectiveLevel RiskLevel `json:"effective_level"`
	Reason      string       `json:"reason"`
	Blocked     bool         `json:"blocked"`
	RequiresConfirmation bool `json:"requires_confirmation"`
}

// RiskPolicy holds the rules from .branchbase/risk-policy.yml.
type RiskPolicy struct {
	Version             int                     `yaml:"version"`
	OnRisk              map[string]string       `yaml:"on_risk"`
	ConfidenceThreshold float64                 `yaml:"confidence_threshold"`
	OnJevUnavailable    string                  `yaml:"on_jev_unavailable"`
	AlwaysCritical      []string                `yaml:"always_critical"`
	ExcludePaths        []string                `yaml:"exclude_paths"`
	ReportFormat        string                  `yaml:"report_format"`
	SaveReport          string                  `yaml:"save_report"`
}

// DefaultPolicy returns safe, standard defaults for the risk policy.
func DefaultPolicy() *RiskPolicy {
	return &RiskPolicy{
		Version: 1,
		OnRisk: map[string]string{
			string(RiskLow):      string(ActionAllow),
			string(RiskMedium):   string(ActionWarn),
			string(RiskHigh):     string(ActionConfirm),
			string(RiskCritical): string(ActionBlock),
		},
		ConfidenceThreshold: 0.75,
		OnJevUnavailable:    string(ActionWarn),
		AlwaysCritical: []string{
			"DROP TABLE",
			"TRUNCATE",
			"DROP DATABASE",
			"DROP SCHEMA",
		},
		ExcludePaths: []string{
			"migrations/seeds/**",
			"migrations/test_data/**",
		},
		ReportFormat: "pretty",
		SaveReport:   ".branchbase/last-risk-report.json",
	}
}

// LoadPolicy attempts to read .branchbase/risk-policy.yml from repoRoot or custom path.
// If the file does not exist, it returns the DefaultPolicy without error.
func LoadPolicy(repoRoot, customPath string) (*RiskPolicy, error) {
	targetPath := customPath
	if targetPath == "" {
		targetPath = filepath.Join(repoRoot, ".branchbase", "risk-policy.yml")
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultPolicy(), nil
		}
		return nil, fmt.Errorf("failed to read risk policy at %s: %w", targetPath, err)
	}

	policy := DefaultPolicy()
	if err := yaml.Unmarshal(data, policy); err != nil {
		return nil, fmt.Errorf("invalid YAML in risk policy %s: %w", targetPath, err)
	}

	if err := policy.Validate(); err != nil {
		return nil, fmt.Errorf("invalid risk policy configuration: %w", err)
	}

	return policy, nil
}

// Validate ensures all actions and thresholds in the policy are valid.
func (p *RiskPolicy) Validate() error {
	if p.ConfidenceThreshold < 0 || p.ConfidenceThreshold > 1.0 {
		return fmt.Errorf("confidence_threshold must be between 0.0 and 1.0, got %f", p.ConfidenceThreshold)
	}

	validActions := map[string]bool{
		string(ActionAllow):   true,
		string(ActionWarn):    true,
		string(ActionConfirm): true,
		string(ActionBlock):   true,
	}

	for lvl, act := range p.OnRisk {
		if !validActions[strings.ToLower(act)] {
			return fmt.Errorf("invalid action %q for risk level %q", act, lvl)
		}
	}

	if p.OnJevUnavailable != "" && !validActions[strings.ToLower(p.OnJevUnavailable)] {
		return fmt.Errorf("invalid on_jev_unavailable action %q", p.OnJevUnavailable)
	}

	return nil
}

// IsPathExcluded checks if a file path matches any exclusion glob patterns.
func (p *RiskPolicy) IsPathExcluded(filePath string) bool {
	cleanPath := filepath.ToSlash(filePath)
	for _, pattern := range p.ExcludePaths {
		cleanPattern := filepath.ToSlash(pattern)
		// Match exact prefix for glob trailing **
		if strings.HasSuffix(cleanPattern, "/**") {
			prefix := strings.TrimSuffix(cleanPattern, "/**")
			if strings.HasPrefix(cleanPath, prefix) {
				return true
			}
		}
		if matched, _ := filepath.Match(cleanPattern, cleanPath); matched {
			return true
		}
	}
	return false
}

// MatchesAlwaysCritical checks if raw SQL contains any always-critical pattern.
func (p *RiskPolicy) MatchesAlwaysCritical(sql string) (bool, string) {
	upperSQL := strings.ToUpper(sql)
	for _, pattern := range p.AlwaysCritical {
		if strings.Contains(upperSQL, strings.ToUpper(pattern)) {
			return true, pattern
		}
	}
	return false, ""
}

// Evaluate determines the policy action for a given migration risk assessment.
func (p *RiskPolicy) Evaluate(risk *MigrationRisk) PolicyDecision {
	effectiveLevel := risk.Level
	reason := fmt.Sprintf("Evaluated level %s", risk.Level)

	// Step 1: Check confidence threshold escalation
	if risk.Confidence > 0 && risk.Confidence < p.ConfidenceThreshold {
		oldLevel := effectiveLevel
		effectiveLevel = effectiveLevel.Escalate()
		reason = fmt.Sprintf("Confidence (%.0f%%) below threshold (%.0f%%); escalated %s -> %s",
			risk.Confidence*100, p.ConfidenceThreshold*100, oldLevel, effectiveLevel)
	}

	// Step 2: Resolve configured action for effective level
	actionStr, exists := p.OnRisk[string(effectiveLevel)]
	if !exists {
		// Fallback defaults
		switch effectiveLevel {
		case RiskLow:
			actionStr = string(ActionAllow)
		case RiskMedium:
			actionStr = string(ActionWarn)
		case RiskHigh:
			actionStr = string(ActionConfirm)
		case RiskCritical:
			actionStr = string(ActionBlock)
		default:
			actionStr = string(ActionWarn)
		}
	}

	action, _ := ParsePolicyAction(actionStr)

	return PolicyDecision{
		Action:               action,
		EffectiveLevel:       effectiveLevel,
		Reason:               reason,
		Blocked:              action == ActionBlock,
		RequiresConfirmation: action == ActionConfirm,
	}
}
