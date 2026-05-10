package hook

import (
	"context"
	"testing"
)

func TestNewHookRegistry(t *testing.T) {
	hr := NewHookRegistry()
	if hr == nil {
		t.Fatal("expected non-nil registry")
	}
	if hr.Len() != 0 {
		t.Errorf("expected len 0, got %d", hr.Len())
	}
}

func TestRegisterDispatch(t *testing.T) {
	hr := NewHookRegistry()
	called := false

	hr.Register(HookToolBefore, "test", func(ctx context.Context, event *HookEvent) (*HookResult, error) {
		called = true
		if event.ToolName != "bash" {
			t.Errorf("expected tool 'bash', got %q", event.ToolName)
		}
		return &HookResult{Allow: true}, nil
	})

	results, err := hr.Dispatch(context.Background(), HookToolBefore, &HookEvent{
		Point:    HookToolBefore,
		ToolName: "bash",
		Args:     map[string]interface{}{"command": "ls"},
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !called {
		t.Fatal("handler was not called")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Allow {
		t.Error("expected Allow=true")
	}
}

func TestDispatchNoHandlers(t *testing.T) {
	hr := NewHookRegistry()
	results, err := hr.Dispatch(context.Background(), HookToolAfter, &HookEvent{
		Point:    HookToolAfter,
		ToolName: "read",
	})
	if err != nil {
		t.Errorf("expected no error: %v", err)
	}
	if results != nil {
		t.Error("expected nil results for empty dispatch")
	}
}

func TestMultipleHandlersOrder(t *testing.T) {
	hr := NewHookRegistry()
	var order []int

	hr.Register(HookToolBefore, "first", func(ctx context.Context, event *HookEvent) (*HookResult, error) {
		order = append(order, 1)
		return &HookResult{Allow: true}, nil
	})
	hr.Register(HookToolBefore, "second", func(ctx context.Context, event *HookEvent) (*HookResult, error) {
		order = append(order, 2)
		return &HookResult{Deny: true, Reason: "blocked"}, nil
	})
	hr.Register(HookToolBefore, "third", func(ctx context.Context, event *HookEvent) (*HookResult, error) {
		order = append(order, 3)
		return nil, nil // pass-through
	})

	results, err := hr.Dispatch(context.Background(), HookToolBefore, &HookEvent{
		Point:    HookToolBefore,
		ToolName: "bash",
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Errorf("expected order [1,2,3], got %v", order)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results (third returned nil), got %d", len(results))
	}
	if !results[1].Deny {
		t.Error("expected second result to deny")
	}
}

func TestContextCancellation(t *testing.T) {
	hr := NewHookRegistry()
	hr.Register(HookToolBefore, "slow", func(ctx context.Context, event *HookEvent) (*HookResult, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := hr.Dispatch(ctx, HookToolBefore, &HookEvent{
		Point:    HookToolBefore,
		ToolName: "bash",
	})
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestAllHookPoints(t *testing.T) {
	points := []HookPoint{HookToolBefore, HookToolAfter, HookSessionStart, HookSessionEnd, HookMessage}
	for _, p := range points {
		hr := NewHookRegistry()
		called := false
		hr.Register(p, "test", func(ctx context.Context, event *HookEvent) (*HookResult, error) {
			called = true
			return &HookResult{Allow: true}, nil
		})
		results, err := hr.Dispatch(context.Background(), p, &HookEvent{Point: p})
		if err != nil {
			t.Errorf("point %s: Dispatch error: %v", p, err)
		}
		if !called {
			t.Errorf("point %s: handler not called", p)
		}
		if len(results) != 1 {
			t.Errorf("point %s: expected 1 result, got %d", p, len(results))
		}
	}
}
