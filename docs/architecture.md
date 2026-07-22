# 云渡 YunDu 项目架构文档

> 最后更新：2026-07-01
> 状态：生产运行 XBoard + 本地开发 YunDu 前端/网关

## 一、架构总览

云渡机场采用**双轨架构**：
- **生产环境（VPS）**：运行 XBoard（Docker容器化）作为成熟稳定的后端面板，承载实际用户、套餐、节点、订阅逻辑
- **本地开发环境**：YunDu Node.js 网关 + React 前端（admin-web / user-web）通过 SSH 隧道代理到 XBoard API，用于新功能开发和UI迭代
- **未来架构**：Go 微服务集群（api-gateway / identity-service / node-service / subscription-service / traffic-service / node-agent）正在 `apps/` 目录中开发，目标是逐步替换 XBoard

```
                         ┌─────────────────────────────────────────────────┐
                         │              用户浏览器                           │
                         └───────────┬─────────────────────┬───────────────┘
                                     │                     │
                    ┌────────────────▼──────┐   ┌──────────▼──────────────┐
                    │  user-web :5178       │   │  admin-web :5177         │
                    │  (React + Vite)       │   │  (React + Vite)          │
                    │  用户面板/注册/订阅    │   │  管理面板/节点/诊断      │
                    └───────────┬───────────┘   └──────────┬───────────────┘
                                │                          │
                                └────────────┬─────────────┘
                                             │
                                  ┌──────────▼───────────┐
                                  │  YunDu API Gateway   │
                                  │  (Express.js :8080)  │
                                  │  apps/server/src/    │
                                  │                      │
                                  │  /api/v1/yundu/*     │──→ AI(DeepSeek),
                                  │     (自定义增强API)  │    Dashboard聚合
                                  │  /api/v1/admin/*     │──→ /api/v2/adminifanr520/*
                                  │  /api/v1/*, /sub/*   │──→ XBoard直接代理
                                  └──────────┬───────────┘
                                             │ SSH Tunnel (本地)
                                    ┌────────▼─────────┐
                                    │  127.0.0.1:7001  │
                                    └────────┬─────────┘
                                             │
                          ┌──────────────────▼─────────────────────┐
                          │         VPS (43.135.147.190)           │
                          │                                        │
                          │  ┌────────────────────────────────┐    │
                          │  │  Caddy (:7001)                 │    │
                          │  │  XBoard PHP-FPM (Octane :7002) │    │
                          │  │  Docker Container: xboard      │    │
                          │  └────────────┬───────────────────┘    │
                          │               │                        │
                          │  ┌────────────▼───────────────────┐    │
                          │  │  MariaDB (xboard-mariadb)      │    │
                          │  │  Redis (内建)                  │    │
                          │  └────────────┬───────────────────┘    │
                          │               │                        │
                          │  ┌────────────▼───────────────────┐    │
                          │  │  xboard-node (节点通信agent)    │    │
                          │  └────────────┬───────────────────┘    │
                          │               │                        │
                          │  ┌────────────▼───────────────────┐    │
                          │  │  xboard-nginx (对外:80/443)     │    │
                          │  │  https://99.xinti.na.am/       │    │
                          │  │    /            → 用户前台     │    │
                          │  │    /adminifanr520 → 管理后台   │    │
                          │  │    /s/          → 订阅链接     │    │
                          │  └────────────────────────────────┘    │
                          │                                        │
                          │  附加服务：                            │
                          │  - nanoroute (:30128)                  │
                          │  - 9router (:20128)                    │
                          │  - subconverter (:25500)               │
                          │  - warp_socks (:9091)                  │
                          └────────────────────────────────────────┘
```

## 二、VPS 生产环境

### 2.1 域名与路由策略

单域名 `99.xinti.na.am`，使用路径区分前台/后台/订阅，无需多端口：

| 路径 | 用途 | 后端处理 |
|------|------|----------|
| `/` | 用户前台（登录/注册/购买/仪表盘/订阅） | XBoard 用户端 |
| `/adminifanr520` | 管理员后台（节点/用户/订单/配置） | XBoard 管理端（secure_path可配置） |
| `/s/{token}` | 订阅链接（客户端拉取节点配置） | XBoard 订阅服务 |
| `/link/{token}` | 单节点分享链接 | XBoard |
| `/api/v1/*` | 用户/公开 API | XBoard |
| `/api/v2/adminifanr520/*` | 管理 API | XBoard 管理端 |

