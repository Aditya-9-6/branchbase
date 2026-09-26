package risk

import (
	"testing"
	"time"
)

func TestRiskCache(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewRiskCache(tmpDir)
	if err != nil {
		t.Fatalf("failed to create risk cache: %v", err)
	}

	key := ComputeCacheKey("ALTER TABLE users DROP COLUMN email;", "table:users;cols:email,id,name")

	// Verify cache miss
	if _, found := cache.Get(key); found {
		t.Errorf("expected cache miss initially")
	}

	// Store risk
	risk := &MigrationRisk{
		File:          "001_drop_email.sql",
		Level:         RiskCritical,
		Confidence:    0.96,
		Category:      CategorySchemaChange,
		TouchesFKs:    true,
		ExclusiveLock: true,
		AnalyzedAt:    time.Now(),
		Source:        "jev",
	}

	if err := cache.Set(key, risk); err != nil {
		t.Fatalf("failed to cache risk: %v", err)
	}

	// Verify cache hit
	cached, found := cache.Get(key)
	if !found {
		t.Fatalf("expected cache hit")
	}

	if cached.Level != RiskCritical || !cached.TouchesFKs || cached.Source != "cache" {
		t.Errorf("cached risk mismatch: %+v", cached)
	}

	// Reload from disk into fresh cache instance
	cache2, err := NewRiskCache(tmpDir)
	if err != nil {
		t.Fatalf("failed to reload risk cache: %v", err)
	}

	cached2, found2 := cache2.Get(key)
	if !found2 || cached2.Level != RiskCritical {
		t.Errorf("reloaded cache failed to retrieve entry: found=%v, risk=%+v", found2, cached2)
	}
}
