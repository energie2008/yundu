# 云渡（YunDu）机场系统 — 新一代双内核机场管理面板

> 三平面架构 · 移动端优先 · 协议无关中间层抽象 · 双内核（Xray v26.x / Sing-box v1.13+）一等公民 · 节点稳定性治理 · 零 SSH 全闭环自动化 · 商业生产可用

> 📌 **当前状态单一真相源**：[CURRENT_STATE.md](file:///d:/机场搭建/air/docs/CURRENT_STATE.md)（2026-07-12 阶段 0-10 全项目改造完成后的最新状态汇总）

---

## 项目概述

云渡（YunDu）是一套面向商业生产使用的机场管理系统，不是传统面板（如 XBoard/V2Board）的二次修补，而是从系统设计出发重建，重点解决传统面板的三个结构性痛点：

1. **表单覆盖不全冷门协议参数**（h2/httpupgrade/xhttp/kcp/quic 等）→ 双模式编辑器（结构化表单 + YAML/JSON 原始编辑）双向同步
2. **双内核中转/套娃不工作**（Xray 语法不兼容 Sing-box）→ ChainSpec 协议无关链路抽象，两内核各自原生渲染
3. **分流规则无法统一下发**（XBoard 完全交给客户端）→ RuleSet+NodeGroup 分层设计，全局规则+节点组标签双正交

核心目标：

- 手机网页可完成 80% 后台运维操作
- **零 SSH 全闭环**：面板配置变更通过 gRPC/WS/HTTP 三通道自动下发到 Agent，Agent 自主完成 DryRun → 备份 → 应用 → 健康检查 → 自动回滚，全程无需 SSH 登录节点
- 节点发布、回滚、健康熔断为平台级内建能力
- 协议、传输、安全、中转链路、分流规则全部通过协议无关中间层抽象，两内核各自渲染，新增协议无需修改主干业务逻辑
- 控制平面、执行平面、订阅编排平面清晰解耦

---

## 架构总览

```
控制平面 (Control Plane)                               :8080 统一入口
  api-gateway  ──→ identity-service   (:8081)   认证/RBAC/用户/订单/支付/工单/公告/通知/佣金
              ──→ node-service       (:8082)   节点/健康/发布/校验/中转/分流/URI导入/证书/诊断
              ──→ subscription-service (:8083) 订阅生成/多客户端渲染/负载均衡/UA识别
              ──→ traffic-service    (:8084)   流量统计/在线会话

执行平面 (Execution Plane)
  node-service ←gRPC/WS/HTTP三通道→ node-agent (部署在节点VPS)
  node-agent ←→ xray-core v26.x / sing-box v1.13+ (双内核并列一等公民)
  node-agent → warp sidecar (Cloudflare WARP 出口)

订阅编排平面 (Subscription Plane)
  subscription-service
    → 协议无关中间层 (packages/subscription)
        ├─ nodespec     NodeSpec: 协议+传输+安全+RawSettings透传
        ├─ chain        ChainSpec: 中转链路(LandingNode+Relays)
        └─ ruleset      RuleSet+NodeGroup: 分层分流规则
    → 双内核渲染器
        ├─ ToXrayConfig/ToSingboxConfig     节点配置
        ├─ BuildXrayChain/BuildSingboxChain 中转链路
        └─ BuildXrayRouting/BuildSingboxRoute 分流路由
    → 客户端渲染: Clash Meta / Mihomo / Sing-box / Shadowrocket / V2rayN
                / Surge / Stash / Loon / Quantumult X / URI base64

基础设施
  PostgreSQL 16  ──  主数据存储 (5432)
  Redis 7        ──  缓存/限流/会话 (6379)
  NATS JetStream ──  事件总线 (4222)
```

**服务端口一览**：

| 服务 | 端口 | 说明 |
|------|------|------|
| api-gateway | 8080 | 对外统一入口（生产环境 Caddy/Nginx 反代到 443） |
| identity-service | 8081 | 用户/订单/支付/工单/通知 |
| node-service | 8082 | 节点管理/配置渲染/agent 通信 |
| subscription-service | 8083 | 订阅生成（订阅域名单独指向） |
| traffic-service | 8084 | 流量上报 |
| PostgreSQL | 5432 | 数据库 |
| Redis | 6379 | 缓存 |
| NATS | 4222 | 消息总线 |
| user-web (dev) | 5178 | 用户端 Vite dev server |
| admin-web (dev) | 5173 | 管理端 Vite dev server |

---

## 目录结构

```
/apps
  /admin-web          管理后台 (React+Tailwind, mobile-first)
    └─ src/pages/     Dashboard/Users/Nodes/Plans/Orders/Payments/
                      Coupons/Tickets/Announcements/Notifications/Settings/AuditLogs
                      /MailTemplates/SubscriptionTemplates/RoutePolicies/RouteRuleSets
                      /Servers/Machines/ProtocolRegistry/NodeDoctor/DiagnosticsAI
  /user-web           用户中心 (React+Tailwind, 深色模式强制)
    └─ src/pages/     Landing/Login/Register/Dashboard/Plans/Checkout/
                      Orders/OrderDetail/Subscription/Invite/Tickets/Profile/Docs
                      /Notifications/Announcements/VerifyEmail
  /api-gateway        Go 统一 API 入口 (Gin, RBAC中间件, 限流, 反向代理)
  /identity-service   Go 认证/RBAC/用户/套餐/订单/支付/工单/通知/公告/佣金
    └─ internal/service/
        ├─ payment_service.go   TRC20/ERC20 轮询监听+自动开通
        ├─ user_service.go      注册/登录/JWT/通知偏好
        ├─ ticket_service.go    工单创建/回复/关闭
        ├─ coupon_service.go    优惠券校验/使用
        └─ commission_service.go 邀请返利/结算/提现
  /node-service       Go 节点管理/健康/发布/双内核渲染/URI导入/中转/分流/诊断
  /subscription-service Go 订阅生成/多客户端渲染/负载均衡/UA识别
  /traffic-service    Go 流量统计/在线会话
  /node-agent         Go 节点端 agent (gRPC/WS/HTTP三通道, WARP sidecar)
  /server             Express 静态资源服务 (生产环境用 Caddy 替代)
/packages
  /ui                 共用 React Shadcn/UI 组件 (button/card/dialog/toast/...)
  /proto              gRPC protobuf (node-agent 通信)
  /config             Go 共用配置/DB/Redis/中间件/错误码/响应封装
  /subscription       ⭐ 协议无关中间层（核心差异化能力）
    ├─ nodespec/spec.go     NodeSpec + Transport RawSettings 兜底冷门参数
    ├─ chain/chain.go       ChainSpec (LandingNode+Relays[])
    ├─ ruleset/ruleset.go   RuleSet + NodeGroup 分层分流
    ├─ renderer/            7种客户端订阅渲染(Clash/Sing-box/URI/Surge/Loon/QuanX/...)
    └─ validate.go          统一校验入口
  /tsconfig           TypeScript 基础配置
/docs
  /adr                架构决策记录
  /openapi            OpenAPI 3.1 文档
  /runbooks           运维手册
/deploy
  /docker             Docker Compose 编排 (.env + Caddy + Prometheus + Loki)
  /helm               Kubernetes Helm Chart (预留)
/migrations           goose migration SQL (000001~000057)
/scripts              开发/运维脚本 (dev-start/dev-stop/dev-health/rebuild)
/key                  VPS SSH 密钥 (不提交生产环境)
/tmp-bin              临时脚本/二进制/调试工具
```

---

## 已完成工作（Status）

### 🏗️ 微服务骨架 + 控制平面

- ✅ 5 个 Go 微服务 + 1 个 node-agent，全部以 Linux 静态二进制运行（WSL 生产化编译）
- ✅ JWT 认证、RBAC 三角色（super_admin/admin/viewer）、点分隔权限码（`nodes.read` 等）
- ✅ PostgreSQL + Redis + NATS 三件套集成，goose migration 跑到 000057
- ✅ API Gateway 反向代理 + 统一错误码 + CORS + 限流 + 30s 超时 + 审计日志
- ✅ 公共 `/healthz` `/readyz` `/metrics` 端点

### 💳 支付系统（Phase 10 主体完成）

- ✅ **TRC20-USDT 支付全链路**：`payment_orders` 表 → `StartPolling` 15s 轮询 → TronGrid API 拉取转账 → 金额尾数唯一匹配（±0.01 容错）→ 自动开通/续期套餐
- ✅ **双网络 SDK**：TronGrid（TRC20）+ Etherscan（ERC20）结构体完整封装
- ✅ **支付 URI**：`tron:地址?amount=X.XX&contract=TR7N...` 扫码支付（TokenPocket/imToken 兼容）
- ✅ **优惠券系统**：固定金额/百分比折扣、最低消费、使用次数、有效期；`coupon_usages` 记录表
- ✅ **邀请返利/佣金系统**：20% 返利、3 天结算等待期、`commission_balance`/`commission_total`、`commission_withdrawals` 提现表
- ✅ **提现功能**：支持支付宝 + USDT-TRC20；默认开启（`withdraw_enable=true`，`min_withdraw=10 USDT`）
- ✅ **TRC20 默认地址**：`TLAoiTwPNCtFXpJWmPrvgm8tRpC9ggP42H`，ERC20 已禁用
- ✅ **测试优惠码**：`TEST2OFF`（$2 固定折扣）

### 📡 订阅系统（Phase 9/11 主体完成）

- ✅ `/sub/{token}` 返回真实 base64 编码 URI 列表（实测 17 节点 1732 字节）
- ✅ `subscription-userinfo` 响应头（upload/download/total/expire）
- ✅ URI 渲染覆盖 VLESS+XHTTP、VLESS+WS+TLS、VLESS+REALITY、Trojan、SS、Hysteria2、TUIC、AnyTLS、SOCKS5、HTTP
- ✅ 7 种客户端格式渲染器：Clash Meta/Mihomo、Sing-box、Shadowrocket、V2rayN、Surge、Stash、Loon、Quantumult X
- ✅ UA 自动识别 40+ 主流客户端
- ✅ 订阅 Token 可重置、多 Token 支持、HMAC 鉴权
- ✅ 订阅内容包含正确的协议参数（VLESS+XHTTP 必须带 `type=xhttp`、`path`、`mode=auto`）
- ✅ 订阅响应头完整：`Subscription-Userinfo`、`Profile-Update-Interval`、`Content-Disposition`、`Profile-Title`、`Profile-Web-Page-URL`

### 🖥️ 前端双端

- ✅ **user-web**：Landing/Dashboard/Plans/Checkout/Orders/OrderDetail/Subscription/Invite/Tickets/Profile/Docs/Notifications/Announcements/VerifyEmail 全部连通
- ✅ **admin-web**：Dashboard/Users/Nodes/Servers/Plans/Orders/Payments/Coupons/Tickets/Announcements/Notifications/Settings/AuditLogs/DiagnosticsAI/NodeDoctor/MailTemplates/SubscriptionTemplates 35+ 页面
- ✅ 统一 `@airport/ui` 组件库
- ✅ 深色模式强制、mobile-first、响应式
- ✅ 订阅链接 + 二维码显示（QRCode 组件）
- ✅ 通知铃铛 + 未读计数
- ✅ 工单对话界面（创建/回复/状态）
- ✅ 通知偏好设置（到期前提醒/流量不足提醒/工单回复提醒）
- ✅ 佣金邀请页面 + 提现申请

### 🤖 节点管控差异化能力（核心竞争力，XBoard 不具备）

- ✅ **协议注册中心**：17 种协议组合（P01-P17）YAML 预设定义，覆盖直连/CDN/混合/UDP/叠加层全场景
- ✅ **双内核抽象**：Xray v26.x + Sing-box v1.13.x 同节点双配置渲染（snake_case vs camelCase 字段差异全部踩平）
- ✅ **DualKernelValidator**：4 步校验链路（Enhancement 专项 → 双核渲染 → 真实 dry-run → 语义等价性），adminValidationHandler 已接入
- ✅ **中转链路 ChainSpec**：LandingNode+Relays[] 协议无关抽象、BillAtLanding 计费点
- ✅ **分流规则 RuleSet**：geosite/geoip 全局规则 + NodeGroup 标签池两层正交
- ✅ **gRPC+WebSocket+HTTP 三通道降级**（Channel Manager 优先级 0/1/2，3 次失败 failover，60s 尝试升级）
- ✅ **配置版本 + 灰度发布骨架**：config_versions/deployment_batches/deployment_targets 表结构已建
- ✅ **AI 诊断 aidiag** + Node Doctor 健康探针
- ✅ **TLS ACME 证书** 框架 + TLS 配置档案
- ✅ **WARP sidecar** 抽象（mock/real 模式切换）
- ✅ **双模式编辑器**：表单+YAML 双向同步、TransportSpec.RawSettings 透传冷门参数
- ✅ **Enhancement 面板**：uTLS/ECH/Mux 独立 Tab，ECH 完整配置（enabled/config/priority/enable_dhps）+ uTLS/Mux 状态摘要
- ✅ **download_settings 编辑器**：XHTTP 下行通道折叠面板，支持 CDN/IPv4/IPv6/REALITY/TLS 多种下行配置
- ✅ **URI 批量导入器**：vless/vmess/trojan/ss/hysteria2/tuic 6 种格式解析框架
- ✅ **17 协议生产部署 + 测速验收**：VPS206 (xray 26.3.27, 14 inbound) + VPS190 (xray 6 + sing-box 4)，17/24 节点达标，0 元购闭环验证通过

### 🔧 Xboard 差距修复（P0-P3，2026-07-09 完成）

#### P0 — 核心功能修复

- ✅ **限速/设备限制链路全通**：`modelNodeToNodeSpec` 映射 SpeedLimitMbps/DeviceLimit → 凭证查询 JOIN subscription+plan 获取 per-user 限速 → `_limiter` 元数据注入 per-user 差异化限速 → `renderLimiterMeta` 优先使用 per-user 值回退节点级
- ✅ **Checkout 优惠券真实验证**：前端调用 `POST /api/v1/coupons/validate`，后端 `coupon_validate_handler.go` 调用 `CouponService.ValidateCoupon` 9 步校验，折扣金额从后端返回（不再硬编码 10%）
- ✅ **Node 模型扩展**：新增 `DeviceLimit`、`PaddingScheme`、`RateTimeEnable`、`RateTimeRanges`、`TransferEnableBytes` 字段（migration 000057）

#### P1 — 前端页面补全

- ✅ **user-web 通知中心页面**：分页列表、已读/未读筛选、单条/全部标记已读、未读数量徽章、通知类型图标
- ✅ **user-web 公告列表页面**：置顶优先、详情弹窗、自动标记已读
- ✅ **user-web 邮件验证页面**：URL token 验证、四种状态（loading/success/error/no-token）
- ✅ **admin-web 邮件模板管理页面**：模板列表、编辑 subject/body、测试发送、重载缓存
- ✅ **admin-web 订阅模板管理页面**：模板列表、YAML 编辑器、启用/禁用切换、预览

#### P2 — 功能完善

- ✅ **订阅头补全**：`Profile-Title`、`Profile-Web-Page-URL` 头已添加
- ✅ **节点在线连接数展示**：admin-web Nodes 页面调用 channel health API 获取在线用户数
- ✅ **节点流量限额 UI**：设备数限制、速度限制、流量限额(GB/MB)、AnyTLS padding_scheme 下拉编辑
- ✅ **GET /install.sh 一键安装脚本**：`curl -fsSL https://panel.example.com/api/v1/install.sh | bash -s -- --token=xxx --endpoint=https://panel.example.com`，自动检测架构、下载 agent+内核、生成 systemd unit
- ✅ **Agent 限速器配置通路**：per-user 限速通过 `_limiter` 元数据下发，Agent 解析后初始化 SpeedLimiter/DeviceLimiter

#### P3 — 增强特性

- ✅ **AnyTLS padding_scheme 渲染**：sing-box 内核渲染时注入 `padding_scheme` 字段（max-0 到 max-8）
- ✅ **时间倍率（按时段动态）**：`computeDynamicTrafficRate` 函数支持跨午夜时段匹配，根据当前时间动态计算 TrafficRate
- ✅ **节点级流量限额**：`NodeTrafficQuotaService` 每 5 分钟检查节点累计流量，超限自动禁用 + metadata 标记
- ✅ **SOCKS5/HTTP 代理协议渲染**：xray + sing-box 双内核服务端配置 + 客户端 URI 渲染全链路支持

### 🚀 边缘自治 + 证书链路 + 限速集成（2026-07-10 完成，15 项任务全闭环）

#### P0 — 证书链路修复（渲染器 + 订阅 + Agent）

- ✅ **P0-1 证书注入**：`injectCertFromBundle` 从 cert_bundles 表查询 PEM 证书注入 node.ConfigJSON（支持 cert_bundle_id 精确查询 + SNI 匹配 SAN 回退）
- ✅ **P0-2 flow 注入约束**：REALITY flow 仅在 TCP 传输层注入（XHTTP/WS 等禁止注入，修复 B12 XHTTP+REALITY 连接失败）
- ✅ **P0-3 订阅证书锁定**：直连 IP 节点用 `pinnedPeerCertSha256`（替代 Xray v26.2.4+ 移除的 `allowInsecure`），CDN 域名节点保留 `allowInsecure`
- ✅ **P0-4 多用户渲染**：renderSettings 优先使用 `spec.Clients`（多用户路径），为空时回退 `spec.Credentials`

#### P1 — Agent 补全 + 限速 + Payload 加密

- ✅ **P1-7 限速渲染**：`computeSpeedLevels` 为不同限速值分配独立 xray level，policy.levels 设置 up_mbps/down_mbps + statsUserOnline
- ✅ **P1-8 设备限制执行端**：DeviceEnforcer（gRPC StatsService 查询在线 IP + 超限移除用户）+ device_limiter SyncLocalDevices
- ✅ **P1-9 Payload 加密切换**：Agent `fetchConfigViaPayload` AES-GCM 解密，applyConfig 优先使用加密路径
- ✅ **P1-10 HealthChecker 接入**：applyConfig 已接入 HealthChecker（LKG 回滚 + NACK 上报）
- ✅ **P1-11 AlterInbound 验证**：原生内嵌模式不暴露 gRPC API，使用 `runtimeExec.Reload()` 全量重载
- ✅ **P1-12 TLSMaterials 填充**：`buildTLSMaterials` 在 CreatePayload 时填充 PayloadContent.TLSMaterials
- ✅ **P1-13 downloadSettings 排除**：xray 渲染器排除 downloadSettings（规避 xray 26.3.27 静默失败 bug）

#### P2 — 基础设施 + sing-box 蓝绿

- ✅ **P2-14 exposure 死代码清理**：删除 RenderXrayConfigWithCreds 等 400 行死代码 + 6 个依赖测试
- ✅ **P2-15 sing-box 蓝绿热转**：SingBoxBlueGreen 组件（端口偏移 + iptables/socat/noop PortMapper + 健康检查 + 排水 + 回滚）

#### 节点配置修复

- ✅ **P15 REALITY 密钥对修复**：DB 更新为项目固定密钥对 + 补全 sni/short_id/server_name/fingerprint/alpn 字段
- ✅ **P06 download_settings 修复**：DB 更新 download_settings.enabled=false（渲染器+数据库双重对齐）
- ✅ **三 VPS 代码部署**：VPS190(node-service) + VPS206/VPS81(node-agent) 全部编译部署，服务 active

### 📧 基础通讯

- ✅ MailService 骨架（SMTP 可配置在 `system_settings` 的 `email.smtp` 下）
- ✅ 邮件模板：注册验证、重置密码、支付成功、工单回复通知
- ✅ 邮件模板管理页（admin-web）：列表/编辑/测试发送/重载缓存
- ✅ 订阅模板管理页（admin-web）：列表/YAML编辑/启用切换/预览

### 🔄 零 SSH 全闭环（Phase 0-4 + 全项目改造阶段 0-10，2026-07-12 完成）

> 详细进度见 [CURRENT_STATE.md](file:///d:/机场搭建/air/docs/CURRENT_STATE.md)（最新单一真相源）
>
> 历史进度：[YunDu-开发进度总结-20260711.md](file:///d:/机场搭建/air/docs/YunDu-开发进度总结-20260711.md)（Phase 0-4 交接文档）
#### Phase 0 — 紧急止血（致命 Bug 全部修复）

- ✅ **D1 配置版本状态机**：`GetLatestActiveConfigVersion` 排除 pending 状态，Agent 不再拉取未完成编译的配置
- ✅ **D2/E2 版本回退**：配置应用失败后三层回滚（.bak 恢复 + LKG 回滚 + version.txt 保持旧版本）
- ✅ **D3 启动强制刷新**：Agent 启动后首次心跳立即检查面板最新版本
- ✅ **D4 Publish API SQL**：`BatchPlan` JSON marshal 为 `[]byte`，解决 SQL 类型不匹配
- ✅ **D7/D8 Cloudflared + Devices API**：新增 `cloudflared-tunnels`/`alive-devices`/`binary-spec` 端点
- ✅ **D9 节点自动绑定计划**：`CreateNodeRequest` 新增 `PlanIDs` 字段，创建节点时自动关联 plan
- ✅ **S1/S2/S3 安全漏洞**：移除 REALITY 硬编码私钥/short_id、Trojan 硬编码密码

#### Phase 1 — 渲染器统一（双渲染器分叉消除）

- ✅ **R1 删除 exposure 渲染路径**：`buildRuntimeConfig` 和 `buildChainRuntimeConfig` 强制走 `kernelrender`
- ✅ **R2 CDN 镜像 inbound 自动生成**：CDN 节点自动生成双 inbound（TLS 直连 + 非 TLS 内部）
- ✅ **R3 modelNodeToNodeSpec 字段补全**：补全 Tags/Priority/IsVisible/NodeType/AddressIPv6/ClientPort/ServerPort/Group/Region
- ✅ **R5 TLS certificates 渲染**：从 config_json 读取 cert_pem/key_pem 注入 certificates 字段
- ✅ **R6 flow 注入约束**：flow 为空时不注入默认值
- ✅ **R7 gRPC ALPN**：gRPC 固定 `['h2']`，WS/HTTPUpgrade 为 `['h2','http/1.1']`
- ✅ **R8 Path 唯一性校验**：`CheckPathUnique` 防止同 runtime 下 path 重复
- ✅ **R9 DryRun 真实校验**：执行 `xray -test` / `sing-box check`，不再返回 Stub Valid
- ✅ **R12 配置签名一致**：`injectNginxVhosts`/`injectTrafficQuota` 移到 hash 计算之前

#### Phase 2 — Agent 自治（统一部署流水线）

- ✅ **D11 Pipeline.Run 统一**：`applyConfig` 重构为调用 `Pipeline.Run`，消除双路径。Pipeline.Run 管理完整八阶段事务：lock → precheck → write temp → dry-run → backup → activate → apply → health-check → success/rollback
- ✅ **E8 单次 Reload**：OnRollback 回调中只执行一次 LKG 恢复 + 单次 Reload，消除双重载
- ✅ **E10 Nginx 同步统一**：移除 `syncNginxVhosts` 直接调用，统一走 `NginxReconciler`（30s 独立循环）
- ✅ **E3 主动拨测**：60s 周期拨测 + 配置应用后快速拨测，连续 3 次失败触发 LKG 回滚
- ✅ **E7 UDP 端口管理**：`ExtractPortsFromConfig` 支持 UDP（Hysteria2/TUIC）+ 端口跳跃范围
- ✅ **Pipeline 单元测试 10/10 通过**

#### Phase 3 — 实时推送（代码审查确认已实现）

- ✅ **D5 WS/gRPC 双通道实时推送**：`CompositeConfigPusher` fan-out + Jitter Pull（0-3000ms 随机抖动）
- ✅ **D6/U4 gRPC AlterInbound 真增量**：`NativeXray.alterInboundViaGRPC` 通过 xray gRPC HandlerService 执行 AddInboundUser/RemoveInboundUser，用户增删零中断
- ✅ **D10 AES-GCM 配置加密**：`fetchConfigViaPayload` + AES-GCM 解密，密钥由 payloadKey 经 SHA-256 派生
- ✅ **E5 sing-box 蓝绿热转**：SO_REUSEPORT 双实例共存 + drain 排空，配置变更不断流

#### Phase 4 — 用户管控执行（内核级限速/设备/安全）

- ✅ **U1 限速执行（Sing-box 全模式 + Xray 子进程模式）**：`SpeedLimiter` 令牌桶限速 + `LimiterIntegration` 集成到 XrayExecutor/SingBoxExecutor；**Sing-box Native 模式已通过 ConnTracker 在 Read/Write 数据路径真正执行限速**（阻塞式 Wait，per-connection 级）；Xray Native 模式下限速元数据已解析（SpeedLimiter 已创建并通过 SetLimit 更新），但 xray-core 进程内 gRPC API 不支持 per-connection 限速拦截，数据路径 enforcement 待实现（可通过 xray policy.levels 配置 up/down mbps 实现）
- ✅ **U2 设备限制**：`DeviceLimiter` 本地 IP 集合（ip→refcount 引用计数，同一 IP 多连接复用）+ 面板下发全局设备态（WS 推送，60s 过期退化本地判定，取 max(local,global)）+ `DeviceEnforcer` 通过 gRPC StatsService 查询在线 IP 同步 + Sing-box Native 模式通过 SingboxDeviceChecker 适配器接入 ConnTracker 在连接建立时检查
- ✅ **S6/S7/S8 安全路由规则**：xray + sing-box 双核渲染器注入 SSRF 防护（阻断私有 IP 段）+ BT 防护（阻断 BitTorrent 协议）
- ✅ **U3 IP 限制**：`IPLimiter` 接通 ConnTracker + PluginAdapter + syncLimiters，支持全局黑名单（BlockIP/UnblockIP）、per-user 白名单（SetAllowedIPs）、per-user IP 数限制（活跃IP引用计数），判定优先级：黑名单 > 白名单 > IP数限制

### 🖥️ Machine 多节点单进程模式（2026-07-13 完成）

Machine 模式是 node-agent 的单进程多节点托管模式，一台 VPS 上用一个 agent 进程管理多个节点（多个 server_code），无需每个节点独立 systemd 服务。参考 XBoard Node 的多节点管理模式，解决 1 核 1G VPS 部署多个节点时的资源浪费问题。

**核心设计**：

- **端口隔离**：基于文件锁 + JSON 持久化的 `APIPortAllocator`，Xray API 端口池 `10085-10584`、Singbox Clash API 端口池 `20086-20585`，每个节点分配独立 API 端口
- **配置目录隔离**：每个节点独立配置目录 `{ConfigDir}/nodes/{serverCode}/` 和日志目录 `{LogDir}/nodes/{serverCode}/`
- **连接 epoch 隔离**：connEpoch 原子计数器解决 WS 重连时旧 goroutine 状态污染
- **配置端口强制改写**：`rewriteAPIInboundPort`（Xray）和 `rewriteClashAPIPort`（Singbox）在 Start 时防御性改写 API inbound 端口，双重保险避免端口冲突

**避坑清单已处理（P0 级风险）**：

| 风险 | 解决方案 |
|------|---------|
| SelfUpgrader 升级导致全节点宕机 | Orchestrator 唯一持有 SelfUpgrader，升级前 `gracefulShutdownAll` 优雅关闭所有节点后退出进程 |
| Prometheus 指标重复注册 panic | 每个 sub-Agent 使用独立 Registry + `WrapRegistererWith(server_code)` ConstLabels，`/metrics` 端点通过 `prometheus.Gatherers` 聚合 |
| 多节点重复 ACME 请求速率限制 | sub-Agent 设置 `skipSharedResources=true` 跳过 Nginx/Cert/Cloudflared/Firewall，由 Orchestrator 单例持有（待完成：Orchestrator 层共享资源启动） |
| 旧连接残留干扰新连接状态 | connEpoch 原子计数器 + goroutine 退出前 epoch 校验 |
| 日志目录/配置文件冲突 | 节点级独立 ConfigDir/LogDir |
| 心跳上报端口不准确 | RegisterRequest/HeartbeatRequest 新增 `xray_api_port`/`singbox_clash_port` 字段，注册和心跳上报实际分配端口 |

**统一 HTTP 服务**（Orchestrator 启动，绑定 127.0.0.1）：

| 端点 | 说明 |
|------|------|
| `GET /healthz?server_code=xxx` | 节点健康检查（不传 server_code 返回 orchestrator 全局状态） |
| `POST /delta?server_code=xxx` | 代理到具体节点的配置增量推送 |
| `GET /metrics` | 聚合所有节点 Prometheus 指标（带 server_code 标签） |
| `GET /nodes` | 列出所有托管节点（server_code/running/ports/config_dir） |
| `GET/POST /v1/status\|refresh\|restart\|diag?server_code=xxx` | 代理到具体节点的控制操作 |

**yunductl CLI 扩展**：

- `--http <addr>` flag：Machine 模式下通过 HTTP 连接 Orchestrator（替代 Unix Socket）
- `--node <code>` flag：指定操作某个节点
- `yunductl nodes`：列出所有托管节点
- `yunductl logs --lines N`：查看节点最近日志（支持按节点过滤）
- `yunductl bind`：查看端口绑定信息
- 原有 `status/refresh/restart/diag` 命令自动适配双模式（Unix Socket 单节点 / HTTP 多节点）

**架构图**：

```
MachineOrchestrator (单进程)
  ├─ 统一 HTTP Server (127.0.0.1:10000)
  │    ├─ /healthz, /delta, /metrics, /nodes
  │    └─ /v1/status|refresh|restart|diag (按 server_code 路由)
  ├─ SelfUpgrader (唯一持有，升级前协调关闭所有节点)
  ├─ PortAllocator (Xray: 10085-10584, Singbox: 20086-20585, 文件锁持久化)
  ├─ sub-Agent[server_code_1]
  │    ├─ NativeXray (api: 10085, 独立 Registry, skipOwnHTTPServer/skipSelfUpgrader/skipSharedResources)
  │    ├─ NativeSingbox (clash: 20086, 独立 ConnTracker)
  │    └─ ConfigDir: /etc/yundu/nodes/code1/, LogDir: /var/log/yundu/nodes/code1/
  ├─ sub-Agent[server_code_2]
  │    ├─ NativeXray (api: 10086)
  │    ├─ NativeSingbox (clash: 20087)
  │    └─ ConfigDir: /etc/yundu/nodes/code2/
  └─ gracefulShutdownAll: 并发 flush 流量(5s) → Stop ChannelManager → Stop Runtime(3s)，10s 总超时
```

**关键文件**：
- [machine/constants.go](file:///d:/机场搭建/air/apps/node-agent/internal/machine/constants.go) — 端口池范围常量 + IsInternalAPIPort
- [machine/port_allocator.go](file:///d:/机场搭建/air/apps/node-agent/internal/machine/port_allocator.go) — 文件锁持久化端口分配器
- [cmd/agent/machine.go](file:///d:/机场搭建/air/apps/node-agent/cmd/agent/machine.go) — MachineOrchestrator 完整实现
- [apps/yunductl/main.go](file:///d:/机场搭建/air/apps/yunductl/main.go) — CLI 扩展支持 HTTP/--node 模式

### 📊 监控与可观测性

- ✅ Prometheus metrics 埋点（每个服务 `/metrics`）
- ✅ RequestID 全链路透传
- ✅ 结构化日志（slog JSON 格式）
- ✅ Docker Compose 内含 Prometheus + Loki + Promtail + Grafana 配置

---

## 工程阶段进度

| 阶段 | 内容 | 状态 |
|------|------|------|
| Phase 0 | 产品确认、架构 ADR | ✅ 完成 |
| Phase 1 | monorepo、服务模板、迁移框架、可观测性底座、前端壳 | ✅ 完成 |
| Phase 2 | 身份、RBAC、审计、系统设置、后台壳 | ✅ 完成 |
| Phase 3 | 节点、服务器、健康、链式代理、发布、node-agent、TLS 证书、边缘暴露、节点体检、配置导入、协议注册中心、出站策略、WARP、runtime 升级 | ✅ 完成 |
| Phase 4 | 订阅引擎、模板渲染、多客户端订阅、客户端兼容矩阵、路由规则集、ConfigRenderer、节点组 LB | ✅ 完成 |
| Phase 5 | 套餐、流量、额度、重置、封禁 | ✅ 骨架完成 |
| Phase 6 | 工单、公告、通知、RBAC 权限码统一、API 路径校验 | ✅ 完成 |
| Phase 8/9/10 | 用户注册/登录、TRC20 支付自动开通、订阅链接真实返回、优惠券、邀请返利、佣金提现 | ✅ 主体完成 |
| YD-601~603 | 双模式节点编辑器 + 双内核校验 + RawSettings 透传 | ✅ 完成 |
| YD-701~703 | ChainSpec 中转链路 + BillAtLanding 计费 | ✅ 完成 |
| YD-801~802 | RuleSet 分层分流 + 双内核 Golden Test | ✅ 完成 |
| Phase 7-节点连通 | 真实节点连通、17 协议生产部署、客户端实测、0 元购闭环 | ✅ 完成（17/24 节点达标） |
| Phase 11-前端 | 17 协议预设 UI、Enhancement 面板、download_settings 编辑器 | ✅ 完成 |
| Phase 12-部分 | Express 网关代理 node-service、JWT 注入、DualKernelValidator 接入 | ✅ 完成 |
| **零SSH-Phase 0** | **紧急止血：D1-D9 + S1-S3 致命 Bug 修复** | ✅ **完成** |
| **零SSH-Phase 1** | **渲染器统一：R1-R12 双渲染器分叉消除** | ✅ **完成** |
| **零SSH-Phase 2** | **Agent 自治：D11 Pipeline.Run 统一 + E3/E7/E8/E10** | ✅ **完成** |
| **零SSH-Phase 3** | **实时推送：D5/D6/D10/E5 WS+gRPC+AES-GCM+蓝绿热转** | ✅ **完成** |
| **零SSH-Phase 4** | **用户管控：U1/U2 限速+设备 + S6-S8 安全路由** | ✅ **大部分完成** |
| **零SSH-Phase 5** | **面板产品化：F2-F6 配置预览/部署状态/回滚UI** | ⏳ **暂停（后端API已就绪，待前端开发）** |
| **全项目改造-阶段 1** | **端口语义修复：CDN/Tunnel/SaaS 强制 local_port** | ✅ **完成** |
| **全项目改造-阶段 2** | **xray gRPC api inbound 强制注入** | ✅ **完成** |
| **全项目改造-阶段 3** | **IPLimiter 接通：236 行孤儿代码激活** | ✅ **完成** |
| **全项目改造-阶段 4** | **NativeSingbox 蓝绿热转启用：useBlueGreen=true** | ✅ **完成** |
| **全项目改造-阶段 5** | **xray StatsService Reset=false 改造** | ✅ **完成** |
| **全项目改造-阶段 6** | **main.go 8 文件拆分：goTrack + gracefulShutdown 强化** | ✅ **完成** |
| **全项目改造-阶段 7** | **yunductl CLI 独立实现：8 子命令通过 Unix Socket** | ✅ **完成** |
| **全项目改造-阶段 8** | **面板侧 Machine 节点发现 API** | ✅ **完成** |
| **全项目改造-阶段 9** | **install.sh Machine 模式支持（--mode machine）** | ✅ **完成** |
| **全项目改造-阶段 10** | **文档对齐收尾：CURRENT_STATE.md 单一真相源** | ✅ **完成** |
| **Machine模式-Phase 1** | **基础设施：internal/machine 包(常量+端口分配器)、Config/Agent 扩展 skip 开关、端口范围判断** | ✅ **完成** |
| **Machine模式-Phase 2** | **端口隔离：NativeXray/NativeSingbox 端口改写、MultiRuntime 透传、DeviceEnforcer 动态端点、配置/日志目录隔离、sub-Agent skip 开关+独立 Registry** | ✅ **完成** |
| **Machine模式-Phase 3** | **P0 避坑修复：SelfUpgrader 收归 Orchestrator、Prometheus Registry 隔离+聚合、统一 HTTP Server、Nginx/Cert 共享资源协调、优雅退出并发 flush** | ✅ **完成** |
| **Machine模式-Phase 4** | **CLI/配套：yunductl 扩展 HTTP 模式/bind/logs/nodes、心跳上报实际分配端口(xray_api_port/singbox_clash_port)** | ✅ **完成** |
| **P3 生产部署** | **Phase 12A 数据迁移、Caddy HTTPS、真实密钥、Go 微服务编译部署** | ⏳ **待启动** |
| **P4 可观测性** | **Prometheus/Grafana/Loki/OTel 栈、流量自动化三定时任务** | ⏳ **待启动** |
| **P5 协议收尾** | **P07/P17 downloadSettings 降级、P13 WARP 凭据、VPS190 xray 升级** | ⏳ **待启动** |
| **P6 前端收尾** | **TypeScript 错误修复、ECH 服务端、download_settings 后端渲染** | ⏳ **待启动** |
| **P7 上线验收** | **全协议复测、故障演练、文档归档** | ⏳ **待启动** |

---

## 剩余工作（按 P3-P7 阶段路线图）

> 完整阶段规划见 [YunDu项目阶段存档与上线规划.md](file:///d:/机场搭建/进度/YunDu项目阶段存档与上线规划.md)
>
> 路线图：`当前 ──→ P3 生产部署 ──→ P4 可观测性 ──→ P5 协议收尾 ──→ P6 前端收尾 ──→ P7 验收 ──→ 上线`

### 🔴 P3 — 生产部署准备（1-2 周，上线硬阻塞）

| # | 工作 | 难度 | 说明 |
|---|------|------|------|
| 1 | **Phase 12A 数据迁移**：备份数据库、设置真实 JWT_SECRET/ARGON2_SALT、补全种子数据 | ⭐⭐⭐ | 不可逆操作，密钥禁止默认值 |
| 2 | **Caddy 生产入口 HTTPS**：ACME 证书、HSTS/CORS 安全头、WebSocket 支持、HTTP/2+HTTP/3 | ⭐⭐ | 接入 lego 库，每 6 小时续期检查 |
| 3 | **Go 微服务编译部署**：node-service/api-gateway/identity/subscription/traffic/node-agent 编译 + systemd | ⭐⭐⭐ | gRPC 9000 端口不可对公网暴露 |
| 4 | **Docker Compose 生产打磨**：数据卷持久化、日志轮转、healthcheck、restart policy | ⭐⭐ | — |
| 5 | **SMTP 邮件真实对接**（Mailgun/腾讯企业邮/SendCloud） | ⭐⭐ | xboard `app/Mail/` 模板可借鉴 |
| 6 | **订阅 Redis 缓存 + 降级**：60s 缓存、配置变更主动失效、DB 故障返回缓存 | ⭐⭐ | — |
| 7 | **订单 WebSocket 推送**：`/user/orders/:id/ws` 实时推送 paid 事件 | ⭐⭐ | — |

### 🟡 P4 — 可观测性与运维（1 周）

| # | 工作 | 难度 | 说明 |
|---|------|------|------|
| 8 | **可观测性栈部署**：Prometheus + Grafana + Loki（tsdb v13，7 天保留）+ OTel Collector（memory_limiter 75%、10% 采样） | ⭐⭐⭐ | 仅采集 `com.docker.compose.project=airport` 容器日志 |
| 9 | **Grafana 仪表盘**：HTTP 层（Airport Panel Overview）+ 业务层（Node Service Business Metrics） | ⭐⭐ | — |
| 10 | **流量自动化三定时任务**：每分钟检查超额/过期、每日处理日结、每月重置月度流量 | ⭐⭐⭐ | xboard `app/Console/Commands/TrafficFetch.php` 可借鉴 |
| 11 | **告警规则**：流量异常、节点掉线、证书过期 | ⭐⭐ | — |
| 12 | **Cron 定时任务**：订阅过期、订单过期、月度流量重置、日汇总、佣金自动结算 | ⭐⭐ | xboard `app/Console/Kernel.php` 可借鉴 |
| 13 | **封禁联动 NATS 事件**：banned 用户订阅 403、踢在线会话、agent 配置移除 UUID | ⭐⭐ | — |

### 🟡 P5 — 协议层收尾（3-5 天）

| # | 工作 | 难度 | 说明 |
|---|------|------|------|
| 14 | **P07/P17 downloadSettings 临时降级**：~~移除 downloadSettings 字段~~ ✅ 已在渲染器排除（P1-13）+ DB 同步 | ⭐⭐ | 等 xray 上游修复静默失败 bug |
| 15 | **P13 WARP 凭据注册**：`warp-cli register` 替换占位符 | ⭐ | — |
| 16 | **VPS190 xray 恢复到 26.3.27**：保持版本一致 | ⭐ | 先 `systemctl stop` 再 `cp` 二进制 |
| 17 | **订阅 URL path 为空 bug 修复**：subscription-service renderer | ⭐⭐ | — |
| 18 | ~~**P15 SS+REALITY 实验性评估**~~ ✅ 已修复：改为 VLESS+XHTTP+REALITY stream-up，密钥对已对齐 | ⭐ | — |

### 🟢 P6 — 前端收尾（3-5 天）

| # | 工作 | 难度 | 说明 |
|---|------|------|------|
| 19 | **presets.ts 的 5 个 flow 字段 TypeScript 错误修复** | ⭐ | 预存在错误 |
| 20 | **ECH 服务端配置实现**：`xray tls ech` 生成密钥对（UI 已就绪） | ⭐⭐ | 前后端联动 |
| 21 | **download_settings 后端渲染验证**：`renderXHTTPDownload` 序列化 | ⭐⭐ | 前后端联动 |
| 22 | **CreateNodeWizard 添加 download_settings 编辑器** | ⭐ | — |
| 23 | **后台用户详情 6 Tab**：概览/订阅Token/订单/工单/流量明细/操作日志 | ⭐⭐ | — |
| 24 | **用户管理操作 API**：ban/unban/reset-traffic/add-traffic/extend/change-plan/reset-sub/impersonate | ⭐⭐ | — |
| 25 | **套餐 CRUD 向导页面** + **后台订单管理**（手动确认/退款/标记已付） | ⭐⭐ | — |

### 🟢 P7 — 上线前验收（2-3 天）

| # | 工作 | 说明 |
|---|------|------|
| 26 | **全协议复测**：17 协议 + 速度 ≥1Mbps | — |
| 27 | **0 元购闭环复测** | — |
| 28 | **流量统计准确性对账** | — |
| 29 | **故障演练**：SSH 断开、VPS 重启、证书过期 | — |
| 30 | **文档归档**：运维手册、应急预案、配置变更记录 | — |

### 🔄 零 SSH 全闭环 — 剩余工作

> 阶段 0-10 全项目改造已完成（2026-07-12），详见 [CURRENT_STATE.md](file:///d:/机场搭建/air/docs/CURRENT_STATE.md)
>
> 零 SSH 完成度从 96% 提升至 99%，IP 限制（U3）、蓝绿热转、流量统计、main.go 重构、yunductl CLI、Machine 模式全部落地。

| # | 任务 | 说明 | 优先级 |
|---|------|------|--------|
| ~~1~~ | ~~**U3 IP 限制实现**~~ | ✅ **已在全项目改造阶段 3 完成**：IPLimiter 接通 ConnTracker + PluginAdapter + syncLimiters，236 行孤儿代码激活 | ~~中~~ |
| 2 | **Phase 5 面板产品化** | F2 配置预览 UI / F3 部署状态显示 / F4 回滚 UI / F5 节点健康面板 / F6 CDN 模板。后端 API 已就绪（dry-run/rollback/deployments） | 高 |
| 3 | **实际部署验证** | 在 VPS190/VPS81 部署新编译二进制，验证零 SSH 全链路 | 高 |
| 4 | **E6 内核自动升级验证** | Agent 自升级已实现，需验证 xray-core 版本升级闭环 | 中 |
| 5 | **certmagic 迁移** | acme.sh → certmagic 纯 Go 库，消除外部依赖 | 低 |

### ⚙️ P2 — 上线后持续优化

- URI 批量导入补全 6 种协议解析器（框架已有，参数映射待完善）
- 配置发布 UI（版本 diff、灰度进度、回滚按钮）
- 流量统计图表（日/周/月）
- Telegram Bot 绑定/通知
- 公告系统用户端展示（Announcements 页面）
- pg_dump 定时备份脚本
- 2FA TOTP 登录
- WAF/CC 防护规则
- 支付宝提现人工审核流程

---

## 难点与踩坑记录（已解决）

完整踩坑记录见项目记忆 `c:\Users\Administrator\.trae-cn\memory\projects\-d------air\project_memory.md`，关键项：

- **WSL2 进程稳定运行**：必须用 `nohup + disown` 而非 setsid，否则 api-gateway 立即死亡
- **Go 二进制存放位置**：必须输出到 Linux 原生 `/tmp/yundu-bin/`，Windows mount 下二进制损坏
- **网关路由白名单**：新增 API 必须同步在 api-gateway 注册三段（public/userAPI/adminAPI），否则 404
- **JWT Claims 跨服务对齐**：identity/node/api-gateway 三方 Claims 字段必须对齐（permissions/admin_id）
- **PostgreSQL inet 类型**：Go 端必须用 `ip_address::text` 强转再扫描
- **Shadowrocket 兼容**：VMess 必须用非 base64 包裹 URI、fp 小写、WS 路径不带多余参数、无 BOM
- **xray vs sing-box 字段差异**：protocol/type、port/listen_port、clients/users、camelCase/snake_case、freedom/direct、reality 位置不同（project_memory 有 98 行规范）
- **订单金额 NULL**：SQL 中必须用 `COALESCE(final_amount, amount_usdt)` 处理
- **工单字段前后端不一致**：前端发 `message` 后端要求 `description`，DTO 双字段兼容
- **Cloudflare gRPC 截断**：keepalive 间隔必须 >60s
- **Docker 环境变量**：各服务使用独立前缀（IDENTITY_SERVICE_PORT/NODE_SERVICE_PORT/...），不能统一用 PORT
- **migrate 工具 MIGRATIONS_DIR**：Docker 默认 `/app/migrations`，Windows 默认 `d:\机场搭建\air\migrations`

---

## XBoard 移植建议（高 ROI 项）

xboard 参考代码已解压在 [tmp-bin/xboard-ref/extracted/](file:///d:/机场搭建/air/tmp-bin/xboard-ref/extracted/)（payments.tar.gz、mail.tar.gz、services.tar.gz、models.tar.gz、routes.tar.gz、console.tar.gz）。

### ⭐⭐⭐ 强烈建议移植（xboard 已打磨 5 年，自己重写踩坑成本极高）

| xboard 模块 | 移植内容 | 目标位置 |
|------------|---------|---------|
| `app/Utils/URL.php` | 6 种协议 URI 拼接逻辑、Shadowrocket 兼容、特殊字符编码 | `subscription-service/renderer/uri.go` |
| `app/Protocols/` | 18 种协议 Clash/Sing-box 配置数组生成、字段映射 | `subscription-service/renderer/clash.go`、`singbox.go` |
| `app/Console/Commands/TrafficFetch.php` | xray StatsService 按 email 维度拉 up/down | `node-agent/executor/` 或 `traffic-service` |
| `app/Utils/Client.php` | UA 识别 30+ 种客户端正则 | `subscription-service/middleware/ua.go` |
| `resources/views/client/*.blade.php` | Clash/Sing-box 完整模板（proxy-groups/rules/锚点复用） | `subscription-service/templates/` |
| `app/Mail/` | 6 种邮件模板内容（验证/重置/支付/工单/到期/流量告警） | `identity-service/templates/mail/` |
| `app/Console/Kernel.php` | 定时任务调度清单（过期检查/流量重置/佣金结算/日汇总） | 各服务 cron 模块 |

### ⭐⭐ 建议移植（节省时间）

| xboard 模块 | 移植内容 |
|------------|---------|
| `app/Services/Coupon.php` | 优惠券校验细节（最低消费、叠加规则、一次性/重复使用） |
| `app/Http/Controllers/User/InviteController.php` | 邀请返利明细、佣金结算、邀请链接生成 |
| `app/Services/Config.php` | 订阅配置合并（用户自定义 DNS、节点过滤、地区分组规则） |
| `app/Http/Middleware/` | 各种安全中间件（防爆破、防爬、异常 UA 拦截） |

### ❌ 不要移植（保留云渡差异化优势）

- **节点管理逻辑**：xboard SSH 改配置文件，云渡是 agent 灰度发布+自动回滚
- **分流规则**：云渡 RuleSet 两层正交比 xboard 单一 geosite 强
- **中转链路**：ChainSpec 协议无关抽象比 xboard proxySettings.tag 串联优雅
- **双内核支持**：xboard 主要支持 xray，云渡 xray+sing-box 一等公民
- **AI 诊断 / Node Doctor / TLS ACME / WARP / 配置版本灰度**：xboard 没有这些能力

---

## VPS 部署步骤

### 准备

- VPS 推荐 4C8G Ubuntu 22.04（测试 VPS：43.135.147.190）
- 域名 2 个：面板域名（`panel.example.com`）+ 订阅域名（`sub.example.com`）
- 开放端口：22（SSH）、80/443（HTTP/HTTPS）、节点协议端口（443/8443 UDP+TCP）
- TronGrid API Key（免费注册 https://www.trongrid.io/，100k 请求/天足够）
- SMTP 账号（Mailgun/腾讯企业邮/SendCloud 任意）

### 部署步骤

```bash
# 1. 登录 VPS
ssh -i key/190key.pem root@<vps-ip>

# 2. 安装 Docker + Docker Compose
#    参见 scripts/install-docker-wsl.sh 调整为 Ubuntu 版本
apt update && apt install -y docker.io docker-compose-plugin

# 3. 上传项目代码（rsync 或 git clone）
rsync -avz -e "ssh -i key/190key.pem" ./ root@<vps-ip>:/opt/yundu/

# 4. 配置生产环境变量
cd /opt/yundu/deploy/docker
cp .env.production .env
# 编辑 .env：
#   - POSTGRES_PASSWORD 强密码
#   - JWT_SECRET 64位随机串
#   - AGENT_API_TOKEN_SALT 64位随机串
#   - TRC20_ADDRESS=TLAoiTwPNCtFXpJWmPrvgm8tRpC9ggP42H
#   - TRONGRID_API_KEY=xxx
#   - SMTP_* 配置
#   - DOMAIN_PANEL=panel.example.com
#   - DOMAIN_SUB=sub.example.com

# 5. 启动基础设施 + 应用
docker compose -f docker-compose.full.yml up -d

# 6. 执行数据库迁移
docker compose exec identity-service ./migrate -path /app/migrations up

# 7. 创建超级管理员
docker compose exec identity-service ./identity-service createuser \
  --email a****@*********** --password <strong-password> --superadmin

# 8. 验证健康
curl -f http://127.0.0.1:8080/healthz
curl -f http://127.0.0.1:8081/healthz
curl -f http://127.0.0.1:8083/healthz

# 9. 配置 Caddy 自动 HTTPS（deploy/docker/caddy/Caddyfile 已有模板）
#    Caddy 会自动申请 Let's Encrypt 证书
docker compose restart caddy

# 10. 在节点 VPS 上安装 node-agent
#     后台"添加服务器"生成 AGENT_TOKEN
#     使用一键安装脚本（自动检测架构、下载 agent+内核、生成 systemd unit）：
curl -fsSL https://panel.example.com/api/v1/install.sh | \
  bash -s -- --token=<AGENT_TOKEN> --endpoint=https://panel.example.com

# 11. 后台创建节点 → 创建套餐 → 绑定节点
# 12. 端到端测试：注册→登录→购买→支付→订阅→客户端连通→流量统计
```

### 节点安装后验证

```bash
# 在节点 VPS 上检查 agent 状态
systemctl status yundu-node-agent

# 检查 xray 是否运行
systemctl status xray

# 检查配置已下发
ls /etc/yundu/config/
xray run -test -c /etc/yundu/config/xray.json  # 配置测试
```

---

## 快速开始（开发环境）

### 依赖

- Go 1.24+
- Node.js 22+ + pnpm 9+
- WSL2（Ubuntu 22.04，Windows 开发环境）
- goose CLI：`go install github.com/pressly/goose/v3/cmd/goose@latest`

### 启动基础设施（WSL2 内）

```bash
bash scripts/start-infra-wsl.sh
# 启动 PostgreSQL (5432)、Redis (6379)、NATS (4222)
```

### 执行数据库迁移

```bash
bash scripts/apply-migrations.sh
```

### 启动所有后端服务

```bash
bash scripts/dev-start.sh
# 编译 Linux 二进制到 /tmp/yundu-bin/
# 启动 5 个服务，日志在 /tmp/logs/
# PID 文件在 /tmp/pids/
```

### 健康检查

```bash
bash scripts/dev-health.sh
```

### 启动前端

```bash
# 用户端（5178 端口）
cd apps/user-web && pnpm dev

# 管理端（5173 端口）
cd apps/admin-web && pnpm dev
```

### 停止所有服务

```bash
bash scripts/dev-stop.sh
```

---

## 环境变量

复制 `.env.example` 为 `.env`：

```bash
cp .env.example .env
```

核心变量：

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `POSTGRES_DSN` | PostgreSQL 连接串 | `postgres://app:app@localhost:5432/airport?sslmode=disable` |
| `REDIS_ADDR` | Redis 地址 | `localhost:6379` |
| `NATS_URL` | NATS 地址 | `nats://localhost:4222` |
| `JWT_SECRET` | JWT 签名密钥 | **必须设置（生产）** |
| `ARGON2_SALT` | argon2id 盐种子 | **必须设置** |
| `AGENT_API_TOKEN_SALT` | node-agent HMAC 盐 | **必须设置（见 .env）** |
| `TRC20_ADDRESS` | 默认 TRC20 收款地址 | `TLAoiTwPNCtFXpJWmPrvgm8tRpC9ggP42H` |
| `TRC20_CONTRACT` | USDT-TRC20 合约 | `TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t` |
| `TRONGRID_API` | TronGrid API 地址 | `https://api.trongrid.io` |
| `TRONGRID_API_KEY` | TronGrid API Key | 空（免费额度足够） |
| `PUBLIC_URL` | 对外公开访问 URL | `http://127.0.0.1:5178` |

**默认测试账号**（开发环境）：

| 角色 | 邮箱 | 密码 |
|------|------|------|
| 普通用户 | `T@t.com` | `123456` |
| 超级管理员 | 通过 `cmd/createuser` 或迁移种子创建 | |

**测试优惠码**：`TEST2OFF`（$2 固定折扣，用于购买流程测试）

---

## 技术栈

| 层级 | 技术 |
|------|------|
| 后端语言 | Go 1.24+ |
| Web 框架 | Gin |
| 数据库 | PostgreSQL 16 |
| 缓存 | Redis 7 |
| 消息总线 | NATS JetStream |
| 前端 | React 18 + TypeScript + Vite |
| UI 组件 | Shadcn/UI + TailwindCSS |
| 状态管理 | TanStack Query + Zustand |
| 表单 | react-hook-form + zod |
| 迁移工具 | goose |
| 代理内核 | xray-core v26.x / sing-box v1.13+ |
| 反向代理 | Caddy 2（自动 HTTPS） |
| 容器 | Docker + Docker Compose |
| 可观测性 | Prometheus + Grafana + Loki |

---

## 订阅 URL 规范

**主订阅URL**（永久有效，token 可重置）：

```
https://sub.example.com/{token}
https://sub.example.com/{token}?client=clash
https://sub.example.com/{token}?client=singbox
https://sub.example.com/{token}?sub=clash
```

**兼容短链**：

```
https://panel.example.com/api/v1/sub/{token}
https://panel.example.com/sub/{token}
```

**客户端 UA 自动识别**：不指定 client 参数时通过 User-Agent 自动识别，默认返回 URI base64 格式。

**subscription-userinfo 响应头**（所有客户端通用）：

```
subscription-userinfo: upload=0; download=1073741824; total=107374182400; expire=1767225600
profile-update-interval: 6
profile-title: YunDu
profile-web-page-url: panel.example.com
content-disposition: attachment;filename="yundu-clash.yaml"
```

---

## 测试

```bash
# 运行所有 Go 单元测试 + Golden Test
cd /path/to/air
go test ./packages/... ./apps/... -count=1

# 关键测试包
cd apps/node-service && go test ./internal/exposure/... -v -count=1   # 双内核渲染
cd packages/subscription && go test ./... -v -count=1                 # 中间层+订阅渲染
cd apps/node-service && go test ./internal/importer/... -v -count=1   # URI 导入

# E2E API 测试（需所有服务运行）
bash tmp-bin/e2e_test.sh
```

---

## 协议支持矩阵（2026 基准，17 协议已部署测速）

> 17/24 节点达标（>1Mbps），17 协议 100% 覆盖，速度范围 3.40~13.99 Mbps  
> 完整配置表与避坑指南见 [YunDu-17协议配置表与避坑指南.md](file:///d:/机场搭建/进度/YunDu-17协议配置表与避坑指南.md)

| ID | 协议 | 传输/安全 | VPS206 | VPS190 | 内核兼容 | CDN | 状态 |
|---|---|---|---|---|---|---|---|
| **P01** | VLESS+REALITY+Vision | tcp/reality | 9450 ✅ | 40001 ✅ | both | ❌ | ✅ 10.83 Mbps |
| **P02** | Trojan+TLS | tcp/tls | 9447 ✅ | 40002 ✅ | both | ❌ | ✅ |
| **P03** | VLESS+WS+TLS (CDN) | ws/tls | 9445 ✅ | 40003 ✅ | both | ✅ | ✅ |
| **P04** | Trojan+WS+TLS (CDN) | ws/tls | 9446 ✅ | 40004 ✅ | both | ✅ | ✅ |
| **P05** | AnyTLS | tcp/tls | - | 40009 ✅ | sb only | ❌ | ✅ 13.99 Mbps |
| **P06** | XHTTP 上CDN\|下REALITY | xhttp/tls+reality | 9453 ✅ | - | xray only | hybrid | ✅ dl已禁用 |
| **P07** | XHTTP 上REALITY\|下CDN | xhttp/reality+tls | 9454 ⚠️ | - | xray only | hybrid | ⚠️ dl bug |
| **P08** | XHTTP+TLS+CDN | xhttp/tls | 9451 ✅ | - | both | ✅ | ✅ |
| **P09** | XHTTP stream-up+REALITY+XMUX | xhttp/reality | 9455 ✅ | 40008 ✅ | xray only | ❌ | ✅ 11.24 Mbps |
| **P10** | VLESS+HTTPUpgrade+TLS | httpupgrade/tls | 9456 ✅ | - | both | ✅ | ✅ |
| **P11** | Hysteria2 | quic/tls | - | 40005 ✅ | sb only | ❌ | ✅ |
| **P12** | TUIC v5 | quic/tls | - | 40006 ✅ | both | ❌ | ✅ 13.37 Mbps |
| **P13** | WARP MASQUE 叠加层 | tcp/none | - | 40010 ⚠️ | sb only | overlay | ⚠️ 占位符 |
| **P14** | VLESS+WS+TLS+SS2022 | ws/tls | 9457 ✅ | - | xray only | ✅ | ✅ |
| **P15** | VLESS+XHTTP+REALITY stream-up | xhttp/reality | 9458 ✅ | - | xray only | ❌ | ✅ 密钥已修复 |
| **P16** | Trojan+gRPC+TLS | grpc/tls | 9448 ✅ | 40007 ✅ | both | hybrid | ✅ 13.23 Mbps |
| **P17** | XHTTP stream-up+REALITY+XMUX+v4v6 | xhttp/reality | 9452 ⚠️ | - | xray only | ❌ | ⚠️ dl bug |

**传输层优先级**：XHTTP > REALITY > Hysteria2 > TUICv5 > gRPC > WS > TCP
**安全层优先级**：TLS 1.3 + ECH > REALITY > TLS 1.3 > none
**默认指纹**：chrome（桌面）/ chrome（ios 通过 fp 参数区分 safari）

**关键密钥**（生产环境实际值）:
- REALITY 私钥: `cHAWz_DP00iHGudE9Uq-8txkbwiZGCTAV1GvDQ8Z7U4`
- REALITY 公钥: `nS2ld_0Xn_GntyX-HqW11DqFbHn72FJviEwJoZ2vUx0`（旧值 `i_LzFZ-...` 已废弃）
- REALITY short_id: `e571783bd3842eae`
- VLESS UUID: `34df02b2-f5a5-43da-9091-8ab529c82530`
- Trojan 密码: `a63b5e0dbfc08a2735ba6a717fe2e542`
- WS path: `/ws9ad966ec` / XHTTP path: `/xhb4cc53b6`
- VPS206 证书 SHA256: `d857e1b091b2d648e51d596cdf1464c69cf7524513c599dd71b481eb26810fb3`（自签名，直连节点客户端必须配置 `pinnedPeerCertSha256`）

---

## 贡献规范

- 先阅读 `CLAUDE.md` 了解代码边界规范
- 所有新 API 必须先更新 OpenAPI 文档（`docs/openapi/`）
- 所有新表或改表必须有 goose migration 文件
- 涉及双内核配置改动必须同时更新 Xray 和 Sing-box 渲染器，并补充 Golden Test
- 涉及节点字段改动必须同步更新 URI importer 解析逻辑
- Commit message 格式：`feat(service): 描述` / `fix(service): 描述` / `chore: 描述`
- 所有路径引用必须使用绝对路径 link 格式（见 CLAUDE.md）

---

## 相关文档

### ⭐ 当前状态单一真相源
- [CURRENT_STATE.md](file:///d:/机场搭建/air/docs/CURRENT_STATE.md) — **2026-07-12 阶段 0-10 全项目改造完成后的最新状态汇总（优先阅读）**

### 项目存档（进度文档）
- [YunDu-开发进度总结-20260711.md](file:///d:/机场搭建/air/docs/YunDu-开发进度总结-20260711.md) — ⭐ 零 SSH 全闭环 Phase 0-4 开发进度与交接文档（2026-07-11）
- [YunDu-下阶段执行开发计划-面板自动下发Agent零SSH全闭环-20260711.md](file:///d:/机场搭建/YunDu-下阶段执行开发计划-面板自动下发Agent零SSH全闭环-20260711.md) — ⭐ 零 SSH 全闭环 Phase 0-6 完整开发计划
- [YunDu-标准化多VPS架构设计标准模板手册-20260710.md](file:///d:/机场搭建/YunDu-标准化多VPS架构设计标准模板手册-20260710.md) — 多 VPS 架构设计标准模板
- [YunDu项目分析报告-零SSH自动化与架构优化-20260710终极版.md](file:///d:/机场搭建/YunDu项目分析报告-零SSH自动化与架构优化-20260710终极版.md) — 零 SSH 全链路测试报告
- [YunDu-17协议配置表与避坑指南.md](file:///d:/机场搭建/进度/YunDu-17协议配置表与避坑指南.md) — ⭐ 17 协议完整配置表 + 避坑指南（2026-07-04 存档）
- [YunDu项目阶段存档与上线规划.md](file:///d:/机场搭建/进度/YunDu项目阶段存档与上线规划.md) — ⭐ P3-P7 阶段路线图 + 上线清单（2026-07-04 存档）
- [architecture.md](file:///d:/机场搭建/进度/architecture.md) — 双轨架构文档（生产 XBoard + 开发 YunDu）

### 工程文档
- [商业生产落地工程单](file:///d:/机场搭建/air/商业生产落地工程单.md) — 完整施工蓝图
- [搭建指南](file:///d:/机场搭建/air/airport_搭建指南.md)
- [节点配置手册](file:///d:/机场搭建/air/搭建机场高阶节点配置完全手册.md)
- [架构 ADR](file:///d:/机场搭建/air/docs/adr/0001-system-architecture.md)
- [gRPC CDN 兼容文档](file:///d:/机场搭建/air/docs/grpc-cdn-compat.md)
- [数据库 Schema v1](file:///d:/机场搭建/air/数据库%20Schema%20v1.md)
- [运维 Runbook](file:///d:/机场搭建/air/docs/runbooks/db-bootstrap.md)
- `CLAUDE.md` — AI 辅助开发全局约束
- `DESIGN_SYSTEM.md` — UI 设计规范
