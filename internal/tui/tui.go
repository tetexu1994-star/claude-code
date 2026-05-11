package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tetexu/tlaude-code/internal/agent"
	"github.com/tetexu/tlaude-code/internal/compact"
	"github.com/tetexu/tlaude-code/internal/config"
	"github.com/tetexu/tlaude-code/internal/cost"
	"github.com/tetexu/tlaude-code/internal/llm"
	"github.com/tetexu/tlaude-code/internal/logging"
	"github.com/tetexu/tlaude-code/internal/mcp"
	"github.com/tetexu/tlaude-code/internal/memory"
	"github.com/tetexu/tlaude-code/internal/moa"
	"github.com/tetexu/tlaude-code/internal/plan"
	"github.com/tetexu/tlaude-code/internal/plugin"
	"github.com/tetexu/tlaude-code/internal/sandbox"
	"github.com/tetexu/tlaude-code/internal/session"
	"github.com/tetexu/tlaude-code/internal/tool"
	"github.com/tetexu/tlaude-code/internal/tools"
)

var (
	chatStyle = lipgloss.NewStyle().
			PaddingLeft(1)

	statusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("63")).
			Foreground(lipgloss.Color("15")).
			Padding(0, 1)

	userMsgStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39"))

	assistantMsgStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("82"))

	systemMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	codeBlockStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Padding(0, 1).
			MarginLeft(2)

	codeHeaderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			MarginLeft(2)

	moaDetailStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))
)

// Model is the Bubble Tea model for the TUI.
type Model struct {
	messages  []llm.Message
	input     textarea.Model
	chatView  viewport.Model
	cfg       *config.Config
	provider  llm.Provider
	sessStore *session.Store
	session   *session.Session
	width     int
	height    int
	ready     bool
	streaming bool
	streamErr error
	streamBuf strings.Builder
	streamCh  <-chan llm.Chunk
	quitting  bool
	statusMsg string
	showHelp  bool

	// Approval flow.
	pendingApproval *ApprovalRequest
	waitingApproval bool

	// Diff view.
	diffViewActive bool
	diffContent    string
	diffFilePath   string

	// Current working directory for tool execution.
	cwd string

	// Track pending tool calls for multi-tool responses.
	pendingToolCalls []llm.ToolCall

	// MoA (Mixture of Agents).
	moaOrchestrator *moa.Orchestrator
	moaEnabled      bool
	moaResults      []moa.ParallelResult
	moaResult       *moa.MoAResult
	moaExecuting    bool

	// Sandbox.
	sandboxer sandbox.Sandboxer

	// Cost tracking.
	costTracker *cost.Tracker

	// Smart routing.
	costRouter   *cost.Router
	smartRouting bool
	routedResult *cost.RouteResult

	// Memory search.
	memorySearch *memory.Searcher

	// MCP.
	mcpManager *mcp.Manager

	// Agent system (Phase 2).
	agentType    string
	agentStore   *agent.AgentDefStore
	agentRuntime *agent.AgentRuntime
	toolReg      *tool.Registry
	taskManager  *tool.TaskManager

	// Plan mode (Phase 3).
	planManager *plan.Manager

	// Plugin system.
	pluginManager *plugin.Manager

	// Compact system.
	compactManager *compact.Manager

	// Context for cancellation.
	ctx    context.Context
	cancel context.CancelFunc
}

// NewModel creates a new TUI model.
func NewModel(cfg *config.Config, provider llm.Provider, sessStore *session.Store, orchestrator *moa.Orchestrator, costTracker *cost.Tracker, costRouter *cost.Router, memSearch *memory.Searcher, mcpManager *mcp.Manager, agentStore *agent.AgentDefStore, agentRuntime *agent.AgentRuntime, toolReg *tool.Registry, taskManager *tool.TaskManager, planManager *plan.Manager, pluginManager *plugin.Manager, compactManager *compact.Manager) Model {
	ta := textarea.New()
	ta.Placeholder = "Type your message... (Enter to send, Ctrl+C to quit)"
	ta.ShowLineNumbers = false
	ta.SetHeight(3)
	ta.Focus()
	ta.CharLimit = 32000

	vp := viewport.New(80, 20)

	moaEnabled := cfg.MoA.Enabled && orchestrator != nil
	smartRouting := cfg.SmartRouting

	sb, err := sandbox.New(cfg.Sandbox)
	if err != nil {
		logging.Warn("failed to create sandbox, using Direct mode", "error", err)
		cfg.Sandbox.Mode = sandbox.ModeOff
		sb, _ = sandbox.New(cfg.Sandbox)
	}

	ctx, cancel := context.WithCancel(context.Background())

	cwd, _ := os.Getwd()

	m := Model{
		messages:        make([]llm.Message, 0),
		input:           ta,
		chatView:        vp,
		cfg:             cfg,
		provider:        provider,
		sessStore:       sessStore,
		cwd:             cwd,
		moaOrchestrator: orchestrator,
		moaEnabled:      moaEnabled,
		sandboxer:       sb,
		costTracker:     costTracker,
		costRouter:      costRouter,
		memorySearch:    memSearch,
		mcpManager:      mcpManager,
		smartRouting:    smartRouting,
		agentType:       cfg.Agent.DefaultAgent,
		agentStore:      agentStore,
		agentRuntime:    agentRuntime,
		toolReg:         toolReg,
		taskManager:     taskManager,
		planManager:     planManager,
		pluginManager:   pluginManager,
		compactManager:  compactManager,
		ctx:             ctx,
		cancel:          cancel,
	}
	if m.compactManager == nil {
		compactCfg := compact.Config{
			Enabled:          cfg.Compact.Enabled,
			AutoCompact:      cfg.Compact.AutoCompact,
			TokenBudget:      cfg.Compact.TokenBudget,
			MaxSummaryTokens: 20000,
			MaxPTLRetries:    3,
		}
		m.compactManager = compact.NewManager(compactCfg)
	}
	m.statusMsg = m.buildStatusMsg()
	return m
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		tea.EnterAltScreen,
	)
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

		m.input.SetWidth(msg.Width - 4)
		m.chatView.Width = msg.Width - 2
		m.chatView.Height = msg.Height - 7

		cmds = append(cmds, m.rebuildChat())

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			if m.diffViewActive {
				m.diffViewActive = false
				cmds = append(cmds, m.rebuildChat())
				break
			}
			m.quitting = true
			m.cancel()
			return m, tea.Quit
		}

		if m.diffViewActive {
			break
		}

		// Approval key handling captures keystrokes even while streaming.
		if m.waitingApproval && m.pendingApproval != nil {
			switch msg.String() {
			case "y":
				cmds = append(cmds, m.approveToolCall(m.pendingApproval, false))
			case "n":
				cmds = append(cmds, m.rejectToolCall(m.pendingApproval))
			case "d":
				m.diffViewActive = true
				cmds = append(cmds, m.rebuildChat())
			case "a":
				cmds = append(cmds, m.approveToolCall(m.pendingApproval, true))
			}
			break
		}

		if m.showHelp {
			m.showHelp = false
			cmds = append(cmds, m.rebuildChat())
			break
		}

		switch msg.String() {
		case "ctrl+h":
			m.showHelp = true
			cmds = append(cmds, m.rebuildChat())
		}

		if m.streaming {
			break
		}

		switch msg.String() {
		case "enter":
			input := strings.TrimSpace(m.input.Value())
			if input == "" {
				break
			}
			if strings.HasPrefix(input, "/") {
				cmds = append(cmds, m.handleCommand(input))
			} else {
				cmds = append(cmds, m.sendMessage(input))
			}
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			cmds = append(cmds, cmd)
		}

	case streamChunkMsg:
		if msg.Error != nil {
			m.streamErr = msg.Error
			m.streaming = false
			m.statusMsg = fmt.Sprintf("Error: %v", msg.Error)
		} else if msg.Done {
			if len(msg.ToolCalls) > 0 {
				m.streamBuf.Reset()
				m.streaming = false
				cmds = append(cmds, m.handleToolCalls(msg.ToolCalls))
			} else {
				content := m.streamBuf.String()
				m.streamBuf.Reset()
				m.streaming = false
				m.messages = append(m.messages, llm.Message{Role: "assistant", Content: content})
				m.recordCost(content)
				m.statusMsg = m.buildStatusMsg()
				if m.session != nil {
					m.session.Messages = m.messages
					if err := m.sessStore.Save(m.session); err != nil {
						logging.Warn("failed to save session", "error", err)
					}
				}
			}
		} else {
			m.streamBuf.WriteString(msg.Content)
			cmds = append(cmds, m.nextChunk())
		}
		cmds = append(cmds, m.rebuildChat())

	case moaResultMsg:
		m.moaExecuting = false
		if msg.Error != nil {
			m.streamErr = msg.Error
			m.streaming = false
			m.statusMsg = fmt.Sprintf("MoA Error: %v", msg.Error)
		} else if msg.Result != nil {
			m.streaming = false
			m.moaResult = msg.Result
			m.moaResults = msg.Result.Responses
			m.messages = append(m.messages, llm.Message{Role: "assistant", Content: msg.Result.FinalContent})
			m.recordCost(msg.Result.FinalContent)
			m.statusMsg = moa.BuildMoASummary(msg.Result)
			if m.session != nil {
				m.session.Messages = m.messages
				if err := m.sessStore.Save(m.session); err != nil {
					logging.Warn("failed to save session", "error", err)
				}
			}
		}
		cmds = append(cmds, m.rebuildChat())
	}

	return m, tea.Batch(cmds...)
}

