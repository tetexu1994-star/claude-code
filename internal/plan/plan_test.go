package plan

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/tetexu/tlaude-code/internal/llm"
)

// ---- PlanStore Tests ----

func TestPlanStoreCreate(t *testing.T) {
	s := NewPlanStoreAt(t.TempDir())
	p := s.Create("Test Plan", "A test plan description")

	if p.ID == "" {
		t.Error("expected non-empty ID")
	}
	if p.Title != "Test Plan" {
		t.Errorf("Title = %q, want %q", p.Title, "Test Plan")
	}
	if p.Description != "A test plan description" {
		t.Errorf("Description = %q, want %q", p.Description, "A test plan description")
	}
	if p.Status != PlanDraft {
		t.Errorf("Status = %q, want %q", p.Status, PlanDraft)
	}
	if p.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestPlanStoreGet(t *testing.T) {
	s := NewPlanStoreAt(t.TempDir())
	p := s.Create("Test", "desc")

	got, ok := s.Get(p.ID)
	if !ok {
		t.Fatal("Get returned false for existing plan")
	}
	if got.ID != p.ID {
		t.Errorf("Get ID = %q, want %q", got.ID, p.ID)
	}

	_, ok = s.Get("nonexistent")
	if ok {
		t.Error("Get returned true for non-existent plan")
	}
}

func TestPlanStoreList(t *testing.T) {
	s := NewPlanStoreAt(t.TempDir())
	p1 := s.Create("First", "first desc")
	p2 := s.Create("Second", "second desc")

	list := s.List()
	if len(list) != 2 {
		t.Fatalf("List len = %d, want 2", len(list))
	}
	// p2 created after p1, so should appear first (descending by CreatedAt)
	if list[0].ID != p2.ID {
		t.Error("expected most recently created plan first")
	}
	if list[1].ID != p1.ID {
		t.Error("expected older plan second")
	}
}

func TestPlanStoreUpdate(t *testing.T) {
	s := NewPlanStoreAt(t.TempDir())
	p := s.Create("Original", "original desc")
	p.Title = "Updated"
	p.Status = PlanPending
	s.Update(p)

	got, ok := s.Get(p.ID)
	if !ok {
		t.Fatal("Get returned false after update")
	}
	if got.Title != "Updated" {
		t.Errorf("Title = %q, want %q", got.Title, "Updated")
	}
	if got.Status != PlanPending {
		t.Errorf("Status = %q, want %q", got.Status, PlanPending)
	}
}

func TestPlanStoreDelete(t *testing.T) {
	s := NewPlanStoreAt(t.TempDir())
	p := s.Create("Test", "desc")
	s.Delete(p.ID)

	_, ok := s.Get(p.ID)
	if ok {
		t.Error("Get returned true after delete")
	}
}

func TestPlanStoreSaveLoad(t *testing.T) {
	s := NewPlanStoreAt(t.TempDir())
	p := s.Create("Save Test", "save desc")
	p.Steps = []PlanStep{
		{Index: 1, Description: "step 1", Status: StepPending},
		{Index: 2, Description: "step 2", Status: StepCompleted},
	}
	s.Update(p)

	if err := s.Save(p); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Create a new store pointing to same dir to load from disk
	s2 := NewPlanStoreAt(s.Dir())
	loaded, err := s2.Load(p.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Title != "Save Test" {
		t.Errorf("Title = %q, want %q", loaded.Title, "Save Test")
	}
	if len(loaded.Steps) != 2 {
		t.Errorf("Steps len = %d, want 2", len(loaded.Steps))
	}
	if loaded.Steps[1].Status != StepCompleted {
		t.Errorf("Step 2 status = %q, want %q", loaded.Steps[1].Status, StepCompleted)
	}
}

func TestPlanStoreLoadAll(t *testing.T) {
	dir := t.TempDir()
	s := NewPlanStoreAt(dir)

	// Create and save two plans
	p1 := s.Create("Plan One", "desc one")
	p1.Steps = []PlanStep{{Index: 1, Description: "s1", Status: StepPending}}
	s.Save(p1)

	p2 := s.Create("Plan Two", "desc two")
	s.Save(p2)

	// New empty store loads all from disk
	s2 := NewPlanStoreAt(dir)
	if err := s2.LoadAll(); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	got1, ok := s2.Get(p1.ID)
	if !ok {
		t.Error("p1 not loaded")
	}
	if got1.Title != "Plan One" {
		t.Errorf("p1 Title = %q", got1.Title)
	}

	got2, ok := s2.Get(p2.ID)
	if !ok {
		t.Error("p2 not loaded")
	}
	if got2.Title != "Plan Two" {
		t.Errorf("p2 Title = %q", got2.Title)
	}

	if len(s2.List()) != 2 {
		t.Errorf("List len = %d, want 2", len(s2.List()))
	}
}

func TestPlanStoreCreateUniqueID(t *testing.T) {
	s := NewPlanStoreAt(t.TempDir())
	p1 := s.Create("A", "a")
	p2 := s.Create("B", "b")

	if p1.ID == "" || p2.ID == "" {
		t.Fatal("IDs should not be empty")
	}
	if p1.ID == p2.ID {
		t.Error("expected unique IDs for consecutive creates")
	}
}

func TestPlanStoreConcurrency(t *testing.T) {
	s := NewPlanStoreAt(t.TempDir())
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			p := s.Create("Concurrent", "desc")
			s.Update(p)
			s.Get(p.ID)
			s.List()
		}(i)
	}
	wg.Wait()
}

