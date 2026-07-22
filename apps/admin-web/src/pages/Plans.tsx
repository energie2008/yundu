import { useState, useEffect, useCallback } from 'react'
import {
  Plus,
  MoreHorizontal,
  Pencil,
  Trash2,
  Search,
  X,
  GripVertical,
  Network,
  Check,
  Loader2,
} from 'lucide-react'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Button,
  Input,
  Badge,
  Skeleton,
  EmptyState,
  Switch,
  Label,
  Textarea,
  Select,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  useToast,
} from '@airport/ui'
import { api } from '@/lib/api'
import { EP } from '@/lib/endpoints'

interface PlanPrice {
  period_code: string
  price_usdt: number
  price_cny?: number
}

interface Plan {
  id: string
  code?: string
  name: string
  description?: string
  content?: string
  status: string
  billing_type: string
  traffic_bytes: number
  can_renew?: boolean
  sort_order?: number
  prices?: PlanPrice[]
  tags?: string[]
  speed_limit_mbps?: number
  device_limit?: number
  ip_limit?: number
  reset_cycle?: string
  duration_days?: number
  feature_flags?: Record<string, unknown>
  // 会员分组ID（关联 node_groups 表），决定购买此套餐的用户可见节点范围
  group_id?: string | null
  created_at?: string
  updated_at?: string
}

// 节点会员分组选项（从 /admin/node-groups/all 拉取）
interface GroupOption {
  id: string
  code: string
  name: string
}

const PLAN_TYPE_MAP: Record<number, { label: string; color: string }> = {
  1: { label: '日付', color: 'bg-blue-900/50 text-blue-300 border-blue-800/50' },
  2: { label: '周付', color: 'bg-cyan-900/50 text-cyan-300 border-cyan-800/50' },
  3: { label: '月付', color: 'bg-indigo-900/50 text-indigo-300 border-indigo-800/50' },
  4: { label: '季付', color: 'bg-violet-900/50 text-violet-300 border-violet-800/50' },
  5: { label: '半年', color: 'bg-purple-900/50 text-purple-300 border-purple-800/50' },
  6: { label: '年付', color: 'bg-fuchsia-900/50 text-fuchsia-300 border-fuchsia-800/50' },
  7: { label: '一次性', color: 'bg-amber-900/50 text-amber-300 border-amber-800/50' },
  8: { label: '流量重置', color: 'bg-emerald-900/50 text-emerald-300 border-emerald-800/50' },
}

const RESET_METHOD_MAP: Record<number, string> = {
  0: '每月重置',
  1: '不重置',
  2: '每年重置',
}

const BILLING_TYPE_MAP: Record<number, { billing_type: string; duration_days: number }> = {
  1: { billing_type: 'periodic', duration_days: 1 },
  2: { billing_type: 'periodic', duration_days: 7 },
  3: { billing_type: 'periodic', duration_days: 30 },
  4: { billing_type: 'periodic', duration_days: 90 },
  5: { billing_type: 'periodic', duration_days: 180 },
  6: { billing_type: 'periodic', duration_days: 365 },
  7: { billing_type: 'one_time', duration_days: 0 },
  8: { billing_type: 'traffic', duration_days: 0 },
}

const RESET_CYCLE_MAP: Record<number, string> = {
  0: 'monthly',
  1: 'never',
  2: 'yearly',
}

const RESET_CYCLE_REVERSE: Record<string, number> = {
  monthly: 0,
  never: 1,
  yearly: 2,
}

const DURATION_TO_TYPE: Record<string, number> = {
  '1': 1,
  '7': 2,
  '30': 3,
  '90': 4,
  '180': 5,
  '365': 6,
}

const BYTES_PER_GB = 1024 * 1024 * 1024

