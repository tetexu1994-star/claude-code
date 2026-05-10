# Phase 1: Tool System + Permission System + StreamingExecutor

> 基于 Claude Code v2.1.88 源码架构的 Go 重写设计
> 参考源: hhxpoint/claude_code (CC-Source/src/)

## 架构概览

```
用户输入 → Query Loop → LLM API Stream → StreamingToolExecutor → Tool Results → Loop Back
                            ↓                    ↓
                     [Tool uses arrive]    [Concurrent execution]
                            ↓                    ↓
                     Tool Registry ←── Permission System ──→ Permission Rules
```

## 1. Tool 接口体系

### 核心接口

```go
// internal/tool/tool.go

// ToolDefinition 是工具的定义（注入模型提示用）
type ToolDefinition struct {
    Name        string          `json:"name"`
    Description string          `json:"description"`
    InputSchema json.RawMessage `json:"input_schema"` // JSON Schema
}

// Tool 是工具的运行时接口
type Tool interface {
    // 工具元数据
    Name() string
    Description() string
    ToolDefinition() ToolDefinition
    
    // 生命周期
    IsEnabled() bool                           // 是否在当前环境可用
    
    // 执行
    Execute(ctx context.Context, input json.RawMessage, txCtx *ToolContext) (*ToolResult, error)
    
    // 权限：返回规则用于权限匹配（空 = 不限制）
    PermissionRules() []PermissionRuleMatcher   // 如 Bash(prefix:git), Bash(domain:*.com)
    
    // 并发安全：true = 可以和其他并发安全工具并行执行
    IsConcurrencySafe() bool
}

// ToolContext 工具执行上下文
type ToolContext struct {
    CWD            string
    Env            map[string]string
    AbortSignal    <-chan struct{}
    PermissionCtx  *PermissionContext
    SandboxConfig  *SandboxConfig
    Logger         *slog.Logger
    ToolHistory    []ToolResult // 用于工具间共享状态
    AppState       *AppState
}

// ToolResult 工具执行结果
type ToolResult struct {
    ToolUseID   string
    Content     string
    IsError     bool
    SourceTool  string
}
```

### 工具注册

```go
// internal/tool/registry.go
// 对照 Claude Code 的 getAllBaseTools()

type Registry struct {
    tools map[string]Tool
    mu    sync.RWMutex
}

func (r *Registry) Register(tool Tool) error
func (r *Registry) Get(name string) (Tool, bool)
func (r *Registry) GetAll(ctx *PermissionContext) []Tool  // 过滤 deny rules
func (r *Registry) AssembleToolPool(ctx *PermissionContext, mcpTools []Tool) []Tool
```

### 第一阶段实现的 15 个工具

| 工具 | 优先级 | 对应源码文件 | 说明 |
|------|--------|-------------|------|
| BashTool | P0 | BashTool.tsx | 命令行执行，命令分类、权限、sandbox |
| FileReadTool | P0 | FileReadTool.ts | 读文件，缓存，二进制检测，MCP资源集成 |
| FileEditTool | P0 | FileEditTool.ts | 文本编辑，带文件历史和 diff |
| FileWriteTool | P0 | FileWriteTool.ts | 写文件 |
| GlobTool | P0 | GlobTool.ts | 文件搜索 |
| GrepTool | P0 | GrepTool.ts | 文本搜索 |
| WebFetchTool | P1 | WebFetchTool.ts | HTTP 获取 |
| WebSearchTool | P1 | WebSearchTool.ts | 网页搜索 |
| TodoWriteTool | P1 | TodoWriteTool.ts | 任务管理 |
| AgentTool | P1 | AgentTool.tsx | 子Agent派发（基础版） |
| TaskCreateTool | P1 | TaskCreateTool.ts | 创建子任务 |
| TaskGetTool | P1 | TaskGetTool.ts | 获取任务状态 |
| TaskListTool | P1 | TaskListTool.ts | 列出任务 |
| TaskStopTool | P1 | TaskStopTool.ts | 停止任务 |
| AskUserQuestionTool | P1 | AskUserQuestionTool.ts | 提问用户 |

