package agent

// BuiltInAgents returns the five built-in agent definitions.
// Each agent specifies a ModelRef to choose the most appropriate model for its task.
func BuiltInAgents() []AgentDefinition {
	return []AgentDefinition{
		{
			AgentType:   "general",
			Name:        "General Purpose",
			Description: "General-purpose agent for researching complex questions, searching for code, and executing multi-step tasks.",
			WhenToUse:   "Suitable for most programming tasks. When in doubt, choose this one.",
			Tools:       []string{"*"},
			MaxTurns:    200,
			Source:      "built-in",
			Color:       "#6C5CE7",
			// Use session default model (inherit).
			ModelRef: ModelRef{
				Reason: "Inherits session default model",
			},
		},
		{
			AgentType:   "explore",
			Name:        "Explore",
			Description: "Fast read-only search agent for locating code. Use it to find files by pattern, grep for symbols or keywords, or answer \"where is X defined\".",
			WhenToUse:   "When you need to understand unfamiliar codebases, find files, or read documentation. Do NOT use for code review or cross-file consistency checks.",
			Tools:       []string{"Read", "Glob", "Grep", "Bash"},
			MaxTurns:    50,
			Source:      "built-in",
			Color:       "#00B894",
			Background:  true,
			ModelRef: ModelRef{
				Provider: "deepseek",
				Model:    "deepseek-chat",
				Reason:   "探索任务不需要最高精度，DeepSeek性价比最高",
			},
		},
		{
			AgentType:   "code",
			Name:        "Code Writer",
			Description: "Specialized agent for writing, modifying, or refactoring code.",
			WhenToUse:   "When you need to write, modify, or refactor code.",
			Tools:       []string{"*"},
			DisallowedTools: []string{"Agent"},
			MaxTurns:    100,
			Source:      "built-in",
			Color:       "#0984E3",
			ModelRef: ModelRef{
				Provider: "anthropic",
				Model:    "claude-sonnet-4-20250514",
				Reason:   "代码编写需要最高精度",
			},
		},
		{
			AgentType:   "review",
			Name:        "Code Reviewer",
			Description: "Code review agent for auditing changes, checking security issues, and evaluating code quality.",
			WhenToUse:   "When you need to review code changes, check for security issues, or evaluate code quality.",
			Tools:       []string{"Read", "Glob", "Grep", "Bash"},
			MaxTurns:    50,
			Source:      "built-in",
			Color:       "#D63031",
			ModelRef: ModelRef{
				Provider: "openai",
				Model:    "gpt-4o",
				Reason:   "交叉验证——用不同模型审查同一个输出",
			},
		},
		{
			AgentType:   "moa",
			Name:        "MoA Agent",
			Description: "Multi-model parallel reasoning. Runs the same prompt across multiple models and aggregates results.",
			WhenToUse:   "When you need multiple models to analyze a problem from different angles, or need the highest accuracy answer.",
			Tools:       []string{"Read", "Glob", "Grep", "Bash"},
			MaxTurns:    30,
			Source:      "built-in",
			Color:       "#FDCB6E",
			PermissionMode: "accepts",
			ModelStrategy: []ModelRef{
				{Provider: "anthropic", Model: "claude-sonnet-4-20250514", Reason: "主模型"},
				{Provider: "deepseek", Model: "deepseek-chat", Reason: "辅助模型"},
				{Provider: "openai", Model: "gpt-4o", Reason: "验证模型"},
			},
		},
	}
}

// BuiltInAgentTypes returns the list of built-in agent type names.
func BuiltInAgentTypes() []string {
	return []string{"general", "explore", "code", "review", "moa"}
}
