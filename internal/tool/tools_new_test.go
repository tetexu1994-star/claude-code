package tool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// --- WebFetch tests ---

func TestWebFetchTool_Execute(t *testing.T) {
	// Skip in short mode since it makes an external HTTP call.
	if testing.Short() {
		t.Skip("skipping WebFetch test in short mode")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html><body><h1>Hello</h1><p>world</p></body></html>"))
	}))
	defer srv.Close()

	tool := &WebFetchTool{}
	input, _ := json.Marshal(map[string]string{
		"url":    srv.URL,
		"prompt": "extract content",
	})

	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Content)
	}
	if result.Content == "" {
		t.Error("expected non-empty content")
	}
}

func TestWebFetchTool_BadURL(t *testing.T) {
	t.Parallel()

	tool := &WebFetchTool{}
	input, _ := json.Marshal(map[string]string{
		"url":    "://invalid-url",
		"prompt": "test",
	})

	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for bad URL")
	}
}

func TestWebFetchTool_MissingURL(t *testing.T) {
	t.Parallel()

	tool := &WebFetchTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for missing url")
	}
}

func TestWebFetchTool_Definition(t *testing.T) {
	t.Parallel()

	tool := &WebFetchTool{}
	if tool.Name() != "WebFetch" {
		t.Errorf("expected WebFetch, got %s", tool.Name())
	}
	if !tool.IsEnabled() {
		t.Error("expected enabled")
	}
	if !tool.IsConcurrencySafe() {
		t.Error("expected concurrency safe")
	}

	td := tool.ToolDefinition()
	if td.Name != "WebFetch" {
		t.Errorf("expected WebFetch, got %s", td.Name)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(td.InputSchema, &m); err != nil {
		t.Errorf("invalid schema: %v", err)
	}
}

// --- WebSearch tests ---

func TestWebSearchTool_Definition(t *testing.T) {
	t.Parallel()

	tool := &WebSearchTool{}
	if tool.Name() != "WebSearch" {
		t.Errorf("expected WebSearch, got %s", tool.Name())
	}
	if !tool.IsEnabled() {
		t.Error("expected enabled")
	}
	if !tool.IsConcurrencySafe() {
		t.Error("expected concurrency safe")
	}
}

func TestWebSearchTool_MissingQuery(t *testing.T) {
	t.Parallel()

	tool := &WebSearchTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for missing query")
	}
}

// --- TodoWrite tests ---

func TestTodoWriteTool_Execute(t *testing.T) {
	t.Parallel()

	tool := NewTodoWriteTool()
	todos := []TodoItem{
		{Content: "Task 1", Status: "pending", ActiveForm: "Doing task 1"},
		{Content: "Task 2", Status: "completed", ActiveForm: "Doing task 2"},
	}

	input, _ := json.Marshal(map[string]interface{}{
		"todos": todos,
	})

	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Content)
	}

	// Verify file was written.
	data, err := os.ReadFile(tool.storePath)
	if err != nil {
		t.Fatalf("failed to read todos file: %v", err)
	}
	var saved []TodoItem
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("failed to unmarshal todos: %v", err)
	}
	if len(saved) != 2 {
		t.Errorf("expected 2 todos, got %d", len(saved))
	}
}

func TestTodoWriteTool_Definition(t *testing.T) {
	t.Parallel()

	tool := NewTodoWriteTool()
	if tool.Name() != "TodoWrite" {
		t.Errorf("expected TodoWrite, got %s", tool.Name())
	}
	if !tool.IsEnabled() {
		t.Error("expected enabled")
	}
	if tool.IsConcurrencySafe() {
		t.Error("expected not concurrency safe (writes to file)")
	}
}

// --- Agent tests ---

func TestAgentTool_Execute(t *testing.T) {
	t.Parallel()

	tm := NewTaskManager()
	tool := NewAgentTool(tm)

	input, _ := json.Marshal(map[string]string{
		"description":   "Test agent",
		"prompt":        "Search for files",
		"subagent_type": "Explore",
	})

	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Content)
	}

	// Verify task was created.
	tasks := tm.List()
	if len(tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(tasks))
	}
}

func TestAgentTool_MissingFields(t *testing.T) {
	t.Parallel()

	tm := NewTaskManager()
	tool := NewAgentTool(tm)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for missing fields")
	}
}

