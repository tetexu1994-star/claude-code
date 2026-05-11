package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tetexu/tlaude-code/internal/agent"
	"github.com/tetexu/tlaude-code/internal/config"
	"github.com/tetexu/tlaude-code/internal/cost"
	"github.com/tetexu/tlaude-code/internal/llm"
	"github.com/tetexu/tlaude-code/internal/logging"
	"github.com/tetexu/tlaude-code/internal/mcp"
	"github.com/tetexu/tlaude-code/internal/memory"
	"github.com/tetexu/tlaude-code/internal/moa"
	"github.com/tetexu/tlaude-code/internal/plan"
	"github.com/tetexu/tlaude-code/internal/plugin"
	"github.com/tetexu/tlaude-code/internal/plugin/lua"
	"github.com/tetexu/tlaude-code/internal/session"
	"github.com/tetexu/tlaude-code/internal/swarm"
	"github.com/tetexu/tlaude-code/internal/tool"
	"github.com/tetexu/tlaude-code/internal/tui"

	// Providers register themselves via init() functions.
	_ "github.com/tetexu/tlaude-code/internal/llm/anthropic"
	_ "github.com/tetexu/tlaude-code/internal/llm/deepseek"
	_ "github.com/tetexu/tlaude-code/internal/llm/openai"
	_ "github.com/tetexu/tlaude-code/internal/llm/openrouter"
	_ "github.com/tetexu/tlaude-code/internal/llm/siliconflow"
	_ "github.com/tetexu/tlaude-code/internal/llm/tongyi"
	_ "github.com/tetexu/tlaude-code/internal/llm/zhipu"
)