// View implements tea.Model.
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	if !m.ready {
		return "Initializing...\n"
	}

	if m.showHelp {
		return m.renderHelp()
	}

	chatContent := m.chatView.View()

	inputStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")).
		Width(m.width - 2)
	inputView := inputStyle.Render(m.input.View())

	statusText := m.statusMsg
	if m.streaming {
		statusText += " ●"
	}
	statusBar := statusBarStyle.Width(m.width).Render(statusText)

	// Full-screen diff view
	if m.diffViewActive {
		diffContent := renderDiffFull(m.diffContent, m.diffFilePath)
		return chatStyle.Render(diffContent)
	}

	// Approval bar below chat when waiting for user input
	if m.waitingApproval && m.pendingApproval != nil {
		approvalBar := renderApprovalBar(m.pendingApproval)
		chatContent = chatContent + "\n" + approvalBar
	}

	return chatStyle.Render(lipgloss.JoinVertical(
		lipgloss.Top,
		chatContent,
		inputView,
		statusBar,
	))
}

type streamChunkMsg struct {
	Content   string
	Done      bool
	Error     error
	ToolCalls []llm.ToolCall
}

type moaResultMsg struct {
	Result *moa.MoAResult
	Error  error
}

// sendMessage appends the user message and starts an LLM call.
// When MoA is enabled, it executes a parallel multi-provider call.
func (m *Model) sendMessage(input string) tea.Cmd {
	m.messages = append(m.messages, llm.Message{Role: "user", Content: input})
	m.streamErr = nil
	m.input.Reset()
	m.moaResults = nil
	m.moaResult = nil

	if m.session != nil {
		m.session.Messages = m.messages
		if err := m.sessStore.Save(m.session); err != nil {
			logging.Warn("failed to save session", "error", err)
		}
	}

	// Smart routing: classify prompt and select provider.
	if m.smartRouting && m.costRouter != nil {
		complexity := cost.ClassifyPromptText(input)
		available := llm.GlobalRegistry().List()
		result := m.costRouter.Select(complexity, available)
		m.routedResult = result

		if result.Provider != m.provider.Name() || result.Model != m.cfg.Model {
			if p, ok := llm.GlobalRegistry().Get(result.Provider); ok {
				m.provider = p
				// Update config model if router picked a different one.
				if result.Model != "" {
					m.cfg.Model = result.Model
				}
				logging.Info("smart route",
					"complexity", complexity,
					"provider", result.Provider,
					"model", result.Model,
					"reason", result.Reason,
				)
				m.messages = append(m.messages, llm.Message{
					Role:    "system",
					Content: fmt.Sprintf("🔄 Routed to %s/%s (%s)", result.Provider, result.Model, result.Reason),
				})
			}
		}
	}

	// MoA mode: parallel multi-provider non-streaming call.
	if m.moaEnabled && m.moaOrchestrator != nil {
		m.moaExecuting = true
		m.streaming = true
		m.streamBuf.Reset()
		m.statusMsg = fmt.Sprintf("MoA: calling %d providers...", m.moaProviderCount())

		// Copy cfg values to avoid data race from the tea.Cmd goroutine.
		cfgModel := m.cfg.Model
		cfgTemperature := m.cfg.Temperature
		cfgMaxTokens := m.cfg.MaxTokens

		req := llm.ChatRequest{
			Model:       cfgModel,
			Messages:    m.messages,
			Temperature: cfgTemperature,
			MaxTokens:   cfgMaxTokens,
		}

		return func() tea.Msg {
			result, err := m.moaOrchestrator.Execute(m.ctx, req)
			if err != nil {
				return moaResultMsg{Error: err}
			}
			return moaResultMsg{Result: result}
		}
	}

	// Standard single-provider streaming mode.
	m.streaming = true
	m.streamBuf.Reset()
	m.statusMsg = m.buildStatusMsg() + " | thinking..."

	// Copy cfg values to avoid data race from the tea.Cmd goroutine.
	cfgModel := m.cfg.Model
	cfgTemperature := m.cfg.Temperature
	cfgMaxTokens := m.cfg.MaxTokens

	req := llm.ChatRequest{
		Model:       cfgModel,
		Messages:    m.messages,
		Stream:      true,
		Temperature: cfgTemperature,
		MaxTokens:   cfgMaxTokens,
		Tools:       tools.GetToolDefinitionsFromRegistry(m.toolReg),
	}

	return func() tea.Msg {
		ch, err := m.provider.ChatStream(m.ctx, req)
		if err != nil {
			return streamChunkMsg{Error: err}
		}
		m.streamCh = ch
		return readChunk(ch)
	}
}

