// apps/admin-web/src/lib/endpoints.ts
// 唯一真相来源：所有后端 API 路径在此集中定义。
// 组件中不允许硬写路径字符串，必须引用 EP 常量。
//
// 路径来源：后端 DUMP_ROUTES=1 生成的 tmp/*-routes.json
// 前缀 /api/v1 由 api.ts 的 baseURL 自动添加，这里只写 /admin/... 部分

export const EP = {
  // ===== Admin Auth (identity-service) =====
  ADMIN_AUTH_LOGIN: '/admin/auth/login',
  ADMIN_AUTH_LOGOUT: '/admin/auth/logout',
  ADMIN_ME: '/admin/me',

  // ===== Admins management (identity-service) =====
  ADMINS: '/admin/admins',

  // ===== System Settings (identity-service) =====
  SYSTEM_SETTINGS: '/admin/system/settings',
  SYSTEM_SETTING_UPDATE: (group: string, key: string) => `/admin/system/settings/${group}/${key}`,

  // ===== Audit logs (identity-service) =====
  AUDIT_LOGS: '/admin/audit-logs',

  // ===== Traffic overview (traffic-service) =====
  TRAFFIC_OVERVIEW: '/admin/traffic/overview',
  TRAFFIC_USER: (id: string) => `/admin/traffic/user/${id}`,
  TRAFFIC_USER_QUOTA: (id: string) => `/admin/traffic/user/${id}/quota`,
  TRAFFIC_USER_RESET: (id: string) => `/admin/traffic/user/${id}/reset`,

  // ===== Node health (node-service) =====
  NODE_HEALTH: (id: string) => `/admin/nodes/${id}/health`,

  // ===== TLS 证书 (node-service) =====
  TLS_CERTIFICATES: '/admin/tls-certificates',
  TLS_CERTIFICATE: (id: string) => `/admin/tls-certificates/${id}`,
  TLS_CERTIFICATE_RENEW: (id: string) => `/admin/tls-certificates/${id}/renew`,
  TLS_CERTIFICATE_DEPLOY_STATUS: (id: string) => `/admin/tls-certificates/${id}/deploy-status`,

  // ===== TLS Profile (node-service) =====
  TLS_PROFILES: '/admin/tls-profiles',
  TLS_PROFILE: (id: string) => `/admin/tls-profiles/${id}`,

  // ===== 边缘暴露 (node-service, 按服务器维度) =====
  SERVER_EXPOSURE: (serverId: string) => `/admin/servers/${serverId}/exposure`,
  SERVER_EXPOSURE_PREVIEW: (serverId: string) => `/admin/servers/${serverId}/exposure/preview`,
  SERVER_EXPOSURE_VALIDATE: (serverId: string) => `/admin/servers/${serverId}/exposure/validate`,
  SERVER_EXPOSURE_APPLY: (serverId: string) => `/admin/servers/${serverId}/exposure/apply`,

  // ===== 节点体检 (node-service) =====
  NODE_DOCTOR_REPORTS: (nodeId: string) => `/admin/nodes/${nodeId}/doctor-reports`,
  NODE_DOCTOR_REPORT_LATEST: (nodeId: string) => `/admin/nodes/${nodeId}/doctor-reports/latest`,
  NODE_DOCTOR_CHECK: (nodeId: string) => `/admin/nodes/${nodeId}/doctor/check`,
  NODE_DOCTOR_AUTOFIX: (nodeId: string) => `/admin/nodes/${nodeId}/doctor/autofix`,

  // ===== 配置导入 (node-service) =====
  // 注意：列表用复数 config-imports，创建用单数 config-import
  CONFIG_IMPORTS: '/admin/config-imports',
  CONFIG_IMPORT_CREATE: '/admin/config-import',
  CONFIG_IMPORT_DETAIL: (id: string) => `/admin/config-import/${id}`,
  CONFIG_IMPORT_APPLY: (id: string) => `/admin/config-import/${id}/apply`,

  // ===== Protocol Registry & Presets (node-service) =====
  PROTOCOL_REGISTRY: '/admin/protocol-registry',
  PROTOCOL_REGISTRY_ITEM: (id: string) => `/admin/protocol-registry/${id}`,
  PROTOCOL_PRESETS: '/admin/protocol-presets',
  PROTOCOL_PRESET_DETAIL: (id: string) => `/admin/protocol-presets/${id}`,
  PROTOCOL_PRESET_FORK: (id: string) => `/admin/protocol-presets/${id}/fork`,
  PUBLIC_PROTOCOL_PRESETS: '/public/protocol-presets',

  // ===== Plans (identity-service) =====
  PLANS: '/admin/plans',
  PLAN_DETAIL: (id: string) => `/admin/plans/${id}`,
  PLAN_NODES: (id: string) => `/admin/plans/${id}/nodes`,

  // ===== Coupons (identity-service) =====
  COUPONS: '/admin/coupons',
  COUPON_DETAIL: (id: string) => `/admin/coupons/${id}`,

  // ===== Orders (identity-service) =====
  ORDERS: '/admin/orders',
  ORDER_DETAIL: (id: string) => `/admin/orders/${id}`,
  ORDER_CANCEL: (id: string) => `/admin/orders/${id}/cancel`,
  ORDER_MARK_PAID: (id: string) => `/admin/orders/${id}/mark-paid`,

  // ===== Payment Methods (identity-service) =====
  PAYMENT_METHODS: '/admin/payment-methods',
  PAYMENT_METHOD_DETAIL: (method: string) => `/admin/payment-methods/${method}`,
  PAYMENT_METHOD_TOGGLE: (method: string) => `/admin/payment-methods/${method}/toggle`,
  PAYMENT_EXCHANGE_RATE: '/admin/payment-methods/exchange-rate',

  // ===== Node import =====
  NODES_IMPORT_URI_PREVIEW: '/admin/nodes/import-uri',
  NODES_IMPORT_URI_CONFIRM: '/admin/nodes/import-uri/confirm',

  // ===== Node management =====
  NODE_CONFIG_VERSIONS: (id: string) => `/admin/nodes/${id}/config-versions`,
  NODE_DEPLOY: (id: string) => `/admin/nodes/${id}/deploy`,
  NODE_ROLLBACK: (id: string) => `/admin/nodes/${id}/rollback`,
  NODE_VALIDATE: '/admin/nodes/validate',
  NODE_REALITY_KEYPAIR: '/admin/nodes/reality-keypair',

  // ===== Runtimes =====
  RUNTIMES: '/admin/runtimes',

  // ===== 配置模板 (node-service) =====
  // 后端使用 PUT /admin/config-templates/:code 做 upsert，没有 POST 创建接口
  CONFIG_TEMPLATES: '/admin/config-templates',
  CONFIG_TEMPLATE_UPSERT: (code: string) => `/admin/config-templates/${code}`,
  CONFIG_TEMPLATE_RENDER: (code: string) => `/admin/config-templates/${code}/render`,

  // ===== 路由规则集 (node-service) =====
  ROUTE_RULE_SETS: '/admin/route-rule-sets',
  ROUTE_RULE_SET: (id: string) => `/admin/route-rule-sets/${id}`,
  ROUTE_RULE_SET_SYNC: (id: string) => `/admin/route-rule-sets/${id}/sync`,

  // ===== 路由策略 (node-service) =====
  ROUTE_POLICIES: '/admin/route-policies',
  ROUTE_POLICY: (id: string) => `/admin/route-policies/${id}`,
  ROUTE_POLICY_RULES: (id: string) => `/admin/route-policies/${id}/rules`,
  ROUTE_POLICY_REORDER: (id: string) => `/admin/route-policies/${id}/reorder`,
  ROUTE_POLICY_CLONE: (id: string) => `/admin/route-policies/${id}/clone`,
  ROUTE_POLICY_RULE: (ruleId: string) => `/admin/route-policy-rules/${ruleId}`,

  // ===== 客户端兼容 (subscription-service) =====
  // 注意：后端仅提供 GET /admin/client-profiles，无 POST/PATCH/DELETE
  CLIENT_PROFILES: '/admin/client-profiles',
  CLIENT_COMPAT_MATRIX: '/admin/client-compat-matrix',
  CLIENT_COMPAT_MATRIX_SYNC: '/admin/client-compat-matrix/sync',

  // ===== 订阅模板 - 按 code+target_client 索引 (subscription-service) =====
  SUB_TEMPLATES: '/admin/subscription/templates',
  SUB_TEMPLATE_SET_DEFAULT: (id: string) => `/admin/subscription/templates/${id}/default`,

  // ===== 订阅模板 - 按名称索引 (subscription-service, admin_template_handler) =====
  // 对齐 Xboard subscribe_template('clash') helper，渲染器按内核名取模板内容
  SUBSCRIBE_TEMPLATES: '/admin/subscribe/templates',
  SUBSCRIBE_TEMPLATE_DETAIL: (id: string) => `/admin/subscribe/templates/${id}`,
  SUBSCRIBE_TEMPLATES_RELOAD: '/admin/subscribe/templates/reload',

  // ===== 邮件模板 (identity-service) =====
  // 列表返回全量（不分页），PUT 仅更新 subject+body，无单独详情/启用切换接口
  MAIL_TEMPLATES: '/admin/mail/templates',
  MAIL_TEMPLATE_DETAIL: (id: string) => `/admin/mail/templates/${id}`,
  MAIL_TEMPLATES_RELOAD: '/admin/mail/templates/reload',
  // 测试发送：POST /admin/mail/test，body={ to, subject, body }
  MAIL_TEST_SEND: '/admin/mail/test',

  // ===== 订阅访问统计 (subscription-service) =====
  SUB_ACCESS_OVERVIEW: '/admin/subscription/access-overview',
  SUB_ACCESS_LOGS: '/admin/subscription/access-logs',

  // ===== 订阅Token管理 (subscription-service) =====
  SUB_TOKENS: '/admin/subscription-tokens',

  // ===== 节点 / 服务器 (node-service) =====
  NODES: '/admin/nodes',
  NODE_DETAIL: (id: string) => `/admin/nodes/${id}`,
  SERVERS: '/admin/servers',
  SERVER_DETAIL: (id: string) => `/admin/servers/${id}`,
  SERVER_TOKEN: (id: string) => `/admin/servers/${id}/token`,
  SERVER_LOGS: (id: string) => `/admin/servers/${id}/logs`,
  SERVER_RUNTIMES: (serverId: string) => `/admin/servers/${serverId}/runtimes`,
  SERVER_RUNTIME_UPGRADES: (serverId: string) => `/admin/servers/${serverId}/runtime-upgrades`,
  SERVER_RUNTIME_UPGRADE: (serverId: string) => `/admin/servers/${serverId}/runtime-upgrade`,
  RUNTIME_UPGRADE_DETAIL: (taskId: string) => `/admin/runtime-upgrades/${taskId}`,
  RUNTIME_UPGRADES_BATCH: '/admin/runtime-upgrades/batch',
  RUNTIME_UPGRADES_CANARY: '/admin/runtime-upgrades/canary',
  RUNTIME_UPGRADE_ROLLBACK: (taskId: string) => `/admin/runtime-upgrades/${taskId}/rollback`,

  // ===== 出站策略 / WARP (node-service) =====
  NODE_OUTBOUND_POLICIES: (nodeId: string) => `/admin/nodes/${nodeId}/outbound-policies`,
  NODE_OUTBOUND_POLICY: (nodeId: string, pid: string) => `/admin/nodes/${nodeId}/outbound-policies/${pid}`,
  NODE_OUTBOUND_GROUPS: (nodeId: string) => `/admin/nodes/${nodeId}/outbound-groups`,
  WARP_PROFILES: '/admin/warp-profiles',

  // ===== 代理链 (node-service) =====
  PROXY_CHAINS: '/admin/proxy-chains',
  PROXY_CHAIN: (id: string) => `/admin/proxy-chains/${id}`,
  PROXY_CHAIN_HOPS: (id: string) => `/admin/proxy-chains/${id}/hops`,
  PROXY_CHAIN_HOP: (id: string, index: number) => `/admin/proxy-chains/${id}/hops/${index}`,
  PROXY_CHAIN_BIND: (id: string) => `/admin/proxy-chains/${id}/bind`,
  PROXY_CHAIN_BIND_NODE: (id: string, nodeId: string) => `/admin/proxy-chains/${id}/bind/${nodeId}`,

  // ===== 节点会员分组 (node-service) =====
  // 提供 CRUD + 批量绑定/解绑节点 + 不分页全量列表
  NODE_GROUPS: '/admin/node-groups',
  NODE_GROUPS_ALL: '/admin/node-groups/all',
  NODE_GROUP_DETAIL: (id: string) => `/admin/node-groups/${id}`,
  NODE_GROUP_NODES: (id: string) => `/admin/node-groups/${id}/nodes`,
  NODE_GROUP_BIND_NODES: (id: string) => `/admin/node-groups/${id}/bind-nodes`,
  NODE_GROUP_UNBIND_NODES: (id: string) => `/admin/node-groups/${id}/unbind-nodes`,

  // ===== 节点分组负载均衡 (node-service) =====
  NODE_GROUP_LB_POLICY: (groupId: string) => `/admin/node-groups/${groupId}/lb-policy`,

  // ===== 节点路由绑定 (node-service) =====
  NODE_ROUTE_BINDINGS: (nodeId: string) => `/admin/nodes/${nodeId}/route-bindings`,
  NODE_ROUTE_BINDING: (nodeId: string, policyId: string) => `/admin/nodes/${nodeId}/route-bindings/${policyId}`,
  NODE_ROUTING_CONFIG: (nodeId: string) => `/admin/nodes/${nodeId}/routing-config`,

  // ===== 用户 (identity-service, 经 api-gateway 代理) =====
  USERS: '/admin/users',
  USER_DETAIL: (id: string) => `/admin/users/${id}`,
  USER_BAN: (id: string) => `/admin/users/${id}/ban`,
  USER_UNBAN: (id: string) => `/admin/users/${id}/unban`,
  USER_RESET_PASSWORD: (id: string) => `/admin/users/${id}/reset-password`,
  USER_RESET_TRAFFIC: (id: string) => `/admin/users/${id}/reset-traffic`,
  USER_ADD_TRAFFIC: (id: string) => `/admin/users/${id}/add-traffic`,
  USER_EXTEND: (id: string) => `/admin/users/${id}/extend`,
  USER_CHANGE_PLAN: (id: string) => `/admin/users/${id}/change-plan`,
  USER_RESET_SUB: (id: string) => `/admin/users/${id}/subscription/reset`,
  USER_IMPERSONATE: (id: string) => `/admin/users/${id}/impersonate`,

  // ===== Batch user operations =====
  USERS_BATCH_BAN: '/admin/users/batch/ban',
  USERS_BATCH_UNBAN: '/admin/users/batch/unban',
  USERS_BATCH_RESET_TRAFFIC: '/admin/users/batch/reset-traffic',
  USERS_BATCH_DELETE: '/admin/users/batch/delete',

  // ===== Phase 6: 工单 (identity-service) =====
  TICKETS: '/admin/tickets',
  TICKET_DETAIL: (id: string) => `/admin/tickets/${id}`,
  TICKET_STATS: '/admin/tickets/stats',
  TICKET_ASSIGN: (id: string) => `/admin/tickets/${id}/assign`,
  TICKET_REPLIES: (id: string) => `/admin/tickets/${id}/replies`,

  // ===== Phase 6: 公告 (identity-service) =====
  ANNOUNCEMENTS: '/admin/announcements',
  ANNOUNCEMENT_DETAIL: (id: string) => `/admin/announcements/${id}`,
  ANNOUNCEMENT_STATS: '/admin/announcements/stats',
  ANNOUNCEMENT_PUBLISH: (id: string) => `/admin/announcements/${id}/publish`,
  ANNOUNCEMENT_ARCHIVE: (id: string) => `/admin/announcements/${id}/archive`,
  ANNOUNCEMENT_READ: (id: string) => `/admin/announcements/${id}/read`,

  // ===== 返利/提现管理 (identity-service) =====
  COMMISSION_WITHDRAWALS: '/admin/commissions/withdrawals',
  COMMISSION_WITHDRAWAL_APPROVE: (id: string) => `/admin/commissions/withdrawals/${id}/approve`,
  COMMISSION_WITHDRAWAL_REJECT: (id: string) => `/admin/commissions/withdrawals/${id}/reject`,
  // ===== Phase 6: 通知 (identity-service) =====
  NOTIFICATIONS: '/admin/notifications',
  NOTIFICATION_DETAIL: (id: string) => `/admin/notifications/${id}`,
  NOTIFICATION_READ: (id: string) => `/admin/notifications/${id}/read`,
  NOTIFICATION_ARCHIVE: (id: string) => `/admin/notifications/${id}/archive`,
  NOTIFICATION_TEMPLATES: '/admin/notification-templates',
  NOTIFICATION_TEMPLATE_DETAIL: (code: string) => `/admin/notification-templates/${code}`,
  NOTIFICATION_TEMPLATE_ENABLE: (code: string) => `/admin/notification-templates/${code}/enabled`,

  // ===== 通道健康 (node-service) =====
  CHANNEL_HEALTH: '/admin/channels/health',
  CHANNEL_HEALTH_DETAIL: (serverId: string) => `/admin/channels/health/${serverId}`,
  CHANNEL_HEALTH_SNAPSHOTS: (serverId: string) => `/admin/channels/health/${serverId}/snapshots`,
  CHANNEL_FAILOVER_EVENTS: '/admin/channels/failover-events',
  CHANNEL_SWITCH: '/admin/channels/switch',

  // ===== AI 诊断 (node-service) =====
  DIAGNOSIS_SESSIONS: '/admin/diagnosis/sessions',
  DIAGNOSIS_SESSION_DETAIL: (id: string) => `/admin/diagnosis/sessions/${id}`,
  DIAGNOSIS_AUTOFIX: (id: string) => `/admin/diagnosis/sessions/${id}/autofix`,
  DIAGNOSIS_KNOWLEDGE: '/admin/diagnosis/knowledge',

  // ===== 节点体验 (node-service) =====
  EXPERIENCE_SCORES: '/admin/experience/scores',
  EXPERIENCE_SCORE_DETAIL: (nodeId: string) => `/admin/experience/scores/${nodeId}`,
  EXPERIENCE_SCORE_HISTORY: (nodeId: string) => `/admin/experience/scores/${nodeId}/history`,
  EXPERIENCE_RECALCULATE: '/admin/experience/recalculate',
  EXPERIENCE_CONFIG: '/admin/experience/config',

  // ===== 发布批次 (node-service) =====
  DEPLOYMENTS: '/admin/deployments',
  DEPLOYMENT_PUBLISH: '/admin/deployments/publish',
  DEPLOYMENT_ROLLBACK: (id: string) => `/admin/deployments/${id}/rollback`,
  DEPLOYMENT_RESULTS: (id: string) => `/admin/deployments/${id}/results`,
  DEPLOYMENT_DIFF: (id: string) => `/admin/deployments/${id}/diff`,
  DEPLOYMENT_DRY_RUN: '/admin/deployments/dry-run',
  DEPLOYMENT_REFRESH: '/admin/deployments/refresh',
  DEPLOYMENT_TARGET_RESULT: (targetId: string) => `/admin/deployments/targets/${targetId}/result`,
} as const
