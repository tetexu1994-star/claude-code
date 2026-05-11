package compact

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/tetexu/tlaude-code/internal/llm"
)

// Error messages (exported constants, matching CC's pattern).
const (
	ErrNotEnoughMessages  = "Not enough messages to compact"
	ErrPromptTooLong      = "Conversation too long; cannot compact further"
	ErrUserAborted        = "API Error: Request was aborted"
	ErrIncompleteResponse = "Compaction interrupted: please try again"
	ErrNoSummary          = "Failed to generate conversation summary — response did not contain valid text content"
)

// Defaults for the compact config.
const (
	DefaultEnabled          = true
	DefaultAutoCompact      = true
	DefaultTokenBudget      = 40000
	DefaultMaxSummaryTokens = 20000
	DefaultMaxPTLRetries    = 3
)

// Config holds all compaction settings.
type Config struct {
	Enabled          bool // Master switch
	AutoCompact      bool // Auto-trigger
	TokenBudget      int  // Messages to keep verbatim (token budget for recent)
	MaxSummaryTokens int  // Max output tokens for the summary LLM call
	MaxPTLRetries    int  // Max prompt-too-long retries
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Enabled:          DefaultEnabled,
		AutoCompact:      DefaultAutoCompact,
		TokenBudget:      DefaultTokenBudget,
		MaxSummaryTokens: DefaultMaxSummaryTokens,
		MaxPTLRetries:    DefaultMaxPTLRetries,
	}
}

// CompactResult holds the output of a compaction run.
type CompactResult struct {
	SummaryMessage   llm.Message  // The LLM-generated summary as a user message
	MessagesKept     []llm.Message // Recent messages preserved verbatim
	BoundaryMessage  llm.Message  // System message marking the compact boundary
	PreCompactTokens  int          // Estimated tokens before compaction
	PostCompactTokens int          // Estimated tokens after compaction
	CompactUsage      llm.Usage    // Token usage for the compaction LLM call
}

// Manager orchestrates compaction operations, including the optional auto-compact
// circuit breaker state.
type Manager struct {
	mu          sync.Mutex
	cfg         Config
	autoState   AutoCompactState
}

// NewManager creates a new compact Manager.
func NewManager(cfg Config) *Manager {
	return &Manager{cfg: cfg}
}

// Config returns the current compact configuration (thread-safe copy).
func (m *Manager) Config() Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

// SetConfig updates the compact configuration.
func (m *Manager) SetConfig(cfg Config) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = cfg
}

// AutoState returns a copy of the auto-compact tracking state.
func (m *Manager) AutoState() AutoCompactState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.autoState
}

// ResetAutoState clears the auto-compact circuit breaker.
func (m *Manager) ResetAutoState() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.autoState = AutoCompactState{}
}

// RecordAutoFailure increments the consecutive failure counter.
// Returns true if the circuit breaker has tripped.
func (m *Manager) RecordAutoFailure() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.autoState.ConsecutiveFailures++
	return m.autoState.ConsecutiveFailures >= MaxConsecutiveAutoCompactFailures
}

// RecordAutoSuccess resets the failure counter and marks compacted=true.
func (m *Manager) RecordAutoSuccess() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.autoState.Compacted = true
	m.autoState.TurnCounter = 0
	m.autoState.ConsecutiveFailures = 0
}

// IncrementTurn bumps the turn counter for auto-compact tracking.
func (m *Manager) IncrementTurn() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.autoState.TurnCounter++
}

// --- Package-level convenience functions ---