func TestPlanStoreSaveConcurrency(t *testing.T) {
	s := NewPlanStoreAt(t.TempDir())
	var wg sync.WaitGroup
	p := s.Create("Shared", "shared desc")

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Save(p)
		}()
	}
	wg.Wait()

	// Verify data is intact after concurrent saves
	got, ok := s.Get(p.ID)
	if !ok {
		t.Fatal("plan lost after concurrent saves")
	}
	if got.Title != "Shared" {
		t.Errorf("Title = %q", got.Title)
	}
}

func TestPlanStoreLoadAllEmptyDir(t *testing.T) {
	s := NewPlanStoreAt(t.TempDir())
	if err := s.LoadAll(); err != nil {
		t.Fatalf("LoadAll on empty dir should not error: %v", err)
	}
	if len(s.List()) != 0 {
		t.Errorf("expected 0 plans, got %d", len(s.List()))
	}
}

func TestPlanStoreLoadNonExistent(t *testing.T) {
	s := NewPlanStoreAt(t.TempDir())
	_, err := s.Load("nonexistent")
	if err == nil {
		t.Error("expected error loading non-existent plan")
	}
}

func TestPlanStoreSaveCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "plans")
	s := NewPlanStoreAt(dir)
	p := s.Create("Test", "desc")

	if err := s.Save(p); err != nil {
		t.Fatalf("Save should create nested dirs: %v", err)
	}
}

func TestPlanStoreSaveToDiskFormat(t *testing.T) {
	dir := t.TempDir()
	s := NewPlanStoreAt(dir)
	p := s.Create("Format Test", "format desc")
	p.Steps = []PlanStep{{Index: 1, Description: "step", Status: StepPending}}
	s.Save(p)

	// Read the raw file and verify JSON format
	data, err := os.ReadFile(filepath.Join(dir, p.ID+".json"))
	if err != nil {
		t.Fatalf("reading saved file: %v", err)
	}
	var parsed Plan
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshaling saved file: %v", err)
	}
	if parsed.ID != p.ID {
		t.Errorf("saved JSON ID = %q, want %q", parsed.ID, p.ID)
	}
	if parsed.Title != "Format Test" {
		t.Errorf("saved JSON Title = %q", parsed.Title)
	}
	if len(parsed.Steps) != 1 {
		t.Errorf("saved JSON Steps len = %d, want 1", len(parsed.Steps))
	}
}

// ---- Manager Tests ----

