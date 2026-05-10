package cost

import (
	"fmt"
	"strings"
)

// Complexity classifies how demanding a user prompt is.
type Complexity int

const (
	ComplexitySimple  Complexity = iota // ls, cat, echo — simple commands
	ComplexityNormal                    // editing files, Q&A
	ComplexityComplex                   // MoA, reasoning, long code generation
)

// RouteResult describes the router's selection.
type RouteResult struct {
	Provider string
	Model    string
	Reason   string
}

// qualityScore maps provider+model to a rough quality score (1-10).
func qualityScore(provider, model string) int {
	key := provider + ":" + model
	scores := map[string]int{
		"anthropic:claude-sonnet-4-20250514": 10,
		"openai:gpt-4o":                      9,
		"deepseek:deepseek-reasoner":         8,
		"tongyi:qwen-max":                    8,
		"zhipu:glm-4-plus":                   7,
		"deepseek:deepseek-chat":             7,
		"openrouter:claude-sonnet-4":         10,
		"openrouter:gpt-4o":                  9,
		"tongyi:qwen-plus":                   6,
		"openai:gpt-4o-mini":                 5,
		"anthropic:claude-haiku-3-5":         5,
		"siliconflow:deepseek-chat":          7,
	}
	if s, ok := scores[key]; ok {
		return s
	}
	return 5
}

// Router selects the most appropriate provider+model based on task complexity.
type Router struct {
	preferredProvider string
	defaultModel      string
}

// NewRouter creates a cost-aware router.
func NewRouter(preferredProvider, defaultModel string) *Router {
	return &Router{
		preferredProvider: preferredProvider,
		defaultModel:      defaultModel,
	}
}

// Select picks the best provider+model for the given complexity and available providers.
func (r *Router) Select(complexity Complexity, availableProviders []string) *RouteResult {
	switch complexity {
	case ComplexitySimple:
		return r.selectCheapest(availableProviders)
	case ComplexityComplex:
		return r.selectStrongest(availableProviders)
	default:
		return r.selectDefault()
	}
}

// selectCheapest picks the provider with the lowest combined rate among available ones.
func (r *Router) selectCheapest(available []string) *RouteResult {
	type candidate struct {
		provider string
		model    string
		rate     float64
	}

	var best *candidate
	for _, name := range available {
		model := r.defaultModelForProvider(name)
		rate := GetRate(name, model)
		cost := rate.Input + rate.Output
		if best == nil || cost < best.rate {
			best = &candidate{provider: name, model: model, rate: cost}
		}
		for _, cheapModel := range cheapModels(name) {
			rate := GetRate(name, cheapModel)
			cost := rate.Input + rate.Output
			if best == nil || cost < best.rate {
				best = &candidate{provider: name, model: cheapModel, rate: cost}
			}
		}
	}

	if best == nil {
		return r.selectDefault()
	}
	return &RouteResult{
		Provider: best.provider,
		Model:    best.model,
		Reason:   fmt.Sprintf("cheapest available (est. $%.2f/M tokens)", best.rate),
	}
}

// selectStrongest picks the provider+model with the highest quality score.
func (r *Router) selectStrongest(available []string) *RouteResult {
	type candidate struct {
		provider string
		model    string
		score    int
	}

	var best *candidate
	for _, name := range available {
		model := r.defaultModelForProvider(name)
		score := qualityScore(name, model)
		if best == nil || score > best.score {
			best = &candidate{provider: name, model: model, score: score}
		}
		for _, strongModel := range strongModels(name) {
			score := qualityScore(name, strongModel)
			if best == nil || score > best.score {
				best = &candidate{provider: name, model: strongModel, score: score}
			}
		}
	}

	if best == nil {
		return r.selectDefault()
	}
	return &RouteResult{
		Provider: best.provider,
		Model:    best.model,
		Reason:   fmt.Sprintf("strongest available (quality score %d/10)", best.score),
	}
}

// selectDefault returns the user's preferred provider and model.
func (r *Router) selectDefault() *RouteResult {
	return &RouteResult{
		Provider: r.preferredProvider,
		Model:    r.defaultModel,
		Reason:   "fixed (user preference)",
	}
}

// defaultModelForProvider returns a typical model for a given provider.
func (r *Router) defaultModelForProvider(provider string) string {
	defaults := map[string]string{
		"anthropic":   "claude-sonnet-4-20250514",
		"openai":      "gpt-4o",
		"deepseek":    "deepseek-chat",
		"openrouter":  "claude-sonnet-4",
		"siliconflow": "deepseek-chat",
		"tongyi":      "qwen-max",
		"zhipu":       "glm-4-plus",
	}
	if m, ok := defaults[provider]; ok {
		return m
	}
	return r.defaultModel
}

// cheapModels returns known budget models for a provider.
func cheapModels(provider string) []string {
	switch provider {
	case "openai":
		return []string{"gpt-4o-mini"}
	case "anthropic":
		return []string{"claude-haiku-3-5"}
	case "deepseek":
		return []string{"deepseek-chat"}
	case "tongyi":
		return []string{"qwen-plus"}
	default:
		return nil
	}
}

// strongModels returns known high-quality models for a provider.
func strongModels(provider string) []string {
	switch provider {
	case "anthropic":
		return []string{"claude-sonnet-4-20250514"}
	case "openai":
		return []string{"gpt-4o"}
	case "deepseek":
		return []string{"deepseek-reasoner"}
	case "tongyi":
		return []string{"qwen-max"}
	case "zhipu":
		return []string{"glm-4-plus"}
	case "openrouter":
		return []string{"claude-sonnet-4", "gpt-4o"}
	default:
		return nil
	}
}

// ClassifyPrompt estimates task complexity from the total input token count.
func ClassifyPrompt(inputTokens int) Complexity {
	if inputTokens < 500 {
		return ComplexitySimple
	}
	if inputTokens < 3000 {
		return ComplexityNormal
	}
	return ComplexityComplex
}

// ClassifyPromptText estimates complexity from raw text content.
func ClassifyPromptText(text string) Complexity {
	tokens := estimateTokensFromString(text)
	lower := strings.ToLower(text)

	// Complex indicators.
	if strings.Contains(lower, "explain") || strings.Contains(lower, "analyze") ||
		strings.Contains(lower, "refactor") || strings.Contains(lower, "design") ||
		strings.Contains(lower, "architecture") || strings.Contains(lower, "implement") ||
		strings.Contains(lower, "review") {
		if tokens > 200 {
			return ComplexityComplex
		}
	}
	// Simple indicators.
	if strings.HasPrefix(lower, "ls ") || strings.HasPrefix(lower, "cat ") ||
		strings.HasPrefix(lower, "echo ") || strings.HasPrefix(lower, "pwd") ||
		strings.HasPrefix(lower, "cd ") || strings.HasPrefix(lower, "which ") ||
		strings.HasPrefix(lower, "whoami") {
		return ComplexitySimple
	}

	return ClassifyPrompt(tokens)
}