function planToLegacyType(plan: Plan): number {
  if (plan.billing_type === 'one_time') return 7
  if (plan.billing_type === 'traffic') return 8
  if (plan.billing_type === 'periodic' && plan.duration_days) {
    const key = String(plan.duration_days)
    if (DURATION_TO_TYPE[key]) return DURATION_TO_TYPE[key]
  }
  return 3
}

function pricesToMap(prices?: PlanPrice[]): Record<string, number> {
  // 优先读取 CNY 价格（管理员以人民币录入），回退到 USDT
  const map: Record<string, number> = {}
  if (!prices || !Array.isArray(prices)) return map
  for (const p of prices) {
    if (p && p.period_code) {
      map[p.period_code] = p.price_cny || p.price_usdt || 0
    }
  }
  return map
}

function getPlanPrice(plan: Plan): number {
  const priceMap = pricesToMap(plan.prices)
  const keys = ['month', 'quarter', 'half_year', 'year', 'onetime', 'day', 'week']
  for (const k of keys) {
    if (priceMap[k] !== undefined && priceMap[k] > 0) {
      return priceMap[k]
    }
  }
  return 0
}

function getPlanPriceCNY(plan: Plan): number {
  if (!plan.prices || !Array.isArray(plan.prices)) return 0
  const keys = ['month', 'quarter', 'half_year', 'year', 'onetime', 'day', 'week']
  for (const k of keys) {
    const p = plan.prices.find(pr => pr.period_code === k)
    if (p && p.price_cny && p.price_cny > 0) {
      return p.price_cny
    }
  }
  return 0
}

function getPlanContent(plan: Plan): string {
  // 优先使用顶层 description 字段，向后兼容 feature_flags.description
  if (plan.description) return plan.description
  if (plan.feature_flags && typeof plan.feature_flags === 'object') {
    const desc = (plan.feature_flags as Record<string, unknown>).description
    if (typeof desc === 'string') return desc
  }
  return plan.content || ''
}

function slugify(str: string): string {
  // 套餐编码格式：小写字母+数字+连字符，与后端正则 ^[a-z0-9]+(-[a-z0-9]+)*$ 对齐
  // 中文名称中的非 ASCII 字符会被移除（不保留中文），避免后端校验失败
  return str
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .replace(/-+/g, '-')
    || 'plan'
}

interface FormData {
  name: string
  description: string
  content: string
  type: number
  transfer_enable: number
  device_limit: number
  speed_limit: number
  month_price: number
  quarter_price: number
  half_year_price: number
  year_price: number
  onetime_price: number
  reset_traffic_method: number
  show: number
  sort: number
  group_id: string
}

const initialFormData: FormData = {
  name: '',
  description: '',
  content: '',
  type: 3,
  transfer_enable: 0,
  device_limit: 0,
  speed_limit: 0,
  month_price: 0,
  quarter_price: 0,
  half_year_price: 0,
  year_price: 0,
  onetime_price: 0,
  reset_traffic_method: 0,
  show: 1,
  sort: 0,
  group_id: '',
}

