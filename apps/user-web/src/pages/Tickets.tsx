import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  MessageSquare,
  Plus,
  AlertCircle,
  Clock,
  CheckCircle2,
  XCircle,
  ChevronRight,
  Send,
  ArrowLeft,
  Loader2,
} from 'lucide-react'
import {
  Button,
  Badge,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  Textarea,
  Input,
} from '@airport/ui'
import { useToast } from '@/lib/toast'
import { api } from '@/lib/api'
import {
  EP,
  TicketResponse,
  TicketReplyResponse,
  PaginatedResponse,
  formatDateTime,
  formatTimeAgo,
  getStatusLabel,
} from '@/lib/endpoints'
import clsx from 'clsx'

function getStatusConfig(status: string) {
  const configs: Record<string, { label: string; color: string; bg: string; icon: React.ElementType }> = {
    open: { label: '处理中', color: 'var(--primary)', bg: 'rgba(124,92,252,0.1)', icon: Clock },
    pending: { label: '待处理', color: '#f59e0b', bg: 'rgba(245,158,11,0.1)', icon: AlertCircle },
    replied: { label: '已回复', color: 'var(--primary)', bg: 'rgba(124,92,252,0.1)', icon: MessageSquare },
    resolved: { label: '已解决', color: 'var(--success)', bg: 'rgba(34,197,94,0.1)', icon: CheckCircle2 },
    closed: { label: '已关闭', color: 'var(--muted-foreground)', bg: 'var(--muted)', icon: XCircle },
  }
  return configs[status] || { label: getStatusLabel(status), color: 'var(--muted-foreground)', bg: 'var(--muted)', icon: AlertCircle }
}

