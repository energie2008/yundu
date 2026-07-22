# ADR-0003: 节点领域模型

## 状态

已采纳 (Accepted) - 2026-06-30

## 背景

机场面板的执行平面需要管理多协议代理节点（vless-reality / hysteria2 / tuic / shadowsocks2022 等），
涉及物理服务器、运行时（runtime）、节点实例、代理链、健康监控、配置版本与发布等多个概念。
早期方案试图用单一 `nodes` 表承载所有信息，导致：

1. 服务器与运行时耦合（一台物理机多 runtime 无法表达）。
2. 节点配置无版本化，回滚困难。
3. 健康状态与节点本体混存，写入频繁拖慢列表查询。
4. 代理链（proxy chain）无独立实体，只能写死在节点 JSON 里。

## 决策

将节点领域拆分为 7 个核心聚合，每个聚合有独立的 migration 和 repo：

### 1. Server（物理服务器）
- `servers` 表：代表一台物理机或 VPS。
- 字段：`hostname`、`public_ip`、`ssh_port`、`region_id`、`tags`。
- 一个 server 可承载多个 runtime。

### 2. Runtime（运行时实例）
- `runtimes` 表：代表 server 上安装的一个代理内核实例（xray / sing-box）。
- 字段：`server_id`、`runtime_type`、`version`、`api_endpoint`、`api_token`、`capabilities`。
- `capabilities` 为 JSONB，记录 `warp_sidecar`、`xhttp`、`reality` 等能力位。

### 3. Node（逻辑节点 / 入口配置）
- `nodes` 表：代表一个对用户暴露的逻辑入口（一个 runtime 可承载多 node）。
- 字段：`runtime_id`、`protocol_type`、`transport_type`、`security_type`、`config_json`、`priority`。
- `config_json` 必须符合 `protocol_registry` 中对应的 JSON schema。

### 4. NodeGroup（节点组）
- `node_groups` 表：用于订阅分组的逻辑集合。
- 一个 node 可属于多个 group（多对多通过 `node_group_members`）。
- group 上挂载 LB 策略（`lb_policy` 字段）。

### 5. ProxyChain（代理链）
- `proxy_chains` + `proxy_chain_hops` 表：有序的出站链路（如 node → warp → direct）。
- `node_chain_bindings` 将 node 与 chain 绑定。

### 6. HealthProfile + HealthStatus（健康监控）
- `health_profiles`：健康检查策略（间隔、超时、阈值）。
- `node_health_status`：实时状态（`healthy` / `degraded` / `offline` + 在线人数、RTT）。
- `node_health_events`：状态变更历史（用于审计和告警）。

### 7. ConfigVersion + DeploymentBatch（配置版本与发布）
- `config_versions`：每次节点配置变更生成一个版本，含 `diff_summary`。
- `deployment_batches`：一批发布任务。
- `deployment_targets`：每个目标（runtime）的发布状态。
- 支持 `dry_run`（只校验不应用）和回滚标记。

## 关系图

```
regions 1───N servers 1───N runtimes 1───N nodes
                                     │
                                     ├── N───M node_groups
                                     │
                                     └── N───M proxy_chains (via node_chain_bindings)

health_profiles 1───N nodes ─── node_health_status (1:1)
                                  └── node_health_events (1:N)

config_versions N───1 nodes
deployment_batches 1───N deployment_targets N───1 runtimes
```

## 后果

- 查询节点详情需要 join 多表，但在 repo 层封装后 service 层调用清晰。
- `config_json` 的 schema 校验由 `protocol_registry` 模块负责（见任务 15/21）。
- 配置变更必须走 `config_versions` + `deployment_batches` 流程，禁止直接 PATCH `nodes.config_json`。
- 健康状态写入与节点列表查询分离，避免高频心跳写入阻塞管理端列表。
- JSONB 字段（`config_json`、`capabilities`、`tags`）统一携带 `schema_version`。
