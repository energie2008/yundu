import { useState, useEffect, useCallback } from 'react'
import {
  Server,
  AlertTriangle,
  CheckCircle2,
  XCircle,
  Cpu,
  HardDrive,
  MemoryStick,
  RefreshCw,
  Clock,
} from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle, Badge, Button, Skeleton } from '@airport/ui'
import { api } from '@/lib/api'
import { EP } from '@/lib/endpoints'
import { ADMIN_CARD, ADMIN_BORDER, ADMIN_TEXT, ADMIN_TEXT_SECONDARY, ADMIN_TEXT_MUTED, ADMIN_SUCCESS, ADMIN_WARNING, ADMIN_DANGER } from '@/lib/theme'

interface ServerHealth {
  id: string
  code: string
  name: string
  host: string
  status: string
  online: boolean
  agent_version: string
  last_heartbeat_at?: string
  node_count: number
  online_users: number
  cpu_percent: number
  mem_percent: number
  disk_percent: number
  uptime_seconds: number
  runtimes: Array<{
    type: string
    status: string
    restart_count: number
    uptime_seconds: number
    last_heartbeat_at?: string
  }>
  alerts?: string[]
}

interface HealthDashboard {
  summary: {
    total_servers: number
    online: number
    offline: number
    alert_count: number
  }
  servers: ServerHealth[]
}

const alertLabels: Record<string, string> = {
  heartbeat_stale: '心跳超时',
  no_heartbeat: '无心跳',
  disk_high: '磁盘空间不足',
  cpu_high: 'CPU 过载',
  kernel_restart_frequent: '内核频繁重启',
  server_offline: '服务器离线',
  server_retired: '服务器已退役',
}

function formatHeartbeat(isoString?: string): string {
  if (!isoString) return '从未'
  const diff = Date.now() - new Date(isoString).getTime()
  const seconds = Math.floor(diff / 1000)
  if (seconds < 60) return `${seconds}秒前`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}分钟前`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}小时前`
  return `${Math.floor(hours / 24)}天前`
}

function getAlertBadgeClass(alert: string): string {
  switch (alert) {
    case 'heartbeat_stale':
    case 'no_heartbeat':
    case 'server_offline':
    case 'server_retired':
      return 'bg-red-900/50 text-red-300 border-red-800/50'
    case 'disk_high':
    case 'cpu_high':
    case 'kernel_restart_frequent':
      return 'bg-amber-900/50 text-amber-300 border-amber-800/50'
    default:
      return 'bg-zinc-800 text-zinc-400 border-zinc-700'
  }
}

