import { useState, useEffect, useCallback } from 'react'
import {
  Server,
  Activity,
  AlertTriangle,
  CheckCircle2,
  XCircle,
  Cpu,
  HardDrive,
  MemoryStick,
  Users,
  RefreshCw,
  Clock,
} from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle, Badge, Button, Skeleton } from '@airport/ui'
import { api } from '@/lib/api'
import { EP } from '@/lib/endpoints'
import { ADMIN_CARD, ADMIN_BORDER, ADMIN_TEXT, ADMIN_TEXT_SECONDARY, ADMIN_TEXT_MUTED, ADMIN_SUCCESS, ADMIN_WARNING, ADMIN_DANGER, ADMIN_INFO } from '@/lib/theme'

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

function formatUptime(seconds: number): string {
  if (seconds === 0) return '-'
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (days > 0) return `${days}天 ${hours}小时`
  if (hours > 0) return `${hours}小时 ${minutes}分钟`
  return `${minutes}分钟`
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
      const resp = await api.get(EP.SERVER_HEALTH_DASHBOARD)
      setData(resp.data)
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

  return (
    <div className="space-y-6">
      {/* 页面标题 */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className={`text-2xl font-bold ${ADMIN_TEXT}`}>节点健康仪表盘</h1>
          <p className={`text-sm ${ADMIN_TEXT_MUTED}`}>实时监控所有 VPS 的运行状态</p>
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

      {/* 服务器健康列表 */}
      <div className="space-y-3">
        {servers.map((srv) => (
          <Card key={srv.id} className={`${ADMIN_CARD} ${ADMIN_BORDER}`}>
            <CardHeader className="pb-3">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className={`h-3 w-3 rounded-full ${srv.online ? 'bg-emerald-500' : 'bg-red-500'}`} />
                  <CardTitle className={ADMIN_TEXT}>{srv.name || srv.code}</CardTitle>
                  <Badge variant="outline" className={ADMIN_TEXT_MUTED}>
                    {srv.code}
                  </Badge>
                  {srv.agent_version && (
                    <Badge variant="outline" className={ADMIN_INFO}>
                      Agent {srv.agent_version}
                    </Badge>
                  )}
                </div>
                <div className="flex items-center gap-2">
                  {srv.alerts?.map((alert) => (
                    <Badge key={alert} className={`border ${getAlertBadgeClass(alert)}`}>
                      {alertLabels[alert] || alert}
                    </Badge>
                  ))}
                </div>
              </div>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 md:grid-cols-6 gap-4 text-sm">
                {/* 心跳 */}
                <div className="flex items-center gap-2">
                  <Clock className="h-4 w-4 text-zinc-500" />
                  <div>
                    <p className={ADMIN_TEXT_MUTED}>心跳</p>
                    <p className={ADMIN_TEXT_SECONDARY}>{formatHeartbeat(srv.last_heartbeat_at)}</p>
                  </div>
                </div>

                {/* 在线用户 */}
                <div className="flex items-center gap-2">
                  <Users className="h-4 w-4 text-zinc-500" />
                  <div>
                    <p className={ADMIN_TEXT_MUTED}>在线用户</p>
                    <p className={ADMIN_TEXT_SECONDARY}>{srv.online_users}</p>
                  </div>
                </div>

                {/* CPU */}
                <div className="flex items-center gap-2">
                  <Cpu className="h-4 w-4 text-zinc-500" />
                  <div>
                    <p className={ADMIN_TEXT_MUTED}>CPU</p>
                    <p className={srv.cpu_percent > 90 ? ADMIN_DANGER : srv.cpu_percent > 70 ? ADMIN_WARNING : ADMIN_SUCCESS}>
                      {srv.cpu_percent.toFixed(1)}%
                    </p>
                  </div>
                </div>

                {/* 内存 */}
                <div className="flex items-center gap-2">
                  <MemoryStick className="h-4 w-4 text-zinc-500" />
                  <div>
                    <p className={ADMIN_TEXT_MUTED}>内存</p>
                    <p className={srv.mem_percent > 90 ? ADMIN_DANGER : srv.mem_percent > 70 ? ADMIN_WARNING : ADMIN_SUCCESS}>
                      {srv.mem_percent.toFixed(1)}%
                    </p>
                  </div>
                </div>

                {/* 磁盘 */}
                <div className="flex items-center gap-2">
                  <HardDrive className="h-4 w-4 text-zinc-500" />
                  <div>
                    <p className={ADMIN_TEXT_MUTED}>磁盘</p>
                    <p className={srv.disk_percent > 85 ? ADMIN_DANGER : srv.disk_percent > 70 ? ADMIN_WARNING : ADMIN_SUCCESS}>
                      {srv.disk_percent.toFixed(1)}%
                    </p>
                  </div>
                </div>

                {/* 运行时间 */}
                <div className="flex items-center gap-2">
                  <Activity className="h-4 w-4 text-zinc-500" />
                  <div>
                    <p className={ADMIN_TEXT_MUTED}>运行时间</p>
                    <p className={ADMIN_TEXT_SECONDARY}>{formatUptime(srv.uptime_seconds)}</p>
                  </div>
                </div>
              </div>

              {/* 内核状态 */}
              {srv.runtimes.length > 0 && (
                <div className="mt-3 flex items-center gap-4 border-t border-zinc-800 pt-3">
                  <span className={`text-xs ${ADMIN_TEXT_MUTED}`}>内核状态:</span>
                  {srv.runtimes.map((rt, i) => (
                    <div key={i} className="flex items-center gap-1">
                      <span className={`text-xs ${ADMIN_TEXT_SECONDARY}`}>
                        {rt.type === 'sing-box' ? 'sing-box' : rt.type}
                      </span>
                      <Badge
                        className={
                          rt.status === 'running'
                            ? 'bg-emerald-900/50 text-emerald-300 border-emerald-800/50'
                            : 'bg-red-900/50 text-red-300 border-red-800/50'
                        }
                      >
                        {rt.status}
                      </Badge>
                      {rt.restart_count > 0 && (
                        <span className={`text-xs ${ADMIN_WARNING}`}>
                          (重启 {rt.restart_count} 次)
                        </span>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  )
}
