import { useState, useEffect, useRef } from 'react'
import {
  UploadCloud,
  FileText,
  ClipboardPaste,
  CheckCircle2,
  XCircle,
  Loader2,
  RefreshCw,
  Eye,
  FileCode,
} from 'lucide-react'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Button,
  Badge,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  Skeleton,
  EmptyState,
  Textarea,
  Select,
  Separator,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  useToast,
} from '@airport/ui'
import { api, ApiError } from '@/lib/api'
import { EP } from '@/lib/endpoints'

// ===== 类型定义 =====
type ConfigType = 'xray' | 'sing-box' | 'nginx' | 'cloudflared'
type ImportJobStatus = 'pending' | 'parsing' | 'confirmed' | 'completed' | 'failed'

interface ParsedConfig {
  protocol?: string
  port?: number
  sni?: string
  ws_path?: string
  cert_source?: string
  exposure?: string
  raw?: Record<string, unknown>
  [key: string]: unknown
}

interface ImportJob {
  id: string
  config_type: ConfigType
  status: ImportJobStatus
  source?: string
  parsed?: ParsedConfig
  parse_error?: string
  result_message?: string
  created_at?: string
  updated_at?: string
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

const configTypeLabel: Record<ConfigType, string> = {
  xray: 'Xray Config',
  'sing-box': 'sing-box Config',
  nginx: 'Nginx Conf',
  cloudflared: 'Cloudflared YML',
}

const jobStatusConfig: Record<
  ImportJobStatus,
  { label: string; variant: 'secondary' | 'warning' | 'success' | 'destructive' }
> = {
  pending: { label: '待处理', variant: 'secondary' },
  parsing: { label: '解析中', variant: 'warning' },
  confirmed: { label: '已确认', variant: 'warning' },
  completed: { label: '已完成', variant: 'success' },
  failed: { label: '失败', variant: 'destructive' },
}

function getJobStatusBadge(status: ImportJobStatus) {
  const cfg = jobStatusConfig[status] || jobStatusConfig.pending
  return <Badge variant={cfg.variant}>{cfg.label}</Badge>
}

// 解析预览字段
const parsedFields: Array<{ key: string; label: string }> = [
  { key: 'protocol', label: '协议类型' },
  { key: 'port', label: '端口' },
  { key: 'sni', label: 'SNI' },
  { key: 'ws_path', label: 'WS 路径' },
  { key: 'cert_source', label: '证书来源' },
  { key: 'exposure', label: '暴露方式' },
]

// ===== 主组件 =====
export default function ConfigImporter() {
  const { toast } = useToast()

  const [configType, setConfigType] = useState<ConfigType>('xray')
  const [inputMode, setInputMode] = useState<'upload' | 'paste'>('upload')
  const [pasteText, setPasteText] = useState('')
  const [fileName, setFileName] = useState('')
  const [fileContent, setFileContent] = useState('')
  const [dragOver, setDragOver] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  const [jobs, setJobs] = useState<ImportJob[]>([])
  const [jobsLoading, setJobsLoading] = useState(true)
  const [confirmingId, setConfirmingId] = useState<string | null>(null)
  const [detailJob, setDetailJob] = useState<ImportJob | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)

  const fileInputRef = useRef<HTMLInputElement>(null)

  const loadJobs = async () => {
    setJobsLoading(true)
    try {
      // 后端列表用复数 config-imports，创建用单数 config-import
      const data = await api.get(EP.CONFIG_IMPORTS)
      setJobs(normalizeList<ImportJob>(data))
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '加载导入任务失败'
      toast({ title: '加载失败', description: msg, variant: 'destructive' })
    } finally {
      setJobsLoading(false)
    }
  }

  useEffect(() => {
    loadJobs()
  }, [])

  const handleFile = (file: File) => {
    setFileName(file.name)
    const reader = new FileReader()
    reader.onload = () => {
      setFileContent(String(reader.result || ''))
    }
    reader.readAsText(file)
  }

  const onDrop = (e: React.DragEvent) => {
    e.preventDefault()
    setDragOver(false)
    const file = e.dataTransfer.files?.[0]
    if (file) handleFile(file)
  }