export default function HealthDashboardPage() {
  const [data, setData] = useState<HealthDashboard | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fetchData = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const resp = await api.get<any>(EP.SERVER_HEALTH_DASHBOARD)
      setData(resp.data ?? resp)
    } catch (err: any) {
      setError(err?.message || '获取健康数据失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchData()
    // 每 30s 自动刷新
    const interval = setInterval(fetchData, 30000)
    return () => clearInterval(interval)
  }, [fetchData])

  if (loading && !data) {
    return (
      <div className="space-y-4">
        {[...Array(3)].map((_, i) => (
          <Skeleton key={i} className="h-32 w-full" />
        ))}
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className={`rounded-lg border p-6 ${ADMIN_CARD} ${ADMIN_BORDER}`}>
          <AlertTriangle className="h-8 w-8 text-red-400 mx-auto mb-2" />
          <p className={`text-center ${ADMIN_TEXT}`}>{error}</p>
          <Button onClick={fetchData} className="mt-4 mx-auto block" variant="outline" size="sm">
            <RefreshCw className="h-4 w-4 mr-1" /> 重试
          </Button>
        </div>
      </div>
    )
  }

  if (!data) return null

  const { summary, servers } = data

  // 健康仪表盘聚焦"告警与异常"，与"服务器管理"（逐台完整指标）区分：
  // 只把离线/有告警的服务器列入告警中心并放大呈现，健康服务器折叠为紧凑徽章条
  const problemServers = servers.filter((s) => !s.online || (s.alerts && s.alerts.length > 0))
  const healthyServers = servers.filter((s) => s.online && (!s.alerts || s.alerts.length === 0))

  return (
    <div className="space-y-6">
      {/* 页面标题 */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className={`text-2xl font-bold ${ADMIN_TEXT}`}>节点健康仪表盘</h1>
          <p className={`text-sm ${ADMIN_TEXT_MUTED}`}>聚焦告警与异常，逐台完整指标请见「服务器管理」</p>
        </div>
        <Button onClick={fetchData} variant="outline" size="sm" disabled={loading}>
          <RefreshCw className={`h-4 w-4 mr-1 ${loading ? 'animate-spin' : ''}`} /> 刷新
        </Button>
      </div>

      {/* 汇总卡片 */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <Card className={`${ADMIN_CARD} ${ADMIN_BORDER}`}>
          <CardContent className="pt-6">
            <div className="flex items-center justify-between">
              <div>
                <p className={`text-sm ${ADMIN_TEXT_MUTED}`}>总服务器</p>
                <p className={`text-2xl font-bold ${ADMIN_TEXT}`}>{summary.total_servers}</p>
              </div>
              <Server className="h-8 w-8 text-blue-400" />
            </div>
          </CardContent>
        </Card>

        <Card className={`${ADMIN_CARD} ${ADMIN_BORDER}`}>
          <CardContent className="pt-6">
            <div className="flex items-center justify-between">
              <div>
                <p className={`text-sm ${ADMIN_TEXT_MUTED}`}>在线</p>
                <p className={`text-2xl font-bold ${ADMIN_SUCCESS}`}>{summary.online}</p>
              </div>
              <CheckCircle2 className="h-8 w-8 text-emerald-400" />
            </div>
          </CardContent>
        </Card>

        <Card className={`${ADMIN_CARD} ${ADMIN_BORDER}`}>
          <CardContent className="pt-6">
            <div className="flex items-center justify-between">
              <div>
                <p className={`text-sm ${ADMIN_TEXT_MUTED}`}>离线</p>
                <p className={`text-2xl font-bold ${ADMIN_DANGER}`}>{summary.offline}</p>
              </div>
              <XCircle className="h-8 w-8 text-red-400" />
            </div>
          </CardContent>
        </Card>

        <Card className={`${ADMIN_CARD} ${ADMIN_BORDER}`}>
          <CardContent className="pt-6">
            <div className="flex items-center justify-between">
              <div>
                <p className={`text-sm ${ADMIN_TEXT_MUTED}`}>告警</p>
                <p className={`text-2xl font-bold ${ADMIN_WARNING}`}>{summary.alert_count}</p>
              </div>
              <AlertTriangle className="h-8 w-8 text-amber-400" />
            </div>
          </CardContent>
        </Card>
      </div>

      {/* 告警中心：仅列出离线或有告警的服务器 */}
      <div>
        <div className="flex items-center gap-2 mb-3">
          <AlertTriangle className="h-5 w-5 text-amber-400" />
          <h2 className={`text-lg font-semibold ${ADMIN_TEXT}`}>告警中心</h2>
          <Badge className="bg-amber-900/50 text-amber-300 border border-amber-800/50">{problemServers.length}</Badge>
        </div>

        {problemServers.length === 0 ? (
          <Card className={`${ADMIN_CARD} ${ADMIN_BORDER}`}>
            <CardContent className="py-10">
              <div className="flex flex-col items-center justify-center gap-2">
                <CheckCircle2 className="h-10 w-10 text-emerald-400" />
                <p className={ADMIN_TEXT}>所有服务器运行正常，暂无告警</p>
              </div>
            </CardContent>
          </Card>
        ) : (
          <div className="space-y-3">
            {problemServers.map((srv) => (
              <Card key={srv.id} className={`${ADMIN_CARD} border-l-4 ${srv.online ? 'border-l-amber-500' : 'border-l-red-500'} ${ADMIN_BORDER}`}>
                <CardHeader className="pb-3">
                  <div className="flex items-center justify-between flex-wrap gap-2">
                    <div className="flex items-center gap-3">
                      <div className={`h-3 w-3 rounded-full ${srv.online ? 'bg-amber-500' : 'bg-red-500'}`} />
                      <CardTitle className={ADMIN_TEXT}>{srv.name || srv.code}</CardTitle>
                      <Badge variant="outline" className={ADMIN_TEXT_MUTED}>{srv.code}</Badge>
                      {!srv.online && (
                        <Badge className="border bg-red-900/50 text-red-300 border-red-800/50">离线</Badge>
                      )}
                    </div>
                    <div className="flex items-center gap-2 flex-wrap">
                      {srv.alerts?.map((alert) => (
                        <Badge key={alert} className={`border ${getAlertBadgeClass(alert)}`}>
                          {alertLabels[alert] || alert}
                        </Badge>
                      ))}
                    </div>
                  </div>
                </CardHeader>
                <CardContent>
                  {/* 只呈现与告警相关的关键指标，避免与服务器管理页重复铺陈全量指标 */}
                  <div className="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
                    <div className="flex items-center gap-2">
                      <Clock className="h-4 w-4 text-zinc-500" />
                      <div>
                        <p className={ADMIN_TEXT_MUTED}>最后心跳</p>
                        <p className={ADMIN_TEXT_SECONDARY}>{formatHeartbeat(srv.last_heartbeat_at)}</p>
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      <Cpu className="h-4 w-4 text-zinc-500" />
                      <div>
                        <p className={ADMIN_TEXT_MUTED}>CPU</p>
                        <p className={srv.cpu_percent > 90 ? ADMIN_DANGER : srv.cpu_percent > 70 ? ADMIN_WARNING : ADMIN_SUCCESS}>
                          {srv.cpu_percent.toFixed(1)}%
                        </p>
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      <MemoryStick className="h-4 w-4 text-zinc-500" />
                      <div>
                        <p className={ADMIN_TEXT_MUTED}>内存</p>
                        <p className={srv.mem_percent > 90 ? ADMIN_DANGER : srv.mem_percent > 70 ? ADMIN_WARNING : ADMIN_SUCCESS}>
                          {srv.mem_percent.toFixed(1)}%
                        </p>
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      <HardDrive className="h-4 w-4 text-zinc-500" />
                      <div>
                        <p className={ADMIN_TEXT_MUTED}>磁盘</p>
                        <p className={srv.disk_percent > 85 ? ADMIN_DANGER : srv.disk_percent > 70 ? ADMIN_WARNING : ADMIN_SUCCESS}>
                          {srv.disk_percent.toFixed(1)}%
                        </p>
                      </div>
                    </div>
                  </div>

                  {/* 内核异常（仅非 running 或有重启的内核） */}
                  {srv.runtimes.some((rt) => rt.status !== 'running' || rt.restart_count > 0) && (
                    <div className="mt-3 flex items-center gap-4 border-t border-zinc-800 pt-3 flex-wrap">
                      <span className={`text-xs ${ADMIN_TEXT_MUTED}`}>内核异常:</span>
                      {srv.runtimes
                        .filter((rt) => rt.status !== 'running' || rt.restart_count > 0)
                        .map((rt, i) => (
                          <div key={i} className="flex items-center gap-1">
                            <span className={`text-xs ${ADMIN_TEXT_SECONDARY}`}>{rt.type}</span>
                            <Badge
                              className={
                                rt.status === 'running'
                                  ? 'bg-amber-900/50 text-amber-300 border-amber-800/50'
                                  : 'bg-red-900/50 text-red-300 border-red-800/50'
                              }
                            >
                              {rt.status}
                            </Badge>
                            {rt.restart_count > 0 && (
                              <span className={`text-xs ${ADMIN_WARNING}`}>(重启 {rt.restart_count} 次)</span>
                            )}
                          </div>
                        ))}
                    </div>
                  )}
                </CardContent>
              </Card>
            ))}
          </div>
        )}
      </div>

      {/* 健康服务器：折叠为紧凑徽章条 */}
      {healthyServers.length > 0 && (
        <div>
          <div className="flex items-center gap-2 mb-3">
            <CheckCircle2 className="h-5 w-5 text-emerald-400" />
            <h2 className={`text-lg font-semibold ${ADMIN_TEXT}`}>运行正常</h2>
            <Badge className="bg-emerald-900/50 text-emerald-300 border border-emerald-800/50">{healthyServers.length}</Badge>
          </div>
          <Card className={`${ADMIN_CARD} ${ADMIN_BORDER}`}>
            <CardContent className="py-4">
              <div className="flex flex-wrap gap-2">
                {healthyServers.map((srv) => (
                  <span key={srv.id} className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full bg-emerald-900/20 border border-emerald-800/40 text-xs">
                    <span className="h-2 w-2 rounded-full bg-emerald-500" />
                    <span className={ADMIN_TEXT_SECONDARY}>{srv.name || srv.code}</span>
                  </span>
                ))}
              </div>
            </CardContent>
          </Card>
        </div>
      )}
    </div>
  )
}
