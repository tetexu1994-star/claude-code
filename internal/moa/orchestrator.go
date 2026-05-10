package moa

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tetexu/tlaude-code/internal/llm"
	"github.com/tetexu/tlaude-code/internal/logging"
)

// Mode constants for result aggregation strategy.
const (
	ModeFastest    = "fastest"
	ModeConsensus  = "consensus"
	ModeMajority   = "majority"
	ModeSynthesize = "synthesize"
)

// ParallelResult captures a single provider's response in a parallel MoA call.
type ParallelResult struct {
	ProviderName string
	Content      string
	Latency      time.Duration
	Error        error
}

// MoAConfig configures the Mixture of Agents orchestrator.
type MoAConfig struct {
	Enabled       bool     `yaml:"enabled" json:"enabled"`
	ProviderNames []string `yaml:"providers" json:"providers"`
	Synthesizer   string   `yaml:"synthesizer" json:"synthesizer"`
	TimeoutSec    int      `yaml:"timeout_sec" json:"timeout_sec"`
	MaxParallel   int      `yaml:"max_parallel" json:"max_parallel"`
	Mode          string   `yaml:"mode" json:"mode"`
}

// MoAResult aggregates all provider responses and the final synthesized output.
type MoAResult struct {
	FinalContent string
	Mode         string
	Responses    []ParallelResult
	WinningName  string
	Duration     time.Duration
}

// Orchestrator coordinates parallel LLM calls and result aggregation.
type Orchestrator struct {
	registry    *llm.Registry
	synthesizer llm.Provider
	cfg         MoAConfig
	timeout     time.Duration
	maxParallel int
}

// NewOrchestrator creates an Orchestrator from the provider registry and config.
func NewOrchestrator(registry *llm.Registry, cfg MoAConfig) *Orchestrator {
	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	maxParallel := cfg.MaxParallel
	if maxParallel <= 0 {
		maxParallel = 3
	}

	o := &Orchestrator{
		registry:    registry,
		cfg:         cfg,
		timeout:     timeout,
		maxParallel: maxParallel,
	}

	if cfg.Synthesizer != "" {
		if p, ok := registry.Get(cfg.Synthesizer); ok {
			o.synthesizer = p
		}
	}

	return o
}

// Execute runs the same request against all configured providers in parallel,
// then aggregates results according to the configured mode.
func (o *Orchestrator) Execute(ctx context.Context, req llm.ChatRequest) (*MoAResult, error) {
	start := time.Now()

	providers := o.selectProviders()
	if len(providers) == 0 {
		return nil, fmt.Errorf("no providers available for MoA")
	}

	ctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()

	results := o.executeParallel(ctx, providers, req)

	result := &MoAResult{
		Responses: results,
		Duration:  time.Since(start),
	}

	var success []ParallelResult
	for _, r := range results {
		if r.Error == nil && r.Content != "" {
			success = append(success, r)
		}
	}
	if len(success) == 0 {
		return nil, fmt.Errorf("all %d providers failed", len(results))
	}

	switch o.cfg.Mode {
	case ModeFastest:
		result.Mode = ModeFastest
		result.FinalContent = success[0].Content
		result.WinningName = success[0].ProviderName

	case ModeConsensus:
		result.Mode = ModeConsensus
		allSame := true
		for i := 1; i < len(success); i++ {
			if success[i].Content != success[0].Content {
				allSame = false
				break
			}
		}
		if allSame {
			result.FinalContent = success[0].Content
		} else {
			result.FinalContent = BuildConsensusReport(success)
		}

	case ModeMajority:
		result.Mode = ModeMajority
		content, winner := majorityVote(success)
		result.FinalContent = content
		result.WinningName = winner

	default: // ModeSynthesize
		result.Mode = ModeSynthesize
		synthProvider := o.synthesizer
		if synthProvider == nil {
			result.FinalContent = success[0].Content
			result.WinningName = success[0].ProviderName
		} else {
			prompt := o.buildSynthesisPrompt(success, "")
			synthReq := llm.ChatRequest{
				Messages:    []llm.Message{{Role: "user", Content: prompt}},
				Temperature: req.Temperature,
				MaxTokens:   req.MaxTokens,
			}
			if cfg, ok := o.registry.GetConfig(synthProvider.Name()); ok && cfg.Model != "" {
				synthReq.Model = cfg.Model
			} else {
				synthReq.Model = req.Model
			}
			synthCtx, synthCancel := context.WithTimeout(ctx, o.timeout)
			defer synthCancel()
			synthResp, err := synthProvider.Chat(synthCtx, synthReq)
			if err != nil {
				logging.Warn("moa synthesizer failed, using first response", "error", err)
				result.FinalContent = success[0].Content
				result.WinningName = success[0].ProviderName
			} else {
				result.FinalContent = synthResp.Message.Content
			}
		}
	}

	return result, nil
}