  const submitImport = async () => {
    const content = inputMode === 'paste' ? pasteText : fileContent
    if (!content.trim()) {
      toast({ title: '请提供配置内容', description: '上传文件或粘贴配置文本', variant: 'destructive' })
      return
    }
    setSubmitting(true)
    try {
      const payload: Record<string, unknown> = {
        config_type: configType,
        content,
      }
      if (inputMode === 'upload' && fileName) payload.source = fileName
      await api.post(EP.CONFIG_IMPORT_CREATE, payload)
      toast({ title: '已创建导入任务', description: '配置已提交解析', variant: 'success' })
      setPasteText('')
      setFileName('')
      setFileContent('')
      loadJobs()
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '提交失败'
      toast({ title: '导入失败', description: msg, variant: 'destructive' })
    } finally {
      setSubmitting(false)
    }
  }

  const confirmImport = async (job: ImportJob) => {
    setConfirmingId(job.id)
    try {
      // 后端路由为 /config-import/:id/apply（非 confirm）
      await api.post(EP.CONFIG_IMPORT_APPLY(job.id))
      toast({ title: '已确认导入', description: '配置正在应用', variant: 'success' })
      loadJobs()
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '确认失败'
      toast({ title: '确认失败', description: msg, variant: 'destructive' })
    } finally {
      setConfirmingId(null)
    }
  }

