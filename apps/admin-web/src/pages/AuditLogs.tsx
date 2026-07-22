import { useState, useEffect } from 'react'
import { ScrollText, Server, HardDrive, Cpu, Clock, Activity, Search, ChevronLeft, ChevronRight, ListTodo } from 'lucide-react'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Button,
  Input,
  Label,
  Badge,
  Select,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  Skeleton,
  EmptyState,
} from '@airport/ui'
import { api, xbAdminApi } from '@/lib/api'
import { EP } from '@/lib/endpoints'

interface AuditLog {
  id: number
  admin_id?: number
  admin_email?: string
  action: string
  resource_type?: string
  resource_id?: string
  ip?: string
  user_agent?: string
  details?: unknown
  created_at: string
}

interface SystemStatus {
  uptime?: number | string
  load?: number[] | string
  load_avg?: number[] | string
  memory?: {
    total?: number
    used?: number
    free?: number
    usage_percent?: number
  }
  mem?: {
    total?: number
    used?: number
    free?: number
  }
  disk?: {
    total?: number
    used?: number
    free?: number
    usage_percent?: number
  }
  cpu_usage?: number
  [key: string]: unknown
}

interface QueueStats {
  pending?: number
  processing?: number
  failed?: number
  total?: number
  [key: string]: unknown
}

interface LogListResponse {
  items: AuditLog[]
  total: number
  page: number
  page_size: number
}

function extractList<T>(resp: unknown): T[] {
  if (Array.isArray(resp)) return resp as T[]
  if (!resp || typeof resp !== 'object') return []
  const obj = resp as Record<string, unknown>
  const dataField = obj.data
  if (dataField && typeof dataField === 'object') {
    if (Array.isArray(dataField)) return dataField as T[]
    const dataObj = dataField as Record<string, unknown>
    if (Array.isArray(dataObj.data)) return dataObj.data as T[]
    if (Array.isArray(dataObj.items)) return dataObj.items as T[]
    if (Array.isArray(dataObj.logs)) return dataObj.logs as T[]
    if (Array.isArray(dataObj.list)) return dataObj.list as T[]
  }
  if (Array.isArray(obj.data)) return obj.data as T[]
  if (Array.isArray(obj.logs)) return obj.logs as T[]
  if (Array.isArray(obj.items)) return obj.items as T[]
  return []
}

function extractObject<T>(resp: unknown): T | null {
  if (!resp || typeof resp !== 'object') return null
  const obj = resp as Record<string, unknown>
  const dataField = obj.data
  if (dataField && typeof dataField === 'object' && !Array.isArray(dataField)) {
    return dataField as T
  }
  return obj as T
}

