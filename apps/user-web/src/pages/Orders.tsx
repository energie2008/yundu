import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { FileText, ChevronRight, CreditCard } from 'lucide-react'
import { Button, Badge, Tabs, TabsList, TabsTrigger } from '@airport/ui'
import { useToast } from '@/lib/toast'
import { api } from '@/lib/api'
import {
  EP,
  OrderResponse,
  formatUSDT,
  formatDateTime,
  getPeriodLabel,
  PaginatedResponse,
  adaptOrdersPage,
} from '@/lib/endpoints'

type StatusFilter = 'all' | 'pending' | 'paid' | 'canceled' | 'expired'

const tabs: { key: StatusFilter; label: string }[] = [
  { key: 'all', label: '全部' },
  { key: 'pending', label: '待支付' },
  { key: 'paid', label: '已完成' },
  { key: 'canceled', label: '已取消' },
]

function getStatusBadge(status: OrderResponse['status']) {
  switch (status) {
    case 'pending':
      return { variant: 'warning' as const, label: '待支付' }
    case 'paid':
      return { variant: 'success' as const, label: '已完成' }
    case 'canceled':
      return { variant: 'secondary' as const, label: '已取消' }
    case 'expired':
      return { variant: 'destructive' as const, label: '已过期' }
    default:
      return { variant: 'secondary' as const, label: status }
  }
}

function OrderRow({ order }: { order: OrderResponse }) {
  const navigate = useNavigate()
  const badge = getStatusBadge(order.status)
  const isPending = order.status === 'pending'

  return (
    <tr
      className="border-b transition-colors cursor-pointer"
      style={{ borderColor: 'var(--border)' }}
      onMouseEnter={e => (e.currentTarget.style.background = 'var(--muted)')}
      onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
      onClick={() => navigate(`/orders/${order.id}`)}
    >
      <td className="px-5 py-4">
        <span className="text-sm font-mono" style={{ color: 'var(--foreground)' }}>{order.order_no}</span>
      </td>
      <td className="px-5 py-4">
        <div>
          <div className="text-sm font-medium" style={{ color: 'var(--foreground)' }}>
            {order.plan_name || '-'}
          </div>
          <div className="text-xs mt-0.5" style={{ color: 'var(--muted-foreground)' }}>
            {formatUSDT(order.amount_usdt)} USDT / {getPeriodLabel(order.period_code)}
          </div>
        </div>
      </td>
      <td className="px-5 py-4">
        <Badge variant={badge.variant}>{badge.label}</Badge>
      </td>
      <td className="px-5 py-4">
        <span className="text-sm" style={{ color: 'var(--muted-foreground)' }}>{formatDateTime(order.created_at)}</span>
      </td>
      <td className="px-5 py-4">
        <div className="flex items-center gap-2">
          <button
            onClick={(e) => {
              e.stopPropagation()
              navigate(`/orders/${order.id}`)
            }}
            className="text-sm font-medium transition-colors flex items-center gap-1"
            style={{ color: 'var(--primary)' }}
            onMouseEnter={e => (e.currentTarget.style.opacity = '0.8')}
            onMouseLeave={e => (e.currentTarget.style.opacity = '1')}
          >
            详情
            <ChevronRight className="w-4 h-4" />
          </button>
          {isPending && (
            <button
              onClick={(e) => {
                e.stopPropagation()
                navigate(`/orders/${order.id}`)
              }}
              className="text-sm font-medium text-white px-3 py-1 rounded-lg transition-opacity flex items-center gap-1"
              style={{ background: 'var(--primary)' }}
              onMouseEnter={e => (e.currentTarget.style.opacity = '0.9')}
              onMouseLeave={e => (e.currentTarget.style.opacity = '1')}
            >
              <CreditCard className="w-3 h-3" />
              去支付
            </button>
          )}
        </div>
      </td>
    </tr>
  )
}

