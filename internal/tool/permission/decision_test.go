package permission

import (
	"testing"
)

func TestDecideNilContext(t *testing.T) {
	t.Parallel()

	decision := Decide(nil, "bash", "echo hello")
	if decision.Behavior != BehaviorAllow {
		t.Errorf("nil context should allow, got %v", decision.Behavior)
	}
}

func TestDecideBypassPermissions(t *testing.T) {
	t.Parallel()

	ctx := NewContext(ModeBypassPermissions)
	decision := Decide(ctx, "bash", "echo hello")
	if decision.Behavior != BehaviorAllow {
		t.Errorf("bypass mode should allow, got %v", decision.Behavior)
	}
}

func TestDecideDenyRule(t *testing.T) {
	t.Parallel()

	ctx := NewContext(ModeAccepts)
	ctx.AddDenyRule(SourceUser, "bash", nil)

	decision := Decide(ctx, "bash", "echo hello")
	if decision.Behavior != BehaviorDeny {
		t.Errorf("deny rule should deny, got %v", decision.Behavior)
	}
}

func TestDecideAllowRule(t *testing.T) {
	t.Parallel()

	ctx := NewContext(ModeAccepts)
	ctx.AddAllowRule(SourceUser, "bash", nil)

	decision := Decide(ctx, "bash", "echo hello")
	if decision.Behavior != BehaviorAllow {
		t.Errorf("allow rule should allow, got %v", decision.Behavior)
	}
}

func TestDecideAskRule(t *testing.T) {
	t.Parallel()

	ctx := NewContext(ModeAccepts)
	ctx.AddAskRule(SourceUser, "WebFetch", nil)

	decision := Decide(ctx, "WebFetch", "https://example.com")
	if decision.Behavior != BehaviorAsk {
		t.Errorf("ask rule should ask, got %v", decision.Behavior)
	}
}

func TestDecidePlanMode(t *testing.T) {
	t.Parallel()

	ctx := NewContext(ModePlan)

	// Plan mode allows read tools
	decision := Decide(ctx, "read_file", "file.txt")
	if decision.Behavior != BehaviorAllow {
		t.Errorf("plan mode should allow read_file, got %v", decision.Behavior)
	}

	// Plan mode blocks write tools
	decision = Decide(ctx, "write_file", "file.txt")
	if decision.Behavior != BehaviorDeny {
		t.Errorf("plan mode should block write_file, got %v", decision.Behavior)
	}

	decision = Decide(ctx, "bash", "echo hello")
	if decision.Behavior != BehaviorDeny {
		t.Errorf("plan mode should block bash, got %v", decision.Behavior)
	}
}

func TestDecideAcceptsFallback(t *testing.T) {
	t.Parallel()

	ctx := NewContext(ModeAccepts)
	decision := Decide(ctx, "some_tool", "input")
	if decision.Behavior != BehaviorAllow {
		t.Errorf("accepts mode should fall back to allow, got %v", decision.Behavior)
	}
}

func TestDecideAutoMode(t *testing.T) {
	t.Parallel()

	ctx := NewContext(ModeAuto)

	// Simple tools auto-allowed
	decision := Decide(ctx, "read_file", "file.txt")
	if decision.Behavior != BehaviorAllow {
		t.Errorf("auto mode should allow read_file, got %v", decision.Behavior)
	}

	// Complex tools ask
	decision = Decide(ctx, "bash", "echo hello")
	if decision.Behavior != BehaviorAsk {
		t.Errorf("auto mode should ask for bash, got %v", decision.Behavior)
	}
}

func TestDecideEditMode(t *testing.T) {
	t.Parallel()

	ctx := NewContext(ModeEdit)
	decision := Decide(ctx, "write_file", "content")
	if decision.Behavior != BehaviorAsk {
		t.Errorf("edit mode should ask, got %v", decision.Behavior)
	}
}

