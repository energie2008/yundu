export const EP = {
  AUTH_REGISTER: '/api/v1/user/auth/register',
  AUTH_SEND_EMAIL_CODE: '/api/v1/user/auth/send-email-code',
  AUTH_LOGIN: '/api/v1/user/auth/login',
  AUTH_LOGOUT: '/api/v1/user/auth/logout',
  AUTH_REFRESH: '/api/v1/user/auth/refresh',
  AUTH_FORGOT_PASSWORD: '/api/v1/user/auth/forgot-password',
  AUTH_RESET_PASSWORD: '/api/v1/user/auth/reset-password',
  AUTH_CHANGE_PASSWORD: '/api/v1/user/auth/change-password',
  ME: '/api/v1/user/me',
  UPDATE_ME: '/api/v1/user/me',
  SUBSCRIPTION: '/api/v1/user/subscription',
  SUBSCRIPTION_TOKENS: '/api/v1/user/subscription/tokens',
  SUBSCRIPTION_CREATE_TOKEN: '/api/v1/user/subscription/tokens',
  SUBSCRIPTION_RESET_TOKEN: (id: string) => `/api/v1/user/subscription/tokens/${id}/reset`,
  SUBSCRIPTION_REVOKE_TOKEN: (id: string) => `/api/v1/user/subscription/tokens/${id}`,
  SUBSCRIPTION_RESET: '/api/v1/user/subscription/reset',
  PLANS: '/api/v1/plans',
  PLANS_GUEST: '/api/v1/plans',
  PLAN_DETAIL: (id: string) => `/api/v1/plans/${id}`,
  ORDERS: '/api/v1/user/orders',
  ORDER_DETAIL: (id: string) => `/api/v1/user/orders/${id}`,
  ORDER_CREATE: '/api/v1/user/orders',
  COUPON_VALIDATE: '/api/v1/coupons/validate',
  PAYMENT_METHODS: '/api/v1/user/payment-methods',
  GUEST_CONFIG: '/api/v1/guest/config',
  PLAN_NODES: (id: string) => `/api/v1/plans/${id}/nodes`,
  MY_NODES: '/api/v1/me/nodes',
  TICKETS: '/api/v1/me/tickets',
  TICKET_DETAIL: (id: string) => `/api/v1/user/tickets/${id}`,
  TICKET_REPLIES: (id: string) => `/api/v1/user/tickets/${id}/replies`,
  TICKET_ADD_REPLY: (id: string) => `/api/v1/user/tickets/${id}/replies`,
  NOTIFICATIONS: '/api/v1/me/notifications',
  NOTIFICATION_READ: (id: string) => `/api/v1/me/notifications/${id}/read`,
  NOTIFICATIONS_READ_ALL: '/api/v1/me/notifications/read-all',
  NOTIFICATIONS_UNREAD_COUNT: '/api/v1/me/notifications/unread-count',
  PREFERENCES: '/api/v1/user/preferences',
  // 用户端公告（参考 XBoard /api/v1/user/notice/fetch）
  ANNOUNCEMENTS: '/api/v1/me/announcements',
  ANNOUNCEMENT_DETAIL: (id: string) => `/api/v1/me/announcements/${id}`,
  ANNOUNCEMENT_READ: (id: string) => `/api/v1/me/announcements/${id}/read`,
  EMAIL_VERIFY: '/api/v1/me/verify-email',
  COMMISSION_SUMMARY: '/api/v1/user/commissions/summary',
  COMMISSION_WITHDRAW: '/api/v1/user/commissions/withdraw',
  COMMISSION_WITHDRAWALS: '/api/v1/user/commissions/withdrawals',
  COMMISSION_DETAILS: '/api/v1/user/commissions/details',
  INVITATIONS: '/api/v1/me/invitations',
  INVITE_CODE: '/api/v1/me/invite-code',
  TRAFFIC_LOGS: '/api/v1/me/traffic-logs',
  KNOWLEDGE_CATEGORIES: '/api/v1/me/knowledge/categories',
  KNOWLEDGE_ARTICLES: '/api/v1/me/knowledge/articles',
  KNOWLEDGE_ARTICLE: (id: string | number) => `/api/v1/me/knowledge/articles/${id}`,
  ENSURE_TOKEN: '/api/v1/user/subscription/token/ensure',
  SUB_SHORT_CODE: (token: string) => `/api/v1/user/sub/${token}/short`,
  SUB_STATS: '/api/v1/user/subscription/stats',
} as const

