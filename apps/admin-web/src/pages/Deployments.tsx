import { useState, useEffect, useRef, useCallback } from 'react'
import {
  Rocket,
  RotateCcw,
  Eye,
  RefreshCw,
  Search,
  Clock,
  CheckCircle2,
  XCircle,
  Loader2,
  AlertTriangle,
  FileJson,
  ChevronDown,
  ChevronUp,
  Layers,
} from 'lucide-react'
import {
  Card,
  CardContent,
  Badge,
  Button,
  Input,
  Skeleton,
  useToast,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  Separator,
} from '@airport/ui'
import { api, ApiError } from '@/lib/api'
import { EP } from '@/lib/endpoints'

// ===== 类型定义（对齐后端 model.DeploymentBatch / DeploymentTarget）=====

interface DeploymentTarget {
  id: string
  deployment_batch_id: string
  target_type: string  // "node" | "runtime"
  target_id: string
  target_version_id: string
  previous_version_id?: string
  phase_no: number
  status: string  // pending/precheck/applying/verifying/success/failed/rolling_back/rolled_back/paused
  precheck_result?: Record<string, unknown>
  apply_result?: Record<string, unknown>
  rollback_result?: Record<string, unknown>
  started_at?: string
  finished_at?: string
  created_at: string
}

interface DeploymentBatch {
  id: string
  scope_type: string     // node/runtime/group/global
  scope_id: string
  target_version_id: string
  strategy: string       // rolling/blue_green/canary/all_at_once
  batch_plan: unknown[]
  status: string         // pending/running/paused/success/failed/rolled_back
  started_at?: string
  finished_at?: string
  created_by_admin_id?: string
  created_at: string
  // 以下字段由前端 loadBatchResults 填充
  targets?: DeploymentTarget[]
}

// GetBatchResults 返回结构
interface BatchResultsResponse {
  batch_id: string
  targets: DeploymentTarget[]
  target_count: number
  status_counts: Record<string, number>
  phases: Array<{ phase_no: number; target_count: number; status_counts: Record<string, number> }>
}

interface DryRunResult {
  xray_inbound?: Record<string, unknown>
  nginx_location?: Record<string, unknown> | string
  [key: string]: unknown
}

interface DiffResult {
  old_config?: Record<string, unknown>
  new_config?: Record<string, unknown>
  changes?: Array<{ path: string; old_value: unknown; new_value: unknown }>
  [key: string]: unknown
}

// ===== 状态颜色映射（对齐后端 DeploymentStatus / TargetStatus）=====

const STATUS_CONFIG: Record<string, { color: string; bg: string; text: string; icon: typeof CheckCircle2 }> = {
  // DeploymentBatch 状态
  pending: { color: 'bg-amber-500', bg: 'bg-amber-500/10', text: 'text-amber-400', icon: Clock },
  running: { color: 'bg-blue-500', bg: 'bg-blue-500/10', text: 'text-blue-400', icon: Loader2 },
  paused: { color: 'bg-zinc-500', bg: 'bg-zinc-500/10', text: 'text-zinc-400', icon: Clock },
  success: { color: 'bg-emerald-500', bg: 'bg-emerald-500/10', text: 'text-emerald-400', icon: CheckCircle2 },
  failed: { color: 'bg-red-500', bg: 'bg-red-500/10', text: 'text-red-400', icon: XCircle },
  rolled_back: { color: 'bg-zinc-500', bg: 'bg-zinc-800', text: 'text-zinc-400', icon: RotateCcw },
  // DeploymentTarget 状态
  precheck: { color: 'bg-indigo-500', bg: 'bg-indigo-500/10', text: 'text-indigo-400', icon: Loader2 },
  applying: { color: 'bg-blue-500', bg: 'bg-blue-500/10', text: 'text-blue-400', icon: Loader2 },
  verifying: { color: 'bg-cyan-500', bg: 'bg-cyan-500/10', text: 'text-cyan-400', icon: Loader2 },
  rolling_back: { color: 'bg-amber-500', bg: 'bg-amber-500/10', text: 'text-amber-400', icon: RotateCcw },
}

