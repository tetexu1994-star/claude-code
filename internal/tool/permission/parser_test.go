package permission

import (
	"testing"
)

func TestParseRuleString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantName  string
		wantCont  *string
		wantErr   bool
	}{
		{"simple tool", "Bash", "Bash", nil, false},
		{"path constraint", "Bash(/tmp/*)", "Bash", strPtr("/tmp/*"), false},
		{"prefix constraint", "Bash(prefix:git)", "Bash", strPtr("prefix:git"), false},
		{"multi prefix", "Bash(prefix:git,pip)", "Bash", strPtr("prefix:git,pip"), false},
		{"domain constraint", "WebFetch(domain:*.com)", "WebFetch", strPtr("domain:*.com"), false},
		{"agent subtype", "Agent(Explore)", "Agent", strPtr("Explore"), false},
		{"mcp server", "mcp__server1", "mcp__server1", nil, false},
		{"mcp server wildcard", "mcp__server1__*", "mcp__server1__*", nil, false},
		{"mcp tool", "mcp__server1__tool1", "mcp__server1__tool1", nil, false},
		{"empty content", "Bash()", "Bash", nil, false},
		{"empty string", "", "", nil, true},
		{"malformed no close", "Bash(/tmp", "", nil, true},
		{"no parens is valid name", "Bash/tmp)", "Bash/tmp)", nil, false},
		{"extra after close", "Bash(/tmp)extra", "", nil, true},
		{"whitespace trimmed", "  Bash  ", "Bash", nil, false},
		{"content with special chars", "Bash(path:*/.git/*)", "Bash", strPtr("path:*/.git/*"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, err := ParseRuleString(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for input %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rule.ToolName != tt.wantName {
				t.Errorf("expected tool name %q, got %q", tt.wantName, rule.ToolName)
			}
			if tt.wantCont == nil {
				if rule.RuleContent != nil {
					t.Errorf("expected nil content, got %q", *rule.RuleContent)
				}
			} else {
				if rule.RuleContent == nil {
					t.Fatalf("expected content %q, got nil", *tt.wantCont)
				}
				if *rule.RuleContent != *tt.wantCont {
					t.Errorf("expected content %q, got %q", *tt.wantCont, *rule.RuleContent)
				}
			}
		})
	}
}

