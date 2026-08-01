import { useCallback, useEffect, useState } from 'react'
import { RefreshCw, AlertTriangle, CheckCircle2 } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle, Badge, Button, Skeleton } from '@airport/ui'
import { api } from '@/lib/api'
import { EP } from '@/lib/endpoints'
import { ADMIN_CARD, ADMIN_BORDER, ADMIN_TEXT, ADMIN_TEXT_SECONDARY, ADMIN_SUCCESS, ADMIN_WARNING, ADMIN_DANGER } from '@/lib/theme'

interface HealthCheckSummary {
  total_nodes: number
  pushed: number
  applied: number
  failed: number
  failed_nodes: string[]
  null_provider_ref: number
  cv_10min: number
  duplicate_runtimes: number
}

function MetricCard({ label, value, tone }: { label: string; value: number | string; tone: 'ok' | 'warn' | 'danger' | 'muted' }) {
  const toneClass =
    tone === 'ok' ? ADMIN_SUCCESS :
    tone === 'warn' ? ADMIN_WARNING :
    tone === 'danger' ? ADMIN_DANGER : ADMIN_TEXT_SECONDARY
  return (
    <Card className={ADMIN_CARD}>
      <CardContent className="p-4">
        <div className={`text-xs ${ADMIN_TEXT_SECONDARY}`}>{label}</div>
        <div className={`text-2xl font-semibold mt-1 ${toneClass}`}>{value}</div>
      </CardContent>
    </Card>
  )
}

export default function HealthChecks() {
  const [data, setData] = useState<HealthCheckSummary | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const res = await api.get<HealthCheckSummary>(EP.HEALTH_CHECKS_SUMMARY)
      setData(res ?? null)
    } catch (e) {
      setError(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  const healthy = data && data.failed === 0 && data.duplicate_runtimes === 0 && data.null_provider_ref === 0

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold">健康体检</h2>
          <p className={`text-sm ${ADMIN_TEXT_SECONDARY}`}>对应维护指南 SQL 巡检 A/B/C，替代 SSH + psql</p>
        </div>
        <Button onClick={load} disabled={loading}>
          <RefreshCw className="w-4 h-4 mr-1" />
          刷新
        </Button>
      </div>

      {healthy && (
        <div className="flex items-center gap-2 text-sm">
          <CheckCircle2 className={`w-4 h-4 ${ADMIN_SUCCESS}`} />
          <span className={ADMIN_SUCCESS}>全部通过</span>
        </div>
      )}
      {!healthy && data && (
        <div className="flex items-center gap-2 text-sm">
          <AlertTriangle className={`w-4 h-4 ${ADMIN_WARNING}`} />
          <span className={ADMIN_WARNING}>存在异常项，请检查下方指标</span>
        </div>
      )}
      {error && <div className={`text-sm ${ADMIN_DANGER}`}>{error}</div>}

      {loading && !data ? (
        <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
          {[0, 1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-24" />
          ))}
        </div>
      ) : data ? (
        <>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
            <MetricCard label="节点总数" value={data.total_nodes} tone="muted" />
            <MetricCard label="pushed" value={data.pushed} tone="ok" />
            <MetricCard label="applied" value={data.applied} tone="ok" />
            <MetricCard label="failed" value={data.failed} tone={data.failed > 0 ? 'danger' : 'ok'} />
            <MetricCard label="NULL provider_ref" value={data.null_provider_ref} tone={data.null_provider_ref > 0 ? 'danger' : 'ok'} />
            <MetricCard label="10 分钟新增版本" value={data.cv_10min} tone={data.cv_10min > 5 ? 'warn' : 'ok'} />
            <MetricCard label="重复 runtime" value={data.duplicate_runtimes} tone={data.duplicate_runtimes > 0 ? 'danger' : 'ok'} />
          </div>

          <Card className={ADMIN_CARD}>
            <CardHeader>
              <CardTitle>失败节点</CardTitle>
            </CardHeader>
            <CardContent>
              {data.failed_nodes.length === 0 ? (
                <div className={`text-sm ${ADMIN_TEXT_SECONDARY}`}>无失败节点</div>
              ) : (
                <div className="flex flex-wrap gap-2">
                  {data.failed_nodes.map((code) => (
                    <Badge key={code} variant="destructive">{code}</Badge>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </>
      ) : null}
    </div>
  )
}
