import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Search,
  RefreshCw,
  Receipt,
  ChevronLeft,
  ChevronRight,
  XCircle,
  CheckCircle,
  Eye,
} from 'lucide-react'
import {
  Card,
  CardContent,
  Button,
  Input,
  Badge,
  Skeleton,
  EmptyState,
  Select,
} from '@airport/ui'
import {
  ADMIN_CARD,
  ADMIN_BORDER,
  ADMIN_TEXT,
  ADMIN_TEXT_SECONDARY,
  ADMIN_TEXT_MUTED,
  ADMIN_INPUT_BG,
  ADMIN_INPUT_BORDER,
} from '@/lib/theme'
import { api } from '@/lib/api'
import { EP } from '@/lib/endpoints'

const ORDER_STATUS_MAP: Record<string, { label: string; color: string }> = {
  pending: { label: '待支付', color: 'bg-amber-900/50 text-amber-300 border-amber-800/50' },
  paid: { label: '已支付', color: 'bg-green-900/50 text-green-300 border-green-800/50' },
  expired: { label: '已过期', color: 'bg-zinc-800/50 text-zinc-400 border-zinc-700/50' },
  canceled: { label: '已取消', color: 'bg-red-900/50 text-red-300 border-red-800/50' },
}

interface OrderItem {
  id: string
  order_no: string
  plan_id: string
  plan_name: string
  period_code: string
  amount_usdt: number
  amount_cny?: number
  exchange_rate?: number
  discount_amount?: number
  coupon_code?: string
  status: string
  pay_address: string
  pay_currency: string
  payment_method: string
  tx_hash?: string | null
  paid_amount?: number | null
  paid_at?: string | null
  expires_at: string
  created_at: string
}

interface OrdersResponse {
  page: number
  page_size: number
  total: number
  items: OrderItem[]
}

const PERIOD_MAP: Record<string, string> = {
  month: '月付',
  quarter: '季付',
  half_year: '半年付',
  year: '年付',
  onetime: '一次性',
  monthly: '月付',
  quarterly: '季付',
  half_yearly: '半年付',
  yearly: '年付',
}

function formatUSDT(amount: number): string {
  return `$${amount.toFixed(2)}`
}