const STATUS_LABELS: Record<string, string> = {
  pending: '等待中',
  running: '部署中',
  paused: '已暂停',
  success: '已应用',
  failed: '失败',
  rolled_back: '已回滚',
  precheck: '预检中',
  applying: '应用中',
  verifying: '验证中',
  rolling_back: '回滚中',
}

function StatusBadge({ status }: { status: string }) {
  const cfg = STATUS_CONFIG[status] || STATUS_CONFIG.pending
  const label = STATUS_LABELS[status] || status
  const Icon = cfg.icon
  const isSpinning = status === 'deploying'
  return (
    <span className={`inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded ${cfg.bg} ${cfg.text}`}>
      <Icon className={`w-3 h-3 ${isSpinning ? 'animate-spin' : ''}`} />
      {label}
    </span>
  )
}

// ===== JSON 语法高亮 =====

function JsonHighlight({ data }: { data: unknown }) {
  const jsonStr = typeof data === 'string' ? data : JSON.stringify(data, null, 2)
  return (
    <pre className="text-xs font-mono whitespace-pre-wrap break-all leading-relaxed">
      {jsonStr.split('\n').map((line, i) => {
        const highlighted = line
          .replace(/("(?:[^"\\]|\\.)*")\s*:/g, '<k>$1</k>')
          .replace(/:\s*("(?:[^"\\]|\\.)*")/g, ':<s>$1</s>')
          .replace(/:\s*(\d+\.?\d*)/g, ':<n>$1</n>')
          .replace(/:\s*(true|false)/g, ':<b>$1</b>')
          .replace(/:\s*(null)/g, ':<null>$1</null>')
        const parts = highlighted.split(/(<\/?(?:k|s|n|b|null)>)/)
        return (
          <span key={i}>
            {parts.map((part, j) => {
              if (part === '<k>' || part === '</k>') return null
              if (part === '<s>' || part === '</s>') return null
              if (part === '<n>' || part === '</n>') return null
              if (part === '<b>' || part === '</b>') return null
              if (part === '<null>' || part === '</null>') return null
              const prevTag = parts[j - 1]
              if (prevTag === '<k>') return <span key={j} className="text-indigo-400">{part}</span>
              if (prevTag === '<s>') return <span key={j} className="text-emerald-400">{part}</span>
              if (prevTag === '<n>') return <span key={j} className="text-amber-400">{part}</span>
              if (prevTag === '<b>') return <span key={j} className="text-blue-400">{part}</span>
              if (prevTag === '<null>') return <span key={j} className="text-zinc-500">{part}</span>
              return <span key={j} className="text-zinc-300">{part}</span>
            })}
            {'\n'}
          </span>
        )
      })}
    </pre>
  )
}

// ===== 主页面 =====

