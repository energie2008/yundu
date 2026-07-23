# 云渡（YunDu）机场管理面板

> 双内核（Xray + Sing-box）一等公民架构 · 零 SSH 运维 · 证书自动化 · 配置零断流热更
>
> 代码基线：`2b38b68` ｜ 健康度评级 B+ ｜ 目前开发中

云渡是一套面向机场（代理服务）运营的全栈管理平台，采用**三平面架构**：管理后台（admin-web）+ 用户端（user-web）+ 节点代理（node-agent），中间由 6 个 Go 微服务承接。核心亮点是 **Xray 与 Sing-box 双内核真正并行运行**（MultiRuntimePlugin）、统一渲染 IR（kernelrender）与 gRPC 双向长连接（AgentChannel）。

---

## ✨ 核心特性

- **双内核一等公民**：Xray 主内核常驻，Sing-box 按需懒启动；两核独立热重载、per-user 流量合并统计。
- **统一渲染 IR**：`NodeSpec` → kernelrender 双核渲染，带 dry-run（`xray -test` / `sing-box check`）与双核等价性测试。
- **零 SSH 运维**：Agent 一键安装（`curl | bash`）、Bootstrap 零配置启动、自升级、Nginx 骨架幂等生成与 443 冲突自愈。
- **TLS 终止分离**：`TerminationClass` 四象限分类（cf_edge / nginx / self_tcp / self_udp / reality）统一渲染决策；443 stream SNI 分流 + 8445 TLS termination。
- **证书自动化**：6 种签发模式（http / dns / certmagic / self / file / content）、15 天自动续期、`atomic.Pointer` 零中断热替换、SAN 自动同步。
- **配置零断流热更**：`ConfigPush(Full/Delta)` + SHA256 校验 + LKG 回滚；Delta Sync 走 `UpdateUsers()`。
- **套娃链式代理**：多跳中继 + `bill_at_landing` / `bill_at_entry` 计费策略。
- **完整运营能力**：套餐/优惠券/邀请返佣（含 ERC20）/工单/公告/通知/AI 诊断/TRC20 支付。
- **可观测性**：Prometheus + Grafana + Loki + OpenTelemetry 开箱即用。

---

## 🏗️ 系统架构

```
                 ┌─────────────┐   ┌────────────┐
   管理员 ─────▶ │  admin-web  │   │  user-web  │ ◀───── 终端用户
                 │   (5173)    │   │   (5174)   │
                 └──────┬──────┘   └─────┬──────┘
                        └──────┬─────────┘
                        ┌──────▼───────┐
                        │ api-gateway  │  (8080, REST + gRPC)
                        └──────┬───────┘
        ┌──────────────┬───────┼────────────┬───────────────┐
   ┌────▼────┐   ┌─────▼────┐ ┌▼──────────┐ ┌▼─────────────┐
   │identity │   │  node    │ │subscription│ │  traffic     │
   │ (8081)  │   │ (8082)   │ │  (8083)    │ │  (8084)      │
   └─────────┘   └────┬─────┘ └────────────┘ └──────────────┘
                      │ gRPC AgentChannel（双向长连接）
                 ┌────▼─────────┐
                 │  node-agent  │  MultiRuntimePlugin
                 │  (VPS)       │  ├─ Xray（常驻）
                 └──────────────┘  └─ Sing-box（按需）
```

---

## 📦 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go（`go.work` 多模块 workspace）、gRPC、PostgreSQL 16、Redis、NATS(可选)、MinIO(可选) |
| 前端 | React 18、Vite 5、TypeScript、Tailwind CSS、TanStack Query、Node.js 22 / pnpm 9 |
| 运行时内核 | sing-box `v1.13.14`、xray-core `v1.260327.0`（均 Go 库原生集成） |
| Agent | node-agent `v0.2.12`（纯静态二进制，amd64 + arm64） |
| 可观测性 | Prometheus、Grafana、Loki、OpenTelemetry Collector |
| 入口 | Caddy（面板自身 HTTPS 自动证书） |

---

## 🚀 快速开始（本地开发）

```bash
# 1. 复制环境变量并生成密钥
cp .env.example .env
go run ./cmd/genkey        # 生成 JWT_SECRET / ARGON2_SALT / HMAC_SECRET / AGENT_API_TOKEN_SALT

# 2. 启动基础设施（PostgreSQL / Redis / NATS ...）
docker compose -f deploy/docker/docker-compose.dev.yml up -d

# 3. 执行数据库迁移
go run ./cmd/migrate up

# 4. 启动微服务（各自 cmd/api）与前端（pnpm dev）
```

## 📡 端口一览

| 端口 | 服务 | 端口 | 组件 |
|---|---|---|---|
| 8080 | api-gateway | 5432 | PostgreSQL |
| 8081 | identity-service | 6379 | Redis |
| 8082 | node-service | 4222 | NATS |
| 8083 | subscription-service | 9000 | MinIO |
| 8084 | traffic-service | 9090 / 3000 | Prometheus / Grafana |
| 5173 | admin-web | 3100 / 4317 | Loki / OTel |
| 5174 | user-web | 80 / 443 | Caddy Ingress |

> 节点端（Agent VPS）：10000（控制面）、443（stream SNI）、8445（HTTPS 回源）、10085–10584（Xray API）、20086–20585（Sing-box Clash API）。

---

## 📂 目录结构

```
apps/
  admin-web/            管理后台前端（React，~40 页面）
  user-web/             用户端前端
  api-gateway/          网关（REST + gRPC 聚合）
  identity-service/     身份认证 / 用户 / 管理员
  node-service/         节点管理 / 配置渲染 / 证书下发
  node-agent/           节点代理（双内核运行时）
  subscription-service/ 订阅服务
  traffic-service/      流量服务
  yunductl/             运维 CLI
packages/
  proto/                gRPC 协议定义（AgentChannel）
  subscription/         渲染 IR（kernelrender / nodespec / chain / ruleset / validator）
  ui / config / tsconfig
migrations/             数据库迁移（000001 → 000061）
deploy/                 Docker / Helm / 安装脚本
docs/                   架构与运维文档
```

---

## 🛠️ 部署与运维

- **一键安装**：`install.sh`（子命令 `agent` / `panel` / `upgrade agent` / `upgrade panel` / `download`）。
- **CI/CD**：`.github/workflows/release.yml`，push tag `v*` 自动构建 `CGO_ENABLED=0` 纯静态二进制（linux amd64 + arm64）。
- **Agent 自升级**：面板下发 `HEARTBEAT_ACTION_UPGRADE` 后自动替换二进制并经 systemd 重启。

---

## 📚 文档

- [架构技术版本更新（代码真相基准，2026-07-24）](docs/架构技术版本更新-20260724.md)
- [下一阶段执行计划](docs/下一阶段执行计划-20260724.md)
- [系统架构](docs/architecture.md) · [ADR](docs/adr/) · [Runbooks](docs/runbooks/)

---

## ⚠️ 项目状态

目前开发中。已知待办（详见下一阶段执行计划）：清理 zombie 代码、移除硬编码密钥 fallback、完善 `ACTION_DRAIN` 优雅排空、10000 端口纳入防火墙、端口常量收敛。
