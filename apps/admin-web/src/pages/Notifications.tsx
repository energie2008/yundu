import { useState, useEffect } from 'react'
import { Bell, Mail, Archive, Trash2, Check, Pencil, Plus, Power } from 'lucide-react'
import {
  Card,
  CardContent,
  Button,
  Input,
  Label,
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
  DialogDescription,
  DialogFooter,
  useToast,
} from '@airport/ui'
import { api } from '@/lib/api'
import { EP } from '@/lib/endpoints'

// ===== 类型定义 =====

interface Notification {
  id: string
  user_id: string
  title: string
  body: string
  category: string
  channel: string
  status: string
  priority: string
  scheduled_at?: string
  sent_at?: string
  read_at?: string
  archived_at?: string
  created_at: string
  updated_at: string
}

interface NotificationTemplate {
  code: string
  name: string
  category: string
  title_template: string
  body_template: string
  channels: string[]
  enabled: boolean
  created_at: string
  updated_at: string
}

// ===== 常量映射 =====

const CATEGORY_LABEL: Record<string, string> = {
  traffic_expiry: '流量到期',
  plan_expiry: '套餐到期',
  node_change: '节点变更',
  system: '系统',
  subscription: '订阅',
  billing: '账单',
  ticket: '工单',
  announcement: '公告',
}

const CHANNEL_LABEL: Record<string, string> = {
  in_app: '站内',
  email: '邮件',
  telegram: 'TG',
  bark: 'Bark',
}

const STATUS_LABEL: Record<string, string> = {
  pending: '待发送',
  sent: '已发送',
  delivered: '已送达',
  failed: '失败',
  read: '已读',
}

const PRIORITY_LABEL: Record<string, string> = {
  low: '低',
  normal: '中',
  high: '高',
  urgent: '紧急',
}

const CHANNEL_OPTIONS = [
  { value: 'in_app', label: '站内' },
  { value: 'email', label: '邮件' },
  { value: 'telegram', label: 'TG' },
  { value: 'bark', label: 'Bark' },
]

// ===== 工具函数 =====

