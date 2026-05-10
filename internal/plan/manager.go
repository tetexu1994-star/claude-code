package plan

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/tetexu/tlaude-code/internal/llm"
)

// stepPattern matches numbered step lines like "1. Do something" or "- Do something".
var stepPattern = regexp.MustCompile(`(?m)^(?:\d+[.)]\s*|[-*]\s+)(.+)$`)

// Manager manages the plan lifecycle: creation, approval, execution, and progress.
type Manager struct {
	mu     sync.RWMutex
	store  *PlanStore
	active *Plan        // current active plan for the session
	logger *slog.Logger
}

// NewManager creates a new plan Manager.
func NewManager(store *PlanStore) *Manager {
	return &Manager{
		store:  store,
		logger: slog.Default().With("component", "plan-manager"),
	}
}

// Active returns the currently active plan, or nil.
func (m *Manager) Active() *Plan {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

// SetActive sets the active plan for the session.
func (m *Manager) SetActive(p *Plan) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active = p
}

// ClearActive clears the active plan (e.g. on /clear).
func (m *Manager) ClearActive() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active = nil
}

// BuildFromDescription creates a new plan by parsing a free-form description.
// Lines starting with numbers or bullet points become steps.
func (m *Manager) BuildFromDescription(title, description string) *Plan {
	plan := m.store.Create(title, description)

	// Parse steps from the description.
	matches := stepPattern.FindAllStringSubmatch(description, -1)
	for i, match := range matches {
		stepDesc := strings.TrimSpace(match[1])
		plan.Steps = append(plan.Steps, PlanStep{
			Index:       i + 1,
			Description: stepDesc,
			Status:      StepPending,
		})
	}

	// If no structured steps found, treat the entire description as one step.
	if len(plan.Steps) == 0 && description != "" {
		plan.Steps = []PlanStep{{
			Index:       1,
			Description: description,
			Status:      StepPending,
		}}
	}

	plan.Status = PlanDraft
	m.store.Update(plan)
	m.SetActive(plan)

	m.logger.Info("plan created",
		"id", plan.ID,
		"title", plan.Title,
		"steps", len(plan.Steps),
	)
	return plan
}

// BuildFromMessages creates a plan by parsing LLM output from messages.
// It looks for assistant messages containing plan content.
func (m *Manager) BuildFromMessages(messages []llm.Message) *Plan {
	var planContent string
	var title string

	// Find the last assistant message with plan-like content.
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role != "assistant" {
			continue
		}
		// Check for EnterPlanMode tool call content.
		for _, tc := range msg.ToolCalls {
			if tc.Name == "EnterPlanMode" {
				if p, ok := tc.Args["plan"].(string); ok && p != "" {
					planContent = p
				}
				if t, ok := tc.Args["scope"].(string); ok {
					title = t
				}
				break
			}
		}
		if planContent != "" {
			break
		}
		// Fallback: check if the message content looks like a plan.
		if strings.Contains(msg.Content, "## Plan") || strings.Contains(msg.Content, "## Implementation Plan") {
			planContent = msg.Content
			break
		}
	}

	if planContent == "" {
		return nil
	}

	if title == "" {
		// Extract title from first heading or first line.
		lines := strings.SplitN(planContent, "\n", 3)
		for _, line := range lines {
			line = strings.TrimSpace(line)
			line = strings.TrimLeft(line, "# ")
			if line != "" {
				title = line
				break
			}
		}
	}

	return m.BuildFromDescription(title, planContent)
}

// Submit submits a draft plan for user approval.
func (m *Manager) Submit(planID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	plan, ok := m.store.Get(planID)
	if !ok {
		return fmt.Errorf("plan %s not found", planID)
	}
	if plan.Status != PlanDraft {
		return fmt.Errorf("plan %s is %s, expected draft", planID, plan.Status)
	}
	plan.Status = PlanPending
	m.store.Update(plan)
	m.logger.Info("plan submitted for approval", "id", planID)
	return nil
}

// Approve marks a plan as approved and ready to execute.
func (m *Manager) Approve(planID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	plan, ok := m.store.Get(planID)
	if !ok {
		return fmt.Errorf("plan %s not found", planID)
	}
	if plan.Status != PlanPending && plan.Status != PlanDraft {
		return fmt.Errorf("plan %s is %s, cannot approve", planID, plan.Status)
	}

	now := time.Now()
	plan.ApprovedAt = &now
	plan.Status = PlanApproved
	m.store.Update(plan)
	m.logger.Info("plan approved", "id", planID)
	return nil
}

// Reject rejects a plan with a reason.
func (m *Manager) Reject(planID string, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	plan, ok := m.store.Get(planID)
	if !ok {
		return fmt.Errorf("plan %s not found", planID)
	}
	plan.Status = PlanRejected
	m.store.Update(plan)

	// If the rejected plan was the active one, clear it.
	if m.active != nil && m.active.ID == planID {
		m.active = nil
	}

	m.logger.Info("plan rejected", "id", planID, "reason", reason)
	return nil
}

