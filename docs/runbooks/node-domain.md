# 节点领域模型文档

## 概述

节点领域是执行平面核心，管理从物理服务器到用户可访问入口的完整链路。
领域模型分为 7 个聚合，详见 [ADR-0003](../adr/0003-node-domain-model.md)。

## 实体关系

```
regions ── servers ── runtimes ── nodes
                                      ├── node_groups (多对多)
                                      ├── proxy_chains (多对多)
                                      └── health_status / health_events

config_versions ── nodes
deployment_batches ── deployment_targets ── runtimes
```

## 核心表说明

### servers（物理服务器）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | UUID | 主键 |
| hostname | text | 主机名 |
| public_ip | inet | 公网 IP |
| ssh_port | int | SSH 端口 |
| region_id | UUID | 关联 regions |
| tags | jsonb | 自由标签 |

### runtimes（运行时实例）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | UUID | 主键 |
| server_id | UUID | 关联 servers |
| runtime_type | text | `xray` / `sing-box` |
| version | text | 内核版本 |
| api_endpoint | text | API 地址 |
| api_token | text | API token |
| capabilities | jsonb | 能力位（warp_sidecar/xhttp 等）|
| provider_type | text | `node-agent` / `3x-ui` |
| provider_config | jsonb | provider 特定配置 |

### nodes（逻辑节点）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | UUID | 主键 |
| runtime_id | UUID | 关联 runtimes |
| protocol_type | text | `vless` / `hysteria2` / `tuic` / `shadowsocks` |
| transport_type | text | `tcp` / `ws` / `grpc` / `httpupgrade` |
| security_type | text | `reality` / `tls` / `none` |
| config_json | jsonb | 协议特定配置（需通过 schema 校验）|
| priority | int | 优先级（用于 LB 加权）|

### node_groups（节点组）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | UUID | 主键 |
| code | text | 组编码 |
| name | text | 组名称 |
| lb_policy | text | LB 策略（round_robin/weighted/least_conn/latency/sticky_user/geo_affinity）|

### proxy_chains（代理链）

有序的出站链路，如 `node → warp → direct`。

### health_status（健康状态）

| 字段 | 类型 | 说明 |
|------|------|------|
| node_id | UUID | 关联 nodes |
| status | text | `healthy` / `degraded` / `offline` |
| current_online_users | int | 当前在线人数 |
| current_rtt_ms | int | 当前 RTT（毫秒）|

### config_versions（配置版本）

每次节点配置变更生成一个版本，含 diff 摘要，支持回滚。

### deployment_batches（发布批次）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | UUID | 主键 |
| batch_type | text | `dry_run` / `deploy` / `rollback` |
| status | text | `pending` / `running` / `succeeded` / `failed` / `rolled_back` |

## Provider 抽象

节点通过 `runtimes.provider_type` 选择通信后端：

| Provider | 能力 | 适用场景 |
|----------|------|---------|
| node-agent | 全部 | 自研 Agent，支持 WARP/dry-run/upgrade |
| 3x-ui | config_push + stats_pull | 已有 3X-UI 面板的存量迁移 |
| mock | 全部（测试用） | 单元测试和本地开发 |

详见 [ADR-0004](../adr/0004-runtime-provider.md)。

## 配置变更流程

1. 管理员修改节点配置 → 生成 `config_version`（含 diff 摘要）
2. 创建 `deployment_batch`（type=dry_run）→ 校验 schema + 渲染配置
3. 确认无误后创建 `deployment_batch`（type=deploy）
4. 通过 Provider 推送配置到 runtime
5. 失败时创建 `deployment_batch`（type=rollback）回滚

所有状态变更必须写 audit log。