function extractList<T>(resp: unknown): T[] {
  if (Array.isArray(resp)) return resp as T[]
  if (!resp || typeof resp !== 'object') return []
  const obj = resp as Record<string, unknown>
  const dataField = obj.data
  if (dataField && typeof dataField === 'object') {
    if (Array.isArray(dataField)) return dataField as T[]
    const dataObj = dataField as Record<string, unknown>
    if (Array.isArray(dataObj.items)) return dataObj.items as T[]
    if (Array.isArray(dataObj.list)) return dataObj.list as T[]
  }
  if (Array.isArray(obj.items)) return obj.items as T[]
  if (Array.isArray(obj.list)) return obj.list as T[]
  if (Array.isArray(obj.data)) return obj.data as T[]
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

function categoryBadge(category: string) {
  const label = CATEGORY_LABEL[category] || category || '-'
  return <Badge variant="secondary">{label}</Badge>
}

function channelBadge(channel: string) {
  const label = CHANNEL_LABEL[channel] || channel || '-'
  return <Badge variant="secondary">{label}</Badge>
}

function statusBadge(status: string) {
  const label = STATUS_LABEL[status] || status || '-'
  switch (status) {
    case 'pending':
      return <Badge variant="warning">{label}</Badge>
    case 'sent':
      return <Badge variant="default">{label}</Badge>
    case 'delivered':
      return <Badge variant="success">{label}</Badge>
    case 'failed':
      return <Badge variant="destructive">{label}</Badge>
    case 'read':
      return <Badge variant="secondary">{label}</Badge>
    default:
      return <Badge variant="secondary">{label}</Badge>
  }
}

function priorityBadge(priority: string) {
  const label = PRIORITY_LABEL[priority] || priority || '-'
  switch (priority) {
    case 'urgent':
      return <Badge variant="destructive">{label}</Badge>
    case 'high':
      return <Badge variant="warning">{label}</Badge>
    case 'normal':
      return <Badge variant="default">{label}</Badge>
    case 'low':
      return <Badge variant="secondary">{label}</Badge>
    default:
      return <Badge variant="secondary">{label}</Badge>
  }
}

function formatDate(s?: string): string {
  if (!s) return '-'
  const d = new Date(s)
  if (isNaN(d.getTime())) return s
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function truncate(s: string, max = 60): string {
  if (!s) return '-'
  return s.length > max ? s.slice(0, max) + '…' : s
}

// ===== 主组件 =====

export default function Notifications() {
  const { toast } = useToast()
  const [tab, setTab] = useState<'list' | 'templates'>('list')

  // 通知列表状态
  const [loading, setLoading] = useState(true)
  const [notifications, setNotifications] = useState<Notification[]>([])
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  const [priorityFilter, setPriorityFilter] = useState('all')
  const [categoryFilter, setCategoryFilter] = useState('all')
  const [channelFilter, setChannelFilter] = useState('all')
  const [actionLoading, setActionLoading] = useState(false)

  // 模板状态
  const [tplLoading, setTplLoading] = useState(true)
  const [templates, setTemplates] = useState<NotificationTemplate[]>([])
  const [tplDialogOpen, setTplDialogOpen] = useState(false)
  const [editingCode, setEditingCode] = useState<string | null>(null)
  const [tplSubmitting, setTplSubmitting] = useState(false)
  const [tplForm, setTplForm] = useState({
    code: '',
    name: '',
    category: '',
    title_template: '',
    body_template: '',
    enabled: true,
  })
  const [tplChannels, setTplChannels] = useState<string[]>([])

  // ===== 通知列表加载 =====

  const loadNotifications = async () => {
    setLoading(true)
    try {
      const resp = await api.get<unknown>(EP.NOTIFICATIONS)
      setNotifications(extractList<Notification>(resp))
    } catch (err) {
      toast({
        title: '加载失败',
        description: err instanceof Error ? err.message : '无法获取通知列表',
        variant: 'destructive',
      })
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadNotifications()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // ===== 统计（从列表数据计算） =====

  const totalCount = notifications.length
  const unreadCount = notifications.filter(
    (n) => n.status !== 'read' && !n.archived_at
  ).length
  const readCount = notifications.filter((n) => n.status === 'read').length
  const archivedCount = notifications.filter((n) => !!n.archived_at).length

  // ===== 客户端过滤 =====

  const filteredNotifications = notifications.filter((n) => {
    const kw = search.trim().toLowerCase()
    const matchSearch =
      !kw ||
      n.title.toLowerCase().includes(kw) ||
      n.id.toLowerCase().includes(kw) ||
      n.user_id.toLowerCase().includes(kw) ||
      n.body.toLowerCase().includes(kw)
    const matchStatus = statusFilter === 'all' || n.status === statusFilter
    const matchPriority = priorityFilter === 'all' || n.priority === priorityFilter
    const matchCategory = categoryFilter === 'all' || n.category === categoryFilter
    const matchChannel = channelFilter === 'all' || n.channel === channelFilter
    return matchSearch && matchStatus && matchPriority && matchCategory && matchChannel
  })

  // ===== 通知操作 =====

  const markRead = async (n: Notification) => {
    setActionLoading(true)
    try {
      await api.post(EP.NOTIFICATION_READ(n.id))
      toast({ title: '已标记为已读', variant: 'success' })
      await loadNotifications()
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

  const archive = async (n: Notification) => {
    setActionLoading(true)
    try {
      await api.post(`${EP.NOTIFICATIONS}/${n.id}/archive`)
      toast({ title: '已归档', variant: 'success' })
      await loadNotifications()
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

  const removeNotification = async (n: Notification) => {
    if (!window.confirm(`确认删除通知「${n.title || n.id}」吗？`)) return
    setActionLoading(true)
    try {
      await api.delete(`${EP.NOTIFICATIONS}/${n.id}`)
      toast({ title: '已删除', variant: 'success' })
      await loadNotifications()
    } catch (err) {
      toast({
        title: '删除失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setActionLoading(false)
    }
  }

  // ===== 模板加载 =====

  const loadTemplates = async () => {
    setTplLoading(true)
    try {
      const resp = await api.get<unknown>(EP.NOTIFICATION_TEMPLATES)
      setTemplates(extractList<NotificationTemplate>(resp))
    } catch (err) {
      toast({
        title: '加载失败',
        description: err instanceof Error ? err.message : '无法获取通知模板',
        variant: 'destructive',
      })
    } finally {
      setTplLoading(false)
    }
  }

  useEffect(() => {
    if (tab === 'templates' && templates.length === 0 && tplLoading) {
      loadTemplates()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tab])

  // ===== 模板表单 =====

  const resetTplForm = () => {
    setTplForm({
      code: '',
      name: '',
      category: '',
      title_template: '',
      body_template: '',
      enabled: true,
    })
    setTplChannels([])
    setEditingCode(null)
  }

  const openCreateTpl = () => {
    resetTplForm()
    setTplDialogOpen(true)
  }

  const openEditTpl = (tpl: NotificationTemplate) => {
    setEditingCode(tpl.code)
    setTplForm({
      code: tpl.code,
      name: tpl.name || '',
      category: tpl.category || '',
      title_template: tpl.title_template || '',
      body_template: tpl.body_template || '',
      enabled: !!tpl.enabled,
    })
    setTplChannels(Array.isArray(tpl.channels) ? tpl.channels : [])
    setTplDialogOpen(true)
  }

  const toggleChannel = (value: string) => {
    setTplChannels((prev) =>
      prev.includes(value) ? prev.filter((c) => c !== value) : [...prev, value]
    )
  }

  const submitTemplate = async () => {
    if (!tplForm.code.trim()) {
      toast({ title: '校验失败', description: '请输入模板编码', variant: 'destructive' })
      return
    }
    if (!tplForm.name.trim()) {
      toast({ title: '校验失败', description: '请输入模板名称', variant: 'destructive' })
      return
    }
    setTplSubmitting(true)
    try {
      const body = {
        code: tplForm.code.trim(),
        name: tplForm.name.trim(),
        category: tplForm.category || '',
        title_template: tplForm.title_template || '',
        body_template: tplForm.body_template || '',
        channels: tplChannels,
        enabled: tplForm.enabled,
      }
      await api.put(EP.NOTIFICATION_TEMPLATE_DETAIL(body.code), body)
      toast({
        title: editingCode ? '模板已更新' : '模板已创建',
        variant: 'success',
      })
      setTplDialogOpen(false)
      resetTplForm()
      await loadTemplates()
    } catch (err) {
      toast({
        title: '保存失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setTplSubmitting(false)
    }
  }

  const toggleTemplateEnabled = async (tpl: NotificationTemplate) => {
    try {
      await api.patch(EP.NOTIFICATION_TEMPLATE_ENABLE(tpl.code), {
        enabled: !tpl.enabled,
      })
      toast({
        title: tpl.enabled ? '已禁用' : '已启用',
        variant: 'success',
      })
      await loadTemplates()
    } catch (err) {
      toast({
        title: '操作失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    }
  }

  const removeTemplate = async (tpl: NotificationTemplate) => {
    if (!window.confirm(`确认删除模板「${tpl.name || tpl.code}」吗？`)) return
    try {
      await api.delete(EP.NOTIFICATION_TEMPLATE_DETAIL(tpl.code))
      toast({ title: '已删除', variant: 'success' })
      await loadTemplates()
    } catch (err) {
      toast({
        title: '删除失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    }
  }

  // ===== 统计卡片 =====

  const statCards: { key: 'total' | 'unread' | 'read' | 'archived'; label: string; variant: 'default' | 'warning' | 'success' | 'secondary' }[] = [
    { key: 'total', label: '总数', variant: 'default' },
    { key: 'unread', label: '未读', variant: 'warning' },
    { key: 'read', label: '已读', variant: 'success' },
    { key: 'archived', label: '已归档', variant: 'secondary' },
  ]

  const statValues: Record<string, number> = {
    total: totalCount,
    unread: unreadCount,
    read: readCount,
    archived: archivedCount,
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-zinc-100">通知管理</h2>
      </div>

      {/* Tab 切换 */}
      <div className="flex items-center gap-1 border-b border-zinc-800">
        <button
          className={`px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px ${
            tab === 'list'
              ? 'border-indigo-500 text-indigo-300'
              : 'border-transparent text-zinc-400 hover:text-zinc-200'
          }`}
          onClick={() => setTab('list')}
        >
          通知列表
        </button>
        <button
          className={`px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px ${
            tab === 'templates'
              ? 'border-indigo-500 text-indigo-300'
              : 'border-transparent text-zinc-400 hover:text-zinc-200'
          }`}
          onClick={() => setTab('templates')}
        >
          通知模板
        </button>
      </div>

      {/* ===== Tab 1: 通知列表 ===== */}
      {tab === 'list' && (
        <>
          {/* 顶部统计卡片 */}
          <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
            {statCards.map((c) => (
              <Card key={c.key} className="bg-zinc-900 border-zinc-800">
                <CardContent className="p-3">
                  <div className="flex items-center justify-between">
                    <span className="text-xs text-zinc-400">{c.label}</span>
                    <Badge variant={c.variant}>{statValues[c.key] ?? 0}</Badge>
                  </div>
                  <div className="mt-1 text-2xl font-semibold text-zinc-100">
                    {statValues[c.key] ?? 0}
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>

          {/* 筛选区 */}
          <Card className="bg-zinc-900 border-zinc-800">
            <CardContent className="p-3 space-y-3">
              <Input
                placeholder="搜索通知标题、内容、ID 或用户..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500"
              />
              <div className="grid grid-cols-1 md:grid-cols-4 gap-2">
                <Select
                  value={categoryFilter}
                  onChange={(e) => setCategoryFilter(e.target.value)}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                >
                  <option value="all">全部分类</option>
                  <option value="traffic_expiry">流量到期</option>
                  <option value="plan_expiry">套餐到期</option>
                  <option value="node_change">节点变更</option>
                  <option value="system">系统</option>
                  <option value="subscription">订阅</option>
                  <option value="billing">账单</option>
                  <option value="ticket">工单</option>
                  <option value="announcement">公告</option>
                </Select>
                <Select
                  value={channelFilter}
                  onChange={(e) => setChannelFilter(e.target.value)}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                >
                  <option value="all">全部渠道</option>
                  <option value="in_app">站内</option>
                  <option value="email">邮件</option>
                  <option value="telegram">TG</option>
                  <option value="bark">Bark</option>
                </Select>
                <Select
                  value={statusFilter}
                  onChange={(e) => setStatusFilter(e.target.value)}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                >
                  <option value="all">全部状态</option>
                  <option value="pending">待发送</option>
                  <option value="sent">已发送</option>
                  <option value="delivered">已送达</option>
                  <option value="failed">失败</option>
                  <option value="read">已读</option>
                </Select>
                <Select
                  value={priorityFilter}
                  onChange={(e) => setPriorityFilter(e.target.value)}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                >
                  <option value="all">全部优先级</option>
                  <option value="urgent">紧急</option>
                  <option value="high">高</option>
                  <option value="normal">中</option>
                  <option value="low">低</option>
                </Select>
              </div>
            </CardContent>
          </Card>

          {/* 通知表格 */}
          <Card className="bg-zinc-900 border-zinc-800 overflow-hidden">
            <CardContent className="p-0">
              {loading ? (
                <div className="p-4 space-y-3">
                  {[1, 2, 3, 4].map((i) => (
                    <Skeleton key={i} className="h-14 w-full bg-zinc-800 rounded-lg" />
                  ))}
                </div>
              ) : filteredNotifications.length === 0 ? (
                <EmptyState
                  title="暂无通知"
                  description="没有符合条件的通知记录"
                  className="py-12"
                />
              ) : (
                <div className="overflow-x-auto">
                  <Table>
                    <TableHeader>
                      <TableRow className="border-zinc-800 hover:bg-transparent">
                        <TableHead className="text-zinc-400 text-xs font-medium">标题</TableHead>
                        <TableHead className="text-zinc-400 text-xs font-medium">分类</TableHead>
                        <TableHead className="text-zinc-400 text-xs font-medium">渠道</TableHead>
                        <TableHead className="text-zinc-400 text-xs font-medium">状态</TableHead>
                        <TableHead className="text-zinc-400 text-xs font-medium">优先级</TableHead>
                        <TableHead className="text-zinc-400 text-xs font-medium hidden md:table-cell">创建时间</TableHead>
                        <TableHead className="text-zinc-400 text-xs font-medium w-32"></TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {filteredNotifications.map((n) => (
                        <TableRow key={n.id} className="border-zinc-800 hover:bg-zinc-800/50">
                          <TableCell className="py-3">
                            <div className="flex items-center gap-2">
                              <div className="p-1.5 rounded-md bg-zinc-800">
                                <Bell className="w-4 h-4 text-zinc-400" />
                              </div>
                              <div className="min-w-0">
                                <div className="font-medium text-zinc-200 text-sm truncate max-w-[200px]">
                                  {n.title || '(无标题)'}
                                </div>
                                <div className="text-xs text-zinc-500 truncate max-w-[260px]">
                                  {truncate(n.body, 60)}
                                </div>
                              </div>
                            </div>
                          </TableCell>
                          <TableCell className="py-3">{categoryBadge(n.category)}</TableCell>
                          <TableCell className="py-3">{channelBadge(n.channel)}</TableCell>
                          <TableCell className="py-3">{statusBadge(n.status)}</TableCell>
                          <TableCell className="py-3">{priorityBadge(n.priority)}</TableCell>
                          <TableCell className="py-3 text-sm text-zinc-400 hidden md:table-cell">
                            {formatDate(n.created_at)}
                          </TableCell>
                          <TableCell className="py-3">
                            <div className="flex items-center gap-1">
                              {n.status !== 'read' && !n.archived_at && (
                                <Button
                                  variant="ghost"
                                  size="icon"
                                  className="h-8 w-8 text-zinc-400 hover:text-emerald-300"
                                  onClick={() => markRead(n)}
                                  disabled={actionLoading}
                                  title="标记已读"
                                >
                                  <Check className="w-4 h-4" />
                                </Button>
                              )}
                              {!n.archived_at && (
                                <Button
                                  variant="ghost"
                                  size="icon"
                                  className="h-8 w-8 text-zinc-400 hover:text-amber-300"
                                  onClick={() => archive(n)}
                                  disabled={actionLoading}
                                  title="归档"
                                >
                                  <Archive className="w-4 h-4" />
                                </Button>
                              )}
                              <Button
                                variant="ghost"
                                size="icon"
                                className="h-8 w-8 text-zinc-400 hover:text-red-300"
                                onClick={() => removeNotification(n)}
                                disabled={actionLoading}
                                title="删除"
                              >
                                <Trash2 className="w-4 h-4" />
                              </Button>
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
        </>
      )}

      {/* ===== Tab 2: 通知模板 ===== */}
      {tab === 'templates' && (
        <>
          <div className="flex items-center justify-end">
            <Button
              size="sm"
              className="bg-indigo-600 hover:bg-indigo-500"
              onClick={openCreateTpl}
            >
              <Plus className="w-4 h-4 mr-1" />
              新建模板
            </Button>
          </div>

          <Card className="bg-zinc-900 border-zinc-800 overflow-hidden">
            <CardContent className="p-0">
              {tplLoading ? (
                <div className="p-4 space-y-3">
                  {[1, 2, 3].map((i) => (
                    <Skeleton key={i} className="h-14 w-full bg-zinc-800 rounded-lg" />
                  ))}
                </div>
              ) : templates.length === 0 ? (
                <EmptyState
                  title="暂无模板"
                  description="点击「新建模板」创建通知模板"
                  className="py-12"
                />
              ) : (
                <div className="overflow-x-auto">
                  <Table>
                    <TableHeader>
                      <TableRow className="border-zinc-800 hover:bg-transparent">
                        <TableHead className="text-zinc-400 text-xs font-medium">编码</TableHead>
                        <TableHead className="text-zinc-400 text-xs font-medium">名称</TableHead>
                        <TableHead className="text-zinc-400 text-xs font-medium">分类</TableHead>
                        <TableHead className="text-zinc-400 text-xs font-medium">标题模板</TableHead>
                        <TableHead className="text-zinc-400 text-xs font-medium">渠道</TableHead>
                        <TableHead className="text-zinc-400 text-xs font-medium">状态</TableHead>
                        <TableHead className="text-zinc-400 text-xs font-medium w-24"></TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {templates.map((tpl) => (
                        <TableRow key={tpl.code} className="border-zinc-800 hover:bg-zinc-800/50">
                          <TableCell className="py-3">
                            <span className="font-mono text-xs text-zinc-300">{tpl.code}</span>
                          </TableCell>
                          <TableCell className="py-3">
                            <span className="font-medium text-zinc-200 text-sm">
                              {tpl.name || '(无名称)'}
                            </span>
                          </TableCell>
                          <TableCell className="py-3">{categoryBadge(tpl.category)}</TableCell>
                          <TableCell className="py-3">
                            <span className="text-sm text-zinc-400 truncate max-w-[220px] inline-block align-bottom">
                              {truncate(tpl.title_template, 50)}
                            </span>
                          </TableCell>
                          <TableCell className="py-3">
                            <div className="flex flex-wrap gap-1">
                              {Array.isArray(tpl.channels) && tpl.channels.length > 0 ? (
                                tpl.channels.map((ch) => (
                                  <span key={ch}>{channelBadge(ch)}</span>
                                ))
                              ) : (
                                <span className="text-xs text-zinc-500">-</span>
                              )}
                            </div>
                          </TableCell>
                          <TableCell className="py-3">
                            {tpl.enabled ? (
                              <Badge variant="success">启用</Badge>
                            ) : (
                              <Badge variant="secondary">禁用</Badge>
                            )}
                          </TableCell>
                          <TableCell className="py-3">
                            <div className="flex items-center gap-1">
                              <Button
                                variant="ghost"
                                size="icon"
                                className="h-8 w-8 text-zinc-400 hover:text-indigo-300"
                                onClick={() => openEditTpl(tpl)}
                                title="编辑"
                              >
                                <Pencil className="w-4 h-4" />
                              </Button>
                              <Button
                                variant="ghost"
                                size="icon"
                                className="h-8 w-8 text-zinc-400 hover:text-amber-300"
                                onClick={() => toggleTemplateEnabled(tpl)}
                                title={tpl.enabled ? '禁用' : '启用'}
                              >
                                <Power className="w-4 h-4" />
                              </Button>
                              <Button
                                variant="ghost"
                                size="icon"
                                className="h-8 w-8 text-zinc-400 hover:text-red-300"
                                onClick={() => removeTemplate(tpl)}
                                title="删除"
                              >
                                <Trash2 className="w-4 h-4" />
                              </Button>
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

          {/* 模板创建/编辑 Dialog */}
          <Dialog open={tplDialogOpen} onOpenChange={setTplDialogOpen}>
            <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-2xl max-h-[90vh] overflow-y-auto">
              <DialogHeader>
                <DialogTitle className="flex items-center gap-2">
                  <Mail className="w-5 h-5 text-zinc-400" />
                  <span>{editingCode ? '编辑模板' : '新建模板'}</span>
                </DialogTitle>
                <DialogDescription>
                  {editingCode ? `修改模板：${editingCode}` : '创建一个新的通知模板'}
                </DialogDescription>
              </DialogHeader>

              <div className="space-y-3 pt-2">
                <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                  <div className="space-y-1.5">
                    <Label htmlFor="tpl-code" className="text-zinc-300 text-xs">
                      模板编码
                    </Label>
                    <Input
                      id="tpl-code"
                      placeholder="如 traffic_expiry_warn"
                      value={tplForm.code}
                      onChange={(e) =>
                        setTplForm((f) => ({ ...f, code: e.target.value }))
                      }
                      disabled={!!editingCode}
                      className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500 disabled:opacity-60"
                    />
                  </div>
                  <div className="space-y-1.5">
                    <Label htmlFor="tpl-name" className="text-zinc-300 text-xs">
                      模板名称
                    </Label>
                    <Input
                      id="tpl-name"
                      placeholder="如 流量到期提醒"
                      value={tplForm.name}
                      onChange={(e) =>
                        setTplForm((f) => ({ ...f, name: e.target.value }))
                      }
                      className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500"
                    />
                  </div>
                </div>

                <div className="space-y-1.5">
                  <Label htmlFor="tpl-category" className="text-zinc-300 text-xs">
                    分类
                  </Label>
                  <Select
                    id="tpl-category"
                    value={tplForm.category}
                    onChange={(e) =>
                      setTplForm((f) => ({ ...f, category: e.target.value }))
                    }
                    className="bg-zinc-800 border-zinc-700 text-zinc-100"
                  >
                    <option value="">请选择分类</option>
                    <option value="traffic_expiry">流量到期</option>
                    <option value="plan_expiry">套餐到期</option>
                    <option value="node_change">节点变更</option>
                    <option value="system">系统</option>
                    <option value="subscription">订阅</option>
                    <option value="billing">账单</option>
                    <option value="ticket">工单</option>
                    <option value="announcement">公告</option>
                  </Select>
                </div>

                <div className="space-y-1.5">
                  <Label htmlFor="tpl-title" className="text-zinc-300 text-xs">
                    标题模板
                  </Label>
                  <Input
                    id="tpl-title"
                    placeholder="如 您的流量将于 {{.Days}} 天后到期"
                    value={tplForm.title_template}
                    onChange={(e) =>
                      setTplForm((f) => ({ ...f, title_template: e.target.value }))
                    }
                    className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500"
                  />
                </div>

                <div className="space-y-1.5">
                  <Label htmlFor="tpl-body" className="text-zinc-300 text-xs">
                    正文模板
                  </Label>
                  <Textarea
                    id="tpl-body"
                    placeholder="支持模板变量，如 {{.UserName}} {{.Days}} ..."
                    value={tplForm.body_template}
                    onChange={(e) =>
                      setTplForm((f) => ({ ...f, body_template: e.target.value }))
                    }
                    className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500 focus:border-indigo-500 min-h-[100px]"
                  />
                </div>

                <div className="space-y-1.5">
                  <Label className="text-zinc-300 text-xs">投递渠道（可多选）</Label>
                  <div className="flex flex-wrap gap-4 pt-1">
                    {CHANNEL_OPTIONS.map((opt) => (
                      <label
                        key={opt.value}
                        className="flex items-center gap-2 cursor-pointer select-none"
                      >
                        <input
                          type="checkbox"
                          checked={tplChannels.includes(opt.value)}
                          onChange={() => toggleChannel(opt.value)}
                          className="h-4 w-4 rounded border-zinc-600 bg-zinc-800 text-indigo-600 focus:ring-indigo-500 focus:ring-offset-zinc-900"
                        />
                        <span className="text-sm text-zinc-300">{opt.label}</span>
                      </label>
                    ))}
                  </div>
                </div>

                <div className="space-y-1.5">
                  <label className="flex items-center gap-2 cursor-pointer select-none">
                    <input
                      type="checkbox"
                      checked={tplForm.enabled}
                      onChange={(e) =>
                        setTplForm((f) => ({ ...f, enabled: e.target.checked }))
                      }
                      className="h-4 w-4 rounded border-zinc-600 bg-zinc-800 text-indigo-600 focus:ring-indigo-500 focus:ring-offset-zinc-900"
                    />
                    <span className="text-sm text-zinc-300">启用此模板</span>
                  </label>
                </div>
              </div>

              <DialogFooter className="pt-2">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setTplDialogOpen(false)}
                  className="border-zinc-700 text-zinc-300"
                >
                  取消
                </Button>
                <Button
                  type="button"
                  className="bg-indigo-600 hover:bg-indigo-500"
                  onClick={submitTemplate}
                  disabled={tplSubmitting}
                  isLoading={tplSubmitting}
                >
                  {editingCode ? '保存修改' : '创建模板'}
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </>
      )}
    </div>
  )
}
