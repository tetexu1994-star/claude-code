# 类 Claude Code 项目 — 架构设计 v0.1

## 设计原则
1. **国内优先** — LLM Provider 以国内可直达 API 为主
2. **可互换 Provider** — 切换不丢上下文
3. **自动选路** — 探测可用性，故障切换
4. **TUI 优先** — 对标 Claude Code 终端体验

## 核心模块
- LLM Provider: DeepSeek, 硅基流动, 智谱, 通义, OpenRouter(备选), Ollama(本地)
- TUI: Bubble Tea + Lip Gloss, 聊天视图/输入/代码块/Diff/状态栏
- Tools: Bash, 文件系统, Web搜索, MCP桥接, 审批流
- Session: JSON/SQLite持久化
- Config: YAML + 自动网络检测
