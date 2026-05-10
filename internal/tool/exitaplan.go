package tool

import (
	"context"
	"encoding/json"
	"fmt"
)

// ExitPlanModeTool is the tool the LLM calls to exit plan mode.
// Equivalent to Claude Code's ExitPlanModeV2Tool.
type ExitPlanModeTool struct {
	bridge *PlanBridge
}

// NewExitPlanModeTool creates a new ExitPlanMode tool.
func NewExitPlanModeTool() *ExitPlanModeTool {
	return &ExitPlanModeTool{}
}

func (t *ExitPlanModeTool) Name() string        { return "ExitPlanMode" }
func (t *ExitPlanModeTool) IsEnabled() bool      { return true }
func (t *ExitPlanModeTool) IsConcurrencySafe() bool { return false }

func (t *ExitPlanModeTool) Description() string {
	return "Use this tool when you are in plan mode and have finished writing your plan. " +
		"This transitions the session out of plan mode and into implementation mode. " +
		"The user will review and approve (or reject) the plan before implementation begins."
}

func (t *ExitPlanModeTool) ToolDefinition() ToolDefinition {
	schema := json.RawMessage(`{
  "type": "object",
  "properties": {
    "summary": {
      "type": "string",
      "description": "Brief summary of the plan that was created and what it covers."
    },
    "allowed_tools": {
      "type": "array",
      "description": "Optional list of tool-related prompts needed to implement the plan (e.g. 'run tests', 'install dependencies').",
      "items": {
        "type": "object",
        "properties": {
          "tool": {"type": "string", "description": "The tool name"},
          "prompt": {"type": "string", "description": "Semantic description of the action"}
        }
      }
    }
  },
  "required": ["summary"]
}`)
	return ToolDefinition{
		Name:        "ExitPlanMode",
		Description: t.Description(),
		InputSchema: schema,
	}
}

func (t *ExitPlanModeTool) Execute(ctx context.Context, input json.RawMessage, toolCtx *ToolContext) (*ToolResult, error) {
	var params struct {
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return &ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}

	if params.Summary == "" {
		return &ToolResult{IsError: true, Content: "summary field is required"}, nil
	}

	bridge := sharedPlanBridge
	if t.bridge != nil {
		bridge = t.bridge
	}
	if bridge == nil || bridge.ExitPlan == nil {
		return &ToolResult{Content: "Plan mode is not available (no plan bridge configured)."}, nil
	}

	planID, status, err := bridge.ExitPlan(params.Summary)
	if err != nil {
		return &ToolResult{IsError: true, Content: fmt.Sprintf("failed to exit plan mode: %v", err)}, nil
	}

	return &ToolResult{
		Content: fmt.Sprintf("Exited plan mode. Plan %s status: %s\n\nSummary: %s", planID, status, params.Summary),
	}, nil
}