// 订阅服务基础地址：必须固定为订阅服务域名（API 网关域名），
// 不能用 window.location.origin（user-web 域名与订阅服务域名不同，且 dev/生产 环境会变化导致订阅地址不稳定）
// 优先级：VITE_SUB_BASE 环境变量 > 硬编码默认值
export const SUB_BASE = import.meta.env.VITE_SUB_BASE || 'https://ad.tiktokplay.na.am'

export function getSubscriptionUrl(token: string, client?: string): string {
  const base = `${SUB_BASE}/sub/${token}`
  return client ? `${base}?client=${encodeURIComponent(client)}` : base
}

export function getShortUrl(shortCode: string): string {
  return `${SUB_BASE}/s/${shortCode}`
}

export type DetectedClient = {
  id: string
  name: string
  scheme: string
  icon?: string
}

const CLIENT_SCHEMES: Record<string, DetectedClient> = {
  clash: { id: 'clash', name: 'Clash for Windows', scheme: 'clash://install-config?url=' },
  clashmeta: { id: 'clashmeta', name: 'Clash Meta', scheme: 'clash://install-config?url=' },
  mihomo: { id: 'mihomo', name: 'Mihomo Party', scheme: 'clash://install-config?url=' },
  'clash-verge': { id: 'clash-verge', name: 'Clash Verge', scheme: 'clash://install-config?url=' },
  shadowrocket: { id: 'shadowrocket', name: 'Shadowrocket', scheme: 'shadowrocket://add/sub?url=' },
  singbox: { id: 'singbox', name: 'Sing-box', scheme: 'sing-box://import-remote-profile?url=' },
  sfi: { id: 'sfi', name: 'SFI', scheme: 'sing-box://import-remote-profile?url=' },
  sfa: { id: 'sfa', name: 'SFA', scheme: 'sing-box://import-remote-profile?url=' },
  v2rayn: { id: 'v2rayn', name: 'v2rayN', scheme: 'v2rayng://install-sub?url=' },
  v2rayng: { id: 'v2rayng', name: 'v2rayNG', scheme: 'v2rayng://install-sub?url=' },
  quantumult: { id: 'quantumult', name: 'Quantumult', scheme: 'quantumult://configuration?sub=' },
  quantumultx: { id: 'quantumultx', name: 'Quantumult X', scheme: 'quantumult-x:///update-configuration?remote-resource=' },
  surge: { id: 'surge', name: 'Surge', scheme: 'surge:///install-config?url=' },
  loon: { id: 'loon', name: 'Loon', scheme: 'loon://import?sub=' },
  stash: { id: 'stash', name: 'Stash', scheme: 'stash://install-config?url=' },
  karing: { id: 'karing', name: 'Karing', scheme: 'clash://install-config?url=' },
}

export function detectClientFromUA(): DetectedClient | null {
  if (typeof navigator === 'undefined') return null
  const ua = navigator.userAgent.toLowerCase()
  const mappings: [string, string][] = [
    ['clash verge', 'clash-verge'],
    ['clashverge', 'clash-verge'],
    ['mihomo', 'mihomo'],
    ['clash meta', 'clashmeta'],
    ['clash-meta', 'clashmeta'],
    ['cfw', 'clash'],
    ['clash for windows', 'clash'],
    ['clashx', 'clash'],
    ['shadowrocket', 'shadowrocket'],
    ['sing-box', 'singbox'],
    ['sfi', 'sfi'],
    ['sfa', 'sfa'],
    ['v2rayn', 'v2rayn'],
    ['v2rayng', 'v2rayng'],
    ['v2rayng', 'v2rayng'],
    ['nekobox', 'v2rayn'],
    ['quantumult x', 'quantumultx'],
    ['quantumultx', 'quantumultx'],
    ['quantumult', 'quantumult'],
    ['surge', 'surge'],
    ['loon', 'loon'],
    ['stash', 'stash'],
    ['karing', 'karing'],
  ]
  for (const [keyword, id] of mappings) {
    if (ua.includes(keyword)) {
      return CLIENT_SCHEMES[id] || null
    }
  }
  return null
}

export function getOneClickImportUrl(clientId: string, subscriptionUrl: string): string | null {
  const client = CLIENT_SCHEMES[clientId]
  if (!client) return null
  return `${client.scheme}${encodeURIComponent(subscriptionUrl)}`
}