// ExecuteStep marks a step as running, executes it via the provided executor,
// then marks it as completed or failed.
func (m *Manager) ExecuteStep(ctx context.Context, planID string, stepIndex int, exec func(ctx context.Context, step *PlanStep) error) error {
	plan, ok := m.store.Get(planID)
	if !ok {
		return fmt.Errorf("plan %s not found", planID)
	}

	if stepIndex < 0 || stepIndex >= len(plan.Steps) {
		return fmt.Errorf("step index %d out of range [0, %d)", stepIndex, len(plan.Steps))
	}

	step := &plan.Steps[stepIndex]
	step.Status = StepRunning
	plan.Status = PlanExecuting
	m.store.Update(plan)

	if err := exec(ctx, step); err != nil {
		step.Status = StepFailed
		step.Result = err.Error()
		m.store.Update(plan)
		return fmt.Errorf("step %d failed: %w", stepIndex+1, err)
	}

	step.Status = StepCompleted
	m.store.Update(plan)

	// Check if all steps are done.
	allDone := true
	for _, s := range plan.Steps {
		if s.Status != StepCompleted && s.Status != StepSkipped {
			allDone = false
			break
		}
	}
	if allDone {
		plan.Status = PlanCompleted
		m.store.Update(plan)
		// Clear active if this was the active plan.
		m.mu.Lock()
		if m.active != nil && m.active.ID == planID {
			m.active = nil
		}
		m.mu.Unlock()
	}

	return nil
}

// ExecuteAll runs all pending steps in sequence.
func (m *Manager) ExecuteAll(ctx context.Context, planID string, exec func(ctx context.Context, step *PlanStep) error) error {
	plan, ok := m.store.Get(planID)
	if !ok {
		return fmt.Errorf("plan %s not found", planID)
	}

	plan.Status = PlanExecuting
	m.store.Update(plan)

	for i := range plan.Steps {
		step := &plan.Steps[i]
		if step.Status == StepCompleted || step.Status == StepSkipped {
			continue
		}
		if err := m.ExecuteStep(ctx, planID, i, exec); err != nil {
			return err
		}
	}

	return nil
}

// GetProgress returns the execution progress of a plan.
func (m *Manager) GetProgress(planID string) (PlanProgress, error) {
	plan, ok := m.store.Get(planID)
	if !ok {
		return PlanProgress{}, fmt.Errorf("plan %s not found", planID)
	}

	var prog PlanProgress
	prog.TotalSteps = len(plan.Steps)
	prog.CurrentStep = -1
	for i, s := range plan.Steps {
		switch s.Status {
		case StepCompleted:
			prog.CompletedSteps++
		case StepFailed:
			prog.FailedSteps++
		case StepRunning:
			prog.CurrentStep = i
		}
	}
	if prog.TotalSteps > 0 {
		prog.Percent = float64(prog.CompletedSteps) / float64(prog.TotalSteps) * 100
	}
	return prog, nil
}

// FormatProgress returns a human-readable progress string.
func FormatProgress(prog PlanProgress) string {
	if prog.TotalSteps == 0 {
		return "No steps"
	}
	bar := buildProgressBar(prog.Percent, 20)
	return fmt.Sprintf("[%s] %d/%d steps (%.0f%%)",
		bar, prog.CompletedSteps, prog.TotalSteps, prog.Percent)
}

func buildProgressBar(percent float64, width int) string {
	filled := int(percent / 100.0 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

// IsInPlanMode returns true if there's an active plan.
func (m *Manager) IsInPlanMode() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active != nil && (m.active.Status == PlanDraft || m.active.Status == PlanPending || m.active.Status == PlanApproved || m.active.Status == PlanExecuting)
}

// FormatPlan returns a multi-line summary of the plan for display.
func (m *Manager) FormatPlan(planID string) string {
	plan, ok := m.store.Get(planID)
	if !ok {
		return "Plan not found"
	}
	return FormatPlan(plan)
}

// FormatPlan formats a plan for display.
func FormatPlan(p *Plan) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Plan: %s\n", p.Title))
	sb.WriteString(fmt.Sprintf("Status: %s\n", p.Status))
	sb.WriteString(fmt.Sprintf("ID: %s\n", p.ID))
	if p.ApprovedAt != nil {
		sb.WriteString(fmt.Sprintf("Approved: %s\n", p.ApprovedAt.Format(time.RFC3339)))
	}
	sb.WriteString("\nSteps:\n")
	for _, s := range p.Steps {
		icon := "○"
		switch s.Status {
		case StepCompleted:
			icon = "✓"
		case StepFailed:
			icon = "✗"
		case StepRunning:
			icon = "●"
		case StepSkipped:
			icon = "→"
		}
		sb.WriteString(fmt.Sprintf("  %s %d. %s", icon, s.Index, s.Description))
		if s.Result != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", s.Result))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
