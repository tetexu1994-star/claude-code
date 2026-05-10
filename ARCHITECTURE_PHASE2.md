# Phase 2: 多Agent + MoA 融合系统架构设计

> 基于 CC 源码 AgentTool/runAgent/Coordinator/Swarm 的 Go 实现
> 参考: agentTool.tsx, runAgent.ts, forkSubagent.ts, builtInAgents.ts, coordinatorMode.ts

## 1. 架构概览

```
                    ┌──────────────────────────────────────┐
                    │         Orchestrator (主LLM循环)       │
                    │   AgentTool 派发 → StreamingExecutor   │
                    └──────────────┬───────────────────────┘
                                   │ AgentTool 调用
                                   ▼
                    ┌──────────────────────────────────────┐
                    │           Agent Runtime                │
                    │  (agent/runtime.go - 对标 runAgent.ts) │
                    └──────┬──────────┬──────────┬─────────┘
                           │          │          │
              ┌────────────┼──────────┼────────────┐
              ▼            ▼          ▼            ▼
        ┌──────────┐ ┌──────────┐ ┌──────┐ ┌──────────┐
        │Sync Agent│ │Async Agent│ │Fork  │ │Worktree  │
        │ (同步)   │ │ (后台)   │ │(继承)│ │(git隔离)  │
        └──────────┘ └──────────┘ └──────┘ └──────────┘
                                         │
                              ┌──────────┴──────────┐
                              ▼                     ▼
                        ┌──────────┐         ┌──────────┐
                        │ Agent A  │         │ Agent B  │  ← MoA 多模型
                        │ Claude   │         │ DeepSeek │
                        └──────────┘         └──────────┘
                              │                     │
                              └──────────┬──────────┘
                                         ▼
                                   ┌──────────┐
                                   │Aggregator│  ← 结果聚合/投票
                                   └──────────┘
```

## 2. Agent 定义系统

### 2.1 核心类型

```go
// internal/agent/types.go

// AgentDefinition 定义了一个Agent的类型（对标 CC 的 AgentDefinition）
type AgentDefinition struct {
    AgentType       string            `yaml:"agent_type"`       // 唯一标识符
    Name            string            `yaml:"name"`             // 显示名称
    Description     string            `yaml:"description"`      // 模型看的提示
    WhenToUse       string            `yaml:"when_to_use"`      // 何时使用
    Tools           []string          `yaml:"tools"`            // ['*'] = 全部
    DisallowedTools []string          `yaml:"disallowed_tools"` // 排除的工具
    MaxTurns        int               `yaml:"max_turns"`       // 默认 200
    Model           string            `yaml:"model"`            // 覆盖模型名
    Provider        string            `yaml:"provider"`         // 覆盖Provider
    PermissionMode  string            `yaml:"permission_mode"`  // 权限模式覆盖
    Source          string            `yaml:"source"`           // built-in / user / plugin
    Color           string            `yaml:"color"`            // UI颜色
    Background      bool              `yaml:"background"`       // 默认后台
    Isolation       string            `yaml:"isolation"`        // worktree / remote
    Memory          string            `yaml:"memory"`           // 记忆作用域
    SystemPrompt    string            `yaml:"system_prompt"`    // 系统提示
    Env             map[string]string `yaml:"env"`              // 额外环境变量
}

// AgentDefStore 存储所有Agent定义
type AgentDefStore struct {
    builtIns map[string]*AgentDefinition
    userDefs map[string]*AgentDefinition  // 从 ~/.tlaude-code/agents/ 加载
    plugins  map[string]*AgentDefinition
    mu       sync.RWMutex
}

func NewAgentDefStore() *AgentDefStore
func (s *AgentDefStore) Get(agentType string) (*AgentDefinition, bool)
func (s *AgentDefStore) List() []*AgentDefinition
func (s *AgentDefStore) LoadUserAgents(dir string) error
```

### 2.2 内置Agent定义

