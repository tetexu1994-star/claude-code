package plugin

import (
	"github.com/tetexu/tlaude-code/internal/hook"
)

// Re-export hook types for backward compatibility within the plugin package.
// The actual implementations live in internal/hook to break the circular import
// chain: plugin/ → plugin/lua/ → plugin/ (was circular).
// Now: plugin → plugin/lua → hook (no cycle).

type (
	HookPoint     = hook.HookPoint
	HookEvent     = hook.HookEvent
	HookResult    = hook.HookResult
	HookHandler   = hook.HookHandler
	HookRegistry  = hook.HookRegistry
)

var (
	HookToolBefore    = hook.HookToolBefore
	HookToolAfter     = hook.HookToolAfter
	HookSessionStart  = hook.HookSessionStart
	HookSessionEnd    = hook.HookSessionEnd
	HookMessage       = hook.HookMessage
)

// NewHookRegistry delegates to hook.NewHookRegistry.
func NewHookRegistry() *HookRegistry {
	return hook.NewHookRegistry()
}
