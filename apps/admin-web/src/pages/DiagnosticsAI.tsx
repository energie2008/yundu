import { useState, useEffect, useCallback } from 'react'
import {
  Zap,
  Server,
  CheckCircle,
  XCircle,
  AlertTriangle,
  RefreshCw,
  FileText,
  ExternalLink,
  Clock,
  Wrench,
  Loader2,
  BookOpen,
} from 'lucide-react'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
  Button,
  Badge,
  Select,
  Textarea,
  Table,
  TableHeader,
  TableBody,
  TableHead,
  TableRow,
  TableCell,
  Skeleton,
  EmptyState,
  useToast,
} from '@airport/ui'
import { api, ApiError } from '@/lib/api'
import { EP } from '@/lib/endpoints'

// ===== 类型定义 =====
type DiagnosisCategory =
  | 'config_error'
  | 'network_issue'
  | 'cert_expired'
  | 'kernel_compat'
  | 'rate_limit'
  | 'resource_exhausted'
  | 'dns_issue'
  | 'firewall_block'
  | 'normal'
  | string

type SessionStatus = 'pending' | 'collecting' | 'analyzing' | 'done' | 'failed'

interface ServerOption {
  id: string
  code: string
  name: string
  host?: string
  status?: string
}

interface Suggestion {
  title: string
  description: string
  action?: string
  auto_fixable: boolean
}

interface DocLink {
  title: string
  url: string
}

interface DiagnosisSession {
  id: string
  server_id?: string
  node_id?: string
  status: SessionStatus
  trigger_source: string
  time_window_start?: string
  time_window_end?: string
  raw_logs?: string
  llm_provider: string
  llm_model?: string
  root_cause_category?: DiagnosisCategory
  root_cause_description?: string
  confidence?: number
  suggestions: Suggestion[]
  doc_links: DocLink[]
  autofix_applied: boolean
  autofix_result?: Record<string, unknown>
  duration_ms?: number
  created_at: string
  completed_at?: string
}