## 2. Permission System

### 规则格式（兼容 Claude Code DSL）

```
ToolName                    # 整个工具允许/拒绝/询问
ToolName(path_pattern)      # 匹配路径 (Bash, FileRead, FileEdit)
ToolName(domain:example.com)# 匹配域名 (WebFetch)
ToolName(prefix:git)        # 匹配命令前缀 (Bash)
Agent(agent_type)           # 匹配Agent类型
mcp__serverName             # 匹配整个MCP服务器
mcp__serverName__*          # 匹配MCP服务器所有工具
```

### 规则来源优先级（从高到低）

```go
type RuleSource int
const (
    SourceFlag       RuleSource = iota // --allow-read 等 CLI flag
    SourcePolicy                        // 企业策略
    SourceCLIArg                        // --permission 参数
    SourceUser                          // ~/.tlaude-code/settings.json
    SourceProject                       // .claude/settings.json
    SourceLocal                         // 项目 .claude/local/settings.json
    SourceSession                       // 会话内临时规则
    SourceCommand                       // 斜杠命令设置
)
```

### 权限决策流

```go
func hasPermissionsToUseTool(tool Tool, input json.RawMessage, ctx *PermissionContext) PermissionDecision {
    // Step 1: Check deny rules → block
    if rule := getDenyRuleForTool(ctx, tool, input); rule != nil {
        return DenyDecision{message: rule.message()}
    }
    
    // Step 2: Auto mode classifier (bash-specific)
    if ctx.Mode == ModeAuto && isBashTool(tool) {
        decision = classifyYoloAction(tool, input)
        if decision == Allow { return AllowDecision{} }
        if decision == Deny { return DenyDecision{message: "Auto-classifier denied"} }
    }
    
    // Step 3: Check allow rules → auto-approve
    if rule := getAllowRuleForTool(ctx, tool, input); rule != nil {
        return AllowDecision{}
    }
    
    // Step 4: Check ask rules → prompt user
    if rule := getAskRuleForTool(ctx, tool, input); rule != nil {
        return AskDecision{rule: rule}
    }
    
    // Step 5: Mode-based fallback
    switch ctx.Mode {
    case ModeAccepts:
        return AskDecision{}
    case ModeBypassPermissions:
        return AllowDecision{}
    case ModePlan, ModeEdit, ModeChat:
        return AskDecision{}
    case ModeAuto:
        return AskDecision{} // fallback
    }
}
```

### 权限模式

```go
type PermissionMode string
const (
    ModeAccepts           PermissionMode = "accepts"            // 默认
    ModePlan              PermissionMode = "plan"               // 仅计划
    ModeEdit              PermissionMode = "edit"               // 编辑模式
    ModeBypassPermissions PermissionMode = "bypass_permissions" // 无限制
    ModeAuto              PermissionMode = "auto"               // 自动分类器
    ModeChat              PermissionMode = "chat"               // 仅聊天
)
```

### 拒绝追踪（防拒绝风暴）

```go
type DenialTracking struct {
    ConsecutiveDenials int
    LastDenialTime     time.Time
}

// 连续 N 次拒绝后，自动切换回 ask 模式
const MaxConsecutiveDenials = 3
```

## 3. StreamingToolExecutor

### 设计

