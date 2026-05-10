# Phase 5: Sandbox 沙箱安全执行

## 目标
LLM 调用代码执行时，在隔离沙箱中运行，防止恶意/有问题的命令损害宿主机。

## 方案选型

### 方案 A: WASM (wazero) — 推荐
- **wazero**: 纯 Go WASM 运行时，零外部依赖，无需 CGO
- 优点：真正的沙箱隔离，Go 生态原声集成
- 缺点：需要 WASM 二进制，对 Python/JS 支持有限

### 方案 B: Restricted Shell — 兜底
- 在子进程中运行命令，限制：超时、只读 /tmp、无网络
- 优点：兼容所有现有工具（bash/python/node）
- 缺点：非真正沙箱，有逃逸风险

### 决策：双模式 Sandbox
**WASM为主，Restricted Shell为兜底**

1. WASM 模式：用 wazero 执行 `.wasm` 二进制（安全、隔离）
2. Restricted 模式：有限制的子进程执行（兼容、灵活）
3. 用户可选：`/sandbox wasm` 或 `/sandbox restricted`

## 实现计划

### 文件结构
```
internal/sandbox/
├── sandbox.go          — 主接口 + 双模式调度
├── wasm.go             — wazero WASM 执行器
├── restricted.go       — 受限子进程执行器
└── sandbox_test.go     — 测试
```

### TUI 集成
- `/sandbox` — 查看当前沙箱模式
- `/sandbox wasm` — 切换到 WASM 模式
- `/sandbox restricted` — 切换到受限模式
- 状态栏显示当前模式：`WASM Sandbox` / `Restricted Sandbox`
- 执行结果时显示沙箱标识

### 配置
```yaml
sandbox:
  mode: wasm               # wasm | restricted | off
  timeout_sec: 30
  max_memory_mb: 128
  allow_network: false
  allow_write: false
  temp_dir: /tmp/claude-code-sandbox
```