function formatDate(ts?: number | string): string {
  if (!ts) return '-'
  const d = typeof ts === 'number' ? (ts > 1e12 ? new Date(ts) : new Date(ts * 1000)) : new Date(ts)
  if (isNaN(d.getTime())) return String(ts)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

function formatBytes(bytes?: number): string {
  if (!bytes || bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`
}

function formatUptime(up?: number | string): string {
  if (!up) return '-'
  if (typeof up === 'string') return up
  const seconds = typeof up === 'number' && up > 1e9 ? Math.floor(up / 1000) : up
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (d > 0) return `${d}天 ${h}小时`
  if (h > 0) return `${h}小时 ${m}分`
  return `${m}分钟`
}

function getActionBadge(action: string) {
  const a = action.toLowerCase()
  if (a.includes('login') || a.includes('登录') || a.includes('create') || a.includes('创建')) {
    return <Badge variant="success" className="text-xs">{action}</Badge>
  }
  if (a.includes('delete') || a.includes('删除') || a.includes('disable') || a.includes('停用')) {
    return <Badge variant="destructive" className="text-xs">{action}</Badge>
  }
  if (a.includes('restart') || a.includes('重启') || a.includes('update') || a.includes('更新') || a.includes('edit') || a.includes('修改') || a.includes('save') || a.includes('保存')) {
    return <Badge variant="warning" className="text-xs">{action}</Badge>
  }
  return <Badge variant="secondary" className="bg-zinc-800 text-zinc-300 text-xs">{action || '-'}</Badge>
}

const ACTION_FILTERS = [
  { v: 'all', l: '全部操作' },
  { v: 'login', l: '登录' },
  { v: 'create', l: '创建' },
  { v: 'update', l: '更新' },
  { v: 'delete', l: '删除' },
]

export default function AuditLogs() {
  const [loading, setLoading] = useState(true)
  const [logs, setLogs] = useState<AuditLog[]>([])
  const [page, setPage] = useState(1)
  const [pageSize] = useState(15)
  const [total, setTotal] = useState(0)
  const [search, setSearch] = useState('')
  const [actionFilter, setActionFilter] = useState('all')
  const [dateFrom, setDateFrom] = useState('')
  const [dateTo, setDateTo] = useState('')

  const [sysStatus, setSysStatus] = useState<SystemStatus | null>(null)
  const [queueStats, setQueueStats] = useState<QueueStats | null>(null)
  const [statusLoading, setStatusLoading] = useState(true)

  useEffect(() => {
    loadLogs()
    loadSystemStatus()
  }, [page])

  const loadLogs = async () => {
    setLoading(true)
    try {
      const params: Record<string, string | number | boolean | undefined> = {
        page,
        page_size: pageSize,
      }
      if (actionFilter !== 'all') params.action = actionFilter
      const data = await api.get<LogListResponse>(EP.AUDIT_LOGS, { params })
      setLogs(data?.items || [])
      setTotal(data?.total || 0)
    } catch (err) {
      // silently fail for logs
    } finally {
      setLoading(false)
    }
  }

  const loadSystemStatus = async () => {
    setStatusLoading(true)
    try {
      const [statusResp, queueResp] = await Promise.all([
        xbAdminApi.get<unknown>('/system/getSystemStatus').catch(() => null),
        xbAdminApi.get<unknown>('/system/getQueueStats').catch(() => null),
      ])
      if (statusResp) {
        setSysStatus(extractObject<SystemStatus>(statusResp))
      }
      if (queueResp) {
        setQueueStats(extractObject<QueueStats>(queueResp))
      }
    } catch {
      // silent
    } finally {
      setStatusLoading(false)
    }
  }

  const handleSearch = () => {
    setPage(1)
    loadLogs()
  }

  const totalPages = Math.ceil(total / pageSize) || 1
  const mem = sysStatus?.memory || sysStatus?.mem
  const memUsed = mem?.used
  const memTotal = mem?.total
  const memPercent =
    (mem && 'usage_percent' in mem && typeof mem.usage_percent === 'number' ? mem.usage_percent : 0) ||
    (memUsed && memTotal ? Math.round((memUsed / memTotal) * 100) : 0)
  const diskPercent = sysStatus?.disk?.usage_percent ?? (sysStatus?.disk?.used && sysStatus?.disk?.total ? Math.round((sysStatus.disk.used / sysStatus.disk.total) * 100) : 0)
  const loadArr = sysStatus?.load || sysStatus?.load_avg
  const loadStr = Array.isArray(loadArr) ? loadArr.map((v: number) => typeof v === 'number' ? v.toFixed(2) : String(v)).join(', ') : (typeof loadArr === 'string' ? loadArr : '-')

  return (
    <div className="space-y-4">
      <h2 className="text-lg font-semibold text-zinc-100">系统日志</h2>

      <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
        <Card className="bg-zinc-900 border-zinc-800">
          <CardContent className="p-3">
            <div className="flex items-center gap-2 mb-2">
              <Clock className="w-4 h-4 text-indigo-400" />
              <span className="text-xs text-zinc-400">运行时间</span>
            </div>
            {statusLoading ? (
              <Skeleton className="h-6 w-24 bg-zinc-800 rounded" />
            ) : (
              <div className="text-lg font-semibold text-zinc-100">{formatUptime(sysStatus?.uptime as number)}</div>
            )}
          </CardContent>
        </Card>

        <Card className="bg-zinc-900 border-zinc-800">
          <CardContent className="p-3">
            <div className="flex items-center gap-2 mb-2">
              <Cpu className="w-4 h-4 text-amber-400" />
              <span className="text-xs text-zinc-400">系统负载</span>
            </div>
            {statusLoading ? (
              <Skeleton className="h-6 w-24 bg-zinc-800 rounded" />
            ) : (
              <div className="text-lg font-semibold text-zinc-100 font-mono text-sm">{loadStr}</div>
            )}
          </CardContent>
        </Card>

        <Card className="bg-zinc-900 border-zinc-800">
          <CardContent className="p-3">
            <div className="flex items-center gap-2 mb-2">
              <Server className="w-4 h-4 text-emerald-400" />
              <span className="text-xs text-zinc-400">内存使用</span>
            </div>
            {statusLoading ? (
              <Skeleton className="h-6 w-24 bg-zinc-800 rounded" />
            ) : (
              <div>
                <div className="text-lg font-semibold text-zinc-100">{memPercent}%</div>
                <div className="text-xs text-zinc-500">
                  {formatBytes(memUsed)} / {formatBytes(memTotal)}
                </div>
              </div>
            )}
          </CardContent>
        </Card>

        <Card className="bg-zinc-900 border-zinc-800">
          <CardContent className="p-3">
            <div className="flex items-center gap-2 mb-2">
              <HardDrive className="w-4 h-4 text-purple-400" />
              <span className="text-xs text-zinc-400">磁盘使用</span>
            </div>
            {statusLoading ? (
              <Skeleton className="h-6 w-24 bg-zinc-800 rounded" />
            ) : (
              <div>
                <div className="text-lg font-semibold text-zinc-100">{diskPercent}%</div>
                <div className="text-xs text-zinc-500">
                  {formatBytes(sysStatus?.disk?.used)} / {formatBytes(sysStatus?.disk?.total)}
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {queueStats && (
        <Card className="bg-zinc-900 border-zinc-800">
          <CardHeader className="pb-2 pt-3">
            <CardTitle className="text-sm flex items-center gap-2">
              <ListTodo className="w-4 h-4 text-indigo-400" />
              队列状态
            </CardTitle>
          </CardHeader>
          <CardContent className="pt-0 pb-3">
            <div className="flex flex-wrap gap-4">
              {(queueStats.pending !== undefined || queueStats.total !== undefined) && (
                <div className="flex items-center gap-2">
                  <Badge variant="warning">{queueStats.pending ?? 0}</Badge>
                  <span className="text-xs text-zinc-400">待处理</span>
                </div>
              )}
              {queueStats.processing !== undefined && (
                <div className="flex items-center gap-2">
                  <Badge variant="default">{queueStats.processing}</Badge>
                  <span className="text-xs text-zinc-400">处理中</span>
                </div>
              )}
              {queueStats.failed !== undefined && (
                <div className="flex items-center gap-2">
                  <Badge variant="destructive">{queueStats.failed}</Badge>
                  <span className="text-xs text-zinc-400">失败</span>
                </div>
              )}
              {queueStats.total !== undefined && (
                <div className="flex items-center gap-2">
                  <Badge variant="secondary" className="bg-zinc-800 text-zinc-300">{queueStats.total}</Badge>
                  <span className="text-xs text-zinc-400">总任务</span>
                </div>
              )}
            </div>
          </CardContent>
        </Card>
      )}

      <Card className="bg-zinc-900 border-zinc-800">
        <CardContent className="p-3">
          <div className="grid grid-cols-1 md:grid-cols-4 gap-2">
            <div className="relative">
              <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-zinc-500" />
              <Input
                placeholder="搜索操作/管理员/IP..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
                className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500 pl-9"
              />
            </div>
            <Select
              value={actionFilter}
              onChange={(e) => { setActionFilter(e.target.value); setPage(1) }}
              className="bg-zinc-800 border-zinc-700 text-zinc-100"
            >
              {ACTION_FILTERS.map((f) => (
                <option key={f.v} value={f.v}>{f.l}</option>
              ))}
            </Select>
            <Input
              type="date"
              value={dateFrom}
              onChange={(e) => setDateFrom(e.target.value)}
              className="bg-zinc-800 border-zinc-700 text-zinc-100"
            />
            <div className="flex gap-2">
              <Input
                type="date"
                value={dateTo}
                onChange={(e) => setDateTo(e.target.value)}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              />
              <Button
                size="sm"
                className="bg-indigo-600 hover:bg-indigo-500 shrink-0"
                onClick={handleSearch}
              >
                搜索
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card className="bg-zinc-900 border-zinc-800 overflow-hidden">
        <CardHeader className="pb-2">
          <CardTitle className="text-base flex items-center gap-2">
            <ScrollText className="w-4 h-4 text-indigo-400" />
            操作日志
          </CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {loading ? (
            <div className="p-4 space-y-3">
              {[1, 2, 3, 4, 5, 6].map((i) => (
                <Skeleton key={i} className="h-12 w-full bg-zinc-800 rounded-lg" />
              ))}
            </div>
          ) : logs.length === 0 ? (
            <EmptyState title="暂无日志" description="没有符合条件的操作日志" className="py-12" />
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="border-zinc-800 hover:bg-transparent">
                    <TableHead className="text-zinc-400 text-xs font-medium w-12">ID</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium">管理员</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium">操作</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium hidden md:table-cell">IP</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium hidden md:table-cell">时间</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {logs.map((l) => (
                    <TableRow key={l.id} className="border-zinc-800 hover:bg-zinc-800/50">
                      <TableCell className="py-3 text-sm text-zinc-500 font-mono">{l.id}</TableCell>
                      <TableCell className="py-3">
                        <div className="flex items-center gap-2">
                          <div className="w-7 h-7 rounded-full bg-zinc-800 flex items-center justify-center text-xs text-zinc-400 font-medium">
                            {(l.admin_email || 'A')[0]?.toUpperCase()}
                          </div>
                          <span className="text-sm text-zinc-200">{l.admin_email || `Admin #${l.admin_id}`}</span>
                        </div>
                      </TableCell>
                      <TableCell className="py-3">{getActionBadge(l.action)}</TableCell>
                      <TableCell className="py-3 text-sm text-zinc-400 font-mono hidden md:table-cell">
                        {l.ip || '-'}
                      </TableCell>
                      <TableCell className="py-3 text-sm text-zinc-400 hidden md:table-cell whitespace-nowrap">
                        {formatDate(l.created_at)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}

          {totalPages > 1 && (
            <div className="flex items-center justify-between p-3 border-t border-zinc-800">
              <div className="text-xs text-zinc-500">共 {total} 条记录，第 {page}/{totalPages} 页</div>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 h-8 w-8 p-0"
                  disabled={page <= 1 || loading}
                  onClick={() => setPage((p) => Math.max(1, p - 1))}
                >
                  <ChevronLeft className="w-4 h-4" />
                </Button>
                <span className="text-sm text-zinc-400">{page} / {totalPages}</span>
                <Button
                  variant="outline"
                  size="sm"
                  className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 h-8 w-8 p-0"
                  disabled={page >= totalPages || loading}
                  onClick={() => setPage((p) => p + 1)}
                >
                  <ChevronRight className="w-4 h-4" />
                </Button>
              </div>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