```go
// internal/agent/builtin.go
// 对标 CC 的 builtInAgents.ts

var BuiltInAgents = []AgentDefinition{
    {
        AgentType:   "general",
        Name:        "General Purpose",
        Description: "全能Agent，默认选择",
        WhenToUse:   "适合大多数编程任务。如果不确定用哪个，选这个。",
        Tools:       []string{"*"},
        MaxTurns:    200,
        Source:      "built-in",
        Color:       "#6C5CE7",
    },
    {
        AgentType:   "explore",
        Name:        "Explore",
        Description: "只读探索代码库",
        WhenToUse:   "当需要理解不熟悉的代码库、查找文件或阅读文档时",
        Tools:       []string{"read_file", "Glob", "Grep", "bash"},
        MaxTurns:    50,
        Source:      "built-in",
        Color:       "#00B894",
        Background:  true,
    },
    {
        AgentType:   "code",
        Name:        "Code Writer",
        Description: "专门写代码的Agent",
        WhenToUse:   "当需要编写、修改或重构代码时",
        Tools:       []string{"*"},
        DisallowedTools: []string{"Agent"}, // 禁止递归派发
        MaxTurns:    100,
        Source:      "built-in",
        Color:       "#0984E3",
    },
    {
        AgentType:   "review",
        Name:        "Code Reviewer",
        Description: "代码审查Agent",
        WhenToUse:   "当需要审查代码变更、检查安全问题或评估代码质量时",
        Tools:       []string{"read_file", "Glob", "Grep", "bash"},
        MaxTurns:    50,
        Source:      "built-in",
        Color:       "#D63031",
    },
    {
        AgentType:   "moa",
        Name:        "MoA Agent",
        Description: "多模型并行推理",
        WhenToUse:   "当需要多个模型从不同角度分析问题时，或需要最高准确度的答案时",
        Tools:       []string{"read_file", "Glob", "Grep", "bash"}, // MoA agent 本身是只读的，决策由 Orchestrator 做
        MaxTurns:    30,
        Source:      "built-in",
        Color:       "#FDCB6E",
        PermissionMode: "accepts",
    },
}
```

### 2.3 YAML 用户定义Agent

```yaml
# ~/.tlaude-code/agents/my-reviewer.yaml
agent_type: my-reviewer
name: "My Custom Reviewer"
description: "自定义代码审查"
when_to_use: "审查特定类型的代码"
tools: ["read_file", "Glob", "Grep"]
max_turns: 50
model: claude-sonnet-4
provider: anthropic
permission_mode: accepts
color: "#E17055"
system_prompt: |
  你是一位资深代码审查者，关注以下方面：
  1. 安全性问题
  2. 性能瓶颈
  3. 代码可维护性
  4. 最佳实践
```

## 3. Agent 运行时

### 3.1 Agent 状态

```go
// internal/agent/types.go

type AgentState string
const (
    AgentPending    AgentState = "pending"
    AgentRunning    AgentState = "running"
    AgentCompleted  AgentState = "completed"
    AgentFailed     AgentState = "failed"
    AgentCancelled  AgentState = "cancelled"
)

type AgentRun struct {
    ID           string
    Definition   *AgentDefinition
    State        AgentState
    Prompt       string          // 原始提示
    SystemPrompt string          // 最终的 System Prompt
    Model        string          // 实际使用的模型
    Provider     string          // 实际使用的Provider
    ParentID     string          // 父Agent ID（用于追踪嵌套）
    CreatedAt    time.Time
    CompletedAt  *time.Time
    Result       string          // 最终结果摘要
    TokensInput  int
    TokensOutput int
    Messages     []Message       // 对话消息
    Error        string
    AbortSignal  chan struct{}   // 用于取消
}
```

### 3.2 Agent 运行时

```go
// internal/agent/runtime.go
// 对标 CC 的 runAgent.ts

type AgentRuntime struct {
    store      *AgentDefStore
    toolReg    *tool.Registry
    llmRouter  *llm.Router     // 多Provider路由
    taskMgr    *TaskManager
    costRouter *cost.Router
    logger     *slog.Logger
}

func NewAgentRuntime(store *AgentDefStore, toolReg *tool.Registry, ...) *AgentRuntime

// RunAgent 同步运行一个Agent（对标 CC 的 runAgent.ts）
// 返回 AgentRun，调用者可以通过 agentRun.Messages 流式读取结果
func (r *AgentRuntime) RunAgent(ctx context.Context, def *AgentDefinition, prompt string, opts *RunOptions) (*AgentRun, error)

// RunAgentAsync 异步运行一个Agent（后台模式）
// 任务完成后通过回调通知
func (r *AgentRuntime) RunAgentAsync(ctx context.Context, def *AgentDefinition, prompt string, opts *RunOptions) (agentID string, err error)

// RunForkAgent 运行一个Fork子Agent（继承上下文）
// 对标 CC 的 forkSubagent.ts
func (r *AgentRuntime) RunForkAgent(ctx context.Context, parentCtx *AgentContext, directive string, opts *ForkOptions) (*AgentRun, error)

// RunMoAAgent 运行MoA多模型Agent
// 同一个任务派给多个模型并行执行，聚合结果
func (r *AgentRuntime) RunMoAAgent(ctx context.Context, def *AgentDefinition, prompt string, models []ModelConfig) (*AggregatedResult, error)

type RunOptions struct {
    ParentID       string
    AbortSignal    <-chan struct{}
    Isolation      string        // "" / "worktree" / "remote"
    PermissionMode string
    Env            map[string]string
    MaxTurns       int
    Tools          []string      // 工具白名单（nil = 用定义里的）
}

type ForkOptions struct {
    ParentMessages    []Message    // 父Agent的完整对话
    ParentSystemPrompt string      // 父Agent的System Prompt（字节相同，缓存共享）
    Directive         string      // 子Agent的指令
}
```