// moaProviderCount returns the number of providers that would participate in MoA.
func (m *Model) moaProviderCount() int {
	if m.moaOrchestrator == nil {
		return 0
	}
	names := m.cfg.MoA.ProviderNames
	if len(names) == 0 {
		reg := llm.GlobalRegistry()
		names = reg.List()
	}
	count := 0
	reg := llm.GlobalRegistry()
	for _, name := range names {
		if _, ok := reg.Get(name); ok {
			count++
		}
	}
	return count
}

// nextChunk returns a command that reads the next chunk from the stream.
func (m *Model) nextChunk() tea.Cmd {
	ch := m.streamCh
	return func() tea.Msg {
		return readChunk(ch)
	}
}

// readChunk reads a single chunk from a stream channel and converts it to a streamChunkMsg.
func readChunk(ch <-chan llm.Chunk) streamChunkMsg {
	chunk, ok := <-ch
	if !ok {
		return streamChunkMsg{Done: true}
	}
	if chunk.Error != nil {
		return streamChunkMsg{Error: chunk.Error}
	}
	if chunk.Done {
		return streamChunkMsg{Done: true, ToolCalls: chunk.ToolCalls}
	}
	return streamChunkMsg{Content: chunk.Content}
}

func (m *Model) handleCommand(input string) tea.Cmd {
	defer m.input.Reset()

	switch {
	case input == "/help":
		m.showHelp = true
		return m.rebuildChat()

	case input == "/quit" || input == "/exit":
		m.quitting = true
		m.cancel()
		return tea.Quit

	case input == "/config":
		info := fmt.Sprintf(
			"Provider: %s\nModel: %s\nTemperature: %.1f\nMaxTokens: %d",
			m.cfg.Provider, m.cfg.Model, m.cfg.Temperature, m.cfg.MaxTokens,
		)
		if m.moaEnabled {
			info += fmt.Sprintf("\nMoA: enabled (mode=%s, max_parallel=%d, timeout=%ds)",
				m.cfg.MoA.Mode, m.cfg.MoA.MaxParallel, m.cfg.MoA.TimeoutSec)
			if len(m.cfg.MoA.ProviderNames) > 0 {
				info += "\nMoA Providers: " + strings.Join(m.cfg.MoA.ProviderNames, ", ")
			}
		} else {
			info += "\nMoA: disabled"
		}
		m.messages = append(m.messages, llm.Message{Role: "system", Content: info})
		return m.rebuildChat()

	case input == "/clear":
		m.messages = make([]llm.Message, 0)
		m.chatView.SetContent("")
		m.moaResults = nil
		m.moaResult = nil
		if m.session != nil {
			m.session.Messages = m.messages
			m.sessStore.Save(m.session)
		}
		return nil

	case input == "/save":
		if m.session == nil {
			m.session = m.sessStore.New(m.provider.Name(), m.cfg.Model)
		}
		m.session.Messages = m.messages
		if err := m.sessStore.Save(m.session); err != nil {
			return func() tea.Msg {
				return streamChunkMsg{Error: fmt.Errorf("save: %w", err)}
			}
		}
		m.statusMsg = fmt.Sprintf("Session saved (%s)", m.session.ID[:8])
		return m.rebuildChat()

	case input == "/moa" || input == "/moa status":
		info := m.buildMoAStatus()
		m.messages = append(m.messages, llm.Message{Role: "system", Content: info})
		return m.rebuildChat()

	case strings.HasPrefix(input, "/moa strategy "):
		strategy := strings.TrimPrefix(input, "/moa strategy ")
		strategy = strings.TrimSpace(strategy)
		valid := map[string]bool{"fastest": true, "consensus": true, "majority": true, "synthesize": true}
		if !valid[strategy] {
			m.messages = append(m.messages, llm.Message{Role: "system", Content: fmt.Sprintf("Invalid MoA strategy: %s. Use: fastest, consensus, majority, synthesize", strategy)})
			return m.rebuildChat()
		}
		m.cfg.MoA.Mode = strategy
		m.statusMsg = m.buildStatusMsg()
		m.messages = append(m.messages, llm.Message{Role: "system", Content: "MoA strategy set to: " + strategy})
		return m.rebuildChat()

	case input == "/moa on":
		if !m.cfg.MoA.Enabled {
			m.messages = append(m.messages, llm.Message{Role: "system", Content: "MoA is not enabled in config. Set moa.enabled: true in ~/.tlaude-code/config.yaml"})
			return m.rebuildChat()
		}
		if m.moaOrchestrator == nil {
			m.messages = append(m.messages, llm.Message{Role: "system", Content: "MoA orchestrator not available (no providers registered)."})
			return m.rebuildChat()
		}
		m.moaEnabled = true
		m.statusMsg = m.buildStatusMsg()
		m.messages = append(m.messages, llm.Message{Role: "system", Content: "MoA enabled — mode: " + m.cfg.MoA.Mode})
		return m.rebuildChat()

	case input == "/moa off":
		m.moaEnabled = false
		m.statusMsg = m.buildStatusMsg()
		m.messages = append(m.messages, llm.Message{Role: "system", Content: "MoA disabled"})
		return m.rebuildChat()

	case input == "/mcp":
		info := m.buildMCPStatus()
		m.messages = append(m.messages, llm.Message{Role: "system", Content: info})
		return m.rebuildChat()

	case input == "/cost":
		info := m.buildCostReport()
		m.messages = append(m.messages, llm.Message{Role: "system", Content: info})
		return m.rebuildChat()

	case input == "/route" || input == "/route status":
		info := m.buildRouteStatus()
		m.messages = append(m.messages, llm.Message{Role: "system", Content: info})
		return m.rebuildChat()

	case input == "/route smart":
		m.smartRouting = true
		m.cfg.SmartRouting = true
		m.statusMsg = m.buildStatusMsg()
		m.messages = append(m.messages, llm.Message{Role: "system", Content: "Smart routing enabled. Tasks will be routed to the most cost-effective provider."})
		return m.rebuildChat()

	case input == "/route fixed":
		m.smartRouting = false
		m.cfg.SmartRouting = false
		m.routedResult = nil
		m.statusMsg = m.buildStatusMsg()
		m.messages = append(m.messages, llm.Message{Role: "system", Content: "Fixed routing restored. Using configured provider."})
		return m.rebuildChat()

	case strings.HasPrefix(input, "/search "):
		query := strings.TrimPrefix(input, "/search ")
		query = strings.TrimSpace(query)
		if query == "" {
			m.messages = append(m.messages, llm.Message{Role: "system", Content: "Usage: /search <query>"})
			return m.rebuildChat()
		}
		results, err := m.memorySearch.Search(query, 5)
		if err != nil {
			m.messages = append(m.messages, llm.Message{Role: "system", Content: fmt.Sprintf("Search error: %v", err)})
		} else {
			m.messages = append(m.messages, llm.Message{Role: "system", Content: memory.FormatResults(results)})
		}
		return m.rebuildChat()

	case input == "/sandbox" || strings.HasPrefix(input, "/sandbox "):
		return m.handleSandboxCommand(input)

	case input == "/agent" || input == "/agent status":
		return m.handleAgentCommand(input)
	case strings.HasPrefix(input, "/agent "):
		return m.handleAgentCommand(input)

	case strings.HasPrefix(input, "/tasks"):
		return m.handleTasksCommand(input)

		case input == "/plan" || input == "/plan status":
			return m.handlePlanCommand(input)
		case strings.HasPrefix(input, "/plan "):
			return m.handlePlanCommand(input)

		case input == "/compact" || strings.HasPrefix(input, "/compact "):
			return m.handleCompactCommand(input)

		case input == "/plugins":
			info := m.buildPluginStatus()
			m.messages = append(m.messages, llm.Message{Role: "system", Content: info})
			return m.rebuildChat()

		default:
		m.messages = append(m.messages, llm.Message{Role: "system", Content: "Unknown command: " + input})
		return m.rebuildChat()
	}
}

