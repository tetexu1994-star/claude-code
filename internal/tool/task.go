package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// TaskStatus represents the current state of a task.
type TaskStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskRunning   TaskStatus = "running"
	TaskCompleted TaskStatus = "completed"
	TaskFailed    TaskStatus = "failed"
	TaskStopped   TaskStatus = "stopped"
)

// Task represents a tracked background task.
type Task struct {
	ID          string     `json:"id"`
	Description string     `json:"description"`
	Prompt      string     `json:"prompt"`
	AgentType   string     `json:"agent_type"`
	Status      TaskStatus `json:"status"`
	Result      string     `json:"result,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	agentID     string     `json:"-"` // linked AgentRuntime agent ID
	cancel      context.CancelFunc `json:"-"`
}

// TaskManager manages task lifecycle and access.
type TaskManager struct {
	mu     sync.RWMutex
	tasks  map[string]*Task
	nextID int

	// AgentRuntime bridge — set via SetAgentRuntimeBridge.
	agentRunAsync  func(ctx context.Context, agentType, prompt string) (agentID string, err error)
	agentStop      func(agentID string) error
	agentGetState  func(agentID string) (state string, result string, ok bool)
}

// NewTaskManager creates a new task manager.
func NewTaskManager() *TaskManager {
	return &TaskManager{
		tasks: make(map[string]*Task),
	}
}

// Create creates a new task and returns it.
func (tm *TaskManager) Create(description, prompt, agentType string) *Task {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.nextID++
	id := fmt.Sprintf("task-%d", tm.nextID)
	now := time.Now()
	task := &Task{
		ID:          id,
		Description: description,
		Prompt:      prompt,
		AgentType:   agentType,
		Status:      TaskPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	tm.tasks[id] = task
	return task
}

// Get retrieves a task by ID.
func (tm *TaskManager) Get(id string) (*Task, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	t, ok := tm.tasks[id]
	return t, ok
}

// List returns all tasks.
func (tm *TaskManager) List() []*Task {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	result := make([]*Task, 0, len(tm.tasks))
	for _, t := range tm.tasks {
		result = append(result, t)
	}
	return result
}

// Complete marks a task as completed with a result.
func (tm *TaskManager) Complete(id string, result string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if t, ok := tm.tasks[id]; ok {
		t.Status = TaskCompleted
		t.Result = result
		t.UpdatedAt = time.Now()
	}
}

// Fail marks a task as failed.
func (tm *TaskManager) Fail(id string, err string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if t, ok := tm.tasks[id]; ok {
		t.Status = TaskFailed
		t.Result = err
		t.UpdatedAt = time.Now()
	}
}

// Stop stops a running task (cancels its context and stops the linked agent).
func (tm *TaskManager) Stop(id string) bool {
	tm.mu.Lock()
	t, ok := tm.tasks[id]
	if !ok {
		tm.mu.Unlock()
		return false
	}
	if t.Status != TaskPending && t.Status != TaskRunning {
		tm.mu.Unlock()
		return false
	}
	t.Status = TaskStopped
	t.UpdatedAt = time.Now()
	agentID := t.agentID
	cancel := t.cancel
	agentStop := tm.agentStop
	tm.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if agentStop != nil && agentID != "" {
		agentStop(agentID)
	}
	return true
}

// SetAgentRuntimeBridge wires the AgentRuntime into the TaskManager.
// This avoids a circular import between tool and agent packages.
func (tm *TaskManager) SetAgentRuntimeBridge(
	runAsync func(ctx context.Context, agentType, prompt string) (agentID string, err error),
	stop func(agentID string) error,
	getState func(agentID string) (state string, result string, ok bool),
) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.agentRunAsync = runAsync
	tm.agentStop = stop
	tm.agentGetState = getState
}

// StartAgent starts an agent for a task via the AgentRuntime bridge.
func (tm *TaskManager) StartAgent(ctx context.Context, id, agentType, prompt string) {
	tm.mu.Lock()
	runAsync := tm.agentRunAsync
	tm.mu.Unlock()

	if runAsync == nil {
		return
	}

	tm.mu.Lock()
	task, ok := tm.tasks[id]
	tm.mu.Unlock()
	if !ok {
		return
	}

	agentCtx, cancel := context.WithCancel(ctx)
	tm.SetCancel(id, cancel)

	agentID, err := runAsync(agentCtx, agentType, prompt)
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if err != nil {
		task.Status = TaskFailed
		task.Result = err.Error()
	} else {
		task.Status = TaskRunning
		task.agentID = agentID
	}
	task.UpdatedAt = time.Now()
}

// GetAgentState returns the current agent state for a task's linked agent.
func (tm *TaskManager) GetAgentState(id string) (state string, result string, ok bool) {
	tm.mu.RLock()
	getState := tm.agentGetState
	task, taskOk := tm.tasks[id]
	tm.mu.RUnlock()

	if !taskOk || task.agentID == "" || getState == nil {
		return "", "", false
	}
	return getState(task.agentID)
}

// SetCancel sets the cancel function for a task.
func (tm *TaskManager) SetCancel(id string, cancel context.CancelFunc) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if t, ok := tm.tasks[id]; ok {
		t.cancel = cancel
	}
}

// --- Tool Implementations ---

// TaskCreateTool creates a new background task.
type TaskCreateTool struct {
	manager *TaskManager
}

func NewTaskCreateTool(tm *TaskManager) *TaskCreateTool {
	return &TaskCreateTool{manager: tm}
}

func (t *TaskCreateTool) Name() string        { return "TaskCreate" }
func (t *TaskCreateTool) Description() string { return "Create a new background task." }
func (t *TaskCreateTool) IsEnabled() bool     { return true }
func (t *TaskCreateTool) IsConcurrencySafe() bool { return false }

func (t *TaskCreateTool) ToolDefinition() ToolDefinition {
	schema := json.RawMessage(`{
  "type": "object",
  "properties": {
    "description": {
      "type": "string",
      "description": "A short description of the task"
    },
    "prompt": {
      "type": "string",
      "description": "The task prompt/instructions"
    },
    "agent_type": {
      "type": "string",
      "description": "The type of agent for this task"
    }
  },
  "required": ["description", "prompt"]
}`)
	return ToolDefinition{Name: "TaskCreate", Description: t.Description(), InputSchema: schema}
}

func (t *TaskCreateTool) Execute(ctx context.Context, input json.RawMessage, toolCtx *ToolContext) (*ToolResult, error) {
	var params struct {
		Description string `json:"description"`
		Prompt      string `json:"prompt"`
		AgentType   string `json:"agent_type"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return &ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}

	task := t.manager.Create(params.Description, params.Prompt, params.AgentType)
	return &ToolResult{Content: fmt.Sprintf("Task created: %s (%s)", task.ID, task.Description)}, nil
}