### 3.3 Agent 上下文继承 (Fork 模式)

```
父Agent对话 [m1, m2, ..., tool_use_block → tool_result → m3]
                        │
                        ▼
Fork 子Agent 消息构建 (对标 CC 的 buildForkedMessages):
  [fork_boilerplate 系统提示]   ← 来自父Agent的SystemPrompt字节（缓存命中）
  [父 tool_use / placeholder tool_result]  ← 占位符，所有子Agent相同（缓存命中）
  [子Agent 专属指令]            ← 唯一变化的部分
```

```go
// internal/agent/fork.go
// 对标 CC 的 forkSubagent.ts

// BuildForkedMessages 构建Fork子Agent的消息列表
// 关键是：所有子Agent共享相同的 SystemPrompt + tool_result 占位符
// 这样API请求的前缀字节完全相同 → prompt cache命中率最大化
func BuildForkedMessages(parentSystemPrompt string, parentMessages []Message, directive string) (sysPrompt string, messages []Message) {
    // 1. 使用父Agent的SystemPrompt（字节相同）
    sysPrompt = parentSystemPrompt
    
    // 2. 使用占位符填充所有tool_result
    var toolUseBlocks []ToolUseBlock
    for _, msg := range parentMessages {
        for _, block := range msg.Content {
            if block.Type == "tool_use" {
                toolUseBlocks = append(toolUseBlocks, block)
            }
        }
    }
    
    // 3. 所有子Agent共享相同的 tool_result 占位符内容
    placeholderContent := "[Fork started — running in background]"
    
    // 4. 构建消息列表
    messages = appendPlaceholderResults(toolUseBlocks, placeholderContent)
    messages = append(directiveMessage(directive))
    
    return
}
```

## 4. MoA 多Agent融合

### 4.1 MoA Agent 执行

```go
// internal/agent/moa.go

type ModelConfig struct {
    Provider string
    Model    string
    Weight   float64     // 权重（用于加权投票/聚合）
    Priority int         // 优先级（fastest模式用）
}

// AggregatedResult MoA聚合结果
type AggregatedResult struct {
    Results    []*AgentRun       // 每个模型的独立结果
    Final      string            // 最终聚合结果
    Strategy   string            // fastest/consensus/majority/synthesize
    Consensus  float64           // 共识度
    TimeCost   time.Duration
    TokenCost  float64           // 总token成本（美元）
}

// RunMoAAgent 运行MoA多模型并行Agent
func (r *AgentRuntime) RunMoAAgent(ctx context.Context, def *AgentDefinition, prompt string, models []ModelConfig) (*AggregatedResult, error) {
    // 1. 并行派发给多个模型
    results := make(chan *AgentRun, len(models))
    ctx, cancel := context.WithCancel(ctx)
    defer cancel()
    
    for _, mc := range models {
        go func(mc ModelConfig) {
            agentDef := *def
            agentDef.Model = mc.Model
            agentDef.Provider = mc.Provider
            run, _ := r.RunAgent(ctx, &agentDef, prompt, nil)
            results <- run
        }(mc)
    }
    
    // 2. 收集所有结果
    var agentRuns []*AgentRun
    for i := 0; i < len(models); i++ {
        agentRuns = append(agentRuns, <-results)
    }
    
    // 3. 聚合结果（使用现有的 MoA 合成器）
    return r.aggregate(agentRuns, models)
}

// aggregate 聚合多个Agent的结果
func (r *AgentRuntime) aggregate(runs []*AgentRun, models []ModelConfig) (*AggregatedResult, error) {
    // 使用 internal/moa/synthesizer.go 的合成逻辑
    // strategy 从 config 读取: fastest / consensus / majority / synthesize
}
```

### 4.2 成本路由 + Agent 选择

```go
// internal/agent/router.go

// SelectModelsForTask 根据任务自动选择模型组合
func (r *AgentRuntime) SelectModelsForTask(task string, complexity int) []ModelConfig {
    // 简单任务: 只用快速便宜模型
    if complexity == 0 {
        return []ModelConfig{
            {Provider: "deepseek", Model: "deepseek-chat", Weight: 1.0},
        }
    }
    // 中等任务: 一个主力 + 一个验证
    if complexity == 1 {
        return []ModelConfig{
            {Provider: "anthropic", Model: "claude-sonnet-4", Weight: 0.7},
            {Provider: "deepseek", Model: "deepseek-chat", Weight: 0.3},
        }
    }
    // 复杂任务: MoA 多模型并行
    return []ModelConfig{
        {Provider: "anthropic", Model: "claude-sonnet-4", Weight: 0.4},
        {Provider: "openai", Model: "gpt-4o", Weight: 0.3},
        {Provider: "deepseek", Model: "deepseek-chat", Weight: 0.2},
        {Provider: "tongyi", Model: "qwen-plus", Weight: 0.1},
    }
}
```