func TestManagerBuildFromDescription(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	p := m.BuildFromDescription("My Plan", "1. First step\n2. Second step\n3. Third step")

	if p.Title != "My Plan" {
		t.Errorf("Title = %q", p.Title)
	}
	if len(p.Steps) != 3 {
		t.Fatalf("Steps len = %d, want 3", len(p.Steps))
	}
	if p.Steps[0].Description != "First step" {
		t.Errorf("Step 0 = %q", p.Steps[0].Description)
	}
	if p.Steps[1].Description != "Second step" {
		t.Errorf("Step 1 = %q", p.Steps[1].Description)
	}
	if p.Steps[2].Description != "Third step" {
		t.Errorf("Step 2 = %q", p.Steps[2].Description)
	}
	if p.Status != PlanDraft {
		t.Errorf("Status = %q, want %q", p.Status, PlanDraft)
	}
}

func TestManagerBuildFromDescriptionBulletPoints(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	p := m.BuildFromDescription("Bullet Plan", "- First bullet\n- Second bullet\n* Third star")

	if len(p.Steps) != 3 {
		t.Fatalf("Steps len = %d, want 3", len(p.Steps))
	}
	if p.Steps[0].Description != "First bullet" {
		t.Errorf("Step 0 = %q", p.Steps[0].Description)
	}
	if p.Steps[2].Description != "Third star" {
		t.Errorf("Step 2 = %q", p.Steps[2].Description)
	}
}

func TestManagerBuildFromDescriptionNumberedDots(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	p := m.BuildFromDescription("Numbered Dots", "1) Step A\n2) Step B")

	if len(p.Steps) != 2 {
		t.Fatalf("Steps len = %d, want 2", len(p.Steps))
	}
	if p.Steps[0].Description != "Step A" {
		t.Errorf("Step 0 = %q", p.Steps[0].Description)
	}
}

func TestManagerBuildFromDescriptionNoSteps(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	p := m.BuildFromDescription("No Steps", "This is just a plain description without structured steps.")

	if len(p.Steps) != 1 {
		t.Fatalf("Steps len = %d, want 1", len(p.Steps))
	}
	if p.Steps[0].Description != "This is just a plain description without structured steps." {
		t.Errorf("Step 0 = %q", p.Steps[0].Description)
	}
	if p.Steps[0].Index != 1 {
		t.Errorf("Step Index = %d, want 1", p.Steps[0].Index)
	}
}

func TestManagerBuildFromDescriptionEmptyDescription(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	p := m.BuildFromDescription("Empty", "")

	if len(p.Steps) != 0 {
		t.Errorf("Steps len = %d, want 0", len(p.Steps))
	}
}

func TestManagerBuildFromDescriptionSetsActive(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	p := m.BuildFromDescription("Active Test", "1. step")

	active := m.Active()
	if active == nil {
		t.Fatal("expected active plan to be set")
	}
	if active.ID != p.ID {
		t.Errorf("Active ID = %q, want %q", active.ID, p.ID)
	}
}

func TestManagerBuildFromMessagesWithEnterPlanMode(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	messages := []llm.Message{
		{Role: "user", Content: "Make a plan"},
		{Role: "assistant", Content: "I'll plan this out",
			ToolCalls: []llm.ToolCall{{
				Name: "EnterPlanMode",
				Args: map[string]interface{}{
					"plan":  "1. Research\n2. Implement\n3. Test",
					"scope": "Feature X Implementation",
				},
			}},
		},
	}

	p := m.BuildFromMessages(messages)
	if p == nil {
		t.Fatal("expected non-nil plan from messages")
	}
	if p.Title != "Feature X Implementation" {
		t.Errorf("Title = %q, want %q", p.Title, "Feature X Implementation")
	}
	if len(p.Steps) != 3 {
		t.Fatalf("Steps len = %d, want 3", len(p.Steps))
	}
}

func TestManagerBuildFromMessagesWithPlanHeading(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	messages := []llm.Message{
		{Role: "user", Content: "Help me"},
		{Role: "assistant", Content: "## Plan\n\n1. Do this\n2. Do that\n\nLet's proceed."},
	}

	p := m.BuildFromMessages(messages)
	if p == nil {
		t.Fatal("expected non-nil plan from messages")
	}
	if p.Title != "Plan" {
		t.Errorf("Title = %q, want %q", p.Title, "Plan")
	}
	if len(p.Steps) != 2 {
		t.Fatalf("Steps len = %d, want 2", len(p.Steps))
	}
}

