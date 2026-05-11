package definition

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tetexu/tlaude-code/internal/memory"
)

func TestLoadAgentMemoryPrompt_UserScope(t *testing.T) {
	prompt := LoadAgentMemoryPrompt("test-agent", MemUser)
	if prompt == "" {
		t.Error("expected non-empty prompt")
	}
	// User scope note should be present.
	if !strings.Contains(prompt, "user-scope") {
		t.Error("expected user-scope note in prompt")
	}
}

func TestLoadAgentMemoryPrompt_ProjectScope(t *testing.T) {
	prompt := LoadAgentMemoryPrompt("test-agent", MemProject)
	if prompt == "" {
		t.Error("expected non-empty prompt")
	}
	if !strings.Contains(prompt, "project-scope") {
		t.Error("expected project-scope note in prompt")
	}
}

func TestLoadAgentMemoryPrompt_LocalScope(t *testing.T) {
	prompt := LoadAgentMemoryPrompt("test-agent", MemLocal)
	if prompt == "" {
		t.Error("expected non-empty prompt")
	}
	if !strings.Contains(prompt, "local-scope") {
		t.Error("expected local-scope note in prompt")
	}
}

func TestLoadAgentMemoryPrompt_DefaultScope(t *testing.T) {
	// Empty/invalid scope should default to user.
	prompt := LoadAgentMemoryPrompt("test-agent", MemoryScope("invalid"))
	if prompt == "" {
		t.Error("expected non-empty prompt for default scope")
	}
}

func TestGetAgentMemoryDir(t *testing.T) {
	tests := []struct {
		agentType string
		scope     MemoryScope
		wantSuffix string
	}{
		{"general-purpose", MemUser, "agent-memory/general-purpose/"},
		{"explore", MemProject, "agent-memory/explore/"},
		{"plan", MemLocal, "agent-memory-local/plan/"},
	}

	for _, tt := range tests {
		t.Run(tt.agentType+"/"+string(tt.scope), func(t *testing.T) {
			dir := GetAgentMemoryDir(tt.agentType, tt.scope)
			if !strings.HasSuffix(dir, tt.wantSuffix) {
				t.Errorf("GetAgentMemoryDir(%q, %q) = %q, want suffix %q",
					tt.agentType, tt.scope, dir, tt.wantSuffix)
			}
		})
	}
}

func TestLoadAgentMemoryPrompt_IntegratesWithMemDir(t *testing.T) {
	// Create a temp directory and verify the memory system creates MEMORY.md.
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "agent-memory", "test-agent")
	os.MkdirAll(agentDir, 0755)

	store := memory.NewStore(agentDir)
	store.EnsureDir()

	// Write a memory file.
	_, err := store.Write("test-memory", "A test memory", "user", `---
name: test-memory
description: A test memory
type: user
---
Remember this for testing.`)
	if err != nil {
		t.Fatalf("store.Write: %v", err)
	}

	// Load memory prompt via our wrapper.
	prompt := LoadAgentMemoryPrompt("test-agent", MemUser)
	if prompt == "" {
		t.Error("expected non-empty prompt")
	}
	if !strings.Contains(prompt, "# auto memory") {
		t.Error("expected memory prompt header")
	}
}