// handleToolCalls separates auto-approved tool calls from those needing user
// confirmation, executes the former immediately, and queues the latter for approval.
func (m *Model) handleToolCalls(tcs []llm.ToolCall) tea.Cmd {
	// Add the assistant message (with tool calls) to the conversation.
	assistantMsg := llm.Message{Role: "assistant", ToolCalls: tcs}
	if tcs[0].Result == "" {
		assistantMsg.Content = " " // non-empty to satisfy some APIs
	}
	m.messages = append(m.messages, assistantMsg)

	// Separate auto-approved from manual-approval calls.
	var needApproval []llm.ToolCall
	for _, tc := range tcs {
		if m.cfg.ShouldAutoApprove(tc.Name, tc.Args) {
			m.pendingToolCalls = append(m.pendingToolCalls, tc)
		} else {
			needApproval = append(needApproval, tc)
		}
	}

	var cmd tea.Cmd
	// Execute auto-approved calls first.
	if len(m.pendingToolCalls) > 0 {
		tc := m.pendingToolCalls[0]
		m.pendingToolCalls = m.pendingToolCalls[1:]
		cmd = m.executeToolCall(tc)
	}

	// Queue remaining calls that need approval.
	if len(needApproval) > 0 {
		m.pendingToolCalls = append(m.pendingToolCalls, needApproval...)
		tc := needApproval[0]
		req := &ApprovalRequest{
			Type:    tc.Name,
			Summary: buildApprovalSummary(tc.Name, tc.Args),
			Detail:  buildApprovalDetail(tc.Name, tc.Args),
			TC:      tc,
		}
		if path, ok := tc.Args["path"].(string); ok {
			req.Path = path
		}
		m.pendingApproval = req
		m.waitingApproval = true
	}

	return cmd
}

// buildMoAStatus formats the MoA status for display.
func (m *Model) buildMoAStatus() string {
	var sb strings.Builder
	sb.WriteString("MoA Configuration:\n")
	sb.WriteString(fmt.Sprintf("  Enabled:      %v\n", m.cfg.MoA.Enabled))
	sb.WriteString(fmt.Sprintf("  Active:       %v\n", m.moaEnabled))
	sb.WriteString(fmt.Sprintf("  Mode:         %s\n", m.cfg.MoA.Mode))
	sb.WriteString(fmt.Sprintf("  Max Parallel: %d\n", m.cfg.MoA.MaxParallel))
	sb.WriteString(fmt.Sprintf("  Timeout:      %ds\n", m.cfg.MoA.TimeoutSec))
	sb.WriteString(fmt.Sprintf("  Synthesizer:  %s\n", m.cfg.MoA.Synthesizer))

	providers := m.cfg.MoA.ProviderNames
	if len(providers) == 0 {
		reg := llm.GlobalRegistry()
		providers = reg.List()
	}
	sb.WriteString(fmt.Sprintf("  Providers:    %s\n", strings.Join(providers, ", ")))

	if m.moaResult != nil {
		sb.WriteString("\nLast MoA Result:\n")
		sb.WriteString(moa.BuildMoADetail(m.moaResult))
	}

	return sb.String()
}

// buildMCPStatus formats the MCP server status for display.
func (m *Model) buildMCPStatus() string {
	if m.mcpManager == nil {
		return "MCP not available."
	}
	status := m.mcpManager.Status()
	var sb strings.Builder
	sb.WriteString("MCP Servers:\n")
	for _, s := range status {
		sb.WriteString(fmt.Sprintf("  %s: %s\n", s.Name, s.Status))
	}
	return sb.String()
}

// handleSandboxCommand processes /sandbox commands.
func (m *Model) handleSandboxCommand(input string) tea.Cmd {
	parts := strings.Fields(input)
	if len(parts) == 1 || parts[1] == "status" {
		info := m.buildSandboxStatus()
		m.messages = append(m.messages, llm.Message{Role: "system", Content: info})
		return m.rebuildChat()
	}

	switch strings.ToLower(parts[1]) {
	case "restricted":
		m.cfg.Sandbox.Mode = sandbox.ModeRestricted
	case "off":
		m.cfg.Sandbox.Mode = sandbox.ModeOff
	case "wasm":
		m.messages = append(m.messages, llm.Message{Role: "system", Content: "WASM sandbox is not available (wazero dependency not installed)"})
		return m.rebuildChat()
	default:
		m.messages = append(m.messages, llm.Message{Role: "system", Content: "Unknown sandbox mode: " + parts[1] + ". Use: restricted, off"})
		return m.rebuildChat()
	}

	sb, err := sandbox.New(m.cfg.Sandbox)
	if err != nil {
		m.messages = append(m.messages, llm.Message{Role: "system", Content: fmt.Sprintf("Failed to switch sandbox: %v", err)})
		return m.rebuildChat()
	}
	m.sandboxer = sb
	m.statusMsg = m.buildStatusMsg()
	m.messages = append(m.messages, llm.Message{Role: "system", Content: fmt.Sprintf("Sandbox mode set to %s", sb.Name())})
	return m.rebuildChat()
}

