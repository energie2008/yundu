import { useState, useEffect, useCallback } from 'react'
import {
  Plus,
  Pencil,
  Trash2,
  Search,
  RefreshCw,
  RefreshCcw,
  Ticket,
} from 'lucide-react'
import {
  Card,
  CardContent,
  Button,
  Input,
  Badge,
  Skeleton,
  EmptyState,
  Switch,
  Label,
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

// ===== 类型定义 =====
interface Plan {
  id: string
  name: string
  code?: string
}

interface Coupon {
  id: string
  code: string
  name: string
  // 'percentage' = 比例折扣(0-100)，'fixed' = 固定金额
  discount_type: string
  discount_value: number
  max_uses: number
  used_count: number
  min_order_amount: number
  // 限定单个套餐（兼容旧字段），null=不限制
  plan_id?: string | null
  // 每用户使用次数上限，0=不限
  limit_use_by_user: number
  // 限定多个套餐可用，空=不限制
  limit_plan_ids?: string[]
  // 仅限新用户
  new_user_only: boolean
  // ISO8601 时间字符串，null=不限
  starts_at?: string | null
  expires_at?: string | null
  is_active: boolean
  created_at?: string
  updated_at?: string
}

// ===== 工具函数 =====
const DISCOUNT_TYPE_MAP: Record<string, { label: string; color: string }> = {
  percentage: { label: '比例折扣', color: 'bg-emerald-900/50 text-emerald-300 border-emerald-800/50' },
  fixed: { label: '固定金额', color: 'bg-amber-900/50 text-amber-300 border-amber-800/50' },
}

const generateRandomCode = () => {
  const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789'
  let code = ''
  for (let i = 0; i < 8; i++) {
    code += chars.charAt(Math.floor(Math.random() * chars.length))
  }
  return code
}

// 将 ISO8601 字符串转换为 datetime-local input 可用的格式
function toDatetimeLocal(iso?: string | null): string {
  if (!iso) return ''
  try {
    const d = new Date(iso)
    if (isNaN(d.getTime())) return ''
    // 转换为本地时间并截断到分钟
    const tzOffset = d.getTimezoneOffset() * 60000
    return new Date(d.getTime() - tzOffset).toISOString().slice(0, 16)
  } catch {
    return ''
  }
}

// 将 datetime-local 字符串转换为 ISO8601
function fromDatetimeLocal(local: string): string | null {
  if (!local) return null
  try {
    const d = new Date(local)
    if (isNaN(d.getTime())) return null
    return d.toISOString()
  } catch {
    return null
  }
}

function formatDate(iso?: string | null): string {
  if (!iso) return '永久'
  try {
    return new Date(iso).toLocaleDateString('zh-CN')
  } catch {
    return '-'
  }
}

interface FormData {
  code: string
  name: string
  discount_type: string
  discount_value: number
  max_uses: number
  min_order_amount: number
  limit_use_by_user: number
  limit_plan_ids: string[]
  new_user_only: boolean
  starts_at: string
  expires_at: string
  is_active: boolean
}

const initialFormData: FormData = {
  code: '',
  name: '',
  discount_type: 'fixed',
  discount_value: 0,
  max_uses: 0,
  min_order_amount: 0,
  limit_use_by_user: 0,
  limit_plan_ids: [],
  new_user_only: false,
  starts_at: '',
  expires_at: '',
  is_active: true,
}

export default function Coupons() {
  const { toast } = useToast()
  const [coupons, setCoupons] = useState<Coupon[]>([])
  const [plans, setPlans] = useState<Plan[]>([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState('')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingCoupon, setEditingCoupon] = useState<Coupon | null>(null)
  const [formData, setFormData] = useState<FormData>(initialFormData)
  const [submitting, setSubmitting] = useState(false)

  // ===== 拉取套餐列表（供限制套餐选择使用）=====
  const fetchPlans = useCallback(async () => {
    try {
      const data = await api.get<{ items: Plan[] }>(EP.PLANS, { params: { page: 1, page_size: 100 } })
      setPlans(Array.isArray(data.items) ? data.items : [])
    } catch (err) {
      console.error('fetch plans failed:', err)
      setPlans([])
    }
  }, [])

  // ===== 拉取优惠券列表 =====
  const fetchCoupons = useCallback(async () => {
    try {
      setLoading(true)
      const data = await api.get<{ items: Coupon[]; total: number }>(EP.COUPONS, {
        params: { page: 1, page_size: 200 },
      })
      setCoupons(Array.isArray(data.items) ? data.items : [])
    } catch (err) {
      toast({
        title: '加载失败',
        description: err instanceof Error ? err.message : '无法获取优惠券列表',
        variant: 'destructive',
      })
      setCoupons([])
    } finally {
      setLoading(false)
    }
  }, [toast])

  useEffect(() => {
    fetchPlans()
    fetchCoupons()
  }, [fetchPlans, fetchCoupons])

  const filteredCoupons = coupons.filter((c) =>
    c.code.toLowerCase().includes(search.toLowerCase()) ||
    c.name.toLowerCase().includes(search.toLowerCase())
  )

  // ===== 对话框操作 =====
  const openCreateDialog = () => {
    setEditingCoupon(null)
    setFormData({
      ...initialFormData,
      code: generateRandomCode(),
    })
    setDialogOpen(true)
  }

  const openEditDialog = (coupon: Coupon) => {
    setEditingCoupon(coupon)
    setFormData({
      code: coupon.code,
      name: coupon.name,
      discount_type: coupon.discount_type,
      discount_value: coupon.discount_value,
      max_uses: coupon.max_uses,
      min_order_amount: coupon.min_order_amount,
      limit_use_by_user: coupon.limit_use_by_user,
      limit_plan_ids: Array.isArray(coupon.limit_plan_ids) ? coupon.limit_plan_ids : [],
      new_user_only: !!coupon.new_user_only,
      starts_at: toDatetimeLocal(coupon.starts_at),
      expires_at: toDatetimeLocal(coupon.expires_at),
      is_active: coupon.is_active,
    })
    setDialogOpen(true)
  }

  const closeDialog = () => {
    setDialogOpen(false)
    setEditingCoupon(null)
    setFormData(initialFormData)
  }

  // ===== 提交保存 =====
  const handleSubmit = async () => {
    if (!formData.code.trim()) {
      toast({ title: '请填写优惠码', variant: 'destructive' })
      return
    }
    if (!formData.name.trim()) {
      toast({ title: '请填写优惠券名称', variant: 'destructive' })
      return
    }
    if (formData.discount_type === 'percentage' && (formData.discount_value < 0 || formData.discount_value > 100)) {
      toast({ title: '比例折扣值必须在 0-100 之间', variant: 'destructive' })
      return
    }
    if (formData.discount_type === 'fixed' && formData.discount_value < 0) {
      toast({ title: '固定金额不能为负数', variant: 'destructive' })
      return
    }

    try {
      setSubmitting(true)
      // 构造请求体：时间字段为空字符串时传 null
      const payload = {
        code: formData.code.toUpperCase(),
        name: formData.name,
        discount_type: formData.discount_type,
        discount_value: Number(formData.discount_value) || 0,
        max_uses: Number(formData.max_uses) || 0,
        min_order_amount: Number(formData.min_order_amount) || 0,
        limit_use_by_user: Number(formData.limit_use_by_user) || 0,
        limit_plan_ids: formData.limit_plan_ids,
        new_user_only: formData.new_user_only,
        starts_at: fromDatetimeLocal(formData.starts_at),
        expires_at: fromDatetimeLocal(formData.expires_at),
        is_active: formData.is_active,
      }

      if (editingCoupon) {
        // 更新：不传 code（不可修改）
        const { code, ...updatePayload } = payload
        await api.patch(EP.COUPON_DETAIL(editingCoupon.id), updatePayload)
        toast({ title: '优惠券已更新', variant: 'success' })
      } else {
        await api.post(EP.COUPONS, payload)
        toast({ title: '优惠券已创建', variant: 'success' })
      }
      closeDialog()
      fetchCoupons()
    } catch (err) {
      toast({
        title: '保存失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setSubmitting(false)
    }
  }

  const handleDelete = async (coupon: Coupon) => {
    if (!confirm(`确定要删除优惠券 "${coupon.code}" 吗？\n\n删除后不可恢复，相关使用记录也会被清除。`)) return
    try {
      await api.delete(EP.COUPON_DETAIL(coupon.id))
      toast({ title: '优惠券已删除', variant: 'success' })
      fetchCoupons()
    } catch (err) {
      toast({
        title: '删除失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    }
  }

  const toggleActive = async (coupon: Coupon) => {
    try {
      await api.patch(EP.COUPON_DETAIL(coupon.id), { is_active: !coupon.is_active })
      fetchCoupons()
    } catch (err) {
      toast({
        title: '更新失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    }
  }

  // ===== 限制套餐多选 =====
  const togglePlanInLimit = (planId: string) => {
    setFormData(prev => {
      const set = new Set(prev.limit_plan_ids)
      if (set.has(planId)) set.delete(planId)
      else set.add(planId)
      return { ...prev, limit_plan_ids: Array.from(set) }
    })
  }

  const getPlanName = (planId?: string | null): string => {
    if (!planId) return '全部套餐'
    const plan = plans.find(p => p.id === planId)
    return plan ? plan.name : '未知套餐'
  }

  const getValueDisplay = (coupon: Coupon): string => {
    if (coupon.discount_type === 'fixed') {
      return `¥${coupon.discount_value}`
    }
    if (coupon.discount_type === 'percentage') {
      const off = 100 - coupon.discount_value
      return `${off}%折（${coupon.discount_value}% off）`
    }
    return String(coupon.discount_value)
  }

  // ===== 渲染 =====
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-zinc-100 flex items-center gap-2">
            <Ticket className="w-5 h-5 text-amber-400" />
            优惠券管理
          </h1>
          <p className="text-sm text-zinc-500 mt-1">创建和管理优惠码，支持比例折扣/固定金额、限购、限用户、限套餐</p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={fetchCoupons}>
            <RefreshCw className="w-4 h-4 mr-1" />刷新
          </Button>
          <Button size="sm" className="bg-indigo-600 hover:bg-indigo-500" onClick={openCreateDialog}>
            <Plus className="w-4 h-4 mr-1" />生成优惠券
          </Button>
        </div>
      </div>

      <Card className="bg-zinc-900 border-zinc-800">
        <CardContent className="p-3">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-zinc-500" />
            <Input
              placeholder="搜索优惠码或名称..."
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
              {[1, 2, 3, 4].map((i) => (
                <Skeleton key={i} className="h-16 w-full bg-zinc-800 rounded-lg" />
              ))}
            </div>
          ) : filteredCoupons.length === 0 ? (
            <EmptyState title="暂无优惠券" description="点击右上角生成按钮创建第一个优惠码" className="py-12" />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-zinc-800">
                    <th className="text-left p-3 text-xs font-medium text-zinc-400">优惠码</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400">名称</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400">类型</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400">优惠值</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400 hidden md:table-cell">使用限制</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400 hidden lg:table-cell">限定套餐</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400 hidden lg:table-cell">有效期</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400">已用</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400">启用</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400 w-10"></th>
                  </tr>
                </thead>
                <tbody>
                  {filteredCoupons.map((coupon) => {
                    const typeInfo = DISCOUNT_TYPE_MAP[coupon.discount_type] || { label: '未知', color: 'bg-zinc-800 text-zinc-400 border-zinc-700' }
                    const limitedPlans = Array.isArray(coupon.limit_plan_ids) && coupon.limit_plan_ids.length > 0
                    const planText = limitedPlans
                      ? `${coupon.limit_plan_ids!.length} 个套餐`
                      : (coupon.plan_id ? getPlanName(coupon.plan_id) : '全部套餐')
                    return (
                      <tr key={coupon.id} className="border-b border-zinc-800 last:border-0 hover:bg-zinc-800/50 transition-colors">
                        <td className="p-3">
                          <code className="text-sm font-mono text-indigo-400 bg-indigo-950/50 px-2 py-0.5 rounded">{coupon.code}</code>
                        </td>
                        <td className="p-3 text-sm text-zinc-200">{coupon.name}</td>
                        <td className="p-3">
                          <Badge variant="outline" className={typeInfo.color}>{typeInfo.label}</Badge>
                        </td>
                        <td className="p-3 text-sm font-medium text-zinc-100">{getValueDisplay(coupon)}</td>
                        <td className="p-3 text-sm text-zinc-400 hidden md:table-cell">
                          <div>总限: {coupon.max_uses > 0 ? coupon.max_uses : '无限'}</div>
                          <div className="text-xs text-zinc-500">
                            单用户: {coupon.limit_use_by_user > 0 ? coupon.limit_use_by_user : '无限'}
                            {coupon.new_user_only && <span className="text-amber-500 ml-1">·仅新用户</span>}
                          </div>
                        </td>
                        <td className="p-3 text-sm text-zinc-400 hidden lg:table-cell">{planText}</td>
                        <td className="p-3 text-sm text-zinc-400 hidden lg:table-cell">
                          <div>{formatDate(coupon.starts_at)}</div>
                          <div className="text-xs text-zinc-500">至 {formatDate(coupon.expires_at)}</div>
                        </td>
                        <td className="p-3 text-sm text-zinc-300">
                          <span className={coupon.max_uses > 0 && coupon.used_count >= coupon.max_uses ? 'text-red-400' : ''}>
                            {coupon.used_count}{coupon.max_uses > 0 ? `/${coupon.max_uses}` : ''}
                          </span>
                        </td>
                        <td className="p-3">
                          <Switch
                            checked={coupon.is_active}
                            onChange={() => toggleActive(coupon)}
                          />
                        </td>
                        <td className="p-3">
                          <div className="flex items-center gap-1">
                            <Button variant="ghost" size="sm" className="h-8 w-8 p-0" onClick={() => openEditDialog(coupon)}>
                              <Pencil className="w-4 h-4 text-zinc-400" />
                            </Button>
                            <Button variant="ghost" size="sm" className="h-8 w-8 p-0" onClick={() => handleDelete(coupon)}>
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
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-xl max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{editingCoupon ? '编辑优惠券' : '生成优惠券'}</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label className="text-zinc-300 text-sm">优惠码 *</Label>
                <div className="flex gap-2">
                  <Input
                    value={formData.code}
                    onChange={(e) => setFormData({ ...formData, code: e.target.value.toUpperCase() })}
                    placeholder="优惠码"
                    className="flex-1 bg-zinc-800 border-zinc-700 text-zinc-100 font-mono"
                    disabled={!!editingCoupon}
                  />
                  {!editingCoupon && (
                    <Button
                      variant="outline"
                      size="sm"
                      className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 px-2"
                      onClick={() => setFormData({ ...formData, code: generateRandomCode() })}
                      title="随机生成"
                    >
                      <RefreshCcw className="w-4 h-4" />
                    </Button>
                  )}
                </div>
                {editingCoupon && (
                  <p className="text-xs text-zinc-500">优惠码创建后不可修改</p>
                )}
              </div>
              <div className="space-y-2">
                <Label className="text-zinc-300 text-sm">名称 *</Label>
                <Input
                  value={formData.name}
                  onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                  placeholder="优惠券名称"
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
              </div>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label className="text-zinc-300 text-sm">优惠类型</Label>
                <Select
                  value={formData.discount_type}
                  onChange={(e) => setFormData({ ...formData, discount_type: e.target.value })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                >
                  <option value="fixed">固定金额（满减）</option>
                  <option value="percentage">比例折扣（0-100）</option>
                </Select>
              </div>
              <div className="space-y-2">
                <Label className="text-zinc-300 text-sm">
                  优惠值 {formData.discount_type === 'fixed' ? '(元)' : '(%)'}
                </Label>
                <Input
                  type="number"
                  value={formData.discount_value}
                  onChange={(e) => setFormData({ ...formData, discount_value: Number(e.target.value) })}
                  placeholder={formData.discount_type === 'fixed' ? '如: 10（减10元）' : '如: 20（打8折=减20%）'}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
                <p className="text-xs text-zinc-500">
                  {formData.discount_type === 'percentage'
                    ? '填写折扣比例，如 20 表示减20%（即8折）'
                    : '填写减免金额，如 10 表示减10元'}
                </p>
              </div>
            </div>

            <div className="grid grid-cols-3 gap-4">
              <div className="space-y-2">
                <Label className="text-zinc-300 text-sm">总使用次数</Label>
                <Input
                  type="number"
                  value={formData.max_uses}
                  onChange={(e) => setFormData({ ...formData, max_uses: Number(e.target.value) })}
                  placeholder="0=无限"
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
              </div>
              <div className="space-y-2">
                <Label className="text-zinc-300 text-sm">单用户次数</Label>
                <Input
                  type="number"
                  value={formData.limit_use_by_user}
                  onChange={(e) => setFormData({ ...formData, limit_use_by_user: Number(e.target.value) })}
                  placeholder="0=无限"
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
              </div>
              <div className="space-y-2">
                <Label className="text-zinc-300 text-sm">最低消费</Label>
                <Input
                  type="number"
                  value={formData.min_order_amount}
                  onChange={(e) => setFormData({ ...formData, min_order_amount: Number(e.target.value) })}
                  placeholder="0=不限"
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
              </div>
            </div>

            {/* 限定套餐多选 */}
            <div className="space-y-2">
              <Label className="text-zinc-300 text-sm">
                限定套餐 <span className="text-zinc-500 text-xs">（不勾选=全部套餐可用）</span>
              </Label>
              {plans.length === 0 ? (
                <p className="text-xs text-amber-500/80">暂无套餐数据</p>
              ) : (
                <div className="grid grid-cols-2 gap-2 max-h-32 overflow-y-auto p-2 bg-zinc-800/50 border border-zinc-700 rounded">
                  {plans.map(p => {
                    const checked = formData.limit_plan_ids.includes(p.id)
                    return (
                      <label
                        key={p.id}
                        className={`flex items-center gap-2 px-2 py-1.5 rounded cursor-pointer text-sm transition-colors ${
                          checked ? 'bg-indigo-900/40 text-indigo-300' : 'text-zinc-300 hover:bg-zinc-700/50'
                        }`}
                      >
                        <input
                          type="checkbox"
                          checked={checked}
                          onChange={() => togglePlanInLimit(p.id)}
                          className="rounded border-zinc-600"
                        />
                        <span className="truncate">{p.name}</span>
                      </label>
                    )
                  })}
                </div>
              )}
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label className="text-zinc-300 text-sm">开始时间</Label>
                <Input
                  type="datetime-local"
                  value={formData.starts_at}
                  onChange={(e) => setFormData({ ...formData, starts_at: e.target.value })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
                <p className="text-xs text-zinc-500">留空=立即生效</p>
              </div>
              <div className="space-y-2">
                <Label className="text-zinc-300 text-sm">结束时间</Label>
                <Input
                  type="datetime-local"
                  value={formData.expires_at}
                  onChange={(e) => setFormData({ ...formData, expires_at: e.target.value })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
                <p className="text-xs text-zinc-500">留空=永不过期</p>
              </div>
            </div>

            <div className="flex items-center justify-between">
              <Label className="text-zinc-300 text-sm">
                仅限新用户
                <span className="text-zinc-500 text-xs ml-1">（未购买过任何套餐的用户）</span>
              </Label>
              <Switch
                checked={formData.new_user_only}
                onChange={(e) => setFormData({ ...formData, new_user_only: e.target.checked })}
              />
            </div>

            <div className="flex items-center justify-between">
              <Label className="text-zinc-300 text-sm">启用优惠券</Label>
              <Switch
                checked={formData.is_active}
                onChange={(e) => setFormData({ ...formData, is_active: e.target.checked })}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={closeDialog}>
              取消
            </Button>
            <Button className="bg-indigo-600 hover:bg-indigo-500" onClick={handleSubmit} disabled={submitting}>
              {submitting ? '保存中...' : editingCoupon ? '更新' : '创建'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