export const ALL_IMPORT_CLIENTS: DetectedClient[] = [
  CLIENT_SCHEMES.clash,
  CLIENT_SCHEMES.shadowrocket,
  CLIENT_SCHEMES.singbox,
  CLIENT_SCHEMES.v2rayn,
  CLIENT_SCHEMES.quantumultx,
  CLIENT_SCHEMES.surge,
  CLIENT_SCHEMES.loon,
  CLIENT_SCHEMES.stash,
].filter(Boolean)

export const TOKEN_KEYS = {
  ACCESS: 'yundu_access_token',
  REFRESH: 'yundu_refresh_token',
} as const

export interface TokenResponse {
  access_token: string
  refresh_token: string
  token_type: string
  expires_in: number
}

export interface LoginResponse {
  token: TokenResponse
  user: UserResponse
}

export interface RegisterResponse {
  user_id: string
  requires_verification: boolean
}

export interface UserResponse {
  id: string
  email: string
  username?: string | null
  avatar_url?: string | null
  status: string
  email_verified: boolean
  locale: string
  timezone: string
  balance?: number
  commission_balance?: number
  commission_total?: number
  notify_expiry?: boolean
  notify_traffic?: boolean
  notify_ticket_reply?: boolean
  created_at: string
  last_login_at?: string | null
  subscription?: SubscriptionResponse | null
  active_plan_name?: string | null
}

export interface NotificationPreferences {
  notify_expiry: boolean
  notify_traffic: boolean
  notify_ticket_reply: boolean
}

export interface UserDetailResponse extends UserResponse {
  profile?: UserProfile | null
}

export interface UserProfile {
  user_id: string
  avatar_url?: string | null
  contact_email?: string | null
  phone?: string | null
  country_code?: string | null
  tags: string[]
  metadata?: Record<string, unknown> | null
  updated_at: string
}

export interface SubscriptionResponse {
  id: string
  user_id: string
  plan_id: string
  plan_name: string
  status: string
  started_at: string
  expires_at?: string | null
  traffic_quota_bytes: number
  traffic_used_bytes: number
  upload_bytes?: number
  download_bytes?: number
  speed_limit_mbps: number
  device_limit: number
  reset_cycle?: string
  reset_at?: string | null
  token?: string
  price?: number | null
}

export interface SubscriptionTokenResponse {
  id: string
  token?: string
  token_preview: string
  client_hint?: string | null
  status: string
  bound_ip?: string | null
  last_access_at?: string | null
  last_access_ip?: string | null
  expires_at?: string | null
  created_at: string
}

export interface PlanPrice {
  period_code: string
  price_usdt: number
  price_cny: number
}

export interface PlanResponse {
  id: string
  code: string
  name: string
  description?: string
  content?: string
  status: string
  billing_type: string
  traffic_bytes: number
  speed_limit_mbps?: number
  device_limit?: number
  reset_cycle?: string
  duration_days?: number
  features?: string[]
  feature_flags?: Record<string, unknown>
  prices: PlanPrice[]
  node_count?: number
  created_at: string
}

export interface OrderResponse {
  id: string
  order_no: string
  plan_id: string
  plan_name: string
  period_code: string
  period_label?: string
  amount_usdt: number
  amount_cny: number
  exchange_rate: number
  discount_amount?: number
  final_amount?: number
  coupon_code?: string
  pay_address?: string
  pay_currency?: string
  payment_method?: string
  payment_uri?: string
  status: 'pending' | 'paid' | 'expired' | 'canceled'
  tx_hash?: string | null
  paid_amount?: number | null
  paid_at?: string | null
  expires_at: string
  created_at: string
}

export interface CouponValidateRequest {
  coupon_code: string
  plan_id: string
  period_code: string
  amount_cny: number
}

export interface CouponValidateResponse {
  valid: boolean
  coupon_code: string
  discount_type: 'percentage' | 'fixed'
  discount_amount: number
  discount_value?: number
  final_amount?: number
}

export interface PaginatedResponse<T> {
  page: number
  page_size: number
  total: number
  items: T[]
}

export interface TrafficLog {
  date: string
  upload: number
  download: number
  total: number
}

export interface NodeInfo {
  id: string
  numeric_id?: number
  name: string
  country_code?: string
  country_flag?: string
  region_code?: string
  rate?: number
  protocol_type?: string
  protocol?: string
  tags?: string[]
  is_online: boolean
}