// buildSandboxStatus formats the sandbox status for display.
func (m *Model) buildSandboxStatus() string {
	var sb strings.Builder
	sb.WriteString("Sandbox Configuration:\n")
	sb.WriteString(fmt.Sprintf("  Mode:        %s\n", m.cfg.Sandbox.Mode))
	sb.WriteString(fmt.Sprintf("  Active:      %s\n", m.sandboxer.Name()))
	sb.WriteString(fmt.Sprintf("  Timeout:     %ds\n", m.cfg.Sandbox.TimeoutSec))
	sb.WriteString(fmt.Sprintf("  Max Memory:  %d MB\n", m.cfg.Sandbox.MaxMemoryMB))
	sb.WriteString(fmt.Sprintf("  Network:     %v\n", m.cfg.Sandbox.AllowNetwork))
	sb.WriteString(fmt.Sprintf("  Write:       %v\n", m.cfg.Sandbox.AllowWrite))
	return sb.String()
}

// handleAgentCommand processes /agent commands.
func (m *Model) handleAgentCommand(input string) tea.Cmd {
	parts := strings.Fields(input)
	if len(parts) == 1 {
		// /agent or /agent status — show current agent info.
		info := m.buildAgentStatus()
		m.messages = append(m.messages, llm.Message{Role: "system", Content: info})
		return m.rebuildChat()
	}

	sub := strings.ToLower(parts[1])
	switch sub {
	case "list":
		info := m.buildAgentList()
		m.messages = append(m.messages, llm.Message{Role: "system", Content: info})
		return m.rebuildChat()
	case "moa":
		m.agentType = "moa"
		m.moaEnabled = true
		m.statusMsg = m.buildStatusMsg()
		m.messages = append(m.messages, llm.Message{Role: "system", Content: "Switched to MoA agent mode"})
		return m.rebuildChat()
	default:
		// Try switching to a specific agent type.
		if def, ok := m.agentStore.Get(sub); ok {
			m.agentType = def.AgentType
			m.statusMsg = m.buildStatusMsg()
			m.messages = append(m.messages, llm.Message{Role: "system", Content: fmt.Sprintf("Agent switched to: %s (%s)", def.Name, def.Description)})
		} else {
			m.messages = append(m.messages, llm.Message{Role: "system", Content: fmt.Sprintf("Unknown agent type: %s. Use /agent list to see available agents.", sub)})
		}
		return m.rebuildChat()
	}
}

// handleTasksCommand processes /tasks commands.
func (m *Model) handleTasksCommand(input string) tea.Cmd {
	parts := strings.Fields(input)
	if len(parts) == 1 {
		// /tasks — show all background tasks.
		info := m.buildTasksStatus()
		m.messages = append(m.messages, llm.Message{Role: "system", Content: info})
		return m.rebuildChat()
	}

	if len(parts) >= 3 && parts[1] == "stop" {
		taskID := parts[2]
		if m.taskManager.Stop(taskID) {
			m.statusMsg = m.buildStatusMsg()
			m.messages = append(m.messages, llm.Message{Role: "system", Content: fmt.Sprintf("Task %s stopped", taskID)})
		} else {
			m.messages = append(m.messages, llm.Message{Role: "system", Content: fmt.Sprintf("Task %s not found or already completed", taskID)})
		}
		return m.rebuildChat()
	}

	m.messages = append(m.messages, llm.Message{Role: "system", Content: "Usage: /tasks [stop <id>]"})
	return m.rebuildChat()
}

// handlePlanCommand processes /plan commands.
func (m *Model) handlePlanCommand(input string) tea.Cmd {
	parts := strings.Fields(input)

	// /plan or /plan status — show current plan.
	if len(parts) == 1 || (len(parts) == 2 && parts[1] == "status") {
		info := m.buildPlanStatus()
		m.messages = append(m.messages, llm.Message{Role: "system", Content: info})
		return m.rebuildChat()
	}

	sub := strings.ToLower(parts[1])
	switch sub {
	case "create":
		if len(parts) < 3 {
			m.messages = append(m.messages, llm.Message{Role: "system", Content: "Usage: /plan create <description>"})
			return m.rebuildChat()
		}
		desc := strings.Join(parts[2:], " ")
		if m.planManager == nil {
			m.messages = append(m.messages, llm.Message{Role: "system", Content: "Plan manager not available."})
			return m.rebuildChat()
		}
		p := m.planManager.BuildFromDescription("User Plan", desc)
		m.planManager.SetActive(p)
		info := "Plan created: " + p.ID + "\n\n" + plan.FormatPlan(p)
		m.messages = append(m.messages, llm.Message{Role: "system", Content: info})
		m.statusMsg = m.buildStatusMsg()
		return m.rebuildChat()

	case "approve":
		if m.planManager == nil || m.planManager.Active() == nil {
			m.messages = append(m.messages, llm.Message{Role: "system", Content: "No active plan to approve."})
			return m.rebuildChat()
		}
		active := m.planManager.Active()
		if err := m.planManager.Approve(active.ID); err != nil {
			m.messages = append(m.messages, llm.Message{Role: "system", Content: fmt.Sprintf("Approve failed: %v", err)})
		} else {
			m.messages = append(m.messages, llm.Message{Role: "system", Content: "Plan approved. Executing..."})
			m.statusMsg = m.buildStatusMsg()
		}
		return m.rebuildChat()

	case "reject":
		if m.planManager == nil || m.planManager.Active() == nil {
			m.messages = append(m.messages, llm.Message{Role: "system", Content: "No active plan to reject."})
			return m.rebuildChat()
		}
		reason := "rejected by user"
		if len(parts) > 2 {
			reason = strings.Join(parts[2:], " ")
		}
		active := m.planManager.Active()
		if err := m.planManager.Reject(active.ID, reason); err != nil {
			m.messages = append(m.messages, llm.Message{Role: "system", Content: fmt.Sprintf("Reject failed: %v", err)})
		} else {
			m.messages = append(m.messages, llm.Message{Role: "system", Content: fmt.Sprintf("Plan rejected: %s", reason)})
			m.statusMsg = m.buildStatusMsg()
		}
		return m.rebuildChat()

	case "execute":
		if m.planManager == nil || m.planManager.Active() == nil {
			m.messages = append(m.messages, llm.Message{Role: "system", Content: "No active plan to execute."})
			return m.rebuildChat()
		}
		_ = m.planManager.Approve(m.planManager.Active().ID)
		m.messages = append(m.messages, llm.Message{Role: "system", Content: "Plan approved for execution. Describe the next action to begin implementation."})
		m.statusMsg = m.buildStatusMsg()
		return m.rebuildChat()

	default:
		m.messages = append(m.messages, llm.Message{Role: "system", Content: "Usage: /plan [status|create <desc>|approve|reject [reason]|execute]"})
		return m.rebuildChat()
	}
}

	// handleCompactCommand processes /compact commands.
	func (m *Model) handleCompactCommand(input string) tea.Cmd {
		parts := strings.Fields(input)

		if len(parts) == 1 || (len(parts) == 2 && parts[1] == "status") {
			info := m.buildCompactStatus()
			m.messages = append(m.messages, llm.Message{Role: "system", Content: info})
			return m.rebuildChat()
		}

		sub := strings.ToLower(parts[1])
		switch sub {
		case "auto":
			if m.compactManager == nil {
				m.messages = append(m.messages, llm.Message{Role: "system", Content: "Compact manager not available."})
				return m.rebuildChat()
			}
			cfg := m.compactManager.Config()
			cfg.AutoCompact = !cfg.AutoCompact
			m.compactManager.SetConfig(cfg)
			m.cfg.Compact.AutoCompact = cfg.AutoCompact
			state := "disabled"
			if cfg.AutoCompact {
				state = "enabled"
			}
			m.messages = append(m.messages, llm.Message{Role: "system", Content: fmt.Sprintf("Auto-compact %s.", state)})
			m.statusMsg = m.buildStatusMsg()
			return m.rebuildChat()
		default:
			m.messages = append(m.messages, llm.Message{Role: "system", Content: "Usage: /compact [status|auto]"})
			return m.rebuildChat()
		}
	}

	// buildCompactStatus formats compact state for display.
	func (m *Model) buildCompactStatus() string {
		if m.compactManager == nil {
			return "Compact system not available."
		}
		cfg := m.compactManager.Config()
		state := compact.CalculateTokenState(m.messages, m.cfg.Model, cfg.MaxSummaryTokens)
		autoState := m.compactManager.AutoState()

		var sb strings.Builder
		sb.WriteString("Compact System Status:\n\n")
		sb.WriteString(fmt.Sprintf("  Enabled:       %v\n", cfg.Enabled))
		sb.WriteString(fmt.Sprintf("  Auto-compact:  %v\n", cfg.AutoCompact))
		sb.WriteString(fmt.Sprintf("  Token Budget:  %d\n", cfg.TokenBudget))
		sb.WriteString(fmt.Sprintf("  Est. Tokens:   %d\n", compact.EstimateTokens(m.messages)))
		sb.WriteString(fmt.Sprintf("  Tokens Left:   %d%%\n", state.PercentLeft))
		if state.IsAtBlockingLimit {
			sb.WriteString("  Status:        AT BLOCKING LIMIT\n")
		} else if state.IsAboveError {
			sb.WriteString("  Status:        Above error threshold\n")
		} else if state.IsAboveWarning {
			sb.WriteString("  Status:        Above warning threshold\n")
		} else if state.IsAboveAutoCompact {
			sb.WriteString("  Status:        Above auto-compact threshold\n")
		} else {
			sb.WriteString("  Status:        Normal\n")
		}
		if autoState.Compacted {
			sb.WriteString(fmt.Sprintf("  Last Compact:  turn %d\n", autoState.TurnCounter))
		}
		if autoState.ConsecutiveFailures > 0 {
			sb.WriteString(fmt.Sprintf("  Failures:      %d\n", autoState.ConsecutiveFailures))
		}
		return sb.String()
	}