```go
// internal/tool/executor.go

type ToolStatus string
const (
    StatusQueued     ToolStatus = "queued"
    StatusExecuting  ToolStatus = "executing"
    StatusCompleted  ToolStatus = "completed"
    StatusYielded    ToolStatus = "yielded"   // 后台任务
)

type TrackedTool struct {
    ID              string
    Tool            Tool
    Input           json.RawMessage
    Status          ToolStatus
    IsConcurrencySafe bool
    Promise         chan *ToolResult
}

type StreamingToolExecutor struct {
    tools          []*TrackedTool
    toolDefinitions []Tool
    permissionCtx  *PermissionContext
    abortSignal    <-chan struct{}
    concurrencyLimit int          // 默认 10
    results        chan *ToolResult // 按原始顺序输出
    mu             sync.Mutex
}

func NewStreamingToolExecutor(defs []Tool, pCtx *PermissionContext, opts ...ExecutorOption) *StreamingToolExecutor

// AddTool 添加工具到执行队列
func (e *StreamingToolExecutor) AddTool(block ToolUseBlock) 

// GetNextResult 阻塞等待下一个结果（按工具接收顺序）
func (e *StreamingToolExecutor) GetNextResult(ctx context.Context) (*ToolResult, bool)

// Discard 丢弃所有待执行工具
func (e *StreamingToolExecutor) Discard()
```

### 并发策略

```
并发安全工具（FileReadTool, GlobTool, GrepTool）：
  ─ 最多 10 个并行

非并发工具（BashTool, FileEditTool, FileWriteTool）：
  ─ 串行执行，每次只有一个
  ─ BashTool 执行期间，其他安全工具仍然可以并行

结果顺序：
  ─ 按工具被接收的顺序输出
  ─ 防止模型困惑
```

## 4. 集成到现有系统

### 现有代码的迁移

| 现有文件 | 迁移方式 |
|---------|---------|
| `internal/tools/bash/bash.go` | 实现新 Tool 接口，保留执行逻辑 |
| `internal/tools/filesystem/filesystem.go` | 拆分为 FileReadTool + FileWriteTool + FileEditTool |
| `internal/tui/tui.go` | 添加 approval flow UI |
| `internal/mcp/client.go` | 保留，MCP 工具通过 assembleToolPool 合并 |
| `internal/session/session.go` | 保留，会话仍用现有格式 |
| `internal/sandbox/` | 保留，BashTool 集成 SandboxManager |

### Config 新增字段

```yaml
# ~/.tlaude-code/config.yaml

permission_mode: auto                  # accepts/plan/edit/bypass_permissions/auto/chat
always_allow:
  - Bash(prefix:git)
  - Bash(prefix:ls)
  - FileReadTool(/Users/tetexu/projects/*)
always_deny:
  - Bash(rm *)
  - Bash(> /dev/*)
always_ask:
  - FileEditTool
  - WebFetchTool

tools:
  bash:
    timeout_ms: 30000
    sandbox: restricted
  web_search:
    provider: tavily  # or serpapi
  web_fetch:
    max_content_length: 100000
```

## 5. 实现顺序

### Step 1: 接口层 (internal/tool/)
- [ ] Tool 接口 + ToolDefinition
- [ ] ToolRegistry + 注册/获取/过滤
- [ ] PermissionContext + PermissionDecision 类型
- [ ] 规则解析器 (ToolName(path) DSL)

### Step 2: 权限系统 (internal/tool/permission/)
- [ ] 规则来源管理
- [ ] 决策流 hasPermissionsToUseTool
- [ ] 拒绝追踪
- [ ] 审批 UI 集成到 TUI

### Step 3: 核心工具 (internal/tool/implementations/)
- [ ] FileReadTool (P0)
- [ ] FileEditTool (P0) 
- [ ] FileWriteTool (P0)
- [ ] BashTool (P0) - 用现有 internal/tools/bash 适配
- [ ] GlobTool (P0)
- [ ] GrepTool (P0)
- [ ] WebFetchTool (P1)
- [ ] WebSearchTool (P1)
- [ ] TodoWriteTool (P1) - 用现有 internal/tools 适配

### Step 4: StreamingExecutor (internal/tool/executor/)
- [ ] StreamingToolExecutor
- [ ] 并发池管理
- [ ] 结果排序

### Step 5: Agent 工具 (P1)
- [ ] AgentTool - 基础子Agent派发
- [ ] Task 工具套件

### Step 6: 集成
- [ ] TUI 集成 permission prompts
- [ ] Config schema 更新
- [ ] CLI flags 更新 (--permission, --allow-read, --allow-write)
- [ ] 现有 Query Loop 集成 StreamingExecutor
