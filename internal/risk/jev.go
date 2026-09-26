package risk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	DefaultTypeSafeEndpoint = "https://api.typesafe.ai/v1/system-one"
	DefaultOpenRouterEndpoint = "https://openrouter.ai/api/v1/chat/completions"
	DefaultJevModel         = "typesafe/jev-1.13"
)

// JevConfig holds configuration for the TypeSafe Jev / OpenRouter classifier.
type JevConfig struct {
	Provider   string        // "typesafe", "openrouter", or "custom"
	Endpoint   string        // API URL
	APIKey     string        // Auth token
	Model      string        // Target model name
	HTTPClient *http.Client // Custom HTTP client
	Timeout    time.Duration // Request timeout
}

// LoadJevConfigFromEnv resolves Jev configuration from environment variables.
func LoadJevConfigFromEnv() *JevConfig {
	provider := strings.ToLower(os.Getenv("BB_JEV_PROVIDER"))
	apiKey := os.Getenv("TYPESAFE_API_KEY")
	endpoint := os.Getenv("BB_JEV_ENDPOINT")
	model := os.Getenv("BB_JEV_MODEL")

	if model == "" {
		model = DefaultJevModel
	}

	if provider == "openrouter" || (apiKey == "" && os.Getenv("OPENROUTER_API_KEY") != "") {
		provider = "openrouter"
		if apiKey == "" {
			apiKey = os.Getenv("OPENROUTER_API_KEY")
		}
		if endpoint == "" {
			endpoint = DefaultOpenRouterEndpoint
		}
	} else if provider == "custom" || endpoint != "" {
		provider = "custom"
		if apiKey == "" {
			apiKey = os.Getenv("BB_JEV_API_KEY")
		}
	} else {
		provider = "typesafe"
		if endpoint == "" {
			endpoint = DefaultTypeSafeEndpoint
		}
	}

	return &JevConfig{
		Provider: provider,
		Endpoint: endpoint,
		APIKey:   apiKey,
		Model:    model,
		Timeout:  5 * time.Second,
	}
}

// JevClassifier implements RiskClassifier using TypeSafe Jev or OpenRouter.
type JevClassifier struct {
	cfg        *JevConfig
	httpClient *http.Client
}

// NewJevClassifier creates a classifier instance with the given configuration.
func NewJevClassifier(cfg *JevConfig) *JevClassifier {
	if cfg == nil {
		cfg = LoadJevConfigFromEnv()
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: cfg.Timeout,
		}
	}
	return &JevClassifier{
		cfg:        cfg,
		httpClient: client,
	}
}

func (j *JevClassifier) Name() string {
	return "jev"
}

// SystemOneRequest represents the payload for TypeSafe System One API.
type SystemOneRequest struct {
	Model     string                 `json:"model"`
	State     map[string]interface{} `json:"state"`
	Questions map[string]interface{} `json:"questions"`
}

// SystemOneChoice defines a multiple-choice question for Jev.
type SystemOneChoice struct {
	Type    string            `json:"type"` // "choice"
	Prompt  string            `json:"prompt"`
	Options map[string]string `json:"options"`
}

// SystemOneNoul defines a boolean calibrated question for Jev.
type SystemOneNoul struct {
	Type   string `json:"type"` // "noul"
	Prompt string `json:"prompt"`
}

// SystemOneResponse holds typed answers from Jev.
type SystemOneResponse struct {
	Answers map[string]struct {
		Value       interface{} `json:"value"`
		Confidence  float64     `json:"confidence"`
		Explanation string      `json:"explanation,omitempty"`
	} `json:"answers"`
	Error string `json:"error,omitempty"`
}