// selectProviders returns the providers that should participate in MoA,
// respecting the configured ProviderNames filter.
func (o *Orchestrator) selectProviders() []llm.Provider {
	var names []string
	if len(o.cfg.ProviderNames) > 0 {
		names = o.cfg.ProviderNames
	} else {
		names = o.registry.List()
	}

	var providers []llm.Provider
	for _, name := range names {
		if p, ok := o.registry.Get(name); ok {
			providers = append(providers, p)
		}
	}
	return providers
}

// executeParallel fans out the request to all providers concurrently,
// limiting concurrency with a semaphore channel.
func (o *Orchestrator) executeParallel(ctx context.Context, providers []llm.Provider, req llm.ChatRequest) []ParallelResult {
	var wg sync.WaitGroup
	sem := make(chan struct{}, o.maxParallel)
	resultsCh := make(chan ParallelResult, len(providers))

	for _, p := range providers {
		wg.Add(1)
		go func(provider llm.Provider) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				resultsCh <- ParallelResult{
					ProviderName: provider.Name(),
					Error:        ctx.Err(),
				}
				return
			}

			// Use provider-specific model if configured, otherwise use the request model.
			provReq := req
			if cfg, ok := o.registry.GetConfig(provider.Name()); ok && cfg.Model != "" {
				provReq.Model = cfg.Model
			}

			start := time.Now()
			resp, err := provider.Chat(ctx, provReq)
			latency := time.Since(start)

			if err != nil {
				logging.Warn("moa provider failed", "provider", provider.Name(), "error", err)
				resultsCh <- ParallelResult{
					ProviderName: provider.Name(),
					Latency:      latency,
					Error:        err,
				}
				return
			}

			resultsCh <- ParallelResult{
				ProviderName: provider.Name(),
				Content:      resp.Message.Content,
				Latency:      latency,
			}
		}(p)
	}

	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	var results []ParallelResult
	for r := range resultsCh {
		results = append(results, r)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Latency < results[j].Latency
	})

	return results
}

// buildSynthesisPrompt constructs the prompt for the synthesizer provider.
func (o *Orchestrator) buildSynthesisPrompt(results []ParallelResult, userPrompt string) string {
	var sb strings.Builder
	sb.WriteString("The user asked: ")
	sb.WriteString(userPrompt)
	sb.WriteString("\n\nMultiple AI responses were generated. Synthesize them into the best possible single response, combining strengths and resolving conflicts:\n\n")

	for _, r := range results {
		sb.WriteString(fmt.Sprintf("[Provider: %s - %v]:\n", r.ProviderName, r.Latency.Round(time.Millisecond)))
		sb.WriteString(r.Content)
		sb.WriteString("\n---\n\n")
	}

	sb.WriteString("Synthesized response:")
	return sb.String()
}

// majorityVote returns the content that appears most frequently among results.
func majorityVote(results []ParallelResult) (content string, winner string) {
	counts := make(map[string]int)
	first := make(map[string]string) // content -> first provider name

	for _, r := range results {
		key := strings.TrimSpace(r.Content)
		counts[key]++
		if _, ok := first[key]; !ok {
			first[key] = r.ProviderName
		}
	}

	var maxCount int
	for k, c := range counts {
		if c > maxCount {
			maxCount = c
			content = k
			winner = first[k]
		}
	}

	return content, winner
}