## 5. Agent 工具 (AgentTool)

```go
// internal/tool/agent.go (增强现有版本)
// 对标 CC 的 AgentTool.tsx

type AgentTool struct {
    runtime *agent.AgentRuntime
}

func (t *AgentTool) Execute(ctx context.Context, input json.RawMessage, toolCtx *ToolContext) (*ToolResult, error) {
    // 1. 解析参数
    var params struct {
        Description  string `json:"description"`
        Prompt       string `json:"prompt"`
        SubagentType string `json:"subagent_type"`
        Background   bool   `json:"run_in_background"`
        Isolation    string `json:"isolation"`
        Model        string `json:"model"`
        Provider     string `json:"provider"`
    }
    
    // 2. 查找Agent定义
    def, ok := t.runtime.GetDefinition(params.SubagentType)
    if !ok {
        def = t.runtime.GetDefault()
    }
    
    // 3. 选择执行模式
    if params.Background || def.Background {
        // Async 模式: 创建后台任务，立即返回 task_id
        agentID, _ := t.runtime.RunAgentAsync(ctx, def, params.Prompt, &RunOptions{
            Isolation: params.Isolation,
        })
        return &ToolResult{
            Content: fmt.Sprintf("后台Agent已启动 (ID: %s)\n任务将在后台继续执行。\n使用 /tasks 查看状态。", agentID),
        }, nil
    }
    
    // 4. 检查MoA模式（如果配置了并行）
    if def.AgentType == "moa" {
        models := t.runtime.SelectModelsForTask(params.Prompt, 2)
        result, _ := t.runtime.RunMoAAgent(ctx, def, params.Prompt, models)
        return &ToolResult{Content: result.Final}, nil
    }
    
    // 5. Sync 模式: 等待Agent完成
    run, err := t.runtime.RunAgent(ctx, def, params.Prompt, &RunOptions{
        Isolation: params.Isolation,
    })
    return &ToolResult{Content: run.Result}, nil
}
```

## 6. 集成与配置

### 6.1 配置新增

```yaml
# ~/.tlaude-code/config.yaml

agent:
  default_model: claude-sonnet-4
  max_turns: 200
  moa:
    enabled: true
    default_strategy: synthesize  # fastest/consensus/majority/synthesize
    auto_select: true             # 自动根据复杂度选择模型组合
  agents_dir: ~/.tlaude-code/agents/
  
  # 内置Agent开关
  builtin_agents:
    explore: true
    code: true
    review: true
    moa: true
```

### 6.2 CLI 支持

```bash
# 列出可用Agent
tlaude-code --list-agents

# 指定Agent
tlaude-code --agent explore "研究这个代码库的结构"

# MoA模式（开箱即用）
tlaude-code --moa "这个架构有什么问题？"  # 自动多模型并行
tlaude-code --moa-strategy consensus     # 指定策略
```

### 6.3 TUI 集成

- `/agent list` — 列出可用Agent
- `/agent my-reviewer "审查最近改动"` — 指定Agent
- `/moa` — 显示MoA状态
- `/moa on` — 启用MoA
- `/tasks` — 查看后台Agent任务
- Agent 状态栏显示（颜色标识、运行状态）
- Fork 子Agent 通知

## 7. 实现顺序

### Step 1: 基础设施
- `internal/agent/types.go` — 核心类型
- `internal/agent/builtin.go` — 内置Agent定义
- `internal/agent/store.go` — Agent定义存储
- `internal/agent/fork.go` — Fork消息构建

### Step 2: Agent运行时
- `internal/agent/runtime.go` — 执行引擎
- `internal/agent/runtime_sync.go` — 同步执行
- `internal/agent/runtime_async.go` — 异步执行 + 任务管理

### Step 3: MoA 融合
- `internal/agent/moa.go` — MoA多Agent执行
- `internal/agent/router.go` — 成本路由+模型选择
- 增强 `internal/moa/synthesizer.go` — 多模型结果聚合

### Step 4: 集成
- 增强 `internal/tool/agent.go` — AgentTool 接入新运行时
- TUI 集成（/agent /moa /tasks 命令）
- Config 集成
- CLI 标志

## 8. 参考源码

| 模块 | CC 源码文件 |
|------|------------|
| Agent定义 | builtInAgents.ts, loadAgentsDir.ts |
| Agent运行时 | runAgent.ts, agentToolUtils.ts |
| Fork子Agent | forkSubagent.ts |
| Coordinator | coordinatorMode.ts |
| Swarm | swarm_InProcessBackend.ts, swarm_inProcessRunner.ts |
| AgentTool | AgentTool.tsx |
| Plan模式 | planModeV2.ts |
