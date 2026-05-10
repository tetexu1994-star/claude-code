package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/tetexu/tlaude-code/internal/llm"
	"github.com/tetexu/tlaude-code/internal/moa"
)

// MoAMode defines the aggregation strategy.
type MoAMode string

const (
	MoAFastest    MoAMode = "fastest"
	MoAConsensus  MoAMode = "consensus"
	MoAMajority   MoAMode = "majority"
	MoASynthesize MoAMode = "synthesize"
)

// RunMoA executes the same prompt against multiple models in parallel,
// then aggregates the results according to the configured strategy.
// If models is empty, it uses def.ModelStrategy. If that's also empty,
// SelectModelsForTask is used to pick models automatically.
func (r *AgentRuntime) RunMoA(ctx context.Context, def *AgentDefinition, prompt string, opts *RunOptions, models []ModelConfig) (*AggregatedResult, error) {
	if opts == nil {
		opts = &RunOptions{}
	}

	// Resolve which models to use.
	cfgs := r.resolveModelConfigs(def, models)
	if len(cfgs) == 0 {
		return nil, fmt.Errorf("no models available for MoA execution")
	}

	start := time.Now()
	results, err := r.executeParallelModels(ctx, def, prompt, opts, cfgs)
	if err != nil {
		return nil, err
	}

	success := filterSuccess(results)
	if len(success) == 0 {
		return nil, fmt.Errorf("all %d model(s) failed for MoA", len(results))
	}

	mode := MoASynthesize
	if len(success) == 1 {
		mode = MoAFastest
	} else if len(cfgs) > 1 {
		mode = MoAMajority
	}

	result := &AggregatedResult{
		Results:   results,
		ModelCfgs: cfgs,
		TimeCost:  time.Since(start),
		Strategy:  string(mode),
	}

	result.aggregate(success, mode)

	return result, nil
}

// resolveModelConfigs builds the list of ModelConfig to use.
// Priority: explicit models param > def.ModelStrategy > SelectModelsForTask.
func (r *AgentRuntime) resolveModelConfigs(def *AgentDefinition, models []ModelConfig) []ModelConfig {
	if len(models) > 0 {
		for i := range models {
			if models[i].Weight == 0 {
				models[i].Weight = 1.0
			}
		}
		return models
	}

	if len(def.ModelStrategy) > 0 {
		cfgs := make([]ModelConfig, 0, len(def.ModelStrategy))
		for _, mr := range def.ModelStrategy {
			cfgs = append(cfgs, ModelConfig{
				Provider: mr.Provider,
				Model:    mr.Model,
				Weight:   1.0,
			})
		}
		return cfgs
	}

	return SelectModelsForTask(def.AgentType, r.llmReg)
}

// executeParallelModels runs the prompt against all model configs concurrently.
func (r *AgentRuntime) executeParallelModels(ctx context.Context, def *AgentDefinition, prompt string, opts *RunOptions, cfgs []ModelConfig) ([]*AgentRun, error) {
	var wg sync.WaitGroup
	resultsCh := make(chan *AgentRun, len(cfgs))

	for i := range cfgs {
		wg.Add(1)
		go func(cfg ModelConfig) {
			defer wg.Done()

			// Build a per-model copy of RunOptions with overridden provider/model.
			modelOpts := *opts
			if cfg.Provider != "" {
				modelOpts.SessionProvider = cfg.Provider
			}
			if cfg.Model != "" {
				modelOpts.SessionModel = cfg.Model
			}

			// Force the agent to use the specific model.
			perDef := *def
			perDef.ModelRef = ModelRef{
				Provider: cfg.Provider,
				Model:    cfg.Model,
			}

			result, err := r.RunAgent(ctx, &perDef, prompt, &modelOpts)
			if err != nil {
				result = &AgentRun{
					State:  AgentFailed,
					Error:  err.Error(),
					Prompt: prompt,
					Model:  cfg.Model,
				}
			}
			resultsCh <- result
		}(cfgs[i])
	}

	wg.Wait()
	close(resultsCh)

	var results []*AgentRun
	for r := range resultsCh {
		results = append(results, r)
	}

	return results, nil
}

// aggregate combines multiple agent results into a single final output.
func (r *AggregatedResult) aggregate(success []*AgentRun, mode MoAMode) {
	switch mode {
	case MoAFastest:
		r.Strategy = string(MoAFastest)
		r.Final = success[0].Result
		r.Consensus = 1.0

	case MoAConsensus:
		r.Strategy = string(MoAConsensus)
		allSame := true
		for i := 1; i < len(success); i++ {
			if success[i].Result != success[0].Result {
				allSame = false
				break
			}
		}
		if allSame {
			r.Final = success[0].Result
			r.Consensus = 1.0
		} else {
			r.Final = buildConsensusReport(success)
			r.Consensus = 0.0
		}

	case MoAMajority:
		r.Strategy = string(MoAMajority)
		content, count := majorityVoteResults(success)
		r.Final = content
		r.Consensus = float64(count) / float64(len(success))

	default: // MoASynthesize
		r.Strategy = string(MoASynthesize)
		r.Final = buildSynthesis(success)
		r.Consensus = 0.5
	}

	// Compute total token cost.
	for _, res := range r.Results {
		rate := estimateCostUSD(res.Provider, res.Model, res.TokensInput, res.TokensOutput)
		r.TokenCost += rate
	}
}

