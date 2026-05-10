package tool

import (
	"context"
	"encoding/json"
	"fmt"
)

// PlanBridge is set by the TUI to bridge plan mode tools with the Plan Manager.
// It avoids a circular import between the tool and plan packages.
type PlanBridge struct {
	EnterPlan func(planContent, scope string) (string, error)
	ExitPlan  func(summary string) (string, string, error) // returns planID, status, error
}

var sharedPlanBridge *PlanBridge

// SetPlanBridge sets the shared plan bridge for EnterPlanMode/ExitPlanMode tools.
func SetPlanBridge(bridge *PlanBridge) {
	sharedPlanBridge = bridge
}

// EnterPlanModeTool is the tool the LLM calls to enter plan mode.
// Equivalent to Claude Code's EnterPlanModeTool.
type EnterPlanModeTool struct {
	bridge *PlanBridge
}

// NewEnterPlanModeTool creates a new EnterPlanMode tool.
func NewEnterPlanModeTool() *EnterPlanModeTool {
	return &EnterPlanModeTool{}
}

func (t *EnterPlanModeTool) Name() string        { return "EnterPlanMode" }
func (t *EnterPlanModeTool) IsEnabled() bool      { return true }
func (t *EnterPlanModeTool) IsConcurrencySafe() bool { return false }

func (t *EnterPlanModeTool) Description() string {
	return "Use this tool proactively when you're about to start a non-trivial implementation task. " +
		"Getting user sign-off on your approach before writing code prevents wasted effort and ensures alignment. " +
		"This tool transitions the session into plan mode where you can explore the codebase and design an " +
		"implementation approach for user approval."
}

func (t *EnterPlanModeTool) ToolDefinition() ToolDefinition {
	schema := json.RawMessage(`{
  "type": "object",
  "properties": {
    "plan": {
      "type": "string",
      "description": "The plan content describing the implementation approach, including steps, files to modify, and design decisions. Markdown format recommended."
    },
    "scope": {
      "type": "string",
      "description": "Short description of the scope or area being modified (e.g. 'config module', 'auth system'). Used as the plan title."
    }
  },
  "required": ["plan"]
}`)
	return ToolDefinition{
		Name:        "EnterPlanMode",
		Description: t.Description(),
		InputSchema: schema,
	}
}

func (t *EnterPlanModeTool) Execute(ctx context.Context, input json.RawMessage, toolCtx *ToolContext) (*ToolResult, error) {
	var params struct {
		Plan  string `json:"plan"`
		Scope string `json:"scope"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return &ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}

	if params.Plan == "" {
		return &ToolResult{IsError: true, Content: "plan field is required"}, nil
	}

	bridge := sharedPlanBridge
	if t.bridge != nil {
		bridge = t.bridge
	}
	if bridge == nil || bridge.EnterPlan == nil {
		return &ToolResult{Content: "Plan mode is not available (no plan bridge configured)."}, nil
	}

	planID, err := bridge.EnterPlan(params.Plan, params.Scope)
	if err != nil {
		return &ToolResult{IsError: true, Content: fmt.Sprintf("failed to enter plan mode: %v", err)}, nil
	}

	return &ToolResult{
		Content: fmt.Sprintf("Entered plan mode. Plan created: %s\n\nPlan content:\n%s", planID, params.Plan),
	}, nil
}
