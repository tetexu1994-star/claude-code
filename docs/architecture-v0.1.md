# 类 Claude Code 项目 — 架构设计 v0.1

> 设计者：ALPHA (for tete)
> 状态：初稿，待审查

---

## 一、项目概述

一个对标 Claude Code 的终端 AI 编程助手，使用 Go 语言构建。

### 核心目标
- **TUI 交互**：类 Claude Code 的沉浸式终端聊天体验
- **多 Provider 支持**：重点覆盖中国大陆可直达的 API
- **工具系统**：文件操作、命令执行、Web 搜索、MCP 扩展
- **会话管理**：持久化对话历史，支持恢复
- **审批流**：用户可控的工具执行权限

### 目标用户
- 中国大陆开发者（主力）
- 海外开发者（备选）

---

## 二、技术选型

| 维度 | 选择 | 理由 |
|:----|:----:|:------|
| 语言 | Go | 单二进制部署、并发强、启动快、跨平台 |
| TUI 框架 | Bubble Tea + Lip Gloss | 最成熟的 Go TUI 组合 |
| LLM 网关 | 多 Provider 注册 | 中国大陆网络友好 |
| 会话存储 | JSON 文件（默认） | 简单可查，用户可编辑 |
| 工具协议 | 原生 Tool + MCP 桥接 | 灵活性 + 生态兼容 |
| 构建 | Makefile | 跨平台、版本注入 |

---

## 三、模块架构

```
cmd/claude-code/main.go
│
├── internal/
│   ├── app/           ← 应用生命周期
│   ├── tui/           ← 终端界面
│   ├── llm/           ← LLM Provider 层（核心）
│   ├── tools/         ← 工具系统（核心）
│   ├── session/       ← 会话管理
│   ├── config/        ← 配置管理
│   └── mcp/           ← MCP 协议桥接
│
├── pkg/               ← 公共库
├── config.yaml
└── Makefile
```

---

## 四、核心模块详解

### 4.1 LLM Provider 层 (`internal/llm/`)

**设计目标：** Provider 之间完全可互换，切换不掉上下文。

```
Provider 接口:
- Chat(ctx, req) → (*ChatResponse, error)       // 非流式
- ChatStream(ctx, req) → (<-chan StreamChunk)   // 流式
- Models() → ([]Model, error)                   // 模型列表
- Name() → string                               // 唯一标识
- IsAvailable() → bool                          // 可达性探测
```

**支持的 Provider（按优先级）：**
1. DeepSeek — 国内直达，便宜，首选
2. 硅基流动 — API 兼容 OpenAI 格式
3. 智谱 GLM — 国内合规
4. 阿里通义 — 稳定量大
5. Ollama — 本地离线，隐私优先
6. OpenRouter — 海外备选（需科学上网）
7. OpenAI / Anthropic — 直连备选

**自动选路逻辑：**
1. 启动时并发探测所有配置的 Provider
2. 标记可达/不可达
3. 用户首选 — 可达最快的 — 备选
4. 调用失败 — 自动切换到下一个可达 Provider
5. 全部不可达 — 引导用户配置或切换到本地 Ollama

### 4.2 TUI 层 (`internal/tui/`)

**基于 Bubble Tea + Lip Gloss**

```
布局结构:
┌──────────────────────────────────────┐
│  消息列表 (MessageList)               │
│  ┌──────────────────────────────────┐│
│  │ User: 帮我修改这个函数             ││
│  │ Assist: 好的，我来看看...          ││
│  │ 正在执行: cat src/main.go         ││
│  │ ┌────────────────────────────┐   ││
│  │ │ 代码块高亮区域               │   ││
│  │ └────────────────────────────┘   ││
│  └──────────────────────────────────┘│
│                                      │
│  输入区 (InputArea)                   │
│  ┌──────────────────────────────────┐│
│  │ > /help 或直接输入问题...          ││
│  └──────────────────────────────────┘│
│                                      │
│  状态栏 (StatusBar)                   │
│  ┌──────────────────────────────────┐│
│  │ Provider: DeepSeek | 模型: ...   ││
│  └──────────────────────────────────┘│
└──────────────────────────────────────┘
```

**渲染特性：**
- Markdown 渲染（标题、列表、表格）
- 代码块语法高亮
- Diff 格式可视渲染
- 流式逐 token 输出

### 4.3 工具系统 (`internal/tools/`)

**设计目标：** 安全、可扩展、用户可控。

```
Tool 接口:
- Name() → string
- Description() → string
- Parameters() → []ParameterSpec
- Execute(ctx, args) → (*ToolResult, error)
- RiskLevel() → RiskLevel  // NONE | LOW | HIGH | DESTRUCTIVE
```

**内置工具：**
| 工具 | 风险等级 | 说明 |
|:----|:--------:|:-----|
| read_file | NONE | 读文件 |
| list_dir | NONE | 列目录 |
| grep_search | NONE | 搜索文件内容 |
| write_file | HIGH | 写文件 |
| edit_file | HIGH | 精确修改 |
| run_command | HIGH | 执行命令 |
| web_search | LOW | 网页搜索 |
| mcp_tool | 动态 | MCP 扩展工具 |

**审批流程：**
```
LLM 决定调用 Tool
  → ApprovalManager 检查风险等级
  → NONE → 自动执行
  → LOW → 通知用户（无需确认）
  → HIGH → 弹审批弹窗
  → DESTRUCTIVE → 弹审批弹窗 + 红色警告
  → 用户选择：允许 | 拒绝 | 始终允许本次会话
  → 执行或拒绝
  → 结果返回给 LLM
```

### 4.4 会话管理 (`internal/session/`)

```
Session 结构:
- ID: string          // UUID
- CreatedAt: time.Time
- UpdatedAt: time.Time
- Provider: string    // 当前使用的 Provider
- Model: string       // 当前模型
- Messages: []Message // 对话历史
- Metadata: map[string]any

存储: JSON 文件，每个会话一个文件
路径: ~/.claude-code/sessions/{id}.json
```

### 4.5 配置管理 (`internal/config/`)

**首次启动流程：**
1. 检查 ~/.claude-code/config.yaml 是否存在
2. 不存在 → 运行首次配置向导
3. 并发探测各个 Provider
4. 列出可达的 Provider 让用户选择
5. 保存配置
6. 如果全部不可达 → 推荐安装 Ollama

### 4.6 MCP 桥接 (`internal/mcp/`)

支持通过 MCP 协议扩展工具生态：
- 支持 stdio 传输（启动子进程）
- 支持 HTTP/SSE 传输
- 将 MCP Server 的 tools 注册到系统工具列表中
- 统一纳入审批流程

---

## 五、数据流

```
用户输入
  → TUI 捕获
  → 构建 Messages（含历史上下文）
  → 发送给当前 Provider
  → 流式接收回复
  → 解析工具调用
  → 执行工具（含审批）
  → 结果返回给 Provider
  → 继续生成回复
  → 流式渲染到 TUI
  → 保存到会话
```

---

## 六、目录结构

```
~/.claude-code/
├── config.yaml           # 用户配置
├── sessions/             # 会话文件 (JSON)
│   └── {uuid}.json
├── keys/                 # API Key 存储
│   └── {provider}.key
└── mcp-servers/          # MCP Server 配置
    └── servers.json
```

