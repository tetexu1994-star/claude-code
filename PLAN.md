# Claude Code 替代品 — 实施计划 v1

> 目标：可商用的 Claude Code 替代工具，能接入 CC Switch，Mac 优先
> 分工：Delta（架构/策略/MCP），Mercury（监控/审批流），Claude Code（代码落地）

---

## ✅ 准备工作（已完成）

| 任务 | 谁做 | 状态 |
|------|------|:----:|
| ✅ 项目骨架 | Mercury | ✅ |
| ✅ 7 个 LLM Provider（全部实现） | Mercury + Delta | ✅ |
| ✅ 编译通过 + Go 1.23.6 安装 | Delta | ✅ |
| ✅ CLAUDE.md 创建 | Delta | ✅ |
| ✅ STRATEGY.md 更新 | Delta | ✅ |

## ✅ 阶段 1：基础加固（已完成）

| 任务 | 谁做 | 结果 |
|------|------|:----:|
| ✅ 错误处理 + 日志系统 | Claude Code | slog结构化日志 + 自定义错误类型 |
| ✅ Provider 配置向导优化 | Claude Code | 自动检测环境变量 + 连接探测 |
| ✅ 基本测试覆盖 | Claude Code | registry + config 单元测试通过 |

## ⏳ 阶段 2：核心功能（进行中 — Claude Code 后台运行）

> 🔴 代码工作全归 Claude Code，Delta 不写一行代码。Delta 只负责策略/协作板/文档。

| 任务 | 谁做 | 状态 |
|------|:----:|:----:|
| **TUI（Bubble Tea）** | Claude Code | ⏳ |
| Session 持久化（JSON） | Claude Code | ⏳ |
| **MCP 集成（stdio + SSE）** | Claude Code | ⏳ |
| CLI 入口更新（--resume等） | Claude Code | ⏳ |

## 阶段 3：差异化核心（根据竞品分析调整后）

> 竞品分析结论：**Goose**（Go, 14k⭐）是最近竞品，有MCP+TUI但没有沙箱、diff、记忆。**Aider**（Python, 27k⭐）有架构师/编辑器双模式但没有TUI。我们要打差异化的点。

| 任务 | 谁做 | 优先级 |
|------|------|:------:|
| ☐ **Diff 视图 + 内联代码审查**（Goose没有） | Claude Code | 🔴 |
| ☐ **智能 Token 预算 + 成本感知路由**（Goose/Python都没有） | Mercury + Claude Code | 🔴 |
| ☐ **Tool 审批流**（安全管控） | Mercury | 🟡 |
| ☐ **CC Switch 接入 + Manifest注册** | Delta + Claude Code | 🟡 |
| ☐ 中文优化（提示词/文档） | Delta | 🟢 |
| ☐ Windows 构建支持 | Claude Code | 🟢 |

## 阶段 4：杀手级功能（我们的护城河）

| 任务 | 谁做 | 估算 |
|------|------|:----:|
| ☐ **多 Provider 并行调用 + 择优**（Goose/Aider都没有） | Delta + Claude Code | 4h |
| ☐ **MoA（Mixture of Agents）模式** | Delta | 3h |
| ☐ **Sandbox 代码执行**（WASM内嵌—Go优势） | Claude Code + Delta | 4h |
| ☐ **持久记忆 + 语义搜索**（SQLite + embeddings—Prospector有但我们更全） | Delta + Claude Code | 3h |
| ☐ **架构师/编辑器双模式**（Aider有但Go项目没有） | Delta + Claude Code | 3h |
| ☐ 三方协作板原生集成 | Delta | 2h |

## 竞争优势设计

1. **多 Provider 同时跑** — Claude Code 只用一个模型，我们可以同时调 DeepSeek + 硅基流动 + 智谱，取最优结果
2. **MoA 模式** — 多个模型合作解难题（Hermes 已有这个能力）
3. **国内直达** — 国内用户不需要翻墙
4. **三方协作板** — 天然支持多 Agent 协作
5. **Token 省钱** — 国内 Provider 价格是 Anthropic 的 1/10
