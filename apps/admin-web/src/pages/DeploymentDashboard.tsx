import { useState, useEffect } from 'react'
import {
  Rocket, CheckCircle2, XCircle, Clock, Loader2, RefreshCw,
  Server, TrendingUp, AlertTriangle, Activity
} from 'lucide-react'
import {
  Card, CardContent, CardHeader, CardTitle,
  Badge, Button, Skeleton, useToast
} from '@airport/ui'
import { api, ApiError } from '@/lib/api'
import { EP } from '@/lib/endpoints'

interface DeploymentTarget {
  id: string
  status: string
  target_type: string
  target_id: string
  started_at?: string
  finished_at?: string
}

interface DeploymentBatch {
  id: string
  scope_type: string
  scope_id: string
  strategy: string
  status: string
  created_at: string
  targets?: DeploymentTarget[]
}

interface DashboardStats {
  total: number
  success: number
  failed: number
  inProgress: number
  successRate: number
}

function computeStats(batches: DeploymentBatch[]): DashboardStats {
  const total = batches.length
  let success = 0, failed = 0, inProgress = 0
  for (const b of batches) {
    if (b.status === 'success') success++
    else if (b.status === 'failed') failed++
    else if (['pending', 'precheck', 'applying', 'verifying', 'paused'].includes(b.status)) inProgress++
  }
  return {
    total, success, failed, inProgress,
    successRate: total > 0 ? Math.round((success / total) * 100) : 0,
  }
}

function StatusBadge({ status }: { status: string }) {
  const config: Record<string, { color: string; icon: React.ReactNode }> = {
    success: { color: 'bg-green-100 text-green-700 border-green-200', icon: <CheckCircle2 className="w-3 h-3" /> },
    failed: { color: 'bg-red-100 text-red-700 border-red-200', icon: <XCircle className="w-3 h-3" /> },
    pending: { color: 'bg-gray-100 text-gray-600 border-gray-200', icon: <Clock className="w-3 h-3" /> },
    applying: { color: 'bg-blue-100 text-blue-700 border-blue-200', icon: <Loader2 className="w-3 h-3 animate-spin" /> },
    precheck: { color: 'bg-yellow-100 text-yellow-700 border-yellow-200', icon: <Clock className="w-3 h-3" /> },
    verifying: { color: 'bg-purple-100 text-purple-700 border-purple-200', icon: <Loader2 className="w-3 h-3 animate-spin" /> },
    rolling_back: { color: 'bg-orange-100 text-orange-700 border-orange-200', icon: <AlertTriangle className="w-3 h-3" /> },
    rolled_back: { color: 'bg-gray-100 text-gray-700 border-gray-200', icon: <XCircle className="w-3 h-3" /> },
    paused: { color: 'bg-yellow-100 text-yellow-700 border-yellow-200', icon: <Clock className="w-3 h-3" /> },
  }
  const c = config[status] || config.pending
  return (
    <Badge variant="outline" className={`text-xs ${c.color}`}>
      <span className="flex items-center gap-1">{c.icon}{status}</span>
    </Badge>
  )
}

function StatCard({ icon, label, value, color }: { icon: React.ReactNode; label: string; value: number | string; color: string }) {
  return (
    <Card>
      <CardContent className="p-4">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-xs text-muted-foreground mb-1">{label}</p>
            <p className="text-2xl font-bold">{value}</p>
          </div>
          <div className={`w-10 h-10 rounded-lg flex items-center justify-center ${color}`}>
            {icon}
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

export default function DeploymentDashboard() {
  const [batches, setBatches] = useState<DeploymentBatch[]>([])
  const [loading, setLoading] = useState(true)
  const [lastRefresh, setLastRefresh] = useState<Date>(new Date())
  const { toast } = useToast()

  const fetchData = async () => {
    try {
      const resp = await api.get(EP.DEPLOYMENTS, { params: { limit: 50 } })
      const data = (resp as any)?.data || resp
      setBatches(Array.isArray(data) ? data : (data?.items || []))
    } catch (err) {
      if (err instanceof ApiError) {
        toast({ title: '加载失败', description: err.message, variant: 'destructive' })
      }
    } finally {
      setLoading(false)
      setLastRefresh(new Date())
    }
  }

  useEffect(() => {
    fetchData()
    const interval = setInterval(fetchData, 10000)
    return () => clearInterval(interval)
  }, [])

  const stats = computeStats(batches)
  const recentBatches = batches.slice(0, 20)

  return (
    <div className="space-y-6 p-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <Activity className="w-6 h-6" />
            部署状态大盘
          </h1>
          <p className="text-sm text-muted-foreground mt-1">
            实时部署健康概览 · 最后更新: {lastRefresh.toLocaleTimeString('zh-CN')}
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={fetchData} disabled={loading}>
          <RefreshCw className={`w-4 h-4 mr-2 ${loading ? 'animate-spin' : ''}`} />
          刷新
        </Button>
      </div>

      {/* Summary Cards */}
      <div className="grid grid-cols-1 md:grid-cols-5 gap-4">
        <StatCard icon={<Rocket className="w-5 h-5 text-white" />} label="总部署数" value={stats.total} color="bg-blue-500" />
        <StatCard icon={<CheckCircle2 className="w-5 h-5 text-white" />} label="成功" value={stats.success} color="bg-green-500" />
        <StatCard icon={<XCircle className="w-5 h-5 text-white" />} label="失败" value={stats.failed} color="bg-red-500" />
        <StatCard icon={<Loader2 className="w-5 h-5 text-white" />} label="进行中" value={stats.inProgress} color="bg-yellow-500" />
        <StatCard icon={<TrendingUp className="w-5 h-5 text-white" />} label="成功率" value={`${stats.successRate}%`} color="bg-purple-500" />
      </div>

      {/* Recent Deployments Timeline */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">最近部署批次</CardTitle>
        </CardHeader>
        <CardContent>
          {loading ? (
            <div className="space-y-2">
              {Array.from({ length: 5 }).map((_, i) => (
                <Skeleton key={i} className="h-12 w-full" />
              ))}
            </div>
          ) : recentBatches.length === 0 ? (
            <div className="text-center py-8 text-muted-foreground">暂无部署记录</div>
          ) : (
            <div className="space-y-2">
              {recentBatches.map((batch) => (
                <div key={batch.id} className="flex items-center justify-between p-3 rounded-lg border border-border hover:bg-muted/50 transition-colors">
                  <div className="flex items-center gap-3">
                    <Server className="w-4 h-4 text-muted-foreground" />
                    <div>
                      <div className="text-sm font-medium">
                        {batch.scope_type} · {batch.scope_id?.slice(0, 8) || '-'}
                      </div>
                      <div className="text-xs text-muted-foreground">
                        {batch.strategy} · {new Date(batch.created_at).toLocaleString('zh-CN')}
                      </div>
                    </div>
                  </div>
                  <StatusBadge status={batch.status} />
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