### 2.2 Docker 服务列表

| 容器名 | 镜像 | 端口 | 作用 |
|--------|------|------|------|
| xboard | ghcr.io/cedar2025/xboard:latest | 内部7001/7002 | XBoard主应用（Caddy+PHP Octane） |
| xboard-mariadb | mariadb:10.11 | 内部3307 | 数据库 |
| xboard-nginx | nginx:alpine | 80/443 | 反向代理+HTTPS |
| xboard-node | ghcr.io/cedar2025/xboard-node:latest | - | 节点通信agent |
| xboard-subconverter | tindy2013/subconverter | 25500 | 订阅格式转换（Clash/Shadowrocket等） |
| nanoroute | ghcr.io/energie2008/nanoroute | 30128 | 路由/隧道服务 |
| 9router | decolua/9router:0.5.12 | 20128 | 智能路由 |
| warp_socks | mon-ius/docker-warp-socks | 9091 | Cloudflare WARP SOCKS代理 |

### 2.3 数据库核心表（XBoard v2_settings）

| 配置项 | 当前值 | 说明 |
|--------|--------|------|
| stop_register | 0 | 开放注册 |
| register_mode | 0 | 开放注册模式（无需邀请码） |
| try_out_plan_id | 4 | 试用套餐ID |
| try_out_hour | 24 | 试用时长（小时） |
| email_verify | 0 | 关闭邮箱验证（方便测试） |
| captcha_enable | 0 | 关闭验证码（方便测试） |
| secure_path | adminifanr520 | 管理后台路径 |

### 2.4 节点配置概览

当前7个VLESS节点 + 1个VMESS节点，全部关联到group_id=1（vip1会员组）：

| ID | 名称 | 协议 | 地址 | 端口 | TLS/传输 |
|----|------|------|------|------|----------|
| 320 | US3-02w 美国轻量节点 | VLESS | node.tiktokplay.na.am | 40014 | REALITY/TCP, SNI=rust-lang.org |
| 300 | US01-05 AI推荐 | VLESS | cdn.dannelblog.na.am | 54551 | REALITY/TCP |
| 352 | aargo | VLESS | 162.159.160.46 | 443 | TLS/WS, Cloudflare Argo |
| 353 | gogogo2 | VLESS | 162.159.160.46 | 443 | TLS/WS |
| 358 | warp | VLESS | 162.159.160.46 | 443 | TLS/WS (Chrome指纹) |
| 361 | one-saas | VLESS | 162.159.160.46 | 443 | TLS/WS (Safari指纹) |
| 362 | cf-dance | VLESS | 162.159.160.46 | 443 | TLS/WS (iOS指纹) |
| 359 | tiktok | VMESS | 162.159.160.46 | 443 | TLS/WS |

### 2.5 套餐配置

| ID | 名称 | 流量 | 价格 | 类型 |
|----|------|------|------|------|
| 1 | 轻量-66G-月付 | 60GB/月 | ¥6/月 | 付费月付 |
| 2 | 轻量-156G-月付 | 156GB/月 | ¥14/月 | 付费月付 |
| 3 | 轻量-80G-不限时 | 80GB不限时 | ¥18一次性 | 付费一次性 |
| 4 | Trial-5GB-1Day | 5GB | ¥0 | **免费试用**（24小时，2设备） |

## 三、本地开发环境

### 3.1 服务组件

| 服务 | 端口 | 启动目录 | 说明 |
|------|------|----------|------|
| YunDu API Gateway | 8080 | apps/server/ | Express.js 代理服务器 |
| admin-web | 5177 | apps/admin-web/ | React 管理面板 |
| user-web | 5178 | apps/user-web/ | React 用户面板 |
| SSH 隧道 | 7001 | - | `ssh -L 127.0.0.1:7001:127.0.0.1:7001` |

### 3.2 启动顺序

```bash
# 1. 建立 SSH 隧道到 VPS（必须先于其他服务）
ssh -fN -i ~/190key.pem -L 127.0.0.1:7001:127.0.0.1:7001 root@43.135.147.190

# 2. 启动 YunDu 网关
cd apps/server && npm start

# 3. 启动用户端
cd apps/user-web && npm run dev

# 4. 启动管理端
cd apps/admin-web && npm run dev
```

