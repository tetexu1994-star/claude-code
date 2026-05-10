// Package plan defines data structures and management for the Plan Mode feature
// (a "plan first, then execute" workflow inspired by Claude Code's plan mode).
package plan

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Plan represents an action plan created during plan mode.
type Plan struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Steps       []PlanStep `json:"steps"`
	Status      PlanStatus `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	ApprovedAt  *time.Time `json:"approved_at,omitempty"`
}

// PlanStep is a single step within a plan.
type PlanStep struct {
	Index       int            `json:"index"`
	Description string         `json:"description"`
	ToolCalls   []ToolCallSpec `json:"tool_calls,omitempty"`
	Status      StepStatus     `json:"status"`
	Result      string         `json:"result,omitempty"`
}

// ToolCallSpec describes a tool call planned for a step.
type ToolCallSpec struct {
	ToolName    string                 `json:"tool"`
	Description string                 `json:"description"`
	Args        map[string]interface{} `json:"args,omitempty"`
}

// PlanStatus is the lifecycle status of a plan.
type PlanStatus string

const (
	PlanDraft     PlanStatus = "draft"
	PlanPending   PlanStatus = "pending_approval"
	PlanApproved  PlanStatus = "approved"
	PlanExecuting PlanStatus = "executing"
	PlanCompleted PlanStatus = "completed"
	PlanRejected  PlanStatus = "rejected"
)

// StepStatus is the execution status of a single step.
type StepStatus string

const (
	StepPending   StepStatus = "pending"
	StepRunning   StepStatus = "running"
	StepCompleted StepStatus = "completed"
	StepFailed    StepStatus = "failed"
	StepSkipped   StepStatus = "skipped"
)

// PlanProgress summarizes the execution progress of a plan.
type PlanProgress struct {
	TotalSteps     int
	CompletedSteps int
	FailedSteps    int
	CurrentStep    int
	Percent        float64
}

// PlanStore persists plans to ~/.tlaude-code/plans/ as JSON files.
type PlanStore struct {
	mu    sync.RWMutex
	dir   string
	plans map[string]*Plan
}

// NewPlanStore creates a PlanStore using the default plans directory.
func NewPlanStore() *PlanStore {
	home, _ := os.UserHomeDir()
	return newPlanStoreAt(filepath.Join(home, ".tlaude-code", "plans"))
}

// NewPlanStoreAt creates a PlanStore rooted at the given directory.
func NewPlanStoreAt(dir string) *PlanStore {
	return newPlanStoreAt(dir)
}

func newPlanStoreAt(dir string) *PlanStore {
	return &PlanStore{
		dir:   dir,
		plans: make(map[string]*Plan),
	}
}

// Create creates a new plan with the given title and description.
func (s *PlanStore) Create(title, description string) *Plan {
	s.mu.Lock()
	defer s.mu.Unlock()

	p := &Plan{
		ID:          newPlanID(),
		Title:       title,
		Description: description,
		Steps:       make([]PlanStep, 0),
		Status:      PlanDraft,
		CreatedAt:   time.Now(),
	}
	s.plans[p.ID] = p
	return p
}

// Get retrieves a plan by ID.
func (s *PlanStore) Get(id string) (*Plan, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.plans[id]
	return p, ok
}

// List returns all plans sorted by CreatedAt descending.
func (s *PlanStore) List() []*Plan {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Plan, 0, len(s.plans))
	for _, p := range s.plans {
		result = append(result, p)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result
}

// Update persists a plan to the in-memory map.
func (s *PlanStore) Update(plan *Plan) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.plans[plan.ID] = plan
}

// Delete removes a plan from memory and disk.
func (s *PlanStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.plans, id)
	_ = os.Remove(s.planPath(id))
}

// Save persists a plan to disk as JSON.
func (s *PlanStore) Save(plan *Plan) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.plans[plan.ID] = plan

	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return fmt.Errorf("creating plans directory: %w", err)
	}

	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling plan: %w", err)
	}

	if err := os.WriteFile(s.planPath(plan.ID), data, 0644); err != nil {
		return fmt.Errorf("writing plan file: %w", err)
	}

	return nil
}

// Load reads a plan from disk by ID and caches it in memory.
func (s *PlanStore) Load(id string) (*Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.planPath(id)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("plan %s not found", id)
		}
		return nil, fmt.Errorf("reading plan file: %w", err)
	}

	var p Plan
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("unmarshaling plan: %w", err)
	}
	s.plans[p.ID] = &p
	return &p, nil
}

// LoadAll reads all plans from disk into the in-memory cache.
func (s *PlanStore) LoadAll() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return fmt.Errorf("creating plans directory: %w", err)
	}

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading plans directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(s.dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var p Plan
		if err := json.Unmarshal(data, &p); err != nil {
			continue
		}
		s.plans[p.ID] = &p
	}
	return nil
}

// Dir returns the store's directory path.
func (s *PlanStore) Dir() string {
	return s.dir
}

func (s *PlanStore) planPath(id string) string {
	return filepath.Join(s.dir, id+".json")
}

// GenerateSlug generates a human-readable word slug for a plan file (markdown export).
func GenerateSlug() string {
	words := []string{
		"amber", "azure", "bold", "brave", "bright", "calm", "clear", "coral",
		"crisp", "dawn", "deep", "delta", "dream", "eager", "echo", "ember",
		"fair", "fern", "flare", "frost", "gentle", "gold", "green", "hazel",
		"humble", "iris", "jade", "keen", "kind", "leaf", "light", "lunar",
		"maple", "mint", "mist", "noble", "nova", "ocean", "opal", "peace",
		"pixel", "pure", "quartz", "quick", "rapid", "reed", "rise", "river",
		"sage", "sharp", "silk", "silver", "solar", "spark", "star", "storm",
		"swift", "terra", "tide", "trail", "true", "valley", "vapor", "vivid",
		"water", "wild", "wind", "wise", "zephyr", "zone",
	}
	var b [2]byte
	rand.Read(b[:])
	idx1 := int(b[0]) % len(words)
	idx2 := int(b[1]) % len(words)
	return words[idx1] + "-" + words[idx2]
}

func newPlanID() string {
	var b [8]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