// CompactConversation summarizes old messages via a separate LLM call and
// returns a compressed message set: [boundary, summary, ...kept].
func CompactConversation(
	ctx context.Context,
	messages []llm.Message,
	provider llm.Provider,
	cfg Config,
) (*CompactResult, error) {
	if len(messages) < 2 {
		return nil, errors.New(ErrNotEnoughMessages)
	}

	preTokens := EstimateTokens(messages)

	// Strip images from user messages (images not needed for summary).
	stripped := stripImagesFromMessages(messages)

	// Build the compact prompt.
	compactPrompt := GetCompactPrompt("")

	// Build messages for the compaction LLM call:
	// the full conversation + a user message with the compact prompt.
	compactMessages := make([]llm.Message, len(stripped))
	copy(compactMessages, stripped)

	// Replace the last user message content with the compact prompt for efficiency,
	// or append a new user message if the conversation ends with assistant/tool.
	lastIdx := len(compactMessages) - 1
	if compactMessages[lastIdx].Role == "assistant" || compactMessages[lastIdx].Role == "tool" {
		compactMessages = append(compactMessages, llm.Message{
			Role:    "user",
			Content: compactPrompt,
		})
	} else {
		// Append as a new message rather than replacing.
		compactMessages = append(compactMessages, llm.Message{
			Role:    "user",
			Content: compactPrompt,
		})
	}

	// PTL retry loop.
	var summary string
	var lastErr error
	maxRetries := cfg.MaxPTLRetries
	if maxRetries <= 0 {
		maxRetries = DefaultMaxPTLRetries
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		req := llm.ChatRequest{
			Model:     "", // will be filled by provider
			Messages:  compactMessages,
			System:    "You are a helpful AI assistant tasked with summarizing conversations.",
			MaxTokens: cfg.MaxSummaryTokens,
		}
		if req.MaxTokens <= 0 {
			req.MaxTokens = DefaultMaxSummaryTokens
		}

		resp, err := provider.Chat(ctx, req)
		if err != nil {
			lastErr = err
			// Check for context cancellation / abort.
			if errors.Is(err, context.Canceled) {
				return nil, errors.New(ErrUserAborted)
			}
			continue
		}

		summary = resp.Message.Content

		// Check for prompt-too-long in the response.
		if strings.Contains(strings.ToLower(summary), "prompt_too_long") {
			if attempt >= maxRetries {
				return nil, errors.New(ErrPromptTooLong)
			}
			// Truncate oldest messages and retry.
			compactMessages = truncateHead(compactMessages)
			if len(compactMessages) == 0 {
				return nil, errors.New(ErrPromptTooLong)
			}
			continue
		}

		// Got a valid response.
		break
	}

	if summary == "" {
		if lastErr != nil {
			return nil, fmt.Errorf("%s: %w", ErrIncompleteResponse, lastErr)
		}
		return nil, errors.New(ErrNoSummary)
	}

	// Format the summary.
	formatted := GetCompactUserSummaryMessage(summary, true)

	// Determine how many messages to keep verbatim (token budget from end).
	messagesKept := selectMessagesToKeep(messages, cfg.TokenBudget)

	// Build compact boundary message.
	boundary := llm.Message{
		Role:    "system",
		Content: fmt.Sprintf("[compact boundary: %s]", newBoundaryID()),
	}

	// Build the summary message (injected as a user message).
	summaryMsg := llm.Message{
		Role:    "user",
		Content: formatted,
	}

	postTokens := EstimateTokens(append([]llm.Message{boundary, summaryMsg}, messagesKept...))

	// Compute usage from the response (best effort).
	// The caller can fill this in from the actual ChatResponse.
	usage := llm.Usage{
		InputTokens:  preTokens,
		OutputTokens: EstimateTokens([]llm.Message{{Content: summary}}),
	}

	return &CompactResult{
		SummaryMessage:   summaryMsg,
		MessagesKept:     messagesKept,
		BoundaryMessage:  boundary,
		PreCompactTokens:  preTokens,
		PostCompactTokens: postTokens,
		CompactUsage:      usage,
	}, nil
}

// selectMessagesToKeep walks messages from the end and keeps them until
// the estimated token budget is exceeded. Always keeps at least 2
// message pairs (4 messages minimum).
func selectMessagesToKeep(messages []llm.Message, tokenBudget int) []llm.Message {
	if tokenBudget <= 0 {
		tokenBudget = DefaultTokenBudget
	}

	// Walk from end, accumulating tokens.
	acc := 0
	keepFrom := len(messages)
	for i := len(messages) - 1; i >= 0; i-- {
		msgTokens := estimateMessageTokens(messages[i])
		if acc+msgTokens > tokenBudget {
			keepFrom = i + 1
			break
		}
		acc += msgTokens
		keepFrom = i
	}

	// Always keep at least the last 4 messages (2 message pairs).
	minKeep := len(messages) - 4
	if minKeep < 0 {
		minKeep = 0
	}
	if keepFrom > minKeep {
		keepFrom = minKeep
	}

	if keepFrom >= len(messages) {
		return nil
	}

	result := make([]llm.Message, len(messages)-keepFrom)
	copy(result, messages[keepFrom:])
	return result
}

func estimateMessageTokens(msg llm.Message) int {
	total := len(msg.Content)
	for _, tc := range msg.ToolCalls {
		total += len(tc.Name) + len(tc.Result) + 50 // rough JSON overhead
	}
	return total / 3 // ~3 chars per token for estimation
}

// truncateHead drops the oldest messages from the slice, keeping at least
// the last message (the compact prompt). Used for PTL retry escalation.
func truncateHead(messages []llm.Message) []llm.Message {
	if len(messages) <= 2 {
		return messages
	}
	// Drop ~20% of the oldest non-prompt messages, keeping the last user prompt.
	drop := len(messages) / 5
	if drop < 1 {
		drop = 1
	}
	// Minimum: keep the last 2 messages.
	if len(messages)-drop < 2 {
		drop = len(messages) - 2
	}
	return messages[drop:]
}

// stripImagesFromMessages replaces image/document content markers in user
// messages with text placeholders. In the Go message model, we check for
// base64 image data patterns and replace them.
func stripImagesFromMessages(messages []llm.Message) []llm.Message {
	result := make([]llm.Message, len(messages))
	for i, msg := range messages {
		m := msg
		if m.Role == "user" {
			m.Content = stripInlineImages(m.Content)
		}
		result[i] = m
	}
	return result
}

func stripInlineImages(content string) string {
	// Replace base64 image data URIs with a text marker.
	// Pattern: data:image/...;base64,...
	replaced := content
	// Simple marker replacement for data URIs.
	if idx := findImageDataURI(content); idx >= 0 {
		replaced = content[:idx] + "[image]"
	}
	return replaced
}

func findImageDataURI(s string) int {
	// Search for "data:image/" prefix which indicates an inline image.
	return strings.Index(strings.ToLower(s), "data:image/")
}

func newBoundaryID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "compact-" + fmt.Sprintf("%d", b)
	}
	return "compact-" + hex.EncodeToString(b[:])
}
