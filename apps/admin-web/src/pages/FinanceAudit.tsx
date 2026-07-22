import { useState, useEffect } from 'react'
import { Receipt, AlertTriangle, RefreshCw, CreditCard, TrendingUp, History, DollarSign, Eye, Play, ArrowRight } from 'lucide-react'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
  Button,
  Badge,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
  Table,
  TableHeader,
  TableBody,
  TableHead,
  TableRow,
  TableCell,
  useToast,
} from '@airport/ui'
import { api, ApiError } from '@/lib/api'

interface FailedPayment {
  id: string
  orderNo: string
  user: string
  amount: number
  channel: string
  reason: '签名校验失败' | '回调超时' | '域名不匹配'
  time: string
  replayed: boolean
}

interface TrafficEvent {
  id: string
  time: string
  user: string
  node: string
  trafficMB: number
  multiplier: number
  charge: number
  linkType: '直连' | '中转'
  note: string
}

interface MultiplierChange {
  id: string
  time: string
  operator: string
  node: string
  oldMultiplier: number
  newMultiplier: number
  impact: string
}

interface KpiData {
  callbackSuccessRate: number | null
  pendingFailedOrders: number | null
  trafficAlerts: number | null
  todayIncome: number | null
}

function getReasonBadge(reason: FailedPayment['reason']) {
  const configs: Record<FailedPayment['reason'], { className: string }> = {
    '签名校验失败': { className: 'bg-red-900/50 text-red-300 border-red-800/50' },
    '回调超时': { className: 'bg-amber-900/50 text-amber-300 border-amber-800/50' },
    '域名不匹配': { className: 'bg-orange-900/50 text-orange-300 border-orange-800/50' },
  }
  return (
    <Badge variant="outline" className={configs[reason].className}>
      {reason}
    </Badge>
  )
}

