import { useState, useEffect } from 'react'
import {
  RefreshCw,
  Stethoscope,
  AlertTriangle,
  XCircle,
  CheckCircle2,
  HelpCircle,
  ChevronDown,
  ChevronRight,
  Wrench,
  ShieldAlert,
} from 'lucide-react'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Button,
  Badge,
  Skeleton,
  EmptyState,
  Select,
  useToast,
} from '@airport/ui'
import { api, ApiError } from '@/lib/api'
import { EP } from '@/lib/endpoints'

// ===== 类型定义 =====
type CheckResultStatus = 'pass' | 'fail' | 'warn' | 'unknown'

interface DoctorCheckDef {
  id: string
  code: string
  name: string
  description?: string
  category?: string
}

interface DoctorCheckResult {
  def_id?: string
  def_code?: string
  name: string
  description?: string
  status: CheckResultStatus
  message?: string
  suggestion?: string
}

interface DoctorReport {
  id: string
  node_id: string
  node_name?: string
  status?: 'healthy' | 'degraded' | 'offline' | 'unknown'
  summary?: string
  checks?: DoctorCheckResult[]
  pass_count?: number
  fail_count?: number
  warn_count?: number
  created_at?: string
}

interface NodeOption {
  id: string
  name: string
  status?: string
}