export interface TicketResponse {
  id: string
  ticket_no: string
  subject: string
  status: string
  priority?: string
  created_at: string
  updated_at?: string
  last_message_at?: string
  last_reply_at?: string
  message?: string
  unread_count?: number
}

export interface TicketReplyResponse {
  id: string
  ticket_id: string
  user_id?: string
  admin_id?: string
  author_name?: string
  is_admin: boolean
  content: string
  created_at: string
}

export interface NotificationItem {
  id: string
  type: string
  title: string
  content: string
  is_read: boolean
  created_at: string
  link?: string | null
}

export interface AnnouncementItem {
  id: string
  title: string
  summary?: string
  content?: string
  is_read: boolean
  is_pinned?: boolean
  published_at: string
  created_at?: string
  author?: string | null
}

export interface CommissionSummary {
  available_balance: number
  total_earned: number
  pending_settlement: number
  invited_count: number
  withdrawn_total: number
  /** Commission rate as integer percent (e.g. 20 = 20%). Backend returns int. */
  rate: number
  min_withdraw: number
  withdraw_enabled: boolean
}

export interface WithdrawResponse {
  id: string
  amount: number
  method: string
  account: string
  real_name?: string
  status: number
  remark?: string
  created_at: string
}

export interface InviteSummary {
  invited_count: number
  settled_commission: number
  pending_commission: number
  commission_rate: number
  available_balance: number
}

export interface InviteCode {
  id: string
  code: string
  used_count: number
  created_at: string
}

// 佣金明细记录（对齐 xboard CommissionLog）
export interface CommissionDetail {
  id: string
  invitee_id: string
  order_id?: string | null
  trade_no?: string | null
  order_amount: number
  get_amount: number
  commission_balance: number
  status: number       // 0=待结算 1=已结算 2=已取消
  status_text: string
  created_at: string
}

// 被邀请用户（对齐 xboard invite details）
export interface InvitationItem {
  id: string
  email: string
  username?: string | null
  status: string
  email_verified: boolean
  registered_at?: string | null
  created_at: string
}

export interface DocArticle {
  id: string
  title: string
  slug: string
  sort_order: number
}

export interface DocCategory {
  id: string
  title: string
  sort_order: number
  articles: DocArticle[]
}

export interface PaymentMethod {
  method: string
  name: string
  icon?: string
  currency: string
  enabled: boolean
  fiat: boolean
}

export interface PaymentMethodsResponse {
  methods: PaymentMethod[]
  exchange_rate: number
  base_currency: string
}

export function adaptUser(raw: UserDetailResponse): UserResponse {
  return {
    id: raw.id,
    email: raw.email,
    username: raw.username ?? null,
    avatar_url: raw.profile?.avatar_url ?? null,
    status: raw.status,
    email_verified: raw.email_verified,
    locale: raw.locale,
    timezone: raw.timezone,
    balance: raw.balance ?? 0,
    commission_balance: raw.commission_balance ?? 0,
    commission_total: raw.commission_total ?? 0,
    notify_expiry: raw.notify_expiry ?? true,
    notify_traffic: raw.notify_traffic ?? true,
    notify_ticket_reply: raw.notify_ticket_reply ?? true,
    created_at: raw.created_at,
    last_login_at: raw.last_login_at ?? null,
    subscription: raw.subscription ? adaptSubscription(raw.subscription) : null,
    active_plan_name: raw.subscription?.plan_name ?? null,
  }
}

export function adaptPlan(raw: PlanResponse): PlanResponse {
  const prices = (raw.prices || []).map(p => ({
    ...p,
    price_cny: p.price_cny,
  }))
  // 优先使用 feature_flags.description（admin 表单保存位置），否则用顶层 description
  let description = ''
  if (raw.feature_flags && typeof raw.feature_flags.description === 'string' && raw.feature_flags.description.trim()) {
    description = raw.feature_flags.description
  } else if (raw.description) {
    description = raw.description
  }
  // features 从 description 按行分割（保留 admin 输入的 ✅ 等格式）
  let features: string[] = []
  if (description) {
    features = description.split('\n').map(line => line.trim()).filter(line => line.length > 0)
  } else {
    features = raw.features || []
  }
  return {
    ...raw,
    prices,
    description,
    features,
    node_count: raw.node_count ?? 0,
  }
}

