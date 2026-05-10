# Phase 6: 成本感知路由 + 语义记忆搜索

## 目标
1. **路由到最便宜的可用 Provider** — 根据任务复杂度自动选 Provider
2. **跨会话记忆搜索** — 从历史 Session 中找到相关上下文

## 设计方案

### 6A: Cost Tracker（成本追踪）
- 跟踪每个 Provider 的 token 消耗（输入/输出）
- 用内部成本表估算费用（美元/百万 token）
- 存储在 session JSON 中，累计到全局统计

### 6B: Cost-Aware Router（成本路由）
- 简单任务（ls/cat/echo）→ 最便宜的 Provider
- 中等任务（编辑、问答）→ 默认 Provider
- 复杂任务（MoA、推理）→ 最强 Provider
- 阈值：输入 token 数 < 1000 = 简单，> 4000 = 复杂

### 6C: Session Memory Search（会话记忆搜索）
- 跨 session 搜索：关键字匹配 history 文件
- 用 `session.Store` 已有的 List/Load 功能
- LLM 做语义重排序（不引入第三方 embedding）

### 6D: TUI 集成
- 状态栏显示当前 Provider + 预估成本
- `/cost` 命令：显示各 Provider 的累计 token 和费用
- `/route` 命令：查看/切换路由策略
- `/search` 命令：搜索历史会话

## 文件结构
```
internal/
  cost/
    tracker.go      — Token/费用追踪
    router.go       — 成本感知路由
    rates.go        — Provider 单价表
  memory/
    searcher.go     — 跨会话关键字搜索
    result.go       — 搜索结果格式化
```
