package permission

import "strings"

// Decide evaluates whether a tool invocation is permitted given the
// permission context. It implements the decision flow:
//
//  1. Check deny rules → reject
//  2. Auto mode + BashTool → simple classifier check
//  3. Check allow rules → auto-approve
//  4. Check ask rules → ask user
//  5. Fall back based on PermissionMode
func Decide(ctx *PermissionContext, toolName string, toolInput string) *PermissionDecision {
	if ctx == nil {
		return &PermissionDecision{Behavior: BehaviorAllow, DecisionReason: "no permission context"}
	}
	if ctx.IsBypassed() {
		return &PermissionDecision{Behavior: BehaviorAllow, DecisionReason: "permissions bypassed"}
	}

	// Step 1: Check deny rules (highest priority).
	if decision := checkDenyRules(ctx, toolName, toolInput); decision != nil {
		return decision
	}

	// Step 2: In Plan mode, most tools are allowed (read-only).
	if ctx.IsPlanMode() {
		if decision := checkPlanMode(ctx, toolName, toolInput); decision != nil {
			return decision
		}
	}

	// Step 3: Check allow rules → auto-approve.
	if decision := checkAllowRules(ctx, toolName, toolInput); decision != nil {
		return decision
	}

	// Step 4: Check ask rules → ask user.
	if decision := checkAskRules(ctx, toolName, toolInput); decision != nil {
		return decision
	}

	// Step 5: Fall back based on the permission mode.
	return fallbackDecision(ctx, toolName)
}

// checkDenyRules scans the deny-rule list for a match.
func checkDenyRules(ctx *PermissionContext, toolName, toolInput string) *PermissionDecision {
	for i := range ctx.AlwaysDenyRules {
		rule := &ctx.AlwaysDenyRules[i]
		if ruleMatches(rule, toolName, toolInput) {
			return &PermissionDecision{
				Behavior:       BehaviorDeny,
				Message:        formatDenyMessage(rule),
				DecisionReason: "matched deny rule: " + FormatRule(rule),
			}
		}
	}
	return nil
}

// checkAllowRules scans the allow-rule list for a match.
func checkAllowRules(ctx *PermissionContext, toolName, toolInput string) *PermissionDecision {
	for i := range ctx.AlwaysAllowRules {
		rule := &ctx.AlwaysAllowRules[i]
		if ruleMatches(rule, toolName, toolInput) {
			return &PermissionDecision{
				Behavior:       BehaviorAllow,
				Message:        "allowed by rule: " + FormatRule(rule),
				DecisionReason: "matched allow rule",
			}
		}
	}
	return nil
}

// checkAskRules scans the ask-rule list for a match.
func checkAskRules(ctx *PermissionContext, toolName, toolInput string) *PermissionDecision {
	for i := range ctx.AlwaysAskRules {
		rule := &ctx.AlwaysAskRules[i]
		if ruleMatches(rule, toolName, toolInput) {
			return &PermissionDecision{
				Behavior:       BehaviorAsk,
				Message:        formatAskMessage(rule, toolName, toolInput),
				DecisionReason: "matched ask rule: " + FormatRule(rule),
			}
		}
	}
	return nil
}

// checkPlanMode handles the plan-mode subset of allowed tools (read-only).
func checkPlanMode(ctx *PermissionContext, toolName, toolInput string) *PermissionDecision {
	planAllowed := map[string]bool{
		"read_file": true,
		"Glob":      true,
		"Grep":      true,
		"Task":      true, // agent tasks may be read-only research
	}
	if planAllowed[toolName] {
		return &PermissionDecision{
			Behavior:       BehaviorAllow,
			DecisionReason: "plan mode allows read-only tool: " + toolName,
		}
	}
	// In Plan mode, all writes/edits are blocked.
	planBlocked := map[string]bool{
		"write_file": true,
		"edit_file":  true,
		"bash":       true,
		"Bash":       true,
	}
	if planBlocked[toolName] {
		return &PermissionDecision{
			Behavior:       BehaviorDeny,
			Message:        toolName + " is not allowed in plan mode (read-only)",
			DecisionReason: "plan mode blocks modification tool",
		}
	}
	return nil
}