func filterSuccess(results []*AgentRun) []*AgentRun {
	var out []*AgentRun
	for _, r := range results {
		if r.GetState() == AgentCompleted && r.Result != "" {
			out = append(out, r)
		}
	}
	return out
}

func buildConsensusReport(results []*AgentRun) string {
	parallel := agentRunsToParallel(results)
	return moa.BuildConsensusReport(parallel)
}

func majorityVoteResults(results []*AgentRun) (content string, count int) {
	counts := make(map[string]int)
	for _, r := range results {
		key := strings.TrimSpace(r.Result)
		counts[key]++
	}
	var maxCount int
	for k, c := range counts {
		if c > maxCount {
			maxCount = c
			content = k
			count = c
		}
	}
	return
}

func buildSynthesis(results []*AgentRun) string {
	parallel := agentRunsToParallel(results)
	return moa.BuildSynthesisPrompt(parallel, "")
}

// agentRunsToParallel converts AgentRun results to moa.ParallelResult format.
func agentRunsToParallel(results []*AgentRun) []moa.ParallelResult {
	parallel := make([]moa.ParallelResult, 0, len(results))
	for _, r := range results {
		parallel = append(parallel, moa.ParallelResult{
			ProviderName: r.Provider + "/" + r.Model,
			Content:      r.Result,
		})
	}
	return parallel
}

// SelectModelsForTask picks appropriate models based on the agent type.
// llmReg is used to filter out providers that are not registered.
// This is used when no explicit ModelStrategy is configured.
func SelectModelsForTask(agentType string, llmReg *llm.Registry) []ModelConfig {
	candidates := selectModelsForTask(agentType)
	var available []ModelConfig
	for _, mc := range candidates {
		if llmReg != nil {
			if _, ok := llmReg.Get(mc.Provider); !ok {
				continue
			}
		}
		available = append(available, mc)
	}
	if len(available) == 0 {
		return candidates // fall back to candidates even if providers aren't registered
	}
	return available
}

func selectModelsForTask(agentType string) []ModelConfig {
	switch agentType {
	case "explore":
		return []ModelConfig{
			{Provider: "deepseek", Model: "deepseek-chat", Weight: 1.0},
		}
	case "code":
		return []ModelConfig{
			{Provider: "anthropic", Model: "claude-sonnet-4-20250514", Weight: 1.0},
		}
	case "review":
		return []ModelConfig{
			{Provider: "anthropic", Model: "claude-sonnet-4-20250514", Weight: 1.0},
			{Provider: "openai", Model: "gpt-4o", Weight: 0.8},
		}
	case "moa":
		return []ModelConfig{
			{Provider: "anthropic", Model: "claude-sonnet-4-20250514", Weight: 1.0},
			{Provider: "deepseek", Model: "deepseek-chat", Weight: 0.8},
			{Provider: "openai", Model: "gpt-4o", Weight: 0.8},
		}
	default:
		return []ModelConfig{
			{Provider: "anthropic", Model: "claude-sonnet-4-20250514", Weight: 1.0},
		}
	}
}

// estimateCostUSD estimates the USD cost based on tokens and rates.
func estimateCostUSD(provider, model string, inputTokens, outputTokens int) float64 {
	rates := map[string]struct{ input, output float64 }{
		"anthropic:claude-sonnet-4-20250514": {3.00, 15.00},
		"anthropic:claude-haiku-3-5":         {0.80, 4.00},
		"anthropic:claude-sonnet-4":          {3.00, 15.00},
		"openai:gpt-4o":                      {2.50, 10.00},
		"openai:gpt-4o-mini":                 {0.15, 0.60},
		"deepseek:deepseek-chat":             {0.27, 1.10},
		"deepseek:deepseek-reasoner":         {0.55, 2.19},
		"tongyi:qwen-max":                    {1.60, 6.40},
		"tongyi:qwen-plus":                   {0.80, 2.00},
		"zhipu:glm-4-plus":                   {1.00, 1.00},
		"siliconflow:deepseek-chat":          {0.27, 1.10},
	}

	key := provider + ":" + model
	r, ok := rates[key]
	if !ok {
		r = struct{ input, output float64 }{1.00, 3.00}
	}
	inputCost := float64(inputTokens) / 1_000_000.0 * r.input
	outputCost := float64(outputTokens) / 1_000_000.0 * r.output
	return inputCost + outputCost
}