### 3.3 API 代理路径映射

YunDu 网关（[index.js](file:///d:/机场搭建/air/apps/server/src/index.js)）的代理规则：

```
/api/v1/yundu/admin/login  → 直接处理（管理员登录获取token）
/api/v1/yundu/*            → YunDu自定义API（AI聊天/诊断/仪表盘聚合）
/api/v1/admin/*            → 代理到 XBOARD_URL/api/v2/adminifanr520/*（管理API路径重写）
/api/v1/*, /sub/*, /link/* → 直接代理到 XBoard（透传）
```

### 3.4 Vite 开发服务器配置

两个前端都配置了代理到 YunDu 网关（8080）：
- [vite.config.ts (admin-web)](file:///d:/机场搭建/air/apps/admin-web/vite.config.ts): `host: '127.0.0.1'`, proxy `/api` → `http://127.0.0.1:8080`
- [vite.config.ts (user-web)](file:///d:/机场搭建/air/apps/user-web/vite.config.ts): `host: '127.0.0.1'`, proxy `/api`, `/sub`, `/link` → `http://127.0.0.1:8080`

### 3.5 SSH 连接方式

VPS 禁用密码登录，使用密钥认证：
- 密钥文件：[190key.pem](file:///d:/机场搭建/air/key/190key.pem)
- 用户：root
- 在 WSL 中使用：`ssh -i ~/190key.pem root@43.135.147.190`（需先chmod 600）

## 四、未来架构（Go 微服务）

在 `apps/` 目录下已规划并开始实现的 Go 微服务：

```
apps/
├── api-gateway/        # API网关(Gin) - 统一入口/认证/限流/路由
├── identity-service/   # 身份服务 - 用户/管理员/RBAC/JWT/套餐/订单/工单
├── node-service/       # 节点服务 - 服务器管理/协议配置/健康检查/链式代理
├── subscription-service/ # 订阅服务 - 订阅渲染/客户端兼容/负载均衡
├── traffic-service/    # 流量服务 - 流量统计/额度控制/在线会话
├── node-agent/         # 节点Agent - 部署到VPS执行节点配置
├── admin-web/          # 管理后台(React+TS+Tailwind)
├── user-web/           # 用户面板(React+TS+Tailwind)
└── server/             # Node.js开发网关（临时方案）
```

共享包：
- `packages/config` - DB/Redis/中间件/日志/配置
- `packages/proto` - gRPC protobuf定义（agent通信）
- `packages/subscription` - 订阅渲染/节点规格/规则集
- `packages/ui` - 共享React组件库
- `packages/tsconfig` - TypeScript配置

详见 [ADR-0001](file:///d:/机场搭建/air/docs/adr/0001-system-architecture.md)。

## 五、关键运维操作

### 管理员账号
- 邮箱：a***@example.com
- 密码：admin12345
- 本地访问：http://localhost:5177/
- 生产访问：https://99.xinti.na.am/adminifanr520

### VPS 常用命令
```bash
# 查看容器状态
docker ps

# 进入xboard容器
docker exec -it xboard sh

# 执行Artisan命令（清除缓存等）
docker exec -u www xboard php /www/artisan config:clear
docker exec -u www xboard php /www/artisan cache:clear
docker exec -u www xboard php /www/artisan octane:reload

# 重启xboard
docker restart xboard

# 查看日志
docker logs -f xboard
```

### 数据库直接操作
```bash
# 在容器内执行PHP
docker exec -u www xboard php -r 'require "/www/vendor/autoload.php"; ...'

# 直接连MariaDB
docker exec -it xboard-mariadb mysql -uxboard -pxboard123456 xboard
```

## 六、已完成的试用套餐配置

1. **创建套餐**：Plan ID=4，名称=Trial-5GB-1Day，5GB流量，价格¥0/月，2设备限制，group_id=1
2. **开启注册**：stop_register=0，register_mode=0（无需邀请码）
3. **启用自动试用**：try_out_plan_id=4，try_out_hour=24
4. **关闭验证障碍**：email_verify=0，captcha_enable=0（方便开发测试）
5. **修复节点关联**：所有VLESS节点均在group_id=1中，试用用户可访问全部8个节点
6. **验证通过**：注册测试用户自动获得plan_id=4，订阅链接正常返回8个节点配置