// Build-time version injection (set via -ldflags).
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func buildVersion() string {
	return fmt.Sprintf("Tlaude Code %s (commit: %s, built: %s, go: %s)", version, commit, date, runtime.Version())
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "Fatal error: %v\n", r)
			os.Exit(1)
		}
	}()

	var (
		provider    string
		modelName   string
		temperature float64
		maxTokens   int
		version     bool
		printMode   bool
		resume      bool
		sessionID   string

		// Agent flags.
		agentType    string
		moaFlag      bool
		moaStrategy  string
		listAgents   bool
	)

	flag.StringVar(&provider, "provider", "", "LLM provider to use (overrides config)")
	flag.StringVar(&modelName, "model", "", "Model name to use (overrides config)")
	flag.Float64Var(&temperature, "temperature", 0, "Temperature for generation (overrides config)")
	flag.IntVar(&maxTokens, "max-tokens", 0, "Max tokens for response (overrides config)")
	flag.BoolVar(&version, "version", false, "Print version and exit")
	flag.BoolVar(&printMode, "print", false, "Print a single response to stdout and exit (non-interactive)")
	flag.BoolVar(&resume, "resume", false, "Resume the most recent session")
	flag.StringVar(&sessionID, "session", "", "Resume a specific session by ID")
	flag.StringVar(&agentType, "agent", "", "Agent type to use (general/explore/code/review/moa)")
	flag.BoolVar(&moaFlag, "moa", false, "Enable MoA multi-model mode")
	flag.StringVar(&moaStrategy, "moa-strategy", "", "MoA strategy (fastest/consensus/majority/synthesize)")
	flag.BoolVar(&listAgents, "list-agents", false, "List all available agents and exit")
	flag.Parse()

	if version {
		fmt.Println(buildVersion())
		return
	}

	// List agents mode: print available agents and exit.
	if listAgents {
		store := agent.NewAgentDefStore()
		listAgentDefinitions(store)
		return
	}

	// Load config.
	cfg, err := config.Load()
	if err != nil {
		logging.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	if err := cfg.FirstRunWizard(); err != nil {
		logging.Error("setup failed", "error", err)
		os.Exit(1)
	}

	// CLI overrides.
	if provider != "" {
		cfg.Provider = provider
	}
	if modelName != "" {
		cfg.Model = modelName
	}
	if temperature > 0 {
		cfg.Temperature = temperature
	}
	if maxTokens > 0 {
		cfg.MaxTokens = maxTokens
	}

	// Agent CLI overrides.
	if agentType != "" {
		cfg.Agent.DefaultAgent = agentType
	}
	if moaFlag {
		cfg.MoA.Enabled = true
		cfg.Agent.MoA.Enabled = true
	}
	if moaStrategy != "" {
		cfg.MoA.Mode = moaStrategy
		cfg.Agent.MoA.DefaultStrategy = moaStrategy
	}

	// Register all configured providers into the global registry.
	reg := llm.GlobalRegistry()
	registerProviders(reg, cfg)

	// Resolve provider.
	selectedProvider, providerName, err := reg.SelectAvailable(cfg.Provider)
	if err != nil {
		logging.Info("no available provider found, probing all")
		if err := probeAllProviders(reg, cfg); err != nil {
			logging.Error("no available LLM provider", "error", err)
			os.Exit(1)
		}
		selectedProvider, providerName, err = reg.SelectAvailable(cfg.Provider)
		if err != nil {
			logging.Error("failed to select provider", "error", err)
			os.Exit(1)
		}
	}

	logging.Info("using provider", "provider", providerName, "model", cfg.Model)

	// Print mode: one-shot response to stdout.
	if printMode {
		if err := runPrintMode(cfg, selectedProvider); err != nil {
			logging.Error("print mode error", "error", err)
			os.Exit(1)
		}
		return
	}

	// Session store.
	sessStore := session.NewStore(cfg.SessionDir)

	// Cost tracker.
	costTracker, err := cost.NewTracker(cfg.SessionDir)
	if err != nil {
		logging.Warn("failed to create cost tracker", "error", err)
	}

	// Cost-aware router.
	costRouter := cost.NewRouter(cfg.Provider, cfg.Model)

	// Memory searcher.
	memSearch := memory.NewSearcher(cfg.SessionDir)
	memStore := memory.DefaultStore()
	if cfg.Memory.BaseDir != "" {
		memStore = memory.NewStore(cfg.Memory.BaseDir)
	}
	_ = memStore.EnsureDir()

	// MoA orchestrator (if enabled in config).
	var orchestrator *moa.Orchestrator
	if cfg.MoA.Enabled {
		orchestrator = moa.NewOrchestrator(reg, cfg.MoA)
		logging.Info("moa orchestrator created",
			"mode", cfg.MoA.Mode,
			"providers", len(cfg.MoA.ProviderNames),
			"max_parallel", cfg.MoA.MaxParallel,
		)
	}

	// MCP Manager.
	mcpManager := mcp.NewManager()
	if len(cfg.MCPServers) > 0 {
		parentCtx := context.Background()
		for _, srv := range cfg.MCPServers {
			if !srv.Enabled {
				continue
			}
			mcpCtx, mcpCancel := context.WithTimeout(parentCtx, 10*time.Second)
			var transport mcp.Transport
			if srv.Command != "" {
				cmd := exec.CommandContext(mcpCtx, srv.Command, srv.Args...)
				transport = mcp.NewStdioTransport(cmd)
			} else if srv.URL != "" {
				transport = mcp.NewSSETransport(srv.URL)
			} else {
				mcpCancel()
				logging.Warn("mcp server has no command or url", "name", srv.Name)
				continue
			}
			client := mcp.NewClient(transport)
			if err := mcpManager.Add(mcpCtx, srv.Name, client); err != nil {
				mcpCancel()
				logging.Warn("mcp server start failed", "name", srv.Name, "error", err)
				continue
			}
			mcpCancel()
			logging.Info("mcp server connected", "name", srv.Name)
		}
	}

	// Agent system initialization (Phase 2).
	agentStore := agent.NewAgentDefStore()
	if cfg.Agent.AgentsDir != "" {
		if err := agentStore.LoadUserAgents(cfg.Agent.AgentsDir); err != nil {
			logging.Debug("no user agents loaded", "dir", cfg.Agent.AgentsDir, "error", err)
		}
	}
	toolReg := tool.DefaultRegistry()
	agentRuntime := agent.NewAgentRuntime(agentStore, toolReg, reg)
	agentRuntime.SetMemoryStore(memStore)
	tm := tool.SharedTaskManager()

	// Plan mode setup (Phase 3).
	planStore := plan.NewPlanStore()
	_ = planStore.LoadAll() // load any persisted plans
	planManager := plan.NewManager(planStore)

	// Plugin system setup.
	pluginsDir := cfg.Plugins.Dir
	logging.Info("plugin system", "dir", pluginsDir)
	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		logging.Warn("failed to create plugins directory", "dir", pluginsDir, "error", err)
	}
	pluginLoader := plugin.NewLoader(pluginsDir)
	pluginRegistry := plugin.NewRegistry()
	pluginManager := plugin.NewManager(pluginsDir, pluginLoader, pluginRegistry, lua.Options{})
	pluginCtx := context.Background()
	if err := pluginManager.Start(pluginCtx); err != nil {
		logging.Warn("plugin system start failed", "error", err)
	} else {
		defer pluginManager.Stop()
	}
	if err := pluginManager.RegisterPluginTools(toolReg); err != nil {
		logging.Warn("failed to register plugin tools", "error", err)
	}

	// Wire EnterPlanMode/ExitPlanMode tools to the plan manager.
	tool.SetPlanBridge(&tool.PlanBridge{
		EnterPlan: func(planContent, scope string) (string, error) {
			if scope == "" {
				scope = "Implementation Plan"
			}
			p := planManager.BuildFromDescription(scope, planContent)
			if err := planManager.Submit(p.ID); err != nil {
				return "", err
			}
			if err := planStore.Save(p); err != nil {
				logging.Warn("failed to save plan", "error", err)
			}
			logging.Info("plan mode entered via tool", "plan_id", p.ID, "title", p.Title, "steps", len(p.Steps))
			return p.ID, nil
		},
		ExitPlan: func(summary string) (string, string, error) {
			active := planManager.Active()
			if active == nil {
				return "", "", fmt.Errorf("no active plan")
			}
			active.Description += "\n\n## Exit Summary\n" + summary
			planStore.Save(active)
			// Clear active plan on exit.
			planManager.ClearActive()
			logging.Info("plan mode exited via tool", "plan_id", active.ID, "summary", summary)
			return active.ID, string(active.Status), nil
		},
	})

	// Wire AgentRuntime bridge to AgentTool.
	if agentTool, ok := toolReg.Get("Agent"); ok {
		if at, ok2 := agentTool.(*tool.AgentTool); ok2 {
			at.SetRuntimeBridge(
				agentRuntime.RunAgentByType,
				agentRuntime.RunMoAByType,
			)
			// Register external subprocess backends from config.
			if len(cfg.Agent.Backends) > 0 {
				for name, be := range cfg.Agent.Backends {
					timeout := time.Duration(be.Timeout) * time.Second
					if timeout <= 0 {
						timeout = 120 * time.Second
					}
					backend := agent.NewSubprocessBackend(name, be.Command, be.Args, timeout)
					at.RegisterBackend(name, agent.NewBackendAdapter(backend))
				}
			} else {
				at.RegisterBackend("claude-code", agent.NewBackendAdapter(agent.NewClaudeCodeBackend(120*time.Second)))
				at.RegisterBackend("hermes", agent.NewBackendAdapter(agent.NewHermesBackend(120*time.Second)))
			}
		}
	}

	// Wire AgentRuntime bridge to TaskManager.
	tm.SetAgentRuntimeBridge(
		func(ctx context.Context, agentType, prompt string) (string, error) {
			def, ok := agentStore.Get(agentType)
			if !ok {
				return "", fmt.Errorf("agent type %q not found", agentType)
			}
			id, err := agentRuntime.RunAgentAsync(ctx, def, prompt, nil)
			return id, err
		},
		agentRuntime.StopAgent,
		func(agentID string) (string, string, bool) {
			run, ok := agentRuntime.GetAgent(agentID)
			if !ok {
				return "", "", false
			}
			return string(run.State), run.Result, true
		},
	)

	logging.Info("agent system initialized",
		"default_agent", cfg.Agent.DefaultAgent,
		"builtin_agents", agentStore.CountBuiltIn(),
		"total_agents", agentStore.Count(),
	)

	// Swarm/Teams system initialization.
	var swarmStore *swarm.SwarmStore
	if cfg.Swarm.Enabled {
		var err error
		swarmStore, err = swarm.NewSwarmStore()
		if err != nil {
			logging.Warn("failed to create swarm store", "error", err)
		} else {
			backend := swarm.NewInProcessBackend(agentRuntime, agentStore, reg, toolReg)
			backend.SetSwarmStore(swarmStore)
			backend.SetContext(context.Background())
			swarmStore.SetExecutor(backend)
			logging.Info("swarm system initialized",
				"teams_dir", swarm.TeamsDir(),
				"max_teammates", cfg.Swarm.MaxTeammates,
			)
		}
	}

	// Launch TUI.
	var model tui.Model
	if sessionID != "" {
		sess, err := sessStore.Load(sessionID)
		if err != nil {
			logging.Error("failed to load session", "id", sessionID, "error", err)
			os.Exit(1)
		}
		model = tui.NewModel(cfg, selectedProvider, sessStore, orchestrator, costTracker, costRouter, memSearch, memStore, mcpManager, agentStore, agentRuntime, toolReg, tm, planManager, pluginManager, nil, swarmStore)
		model.SetSession(sess)
	} else if resume {
		sess, err := sessStore.Latest()
		if err != nil {
			logging.Error("failed to list sessions", "error", err)
			os.Exit(1)
		}
		model = tui.NewModel(cfg, selectedProvider, sessStore, orchestrator, costTracker, costRouter, memSearch, memStore, mcpManager, agentStore, agentRuntime, toolReg, tm, planManager, pluginManager, nil, swarmStore)
		if sess != nil {
			model.SetSession(sess)
		} else {
			logging.Info("no sessions to resume")
		}
	} else {
		model = tui.NewModel(cfg, selectedProvider, sessStore, orchestrator, costTracker, costRouter, memSearch, memStore, mcpManager, agentStore, agentRuntime, toolReg, tm, planManager, pluginManager, nil, swarmStore)
	}

	p := tea.NewProgram(&model, tea.WithAltScreen())

	// Handle signals for graceful shutdown.
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signalCh
		if model.Quitting() {
			return
		}
		model.SaveSession()
		p.Quit()
	}()

	if _, err := p.Run(); err != nil {
		logging.Error("TUI error", "error", err)
		os.Exit(1)
	}
}

