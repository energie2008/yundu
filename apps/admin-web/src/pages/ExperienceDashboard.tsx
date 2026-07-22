import { useState, useEffect, useCallback } from 'react'
import {
  BarChart3,
  Trophy,
  TrendingUp,
  Gauge,
  Activity,
  RefreshCw,
  Settings,
  ShieldOff,
  CheckCircle,
  AlertTriangle,
  XCircle,
} from 'lucide-react'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
  Button,
  Badge,
  Skeleton,
  Select,
  Table,
  TableHeader,
  TableBody,
  TableHead,
  TableRow,
  TableCell,
  EmptyState,
  useToast,
} from '@airport/ui'
import { api, ApiError } from '@/lib/api'
import { EP } from '@/lib/endpoints'

// ===== 类型定义 =====
type Grade = 'excellent' | 'good' | 'fair' | 'poor' | 'critical' | 'unknown'

interface ExperienceScore {
  node_id: string
  overall_score: number
  latency_score: number
  stability_score: number
  speed_score: number
  success_rate_score: number
  p50_latency_ms?: number | null
  p95_latency_ms?: number | null
  p99_latency_ms?: number | null
  heartbeat_success_rate?: number | null
  channel_failover_count_24h?: number | null
  measured_bandwidth_mbps?: number | null
  connection_success_rate?: number | null
  grade: Grade
  isolated: boolean
  calculated_at: string
}

interface ExperienceConfig {
  weight_latency: number
  weight_stability: number
  weight_speed: number
  weight_success_rate: number
  excellent_threshold: number
  good_threshold: number
  fair_threshold: number
  poor_threshold: number
  isolate_threshold: number
  calc_interval_seconds: number
  probe_interval_seconds: number
  auto_isolate_enabled: boolean
}

