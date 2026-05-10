# 项目修复策略 — ALPHA 调度

## 当前状态

Claude Code CLI 已完成项目重构，采用 Go 标准布局：
```
cmd/claude-code/main.go         # 主入口 ✅
internal/llm/provider.go         # Provider 接口定义 ✅
internal/llm/registry.go         # 注册中心 ✅
internal/llm/anthropic/          # 完整实现 ✅
internal/llm/deepseek/           # 完整实现 ✅
internal/llm/openai/             # 存根 ❌
internal/llm/openrouter/         # 存根 ❌
internal/llm/siliconflow/        # 存根 ❌
internal/llm/tongyi/             # 存根 ❌
internal/llm/zhipu/              # 存根 ❌
internal/tools/bash/             # 完整实现 ✅
internal/tools/filesystem/       # 完整实现 ✅
internal/config/                 # 完整实现 ✅
```

## 三个问题（按严重排序）

### 🔴 问题1：注册方式不统一 — 0→5 providers 不可用

**现状：**
- `anthropic`、`deepseek` 正确使用 `llm.RegisterFactory()` 注册
- 其他5个 provider 错误调用 `llm.GlobalRegistry().Register("name", &Provider{}, ...)` 
  → 传的是 `*Provider` 而不是 `ProviderFactory` 函数

**修正方案：** 全部统一为 `llm.RegisterFactory()` 模式

### 🔴 问题2：方法名不匹配 — 5个 provider 编译失败

**现状：**
- 接口要求：`ChatStream(ctx, ChatRequest) (<-chan Chunk, error)`
- 存根实现：`Stream(ctx, ChatRequest) (<-chan Chunk, error)` ← 方法名错了

**修正方案：** `Stream` → `ChatStream`

| ✅ 问题3：7个 provider 已全部实现

**当前状态：** 所有 7 个 provider 全部使用 `RegisterFactory` 模式，方法名统一为 `ChatStream`，全部编译通过。

## 已完成的准备工作

| 项目 | 状态 | 说明 |
|------|:----:|------|
| Go 1.23.6 安装 | ✅ | 路径：`~/.local/go/` |
| CLAUDE.md 创建 | ✅ | 项目上下文文件 |
| 7 个 Provider 修复 | ✅ | 全编译通过 |
| go.sum 依赖 | ✅ | 自动下载 |