func TestManagerBuildFromMessagesWithImplPlanHeading(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	messages := []llm.Message{
		{Role: "assistant", Content: "## Implementation Plan\n\n- Research approach\n- Write code\n- Test"},
	}

	p := m.BuildFromMessages(messages)
	if p == nil {
		t.Fatal("expected non-nil plan from messages")
	}
	if len(p.Steps) != 3 {
		t.Fatalf("Steps len = %d, want 3", len(p.Steps))
	}
}

func TestManagerBuildFromMessagesNoPlan(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	messages := []llm.Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi, how can I help?"},
	}

	p := m.BuildFromMessages(messages)
	if p != nil {
		t.Error("expected nil when no plan content found in messages")
	}
}

func TestManagerBuildFromMessagesFindsLastPlan(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	messages := []llm.Message{
		{Role: "assistant", Content: "## Plan\n\n1. Old plan"},
		{Role: "user", Content: "Revise please"},
		{Role: "assistant", Content: "## Plan\n\n1. New plan\n2. Better steps"},
	}

	p := m.BuildFromMessages(messages)
	if p == nil {
		t.Fatal("expected non-nil plan")
	}
	if len(p.Steps) != 2 {
		t.Fatalf("Steps len = %d, want 2 (the last plan)", len(p.Steps))
	}
}

func TestManagerSubmit(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	p := m.BuildFromDescription("Submit Test", "1. step")
	p.Status = PlanDraft
	m.store.Update(p)

	err := m.Submit(p.ID)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	got, _ := m.store.Get(p.ID)
	if got.Status != PlanPending {
		t.Errorf("Status = %q, want %q", got.Status, PlanPending)
	}
}

func TestManagerSubmitNonDraft(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	p := m.BuildFromDescription("Test", "1. step")
	p.Status = PlanApproved
	m.store.Update(p)

	err := m.Submit(p.ID)
	if err == nil {
		t.Error("expected error when submitting non-draft plan")
	}
}

func TestManagerSubmitNonExistent(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	err := m.Submit("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent plan")
	}
}

func TestManagerApprove(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	p := m.BuildFromDescription("Approve Test", "1. step")
	m.Submit(p.ID)

	err := m.Approve(p.ID)
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}

	got, _ := m.store.Get(p.ID)
	if got.Status != PlanApproved {
		t.Errorf("Status = %q, want %q", got.Status, PlanApproved)
	}
	if got.ApprovedAt == nil {
		t.Error("expected ApprovedAt to be set")
	}
}

func TestManagerApproveFromDraft(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	p := m.BuildFromDescription("Approve Draft", "1. step")
	// p is already Draft, approve directly

	err := m.Approve(p.ID)
	if err != nil {
		t.Fatalf("Approve from draft: %v", err)
	}

	got, _ := m.store.Get(p.ID)
	if got.Status != PlanApproved {
		t.Errorf("Status = %q, want %q", got.Status, PlanApproved)
	}
}

func TestManagerApproveFromPending(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	p := m.BuildFromDescription("Approve Pending", "1. step")
	m.Submit(p.ID)

	err := m.Approve(p.ID)
	if err != nil {
		t.Fatalf("Approve from pending: %v", err)
	}

	got, _ := m.store.Get(p.ID)
	if got.Status != PlanApproved {
		t.Errorf("Status = %q, want %q", got.Status, PlanApproved)
	}
}

func TestManagerApproveInvalidState(t *testing.T) {
	tests := []struct {
		name   string
		status PlanStatus
	}{
		{"completed", PlanCompleted},
		{"rejected", PlanRejected},
		{"executing", PlanExecuting},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager(NewPlanStoreAt(t.TempDir()))
			p := m.BuildFromDescription("Test", "1. step")
			p.Status = tt.status
			m.store.Update(p)

			err := m.Approve(p.ID)
			if err == nil {
				t.Errorf("expected error approving %s plan", tt.status)
			}
		})
	}
}