// defaultModels maps each provider to its default model name.
var defaultModels = map[string]string{
	"anthropic":   "claude-sonnet-4-20250514",
	"deepseek":    "deepseek-chat",
	"openai":      "gpt-4o",
	"openrouter":  "openrouter/auto",
	"siliconflow": "Qwen/Qwen2.5-72B-Instruct",
	"tongyi":      "qwen-plus",
	"zhipu":       "glm-4-plus",
}

// registerProviders creates provider instances from registered factories and
// adds them to the global registry using each provider's config (API key, etc.).
func registerProviders(reg *llm.Registry, cfg *config.Config) {
	providerNames := []string{
		"anthropic", "openai", "deepseek", "openrouter",
		"siliconflow", "tongyi", "zhipu",
	}

	for _, name := range providerNames {
		factory, ok := llm.GetFactory(name)
		if !ok {
			logging.Debug("no factory registered for provider", "name", name)
			continue
		}

		apiKey := cfg.GetAPIKey(name)
		if apiKey == "" {
			logging.Debug("no API key for provider, skipping registration", "name", name)
			continue
		}

		provCfg := llm.DefaultConfig()
		provCfg.APIKey = apiKey

		// Apply provider-specific base URLs.
		switch name {
		case "anthropic":
			provCfg.BaseURL = "https://api.anthropic.com"
		case "openai":
			provCfg.BaseURL = "https://api.openai.com/v1"
		case "deepseek":
			provCfg.BaseURL = "https://api.deepseek.com"
		case "openrouter":
			provCfg.BaseURL = "https://openrouter.ai/api"
		case "siliconflow":
			provCfg.BaseURL = "https://api.siliconflow.cn/v1"
		case "tongyi":
			provCfg.BaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
		case "zhipu":
			provCfg.BaseURL = "https://open.bigmodel.cn/api/paas/v4"
		}

			// If model is still the old Anthropic-only default, swap to
			// a provider-specific default when using a different provider.
			if cfg.Model == "claude-sonnet-4-20250514" && name != "anthropic" && cfg.Provider == name {
				cfg.Model = defaultModels[name]
			}


		if err := reg.Register(name, factory, provCfg); err != nil {
			logging.Warn("failed to register provider", "name", name, "error", err)
		}
	}
}

