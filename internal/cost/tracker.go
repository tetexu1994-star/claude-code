package cost

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// UsageRecord is a single LLM call record.
type UsageRecord struct {
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	CostUSD      float64   `json:"cost_usd"`
	Timestamp    time.Time `json:"timestamp"`
}

// ProviderStats holds cumulative statistics for a provider+model pair.
type ProviderStats struct {
	TotalInputTokens  int     `json:"total_input_tokens"`
	TotalOutputTokens int     `json:"total_output_tokens"`
	TotalCostUSD      float64 `json:"total_cost_usd"`
	TotalCalls        int     `json:"total_calls"`
}

// Tracker records and aggregates LLM usage and cost.
type Tracker struct {
	mu       sync.RWMutex
	records  []UsageRecord
	stats    map[string]*ProviderStats
	dataFile string
}

// NewTracker creates a Tracker and loads any existing history from dataDir.
func NewTracker(dataDir string) (*Tracker, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("creating cost data dir: %w", err)
	}

	t := &Tracker{
		records:  make([]UsageRecord, 0),
		stats:    make(map[string]*ProviderStats),
		dataFile: filepath.Join(dataDir, "usage.json"),
	}
	if err := t.Load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("loading usage data: %w", err)
	}
	return t, nil
}

// Record logs a single call and updates cumulative stats.
func (t *Tracker) Record(provider, model string, inputTokens, outputTokens int) {
	cost := EstimateCost(provider, model, inputTokens, outputTokens)
	key := provider + ":" + model

	t.mu.Lock()
	defer t.mu.Unlock()

	t.records = append(t.records, UsageRecord{
		Provider:     provider,
		Model:        model,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		CostUSD:      cost,
		Timestamp:    time.Now(),
	})

	if t.stats[key] == nil {
		t.stats[key] = &ProviderStats{}
	}
	s := t.stats[key]
	s.TotalInputTokens += inputTokens
	s.TotalOutputTokens += outputTokens
	s.TotalCostUSD += cost
	s.TotalCalls++
}

// GetStats returns cumulative stats for a provider+model pair.
func (t *Tracker) GetStats(provider, model string) *ProviderStats {
	t.mu.RLock()
	defer t.mu.RUnlock()
	key := provider + ":" + model
	if s, ok := t.stats[key]; ok {
		cp := *s
		return &cp
	}
	return &ProviderStats{}
}

// GetAllStats returns cumulative stats for all provider+model pairs.
func (t *Tracker) GetAllStats() map[string]*ProviderStats {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make(map[string]*ProviderStats, len(t.stats))
	for k, v := range t.stats {
		cp := *v
		out[k] = &cp
	}
	return out
}

// TotalCost returns the sum of all recorded costs.
func (t *Tracker) TotalCost() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var total float64
	for _, s := range t.stats {
		total += s.TotalCostUSD
	}
	return total
}

// Save persists usage data to disk as JSON.
func (t *Tracker) Save() error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	data := struct {
		Records []UsageRecord             `json:"records"`
		Stats   map[string]*ProviderStats `json:"stats"`
	}{
		Records: t.records,
		Stats:   t.stats,
	}

	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling usage data: %w", err)
	}
	if err := os.WriteFile(t.dataFile, b, 0644); err != nil {
		return fmt.Errorf("writing usage data: %w", err)
	}
	return nil
}

// Load reads saved usage data from disk.
func (t *Tracker) Load() error {
	b, err := os.ReadFile(t.dataFile)
	if err != nil {
		return err
	}

	var data struct {
		Records []UsageRecord             `json:"records"`
		Stats   map[string]*ProviderStats `json:"stats"`
	}
	if err := json.Unmarshal(b, &data); err != nil {
		return fmt.Errorf("unmarshaling usage data: %w", err)
	}

	t.records = data.Records
	if data.Stats != nil {
		t.stats = data.Stats
	}
	return nil
}
