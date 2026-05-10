package cost

// Rate is the cost per million tokens (USD).
type Rate struct {
	Input  float64
	Output float64
}

// GetRate returns the pricing rate for a provider+model pair.
// Returns a default estimated rate when the specific model is unknown.
func GetRate(provider, model string) Rate {
	rates := map[string]Rate{
		// DeepSeek
		"deepseek:deepseek-chat":     {Input: 0.27, Output: 1.10},
		"deepseek:deepseek-reasoner": {Input: 0.55, Output: 2.19},
		// Anthropic
		"anthropic:claude-sonnet-4-20250514": {Input: 3.00, Output: 15.00},
		"anthropic:claude-haiku-3-5":         {Input: 0.80, Output: 4.00},
		// OpenAI
		"openai:gpt-4o":      {Input: 2.50, Output: 10.00},
		"openai:gpt-4o-mini": {Input: 0.15, Output: 0.60},
		// SiliconFlow (serves DeepSeek models at similar rates)
		"siliconflow:deepseek-chat": {Input: 0.27, Output: 1.10},
		// Tongyi / Qwen
		"tongyi:qwen-max":  {Input: 1.60, Output: 6.40},
		"tongyi:qwen-plus": {Input: 0.80, Output: 2.00},
		// Zhipu / GLM
		"zhipu:glm-4-plus": {Input: 1.00, Output: 1.00},
	}

	key := provider + ":" + model
	if r, ok := rates[key]; ok {
		return r
	}
	return Rate{Input: 1.00, Output: 3.00}
}

// EstimateCost estimates the USD cost for a call given token counts.
// Prices are approximate and may not reflect current market rates.
func EstimateCost(provider, model string, inputTokens, outputTokens int) float64 {
	rate := GetRate(provider, model)
	inputCost := float64(inputTokens) / 1_000_000.0 * rate.Input
	outputCost := float64(outputTokens) / 1_000_000.0 * rate.Output
	return inputCost + outputCost
}

// estimateTokensFromString estimates token count from a UTF-8 string.
// Uses a more accurate heuristic: ~4 characters per token for English,
// ~1.5 characters per token for CJK (Chinese/Japanese/Korean).
func estimateTokensFromString(s string) int {
	if s == "" {
		return 1
	}
	var cjkCount, asciiCount int
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF || r >= 0x3040 && r <= 0x30FF || r >= 0xAC00 && r <= 0xD7AF {
			cjkCount++
		} else {
			asciiCount++
		}
	}
	// CJK: ~1.5 chars/token, ASCII: ~4 chars/token
	return max(1, cjkCount/2+asciiCount/4)
}
