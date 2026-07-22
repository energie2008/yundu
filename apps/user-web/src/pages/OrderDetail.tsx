import { useState, useEffect, type ReactNode } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import {
  ArrowLeft,
  Clock,
  CheckCircle,
  XCircle,
  RefreshCw,
  Copy,
  Check,
  Wallet,
} from 'lucide-react'
import { useToast } from '@/lib/toast'
import { api } from '@/lib/api'
import {
  EP,
  OrderResponse,
  formatUSDT,
  formatDateTime,
  getPeriodLabel,
  adaptOrder,
} from '@/lib/endpoints'
import { QRCode } from '@/components/QRCode'

const POLL_INTERVAL = 10000

function Countdown({ expiresAt }: { expiresAt: string }) {
  const [timeLeft, setTimeLeft] = useState({ hours: 0, minutes: 0, seconds: 0 })

  useEffect(() => {
    const calculate = () => {
      const now = Date.now()
      const exp = new Date(expiresAt).getTime()
      const diff = exp - now

      if (diff <= 0) {
        setTimeLeft({ hours: 0, minutes: 0, seconds: 0 })
        return
      }

      const hours = Math.floor(diff / (1000 * 60 * 60))
      const minutes = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60))
      const seconds = Math.floor((diff % (1000 * 60)) / 1000)
      setTimeLeft({ hours, minutes, seconds })
    }

    calculate()
    const timer = setInterval(calculate, 1000)
    return () => clearInterval(timer)
  }, [expiresAt])

  const pad = (n: number) => n.toString().padStart(2, '0')

  return (
    <div className="flex gap-2 justify-center">
      {[
        { value: timeLeft.hours, label: '时' },
        { value: timeLeft.minutes, label: '分' },
        { value: timeLeft.seconds, label: '秒' },
      ].map((item, i) => (
        <div key={i} className="flex items-center gap-1">
          <div className="rounded-lg px-3 py-2 text-center min-w-[48px]" style={{ background: 'var(--muted)' }}>
            <span className="text-xl font-bold font-mono" style={{ color: 'var(--primary)' }}>{pad(item.value)}</span>
          </div>
          {i < 2 && <span className="font-bold" style={{ color: 'var(--muted-foreground)' }}>:</span>}
        </div>
      ))}
    </div>
  )
}

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)
  const { toast } = useToast()

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      toast({ title: '已复制', variant: 'success' })
      setTimeout(() => setCopied(false), 2000)
    } catch {
      toast({ title: '复制失败', variant: 'destructive' })
    }
  }

  return (
    <button
      onClick={copy}
      className="flex items-center gap-1 px-2 py-1 rounded text-xs transition-colors"
      style={{
        color: copied ? 'var(--success)' : 'var(--primary)',
        backgroundColor: copied ? 'rgba(34,197,94,0.1)' : 'rgba(124,92,252,0.1)',
      }}
    >
      {copied ? <Check className="w-3 h-3" /> : <Copy className="w-3 h-3" />}
      {copied ? '已复制' : '复制'}
    </button>
  )
}

function getStatusBadge(status: OrderResponse['status']) {
  switch (status) {
    case 'pending':
      return { bg: 'rgba(245,158,11,0.1)', color: '#f59e0b', label: '待支付' }
    case 'paid':
      return { bg: 'rgba(34,197,94,0.1)', color: 'var(--success)', label: '已完成' }
    case 'canceled':
      return { bg: 'var(--muted)', color: 'var(--muted-foreground)', label: '已取消' }
    case 'expired':
      return { bg: 'rgba(239,68,68,0.1)', color: 'var(--destructive)', label: '已过期' }
    default:
      return { bg: 'var(--muted)', color: 'var(--muted-foreground)', label: status }
  }
}

function DetailRow({ label, value, highlight }: { label: string; value: ReactNode; highlight?: boolean }) {
  return (
    <div className="flex justify-between py-3 border-b last:border-b-0" style={{ borderColor: 'var(--border)' }}>
      <span className="text-sm" style={{ color: 'var(--muted-foreground)' }}>{label}</span>
      <span className={`text-sm font-medium ${highlight ? 'text-lg font-bold' : ''}`} style={{ color: highlight ? 'var(--primary)' : 'var(--foreground)' }}>{value}</span>
    </div>
  )
}

