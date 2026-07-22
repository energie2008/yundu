
# ADR-0001 系统总体架构

- 状态：Accepted
- 日期：2026-06-29
- 决策者：Platform Team

## 背景

传统机场面板通常以单体应用为中心，把用户、套餐、节点、订阅、支付、工单、系统配置全部堆在同一套后台中。这样的做法部署简单，但在商业生产场景下会迅速遇到几个问题：后台复杂度失控、节点管理与业务管理耦合、协议更新成本高、灰度发布困难、移动端管理体验差、链式代理与稳定性治理能力薄弱。[code_file:38][code_file:40]

同时，现有开源项目在不同方向各有强项。3X-UI 已覆盖多协议、多安全方式、多节点、REST API、订阅服务器、出站代理链和 PostgreSQL 等能力，更适合作为 runtime/provider 参考或兼容对象，而不是完整商业机场控制平面。[page:1] Air-Universe 一类项目则验证了“节点执行器 + 多面板适配”的模式是可行的，但其项目更新较早，更适合作为接口抽象思路参考。[page:2]

本项目目标不是修补旧面板，而是建设一套“控制平面 + 执行平面 + 订阅编排平面”分层的新系统，支持移动端优先、链式代理优先、协议抽象优先、节点稳定性优先。[code_file:38]

## 决策

采用三平面架构：
- 控制平面 Control Plane：负责用户、权限、套餐、订单、节点资源、配置中心、审计、后台 UI。[code_file:38]
- 执行平面 Execution Plane：负责 node-agent、runtime provider、配置下发、节点心跳、健康探测、灰度发布、回滚。[code_file:38]
- 订阅编排平面 Subscription Plane：负责模板渲染、客户端兼容、节点筛选、权重排序、缓存和访问日志。[code_file:38]

采用单仓多服务 monorepo 组织方式，核心服务包括：api-gateway、identity-service、node-service、subscription-service、traffic-service、node-agent，并预留 billing、support、notification、audit、analytics 的扩展位。[code_file:38][code_file:39]

数据库以 PostgreSQL 为主库，Redis 承担热数据与实时计数，JSONB 承载高变动协议字段；核心模型显式拆分为 users、subscription_tokens、servers、runtimes、nodes、proxy_chains、config_versions、deployment_batches 等对象，确保宿主资源、协议实例、逻辑节点、链式代理、配置版本互相解耦。[code_file:40]

## 架构原则

### 1. Mobile-first
所有后台页面先做手机端断点和操作路径，再做桌面端增强。移动端必须能完成值班场景下的核心任务：查看节点状态、处理告警、发布配置、回滚、封禁用户、查看工单。[code_file:38][code_file:39]

### 2. Ops-first
优先保证节点稳定性、链式代理治理、发布安全、可观测性和快速回滚，而不是优先开发营销页或支付周边。[code_file:38]

### 3. Protocol-agnostic
协议配置不直接写死在数据库列上，而是使用 protocol_type、transport_type、security_type、config_json、schema_version 的组合模型，通过 JSON Schema 和模板驱动协议扩展。[code_file:38][code_file:40]

### 4. API-first
所有 UI 都只能消费正式 API，不允许前端绕过网关或依赖后端内部结构。OpenAPI 与 proto 是工程边界的一部分，必须先定义再实现。[code_file:39]

### 5. Production-ready
从第一阶段开始纳入审计、监控、灰度发布、配置版本化、差异比较、回滚入口、种子数据、环境隔离和备份恢复要求。[code_file:38][code_file:39][code_file:40]

## 服务边界

### api-gateway
- 统一认证入口。
- 路由聚合。
- 基础限流、审计上下文、request id、统一错误包装。[code_file:39]

### identity-service
- 用户、管理员、认证、session、2FA、RBAC。
- 输出当前用户和管理员身份信息。
- 管理 subscription token 生命周期的一部分。[code_file:39][code_file:40]