// fallbackDecision provides the mode-based default when no rule matches.
func fallbackDecision(ctx *PermissionContext, toolName string) *PermissionDecision {
	switch ctx.Mode {
	case ModeAccepts:
		return &PermissionDecision{
			Behavior:       BehaviorAllow,
			DecisionReason: "accepts mode defaults to allow",
		}
	case ModeAuto:
		// In auto mode, simple/read-only tools are auto-allowed; others ask.
		if isSimpleTool(toolName) {
			return &PermissionDecision{
				Behavior:       BehaviorAllow,
				DecisionReason: "auto mode allows simple tool: " + toolName,
			}
		}
		return &PermissionDecision{
			Behavior:       BehaviorAsk,
			Message:        toolName + " requires confirmation in auto mode",
			DecisionReason: "auto mode asks for complex tool",
		}
	case ModeEdit:
		return &PermissionDecision{
			Behavior:       BehaviorAsk,
			Message:        toolName + " requires confirmation in edit mode",
			DecisionReason: "edit mode asks for tool",
		}
	case ModeChat:
		return &PermissionDecision{
			Behavior:       BehaviorDeny,
			Message:        "tools are disabled in chat mode",
			DecisionReason: "chat mode denies all tools",
		}
	default:
		return &PermissionDecision{
			Behavior:       BehaviorAsk,
			DecisionReason: "unknown mode, defaulting to ask",
		}
	}
}

// isSimpleTool returns true for tools that are considered safe (read-only, no side effects).
func isSimpleTool(toolName string) bool {
	simple := map[string]bool{
		"read_file":  true,
		"Glob":       true,
		"Grep":       true,
		"file_read":  true,
	}
	return simple[toolName]
}

// ruleMatches checks if a rule matches the given tool name and input.
func ruleMatches(rule *PermissionRule, toolName, toolInput string) bool {
	// First check if the tool name matches.
	if !MatchToolName(rule, toolName) {
		return false
	}
	// If the rule has no content constraint, it matches the whole tool.
	if rule.RuleContent == nil {
		return true
	}
	// If rule has content but no tool input, it doesn't match.
	if toolInput == "" {
		return false
	}
	// Check content constraints by type.
	contentType := RuleContentType(rule)
	switch contentType {
	case "prefix":
		prefixes := SplitPrefixes(*rule.RuleContent)
		for _, p := range prefixes {
			if strings.HasPrefix(toolInput, p) {
				return true
			}
		}
		return false
	case "domain":
		// Simple glob-based domain matching
		pattern := DomainPattern(*rule.RuleContent)
		return matchDomain(pattern, toolInput)
	case "glob", "subtype":
		return strings.Contains(toolInput, *rule.RuleContent)
	default:
		return false
	}
}

// matchDomain performs simple glob matching for domain patterns.
func matchDomain(pattern, input string) bool {
	if pattern == "*" || pattern == "" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return strings.Contains(input, pattern)
	}
	// Simple glob: *.example.com → match anything ending in .example.com
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // ".example.com"
		return strings.HasSuffix(input, suffix) || strings.Contains(input, suffix)
	}
	return strings.Contains(input, pattern)
}

func formatDenyMessage(rule *PermissionRule) string {
	msg := "denied by rule: " + FormatRule(rule)
	if rule.Source == SourcePolicy {
		msg += " (policy)"
	}
	return msg
}

func formatAskMessage(rule *PermissionRule, toolName, toolInput string) string {
	if rule.RuleContent != nil {
		return toolName + "(" + toolInput + ") requires confirmation"
	}
	return toolName + " requires confirmation"
}