export function adaptSubscription(raw: SubscriptionResponse): SubscriptionResponse {
  return {
    ...raw,
    upload_bytes: raw.upload_bytes ?? 0,
    download_bytes: raw.download_bytes ?? raw.traffic_used_bytes,
    reset_cycle: raw.reset_cycle ?? 'monthly',
  }
}

export function adaptSubscriptionToken(raw: SubscriptionTokenResponse): SubscriptionTokenResponse {
  return raw
}

export function adaptOrder(raw: OrderResponse): OrderResponse {
  return {
    ...raw,
    amount_cny: raw.amount_cny,
    exchange_rate: raw.exchange_rate,
    period_label: getPeriodLabel(raw.period_code),
    pay_address: raw.pay_address || (raw.status === 'pending' ? 'TLAoiTwPNCtFXpJWmPrvgm8tRpC9ggP42H' : undefined),
    pay_currency: raw.pay_currency || 'USDT_TRC20',
  }
}

export function adaptOrdersPage(raw: PaginatedResponse<OrderResponse>): PaginatedResponse<OrderResponse> {
  return { ...raw, items: raw.items.map(adaptOrder) }
}

export function bytesToGB(bytes: number): number {
  return bytes / (1024 * 1024 * 1024)
}

export function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const gb = bytesToGB(bytes)
  if (gb >= 1) return `${gb.toFixed(2)} GB`
  const mb = bytes / (1024 * 1024)
  if (mb >= 1) return `${mb.toFixed(2)} MB`
  const kb = bytes / 1024
  return `${kb.toFixed(0)} KB`
}

export function formatCNY(amount: number): string {
  return `¥${amount.toFixed(2)}`
}

export function formatUSDT(amount: number): string {
  return `$${amount.toFixed(2)}`
}

export function formatDate(dateStr: string): string {
  const date = new Date(dateStr)
  return date.toLocaleDateString('zh-CN', {
    year: 'numeric',
    month: 'numeric',
    day: 'numeric',
  })
}

export function formatDateTime(dateStr: string): string {
  const date = new Date(dateStr)
  return date.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export function formatTimeAgo(dateStr: string): string {
  const date = new Date(dateStr)
  const now = new Date()
  const diff = Math.floor((now.getTime() - date.getTime()) / 1000)
  if (diff < 60) return `${diff}秒前`
  if (diff < 3600) return `${Math.floor(diff / 60)}分钟前`
  if (diff < 86400) return `${Math.floor(diff / 3600)}小时前`
  if (diff < 604800) return `${Math.floor(diff / 86400)}天前`
  return date.toLocaleDateString('zh-CN')
}

export function getDaysRemaining(expiresAt?: string | null): number | null {
  if (!expiresAt) return null
  const now = new Date().getTime()
  const exp = new Date(expiresAt).getTime()
  const days = Math.ceil((exp - now) / (1000 * 60 * 60 * 24))
  return days > 0 ? days : 0
}

export function getTrafficPercentage(used: number, quota: number): number {
  if (quota <= 0) return 0
  return Math.min(Math.round((used / quota) * 100), 100)
}

export function getPeriodLabel(code: string): string {
  const labels: Record<string, string> = {
    onetime: '一次性',
    month: '月付',
    quarter: '季付',
    half_year: '半年付',
    year: '年付',
    monthly: '月付',
    quarterly: '季付',
    half_yearly: '半年付',
    yearly: '年付',
    two_yearly: '两年付',
    three_yearly: '三年付',
  }
  return labels[code] || code
}

export function getPeriodDays(code: string): number {
  const days: Record<string, number> = {
    onetime: 0,
    month: 30,
    quarter: 90,
    half_year: 180,
    year: 365,
    monthly: 30,
    quarterly: 90,
    half_yearly: 180,
    yearly: 365,
    two_yearly: 730,
    three_yearly: 1095,
  }
  return days[code] || 30
}

export function getStatusLabel(status: string): string {
  const labels: Record<string, string> = {
    active: '正常',
    pending: '待支付',
    paid: '已完成',
    expired: '已过期',
    canceled: '已取消',
    disabled: '已禁用',
    banned: '已封禁',
    pending_email: '待验证邮箱',
    open: '处理中',
    replied: '已回复',
    closed: '已关闭',
  }
  return labels[status] || status
}