func TestManagerReject(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	p := m.BuildFromDescription("Reject Test", "1. step")
	m.Submit(p.ID)
	m.SetActive(p)

	err := m.Reject(p.ID, "not needed")
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}

	got, _ := m.store.Get(p.ID)
	if got.Status != PlanRejected {
		t.Errorf("Status = %q, want %q", got.Status, PlanRejected)
	}
	if m.Active() != nil {
		t.Error("expected active plan to be cleared after rejection")
	}
}

func TestManagerRejectNonActive(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	p := m.BuildFromDescription("Test", "1. step")
	m.ClearActive()

	err := m.Reject(p.ID, "reason")
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}

	got, _ := m.store.Get(p.ID)
	if got.Status != PlanRejected {
		t.Errorf("Status = %q, want %q", got.Status, PlanRejected)
	}
}

func TestManagerRejectNonExistent(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	err := m.Reject("nonexistent", "reason")
	if err == nil {
		t.Error("expected error for non-existent plan")
	}
}

func TestManagerExecuteStep(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	p := m.BuildFromDescription("Exec Test", "1. step one\n2. step two")
	m.Approve(p.ID)

	err := m.ExecuteStep(context.Background(), p.ID, 0, func(ctx context.Context, step *PlanStep) error {
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteStep: %v", err)
	}

	got, _ := m.store.Get(p.ID)
	if got.Steps[0].Status != StepCompleted {
		t.Errorf("Step 0 status = %q, want %q", got.Steps[0].Status, StepCompleted)
	}
	if got.Status != PlanExecuting {
		t.Errorf("Plan status = %q, want %q (one step done, one pending)", got.Status, PlanExecuting)
	}
}

func TestManagerExecuteStepFailed(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	p := m.BuildFromDescription("Fail Test", "1. step")
	m.Approve(p.ID)

	execErr := errors.New("execution failed")
	err := m.ExecuteStep(context.Background(), p.ID, 0, func(ctx context.Context, step *PlanStep) error {
		return execErr
	})
	if err == nil {
		t.Fatal("expected error from failed step")
	}

	got, _ := m.store.Get(p.ID)
	if got.Steps[0].Status != StepFailed {
		t.Errorf("Step 0 status = %q, want %q", got.Steps[0].Status, StepFailed)
	}
	if got.Steps[0].Result != "execution failed" {
		t.Errorf("Step 0 result = %q, want %q", got.Steps[0].Result, "execution failed")
	}
}

func TestManagerExecuteStepOutOfRange(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	p := m.BuildFromDescription("Test", "1. step")
	m.Approve(p.ID)

	err := m.ExecuteStep(context.Background(), p.ID, -1, nil)
	if err == nil {
		t.Error("expected error for negative index")
	}

	err = m.ExecuteStep(context.Background(), p.ID, 1, nil)
	if err == nil {
		t.Error("expected error for out-of-range index")
	}
}

func TestManagerExecuteAllStepsCompletesPlan(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	p := m.BuildFromDescription("All Steps", "1. step one\n2. step two")
	m.Approve(p.ID)
	m.SetActive(p)

	err := m.ExecuteAll(context.Background(), p.ID, func(ctx context.Context, step *PlanStep) error {
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteAll: %v", err)
	}

	got, _ := m.store.Get(p.ID)
	if got.Status != PlanCompleted {
		t.Errorf("Plan status = %q, want %q", got.Status, PlanCompleted)
	}
	if m.Active() != nil {
		t.Error("expected active plan cleared after completion")
	}
}

func TestManagerExecuteAllSkipCompleted(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	p := m.BuildFromDescription("Skip Test", "1. step one\n2. step two\n3. step three")
	m.Approve(p.ID)

	// Mark step 0 as already completed
	p.Steps[0].Status = StepCompleted
	m.store.Update(p)

	executedSteps := make(map[int]bool)
	err := m.ExecuteAll(context.Background(), p.ID, func(ctx context.Context, step *PlanStep) error {
		executedSteps[step.Index] = true
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteAll: %v", err)
	}

	if executedSteps[1] {
		t.Error("step 1 (index 0) should have been skipped (already completed)")
	}
	if !executedSteps[2] {
		t.Error("step 2 (index 1) should have been executed")
	}
	if !executedSteps[3] {
		t.Error("step 3 (index 2) should have been executed")
	}
}

func TestManagerExecuteAllNonExistent(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	err := m.ExecuteAll(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Error("expected error for non-existent plan")
	}
}

func TestManagerGetProgress(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	p := m.BuildFromDescription("Progress", "1. step one\n2. step two\n3. step three\n4. step four")
	m.Approve(p.ID)

	// Execute first step
	m.ExecuteStep(context.Background(), p.ID, 0, func(ctx context.Context, step *PlanStep) error {
		return nil
	})

	prog, err := m.GetProgress(p.ID)
	if err != nil {
		t.Fatalf("GetProgress: %v", err)
	}
	if prog.TotalSteps != 4 {
		t.Errorf("TotalSteps = %d, want 4", prog.TotalSteps)
	}
	if prog.CompletedSteps != 1 {
		t.Errorf("CompletedSteps = %d, want 1", prog.CompletedSteps)
	}
	if prog.FailedSteps != 0 {
		t.Errorf("FailedSteps = %d, want 0", prog.FailedSteps)
	}
	if prog.Percent != 25.0 {
		t.Errorf("Percent = %f, want 25.0", prog.Percent)
	}
}

func TestManagerGetProgressWithFailed(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	p := m.BuildFromDescription("Progress Fail", "1. good step\n2. bad step\n3. another good")
	m.Approve(p.ID)

	m.ExecuteStep(context.Background(), p.ID, 0, func(ctx context.Context, step *PlanStep) error {
		return nil
	})
	m.ExecuteStep(context.Background(), p.ID, 1, func(ctx context.Context, step *PlanStep) error {
		return errors.New("fail")
	})

	prog, err := m.GetProgress(p.ID)
	if err != nil {
		t.Fatalf("GetProgress: %v", err)
	}
	if prog.CompletedSteps != 1 {
		t.Errorf("CompletedSteps = %d, want 1", prog.CompletedSteps)
	}
	if prog.FailedSteps != 1 {
		t.Errorf("FailedSteps = %d, want 1", prog.FailedSteps)
	}
	// 1 completed out of 3 total = ~33.3%
	if abs(prog.Percent-100.0/3.0) > 0.01 {
		t.Errorf("Percent = %f, want ~33.333", prog.Percent)
	}
}

func TestManagerGetProgressNonExistent(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	_, err := m.GetProgress("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent plan")
	}
}

func TestManagerGetProgressNoSteps(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	p := m.BuildFromDescription("No Steps", "")
	m.Approve(p.ID)

	prog, err := m.GetProgress(p.ID)
	if err != nil {
		t.Fatalf("GetProgress: %v", err)
	}
	if prog.Percent != 0.0 {
		t.Errorf("Percent = %f, want 0.0 for no steps", prog.Percent)
	}
}

func TestManagerGetProgressHundredPercent(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	p := m.BuildFromDescription("Full", "1. step one\n2. step two")
	m.Approve(p.ID)

	m.ExecuteAll(context.Background(), p.ID, func(ctx context.Context, step *PlanStep) error {
		return nil
	})

	prog, err := m.GetProgress(p.ID)
	if err != nil {
		t.Fatalf("GetProgress: %v", err)
	}
	if prog.Percent != 100.0 {
		t.Errorf("Percent = %f, want 100.0", prog.Percent)
	}
}

func TestManagerIsInPlanMode(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))

	// No active plan
	if m.IsInPlanMode() {
		t.Error("IsInPlanMode should be false with no active plan")
	}

	p := m.BuildFromDescription("Test", "1. step")
	// BuildFromDescription sets active
	if !m.IsInPlanMode() {
		t.Error("IsInPlanMode should be true with Draft active")
	}

	p.Status = PlanPending
	m.store.Update(p)
	if !m.IsInPlanMode() {
		t.Error("IsInPlanMode should be true with Pending active")
	}

	p.Status = PlanApproved
	m.store.Update(p)
	if !m.IsInPlanMode() {
		t.Error("IsInPlanMode should be true with Approved active")
	}

	p.Status = PlanExecuting
	m.store.Update(p)
	if !m.IsInPlanMode() {
		t.Error("IsInPlanMode should be true with Executing active")
	}

	p.Status = PlanCompleted
	m.store.Update(p)
	if m.IsInPlanMode() {
		t.Error("IsInPlanMode should be false with Completed active")
	}

	p.Status = PlanRejected
	m.store.Update(p)
	if m.IsInPlanMode() {
		t.Error("IsInPlanMode should be false with Rejected active")
	}

	m.ClearActive()
	if m.IsInPlanMode() {
		t.Error("IsInPlanMode should be false after ClearActive")
	}
}

