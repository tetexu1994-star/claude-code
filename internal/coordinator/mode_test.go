package coordinator

import (
	"os"
	"testing"
)

func TestIsCoordinatorMode_DefaultDisabled(t *testing.T) {
	// Ensure env var is not set
	os.Unsetenv(CoordinatorEnvVar)
	if IsCoordinatorMode() {
		t.Error("coordinator mode should be disabled by default")
	}
}

func TestIsCoordinatorMode_Enabled(t *testing.T) {
	os.Setenv(CoordinatorEnvVar, "1")
	defer os.Unsetenv(CoordinatorEnvVar)
	if !IsCoordinatorMode() {
		t.Error("coordinator mode should be enabled when TLAUDE_CODE_COORDINATOR_MODE=1")
	}
}

func TestIsCoordinatorMode_EmptyString(t *testing.T) {
	os.Setenv(CoordinatorEnvVar, "")
	defer os.Unsetenv(CoordinatorEnvVar)
	if IsCoordinatorMode() {
		t.Error("coordinator mode should not be enabled with empty string")
	}
}

func TestMatchSessionMode_NormalMatching(t *testing.T) {
	// Starting in normal mode (env not set)
	os.Unsetenv(CoordinatorEnvVar)

	tests := []struct {
		name         string
		sessionMode  string
		wantSwitched bool
		wantWarning  string
		checkFunc    func() bool // what IsCoordinatorMode() should return after
	}{
		{
			name:         "empty session mode",
			sessionMode:  "",
			wantSwitched: false,
			wantWarning:  "",
			checkFunc:    func() bool { return false },
		},
		{
			name:         "matching normal mode",
			sessionMode:  "normal",
			wantSwitched: false,
			wantWarning:  "",
			checkFunc:    func() bool { return false },
		},
		{
			name:         "switch to coordinator for resumed session",
			sessionMode:  "coordinator",
			wantSwitched: true,
			wantWarning:  "Entered coordinator mode to match resumed session.",
			checkFunc:    func() bool { return true },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset env before each test
			os.Unsetenv(CoordinatorEnvVar)

			warning, switched := MatchSessionMode(tt.sessionMode)

			if switched != tt.wantSwitched {
				t.Errorf("switched = %v, want %v", switched, tt.wantSwitched)
			}
			if warning != tt.wantWarning {
				t.Errorf("warning = %q, want %q", warning, tt.wantWarning)
			}
			if got := IsCoordinatorMode(); got != tt.checkFunc() {
				t.Errorf("IsCoordinatorMode() = %v, want %v", got, tt.checkFunc())
			}
		})
	}
}

func TestMatchSessionMode_CoordinatorToNormal(t *testing.T) {
	// Start in coordinator mode, session says normal
	os.Setenv(CoordinatorEnvVar, "1")
	defer os.Unsetenv(CoordinatorEnvVar)

	warning, switched := MatchSessionMode("normal")

	if !switched {
		t.Error("should have switched")
	}
	if warning != "Exited coordinator mode to match resumed session." {
		t.Errorf("warning = %q", warning)
	}
	if IsCoordinatorMode() {
		t.Error("should now be in normal mode")
	}
}

func TestMatchSessionMode_CoordinatorToCoordinator(t *testing.T) {
	// Already in coordinator mode, session also coordinator
	os.Setenv(CoordinatorEnvVar, "1")
	defer os.Unsetenv(CoordinatorEnvVar)

	warning, switched := MatchSessionMode("coordinator")

	if switched {
		t.Error("should not have switched")
	}
	if warning != "" {
		t.Errorf("warning should be empty, got %q", warning)
	}
	if !IsCoordinatorMode() {
		t.Error("should still be in coordinator mode")
	}
}

func TestWorkerTools_SimpleMode(t *testing.T) {
	tools := WorkerTools(true)
	expected := []string{"Bash", "Read", "Edit"}
	if len(tools) != len(expected) {
		t.Fatalf("got %d tools, want %d", len(tools), len(expected))
	}
	for i, want := range expected {
		if i >= len(tools) || tools[i] != want {
			t.Errorf("tools[%d] = %q, want %q", i, tools[i], want)
		}
	}
}

func TestWorkerTools_FullMode(t *testing.T) {
	tools := WorkerTools(false)

	// Should include core tools
	mustHave := []string{"Agent", "Bash", "Edit", "Glob", "Grep", "Read", "Write"}
	for _, want := range mustHave {
		found := false
		for _, tool := range tools {
			if tool == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("full mode should include %q", want)
		}
	}

	// Should NOT include internal tools
	mustNotHave := map[string]bool{
		"TeamCreate":      true,
		"TeamDelete":      true,
		"SendMessage":     true,
		"SyntheticOutput": true,
	}
	for _, tool := range tools {
		if mustNotHave[tool] {
			t.Errorf("full mode should NOT include internal tool %q", tool)
		}
	}
}

func TestGetCoordinatorUserContext_NotEnabled(t *testing.T) {
	os.Unsetenv(CoordinatorEnvVar)
	ctx := GetCoordinatorUserContext(nil, "")
	if ctx != nil {
		t.Error("should return nil when coordinator mode is not enabled")
	}
}

func TestGetCoordinatorUserContext_Enabled(t *testing.T) {
	os.Setenv(CoordinatorEnvVar, "1")
	defer os.Unsetenv(CoordinatorEnvVar)

	ctx := GetCoordinatorUserContext(nil, "")
	if ctx == nil {
		t.Fatal("should return context when coordinator mode is enabled")
	}

	content, ok := ctx["workerToolsContext"]
	if !ok {
		t.Fatal("should have workerToolsContext key")
	}
	if content == "" {
		t.Error("workerToolsContext should not be empty")
	}
}

func TestGetCoordinatorUserContext_WithMCP(t *testing.T) {
	os.Setenv(CoordinatorEnvVar, "1")
	defer os.Unsetenv(CoordinatorEnvVar)

	mcpClients := []MCPServerInfo{
		{Name: "filesystem"},
		{Name: "github"},
	}

	ctx := GetCoordinatorUserContext(mcpClients, "")
	content := ctx["workerToolsContext"]
	if content == "" {
		t.Fatal("should have content")
	}
}

func TestGetCoordinatorUserContext_WithScratchpad(t *testing.T) {
	os.Setenv(CoordinatorEnvVar, "1")
	defer os.Unsetenv(CoordinatorEnvVar)

	ctx := GetCoordinatorUserContext(nil, "/tmp/scratchpad")
	if ctx == nil {
		t.Fatal("should return context")
	}
	content := ctx["workerToolsContext"]
	if content == "" {
		t.Fatal("should have content")
	}
}

func TestIsSimpleMode_Default(t *testing.T) {
	os.Unsetenv("TLAUDE_CODE_SIMPLE")
	if IsSimpleMode() {
		t.Error("simple mode should be disabled by default")
	}
}

func TestIsSimpleMode_Enabled(t *testing.T) {
	os.Setenv("TLAUDE_CODE_SIMPLE", "1")
	defer os.Unsetenv("TLAUDE_CODE_SIMPLE")
	if !IsSimpleMode() {
		t.Error("simple mode should be enabled with TLAUDE_CODE_SIMPLE=1")
	}
}