// ===== 工具函数 =====
function formatTime(dateStr?: string) {
  if (!dateStr) return '-'
  return new Date(dateStr).toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function normalizeList<T>(data: unknown): T[] {
  if (Array.isArray(data)) return data as T[]
  if (data && typeof data === 'object') {
    const obj = data as Record<string, unknown>
    if (Array.isArray(obj.items)) return obj.items as T[]
    if (Array.isArray(obj.list)) return obj.list as T[]
    // 处理嵌套结构：{code:0, data:{items:[...]}}
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

const statusConfig: Record<
  CheckResultStatus,
  { label: string; Icon: typeof CheckCircle2; color: string; badgeClass: string }
> = {
  pass: { label: '通过', Icon: CheckCircle2, color: 'text-emerald-400', badgeClass: 'bg-emerald-900/50 text-emerald-300 border-emerald-800/50' },
  fail: { label: '失败', Icon: XCircle, color: 'text-red-400', badgeClass: 'bg-red-900/50 text-red-300 border-red-800/50' },
  warn: { label: '警告', Icon: AlertTriangle, color: 'text-amber-400', badgeClass: 'bg-amber-900/50 text-amber-300 border-amber-800/50' },
  unknown: { label: '未知', Icon: HelpCircle, color: 'text-zinc-400', badgeClass: 'bg-zinc-800 text-zinc-400 border-zinc-700' },
}

function StatusIcon({ status }: { status: CheckResultStatus }) {
  const cfg = statusConfig[status] || statusConfig.unknown
  const Icon = cfg.Icon
  return <Icon className={`w-4 h-4 ${cfg.color}`} />
}

function getReportStatusBadge(status?: string) {
  const map: Record<string, { label: string; variant: 'success' | 'warning' | 'destructive' | 'secondary' }> = {
    healthy: { label: '健康', variant: 'success' },
    degraded: { label: '降级', variant: 'warning' },
    offline: { label: '离线', variant: 'destructive' },
    unknown: { label: '未知', variant: 'secondary' },
  }
  const v = (status && map[status]) || map.unknown
  return <Badge variant={v.variant}>{v.label}</Badge>
}

// ===== 主组件 =====
export default function NodeDoctor() {
  const { toast } = useToast()

  const [nodes, setNodes] = useState<NodeOption[]>([])
  const [nodesLoading, setNodesLoading] = useState(true)
  const [selectedNodeId, setSelectedNodeId] = useState('')

  // checkDefs 由后端报告内嵌返回，后端无独立列表接口
  const [checkDefs, setCheckDefs] = useState<DoctorCheckDef[]>([])
  const [reports, setReports] = useState<DoctorReport[]>([])
  const [reportsLoading, setReportsLoading] = useState(false)
  const [running, setRunning] = useState(false)
  const [expandedReportId, setExpandedReportId] = useState<string | null>(null)

  const loadNodes = async () => {
    setNodesLoading(true)
    try {
      const data = await api.get(EP.NODES)
      const list = normalizeList<NodeOption>(data)
      setNodes(list)
      if (list.length > 0 && !selectedNodeId) {
        setSelectedNodeId(list[0].id)
      }
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '加载节点列表失败'
      toast({ title: '加载失败', description: msg, variant: 'destructive' })
    } finally {
      setNodesLoading(false)
    }
  }

  // 后端无 /admin/doctor-check-defs 接口；检查项定义由最新报告内嵌返回
  const loadCheckDefs = async () => {
    setCheckDefs([])
  }

  const loadReports = async (nodeId: string) => {
    if (!nodeId) return
    setReportsLoading(true)
    setReports([])
    try {
      const data = await api.get(EP.NODE_DOCTOR_REPORTS(nodeId))
      const list = normalizeList<DoctorReport>(data)
      setReports(list)
      if (list.length > 0) setExpandedReportId(list[0].id)
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '加载体检报告失败'
      toast({ title: '加载失败', description: msg, variant: 'destructive' })
    } finally {
      setReportsLoading(false)
    }
  }

  useEffect(() => {
    loadNodes()
    loadCheckDefs()
  }, [])

  useEffect(() => {
    if (selectedNodeId) loadReports(selectedNodeId)
  }, [selectedNodeId])

  const runCheck = async () => {
    if (!selectedNodeId) return
    setRunning(true)
    try {
      await api.post(EP.NODE_DOCTOR_CHECK(selectedNodeId))
      toast({ title: '检测已触发', description: '正在重新检测节点，请稍候', variant: 'success' })
      await loadReports(selectedNodeId)
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '触发检测失败'
      toast({ title: '检测失败', description: msg, variant: 'destructive' })
    } finally {
      setRunning(false)
    }
  }

  // 最新报告
  const latestReport = reports[0] || null

  // 当前检查项状态映射（基于最新报告）
  const currentCheckMap = new Map<string, DoctorCheckResult>()
  if (latestReport?.checks) {
    for (const c of latestReport.checks) {
      const key = c.def_code || c.def_id || c.name
      currentCheckMap.set(key, c)
    }
  }

  // 问题汇总：fail / warn
  const problems: DoctorCheckResult[] = []
  if (latestReport?.checks) {
    for (const c of latestReport.checks) {
      if (c.status === 'fail' || c.status === 'warn') problems.push(c)
    }
  }

  // 统计
  const passCount =
    latestReport?.pass_count ?? latestReport?.checks?.filter((c) => c.status === 'pass').length ?? 0
  const failCount =
    latestReport?.fail_count ?? latestReport?.checks?.filter((c) => c.status === 'fail').length ?? 0
  const warnCount =
    latestReport?.warn_count ?? latestReport?.checks?.filter((c) => c.status === 'warn').length ?? 0

  const checkListSource =
    checkDefs.length > 0
      ? checkDefs
      : latestReport?.checks?.map((c) => ({
          id: c.def_id || c.name,
          code: c.def_code || '',
          name: c.name,
          description: c.description,
        })) || []

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-zinc-100 flex items-center gap-2">
          <Stethoscope className="w-5 h-5 text-indigo-400" />
          节点体检
        </h2>
      </div>

      {/* 顶部控制栏 */}
      <Card className="bg-zinc-900 border-zinc-800">
        <CardContent className="p-3">
          <div className="flex flex-col sm:flex-row gap-2">
            <div className="flex-1 space-y-1">
              <label className="text-xs text-zinc-500">选择节点</label>
              <Select
                value={selectedNodeId}
                onChange={(e) => setSelectedNodeId(e.target.value)}
                disabled={nodesLoading}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              >
                {nodesLoading && <option value="">加载中...</option>}
                {!nodesLoading && nodes.length === 0 && <option value="">暂无节点</option>}
                {nodes.map((n) => (
                  <option key={n.id} value={n.id}>
                    {n.name}
                  </option>
                ))}
              </Select>
            </div>
            <div className="flex items-end">
              <Button
                className="bg-indigo-600 hover:bg-indigo-500"
                onClick={runCheck}
                disabled={!selectedNodeId || running}
              >
                <RefreshCw className={`w-4 h-4 mr-1.5 ${running ? 'animate-spin' : ''}`} />
                {running ? '检测中...' : '重新检测'}
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      {!selectedNodeId ? (
        <Card className="bg-zinc-900 border-zinc-800">
          <CardContent>
            <EmptyState title="请选择节点" description="选择一个节点以查看体检结果" className="py-12" />
          </CardContent>
        </Card>
      ) : (
        <>
          {/* 统计概览 */}
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            <Card className="bg-zinc-900 border-zinc-800">
              <CardContent className="p-3">
                <div className="text-xs text-zinc-500">总检查项</div>
                <div className="text-xl font-semibold text-zinc-100 mt-1">
                  {checkDefs.length || latestReport?.checks?.length || 0}
                </div>
              </CardContent>
            </Card>
            <Card className="bg-zinc-900 border-zinc-800">
              <CardContent className="p-3">
                <div className="text-xs text-zinc-500 flex items-center gap-1">
                  <CheckCircle2 className="w-3 h-3 text-emerald-400" /> 通过
                </div>
                <div className="text-xl font-semibold text-emerald-400 mt-1">{passCount}</div>
              </CardContent>
            </Card>
            <Card className="bg-zinc-900 border-zinc-800">
              <CardContent className="p-3">
                <div className="text-xs text-zinc-500 flex items-center gap-1">
                  <AlertTriangle className="w-3 h-3 text-amber-400" /> 警告
                </div>
                <div className="text-xl font-semibold text-amber-400 mt-1">{warnCount}</div>
              </CardContent>
            </Card>
            <Card className="bg-zinc-900 border-zinc-800">
              <CardContent className="p-3">
                <div className="text-xs text-zinc-500 flex items-center gap-1">
                  <XCircle className="w-3 h-3 text-red-400" /> 失败
                </div>
                <div className="text-xl font-semibold text-red-400 mt-1">{failCount}</div>
              </CardContent>
            </Card>
          </div>

          {/* 问题摘要 */}
          {problems.length > 0 && (
            <Card className="bg-zinc-900 border-amber-800/40">
              <CardHeader className="pb-3">
                <CardTitle className="text-base flex items-center gap-2">
                  <ShieldAlert className="w-4 h-4 text-amber-400" />
                  问题摘要与修复建议
                  <Badge variant="warning" className="ml-1">{problems.length}</Badge>
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                {problems.map((p, idx) => {
                  const cfg = statusConfig[p.status] || statusConfig.warn
                  const Icon = cfg.Icon
                  return (
                    <div key={idx} className="rounded-lg border border-zinc-800 bg-zinc-950/40 p-3">
                      <div className="flex items-start gap-2">
                        <Icon className={`w-4 h-4 mt-0.5 ${cfg.color}`} />
                        <div className="flex-1 min-w-0 space-y-1">
                          <div className="flex items-center gap-2 flex-wrap">
                            <span className="text-sm font-medium text-zinc-200">{p.name}</span>
                            <Badge variant="outline" className={cfg.badgeClass}>{cfg.label}</Badge>
                          </div>
                          {p.message && <p className="text-xs text-zinc-400">{p.message}</p>}
                          {p.suggestion && (
                            <div className="flex items-start gap-1.5 mt-1.5 rounded-md bg-indigo-950/30 border border-indigo-900/40 p-2">
                              <Wrench className="w-3.5 h-3.5 text-indigo-400 mt-0.5 flex-shrink-0" />
                              <div>
                                <div className="text-xs text-indigo-300">修复建议</div>
                                <p className="text-xs text-zinc-300 mt-0.5">{p.suggestion}</p>
                              </div>
                            </div>
                          )}
                        </div>
                      </div>
                    </div>
                  )
                })}
              </CardContent>
            </Card>
          )}

          {/* 检查项列表 */}
          <Card className="bg-zinc-900 border-zinc-800">
            <CardHeader className="pb-3">
              <CardTitle className="text-base">检查项</CardTitle>
            </CardHeader>
            <CardContent>
              {reportsLoading ? (
                <div className="space-y-2">
                  {[1, 2, 3, 4].map((i) => (
                    <Skeleton key={i} className="h-12 w-full bg-zinc-800 rounded-lg" />
                  ))}
                </div>
              ) : checkListSource.length === 0 ? (
                <EmptyState title="暂无检查项" description="尚未配置节点体检检查定义" className="py-8" />
              ) : (
                <div className="space-y-2">
                  {checkListSource.map((def) => {
                    const key = def.code || def.id || def.name
                    const result = currentCheckMap.get(key)
                    const status: CheckResultStatus = result?.status || 'unknown'
                    const cfg = statusConfig[status]
                    const Icon = cfg.Icon
                    return (
                      <div key={def.id || key} className="flex items-start gap-3 rounded-lg border border-zinc-800 bg-zinc-950/30 p-3">
                        <Icon className={`w-4 h-4 mt-0.5 ${cfg.color}`} />
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center justify-between gap-2">
                            <span className="text-sm font-medium text-zinc-200">{def.name}</span>
                            <Badge variant="outline" className={cfg.badgeClass}>{cfg.label}</Badge>
                          </div>
                          {def.description && <p className="text-xs text-zinc-500 mt-0.5">{def.description}</p>}
                          {result?.message && <p className="text-xs text-zinc-400 mt-1">{result.message}</p>}
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}
            </CardContent>
          </Card>

          {/* 体检报告历史 */}
          <Card className="bg-zinc-900 border-zinc-800">
            <CardHeader className="pb-3">
              <CardTitle className="text-base flex items-center gap-2">
                体检报告历史
                {reports.length > 0 && (
                  <Badge variant="secondary" className="bg-zinc-800 text-zinc-300 text-xs">{reports.length}</Badge>
                )}
              </CardTitle>
            </CardHeader>
            <CardContent>
              {reportsLoading ? (
                <div className="space-y-2">
                  {[1, 2, 3].map((i) => (
                    <Skeleton key={i} className="h-16 w-full bg-zinc-800 rounded-lg" />
                  ))}
                </div>
              ) : reports.length === 0 ? (
                <EmptyState title="暂无报告" description="点击「重新检测」生成首份体检报告" className="py-8" />
              ) : (
                <div className="relative">
                  <div className="absolute left-[19px] top-0 bottom-0 w-px bg-zinc-800" />
                  <div className="space-y-0">
                    {reports.map((r) => {
                      const expanded = expandedReportId === r.id
                      const rPass = r.pass_count ?? r.checks?.filter((c) => c.status === 'pass').length ?? 0
                      const rFail = r.fail_count ?? r.checks?.filter((c) => c.status === 'fail').length ?? 0
                      const rWarn = r.warn_count ?? r.checks?.filter((c) => c.status === 'warn').length ?? 0
                      return (
                        <div key={r.id} className="relative pl-10 py-3">
                          <button
                            className="absolute left-4 top-4 w-2 h-2 rounded-full bg-indigo-500 border-2 border-zinc-900"
                            onClick={() => setExpandedReportId(expanded ? null : r.id)}
                          />
                          <div
                            className="rounded-lg border border-zinc-800 bg-zinc-950/30 hover:bg-zinc-800/30 cursor-pointer transition-colors"
                            onClick={() => setExpandedReportId(expanded ? null : r.id)}
                          >
                            <div className="p-3 flex items-center justify-between gap-2">
                              <div className="flex items-center gap-2 flex-wrap">
                                {expanded ? (
                                  <ChevronDown className="w-4 h-4 text-zinc-400" />
                                ) : (
                                  <ChevronRight className="w-4 h-4 text-zinc-400" />
                                )}
                                {getReportStatusBadge(r.status)}
                                <span className="text-xs text-zinc-500">
                                  通过 <span className="text-emerald-400">{rPass}</span> · 警告{' '}
                                  <span className="text-amber-400">{rWarn}</span> · 失败{' '}
                                  <span className="text-red-400">{rFail}</span>
                                </span>
                              </div>
                              <div className="text-xs text-zinc-500">{formatTime(r.created_at)}</div>
                            </div>
                            {expanded && (
                              <div className="px-3 pb-3 pt-1 border-t border-zinc-800 space-y-2">
                                {r.summary && <p className="text-xs text-zinc-400 pt-2">{r.summary}</p>}
                                {!r.checks || r.checks.length === 0 ? (
                                  <p className="text-xs text-zinc-500 pt-2">无详细检查结果</p>
                                ) : (
                                  r.checks.map((c, i) => {
                                    const cfg = statusConfig[c.status] || statusConfig.unknown
                                    return (
                                      <div key={i} className="flex items-start gap-2 py-1">
                                        <StatusIcon status={c.status} />
                                        <div className="flex-1 min-w-0">
                                          <div className="flex items-center gap-2 flex-wrap">
                                            <span className="text-sm text-zinc-200">{c.name}</span>
                                            <Badge variant="outline" className={cfg.badgeClass}>{cfg.label}</Badge>
                                          </div>
                                          {c.message && <p className="text-xs text-zinc-500 mt-0.5">{c.message}</p>}
                                          {c.suggestion && (
                                            <p className="text-xs text-indigo-300 mt-0.5 flex items-start gap-1">
                                              <Wrench className="w-3 h-3 mt-0.5 flex-shrink-0" />
                                              {c.suggestion}
                                            </p>
                                          )}
                                        </div>
                                      </div>
                                    )
                                  })
                                )}
                              </div>
                            )}
                          </div>
                        </div>
                      )
                    })}
                  </div>
                </div>
              )}
            </CardContent>
          </Card>
        </>
      )}
    </div>
  )
}