// buildPluginStatus formats the plugin system status for display.
func (m *Model) buildPluginStatus() string {
	if m.pluginManager == nil {
		return "Plugin system not available."
	}

	plugins := m.pluginManager.List()
	if len(plugins) == 0 {
		return "No plugins loaded."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Plugins (%d loaded):\n\n", len(plugins)))

	for _, p := range plugins {
		status := "●"
		if !p.Enabled() {
			status = "○"
		}
		provides := make([]string, len(p.Provides()))
		for i, pr := range p.Provides() {
			provides[i] = string(pr)
		}
		sb.WriteString(fmt.Sprintf("  %s %s v%s [%s]\n", status, p.Name(), p.Version(), p.Type()))
		if len(provides) > 0 {
			sb.WriteString(fmt.Sprintf("    provides: %s\n", strings.Join(provides, ", ")))
		}
		sb.WriteString(fmt.Sprintf("    %s\n\n", p.Description()))
	}

	poolSize, activeVMs := m.pluginManager.LuaEngineStatus()
	sb.WriteString(fmt.Sprintf("Lua Engine: pool_size=%d, active_vms=%d\n", poolSize, activeVMs))

	return sb.String()
}

// buildPlanStatus formats the current plan status for display.
func (m *Model) buildPlanStatus() string {
	if m.planManager == nil {
		return "Plan mode not available."
	}
	active := m.planManager.Active()
	if active == nil {
		return "No active plan. Use /plan create <description> to create one, or let the assistant use EnterPlanMode."
	}
	info := plan.FormatPlan(active)
	prog, err := m.planManager.GetProgress(active.ID)
	if err == nil && prog.TotalSteps > 0 {
		info += "\n\nProgress: " + plan.FormatProgress(prog)
	}
	return info
}

// buildAgentStatus formats the current agent configuration for display.
func (m *Model) buildAgentStatus() string {
	var sb strings.Builder
	sb.WriteString("Agent Configuration:\n")
	sb.WriteString(fmt.Sprintf("  Active Agent: %s\n", m.agentType))
	if def, ok := m.agentStore.Get(m.agentType); ok {
		sb.WriteString(fmt.Sprintf("  Name:         %s\n", def.Name))
		sb.WriteString(fmt.Sprintf("  Description:  %s\n", def.Description))
		if def.ModelRef.Model != "" || def.ModelRef.Provider != "" {
			sb.WriteString(fmt.Sprintf("  Model:        %s / %s\n", def.ModelRef.Provider, def.ModelRef.Model))
			sb.WriteString(fmt.Sprintf("  Reason:       %s\n", def.ModelRef.Reason))
		}
		sb.WriteString(fmt.Sprintf("  Max Turns:    %d\n", def.MaxTurns))
		sb.WriteString(fmt.Sprintf("  Background:   %v\n", def.Background))
		if len(def.Tools) > 0 {
			sb.WriteString(fmt.Sprintf("  Tools:        %s\n", strings.Join(def.Tools, ", ")))
		}
	}
	sb.WriteString("\nUse /agent list to see all available agents.")
	sb.WriteString("\nUse /agent <type> to switch agent type.")
	return sb.String()
}

// buildAgentList formats all available agents for display.
func (m *Model) buildAgentList() string {
	defs := m.agentStore.List()
	if len(defs) == 0 {
		return "No agents registered."
	}
	var sb strings.Builder
	sb.WriteString("Available Agents:\n\n")
	for _, def := range defs {
		marker := " "
		if def.AgentType == m.agentType {
			marker = "*"
		}
		sb.WriteString(fmt.Sprintf("  %s %-12s %s\n", marker, def.AgentType, def.Name))
		sb.WriteString(fmt.Sprintf("    %s\n", def.Description))
		if def.ModelRef.Model != "" || def.ModelRef.Provider != "" {
			sb.WriteString(fmt.Sprintf("    Model: %s/%s (%s)\n", def.ModelRef.Provider, def.ModelRef.Model, def.ModelRef.Reason))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// buildTasksStatus formats background task status for display.
func (m *Model) buildTasksStatus() string {
	tasks := m.taskManager.List()
	if len(tasks) == 0 {
		return "No background tasks."
	}
	var sb strings.Builder
	sb.WriteString("Background Tasks:\n\n")
	for _, task := range tasks {
		statusIcon := " "
		switch task.Status {
		case "pending":
			statusIcon = "○"
		case "running":
			statusIcon = "●"
		case "completed":
			statusIcon = "✓"
		case "failed":
			statusIcon = "✗"
		case "stopped":
			statusIcon = "■"
		}
		sb.WriteString(fmt.Sprintf("  %s %s [%s] %s\n", statusIcon, task.ID, task.Status, task.Description))
	}
	sb.WriteString("\nUse /tasks stop <id> to stop a running task.")
	return sb.String()
}

// recordCost estimates and records the cost of an assistant response.
func (m *Model) recordCost(content string) {
	if m.costTracker == nil {
		return
	}
	// Estimate tokens from content length (~4 chars/token).
	inputTokens := 0
	for _, msg := range m.messages {
		inputTokens += len(msg.Content) / 4
	}
	outputTokens := len(content) / 4
	if inputTokens < 1 {
		inputTokens = 1
	}
	if outputTokens < 1 {
		outputTokens = 1
	}
	m.costTracker.Record(m.provider.Name(), m.cfg.Model, inputTokens, outputTokens)
}

// buildCostReport formats cumulative cost statistics for all providers.
func (m *Model) buildCostReport() string {
	if m.costTracker == nil {
		return "Cost tracking is not available."
	}

	stats := m.costTracker.GetAllStats()
	if len(stats) == 0 {
		return "No usage recorded yet. Costs are estimated based on provider pricing.\n\nTip: Prices are approximate and may not reflect current market rates."
	}

	var sb strings.Builder
	sb.WriteString("Cost Report (estimated):\n")
	sb.WriteString(fmt.Sprintf("  Total cost: $%.4f\n\n", m.costTracker.TotalCost()))

	for key, s := range stats {
		sb.WriteString(fmt.Sprintf("  %s:\n", key))
		sb.WriteString(fmt.Sprintf("    Calls:        %d\n", s.TotalCalls))
		sb.WriteString(fmt.Sprintf("    Input:        %d tokens\n", s.TotalInputTokens))
		sb.WriteString(fmt.Sprintf("    Output:       %d tokens\n", s.TotalOutputTokens))
		sb.WriteString(fmt.Sprintf("    Cost:         $%.4f\n", s.TotalCostUSD))
	}

	sb.WriteString("\nPrices are approximate and may not reflect current market rates.")
	return sb.String()
}

// buildRouteStatus formats the current routing configuration for display.
func (m *Model) buildRouteStatus() string {
	var sb strings.Builder
	sb.WriteString("Routing Configuration:\n")

	mode := "fixed (user preference)"
	if m.smartRouting {
		mode = "smart (cost-aware)"
	}
	sb.WriteString(fmt.Sprintf("  Mode:     %s\n", mode))
	sb.WriteString(fmt.Sprintf("  Provider: %s\n", m.provider.Name()))
	sb.WriteString(fmt.Sprintf("  Model:    %s\n", m.cfg.Model))

	if m.routedResult != nil {
		sb.WriteString(fmt.Sprintf("\n  Last Route: %s | %s\n", m.routedResult.Provider, m.routedResult.Model))
		sb.WriteString(fmt.Sprintf("  Reason:      %s\n", m.routedResult.Reason))
	}

	sb.WriteString("\nUse /route smart to enable cost-aware routing.")
	sb.WriteString("\nUse /route fixed to disable it.")
	return sb.String()
}

// buildStatusMsg constructs the status bar text.
func (m *Model) buildStatusMsg() string {
	routeLabel := "Fixed Route"
	if m.smartRouting {
		routeLabel = "Smart Route"
	}

	totalCost := 0.0
	if m.costTracker != nil {
		totalCost = m.costTracker.TotalCost()
	}

	// Agent type with color mapping.
	agentLabel := m.agentType
	if agentLabel == "" {
		agentLabel = "general"
	}

	msg := fmt.Sprintf("%s | %s | %s | %s | $%.2f",
		m.provider.Name(), m.cfg.Model, routeLabel, agentLabel, totalCost)

	if m.sandboxer != nil {
		msg += " | Sandbox:" + m.sandboxer.Name()
	}
	if m.moaEnabled {
		msg += " | MoA:" + m.cfg.MoA.Mode
	}

	// Background task count.
	if m.taskManager != nil {
		tasks := m.taskManager.List()
		activeCount := 0
		for _, t := range tasks {
			if t.Status == "pending" || t.Status == "running" {
				activeCount++
			}
		}
		if activeCount > 0 {
			msg += fmt.Sprintf(" | Tasks:%d", activeCount)
		}
	}

	if m.planManager != nil && m.planManager.IsInPlanMode() {
		active := m.planManager.Active()
		if active != nil {
			prog, err := m.planManager.GetProgress(active.ID)
			if err == nil && prog.TotalSteps > 0 {
				msg += fmt.Sprintf(" | PLAN[%d/%d]", prog.CompletedSteps, prog.TotalSteps)
			} else {
				msg += " | PLAN"
			}
		}
	}

	return msg
}

func (m *Model) rebuildChat() tea.Cmd {
	var sb strings.Builder

	for _, msg := range m.messages {
		switch msg.Role {
		case "user":
			sb.WriteString(userMsgStyle.Render("You: "))
			sb.WriteString(msg.Content)
		case "assistant":
			sb.WriteString(assistantMsgStyle.Render("Assistant: "))
			sb.WriteString(renderContent(msg.Content))
		case "system":
			sb.WriteString(systemMsgStyle.Render("[") + msg.Content + systemMsgStyle.Render("]"))
		}
		sb.WriteString("\n\n")
	}

	if m.streaming && m.streamBuf.Len() > 0 {
		sb.WriteString(assistantMsgStyle.Render("Assistant: "))
		sb.WriteString(renderContent(m.streamBuf.String()))
		sb.WriteString("█\n")
	}

	if m.streamErr != nil {
		sb.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.streamErr)))
		sb.WriteString("\n")
	}

	// Show MoA detail after the last assistant message when available.
	if m.moaResult != nil && !m.moaExecuting {
		sb.WriteString(moaDetailStyle.Render(moa.BuildMoADetail(m.moaResult)))
		sb.WriteString("\n")
	}

	m.chatView.SetContent(sb.String())
	m.chatView.GotoBottom()
	return viewport.Sync(m.chatView)
}

func renderContent(content string) string {
	var sb strings.Builder
	inCode := false

	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "```") {
			if !inCode {
				lang := strings.TrimPrefix(line, "```")
				lang = strings.TrimSpace(lang)
				if lang != "" {
					sb.WriteString(codeHeaderStyle.Render("[" + lang + "]"))
					sb.WriteByte('\n')
				}
			}
			inCode = !inCode
			continue
		}
		if inCode {
			sb.WriteString(codeBlockStyle.Render(line))
		} else {
			sb.WriteString(line)
		}
		sb.WriteByte('\n')
	}

	return sb.String()
}