func TestManagerActiveSetClear(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	p := m.BuildFromDescription("Test", "1. step")

	if m.Active() == nil {
		t.Fatal("expected active after BuildFromDescription")
	}

	m.ClearActive()
	if m.Active() != nil {
		t.Error("expected nil active after ClearActive")
	}

	m.SetActive(p)
	if m.Active() == nil {
		t.Error("expected non-nil active after SetActive")
	}
	if m.Active().ID != p.ID {
		t.Errorf("Active ID = %q, want %q", m.Active().ID, p.ID)
	}
}

func TestManagerFormatPlan(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	p := m.BuildFromDescription("Format Test", "1. First step\n2. Second step")
	m.Approve(p.ID)
	m.ExecuteStep(context.Background(), p.ID, 0, func(ctx context.Context, step *PlanStep) error {
		return nil
	})

	output := m.FormatPlan(p.ID)

	if !strings.Contains(output, "Format Test") {
		t.Error("output should contain plan title")
	}
	if !strings.Contains(output, "executing") {
		t.Error("output should contain status (executing after ExecuteStep)")
	}
	if !strings.Contains(output, "✓") {
		t.Error("output should contain checkmark for completed step")
	}
	if !strings.Contains(output, "○") {
		t.Error("output should contain circle for pending step")
	}
}