interface ScoreHistoryItem {
  id: number
  node_id: string
  overall_score: number
  latency_score: number
  stability_score: number
  speed_score: number
  success_rate_score: number
  p95_latency_ms?: number | null
  measured_bandwidth_mbps?: number | null
  grade: Grade
  isolated: boolean
  calculated_at: string
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

const gradeConfig: Record<Grade, { label: string; color: string; bgClass: string; Icon: typeof Trophy }> = {
  excellent: { label: '优秀', color: 'text-emerald-400', bgClass: 'bg-emerald-900/50 text-emerald-300 border-emerald-800/50', Icon: Trophy },
  good: { label: '良好', color: 'text-sky-400', bgClass: 'bg-sky-900/50 text-sky-300 border-sky-800/50', Icon: CheckCircle },
  fair: { label: '一般', color: 'text-amber-400', bgClass: 'bg-amber-900/50 text-amber-300 border-amber-800/50', Icon: Activity },
  poor: { label: '较差', color: 'text-orange-400', bgClass: 'bg-orange-900/50 text-orange-300 border-orange-800/50', Icon: AlertTriangle },
  critical: { label: '极差', color: 'text-red-400', bgClass: 'bg-red-900/50 text-red-300 border-red-800/50', Icon: XCircle },
  unknown: { label: '未知', color: 'text-zinc-400', bgClass: 'bg-zinc-800 text-zinc-400 border-zinc-700', Icon: Activity },
}

function getGradeBadge(grade: Grade) {
  const cfg = gradeConfig[grade] || gradeConfig.unknown
  const Icon = cfg.Icon
  return (
    <Badge variant="secondary" className={cfg.bgClass}>
      <Icon className="w-3 h-3 mr-1" />
      {cfg.label}
    </Badge>
  )
}

function getScoreColor(score: number): string {
  if (score >= 85) return 'text-emerald-400'
  if (score >= 70) return 'text-sky-400'
  if (score >= 60) return 'text-amber-400'
  if (score >= 40) return 'text-orange-400'
  return 'text-red-400'
}

function getScoreBar(score: number): string {
  if (score >= 85) return 'bg-emerald-500'
  if (score >= 70) return 'bg-sky-500'
  if (score >= 60) return 'bg-amber-500'
  if (score >= 40) return 'bg-orange-500'
  return 'bg-red-500'
}

function formatTime(dateStr?: string) {
  if (!dateStr) return '-'
  const d = new Date(dateStr)
  if (isNaN(d.getTime())) return '-'
  return d.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

// 简易雷达图（SVG 实现，避免引入额外依赖）
function RadarChart({ scores }: { scores: { label: string; value: number }[] }) {
  const size = 220
  const center = size / 2
  const radius = 80
  const levels = 4
  const angleStep = (Math.PI * 2) / scores.length

  const pointAt = (angle: number, r: number) => ({
    x: center + r * Math.cos(angle - Math.PI / 2),
    y: center + r * Math.sin(angle - Math.PI / 2),
  })

  const dataPoints = scores.map((s, i) => {
    const angle = i * angleStep
    const r = (s.value / 100) * radius
    return pointAt(angle, r)
  })

  const polygonPoints = dataPoints.map((p) => `${p.x},${p.y}`).join(' ')

  return (
    <svg width={size} height={size} className="mx-auto">
      {/* 网格 */}
      {Array.from({ length: levels }).map((_, levelIdx) => {
        const r = (radius * (levelIdx + 1)) / levels
        const gridPoints = scores.map((_, i) => {
          const angle = i * angleStep
          const p = pointAt(angle, r)
          return `${p.x},${p.y}`
        }).join(' ')
        return (
          <polygon
            key={levelIdx}
            points={gridPoints}
            fill="none"
            stroke="#27272a"
            strokeWidth="1"
          />
        )
      })}
      {/* 轴线 */}
      {scores.map((_, i) => {
        const angle = i * angleStep
        const p = pointAt(angle, radius)
        return (
          <line
            key={i}
            x1={center}
            y1={center}
            x2={p.x}
            y2={p.y}
            stroke="#27272a"
            strokeWidth="1"
          />
        )
      })}
      {/* 数据多边形 */}
      <polygon
        points={polygonPoints}
        fill="rgba(139, 92, 246, 0.25)"
        stroke="#8b5cf6"
        strokeWidth="2"
      />
      {/* 数据点 */}
      {dataPoints.map((p, i) => (
        <circle key={i} cx={p.x} cy={p.y} r="3" fill="#8b5cf6" />
      ))}
      {/* 标签 */}
      {scores.map((s, i) => {
        const angle = i * angleStep
        const labelPos = pointAt(angle, radius + 18)
        const anchor = Math.abs(Math.cos(angle - Math.PI / 2)) < 0.3
          ? 'middle'
          : Math.sin(angle - Math.PI / 2) > 0.3 ? 'middle' : (labelPos.x < center ? 'end' : 'start')
        return (
          <text
            key={i}
            x={labelPos.x}
            y={labelPos.y}
            fill="#a1a1aa"
            fontSize="11"
            textAnchor={anchor as 'start' | 'middle' | 'end'}
            dominantBaseline="middle"
          >
            {s.label} {s.value.toFixed(0)}
          </text>
        )
      })}
    </svg>
  )
}

// 简易趋势图（SVG 实现）
function TrendChart({ data }: { data: ScoreHistoryItem[] }) {
  const width = 600
  const height = 180
  const padding = { top: 20, right: 20, bottom: 30, left: 40 }
  const chartW = width - padding.left - padding.right
  const chartH = height - padding.top - padding.bottom

  if (data.length === 0) {
    return <div className="text-center text-sm text-zinc-500 py-8">暂无历史数据</div>
  }

  const sorted = [...data].sort((a, b) => new Date(a.calculated_at).getTime() - new Date(b.calculated_at).getTime())
  const xs = sorted.map((_, i) => padding.left + (chartW * i) / Math.max(1, sorted.length - 1))
  const yScale = (v: number) => padding.top + chartH - (v / 100) * chartH

  const overallPath = sorted.map((d, i) => `${i === 0 ? 'M' : 'L'} ${xs[i]} ${yScale(d.overall_score)}`).join(' ')
  const latencyPath = sorted.map((d, i) => `${i === 0 ? 'M' : 'L'} ${xs[i]} ${yScale(d.latency_score)}`).join(' ')
  const stabilityPath = sorted.map((d, i) => `${i === 0 ? 'M' : 'L'} ${xs[i]} ${yScale(d.stability_score)}`).join(' ')

  return (
    <svg width="100%" viewBox={`0 0 ${width} ${height}`} className="overflow-visible">
      {/* Y 轴刻度 */}
      {[0, 25, 50, 75, 100].map((v) => (
        <g key={v}>
          <line
            x1={padding.left}
            y1={yScale(v)}
            x2={width - padding.right}
            y2={yScale(v)}
            stroke="#27272a"
            strokeWidth="1"
            strokeDasharray={v === 0 ? '0' : '2,2'}
          />
          <text x={padding.left - 6} y={yScale(v)} fill="#71717a" fontSize="10" textAnchor="end" dominantBaseline="middle">
            {v}
          </text>
        </g>
      ))}
      {/* X 轴标签（首尾） */}
      <text x={padding.left} y={height - 8} fill="#71717a" fontSize="10" textAnchor="start">
        {formatTime(sorted[0]?.calculated_at)}
      </text>
      <text x={width - padding.right} y={height - 8} fill="#71717a" fontSize="10" textAnchor="end">
        {formatTime(sorted[sorted.length - 1]?.calculated_at)}
      </text>
      {/* 数据线 */}
      <path d={overallPath} fill="none" stroke="#8b5cf6" strokeWidth="2" />
      <path d={latencyPath} fill="none" stroke="#10b981" strokeWidth="1.5" strokeDasharray="3,2" />
      <path d={stabilityPath} fill="none" stroke="#f59e0b" strokeWidth="1.5" strokeDasharray="3,2" />
      {/* 图例 */}
      <g transform={`translate(${padding.left}, 4)`}>
        <line x1="0" y1="8" x2="14" y2="8" stroke="#8b5cf6" strokeWidth="2" />
        <text x="18" y="11" fill="#a1a1aa" fontSize="10">总分</text>
        <line x1="60" y1="8" x2="74" y2="8" stroke="#10b981" strokeWidth="1.5" strokeDasharray="3,2" />
        <text x="78" y="11" fill="#a1a1aa" fontSize="10">延迟分</text>
        <line x1="130" y1="8" x2="144" y2="8" stroke="#f59e0b" strokeWidth="1.5" strokeDasharray="3,2" />
        <text x="148" y="11" fill="#a1a1aa" fontSize="10">稳定分</text>
      </g>
    </svg>
  )
}

// ===== 主组件 =====
export default function ExperienceDashboard() {
  const { toast } = useToast()

  const [scores, setScores] = useState<ExperienceScore[]>([])
  const [scoresLoading, setScoresLoading] = useState(true)

  const [selectedNodeId, setSelectedNodeId] = useState('')
  const [history, setHistory] = useState<ScoreHistoryItem[]>([])
  const [historyLoading, setHistoryLoading] = useState(false)

  const [config, setConfig] = useState<ExperienceConfig | null>(null)
  const [configLoading, setConfigLoading] = useState(true)
  const [configEditing, setConfigEditing] = useState(false)
  const [configSaving, setConfigSaving] = useState(false)
  const [configDraft, setConfigDraft] = useState<Partial<ExperienceConfig>>({})

  const [recalculating, setRecalculating] = useState(false)
  const [filterGrade, setFilterGrade] = useState('')
  const [filterOnlyIsolated, setFilterOnlyIsolated] = useState(false)

  const fetchScores = useCallback(async () => {
    setScoresLoading(true)
    try {
      const params: Record<string, string | number | boolean | undefined> = {
        page: 1,
        page_size: 100,
      }
      if (filterGrade) params.grade = filterGrade
      if (filterOnlyIsolated) params.only_isolated = 'true'
      const data = await api.get<unknown>(EP.EXPERIENCE_SCORES, { params })
      const list = normalizeList<ExperienceScore>(data)
      setScores(list)
      if (list.length > 0 && !selectedNodeId) {
        setSelectedNodeId(list[0].node_id)
      }
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '加载体验分数据失败'
      toast({ title: '加载失败', description: msg, variant: 'destructive' })
      setScores([])
    } finally {
      setScoresLoading(false)
    }
  }, [toast, filterGrade, filterOnlyIsolated, selectedNodeId])

  const fetchHistory = useCallback(async (nodeId: string) => {
    if (!nodeId) return
    setHistoryLoading(true)
    try {
      const data = await api.get<unknown>(EP.EXPERIENCE_SCORE_HISTORY(nodeId), {
        params: { limit: 50 },
      })
      setHistory(normalizeList<ScoreHistoryItem>(data))
    } catch (err) {
      setHistory([])
    } finally {
      setHistoryLoading(false)
    }
  }, [])

  const fetchConfig = useCallback(async () => {
    setConfigLoading(true)
    try {
      const data = await api.get<ExperienceConfig>(EP.EXPERIENCE_CONFIG)
      setConfig(data)
      setConfigDraft(data)
    } catch (err) {
      // 配置未初始化时使用默认
      setConfig(null)
    } finally {
      setConfigLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchScores()
    fetchConfig()
  }, [fetchScores, fetchConfig])

  useEffect(() => {
    if (selectedNodeId) {
      fetchHistory(selectedNodeId)
    } else {
      setHistory([])
    }
  }, [selectedNodeId, fetchHistory])

  const handleRecalculate = async () => {
    setRecalculating(true)
    try {
      await api.post(EP.EXPERIENCE_RECALCULATE)
      toast({ title: '已触发重新计算', description: '后台正在异步计算所有节点体验分' })
      setTimeout(() => fetchScores(), 3000)
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '触发重新计算失败'
      toast({ title: '操作失败', description: msg, variant: 'destructive' })
    } finally {
      setRecalculating(false)
    }
  }

  const handleSaveConfig = async () => {
    setConfigSaving(true)
    try {
      const updated = await api.put<ExperienceConfig>(EP.EXPERIENCE_CONFIG, configDraft)
      setConfig(updated)
      setConfigDraft(updated)
      setConfigEditing(false)
      toast({ title: '配置已保存' })
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '保存配置失败'
      toast({ title: '保存失败', description: msg, variant: 'destructive' })
    } finally {
      setConfigSaving(false)
    }
  }

  // 聚合统计
  const totalNodes = scores.length
  const excellentCount = scores.filter((s) => s.grade === 'excellent').length
  const isolatedCount = scores.filter((s) => s.isolated).length
  const avgScore = totalNodes > 0 ? scores.reduce((sum, s) => sum + s.overall_score, 0) / totalNodes : 0

  // 排行榜（按总分降序）
  const rankedScores = [...scores].sort((a, b) => b.overall_score - a.overall_score)
  const selectedScore = scores.find((s) => s.node_id === selectedNodeId)

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold text-zinc-100 flex items-center gap-2">
            <BarChart3 className="w-5 h-5 text-violet-400" />
            节点体验
          </h2>
          <p className="text-sm text-zinc-500 mt-0.5">
            综合延迟、稳定性、速度、成功率多维度评估节点用户体验，自动隔离低分节点
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="secondary"
            size="sm"
            onClick={handleRecalculate}
            disabled={recalculating}
            className="bg-zinc-800 hover:bg-zinc-700 border-zinc-700"
          >
            <RefreshCw className={`w-3.5 h-3.5 mr-1 ${recalculating ? 'animate-spin' : ''}`} />
            重新计算
          </Button>
          <Button
            variant="secondary"
            size="sm"
            onClick={() => {
              setConfigEditing(true)
              setConfigDraft(config || {})
            }}
            className="bg-zinc-800 hover:bg-zinc-700 border-zinc-700"
          >
            <Settings className="w-3.5 h-3.5 mr-1" />
            评分配置
          </Button>
        </div>
      </div>

      {/* 顶部统计卡 */}
      <Card className="bg-zinc-900 border-zinc-800">
        <CardContent className="p-4">
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
            <div className="space-y-1">
              <div className="text-xs text-zinc-500">监控节点数</div>
              <div className="text-xl font-semibold text-zinc-100 mt-1">{totalNodes}</div>
            </div>
            <div className="space-y-1">
              <div className="text-xs text-zinc-500">平均体验分</div>
              <div className={`text-xl font-semibold mt-1 ${getScoreColor(avgScore)}`}>
                {avgScore.toFixed(1)}
              </div>
            </div>
            <div className="space-y-1">
              <div className="text-xs text-zinc-500">优秀节点</div>
              <div className="text-xl font-semibold text-emerald-400 mt-1">{excellentCount}</div>
            </div>
            <div className="space-y-1">
              <div className="text-xs text-zinc-500">已隔离</div>
              <div className="text-xl font-semibold text-red-400 mt-1">{isolatedCount}</div>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* 排行榜 */}
      <Card className="bg-zinc-900 border-zinc-800">
        <CardHeader className="pb-3">
          <CardTitle className="text-base flex items-center gap-2">
            <Trophy className="w-4 h-4 text-amber-400" />
            节点体验排行榜
          </CardTitle>
          <div className="flex items-center gap-2 mt-2">
            <Select
              value={filterGrade}
              onChange={(e) => setFilterGrade(e.target.value)}
              className="max-w-[140px]"
            >
              <option value="">全部等级</option>
              <option value="excellent">优秀</option>
              <option value="good">良好</option>
              <option value="fair">一般</option>
              <option value="poor">较差</option>
              <option value="critical">极差</option>
              <option value="unknown">未知</option>
            </Select>
            <label className="flex items-center gap-1.5 text-xs text-zinc-400 cursor-pointer">
              <input
                type="checkbox"
                checked={filterOnlyIsolated}
                onChange={(e) => setFilterOnlyIsolated(e.target.checked)}
                className="rounded border-zinc-700 bg-zinc-900"
              />
              仅显示已隔离
            </label>
          </div>
        </CardHeader>
        <CardContent className="p-0">
          {scoresLoading ? (
            <div className="p-4 space-y-2">
              {[1, 2, 3, 4, 5].map((i) => (
                <Skeleton key={i} className="h-10 w-full" />
              ))}
            </div>
          ) : rankedScores.length === 0 ? (
            <EmptyState
              icon={<BarChart3 className="w-8 h-8 text-zinc-600" />}
              title="暂无体验分数据"
              description="点击右上角「重新计算」触发评分计算"
            />
          ) : (
            <Table>
              <TableHeader>
                <TableRow className="border-zinc-800 hover:bg-transparent">
                  <TableHead className="w-12">排名</TableHead>
                  <TableHead>节点ID</TableHead>
                  <TableHead>总分</TableHead>
                  <TableHead className="hidden sm:table-cell">延迟分</TableHead>
                  <TableHead className="hidden sm:table-cell">稳定分</TableHead>
                  <TableHead className="hidden md:table-cell">速度分</TableHead>
                  <TableHead className="hidden md:table-cell">成功率分</TableHead>
                  <TableHead>等级</TableHead>
                  <TableHead className="hidden lg:table-cell">P95延迟</TableHead>
                  <TableHead className="hidden lg:table-cell">带宽</TableHead>
                  <TableHead>状态</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rankedScores.map((score, idx) => (
                  <TableRow
                    key={score.node_id}
                    className={`border-zinc-800 cursor-pointer hover:bg-zinc-800/30 ${
                      selectedNodeId === score.node_id ? 'bg-violet-900/10' : ''
                    }`}
                    onClick={() => setSelectedNodeId(score.node_id)}
                  >
                    <TableCell>
                      <div className="flex items-center gap-1">
                        {idx === 0 && <Trophy className="w-3.5 h-3.5 text-amber-400" />}
                        {idx === 1 && <Trophy className="w-3.5 h-3.5 text-zinc-300" />}
                        {idx === 2 && <Trophy className="w-3.5 h-3.5 text-amber-700" />}
                        <span className="text-zinc-500 text-xs font-mono">{idx + 1}</span>
                      </div>
                    </TableCell>
                    <TableCell>
                      <span className="text-xs font-mono text-zinc-300">{score.node_id.slice(0, 8)}</span>
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <span className={`text-sm font-semibold ${getScoreColor(score.overall_score)}`}>
                          {score.overall_score.toFixed(1)}
                        </span>
                        <div className="w-12 h-1.5 bg-zinc-800 rounded-full overflow-hidden hidden lg:block">
                          <div
                            className={`h-full rounded-full ${getScoreBar(score.overall_score)}`}
                            style={{ width: `${score.overall_score}%` }}
                          />
                        </div>
                      </div>
                    </TableCell>
                    <TableCell className="hidden sm:table-cell text-xs text-zinc-300">{score.latency_score.toFixed(0)}</TableCell>
                    <TableCell className="hidden sm:table-cell text-xs text-zinc-300">{score.stability_score.toFixed(0)}</TableCell>
                    <TableCell className="hidden md:table-cell text-xs text-zinc-300">{score.speed_score.toFixed(0)}</TableCell>
                    <TableCell className="hidden md:table-cell text-xs text-zinc-300">{score.success_rate_score.toFixed(0)}</TableCell>
                    <TableCell>{getGradeBadge(score.grade)}</TableCell>
                    <TableCell className="hidden lg:table-cell text-xs text-zinc-400">
                      {score.p95_latency_ms != null ? `${score.p95_latency_ms.toFixed(0)}ms` : '-'}
                    </TableCell>
                    <TableCell className="hidden lg:table-cell text-xs text-zinc-400">
                      {score.measured_bandwidth_mbps != null ? `${score.measured_bandwidth_mbps.toFixed(0)}Mbps` : '-'}
                    </TableCell>
                    <TableCell>
                      {score.isolated ? (
                        <Badge variant="destructive" className="bg-red-900/50 text-red-300 border-red-800/50">
                          <ShieldOff className="w-3 h-3 mr-1" />
                          已隔离
                        </Badge>
                      ) : (
                        <Badge variant="success" className="bg-emerald-900/50 text-emerald-300 border-emerald-800/50">
                          <CheckCircle className="w-3 h-3 mr-1" />
                          在线
                        </Badge>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      {/* 单节点详情：雷达图 + 趋势图 */}
      {selectedScore && (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
          <Card className="bg-zinc-900 border-zinc-800">
            <CardHeader className="pb-3">
              <CardTitle className="text-base flex items-center gap-2">
                <Gauge className="w-4 h-4 text-violet-400" />
                节点雷达图
              </CardTitle>
              <CardDescription className="text-zinc-500">
                节点 {selectedScore.node_id.slice(0, 8)} · 更新于 {formatTime(selectedScore.calculated_at)}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <RadarChart
                scores={[
                  { label: '延迟', value: selectedScore.latency_score },
                  { label: '稳定', value: selectedScore.stability_score },
                  { label: '速度', value: selectedScore.speed_score },
                  { label: '成功率', value: selectedScore.success_rate_score },
                  { label: '总分', value: selectedScore.overall_score },
                ]}
              />
              <div className="mt-4 grid grid-cols-2 gap-3 text-xs">
                <div className="flex items-center justify-between bg-zinc-950/30 border border-zinc-800 rounded px-2 py-1.5">
                  <span className="text-zinc-500">P50 延迟</span>
                  <span className="text-zinc-200 font-mono">
                    {selectedScore.p50_latency_ms != null ? `${selectedScore.p50_latency_ms.toFixed(0)}ms` : '-'}
                  </span>
                </div>
                <div className="flex items-center justify-between bg-zinc-950/30 border border-zinc-800 rounded px-2 py-1.5">
                  <span className="text-zinc-500">P95 延迟</span>
                  <span className="text-zinc-200 font-mono">
                    {selectedScore.p95_latency_ms != null ? `${selectedScore.p95_latency_ms.toFixed(0)}ms` : '-'}
                  </span>
                </div>
                <div className="flex items-center justify-between bg-zinc-950/30 border border-zinc-800 rounded px-2 py-1.5">
                  <span className="text-zinc-500">心跳成功率</span>
                  <span className="text-zinc-200 font-mono">
                    {selectedScore.heartbeat_success_rate != null
                      ? `${(selectedScore.heartbeat_success_rate * 100).toFixed(1)}%`
                      : '-'}
                  </span>
                </div>
                <div className="flex items-center justify-between bg-zinc-950/30 border border-zinc-800 rounded px-2 py-1.5">
                  <span className="text-zinc-500">连接成功率</span>
                  <span className="text-zinc-200 font-mono">
                    {selectedScore.connection_success_rate != null
                      ? `${(selectedScore.connection_success_rate * 100).toFixed(1)}%`
                      : '-'}
                  </span>
                </div>
                <div className="flex items-center justify-between bg-zinc-950/30 border border-zinc-800 rounded px-2 py-1.5">
                  <span className="text-zinc-500">实测带宽</span>
                  <span className="text-zinc-200 font-mono">
                    {selectedScore.measured_bandwidth_mbps != null
                      ? `${selectedScore.measured_bandwidth_mbps.toFixed(1)}Mbps`
                      : '-'}
                  </span>
                </div>
                <div className="flex items-center justify-between bg-zinc-950/30 border border-zinc-800 rounded px-2 py-1.5">
                  <span className="text-zinc-500">24h 降级</span>
                  <span className="text-zinc-200 font-mono">
                    {selectedScore.channel_failover_count_24h ?? 0} 次
                  </span>
                </div>
              </div>
            </CardContent>
          </Card>

          <Card className="bg-zinc-900 border-zinc-800">
            <CardHeader className="pb-3">
              <CardTitle className="text-base flex items-center gap-2">
                <TrendingUp className="w-4 h-4 text-emerald-400" />
                评分趋势
              </CardTitle>
              <CardDescription className="text-zinc-500">
                最近 {history.length} 次评分记录
              </CardDescription>
            </CardHeader>
            <CardContent>
              {historyLoading ? (
                <Skeleton className="h-44 w-full" />
              ) : (
                <TrendChart data={history} />
              )}
            </CardContent>
          </Card>
        </div>
      )}

      {/* 配置编辑弹窗 */}
      {configEditing && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
          <div className="bg-zinc-900 border border-zinc-800 rounded-lg p-6 w-[520px] max-w-[90vw] max-h-[85vh] overflow-y-auto space-y-4">
            <div className="flex items-center justify-between sticky top-0 bg-zinc-900 pb-2 -mt-2 pt-2">
              <h3 className="text-base font-semibold text-zinc-100 flex items-center gap-2">
                <Settings className="w-4 h-4 text-zinc-400" />
                评分配置
              </h3>
              <button
                onClick={() => setConfigEditing(false)}
                className="text-zinc-500 hover:text-zinc-300"
              >
                ✕
              </button>
            </div>

            {configLoading ? (
              <Skeleton className="h-40 w-full" />
            ) : (
              <div className="space-y-4">
                <div>
                  <h4 className="text-xs text-zinc-500 mb-2 uppercase tracking-wide">维度权重</h4>
                  <div className="grid grid-cols-2 gap-3">
                    <ConfigInput
                      label="延迟权重"
                      value={configDraft.weight_latency}
                      step={0.05}
                      onChange={(v) => setConfigDraft({ ...configDraft, weight_latency: v })}
                    />
                    <ConfigInput
                      label="稳定权重"
                      value={configDraft.weight_stability}
                      step={0.05}
                      onChange={(v) => setConfigDraft({ ...configDraft, weight_stability: v })}
                    />
                    <ConfigInput
                      label="速度权重"
                      value={configDraft.weight_speed}
                      step={0.05}
                      onChange={(v) => setConfigDraft({ ...configDraft, weight_speed: v })}
                    />
                    <ConfigInput
                      label="成功率权重"
                      value={configDraft.weight_success_rate}
                      step={0.05}
                      onChange={(v) => setConfigDraft({ ...configDraft, weight_success_rate: v })}
                    />
                  </div>
                  <p className="text-xs text-zinc-600 mt-1">建议四项权重之和为 1.0</p>
                </div>

                <div>
                  <h4 className="text-xs text-zinc-500 mb-2 uppercase tracking-wide">等级阈值</h4>
                  <div className="grid grid-cols-2 gap-3">
                    <ConfigInput
                      label="优秀阈值"
                      value={configDraft.excellent_threshold}
                      onChange={(v) => setConfigDraft({ ...configDraft, excellent_threshold: v })}
                    />
                    <ConfigInput
                      label="良好阈值"
                      value={configDraft.good_threshold}
                      onChange={(v) => setConfigDraft({ ...configDraft, good_threshold: v })}
                    />
                    <ConfigInput
                      label="一般阈值"
                      value={configDraft.fair_threshold}
                      onChange={(v) => setConfigDraft({ ...configDraft, fair_threshold: v })}
                    />
                    <ConfigInput
                      label="较差阈值"
                      value={configDraft.poor_threshold}
                      onChange={(v) => setConfigDraft({ ...configDraft, poor_threshold: v })}
                    />
                    <ConfigInput
                      label="隔离阈值"
                      value={configDraft.isolate_threshold}
                      onChange={(v) => setConfigDraft({ ...configDraft, isolate_threshold: v })}
                    />
                  </div>
                </div>

                <div>
                  <h4 className="text-xs text-zinc-500 mb-2 uppercase tracking-wide">计算参数</h4>
                  <div className="grid grid-cols-2 gap-3">
                    <ConfigInput
                      label="计算间隔（秒）"
                      value={configDraft.calc_interval_seconds}
                      onChange={(v) => setConfigDraft({ ...configDraft, calc_interval_seconds: v })}
                    />
                    <ConfigInput
                      label="探针间隔（秒）"
                      value={configDraft.probe_interval_seconds}
                      onChange={(v) => setConfigDraft({ ...configDraft, probe_interval_seconds: v })}
                    />
                  </div>
                  <label className="flex items-center gap-2 mt-3 text-sm text-zinc-300 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={configDraft.auto_isolate_enabled ?? false}
                      onChange={(e) => setConfigDraft({ ...configDraft, auto_isolate_enabled: e.target.checked })}
                      className="rounded border-zinc-700 bg-zinc-900"
                    />
                    启用自动隔离（达到隔离阈值时自动下线节点）
                  </label>
                </div>

                <div className="flex justify-end gap-2 pt-2 border-t border-zinc-800">
                  <Button
                    variant="secondary"
                    onClick={() => setConfigEditing(false)}
                    className="bg-zinc-800 hover:bg-zinc-700 border-zinc-700"
                  >
                    取消
                  </Button>
                  <Button
                    onClick={handleSaveConfig}
                    disabled={configSaving}
                    className="bg-violet-600 hover:bg-violet-500 text-white"
                  >
                    {configSaving ? '保存中...' : '保存配置'}
                  </Button>
                </div>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

function ConfigInput({
  label,
  value,
  onChange,
  step = 1,
}: {
  label: string
  value?: number
  onChange: (v: number) => void
  step?: number
}) {
  return (
    <div>
      <label className="text-xs text-zinc-500 mb-1 block">{label}</label>
      <input
        type="number"
        value={value ?? 0}
        step={step}
        onChange={(e) => onChange(Number(e.target.value))}
        className="w-full px-2.5 py-1.5 bg-zinc-950 border border-zinc-800 rounded-md text-sm text-zinc-200 focus:outline-none focus:border-zinc-600"
      />
    </div>
  )
}