func (m Model) renderHelp() string {
	helpText := systemMsgStyle.Render(`
Commands:
  /help        Show this help
  /config      Show current configuration
  /clear       Clear chat history
  /save        Save session to disk
  /cost        Show cost report (estimated)
  /route       Show routing configuration
  /route smart Enable smart (cost-aware) routing
  /route fixed Disable smart routing
  /search <q>  Search historical sessions
  /moa         Show MoA status and last result
  /moa on      Enable MoA (requires config)
  /moa off     Disable MoA
  /moa strategy <mode>  Set MoA mode
  /mcp         Show MCP server status
  /sandbox     Show sandbox mode
  /sandbox restricted|off  Switch sandbox mode
  /agent       Show agent configuration
  /agent list  List all available agents
  /agent <type> Switch agent type
  /tasks       Show background tasks
  /plan        Show current plan status
  /plan create <desc>  Create a new plan
  /plan approve        Approve current plan
  /plan reject [reason] Reject current plan
  /plan execute        Execute approved plan
  /plugins     List loaded plugins
  /quit        Exit

Keyboard:
  Enter      Send message
  Ctrl+C     Quit
  Ctrl+H     Show help
  Esc        Quit

Press any key to return.
`)
	return chatStyle.Render(helpText)
}

// approveToolCall executes the approved tool call and feeds the result back.
func (m *Model) approveToolCall(req *ApprovalRequest, remember bool) tea.Cmd {
	m.waitingApproval = false
	m.pendingApproval = nil

	if remember && m.cfg != nil {
		pattern := m.buildAlwaysAllowPattern(req)
		m.cfg.AlwaysAllowPatterns = append(m.cfg.AlwaysAllowPatterns, pattern)
	}

	if len(m.pendingToolCalls) > 0 {
		tc := m.pendingToolCalls[0]
		m.pendingToolCalls = m.pendingToolCalls[1:]
		return m.executeToolCall(tc)
	}

	return m.rebuildChat()
}