// Classify queries Jev or OpenRouter for a typed migration risk assessment.
func (j *JevClassifier) Classify(ctx context.Context, input ClassificationInput) (*MigrationRisk, error) {
	if j.cfg.APIKey == "" {
		return nil, fmt.Errorf("missing API key for Jev classifier (set TYPESAFE_API_KEY or OPENROUTER_API_KEY)")
	}

	state := map[string]interface{}{
		"migration_sql": input.SQL,
		"table_context": input.TableContext,
		"db_engine":     input.Engine,
	}

	questions := map[string]interface{}{
		"risk_level": SystemOneChoice{
			Type:   "choice",
			Prompt: "What is the overall risk level of this migration?",
			Options: map[string]string{
				"low":      "Safe, reversible, no data loss, no FK impact",
				"medium":   "Reversible with effort or minor lock/FK impact",
				"high":     "Difficult to reverse, heavy lock, or significant FK impact",
				"critical": "Irreversible, data loss, or cascade destruction",
			},
		},
		"is_reversible": SystemOneNoul{
			Type:   "noul",
			Prompt: "Can this migration be safely rolled back without data loss?",
		},
		"is_destructive": SystemOneNoul{
			Type:   "noul",
			Prompt: "Does this migration risk permanent data loss?",
		},
		"touches_fks": SystemOneNoul{
			Type:   "noul",
			Prompt: "Does this migration alter or drop columns referenced by foreign keys?",
		},
		"exclusive_lock": SystemOneNoul{
			Type:   "noul",
			Prompt: "Does this migration acquire heavy table-exclusive locks that block concurrent transactions?",
		},
		"category": SystemOneChoice{
			Type:   "choice",
			Prompt: "What best describes the category of this migration?",
			Options: map[string]string{
				"schema_change":    "Structural change (add/drop/alter column or table)",
				"data_migration":   "Data transformation or backfill",
				"index":            "Index creation or deletion only",
				"constraint":       "Constraint add/drop (FK, unique, check)",
				"docs_or_comments": "Comments, grants, or cosmetic changes only",
			},
		},
	}

	reqBody := SystemOneRequest{
		Model:     j.cfg.Model,
		State:     state,
		Questions: questions,
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Jev request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", j.cfg.Endpoint, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+j.cfg.APIKey)
	req.Header.Set("User-Agent", "BranchBase-RiskGate/1.0")

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request to Jev failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read Jev response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Jev API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var parsedResp SystemOneResponse
	if err := json.Unmarshal(bodyBytes, &parsedResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Jev response: %w", err)
	}

	if parsedResp.Error != "" {
		return nil, fmt.Errorf("Jev error: %s", parsedResp.Error)
	}

	return j.mapResponseToRisk(input, &parsedResp), nil
}

func (j *JevClassifier) mapResponseToRisk(input ClassificationInput, resp *SystemOneResponse) *MigrationRisk {
	level := RiskLow
	confidence := 0.80
	reversible := true
	destructive := false
	touchesFKs := false
	exclusiveLock := false
	category := CategorySchemaChange
	var flags []string
	var details []string

	if a, ok := resp.Answers["risk_level"]; ok {
		if valStr, ok := a.Value.(string); ok {
			level = ParseRiskLevel(valStr)
		}
		if a.Confidence > 0 {
			confidence = a.Confidence
		}
		if a.Explanation != "" {
			details = append(details, a.Explanation)
		}
	}

	if a, ok := resp.Answers["is_reversible"]; ok {
		if b, ok := a.Value.(bool); ok {
			reversible = b
		}
	}

	if a, ok := resp.Answers["is_destructive"]; ok {
		if b, ok := a.Value.(bool); ok {
			destructive = b
			if destructive {
				flags = append(flags, "POTENTIAL_DATA_LOSS")
			}
		}
	}

	if a, ok := resp.Answers["touches_fks"]; ok {
		if b, ok := a.Value.(bool); ok {
			touchesFKs = b
			if touchesFKs {
				flags = append(flags, "TOUCHES_FOREIGN_KEYS")
			}
		}
	}

	if a, ok := resp.Answers["exclusive_lock"]; ok {
		if b, ok := a.Value.(bool); ok {
			exclusiveLock = b
			if exclusiveLock {
				flags = append(flags, "ACCESS_EXCLUSIVE_LOCK")
			}
		}
	}

	if a, ok := resp.Answers["category"]; ok {
		if valStr, ok := a.Value.(string); ok {
			category = ParseMigCategory(valStr)
		}
	}

	return &MigrationRisk{
		File:          input.File,
		Level:         level,
		Reversible:    reversible,
		Destructive:   destructive,
		TouchesFKs:    touchesFKs,
		ExclusiveLock: exclusiveLock,
		Category:      category,
		Confidence:    confidence,
		Flags:         uniqueStrings(flags),
		Details:       strings.Join(details, " "),
		Operations:    input.Operations,
		AnalyzedAt:    time.Now(),
		Engine:        input.Engine,
		Source:        "jev",
	}
}
