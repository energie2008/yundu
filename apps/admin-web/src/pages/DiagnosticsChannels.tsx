import { useState, useEffect, useCallback } from 'react'
import {
  Radio,
  Wifi,
  ArrowRightLeft,
  RefreshCw,
  Activity,
  AlertCircle,
  Server,
  Clock,
  ChevronRight,
  Loader2,
} from 'lucide-react'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Badge,
  Button,
  Skeleton,
  Select,
  useToast,
} from '@airport/ui'
import { api, ApiError } from '@/lib/api'
import { EP } from '@/lib/endpoints'

// ===== 类型定义 =====
type ChannelName = 'grpc' | 'ws' | 'http' | string
type ChannelState = 'healthy' | 'degraded' | 'unhealthy' | 'unknown'

interface ChannelHealthItem {
  server_id: string
  server_code: string
  server_name: string
  active_channel: ChannelName
  channel_state: ChannelState
  rtt_ms?: number | null
  fail_count_1h: number
  online_users: number
  failover_count_1h: number
  failover_count_24h: number
  last_failover_at?: string | null
  last_failover_from?: string | null
  last_failover_to?: string | null
  last_failover_reason?: string | null
  updated_at: string
}

interface FailoverEvent {
  id: number
  server_id: string
  from_channel: string
  to_channel: string
  reason: string
  detail?: string
  occurred_at: string
}

interface ListResponse<T> {
  page: number
  page_size: number
  total: number
  items: T[]
}

// ===== 通道元数据配置 =====
const channelMeta: Record<string, { label: string; badgeClass: string; iconClass: string; priority: number }> = {
  grpc: {
    label: 'gRPC',
    badgeClass: 'bg-emerald-900/50 text-emerald-300 border-emerald-800/50 hover:bg-emerald-900/50',
    iconClass: 'text-emerald-400',
    priority: 0,
  },
  ws: {
    label: 'WebSocket',
    badgeClass: 'bg-amber-900/50 text-amber-300 border-amber-800/50 hover:bg-amber-900/50',
    iconClass: 'text-amber-400',
    priority: 1,
  },
  http: {
    label: 'HTTP',
    badgeClass: 'bg-red-900/50 text-red-300 border-red-800/50 hover:bg-red-900/50',
    iconClass: 'text-red-400',
    priority: 2,
  },
}

function getChannelMeta(channel: string) {
  return channelMeta[channel.toLowerCase()] || {
    label: channel || '未知',
    badgeClass: 'bg-zinc-800 text-zinc-300 border-zinc-700',
    iconClass: 'text-zinc-400',
    priority: 99,
  }
}

function ChannelIcon({ channel, className }: { channel: string; className?: string }) {
  const c = (channel || '').toLowerCase()
  if (c === 'grpc') return <Radio className={className} />
  if (c === 'ws') return <Wifi className={className} />
  return <Activity className={className} />
}

const reasonLabels: Record<string, string> = {
  heartbeat_timeout: '连续3次心跳超时',
  manual_switch: '手动切换',
  auto_recovery: '自动恢复',
  connection_error: '连接错误',
  initial_connect: '初始连接',
}

function normalizeList<T>(data: unknown): T[] {
  if (Array.isArray(data)) return data as T[]
  if (data && typeof data === 'object') {
    const obj = data as Record<string, unknown>
    if (Array.isArray(obj.items)) return obj.items as T[]
    if (Array.isArray(obj.list)) return obj.list as T[]
    const dataField = obj.data
    if (dataField && typeof dataField === 'object') {
      if (Array.isArray(dataField)) return dataField as T[]
      const dataObj = dataField as Record<string, unknown>
      if (Array.isArray(dataObj.items)) return dataObj.items as T[]
      if (Array.isArray(dataObj.list)) return dataObj.list as T[]
    }
  }
  return []
}