func TestFormatRuleString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		rule *PermissionRule
		want string
	}{
		{"no content", &PermissionRule{ToolName: "Bash"}, "Bash"},
		{"with content", &PermissionRule{ToolName: "Bash", RuleContent: strPtr("/tmp/*")}, "Bash(/tmp/*)"},
		{"mcp", &PermissionRule{ToolName: "mcp__server1"}, "mcp__server1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatRuleString(tt.rule)
			if got != tt.want {
				t.Errorf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestFormatRule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		rule *PermissionRule
		want string
	}{
		{"no content", &PermissionRule{ToolName: "Bash"}, "Bash"},
		{"with content", &PermissionRule{ToolName: "Bash", RuleContent: strPtr("/tmp/*")}, "Bash(/tmp/*)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatRule(tt.rule)
			if got != tt.want {
				t.Errorf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestIsMCPRule(t *testing.T) {
	t.Parallel()

	if !IsMCPRule(&PermissionRule{ToolName: "mcp__server1"}) {
		t.Error("expected mcp rule")
	}
	if IsMCPRule(&PermissionRule{ToolName: "Bash"}) {
		t.Error("bash is not an mcp rule")
	}
}

func TestRuleContentType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		content  *string
		wantType string
	}{
		{nil, ""},
		{strPtr("prefix:git"), "prefix"},
		{strPtr("domain:*.com"), "domain"},
		{strPtr("/tmp/*"), "glob"},
		{strPtr("Explore"), "subtype"},
	}

	for _, tt := range tests {
		rule := &PermissionRule{RuleContent: tt.content}
		got := RuleContentType(rule)
		if got != tt.wantType {
			t.Errorf("content %v: expected %q, got %q", tt.content, tt.wantType, got)
		}
	}
}

func TestMatchToolName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		rule     *PermissionRule
		toolName string
		want     bool
	}{
		{"exact match", &PermissionRule{ToolName: "Bash"}, "Bash", true},
		{"no match", &PermissionRule{ToolName: "Bash"}, "WebFetch", false},
		{"mcp server matches tool", &PermissionRule{ToolName: "mcp__server1"}, "mcp__server1__tool1", true},
		{"mcp wildcard matches tool", &PermissionRule{ToolName: "mcp__server1__*"}, "mcp__server1__tool1", true},
		{"mcp wildcard no match", &PermissionRule{ToolName: "mcp__server1__*"}, "mcp__server2__tool1", false},
		{"mcp exact tool match", &PermissionRule{ToolName: "mcp__server1__tool1"}, "mcp__server1__tool1", true},
		{"mcp exact tool no match", &PermissionRule{ToolName: "mcp__server1__tool1"}, "mcp__server1__tool2", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchToolName(tt.rule, tt.toolName)
			if got != tt.want {
				t.Errorf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestSplitPrefixes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		content string
		want    []string
	}{
		{"prefix:git", []string{"git"}},
		{"prefix:git,pip", []string{"git", "pip"}},
		{"prefix:ls,cat,echo", []string{"ls", "cat", "echo"}},
		{"not a prefix", nil},
		{"prefix:", nil},
	}

	for _, tt := range tests {
		got := SplitPrefixes(tt.content)
		if len(got) != len(tt.want) {
			t.Errorf("%q: expected %v, got %v", tt.content, tt.want, got)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("%q: index %d: expected %q, got %q", tt.content, i, tt.want[i], got[i])
			}
		}
	}
}

func TestDomainPattern(t *testing.T) {
	t.Parallel()

	if got := DomainPattern("domain:*.example.com"); got != "*.example.com" {
		t.Errorf("expected '*.example.com', got %q", got)
	}
	if got := DomainPattern("not-a-domain"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestRuleBehaviorString(t *testing.T) {
	t.Parallel()

	if BehaviorAllow.String() != "allow" {
		t.Errorf("expected 'allow', got %q", BehaviorAllow.String())
	}
	if BehaviorDeny.String() != "deny" {
		t.Errorf("expected 'deny', got %q", BehaviorDeny.String())
	}
	if BehaviorAsk.String() != "ask" {
		t.Errorf("expected 'ask', got %q", BehaviorAsk.String())
	}
}

func TestPermissionContextHelpers(t *testing.T) {
	t.Parallel()

	ctx := NewContext(ModeAccepts)
	if ctx.Mode != ModeAccepts {
		t.Errorf("expected ModeAccepts, got %v", ctx.Mode)
	}
	if ctx.IsBypassed() {
		t.Error("accepts mode should not be bypassed")
	}
	if ctx.IsPlanMode() {
		t.Error("accepts mode should not be plan")
	}

	ctx.AddAllowRule(SourceUser, "Bash", nil)
	ctx.AddDenyRule(SourceUser, "write_file", strPtr("/etc/*"))
	ctx.AddAskRule(SourceUser, "WebFetch", nil)

	if len(ctx.AlwaysAllowRules) != 1 {
		t.Errorf("expected 1 allow rule, got %d", len(ctx.AlwaysAllowRules))
	}
	if len(ctx.AlwaysDenyRules) != 1 {
		t.Errorf("expected 1 deny rule, got %d", len(ctx.AlwaysDenyRules))
	}
	if len(ctx.AlwaysAskRules) != 1 {
		t.Errorf("expected 1 ask rule, got %d", len(ctx.AlwaysAskRules))
	}

	bypassCtx := NewContext(ModeBypassPermissions)
	if !bypassCtx.IsBypassed() {
		t.Error("bypass permissions should report bypassed")
	}

	planCtx := NewContext(ModePlan)
	if !planCtx.IsPlanMode() {
		t.Error("plan mode should report as plan")
	}
}

func strPtr(s string) *string {
	return &s
}
