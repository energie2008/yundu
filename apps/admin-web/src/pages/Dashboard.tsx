import { useState, useEffect } from 'react'
import {
  Activity,
  Users,
  Database,
  Wifi,
  Server,
  CreditCard,
  AlertCircle,
  ShieldCheck,
  Zap,
  Globe,
  Ticket,
  ArrowRight,
  BarChart3,
  DollarSign,
} from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle, Badge, Button, Skeleton } from '@airport/ui'
import { api } from '@/lib/api'
import { EP } from '@/lib/endpoints'
import { ADMIN_CARD, ADMIN_BORDER, ADMIN_TEXT, ADMIN_TEXT_SECONDARY, ADMIN_TEXT_MUTED, ADMIN_ACCENT, ADMIN_GRADIENT, ADMIN_SUCCESS, ADMIN_WARNING, ADMIN_DANGER, ADMIN_INFO, ADMIN_BG } from '@/lib/theme'

interface TrafficOverview {
  today_upload: number
  today_download: number
  today_total: number
  online_count: number
  top_nodes?: Array<{ node_id: string; node_name: string; upload: number; download: number; total: number }>
}

interface RevenueStats {
  today_revenue_usdt: number
  today_revenue_cny: number
  today_order_count: number
  month_revenue_usdt: number
  month_revenue_cny: number
  month_order_count: number
}

interface AdminNode {
  id: string
  code: string
  name: string
  protocol_type: string
  transport_type: string
  address: string
  port: number
  is_enabled: boolean
  is_visible: boolean
  traffic_rate: number
  health_status?: string
  tags?: string[]
  created_at: string
}

interface AdminPlan {
  id: string
  code: string
  name: string
  status: 'draft' | 'active' | 'archived'
  billing_type: string
  traffic_bytes: number
  can_renew: boolean
  sort_order: number
  prices?: Record<string, number>
  tags?: string[]
  created_at: string
}