func TestManagerFormatPlanNotFound(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	output := m.FormatPlan("nonexistent")
	if output != "Plan not found" {
		t.Errorf("output = %q, want %q", output, "Plan not found")
	}
}

func TestFormatPlanStandalone(t *testing.T) {
	p := &Plan{
		ID:          "test-id",
		Title:       "Standalone",
		Description: "desc",
		Status:      PlanApproved,
		Steps: []PlanStep{
			{Index: 1, Description: "done step", Status: StepCompleted},
			{Index: 2, Description: "working step", Status: StepRunning},
			{Index: 3, Description: "skipped step", Status: StepSkipped},
			{Index: 4, Description: "failed step", Status: StepFailed, Result: "error msg"},
		},
	}

	output := FormatPlan(p)
	if !strings.Contains(output, "Standalone") {
		t.Error("output should contain title")
	}
	if !strings.Contains(output, "✓") {
		t.Error("output should contain ✓ for completed")
	}
	if !strings.Contains(output, "●") {
		t.Error("output should contain ● for running")
	}
	if !strings.Contains(output, "→") {
		t.Error("output should contain → for skipped")
	}
	if !strings.Contains(output, "✗") {
		t.Error("output should contain ✗ for failed")
	}
	if !strings.Contains(output, "error msg") {
		t.Error("output should contain result for failed step")
	}
}

func TestFormatPlanWithApprovedAtNil(t *testing.T) {
	p := &Plan{
		ID:          "test-id",
		Title:       "Approved Plan",
		Description: "desc",
		Status:      PlanApproved,
		Steps:       []PlanStep{{Index: 1, Description: "step", Status: StepPending}},
	}

	output := FormatPlan(p)
	// ApprovedAt is nil, so format should not contain "Approved:"
	if strings.Contains(output, "Approved:") {
		t.Error("output should not contain 'Approved:' when ApprovedAt is nil")
	}
}

