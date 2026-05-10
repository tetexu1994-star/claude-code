// Package hook defines lifecycle hook types used by the plugin system.
// It's a standalone package so both internal/plugin and internal/plugin/lua
// can import it without circular dependency.
package hook

import (
	"context"
	"log/slog"
	"sync"
)

// HookPoint identifies a lifecycle hook location.
type HookPoint string

const (
	HookToolBefore    HookPoint = "on_tool_before"
	HookToolAfter     HookPoint = "on_tool_after"
	HookSessionStart  HookPoint = "on_session_start"
	HookSessionEnd    HookPoint = "on_session_end"
	HookMessage       HookPoint = "on_message"
)

// HookHandler is a function that handles a hook event.
type HookHandler func(ctx context.Context, event *HookEvent) (*HookResult, error)

// HookEvent carries hook context data.
type HookEvent struct {
	Point     HookPoint
	ToolName  string
	Args      map[string]interface{}
	SessionID string
	Message   string
}

// HookResult is what a hook returns.
type HookResult struct {
	Allow  bool
	Deny   bool
	Reason string
	Modify map[string]interface{}
}

type hookEntry struct {
	name    string
	handler HookHandler
}

// HookRegistry manages hook registrations and dispatch.
type HookRegistry struct {
	mu      sync.RWMutex
	entries map[HookPoint][]hookEntry
}

// NewHookRegistry creates an empty hook registry.
func NewHookRegistry() *HookRegistry {
	return &HookRegistry{
		entries: make(map[HookPoint][]hookEntry),
	}
}

// Register adds a handler for a specific hook point.
func (h *HookRegistry) Register(point HookPoint, name string, handler HookHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.entries[point] = append(h.entries[point], hookEntry{name: name, handler: handler})
	slog.Debug("hook registered", "point", point, "name", name)
}

// Len returns the total number of registered hook handlers.
func (h *HookRegistry) Len() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	total := 0
	for _, entries := range h.entries {
		total += len(entries)
	}
	return total
}

// Dispatch calls all handlers registered for the given hook point.
func (h *HookRegistry) Dispatch(ctx context.Context, point HookPoint, event *HookEvent) ([]*HookResult, error) {
	h.mu.RLock()
	entries := h.entries[point]
	h.mu.RUnlock()

	if len(entries) == 0 {
		return nil, nil
	}

	var results []*HookResult
	var firstErr error

	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}
		result, err := entry.handler(ctx, event)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		if result != nil {
			results = append(results, result)
		}
	}
	return results, firstErr
}