  const viewDetail = async (job: ImportJob) => {
    setDetailJob(job)
    setDetailOpen(true)
    try {
      const data = await api.get(EP.CONFIG_IMPORT_DETAIL(job.id))
      if (data && typeof data === 'object') setDetailJob(data as ImportJob)
    } catch (err) {
      // keep cached
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-zinc-100 flex items-center gap-2">
          <FileCode className="w-5 h-5 text-indigo-400" />
          配置导入器
        </h2>
        <Button size="sm" variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={loadJobs}>
          <RefreshCw className="w-3.5 h-3.5 mr-1.5" />
          刷新
        </Button>
      </div>

      {/* 上传区 */}
      <Card className="bg-zinc-900 border-zinc-800">
        <CardHeader className="pb-3">
          <CardTitle className="text-base">导入配置</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {/* 类型 + 模式 */}
          <div className="flex flex-col sm:flex-row gap-3">
            <div className="space-y-1.5 flex-1">
              <label className="text-xs text-zinc-400">配置类型</label>
              <Select
                value={configType}
                onChange={(e) => setConfigType(e.target.value as ConfigType)}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              >
                <option value="xray">Xray Config</option>
                <option value="sing-box">sing-box Config</option>
                <option value="nginx">Nginx Conf</option>
                <option value="cloudflared">Cloudflared YML</option>
              </Select>
            </div>
            <div className="space-y-1.5 flex-1">
              <label className="text-xs text-zinc-400">输入方式</label>
              <div className="flex gap-1 rounded-lg bg-zinc-800/50 p-1">
                <button
                  className={`flex-1 inline-flex items-center justify-center px-3 py-1.5 rounded-md text-sm transition-all ${inputMode === 'upload' ? 'bg-zinc-900 text-zinc-100 shadow-sm' : 'text-zinc-400 hover:text-zinc-100'}`}
                  onClick={() => setInputMode('upload')}
                >
                  <UploadCloud className="w-3.5 h-3.5 mr-1.5" />
                  上传文件
                </button>
                <button
                  className={`flex-1 inline-flex items-center justify-center px-3 py-1.5 rounded-md text-sm transition-all ${inputMode === 'paste' ? 'bg-zinc-900 text-zinc-100 shadow-sm' : 'text-zinc-400 hover:text-zinc-100'}`}
                  onClick={() => setInputMode('paste')}
                >
                  <ClipboardPaste className="w-3.5 h-3.5 mr-1.5" />
                  粘贴文本
                </button>
              </div>
            </div>
          </div>

          {inputMode === 'upload' ? (
            <div
              className={`rounded-lg border-2 border-dashed p-6 text-center transition-colors cursor-pointer ${dragOver ? 'border-indigo-500 bg-indigo-950/20' : 'border-zinc-700 bg-zinc-950/30 hover:border-zinc-600'}`}
              onClick={() => fileInputRef.current?.click()}
              onDragOver={(e) => {
                e.preventDefault()
                setDragOver(true)
              }}
              onDragLeave={() => setDragOver(false)}
              onDrop={onDrop}
            >
              <UploadCloud className="w-8 h-8 mx-auto text-zinc-500" />
              <p className="text-sm text-zinc-300 mt-2">点击或拖拽文件到此处</p>
              <p className="text-xs text-zinc-500 mt-1">支持 .json / .yaml / .conf / .yml 配置文件</p>
              {fileName && (
                <div className="mt-3 inline-flex items-center gap-1.5 px-2 py-1 rounded-md bg-zinc-800 border border-zinc-700">
                  <FileText className="w-3.5 h-3.5 text-indigo-400" />
                  <span className="text-xs text-zinc-300">{fileName}</span>
                </div>
              )}
              <input
                ref={fileInputRef}
                type="file"
                className="hidden"
                onChange={(e) => {
                  const f = e.target.files?.[0]
                  if (f) handleFile(f)
                }}
              />
            </div>
          ) : (
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400">配置文本</label>
              <Textarea
                rows={8}
                placeholder="粘贴配置内容..."
                value={pasteText}
                onChange={(e) => setPasteText(e.target.value)}
                className="bg-zinc-950/40 border-zinc-700 text-zinc-100 font-mono text-xs"
              />
            </div>
          )}

          <div className="flex justify-end">
            <Button className="bg-indigo-600 hover:bg-indigo-500" onClick={submitImport} disabled={submitting}>
              {submitting ? (
                <>
                  <Loader2 className="w-4 h-4 mr-1.5 animate-spin" />
                  提交中...
                </>
              ) : (
                <>
                  <UploadCloud className="w-4 h-4 mr-1.5" />
                  提交解析
                </>
              )}
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* 任务历史 */}
      <Card className="bg-zinc-900 border-zinc-800 overflow-hidden">
        <CardHeader className="pb-3">
          <CardTitle className="text-base">导入任务历史</CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {jobsLoading ? (
            <div className="p-4 space-y-3">
              {[1, 2, 3].map((i) => (
                <Skeleton key={i} className="h-16 w-full bg-zinc-800 rounded-lg" />
              ))}
            </div>
          ) : jobs.length === 0 ? (
            <EmptyState title="暂无导入任务" description="提交配置后将显示在此" className="py-12" />
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="border-zinc-800 hover:bg-transparent">
                    <TableHead className="text-zinc-400 text-xs font-medium">来源</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium hidden sm:table-cell">类型</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium">状态</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium hidden md:table-cell">解析结果</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium hidden lg:table-cell">时间</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium w-24"></TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {jobs.map((job) => (
                    <TableRow key={job.id} className="border-zinc-800 hover:bg-zinc-800/50">
                      <TableCell className="py-3">
                        <div className="font-medium text-zinc-200 text-sm truncate max-w-[160px]">
                          {job.source || '粘贴文本'}
                        </div>
                        <div className="text-xs text-zinc-500 sm:hidden mt-0.5">
                          {configTypeLabel[job.config_type] || job.config_type}
                        </div>
                      </TableCell>
                      <TableCell className="py-3 hidden sm:table-cell">
                        <Badge variant="secondary" className="bg-zinc-800 text-zinc-300 text-xs">
                          {configTypeLabel[job.config_type] || job.config_type}
                        </Badge>
                      </TableCell>
                      <TableCell className="py-3">{getJobStatusBadge(job.status)}</TableCell>
                      <TableCell className="py-3 hidden md:table-cell text-sm text-zinc-400 max-w-[200px] truncate">
                        {job.parse_error ||
                          job.result_message ||
                          (job.parsed?.protocol ? `协议: ${job.parsed.protocol}` : '-')}
                      </TableCell>
                      <TableCell className="py-3 hidden lg:table-cell text-sm text-zinc-500">
                        {formatTime(job.created_at)}
                      </TableCell>
                      <TableCell className="py-3">
                        <div className="flex items-center gap-1">
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-8 w-8 text-zinc-400 hover:text-zinc-200"
                            onClick={() => viewDetail(job)}
                            title="详情"
                          >
                            <Eye className="w-4 h-4" />
                          </Button>
                          {(job.status === 'parsing' || job.status === 'confirmed' || job.status === 'pending') && (
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-8 w-8 text-zinc-400 hover:text-emerald-400"
                              onClick={() => confirmImport(job)}
                              disabled={confirmingId === job.id}
                              title="确认导入"
                            >
                              <CheckCircle2 className={`w-4 h-4 ${confirmingId === job.id ? 'animate-spin' : ''}`} />
                            </Button>
                          )}
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>

      {/* 详情 Dialog */}
      <Dialog open={detailOpen} onOpenChange={setDetailOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-lg">
          {detailJob && (
            <>
              <DialogHeader>
                <DialogTitle className="flex items-center gap-2">
                  导入任务详情
                  {getJobStatusBadge(detailJob.status)}
                </DialogTitle>
                <DialogDescription>
                  {detailJob.source || '粘贴文本'} · {configTypeLabel[detailJob.config_type]}
                </DialogDescription>
              </DialogHeader>
              <div className="space-y-3 pt-2">
                {detailJob.parse_error && (
                  <div className="rounded-md border border-red-900/50 bg-red-950/30 p-3">
                    <div className="text-xs text-red-300 flex items-center gap-1.5">
                      <XCircle className="w-3.5 h-3.5" />
                      解析错误
                    </div>
                    <p className="text-xs text-red-200 mt-1 break-all">{detailJob.parse_error}</p>
                  </div>
                )}
                {detailJob.parsed ? (
                  <div>
                    <div className="text-xs text-zinc-500 mb-2">解析预览</div>
                    <div className="grid grid-cols-2 gap-2">
                      {parsedFields.map((f) => {
                        const val = (detailJob.parsed as Record<string, unknown>)?.[f.key]
                        return (
                          <div key={f.key} className="rounded-md border border-zinc-800 bg-zinc-950/40 p-2">
                            <div className="text-[10px] text-zinc-500">{f.label}</div>
                            <div className="text-sm text-zinc-200 mt-0.5 truncate">
                              {val !== undefined && val !== '' ? String(val) : '-'}
                            </div>
                          </div>
                        )
                      })}
                    </div>
                  </div>
                ) : (
                  <p className="text-xs text-zinc-500">暂无解析结果</p>
                )}
                {detailJob.result_message && (
                  <>
                    <Separator className="bg-zinc-800" />
                    <div>
                      <div className="text-xs text-zinc-500 mb-1">结果消息</div>
                      <p className="text-sm text-zinc-300">{detailJob.result_message}</p>
                    </div>
                  </>
                )}
                <Separator className="bg-zinc-800" />
                <div className="grid grid-cols-2 gap-2 text-xs">
                  <div>
                    <div className="text-zinc-500">创建时间</div>
                    <div className="text-zinc-300 mt-0.5">{formatTime(detailJob.created_at)}</div>
                  </div>
                  <div>
                    <div className="text-zinc-500">更新时间</div>
                    <div className="text-zinc-300 mt-0.5">{formatTime(detailJob.updated_at)}</div>
                  </div>
                </div>
              </div>
              <DialogFooter>
                {(detailJob.status === 'parsing' ||
                  detailJob.status === 'confirmed' ||
                  detailJob.status === 'pending') && (
                  <Button
                    className="bg-indigo-600 hover:bg-indigo-500"
                    disabled={confirmingId === detailJob.id}
                    onClick={() => confirmImport(detailJob)}
                  >
                    <CheckCircle2
                      className={`w-4 h-4 mr-1.5 ${confirmingId === detailJob.id ? 'animate-spin' : ''}`}
                    />
                    {confirmingId === detailJob.id ? '确认中...' : '确认导入'}
                  </Button>
                )}
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}