export default function OrderDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { toast } = useToast()

  const { data: order, isLoading, refetch } = useQuery<OrderResponse>({
    queryKey: ['order', id],
    queryFn: async () => {
      const raw = await api.get<OrderResponse>(EP.ORDER_DETAIL(id!))
      return adaptOrder(raw)
    },
    enabled: !!id,
    refetchInterval: (query) => {
      if (query.state.data?.status === 'pending') return POLL_INTERVAL
      return false
    },
  })

  useEffect(() => {
    if (order?.status === 'paid') {
      toast({ title: '支付成功！', description: '订阅已激活' })
    }
  }, [order?.status, toast])

  const isPending = order?.status === 'pending'
  const isPaid = order?.status === 'paid'
  const isExpired = order?.status === 'expired'
  const isCanceled = order?.status === 'canceled'
  const badge = order ? getStatusBadge(order.status) : null

  if (isLoading || !order) {
    return (
      <div className="p-6 max-w-3xl mx-auto">
        <div className="flex items-center gap-3 mb-6">
          <button
            onClick={() => navigate('/dashboard/orders')}
            className="flex items-center -ml-2 text-sm transition-colors px-2 py-1 rounded-lg"
            style={{ color: 'var(--muted-foreground)' }}
            onMouseEnter={e => (e.currentTarget.style.color = 'var(--primary)')}
            onMouseLeave={e => (e.currentTarget.style.color = 'var(--muted-foreground)')}
          >
            <ArrowLeft className="w-4 h-4 mr-1" />
            返回
          </button>
          <div className="h-7 w-32 rounded animate-pulse" style={{ background: 'var(--muted)' }} />
        </div>
        <div className="xboard-card p-5">
          <div className="h-80 w-full rounded animate-pulse" style={{ background: 'var(--muted)' }} />
        </div>
      </div>
    )
  }

  return (
    <div className="p-6 max-w-3xl mx-auto">
      <div className="flex items-center gap-3 mb-6">
        <button
          onClick={() => navigate('/dashboard/orders')}
          className="flex items-center text-sm -ml-2 px-2 py-1 rounded-lg transition-colors"
          style={{ color: 'var(--muted-foreground)' }}
          onMouseEnter={e => (e.currentTarget.style.color = 'var(--primary)')}
          onMouseLeave={e => (e.currentTarget.style.color = 'var(--muted-foreground)')}
        >
          <ArrowLeft className="w-4 h-4 mr-1" />
          返回
        </button>
        <h1 className="text-2xl font-bold" style={{ color: 'var(--foreground)' }}>订单详情</h1>
        {badge && (
          <span
            className="text-xs px-2.5 py-1 rounded-full font-medium"
            style={{ background: badge.bg, color: badge.color }}
          >
            {badge.label}
          </span>
        )}
      </div>

      {isPaid && (
        <div className="xboard-card p-6 text-center mb-5" style={{ border: '1px solid rgba(34,197,94,0.3)', background: 'rgba(34,197,94,0.05)' }}>
          <CheckCircle className="w-14 h-14 mx-auto mb-3" style={{ color: 'var(--success)' }} />
          <p className="text-lg font-semibold" style={{ color: 'var(--success)' }}>支付成功！</p>
          <p className="text-sm mt-1" style={{ color: 'var(--muted-foreground)' }}>您的订阅已激活</p>
          <button
            onClick={() => navigate('/dashboard')}
            className="mt-4 text-white px-6 h-10 rounded-lg text-sm font-medium transition-colors shadow-sm border-0"
            style={{ background: 'var(--primary)' }}
            onMouseEnter={e => (e.currentTarget.style.opacity = '0.9')}
            onMouseLeave={e => (e.currentTarget.style.opacity = '1')}
          >
            查看订阅
          </button>
        </div>
      )}

      {isExpired && (
        <div className="xboard-card p-6 text-center mb-5" style={{ border: '1px solid rgba(239,68,68,0.3)', background: 'rgba(239,68,68,0.05)' }}>
          <XCircle className="w-14 h-14 mx-auto mb-3" style={{ color: 'var(--destructive)' }} />
          <p className="text-lg font-semibold" style={{ color: 'var(--destructive)' }}>订单已过期</p>
          <p className="text-sm mt-1" style={{ color: 'var(--muted-foreground)' }}>请重新创建订单</p>
          <button
            onClick={() => navigate('/dashboard/plans')}
            className="mt-4 text-white px-6 h-10 rounded-lg text-sm font-medium transition-colors shadow-sm border-0"
            style={{ background: 'var(--primary)' }}
            onMouseEnter={e => (e.currentTarget.style.opacity = '0.9')}
            onMouseLeave={e => (e.currentTarget.style.opacity = '1')}
          >
            重新购买
          </button>
        </div>
      )}

      {isCanceled && (
        <div className="xboard-card p-6 text-center mb-5">
          <XCircle className="w-14 h-14 mx-auto mb-3" style={{ color: 'var(--muted-foreground)' }} />
          <p className="text-lg font-semibold" style={{ color: 'var(--foreground)' }}>订单已取消</p>
          <button
            onClick={() => navigate('/dashboard/plans')}
            className="mt-4 text-white px-6 h-10 rounded-lg text-sm font-medium transition-colors shadow-sm border-0"
            style={{ background: 'var(--primary)' }}
            onMouseEnter={e => (e.currentTarget.style.opacity = '0.9')}
            onMouseLeave={e => (e.currentTarget.style.opacity = '1')}
          >
            浏览套餐
          </button>
        </div>
      )}

      {isPending && order.pay_address && (
        <div className="xboard-card p-5 mb-5">
          <h2 className="text-base font-semibold mb-4 flex items-center gap-2" style={{ color: 'var(--foreground)' }}>
            <Wallet className="w-4 h-4" style={{ color: 'var(--primary)' }} />
            支付信息
          </h2>

          <div className="text-center mb-5">
            <p className="text-xs mb-2" style={{ color: 'var(--muted-foreground)' }}>剩余支付时间</p>
            <Countdown expiresAt={order.expires_at} />
          </div>

          <div className="rounded-xl p-4 flex justify-center mb-5" style={{ background: 'var(--muted)' }}>
            <QRCode value={order.pay_address} size={180} />
          </div>

          <div className="rounded-lg p-4 mb-5" style={{ background: 'var(--muted)' }}>
            <div className="flex items-center justify-between mb-3">
              <span className="text-sm" style={{ color: 'var(--muted-foreground)' }}>USDT-TRC20 收款地址</span>
              <CopyButton text={order.pay_address} />
            </div>
            <p className="text-xs font-mono break-all" style={{ color: 'var(--foreground)' }}>{order.pay_address}</p>
          </div>

          <div className="rounded-lg p-4 mb-5" style={{ background: 'rgba(245,158,11,0.08)', border: '1px solid rgba(245,158,11,0.2)' }}>
            <p className="text-sm" style={{ color: '#f59e0b' }}>
              <Clock className="w-4 h-4 inline mr-1" />
              请在有效期内支付 <strong style={{ color: 'var(--primary)' }}>{formatUSDT(order.amount_usdt)} USDT</strong>（TRC20网络）到上述地址，支付完成后系统将自动确认并激活您的订阅。
            </p>
          </div>

          <div className="rounded-lg p-4 mb-5" style={{ background: 'var(--muted)' }}>
            <div className="flex justify-between items-center mb-3">
              <span className="text-sm" style={{ color: 'var(--muted-foreground)' }}>套餐</span>
              <span className="text-sm font-medium" style={{ color: 'var(--foreground)' }}>
                {order.plan_name} · {getPeriodLabel(order.period_code)}
              </span>
            </div>
            <div className="flex justify-between items-center">
              <span className="text-sm" style={{ color: 'var(--muted-foreground)' }}>应付金额</span>
              <span className="text-lg font-bold" style={{ color: 'var(--primary)' }}>{formatUSDT(order.amount_usdt)} USDT</span>
            </div>
          </div>

          <button
            onClick={() => refetch()}
            className="w-full h-10 text-sm rounded-lg border transition-colors flex items-center justify-center gap-1.5"
            style={{ borderColor: 'var(--border)', color: 'var(--muted-foreground)' }}
            onMouseEnter={e => { e.currentTarget.style.background = 'var(--muted)'; e.currentTarget.style.color = 'var(--foreground)'; }}
            onMouseLeave={e => { e.currentTarget.style.background = 'transparent'; e.currentTarget.style.color = 'var(--muted-foreground)'; }}
          >
            <RefreshCw className="w-4 h-4" />
            我已支付，刷新状态
          </button>
        </div>
      )}

      <div className="xboard-card p-5">
        <h2 className="text-base font-semibold mb-2" style={{ color: 'var(--foreground)' }}>订单信息</h2>
        <DetailRow label="订单号" value={<span className="font-mono">{order.order_no}</span>} />
        <DetailRow label="套餐名称" value={order.plan_name} />
        <DetailRow label="订购周期" value={getPeriodLabel(order.period_code)} />
        <DetailRow label="商品金额" value={`${formatUSDT(order.amount_usdt)} USDT`} />
        <DetailRow
          label="实付金额"
          value={`${formatUSDT(order.amount_usdt)} USDT`}
          highlight
        />
        <DetailRow label="创建时间" value={formatDateTime(order.created_at)} />
        {order.paid_at && (
          <div className="flex justify-between py-3 border-b" style={{ borderColor: 'var(--border)' }}>
            <span className="text-sm" style={{ color: 'var(--muted-foreground)' }}>支付时间</span>
            <span className="text-sm font-medium" style={{ color: 'var(--success)' }}>{formatDateTime(order.paid_at)}</span>
          </div>
        )}
        {order.expires_at && isPending && (
          <DetailRow label="过期时间" value={formatDateTime(order.expires_at)} />
        )}
      </div>
    </div>
  )
}
