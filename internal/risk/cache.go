package risk

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CacheEntry wraps a cached migration risk assessment with metadata.
type CacheEntry struct {
	Risk      MigrationRisk `json:"risk"`
	CachedAt  time.Time     `json:"cached_at"`
	CacheKey  string        `json:"cache_key"`
}

// RiskCache provides a thread-safe, persisted risk evaluation cache.
type RiskCache struct {
	mu       sync.RWMutex
	filePath string
	entries  map[string]CacheEntry
}

// ComputeCacheKey generates a deterministic SHA256 key from SQL content and table schema signature.
func ComputeCacheKey(migrationSQL string, tableSchemaSignature string) string {
	hasher := sha256.New()
	hasher.Write([]byte(migrationSQL))
	hasher.Write([]byte("::"))
	hasher.Write([]byte(tableSchemaSignature))
	return hex.EncodeToString(hasher.Sum(nil))
}

// NewRiskCache initializes or loads an existing risk cache from the repo root.
func NewRiskCache(repoRoot string) (*RiskCache, error) {
	cacheDir := filepath.Join(repoRoot, ".branchbase", "cache")
	filePath := filepath.Join(cacheDir, "risk_cache.json")

	c := &RiskCache{
		filePath: filePath,
		entries:  make(map[string]CacheEntry),
	}

	if err := c.load(); err != nil && !os.IsNotExist(err) {
		// Log or tolerate corrupt cache, reinitializing empty
		c.entries = make(map[string]CacheEntry)
	}

	return c, nil
}

// load reads cache entries from disk.
func (c *RiskCache) load() error {
	data, err := os.ReadFile(c.filePath)
	if err != nil {
		return err
	}

	var diskEntries map[string]CacheEntry
	if err := json.Unmarshal(data, &diskEntries); err != nil {
		return err
	}

	c.entries = diskEntries
	return nil
}

// Get retrieves a cached MigrationRisk if present.
func (c *RiskCache) Get(key string) (*MigrationRisk, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		return nil, false
	}

	r := entry.Risk
	r.Source = "cache"
	return &r, true
}

// Set stores a MigrationRisk under the specified key and persists it to disk.
func (c *RiskCache) Set(key string, risk *MigrationRisk) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if risk == nil {
		return fmt.Errorf("cannot cache nil risk")
	}

	c.entries[key] = CacheEntry{
		Risk:     *risk,
		CachedAt: time.Now(),
		CacheKey: key,
	}

	return c.persist()
}

// persist writes the cache entries atomically to disk.
func (c *RiskCache) persist() error {
	cacheDir := filepath.Dir(c.filePath)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory %s: %w", cacheDir, err)
	}

	data, err := json.MarshalIndent(c.entries, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode risk cache JSON: %w", err)
	}

	tmpFile := c.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary risk cache file: %w", err)
	}

	if err := os.Rename(tmpFile, c.filePath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to commit risk cache file: %w", err)
	}

	return nil
}

// Clear flushes all cached entries.
func (c *RiskCache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]CacheEntry)
	_ = os.Remove(c.filePath)
	return nil
}