### node-service
- 管理服务器、runtime、逻辑节点、链式代理、健康状态、配置版本、发布批次。
- 对接 node-agent、自定义 provider、3X-UI provider 等适配器。[code_file:39][code_file:40]

### subscription-service
- 负责 subscription token 校验。
- 根据用户权益、节点可见性、客户端类型生成订阅。
- 管理模板和订阅访问日志。[code_file:38][code_file:40]

### traffic-service
- 汇总上传下载流量。
- 执行倍率、额度、重置周期、超额判定。
- 与在线状态和封禁策略联动。[code_file:39][code_file:40]

### node-agent
- 运行于宿主机。
- 周期心跳。
- 拉取目标配置版本。
- dry-run 校验。
- 应用配置、reload、rollback、上报结果。[code_file:39]

## 数据模型原则

1. users 是身份主体，代理接入凭证放在 user_credentials，订阅拉取凭证放在 subscription_tokens，避免登录与订阅令牌混用。[code_file:40]
2. servers 表示宿主资源，runtimes 表示协议执行器，nodes 表示给用户看的逻辑节点，proxy_chains 表示链式代理定义。[code_file:40]
3. config_versions、deployment_batches、deployment_targets 组成“可发布、可追踪、可回滚”的最小闭环。[code_file:40]
4. 高频在线态和瞬时指标优先放 Redis，PostgreSQL 保留审计镜像和统计落表。[code_file:40]

## 运行时适配策略

统一定义 runtime provider adapter 接口，抽象以下能力：RegisterRuntime、PushConfig、PullStats、Reload、Rollback、FetchCapabilities，使 node-agent、自定义 provider、3X-UI provider 都能纳入同一控制平面。[code_file:39]

这样做的原因是：
- 可以渐进迁移旧节点，不要求一次切全量。
- 可以混用不同 runtime 或不同地域策略。
- 可以把 3X-UI 当外部 provider 接入，而不是强耦合它的数据结构。[page:1][code_file:38]

## 发布模型

任何节点配置变更都必须经过以下步骤：
1. 生成目标配置版本。
2. 基于 schema 做 dry-run 校验。
3. 形成 diff 摘要。
4. 创建 deployment batch。
5. 分批执行。
6. 健康检查失败时自动回滚。
7. 全链路审计落表。[code_file:38][code_file:39][code_file:40]

## 可观测性要求

第一版就接入统一日志、Prometheus 指标、OpenTelemetry trace、节点健康事件、订阅访问日志和发布结果记录，保证问题能定位到请求、版本和节点层级。[code_file:39][code_file:40]

## 后果

### 正面影响
- 业务面与节点运维面解耦，后续协议更新成本更低。[code_file:38][code_file:40]
- 节点发布、心跳、健康、回滚成为平台内建能力，而不是人工脚本补丁。[code_file:38][code_file:39]
- 移动端后台从一开始就是目标，不会后期补救。[code_file:38][code_file:39]
- 对接不同 provider 的能力更强，迁移路径更平滑。[page:1][page:2][code_file:39]

### 代价
- 初期工程复杂度高于单体面板。[code_file:38]
- 需要更严格的 API 设计、迁移设计、观测与测试纪律。[code_file:39]
- 团队需要适应服务边界和事件驱动思维。[code_file:38]

## 不采用的方案

### 方案 A：继续基于 Xboard 二开
不采用，原因是历史包袱重、对象边界不清、协议升级和节点治理能力上限有限，更适合局部修补，不适合重建商业生产底座。[code_file:38]

### 方案 B：直接把 3X-UI 当主系统
不采用，原因是 3X-UI 强在节点和协议控制，不强在商业化用户、套餐、权限、后台管理、审计等控制平面能力。[page:1][code_file:38]

### 方案 C：一步到位拆十几个微服务
不采用，原因是第一阶段交付会过重，因此先以 5 个核心服务 + 1 个 agent 起步，再逐步拆分 billing、support、notification 等服务。[code_file:38][code_file:39]
