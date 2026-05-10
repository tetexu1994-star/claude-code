package permission

import (
	"fmt"
	"strings"
)

// ParseRuleString parses a permission rule string into a PermissionRule.
//
// Supported formats:
//
//	Bash                     → entire Bash tool, no content constraint
//	Bash(/tmp/*)             → Bash restricted to matching paths
//	Bash(prefix:git)         → Bash restricted to commands with given prefix
//	Bash(prefix:git,pip)     → Bash restricted to multiple prefixes
//	WebFetch(domain:*.com)   → WebFetch restricted to domain pattern
//	Agent(Explore)           → Agent with Explore sub-type
//	mcp__server1             → entire MCP server
//	mcp__server1__tool1      → specific MCP tool
//	mcp__server1__*          → all tools on an MCP server
func ParseRuleString(rule string) (*PermissionRule, error) {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return nil, fmt.Errorf("empty permission rule")
	}

	result := &PermissionRule{}

	// Check for parenthesized content: ToolName(content)
	openIdx := strings.Index(rule, "(")
	closeIdx := strings.LastIndex(rule, ")")

	if openIdx >= 0 {
		// Has parentheses
		if closeIdx < 0 || closeIdx <= openIdx {
			return nil, fmt.Errorf("malformed rule %q: missing closing parenthesis", rule)
		}
		if closeIdx != len(rule)-1 {
			return nil, fmt.Errorf("malformed rule %q: content after closing parenthesis", rule)
		}
		result.ToolName = rule[:openIdx]
		content := rule[openIdx+1 : closeIdx]
		if content == "" {
			// Empty parens means wildcard/all content for this tool
			// e.g. Bash() means Bash with any content.
			result.RuleContent = nil
		} else {
			result.RuleContent = &content
		}
	} else {
		result.ToolName = rule
	}

	if result.ToolName == "" {
		return nil, fmt.Errorf("empty tool name in rule %q", rule)
	}

	return result, nil
}

// FormatRuleString returns the canonical string form of a rule.
func FormatRuleString(rule *PermissionRule) string {
	if rule.RuleContent != nil {
		return fmt.Sprintf("%s(%s)", rule.ToolName, *rule.RuleContent)
	}
	return rule.ToolName
}

// IsMCPRule reports whether a rule targets an MCP tool or server.
func IsMCPRule(rule *PermissionRule) bool {
	return strings.HasPrefix(rule.ToolName, "mcp__")
}

// RuleContentType returns the type of content constraint in the rule, if any.
// Possible values: "path", "prefix", "domain", "subtype", "glob", "" (no constraint).
func RuleContentType(rule *PermissionRule) string {
	if rule.RuleContent == nil {
		return ""
	}
	content := *rule.RuleContent
	if strings.HasPrefix(content, "prefix:") {
		return "prefix"
	}
	if strings.HasPrefix(content, "domain:") {
		return "domain"
	}
	// Path patterns can contain glob chars
	if strings.ContainsAny(content, "/*?[]") {
		return "glob"
	}
	return "subtype" // simple identifier content
}

// MatchToolName checks whether a tool name matches a PermissionRule.
// This handles MCP-specific matching where mcp__server can match
// mcp__server, mcp__server__tool, and mcp__server__*.
func MatchToolName(rule *PermissionRule, toolName string) bool {
	if rule.ToolName == toolName {
		return true
	}
	// MCP server-wide match: mcp__server matches mcp__server__*
	if IsMCPRule(rule) {
		if strings.HasPrefix(toolName, rule.ToolName+"__") {
			return true
		}
		// mcp__server__* rule matching mcp__server__tool
		if strings.HasSuffix(rule.ToolName, "__*") {
			prefix := strings.TrimSuffix(rule.ToolName, "*")
			if strings.HasPrefix(toolName, prefix) {
				return true
			}
		}
	}
	return false
}

// SplitPrefixes splits a prefix content constraint into individual prefixes.
// e.g. "prefix:git,pip" → ["git", "pip"]
func SplitPrefixes(content string) []string {
	if !strings.HasPrefix(content, "prefix:") {
		return nil
	}
	raw := strings.TrimPrefix(content, "prefix:")
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// DomainPattern extracts the domain pattern from a domain: content constraint.
func DomainPattern(content string) string {
	if !strings.HasPrefix(content, "domain:") {
		return ""
	}
	return strings.TrimPrefix(content, "domain:")
}
