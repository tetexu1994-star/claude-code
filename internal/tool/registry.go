package tool

import (
	"fmt"

	"github.com/tetexu/tlaude-code/internal/tool/permission"
)

// Registry stores all registered tools and supports permission-aware queries.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry. Returns an error if a tool
// with the same name is already registered.
func (r *Registry) Register(t Tool) error {
	if t == nil {
		return fmt.Errorf("cannot register nil tool")
	}
	name := t.Name()
	if name == "" {
		return fmt.Errorf("tool name must not be empty")
	}
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool %q already registered", name)
	}
	r.tools[name] = t
	return nil
}

// Get retrieves a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// GetAll returns all enabled tools, optionally filtered by the permission context.
// If ctx is nil, all enabled tools are returned unfiltered.
func (r *Registry) GetAll(ctx *permission.PermissionContext) []Tool {
	result := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		if !t.IsEnabled() {
			continue
		}
		// In bypass mode or with nil context, include all enabled tools.
		if ctx == nil || ctx.IsBypassed() {
			result = append(result, t)
			continue
		}
		// Check if the tool is denied at the permission level.
		if r.isToolDenied(t, ctx) {
			continue
		}
		result = append(result, t)
	}
	return result
}

// AssembleToolPool merges the registry's base tools with external MCP tools,
// honoring the permission context.
func (r *Registry) AssembleToolPool(ctx *permission.PermissionContext, mcpTools []Tool) []Tool {
	base := r.GetAll(ctx)
	pool := make([]Tool, 0, len(base)+len(mcpTools))
	pool = append(pool, base...)
	for _, mt := range mcpTools {
		if !mt.IsEnabled() {
			continue
		}
		if ctx != nil && !ctx.IsBypassed() && r.isToolDenied(mt, ctx) {
			continue
		}
		pool = append(pool, mt)
	}
	return pool
}

// isToolDenied checks whether a tool is denied by the permission context's deny rules.
func (r *Registry) isToolDenied(t Tool, ctx *permission.PermissionContext) bool {
	for _, rule := range ctx.AlwaysDenyRules {
		if matchesRule(t.Name(), &rule) {
			return true
		}
	}
	return false
}

// matchesRule checks if a tool name matches a permission rule.
func matchesRule(toolName string, rule *permission.PermissionRule) bool {
	return permission.MatchToolName(rule, toolName)
}

// Len returns the number of registered tools.
func (r *Registry) Len() int {
	return len(r.tools)
}