function formatRelativeTime(dateStr?: string | null): string {
  if (!dateStr) return '无'
  const date = new Date(dateStr)
  if (isNaN(date.getTime())) return '无'
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffSec = Math.floor(diffMs / 1000)
  const diffMin = Math.floor(diffSec / 60)
  const diffHour = Math.floor(diffMin / 60)

  if (diffSec < 60) return `${diffSec}秒前`
  if (diffMin < 60) return `${diffMin}分钟前`
  if (diffHour < 24) return `${diffHour}小时前`
  return date.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

function formatAbsoluteTime(dateStr?: string | null): string {
  if (!dateStr) return '-'
  const date = new Date(dateStr)
  if (isNaN(date.getTime())) return '-'
  return date.toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

function getLatencyColor(ms: number | null | undefined): string {
  if (ms == null) return 'text-zinc-500'
  if (ms < 50) return 'text-emerald-400'
  if (ms < 150) return 'text-amber-400'
  return 'text-red-400'
}

function getLatencyProgress(ms: number | null | undefined): number {
  if (ms == null) return 0
  return Math.min(100, (ms / 300) * 100)
}

function getStateBadge(state: ChannelState) {
  const map: Record<ChannelState, { label: string; dot: string }> = {
    healthy: { label: '健康', dot: 'bg-emerald-500' },
    degraded: { label: '降级', dot: 'bg-amber-500' },
    unhealthy: { label: '异常', dot: 'bg-red-500' },
    unknown: { label: '未知', dot: 'bg-zinc-500' },
  }
  const v = map[state] || map.unknown
  return (
    <span className="inline-flex items-center gap-1.5 text-xs">
      <span className={`w-2 h-2 rounded-full ${v.dot}`} />
      <span className="text-zinc-400">{v.label}</span>
    </span>
  )
}

// ===== 主组件 =====
export default function DiagnosticsChannels() {
  const { toast } = useToast()

  const [healthList, setHealthList] = useState<ChannelHealthItem[]>([])
  const [healthLoading, setHealthLoading] = useState(true)
  const [failoverEvents, setFailoverEvents] = useState<FailoverEvent[]>([])
  const [failoverLoading, setFailoverLoading] = useState(true)
  const [switchingServers, setSwitchingServers] = useState<Set<string>>(new Set())

  // 切换通道弹窗状态
  const [switchDialogServer, setSwitchDialogServer] = useState<ChannelHealthItem | null>(null)
  const [switchTargetChannel, setSwitchTargetChannel] = useState<'grpc' | 'ws' | 'http'>('grpc')
  const [switchReason, setSwitchReason] = useState('')

  const fetchHealth = useCallback(async () => {
    setHealthLoading(true)
    try {
      const data = await api.get<unknown>(EP.CHANNEL_HEALTH, {
        params: { page: 1, page_size: 100 },
      })
      setHealthList(normalizeList<ChannelHealthItem>(data))
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '加载通道健康数据失败'
      toast({ title: '加载失败', description: msg, variant: 'destructive' })
      setHealthList([])
    } finally {
      setHealthLoading(false)
    }
  }, [toast])

  const fetchFailoverEvents = useCallback(async () => {
    setFailoverLoading(true)
    try {
      const data = await api.get<unknown>(EP.CHANNEL_FAILOVER_EVENTS, {
        params: { page: 1, page_size: 30 },
      })
      setFailoverEvents(normalizeList<FailoverEvent>(data))
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '加载降级事件失败'
      toast({ title: '加载失败', description: msg, variant: 'destructive' })
      setFailoverEvents([])
    } finally {
      setFailoverLoading(false)
    }
  }, [toast])

  useEffect(() => {
    fetchHealth()
    fetchFailoverEvents()
  }, [fetchHealth, fetchFailoverEvents])

  const handleRefresh = () => {
    fetchHealth()
    fetchFailoverEvents()
  }

  const handleForceReconnect = async (item: ChannelHealthItem) => {
    // 通过 manual switch 接口强制"切换"回当前通道（相当于重连）
    setSwitchingServers((prev) => new Set(prev).add(item.server_id))
    try {
      await api.post(EP.CHANNEL_SWITCH, {
        target_channel: item.active_channel,
        reason: 'manual_reconnect',
      })
      toast({
        title: '已下发重连指令',
        description: `${item.server_name} 通道重连请求已加入队列`,
      })
      // 延迟刷新
      setTimeout(() => fetchHealth(), 1500)
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '下发重连指令失败'
      toast({ title: '操作失败', description: msg, variant: 'destructive' })
    } finally {
      setSwitchingServers((prev) => {
        const next = new Set(prev)
        next.delete(item.server_id)
        return next
      })
    }
  }

  const handleSwitchSubmit = async () => {
    if (!switchDialogServer) return
    setSwitchingServers((prev) => new Set(prev).add(switchDialogServer.server_id))
    try {
      await api.post(EP.CHANNEL_SWITCH, {
        target_channel: switchTargetChannel,
        reason: switchReason || `manual_switch_to_${switchTargetChannel}`,
      })
      toast({
        title: '已下发通道切换指令',
        description: `${switchDialogServer.server_name} → ${getChannelMeta(switchTargetChannel).label}`,
      })
      setSwitchDialogServer(null)
      setSwitchReason('')
      setTimeout(() => fetchHealth(), 1500)
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '通道切换失败'
      toast({ title: '操作失败', description: msg, variant: 'destructive' })
    } finally {
      setSwitchingServers((prev) => {
        const next = new Set(prev)
        next.delete(switchDialogServer?.server_id || '')
        return next
      })
    }
  }

  // 聚合统计
  const healthyCount = healthList.filter((n) => n.channel_state === 'healthy').length
  const degradedCount = healthList.filter((n) => n.channel_state === 'degraded').length
  const unhealthyCount = healthList.filter((n) => n.channel_state === 'unhealthy').length
  const totalFailover24h = healthList.reduce((sum, n) => sum + (n.failover_count_24h || 0), 0)
  const lastFailoverAt = failoverEvents[0]?.occurred_at || null

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold text-zinc-100 flex items-center gap-2">
            <Activity className="w-5 h-5 text-emerald-400" />
            通道健康
          </h2>
          <p className="text-sm text-zinc-500 mt-0.5">
            实时监控 node-agent 与面板的 gRPC/WS/HTTP 三通道连接状态，自动降级与恢复时间线
          </p>
        </div>
        <Button variant="secondary" size="sm" onClick={handleRefresh} className="bg-zinc-800 hover:bg-zinc-700 border-zinc-700">
          <RefreshCw className="w-3.5 h-3.5 mr-1" />
          刷新
        </Button>
      </div>

      <Card className="bg-zinc-900 border-zinc-800">
        <CardContent className="p-4">
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
            <div className="space-y-1">
              <div className="text-xs text-zinc-500">监控服务器数</div>
              <div className="text-xl font-semibold text-zinc-100 mt-1">{healthList.length}</div>
            </div>
            <div className="space-y-1">
              <div className="text-xs text-zinc-500">24h 降级总次数</div>
              <div className="text-xl font-semibold text-zinc-100 mt-1">{totalFailover24h}</div>
            </div>
            <div className="space-y-1">
              <div className="text-xs text-zinc-500">最近 failover</div>
              <div className="text-sm text-zinc-300 mt-1 flex items-center gap-1">
                <Clock className="w-3 h-3" />
                {formatRelativeTime(lastFailoverAt)}
              </div>
            </div>
            <div className="space-y-1">
              <div className="text-xs text-zinc-500">通道状态分布</div>
              <div className="flex items-center gap-2 mt-1">
                <span className="flex items-center gap-1 text-xs">
                  <span className="w-2 h-2 rounded-full bg-emerald-500" />
                  <span className="text-emerald-400">{healthyCount}</span>
                </span>
                <span className="flex items-center gap-1 text-xs">
                  <span className="w-2 h-2 rounded-full bg-amber-500" />
                  <span className="text-amber-400">{degradedCount}</span>
                </span>
                <span className="flex items-center gap-1 text-xs">
                  <span className="w-2 h-2 rounded-full bg-red-500" />
                  <span className="text-red-400">{unhealthyCount}</span>
                </span>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card className="bg-zinc-900 border-zinc-800">
        <CardHeader className="pb-3">
          <CardTitle className="text-base flex items-center gap-2">
            <Server className="w-4 h-4 text-indigo-400" />
            节点通道详情
          </CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {healthLoading ? (
            <div className="p-4 space-y-2">
              {[1, 2, 3, 4].map((i) => (
                <Skeleton key={i} className="h-12 w-full" />
              ))}
            </div>
          ) : healthList.length === 0 ? (
            <div className="p-8 text-center text-sm text-zinc-500">
              暂无通道健康数据，等待 node-agent 上报心跳
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-zinc-800">
                    <th className="text-left text-xs font-medium text-zinc-500 px-4 py-3">服务器</th>
                    <th className="text-left text-xs font-medium text-zinc-500 px-4 py-3">活跃通道</th>
                    <th className="text-left text-xs font-medium text-zinc-500 px-4 py-3">延迟</th>
                    <th className="text-left text-xs font-medium text-zinc-500 px-4 py-3 hidden sm:table-cell">1h降级</th>
                    <th className="text-left text-xs font-medium text-zinc-500 px-4 py-3 hidden md:table-cell">最近切换</th>
                    <th className="text-right text-xs font-medium text-zinc-500 px-4 py-3">操作</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-zinc-800/50">
                  {healthList.map((node) => {
                    const cfg = getChannelMeta(node.active_channel)
                    const isSwitching = switchingServers.has(node.server_id)
                    return (
                      <tr key={node.server_id} className="hover:bg-zinc-800/30 transition-colors">
                        <td className="px-4 py-3">
                          <div className="flex items-center gap-2">
                            <span className={`w-2 h-2 rounded-full ${
                              node.channel_state === 'healthy' ? 'bg-emerald-500'
                              : node.channel_state === 'degraded' ? 'bg-amber-500'
                              : node.channel_state === 'unhealthy' ? 'bg-red-500'
                              : 'bg-zinc-500'
                            }`} />
                            <div>
                              <div className="text-sm font-medium text-zinc-200">{node.server_name}</div>
                              <div className="text-xs text-zinc-500">{node.server_code}</div>
                            </div>
                          </div>
                        </td>
                        <td className="px-4 py-3">
                          <div className="space-y-1">
                            <Badge className={`${cfg.badgeClass} hover:${cfg.badgeClass}`}>
                              <ChannelIcon channel={node.active_channel} className="w-3 h-3 mr-1" />
                              {cfg.label}
                            </Badge>
                            <div>{getStateBadge(node.channel_state)}</div>
                          </div>
                        </td>
                        <td className="px-4 py-3">
                          <div className="flex items-center gap-2">
                            <span className={`text-sm font-mono ${getLatencyColor(node.rtt_ms)}`}>
                              {node.rtt_ms != null ? `${node.rtt_ms}ms` : '-'}
                            </span>
                            <div className="w-16 hidden lg:block h-1.5 bg-zinc-800 rounded-full overflow-hidden">
                              <div
                                className={`h-full rounded-full transition-all ${
                                  node.rtt_ms == null ? 'bg-zinc-600'
                                  : node.rtt_ms < 50 ? 'bg-emerald-500'
                                  : node.rtt_ms < 150 ? 'bg-amber-500'
                                  : 'bg-red-500'
                                }`}
                                style={{ width: `${getLatencyProgress(node.rtt_ms)}%` }}
                              />
                            </div>
                          </div>
                        </td>
                        <td className="px-4 py-3 hidden sm:table-cell">
                          {node.failover_count_1h > 0 ? (
                            <span className={`text-sm ${node.failover_count_1h >= 3 ? 'text-red-400' : 'text-amber-400'}`}>
                              {node.failover_count_1h} 次
                            </span>
                          ) : (
                            <span className="text-sm text-zinc-500">0</span>
                          )}
                        </td>
                        <td className="px-4 py-3 hidden md:table-cell">
                          {node.last_failover_at ? (
                            <div className="flex items-center gap-1.5 text-xs text-zinc-400">
                              <span>{formatRelativeTime(node.last_failover_at)}</span>
                              {node.last_failover_from && node.last_failover_to && (
                                <>
                                  <ArrowRightLeft className="w-3 h-3 text-zinc-600" />
                                  <span className={getChannelMeta(node.last_failover_from).iconClass}>
                                    {getChannelMeta(node.last_failover_from).label}
                                  </span>
                                  <ChevronRight className="w-3 h-3 text-zinc-600" />
                                  <span className={getChannelMeta(node.last_failover_to).iconClass}>
                                    {getChannelMeta(node.last_failover_to).label}
                                  </span>
                                </>
                              )}
                            </div>
                          ) : (
                            <span className="text-xs text-zinc-600">无切换</span>
                          )}
                        </td>
                        <td className="px-4 py-3 text-right">
                          <div className="flex items-center justify-end gap-1">
                            <Button
                              variant="secondary"
                              size="sm"
                              onClick={() => handleForceReconnect(node)}
                              disabled={isSwitching}
                              className="bg-zinc-800 hover:bg-zinc-700 text-zinc-300 border-zinc-700"
                            >
                              <RefreshCw className={`w-3 h-3 mr-1 ${isSwitching ? 'animate-spin' : ''}`} />
                              重连
                            </Button>
                            <Button
                              variant="secondary"
                              size="sm"
                              onClick={() => {
                                setSwitchDialogServer(node)
                                setSwitchTargetChannel('grpc')
                                setSwitchReason('')
                              }}
                              disabled={isSwitching}
                              className="bg-zinc-800 hover:bg-zinc-700 text-zinc-300 border-zinc-700"
                            >
                              <ArrowRightLeft className="w-3 h-3 mr-1" />
                              切换
                            </Button>
                          </div>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <Card className="bg-zinc-900 border-zinc-800 lg:col-span-2">
          <CardHeader className="pb-3">
            <CardTitle className="text-base flex items-center gap-2">
              <ArrowRightLeft className="w-4 h-4 text-amber-400" />
              降级时间线
              <Badge variant="secondary" className="ml-1 bg-zinc-800 text-zinc-400 text-xs">
                最近 {failoverEvents.length} 次
              </Badge>
            </CardTitle>
          </CardHeader>
          <CardContent>
            {failoverLoading ? (
              <div className="space-y-2">
                {[1, 2, 3, 4, 5].map((i) => (
                  <Skeleton key={i} className="h-14 w-full" />
                ))}
              </div>
            ) : failoverEvents.length === 0 ? (
              <div className="p-6 text-center text-sm text-zinc-500">
                暂无降级事件，所有通道运行稳定
              </div>
            ) : (
              <div className="relative">
                <div className="absolute left-[15px] top-0 bottom-0 w-px bg-zinc-800" />
                <div className="space-y-0">
                  {failoverEvents.map((event) => {
                    const isRecovery = event.reason === 'auto_recovery'
                    const isError = event.reason === 'heartbeat_timeout' || event.reason === 'connection_error'
                    return (
                      <div key={event.id} className="relative pl-10 py-2.5 first:pt-0">
                        <div
                          className={`absolute left-3 top-3.5 w-3 h-3 rounded-full border-2 border-zinc-900 ${
                            isRecovery ? 'bg-emerald-500' : isError ? 'bg-red-500' : 'bg-amber-500'
                          }`}
                        />
                        <div className="rounded-lg border border-zinc-800 bg-zinc-950/30 p-3">
                          <div className="flex items-center justify-between gap-2 flex-wrap">
                            <div className="flex items-center gap-2">
                              <span className="text-sm font-medium text-zinc-200">
                                {healthList.find((n) => n.server_id === event.server_id)?.server_name || event.server_id.slice(0, 8)}
                              </span>
                              <div className="flex items-center gap-1 text-xs">
                                <span className={getChannelMeta(event.from_channel).iconClass}>
                                  {getChannelMeta(event.from_channel).label}
                                </span>
                                <ChevronRight className="w-3 h-3 text-zinc-600" />
                                <span className={getChannelMeta(event.to_channel).iconClass}>
                                  {getChannelMeta(event.to_channel).label}
                                </span>
                              </div>
                            </div>
                            <span className="text-xs text-zinc-500">{formatAbsoluteTime(event.occurred_at)}</span>
                          </div>
                          <div className="flex items-center gap-1.5 mt-1.5">
                            {isRecovery ? (
                              <Radio className="w-3 h-3 text-emerald-400" />
                            ) : isError ? (
                              <AlertCircle className="w-3 h-3 text-red-400" />
                            ) : (
                              <ArrowRightLeft className="w-3 h-3 text-amber-400" />
                            )}
                            <span className={`text-xs ${isRecovery ? 'text-emerald-400' : isError ? 'text-red-400' : 'text-zinc-400'}`}>
                              {reasonLabels[event.reason] || event.reason}
                            </span>
                            {event.detail && (
                              <span className="text-xs text-zinc-600 truncate">· {event.detail}</span>
                            )}
                          </div>
                        </div>
                      </div>
                    )
                  })}
                </div>
              </div>
            )}
          </CardContent>
        </Card>

        <Card className="bg-zinc-900 border-zinc-800">
          <CardHeader className="pb-3">
            <CardTitle className="text-base flex items-center gap-2">
              <Activity className="w-4 h-4 text-indigo-400" />
              通道优先级策略
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <p className="text-xs text-zinc-500">自动降级顺序</p>
              <div className="flex items-center gap-2 flex-wrap">
                <Badge className="bg-emerald-900/50 text-emerald-300 border-emerald-800/50">gRPC (0)</Badge>
                <ChevronRight className="w-4 h-4 text-zinc-600" />
                <Badge className="bg-amber-900/50 text-amber-300 border-amber-800/50">WS (1)</Badge>
                <ChevronRight className="w-4 h-4 text-zinc-600" />
                <Badge className="bg-red-900/50 text-red-300 border-red-800/50">HTTP (2)</Badge>
              </div>
            </div>

            <div className="space-y-2 pt-2 border-t border-zinc-800">
              <p className="text-xs text-zinc-500">降级触发条件</p>
              <div className="space-y-1.5">
                <div className="flex items-start gap-2">
                  <AlertCircle className="w-3.5 h-3.5 text-amber-400 mt-0.5 flex-shrink-0" />
                  <p className="text-xs text-zinc-400">连续 3 次健康检查失败，自动 failover 到下一优先级通道</p>
                </div>
              </div>
            </div>

            <div className="space-y-2 pt-2 border-t border-zinc-800">
              <p className="text-xs text-zinc-500">自动恢复机制</p>
              <div className="space-y-1.5">
                <div className="flex items-start gap-2">
                  <RefreshCw className="w-3.5 h-3.5 text-emerald-400 mt-0.5 flex-shrink-0" />
                  <p className="text-xs text-zinc-400">每 60 秒探测高优先级通道，连通后自动升级回 gRPC</p>
                </div>
              </div>
            </div>

            <div className="space-y-2 pt-2 border-t border-zinc-800">
              <p className="text-xs text-zinc-500">通道特性对比</p>
              <div className="space-y-1.5">
                <div className="flex items-center justify-between py-1">
                  <div className="flex items-center gap-2">
                    <Radio className="w-3.5 h-3.5 text-emerald-400" />
                    <span className="text-xs text-zinc-300">gRPC</span>
                  </div>
                  <span className="text-xs text-zinc-500">双向流 · 低延迟 · 首选</span>
                </div>
                <div className="flex items-center justify-between py-1">
                  <div className="flex items-center gap-2">
                    <Wifi className="w-3.5 h-3.5 text-amber-400" />
                    <span className="text-xs text-zinc-300">WebSocket</span>
                  </div>
                  <span className="text-xs text-zinc-500">全双工 · 中延迟 · 备用</span>
                </div>
                <div className="flex items-center justify-between py-1">
                  <div className="flex items-center gap-2">
                    <Activity className="w-3.5 h-3.5 text-red-400" />
                    <span className="text-xs text-zinc-300">HTTP</span>
                  </div>
                  <span className="text-xs text-zinc-500">轮询 · 高延迟 · 兜底</span>
                </div>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* 通道切换弹窗 */}
      {switchDialogServer && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
          <div className="bg-zinc-900 border border-zinc-800 rounded-lg p-6 w-[420px] max-w-[90vw] space-y-4">
            <div className="flex items-center justify-between">
              <h3 className="text-base font-semibold text-zinc-100 flex items-center gap-2">
                <ArrowRightLeft className="w-4 h-4 text-amber-400" />
                切换通道
              </h3>
              <button
                onClick={() => setSwitchDialogServer(null)}
                className="text-zinc-500 hover:text-zinc-300"
              >
                ✕
              </button>
            </div>
            <div className="space-y-3">
              <div>
                <label className="text-xs text-zinc-500">目标服务器</label>
                <div className="text-sm text-zinc-200 mt-1">
                  {switchDialogServer.server_name} ({switchDialogServer.server_code})
                </div>
                <div className="text-xs text-zinc-500 mt-0.5">
                  当前通道: {getChannelMeta(switchDialogServer.active_channel).label}
                </div>
              </div>
              <div>
                <label className="text-xs text-zinc-500">目标通道</label>
                <Select
                  value={switchTargetChannel}
                  onChange={(e) => setSwitchTargetChannel(e.target.value as 'grpc' | 'ws' | 'http')}
                  className="mt-1"
                >
                  <option value="grpc">gRPC (首选)</option>
                  <option value="ws">WebSocket (备用)</option>
                  <option value="http">HTTP (兜底)</option>
                </Select>
              </div>
              <div>
                <label className="text-xs text-zinc-500">切换原因（可选）</label>
                <input
                  type="text"
                  value={switchReason}
                  onChange={(e) => setSwitchReason(e.target.value)}
                  placeholder="例如：排障切换"
                  className="mt-1 w-full px-3 py-2 bg-zinc-950 border border-zinc-800 rounded-md text-sm text-zinc-200 focus:outline-none focus:border-zinc-600"
                />
              </div>
            </div>
            <div className="flex justify-end gap-2 pt-2">
              <Button
                variant="secondary"
                onClick={() => setSwitchDialogServer(null)}
                className="bg-zinc-800 hover:bg-zinc-700 border-zinc-700"
              >
                取消
              </Button>
              <Button
                onClick={handleSwitchSubmit}
                disabled={switchingServers.has(switchDialogServer.server_id)}
                className="bg-amber-600 hover:bg-amber-500 text-white"
              >
                {switchingServers.has(switchDialogServer.server_id) ? (
                  <><Loader2 className="w-4 h-4 mr-1 animate-spin" /> 切换中</>
                ) : (
                  <><ArrowRightLeft className="w-4 h-4 mr-1" /> 确认切换</>
                )}
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
