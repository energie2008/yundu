import { useState, useEffect } from 'react'
import { MessageSquare, Eye, XCircle, Send, Inbox } from 'lucide-react'
import {
  Card,
  CardContent,
  Button,
  Input,
  Badge,
  Select,
  Textarea,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  Skeleton,
  EmptyState,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  useToast,
} from '@airport/ui'
import { api } from '@/lib/api'
import { EP } from '@/lib/endpoints'

interface TicketMessage {
  id: number
  user_id: number
  ticket_id: number
  message: string
  created_at: number | string
  updated_at: number | string
  is_me?: boolean
}

interface Ticket {
  id: number
  user_id: number
  subject: string
  level: number
  status: number
  created_at: number | string
  updated_at: number | string
  reply_status: number
  message?: TicketMessage[]
  user?: {
    id: number
    email: string
  }
}

interface TicketListResponse {
  data: Ticket[]
  total: number
  current_page: number
  last_page?: number
}

const LEVEL_MAP: Record<number, { label: string; variant: 'destructive' | 'warning' | 'default' | 'secondary' }> = {
  0: { label: '低', variant: 'secondary' },
  1: { label: '中', variant: 'default' },
  2: { label: '高', variant: 'warning' },
  3: { label: '紧急', variant: 'destructive' },
}

const STATUS_MAP: Record<number, { label: string; variant: 'warning' | 'default' | 'success' | 'secondary' }> = {
  0: { label: '已关闭', variant: 'secondary' },
  1: { label: '待处理', variant: 'warning' },
  2: { label: '已回复', variant: 'default' },
  3: { label: '已解答', variant: 'success' },
}

const priorityToLevel = (p: string): number => {
  switch (p) {
    case 'low': return 0
    case 'medium': return 1
    case 'high': return 2
    case 'urgent': return 3
    default: return 1
  }
}

const statusToNumber = (s: string): number => {
  switch (s) {
    case 'closed': return 0
    case 'pending': return 1
    case 'open': return 2
    case 'resolved': return 3
    default: return 1
  }
}

const numberToStatus = (n: number): string => {
  switch (n) {
    case 0: return 'closed'
    case 1: return 'pending'
    case 2: return 'open'
    case 3: return 'resolved'
    default: return 'open'
  }
}

const statusFilterToApi = (filter: string): string | undefined => {
  if (filter === 'all') return undefined
  return numberToStatus(Number(filter))
}

interface ApiReply {
  id: number
  ticket_id: number
  user_id: number
  admin_id?: number
  content: string
  created_at: number | string
  is_admin: boolean
}

interface ApiTicket {
  id: number
  subject: string
  status: string
  priority: string
  user_id: number
  assigned_admin_id?: number
  message: string
  created_at: number | string
  updated_at: number | string
  user?: {
    id: number
    email: string
  }
}

const mapApiTicket = (t: ApiTicket): Ticket => ({
  id: t.id,
  user_id: t.user_id,
  subject: t.subject,
  level: priorityToLevel(t.priority),
  status: statusToNumber(t.status),
  created_at: t.created_at,
  updated_at: t.updated_at,
  reply_status: t.status === 'pending' ? 0 : 1,
  user: t.user,
})

const mapApiReply = (r: ApiReply): TicketMessage => ({
  id: r.id,
  user_id: r.user_id,
  ticket_id: r.ticket_id,
  message: r.content,
  created_at: r.created_at,
  updated_at: r.created_at,
  is_me: r.is_admin,
})

const buildInitialMessage = (t: ApiTicket): TicketMessage => ({
  id: 0,
  user_id: t.user_id,
  ticket_id: t.id,
  message: t.message,
  created_at: t.created_at,
  updated_at: t.created_at,
  is_me: false,
})

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
    if (Array.isArray(dataObj.list)) return dataObj.list as T[]
  }
  if (Array.isArray(obj.data)) return obj.data as T[]
  if (Array.isArray(obj.items)) return obj.items as T[]
  if (Array.isArray(obj.list)) return obj.list as T[]
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
  const d = typeof ts === 'number' ? new Date(ts * 1000) : new Date(ts)
  if (isNaN(d.getTime())) return String(ts)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function getLastReplyTime(t: Ticket): number | string | undefined {
  if (t.message && t.message.length > 0) {
    return t.message[t.message.length - 1].created_at
  }
  return t.updated_at
}