func TestAgentTool_Definition(t *testing.T) {
	t.Parallel()

	tm := NewTaskManager()
	tool := NewAgentTool(tm)
	if tool.Name() != "Agent" {
		t.Errorf("expected Agent, got %s", tool.Name())
	}
	if !tool.IsEnabled() {
		t.Error("expected enabled")
	}
	if tool.IsConcurrencySafe() {
		t.Error("expected not concurrency safe")
	}
}

// --- Task tools tests ---

func TestTaskCreateTool_Execute(t *testing.T) {
	t.Parallel()

	tm := NewTaskManager()
	tool := NewTaskCreateTool(tm)

	input, _ := json.Marshal(map[string]string{
		"description": "Test task",
		"prompt":      "Do something",
		"agent_type":  "general-purpose",
	})

	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Content)
	}

	tasks := tm.List()
	if len(tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Description != "Test task" {
		t.Errorf("expected 'Test task', got %q", tasks[0].Description)
	}
}

func TestTaskGetTool_Execute(t *testing.T) {
	t.Parallel()

	tm := NewTaskManager()
	task := tm.Create("my task", "do it", "test")

	tool := NewTaskGetTool(tm)
	input, _ := json.Marshal(map[string]string{
		"task_id": task.ID,
	})

	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Content)
	}
}

func TestTaskGetTool_NotFound(t *testing.T) {
	t.Parallel()

	tm := NewTaskManager()
	tool := NewTaskGetTool(tm)

	input, _ := json.Marshal(map[string]string{
		"task_id": "nonexistent",
	})

	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for nonexistent task")
	}
}

func TestTaskListTool_Execute(t *testing.T) {
	t.Parallel()

	tm := NewTaskManager()
	tm.Create("task a", "do a", "test")
	tm.Create("task b", "do b", "test")

	tool := NewTaskListTool(tm)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Content)
	}
	if result.Content == "no tasks" {
		t.Error("expected tasks in list")
	}
}

func TestTaskListTool_Empty(t *testing.T) {
	t.Parallel()

	tm := NewTaskManager()
	tool := NewTaskListTool(tm)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != "no tasks" {
		t.Errorf("expected 'no tasks', got %q", result.Content)
	}
}

func TestTaskStopTool_Execute(t *testing.T) {
	t.Parallel()

	tm := NewTaskManager()
	task := tm.Create("task to stop", "do it", "test")

	tool := NewTaskStopTool(tm)
	input, _ := json.Marshal(map[string]string{
		"task_id": task.ID,
	})

	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Content)
	}

	task, ok := tm.Get(task.ID)
	if !ok || task.Status != TaskStopped {
		t.Errorf("expected task status 'stopped', got %q", task.Status)
	}
}

func TestTaskStopTool_NotFound(t *testing.T) {
	t.Parallel()

	tm := NewTaskManager()
	tool := NewTaskStopTool(tm)

	input, _ := json.Marshal(map[string]string{
		"task_id": "nonexistent",
	})

	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for nonexistent task")
	}
}

// --- TaskManager tests ---

func TestTaskManager_Create(t *testing.T) {
	t.Parallel()

	tm := NewTaskManager()
	task := tm.Create("test desc", "test prompt", "test-agent")
	if task.ID == "" {
		t.Error("expected task ID")
	}
	if task.Description != "test desc" {
		t.Errorf("expected 'test desc', got %q", task.Description)
	}
}

func TestTaskManager_Get(t *testing.T) {
	t.Parallel()

	tm := NewTaskManager()
	created := tm.Create("my task", "do it", "test")

	got, ok := tm.Get(created.ID)
	if !ok {
		t.Fatal("expected to find task")
	}
	if got.ID != created.ID {
		t.Error("ID mismatch")
	}
}

func TestTaskManager_Complete(t *testing.T) {
	t.Parallel()

	tm := NewTaskManager()
	task := tm.Create("my task", "do it", "test")
	tm.Complete(task.ID, "done")

	got, _ := tm.Get(task.ID)
	if got.Status != TaskCompleted {
		t.Errorf("expected completed, got %s", got.Status)
	}
	if got.Result != "done" {
		t.Errorf("expected result 'done', got %q", got.Result)
	}
}

func TestTaskManager_Stop(t *testing.T) {
	t.Parallel()

	tm := NewTaskManager()
	task := tm.Create("my task", "do it", "test")

	if !tm.Stop(task.ID) {
		t.Error("expected stop to succeed")
	}

	got, _ := tm.Get(task.ID)
	if got.Status != TaskStopped {
		t.Errorf("expected stopped, got %s", got.Status)
	}

	// Can't stop a completed task.
	tm.Create("task2", "do", "test")
	tm.Complete("task2", "done")
	if tm.Stop("task2") {
		t.Error("expected stop to fail for completed task")
	}
}

