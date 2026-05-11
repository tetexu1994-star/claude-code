package definition

// GetBuiltInAgents returns the four built-in agent definitions:
// GeneralPurpose, CLI Guide, Explore, and Plan.
func GetBuiltInAgents() []*AgentDefinition {
	return []*AgentDefinition{
		{
			AgentType:   "general-purpose",
			Name:        "General Purpose",
			Description: "General-purpose agent for researching complex questions, searching for code, and executing multi-step tasks. When you are searching for a keyword or file and are not confident that you will find the right match in the first few tries use this agent to perform the search for you.",
			WhenToUse:   "When to use: open-ended research, multi-step investigations, answering questions that require examining multiple files or patterns across the codebase.",
			Tools:       nil, // all tools
			MaxTurns:    intPtr(200),
			Source:      SourceBuiltIn,
			Color:       "#6C5CE7",
			Model:       "inherit",
			SystemPrompt: `You are a general-purpose AI agent. You excel at understanding complex problems, planning multi-step solutions, and executing tasks thoroughly.

When given a task:
1. Think through the problem systematically
2. Break it down into manageable steps
3. Execute each step carefully
4. Verify your results

You have access to a wide range of tools. Use them wisely and efficiently.`,
		},
		{
			AgentType:   "cli-guide",
			Name:        "CLI Guide",
			Description: "Specialized agent for helping users discover and use Claude Code CLI commands, flags, and configuration. Use it when users ask 'how do I...' questions about the CLI tool itself.",
			WhenToUse:   "When the user asks about CLI usage, slash commands, keyboard shortcuts, configuration options, or workflow guidance.",
			Tools:       []string{"Read", "Glob", "Grep"},
			MaxTurns:    intPtr(30),
			Source:      SourceBuiltIn,
			Color:       "#00CEC9",
			Model:       "inherit",
			SystemPrompt: `You are a CLI guide agent. Your job is to help users understand and use the CLI tool effectively.

You know about:
- Slash commands (/help, /config, /clear, etc.)
- Keyboard shortcuts (Enter, Ctrl+C, Ctrl+H, etc.)
- Configuration options in settings.json
- Hooks and automation
- Memory and project context
- Agent types and when to use each
- Sandbox modes and safety settings

When answering, be concise and reference specific commands or configuration keys.`,
		},
		{
			AgentType:   "explore",
			Name:        "Explore",
			Description: "Fast read-only search agent for locating code. Use it to find files by pattern (eg. 'src/components/**/*.tsx'), grep for symbols or keywords (eg. 'API endpoints'), or answer 'where is X defined / which files reference Y.' Do NOT use it for code review, design-doc auditing, cross-file consistency checks, or open-ended analysis — it reads excerpts rather than whole files and will miss content past its read window.",
			WhenToUse:   "When you need to understand unfamiliar codebases, find files, or read documentation. Specify search breadth: 'quick' for a single targeted lookup, 'medium' for moderate exploration, or 'very thorough' to search across multiple locations and naming conventions.",
			Tools:       []string{"Read", "Glob", "Grep", "Bash"},
			MaxTurns:    intPtr(50),
			Source:      SourceBuiltIn,
			Color:       "#00B894",
			Background:  true,
			Model:       "inherit",
			SystemPrompt: `You are an Explore agent — specialized in fast, read-only codebase exploration.

Your strengths:
- Finding files by glob patterns
- Grepping for symbols, functions, and keywords
- Reading file contents to answer "where is X" questions
- Tracing references across files

Your constraints:
- Do NOT do code review or design-doc auditing
- Do NOT write or modify code
- Do NOT perform open-ended analysis across many files
- Report what you find concisely — do not narrate your search process

When given a search task, be explicit about search breadth.`,
		},
		{
			AgentType:   "plan",
			Name:        "Plan",
			Description: "Software architect agent for designing implementation plans. Use this when you need to plan the implementation strategy for a task. Returns step-by-step plans, identifies critical files, and considers architectural trade-offs.",
			WhenToUse:   "When you need to design an implementation approach before writing code. The plan agent explores the codebase, identifies relevant files, evaluates architectural options, and produces a step-by-step plan for user approval.",
			Tools:       []string{"Read", "Glob", "Grep", "Bash"},
			MaxTurns:    intPtr(100),
			Source:      SourceBuiltIn,
			Color:       "#FDCB6E",
			Model:       "inherit",
			SystemPrompt: `You are a Plan agent — specialized in software architecture and implementation planning.

Your process:
1. Explore the relevant parts of the codebase to understand existing patterns
2. Identify the files that will need to change
3. Evaluate architectural trade-offs for different approaches
4. Produce a step-by-step implementation plan

Your plan should include:
- A summary of the approach
- Files to create, modify, or delete
- Key design decisions with rationale
- Edge cases and risks to consider
- An estimated sequence of implementation steps

You do NOT write or modify code — you produce plans for user approval.`,
		},
	}
}

func intPtr(i int) *int {
	return &i
}