func (m *Model) buildAlwaysAllowPattern(req *ApprovalRequest) string {
	switch req.Type {
	case "bash":
		return "bash:" + req.Summary
	case "write_file":
		return "write:" + req.Path
	case "delete_file":
		return "delete:" + req.Path
	case "read_file":
		return "read:" + req.Path
	default:
		return req.Type
	}
}

// rejectToolCall rejects the tool call and informs the model.
func (m *Model) rejectToolCall(req *ApprovalRequest) tea.Cmd {
	m.waitingApproval = false
	m.pendingApproval = nil

	result := fmt.Sprintf("User rejected tool call: %s (%s)", req.Type, req.Summary)
	m.messages = append(m.messages, llm.Message{Role: "tool", Content: result})

	if len(m.pendingToolCalls) > 0 {
		m.pendingToolCalls = m.pendingToolCalls[1:]
	}

	return m.rebuildChat()
}

// executeToolCall looks up the tool in the registry and executes it.
func (m *Model) executeToolCall(tc llm.ToolCall) tea.Cmd {
	tl, ok := m.toolReg.Get(tc.Name)
	if !ok {
		m.messages = append(m.messages, llm.Message{Role: "tool", Content: fmt.Sprintf("Unknown tool: %s", tc.Name)})
		if len(m.pendingToolCalls) > 0 {
			next := m.pendingToolCalls[0]
			m.pendingToolCalls = m.pendingToolCalls[1:]
			return m.executeToolCall(next)
		}
		return m.sendToolResultsToModel()
	}

	input, _ := json.Marshal(tc.Args)
	result, err := tl.Execute(m.ctx, input, &tool.ToolContext{
		CWD: m.cwd,
	})
	if err != nil {
		m.messages = append(m.messages, llm.Message{Role: "tool", Content: fmt.Sprintf("Error: %v", err)})
	} else if result.IsError {
		m.messages = append(m.messages, llm.Message{Role: "tool", Content: fmt.Sprintf("Error: %s", result.Content)})
	} else {
		m.messages = append(m.messages, llm.Message{Role: "tool", Content: result.Content})
	}

	if len(m.pendingToolCalls) > 0 {
		next := m.pendingToolCalls[0]
		m.pendingToolCalls = m.pendingToolCalls[1:]
		return m.executeToolCall(next)
	}

	return m.sendToolResultsToModel()
}

// sendToolResultsToModel sends accumulated tool results back to the model.
func (m *Model) sendToolResultsToModel() tea.Cmd {
	// Copy cfg values to avoid data race from the tea.Cmd goroutine.
	cfgModel := m.cfg.Model
	cfgTemperature := m.cfg.Temperature
	cfgMaxTokens := m.cfg.MaxTokens

	req := llm.ChatRequest{
		Model:       cfgModel,
		Messages:    m.messages,
		Stream:      true,
		Temperature: cfgTemperature,
		MaxTokens:   cfgMaxTokens,
		Tools:       tools.GetToolDefinitionsFromRegistry(m.toolReg),
	}

	return func() tea.Msg {
		ch, err := m.provider.ChatStream(m.ctx, req)
		if err != nil {
			return streamChunkMsg{Error: err}
		}
		m.streamCh = ch
		m.streaming = true
		m.streamBuf.Reset()
		m.streamErr = nil
		return readChunk(ch)
	}
}

// SetSession sets the active session for the model.
func (m *Model) SetSession(sess *session.Session) {
	m.session = sess
	m.messages = sess.Messages
	m.chatView.GotoBottom()
	m.statusMsg = m.buildStatusMsg() + fmt.Sprintf(" | session: %s", sess.ID[:8])
}

// SaveSession saves the current session to disk.
func (m *Model) SaveSession() {
	if m.session == nil || len(m.messages) == 0 {
		return
	}
	m.session.Messages = m.messages
	if err := m.sessStore.Save(m.session); err != nil {
		logging.Warn("failed to save session on exit", "error", err)
	}
}

// Quitting returns true if the user requested to quit.
func (m Model) Quitting() bool {
	return m.quitting
}
