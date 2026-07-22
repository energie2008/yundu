# ADR-0002: API 公共规范

## 状态

已采纳 (Accepted) - 2026-06-30

## 背景

为保证所有微服务 API 的一致性，降低前端对接成本，需要统一响应格式、错误码、鉴权方式、分页协议等公共约定。

## 决策

### 1. 基础 URL 与版本

- 所有 API 统一前缀 `/api/v1`
- 通过 api-gateway 对外暴露，服务间通过内网直连
- 公开订阅端点：`GET /sub/:token`（无前缀，方便客户端导入）

### 2. 统一响应格式

#### 成功响应

```json
{
  "code": 0,
  "message": "ok",
  "data": { ... },
  "request_id": "a1b2c3d4e5f6..."
}
```

- `code`: 业务错误码，0 表示成功
- `message`: 人类可读消息
- `data`: 响应载荷，对象或数组
- `request_id`: X-Request-ID 回传，用于排查

#### 错误响应

```json
{
  "code": 40001,
  "message": "参数错误: email 为必填项",
  "data": null,
  "request_id": "..."
}
```

### 3. HTTP 状态码与错误码约定

| HTTP | 错误码段 | 含义 |
|------|---------|------|
| 200 | 0 | 成功 |
| 400 | 400xx | 请求参数错误 |
| 401 | 401xx | 未认证 / Token 无效/过期 |
| 403 | 403xx | 无权限 |
| 404 | 404xx | 资源不存在 |
| 409 | 409xx | 资源冲突（如邮箱已注册） |
| 429 | 429xx | 限流 |
| 500 | 500xx | 服务内部错误 |
| 503 | 503xx | 下游服务不可用 |

### 4. 分页协议

**请求参数**（query string）：

| 参数 | 类型 | 默认 | 说明 |
|------|------|------|------|
| page | int | 1 | 页码，从 1 开始 |
| page_size | int | 20 | 每页条数，最大 100 |

**响应 data 结构**：

```json
{
  "items": [ ... ],
  "total": 123,
  "page": 1,
  "page_size": 20,
  "total_pages": 7
}
```

### 5. 鉴权方式

- 用户/管理员：`Authorization: Bearer <access_token>` (JWT, HS256)
- Agent：`X-Agent-Token: <hmac_token>` + `X-Server-Code: <server_code>`
- 订阅：URL 路径中的 token（`/sub/:token`）

### 6. 请求头规范

- `X-Request-ID`: 全链路追踪 ID，网关生成或透传
- `Content-Type: application/json` (POST/PUT/PATCH body)
- `X-Client-Type`: 客户端类型（clash-meta / sing-box / uri / web-admin / web-user）

### 7. 时间与数值

- 时间字段：ISO 8601 格式，UTC 时区，例如 `2026-06-30T16:00:00Z`
- 流量单位：bytes（内部存储），前端展示时转换为 KB/MB/GB/TB
- 所有 UUID 使用标准 `xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx` 格式

### 8. 命名约定

- URL path: kebab-case（如 `/admin/audit-logs`）
- JSON field: snake_case（如 `created_at`, `user_id`）
- 枚举值：小写 + 下划线（如 `healthy`, `degraded`, `offline`）

### 9. 健康检查端点

每个服务独立暴露：

| 路径 | 用途 |
|------|------|
| `GET /healthz` | liveness（进程是否存活） |
| `GET /readyz` | readiness（依赖是否就绪：DB, Redis） |
| `GET /metrics` | Prometheus 指标 |

网关聚合所有服务的 `/metrics`（通过独立端口或 federation）。

### 10. 订阅端点特殊约定

- `GET /sub/:token` 根据 `X-Client-Type` 或 query `?client=` 返回对应客户端配置
- `GET /sub/:token/info` 返回剩余流量、到期时间等轻量信息（Surge 等客户端的 `info` 路径）
- Content-Type: `text/yaml; charset=utf-8`（clash/sing-box）或 `text/plain`（URI）

## 后果

- 所有 Go 服务统一使用 `packages/config/server` 提供的 `OK()` / `Fail()` 响应函数
- 前端 API 客户端只需要判断 `code === 0` 即可处理成功
- 错误码在 `packages/config/errors.go` 中统一定义，各服务不自定义错误码段
