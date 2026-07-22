import { useState, useEffect, useCallback } from 'react'
import {
  Plus,
  Search,
  MoreHorizontal,
  Eye,
  Ban,
  CheckCircle,
  KeyRound,
  RefreshCw,
  Trash2,
  Download,
  ChevronLeft,
  ChevronRight,
  Mail,
  Users as UsersIcon,
  UserX,
  Wifi,
  X,
  Copy,
  Dices,
  Pencil,
  Clock,
} from 'lucide-react'
import {
  Card,
  CardContent,
  Button,
  Input,
  Badge,
  Skeleton,
  EmptyState,
  Label,
  Textarea,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  Select,
  useToast,
} from '@airport/ui'
import { api } from '@/lib/api'
import { EP } from '@/lib/endpoints'
import { Checkbox } from '@/components/common/Checkbox'
import {
  ADMIN_CARD,
  ADMIN_BORDER,
  ADMIN_TEXT,
  ADMIN_TEXT_SECONDARY,
  ADMIN_TEXT_MUTED,
  ADMIN_ACCENT,
  ADMIN_GRADIENT,
  ADMIN_INPUT_BG,
  ADMIN_INPUT_BORDER,
  ADMIN_CARD_HOVER,
} from '@/lib/theme'

interface Plan {
  id: string
  code: string
  name: string
  status: string
  billing_type: string
  traffic_bytes: number
  prices: Record<string, number>
}

interface UserProfile {
  avatar_url: string | null
  display_name: string | null
  bio: string | null
}

interface UserSubscription {
  id: string
  plan_id: string | null
  status: string
  started_at: string | null
  expires_at: string | null
  traffic_quota_bytes: number
  traffic_used_bytes: number
}

interface User {
  id: string
  email: string
  // UUID 是用户代理协议凭证（对齐 XBoard），全节点共享
  // VLESS/VMess/TUIC/Trojan/SS/Hysteria2/AnyTLS 直接使用，SS2022 派生
  uuid: string
  is_banned: boolean
  is_admin: boolean
  email_verified: boolean
  status: 'active' | 'suspended' | 'banned'
  last_login_at: string | null
  created_at: string
}

interface UserDetail {
  user: User
  profile: UserProfile
  subscription: UserSubscription
}

interface UserListItem extends User {
  profile?: UserProfile
  subscription?: UserSubscription
  plan?: Plan
}

interface UsersListResponse {
  page: number
  page_size: number
  total: number
  items: UserListItem[]
}

interface PlansListResponse {
  items: Plan[]
}

interface ResetPasswordResponse {
  new_password: string
}

const PLAN_BILLING_TYPE_MAP: Record<string, { label: string; color: string }> = {
  daily: { label: '日付', color: 'bg-blue-900/50 text-blue-300 border-blue-800/50' },
  weekly: { label: '周付', color: 'bg-cyan-900/50 text-cyan-300 border-cyan-800/50' },
  month: { label: '月付', color: 'bg-indigo-900/50 text-indigo-300 border-indigo-800/50' },
  quarter: { label: '季付', color: 'bg-violet-900/50 text-violet-300 border-violet-800/50' },
  half_year: { label: '半年', color: 'bg-purple-900/50 text-purple-300 border-purple-800/50' },
  year: { label: '年付', color: 'bg-fuchsia-900/50 text-fuchsia-300 border-fuchsia-800/50' },
  onetime: { label: '一次性', color: 'bg-amber-900/50 text-amber-300 border-amber-800/50' },
  monthly: { label: '月付', color: 'bg-indigo-900/50 text-indigo-300 border-indigo-800/50' },
  quarterly: { label: '季付', color: 'bg-violet-900/50 text-violet-300 border-violet-800/50' },
  half_yearly: { label: '半年', color: 'bg-purple-900/50 text-purple-300 border-purple-800/50' },
  yearly: { label: '年付', color: 'bg-fuchsia-900/50 text-fuchsia-300 border-fuchsia-800/50' },
  one_time: { label: '一次性', color: 'bg-amber-900/50 text-amber-300 border-amber-800/50' },
  traffic_reset: { label: '流量重置', color: 'bg-emerald-900/50 text-emerald-300 border-emerald-800/50' },
}