interface KnowledgeEntry {
  id: string
  title: string
  category: string
  root_cause_pattern: string
  solution: string
  auto_fix_action?: string
  doc_links: DocLink[]
  hit_count: number
  is_verified: boolean
  created_at: string
  updated_at: string
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

function formatTime(dateStr?: string) {
  if (!dateStr) return '-'
  const d = new Date(dateStr)
  if (isNaN(d.getTime())) return '-'
  return d.toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

const categoryConfigs: Record<string, { label: string; variant: 'destructive' | 'warning' | 'default' | 'success'; className?: string; Icon: typeof XCircle; color: string }> = {
  config_error: { label: '配置错误', variant: 'destructive', Icon: XCircle, color: 'text-red-400' },
  network_issue: { label: '网络问题', variant: 'warning', className: 'bg-yellow-600 hover:bg-yellow-500', Icon: AlertTriangle, color: 'text-yellow-400' },
  cert_expired: { label: '证书过期', variant: 'warning', className: 'bg-orange-600 hover:bg-orange-500', Icon: AlertTriangle, color: 'text-orange-400' },
  kernel_compat: { label: '内核兼容', variant: 'default', className: 'bg-blue-600 hover:bg-blue-500', Icon: AlertTriangle, color: 'text-blue-400' },
  rate_limit: { label: '限流触发', variant: 'warning', className: 'bg-purple-600 hover:bg-purple-500', Icon: AlertTriangle, color: 'text-purple-400' },
  resource_exhausted: { label: '资源耗尽', variant: 'destructive', className: 'bg-rose-600 hover:bg-rose-500', Icon: AlertTriangle, color: 'text-rose-400' },
  dns_issue: { label: 'DNS异常', variant: 'warning', className: 'bg-cyan-600 hover:bg-cyan-500', Icon: AlertTriangle, color: 'text-cyan-400' },
  firewall_block: { label: '防火墙阻断', variant: 'destructive', className: 'bg-red-700 hover:bg-red-600', Icon: XCircle, color: 'text-red-500' },
  normal: { label: '正常', variant: 'success', Icon: CheckCircle, color: 'text-emerald-400' },
}

function getCategoryConfig(category?: string) {
  if (!category) return { label: '未分类', variant: 'default' as const, Icon: AlertTriangle, color: 'text-zinc-400' }
  return categoryConfigs[category] || { label: category, variant: 'default' as const, Icon: AlertTriangle, color: 'text-zinc-400' }
}

const statusLabels: Record<SessionStatus, string> = {
  pending: '排队中',
  collecting: '采集日志',
  analyzing: 'AI 分析中',
  done: '已完成',
  failed: '失败',
}

function getStatusBadge(status: SessionStatus) {
  const map: Record<SessionStatus, { label: string; className: string }> = {
    pending: { label: '排队中', className: 'bg-zinc-800 text-zinc-300 border-zinc-700' },
    collecting: { label: '采集日志', className: 'bg-blue-900/50 text-blue-300 border-blue-800/50' },
    analyzing: { label: 'AI 分析中', className: 'bg-violet-900/50 text-violet-300 border-violet-800/50' },
    done: { label: '已完成', className: 'bg-emerald-900/50 text-emerald-300 border-emerald-800/50' },
    failed: { label: '失败', className: 'bg-red-900/50 text-red-300 border-red-800/50' },
  }
  const v = map[status] || map.pending
  return <Badge variant="secondary" className={v.className}>{v.label}</Badge>
}

// ===== 主组件 =====
export default function DiagnosticsAI() {
  const { toast } = useToast()

  const [servers, setServers] = useState<ServerOption[]>([])
  const [serversLoading, setServersLoading] = useState(true)
  const [selectedServerId, setSelectedServerId] = useState('')

  const [timeWindowMins, setTimeWindowMins] = useState(30)
  const [creating, setCreating] = useState(false)

  const [currentSession, setCurrentSession] = useState<DiagnosisSession | null>(null)
  const [polling, setPolling] = useState(false)

  const [history, setHistory] = useState<DiagnosisSession[]>([])
  const [historyLoading, setHistoryLoading] = useState(true)

  const [knowledge, setKnowledge] = useState<KnowledgeEntry[]>([])
  const [knowledgeLoading, setKnowledgeLoading] = useState(true)

  const [applyingAutofix, setApplyingAutofix] = useState(false)

  // 加载服务器列表
  const fetchServers = useCallback(async () => {
    setServersLoading(true)
    try {
      const data = await api.get<unknown>(EP.SERVERS, {
        params: { page: 1, page_size: 100 },
      })
      const list = normalizeList<ServerOption>(data)
      setServers(list)
      if (list.length > 0 && !selectedServerId) {
        setSelectedServerId(list[0].id)
      }
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '加载服务器列表失败'
      toast({ title: '加载失败', description: msg, variant: 'destructive' })
    } finally {
      setServersLoading(false)
    }
  }, [toast, selectedServerId])

  // 加载历史会话
  const fetchHistory = useCallback(async () => {
    setHistoryLoading(true)
    try {
      const data = await api.get<unknown>(EP.DIAGNOSIS_SESSIONS, {
        params: { page: 1, page_size: 20 },
      })
      setHistory(normalizeList<DiagnosisSession>(data))
    } catch (err) {
      // 历史加载失败不弹toast，保持安静
      setHistory([])
    } finally {
      setHistoryLoading(false)
    }
  }, [])

  // 加载知识库
  const fetchKnowledge = useCallback(async () => {
    setKnowledgeLoading(true)
    try {
      const data = await api.get<unknown>(EP.DIAGNOSIS_KNOWLEDGE, {
        params: { page: 1, page_size: 20 },
      })
      setKnowledge(normalizeList<KnowledgeEntry>(data))
    } catch (err) {
      setKnowledge([])
    } finally {
      setKnowledgeLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchServers()
    fetchHistory()
    fetchKnowledge()
  }, [fetchServers, fetchHistory, fetchKnowledge])

  // 轮询会话状态
  useEffect(() => {
    if (!currentSession) return
    const status = currentSession.status
    if (status === 'done' || status === 'failed') {
      setPolling(false)
      return
    }
    setPolling(true)
    const timer = setInterval(async () => {
      try {
        const data = await api.get<DiagnosisSession>(EP.DIAGNOSIS_SESSION_DETAIL(currentSession.id))
        setCurrentSession(data)
        if (data.status === 'done' || data.status === 'failed') {
          setPolling(false)
          // 刷新历史
          fetchHistory()
          if (data.status === 'done') {
            toast({ title: '诊断完成', description: `根因: ${getCategoryConfig(data.root_cause_category).label}` })
          } else {
            toast({ title: '诊断失败', variant: 'destructive' })
          }
        }
      } catch (err) {
        setPolling(false)
        const msg = err instanceof ApiError ? err.message : '查询诊断状态失败'
        toast({ title: '轮询失败', description: msg, variant: 'destructive' })
      }
    }, 3000)
    return () => clearInterval(timer)
  }, [currentSession, toast, fetchHistory])

  // 创建诊断会话
  const handleAnalyze = async () => {
    if (!selectedServerId) {
      toast({ title: '请先选择服务器', variant: 'destructive' })
      return
    }
    setCreating(true)
    setCurrentSession(null)
    try {
      const session = await api.post<DiagnosisSession>(EP.DIAGNOSIS_SESSIONS, {
        server_id: selectedServerId,
        time_window_mins: timeWindowMins,
      })
      setCurrentSession(session)
      toast({
        title: '已创建诊断会话',
        description: '正在异步采集日志并调用 LLM 分析',
      })
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '创建诊断会话失败'
      toast({ title: '诊断失败', description: msg, variant: 'destructive' })
    } finally {
      setCreating(false)
    }
  }

  // 应用自动修复
  const handleAutofix = async (suggestionIndex: number) => {
    if (!currentSession) return
    setApplyingAutofix(true)
    try {
      const updated = await api.post<DiagnosisSession>(EP.DIAGNOSIS_AUTOFIX(currentSession.id), {
        suggestion_index: suggestionIndex,
      })
      setCurrentSession(updated)
      toast({ title: '已应用自动修复', description: '请检查节点状态以验证效果' })
      fetchHistory()
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '应用自动修复失败'
      toast({ title: '操作失败', description: msg, variant: 'destructive' })
    } finally {
      setApplyingAutofix(false)
    }
  }

  // 选中历史会话
  const handleSelectHistory = async (sessionId: string) => {
    try {
      const data = await api.get<DiagnosisSession>(EP.DIAGNOSIS_SESSION_DETAIL(sessionId))
      setCurrentSession(data)
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '加载会话详情失败'
      toast({ title: '加载失败', description: msg, variant: 'destructive' })
    }
  }

  const isAnalyzing = creating || polling || (currentSession != null && ['pending', 'collecting', 'analyzing'].includes(currentSession.status))
  const selectedServer = servers.find((s) => s.id === selectedServerId)

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between">
        <div>
          <h1 className="text-2xl font-bold text-zinc-100 flex items-center gap-2">
            <Zap className="w-7 h-7 text-violet-400" />
            AI 智能诊断
          </h1>
          <p className="text-zinc-400 mt-1">
            选择目标服务器，AI 自动采集日志并调用 GLM 进行根因分析，给出修复建议
          </p>
        </div>
      </div>

      {/* 选择服务器 + 创建诊断 */}
      <Card className="bg-zinc-900 border-zinc-800">
        <CardHeader className="pb-3">
          <CardTitle className="text-base flex items-center gap-2">
            <Server className="w-4 h-4 text-indigo-400" />
            选择诊断目标
          </CardTitle>
          <CardDescription className="text-zinc-500">
            诊断将采集最近 {timeWindowMins} 分钟的日志和指标
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
            <div>
              <label className="text-xs text-zinc-500 mb-1 block">服务器</label>
              {serversLoading ? (
                <Skeleton className="h-9 w-full" />
              ) : (
                <Select
                  value={selectedServerId}
                  onChange={(e) => setSelectedServerId(e.target.value)}
                  disabled={isAnalyzing}
                >
                  {servers.length === 0 ? (
                    <option value="">暂无服务器</option>
                  ) : (
                    servers.map((s) => (
                      <option key={s.id} value={s.id}>
                        {s.name} ({s.code})
                      </option>
                    ))
                  )}
                </Select>
              )}
            </div>
            <div>
              <label className="text-xs text-zinc-500 mb-1 block">时间窗口（分钟）</label>
              <Select
                value={String(timeWindowMins)}
                onChange={(e) => setTimeWindowMins(Number(e.target.value))}
                disabled={isAnalyzing}
              >
                <option value="15">15 分钟</option>
                <option value="30">30 分钟</option>
                <option value="60">60 分钟</option>
                <option value="120">2 小时</option>
              </Select>
            </div>
            <div className="flex items-end">
              <Button
                size="lg"
                className="bg-violet-600 hover:bg-violet-500 text-white w-full"
                onClick={handleAnalyze}
                disabled={isAnalyzing || !selectedServerId}
                isLoading={creating}
              >
                {!creating && <Zap className="w-5 h-5 mr-2" />}
                {creating ? '创建中...' : polling ? 'AI 分析中...' : '一键智能诊断'}
              </Button>
            </div>
          </div>
          {selectedServer && (
            <div className="mt-3 text-xs text-zinc-500">
              目标: <span className="text-zinc-300">{selectedServer.name}</span>
              {selectedServer.host && <> · {selectedServer.host}</>}
            </div>
          )}
        </CardContent>
      </Card>

      {/* 当前诊断结果 */}
      {currentSession && (
        <Card className="bg-zinc-900 border-zinc-800 border-violet-500/30">
          <CardHeader className="pb-3">
            <CardTitle className="text-base flex items-center gap-2 flex-wrap">
              {(() => {
                const cfg = getCategoryConfig(currentSession.root_cause_category)
                const Icon = cfg.Icon
                return <Icon className={`w-5 h-5 ${cfg.color}`} />
              })()}
              诊断结果
              {currentSession.root_cause_category && (
                <Badge variant={getCategoryConfig(currentSession.root_cause_category).variant}>
                  {getCategoryConfig(currentSession.root_cause_category).label}
                </Badge>
              )}
              {getStatusBadge(currentSession.status)}
              {currentSession.confidence != null && (
                <Badge variant="outline" className="bg-zinc-800 text-zinc-400 border-zinc-700">
                  置信度 {(currentSession.confidence * 100).toFixed(0)}%
                </Badge>
              )}
              {currentSession.llm_model && (
                <Badge variant="outline" className="bg-zinc-800 text-zinc-500 border-zinc-700">
                  {currentSession.llm_provider}/{currentSession.llm_model}
                </Badge>
              )}
            </CardTitle>
            <CardDescription className="text-zinc-500">
              创建于 {formatTime(currentSession.created_at)}
              {currentSession.duration_ms != null && ` · 耗时 ${currentSession.duration_ms}ms`}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-5">
            {['pending', 'collecting', 'analyzing'].includes(currentSession.status) && (
              <div className="flex items-center justify-center py-8 space-x-3">
                <Loader2 className="w-6 h-6 text-violet-400 animate-spin" />
                <div className="text-zinc-300">
                  {currentSession.status === 'pending' && '排队中，等待采集器启动...'}
                  {currentSession.status === 'collecting' && '正在采集 node-agent 日志和指标...'}
                  {currentSession.status === 'analyzing' && '正在调用 LLM 进行根因分析...'}
                </div>
              </div>
            )}

            {currentSession.status === 'failed' && (
              <div className="flex items-center gap-2 py-4 text-red-400">
                <XCircle className="w-5 h-5" />
                <span>诊断失败，请稍后重试或检查 LLM 配置</span>
              </div>
            )}

            {currentSession.status === 'done' && (
              <>
                {currentSession.root_cause_description && (
                  <div>
                    <h4 className="text-sm font-medium text-zinc-300 mb-2">问题描述</h4>
                    <p className="text-sm text-zinc-400 leading-relaxed bg-zinc-800/50 rounded-lg p-4 whitespace-pre-wrap">
                      {currentSession.root_cause_description}
                    </p>
                  </div>
                )}

                {currentSession.raw_logs && (
                  <div>
                    <h4 className="text-sm font-medium text-zinc-300 mb-2 flex items-center gap-2">
                      <FileText className="w-4 h-4 text-emerald-400" />
                      采集的日志（节选）
                    </h4>
                    <Textarea
                      readOnly
                      value={currentSession.raw_logs.split('\n').slice(-50).join('\n')}
                      className="font-mono text-xs bg-zinc-950 border-zinc-800 text-zinc-300 h-48 resize-none"
                    />
                  </div>
                )}

                {currentSession.suggestions && currentSession.suggestions.length > 0 && (
                  <div>
                    <h4 className="text-sm font-medium text-zinc-300 mb-2 flex items-center gap-2">
                      <Wrench className="w-4 h-4 text-amber-400" />
                      修复建议
                    </h4>
                    <ol className="space-y-2">
                      {currentSession.suggestions.map((s, idx) => (
                        <li key={idx} className="bg-zinc-800/30 border border-zinc-800 rounded-lg p-3">
                          <div className="flex items-start justify-between gap-2 flex-wrap">
                            <div className="flex-1 min-w-0">
                              <div className="flex items-center gap-2">
                                <span className="text-xs text-zinc-500 font-mono">#{idx + 1}</span>
                                <span className="text-sm font-medium text-zinc-200">{s.title}</span>
                                {s.auto_fixable && (
                                  <Badge variant="outline" className="bg-emerald-900/30 text-emerald-300 border-emerald-800/50 text-xs">
                                    可自动修复
                                  </Badge>
                                )}
                              </div>
                              <p className="text-xs text-zinc-400 mt-1 leading-relaxed">{s.description}</p>
                            </div>
                            {s.auto_fixable && (
                              <Button
                                size="sm"
                                variant="secondary"
                                onClick={() => handleAutofix(idx)}
                                disabled={applyingAutofix || currentSession.autofix_applied}
                                className="bg-emerald-900/30 hover:bg-emerald-900/50 text-emerald-300 border-emerald-800/50"
                              >
                                <Wrench className="w-3 h-3 mr-1" />
                                {currentSession.autofix_applied ? '已应用' : '应用修复'}
                              </Button>
                            )}
                          </div>
                        </li>
                      ))}
                    </ol>
                  </div>
                )}

                {currentSession.doc_links && currentSession.doc_links.length > 0 && (
                  <div>
                    <h4 className="text-sm font-medium text-zinc-300 mb-2 flex items-center gap-2">
                      <ExternalLink className="w-4 h-4 text-indigo-400" />
                      相关文档
                    </h4>
                    <div className="flex flex-wrap gap-2">
                      {currentSession.doc_links.map((link, idx) => (
                        <a
                          key={idx}
                          href={link.url}
                          target="_blank"
                          rel="noreferrer"
                          className="inline-flex items-center gap-1 text-sm text-indigo-400 hover:text-indigo-300 bg-indigo-950/30 border border-indigo-800/30 rounded-lg px-3 py-1.5 transition-colors"
                        >
                          {link.title}
                          <ExternalLink className="w-3 h-3" />
                        </a>
                      ))}
                    </div>
                  </div>
                )}

                {currentSession.autofix_applied && currentSession.autofix_result && (
                  <div>
                    <h4 className="text-sm font-medium text-zinc-300 mb-2 flex items-center gap-2">
                      <CheckCircle className="w-4 h-4 text-emerald-400" />
                      自动修复结果
                    </h4>
                    <pre className="text-xs bg-zinc-950 border border-zinc-800 rounded-lg p-3 text-zinc-300 overflow-x-auto">
                      {JSON.stringify(currentSession.autofix_result, null, 2)}
                    </pre>
                  </div>
                )}
              </>
            )}
          </CardContent>
        </Card>
      )}

      {/* 历史诊断记录 */}
      <Card className="bg-zinc-900 border-zinc-800">
        <CardHeader className="pb-3">
          <CardTitle className="text-base flex items-center gap-2">
            <Clock className="w-4 h-4 text-zinc-400" />
            历史诊断记录
            <Button
              variant="ghost"
              size="sm"
              onClick={fetchHistory}
              className="ml-auto text-zinc-400 hover:text-zinc-200"
            >
              <RefreshCw className="w-3.5 h-3.5" />
            </Button>
          </CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {historyLoading ? (
            <div className="p-4 space-y-2">
              {[1, 2, 3, 4].map((i) => (
                <Skeleton key={i} className="h-10 w-full" />
              ))}
            </div>
          ) : history.length === 0 ? (
            <EmptyState
              icon={<Clock className="w-8 h-8 text-zinc-600" />}
              title="暂无历史记录"
              description="完成首次诊断后将在此显示历史记录"
            />
          ) : (
            <Table>
              <TableHeader>
                <TableRow className="border-zinc-800 hover:bg-transparent">
                  <TableHead>时间</TableHead>
                  <TableHead>状态</TableHead>
                  <TableHead>根因分类</TableHead>
                  <TableHead>描述</TableHead>
                  <TableHead>自动修复</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {history.map((record) => {
                  const cfg = getCategoryConfig(record.root_cause_category)
                  return (
                    <TableRow
                      key={record.id}
                      className="border-zinc-800 cursor-pointer hover:bg-zinc-800/30"
                      onClick={() => handleSelectHistory(record.id)}
                    >
                      <TableCell className="text-zinc-400 text-xs font-mono">{formatTime(record.created_at)}</TableCell>
                      <TableCell>{getStatusBadge(record.status)}</TableCell>
                      <TableCell>
                        {record.root_cause_category ? (
                          <Badge variant={cfg.variant} className={cfg.className}>
                            {cfg.label}
                          </Badge>
                        ) : (
                          <span className="text-zinc-600 text-xs">-</span>
                        )}
                      </TableCell>
                      <TableCell className="text-zinc-400 text-sm max-w-md truncate">
                        {record.root_cause_description || '-'}
                      </TableCell>
                      <TableCell>
                        {record.autofix_applied ? (
                          <Badge variant="success" className="bg-emerald-900/50 text-emerald-300">
                            <CheckCircle className="w-3 h-3 mr-1" />
                            已应用
                          </Badge>
                        ) : (
                          <span className="text-zinc-600 text-xs">-</span>
                        )}
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      {/* 知识库 */}
      <Card className="bg-zinc-900 border-zinc-800">
        <CardHeader className="pb-3">
          <CardTitle className="text-base flex items-center gap-2">
            <BookOpen className="w-4 h-4 text-sky-400" />
            诊断知识库
            <Badge variant="secondary" className="ml-1 bg-zinc-800 text-zinc-400 text-xs">
              {knowledge.length} 条
            </Badge>
            <Button
              variant="ghost"
              size="sm"
              onClick={fetchKnowledge}
              className="ml-auto text-zinc-400 hover:text-zinc-200"
            >
              <RefreshCw className="w-3.5 h-3.5" />
            </Button>
          </CardTitle>
        </CardHeader>
        <CardContent>
          {knowledgeLoading ? (
            <div className="space-y-2">
              {[1, 2, 3].map((i) => (
                <Skeleton key={i} className="h-16 w-full" />
              ))}
            </div>
          ) : knowledge.length === 0 ? (
            <div className="p-6 text-center text-sm text-zinc-500">
              暂无知识库条目，诊断产生的新知识会自动入库
            </div>
          ) : (
            <div className="space-y-3">
              {knowledge.map((entry) => (
                <div key={entry.id} className="border border-zinc-800 rounded-lg p-3 bg-zinc-950/30">
                  <div className="flex items-center justify-between gap-2 flex-wrap">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium text-zinc-200">{entry.title}</span>
                      <Badge variant="outline" className="bg-zinc-800 text-zinc-400 border-zinc-700 text-xs">
                        {entry.category}
                      </Badge>
                      {entry.is_verified && (
                        <Badge variant="success" className="bg-emerald-900/50 text-emerald-300 text-xs">
                          已验证
                        </Badge>
                      )}
                    </div>
                    <span className="text-xs text-zinc-500">命中 {entry.hit_count} 次</span>
                  </div>
                  <p className="text-xs text-zinc-400 mt-2">
                    <span className="text-zinc-500">模式:</span> {entry.root_cause_pattern}
                  </p>
                  <p className="text-xs text-zinc-400 mt-1">
                    <span className="text-zinc-500">方案:</span> {entry.solution}
                  </p>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