func TestFormatPlanApprovedAtSet(t *testing.T) {
	m := NewManager(NewPlanStoreAt(t.TempDir()))
	p := m.BuildFromDescription("Test", "1. step")
	m.Approve(p.ID)

	output := m.FormatPlan(p.ID)
	if !strings.Contains(output, "Approved:") {
		t.Error("output should contain 'Approved:' after approval")
	}
}

// ---- Helper Tests ----

func TestGenerateSlug(t *testing.T) {
	slug := GenerateSlug()
	parts := strings.Split(slug, "-")
	if len(parts) != 2 {
		t.Fatalf("GenerateSlug = %q, expected two words separated by '-'", slug)
	}
	if parts[0] == "" || parts[1] == "" {
		t.Errorf("GenerateSlug = %q, neither part should be empty", slug)
	}
}

func TestGenerateSlugConsistency(t *testing.T) {
	// Generate multiple slugs and verify they all have the expected format
	for i := 0; i < 100; i++ {
		slug := GenerateSlug()
		parts := strings.Split(slug, "-")
		if len(parts) != 2 {
			t.Errorf("GenerateSlug = %q, expected 'word-word' format", slug)
		}
	}
}

func TestBuildProgressBar(t *testing.T) {
	// 0%
	bar0 := buildProgressBar(0, 10)
	if bar0 != "░░░░░░░░░░" {
		t.Errorf("0%% bar = %q, want all empty", bar0)
	}

	// 100%
	bar100 := buildProgressBar(100, 10)
	if bar100 != "██████████" {
		t.Errorf("100%% bar = %q, want all filled", bar100)
	}

	// 50%
	bar50 := buildProgressBar(50, 10)
	if bar50 != "█████░░░░░" {
		t.Errorf("50%% bar = %q, want half filled", bar50)
	}

	// boundary values
	barNeg := buildProgressBar(-1, 10)
	if barNeg != "░░░░░░░░░░" {
		t.Errorf("negative %% bar = %q, want all empty", barNeg)
	}

	barOver := buildProgressBar(150, 10)
	if barOver != "██████████" {
		t.Errorf("over-100%% bar = %q, want all filled", barOver)
	}

	// zero width
	barZeroW := buildProgressBar(50, 0)
	if barZeroW != "" {
		t.Errorf("zero width bar = %q, want empty", barZeroW)
	}
}

func TestFormatProgress(t *testing.T) {
	// No steps
	prog0 := PlanProgress{TotalSteps: 0}
	if out := FormatProgress(prog0); out != "No steps" {
		t.Errorf("FormatProgress no steps = %q, want %q", out, "No steps")
	}

	// 0% progress
	prog1 := PlanProgress{TotalSteps: 4, CompletedSteps: 0}
	out1 := FormatProgress(prog1)
	if !strings.Contains(out1, "0/4") || !strings.Contains(out1, "0%") {
		t.Errorf("FormatProgress 0%% = %q", out1)
	}

	// 100% progress
	prog2 := PlanProgress{TotalSteps: 4, CompletedSteps: 4, Percent: 100}
	out2 := FormatProgress(prog2)
	if !strings.Contains(out2, "4/4") || !strings.Contains(out2, "100%") {
		t.Errorf("FormatProgress 100%% = %q", out2)
	}

	// 50% progress
	prog3 := PlanProgress{TotalSteps: 4, CompletedSteps: 2, Percent: 50}
	out3 := FormatProgress(prog3)
	if !strings.Contains(out3, "2/4") || !strings.Contains(out3, "50%") {
		t.Errorf("FormatProgress 50%% = %q", out3)
	}
}

func TestFormatProgressBarVisual(t *testing.T) {
	prog := PlanProgress{TotalSteps: 10, CompletedSteps: 5, Percent: 50}
	out := FormatProgress(prog)

	if !strings.Contains(out, "█") {
		t.Error("progress bar should use filled block chars")
	}
	if !strings.Contains(out, "░") {
		t.Error("progress bar should use empty block chars")
	}
	if !strings.Contains(out, "[") || !strings.Contains(out, "]") {
		t.Error("progress bar should be wrapped in brackets")
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
