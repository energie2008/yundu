# 数据库初始化指南

## 前置条件

1. Docker Desktop 已启动
2. 已安装 `make` 工具

## 快速启动

```bash
# 1. 启动基础依赖（postgres/redis/nats/minio）
make dev-up

# 2. 执行数据库迁移
make db-up

# 3. 查看迁移状态
make db-status
```

## 详细步骤

### 1. 启动 PostgreSQL

```bash
make dev-up
```

这会启动以下容器：
- `airport-postgres` (端口 5432)
- `airport-redis` (端口 6379)
- `airport-nats` (端口 4222)
- `airport-minio` (端口 9000/9001)

### 2. 执行迁移

```bash
make db-up
```

使用 goose 顺序执行 `migrations/` 目录下的所有 `.sql` 文件。

当前迁移文件清单：

| 编号 | 文件 | 说明 |
|------|------|------|
| 000001 | extensions.sql | 启用 pgcrypto 扩展 |
| 000002 | identity_core.sql | users/admins/roles/auth_sessions 等身份表 |
| 000003 | plan_audit_settings.sql | 套餐、审计日志、系统设置 |
| 000004 | node_domain.sql | servers/runtimes/nodes/proxy_chains 等节点表 |
| 000005 | traffic_subscription.sql | 流量统计、订阅令牌 |
| 000006 | seed_minimal.sql | 种子数据：super_admin、基础角色 |
| 000009 | tls_certificates.sql | TLS 证书与 Profile |
| 000010 | edge_exposures.sql | 暴露方式（direct/nginx/cf-tunnel）|
| 000011 | client_compat.sql | 客户端兼容性矩阵 |
| 000012 | node_doctor.sql | 节点诊断报告 |
| 000013 | seed_compat_doctor.sql | 兼容性矩阵种子数据 |
| 000014 | protocol_registry.sql | 协议 schema 注册 |
| 000015 | outbound_policies.sql | 出站策略与 WARP profiles |
| 000016 | runtime_upgrade.sql | 运行时升级任务 |
| 000017 | lb_routing.sql | 负载均衡与路由表 |
| 000018 | seed_routing.sql | 路由种子数据 |

### 3. 验证

```bash
# 查看迁移状态
make db-status

# 手动连接数据库验证
docker exec -it airport-postgres psql -U app -d airport -c "\dt"
```

### 4. 默认管理员账号

迁移完成后，种子数据会创建以下管理员账号：

- 邮箱：`admin@test.com`
- 密码：`testpassword123`
- 角色：`super_admin`

> 生产环境请立即修改密码。

## 回滚

```bash
# 回滚最后一个迁移
make db-down

# 完全重置（危险！会删除所有数据）
make db-reset
```

## 连接信息

| 参数 | 值 |
|------|------|
| Host | localhost |
| Port | 5432 |
| Database | airport |
| User | app |
| Password | app |
| DSN | `postgres://app:app@localhost:5432/airport?sslmode=disable` |