function TableSkeleton() {
  return (
    <div className="xboard-card overflow-hidden">
      <div className="p-5 border-b" style={{ borderColor: 'var(--border)' }}>
        <div className="h-10 w-full rounded animate-pulse" style={{ background: 'var(--muted)' }} />
      </div>
      <div className="divide-y" style={{ borderColor: 'var(--border)' }}>
        {[1, 2, 3].map((i) => (
          <div key={i} className="px-5 py-4">
            <div className="flex justify-between gap-4">
              <div className="h-4 w-32 rounded animate-pulse" style={{ background: 'var(--muted)' }} />
              <div className="h-4 w-40 rounded animate-pulse" style={{ background: 'var(--muted)' }} />
              <div className="h-4 w-16 rounded animate-pulse" style={{ background: 'var(--muted)' }} />
              <div className="h-4 w-32 rounded animate-pulse" style={{ background: 'var(--muted)' }} />
              <div className="h-4 w-20 rounded animate-pulse" style={{ background: 'var(--muted)' }} />
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

export default function Orders() {
  const navigate = useNavigate()
  const [activeTab, setActiveTab] = useState<StatusFilter>('all')
  const [page, setPage] = useState(1)
  const pageSize = 20

  const { data: ordersPage, isLoading } = useQuery<PaginatedResponse<OrderResponse>>({
    queryKey: ['orders', page, pageSize],
    queryFn: async () => {
      const raw = await api.get<PaginatedResponse<OrderResponse>>(EP.ORDERS, { params: { page, page_size: pageSize } })
      return adaptOrdersPage(raw)
    },
  })

  const allOrders = ordersPage?.items || []
  const filteredOrders =
    activeTab === 'all'
      ? allOrders
      : allOrders.filter((o) => o.status === activeTab)

  return (
    <div className="p-6 max-w-5xl mx-auto">
      <div className="mb-6">
        <h1 className="text-xl font-bold" style={{ color: 'var(--foreground)' }}>我的订单</h1>
        <p className="mt-1 text-sm" style={{ color: 'var(--muted-foreground)' }}>查看您的订单历史</p>
      </div>

      <Tabs value={activeTab} onValueChange={(v) => setActiveTab(v as StatusFilter)} className="mb-5">
        <TabsList className="border-b" style={{ borderColor: 'var(--border)', backgroundColor: 'transparent' }}>
          {tabs.map((tab) => (
            <TabsTrigger
              key={tab.key}
              value={tab.key}
              className="data-[state=active]:border-b-2"
              style={{ color: activeTab === tab.key ? 'var(--primary)' : 'var(--muted-foreground)', borderBottomColor: activeTab === tab.key ? 'var(--primary)' : 'transparent' }}
            >
              {tab.label}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>

      {isLoading ? (
        <TableSkeleton />
      ) : filteredOrders.length === 0 ? (
        <div className="xboard-card py-16 text-center">
          <FileText className="w-14 h-14 mx-auto mb-4" style={{ color: 'var(--muted-foreground)' }} />
          <p className="mb-5" style={{ color: 'var(--muted-foreground)' }}>暂无订单</p>
          <Button
            onClick={() => navigate('/plans')}
            className="text-white px-6 h-10 rounded-lg border-0 shadow-sm"
            style={{ background: 'var(--primary)' }}
          >
            浏览套餐
          </Button>
        </div>
      ) : (
        <div className="xboard-card overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="border-b" style={{ background: 'var(--muted)', borderColor: 'var(--border)' }}>
                <th className="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider" style={{ color: 'var(--muted-foreground)' }}>订单号</th>
                <th className="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider" style={{ color: 'var(--muted-foreground)' }}>套餐</th>
                <th className="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider" style={{ color: 'var(--muted-foreground)' }}>状态</th>
                <th className="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider" style={{ color: 'var(--muted-foreground)' }}>创建时间</th>
                <th className="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider" style={{ color: 'var(--muted-foreground)' }}>操作</th>
              </tr>
            </thead>
            <tbody>
              {filteredOrders.map((order) => (
                <OrderRow key={order.id} order={order} />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
