package compact

import "github.com/tetexu/tlaude-code/internal/llm"

const (
	// AutoCompactBufferTokens is the buffer before the context limit used
	// to determine when auto-compact should fire.
	AutoCompactBufferTokens = 13000

	// WarningBufferTokens is used to calculate the warning threshold.
	WarningBufferTokens = 20000

	// ErrorBufferTokens is used to calculate the error threshold.
	ErrorBufferTokens = 20000

	// ManualCompactBufferTokens is the buffer before blocking.
	ManualCompactBufferTokens = 3000

	// MaxConsecutiveAutoCompactFailures is the circuit breaker limit.
	MaxConsecutiveAutoCompactFailures = 3
)

// ModelContextWindows maps model prefixes to their approximate context window
// sizes (in tokens).
var ModelContextWindows = map[string]int{
	"claude-sonnet-4":  200000,
	"claude-opus-4":    200000,
	"claude-haiku-4":   200000,
	"claude-3.5-sonnet": 200000,
	"claude-3-opus":    200000,
	"claude-3-sonnet":  200000,
	"claude-3-haiku":   200000,
	"gpt-4":            128000,
	"gpt-4o":           128000,
	"gpt-4-turbo":      128000,
	"gpt-3.5-turbo":    16385,
	"deepseek-chat":    65536,
	"deepseek-reasoner": 65536,
	"qwen-plus":        131072,
	"qwen-max":         131072,
	"glm-4":            128000,
}

// TokenWarningState describes the token usage relative to various thresholds.
type TokenWarningState struct {
	PercentLeft          int
	IsAboveWarning       bool
	IsAboveError         bool
	IsAboveAutoCompact   bool
	IsAtBlockingLimit    bool
}

// AutoCompactState tracks auto-compact state across turns.
type AutoCompactState struct {
	Compacted           bool
	TurnCounter         int
	ConsecutiveFailures int
}

// GetEffectiveContextWindow returns the model's context window minus the max
// output tokens reserved for the summary LLM call.
func GetEffectiveContextWindow(model string, maxSummaryTokens int) int {
	window := getContextWindow(model)
	if window <= 0 {
		window = 200000 // default fallback
	}
	reserved := maxSummaryTokens
	if reserved <= 0 {
		reserved = 20000
	}
	return window - reserved
}

// GetAutoCompactThreshold returns the token count at which auto-compact
// should trigger.
func GetAutoCompactThreshold(model string, maxSummaryTokens int) int {
	effective := GetEffectiveContextWindow(model, maxSummaryTokens)
	return effective - AutoCompactBufferTokens
}

// NeedsCompact checks if the total estimated tokens exceed the auto-compact
// threshold.
func NeedsCompact(messages []llm.Message, model string, tokenBudget int) bool {
	tokens := EstimateTokens(messages)
	threshold := GetAutoCompactThreshold(model, tokenBudget)
	return tokens >= threshold
}

// CalculateTokenState returns the current token warning state based on the
// estimated token count.
func CalculateTokenState(messages []llm.Message, model string, maxSummaryTokens int) TokenWarningState {
	tokens := EstimateTokens(messages)
	autoThreshold := GetAutoCompactThreshold(model, maxSummaryTokens)
	effectiveWindow := GetEffectiveContextWindow(model, maxSummaryTokens)

	threshold := autoThreshold
	percentLeft := 0
	if threshold > 0 {
		percentLeft = max(0, int(float64(threshold-tokens)/float64(threshold)*100))
	}

	warningThreshold := threshold - WarningBufferTokens
	errorThreshold := threshold - ErrorBufferTokens
	blockingLimit := effectiveWindow - ManualCompactBufferTokens

	return TokenWarningState{
		PercentLeft:        percentLeft,
		IsAboveWarning:     tokens >= warningThreshold,
		IsAboveError:       tokens >= errorThreshold,
		IsAboveAutoCompact: tokens >= autoThreshold,
		IsAtBlockingLimit:  tokens >= blockingLimit,
	}
}

// EstimateTokens estimates the total token count for a slice of messages.
// Uses a rough 4-char-per-token heuristic with a 4/3 padding factor.
func EstimateTokens(messages []llm.Message) int {
	total := 0
	for _, msg := range messages {
		total += len(msg.Content)
		for _, tc := range msg.ToolCalls {
			total += len(tc.Name)
			total += len(tc.Result)
			for _, v := range tc.Args {
				if s, ok := v.(string); ok {
					total += len(s)
				}
			}
		}
	}
	// Rough 4 chars per token, padded by 4/3 to be conservative.
	return int(float64(total)/4.0*4.0/3.0) + 0
}

func getContextWindow(model string) int {
	// Exact match first.
	if w, ok := ModelContextWindows[model]; ok {
		return w
	}
	// Prefix match.
	for prefix, w := range ModelContextWindows {
		if len(prefix) <= len(model) && model[:len(prefix)] == prefix {
			return w
		}
	}
	return 0
}
