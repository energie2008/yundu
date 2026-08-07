# 云渡（YunDu）机场系统 — 完整使用指南

> **版本**: v1.1
> **日期**: 2026-08-07
> **代码基线**: `main`（v0.8.2+）
> **目标读者**: 初次接手 YunDu 项目的运维人员 / 新部署者
> **目标**: 按本指南完成首次部署后，后续运营零 SSH，每台 VPS 标准化运行
> **配套文档**: 《运营维护指南-v2.0.md》（故障排查 / 架构细节 / 速查表，本文档引用而不重复）

---

## 目录

1. [整体认知与使用流程总览](#1-整体认知与使用流程总览)
2. [VPS 准备与搭建](#2-vps-准备与搭建)
3. [面板端安装（控制面 VPS）](#3-面板端安装控制面-vps)
4. [节点端 Agent 安装（节点 VPS）](#4-节点端-agent-安装节点-vps)
5. [面板 ↔ Agent 交互方式](#5-面板--agent-交互方式)
6. [首次配置（面板初始化）](#6-首次配置面板初始化)
7. [证书申请与配置](#7-证书申请与配置)
8. [节点配置与管理](#8-节点配置与管理)
9. [编译、升级、安装、备份、卸载](#9-编译升级安装备份卸载)
10. [注意事项与避坑指南](#10-注意事项与避坑指南)
11. [日常运营 SOP（零 SSH）](#11-日常运营-sop零-ssh)
12. [标准化运营检查清单](#12-标准化运营检查清单)
13. [高级部署模式](#13-高级部署模式)

---

## 1. 整体认知与使用流程总览

### 1.1 系统角色

YunDu 是一套"控制面 + 节点面"分离的机场系统：

| 角色 | 部署位置 | 组件 | 数量 |
|------|---------|------|------|
| 控制面（面板） | 1 台 VPS | api-gateway / identity-service / node-service / subscription-service / traffic-service + PostgreSQL + Redis + NATS + nginx + admin-web/user-web | 1 |
| 节点面 | N 台 VPS | node-agent（内嵌 sing-box + xray 双内核）+ nginx | 按需 |

### 1.2 正确使用流程总览

```
┌─────────────────────────────────────────────────────────────┐
│ 阶段一：准备（一次性）                                        │
│  1. 准备控制面 VPS（Debian/Ubuntu，1C2G+）                    │
│  2. 准备节点 VPS（按地区/线路，1C1G+）                        │
│  3. 准备域名 + Cloudflare 账号                                │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ 阶段二：面板安装（一次性，在控制面 VPS）                       │
│  4. 安装 Docker + 基础设施（PG/Redis/NATS）                   │
│  5. 安装面板微服务（install.sh panel）                        │
│  6. 执行数据库迁移                                            │
│  7. 配置 .env + nginx 反代 + SSL                              │
│  8. 部署 admin-web / user-web 前端                            │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ 阶段三：节点安装（每台节点 VPS 重复一次）                      │
│  9. 面板添加 Server → 获取 agent_token                        │
│  10. 节点 VPS 执行 install.sh agent --token=... --runtime=sing-box │
│  11. Agent 自动 Bootstrap → 拉取配置 → 启动内核                │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ 阶段四：配置运营（面板操作，零 SSH）                           │
│  12. 配置证书（ACME DNS-01 / 自签 / 上传）                     │
│  13. 添加节点 → 选择协议/传输/安全 → 同步下发                  │
│  14. 配置套餐 / 用户 / 支付                                   │
│  15. 配置订阅模板                                              │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ 阶段五：日常运营（零 SSH，仅面板操作）                         │
│  - 增删节点、改配置、看流量、发公告、处理工单                   │
│  - Agent 升级通过面板下发（HEARTBEAT_ACTION_UPGRADE）          │
│  - 证书 15 天自动续期                                          │
└─────────────────────────────────────────────────────────────┘
```

### 1.3 关键设计理念（理解后再操作，避免踩坑）

1. **双内核架构**: sing-box 是主内核（常驻），xray 是辅内核（XHTTP/REALITY 节点时懒加载）。Agent 内嵌双内核，无需独立安装 xray/sing-box 二进制。
2. **IR + Compiler**: 节点配置在面板以 NodeSpec（纯语义 IR）存储，渲染时按内核编译为 xray/sing-box JSON。一次配置，双内核输出。
3. **零 SSH 闭环**: Agent 自升级、配置热更、证书自动下发、nginx 骨架自动生成、端口自动规划——全部通过面板完成。
4. **版本号 `!=` 判据**: 面板与 Agent 版本号只要不相等就触发 reload（不是 `<`），确保 Agent 收敛到面板最新配置。

### 1.4 部署模式总览（v1.1 新增）

YunDu 自 v0.8.2 起支持三种部署模式，按场景选择：

| 模式 | 适用场景 | 面板依赖 | 运维方式 | 本文档覆盖 |
|------|---------|---------|---------|----------|
| 面板+节点模式（标准） | 多用户商业机场、多节点运营 | 必需 | 面板零 SSH 闭环 | §2-§12 主要描述 |
| Standalone 模式 | 单节点自用、个人代理 | 不需要 | 本地配置文件 + systemd | §4.6 / §13.1 |
| Docker / K8s 模式 | 云原生环境、CI/CD 集成、弹性伸缩 | 可选 | 容器编排 | §13.2 / §13.3 |

> **本文档主线**：§2-§12 默认描述"面板+节点模式"部署。若使用 Standalone 或容器化部署，请额外阅读 §13 高级部署模式。

---

## 2. VPS 准备与搭建

### 2.1 控制面 VPS 要求

| 项目 | 最低要求 | 推荐 | 生产示例 |
|------|---------|------|---------|
| CPU | 1 核 | 2 核 | 2 核 |
| 内存 | 1.5 GB | 2 GB+ | 1.9 GB（VPS190） |
| 磁盘 | 20 GB | 40 GB | 40 GB |
| 系统 | Debian 12 / Ubuntu 22.04 | Debian 12 | Debian 12 |
| 架构 | x86_64 | x86_64 | x86_64（arm64 也支持） |
| 公网 | 需固定 IP | 需固定 IP | 43.135.147.190 |

> 控制面 VPS 内存紧张时（<2GB），可只跑 PG+Redis+NATS（约 400MB），Go 微服务以 systemd 运行（每个 200MB MemoryMax），可观测性栈（Prometheus/Grafana/Loki）可选。

### 2.2 节点 VPS 要求

| 项目 | 最低要求 | 推荐 |
|------|---------|------|
| CPU | 1 核 | 1-2 核 |
| 内存 | 512 MB | 1 GB（VPS206/VPS81 为 952M） |
| 磁盘 | 10 GB | 20-45 GB |
| 系统 | Debian/Ubuntu | Ubuntu 22.04 |
| 公网 | 需固定 IP | 需固定 IP |
| 端口 | 443/80 + 节点端口段 | 见 §8.3 端口规划 |

### 2.3 VPS 初始化（每台都做）

```bash
# 1. 更新系统
apt update && apt upgrade -y

# 2. 安装基础工具
apt install -y curl wget vim ufw jq

# 3. 创建普通用户（节点 VPS 用 ubuntu 用户，控制面用 root）
# 控制面：直接用 root
# 节点：useradd -m -s /bin/bash ubuntu && usermod -aG sudo ubuntu

# 4. 配置 SSH 密钥登录（禁用密码登录）
# 编辑 /etc/ssh/sshd_config:
#   PasswordAuthentication no
#   PubkeyAuthentication yes
systemctl restart sshd

# 5. 设置时区
timedatectl set-timezone Asia/Shanghai

# 6.（可选）安装 fail2ban
apt install -y fail2ban
```

### 2.4 域名与 DNS 准备

在 Cloudflare 准备以下域名（DNS 指向控制面 VPS IP，开启 CF Proxy 橙云）：

| 域名 | 用途 | 必需 |
|------|------|------|
| `panel.example.com` | 管理面板（admin-web） | ✅ |
| `user.example.com` | 用户面板（user-web） | ✅ |
| `sub.example.com` | 订阅域名 | ✅ |
| `pay.example.com` | 易支付（如用支付宝/微信） | 可选 |
| `*.node.example.com` | 节点 SNI 域名（通配符或泛解析） | ✅ |

> **节点 SNI 域名**：建议用通配符解析（如 `*.node.example.com` → 节点 VPS IP），或每个节点一个子域名。CDN 节点的 SNI 域名需走 Cloudflare Proxy；直连节点的 SNI 域名建议 DNS 只解析不开 Proxy（灰云）。

---

## 3. 面板端安装（控制面 VPS）

### 3.1 安装 Docker + 基础设施

```bash
# 1. 安装 Docker
curl -fsSL https://get.docker.com | bash
systemctl enable --now docker

# 2. 创建数据目录
mkdir -p /opt/yundu/{bin,config,logs,data/postgres,data/redis,data/nats,backup,agent-upgrade}
mkdir -p /etc/yundu

# 3. 创建 docker-compose 文件
cat > /opt/yundu/docker-compose.yml << 'EOF'
services:
  postgres:
    image: postgres:16-alpine
    container_name: yundu-postgres
    restart: unless-stopped
    environment:
      POSTGRES_USER: app
      POSTGRES_PASSWORD: YOUR_STRONG_PASSWORD_HERE  # 改为强密码
      POSTGRES_DB: airport
      TZ: Asia/Shanghai
    ports:
      - "127.0.0.1:5433:5432"  # 不对公网暴露
    volumes:
      - /opt/yundu/data/postgres:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U app -d airport"]
      interval: 10s
      timeout: 5s
      retries: 10
    mem_limit: 256m
    networks: [yundu]

  redis:
    image: redis:7-alpine
    container_name: yundu-redis
    restart: unless-stopped
    command: ["redis-server", "--appendonly", "yes", "--maxmemory", "64mb", "--maxmemory-policy", "allkeys-lru"]
    ports:
      - "127.0.0.1:6380:6379"  # 不对公网暴露
    volumes:
      - /opt/yundu/data/redis:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 10
    mem_limit: 96m
    networks: [yundu]

  nats:
    image: nats:2-alpine
    container_name: yundu-nats
    restart: unless-stopped
    command: ["-js", "-sd", "/data", "-m", "8222", "--max_mem_store", "64MB", "--max_file_store", "512MB"]
    ports:
      - "127.0.0.1:4223:4222"
      - "127.0.0.1:8223:8222"
    volumes:
      - /opt/yundu/data/nats:/data
    healthcheck:
      test: ["CMD", "wget", "--spider", "-q", "http://localhost:8222/healthz"]
      interval: 10s
      timeout: 5s
      retries: 10
    mem_limit: 96m
    networks: [yundu]

networks:
  yundu:
    driver: bridge
EOF

# 4. 启动基础设施
cd /opt/yundu && docker compose up -d
docker ps  # 确认三个容器 healthy
```

> **端口说明**: PG 用 5433（避开宝塔 mysqld 3306）、Redis 用 6380、NATS 用 4223，均只绑定 127.0.0.1，不对公网暴露。

### 3.2 安装面板微服务

```bash
# 方式A：用 install.sh 一键安装（推荐，需 GitHub Release 已发布）
curl -fsSL https://github.com/energie2008/yundu/raw/main/install.sh | bash -s -- panel

# 方式B：手动下载二进制（见 §9 编译部署）
```

install.sh 会：
1. 下载 5 个微服务二进制 + migrate 工具到 `/opt/yundu/bin/`
2. 创建 systemd 服务文件（`yundu-api-gateway` 等）
3. 启动服务

### 3.3 执行数据库迁移

```bash
cd /opt/yundu/bin

# 设置数据库连接（与 docker-compose 中一致）
export DATABASE_URL="postgres://app:YOUR_STRONG_PASSWORD_HERE@127.0.0.1:5433/airport?sslmode=disable"

# 执行迁移
./migrate up

# 验证迁移版本
docker exec yundu-postgres psql -U app -d airport -c "SELECT max(version_id) FROM goose_db_version;"
# 期望输出: 77（当前最新版本号）
```

> **迁移文件**: 共 74 个（版本号 000001-000077，缺 000007/000008/000051）。关键迁移见《运营维护指南-v2.0》§15.3。

### 3.4 配置环境变量

```bash
cat > /opt/yundu/config/.env << 'EOF'
# 应用
APP_ENV=production
APP_LOG_LEVEL=info

# 数据库
POSTGRES_HOST=127.0.0.1
POSTGRES_PORT=5433
POSTGRES_USER=app
POSTGRES_PASSWORD=YOUR_STRONG_PASSWORD_HERE
POSTGRES_DB=airport
POSTGRES_SSLMODE=disable
POSTGRES_DSN=postgres://app:YOUR_STRONG_PASSWORD_HERE@127.0.0.1:5433/airport?sslmode=disable

# Redis
REDIS_ADDR=127.0.0.1:6380
REDIS_PASSWORD=

# NATS
NATS_URL=nats://127.0.0.1:4223

# JWT（生产必须用强密钥！运行 go run ./cmd/genkey 生成）
JWT_SECRET=YOUR_GENERATED_JWT_SECRET
JWT_ACCESS_TTL_SECONDS=900
JWT_REFRESH_TTL_SECONDS=604800
ARGON2_SALT=YOUR_GENERATED_ARGON2_SALT
HMAC_SECRET=YOUR_GENERATED_HMAC_SECRET
AGENT_API_TOKEN_SALT=YOUR_GENERATED_AGENT_TOKEN_SALT

# 服务端口
API_GATEWAY_PORT=8090
IDENTITY_SERVICE_PORT=8081
NODE_SERVICE_PORT=8082
SUBSCRIPTION_SERVICE_PORT=8083
TRAFFIC_SERVICE_PORT=8084

# 服务地址（api-gateway 调用其他服务）
IDENTITY_SERVICE_ADDR=127.0.0.1:8081
NODE_SERVICE_ADDR=127.0.0.1:8082
SUBSCRIPTION_SERVICE_ADDR=127.0.0.1:8083
TRAFFIC_SERVICE_ADDR=127.0.0.1:8084

# CORS（改为你的域名）
CORS_ALLOWED_ORIGINS=https://panel.example.com,https://user.example.com

# 订阅
SUB_BASE_URL=https://sub.example.com

# ACME 证书
ACME_EMAIL=admin@example.com
ACME_DIR_URL=https://acme-v02.api.letsencrypt.org/directory
ACME_CHALLENGE_TYPE=dns-01
ACME_DNS_PROVIDER=cloudflare

# AI 诊断（可选）
LLM_DEEPSEEK_API_KEY=
EOF

chmod 600 /opt/yundu/config/.env
```

> **生成密钥**: `go run ./cmd/genkey` 会生成 JWT_SECRET / ARGON2_SALT / HMAC_SECRET / AGENT_API_TOKEN_SALT，生产环境必须替换默认值。

### 3.5 配置 nginx 反向代理 + SSL

```bash
# 1. 安装 nginx
apt install -y nginx

# 2. 申请 SSL 证书（用 acme.sh + Cloudflare DNS-01）
curl https://get.acme.sh | sh
export CF_Token="YOUR_CLOUDFLARE_API_TOKEN"
~/.acme.sh/acme.sh --issue --dns dns_cf -d panel.example.com -d user.example.com -d sub.example.com
~/.acme.sh/acme.sh --install-cert -d panel.example.com --key-file /etc/nginx/ssl/panel.key --fullchain-file /etc/nginx/ssl/panel.crt
~/.acme.sh/acme.sh --install-cert -d user.example.com --key-file /etc/nginx/ssl/user.key --fullchain-file /etc/nginx/ssl/user.crt
~/.acme.sh/acme.sh --install-cert -d sub.example.com --key-file /etc/nginx/ssl/sub.key --fullchain-file /etc/nginx/ssl/sub.crt

# 3. 创建 nginx 配置
cat > /etc/nginx/conf.d/panel.example.com.conf << 'EOF'
server {
    listen 443 ssl http2;
    server_name panel.example.com;

    ssl_certificate /etc/nginx/ssl/panel.crt;
    ssl_certificate_key /etc/nginx/ssl/panel.key;
    ssl_protocols TLSv1.2 TLSv1.3;

    # Cloudflare 真实 IP
    set_real_ip_from 103.21.244.0/22;
    set_real_ip_from 104.16.0.0/13;
    real_ip_header CF-Connecting-IP;
    real_ip_recursive on;

    # admin-web 静态文件
    root /var/www/admin-web;
    index index.html;

    location / {
        try_files $uri $uri/ /index.html;
    }

    # API 反代到 api-gateway
    location /api/ {
        proxy_pass http://127.0.0.1:8090;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # Grafana（可选）
    location /grafana/ {
        proxy_pass http://127.0.0.1:3000/;
    }
}

server {
    listen 80;
    server_name panel.example.com;
    return 301 https://$host$request_uri;
}
EOF

# 类似创建 user.example.com.conf（root /var/www/user-web）和 sub.example.com.conf（见 §6.4）

nginx -t && systemctl reload nginx
```

### 3.6 部署前端

```bash
# 从 GitHub Release 下载前端 dist
mkdir -p /var/www/admin-web /var/www/user-web
cd /tmp
wget https://github.com/energie2008/yundu/releases/download/v0.7.21/admin-web-dist.tar.gz
wget https://github.com/energie2008/yundu/releases/download/v0.7.21/user-web-dist.tar.gz
tar -xzf admin-web-dist.tar.gz -C /var/www/admin-web
tar -xzf user-web-dist.tar.gz -C /var/www/user-web
chown -R www-data:www-data /var/www/
```

### 3.7 启动并验证面板

```bash
# 重启所有微服务（按顺序）
systemctl restart yundu-identity-service
sleep 2
systemctl restart yundu-node-service
systemctl restart yundu-subscription-service
systemctl restart yundu-traffic-service
systemctl restart yundu-api-gateway

# 验证
systemctl status 'yundu-*'  # 全部 active
curl -k https://panel.example.com/api/v1/health  # 返回 OK
```

---

## 4. 节点端 Agent 安装（节点 VPS）

### 4.1 面板添加 Server 并获取 Token

1. 登录管理面板 `https://panel.example.com`
2. 进入 **服务器管理 → 添加服务器**
3. 填写：服务器名称、IP 地址、SSH 用户（可选）、备注
4. 保存后，系统生成 `agent_token`（在服务器详情页可见）
5. 复制 token，用于下一步安装命令

### 4.2 在节点 VPS 执行一键安装

```bash
# ⚠️ 关键：必须显式指定 --runtime=sing-box（install.sh 默认是 xray，P2 翻转后 sing-box 是主内核）
curl -fsSL https://github.com/energie2008/yundu/raw/main/install.sh | bash -s -- \
  agent \
  --token=YOUR_AGENT_TOKEN \
  --endpoint=https://panel.example.com \
  --runtime=sing-box
```

install.sh 会自动完成：
1. 下载 node-agent 二进制到 `/opt/yundu/bin/node-agent`
2. 创建配置目录 `/etc/yundu/`（config/certs 子目录）
3. 禁用并 mask 系统 cloudflared.service（避免冲突）
4. 配置 ufw 防火墙放行直连端口段（30000-30200/tcp、40020-40200/udp 等）
5. 创建 systemd 服务 `yundu-node-agent`
6. 启动服务

### 4.3 Agent 启动后的自动流程（零干预）

```
Agent 启动
   │
   ├─ 1. 读取 /etc/yundu/node-agent.env（RUNTIME_TYPE=sing-box）
   ├─ 2. 调用 Bootstrap API: GET /api/v1/agent/bootstrap?token=xxx
   │     → 返回 runtime_type / runtime_bin / 节点列表 / 心跳间隔
   ├─ 3. 启动主内核 sing-box（常驻）
   ├─ 4. 首次心跳上报（携带 X-Runtime-Ref: sing-box 头）
   ├─ 5. 面板匹配 runtime → 下发配置版本
   ├─ 6. Agent 拉取配置 → SHA256 校验 → applyConfig
   ├─ 7. 按需懒加载 xray 辅内核（仅 XHTTP/REALITY 节点）
   ├─ 8. 生成 nginx 骨架（80/443/8445）+ 证书下发
   └─ 9. 开始正常心跳循环（10s 一次）
```

### 4.4 验证 Agent 安装

```bash
# 1. 服务状态
systemctl status yundu-node-agent  # 应为 active

# 2. 日志检查
journalctl -u yundu-node-agent -f --since "2 min ago"
# 期望看到: "bootstrap success" / "config applied successfully" / "heartbeat sent"

# 3. 面板验证
# 进入面板 → 节点列表 → 该节点应显示"在线"
```

### 4.5 多节点批量安装

对每台节点 VPS 重复 §4.1-4.4。每台节点用不同的 agent_token（面板为每台 Server 生成独立 token）。

### 4.6 方式三：Standalone 模式安装（无需面板）

适合单节点自用场景，不需要控制面 VPS，全部配置本地化。详见 §13.1。

```bash
# 方式三：Standalone 模式（无需面板）
# 1. 写入模板配置
node-agent --init-standalone --standalone-config=/etc/yundu/standalone.json
# 2. 编辑配置文件，填入节点信息
vi /etc/yundu/standalone.json
# 3. 启动
node-agent --standalone --standalone-config=/etc/yundu/standalone.json
```

> ⚠️ **Standalone 模式限制**：不支持面板功能（用户管理 / 流量统计 / 订阅下发 / 在线设备踢人），仅适合单节点自用。如需运营机场，请使用面板+节点模式。

---

## 5. 面板 ↔ Agent 交互方式

### 5.1 通信通道（三通道，优先级降级）

```
                    面板 node-service (8082)
                         │
            ┌────────────┼────────────┐
            │            │            │
     ┌──────▼─────┐ ┌───▼────┐ ┌────▼─────┐
     │ gRPC Stream│ │ WS 心跳 │ │ HTTP 心跳│
     │ (9082)     │ │ (8082)  │ │ (8082)   │
     │ 双向流     │ │ 双向    │ │ 单向轮询 │
     │ 优先级最高 │ │ 次选    │ │ 兜底     │
     └────────────┘ └────────┘ └──────────┘
```

| 通道 | 端口 | 用途 | 触发条件 |
|------|------|------|---------|
| gRPC Stream | 9082 | 双向实时推送（配置下发、流量上报、设备踢人） | Agent 默认优先尝试 |
| WebSocket | 8082 | 双向实时（gRPC 不可用时降级） | gRPC 失败后降级 |
| HTTP 轮询 | 8082 | 兜底（WS 也不可用时） | WS 失败后降级，10s 一次 |

> **CF Proxy 场景**: gRPC 需走 443 端口且 CF 需开启 gRPC 支持。若 endpoint 走 CF Proxy 且非 443，需加 `--disable-grpc` 参数禁用 gRPC，直接用 WS。

### 5.2 关键 API 端点

| 端点 | 方法 | 用途 | 认证 |
|------|------|------|------|
| `/api/v1/agent/bootstrap` | GET | 首次启动拉取运行时配置 | token 参数 |
| `/api/v1/agent/heartbeat` | POST | HTTP 心跳上报 | X-Agent-Token 头 |
| `/api/v1/agent/runtime-config` | GET | 拉取最新配置版本 | X-Agent-Token 头 |
| `/api/v1/agent/cloudflared-tunnels` | GET | 拉取隧道配置 | X-Agent-Token 头 |
| `/api/v1/agent/machine/nodes` | GET | Machine 模式节点发现 | server_token 参数 |
| gRPC `AgentChannel.Stream` | Stream | 双向实时通信 | gRPC metadata |

### 5.3 配置下发与版本同步

**面板侧决策**（`agent_handler.go:184-199`）:
1. Agent 心跳上报 `config_version_current`（当前已应用版本号）
2. 面板对比 `currentVersion != targetVersion.VersionNo`
3. 不相等 → 心跳响应携带 `action=RELOAD`
4. 相等 → 无动作

**Agent 侧执行**:
1. 收到 `action=RELOAD` → 调用 `/api/v1/agent/runtime-config` 拉取配置
2. SHA256 校验配置完整性（与面板下发的 `config_signature` 比对）
3. HotDiff 策略应用：
   - 仅用户变更 → AlterInbound（热重载，不重启）
   - 仅路由变更 → ReloadRouting
   - TLS 变更 → ReloadTLS（SIGUSR1）
   - 结构变更 → 30s 防抖合并 → 一次全量重启
   - 失败 → LKG（Last Known Good）自动回滚
4. 应用成功 → 回报 ConfigResult → 面板更新 `dispatch_status=applied`

> **详见**: 《运营维护指南-v2.0》§4.3 版本同步机制

### 5.4 Agent 自升级

```
面板 → 系统设置 → Agent版本管理 → 填写 version / download_url / sha256
   │
   ▼
心跳响应携带 action=HEARTBEAT_ACTION_UPGRADE
   │
   ▼
Agent 下载二进制（SHA256 校验）→ 原子替换 → systemd 重启
   │
   ▼
重启后心跳上报新版本号 → 面板确认收敛
```

> **避坑**: sha256 必须与实际二进制一致；download_url 可用面板自身下载端点（二进制放 `/opt/yundu/agent-upgrade/`）。

---

## 6. 首次配置（面板初始化）

### 6.1 管理员账号

首次访问 `https://panel.example.com`，系统引导创建超级管理员账号（邮箱 + 密码）。登录后进入管理面板。

### 6.2 系统设置

进入 **系统设置**，配置：

| 配置项 | 说明 |
|--------|------|
| 站点名称 | 显示在用户面板 |
| 站点公告 | 用户登录后可见 |
| 订阅基础 URL | `https://sub.example.com` |
| SMTP 邮件 | 注册/找回密码/通知用 |
| ACME 设置 | 证书自动签发（见 §7） |
| Agent 版本 | 自升级用 |

### 6.3 节点分组与套餐

1. **节点分组**：创建节点组（如"香港"、"日本"、"美国"），节点通过分组关联套餐。
   > ⚠️ 节点不直接绑定套餐，仅通过节点分组关联（硬约束）。
2. **套餐管理**：创建套餐（名称、流量、时长、价格、关联节点分组）。
   > ⚠️ 纯中文名套餐会自动生成唯一 code，无需手动填。

### 6.4 订阅域名 nginx 配置（关键！）

订阅域名 `sub.example.com` 的 nginx vhost **必须正确配置**，否则客户端订阅请求会落 default server 返回前端 HTML，导致"无更新服务器"。

```nginx
# /etc/nginx/conf.d/sub.example.com.conf
server {
    listen 443 ssl http2;
    server_name sub.example.com;

    ssl_certificate /etc/nginx/ssl/sub.crt;
    ssl_certificate_key /etc/nginx/ssl/sub.key;
    ssl_protocols TLSv1.2 TLSv1.3;

    set_real_ip_from 103.21.244.0/22;
    set_real_ip_from 104.16.0.0/13;
    real_ip_header CF-Connecting-IP;
    real_ip_recursive on;

    # 订阅接口 → subscription-service
    location ~ ^/(sub|s)/ {
        proxy_pass http://127.0.0.1:8083;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 30s;
    }

    # 其他路径跳转到用户面板
    location / {
        return 302 https://user.example.com/;
    }
}
```

### 6.5 支付配置

进入 **系统设置 → 支付配置**，按需启用：

| 支付方式 | 配置 |
|---------|------|
| 支付宝/微信 | 配置彩虹易支付（epay）网关地址、PID、Key |
| USDT-TRC20 | 填写收款地址，系统通过 TronGrid API 自动查询到账 |
| USDT-BEP20 | 填写收款地址，系统通过 BSC 公共 RPC（eth_getLogs）自动查询到账 |
| USDT-ERC20 | 填写收款地址，系统通过 EVM API 自动查询到账 |

> 支付优先级：支付宝 > 微信 > TRC20 > BEP20 > ERC20
> BEP20 无需 API Key，用 BSC 公共 RPC 节点查询最近 5000 个区块的 Transfer 事件。

---

## 7. 证书申请与配置

### 7.1 证书签发模式（6 种）

| 模式 | 适用场景 | 推荐度 |
|------|---------|--------|
| `dns` | 通用，ACME DNS-01（CF API token） | ⭐⭐⭐ 推荐 |
| `http` | CDN 节点，ACME HTTP-01（80 端口） | ⭐⭐ |
| `certmagic` | Agent 本地自动管理 | ⭐⭐ |
| `self` | 直连伪装（自签 ECDSA P-256） | ⭐ |
| `file` | 已有证书，面板上传 PEM | ⭐⭐ |
| `content` | 面板集中签发 + 推送 | ⭐⭐⭐ |

### 7.2 推荐方案：ACME DNS-01 + Cloudflare

**配置步骤**:

1. 在 Cloudflare 创建 API Token（权限：Zone.DNS.Edit，区域：你的域名）
2. 面板 → **系统设置 → ACME 设置**：
   - Email: `admin@example.com`
   - Directory URL: `https://acme-v02.api.letsencrypt.org/directory`（生产）
   - Challenge Type: `dns-01`
   - DNS Provider: `cloudflare`
   - CF API Token: 粘贴上一步的 token
3. 面板 → **证书管理 → 申请证书**：输入域名（如 `*.node.example.com`）
4. 系统自动签发，15 天自动续期，原子热替换

### 7.3 证书四级回退

当节点需要证书时，面板按以下顺序查找：

1. 节点 `cert_bundle_id` 绑定的证书
2. `cert_bundles` 表 SAN 匹配
3. `tls_certificates` 表 SNI 匹配
4. 自签 ECDSA P-256 兜底（含 SHA256 指纹告警）

### 7.4 证书避坑

1. **必须生成 SAN**: Go 的 crypto/tls 只匹配 SAN 不匹配 CN。用 openssl 配置文件方式，不要用 `-addext`（避免 BasicConstraints 重复）。
2. **自签证书客户端配置**: sing-box 客户端设 `insecure: true`；xray 26.x 废弃了 `allowInsecure`，用 `insecure: true`。
3. **pinSHA256**: 仅 URI 高级选项，证书续期后需更新指纹。
4. **通配符**: 支持 `*.node.example.com` 单层通配符，精确 SAN 优先。
5. **CF Universal SSL**: 仅覆盖一级子域名，不覆盖二级子域名（如 `*.node.example.com` 不覆盖 `a.b.node.example.com`）。

### 7.5 证书与 exposure_mode 的关系

| exposure_mode | TLS 终止点 | 证书持有方 |
|---------------|----------|----------|
| `argo_tunnel` | CF 边缘 | CF（不需要面板证书） |
| `cdn` / `cdn_saas` | nginx 8445 vhost | nginx（面板下发证书） |
| `direct` | xray/sing-box 自终止 | runtime（面板下发证书） |
| `reality` | xray REALITY 握手 | 无（借用目标网站 TLS） |

> **关键原则**: `direct`/`reality` 节点**绝不剥离** TLS；`argo_tunnel`/`cdn`/`cdn_saas` 节点 TLS 在 nginx/CF 剥离。
> **详见**: 《运营维护指南-v2.0》§3.3 ExposurePolicy 表

---

## 8. 节点配置与管理

### 8.1 添加节点（面板操作）

1. 面板 → **节点管理 → 添加节点**
2. 选择 **所属服务器**（已安装 Agent 的 VPS）
3. 选择 **协议 / 传输 / 安全**（见 §8.2 协议矩阵）
4. 配置 **exposure_mode**（见 §8.4）
5. 填写 **SNI 域名**（需与 DNS/证书匹配）
6. 点击 **保存并同步** → 面板自动：
   - 生成 NodeSpec IR
   - 编译为 xray/sing-box JSON
   - L1-L4 校验（preflight_validator）
   - 创建 config_version
   - WS/gRPC 推送到 Agent
   - Agent 应用配置 + 回报状态

### 8.2 协议矩阵（17 种协议）

| 协议 | 渲染内核 | 典型 exposure_mode | 端口范围 |
|------|---------|-------------------|---------|
| VLESS+REALITY+Vision | xray | reality | 30000+ |
| VLESS+WS+TLS | both | cdn / cdn_saas | nginx 8445 |
| Trojan+TLS | both | direct | 30000+ |
| Trojan+WS+TLS | both | cdn / cdn_saas | nginx 8445 |
| AnyTLS | sing-box | direct | 30000+ |
| XHTTP+TLS+CDN | xray | cdn | nginx 8445 |
| XHTTP+REALITY | xray | reality | 30000+ |
| VLESS+HTTPUpgrade+TLS | both | cdn | nginx 8445 |
| Hysteria2 | sing-box | direct (UDP) | 40020+ |
| TUIC v5 | sing-box | direct (UDP) | 40210+ |

> 完整 17 协议列表见《运营维护指南-v2.0》§15.8

### 8.3 端口规划（Agent 自动分配）

Agent 的 PortAllocator 按 runtime 隔离自动分配端口：

| 端口段 | 用途 |
|--------|------|
| 443 | nginx stream SNI 分流入口 |
| 80 | nginx HTTP / ACME 验证 |
| 8445 | nginx HTTPS 回源（CDN 节点） |
| 10000 | node-agent 控制面 API |
| 10085 | xray API（流量统计） |
| 30000-30200 | sing-box/xray 直连 TCP 节点 |
| 30300-30399 | AnyTLS / ShadowTLS 直连 |
| 40020-40200 | Hysteria2 UDP |
| 40210-40299 | TUIC UDP |
| 20530-20699 | Tunnel 回源 |

> 节点 VPS 安全组需放行：443/80 + 30000-30200/tcp + 40020-40200/udp + 40210-40299/udp（install.sh 会自动配置 ufw）。

### 8.4 exposure_mode 选择指南

| 场景 | exposure_mode | 说明 |
|------|--------------|------|
| 走 Cloudflare CDN | `cdn` | CF Proxy 橙云，nginx 终止 TLS |
| 走 CF SaaS（自定义域名回源） | `cdn_saas` | CF SaaS Custom Hostname |
| Argo Tunnel 隧道 | `argo_tunnel` | CF 边缘终止 TLS，无需公网端口 |
| 直连（TCP+TLS） | `direct` | xray/sing-box 自终止 TLS |
| 直连 REALITY | `reality` | xray REALITY 握手，无需证书 |

> **避坑**: `config_json.exposure_mode` 是唯一真相源，独立列 `nodes.exposure_mode` 仅为 DB 索引投影。不要手动改 `nodes.exposure_mode`。

### 8.5 节点配置注意事项

1. **XHTTP mode**: 禁止用 `auto`（连接不稳定）。CDN 场景用 `packet-up`，直连+REALITY 场景用 `stream-up`。
2. **CF Dashboard HTTP/2 to Origin**: 必须关闭（与 XHTTP packet-up 冲突）。
3. **隧道节点 hostname**: 必须用一级子域名（CF Universal SSL 不覆盖二级子域名）。
4. **隧道节点 listen**: 必须为 `127.0.0.1`（TLS 已剥离，不能监听公网）。
5. **cloudflared config.yml**: service 必须用 `http://127.0.0.1:<port>` 而非 `http://localhost:<port>`（localhost 解析优先 IPv6）。
6. **WARP 路由**: 仅给勾选的节点写精确路由 `{"action":"route","inbound":["in-<code>"],"outbound":"warp-pool"}`，`route.final` 保持 null。

### 8.6 修改/删除节点

- **修改**: 节点列表 → 编辑 → 保存 → 自动生成新 config_version → 推送
- **删除**: 节点列表 → 删除（软删，code 保留，释放端口）
- **同步状态**: 节点列表显示 dispatch_status（pushed/applied/failed）

### 8.7 流媒体解锁预置规则（v1.1 新增）

面板「路由管理」页面现支持 **9 大流媒体平台一键规则集**，无需手写路由规则：

| 平台 | 规则集标识 | 典型用途 |
|------|-----------|---------|
| Netflix | `netflix` | 解锁 Netflix 原创剧集 / 区域库 |
| Disney+ | `disney-plus` | 解锁 Disney+ 影视库 |
| YouTube | `youtube` | YouTube Premium 区域判定 |
| HBO Max | `hbo-max` | HBO Max 流媒体 |
| Amazon Prime Video | `prime-video` | 亚马逊原创剧集 |
| Hulu | `hulu` | 美区 Hulu |
| Spotify | `spotify` | 音乐流媒体区域 |
| TikTok | `tiktok` | TikTok 区域解锁 |
| OpenAI / ChatGPT | `openai` | ChatGPT 访问解锁 |

**使用方式**：
1. 面板 → 路由管理 → 新建路由策略
2. 选择「预置规则集」→ 勾选所需平台
3. 关联出站（direct / warp-pool / 自定义落地）
4. 保存 → 自动编译为 sing-box route ruleset / xray routing rules
5. 推送到节点，HotDiff 应用（仅路由变更 → ReloadRouting，不中断连接）

> 预置规则集由面板维护，自动跟进上游 sing-box/rule-set 仓库更新，无需手动同步 geoip/geosite 数据库。

### 8.8 xray 节点限速与 IP 限制 enforcement（v1.1 新增）

v0.8.2 起，xray 节点的**限速 / IP 限制**已完全生效，不再仅依赖 sing-box 的 DeviceEnforcer：

| 维度 | 实现方式 | 检查周期 | 超限动作 |
|------|---------|---------|---------|
| 单用户限速 | enforcement loop 后台轮询 per-user 流量 | 3 秒 | AlterInbound 移除该用户（下个周期允许重连） |
| 单用户 IP 数限制 | DeviceEnforcer 同时检查 device + IP 上限 | 心跳周期 | 移除超出 IP 的 inbound tag |
| 单用户设备数限制 | DeviceEnforcer（原有逻辑） | 心跳周期 | 移除超出设备的 inbound tag |

**机制说明**：
- 限速 enforcement loop 每 **3 秒** 检查一次每个 xray 用户的累计流量，超速用户会被 `AlterInbound` 调用移除（不是限速，而是断开），下个周期允许重新连接。
- IP 限制与设备限制由 DeviceEnforcer 联合判定，对 xray 和 sing-box 节点均生效。
- xray 的 `fullReload` 操作在重启内核前会**等待 5 秒**让现有连接 drain，避免突兀中断。

> 这意味着 xray 节点现在具备与 sing-box 节点一致的运营能力（限速 / IP 限制 / 设备限制），不再需要"为限速只能用 sing-box"的妥协。

---

## 9. 编译、升级、安装、备份、卸载

### 9.1 编译

#### 9.1.1 CI 发版（标准流程）

```bash
# 本地修改源码后
git add -A && git commit -m "feat: xxx"
git push origin main

# 打 tag 触发 CI
git tag v0.7.22
git push origin v0.7.22

# GitHub Actions 自动：
#   1. 跑测试
#   2. 编译 7 个二进制 × 2 架构（amd64 + arm64）
#   3. 编译前端 dist
#   4. 发布 GitHub Release
```

#### 9.1.2 本地交叉编译（热修复）

```bash
cd D:\机场搭建\yundu-src
export GOOS=linux GOARCH=amd64 CGO_ENABLED=0 GOTOOLCHAIN=local
export TAGS="with_utls,with_wireguard,with_gvisor,with_quic,with_grpc,with_clash_api"

# node-agent（需要 tags + 版本号注入）
cd apps/node-agent
go build -tags "$TAGS" -ldflags "-s -w -X main.AgentVersion=v0.7.22" -o node-agent ./cmd/agent

# 其他服务（无需 tags）
cd ../node-service && go build -o node-service ./cmd/api
cd ../api-gateway && go build -o api-gateway ./cmd/api
cd ../identity-service && go build -o identity-service ./cmd/api
cd ../subscription-service && go build -o subscription-service ./cmd/api
cd ../traffic-service && go build -o traffic-service ./cmd/api
```

> ⚠️ **编译标签（仅 node-agent）**: 6 个标签缺一不可：
> - `with_utls` = REALITY 必需
> - `with_wireguard` + `with_gvisor` = WARP/WireGuard 必需
> - `with_quic` = Hysteria2/TUIC 必需
> - `with_grpc` = gRPC 通信必需
> - `with_clash_api` = 流量统计必需
>
> 其他微服务是纯 Go，**不需要** tags。

### 9.2 升级

#### 9.2.1 面板组件升级（控制面 VPS）

```bash
# 方式A：install.sh 升级（需 Release 已发布）
curl -fsSL https://github.com/energie2008/yundu/raw/main/install.sh | bash -s -- upgrade panel

# 方式B：手动 SCP + 重启（热修复）
scp node-service root@PANEL_IP:/tmp/
ssh root@PANEL_IP 'systemctl stop yundu-node-service && cp /opt/yundu/bin/node-service /opt/yundu/bin/node-service.bak.$(date +%Y%m%d%H%M) && cp /tmp/node-service /opt/yundu/bin/ && chmod +x /opt/yundu/bin/node-service && systemctl start yundu-node-service'
```

> **重启顺序**: PG/Redis/NATS → identity → node → subscription → traffic → api-gateway → nginx reload

#### 9.2.2 Agent 升级（节点 VPS）

**方式A：面板自升级（零 SSH，推荐）**

1. 将新 node-agent 二进制放到面板 VPS `/opt/yundu/agent-upgrade/node-agent-v0.7.22`
2. 计算SHA256：`sha256sum /opt/yundu/agent-upgrade/node-agent-v0.7.22`
3. 面板 → 系统设置 → Agent版本管理 → 填写：
   - version: `v0.7.22`
   - download_url: `https://panel.example.com/api/v1/agent-upgrade/node-agent-v0.7.22`
   - sha256: 上一步计算的值
4. 发布 → 所有节点 Agent 自动下载升级

**方式B：install.sh 升级（需 SSH）**

```bash
curl -fsSL https://github.com/energie2008/yundu/raw/main/install.sh | bash -s -- upgrade agent
```

**方式C：手动 SCP（紧急热修复）**

```bash
# ⚠️ 运行中文件 Text file busy，必须先 stop 再 cp
scp node-agent ubuntu@NODE_IP:/tmp/
ssh ubuntu@NODE_IP 'sudo systemctl stop yundu-node-agent && sudo cp /tmp/node-agent /opt/yundu/bin/node-agent && sudo chmod +x /opt/yundu/bin/node-agent && sudo systemctl start yundu-node-agent'
```

### 9.3 备份

#### 9.3.1 数据库备份（定时）

```bash
# 手动备份
docker exec yundu-postgres pg_dump -U app airport | gzip > /opt/yundu/backup/airport_$(date +%Y%m%d_%H%M%S).sql.gz

# 定时备份（crontab -e）
0 3 * * * docker exec yundu-postgres pg_dump -U app airport | gzip > /opt/yundu/backup/airport_$(date +\%Y\%m\%d).sql.gz

# 保留最近 7 天
0 4 * * * find /opt/yundu/backup/ -name "airport_*.sql.gz" -mtime +7 -delete
```

#### 9.3.2 恢复

```bash
gunzip < /opt/yundu/backup/airport_20260807.sql.gz | docker exec -i yundu-postgres psql -U app -d airport
```

#### 9.3.3 配置文件备份

```bash
tar -czf /opt/yundu/backup/config_$(date +%Y%m%d).tar.gz \
  /opt/yundu/config/.env \
  /etc/yundu/ \
  /etc/nginx/conf.d/ \
  /opt/yundu/docker-compose.yml
```

### 9.4 卸载

#### 9.4.1 卸载节点 Agent

```bash
systemctl stop yundu-node-agent
systemctl disable yundu-node-agent
rm -f /etc/systemd/system/yundu-node-agent.service
systemctl daemon-reload

# 删除二进制和配置
rm -rf /opt/yundu/bin/node-agent /etc/yundu/ /var/log/yundu/

# 清理 nginx 配置（Agent 生成的）
rm -f /etc/nginx/conf.d/yundu_*.conf
nginx -t && systemctl reload nginx
```

#### 9.4.2 卸载面板

```bash
# 停止服务
systemctl stop 'yundu-*'
systemctl disable 'yundu-*'
rm -f /etc/systemd/system/yundu-*.service
systemctl daemon-reload

# 删除二进制
rm -rf /opt/yundu/bin/

# 删除前端
rm -rf /var/www/admin-web /var/www/user-web

# ⚠️ 数据库和 Redis 数据按需保留（不删除 /opt/yundu/data/）
# 如需彻底删除：
# docker compose -f /opt/yundu/docker-compose.yml down -v
# rm -rf /opt/yundu/data/
```

### 9.5 Docker 部署（v1.1 新增）

YunDu 提供多阶段构建的 `Dockerfile`，支持容器化部署面板与 Agent。详见 §13.2。

```bash
# 1. 在源码根目录构建镜像
docker build -t yundu:0.8.2 -t yundu:latest .

# 2. 运行面板（单容器，需挂载配置与数据卷）
docker run -d \
  --name yundu-panel \
  --restart unless-stopped \
  -p 8090:8090 -p 8081:8081 -p 8082:8082 -p 8083:8083 -p 8084:8084 \
  -v /etc/yundu:/etc/yundu \
  -v /var/log/yundu:/var/log/yundu \
  -v /opt/yundu/data:/opt/yundu/data \
  --env-file /etc/yundu/panel.env \
  yundu:0.8.2 panel

# 3. 运行 Agent（节点容器）
docker run -d \
  --name yundu-agent \
  --restart unless-stopped \
  --network host \
  --cap-add NET_ADMIN \
  -v /etc/yundu:/etc/yundu \
  -v /var/log/yundu:/var/log/yundu \
  yundu:0.8.2 agent --token=$TOKEN --endpoint=$ENDPOINT --runtime=sing-box
```

> ⚠️ **必需挂载卷**：`/etc/yundu`（配置）和 `/var/log/yundu`（日志）必须挂载到宿主机，否则容器重建后配置丢失。Agent 容器需 `--network host` 与 `NET_ADMIN` capability（用于端口监听与 ufw）。

### 9.6 Kubernetes 部署（v1.1 新增）

YunDu 提供 Helm Chart（`deploy/k8s/helm/`，含 16 个模板文件），支持在 K8s 集群中部署面板微服务。详见 §13.3。

```bash
# 1. 安装 Helm Chart（使用生产环境 values）
helm install yundu ./deploy/k8s/helm \
  -f deploy/k8s/helm/values-production.yaml \
  -n yundu --create-namespace

# 2. 验证部署
kubectl -n yundu get pods
kubectl -n yundu get svc

# 3. 升级
helm upgrade yundu ./deploy/k8s/helm \
  -f deploy/k8s/helm/values-production.yaml \
  -n yundu

# 4. 卸载
helm uninstall yundu -n yundu
```

> 生产环境推荐使用 `values-production.yaml`（已配置 PVC 持久化、HPA 自动扩缩、健康检查、PodDisruptionBudget）。

### 9.7 yunductl 管理工具（v1.1 新增）

`yunductl` 是 v0.8.2 起的标准运维 CLI，**所有 VPS（面板 + 节点）安装时自动部署到 `/usr/local/bin/yunductl`**，提供节点/面板统一运维入口。

| 命令 | 用途 | 示例 |
|------|------|------|
| `yunductl version` | 查看 node-agent / yunductl 版本 | `yunductl version` |
| `yunductl status` | 查看本地 agent 运行状态 | `yunductl status` |
| `yunductl health` | 健康检查（agent + 内核 + nginx + 证书） | `yunductl health` |
| `yunductl refresh` | 手动触发配置刷新 | `yunductl refresh` |
| `yunductl rollback` | 回滚到上一个 LKG 配置 | `yunductl rollback` |
| `yunductl restart` | 重启 node-agent 服务 | `yunductl restart` |
| `yunductl diag` | 一键诊断（收集日志/配置/端口快照） | `yunductl diag` |
| `yunductl nodes` | 列出本机节点 | `yunductl nodes` |
| `yunductl logs` | 查看 agent 日志（支持 -f / --since） | `yunductl logs -f` |
| `yunductl bind` | 绑定到面板（写入 token） | `yunductl bind --token=xxx --endpoint=https://panel.example.com` |
| `yunductl upgrade` | 升级 node-agent 二进制 | `yunductl upgrade --version=v0.8.3` |
| `yunductl config validate` | 校验本地配置文件 | `yunductl config validate /etc/yundu/standalone.json` |
| `yunductl config render` | 渲染配置（dry-run，不应用） | `yunductl config render /etc/yundu/standalone.json` |
| `yunductl server list` | 列出面板已知服务器（需已 bind） | `yunductl server list` |
| `yunductl server status` | 查看面板服务器状态 | `yunductl server status` |

```bash
# 常用组合
yunductl version          # 确认版本
yunductl health           # 健康巡检
yunductl config validate /etc/yundu/standalone.json  # 改完配置先校验
yunductl logs -f --since 5m  # 实时看日志
yunductl diag             # 排障时一键收集诊断信息
```

> `yunductl` 是 SSH 终端运维的统一入口；面板零 SSH 闭环之外的"应急 SSH"操作应优先使用 `yunductl` 而非裸 systemctl/journalctl。

---

## 10. 注意事项与避坑指南

### 10.1 安装阶段避坑

| 坑 | 后果 | 正确做法 |
|----|------|---------|
| install.sh 不传 `--runtime=sing-box` | Agent 以 xray 为主内核，sing-box 节点无法加载 | 必须显式 `--runtime=sing-box` |
| 数据库密码用默认值 | 安全风险 | docker-compose 中改强密码，.env 同步 |
| 迁移未执行就启动服务 | 服务报错 | 先 `./migrate up` 再启动 |
| sub 域名 nginx vhost 缺失 | 订阅请求落 default server → 返回 HTML → 客户端"无更新服务器" | 必须配置 sub.example.com.conf |
| 微服务端口对公网暴露 | 安全风险 | iptables 拦截 8081-8084，仅 127.0.0.1 访问 |
| PG/Redis/NATS 端口对公网暴露 | 安全风险 | docker-compose 绑定 127.0.0.1 |

### 10.2 编译阶段避坑

| 坑 | 后果 | 正确做法 |
|----|------|---------|
| node-agent 缺 `with_utls` | REALITY inbound 启动失败 | 完整 6 标签 |
| node-agent 缺 `with_gvisor` | WireGuard 启动失败 | 完整 6 标签 |
| 给纯 Go 服务加 tags | 无害但无必要 | 仅 node-agent 需要 tags |
| SCP 上传后不 chmod +x | systemd 报 status=203/EXEC | `chmod +x /opt/yundu/bin/xxx` |
| 运行中文件直接 cp | Text file busy | 先 `systemctl stop` 再 cp |

### 10.3 节点配置避坑

| 坑 | 后果 | 正确做法 |
|----|------|---------|
| XHTTP mode 用 `auto` | 连接不稳定 | CDN 用 `packet-up`，直连用 `stream-up` |
| CF Dashboard 开 HTTP/2 to Origin | 与 XHTTP packet-up 冲突 | 关闭 |
| 隧道 hostname 用二级子域名 | CF 边缘 SSL 握手失败 | 用一级子域名 |
| 隧道节点 listen `0.0.0.0` | 攻击者绕过 CF 直连明文端口 | listen `127.0.0.1` |
| cloudflared 用 Token 模式 | Agent 每 30s 终止进程 | 用 Config 模式 |
| cloudflared service 用 localhost | IPv6 解析优先，连接被拒 | 用 `127.0.0.1` |
| WARP 设 `route.final=warp-pool` | 全 VPS 节点都走 WARP | `route.final=null` + 精确 inbound 路由 |
| exposure_mode 手动改 `nodes` 列 | 双源不同步 | 只改 `config_json.exposure_mode` |

### 10.4 运营阶段避坑

| 坑 | 后果 | 正确做法 |
|----|------|---------|
| 版本号用 `<` 比较 | Agent 版本高于面板时不下发 | 代码已改 `!=`，勿回退 |
| 自签证书用 `allowInsecure` | xray 26.x 废弃此字段 | 用 `insecure: true` |
| karing/hiddify UA 误判 | 只显示部分节点 | detector.go 中 karing 识别在 clash 之前（已修复） |
| 纯中文套餐名 | slugify 为空，撞唯一约束 | 系统自动回退 `plan-<时间戳>` |
| tc filter 重复添加 | 累积 8000+ 条规则 | 先删后加幂等（已修复） |
| 磁盘满（>90%） | 服务异常 | 定期清理 /tmp + journal + 备份 |

### 10.5 安全避坑

- **8445 端口**：无需在云安全组放行，防止公网绕过 CF 直连 origin
- **5433/6380/4223 端口**：PG/Redis/NATS，绝不对公网暴露
- **8081-8090/9082 端口**：微服务内部端口，iptables 拦截公网访问
- **iptables 拦截 8081-8090 时**：必须先插入 ACCEPT for Docker 网段（172.16.0.0/12），否则 Prometheus 容器无法抓取宿主机 /metrics

### 10.6 v0.8.2 新机制避坑（v1.1 新增）

| 关注点 | 说明 |
|--------|------|
| **xray 限速 enforcement loop** | 后台循环每 **3 秒** 检查一次 per-user 流量，超速用户会被 `AlterInbound` 移除（断开），下个周期允许重连。这是"硬断开"而非"软限速"，用户感知为短暂掉线后自动恢复。 |
| **xray IP 限制** | `DeviceEnforcer` 现在同时检查 device 上限与 IP 上限，对 xray 和 sing-box 节点均生效。超 IP 数的 inbound tag 会被移除。 |
| **xray fullReload 连接 drain** | xray 内核 `fullReload`（结构变更触发）在重启前会**等待 5 秒**让现有连接 drain，降低瞬断影响。规划维护时仍建议避开业务高峰。 |
| **双核心跳上报** | sing-box 与 xray 双内核状态都上报到面板。**面板 node-service 必须支持 `secondary_kernel` 字段（v0.8.2+）**，否则 xray 状态会被忽略。升级 Agent 到 v0.8.2 前请先升级面板。 |
| **Standalone 模式限制** | Standalone 模式**不支持面板功能**（用户管理 / 流量统计 / 订阅下发 / 在线设备踢人 / 自升级），仅适合单节点自用。配置变更需手动编辑 `/etc/yundu/standalone.json` 后重启 agent。 |
| **Docker 部署卷挂载** | Docker 部署**必须挂载 `/etc/yundu` 与 `/var/log/yundu` 卷**到宿主机持久化目录，否则容器重建后配置与日志丢失。生产环境建议同时挂载 `/opt/yundu/data`（数据库/证书缓存）。 |
| **K8s 部署前置** | K8s Helm Chart 假设集群已存在 StorageClass（PVC 持久化 PG/Redis 数据）；若使用云厂商 CSI，请在 `values.yaml` 中指定 `storageClassName`。 |
| **yunductl 路径** | `yunductl` 自动安装到 `/usr/local/bin/yunductl`；若 PATH 不含该路径，请手动创建符号链接或调整 PATH。旧版升级到 v0.8.2 后需重新执行 install.sh 才会部署 yunductl。 |

---

## 11. 日常运营 SOP（零 SSH）

### 11.1 新增节点

```
1. 面板 → 节点管理 → 添加节点
2. 选服务器 / 协议 / 传输 / 安全 / exposure_mode / SNI
3. 保存并同步
4. 等待 dispatch_status → applied
5. 节点列表确认"在线"
```

### 11.2 修改节点配置

```
1. 面板 → 节点列表 → 编辑
2. 修改参数 → 保存
3. 自动生成新 config_version → 推送
4. HotDiff 自动应用（用户变更热重载，不中断）
```

### 11.3 Agent 升级

```
1. 编译新版本 node-agent → 放到 /opt/yundu/agent-upgrade/
2. 计算 sha256sum
3. 面板 → 系统设置 → Agent版本管理 → 填写 version/url/sha256
4. 发布 → 全部节点自动升级
5. 面板 → 节点列表 → 确认版本号已更新
```

### 11.4 证书管理

```
- 自动续期：15 天自动续期，原子热替换（无需操作）
- 手动申请：面板 → 证书管理 → ACME申请 → 填域名
- 自签证书：面板 → 证书管理 → 自签 → 选域名
```

### 11.5 用户管理

```
- 查看用户：面板 → 用户列表
- 封禁/解封：用户详情 → 状态切换
- 重置流量：用户详情 → 重置流量
- 手动加流量：用户详情 → 调整额度
```

### 11.6 套餐管理

```
- 创建套餐：面板 → 套餐管理 → 添加
- 关联节点分组：套餐 → 节点分组（多选）
- 修改价格：套餐 → 编辑
```

### 11.7 订单与支付

```
- 查看订单：面板 → 订单列表
- 手动激活：订单详情 → 手动激活
- 支付配置：系统设置 → 支付配置
```

### 11.8 监控巡检

```
- Grafana：https://panel.example.com/grafana/
- 关键指标：节点在线率 / 磁盘使用率 / config_versions 增量 / 证书过期
- 日志关键字：error / panic / fatal → 立即排查
```

### 11.9 yunductl 日常运维命令（v1.1 新增）

面板零 SSH 闭环之外的应急 SSH 操作，应优先使用 `yunductl`（已部署到 `/usr/local/bin/yunductl`）。

```bash
# 查看节点版本
yunductl version
# 健康检查（agent + 内核 + nginx + 证书）
yunductl health
# 验证本地配置（改完 standalone.json 先校验，避免重启失败）
yunductl config validate /etc/yundu/standalone.json
# 查看面板服务器列表（需已 yunductl bind）
yunductl server list
# 查看面板服务器状态
yunductl server status
```

**典型 SOP 场景**：

| 场景 | 操作 |
|------|------|
| 节点疑似异常 | `yunductl health` → `yunductl logs -f --since 5m` → `yunductl diag` 收集诊断 |
| 配置改完未生效 | `yunductl config validate` → `yunductl refresh` → `yunductl status` |
| 配置改坏 | `yunductl rollback` 回滚到 LKG → `yunductl status` 确认 |
| 升级后异常 | `yunductl version` 确认版本 → `yunductl diag` 上报 → 必要时 `yunductl upgrade --version=v0.8.2` 回退 |
| 新机绑定面板 | `yunductl bind --token=xxx --endpoint=https://panel.example.com` → `yunductl status` |

> 排障时优先用 `yunductl diag`：它会一次性收集 agent 日志、内核配置、端口监听、证书状态、系统资源快照，便于回传给运维或上游支持。

---

## 12. 标准化运营检查清单

### 12.1 首次部署验收清单

- [ ] 控制面 VPS：Docker 三容器（PG/Redis/NATS）healthy
- [ ] 控制面 VPS：5 个微服务 systemd active
- [ ] 数据库迁移执行到版本 77
- [ ] .env 配置完整（JWT/DB/Redis/CORS/ACME）
- [ ] nginx 反代配置正确（panel/user/sub 三域名）
- [ ] SSL 证书已申请并部署
- [ ] admin-web / user-web 前端已部署
- [ ] 面板可登录，管理员账号已创建
- [ ] 每台节点 VPS：Agent 已安装，`--runtime=sing-box`
- [ ] 每台节点 VPS：Agent 服务 active，面板显示在线
- [ ] 每台节点 VPS：ufw 已放行直连端口段
- [ ] 节点配置已添加并 dispatch_status=applied
- [ ] 套餐 + 节点分组已配置
- [ ] 支付方式已配置（按需）
- [ ] 订阅域名 vhost 正确（sub.example.com → 8083）
- [ ] 数据库定时备份已配置
- [ ] 安全加固：8081-8090/9082 iptables 拦截公网

### 12.2 日常巡检清单（每周）

- [ ] 所有节点在线（面板 → 节点列表）
- [ ] 磁盘使用率 < 85%（`df -h`）
- [ ] 内存使用率 < 90%（`free -h`）
- [ ] config_versions 10 分钟增量 < 10
- [ ] 证书有效期 > 7 天
- [ ] Agent 版本一致（面板 → 节点列表）
- [ ] 数据库备份正常（检查 /opt/yundu/backup/）
- [ ] 无 error/panic/fatal 日志

### 12.3 标准化 VPS 状态（每台节点 VPS 应满足）

```
✅ /opt/yundu/bin/node-agent 存在且可执行
✅ /etc/yundu/node-agent.env 中 RUNTIME_TYPE=sing-box
✅ /etc/yundu/config/sing-box.json 存在（Agent 自动生成）
✅ /etc/yundu/nginx/ 目录存在（Agent 自动生成 nginx 配置）
✅ /etc/yundu/certs/ 目录存在（证书自动下发）
✅ /etc/cloudflared/ 存在（仅隧道节点）
✅ systemctl status yundu-node-agent → active
✅ systemctl status nginx → active
✅ 端口监听：443(nginx) + 10000(agent) + 节点端口段
✅ ufw 规则：30000-30200/tcp + 40020-40200/udp
✅ 系统 cloudflared.service 已 mask
✅ Agent 版本与面板一致
✅ dispatch_status = applied
```

---

## 13. 高级部署模式

本章描述 v0.8.2 引入的三种非标准部署模式与两个 admin-web 高级页面。除 §13.1 Standalone 外，§13.2/§13.3 容器化部署可与面板+节点模式并存（例如面板跑容器、节点跑 systemd）。

### 13.1 Standalone 模式详解

**适用场景**：
- 个人自用代理（1 台 VPS，1 套节点配置）
- 临时部署 / 测试环境（无需面板依赖）
- 边缘节点（无面板可达，仅本地配置）

**配置文件格式**（`/etc/yundu/standalone.json`）：

```jsonc
{
  "runtime_type": "sing-box",        // 主内核：sing-box 或 xray
  "node_name": "my-personal-node",
  "listen_addr": "0.0.0.0",
  "nodes": [
    {
      "code": "hk-vless",
      "protocol": "vless",
      "transport": "ws",
      "security": "tls",
      "exposure_mode": "cdn",
      "port": 30001,
      "sni": "hk.node.example.com",
      "users": [
        { "uuid": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx", "name": "me" }
      ]
    }
  ],
  "cert": {
    "mode": "self",                  // self / file / content
    "path": "/etc/yundu/certs/server.crt"
  },
  "nginx": {
    "enabled": true,
    "http_port": 80,
    "https_port": 8445
  },
  "log_level": "info"
}
```

**安装与启动**：

```bash
# 1. 安装 node-agent 二进制（同 §4.2 但不传 --token/--endpoint）
curl -fsSL https://github.com/energie2008/yundu/raw/main/install.sh | bash -s -- agent --standalone

# 2. 生成模板配置
node-agent --init-standalone --standalone-config=/etc/yundu/standalone.json

# 3. 编辑配置（填入节点 / 用户 / 证书）
vi /etc/yundu/standalone.json

# 4. 校验配置
yunductl config validate /etc/yundu/standalone.json

# 5. 启动（systemd 服务 yundu-node-agent 会读取 standalone 标志）
systemctl start yundu-node-agent
# 或前台调试：
node-agent --standalone --standalone-config=/etc/yundu/standalone.json
```

**限制清单**（Standalone 模式不支持）：
- ❌ 用户管理（无面板用户库）
- ❌ 流量统计与配额（无 traffic-service）
- ❌ 订阅下发（无 subscription-service）
- ❌ 在线设备 / IP 踢人（无 DeviceEnforcer 远端联动）
- ❌ Agent 自升级（无面板版本管理）
- ❌ 证书 ACME 自动签发（仅支持 self / file / content 三种本地模式）
- ❌ 多节点统一管理（每台 VPS 独立配置）
- ✅ 双内核（sing-box + xray）仍可用
- ✅ nginx 骨架自动生成
- ✅ HotDiff 配置热更（仅本地配置触发）

> Standalone 配置变更后，需 `yunductl config validate` 校验 → `systemctl restart yundu-node-agent` 或 `yunductl refresh` 应用。

### 13.2 Docker 部署详解

**Dockerfile 结构**（多阶段构建）：

```dockerfile
# 阶段 1：构建前端
FROM node:20-alpine AS web-builder
WORKDIR /web
COPY web/ .
RUN npm ci && npm run build

# 阶段 2：构建 Go 二进制（带 tags）
FROM golang:1.22-alpine AS go-builder
WORKDIR /src
COPY . .
RUN apk add --no-cache git make
# node-agent 需要 6 个 tags
RUN cd apps/node-agent && \
    CGO_ENABLED=0 go build -tags "with_utls,with_wireguard,with_gvisor,with_quic,with_grpc,with_clash_api" \
    -ldflags "-s -w -X main.AgentVersion=v0.8.2" -o /out/node-agent ./cmd/agent
# 其他微服务（无 tags）
RUN for svc in api-gateway identity-service node-service subscription-service traffic-service; do \
      cd apps/$svc && CGO_ENABLED=0 go build -o /out/$svc ./cmd/api && cd ../..; \
    done

# 阶段 3：运行时镜像
FROM debian:12-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates nginx ufw jq curl && rm -rf /var/lib/apt/lists/*
COPY --from=go-builder /out/ /opt/yundu/bin/
COPY --from=web-builder /web/admin-web/dist /var/www/admin-web
COPY --from=web-builder /web/user-web/dist /var/www/user-web
WORKDIR /opt/yundu
ENTRYPOINT ["/opt/yundu/bin/entrypoint.sh"]
CMD ["agent"]
```

**环境变量**（容器化部署关键变量）：

| 变量 | 默认 | 说明 |
|------|------|------|
| `YUNDU_MODE` | `agent` | 运行模式：`panel` / `agent` / `standalone` |
| `YUNDU_RUNTIME` | `sing-box` | 主内核：`sing-box` / `xray` |
| `YUNDU_AGENT_TOKEN` | - | Agent 模式下的面板 token |
| `YUNDU_ENDPOINT` | - | 面板 endpoint URL |
| `YUNDU_STANDALONE_CONFIG` | `/etc/yundu/standalone.json` | Standalone 配置路径 |
| `DATABASE_URL` | - | 面板模式 PG 连接串 |
| `REDIS_ADDR` | - | 面板模式 Redis 地址 |
| `NATS_URL` | - | 面板模式 NATS 地址 |

**卷挂载清单**：

| 容器路径 | 用途 | 必需 |
|---------|------|------|
| `/etc/yundu` | 配置文件 / 证书 / nginx 骨架 | ✅ |
| `/var/log/yundu` | agent 与内核日志 | ✅ |
| `/opt/yundu/data` | 数据库 / Redis / NATS 数据（面板模式） | 面板模式必需 |
| `/var/www` | 前端静态文件（一般镜像内置，可不挂） | 可选 |

**docker-compose 示例**：

```yaml
services:
  yundu-panel:
    image: yundu:0.8.2
    container_name: yundu-panel
    restart: unless-stopped
    command: ["panel"]
    env_file: /etc/yundu/panel.env
    ports:
      - "8090:8090"   # api-gateway
      - "8081-8084:8081-8084"  # 微服务
    volumes:
      - /etc/yundu:/etc/yundu
      - /var/log/yundu:/var/log/yundu
      - /opt/yundu/data:/opt/yundu/data
    depends_on:
      - postgres
      - redis
      - nats

  postgres:
    image: postgres:16-alpine
    # ... 同 §3.1

  yundu-agent:
    image: yundu:0.8.2
    container_name: yundu-agent
    restart: unless-stopped
    command: ["agent", "--token=${TOKEN}", "--endpoint=${ENDPOINT}", "--runtime=sing-box"]
    network_mode: host
    cap_add:
      - NET_ADMIN
    volumes:
      - /etc/yundu:/etc/yundu
      - /var/log/yundu:/var/log/yundu
```

> **关键约束**：Agent 容器必须 `network_mode: host`（端口监听 + ufw），且需要 `NET_ADMIN` capability。Panel 容器可与基础设施容器同 compose 编排。

### 13.3 Kubernetes 部署详解

**Helm Chart 结构**（`deploy/k8s/helm/`，16 个模板文件）：

```
deploy/k8s/helm/
├── Chart.yaml                 # Chart 元信息
├── values.yaml                # 默认 values
├── values-production.yaml     # 生产环境推荐 values
└── templates/
    ├── _helpers.tpl           # 模板辅助函数
    ├── namespace.yaml         # Namespace
    ├── configmap-panel.yaml   # 面板配置 ConfigMap
    ├── secret-panel.yaml      # 面板密钥 Secret
    ├── deployment-api-gateway.yaml
    ├── deployment-identity.yaml
    ├── deployment-node-service.yaml
    ├── deployment-subscription.yaml
    ├── deployment-traffic.yaml
    ├── service-panel.yaml     # 面板 Service
    ├── ingress-panel.yaml     # Ingress（TLS）
    ├── hpa.yaml               # 水平自动扩缩
    ├── pdb.yaml               # PodDisruptionBudget
    ├── pvc-data.yaml          # 持久化卷
    └── networkpolicy.yaml     # 网络策略
```

**values.yaml 关键配置**：

```yaml
# 镜像
image:
  repository: yundu
  tag: "0.8.2"
  pullPolicy: IfNotPresent

# 命名空间
namespace: yundu

# 面板微服务
panel:
  replicas: 2
  resources:
    requests: { cpu: 200m, memory: 256Mi }
    limits: { cpu: 1000m, memory: 1Gi }
  hpa:
    enabled: true
    minReplicas: 2
    maxReplicas: 6
    cpuTarget: 70

# 持久化
persistence:
  enabled: true
  storageClassName: "standard-rwo"   # 云厂商 CSI
  size: 40Gi

# Ingress
ingress:
  enabled: true
  className: "nginx"
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
  hosts:
    - host: panel.example.com
      paths: ["/"]
  tls:
    - secretName: panel-tls
      hosts: [panel.example.com]

# 数据库（生产建议用云厂商托管 PG，不跑容器）
postgres:
  enabled: true              # false 时使用外部 DB
  external:
    host: ""
    port: 5432
```

**values-production.yaml 推荐**（生产环境覆盖项）：

```yaml
panel:
  replicas: 3
  hpa:
    maxReplicas: 10
    cpuTarget: 60
persistence:
  storageClassName: "premium-rwo"
  size: 100Gi
postgres:
  enabled: false             # 使用云托管 PG
  external:
    host: pg-prod.internal
    port: 5432
ingress:
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
    nginx.ingress.kubernetes.io/proxy-body-size: 50m
```

**部署与运维命令**：

```bash
# 部署
helm install yundu ./deploy/k8s/helm \
  -f deploy/k8s/helm/values-production.yaml \
  -n yundu --create-namespace

# 查看状态
kubectl -n yundu get pods,svc,ingress,hpa

# 查看 Panel 配置
kubectl -n yundu get configmap yundu-panel -o yaml

# 升级
helm upgrade yundu ./deploy/k8s/helm \
  -f deploy/k8s/helm/values-production.yaml -n yundu

# 回滚
helm rollback yundu 1 -n yundu

# 卸载（保留 PVC）
helm uninstall yundu -n yundu
# 彻底清理：
# kubectl -n yundu delete pvc --all
```

> **生产建议**：数据库（PG/Redis/NATS）使用云厂商托管服务，Helm Chart 仅部署无状态微服务 + Ingress。这样滚动升级不涉及数据迁移风险。

### 13.4 编译器工作台（admin-web `/compiler-workbench`）

**入口**：管理面板左侧菜单 → 编译器工作台，或直接访问 `https://panel.example.com/compiler-workbench`。

**5 栏可视化布局**：

```
┌─────────────┬──────────────┬──────────────┬──────────────┬──────────────┐
│  NodeSpec   │  IR 中间表示  │  sing-box    │  xray        │  Dry-run     │
│  (语义输入)  │  (规范化)     │  JSON 输出    │  JSON 输出    │  校验结果     │
│             │              │              │              │              │
│ - 协议       │ - inbound    │ - inbounds   │ - inbounds   │ - L1 语法    │
│ - 传输       │ - outbound   │ - outbounds  │ - outbounds  │ - L2 端口    │
│ - 安全       │ - routing    │ - route      │ - routing    │ - L3 路由    │
│ - exposure  │ - tls        │ - tls        │ - tls        │ - L4 一致性  │
│ - 用户       │ - users      │ - users      │ - users      │              │
└─────────────┴──────────────┴──────────────┴──────────────┴──────────────┘
```

**功能用途**：
1. **NodeSpec 栏**：以表单/JSON 编辑节点语义配置（协议/传输/安全/exposure/用户）
2. **IR 中间表示栏**：实时展示 NodeSpec 经 IR 编译器规范化后的中间结构（inbound/outbound/routing/tls/users）
3. **sing-box JSON 栏**：IR 渲染为 sing-box 配置（含 ruleset、outbound pool、WARP 路由）
4. **xray JSON 栏**：IR 渲染为 xray 配置（含 XHTTP、REALITY、AlterInbound 用户管理）
5. **Dry-run 栏**：执行 L1-L4 preflight 校验，输出每项检查结果（端口冲突 / 路由环路 / TLS 一致性 / 双内核字段对齐）

**使用场景**：
- 调试新协议 / 新传输组合（无需真的下发到节点）
- 验证 NodeSpec 改动是否会引起 L1-L4 校验失败
- 对比 sing-box 与 xray 双内核渲染差异（如 REALITY 字段、XHTTP mode）
- 复制渲染后的 JSON 直接粘贴到本地 sing-box/xray 调试

> **关键价值**：dry-run 校验在面板侧拦截无效配置，避免下发到节点后内核启动失败。所有节点配置变更前建议先在工作台 dry-run。

### 13.5 部署状态大盘（admin-web `/deployment-dashboard`）

**入口**：管理面板左侧菜单 → 部署状态大盘，或直接访问 `https://panel.example.com/deployment-dashboard`。

**实时监控内容**：

| 维度 | 指标 |
|------|------|
| 节点在线率 | 在线/离线/总数、按地区分组 |
| Agent 版本分布 | 各版本节点数（用于判断升级收敛度） |
| dispatch_status | applied / pushed / failed 分布 |
| 配置版本增量 | 最近 1h / 24h config_versions 增量曲线 |
| 证书状态 | 即将过期（<7天）证书列表 |
| 双内核状态 | sing-box 主内核 + xray 辅内核运行状态 |
| Enforcement 告警 | xray 限速踢人事件 / IP 超限事件（最近 1h） |
| 资源水位 | 节点磁盘 / 内存使用率（agent 上报） |

**典型使用场景**：
- **升级收敛监控**：Agent 自升级发布后，大盘实时显示各版本节点数变化，确认收敛到目标版本
- **配置推送异常排查**：dispatch_status=failed 节点会标红，点击查看失败原因（L1-L4 校验错误 / agent 不可达 / applyConfig 失败）
- **证书续期预警**：<7 天过期的证书会高亮，配合 §11.4 证书管理手动续期
- **xray enforcement 异常**：限速踢人频次过高可能意味着限速阈值设置过低，需调整套餐

> 大盘数据通过 NATS 实时推送（agent 心跳 / ConfigResult / enforcement 事件），刷新延迟 < 10s。Grafana 适合长期趋势分析，大盘适合实时运营态势感知。

---

> **配套文档**: 遇到故障时，查阅《运营维护指南-v2.0.md》：
> - §10 故障排查手册
> - §11 常见问题症状与解决思路（21 类问题）
> - §15 附录（文件路径 / 数据库表 / 编译命令 / 协议速查 / 修复历史）
>
> **文档维护**: 本文档应随系统版本更新而更新。部署环境变化时请同步修正。
> **编写日期**: 2026-08-07
> **编写依据**: 源码 install.sh / docker-compose.vps190.yml / .env.example / release.yml / agent_bootstrap_handler.go / acme.go + 《运营维护指南-v2.0.md》