function formatBytes(bytes: number | string): string {
  const b = typeof bytes === 'string' ? parseInt(bytes, 10) : bytes
  if (isNaN(b)) return '0 B'
  if (b < 1024) return `${b} B`
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB`
  if (b < 1024 * 1024 * 1024) return `${(b / (1024 * 1024)).toFixed(1)} MB`
  if (b < 1024 * 1024 * 1024 * 1024) return `${(b / (1024 * 1024 * 1024)).toFixed(2)} GB`
  return `${(b / (1024 * 1024 * 1024 * 1024)).toFixed(2)} TB`
}

function KpiCard({
  title,
  value,
  subValue,
  icon: Icon,
  iconBg,
  iconColor,
  loading,
}: {
  title: string
  value: string | number
  subValue?: string
  icon: React.ComponentType<{ className?: string }>
  iconBg: string
  iconColor: string
  loading?: boolean
}) {
  return (
    <div
      className="relative overflow-hidden rounded-2xl p-5 transition-all hover:scale-[1.02]"
      style={{ backgroundColor: ADMIN_CARD, border: `1px solid ${ADMIN_BORDER}`, boxShadow: 'var(--shadow-sm)' }}
    >
      <div className="absolute -right-4 -top-4 w-24 h-24 rounded-full opacity-5" style={{ background: iconBg }} />
      <div className="flex items-start justify-between mb-4">
        <div className="p-2.5 rounded-xl" style={{ backgroundColor: iconBg }}>
          <Icon className={`w-5 h-5 ${iconColor}`} />
        </div>
      </div>
      <p className="text-sm font-medium" style={{ color: ADMIN_TEXT_MUTED }}>{title}</p>
      {loading ? (
        <Skeleton className="h-9 w-28 mt-2" />
      ) : (
        <div className="flex items-baseline gap-2 mt-1">
          <p className="text-2xl font-bold" style={{ color: ADMIN_TEXT }}>{value}</p>
          {subValue && <span className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>{subValue}</span>}
        </div>
      )}
    </div>
  )
}

function QuickLink({ to, icon: Icon, label, desc, iconBg, iconColor }: { to: string; icon: React.ComponentType<{ className?: string }>; label: string; desc: string; iconBg: string; iconColor: string }) {
  return (
    <a href={to} className="flex items-center gap-3 p-3 rounded-xl transition-all group cursor-pointer hover:scale-[1.01]" style={{ backgroundColor: 'var(--muted)' }}>
      <div className="p-2 rounded-lg" style={{ backgroundColor: iconBg }}>
        <Icon className={`w-4 h-4 ${iconColor}`} />
      </div>
      <div className="flex-1 min-w-0">
        <p className="text-sm font-medium" style={{ color: ADMIN_TEXT }}>{label}</p>
        <p className="text-xs truncate" style={{ color: ADMIN_TEXT_MUTED }}>{desc}</p>
      </div>
      <ArrowRight className="w-4 h-4 transition-all group-hover:translate-x-0.5" style={{ color: ADMIN_TEXT_MUTED }} />
    </a>
  )
}

export default function Dashboard() {
  const [loading, setLoading] = useState(true)
  const [traffic, setTraffic] = useState<TrafficOverview | null>(null)
  const [nodes, setNodes] = useState<AdminNode[]>([])
  const [plans, setPlans] = useState<AdminPlan[]>([])
  const [revenue, setRevenue] = useState<RevenueStats | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    async function load() {
      try {
        setLoading(true)
        setError(null)
        const [trafficData, nodesResp, plansResp, revenueResp] = await Promise.all([
          api.get<TrafficOverview>(EP.TRAFFIC_OVERVIEW).catch(() => null),
          api.get<{ items: AdminNode[]; total: number }>(EP.NODES, { params: { page: 1, page_size: 200 } }).catch(() => ({ items: [], total: 0 })),
          api.get<{ items: AdminPlan[]; total: number }>(EP.PLANS, { params: { page: 1, page_size: 200 } }).catch(() => ({ items: [], total: 0 })),
          api.get<RevenueStats>(EP.ORDER_STATS).catch(() => null),
        ])
        setTraffic(trafficData)
        setNodes(nodesResp?.items || [])
        setPlans(plansResp?.items || [])
        setRevenue(revenueResp)
      } catch (e: any) {
        console.error('Dashboard load error:', e)
        setError(e.message || '加载失败')
      } finally {
        setLoading(false)
      }
    }
    load()
  }, [])

  const totalNodes = nodes.length
  const onlineCount = nodes.filter(n => n.is_enabled && n.health_status === 'healthy').length
  const offlineCount = totalNodes - onlineCount
  const visibleNodes = nodes.filter(n => n.is_visible).length
  const visiblePlans = plans.filter(p => p.status === 'active').length
  const todayUpload = traffic?.today_upload || 0
  const todayDownload = traffic?.today_download || 0
  const todayTotal = traffic?.today_total || 0
  const onlineUsers = traffic?.online_count || 0

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold" style={{ color: ADMIN_TEXT }}>仪表盘</h1>
          <p className="text-sm mt-1" style={{ color: ADMIN_TEXT_MUTED }}>系统运行状态概览与关键指标</p>
        </div>
        <div className="flex items-center gap-2 px-3 py-1.5 rounded-full" style={{ background: 'var(--accent-emerald)', border: '1px solid var(--accent-emerald)' }}>
          <span className="relative flex h-2 w-2">
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full opacity-75" style={{ backgroundColor: ADMIN_SUCCESS }}></span>
            <span className="relative inline-flex rounded-full h-2 w-2" style={{ backgroundColor: ADMIN_SUCCESS }}></span>
          </span>
          <span className="text-xs font-medium" style={{ color: 'var(--accent-emerald-foreground)' }}>系统运行正常</span>
        </div>
      </div>

      {/* Error */}
      {error && (
        <div className="rounded-2xl p-4 flex items-center gap-3" style={{ background: 'var(--accent-rose)', border: '1px solid var(--accent-rose)' }}>
          <AlertCircle className="w-5 h-5" style={{ color: ADMIN_DANGER }} />
          <p className="text-sm" style={{ color: 'var(--accent-rose-foreground)' }}>{error}</p>
        </div>
      )}

      {/* 营收概览（参考 xboard 仪表盘） */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <KpiCard
          title="当日营业收入"
          value={`¥${(revenue?.today_revenue_cny || 0).toFixed(2)}`}
          icon={DollarSign}
          iconBg="var(--accent-emerald)"
          iconColor="text-emerald-400"
          subValue={`$${(revenue?.today_revenue_usdt || 0).toFixed(2)} USDT`}
          loading={loading}
        />
        <KpiCard
          title="当日成交订单"
          value={revenue?.today_order_count || 0}
          icon={CreditCard}
          iconBg="var(--accent-sky)"
          iconColor="text-sky-400"
          subValue="笔已支付"
          loading={loading}
        />
        <KpiCard
          title="当月营业收入"
          value={`¥${(revenue?.month_revenue_cny || 0).toFixed(2)}`}
          icon={BarChart3}
          iconBg="var(--accent)"
          iconColor="text-indigo-400"
          subValue={`$${(revenue?.month_revenue_usdt || 0).toFixed(2)} USDT`}
          loading={loading}
        />
        <KpiCard
          title="当月成交订单"
          value={revenue?.month_order_count || 0}
          icon={Ticket}
          iconBg="var(--accent-pink)"
          iconColor="text-pink-400"
          subValue="笔已支付"
          loading={loading}
        />
      </div>

      {/* KPI Row 1 */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <KpiCard
          title="在线节点"
          value={`${onlineCount}/${totalNodes}`}
          icon={Server}
          iconBg="var(--accent-emerald)"
          iconColor="text-emerald-400"
          subValue="个节点"
          loading={loading}
        />
        <KpiCard
          title="订阅套餐"
          value={plans.length}
          icon={CreditCard}
          iconBg="var(--accent-sky)"
          iconColor="text-sky-400"
          subValue={`已上架 ${visiblePlans}`}
          loading={loading}
        />
        <KpiCard
          title="今日流量"
          value={formatBytes(todayTotal)}
          icon={Database}
          iconBg="var(--accent)"
          iconColor="text-indigo-400"
          subValue={`↑${formatBytes(todayUpload)} ↓${formatBytes(todayDownload)}`}
          loading={loading}
        />
        <KpiCard
          title="在线用户"
          value={onlineUsers}
          icon={Wifi}
          iconBg="var(--accent-pink)"
          iconColor="text-pink-400"
          subValue="当前在线"
          loading={loading}
        />
      </div>

      {/* KPI Row 2 */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <KpiCard
          title="隐藏节点"
          value={totalNodes - visibleNodes}
          icon={Server}
          iconBg="var(--accent-amber)"
          iconColor="text-amber-400"
          subValue="个不可见"
          loading={loading}
        />
        <KpiCard
          title="节点离线"
          value={offlineCount}
          icon={AlertCircle}
          iconBg={offlineCount > 0 ? 'var(--accent-rose)' : 'var(--accent-emerald)'}
          iconColor={offlineCount > 0 ? 'text-rose-400' : 'text-emerald-400'}
          subValue={offlineCount > 0 ? '需关注' : '全部正常'}
          loading={loading}
        />
        <KpiCard
          title="可见节点"
          value={visibleNodes}
          icon={Globe}
          iconBg="var(--accent)"
          iconColor="text-indigo-400"
          subValue="对外服务"
          loading={loading}
        />
        <KpiCard
          title="流量速率"
          value={`${nodes.reduce((s, n) => s + n.traffic_rate, 0).toFixed(1)}x`}
          icon={Zap}
          iconBg="var(--accent-sky)"
          iconColor="text-sky-400"
          subValue="平均倍率"
          loading={loading}
        />
      </div>

      {/* Main Content Grid */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        {/* Node Status */}
        <div className="lg:col-span-2 rounded-2xl overflow-hidden" style={{ backgroundColor: ADMIN_CARD, border: `1px solid ${ADMIN_BORDER}`, boxShadow: 'var(--shadow-sm)' }}>
          <div className="p-5 border-b" style={{ borderColor: ADMIN_BORDER }}>
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <div className="p-2 rounded-lg" style={{ backgroundColor: 'var(--accent-emerald)' }}>
                  <Server className="w-4 h-4" style={{ color: ADMIN_SUCCESS }} />
                </div>
                <h3 className="font-semibold" style={{ color: ADMIN_TEXT }}>节点状态</h3>
              </div>
              <div className="flex items-center gap-3 text-xs">
                <span className="flex items-center gap-1.5">
                  <span className="w-2 h-2 rounded-full" style={{ backgroundColor: ADMIN_SUCCESS }} />
                  <span style={{ color: ADMIN_TEXT_MUTED }}>在线 {onlineCount}</span>
                </span>
                <span className="flex items-center gap-1.5">
                  <span className="w-2 h-2 rounded-full" style={{ backgroundColor: 'var(--muted-foreground)' }} />
                  <span style={{ color: ADMIN_TEXT_MUTED }}>离线 {offlineCount}</span>
                </span>
                <a href="/nodes" className="flex items-center gap-1 px-2 py-1 rounded-lg transition-colors" style={{ color: ADMIN_ACCENT }}>
                  管理 <ArrowRight className="w-3 h-3" />
                </a>
              </div>
            </div>
          </div>
          <div className="p-5">
            {loading ? (
              <div className="space-y-2">
                {[1,2,3,4,5].map(i => <Skeleton key={i} className="h-12 w-full rounded-xl" />)}
              </div>
            ) : nodes.length === 0 ? (
              <div className="text-center py-10">
                <Globe className="w-10 h-10 mx-auto mb-3" style={{ color: ADMIN_TEXT_MUTED }} />
                <p className="text-sm" style={{ color: ADMIN_TEXT_MUTED }}>暂无节点，请先添加节点</p>
                <a href="/nodes">
                  <Button size="sm" className="mt-3" style={{ background: ADMIN_GRADIENT }}>添加节点</Button>
                </a>
              </div>
            ) : (
              <div className="space-y-1.5 max-h-80 overflow-y-auto pr-1">
                {nodes.map(node => {
                  const isOnline = node.is_enabled && node.health_status === 'healthy'
                  return (
                    <a
                      key={node.id}
                      href="/nodes"
                      className="flex items-center justify-between p-3 rounded-xl transition-all hover:scale-[1.01]"
                      style={{ backgroundColor: 'var(--muted)' }}
                    >
                      <div className="flex items-center gap-3 min-w-0">
                        <span className="relative flex h-2.5 w-2.5 flex-shrink-0">
                          {isOnline && (
                            <span className="animate-ping absolute inline-flex h-full w-full rounded-full opacity-50" style={{ backgroundColor: ADMIN_SUCCESS }}></span>
                          )}
                          <span className="relative inline-flex rounded-full h-2.5 w-2.5" style={{ backgroundColor: isOnline ? ADMIN_SUCCESS : (node.health_status === 'degraded' ? ADMIN_WARNING : 'var(--muted-foreground)') }}></span>
                        </span>
                        <div className="min-w-0">
                          <p className="text-sm font-medium truncate" style={{ color: ADMIN_TEXT }}>{node.name}</p>
                          <p className="text-xs font-mono truncate" style={{ color: ADMIN_TEXT_MUTED }}>
                            {node.protocol_type}/{node.transport_type} · {node.address}:{node.port} · {node.traffic_rate}x
                          </p>
                        </div>
                      </div>
                      <div className="flex items-center gap-2 flex-shrink-0">
                        {!node.is_visible && <Badge variant="outline" className="text-xs" style={{ borderColor: ADMIN_BORDER, color: ADMIN_TEXT_MUTED }}>隐藏</Badge>}
                        {!node.is_enabled && <Badge variant="outline" className="text-xs" style={{ borderColor: 'var(--accent-rose)', color: ADMIN_DANGER }}>禁用</Badge>}
                        <span className="text-xs font-mono px-2 py-0.5 rounded" style={{ backgroundColor: 'var(--accent)', color: 'var(--accent-foreground)' }}>
                          {node.code || node.id.substring(0, 8)}
                        </span>
                      </div>
                    </a>
                  )
                })}
              </div>
            )}
          </div>
        </div>

        {/* Quick Actions */}
        <div className="rounded-2xl overflow-hidden" style={{ backgroundColor: ADMIN_CARD, border: `1px solid ${ADMIN_BORDER}`, boxShadow: 'var(--shadow-sm)' }}>
          <div className="p-5 border-b" style={{ borderColor: ADMIN_BORDER }}>
            <div className="flex items-center gap-2">
              <div className="p-2 rounded-lg" style={{ backgroundColor: 'var(--accent)' }}>
                <Zap className="w-4 h-4" style={{ color: ADMIN_ACCENT }} />
              </div>
              <h3 className="font-semibold" style={{ color: ADMIN_TEXT }}>快捷操作</h3>
            </div>
          </div>
          <div className="p-4 space-y-2">
            <QuickLink to="/nodes" icon={Server} label="节点管理" desc="添加/编辑节点配置" iconBg="var(--accent-emerald)" iconColor="text-emerald-400" />
            <QuickLink to="/plans" icon={CreditCard} label="套餐管理" desc="订阅套餐配置" iconBg="var(--accent-amber)" iconColor="text-amber-400" />
            <QuickLink to="/users" icon={Users} label="用户管理" desc="用户列表与操作" iconBg="var(--accent-sky)" iconColor="text-sky-400" />
            <QuickLink to="/orders" icon={BarChart3} label="订单管理" desc="订单与支付记录" iconBg="var(--accent-pink)" iconColor="text-pink-400" />
            <QuickLink to="/tickets" icon={Ticket} label="工单中心" desc="客户工单管理" iconBg="var(--accent-rose)" iconColor="text-rose-400" />
            <QuickLink to="/system/config" icon={Activity} label="系统配置" desc="全局参数设置" iconBg="var(--muted)" iconColor="text-slate-400" />
            <QuickLink to="/diagnostics/ai" icon={ShieldCheck} label="AI 诊断" desc="智能排障与修复" iconBg="var(--accent)" iconColor="text-indigo-400" />
          </div>
        </div>
      </div>

      {/* Stats Row */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        {/* Traffic Stats */}
        <div className="rounded-2xl overflow-hidden" style={{ backgroundColor: ADMIN_CARD, border: `1px solid ${ADMIN_BORDER}`, boxShadow: 'var(--shadow-sm)' }}>
          <div className="p-5 border-b" style={{ borderColor: ADMIN_BORDER }}>
            <div className="flex items-center gap-2">
              <div className="p-2 rounded-lg" style={{ backgroundColor: 'var(--accent)' }}>
                <BarChart3 className="w-4 h-4" style={{ color: ADMIN_ACCENT }} />
              </div>
              <h3 className="font-semibold" style={{ color: ADMIN_TEXT }}>流量统计</h3>
            </div>
          </div>
          <div className="p-5 space-y-5">
            {traffic && (
              <>
                <div className="grid grid-cols-2 gap-4 mb-2">
                  <div className="text-center p-3 rounded-xl" style={{ backgroundColor: 'var(--muted)' }}>
                    <p className="text-xs mb-1" style={{ color: ADMIN_TEXT_MUTED }}>今日上传</p>
                    <p className="text-sm font-semibold" style={{ color: ADMIN_TEXT }}>{formatBytes(todayUpload)}</p>
                  </div>
                  <div className="text-center p-3 rounded-xl" style={{ backgroundColor: 'var(--muted)' }}>
                    <p className="text-xs mb-1" style={{ color: ADMIN_TEXT_MUTED }}>今日下载</p>
                    <p className="text-sm font-semibold" style={{ color: ADMIN_TEXT }}>{formatBytes(todayDownload)}</p>
                  </div>
                </div>
                <div className="text-center p-4 rounded-xl" style={{ background: 'var(--gradient-soft)' }}>
                  <p className="text-xs mb-1" style={{ color: ADMIN_TEXT_MUTED }}>今日总流量</p>
                  <p className="text-xl font-bold" style={{ color: ADMIN_ACCENT }}>{formatBytes(todayTotal)}</p>
                </div>
                {traffic.top_nodes && traffic.top_nodes.length > 0 && (
                  <div className="pt-3 border-t" style={{ borderColor: ADMIN_BORDER }}>
                    <p className="text-xs mb-2" style={{ color: ADMIN_TEXT_MUTED }}>今日流量 Top 节点</p>
                    <div className="space-y-1.5">
                      {traffic.top_nodes.slice(0, 5).map((n, i) => (
                        <div key={n.node_id || i} className="flex items-center justify-between text-xs">
                          <span className="truncate" style={{ color: ADMIN_TEXT }}>{n.node_name}</span>
                          <span className="font-mono" style={{ color: ADMIN_TEXT_MUTED }}>{formatBytes(n.total)}</span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </>
            )}
            {!traffic && !loading && (
              <p className="text-sm text-center py-4" style={{ color: ADMIN_TEXT_MUTED }}>暂无流量数据</p>
            )}
          </div>
        </div>

        {/* Plan Overview */}
        <div className="rounded-2xl overflow-hidden" style={{ backgroundColor: ADMIN_CARD, border: `1px solid ${ADMIN_BORDER}`, boxShadow: 'var(--shadow-sm)' }}>
          <div className="p-5 border-b" style={{ borderColor: ADMIN_BORDER }}>
            <div className="flex items-center gap-2">
              <div className="p-2 rounded-lg" style={{ backgroundColor: 'var(--accent-amber)' }}>
                <CreditCard className="w-4 h-4" style={{ color: ADMIN_WARNING }} />
              </div>
              <h3 className="font-semibold" style={{ color: ADMIN_TEXT }}>套餐概览</h3>
              <Badge variant="outline" className="ml-auto text-xs" style={{ borderColor: ADMIN_BORDER, color: ADMIN_TEXT_MUTED }}>
                共 {plans.length} 个 · 上架 {visiblePlans}
              </Badge>
            </div>
          </div>
          <div className="p-5">
            {loading ? (
              <div className="space-y-2">
                {[1,2,3,4].map(i => <Skeleton key={i} className="h-12 w-full rounded-xl" />)}
              </div>
            ) : plans.length === 0 ? (
              <div className="text-center py-10">
                <CreditCard className="w-10 h-10 mx-auto mb-3" style={{ color: ADMIN_TEXT_MUTED }} />
                <p className="text-sm" style={{ color: ADMIN_TEXT_MUTED }}>暂无套餐</p>
                <a href="/plans">
                  <Button size="sm" className="mt-3" style={{ background: ADMIN_GRADIENT }}>创建套餐</Button>
                </a>
              </div>
            ) : (
              <div className="grid grid-cols-2 gap-3">
                {plans.map(plan => (
                  <a
                    key={plan.id}
                    href="/plans"
                    className="p-3 rounded-xl transition-all hover:scale-[1.02] cursor-pointer"
                    style={{ backgroundColor: 'var(--muted)', border: `1px solid ${ADMIN_BORDER}` }}
                  >
                    <div className="flex items-center justify-between mb-1">
                      <p className="text-sm font-medium truncate" style={{ color: ADMIN_TEXT }}>{plan.name}</p>
                      {plan.status === 'active' ? (
                        <span className="w-2 h-2 rounded-full" style={{ backgroundColor: ADMIN_SUCCESS }} />
                      ) : plan.status === 'draft' ? (
                        <span className="w-2 h-2 rounded-full" style={{ backgroundColor: ADMIN_WARNING }} />
                      ) : (
                        <span className="w-2 h-2 rounded-full" style={{ backgroundColor: 'var(--muted-foreground)' }} />
                      )}
                    </div>
                    <p className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>
                      {plan.status === 'active' ? '已上架' : plan.status === 'draft' ? '草稿' : '已下架'} · {formatBytes(plan.traffic_bytes)}
                    </p>
                  </a>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