// TaskGetTool retrieves a task by ID.
type TaskGetTool struct {
	manager *TaskManager
}

func NewTaskGetTool(tm *TaskManager) *TaskGetTool {
	return &TaskGetTool{manager: tm}
}

func (t *TaskGetTool) Name() string        { return "TaskGet" }
func (t *TaskGetTool) Description() string { return "Retrieve a task by ID." }
func (t *TaskGetTool) IsEnabled() bool     { return true }
func (t *TaskGetTool) IsConcurrencySafe() bool { return true }

func (t *TaskGetTool) ToolDefinition() ToolDefinition {
	schema := json.RawMessage(`{
  "type": "object",
  "properties": {
    "task_id": {
      "type": "string",
      "description": "The task ID to retrieve"
    }
  },
  "required": ["task_id"]
}`)
	return ToolDefinition{Name: "TaskGet", Description: t.Description(), InputSchema: schema}
}

func (t *TaskGetTool) Execute(ctx context.Context, input json.RawMessage, toolCtx *ToolContext) (*ToolResult, error) {
	var params struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return &ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}

	task, ok := t.manager.Get(params.TaskID)
	if !ok {
		return &ToolResult{IsError: true, Content: fmt.Sprintf("task %q not found", params.TaskID)}, nil
	}

	data, _ := json.MarshalIndent(task, "", "  ")
	return &ToolResult{Content: string(data)}, nil
}

// TaskListTool lists all tasks.
type TaskListTool struct {
	manager *TaskManager
}

func NewTaskListTool(tm *TaskManager) *TaskListTool {
	return &TaskListTool{manager: tm}
}

func (t *TaskListTool) Name() string        { return "TaskList" }
func (t *TaskListTool) Description() string { return "List all background tasks." }
func (t *TaskListTool) IsEnabled() bool     { return true }
func (t *TaskListTool) IsConcurrencySafe() bool { return true }

func (t *TaskListTool) ToolDefinition() ToolDefinition {
	schema := json.RawMessage(`{
  "type": "object",
  "properties": {}
}`)
	return ToolDefinition{Name: "TaskList", Description: t.Description(), InputSchema: schema}
}

func (t *TaskListTool) Execute(ctx context.Context, input json.RawMessage, toolCtx *ToolContext) (*ToolResult, error) {
	tasks := t.manager.List()
	if len(tasks) == 0 {
		return &ToolResult{Content: "no tasks"}, nil
	}

	var sb strings.Builder
	for _, task := range tasks {
		sb.WriteString(fmt.Sprintf("%s [%s] %s\n", task.ID, task.Status, task.Description))
	}
	return &ToolResult{Content: sb.String()}, nil
}

// TaskStopTool stops a running task.
type TaskStopTool struct {
	manager *TaskManager
}

func NewTaskStopTool(tm *TaskManager) *TaskStopTool {
	return &TaskStopTool{manager: tm}
}

func (t *TaskStopTool) Name() string        { return "TaskStop" }
func (t *TaskStopTool) Description() string { return "Stop a running background task." }
func (t *TaskStopTool) IsEnabled() bool     { return true }
func (t *TaskStopTool) IsConcurrencySafe() bool { return false }

func (t *TaskStopTool) ToolDefinition() ToolDefinition {
	schema := json.RawMessage(`{
  "type": "object",
  "properties": {
    "task_id": {
      "type": "string",
      "description": "The task ID to stop"
    }
  },
  "required": ["task_id"]
}`)
	return ToolDefinition{Name: "TaskStop", Description: t.Description(), InputSchema: schema}
}

func (t *TaskStopTool) Execute(ctx context.Context, input json.RawMessage, toolCtx *ToolContext) (*ToolResult, error) {
	var params struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return &ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}

	if !t.manager.Stop(params.TaskID) {
		return &ToolResult{IsError: true, Content: fmt.Sprintf("task %q not found or already completed", params.TaskID)}, nil
	}
	return &ToolResult{Content: fmt.Sprintf("Task %s stopped", params.TaskID)}, nil
}
