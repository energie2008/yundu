# ADR-0004: Runtime Provider 抽象

## 状态

已采纳 (Accepted) - 2026-06-30

## 背景

node-service 需要管理多种代理运行时后端：

1. **node-agent**：自研 Agent，通过 HTTP API 与面板通信，支持完整的配置下发、健康上报、WARP sidecar。
2. **3X-UI**：第三方面板（基于 xray），有自己的 API 和数据库，需要适配。
3. **自定义 provider**：未来可能接入的 Marzban、x-ui、sing-box-box 等面板。

早期方案将 node-agent 专用逻辑直接写在 service 层，导致：

- 新增一种后端（如 3X-UI）需要改动大量 service 代码。
- 测试困难（service 层耦合了 HTTP client 细节）。
- 能力差异无法表达（3X-UI 不支持 WARP sidecar、不支持 dry-run）。

## 决策

在 node-service 中引入 `Provider` 接口层，隔离不同运行时后端的通信细节。

### Provider 接口

```go
type Provider interface {
    // RegisterRuntime 向后端注册一个 runtime，返回后端分配的 ID
    RegisterRuntime(ctx context.Context, spec RuntimeSpec) (runtimeRef string, err error)
    // PushConfig 推送配置到后端
    PushConfig(ctx context.Context, runtimeRef string, config string) error
    // PullStats 拉取运行时统计（在线人数、流量等）
    PullStats(ctx context.Context, runtimeRef string) (*RuntimeStats, error)
    // Reload 触发后端 reload
    Reload(ctx context.Context, runtimeRef string) error
    // Rollback 回滚到上一个配置版本
    Rollback(ctx context.Context, runtimeRef string) error
    // FetchCapabilities 查询后端支持的能力
    FetchCapabilities(ctx context.Context) ([]string, error)
}
```

### 能力发现

每个 provider 通过 `FetchCapabilities()` 声明自己支持的能力：

| 能力 | node-agent | 3x-ui |
|------|-----------|-------|
| `config_push` | ✅ | ✅ |
| `health_report` | ✅ | ❌（需轮询） |
| `warp_sidecar` | ✅ | ❌ |
| `dry_run` | ✅ | ❌ |
| `runtime_upgrade` | ✅ | ❌ |
| `stats_pull` | ✅ | ✅ |

### Provider Registry

`ProviderRegistry` 按 `provider_type` 字符串查找 provider 实例：

- `node-agent` → NodeAgentProvider（通过 HTTP API 调用 agent）
- `3x-ui` → ThreeXUIProvider（通过 3X-UI HTTP API）
- `mock` → MockProvider（用于测试）

### 配置

`runtimes.provider_type` 字段决定使用哪个 provider。
`runtimes.provider_config`（JSONB）存储 provider 特定配置（如 3X-UI 的 API URL、用户名、密码）。

## 替代方案

1. **直接在 service 层 if-else 判断 provider_type**：简单但不可扩展，每新增一种后端都要改 service。
2. **用 node-agent 作为唯一中间层**：所有后端都通过 node-agent 适配，但 3X-UI 等已有面板不接受额外 agent 安装。
3. **用 gRPC 插件机制**：过度设计，当前后端类型有限（2-3 种）。

## 后果

- 新增 provider 只需实现 `Provider` 接口 + 注册到 registry，service 层零改动。
- 能力差异通过 `FetchCapabilities()` 显式表达，ConfigRenderer 可据此决定是否渲染某些配置段。
- 测试时用 MockProvider 替换真实 provider，service 层可单元测试。
- 3X-UI provider 的能力有限（无 WARP、无 dry-run、无 upgrade），需在 UI 上明确提示。
