// Package permission defines types and logic for the Claude Code-compatible
// permission system, including rules, contexts, and decision-making.
package permission

import "fmt"

// PermissionMode is the top-level permission posture for a session.
type PermissionMode string

const (
	ModeAccepts           PermissionMode = "accepts"
	ModePlan              PermissionMode = "plan"
	ModeEdit              PermissionMode = "edit"
	ModeBypassPermissions PermissionMode = "bypass_permissions"
	ModeAuto              PermissionMode = "auto"
	ModeChat              PermissionMode = "chat"
)

// RuleBehavior determines what happens when a rule matches a tool invocation.
type RuleBehavior int

const (
	BehaviorAllow RuleBehavior = iota
	BehaviorDeny
	BehaviorAsk
)

func (b RuleBehavior) String() string {
	switch b {
	case BehaviorAllow:
		return "allow"
	case BehaviorDeny:
		return "deny"
	case BehaviorAsk:
		return "ask"
	default:
		return "unknown"
	}
}

// RuleSource tracks where a permission rule originated.
type RuleSource int

const (
	SourceFlag    RuleSource = iota // --permission-* CLI flag
	SourcePolicy                    // policy file (CLAUDE.md, etc.)
	SourceCLIArg                    // --permission-prompt-tool or similar
	SourceUser                      // user global config
	SourceProject                   // project-level config
	SourceLocal                     // local settings
	SourceSession                   // session-level override
	SourceCommand                   // slash-command /config permission
)

// PermissionRule defines a single permission entry:
// ToolName optionally paired with a parenthesized content constraint.
//
// Examples:
//
//	Bash              → entire Bash tool
//	Bash(/tmp/*)      → Bash restricted to /tmp paths
//	Bash(prefix:git)  → Bash with git-prefixed commands only
//	WebFetch(domain:*.com) → WebFetch for *.com domains
//	mcp__server1      → entire MCP server
//	mcp__server1__*   → all tools on MCP server
type PermissionRule struct {
	Source      RuleSource
	Behavior    RuleBehavior
	ToolName    string  // e.g. "Bash", "WebFetch", "mcp__server1"
	RuleContent *string // optional content inside parentheses (nil if none)
}

// MatchResult describes how a rule matched a tool call.
type MatchResult struct {
	Matched       bool
	Specificity   int // higher = more specific match
	Rule          *PermissionRule
	DecisionReason string
}

// PermissionContext captures the full permission state for a tool execution.
type PermissionContext struct {
	Mode               PermissionMode
	AlwaysAllowRules   []PermissionRule
	AlwaysDenyRules    []PermissionRule
	AlwaysAskRules     []PermissionRule
	ConsecutiveDenials int
}

// PermissionDecision is the outcome of checking a tool invocation
// against the permission rules and mode.
type PermissionDecision struct {
	Behavior       RuleBehavior
	Message        string
	DecisionReason string
}

// NewContext creates a PermissionContext with sensible defaults.
func NewContext(mode PermissionMode) *PermissionContext {
	return &PermissionContext{
		Mode: mode,
	}
}

// addRule is a helper to append a rule to the correct slice.
func (pc *PermissionContext) addRule(rule PermissionRule) {
	switch rule.Behavior {
	case BehaviorAllow:
		pc.AlwaysAllowRules = append(pc.AlwaysAllowRules, rule)
	case BehaviorDeny:
		pc.AlwaysDenyRules = append(pc.AlwaysDenyRules, rule)
	case BehaviorAsk:
		pc.AlwaysAskRules = append(pc.AlwaysAskRules, rule)
	}
}

// AddAllowRule adds an allow rule with a given source and tool name.
func (pc *PermissionContext) AddAllowRule(source RuleSource, toolName string, content *string) {
	pc.addRule(PermissionRule{Source: source, Behavior: BehaviorAllow, ToolName: toolName, RuleContent: content})
}

// AddDenyRule adds a deny rule with a given source and tool name.
func (pc *PermissionContext) AddDenyRule(source RuleSource, toolName string, content *string) {
	pc.addRule(PermissionRule{Source: source, Behavior: BehaviorDeny, ToolName: toolName, RuleContent: content})
}

// AddAskRule adds an ask rule with a given source and tool name.
func (pc *PermissionContext) AddAskRule(source RuleSource, toolName string, content *string) {
	pc.addRule(PermissionRule{Source: source, Behavior: BehaviorAsk, ToolName: toolName, RuleContent: content})
}

// IsBypassed returns true if permissions are disabled for this context.
func (pc *PermissionContext) IsBypassed() bool {
	return pc.Mode == ModeBypassPermissions
}

// IsPlanMode returns true if the session is in plan-only mode.
func (pc *PermissionContext) IsPlanMode() bool {
	return pc.Mode == ModePlan
}

// FormatRule returns the string representation of a rule for display.
func FormatRule(rule *PermissionRule) string {
	if rule.RuleContent != nil {
		return fmt.Sprintf("%s(%s)", rule.ToolName, *rule.RuleContent)
	}
	return rule.ToolName
}