// runPrintMode reads stdin and prints a single LLM response.
func runPrintMode(cfg *config.Config, provider llm.Provider) error {
	var input strings.Builder
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		// Pipe input.
		b := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(b)
			if n > 0 {
				input.Write(b[:n])
			}
			if err != nil {
				break
			}
		}
	}

	prompt := strings.TrimSpace(input.String())
	if prompt == "" {
		// Read remaining args as prompt.
		prompt = strings.Join(flag.Args(), " ")
	}
	if prompt == "" {
		return fmt.Errorf("no prompt provided; pipe content or pass arguments")
	}

	ctx := context.Background()
	resp, err := provider.Chat(ctx, llm.ChatRequest{
		Model:       cfg.Model,
		Messages:    []llm.Message{{Role: "user", Content: prompt}},
		Temperature: cfg.Temperature,
		MaxTokens:   cfg.MaxTokens,
	})
	if err != nil {
		return fmt.Errorf("chat failed: %w", err)
	}

	fmt.Println(resp.Message.Content)
	return nil
}

func listAgentDefinitions(store *agent.AgentDefStore) {
	defs := store.List()
	if len(defs) == 0 {
		fmt.Println("No agents registered.")
		return
	}
	fmt.Println("Available agents:")
	fmt.Println("------------------")
	for _, def := range defs {
		source := def.Source
		if source == "" {
			source = "user"
		}
		fmt.Printf("  %-12s %s", def.AgentType, def.Name)
		if def.Source != "built-in" {
			fmt.Printf(" (%s)", source)
		}
		fmt.Println()
		fmt.Printf("               %s", def.Description)
		if def.ModelRef.Model != "" {
			fmt.Printf(" [model: %s]", def.ModelRef.Model)
		}
		if def.ModelRef.Provider != "" {
			fmt.Printf(" [provider: %s]", def.ModelRef.Provider)
		}
		if def.Background {
			fmt.Printf(" [background]")
		}
		fmt.Println()
	}
	fmt.Println()
}

func probeAllProviders(reg *llm.Registry, cfg *config.Config) error {
	ctx := context.Background()
	reg.ProbeAll(ctx)

	statuses := reg.AllStatus()
	for _, s := range statuses {
		if cfg.GetAPIKey(s.Name) != "" {
			logging.Debug("provider status", "name", s.Name, "available", s.Available, "latency", s.Latency)
		}
	}

	_, _, err := reg.SelectAvailable(cfg.Provider)
	return err
}
