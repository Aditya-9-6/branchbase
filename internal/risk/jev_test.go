package risk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJevClassifier(t *testing.T) {
	ctx := context.Background()

	t.Run("Successful Jev classification mapping", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer test-key" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			resp := SystemOneResponse{
				Answers: map[string]struct {
					Value       interface{} `json:"value"`
					Confidence  float64     `json:"confidence"`
					Explanation string      `json:"explanation,omitempty"`
				}{
					"risk_level": {
						Value:       "CRITICAL",
						Confidence:  0.94,
						Explanation: "Column is referenced by foreign keys.",
					},
					"is_reversible": {
						Value:      false,
						Confidence: 0.99,
					},
					"is_destructive": {
						Value:      true,
						Confidence: 0.98,
					},
					"touches_fks": {
						Value:      true,
						Confidence: 0.95,
					},
					"exclusive_lock": {
						Value:      true,
						Confidence: 0.92,
					},
					"category": {
						Value:      "schema_change",
						Confidence: 0.99,
					},
				},
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		classifier := NewJevClassifier(&JevConfig{
			Provider: "typesafe",
			Endpoint: server.URL,
			APIKey:   "test-key",
			Model:    "typesafe/jev-1.13",
		})

		input := ClassificationInput{
			File:         "003_drop_email.sql",
			SQL:          "ALTER TABLE users DROP COLUMN email;",
			Engine:       "postgres",
			TableContext: "users (id, name, email) — 3 FK references",
			Operations: []DDLOperation{
				{Type: "DROP_COLUMN", TargetTable: "users", TargetColumn: "email"},
			},
		}

		risk, err := classifier.Classify(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if risk.Level != RiskCritical {
			t.Errorf("expected level CRITICAL, got %s", risk.Level)
		}
		if risk.Confidence != 0.94 {
			t.Errorf("expected confidence 0.94, got %f", risk.Confidence)
		}
		if !risk.Destructive {
			t.Errorf("expected destructive=true")
		}
		if risk.Reversible {
			t.Errorf("expected reversible=false")
		}
		if !risk.TouchesFKs {
			t.Errorf("expected touches_fks=true")
		}
		if !risk.ExclusiveLock {
			t.Errorf("expected exclusive_lock=true")
		}
	})

	t.Run("API error returns structured error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		}))
		defer server.Close()

		classifier := NewJevClassifier(&JevConfig{
			Endpoint: server.URL,
			APIKey:   "test-key",
		})

		_, err := classifier.Classify(ctx, ClassificationInput{File: "test.sql", SQL: "SELECT 1"})
		if err == nil {
			t.Errorf("expected error on HTTP 429, got nil")
		}
	})
}