function formatDate(dateStr: string): string {
  if (!dateStr) return '-'
  const d = new Date(dateStr)
  return d.toLocaleString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

export default function Orders() {
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [pageSize] = useState(20)
  const [statusFilter, setStatusFilter] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [searchQuery, setSearchQuery] = useState('')
  const [detailOrder, setDetailOrder] = useState<OrderItem | null>(null)

  const { data, isLoading, isFetching } = useQuery<OrdersResponse>({
    queryKey: ['orders', page, pageSize, statusFilter, searchQuery],
    queryFn: async () => {
      const params: Record<string, string | number> = { page, page_size: pageSize }
      if (statusFilter) params.status = statusFilter
      if (searchQuery) {
        // searchQuery 可以是 user_id 或 order_no
        params.user_id = searchQuery
      }
      const raw = await api.get<OrdersResponse>(EP.ORDERS, { params })
      return raw
    },
    retry: false,
  })

  const cancelOrder = useMutation({
    mutationFn: async (id: string) => {
      return api.post(EP.ORDER_CANCEL(id), {})
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['orders'] })
    },
  })

  const markPaid = useMutation({
    mutationFn: async (id: string) => {
      return api.post(EP.ORDER_MARK_PAID(id), {})
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['orders'] })
    },
  })

  const handleSearch = () => {
    setSearchQuery(searchInput.trim())
    setPage(1)
  }

  const handleRefresh = () => {
    queryClient.invalidateQueries({ queryKey: ['orders'] })
  }

  const orders = data?.items ?? []
  const total = data?.total ?? 0
  const totalPages = Math.ceil(total / pageSize) || 1

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold" style={{ color: ADMIN_TEXT }}>订单管理</h1>
          <p className="text-sm mt-1" style={{ color: ADMIN_TEXT_MUTED }}>查看和管理用户订单</p>
        </div>
        <Button variant="outline" size="sm" onClick={handleRefresh} disabled={isFetching}>
          <RefreshCw className={`w-4 h-4 mr-2 ${isFetching ? 'animate-spin' : ''}`} />
          刷新
        </Button>
      </div>

      {/* 筛选栏 */}
      <Card style={{ background: ADMIN_CARD, borderColor: ADMIN_BORDER }}>
        <CardContent className="p-4">
          <div className="flex flex-wrap items-center gap-3">
            <div className="flex items-center gap-2 flex-1 min-w-[240px]">
              <Search className="w-4 h-4" style={{ color: ADMIN_TEXT_MUTED }} />
              <Input
                placeholder="搜索用户 ID 或订单号..."
                value={searchInput}
                onChange={(e) => setSearchInput(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
                style={{ background: ADMIN_INPUT_BG, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }}
                className="flex-1"
              />
            </div>
            <Select
              value={statusFilter}
              onChange={(e) => { setStatusFilter(e.target.value); setPage(1) }}
              style={{ background: ADMIN_INPUT_BG, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT, width: '140px' }}
            >
              <option value="">全部状态</option>
              <option value="pending">待支付</option>
              <option value="paid">已支付</option>
              <option value="expired">已过期</option>
              <option value="canceled">已取消</option>
            </Select>
            <Button size="sm" onClick={handleSearch}>
              <Search className="w-4 h-4 mr-1" /> 搜索
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* 订单列表 */}
      <Card style={{ background: ADMIN_CARD, borderColor: ADMIN_BORDER }}>
        <CardContent className="p-0">
          {isLoading ? (
            <div className="p-6 space-y-3">
              {Array.from({ length: 5 }).map((_, i) => (
                <Skeleton key={i} className="h-12 w-full" />
              ))}
            </div>
          ) : orders.length === 0 ? (
            <div className="p-12">
              <EmptyState
                icon={<Receipt className="w-12 h-12" />}
                title="暂无订单"
                description="还没有任何订单记录"
              />
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b" style={{ borderColor: ADMIN_BORDER }}>
                    <th className="text-left p-3 text-xs font-medium" style={{ color: ADMIN_TEXT_MUTED }}>订单号</th>
                    <th className="text-left p-3 text-xs font-medium" style={{ color: ADMIN_TEXT_MUTED }}>套餐</th>
                    <th className="text-left p-3 text-xs font-medium" style={{ color: ADMIN_TEXT_MUTED }}>周期</th>
                    <th className="text-left p-3 text-xs font-medium" style={{ color: ADMIN_TEXT_MUTED }}>金额(USDT)</th>
                    <th className="text-left p-3 text-xs font-medium" style={{ color: ADMIN_TEXT_MUTED }}>金额(CNY)</th>
                    <th className="text-left p-3 text-xs font-medium" style={{ color: ADMIN_TEXT_MUTED }}>优惠</th>
                    <th className="text-left p-3 text-xs font-medium" style={{ color: ADMIN_TEXT_MUTED }}>状态</th>
                    <th className="text-left p-3 text-xs font-medium" style={{ color: ADMIN_TEXT_MUTED }}>创建时间</th>
                    <th className="text-left p-3 text-xs font-medium" style={{ color: ADMIN_TEXT_MUTED }}>操作</th>
                  </tr>
                </thead>
                <tbody>
                  {orders.map((order) => {
                    const statusInfo = ORDER_STATUS_MAP[order.status] || { label: order.status, color: '' }
                    return (
                      <tr key={order.id} className="border-b hover:bg-zinc-800/30" style={{ borderColor: ADMIN_BORDER }}>
                        <td className="p-3">
                          <code className="text-xs font-mono" style={{ color: ADMIN_TEXT_SECONDARY }}>
                            {order.order_no?.slice(0, 16)}...
                          </code>
                        </td>
                        <td className="p-3 text-sm" style={{ color: ADMIN_TEXT }}>{order.plan_name || '-'}</td>
                        <td className="p-3 text-sm" style={{ color: ADMIN_TEXT_SECONDARY }}>
                          {PERIOD_MAP[order.period_code] || order.period_code}
                        </td>
                        <td className="p-3 text-sm font-medium" style={{ color: ADMIN_TEXT }}>
                          {formatUSDT(order.amount_usdt)}
                        </td>
                        <td className="p-3 text-sm" style={{ color: ADMIN_TEXT_SECONDARY }}>
                          {order.amount_cny ? `¥${order.amount_cny.toFixed(2)}` : '-'}
                        </td>
                        <td className="p-3 text-sm" style={{ color: ADMIN_TEXT_MUTED }}>
                          {order.discount_amount && order.discount_amount > 0 ? (
                            <span className="text-green-400">-{formatUSDT(order.discount_amount)}</span>
                          ) : '-'}
                          {order.coupon_code && (
                            <span className="ml-1 text-xs">({order.coupon_code})</span>
                          )}
                        </td>
                        <td className="p-3">
                          <Badge variant="outline" className={statusInfo.color}>
                            {statusInfo.label}
                          </Badge>
                        </td>
                        <td className="p-3 text-xs" style={{ color: ADMIN_TEXT_MUTED }}>
                          {formatDate(order.created_at)}
                        </td>
                        <td className="p-3">
                          <div className="flex items-center gap-1">
                            <button
                              onClick={() => setDetailOrder(order)}
                              className="p-1.5 rounded hover:bg-zinc-700/50"
                              title="查看详情"
                            >
                              <Eye className="w-4 h-4" style={{ color: ADMIN_TEXT_MUTED }} />
                            </button>
                            {order.status === 'pending' && (
                              <>
                                <button
                                  onClick={() => {
                                    if (confirm('确认手动标记此订单为已支付？')) {
                                      markPaid.mutate(order.id)
                                    }
                                  }}
                                  className="p-1.5 rounded hover:bg-green-900/30"
                                  title="标记已支付（补单）"
                                >
                                  <CheckCircle className="w-4 h-4 text-green-400" />
                                </button>
                                <button
                                  onClick={() => {
                                    if (confirm('确认取消此订单？')) {
                                      cancelOrder.mutate(order.id)
                                    }
                                  }}
                                  className="p-1.5 rounded hover:bg-red-900/30"
                                  title="取消订单"
                                >
                                  <XCircle className="w-4 h-4 text-red-400" />
                                </button>
                              </>
                            )}
                          </div>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          )}

          {/* 分页 */}
          {total > 0 && (
            <div className="flex items-center justify-between p-4 border-t" style={{ borderColor: ADMIN_BORDER }}>
              <span className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>
                共 {total} 条记录，第 {page}/{totalPages} 页
              </span>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page <= 1}
                  onClick={() => setPage(p => Math.max(1, p - 1))}
                >
                  <ChevronLeft className="w-4 h-4" />
                </Button>
                <span className="text-sm" style={{ color: ADMIN_TEXT }}>{page}</span>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page >= totalPages}
                  onClick={() => setPage(p => Math.min(totalPages, p + 1))}
                >
                  <ChevronRight className="w-4 h-4" />
                </Button>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      {/* 订单详情对话框 */}
      {detailOrder && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={() => setDetailOrder(null)}>
          <Card
            className="max-w-2xl w-full mx-4 max-h-[80vh] overflow-y-auto"
            style={{ background: ADMIN_CARD, borderColor: ADMIN_BORDER }}
            onClick={(e) => e.stopPropagation()}
          >
            <CardContent className="p-6">
              <div className="flex items-center justify-between mb-4">
                <h3 className="text-lg font-semibold" style={{ color: ADMIN_TEXT }}>订单详情</h3>
                <button onClick={() => setDetailOrder(null)} className="text-zinc-500 hover:text-zinc-300">×</button>
              </div>
              <div className="space-y-3">
                <DetailRow label="订单号" value={detailOrder.order_no} mono />
                <DetailRow label="套餐" value={detailOrder.plan_name || '-'} />
                <DetailRow label="周期" value={PERIOD_MAP[detailOrder.period_code] || detailOrder.period_code} />
                <DetailRow label="应付金额" value={formatUSDT(detailOrder.amount_usdt)} />
                <DetailRow label="CNY金额" value={detailOrder.amount_cny ? `¥${detailOrder.amount_cny.toFixed(2)}` : '-'} />
                <DetailRow label="汇率(锁定)" value={detailOrder.exchange_rate ? `1 USDT = ${detailOrder.exchange_rate} CNY` : '-'} />
                <DetailRow label="优惠金额" value={detailOrder.discount_amount ? formatUSDT(detailOrder.discount_amount) : '-'} />
                <DetailRow label="优惠券码" value={detailOrder.coupon_code || '-'} />
                <DetailRow label="支付方式" value={detailOrder.payment_method || detailOrder.pay_currency || '-'} />
                <DetailRow label="收款地址" value={detailOrder.pay_address || '-'} mono />
                <DetailRow label="交易哈希" value={detailOrder.tx_hash || '-'} mono />
                <DetailRow label="实付金额" value={detailOrder.paid_amount ? formatUSDT(detailOrder.paid_amount) : '-'} />
                <DetailRow label="支付时间" value={detailOrder.paid_at ? formatDate(detailOrder.paid_at) : '-'} />
                <DetailRow label="过期时间" value={formatDate(detailOrder.expires_at)} />
                <DetailRow label="创建时间" value={formatDate(detailOrder.created_at)} />
                <div className="pt-2">
                  <span className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>状态: </span>
                  <Badge variant="outline" className={`ml-2 ${(ORDER_STATUS_MAP[detailOrder.status] || {}).color}`}>
                    {(ORDER_STATUS_MAP[detailOrder.status] || {}).label || detailOrder.status}
                  </Badge>
                </div>
              </div>
            </CardContent>
          </Card>
        </div>
      )}
    </div>
  )
}

function DetailRow({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-start justify-between gap-4 py-1">
      <span className="text-sm" style={{ color: ADMIN_TEXT_MUTED }}>{label}</span>
      <span
        className={`text-sm text-right break-all ${mono ? 'font-mono' : ''}`}
        style={{ color: ADMIN_TEXT_SECONDARY }}
      >
        {value}
      </span>
    </div>
  )
}
