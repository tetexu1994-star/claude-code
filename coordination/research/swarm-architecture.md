# Swarm/Teams 架构设计

## 参考源 (CC 反编译源码)
| 文件 | 行数 | 核心职责 |
|------|------|----------|
| swarm_constants.ts | 33 | 常量：TEAM_LEAD_NAME, env vars (color/plan/command) |
| swarm_teamHelpers.ts | 683 | Team JSON 文件管理, Member CRUD, 模式切换, Worktree 销毁 |
| swarm_spawnInProcess.ts | 328 | 进程内队友生成 (AsyncLocalStorage → goroutine context) |
| swarm_inProcessRunner.ts | 1552 | Agent 执行循环: 规划, 权限处理, 信箱通信, 空闲通知 |
| swarm_InProcessBackend.ts | 339 | TeammateExecutor 接口 (spawn/terminate/kill/sendMessage/isActive) |

## Go 架构 (新增 internal/swarm/)

```
internal/swarm/
  types.go          — 类型定义 (Team, Member, TeammateIdentity, TeamFile)
  constants.go      — 常量 (team lead name, env vars, mailbox path)
  team.go           — Team 文件管理 (CRUD, config.json)
  mailbox.go        — 文件级信箱通信 (writeToMailbox / readMailbox)
  runner.go         — Agent 执行循环 (goroutine-based, 上下文隔离)
  permission.go     — 权限桥接 (leader 审批 teammate 工具调用)
  backend.go        — TeammateExecutor 接口 + InProcessBackend 实现

internal/tui/team.go — TUI /team 命令 (list/create/spawn/kill/message/remove)
```

## 核心数据结构

```go
type TeamMember struct {
    AgentID       string   // "researcher@my-team"
    Name          string
    AgentType     string
    Model         string
    Prompt        string
    Color         string
    PlanRequired  bool
    JoinedAt      time.Time
    CWD           string
    SessionID     string
    Permissions   []string     // allowed tool names
    IsActive      bool
    Mode          PermissionMode
}

type TeamFile struct {
    Name          string
    Description   string
    CreatedAt     time.Time
    LeadAgentID   string
    Members       []TeamMember
}

type MailboxMessage struct {
    From      string
    Text      string
    Color     string
    Timestamp time.Time
}
```

## 关键设计决策

1. **In-process 队友用 goroutine 替代 AsyncLocalStorage**
   - CC 用 AsyncLocalStorage 隔离队友上下文
   - Go 用 `context.Context` + goroutine-local 实现相同的隔离
   - 每个队友是一个 goroutine，通过 channels 通信

2. **信箱系统用文件 (JSON)**
   - `~/.tlaude-code/teams/<team_name>/mailbox/<agent_name>/` 目录
   - 每个消息一个 JSON 文件，按时间戳命名
   - `leaderToolUseConfirmQueue` 通过内存 channel 实现（性能路径）
   - 文件信箱作为 fallback（持久化路径）

3. **权限桥接两层**
   - 第一层(性能): 内存 queue + channel，leader 直接在 TUI 审批
   - 第二层(回退): 文件信箱轮询，leader 主动回复

4. **复用现有基础设施**
   - 使用 `internal/tool/agent_tool.go` 的 agent 注册机制
   - 使用 `internal/tool/task_manager.go` 的任务管理
   - TUI 扩展 `/team` 子命令

## 实现文件清单

| 文件 | 预估行数 |
|------|----------|
| internal/swarm/types.go | 80 |
| internal/swarm/constants.go | 40 |
| internal/swarm/team.go | 300 |
| internal/swarm/mailbox.go | 180 |
| internal/swarm/runner.go | 400 |
| internal/swarm/permission.go | 250 |
| internal/swarm/backend.go | 250 |
| internal/tui/team.go | 200 |
| 修改文件 | 修改范围 |
| internal/tui/tui.go | 添加 /team 命令处理 |
| internal/agent/runtime.go | 注入 swarm store |
| cmd/tlaude-code/main.go | 初始化 swarm store |
| internal/config/config.go | 追加 SwarmConfig |

**总计**: ~1,700 行新代码 + ~50 行修改