function KpiCard({ title, value, icon: Icon, color, subtext }: {
  title: string
  value: string | number
  icon: React.ComponentType<{ className?: string }>
  color: string
  subtext?: string
}) {
  return (
    <Card className="bg-zinc-900 border-zinc-800">
      <CardContent className="p-4">
        <div className="flex items-center justify-between">
          <div className="space-y-1">
            <p className="text-sm text-zinc-400">{title}</p>
            <p className={`text-2xl font-bold ${color}`}>{value}</p>
            {subtext && <p className="text-xs text-zinc-500">{subtext}</p>}
          </div>
          <div className={`p-2 rounded-lg bg-zinc-800/50 ${color}`}>
            <Icon className="w-5 h-5" />
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

export default function FinanceAudit() {
  const [activeTab, setActiveTab] = useState('pending')
  const [replayingIds, setReplayingIds] = useState<Set<string>>(new Set())
  const [failedPayments, setFailedPayments] = useState<FailedPayment[]>([])
  const [trafficEvents, setTrafficEvents] = useState<TrafficEvent[]>([])
  const [multiplierChanges, setMultiplierChanges] = useState<MultiplierChange[]>([])
  const [loading, setLoading] = useState(true)
  const [kpiData, setKpiData] = useState<KpiData>({
    callbackSuccessRate: null,
    pendingFailedOrders: null,
    trafficAlerts: null,
    todayIncome: null,
  })
  const { toast } = useToast()

  // 后端待 XBoard identity-service 提供 KPI/审计接口，目前为占位
  useEffect(() => {
    // 占位：后端接口就绪后填充
    // 如：api.get<KpiData>(EP.FINANCE_KPI).then(setKpiData)
    //     api.get<unknown>(EP.FAILED_PAYMENTS).then(...)
    setLoading(false)
  }, [])

  const pendingPayments = failedPayments.filter((p) => !p.replayed)
  const replayedPayments = failedPayments.filter((p) => p.replayed)

  const handleReplay = async (id: string) => {
    setReplayingIds((prev) => new Set(prev).add(id))
    await new Promise((resolve) => setTimeout(resolve, 1000))
    setReplayingIds((prev) => {
      const next = new Set(prev)
      next.delete(id)
      return next
    })
  }

  const getFilteredPayments = () => {
    switch (activeTab) {
      case 'pending':
        return pendingPayments
      case 'replayed':
        return replayedPayments
      default:
        return failedPayments
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-zinc-100 flex items-center gap-2">
            <Receipt className="w-7 h-7 text-emerald-400" />
            财务审计
          </h1>
          <p className="text-zinc-400 mt-1">支付回调健康度、流量事件审计、倍率变更历史</p>
        </div>
        <Card className="bg-amber-950/20 border-amber-600/50 max-w-sm flex-shrink-0">
          <CardContent className="p-3">
            <div className="flex gap-2">
              <AlertTriangle className="w-5 h-5 text-amber-400 flex-shrink-0 mt-0.5" />
              <p className="text-xs text-amber-200 leading-relaxed">
                <strong className="text-amber-100">注意：</strong>支付回调URL是独立于用户访问域名的配置项，请勿随意更改。修改前请在支付平台同步更新。
              </p>
            </div>
          </CardContent>
        </Card>
      </div>

      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
        <KpiCard
          title="24h回调成功率"
          value={kpiData.callbackSuccessRate === null ? '—' : `${kpiData.callbackSuccessRate}%`}
          icon={CreditCard}
          color={kpiData.callbackSuccessRate === null ? 'text-zinc-400' : kpiData.callbackSuccessRate >= 99 ? 'text-emerald-400' : 'text-red-400'}
          subtext={kpiData.callbackSuccessRate === null ? '暂无数据' : kpiData.callbackSuccessRate >= 99 ? '健康' : '异常'}
        />
        <KpiCard
          title="待处理失败订单"
          value={kpiData.pendingFailedOrders === null ? '—' : kpiData.pendingFailedOrders}
          icon={AlertTriangle}
          color="text-red-400"
          subtext="需要立即处理"
        />
        <KpiCard
          title="流量异常告警"
          value={kpiData.trafficAlerts === null ? '—' : kpiData.trafficAlerts}
          icon={TrendingUp}
          color="text-amber-400"
          subtext="24小时内"
        />
        <KpiCard
          title="今日收入"
          value={kpiData.todayIncome === null ? '—' : `¥${kpiData.todayIncome.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}`}
          icon={DollarSign}
          color="text-zinc-100"
          subtext="截至当前时刻"
        />
      </div>

      <Card className="bg-zinc-900 border-zinc-800">
        <CardHeader className="pb-3">
          <CardTitle className="text-base flex items-center gap-2">
            <RefreshCw className="w-4 h-4 text-red-400" />
            支付回调失败看板
          </CardTitle>
          <CardDescription className="text-zinc-500">
            共 {failedPayments.length} 条失败记录，{pendingPayments.length} 条待处理
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Tabs value={activeTab} onValueChange={setActiveTab}>
            <TabsList className="bg-zinc-800/50 mb-4">
              <TabsTrigger value="pending">待处理 ({pendingPayments.length})</TabsTrigger>
              <TabsTrigger value="replayed">已重放 ({replayedPayments.length})</TabsTrigger>
              <TabsTrigger value="all">全部 ({failedPayments.length})</TabsTrigger>
            </TabsList>

            <TabsContent value={activeTab}>
              {getFilteredPayments().length === 0 ? (
                <div className="py-8 text-center text-zinc-500">
                  <Receipt className="w-10 h-10 mx-auto mb-2 opacity-30" />
                  <p className="text-sm">暂无失败支付记录</p>
                </div>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow className="border-zinc-800 hover:bg-transparent">
                      <TableHead>订单号</TableHead>
                      <TableHead>用户</TableHead>
                      <TableHead>金额</TableHead>
                      <TableHead>支付渠道</TableHead>
                      <TableHead>失败原因</TableHead>
                      <TableHead>失败时间</TableHead>
                      <TableHead>操作</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {getFilteredPayments().map((payment) => (
                      <TableRow key={payment.id} className="border-zinc-800">
                        <TableCell className="font-mono text-xs text-zinc-300">{payment.orderNo}</TableCell>
                        <TableCell className="text-zinc-400 text-sm">{payment.user}</TableCell>
                        <TableCell className="text-zinc-200 font-medium">¥{payment.amount.toFixed(2)}</TableCell>
                        <TableCell>
                          <Badge variant="secondary" className="bg-zinc-800 text-zinc-300">
                            {payment.channel}
                          </Badge>
                        </TableCell>
                        <TableCell>{getReasonBadge(payment.reason)}</TableCell>
                        <TableCell className="text-zinc-500 text-xs font-mono">{payment.time}</TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2">
                            {!payment.replayed && (
                              <Button
                                size="sm"
                                variant="outline"
                                className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 h-7 px-2"
                                onClick={() => handleReplay(payment.id)}
                                disabled={replayingIds.has(payment.id)}
                                isLoading={replayingIds.has(payment.id)}
                              >
                                {!replayingIds.has(payment.id) && <Play className="w-3 h-3 mr-1" />}
                                一键重放
                              </Button>
                            )}
                            {payment.replayed && (
                              <Badge variant="success" className="bg-emerald-900/50 text-emerald-300">
                                已重放
                              </Badge>
                            )}
                            <Button
                              size="sm"
                              variant="ghost"
                              className="text-zinc-400 hover:text-zinc-200 h-7 w-7 p-0"
                            >
                              <Eye className="w-4 h-4" />
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>

      <Card className="bg-zinc-900 border-zinc-800">
        <CardHeader className="pb-3">
          <CardTitle className="text-base flex items-center gap-2">
            <TrendingUp className="w-4 h-4 text-indigo-400" />
            流量事件审计
          </CardTitle>
          <CardDescription className="text-zinc-500">
            近期流量计费明细，含链路类型与倍率信息
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {trafficEvents.length === 0 ? (
            <div className="py-8 text-center text-zinc-500">
              <TrendingUp className="w-10 h-10 mx-auto mb-2 opacity-30" />
              <p className="text-sm">暂无流量事件记录</p>
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow className="border-zinc-800 hover:bg-transparent">
                  <TableHead>时间</TableHead>
                  <TableHead>用户</TableHead>
                  <TableHead>节点</TableHead>
                  <TableHead>流量(MB)</TableHead>
                  <TableHead>倍率</TableHead>
                  <TableHead>计费额</TableHead>
                  <TableHead>链路类型</TableHead>
                  <TableHead>备注</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {trafficEvents.map((event) => (
                  <TableRow key={event.id} className="border-zinc-800">
                    <TableCell className="text-zinc-500 text-xs font-mono">{event.time}</TableCell>
                    <TableCell className="text-zinc-400 text-sm">{event.user}</TableCell>
                    <TableCell className="text-zinc-200 font-medium">{event.node}</TableCell>
                    <TableCell className="text-zinc-300">{event.trafficMB.toLocaleString()}</TableCell>
                    <TableCell>
                      <Badge
                        variant="outline"
                        className={
                          event.multiplier >= 1.5
                            ? 'bg-red-900/30 text-red-300 border-red-800/50'
                            : event.multiplier < 1
                            ? 'bg-emerald-900/30 text-emerald-300 border-emerald-800/50'
                            : 'bg-zinc-800 text-zinc-300 border-zinc-700'
                        }
                      >
                        {event.multiplier}x
                      </Badge>
                    </TableCell>
                    <TableCell className="text-zinc-300">¥{event.charge.toFixed(3)}</TableCell>
                    <TableCell>
                      <Badge
                        variant="outline"
                        className={
                          event.linkType === '直连'
                            ? 'bg-blue-900/30 text-blue-300 border-blue-800/50'
                            : 'bg-purple-900/30 text-purple-300 border-purple-800/50'
                        }
                      >
                        {event.linkType}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-zinc-500 text-xs max-w-xs truncate">
                      {event.note || '-'}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <Card className="bg-zinc-900 border-zinc-800">
        <CardHeader className="pb-3">
          <CardTitle className="text-base flex items-center gap-2">
            <History className="w-4 h-4 text-amber-400" />
            倍率变更审计
          </CardTitle>
          <CardDescription className="text-zinc-500">
            所有节点倍率调整记录及影响重算结果
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {multiplierChanges.length === 0 ? (
            <div className="py-8 text-center text-zinc-500">
              <History className="w-10 h-10 mx-auto mb-2 opacity-30" />
              <p className="text-sm">暂无倍率变更记录</p>
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow className="border-zinc-800 hover:bg-transparent">
                  <TableHead>时间</TableHead>
                  <TableHead>操作人</TableHead>
                  <TableHead>节点</TableHead>
                  <TableHead>倍率变更</TableHead>
                  <TableHead>影响流量重算结果</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {multiplierChanges.map((change) => (
                  <TableRow key={change.id} className="border-zinc-800">
                    <TableCell className="text-zinc-500 text-xs font-mono">{change.time}</TableCell>
                    <TableCell className="text-zinc-400 text-sm">{change.operator}</TableCell>
                    <TableCell className="text-zinc-200 font-medium">{change.node}</TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2 text-sm">
                        <span className={change.newMultiplier > change.oldMultiplier ? 'text-red-400' : 'text-emerald-400'}>
                          {change.oldMultiplier}x
                        </span>
                        <ArrowRight className="w-4 h-4 text-zinc-600" />
                        <span className={change.newMultiplier > change.oldMultiplier ? 'text-red-400 font-semibold' : 'text-emerald-400 font-semibold'}>
                          {change.newMultiplier}x
                        </span>
                      </div>
                    </TableCell>
                    <TableCell className="text-zinc-400 text-sm">{change.impact}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