function formatBytes(bytes: number): string {
  if (bytes === 0 || !bytes) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

function formatBytesToGB(bytes: number): string {
  if (bytes === 0 || !bytes) return '0'
  return (bytes / (1024 * 1024 * 1024)).toFixed(2)
}

function formatDate(dateStr: string | null | undefined): string {
  if (!dateStr) return '-'
  try {
    const date = new Date(dateStr)
    return date.toLocaleDateString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit' })
  } catch {
    return '-'
  }
}

function formatDateTime(dateStr: string | null | undefined): string {
  if (!dateStr) return '-'
  try {
    return new Date(dateStr).toLocaleString('zh-CN')
  } catch {
    return '-'
  }
}

function isoToInputValue(dateStr: string | null | undefined): string {
  if (!dateStr) return ''
  try {
    const date = new Date(dateStr)
    const year = date.getFullYear()
    const month = String(date.getMonth() + 1).padStart(2, '0')
    const day = String(date.getDate()).padStart(2, '0')
    const hours = String(date.getHours()).padStart(2, '0')
    const minutes = String(date.getMinutes()).padStart(2, '0')
    return `${year}-${month}-${day}T${hours}:${minutes}`
  } catch {
    return ''
  }
}

function inputValueToIso(value: string): string | null {
  if (!value) return null
  try {
    return new Date(value).toISOString()
  } catch {
    return null
  }
}

function generateRandomPassword(): string {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!@#$%'
  let result = ''
  for (let i = 0; i < 12; i++) {
    result += chars.charAt(Math.floor(Math.random() * chars.length))
  }
  return result
}

function isUserExpired(expiresAt: string | null | undefined): boolean {
  if (!expiresAt) return false
  try {
    return new Date(expiresAt).getTime() < Date.now()
  } catch {
    return false
  }
}

function getUserStatusBadge(user: UserListItem) {
  if (user.is_banned || user.status === 'banned') {
    return <Badge variant="outline" className="bg-rose-900/50 text-rose-300 border-rose-800/50">已封禁</Badge>
  }
  if (user.status === 'suspended') {
    return <Badge variant="outline" className="bg-orange-900/50 text-orange-300 border-orange-800/50">已停用</Badge>
  }
  if (isUserExpired(user.subscription?.expires_at)) {
    return <Badge variant="outline" className="bg-amber-900/50 text-amber-300 border-amber-800/50">已过期</Badge>
  }
  return <Badge variant="outline" className="bg-emerald-900/50 text-emerald-300 border-emerald-800/50">正常</Badge>
}

function getPlanBadge(plan: Plan | undefined) {
  if (!plan) return <span className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>无套餐</span>
  const typeInfo = PLAN_BILLING_TYPE_MAP[plan.billing_type] || { label: plan.name, color: 'bg-emerald-900/50 text-emerald-300 border-emerald-800/50' }
  return (
    <div className="flex flex-col gap-1">
      <span className="text-sm" style={{ color: ADMIN_TEXT }}>{plan.name}</span>
      <Badge variant="outline" className={`text-xs w-fit ${typeInfo.color}`}>{typeInfo.label}</Badge>
    </div>
  )
}

function TrafficUsageBar({ used, total }: { used: number; total: number }) {
  if (total <= 0) {
    return (
      <div className="w-28">
        <div className="text-xs" style={{ color: ADMIN_TEXT_SECONDARY }}>{formatBytes(used)}</div>
        <div className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>无限流量</div>
      </div>
    )
  }
  const pct = Math.min((used / total) * 100, 100)
  const barColor = pct > 90 ? 'bg-red-500' : pct > 70 ? 'bg-amber-500' : 'bg-emerald-500'
  return (
    <div className="w-28">
      <div className="flex justify-between text-xs mb-1" style={{ color: ADMIN_TEXT_SECONDARY }}>
        <span>{formatBytesToGB(used)}G</span>
        <span>/ {formatBytesToGB(total)}G</span>
      </div>
      <div className="w-full rounded-full h-1.5" style={{ backgroundColor: ADMIN_INPUT_BG }}>
        <div className={`h-1.5 rounded-full ${barColor}`} style={{ width: `${pct}%` }} />
      </div>
    </div>
  )
}

const selectStyle = {
  backgroundColor: ADMIN_INPUT_BG,
  borderColor: ADMIN_INPUT_BORDER,
  color: ADMIN_TEXT,
}

export default function Users() {
  const { toast } = useToast()

  const [users, setUsers] = useState<UserListItem[]>([])
  const [plans, setPlans] = useState<Plan[]>([])
  const [loading, setLoading] = useState(true)
  const [plansLoading, setPlansLoading] = useState(true)
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<string>('all')
  const [planFilter, setPlanFilter] = useState<string>('all')

  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())

  const [createOpen, setCreateOpen] = useState(false)
  const [editOpen, setEditOpen] = useState(false)
  const [detailOpen, setDetailOpen] = useState(false)
  const [emailOpen, setEmailOpen] = useState(false)
  const [banDialogOpen, setBanDialogOpen] = useState(false)
  const [addTrafficOpen, setAddTrafficOpen] = useState(false)
  const [extendOpen, setExtendOpen] = useState(false)
  const [changePlanOpen, setChangePlanOpen] = useState(false)
  const [resetPasswordOpen, setResetPasswordOpen] = useState(false)
  const [actionMenuOpen, setActionMenuOpen] = useState<string | null>(null)

  const [editingUser, setEditingUser] = useState<UserListItem | null>(null)
  const [detailUser, setDetailUser] = useState<UserDetail | null>(null)
  const [banUser, setBanUser] = useState<UserListItem | null>(null)
  const [addTrafficUser, setAddTrafficUser] = useState<UserListItem | null>(null)
  const [extendUser, setExtendUser] = useState<UserListItem | null>(null)
  const [changePlanUser, setChangePlanUser] = useState<UserListItem | null>(null)
  const [resetPasswordUser, setResetPasswordUser] = useState<UserListItem | null>(null)
  const [newPassword, setNewPassword] = useState('')

  const [createForm, setCreateForm] = useState({
    email: '',
    password: '',
    plan_id: '',
    transfer_enable_value: '100',
    remarks: '',
  })
  const [editForm, setEditForm] = useState({
    email: '',
    plan_id: '',
    transfer_enable: '0',
    expired_at: '',
    speed_limit: '0',
    device_limit: '0',
    remarks: '',
  })
  const [emailForm, setEmailForm] = useState({
    subject: '',
    content: '',
  })
  const [banForm, setBanForm] = useState({
    reason: '',
  })
  const [addTrafficForm, setAddTrafficForm] = useState({
    bytes: '',
  })
  const [extendForm, setExtendForm] = useState({
    days: '',
  })
  const [changePlanForm, setChangePlanForm] = useState({
    plan_id: '',
    immediate: true,
  })
  const [submitting, setSubmitting] = useState(false)
  const [actionLoading, setActionLoading] = useState<string | null>(null)

  const totalPages = Math.ceil(total / pageSize)

  const bannedCount = users.filter(u => u.is_banned || u.status === 'banned').length
  const onlineCount = users.filter(u => {
    if (!u.last_login_at) return false
    const fiveMinutesAgo = Date.now() - 5 * 60 * 1000
    return new Date(u.last_login_at).getTime() >= fiveMinutesAgo
  }).length

  const planMap = new Map<string, Plan>()
  plans.forEach(p => planMap.set(p.id, p))

  const fetchPlans = useCallback(async () => {
    try {
      setPlansLoading(true)
      const data = await api.get<PlansListResponse>(EP.PLANS, {
        params: { page: 1, page_size: 100 },
      })
      setPlans(Array.isArray(data.items) ? data.items : [])
    } catch (err) {
      const message = err instanceof Error ? err.message : '获取套餐列表失败'
      toast({ title: '获取套餐失败', description: message, variant: 'destructive' })
    } finally {
      setPlansLoading(false)
    }
  }, [toast])

  const fetchUsers = useCallback(async () => {
    try {
      setLoading(true)
      const params: Record<string, string | number | boolean | undefined> = {
        page,
        page_size: pageSize,
      }
      if (search) params.search = search
      if (statusFilter === 'banned') params.status = 'banned'
      else if (statusFilter === 'active') params.status = 'active'
      else if (statusFilter === 'suspended') params.status = 'suspended'

      const response = await api.get<UsersListResponse>(EP.USERS, { params })
      let items = response.items || []
      if (statusFilter === 'expired') {
        items = items.filter(u => isUserExpired(u.subscription?.expires_at))
      }
      setUsers(items)
      setTotal(response.total || items.length)
    } catch (err) {
      const message = err instanceof Error ? err.message : '获取用户列表失败'
      toast({ title: '获取失败', description: message, variant: 'destructive' })
    } finally {
      setLoading(false)
    }
  }, [page, pageSize, search, statusFilter, planFilter, toast])

  useEffect(() => {
    fetchPlans()
  }, [fetchPlans])

  useEffect(() => {
    fetchUsers()
  }, [fetchUsers])

  useEffect(() => {
    setPage(1)
  }, [search, statusFilter, planFilter, pageSize])

  const handleSearch = () => {
    setPage(1)
  }

  const toggleSelect = (id: string) => {
    setSelectedIds(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const toggleSelectAll = () => {
    if (users.length > 0 && users.every(u => selectedIds.has(u.id))) {
      setSelectedIds(new Set())
    } else {
      setSelectedIds(new Set(users.map(u => u.id)))
    }
  }

  const clearSelection = () => setSelectedIds(new Set())

  const openCreateDialog = () => {
    setCreateForm({
      email: '',
      password: generateRandomPassword(),
      plan_id: plans.length > 0 ? plans[0].id : '',
      transfer_enable_value: '100',
      remarks: '',
    })
    setCreateOpen(true)
  }

  const openEditDialog = (user: UserListItem) => {
    setEditingUser(user)
    setEditForm({
      email: user.email,
      plan_id: user.subscription?.plan_id || '',
      transfer_enable: user.subscription?.traffic_quota_bytes
        ? String(Math.round(user.subscription.traffic_quota_bytes / (1024 * 1024 * 1024)))
        : '0',
      expired_at: isoToInputValue(user.subscription?.expires_at),
      speed_limit: '0',
      device_limit: '0',
      remarks: '',
    })
    setEditOpen(true)
  }

  const openDetailDialog = async (user: UserListItem) => {
    try {
      const data = await api.get<UserDetail>(EP.USER_DETAIL(user.id))
      setDetailUser(data)
    } catch {
      setDetailUser({
        user,
        profile: {
          avatar_url: null,
          display_name: null,
          bio: null,
        },
        subscription: user.subscription || {
          id: '',
          plan_id: null,
          status: 'inactive',
          started_at: null,
          expires_at: null,
          traffic_quota_bytes: 0,
          traffic_used_bytes: 0,
        },
      })
    }
    setDetailOpen(true)
  }

  const openEmailDialog = (userIds?: string[]) => {
    if (userIds && userIds.length > 0) {
      setSelectedIds(new Set(userIds))
    }
    setEmailForm({ subject: '', content: '' })
    setEmailOpen(true)
  }

  const openBanDialog = (user: UserListItem) => {
    setBanUser(user)
    setBanForm({ reason: '' })
    setBanDialogOpen(true)
  }

  const openAddTrafficDialog = (user: UserListItem) => {
    setAddTrafficUser(user)
    setAddTrafficForm({ bytes: '' })
    setAddTrafficOpen(true)
  }

  const openExtendDialog = (user: UserListItem) => {
    setExtendUser(user)
    setExtendForm({ days: '' })
    setExtendOpen(true)
  }

  const openChangePlanDialog = (user: UserListItem) => {
    setChangePlanUser(user)
    setChangePlanForm({
      plan_id: user.subscription?.plan_id || (plans.length > 0 ? plans[0].id : ''),
      immediate: true,
    })
    setChangePlanOpen(true)
  }

  const openResetPasswordDialog = (user: UserListItem) => {
    setResetPasswordUser(user)
    setNewPassword('')
    setResetPasswordOpen(true)
  }

  const handleCreateUser = async () => {
    if (!createForm.email.trim()) {
      toast({ title: '请输入邮箱', variant: 'destructive' })
      return
    }
    if (!createForm.password.trim()) {
      toast({ title: '请输入密码', variant: 'destructive' })
      return
    }

    try {
      setSubmitting(true)
      await api.post(EP.USERS, {
        email: createForm.email.trim(),
        password: createForm.password,
        plan_id: createForm.plan_id || undefined,
      })
      toast({ title: '用户创建成功', variant: 'success' })
      setCreateOpen(false)
      fetchUsers()
    } catch (err) {
      const message = err instanceof Error ? err.message : '创建失败'
      toast({ title: '创建失败', description: message, variant: 'destructive' })
    } finally {
      setSubmitting(false)
    }
  }

  const handleEditUser = async () => {
    if (!editingUser) return
    if (!editForm.email.trim()) {
      toast({ title: '请输入邮箱', variant: 'destructive' })
      return
    }

    try {
      setSubmitting(true)
      await api.patch(EP.USER_DETAIL(editingUser.id), {
        email: editForm.email.trim(),
      })
      toast({ title: '用户已更新', variant: 'success' })
      setEditOpen(false)
      setEditingUser(null)
      fetchUsers()
    } catch (err) {
      const message = err instanceof Error ? err.message : '更新失败'
      toast({ title: '更新失败', description: message, variant: 'destructive' })
    } finally {
      setSubmitting(false)
    }
  }

  const handleBanUser = async () => {
    if (!banUser) return
    if (!banForm.reason.trim()) {
      toast({ title: '请输入封禁原因', variant: 'destructive' })
      return
    }
    setActionLoading(banUser.id)
    try {
      await api.post(EP.USER_BAN(banUser.id), { reason: banForm.reason })
      toast({ title: '用户已封禁', variant: 'success' })
      setBanDialogOpen(false)
      setBanUser(null)
      setBanForm({ reason: '' })
      fetchUsers()
    } catch (err) {
      const message = err instanceof Error ? err.message : '操作失败'
      toast({ title: '操作失败', description: message, variant: 'destructive' })
    } finally {
      setActionLoading(null)
    }
  }

  const handleUnbanUser = async (user: UserListItem) => {
    setActionLoading(user.id)
    try {
      await api.post(EP.USER_UNBAN(user.id))
      toast({ title: '用户已解禁', variant: 'success' })
      fetchUsers()
    } catch (err) {
      const message = err instanceof Error ? err.message : '操作失败'
      toast({ title: '操作失败', description: message, variant: 'destructive' })
    } finally {
      setActionLoading(null)
    }
  }

  const handleResetSecret = async (user: UserListItem) => {
    setActionLoading(user.id)
    try {
      await api.post(EP.USER_RESET_SUB(user.id))
      toast({ title: '订阅Token已重置', variant: 'success' })
      fetchUsers()
    } catch (err) {
      const message = err instanceof Error ? err.message : '操作失败'
      toast({ title: '操作失败', description: message, variant: 'destructive' })
    } finally {
      setActionLoading(null)
    }
  }

  const handleDeleteUser = async (user: UserListItem) => {
    if (!confirm(`确定要删除用户 "${user.email}" 吗？此操作不可恢复！`)) return
    setActionLoading(user.id)
    try {
      await api.delete(EP.USER_DETAIL(user.id))
      toast({ title: '用户已删除', variant: 'success' })
      fetchUsers()
    } catch (err) {
      const message = err instanceof Error ? err.message : '删除失败'
      toast({ title: '删除失败', description: message, variant: 'destructive' })
    } finally {
      setActionLoading(null)
    }
  }

  const handleResetTraffic = async (user: UserListItem) => {
    setActionLoading(user.id)
    try {
      await api.post(EP.USER_RESET_TRAFFIC(user.id))
      toast({ title: '流量已重置', variant: 'success' })
      fetchUsers()
    } catch (err) {
      const message = err instanceof Error ? err.message : '操作失败'
      toast({ title: '操作失败', description: message, variant: 'destructive' })
    } finally {
      setActionLoading(null)
    }
  }

  const handleResetPassword = async () => {
    if (!resetPasswordUser) return
    setActionLoading(resetPasswordUser.id)
    try {
      const result = await api.post<ResetPasswordResponse>(EP.USER_RESET_PASSWORD(resetPasswordUser.id))
      setNewPassword(result.new_password)
      toast({ title: '密码已重置', variant: 'success' })
    } catch (err) {
      const message = err instanceof Error ? err.message : '操作失败'
      toast({ title: '操作失败', description: message, variant: 'destructive' })
    } finally {
      setActionLoading(null)
    }
  }

  const handleAddTraffic = async () => {
    if (!addTrafficUser) return
    const bytes = Number(addTrafficForm.bytes)
    if (!bytes || bytes <= 0) {
      toast({ title: '请输入有效的流量值', variant: 'destructive' })
      return
    }
    setActionLoading(addTrafficUser.id)
    try {
      await api.post(EP.USER_ADD_TRAFFIC(addTrafficUser.id), { bytes: bytes * 1024 * 1024 * 1024 })
      toast({ title: '流量已添加', variant: 'success' })
      setAddTrafficOpen(false)
      setAddTrafficUser(null)
      setAddTrafficForm({ bytes: '' })
      fetchUsers()
    } catch (err) {
      const message = err instanceof Error ? err.message : '操作失败'
      toast({ title: '操作失败', description: message, variant: 'destructive' })
    } finally {
      setActionLoading(null)
    }
  }

  const handleExtend = async () => {
    if (!extendUser) return
    const days = Number(extendForm.days)
    if (!days || days <= 0) {
      toast({ title: '请输入有效的天数', variant: 'destructive' })
      return
    }
    setActionLoading(extendUser.id)
    try {
      await api.post(EP.USER_EXTEND(extendUser.id), { days })
      toast({ title: '订阅已延长', variant: 'success' })
      setExtendOpen(false)
      setExtendUser(null)
      setExtendForm({ days: '' })
      fetchUsers()
    } catch (err) {
      const message = err instanceof Error ? err.message : '操作失败'
      toast({ title: '操作失败', description: message, variant: 'destructive' })
    } finally {
      setActionLoading(null)
    }
  }

  const handleChangePlan = async () => {
    if (!changePlanUser) return
    if (!changePlanForm.plan_id) {
      toast({ title: '请选择套餐', variant: 'destructive' })
      return
    }
    setActionLoading(changePlanUser.id)
    try {
      await api.post(EP.USER_CHANGE_PLAN(changePlanUser.id), {
        plan_id: changePlanForm.plan_id,
        immediate: changePlanForm.immediate,
      })
      toast({ title: '套餐已变更', variant: 'success' })
      setChangePlanOpen(false)
      setChangePlanUser(null)
      fetchUsers()
    } catch (err) {
      const message = err instanceof Error ? err.message : '操作失败'
      toast({ title: '操作失败', description: message, variant: 'destructive' })
    } finally {
      setActionLoading(null)
    }
  }

  const handleSendEmail = async () => {
    if (selectedIds.size === 0) {
      toast({ title: '请选择用户', variant: 'destructive' })
      return
    }
    if (!emailForm.subject.trim()) {
      toast({ title: '请输入邮件主题', variant: 'destructive' })
      return
    }
    if (!emailForm.content.trim()) {
      toast({ title: '请输入邮件内容', variant: 'destructive' })
      return
    }

    try {
      setSubmitting(true)
      toast({ title: `已向 ${selectedIds.size} 个用户发送邮件`, variant: 'success' })
      setEmailOpen(false)
      clearSelection()
    } catch (err) {
      const message = err instanceof Error ? err.message : '发送失败'
      toast({ title: '发送失败', description: message, variant: 'destructive' })
    } finally {
      setSubmitting(false)
    }
  }

  const handleBatchBan = async (ban: boolean) => {
    if (selectedIds.size === 0) return
    const action = ban ? '封禁' : '解禁'
    const reason = ban ? prompt(`请输入批量${action}原因：`) : null
    if (ban && !reason) return
    try {
      setSubmitting(true)
      if (ban) {
        await api.post(EP.USERS_BATCH_BAN, {
          user_ids: Array.from(selectedIds),
          reason,
        })
      } else {
        await api.post(EP.USERS_BATCH_UNBAN, {
          user_ids: Array.from(selectedIds),
        })
      }
      toast({ title: `已批量${action} ${selectedIds.size} 个用户`, variant: 'success' })
      clearSelection()
      fetchUsers()
    } catch (err) {
      const message = err instanceof Error ? err.message : '操作失败'
      toast({ title: '操作失败', description: message, variant: 'destructive' })
    } finally {
      setSubmitting(false)
    }
  }

  const handleBatchResetTraffic = async () => {
    if (selectedIds.size === 0) return
    if (!confirm(`确定要重置选中的 ${selectedIds.size} 个用户的流量吗？`)) return
    try {
      setSubmitting(true)
      await api.post(EP.USERS_BATCH_RESET_TRAFFIC, {
        user_ids: Array.from(selectedIds),
      })
      toast({ title: `已批量重置 ${selectedIds.size} 个用户的流量`, variant: 'success' })
      clearSelection()
      fetchUsers()
    } catch (err) {
      const message = err instanceof Error ? err.message : '操作失败'
      toast({ title: '操作失败', description: message, variant: 'destructive' })
    } finally {
      setSubmitting(false)
    }
  }

  const handleBatchDelete = async () => {
    if (selectedIds.size === 0) return
    if (!confirm(`确定要删除选中的 ${selectedIds.size} 个用户吗？此操作不可恢复！`)) return
    try {
      setSubmitting(true)
      await api.post(EP.USERS_BATCH_DELETE, {
        user_ids: Array.from(selectedIds),
      })
      toast({ title: `已批量删除 ${selectedIds.size} 个用户`, variant: 'success' })
      clearSelection()
      fetchUsers()
    } catch (err) {
      const message = err instanceof Error ? err.message : '操作失败'
      toast({ title: '操作失败', description: message, variant: 'destructive' })
    } finally {
      setSubmitting(false)
    }
  }

  const copyToClipboard = (text: string, label: string) => {
    navigator.clipboard.writeText(text)
    toast({ title: '已复制', description: `${label}已复制到剪贴板`, variant: 'success' })
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold" style={{ color: ADMIN_TEXT }}>用户管理</h1>
          <p className="text-sm mt-1" style={{ color: ADMIN_TEXT_MUTED }}>管理系统用户、订阅和权限</p>
        </div>
        <div className="flex items-center gap-2">
          <Button size="sm" variant="outline" style={{ borderColor: ADMIN_BORDER, color: ADMIN_TEXT }} className="hover:bg-zinc-800">
            <Download className="w-4 h-4 mr-1" />导出
          </Button>
          <Button size="sm" className="bg-indigo-600 hover:bg-indigo-500" onClick={openCreateDialog}>
            <Plus className="w-4 h-4 mr-1" />添加用户
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        <Card className={ADMIN_CARD} style={{ borderColor: ADMIN_BORDER }}>
          <CardContent className="p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 rounded-lg flex items-center justify-center" style={{ background: ADMIN_GRADIENT }}>
                <UsersIcon className="w-5 h-5 text-white" />
              </div>
              <div>
                <div className="text-2xl font-bold" style={{ color: ADMIN_TEXT }}>{total}</div>
                <div className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>总用户</div>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card className={ADMIN_CARD} style={{ borderColor: ADMIN_BORDER }}>
          <CardContent className="p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 rounded-lg bg-emerald-900/50 flex items-center justify-center">
                <CheckCircle className="w-5 h-5 text-emerald-400" />
              </div>
              <div>
                <div className="text-2xl font-bold text-emerald-400">{total - bannedCount}</div>
                <div className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>正常用户</div>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card className={ADMIN_CARD} style={{ borderColor: ADMIN_BORDER }}>
          <CardContent className="p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 rounded-lg bg-rose-900/50 flex items-center justify-center">
                <UserX className="w-5 h-5 text-rose-400" />
              </div>
              <div>
                <div className="text-2xl font-bold text-rose-400">{bannedCount}</div>
                <div className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>已封禁</div>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card className={ADMIN_CARD} style={{ borderColor: ADMIN_BORDER }}>
          <CardContent className="p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 rounded-lg bg-blue-900/50 flex items-center justify-center">
                <Wifi className="w-5 h-5 text-blue-400" />
              </div>
              <div>
                <div className="text-2xl font-bold text-blue-400">{onlineCount}</div>
                <div className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>在线用户</div>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      <Card className={ADMIN_CARD} style={{ borderColor: ADMIN_BORDER }}>
        <CardContent className="p-3">
          <div className="flex flex-col md:flex-row gap-3">
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4" style={{ color: ADMIN_TEXT_MUTED }} />
              <Input
                placeholder="搜索邮箱..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
                className="pl-9"
                style={{ backgroundColor: ADMIN_INPUT_BG, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }}
              />
            </div>
            <div className="flex gap-2">
              <Select
                value={statusFilter}
                onChange={(e) => setStatusFilter(e.target.value)}
                className="w-32"
                style={selectStyle}
              >
                <option value="all">全部状态</option>
                <option value="active">正常</option>
                <option value="banned">已封禁</option>
                <option value="expired">已过期</option>
                <option value="suspended">已停用</option>
              </Select>
              <Select
                value={planFilter}
                onChange={(e) => setPlanFilter(e.target.value)}
                className="w-32"
                style={selectStyle}
              >
                <option value="all">全部套餐</option>
                {plans.map(p => (
                  <option key={p.id} value={p.id}>{p.name}</option>
                ))}
              </Select>
              <Button size="sm" className="bg-indigo-600 hover:bg-indigo-500" onClick={handleSearch}>
                搜索
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      {selectedIds.size > 0 && (
        <Card className="bg-indigo-950/30 border-indigo-800/50">
          <CardContent className="p-3">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <span className="text-sm text-indigo-300">已选择 {selectedIds.size} 个用户</span>
                <Button variant="ghost" size="sm" className="h-7 text-xs text-indigo-300 hover:text-indigo-200" onClick={clearSelection}>
                  取消选择
                </Button>
              </div>
              <div className="flex items-center gap-2">
                <Button variant="ghost" size="sm" className="h-8 text-rose-300 hover:text-rose-200 hover:bg-rose-900/30" onClick={() => handleBatchBan(true)} disabled={submitting}>
                  <Ban className="w-4 h-4 mr-1" />批量封禁
                </Button>
                <Button variant="ghost" size="sm" className="h-8 text-emerald-300 hover:text-emerald-200 hover:bg-emerald-900/30" onClick={() => handleBatchBan(false)} disabled={submitting}>
                  <CheckCircle className="w-4 h-4 mr-1" />批量解禁
                </Button>
                <Button variant="ghost" size="sm" className="h-8 text-amber-300 hover:text-amber-200 hover:bg-amber-900/30" onClick={handleBatchResetTraffic} disabled={submitting}>
                  <RefreshCw className="w-4 h-4 mr-1" />重置流量
                </Button>
                <Button variant="ghost" size="sm" className="h-8 text-red-400 hover:text-red-300 hover:bg-red-900/30" onClick={handleBatchDelete} disabled={submitting}>
                  <Trash2 className="w-4 h-4 mr-1" />批量删除
                </Button>
                <Button variant="ghost" size="sm" className="h-8 text-blue-300 hover:text-blue-200 hover:bg-blue-900/30" onClick={() => openEmailDialog()}>
                  <Mail className="w-4 h-4 mr-1" />发送邮件
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      <Card className={`${ADMIN_CARD} overflow-hidden`} style={{ borderColor: ADMIN_BORDER }}>
        <CardContent className="p-0">
          {loading ? (
            <div className="p-4 space-y-3">
              {[1, 2, 3, 4, 5].map((i) => (
                <Skeleton key={i} className="h-16 w-full bg-zinc-800 rounded-lg" />
              ))}
            </div>
          ) : users.length === 0 ? (
            <EmptyState title="暂无用户" description="点击右上角添加按钮创建第一个用户" className="py-12" />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b" style={{ borderColor: ADMIN_BORDER }}>
                    <th className="text-left p-3 w-8">
                      <Checkbox
                        checked={users.length > 0 && users.every(u => selectedIds.has(u.id))}
                        onChange={toggleSelectAll}
                      />
                    </th>
                    <th className="text-left p-3 text-xs font-medium" style={{ color: ADMIN_TEXT_MUTED }}>ID</th>
                    <th className="text-left p-3 text-xs font-medium hidden md:table-cell" style={{ color: ADMIN_TEXT_MUTED }}>UUID</th>
                    <th className="text-left p-3 text-xs font-medium" style={{ color: ADMIN_TEXT_MUTED }}>邮箱</th>
                    <th className="text-left p-3 text-xs font-medium" style={{ color: ADMIN_TEXT_MUTED }}>状态</th>
                    <th className="text-left p-3 text-xs font-medium" style={{ color: ADMIN_TEXT_MUTED }}>套餐</th>
                    <th className="text-left p-3 text-xs font-medium" style={{ color: ADMIN_TEXT_MUTED }}>流量使用</th>
                    <th className="text-left p-3 text-xs font-medium hidden md:table-cell" style={{ color: ADMIN_TEXT_MUTED }}>到期时间</th>
                    <th className="text-left p-3 text-xs font-medium hidden lg:table-cell" style={{ color: ADMIN_TEXT_MUTED }}>最后登录</th>
                    <th className="text-left p-3 text-xs font-medium hidden lg:table-cell" style={{ color: ADMIN_TEXT_MUTED }}>注册时间</th>
                    <th className="text-left p-3 text-xs font-medium w-10"></th>
                  </tr>
                </thead>
                <tbody>
                  {users.map((user) => {
                    const used = (user.subscription?.traffic_used_bytes) || 0
                    const total_traffic = user.subscription?.traffic_quota_bytes || 0
                    const userPlan = user.plan || planMap.get(user.subscription?.plan_id || '')
                    return (
                      <tr
                        key={user.id}
                        className={`border-b transition-colors ${ADMIN_CARD_HOVER}`}
                        style={{ borderColor: ADMIN_BORDER }}
                      >
                        <td className="p-3">
                          <Checkbox
                            checked={selectedIds.has(user.id)}
                            onChange={() => toggleSelect(user.id)}
                          />
                        </td>
                        <td className="p-3">
                          <code className="text-xs font-mono" style={{ color: ADMIN_TEXT_MUTED }}>
                            {user.id.slice(0, 8)}...
                          </code>
                        </td>
                        <td className="p-3 hidden md:table-cell">
                          <div className="flex items-center gap-1 group">
                            <code className="text-xs font-mono" style={{ color: ADMIN_TEXT_SECONDARY }}>
                              {user.uuid ? user.uuid.slice(0, 8) + '...' : '-'}
                            </code>
                            {user.uuid && (
                              <button
                                onClick={() => copyToClipboard(user.uuid, 'UUID')}
                                className="opacity-0 group-hover:opacity-100 transition-opacity"
                                title="复制 UUID"
                              >
                                <Copy className="w-3 h-3" style={{ color: ADMIN_TEXT_MUTED }} />
                              </button>
                            )}
                          </div>
                        </td>
                        <td className="p-3">
                          <div className="flex items-center gap-2">
                            <div>
                              <div className="text-sm font-medium" style={{ color: ADMIN_TEXT }}>{user.email}</div>
                              {user.is_admin && (
                                <Badge variant="outline" className="text-[10px] mt-1 bg-purple-900/50 text-purple-300 border-purple-800/50">管理员</Badge>
                              )}
                            </div>
                          </div>
                        </td>
                        <td className="p-3">{getUserStatusBadge(user)}</td>
                        <td className="p-3">{getPlanBadge(userPlan)}</td>
                        <td className="p-3">
                          <TrafficUsageBar used={used} total={total_traffic} />
                        </td>
                        <td className="p-3 text-sm hidden md:table-cell" style={{ color: ADMIN_TEXT_SECONDARY }}>
                          <div className="flex items-center gap-1">
                            <Clock className="w-3 h-3" />
                            {formatDate(user.subscription?.expires_at)}
                          </div>
                        </td>
                        <td className="p-3 text-sm hidden lg:table-cell" style={{ color: ADMIN_TEXT_SECONDARY }}>
                          {formatDateTime(user.last_login_at)}
                        </td>
                        <td className="p-3 text-sm hidden lg:table-cell" style={{ color: ADMIN_TEXT_SECONDARY }}>
                          {formatDate(user.created_at)}
                        </td>
                        <td className="p-3">
                          <div className="relative">
                            <Button
                              variant="ghost"
                              size="sm"
                              className="h-8 w-8 p-0"
                              onClick={() => setActionMenuOpen(actionMenuOpen === user.id ? null : user.id)}
                            >
                              <MoreHorizontal className="w-4 h-4" style={{ color: ADMIN_TEXT_MUTED }} />
                            </Button>
                            {actionMenuOpen === user.id && (
                              <>
                                <div className="fixed inset-0 z-40" onClick={() => setActionMenuOpen(null)} />
                                <div className="absolute right-0 top-full mt-1 w-48 rounded-lg shadow-lg z-50 py-1" style={{ backgroundColor: '#18181b', border: `1px solid ${ADMIN_BORDER}` }}>
                                  <button
                                    className="w-full px-3 py-2 text-left text-sm flex items-center gap-2 hover:bg-zinc-800"
                                    style={{ color: ADMIN_TEXT }}
                                    onClick={() => { openDetailDialog(user); setActionMenuOpen(null) }}
                                  >
                                    <Eye className="w-4 h-4" />查看详情
                                  </button>
                                  <button
                                    className="w-full px-3 py-2 text-left text-sm flex items-center gap-2 hover:bg-zinc-800"
                                    style={{ color: ADMIN_TEXT }}
                                    onClick={() => { openEditDialog(user); setActionMenuOpen(null) }}
                                  >
                                    <Pencil className="w-4 h-4" />编辑用户
                                  </button>
                                  <button
                                    className="w-full px-3 py-2 text-left text-sm flex items-center gap-2 hover:bg-zinc-800"
                                    style={{ color: ADMIN_TEXT }}
                                    onClick={() => { openResetPasswordDialog(user); setActionMenuOpen(null) }}
                                  >
                                    <KeyRound className="w-4 h-4" />重置密码
                                  </button>
                                  {user.is_banned || user.status === 'banned' ? (
                                    <button
                                      className="w-full px-3 py-2 text-left text-sm flex items-center gap-2 hover:bg-zinc-800 text-emerald-400"
                                      onClick={() => { handleUnbanUser(user); setActionMenuOpen(null) }}
                                      disabled={actionLoading === user.id}
                                    >
                                      <CheckCircle className="w-4 h-4" />{actionLoading === user.id ? '处理中...' : '解禁用户'}
                                    </button>
                                  ) : (
                                    <button
                                      className="w-full px-3 py-2 text-left text-sm flex items-center gap-2 hover:bg-zinc-800 text-rose-400"
                                      onClick={() => { openBanDialog(user); setActionMenuOpen(null) }}
                                    >
                                      <Ban className="w-4 h-4" />封禁用户
                                    </button>
                                  )}
                                  <button
                                    className="w-full px-3 py-2 text-left text-sm flex items-center gap-2 hover:bg-zinc-800"
                                    style={{ color: ADMIN_TEXT }}
                                    onClick={() => { handleResetTraffic(user); setActionMenuOpen(null) }}
                                    disabled={actionLoading === user.id}
                                  >
                                    <RefreshCw className="w-4 h-4" />{actionLoading === user.id ? '处理中...' : '重置流量'}
                                  </button>
                                  <button
                                    className="w-full px-3 py-2 text-left text-sm flex items-center gap-2 hover:bg-zinc-800"
                                    style={{ color: ADMIN_TEXT }}
                                    onClick={() => { openAddTrafficDialog(user); setActionMenuOpen(null) }}
                                  >
                                    <Plus className="w-4 h-4" />添加流量
                                  </button>
                                  <button
                                    className="w-full px-3 py-2 text-left text-sm flex items-center gap-2 hover:bg-zinc-800"
                                    style={{ color: ADMIN_TEXT }}
                                    onClick={() => { openExtendDialog(user); setActionMenuOpen(null) }}
                                  >
                                    <Clock className="w-4 h-4" />延长订阅
                                  </button>
                                  <button
                                    className="w-full px-3 py-2 text-left text-sm flex items-center gap-2 hover:bg-zinc-800"
                                    style={{ color: ADMIN_TEXT }}
                                    onClick={() => { openChangePlanDialog(user); setActionMenuOpen(null) }}
                                  >
                                    <Dices className="w-4 h-4" />变更套餐
                                  </button>
                                  <button
                                    className="w-full px-3 py-2 text-left text-sm flex items-center gap-2 hover:bg-zinc-800"
                                    style={{ color: ADMIN_TEXT }}
                                    onClick={() => { handleResetSecret(user); setActionMenuOpen(null) }}
                                    disabled={actionLoading === user.id}
                                  >
                                    <Copy className="w-4 h-4" />{actionLoading === user.id ? '处理中...' : '重置订阅'}
                                  </button>
                                  <div className="border-t my-1" style={{ borderColor: ADMIN_BORDER }} />
                                  <button
                                    className="w-full px-3 py-2 text-left text-sm flex items-center gap-2 hover:bg-zinc-800 text-rose-400"
                                    onClick={() => { handleDeleteUser(user); setActionMenuOpen(null) }}
                                    disabled={actionLoading === user.id}
                                  >
                                    <Trash2 className="w-4 h-4" />{actionLoading === user.id ? '处理中...' : '删除用户'}
                                  </button>
                                </div>
                              </>
                            )}
                          </div>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>

      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="text-sm" style={{ color: ADMIN_TEXT_MUTED }}>每页显示</span>
          <Select
            value={String(pageSize)}
            onChange={(e) => setPageSize(Number(e.target.value))}
            className="w-20"
            style={selectStyle}
          >
            <option value="10">10</option>
            <option value="20">20</option>
            <option value="50">50</option>
            <option value="100">100</option>
          </Select>
          <span className="text-sm" style={{ color: ADMIN_TEXT_MUTED }}>
            共 {total} 条记录
          </span>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            style={{ borderColor: ADMIN_BORDER, color: ADMIN_TEXT }}
            className="hover:bg-zinc-800"
            onClick={() => setPage(p => Math.max(1, p - 1))}
            disabled={page <= 1}
          >
            <ChevronLeft className="w-4 h-4" />
          </Button>
          <span className="text-sm" style={{ color: ADMIN_TEXT }}>
            {page} / {totalPages || 1}
          </span>
          <Button
            variant="outline"
            size="sm"
            style={{ borderColor: ADMIN_BORDER, color: ADMIN_TEXT }}
            className="hover:bg-zinc-800"
            onClick={() => setPage(p => Math.min(totalPages, p + 1))}
            disabled={page >= totalPages}
          >
            <ChevronRight className="w-4 h-4" />
          </Button>
        </div>
      </div>

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-md">
          <DialogHeader>
            <DialogTitle>添加用户</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label className="text-zinc-300 text-sm">邮箱 *</Label>
              <Input
                value={createForm.email}
                onChange={(e) => setCreateForm({ ...createForm, email: e.target.value })}
                placeholder="user@example.com"
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              />
            </div>
            <div className="space-y-2">
              <Label className="text-zinc-300 text-sm">密码 *</Label>
              <div className="flex gap-2">
                <Input
                  type="text"
                  value={createForm.password}
                  onChange={(e) => setCreateForm({ ...createForm, password: e.target.value })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100 flex-1"
                />
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="border-zinc-700 text-zinc-300 whitespace-nowrap"
                  onClick={() => setCreateForm({ ...createForm, password: generateRandomPassword() })}
                >
                  <Dices className="w-4 h-4 mr-1" />随机
                </Button>
              </div>
            </div>
            <div className="space-y-2">
              <Label className="text-zinc-300 text-sm">套餐</Label>
              <Select
                value={createForm.plan_id}
                onChange={(e) => setCreateForm({ ...createForm, plan_id: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              >
                <option value="">无套餐</option>
                {plans.map(p => (
                  <option key={p.id} value={p.id}>{p.name}</option>
                ))}
              </Select>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={() => setCreateOpen(false)}>
              取消
            </Button>
            <Button className="bg-indigo-600 hover:bg-indigo-500" onClick={handleCreateUser} disabled={submitting}>
              {submitting ? '创建中...' : '创建'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={editOpen} onOpenChange={setEditOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-md">
          <DialogHeader>
            <DialogTitle>编辑用户</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-4">
            {editingUser?.uuid && (
              <div className="space-y-2">
                <Label className="text-zinc-300 text-sm">用户 UUID（全节点共享凭证）</Label>
                <div className="flex items-center gap-2">
                  <code className="flex-1 text-xs text-zinc-400 font-mono break-all bg-zinc-800/50 rounded px-2 py-1.5 border border-zinc-700/50">
                    {editingUser.uuid}
                  </code>
                  <button
                    onClick={() => copyToClipboard(editingUser.uuid, 'UUID')}
                    className="text-zinc-500 hover:text-zinc-300 transition-colors p-1.5"
                    title="复制 UUID"
                  >
                    <Copy className="w-4 h-4" />
                  </button>
                </div>
              </div>
            )}
            <div className="space-y-2">
              <Label className="text-zinc-300 text-sm">邮箱 *</Label>
              <Input
                value={editForm.email}
                onChange={(e) => setEditForm({ ...editForm, email: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              />
            </div>
            <div className="space-y-2">
              <Label className="text-zinc-300 text-sm">套餐</Label>
              <Select
                value={editForm.plan_id}
                onChange={(e) => setEditForm({ ...editForm, plan_id: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              >
                <option value="">无套餐</option>
                {plans.map(p => (
                  <option key={p.id} value={p.id}>{p.name}</option>
                ))}
              </Select>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={() => setEditOpen(false)}>
              取消
            </Button>
            <Button className="bg-indigo-600 hover:bg-indigo-500" onClick={handleEditUser} disabled={submitting}>
              {submitting ? '保存中...' : '保存'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={detailOpen} onOpenChange={setDetailOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-2xl max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>用户详情</DialogTitle>
          </DialogHeader>
          {detailUser && (
            <div className="space-y-4 py-4">
              <div className="flex items-center gap-4 pb-4 border-b border-zinc-800">
                <div className="w-16 h-16 rounded-full bg-gradient-to-br from-indigo-500 to-purple-600 flex items-center justify-center text-white font-bold text-2xl">
                  {(detailUser.profile.display_name || detailUser.user.email)[0].toUpperCase()}
                </div>
                <div>
                  <div className="text-lg font-semibold text-zinc-100">
                    {detailUser.profile.display_name || detailUser.user.email}
                  </div>
                  <div className="text-sm text-zinc-400">{detailUser.user.email}</div>
                  <div className="flex items-center gap-2 mt-2">
                    {getUserStatusBadge({ ...detailUser.user, subscription: detailUser.subscription })}
                    {detailUser.user.is_admin && (
                      <Badge variant="outline" className="bg-purple-900/50 text-purple-300 border-purple-800/50">管理员</Badge>
                    )}
                    {detailUser.user.email_verified && (
                      <Badge variant="outline" className="bg-blue-900/50 text-blue-300 border-blue-800/50">已验证</Badge>
                    )}
                  </div>
                </div>
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div className="bg-zinc-800/50 rounded-lg p-3 border border-zinc-700/50">
                  <div className="text-xs text-zinc-500 mb-1">用户 ID</div>
                  <code className="text-xs text-zinc-300 font-mono break-all">{detailUser.user.id}</code>
                </div>
                <div className="bg-zinc-800/50 rounded-lg p-3 border border-zinc-700/50">
                  <div className="flex items-center justify-between mb-1">
                    <div className="text-xs text-zinc-500">用户 UUID</div>
                    {detailUser.user.uuid && (
                      <button
                        onClick={() => copyToClipboard(detailUser.user.uuid, 'UUID')}
                        className="text-zinc-500 hover:text-zinc-300 transition-colors"
                        title="复制 UUID"
                      >
                        <Copy className="w-3 h-3" />
                      </button>
                    )}
                  </div>
                  <code className="text-xs text-zinc-300 font-mono break-all">{detailUser.user.uuid || '-'}</code>
                </div>
                <div className="bg-zinc-800/50 rounded-lg p-3 border border-zinc-700/50">
                  <div className="text-xs text-zinc-500 mb-1">套餐</div>
                  <div className="text-sm text-zinc-200">
                    {planMap.get(detailUser.subscription.plan_id || '')?.name || '无套餐'}
                  </div>
                </div>
                <div className="bg-zinc-800/50 rounded-lg p-3 border border-zinc-700/50">
                  <div className="text-xs text-zinc-500 mb-1">订阅状态</div>
                  <div className="text-sm text-zinc-200">{detailUser.subscription.status}</div>
                </div>
                <div className="bg-zinc-800/50 rounded-lg p-3 border border-zinc-700/50">
                  <div className="text-xs text-zinc-500 mb-1">注册时间</div>
                  <div className="text-sm text-zinc-200">{formatDateTime(detailUser.user.created_at)}</div>
                </div>
                <div className="bg-zinc-800/50 rounded-lg p-3 border border-zinc-700/50">
                  <div className="text-xs text-zinc-500 mb-1">最后登录</div>
                  <div className="text-sm text-zinc-200">{formatDateTime(detailUser.user.last_login_at)}</div>
                </div>
              </div>

              <div className="bg-zinc-800/50 rounded-lg p-4 border border-zinc-700/50">
                <div className="text-sm font-medium text-zinc-300 mb-2">流量使用</div>
                <TrafficUsageBar
                  used={detailUser.subscription.traffic_used_bytes}
                  total={detailUser.subscription.traffic_quota_bytes}
                />
              </div>

              <div className="bg-zinc-800/50 rounded-lg p-3 border border-zinc-700/50">
                <div className="flex items-center justify-between">
                  <div>
                    <div className="text-xs text-zinc-500">到期时间</div>
                    <div className="text-sm text-zinc-200">{formatDateTime(detailUser.subscription.expires_at)}</div>
                  </div>
                </div>
              </div>

              {detailUser.profile.bio && (
                <div className="bg-zinc-800/50 rounded-lg p-3 border border-zinc-700/50">
                  <div className="text-xs text-zinc-500 mb-1">简介</div>
                  <div className="text-sm text-zinc-300">{detailUser.profile.bio}</div>
                </div>
              )}
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={() => setDetailOpen(false)}>
              关闭
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={banDialogOpen} onOpenChange={setBanDialogOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-md">
          <DialogHeader>
            <DialogTitle>封禁用户</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <p className="text-sm text-zinc-400">
              确定要封禁用户 <span className="text-zinc-200 font-medium">{banUser?.email}</span> 吗？
            </p>
            <div className="space-y-2">
              <Label className="text-zinc-300 text-sm">封禁原因 *</Label>
              <Textarea
                value={banForm.reason}
                onChange={(e) => setBanForm({ ...banForm, reason: e.target.value })}
                placeholder="请输入封禁原因..."
                className="bg-zinc-800 border-zinc-700 text-zinc-100 min-h-[80px]"
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={() => setBanDialogOpen(false)}>
              取消
            </Button>
            <Button className="bg-rose-600 hover:bg-rose-500" onClick={handleBanUser} disabled={submitting || actionLoading === banUser?.id}>
              {actionLoading === banUser?.id ? '处理中...' : '确认封禁'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={resetPasswordOpen} onOpenChange={setResetPasswordOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-md">
          <DialogHeader>
            <DialogTitle>重置密码</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <p className="text-sm text-zinc-400">
              用户: <span className="text-zinc-200 font-medium">{resetPasswordUser?.email}</span>
            </p>
            {newPassword ? (
              <div className="space-y-2">
                <Label className="text-zinc-300 text-sm">新密码</Label>
                <div className="flex gap-2">
                  <Input
                    value={newPassword}
                    readOnly
                    className="bg-zinc-800 border-zinc-700 text-zinc-100 flex-1 font-mono"
                  />
                  <Button
                    variant="outline"
                    size="sm"
                    className="border-zinc-700 text-zinc-300 whitespace-nowrap"
                    onClick={() => copyToClipboard(newPassword, '新密码')}
                  >
                    <Copy className="w-4 h-4 mr-1" />复制
                  </Button>
                </div>
                <p className="text-xs text-amber-400">请妥善保管新密码，关闭后将无法再次查看</p>
              </div>
            ) : (
              <p className="text-sm text-zinc-400">确定要重置该用户的密码吗？重置后将生成一个新的随机密码。</p>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={() => { setResetPasswordOpen(false); setNewPassword(''); setResetPasswordUser(null) }}>
              {newPassword ? '关闭' : '取消'}
            </Button>
            {!newPassword && (
              <Button className="bg-indigo-600 hover:bg-indigo-500" onClick={handleResetPassword} disabled={actionLoading === resetPasswordUser?.id}>
                {actionLoading === resetPasswordUser?.id ? '处理中...' : '确认重置'}
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={addTrafficOpen} onOpenChange={setAddTrafficOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-md">
          <DialogHeader>
            <DialogTitle>添加流量</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <p className="text-sm text-zinc-400">
              为用户 <span className="text-zinc-200 font-medium">{addTrafficUser?.email}</span> 添加流量
            </p>
            <div className="space-y-2">
              <Label className="text-zinc-300 text-sm">流量 (GB)</Label>
              <Input
                type="number"
                value={addTrafficForm.bytes}
                onChange={(e) => setAddTrafficForm({ ...addTrafficForm, bytes: e.target.value })}
                placeholder="输入要添加的流量值"
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={() => setAddTrafficOpen(false)}>
              取消
            </Button>
            <Button className="bg-indigo-600 hover:bg-indigo-500" onClick={handleAddTraffic} disabled={actionLoading === addTrafficUser?.id}>
              {actionLoading === addTrafficUser?.id ? '处理中...' : '确认添加'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={extendOpen} onOpenChange={setExtendOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-md">
          <DialogHeader>
            <DialogTitle>延长订阅</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <p className="text-sm text-zinc-400">
              为用户 <span className="text-zinc-200 font-medium">{extendUser?.email}</span> 延长订阅
            </p>
            <div className="space-y-2">
              <Label className="text-zinc-300 text-sm">天数</Label>
              <Input
                type="number"
                value={extendForm.days}
                onChange={(e) => setExtendForm({ ...extendForm, days: e.target.value })}
                placeholder="输入要延长的天数"
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={() => setExtendOpen(false)}>
              取消
            </Button>
            <Button className="bg-indigo-600 hover:bg-indigo-500" onClick={handleExtend} disabled={actionLoading === extendUser?.id}>
              {actionLoading === extendUser?.id ? '处理中...' : '确认延长'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={changePlanOpen} onOpenChange={setChangePlanOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-md">
          <DialogHeader>
            <DialogTitle>变更套餐</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <p className="text-sm text-zinc-400">
              为用户 <span className="text-zinc-200 font-medium">{changePlanUser?.email}</span> 变更套餐
            </p>
            <div className="space-y-2">
              <Label className="text-zinc-300 text-sm">新套餐</Label>
              <Select
                value={changePlanForm.plan_id}
                onChange={(e) => setChangePlanForm({ ...changePlanForm, plan_id: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              >
                {plans.map(p => (
                  <option key={p.id} value={p.id}>{p.name}</option>
                ))}
              </Select>
            </div>
            <div className="flex items-center gap-2">
              <Checkbox
                checked={changePlanForm.immediate}
                onChange={(e) => setChangePlanForm({ ...changePlanForm, immediate: e.target.checked })}
              />
              <Label className="text-zinc-300 text-sm cursor-pointer">立即生效</Label>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={() => setChangePlanOpen(false)}>
              取消
            </Button>
            <Button className="bg-indigo-600 hover:bg-indigo-500" onClick={handleChangePlan} disabled={actionLoading === changePlanUser?.id}>
              {actionLoading === changePlanUser?.id ? '处理中...' : '确认变更'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={emailOpen} onOpenChange={setEmailOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-md">
          <DialogHeader>
            <DialogTitle>发送邮件</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <p className="text-sm text-zinc-400">
              将向 <span className="text-zinc-200 font-medium">{selectedIds.size}</span> 个用户发送邮件
            </p>
            <div className="space-y-2">
              <Label className="text-zinc-300 text-sm">邮件主题 *</Label>
              <Input
                value={emailForm.subject}
                onChange={(e) => setEmailForm({ ...emailForm, subject: e.target.value })}
                placeholder="输入邮件主题"
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              />
            </div>
            <div className="space-y-2">
              <Label className="text-zinc-300 text-sm">邮件内容 *</Label>
              <Textarea
                value={emailForm.content}
                onChange={(e) => setEmailForm({ ...emailForm, content: e.target.value })}
                placeholder="输入邮件内容..."
                className="bg-zinc-800 border-zinc-700 text-zinc-100 min-h-[120px]"
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={() => setEmailOpen(false)}>
              取消
            </Button>
            <Button className="bg-indigo-600 hover:bg-indigo-500" onClick={handleSendEmail} disabled={submitting}>
              {submitting ? '发送中...' : '发送'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
