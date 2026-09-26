package risk

import (
	"bytes"
	"strings"
	"testing"
)

func TestPromptConfirmation(t *testing.T) {
	t.Run("User answers yes", func(t *testing.T) {
		in := strings.NewReader("y\n")
		var out bytes.Buffer

		confirmed, err := PromptConfirmation(in, &out, "Proceed?")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !confirmed {
			t.Errorf("expected confirmed=true for 'y'")
		}
	})

	t.Run("User answers no", func(t *testing.T) {
		in := strings.NewReader("N\n")
		var out bytes.Buffer

		confirmed, err := PromptConfirmation(in, &out, "Proceed?")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if confirmed {
			t.Errorf("expected confirmed=false for 'N'")
		}
	})

	t.Run("User types empty response (defaults to no)", func(t *testing.T) {
		in := strings.NewReader("\n")
		var out bytes.Buffer

		confirmed, err := PromptConfirmation(in, &out, "Proceed?")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if confirmed {
			t.Errorf("expected confirmed=false for empty response")
		}
	})
}