func TestDecideChatMode(t *testing.T) {
	t.Parallel()

	ctx := NewContext(ModeChat)
	decision := Decide(ctx, "bash", "echo hello")
	if decision.Behavior != BehaviorDeny {
		t.Errorf("chat mode should deny all tools, got %v", decision.Behavior)
	}
}

func TestDecideDenyTakesPriority(t *testing.T) {
	t.Parallel()

	// Deny + Allow both match → deny wins
	ctx := NewContext(ModeAccepts)
	ctx.AddAllowRule(SourceUser, "bash", nil)
	ctx.AddDenyRule(SourceUser, "bash", nil)

	decision := Decide(ctx, "bash", "echo hello")
	if decision.Behavior != BehaviorDeny {
		t.Errorf("deny should take priority over allow, got %v", decision.Behavior)
	}
}

func TestDecidePrefixMatching(t *testing.T) {
	t.Parallel()

	// Allow rule with prefix constraint
	ctx := NewContext(ModeAccepts)
	ctx.AddAllowRule(SourceUser, "bash", strPtr("prefix:git"))

	// Bash with git prefix matches
	decision := Decide(ctx, "bash", "git status")
	if decision.Behavior != BehaviorAllow {
		t.Errorf("bash with git prefix should match allow rule, got %v", decision.Behavior)
	}

	// Bash without matching prefix falls through
	decision = Decide(ctx, "bash", "rm -rf /")
	if decision.Behavior != BehaviorAllow {
		t.Errorf("bash without matching prefix falls through to accepts → allow, got %v", decision.Behavior)
	}
}

func TestDecideDomainMatching(t *testing.T) {
	t.Parallel()

	// Domain matching
	ctx := NewContext(ModeAccepts)
	ctx.AddAllowRule(SourceUser, "WebFetch", strPtr("domain:*.example.com"))

	decision := Decide(ctx, "WebFetch", "https://api.example.com/data")
	if decision.Behavior != BehaviorAllow {
		t.Errorf("domain match should allow, got %v", decision.Behavior)
	}

	// Non-matching domain falls through
	decision = Decide(ctx, "WebFetch", "https://evil.com")
	if decision.Behavior != BehaviorAllow {
		t.Errorf("non-matching domain falls through → allow in accepts, got %v", decision.Behavior)
	}
}

func TestIsSimpleTool(t *testing.T) {
	t.Parallel()

	if !isSimpleTool("read_file") {
		t.Error("read_file should be simple")
	}
	if !isSimpleTool("Glob") {
		t.Error("Glob should be simple")
	}
	if !isSimpleTool("Grep") {
		t.Error("Grep should be simple")
	}
	if isSimpleTool("bash") {
		t.Error("bash should not be simple")
	}
	if isSimpleTool("write_file") {
		t.Error("write_file should not be simple")
	}
}

func TestMatchDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		pattern string
		input   string
		want    bool
	}{
		{"*", "anything", true},
		{"", "anything", true},
		{"*.example.com", "api.example.com", true},
		{"*.example.com", "https://api.example.com/path", true},
		{"*.example.com", "evil.com", false},
		{"example.com", "example.com", true},
		{"example", "example.com", true},
	}

	for _, tt := range tests {
		got := matchDomain(tt.pattern, tt.input)
		if got != tt.want {
			t.Errorf("matchDomain(%q, %q): expected %v, got %v", tt.pattern, tt.input, tt.want, got)
		}
	}
}

func TestDecisionMessages(t *testing.T) {
	t.Parallel()

	// Deny message has useful info
	ctx := NewContext(ModeAccepts)
	ctx.AddDenyRule(SourcePolicy, "bash", nil)
	decision := Decide(ctx, "bash", "echo hello")
	if decision.Message == "" {
		t.Error("deny decision should have a message")
	}
	if decision.DecisionReason == "" {
		t.Error("deny decision should have a reason")
	}

	// Ask message includes tool info
	ctx2 := NewContext(ModeAuto)
	decision2 := Decide(ctx2, "bash", "echo hello")
	if decision2.Behavior != BehaviorAsk {
		t.Errorf("expected ask, got %v", decision2.Behavior)
	}
}
