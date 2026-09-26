package risk

import (
	"fmt"
	"strings"
	"time"
)

// RiskLevel classifies the danger profile of a database migration.
type RiskLevel string

const (
	RiskLow      RiskLevel = "LOW"
	RiskMedium   RiskLevel = "MEDIUM"
	RiskHigh     RiskLevel = "HIGH"
	RiskCritical RiskLevel = "CRITICAL"
	RiskUnknown  RiskLevel = "UNKNOWN"
)

// ParseRiskLevel parses a string into a RiskLevel enum.
func ParseRiskLevel(s string) RiskLevel {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "LOW":
		return RiskLow
	case "MEDIUM", "MED":
		return RiskMedium
	case "HIGH":
		return RiskHigh
	case "CRITICAL", "CRIT":
		return RiskCritical
	default:
		return RiskUnknown
	}
}

// Order returns the severity rank of the risk level (higher = more dangerous).
func (r RiskLevel) Order() int {
	switch r {
	case RiskLow:
		return 1
	case RiskMedium:
		return 2
	case RiskHigh:
		return 3
	case RiskCritical:
		return 4
	default:
		return 0
	}
}

// Escalate returns the next severity level if possible.
func (r RiskLevel) Escalate() RiskLevel {
	switch r {
	case RiskLow:
		return RiskMedium
	case RiskMedium:
		return RiskHigh
	case RiskHigh:
		return RiskCritical
	default:
		return RiskCritical
	}
}

// PolicyAction defines what the CLI should do when encountering a given risk level.
type PolicyAction string

const (
	ActionAllow   PolicyAction = "allow"
	ActionWarn    PolicyAction = "warn"
	ActionConfirm PolicyAction = "confirm"
	ActionBlock   PolicyAction = "block"
)

// ParsePolicyAction parses a string into a PolicyAction enum.
func ParsePolicyAction(s string) (PolicyAction, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "allow":
		return ActionAllow, nil
	case "warn":
		return ActionWarn, nil
	case "confirm":
		return ActionConfirm, nil
	case "block":
		return ActionBlock, nil
	default:
		return "", fmt.Errorf("invalid policy action %q (must be allow, warn, confirm, or block)", s)
	}
}

// MigCategory categorizes the structural intent of a migration.
type MigCategory string

const (
	CategorySchemaChange   MigCategory = "schema_change"
	CategoryDataMigration  MigCategory = "data_migration"
	CategoryIndex          MigCategory = "index"
	CategoryConstraint     MigCategory = "constraint"
	CategoryDocsOrComments MigCategory = "docs_or_comments"
)

// ParseMigCategory parses a string into a MigCategory.
func ParseMigCategory(s string) MigCategory {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "schema_change", "structural", "ddl":
		return CategorySchemaChange
	case "data_migration", "data", "backfill":
		return CategoryDataMigration
	case "index", "indexes":
		return CategoryIndex
	case "constraint", "constraints", "foreign_key":
		return CategoryConstraint
	case "docs", "comments", "docs_or_comments":
		return CategoryDocsOrComments
	default:
		return CategorySchemaChange
	}
}

// DDLOperation represents a single parsed database modification statement.
type DDLOperation struct {
	Type         string `json:"type"`          // e.g. "DROP_TABLE", "DROP_COLUMN", "ALTER_TYPE", "ADD_INDEX", "TRUNCATE"
	TargetTable  string `json:"target_table"`  // Target table name
	TargetColumn string `json:"target_column"` // Target column name (if applicable)
	RawSQL       string `json:"raw_sql"`       // Snippet of the SQL statement
	LineNumber   int    `json:"line_number"`   // Line number in original file
}

// ForeignKeyRef describes an inbound foreign key reference pointing to a table/column.
type ForeignKeyRef struct {
	ConstraintName string `json:"constraint_name"`
	FromTable      string `json:"from_table"`
	FromColumn     string `json:"from_column"`
	ToTable        string `json:"to_table"`
	ToColumn       string `json:"to_column"`
}

// TableMetadata holds introspected catalog state for a database table.
type TableMetadata struct {
	TableName        string          `json:"table_name"`
	Columns          []string        `json:"columns"`
	InboundFKs       []ForeignKeyRef `json:"inbound_fks"`        // Other tables referencing this table
	OutboundFKs      []ForeignKeyRef `json:"outbound_fks"`       // This table referencing other tables
	RowCountEstimate int64           `json:"row_count_estimate"` // Estimated row count if available
}

// MigrationRisk represents the complete risk assessment for a migration file or diff.
type MigrationRisk struct {
	File          string        `json:"file"`
	Level         RiskLevel     `json:"level"`
	Reversible    bool          `json:"reversible"`
	Destructive   bool          `json:"destructive"`
	TouchesFKs    bool          `json:"touches_fks"`
	ExclusiveLock bool          `json:"exclusive_lock"` // Flag for ACCESS EXCLUSIVE / long-running locks
	Category      MigCategory   `json:"category"`
	Confidence    float64       `json:"confidence"`
	Flags         []string      `json:"flags"`
	Suggestion    string        `json:"suggestion,omitempty"`
	Details       string        `json:"details,omitempty"`
	Operations    []DDLOperation `json:"operations,omitempty"`
	AnalyzedAt    time.Time     `json:"analyzed_at"`
	Engine        string        `json:"engine"`
	Source        string        `json:"source"` // "jev", "heuristic", or "cache"
}

// Summary returns a concise one-line textual representation of the risk.
func (r *MigrationRisk) Summary() string {
	flags := "none"
	if len(r.Flags) > 0 {
		flags = strings.Join(r.Flags, ", ")
	}
	return fmt.Sprintf("[%s] Level: %s (confidence: %.0f%%) | Category: %s | Flags: %s",
		r.File, r.Level, r.Confidence*100, r.Category, flags)
}