export default function Tickets() {
  const { toast } = useToast()
  const [loading, setLoading] = useState(true)
  const [tickets, setTickets] = useState<Ticket[]>([])
  const [pendingCount, setPendingCount] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize] = useState(10)
  const [total, setTotal] = useState(0)
  const [statusFilter, setStatusFilter] = useState<string>('all')
  const [emailSearch, setEmailSearch] = useState('')
  const [emailSearchInput, setEmailSearchInput] = useState('')

  const [detailOpen, setDetailOpen] = useState(false)
  const [detailLoading, setDetailLoading] = useState(false)
  const [selectedTicket, setSelectedTicket] = useState<Ticket | null>(null)
  const [messages, setMessages] = useState<TicketMessage[]>([])
  const [replyContent, setReplyContent] = useState('')
  const [submittingReply, setSubmittingReply] = useState(false)
  const [actionLoading, setActionLoading] = useState(false)

  useEffect(() => {
    loadTickets()
  }, [page, statusFilter, emailSearch])

  const loadTickets = async () => {
    setLoading(true)
    try {
      const params: Record<string, string | number> = { page, page_size: pageSize }
      const apiStatus = statusFilterToApi(statusFilter)
      if (apiStatus) params.status = apiStatus
      if (emailSearch) params.email = emailSearch
      const [listResp, statsResp] = await Promise.all([
        api.get<{ items: ApiTicket[]; total: number; page: number; page_size: number }>(EP.TICKETS, { params }),
        api.get<{ open: number; pending: number; closed: number; total: number }>(EP.TICKET_STATS),
      ])
      const list = (listResp.items || []).map(mapApiTicket)
      setTickets(list)
      setTotal(listResp.total || 0)
      setPendingCount(statsResp.pending || 0)
    } catch (err) {
      toast({
        title: '加载失败',
        description: err instanceof Error ? err.message : '无法获取工单列表',
        variant: 'destructive',
      })
    } finally {
      setLoading(false)
    }
  }

  const openDetail = async (ticket: Ticket) => {
    setSelectedTicket(ticket)
    setDetailOpen(true)
    setDetailLoading(true)
    setMessages([])
    setReplyContent('')
    try {
      const [detail, repliesResp] = await Promise.all([
        api.get<ApiTicket>(EP.TICKET_DETAIL(String(ticket.id))),
        api.get<{ items: ApiReply[] }>(EP.TICKET_REPLIES(String(ticket.id))),
      ])
      const mappedTicket = mapApiTicket(detail)
      const initialMsg = buildInitialMessage(detail)
      const replies = (repliesResp.items || []).map(mapApiReply)
      setSelectedTicket(mappedTicket)
      setMessages([initialMsg, ...replies])
    } catch (err) {
      setMessages(ticket.message || [])
    } finally {
      setDetailLoading(false)
    }
  }

  const submitReply = async () => {
    if (!selectedTicket) return
    if (!replyContent.trim()) {
      toast({ title: '校验失败', description: '请输入回复内容', variant: 'destructive' })
      return
    }
    setSubmittingReply(true)
    try {
      await api.post(EP.TICKET_REPLIES(String(selectedTicket.id)), { content: replyContent.trim() })
      toast({ title: '回复成功', variant: 'success' })
      setReplyContent('')
      await openDetail(selectedTicket)
      await loadTickets()
    } catch (err) {
      toast({
        title: '回复失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setSubmittingReply(false)
    }
  }

  const closeTicket = async (id: number) => {
    setActionLoading(true)
    try {
      await api.patch(EP.TICKET_DETAIL(String(id)), { status: 'closed' })
      toast({ title: '工单已关闭', variant: 'success' })
      setDetailOpen(false)
      await loadTickets()
    } catch (err) {
      toast({
        title: '操作失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setActionLoading(false)
    }
  }

  const totalPages = Math.ceil(total / pageSize) || 1

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <h2 className="text-lg font-semibold text-zinc-100">工单管理</h2>
          {pendingCount > 0 && (
            <Badge variant="warning" className="animate-pulse">
              <Inbox className="w-3 h-3 mr-1" />
              {pendingCount} 待处理
            </Badge>
          )}
        </div>
      </div>

      <Card className="bg-zinc-900 border-zinc-800">
        <CardContent className="p-3">
          <div className="flex flex-col md:flex-row gap-2">
            <Select
              value={statusFilter}
              onChange={(e) => { setStatusFilter(e.target.value); setPage(1) }}
              className="bg-zinc-800 border-zinc-700 text-zinc-100 w-full md:w-48"
            >
              <option value="all">全部状态</option>
              <option value="0">已关闭</option>
              <option value="1">待处理</option>
              <option value="2">已回复</option>
              <option value="3">已解答</option>
            </Select>
            <div className="flex gap-2 flex-1">
              <Input
                placeholder="按用户邮箱搜索..."
                value={emailSearchInput}
                onChange={(e) => setEmailSearchInput(e.target.value)}
                onKeyDown={(e) => { if (e.key === 'Enter') { setEmailSearch(emailSearchInput.trim()); setPage(1) } }}
                className="bg-zinc-800 border-zinc-700 text-zinc-100 flex-1"
              />
              <Button
                variant="outline"
                size="sm"
                className="border-zinc-700 text-zinc-300 hover:bg-zinc-800"
                onClick={() => { setEmailSearch(emailSearchInput.trim()); setPage(1) }}
              >
                搜索
              </Button>
              {emailSearch && (
                <Button
                  variant="ghost"
                  size="sm"
                  className="text-zinc-400"
                  onClick={() => { setEmailSearch(''); setEmailSearchInput(''); setPage(1) }}
                >
                  清除
                </Button>
              )}
            </div>
          </div>
        </CardContent>
      </Card>

      <Card className="bg-zinc-900 border-zinc-800 overflow-hidden">
        <CardContent className="p-0">
          {loading ? (
            <div className="p-4 space-y-3">
              {[1, 2, 3, 4, 5].map((i) => (
                <Skeleton key={i} className="h-14 w-full bg-zinc-800 rounded-lg" />
              ))}
            </div>
          ) : tickets.length === 0 ? (
            <EmptyState title="暂无工单" description="没有符合条件的工单记录" className="py-12" />
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="border-zinc-800 hover:bg-transparent">
                    <TableHead className="text-zinc-400 text-xs font-medium">ID</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium">主题</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium">用户</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium">优先级</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium">状态</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium hidden md:table-cell">最后回复</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium w-28">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {tickets.map((t) => (
                    <TableRow key={t.id} className="border-zinc-800 hover:bg-zinc-800/50">
                      <TableCell className="py-3 text-sm text-zinc-400 font-mono">#{t.id}</TableCell>
                      <TableCell className="py-3">
                        <div className="flex items-center gap-2">
                          <div className="p-1.5 rounded-md bg-zinc-800">
                            <MessageSquare className="w-4 h-4 text-zinc-400" />
                          </div>
                          <div className="min-w-0">
                            <div className="font-medium text-zinc-200 text-sm truncate max-w-[220px]">
                              {t.subject || '(无主题)'}
                            </div>
                          </div>
                        </div>
                      </TableCell>
                      <TableCell className="py-3 text-sm text-zinc-400">
                        {t.user?.email || (t.user_id ? `UID:${String(t.user_id).slice(0, 8)}...` : '-')}
                      </TableCell>
                      <TableCell className="py-3">
                        <Badge variant={LEVEL_MAP[t.level]?.variant || 'secondary'}>
                          {LEVEL_MAP[t.level]?.label || '未知'}
                        </Badge>
                      </TableCell>
                      <TableCell className="py-3">
                        <Badge variant={STATUS_MAP[t.status]?.variant || 'secondary'}>
                          {STATUS_MAP[t.status]?.label || '未知'}
                        </Badge>
                      </TableCell>
                      <TableCell className="py-3 text-sm text-zinc-400 hidden md:table-cell">
                        {formatDate(getLastReplyTime(t))}
                      </TableCell>
                      <TableCell className="py-3">
                        <div className="flex items-center gap-1">
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-8 w-8 text-zinc-400"
                            onClick={() => openDetail(t)}
                          >
                            <Eye className="w-4 h-4" />
                          </Button>
                          {t.status !== 0 && (
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-8 w-8 text-red-400 hover:text-red-300"
                              onClick={() => closeTicket(t.id)}
                              disabled={actionLoading}
                            >
                              <XCircle className="w-4 h-4" />
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

          {totalPages > 1 && (
            <div className="flex items-center justify-between p-3 border-t border-zinc-800">
              <div className="text-xs text-zinc-500">共 {total} 条记录，第 {page}/{totalPages} 页</div>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  className="border-zinc-700 text-zinc-300 hover:bg-zinc-800"
                  disabled={page <= 1 || loading}
                  onClick={() => setPage((p) => Math.max(1, p - 1))}
                >
                  上一页
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  className="border-zinc-700 text-zinc-300 hover:bg-zinc-800"
                  disabled={page >= totalPages || loading}
                  onClick={() => setPage((p) => p + 1)}
                >
                  下一页
                </Button>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      <Dialog open={detailOpen} onOpenChange={setDetailOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-2xl max-h-[90vh] overflow-y-auto">
          {detailLoading ? (
            <div className="space-y-3 py-6">
              <Skeleton className="h-8 w-2/3 bg-zinc-800 rounded" />
              <Skeleton className="h-24 w-full bg-zinc-800 rounded" />
              <Skeleton className="h-16 w-full bg-zinc-800 rounded" />
            </div>
          ) : selectedTicket ? (
            <>
              <DialogHeader>
                <DialogTitle className="flex items-center gap-2">
                  <MessageSquare className="w-5 h-5 text-zinc-400" />
                  <span className="truncate">{selectedTicket.subject || '(无主题)'}</span>
                </DialogTitle>
                <div className="flex items-center gap-2 pt-1">
                  <span className="text-xs text-zinc-500">工单 #{selectedTicket.id}</span>
                  <Badge variant={LEVEL_MAP[selectedTicket.level]?.variant || 'secondary'}>
                    {LEVEL_MAP[selectedTicket.level]?.label || '未知'}
                  </Badge>
                  <Badge variant={STATUS_MAP[selectedTicket.status]?.variant || 'secondary'}>
                    {STATUS_MAP[selectedTicket.status]?.label || '未知'}
                  </Badge>
                </div>
              </DialogHeader>

              <div className="space-y-3 pt-2 max-h-[50vh] overflow-y-auto pr-1">
                {messages.length === 0 ? (
                  <div className="rounded-lg border border-zinc-800 bg-zinc-950/30 p-4 text-center text-sm text-zinc-500">
                    暂无消息记录
                  </div>
                ) : (
                  messages.map((m, idx) => {
                    const isAdmin = !!m.is_me
                    return (
                      <div
                        key={m.id}
                        className={`rounded-lg border p-3 text-sm ${
                          isAdmin
                            ? 'border-indigo-800/50 bg-indigo-950/20 ml-4'
                            : 'border-zinc-800 bg-zinc-950/30 mr-4'
                        }`}
                      >
                        <div className="flex items-center justify-between mb-1">
                          <Badge
                            variant="secondary"
                            className={isAdmin ? 'bg-indigo-900 text-indigo-200' : 'bg-zinc-800 text-zinc-300'}
                          >
                            {isAdmin ? '管理员' : '用户'}
                          </Badge>
                          <span className="text-xs text-zinc-500">{formatDate(m.created_at)}</span>
                        </div>
                        <div className="text-zinc-200 whitespace-pre-wrap break-words">{m.message}</div>
                      </div>
                    )
                  })
                )}
              </div>

              {selectedTicket.status !== 0 && (
                <div className="space-y-2 pt-2">
                  <Textarea
                    placeholder="请输入回复内容..."
                    value={replyContent}
                    onChange={(e) => setReplyContent(e.target.value)}
                    className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500 focus:border-indigo-500 min-h-[80px]"
                  />
                  <div className="flex items-center justify-between gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      className="border-red-800 text-red-300 hover:bg-red-900/20"
                      onClick={() => closeTicket(selectedTicket.id)}
                      disabled={actionLoading}
                      isLoading={actionLoading}
                    >
                      <XCircle className="w-4 h-4 mr-1" />
                      关闭工单
                    </Button>
                    <Button
                      size="sm"
                      className="bg-indigo-600 hover:bg-indigo-500"
                      onClick={submitReply}
                      disabled={submittingReply}
                      isLoading={submittingReply}
                    >
                      <Send className="w-4 h-4 mr-1" />
                      提交回复
                    </Button>
                  </div>
                </div>
              )}

              <DialogFooter className="pt-2">
                <Button
                  variant="outline"
                  onClick={() => setDetailOpen(false)}
                  className="border-zinc-700 text-zinc-300"
                >
                  关闭
                </Button>
              </DialogFooter>
            </>
          ) : null}
        </DialogContent>
      </Dialog>
    </div>
  )
}