export default function Deployments() {
  const { toast } = useToast()
  const [loading, setLoading] = useState(true)
  const [batches, setBatches] = useState<DeploymentBatch[]>([])
  const [search, setSearch] = useState('')
  const [expandedId, setExpandedId] = useState<string | null>(null)

  // F2: Config Preview
  const [previewOpen, setPreviewOpen] = useState(false)
  const [previewData, setPreviewData] = useState<DryRunResult | null>(null)
  const [previewLoading, setPreviewLoading] = useState(false)

  // F4: Rollback
  const [rollbackDialogId, setRollbackDialogId] = useState<string | null>(null)
  const [rollbackVersion, setRollbackVersion] = useState<string>('')
  const [rollbackLoading, setRollbackLoading] = useState(false)

  // F3: Diff
  const [diffOpen, setDiffOpen] = useState(false)
  const [diffData, setDiffData] = useState<DiffResult | null>(null)
  const [diffLoading, setDiffLoading] = useState(false)

  // F3: 实时轮询
  const pollingRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const [pollingActive, setPollingActive] = useState(false)

  const loadBatches = useCallback(async () => {
    try {
      const data = await api.get<{ items: DeploymentBatch[]; total: number }>(EP.DEPLOYMENTS, {
        params: { page: 1, page_size: 50 },
      })
      setBatches(data.items || [])
      // 如果存在正在部署的批次，启动轮询（后端状态: pending/running）
      const hasDeploying = (data.items || []).some(b => b.status === 'running' || b.status === 'pending')
      setPollingActive(hasDeploying)
    } catch (err) {
      toast({
        title: '加载失败',
        description: err instanceof Error ? err.message : '无法获取部署列表',
        variant: 'destructive',
      })
    } finally {
      setLoading(false)
    }
  }, [toast])

  useEffect(() => {
    loadBatches()
  }, [loadBatches])

  // F3: 5s 轮询部署中的批次
  useEffect(() => {
    if (pollingRef.current) {
      clearInterval(pollingRef.current)
      pollingRef.current = null
    }
    if (pollingActive) {
      pollingRef.current = setInterval(async () => {
        try {
          const data = await api.get<{ items: DeploymentBatch[]; total: number }>(EP.DEPLOYMENTS, {
            params: { page: 1, page_size: 50 },
          })
          setBatches(data.items || [])
          const stillDeploying = (data.items || []).some(b => b.status === 'running' || b.status === 'pending')
          if (!stillDeploying) {
            setPollingActive(false)
          }
        } catch {
          // 静默失败
        }
      }, 5000)
    }
    return () => {
      if (pollingRef.current) {
        clearInterval(pollingRef.current)
      }
    }
  }, [pollingActive])

  // F3: 展开时加载详细结果
  const loadBatchResults = async (batchId: string) => {
    try {
      const data = await api.get<BatchResultsResponse>(EP.DEPLOYMENT_RESULTS(batchId))
      setBatches(prev => prev.map(b => b.id === batchId ? { ...b, targets: data.targets || [] } : b))
    } catch {
      // 静默失败
    }
  }

  const toggleExpand = (batchId: string) => {
    if (expandedId === batchId) {
      setExpandedId(null)
    } else {
      setExpandedId(batchId)
      loadBatchResults(batchId)
    }
  }

  // F2: Config Preview (dry-run)
  const handlePreview = async () => {
    setPreviewLoading(true)
    setPreviewOpen(true)
    try {
      const data = await api.post<DryRunResult>(EP.DEPLOYMENT_DRY_RUN, {})
      setPreviewData(data)
    } catch (err) {
      toast({
        title: '预览失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
      setPreviewOpen(false)
    } finally {
      setPreviewLoading(false)
    }
  }

  // F4: Rollback
  const openRollbackDialog = (batch: DeploymentBatch) => {
    setRollbackDialogId(batch.id)
    setRollbackVersion(batch.target_version_id ? batch.target_version_id.substring(0, 8) : '未知版本')
  }

  const handleRollback = async () => {
    if (!rollbackDialogId) return
    setRollbackLoading(true)
    try {
      await api.post(EP.DEPLOYMENT_ROLLBACK(rollbackDialogId))
      toast({ title: '回滚成功', description: '部署已回滚到上一版本', variant: 'success' })
      setRollbackDialogId(null)
      await loadBatches()
    } catch (err) {
      toast({
        title: '回滚失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setRollbackLoading(false)
    }
  }

  // F3: Diff
  const handleDiff = async (batchId: string) => {
    setDiffLoading(true)
    setDiffOpen(true)
    try {
      const data = await api.get<DiffResult>(EP.DEPLOYMENT_DIFF(batchId))
      setDiffData(data)
    } catch (err) {
      toast({
        title: '获取差异失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
      setDiffOpen(false)
    } finally {
      setDiffLoading(false)
    }
  }

  // 过滤
  const filteredBatches = batches.filter(b => {
    const kw = search.trim().toLowerCase()
    if (!kw) return true
    return (
      b.id.toLowerCase().includes(kw) ||
      (b.scope_type || '').toLowerCase().includes(kw) ||
      (b.strategy || '').toLowerCase().includes(kw) ||
      (b.target_version_id || '').toLowerCase().includes(kw)
    )
  })

  // 统计（对齐后端状态值: pending/running/paused/success/failed/rolled_back）
  const stats = {
    total: batches.length,
    deploying: batches.filter(b => b.status === 'running' || b.status === 'pending').length,
    applied: batches.filter(b => b.status === 'success').length,
    failed: batches.filter(b => b.status === 'failed').length,
  }

  return (
    <div className="space-y-5 pb-20 sm:pb-4">
      {/* 页头 */}
      <div className="flex flex-col sm:flex-row sm:items-start justify-between gap-3">
        <div>
          <h2 className="text-xl font-semibold text-zinc-100 flex items-center gap-2">
            <Rocket className="w-6 h-6 text-indigo-400" />部署管理
          </h2>
          <p className="text-sm text-zinc-400 mt-1">管理配置发布、预览与回滚</p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={handlePreview}
            className="border-zinc-700 text-zinc-300 hover:bg-zinc-800"
          >
            <Eye className="w-4 h-4 mr-1.5" />配置预览
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={loadBatches}
            className="text-zinc-400 hover:text-zinc-200 h-9"
          >
            <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
          </Button>
        </div>
      </div>

      {/* 统计卡片 */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
        <Card className="bg-zinc-900 border-zinc-800">
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs text-zinc-500">部署总数</p>
                <p className="text-2xl font-bold text-zinc-100 mt-1">{stats.total}</p>
              </div>
              <div className="w-10 h-10 rounded-lg bg-zinc-800 flex items-center justify-center">
                <Layers className="w-5 h-5 text-zinc-400" />
              </div>
            </div>
          </CardContent>
        </Card>
        <Card className="bg-zinc-900 border-zinc-800">
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs text-zinc-500">部署中</p>
                <p className="text-2xl font-bold text-blue-400 mt-1">{stats.deploying}</p>
              </div>
              <div className="w-10 h-10 rounded-lg bg-blue-500/10 flex items-center justify-center">
                <Loader2 className="w-5 h-5 text-blue-400 animate-spin" />
              </div>
            </div>
          </CardContent>
        </Card>
        <Card className="bg-zinc-900 border-zinc-800">
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs text-zinc-500">已应用</p>
                <p className="text-2xl font-bold text-emerald-400 mt-1">{stats.applied}</p>
              </div>
              <div className="w-10 h-10 rounded-lg bg-emerald-500/10 flex items-center justify-center">
                <CheckCircle2 className="w-5 h-5 text-emerald-400" />
              </div>
            </div>
          </CardContent>
        </Card>
        <Card className="bg-zinc-900 border-zinc-800">
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs text-zinc-500">失败</p>
                <p className="text-2xl font-bold text-red-400 mt-1">{stats.failed}</p>
              </div>
              <div className="w-10 h-10 rounded-lg bg-red-500/10 flex items-center justify-center">
                <XCircle className="w-5 h-5 text-red-400" />
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* 搜索 */}
      <Card className="bg-zinc-900 border-zinc-800">
        <CardContent className="p-3">
          <div className="relative flex-1">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-zinc-500" />
            <Input
              placeholder="搜索部署批次 ID、描述、版本..."
              value={search}
              onChange={e => setSearch(e.target.value)}
              className="pl-9 bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500 h-9"
            />
          </div>
        </CardContent>
      </Card>

      {/* 批次列表 */}
      {loading ? (
        <div className="space-y-3">
          {Array.from({ length: 4 }).map((_, i) => (
            <Card key={i} className="bg-zinc-900 border-zinc-800">
              <CardContent className="p-4 space-y-3">
                <Skeleton className="h-5 w-64 bg-zinc-800" />
                <Skeleton className="h-4 w-48 bg-zinc-800" />
              </CardContent>
            </Card>
          ))}
        </div>
      ) : filteredBatches.length === 0 ? (
        <Card className="bg-zinc-900 border-zinc-800">
          <CardContent className="py-16 text-center">
            <Rocket className="w-12 h-12 text-zinc-600 mx-auto mb-3" />
            <p className="text-zinc-400 text-sm">暂无部署记录</p>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-3">
          {filteredBatches.map(batch => {
            const isExpanded = expandedId === batch.id
            const isDeploying = batch.status === 'running' || batch.status === 'pending'
            const statusCfg = STATUS_CONFIG[batch.status] || STATUS_CONFIG.pending

            return (
              <Card key={batch.id} className={`bg-zinc-900 border-zinc-800 hover:border-zinc-700 transition-colors`}>
                <CardContent className="p-4">
                  {/* 批次摘要行 */}
                  <div className="flex items-center justify-between gap-3">
                    <div className="flex items-center gap-3 min-w-0 flex-1">
                      <div className={`w-2.5 h-2.5 rounded-full ${statusCfg.color} ${isDeploying ? 'animate-pulse' : ''}`} />
                      <div className="min-w-0">
                        <div className="flex items-center gap-2 flex-wrap">
                          <span className="text-sm font-medium text-zinc-100 truncate">
                            {`部署批次 ${batch.id.substring(0, 8)}`}
                          </span>
                          <StatusBadge status={batch.status} />
                          {batch.target_version_id && (
                            <Badge variant="outline" className="text-[10px] px-1.5 py-0 bg-zinc-800 text-zinc-400 border-zinc-700 font-mono">
                              v{batch.target_version_id.substring(0, 8)}
                            </Badge>
                          )}
                          {batch.scope_type && (
                            <Badge variant="outline" className="text-[10px] px-1.5 py-0 bg-zinc-800/50 text-zinc-500 border-zinc-700">
                              {batch.scope_type}
                            </Badge>
                          )}
                          {batch.strategy && (
                            <Badge variant="outline" className="text-[10px] px-1.5 py-0 bg-zinc-800/50 text-zinc-500 border-zinc-700">
                              {batch.strategy}
                            </Badge>
                          )}
                        </div>
                        <div className="flex items-center gap-3 mt-1 text-[11px] text-zinc-500">
                          <span className="font-mono">{batch.id.substring(0, 8)}</span>
                          <span>{new Date(batch.created_at).toLocaleString('zh-CN')}</span>
                          {batch.started_at && (
                            <span>开始: {new Date(batch.started_at).toLocaleTimeString('zh-CN')}</span>
                          )}
                          {batch.finished_at && (
                            <span>完成: {new Date(batch.finished_at).toLocaleTimeString('zh-CN')}</span>
                          )}
                        </div>
                      </div>
                    </div>

                    <div className="flex items-center gap-1 flex-shrink-0">
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-7 px-2 text-xs text-zinc-400 hover:text-blue-400"
                        onClick={() => handleDiff(batch.id)}
                      >
                        <FileJson className="w-3 h-3 mr-1" />差异
                      </Button>
                      {batch.status !== 'rolled_back' && batch.status !== 'success' && (
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-7 px-2 text-xs text-zinc-400 hover:text-amber-400"
                          onClick={() => openRollbackDialog(batch)}
                        >
                          <RotateCcw className="w-3 h-3 mr-1" />回滚
                        </Button>
                      )}
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-7 w-7 text-zinc-500 hover:text-zinc-200"
                        onClick={() => toggleExpand(batch.id)}
                      >
                        {isExpanded ? <ChevronUp className="w-3.5 h-3.5" /> : <ChevronDown className="w-3.5 h-3.5" />}
                      </Button>
                    </div>
                  </div>

                  {/* F3: 展开的目标状态列表 */}
                  {isExpanded && (
                    <div className="mt-3 pt-3 border-t border-zinc-800">
                      <div className="text-xs text-zinc-500 mb-2">部署目标状态</div>
                      {batch.targets && batch.targets.length > 0 ? (
                        <div className="space-y-2">
                          {batch.targets.map(target => {
                            const tCfg = STATUS_CONFIG[target.status] || STATUS_CONFIG.pending
                            const TIcon = tCfg.icon
                            const isTargetActive = ['applying', 'verifying', 'precheck', 'rolling_back'].includes(target.status)
                            // 从 apply_result 提取错误信息
                            const errorMsg = target.apply_result?.error as string || target.apply_result?.message as string || ''
                            return (
                              <div
                                key={target.id}
                                className="flex items-center justify-between gap-2 rounded-lg border border-zinc-800 bg-zinc-950/40 px-3 py-2"
                              >
                                <div className="flex items-center gap-2 min-w-0">
                                  <TIcon className={`w-3.5 h-3.5 flex-shrink-0 ${tCfg.text} ${isTargetActive ? 'animate-spin' : ''}`} />
                                  <span className="text-sm text-zinc-200 truncate">
                                    {target.target_type} · {target.target_id.substring(0, 8)}
                                  </span>
                                  <span className="text-[10px] text-zinc-500">阶段 {target.phase_no}</span>
                                </div>
                                <div className="flex items-center gap-2 flex-shrink-0">
                                  {errorMsg && (
                                    <span className="text-[10px] text-red-400 max-w-48 truncate" title={errorMsg}>
                                      {errorMsg}
                                    </span>
                                  )}
                                  <StatusBadge status={target.status} />
                                </div>
                              </div>
                            )
                          })}
                        </div>
                      ) : (
                        <div className="text-center py-4">
                          <Loader2 className="w-4 h-4 text-zinc-600 animate-spin mx-auto" />
                          <p className="text-xs text-zinc-500 mt-1">加载目标状态...</p>
                        </div>
                      )}
                    </div>
                  )}
                </CardContent>
              </Card>
            )
          })}
        </div>
      )}

      {/* F2: Config Preview 对话框 */}
      <Dialog open={previewOpen} onOpenChange={setPreviewOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-4xl max-h-[85vh] overflow-hidden flex flex-col">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Eye className="w-5 h-5 text-indigo-400" />
              <span>配置预览 (Dry Run)</span>
            </DialogTitle>
          </DialogHeader>
          <div className="flex-1 overflow-y-auto pr-1 space-y-4">
            {previewLoading ? (
              <div className="space-y-3 py-4">
                <Skeleton className="h-4 w-32 bg-zinc-800" />
                <Skeleton className="h-40 w-full bg-zinc-800 rounded-lg" />
                <Skeleton className="h-4 w-32 bg-zinc-800" />
                <Skeleton className="h-40 w-full bg-zinc-800 rounded-lg" />
              </div>
            ) : previewData ? (
              <>
                {previewData.xray_inbound && (
                  <div className="space-y-2">
                    <div className="text-sm font-medium text-zinc-300 flex items-center gap-1.5">
                      <span className="w-2 h-2 rounded-full bg-indigo-500" />
                      Xray Inbound 配置
                    </div>
                    <div className="rounded-lg border border-zinc-800 bg-zinc-950 p-3 max-h-72 overflow-auto">
                      <JsonHighlight data={previewData.xray_inbound} />
                    </div>
                  </div>
                )}
                {previewData.nginx_location && (
                  <div className="space-y-2">
                    <div className="text-sm font-medium text-zinc-300 flex items-center gap-1.5">
                      <span className="w-2 h-2 rounded-full bg-emerald-500" />
                      Nginx Location 配置
                    </div>
                    <div className="rounded-lg border border-zinc-800 bg-zinc-950 p-3 max-h-72 overflow-auto">
                      <JsonHighlight data={previewData.nginx_location} />
                    </div>
                  </div>
                )}
                {/* 其他字段 */}
                {Object.entries(previewData)
                  .filter(([k]) => k !== 'xray_inbound' && k !== 'nginx_location')
                  .map(([key, value]) => (
                    <div key={key} className="space-y-2">
                      <div className="text-sm font-medium text-zinc-300 flex items-center gap-1.5">
                        <span className="w-2 h-2 rounded-full bg-zinc-500" />
                        {key}
                      </div>
                      <div className="rounded-lg border border-zinc-800 bg-zinc-950 p-3 max-h-48 overflow-auto">
                        <JsonHighlight data={value} />
                      </div>
                    </div>
                  ))}
              </>
            ) : (
              <div className="text-center py-8">
                <p className="text-zinc-500 text-sm">无预览数据</p>
              </div>
            )}
          </div>
          <DialogFooter className="pt-2">
            <Button variant="outline" onClick={() => setPreviewOpen(false)} className="border-zinc-700 text-zinc-300">
              关闭
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* F4: Rollback 确认对话框 */}
      <Dialog open={!!rollbackDialogId} onOpenChange={(o) => { if (!o) setRollbackDialogId(null) }}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <RotateCcw className="w-5 h-5 text-amber-400" />
              <span>确认回滚</span>
            </DialogTitle>
          </DialogHeader>
          <div className="py-4 space-y-3">
            <div className="rounded-lg border border-amber-500/30 bg-amber-500/5 p-3">
              <div className="flex items-start gap-2">
                <AlertTriangle className="w-4 h-4 text-amber-400 flex-shrink-0 mt-0.5" />
                <div className="text-sm text-amber-200">
                  此操作将回滚部署配置到上一版本，正在部署中的目标将中断。请确认操作。
                </div>
              </div>
            </div>
            <div className="space-y-2 text-sm">
              <div className="flex items-center justify-between">
                <span className="text-zinc-400">批次 ID</span>
                <span className="text-zinc-200 font-mono">{rollbackDialogId?.substring(0, 8)}</span>
              </div>
              <Separator className="bg-zinc-800" />
              <div className="flex items-center justify-between">
                <span className="text-zinc-400">回滚至版本</span>
                <span className="text-zinc-200 font-mono">{rollbackVersion}</span>
              </div>
            </div>
          </div>
          <DialogFooter className="gap-2">
            <Button variant="outline" onClick={() => setRollbackDialogId(null)} className="border-zinc-700 text-zinc-300">
              取消
            </Button>
            <Button
              variant="destructive"
              onClick={handleRollback}
              isLoading={rollbackLoading}
            >
              确认回滚
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* F3: Diff 对话框 */}
      <Dialog open={diffOpen} onOpenChange={setDiffOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-4xl max-h-[85vh] overflow-hidden flex flex-col">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <FileJson className="w-5 h-5 text-blue-400" />
              <span>配置差异对比</span>
            </DialogTitle>
          </DialogHeader>
          <div className="flex-1 overflow-y-auto pr-1 space-y-4">
            {diffLoading ? (
              <div className="space-y-3 py-4">
                <Skeleton className="h-4 w-32 bg-zinc-800" />
                <Skeleton className="h-40 w-full bg-zinc-800 rounded-lg" />
              </div>
            ) : diffData ? (
              <>
                {diffData.changes && diffData.changes.length > 0 && (
                  <div className="space-y-2">
                    <div className="text-sm font-medium text-zinc-300">变更列表</div>
                    <div className="space-y-2">
                      {diffData.changes.map((change, i) => (
                        <div key={i} className="rounded-lg border border-zinc-800 bg-zinc-950/40 px-3 py-2">
                          <div className="text-xs font-mono text-indigo-400 mb-1">{change.path}</div>
                          <div className="grid grid-cols-2 gap-2">
                            <div>
                              <div className="text-[10px] text-red-400 mb-0.5">旧值</div>
                              <pre className="text-xs font-mono text-zinc-400 break-all">
                                {JSON.stringify(change.old_value, null, 2)}
                              </pre>
                            </div>
                            <div>
                              <div className="text-[10px] text-emerald-400 mb-0.5">新值</div>
                              <pre className="text-xs font-mono text-zinc-400 break-all">
                                {JSON.stringify(change.new_value, null, 2)}
                              </pre>
                            </div>
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
                {diffData.old_config && (
                  <div className="space-y-2">
                    <div className="text-sm font-medium text-zinc-300 flex items-center gap-1.5">
                      <span className="w-2 h-2 rounded-full bg-red-500" />
                      旧配置
                    </div>
                    <div className="rounded-lg border border-zinc-800 bg-zinc-950 p-3 max-h-48 overflow-auto">
                      <JsonHighlight data={diffData.old_config} />
                    </div>
                  </div>
                )}
                {diffData.new_config && (
                  <div className="space-y-2">
                    <div className="text-sm font-medium text-zinc-300 flex items-center gap-1.5">
                      <span className="w-2 h-2 rounded-full bg-emerald-500" />
                      新配置
                    </div>
                    <div className="rounded-lg border border-zinc-800 bg-zinc-950 p-3 max-h-48 overflow-auto">
                      <JsonHighlight data={diffData.new_config} />
                    </div>
                  </div>
                )}
              </>
            ) : (
              <div className="text-center py-8">
                <p className="text-zinc-500 text-sm">无差异数据</p>
              </div>
            )}
          </div>
          <DialogFooter className="pt-2">
            <Button variant="outline" onClick={() => setDiffOpen(false)} className="border-zinc-700 text-zinc-300">
              关闭
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
