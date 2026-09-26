package risk

import (
	"context"
)

// ClassificationInput supplies full syntactic and catalog context to a classifier.
type ClassificationInput struct {
	File          string          `json:"file"`
	SQL           string          `json:"sql"`
	Engine        string          `json:"engine"`
	TableContext  string          `json:"table_context"`
	TableMetadata *TableMetadata  `json:"table_metadata,omitempty"`
	Operations    []DDLOperation  `json:"operations"`
}

// RiskClassifier provides an abstraction for evaluating migration danger profiles.
type RiskClassifier interface {
	// Name returns the identifier of the classifier engine (e.g. "heuristic", "jev")
	Name() string

	// Classify analyzes the input migration and produces a typed MigrationRisk assessment.
	Classify(ctx context.Context, input ClassificationInput) (*MigrationRisk, error)
}