// --- AgentTool Bridge Tests ---

func TestAgentTool_WithBridge(t *testing.T) {
	tm := NewTaskManager()
	agentTool := NewAgentTool(tm)

	agentTool.SetRuntimeBridge(
		func(ctx context.Context, agentType, prompt, model, provider string) (string, int, int, error) {
			return "Bridge response for " + agentType, 5, 10, nil
		},
		func(ctx context.Context, agentType, prompt string) (string, time.Duration, float64, error) {
			return "MoA response", time.Millisecond * 100, 0.001, nil
		},
	)

	input, _ := json.Marshal(map[string]string{
		"description":   "Bridge test",
		"prompt":        "Test prompt",
		"subagent_type": "general",
	})

	result, err := agentTool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Bridge response") {
		t.Errorf("result should contain bridge response: %s", result.Content)
	}

	task, ok := tm.Get(tm.List()[0].ID)
	if !ok || task.Status != TaskCompleted {
		t.Errorf("expected task completed, got status=%s", task.Status)
	}
}

func TestAgentTool_MoAWithBridge(t *testing.T) {
	tm := NewTaskManager()
	agentTool := NewAgentTool(tm)

	agentTool.SetRuntimeBridge(
		func(ctx context.Context, agentType, prompt, model, provider string) (string, int, int, error) {
			return "inproc", 1, 2, nil
		},
		func(ctx context.Context, agentType, prompt string) (string, time.Duration, float64, error) {
			return "MoA aggregated", time.Millisecond * 50, 0.002, nil
		},
	)

	input, _ := json.Marshal(map[string]string{
		"description":   "MoA test",
		"prompt":        "Analyze code",
		"subagent_type": "moa",
	})

	result, err := agentTool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "MoA aggregated") {
		t.Errorf("result should contain MoA response: %s", result.Content)
	}
}

func TestAgentTool_NoBridgeFallback(t *testing.T) {
	tm := NewTaskManager()
	agentTool := NewAgentTool(tm)

	input, _ := json.Marshal(map[string]string{
		"description":   "No bridge test",
		"prompt":        "Test",
		"subagent_type": "general",
	})

	result, err := agentTool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Agent task created") {
		t.Errorf("expected stub message, got: %s", result.Content)
	}
}

func TestAgentTool_ModelProviderParams(t *testing.T) {
	tm := NewTaskManager()
	agentTool := NewAgentTool(tm)

	var capturedModel, capturedProvider string
	agentTool.SetRuntimeBridge(
		func(ctx context.Context, agentType, prompt, model, provider string) (string, int, int, error) {
			capturedModel = model
			capturedProvider = provider
			return "ok", 5, 10, nil
		},
		nil,
	)

	input, _ := json.Marshal(map[string]string{
		"description":   "Model override test",
		"prompt":        "Test",
		"subagent_type": "general",
		"model":         "gpt-4o",
		"provider":      "openai",
	})

	result, err := agentTool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Content)
	}
	if capturedModel != "gpt-4o" {
		t.Errorf("captured model = %q, want %q", capturedModel, "gpt-4o")
	}
	if capturedProvider != "openai" {
		t.Errorf("captured provider = %q, want %q", capturedProvider, "openai")
	}
}

func TestAgentTool_ExternalType(t *testing.T) {
	tm := NewTaskManager()
	agentTool := NewAgentTool(tm)

	agentTool.SetRuntimeBridge(
		func(ctx context.Context, agentType, prompt, model, provider string) (string, int, int, error) {
			return "should not be called", 0, 0, nil
		},
		nil,
	)

	input, _ := json.Marshal(map[string]string{
		"description":   "External test",
		"prompt":        "Run something",
		"subagent_type": "external.claude-code",
	})

	result, err := agentTool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return error because backend is not configured.
	if !result.IsError {
		t.Errorf("expected error for missing external backend, got: %s", result.Content)
	}
}

func TestAgentTool_RegisterBackend(t *testing.T) {
	tm := NewTaskManager()
	agentTool := NewAgentTool(tm)

	backend := agentTool.Backend("nonexistent")
	if backend != nil {
		t.Error("expected nil for unregistered backend")
	}
}