function TicketDetailView({ ticketId, onBack }: { ticketId: string; onBack: () => void }) {
  const { toast } = useToast()
  const queryClient = useQueryClient()
  const [replyContent, setReplyContent] = useState('')

  const { data: ticket, isLoading: ticketLoading, isError: ticketError } = useQuery<TicketResponse>({
    queryKey: ['ticket-detail', ticketId],
    queryFn: () => api.get<TicketResponse>(EP.TICKET_DETAIL(ticketId)),
    enabled: !!ticketId,
    retry: 1,
  })

  const { data: repliesData, isLoading: repliesLoading } = useQuery<TicketReplyResponse[]>({
    queryKey: ['ticket-replies', ticketId],
    queryFn: async () => {
      const raw = await api.get<unknown>(EP.TICKET_REPLIES(ticketId))
      // API 可能返回数组或 {items: [...]} 分页对象，统一提取为数组
      if (Array.isArray(raw)) return raw as TicketReplyResponse[]
      if (raw && typeof raw === 'object' && 'items' in raw) return (raw as any).items as TicketReplyResponse[]
      return []
    },
    enabled: !!ticketId,
  })
  const replies = Array.isArray(repliesData) ? repliesData : []

  const replyMutation = useMutation({
    mutationFn: (content: string) =>
      api.post<TicketReplyResponse>(EP.TICKET_ADD_REPLY(ticketId), { content }),
    onSuccess: () => {
      setReplyContent('')
      queryClient.invalidateQueries({ queryKey: ['ticket-replies', ticketId] })
      queryClient.invalidateQueries({ queryKey: ['ticket-detail', ticketId] })
      queryClient.invalidateQueries({ queryKey: ['tickets'] })
      toast({ title: '回复已发送', variant: 'success' })
    },
    onError: (e: any) => {
      toast({ title: '发送失败', description: e.message, variant: 'destructive' })
    },
  })

  const handleSendReply = () => {
    if (!replyContent.trim()) return
    replyMutation.mutate(replyContent.trim())
  }

  if (ticketLoading) {
    return (
      <div className="space-y-6">
        <button onClick={onBack} className="flex items-center gap-2 transition-colors" style={{ color: 'var(--muted-foreground)' }}
          onMouseEnter={e => (e.currentTarget.style.color = 'var(--primary)')}
          onMouseLeave={e => (e.currentTarget.style.color = 'var(--muted-foreground)')}>
          <ArrowLeft className="w-4 h-4" />
          <span className="text-sm">返回工单列表</span>
        </button>
        <div className="xboard-card">
          <div className="p-8 text-center">
            <Loader2 className="w-8 h-8 mx-auto animate-spin" style={{ color: 'var(--muted-foreground)' }} />
          </div>
        </div>
      </div>
    )
  }

  // 加载完成但数据为空或出错时，不渲染详情（防止访问 undefined 字段导致黑屏）
  if (!ticket || ticketError) {
    return (
      <div className="space-y-6">
        <button onClick={onBack} className="flex items-center gap-2 transition-colors" style={{ color: 'var(--muted-foreground)' }}
          onMouseEnter={e => (e.currentTarget.style.color = 'var(--primary)')}
          onMouseLeave={e => (e.currentTarget.style.color = 'var(--muted-foreground)')}>
          <ArrowLeft className="w-4 h-4" />
          <span className="text-sm">返回工单列表</span>
        </button>
        <div className="xboard-card">
          <div className="p-8 text-center">
            <AlertCircle className="w-8 h-8 mx-auto mb-3" style={{ color: 'var(--muted-foreground)' }} />
            <p className="text-sm" style={{ color: 'var(--muted-foreground)' }}>工单内容加载失败，请返回重试</p>
          </div>
        </div>
      </div>
    )
  }

  const status = getStatusConfig(ticket.status)
  const StatusIcon = status.icon
  const isClosed = ticket.status === 'closed'

  const allMessages = [
    {
      id: 'initial',
      author_name: '我',
      is_admin: false,
      content: ticket.message || '',
      created_at: ticket.created_at,
    },
    ...replies,
  ]

  return (
    <div className="space-y-6 p-6 max-w-4xl mx-auto">
      <button onClick={onBack} className="flex items-center gap-2 transition-colors" style={{ color: 'var(--muted-foreground)' }}
        onMouseEnter={e => (e.currentTarget.style.color = 'var(--primary)')}
        onMouseLeave={e => (e.currentTarget.style.color = 'var(--muted-foreground)')}>
        <ArrowLeft className="w-4 h-4" />
        <span className="text-sm">返回工单列表</span>
      </button>

      <div className="xboard-card">
        <div className="p-6">
          <div className="flex flex-col sm:flex-row sm:items-start sm:justify-between gap-4 mb-6">
            <div className="flex-1">
              <div className="flex items-center gap-3 mb-2 flex-wrap">
                <span className="text-sm font-mono" style={{ color: 'var(--muted-foreground)' }}>{ticket.ticket_no}</span>
                <Badge className="border-0 text-xs" style={{ backgroundColor: status.bg, color: status.color }}>
                  <StatusIcon className="w-3 h-3 mr-1" />
                  {status.label}
                </Badge>
                {ticket.unread_count && ticket.unread_count > 0 && (
                  <Badge className="border-0 text-xs" style={{ backgroundColor: 'rgba(239,68,68,0.1)', color: 'var(--destructive)' }}>
                    {ticket.unread_count} 条未读
                  </Badge>
                )}
              </div>
              <h1 className="text-xl font-semibold" style={{ color: 'var(--foreground)' }}>{ticket.subject}</h1>
              <div className="flex flex-wrap items-center gap-4 mt-2 text-sm" style={{ color: 'var(--muted-foreground)' }}>
                <span>创建时间：{formatDateTime(ticket.created_at)}</span>
                {ticket.updated_at && <span>更新时间：{formatDateTime(ticket.updated_at)}</span>}
              </div>
            </div>
          </div>

          <div className="space-y-4">
            {repliesLoading && allMessages.length === 1 ? (
              <div className="p-4 rounded-xl" style={{ backgroundColor: 'var(--muted)' }}>
                <Loader2 className="w-5 h-5 animate-spin mx-auto" style={{ color: 'var(--muted-foreground)' }} />
              </div>
            ) : (
              allMessages.map((msg) => (
                <div
                  key={msg.id}
                  className="p-4 rounded-xl"
                  style={{ backgroundColor: msg.is_admin ? 'rgba(124,92,252,0.06)' : 'var(--muted)' }}
                >
                  <div className="flex items-center gap-2 mb-3">
                    <div className="w-8 h-8 rounded-full flex items-center justify-center flex-shrink-0 text-white text-xs font-bold"
                      style={{ background: msg.is_admin ? 'var(--primary)' : 'linear-gradient(135deg, #7c5cfc, #9f87ff)' }}>
                      {(msg.author_name || (msg.is_admin ? '客服' : '我')).charAt(0)}
                    </div>
                    <div className="flex items-center gap-2 flex-wrap">
                      <span className="text-sm font-medium" style={{ color: 'var(--foreground)' }}>
                        {msg.author_name || (msg.is_admin ? '客服' : '我')}
                      </span>
                      {msg.is_admin && (
                        <Badge className="border-0 text-xs" style={{ backgroundColor: 'rgba(124,92,252,0.1)', color: 'var(--primary)' }}>客服</Badge>
                      )}
                      <span className="text-xs" style={{ color: 'var(--muted-foreground)' }}>{formatTimeAgo(msg.created_at)}</span>
                    </div>
                  </div>
                  <p className="text-sm whitespace-pre-line leading-relaxed pl-10" style={{ color: 'var(--foreground)' }}>
                    {msg.content}
                  </p>
                </div>
              ))
            )}
          </div>
        </div>

        {!isClosed && (
          <div className="p-6 pt-0">
            <div className="p-4 rounded-xl" style={{ backgroundColor: 'var(--muted)' }}>
              <Textarea
                placeholder="输入回复内容..."
                value={replyContent}
                onChange={(e) => setReplyContent(e.target.value)}
                className="min-h-[100px] mb-3 resize-none"
                style={{ background: 'var(--card)', borderColor: 'var(--border)', color: 'var(--foreground)' }}
              />
              <div className="flex justify-end">
                <Button
                  className="text-white h-10 px-5 border-0 shadow-sm"
                  style={{ background: 'var(--primary)' }}
                  onClick={handleSendReply}
                  disabled={!replyContent.trim() || replyMutation.isPending}
                  onMouseEnter={e => (e.currentTarget.style.opacity = '0.9')}
                  onMouseLeave={e => (e.currentTarget.style.opacity = '1')}
                >
                  {replyMutation.isPending ? (
                    <Loader2 className="w-4 h-4 mr-2 animate-spin" />
                  ) : (
                    <Send className="w-4 h-4 mr-2" />
                  )}
                  发送回复
                </Button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

export default function Tickets() {
  const { toast } = useToast()
  const queryClient = useQueryClient()
  const [selectedTicketId, setSelectedTicketId] = useState<string | null>(null)
  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [newSubject, setNewSubject] = useState('')
  const [newMessage, setNewMessage] = useState('')

  const { data: ticketsPage, isLoading } = useQuery<PaginatedResponse<TicketResponse>>({
    queryKey: ['tickets'],
    queryFn: async () => {
      const raw = await api.get<unknown>(EP.TICKETS, { params: { page: 1, page_size: 50 } })
      // 兼容分页对象 {items:[...]} 和直接数组 [...]
      if (Array.isArray(raw)) return { items: raw } as PaginatedResponse<TicketResponse>
      return raw as PaginatedResponse<TicketResponse>
    },
  })

  const tickets = ticketsPage?.items || []

  const createMutation = useMutation({
    mutationFn: (data: { subject: string; message: string }) =>
      api.post<TicketResponse>(EP.TICKETS, data),
    onSuccess: (newTicket) => {
      setCreateDialogOpen(false)
      setNewSubject('')
      setNewMessage('')
      queryClient.invalidateQueries({ queryKey: ['tickets'] })
      toast({ title: '工单已创建', variant: 'success' })
      // 防御：API 可能返回 null 或缺少 id 字段，避免 newTicket.id 崩溃
      if (newTicket?.id) {
        setSelectedTicketId(newTicket.id)
      }
    },
    onError: (e: any) => {
      toast({ title: '创建失败', description: e.message, variant: 'destructive' })
    },
  })

  const handleCreateTicket = () => {
    if (!newSubject.trim() || !newMessage.trim()) return
    createMutation.mutate({ subject: newSubject.trim(), message: newMessage.trim() })
  }

  if (selectedTicketId) {
    return <TicketDetailView ticketId={selectedTicketId} onBack={() => setSelectedTicketId(null)} />
  }

  return (
    <div className="p-6 max-w-4xl mx-auto space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-xl font-bold flex items-center gap-3" style={{ color: 'var(--foreground)' }}>
            <div className="w-10 h-10 rounded-xl flex items-center justify-center shadow-sm" style={{ background: 'var(--primary)' }}>
              <MessageSquare className="w-5 h-5 text-white" />
            </div>
            工单支持
          </h1>
          <p className="mt-1 text-sm" style={{ color: 'var(--muted-foreground)' }}>遇到问题？提交工单，我们会尽快为您处理</p>
        </div>
        <Dialog open={createDialogOpen} onOpenChange={setCreateDialogOpen}>
          <Button
            className="h-10 px-4 rounded-lg text-white border-0 shadow-sm text-sm font-medium"
            style={{ background: 'var(--primary)' }}
            onClick={() => setCreateDialogOpen(true)}
            onMouseEnter={e => (e.currentTarget.style.opacity = '0.9')}
            onMouseLeave={e => (e.currentTarget.style.opacity = '1')}
          >
            <Plus className="w-4 h-4 mr-1.5" />
            新建工单
          </Button>
          <DialogContent className="xboard-card" style={{ background: 'var(--card)', borderColor: 'var(--border)' }}>
            <DialogHeader>
              <DialogTitle style={{ color: 'var(--foreground)' }}>新建工单</DialogTitle>
              <DialogDescription style={{ color: 'var(--muted-foreground)' }}>请详细描述您遇到的问题，我们会尽快回复</DialogDescription>
            </DialogHeader>
            <div className="space-y-4 mt-4">
              <div>
                <label className="text-sm mb-1.5 block" style={{ color: 'var(--muted-foreground)' }}>标题</label>
                <Input
                  placeholder="请简要描述问题"
                  value={newSubject}
                  onChange={(e) => setNewSubject(e.target.value)}
                  style={{ background: 'var(--card)', borderColor: 'var(--border)', color: 'var(--foreground)' }}
                />
              </div>
              <div>
                <label className="text-sm mb-1.5 block" style={{ color: 'var(--muted-foreground)' }}>问题描述</label>
                <Textarea
                  placeholder="请详细描述您遇到的问题，包括出现时间、具体现象、已尝试的解决方法等..."
                  value={newMessage}
                  onChange={(e) => setNewMessage(e.target.value)}
                  className="min-h-[150px] resize-none"
                  style={{ background: 'var(--card)', borderColor: 'var(--border)', color: 'var(--foreground)' }}
                />
              </div>
            </div>
            <DialogFooter>
              <Button
                variant="outline"
                className="h-10"
                style={{ borderColor: 'var(--border)', color: 'var(--muted-foreground)', backgroundColor: 'transparent' }}
                onClick={() => setCreateDialogOpen(false)}
              >
                取消
              </Button>
              <Button
                className="h-10 text-white border-0 shadow-sm"
                style={{ background: 'var(--primary)' }}
                onClick={handleCreateTicket}
                disabled={!newSubject.trim() || !newMessage.trim() || createMutation.isPending}
                onMouseEnter={e => (e.currentTarget.style.opacity = '0.9')}
                onMouseLeave={e => (e.currentTarget.style.opacity = '1')}
              >
                {createMutation.isPending ? (
                  <Loader2 className="w-4 h-4 mr-2 animate-spin" />
                ) : (
                  <Send className="w-4 h-4 mr-2" />
                )}
                提交工单
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>

      <div className="xboard-card">
        <div className="p-6">
          {isLoading ? (
            <div className="space-y-3">
              {[0, 1, 2].map(i => (
                <div key={i} className="p-4 rounded-xl animate-pulse" style={{ backgroundColor: 'var(--muted)' }}>
                  <div className="h-4 w-1/3 rounded mb-2" style={{ backgroundColor: 'var(--border)' }} />
                  <div className="h-3 w-2/3 rounded" style={{ backgroundColor: 'var(--border)' }} />
                </div>
              ))}
            </div>
          ) : tickets.length === 0 ? (
            <div className="text-center py-12">
              <div className="w-16 h-16 rounded-2xl flex items-center justify-center mx-auto mb-4" style={{ backgroundColor: 'var(--muted)' }}>
                <MessageSquare className="w-8 h-8" style={{ color: 'var(--muted-foreground)' }} />
              </div>
              <h3 className="text-lg font-semibold mb-2" style={{ color: 'var(--foreground)' }}>暂无工单</h3>
              <p className="text-sm mb-6" style={{ color: 'var(--muted-foreground)' }}>遇到问题？点击右上角按钮提交工单</p>
            </div>
          ) : (
            <div className="space-y-3">
              {tickets.map(ticket => {
                const status = getStatusConfig(ticket.status)
                const StatusIcon = status.icon
                return (
                  <button
                    key={ticket.id}
                    className="w-full p-4 rounded-xl text-left transition-all duration-200 group"
                    style={{ backgroundColor: 'var(--muted)' }}
                    onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = 'var(--border)' }}
                    onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = 'var(--muted)' }}
                    onClick={() => setSelectedTicketId(ticket.id)}
                  >
                    <div className="flex flex-col sm:flex-row sm:items-center gap-3">
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2 mb-1.5 flex-wrap">
                          <span className="text-xs font-mono" style={{ color: 'var(--muted-foreground)' }}>{ticket.ticket_no}</span>
                          <Badge className="border-0 text-xs" style={{ backgroundColor: status.bg, color: status.color }}>
                            <StatusIcon className="w-3 h-3 mr-1" />
                            {status.label}
                          </Badge>
                          {ticket.unread_count && ticket.unread_count > 0 && (
                            <Badge className="border-0 text-xs" style={{ backgroundColor: 'rgba(239,68,68,0.1)', color: 'var(--destructive)' }}>
                              {ticket.unread_count} 未读
                            </Badge>
                          )}
                        </div>
                        <h3 className="font-medium line-clamp-1 transition-colors" style={{ color: 'var(--foreground)' }}>
                          {ticket.subject}
                        </h3>
                        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 mt-1 text-xs" style={{ color: 'var(--muted-foreground)' }}>
                          <span>创建于 {formatDateTime(ticket.created_at)}</span>
                          {ticket.last_reply_at && <span>最后回复 {formatTimeAgo(ticket.last_reply_at)}</span>}
                        </div>
                      </div>
                      <ChevronRight className="w-5 h-5 flex-shrink-0 transition-all group-hover:translate-x-1" style={{ color: 'var(--muted-foreground)' }} />
                    </div>
                  </button>
                )
              })}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