export default function Plans() {
  const { toast } = useToast()
  const [plans, setPlans] = useState<Plan[]>([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState('')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingPlan, setEditingPlan] = useState<Plan | null>(null)
  const [formData, setFormData] = useState<FormData>(initialFormData)
  const [submitting, setSubmitting] = useState(false)
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const pageSize = 100
  // 节点会员分组列表（供套餐下拉选择）
  const [groups, setGroups] = useState<GroupOption[]>([])

  const fetchPlans = async () => {
    try {
      setLoading(true)
      const data = await api.get<{ page: number; page_size: number; total: number; items: Plan[] }>(EP.PLANS, {
        params: { page, page_size: pageSize },
      })
      setPlans(Array.isArray(data.items) ? data.items : [])
      setTotal(data.total || 0)
    } catch (err) {
      const message = err instanceof Error ? err.message : '获取套餐列表失败'
      toast({ title: '获取失败', description: message, variant: 'destructive' })
    } finally {
      setLoading(false)
    }
  }

  // 拉取会员分组列表（不分页，供下拉框使用）
  const fetchGroups = useCallback(async () => {
    try {
      const data = await api.get<{ items: GroupOption[] } | GroupOption[]>(EP.NODE_GROUPS_ALL)
      const list = Array.isArray(data) ? data : (data?.items || [])
      setGroups(list)
    } catch (err) {
      console.error('fetch node groups failed:', err)
      setGroups([])
    }
  }, [])

  useEffect(() => {
    fetchPlans()
    fetchGroups()
  }, [page])

  const filteredPlans = plans.filter((p) =>
    p.name.toLowerCase().includes(search.toLowerCase())
  )

  const openCreateDialog = () => {
    setEditingPlan(null)
    setFormData(initialFormData)
    setDialogOpen(true)
  }

  const openEditDialog = (plan: Plan) => {
    setEditingPlan(plan)
    const legacyType = planToLegacyType(plan)
    const priceMap = pricesToMap(plan.prices)
    setFormData({
      name: plan.name,
      description: getPlanContent(plan),
      content: getPlanContent(plan),
      type: legacyType,
      transfer_enable: plan.traffic_bytes > 0 ? Math.round(plan.traffic_bytes / BYTES_PER_GB) : 0,
      device_limit: plan.device_limit || 0,
      speed_limit: plan.speed_limit_mbps || 0,
      month_price: priceMap['month'] || 0,
      quarter_price: priceMap['quarter'] || 0,
      half_year_price: priceMap['half_year'] || 0,
      year_price: priceMap['year'] || 0,
      onetime_price: priceMap['onetime'] || 0,
      reset_traffic_method: plan.reset_cycle ? (RESET_CYCLE_REVERSE[plan.reset_cycle] ?? 0) : 0,
      show: plan.status === 'active' ? 1 : 0,
      sort: plan.sort_order || 0,
      group_id: plan.group_id || '',
    })
    setDialogOpen(true)
  }

  const closeDialog = () => {
    setDialogOpen(false)
    setEditingPlan(null)
    setFormData(initialFormData)
  }

  // ===== 节点-套餐绑定 =====
  const [nodeBindOpen, setNodeBindOpen] = useState(false)
  const [nodeBindPlan, setNodeBindPlan] = useState<Plan | null>(null)
  const [allNodes, setAllNodes] = useState<Array<{id: string; name: string; protocol_type: string; is_enabled: boolean}>>([])
  const [selectedNodeIds, setSelectedNodeIds] = useState<Set<string>>(new Set())
  const [nodeBindLoading, setNodeBindLoading] = useState(false)
  const [nodeBindSaving, setNodeBindSaving] = useState(false)

  const openNodeBindDialog = useCallback(async (plan: Plan) => {
    setNodeBindPlan(plan)
    setNodeBindOpen(true)
    setNodeBindLoading(true)
    setSelectedNodeIds(new Set())
    try {
      // 并行加载所有节点和当前套餐已绑定的节点
      const [nodesResp, boundResp] = await Promise.all([
        api.get<{ items: Array<{id: string; name: string; protocol_type: string; is_enabled: boolean}>; total: number }>(
          EP.NODES, { params: { page: 1, page_size: 200 } }
        ),
        api.get<{ items: Array<{id: string}>; total: number }>(EP.PLAN_NODES(plan.id)),
      ])
      setAllNodes(nodesResp.items || [])
      const boundSet = new Set((boundResp.items || []).map(n => n.id))
      setSelectedNodeIds(boundSet)
    } catch (e) {
      toast({
        title: '加载节点数据失败',
        description: e instanceof Error ? e.message : '未知错误',
        variant: 'destructive',
      })
    } finally {
      setNodeBindLoading(false)
    }
  }, [toast])

  const toggleNodeSelection = (nodeId: string) => {
    setSelectedNodeIds(prev => {
      const next = new Set(prev)
      if (next.has(nodeId)) next.delete(nodeId)
      else next.add(nodeId)
      return next
    })
  }

  const saveNodeBinding = async () => {
    if (!nodeBindPlan) return
    setNodeBindSaving(true)
    try {
      await api.put(EP.PLAN_NODES(nodeBindPlan.id), { node_ids: Array.from(selectedNodeIds) })
      toast({ title: '节点绑定已保存', variant: 'success' })
      setNodeBindOpen(false)
      setNodeBindPlan(null)
      fetchPlans()
    } catch (e) {
      toast({
        title: '保存失败',
        description: e instanceof Error ? e.message : '未知错误',
        variant: 'destructive',
      })
    } finally {
      setNodeBindSaving(false)
    }
  }

  const buildPricesArray = (fd: FormData): PlanPrice[] => {
    // 管理员以人民币（CNY）录入价格，后端 SetPrices 会自动按汇率换算 USDT
    const prices: PlanPrice[] = []
    const entries: [string, number][] = [
      ['month', Number(fd.month_price) || 0],
      ['quarter', Number(fd.quarter_price) || 0],
      ['half_year', Number(fd.half_year_price) || 0],
      ['year', Number(fd.year_price) || 0],
      ['onetime', Number(fd.onetime_price) || 0],
    ]
    for (const [period_code, price_cny] of entries) {
      if (price_cny > 0) {
        prices.push({ period_code, price_usdt: 0, price_cny })
      }
    }
    return prices
  }

  const handleSubmit = async () => {
    if (!formData.name.trim()) {
      toast({ title: '请输入套餐名称', variant: 'destructive' })
      return
    }

    try {
      setSubmitting(true)
      const billingInfo = BILLING_TYPE_MAP[formData.type] || BILLING_TYPE_MAP[3]
      const pricesArr = buildPricesArray(formData)

      const baseBody = {
        name: formData.name,
        description: formData.content,
        status: formData.show === 1 ? 'active' : 'draft',
        billing_type: billingInfo.billing_type,
        traffic_bytes: formData.transfer_enable > 0 ? formData.transfer_enable * BYTES_PER_GB : 0,
        can_renew: billingInfo.billing_type === 'periodic',
        sort_order: Number(formData.sort) || 0,
        prices: pricesArr,
        tags: [] as string[],
        speed_limit_mbps: Number(formData.speed_limit) || 0,
        device_limit: Number(formData.device_limit) || 0,
        ip_limit: 0,
        reset_cycle: RESET_CYCLE_MAP[formData.reset_traffic_method] || 'monthly',
        duration_days: billingInfo.duration_days > 0 ? billingInfo.duration_days : undefined,
        feature_flags: { description: formData.content },
        // group_id：空字符串表示清空绑定（传 null），非空表示绑定到指定分组
        group_id: formData.group_id || null,
      }

      if (editingPlan) {
        await api.patch(EP.PLAN_DETAIL(editingPlan.id), baseBody)
        toast({ title: '套餐已更新', variant: 'success' })
      } else {
        await api.post(EP.PLANS, {
          ...baseBody,
          code: slugify(formData.name),
        })
        toast({ title: '套餐已创建', variant: 'success' })
      }
      closeDialog()
      fetchPlans()
    } catch (err) {
      const message = err instanceof Error ? err.message : '操作失败'
      toast({ title: '操作失败', description: message, variant: 'destructive' })
    } finally {
      setSubmitting(false)
    }
  }

  const handleDelete = async (plan: Plan) => {
    if (!confirm(`确定要删除套餐 "${plan.name}" 吗？`)) return
    try {
      await api.delete(EP.PLAN_DETAIL(plan.id))
      toast({ title: '套餐已删除', variant: 'success' })
      fetchPlans()
    } catch (err) {
      const message = err instanceof Error ? err.message : '删除失败'
      toast({ title: '删除失败', description: message, variant: 'destructive' })
    }
  }

  const toggleShow = async (plan: Plan) => {
    try {
      const newStatus = plan.status === 'active' ? 'draft' : 'active'
      await api.patch(EP.PLAN_DETAIL(plan.id), { status: newStatus })
      fetchPlans()
    } catch (err) {
      const message = err instanceof Error ? err.message : '更新失败'
      toast({ title: '更新失败', description: message, variant: 'destructive' })
    }
  }

  const updateSort = async (plan: Plan, sortVal: number) => {
    try {
      await api.patch(EP.PLAN_DETAIL(plan.id), { sort_order: sortVal })
      fetchPlans()
    } catch (err) {
      const message = err instanceof Error ? err.message : '排序更新失败'
      toast({ title: '更新失败', description: message, variant: 'destructive' })
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-zinc-100">套餐管理</h1>
          <p className="text-sm text-zinc-500 mt-1">管理订阅套餐和价格配置</p>
        </div>
        <div className="flex items-center gap-2">
          <Button size="sm" className="bg-indigo-600 hover:bg-indigo-500" onClick={openCreateDialog}>
            <Plus className="w-4 h-4 mr-1" />添加套餐
          </Button>
        </div>
      </div>

      <Card className="bg-zinc-900 border-zinc-800">
        <CardContent className="p-3">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-zinc-500" />
            <Input
              placeholder="搜索套餐名称..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="pl-9 bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500"
            />
          </div>
        </CardContent>
      </Card>

      <Card className="bg-zinc-900 border-zinc-800 overflow-hidden">
        <CardContent className="p-0">
          {loading ? (
            <div className="p-4 space-y-3">
              {[1, 2, 3, 4, 5].map((i) => (
                <Skeleton key={i} className="h-16 w-full bg-zinc-800 rounded-lg" />
              ))}
            </div>
          ) : filteredPlans.length === 0 ? (
            <EmptyState title="暂无套餐" description="点击右上角添加按钮创建第一个套餐" className="py-12" />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-zinc-800">
                    <th className="text-left p-3 text-xs font-medium text-zinc-400 w-8"></th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400">ID</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400">名称</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400">类型</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400 hidden lg:table-cell">会员分组</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400">流量(GB)</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400 hidden md:table-cell">价格</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400 hidden lg:table-cell">设备数</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400 hidden lg:table-cell">限速(Mbps)</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400">显示</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400 w-20">排序</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400 w-10"></th>
                  </tr>
                </thead>
                <tbody>
                  {filteredPlans.map((plan) => {
                    const legacyType = planToLegacyType(plan)
                    const typeInfo = PLAN_TYPE_MAP[legacyType] || { label: '未知', color: 'bg-zinc-800 text-zinc-400 border-zinc-700' }
                    const price = getPlanPrice(plan)
                    const priceCNY = getPlanPriceCNY(plan)
                    const content = getPlanContent(plan)
                    const trafficGB = plan.traffic_bytes > 0 ? Math.round(plan.traffic_bytes / BYTES_PER_GB) : 0
                    const sortOrder = plan.sort_order ?? 0
                    // 根据 group_id 查找分组名称
                    const groupName = plan.group_id
                      ? (groups.find(g => g.id === plan.group_id)?.name || '未知分组')
                      : ''
                    return (
                      <tr key={plan.id} className="border-b border-zinc-800 last:border-0 hover:bg-zinc-800/50 transition-colors">
                        <td className="p-3 text-zinc-600">
                          <GripVertical className="w-4 h-4" />
                        </td>
                        <td className="p-3 text-sm text-zinc-500 font-mono">{plan.id.slice(0, 8)}</td>
                        <td className="p-3">
                          <div className="text-sm font-medium text-zinc-200">{plan.name}</div>
                          {content && (
                            <div className="text-xs text-zinc-500 line-clamp-1 max-w-xs">{content}</div>
                          )}
                        </td>
                        <td className="p-3">
                          <Badge variant="outline" className={typeInfo.color}>{typeInfo.label}</Badge>
                        </td>
                        <td className="p-3 text-sm hidden lg:table-cell">
                          {groupName ? (
                            <Badge variant="outline" className="bg-indigo-900/40 text-indigo-300 border-indigo-800/50">{groupName}</Badge>
                          ) : (
                            <span className="text-zinc-600 text-xs">全部节点</span>
                          )}
                        </td>
                        <td className="p-3 text-sm text-zinc-300">
                          {plan.traffic_bytes > 0 ? `${trafficGB} GB` : '无限'}
                        </td>
                        <td className="p-3 text-sm text-zinc-100 font-medium hidden md:table-cell">
                          <div>${price.toFixed(2)} USDT</div>
                          {priceCNY > 0 && <div className="text-xs text-zinc-400">¥{priceCNY.toFixed(2)}</div>}
                        </td>
                        <td className="p-3 text-sm text-zinc-400 hidden lg:table-cell">
                          {plan.device_limit && plan.device_limit > 0 ? plan.device_limit : '不限'}
                        </td>
                        <td className="p-3 text-sm text-zinc-400 hidden lg:table-cell">
                          {plan.speed_limit_mbps && plan.speed_limit_mbps > 0 ? `${plan.speed_limit_mbps} Mbps` : '不限'}
                        </td>
                        <td className="p-3">
                          <Switch
                            checked={plan.status === 'active'}
                            onChange={() => toggleShow(plan)}
                          />
                        </td>
                        <td className="p-3">
                          <Input
                            type="number"
                            value={sortOrder}
                            onChange={(e) => {
                              const val = Number(e.target.value)
                              setPlans(prev => prev.map(p => p.id === plan.id ? { ...p, sort_order: val } : p))
                            }}
                            onBlur={(e) => updateSort(plan, Number(e.target.value))}
                            className="w-16 h-8 bg-zinc-800 border-zinc-700 text-zinc-100 text-sm"
                          />
                        </td>
                        <td className="p-3">
                          <div className="flex items-center gap-1">
                            <Button variant="ghost" size="sm" className="h-8 w-8 p-0" title="节点绑定" onClick={() => openNodeBindDialog(plan)}>
                              <Network className="w-4 h-4 text-indigo-400" />
                            </Button>
                            <Button variant="ghost" size="sm" className="h-8 w-8 p-0" onClick={() => openEditDialog(plan)}>
                              <Pencil className="w-4 h-4 text-zinc-400" />
                            </Button>
                            <Button variant="ghost" size="sm" className="h-8 w-8 p-0" onClick={() => handleDelete(plan)}>
                              <Trash2 className="w-4 h-4 text-red-400" />
                            </Button>
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

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-2xl max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{editingPlan ? '编辑套餐' : '添加套餐'}</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label className="text-zinc-300 text-sm">套餐名称 *</Label>
                <Input
                  value={formData.name}
                  onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                  placeholder="如: Pro 月付套餐"
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
              </div>
              <div className="space-y-2">
                <Label className="text-zinc-300 text-sm">套餐类型</Label>
                <Select value={String(formData.type)} onChange={(e) => setFormData({ ...formData, type: Number(e.target.value) })} className="bg-zinc-800 border-zinc-700 text-zinc-100">
                  <option value="1">日付</option>
                  <option value="2">周付</option>
                  <option value="3">月付</option>
                  <option value="4">季付</option>
                  <option value="5">半年</option>
                  <option value="6">年付</option>
                  <option value="7">一次性</option>
                  <option value="8">流量重置</option>
                </Select>
              </div>
            </div>

            <div className="space-y-2">
              <Label className="text-zinc-300 text-sm">套餐描述</Label>
              <Textarea
                value={formData.content}
                onChange={(e) => setFormData({ ...formData, content: e.target.value })}
                placeholder="套餐详情说明..."
                className="bg-zinc-800 border-zinc-700 text-zinc-100 min-h-[80px]"
              />
            </div>

            <div className="grid grid-cols-3 gap-4">
              <div className="space-y-2">
                <Label className="text-zinc-300 text-sm">流量 (GB)</Label>
                <Input
                  type="number"
                  value={formData.transfer_enable}
                  onChange={(e) => setFormData({ ...formData, transfer_enable: Number(e.target.value) })}
                  placeholder="0=无限"
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
              </div>
              <div className="space-y-2">
                <Label className="text-zinc-300 text-sm">设备数限制</Label>
                <Input
                  type="number"
                  value={formData.device_limit}
                  onChange={(e) => setFormData({ ...formData, device_limit: Number(e.target.value) })}
                  placeholder="0=不限"
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
              </div>
              <div className="space-y-2">
                <Label className="text-zinc-300 text-sm">限速 (Mbps)</Label>
                <Input
                  type="number"
                  value={formData.speed_limit}
                  onChange={(e) => setFormData({ ...formData, speed_limit: Number(e.target.value) })}
                  placeholder="0=不限"
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
              </div>
            </div>

            <div className="space-y-2">
              <Label className="text-zinc-300 text-sm">价格设置 (人民币/元，CNY 自动按汇率换算)</Label>
              <div className="grid grid-cols-5 gap-3">
                <div className="space-y-1">
                  <Label className="text-zinc-500 text-xs">月付</Label>
                  <Input
                    type="number"
                    value={formData.month_price}
                    onChange={(e) => setFormData({ ...formData, month_price: Number(e.target.value) })}
                    className="bg-zinc-800 border-zinc-700 text-zinc-100 h-8"
                  />
                </div>
                <div className="space-y-1">
                  <Label className="text-zinc-500 text-xs">季付</Label>
                  <Input
                    type="number"
                    value={formData.quarter_price}
                    onChange={(e) => setFormData({ ...formData, quarter_price: Number(e.target.value) })}
                    className="bg-zinc-800 border-zinc-700 text-zinc-100 h-8"
                  />
                </div>
                <div className="space-y-1">
                  <Label className="text-zinc-500 text-xs">半年</Label>
                  <Input
                    type="number"
                    value={formData.half_year_price}
                    onChange={(e) => setFormData({ ...formData, half_year_price: Number(e.target.value) })}
                    className="bg-zinc-800 border-zinc-700 text-zinc-100 h-8"
                  />
                </div>
                <div className="space-y-1">
                  <Label className="text-zinc-500 text-xs">年付</Label>
                  <Input
                    type="number"
                    value={formData.year_price}
                    onChange={(e) => setFormData({ ...formData, year_price: Number(e.target.value) })}
                    className="bg-zinc-800 border-zinc-700 text-zinc-100 h-8"
                  />
                </div>
                <div className="space-y-1">
                  <Label className="text-zinc-500 text-xs">一次性</Label>
                  <Input
                    type="number"
                    value={formData.onetime_price}
                    onChange={(e) => setFormData({ ...formData, onetime_price: Number(e.target.value) })}
                    className="bg-zinc-800 border-zinc-700 text-zinc-100 h-8"
                  />
                </div>
              </div>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label className="text-zinc-300 text-sm">流量重置方式</Label>
                <Select value={String(formData.reset_traffic_method)} onChange={(e) => setFormData({ ...formData, reset_traffic_method: Number(e.target.value) })} className="bg-zinc-800 border-zinc-700 text-zinc-100">
                  <option value="0">每月重置</option>
                  <option value="1">不重置</option>
                  <option value="2">每年重置</option>
                </Select>
              </div>
              <div className="space-y-2">
                <Label className="text-zinc-300 text-sm">排序</Label>
                <Input
                  type="number"
                  value={formData.sort}
                  onChange={(e) => setFormData({ ...formData, sort: Number(e.target.value) })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
              </div>
            </div>

            {/* 会员分组选择器：决定购买此套餐的用户可见哪些节点 */}
            <div className="space-y-2">
              <Label className="text-zinc-300 text-sm flex items-center gap-1">
                会员分组
                <span className="text-zinc-500 text-xs">（决定用户可见的节点范围，留空=全部节点）</span>
              </Label>
              <Select
                value={formData.group_id}
                onChange={(e) => setFormData({ ...formData, group_id: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              >
                <option value="">不限制（全部节点）</option>
                {groups.map(g => (
                  <option key={g.id} value={g.id}>{g.name}（{g.code}）</option>
                ))}
              </Select>
              {groups.length === 0 && (
                <p className="text-xs text-amber-500/80">
                  暂无会员分组，请先在「会员分组管理」页面创建
                </p>
              )}
            </div>

            <div className="flex items-center justify-between">
              <Label className="text-zinc-300 text-sm">显示套餐</Label>
              <Switch
                checked={formData.show === 1}
                onChange={(e) => setFormData({ ...formData, show: e.target.checked ? 1 : 0 })}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={closeDialog}>
              取消
            </Button>
            <Button className="bg-indigo-600 hover:bg-indigo-500" onClick={handleSubmit} disabled={submitting}>
              {submitting ? '保存中...' : editingPlan ? '更新' : '创建'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 节点绑定弹窗 */}
      <Dialog open={nodeBindOpen} onOpenChange={setNodeBindOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-2xl max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>
              节点绑定 {nodeBindPlan && <span className="text-zinc-400 font-normal">— {nodeBindPlan.name}</span>}
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-3 py-2">
            <div className="text-xs text-zinc-500">
              已选择 <span className="text-indigo-400 font-medium">{selectedNodeIds.size}</span> / {allNodes.length} 个节点
            </div>
            {nodeBindLoading ? (
              <div className="flex items-center justify-center py-12 text-zinc-500">
                <Loader2 className="w-5 h-5 animate-spin mr-2" /> 加载中...
              </div>
            ) : allNodes.length === 0 ? (
              <div className="text-center py-12 text-zinc-500 text-sm">
                暂无节点，请先在「节点管理」页面添加节点
              </div>
            ) : (
              <div className="space-y-1.5 max-h-[50vh] overflow-y-auto pr-1">
                {allNodes.map(node => {
                  const bound = selectedNodeIds.has(node.id)
                  return (
                    <button
                      key={node.id}
                      type="button"
                      onClick={() => toggleNodeSelection(node.id)}
                      className={`w-full flex items-center justify-between p-2.5 rounded-lg border transition-all text-left ${
                        bound ? 'bg-indigo-950/30 border-indigo-700/50' : 'bg-zinc-800/30 border-zinc-700 hover:border-zinc-600'
                      }`}
                    >
                      <div className="flex items-center gap-3">
                        <div className={`w-2 h-2 rounded-full ${node.is_enabled ? 'bg-emerald-500' : 'bg-zinc-600'}`} />
                        <div>
                          <div className="text-sm text-zinc-200">{node.name}</div>
                          <div className="text-xs text-zinc-500">{node.protocol_type}</div>
                        </div>
                      </div>
                      <div className={`w-5 h-5 rounded border flex items-center justify-center ${bound ? 'bg-indigo-600 border-indigo-500' : 'border-zinc-600'}`}>
                        {bound && <Check className="w-3.5 h-3.5 text-white" />}
                      </div>
                    </button>
                  )
                })}
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={() => setNodeBindOpen(false)}>
              取消
            </Button>
            <Button className="bg-indigo-600 hover:bg-indigo-500" onClick={saveNodeBinding} disabled={nodeBindSaving || nodeBindLoading}>
              {nodeBindSaving ? '保存中...' : '保存绑定'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
