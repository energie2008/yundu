import { useState, useEffect } from 'react'
import { useForm } from 'react-hook-form'
import {
  Plus,
  Search,
  Pencil,
  Trash2,
  Copy,
  ChevronDown,
  ChevronRight,
  ArrowUp,
  ArrowDown,
  Filter,
  Route as RouteIcon,
  ListPlus,
  Layers,
} from 'lucide-react'
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
  Textarea,
  Switch,
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

type PolicyStatus = 'active' | 'inactive'
type OutboundAction = 'proxy' | 'direct' | 'block'
type RuleSource = 'rule_set' | 'inline'

interface RoutePolicyRule {
  id: string
  policy_id: string
  priority: number
  rule_source: RuleSource
  rule_set_id?: string
  rule_set_name?: string
  inline_values?: string[]
  outbound_action: OutboundAction
  outbound_tag?: string
  created_at: string
}

interface RoutePolicy {
  id: string
  code: string
  name: string
  description?: string
  status: PolicyStatus
  is_template?: boolean
  base_template_code?: string
  rules?: RoutePolicyRule[]
  rule_count?: number
  created_at: string
  updated_at: string
}

interface RouteRuleSet {
  id: string
  code: string
  name: string
  rule_type: string
  source_type: string
  status: string
}

interface ListResponse<T> {
  data?: T
  items?: T
  list?: T
  [key: string]: unknown
}

interface PolicyFormData {
  code: string
  name: string
  description: string
  status: boolean
}

interface CloneFormData {
  base_template_code: string
  code: string
  name: string
}

interface RuleFormData {
  rule_source: RuleSource
  rule_set_id: string
  inline_values: string
  outbound_action: OutboundAction
  outbound_tag: string
}

const DEFAULT_POLICY_FORM: PolicyFormData = {
  code: '',
  name: '',
  description: '',
  status: true,
}

const DEFAULT_CLONE_FORM: CloneFormData = {
  base_template_code: '',
  code: '',
  name: '',
}

const DEFAULT_RULE_FORM: RuleFormData = {
  rule_source: 'rule_set',
  rule_set_id: '',
  inline_values: '',
  outbound_action: 'proxy',
  outbound_tag: '',
}

const OUTBOUND_ACTION_LABEL: Record<OutboundAction, string> = {
  proxy: '代理',
  direct: '直连',
  block: '阻断',
}

const OUTBOUND_ACTION_BADGE: Record<OutboundAction, string> = {
  proxy: 'bg-indigo-900/50 text-indigo-300 border-indigo-800/50',
  direct: 'bg-emerald-900/50 text-emerald-300 border-emerald-800/50',
  block: 'bg-red-900/50 text-red-300 border-red-800/50',
}

const RULE_SOURCE_LABEL: Record<RuleSource, string> = {
  rule_set: '规则集',
  inline: '内联',
}

// ===== 工具函数 =====

function extractList<T>(resp: unknown): T[] {
  if (Array.isArray(resp)) return resp as T[]
  if (!resp || typeof resp !== 'object') return []
  const obj = resp as Record<string, unknown>
  // 处理嵌套结构：{code:0, data:{items:[...]}}
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

function formatTime(dateStr?: string) {
  if (!dateStr) return '-'
  try {
    return new Date(dateStr).toLocaleString('zh-CN', {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    })
  } catch {
    return dateStr
  }
}

function getStatusBadge(status: PolicyStatus) {
  if (status === 'active') {
    return (
      <Badge variant="outline" className="bg-emerald-900/50 text-emerald-300 border-emerald-800/50">
        启用
      </Badge>
    )
  }
  return (
    <Badge variant="outline" className="bg-zinc-800 text-zinc-400 border-zinc-700">
      禁用
    </Badge>
  )
}

function getOutboundBadge(action: OutboundAction) {
  return (
    <Badge variant="outline" className={OUTBOUND_ACTION_BADGE[action]}>
      {OUTBOUND_ACTION_LABEL[action]}
    </Badge>
  )
}

// ===== 主组件 =====

export default function RoutePolicies() {
  const { toast } = useToast()
  const [loading, setLoading] = useState(true)
  const [policies, setPolicies] = useState<RoutePolicy[]>([])
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<string>('all')

  const [expandedId, setExpandedId] = useState<string | null>(null)
  const [rulesByPolicy, setRulesByPolicy] = useState<Record<string, RoutePolicyRule[]>>({})
  const [loadingRulesId, setLoadingRulesId] = useState<string | null>(null)

  // 策略 Dialog
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<RoutePolicy | null>(null)
  const [submitting, setSubmitting] = useState(false)

  // 克隆 Dialog
  const [cloneOpen, setCloneOpen] = useState(false)
  const [cloneSubmitting, setCloneSubmitting] = useState(false)

  // 规则 Dialog
  const [ruleDialogOpen, setRuleDialogOpen] = useState(false)
  const [ruleSubmitting, setRuleSubmitting] = useState(false)
  const [rulePolicyId, setRulePolicyId] = useState<string | null>(null)

  // 操作 loading
  const [deletingId, setDeletingId] = useState<string | null>(null)
  const [ruleActionId, setRuleActionId] = useState<string | null>(null)

  // 规则集下拉数据
  const [ruleSets, setRuleSets] = useState<RouteRuleSet[]>([])

  const {
    register: registerPolicy,
    handleSubmit: handlePolicySubmit,
    reset: resetPolicy,
    watch: watchPolicy,
    setValue: setPolicyValue,
    formState: { errors: policyErrors },
  } = useForm<PolicyFormData>({ defaultValues: DEFAULT_POLICY_FORM })

  const {
    register: registerClone,
    handleSubmit: handleCloneSubmit,
    reset: resetClone,
    formState: { errors: cloneErrors },
  } = useForm<CloneFormData>({ defaultValues: DEFAULT_CLONE_FORM })

  const {
    register: registerRule,
    handleSubmit: handleRuleSubmit,
    reset: resetRule,
    watch: watchRule,
    setValue: setRuleValue,
    formState: { errors: ruleErrors },
  } = useForm<RuleFormData>({ defaultValues: DEFAULT_RULE_FORM })

  const watchPolicyStatus = watchPolicy('status')
  const watchRuleSource = watchRule('rule_source')

  // ===== 数据加载 =====

  const loadData = async () => {
    setLoading(true)
    try {
      const resp = await api.get<unknown>(EP.ROUTE_POLICIES)
      setPolicies(extractList<RoutePolicy>(resp))
    } catch (err) {
      toast({
        title: '加载失败',
        description: err instanceof Error ? err.message : '无法获取策略列表',
        variant: 'destructive',
      })
    } finally {
      setLoading(false)
    }
  }

  const loadRuleSets = async () => {
    try {
      const resp = await api.get<unknown>(EP.ROUTE_RULE_SETS)
      setRuleSets(extractList<RouteRuleSet>(resp))
    } catch {
      // 静默失败，规则编辑时再处理
    }
  }

  const loadRules = async (policyId: string) => {
    setLoadingRulesId(policyId)
    try {
      const resp = await api.get<unknown>(EP.ROUTE_POLICY(policyId))
      const detail = (resp && typeof resp === 'object' ? resp : {}) as { rules?: RoutePolicyRule[] }
      const rules = detail.rules || []
      setRulesByPolicy((prev) => ({ ...prev, [policyId]: rules }))
    } catch (err) {
      toast({
        title: '加载规则失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setLoadingRulesId(null)
    }
  }

  useEffect(() => {
    loadData()
  }, [])

  // ===== 过滤 =====

  const filtered = policies.filter((p) => {
    const matchesSearch =
      p.name.toLowerCase().includes(search.toLowerCase()) ||
      p.code.toLowerCase().includes(search.toLowerCase())
    const matchesStatus = statusFilter === 'all' || p.status === statusFilter
    return matchesSearch && matchesStatus
  })

  const templates = policies.filter((p) => p.is_template)

  // ===== 展开/收起 =====

  const toggleExpand = (p: RoutePolicy) => {
    if (expandedId === p.id) {
      setExpandedId(null)
      return
    }
    setExpandedId(p.id)
    if (!rulesByPolicy[p.id]) {
      loadRules(p.id)
    }
  }

  // ===== 策略表单 =====

  const openCreate = () => {
    setEditing(null)
    resetPolicy(DEFAULT_POLICY_FORM)
    setDialogOpen(true)
  }

  const openEdit = (p: RoutePolicy) => {
    setEditing(p)
    resetPolicy({
      code: p.code,
      name: p.name,
      description: p.description || '',
      status: p.status === 'active',
    })
    setDialogOpen(true)
  }

  const onPolicySubmit = async (data: PolicyFormData) => {
    if (!data.code.trim()) {
      toast({ title: '校验失败', description: '请填写策略编码', variant: 'destructive' })
      return
    }
    if (!/^[a-z0-9_-]+$/i.test(data.code.trim())) {
      toast({
        title: '校验失败',
        description: '编码只能包含字母、数字、下划线和连字符',
        variant: 'destructive',
      })
      return
    }
    if (!data.name.trim()) {
      toast({ title: '校验失败', description: '请填写策略名称', variant: 'destructive' })
      return
    }

    const payload = {
      code: data.code.trim(),
      name: data.name.trim(),
      description: data.description.trim(),
      status: data.status ? 'active' : 'inactive',
    }

    setSubmitting(true)
    try {
      if (editing) {
        await api.patch(EP.ROUTE_POLICY(editing.id), payload)
        toast({ title: '更新成功', description: `策略 ${payload.name} 已更新`, variant: 'success' })
      } else {
        await api.post(EP.ROUTE_POLICIES, payload)
        toast({ title: '创建成功', description: `策略 ${payload.name} 已创建`, variant: 'success' })
      }
      setDialogOpen(false)
      await loadData()
    } catch (err) {
      toast({
        title: editing ? '更新失败' : '创建失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setSubmitting(false)
    }
  }

  // ===== 删除策略 =====

  const handleDelete = async (p: RoutePolicy) => {
    if (p.is_template) {
      toast({
        title: '无法删除',
        description: '内置模板策略不可删除',
        variant: 'destructive',
      })
      return
    }
    if (!window.confirm(`确定删除策略「${p.name}」吗？此操作不可恢复。`)) return

    setDeletingId(p.id)
    try {
      await api.delete(EP.ROUTE_POLICY(p.id))
      toast({ title: '删除成功', description: `策略 ${p.name} 已删除`, variant: 'success' })
      if (expandedId === p.id) setExpandedId(null)
      await loadData()
    } catch (err) {
      toast({
        title: '删除失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setDeletingId(null)
    }
  }

  // ===== 克隆模板 =====

  const openClone = () => {
    resetClone(DEFAULT_CLONE_FORM)
    if (ruleSets.length === 0) loadRuleSets()
    if (templates.length === 0) {
      toast({
        title: '暂无模板',
        description: '当前没有可用的内置模板策略',
        variant: 'destructive',
      })
      return
    }
    setCloneOpen(true)
  }

  const onCloneSubmit = async (data: CloneFormData) => {
    if (!data.base_template_code) {
      toast({ title: '校验失败', description: '请选择基础模板', variant: 'destructive' })
      return
    }
    if (!data.code.trim() || !/^[a-z0-9_-]+$/i.test(data.code.trim())) {
      toast({
        title: '校验失败',
        description: '请输入合法的策略编码（字母数字下划线连字符）',
        variant: 'destructive',
      })
      return
    }
    if (!data.name.trim()) {
      toast({ title: '校验失败', description: '请填写策略名称', variant: 'destructive' })
      return
    }

    // 后端路由：POST /admin/route-policies/:id/clone（:id 为模板策略 ID）
    const template = templates.find((t) => t.code === data.base_template_code)
    if (!template) {
      toast({ title: '校验失败', description: '所选模板不存在', variant: 'destructive' })
      return
    }

    const payload = {
      code: data.code.trim(),
      name: data.name.trim(),
    }

    setCloneSubmitting(true)
    try {
      await api.post(EP.ROUTE_POLICY_CLONE(template.id), payload)
      toast({ title: '克隆成功', description: `策略 ${payload.name} 已从模板创建`, variant: 'success' })
      setCloneOpen(false)
      await loadData()
    } catch (err) {
      toast({
        title: '克隆失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setCloneSubmitting(false)
    }
  }

  // ===== 规则管理 =====

  const openAddRule = (policyId: string) => {
    setRulePolicyId(policyId)
    resetRule(DEFAULT_RULE_FORM)
    if (ruleSets.length === 0) loadRuleSets()
    setRuleDialogOpen(true)
  }

  const onRuleSubmit = async (data: RuleFormData) => {
    if (!rulePolicyId) return

    // 校验
    if (data.rule_source === 'rule_set' && !data.rule_set_id) {
      toast({ title: '校验失败', description: '请选择规则集', variant: 'destructive' })
      return
    }
    if (data.rule_source === 'inline') {
      const values = data.inline_values
        .split('\n')
        .map((v) => v.trim())
        .filter(Boolean)
      if (values.length === 0) {
        toast({ title: '校验失败', description: '请至少输入一条内联规则', variant: 'destructive' })
        return
      }
    }

    const payload: Record<string, unknown> = {
      rule_source: data.rule_source,
      outbound_action: data.outbound_action,
      outbound_tag: data.outbound_tag.trim(),
    }

    if (data.rule_source === 'rule_set') {
      payload.rule_set_id = data.rule_set_id
    } else {
      payload.inline_values = data.inline_values
        .split('\n')
        .map((v) => v.trim())
        .filter(Boolean)
    }

    setRuleSubmitting(true)
    try {
      await api.post(EP.ROUTE_POLICY_RULES(rulePolicyId), payload)
      toast({ title: '添加成功', description: '规则已添加到策略', variant: 'success' })
      setRuleDialogOpen(false)
      await loadRules(rulePolicyId)
    } catch (err) {
      toast({
        title: '添加失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setRuleSubmitting(false)
    }
  }

  const handleDeleteRule = async (rule: RoutePolicyRule) => {
    if (!window.confirm('确定删除该规则吗？')) return

    setRuleActionId(rule.id)
    try {
      await api.delete(EP.ROUTE_POLICY_RULE(rule.id))
      toast({ title: '删除成功', description: '规则已删除', variant: 'success' })
      if (rule.policy_id) await loadRules(rule.policy_id)
    } catch (err) {
      toast({
        title: '删除失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setRuleActionId(null)
    }
  }

  const handleReorder = async (policyId: string, ruleId: string, direction: 'up' | 'down') => {
    const rules = rulesByPolicy[policyId] || []
    const idx = rules.findIndex((r) => r.id === ruleId)
    if (idx < 0) return
    const targetIdx = direction === 'up' ? idx - 1 : idx + 1
    if (targetIdx < 0 || targetIdx >= rules.length) return

    const newOrder = [...rules]
    const [moved] = newOrder.splice(idx, 1)
    newOrder.splice(targetIdx, 0, moved)
    const orderedIds = newOrder.map((r) => r.id)

    // 乐观更新
    setRulesByPolicy((prev) => ({
      ...prev,
      [policyId]: newOrder.map((r, i) => ({ ...r, priority: i + 1 })),
    }))

    setRuleActionId(ruleId)
    try {
      await api.post(EP.ROUTE_POLICY_REORDER(policyId), { rule_ids: orderedIds })
      toast({ title: '排序已更新', variant: 'success' })
    } catch (err) {
      toast({
        title: '排序失败',
        description: err instanceof Error ? err.message : '已回滚',
        variant: 'destructive',
      })
      await loadRules(policyId)
    } finally {
      setRuleActionId(null)
    }
  }

  // ===== 渲染规则列表 =====

  const renderRules = (policy: RoutePolicy) => {
    const rules = rulesByPolicy[policy.id] || policy.rules || []
    const isLoading = loadingRulesId === policy.id

    if (isLoading) {
      return (
        <div className="space-y-2 p-3">
          {[1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-12 w-full bg-zinc-800 rounded-lg" />
          ))}
        </div>
      )
    }

    if (rules.length === 0) {
      return (
        <div className="p-4">
          <EmptyState
            icon={<ListPlus className="h-6 w-6" />}
            title="暂无规则"
            description="添加规则以控制流量出站"
            className="py-6"
            action={
              <Button
                size="sm"
                className="bg-indigo-600 hover:bg-indigo-500"
                onClick={() => openAddRule(policy.id)}
              >
                <Plus className="w-4 h-4 mr-1" />
                添加规则
              </Button>
            }
          />
        </div>
      )
    }

    return (
      <div className="p-3 space-y-2">
        {/* 移动端卡片 */}
        <div className="sm:hidden space-y-2">
          {rules.map((rule, idx) => (
            <div
              key={rule.id}
              className="rounded-lg border border-zinc-800 bg-zinc-950/50 p-3"
            >
              <div className="flex items-start justify-between gap-2 mb-2">
                <div className="flex items-center gap-2">
                  <span className="text-xs text-zinc-500 font-mono">#{idx + 1}</span>
                  <Badge variant="outline" className="bg-zinc-800 text-zinc-300 border-zinc-700 text-xs">
                    {RULE_SOURCE_LABEL[rule.rule_source]}
                  </Badge>
                  {getOutboundBadge(rule.outbound_action)}
                </div>
                <div className="flex items-center gap-1">
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-7 w-7 text-zinc-400"
                    disabled={idx === 0 || ruleActionId === rule.id}
                    onClick={() => handleReorder(policy.id, rule.id, 'up')}
                  >
                    <ArrowUp className="w-3.5 h-3.5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-7 w-7 text-zinc-400"
                    disabled={idx === rules.length - 1 || ruleActionId === rule.id}
                    onClick={() => handleReorder(policy.id, rule.id, 'down')}
                  >
                    <ArrowDown className="w-3.5 h-3.5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-7 w-7 text-zinc-400 hover:text-red-400"
                    disabled={ruleActionId === rule.id}
                    onClick={() => handleDeleteRule(rule)}
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </Button>
                </div>
              </div>
              <div className="space-y-1 text-xs text-zinc-400">
                {rule.rule_source === 'rule_set' ? (
                  <div>
                    <span className="text-zinc-500">规则集：</span>
                    <span className="text-zinc-200">{rule.rule_set_name || rule.rule_set_id}</span>
                  </div>
                ) : (
                  <div>
                    <span className="text-zinc-500">内联值：</span>
                    <span className="text-zinc-200 font-mono">
                      {(rule.inline_values || []).join(', ') || '-'}
                    </span>
                  </div>
                )}
                {rule.outbound_tag && (
                  <div>
                    <span className="text-zinc-500">出站标签：</span>
                    <code className="text-zinc-200">{rule.outbound_tag}</code>
                  </div>
                )}
              </div>
            </div>
          ))}
        </div>

        {/* 桌面端表格 */}
        <div className="hidden sm:block overflow-x-auto rounded-lg border border-zinc-800">
          <table className="w-full">
            <thead>
              <tr className="border-b border-zinc-800 bg-zinc-950/50">
                <th className="text-left text-xs font-medium text-zinc-400 px-3 py-2 w-12">序号</th>
                <th className="text-left text-xs font-medium text-zinc-400 px-3 py-2">来源</th>
                <th className="text-left text-xs font-medium text-zinc-400 px-3 py-2">匹配内容</th>
                <th className="text-left text-xs font-medium text-zinc-400 px-3 py-2">出站动作</th>
                <th className="text-left text-xs font-medium text-zinc-400 px-3 py-2">出站标签</th>
                <th className="text-right text-xs font-medium text-zinc-400 px-3 py-2 w-28">操作</th>
              </tr>
            </thead>
            <tbody>
              {rules.map((rule, idx) => (
                <tr key={rule.id} className="border-b border-zinc-800 last:border-0 hover:bg-zinc-800/30">
                  <td className="px-3 py-2 text-xs text-zinc-500 font-mono">#{idx + 1}</td>
                  <td className="px-3 py-2">
                    <Badge variant="outline" className="bg-zinc-800 text-zinc-300 border-zinc-700 text-xs">
                      {RULE_SOURCE_LABEL[rule.rule_source]}
                    </Badge>
                  </td>
                  <td className="px-3 py-2 text-sm text-zinc-300 max-w-xs">
                    {rule.rule_source === 'rule_set' ? (
                      <span>{rule.rule_set_name || rule.rule_set_id}</span>
                    ) : (
                      <code className="text-xs font-mono text-zinc-400">
                        {(rule.inline_values || []).slice(0, 3).join(', ')}
                        {(rule.inline_values || []).length > 3 && '...'}
                      </code>
                    )}
                  </td>
                  <td className="px-3 py-2">{getOutboundBadge(rule.outbound_action)}</td>
                  <td className="px-3 py-2">
                    {rule.outbound_tag ? (
                      <code className="text-xs text-zinc-300 font-mono">{rule.outbound_tag}</code>
                    ) : (
                      <span className="text-zinc-600">-</span>
                    )}
                  </td>
                  <td className="px-3 py-2">
                    <div className="flex items-center justify-end gap-1">
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-7 w-7 text-zinc-400"
                        disabled={idx === 0 || ruleActionId === rule.id}
                        onClick={() => handleReorder(policy.id, rule.id, 'up')}
                        title="上移"
                      >
                        <ArrowUp className="w-3.5 h-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-7 w-7 text-zinc-400"
                        disabled={idx === rules.length - 1 || ruleActionId === rule.id}
                        onClick={() => handleReorder(policy.id, rule.id, 'down')}
                        title="下移"
                      >
                        <ArrowDown className="w-3.5 h-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-7 w-7 text-zinc-400 hover:text-red-400"
                        disabled={ruleActionId === rule.id}
                        onClick={() => handleDeleteRule(rule)}
                        title="删除"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <div className="flex justify-end pt-1">
          <Button
            size="sm"
            variant="outline"
            className="border-zinc-700 text-zinc-300"
            onClick={() => openAddRule(policy.id)}
          >
            <Plus className="w-4 h-4 mr-1" />
            添加规则
          </Button>
        </div>
      </div>
    )
  }

  // ===== 渲染 =====

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between flex-wrap gap-2">
        <h2 className="text-lg font-semibold text-zinc-100">路由策略</h2>
        <div className="flex items-center gap-2">
          <Button
            size="sm"
            variant="outline"
            className="border-zinc-700 text-zinc-300"
            onClick={openClone}
          >
            <Copy className="w-4 h-4 mr-1" />
            克隆模板
          </Button>
          <Button size="sm" className="bg-indigo-600 hover:bg-indigo-500" onClick={openCreate}>
            <Plus className="w-4 h-4 mr-1" />
            新建策略
          </Button>
        </div>
      </div>

      {/* 筛选 */}
      <Card className="bg-zinc-900 border-zinc-800">
        <CardContent className="p-3">
          <div className="flex flex-col sm:flex-row gap-2">
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-zinc-500" />
              <Input
                placeholder="搜索策略编码或名称..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="pl-9 bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500"
              />
            </div>
            <Select
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value)}
              className="bg-zinc-800 border-zinc-700 text-zinc-100 sm:w-40"
            >
              <option value="all">全部状态</option>
              <option value="active">启用</option>
              <option value="inactive">禁用</option>
            </Select>
          </div>
        </CardContent>
      </Card>

      {/* 列表 */}
      {loading ? (
        <div className="space-y-3">
          {[1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-24 w-full bg-zinc-800 rounded-lg" />
          ))}
        </div>
      ) : filtered.length === 0 ? (
        <Card className="bg-zinc-900 border-zinc-800">
          <CardContent>
            <EmptyState
              icon={<Filter className="h-6 w-6" />}
              title="暂无策略"
              description="新建策略或从模板克隆以开始管理路由"
              className="py-12"
            />
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-3">
          {filtered.map((p) => {
            const isExpanded = expandedId === p.id
            const ruleCount = rulesByPolicy[p.id]?.length ?? p.rule_count ?? p.rules?.length ?? 0
            return (
              <Card key={p.id} className="bg-zinc-900 border-zinc-800">
                <CardHeader className="pb-3">
                  <div className="flex items-center justify-between gap-2">
                    <button
                      type="button"
                      className="flex items-center gap-3 flex-1 min-w-0 text-left"
                      onClick={() => toggleExpand(p)}
                    >
                      <div className={`p-2 rounded-lg ${p.status === 'active' ? 'bg-indigo-600/20' : 'bg-zinc-800'}`}>
                        <RouteIcon className={`w-4 h-4 ${p.status === 'active' ? 'text-indigo-400' : 'text-zinc-500'}`} />
                      </div>
                      <div className="min-w-0 flex-1">
                        <CardTitle className="text-sm font-medium text-zinc-100 flex items-center gap-2 flex-wrap">
                          <span className="truncate">{p.name}</span>
                          {getStatusBadge(p.status)}
                          {p.is_template && (
                            <Badge variant="outline" className="bg-amber-900/50 text-amber-300 border-amber-800/50">
                              <Layers className="w-3 h-3 mr-1" />
                              模板
                            </Badge>
                          )}
                          <Badge variant="outline" className="bg-zinc-800 text-zinc-400 border-zinc-700">
                            {ruleCount} 规则
                          </Badge>
                        </CardTitle>
                        <p className="text-xs text-zinc-500 mt-0.5 truncate">
                          <code className="font-mono">{p.code}</code>
                          {p.description && <span className="ml-2">· {p.description}</span>}
                        </p>
                      </div>
                      {isExpanded ? (
                        <ChevronDown className="w-4 h-4 text-zinc-500 flex-shrink-0" />
                      ) : (
                        <ChevronRight className="w-4 h-4 text-zinc-500 flex-shrink-0" />
                      )}
                    </button>
                    <div className="flex items-center gap-1 flex-shrink-0">
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8 text-zinc-400 hover:text-zinc-200"
                        onClick={() => openEdit(p)}
                        title="编辑"
                      >
                        <Pencil className="w-4 h-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8 text-zinc-400 hover:text-red-400"
                        onClick={() => handleDelete(p)}
                        disabled={deletingId === p.id || !!p.is_template}
                        title={p.is_template ? '模板策略不可删除' : '删除'}
                      >
                        <Trash2 className="w-4 h-4" />
                      </Button>
                    </div>
                  </div>
                </CardHeader>
                {isExpanded && (
                  <CardContent className="pt-0 border-t border-zinc-800">
                    <div className="flex items-center justify-between pt-3 pb-1">
                      <h3 className="text-xs font-medium text-zinc-400 uppercase tracking-wide">规则列表</h3>
                      <span className="text-xs text-zinc-500">
                        更新于 {formatTime(p.updated_at)}
                      </span>
                    </div>
                    {renderRules(p)}
                  </CardContent>
                )}
              </Card>
            )
          })}
        </div>
      )}

      {/* 新建/编辑策略 Dialog */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-md">
          <DialogHeader>
            <DialogTitle>{editing ? '编辑策略' : '新建策略'}</DialogTitle>
            <DialogDescription>
              {editing ? `修改策略 ${editing.name}` : '创建一个新的路由策略'}
            </DialogDescription>
          </DialogHeader>

          <form onSubmit={handlePolicySubmit(onPolicySubmit)} className="space-y-4 pt-2">
            <div className="space-y-2">
              <Label htmlFor="policy-code" className="text-zinc-300">
                编码 <span className="text-red-400">*</span>
              </Label>
              <Input
                id="policy-code"
                placeholder="如：cn-policy"
                className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500 focus:border-indigo-500 font-mono"
                disabled={!!editing}
                {...registerPolicy('code', { required: '请输入编码' })}
              />
              {policyErrors.code && (
                <p className="text-sm text-red-400">{policyErrors.code.message}</p>
              )}
              <p className="text-xs text-zinc-500">唯一标识，创建后不可修改</p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="policy-name" className="text-zinc-300">
                名称 <span className="text-red-400">*</span>
              </Label>
              <Input
                id="policy-name"
                placeholder="如：国内分流策略"
                className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500 focus:border-indigo-500"
                {...registerPolicy('name', { required: '请输入名称' })}
              />
              {policyErrors.name && (
                <p className="text-sm text-red-400">{policyErrors.name.message}</p>
              )}
            </div>

            <div className="space-y-2">
              <Label htmlFor="policy-desc" className="text-zinc-300">
                描述
              </Label>
              <Input
                id="policy-desc"
                placeholder="策略描述（可选）"
                className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500 focus:border-indigo-500"
                {...registerPolicy('description')}
              />
            </div>

            <div className="flex items-center justify-between py-2 px-3 rounded-lg bg-zinc-800/50 border border-zinc-800">
              <div>
                <Label className="text-zinc-300 cursor-pointer">启用状态</Label>
                <p className="text-xs text-zinc-500 mt-0.5">禁用后策略不会被加载</p>
              </div>
              <Switch
                checked={!!watchPolicyStatus}
                onChange={(e) => setPolicyValue('status', e.target.checked)}
              />
            </div>

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setDialogOpen(false)}
                disabled={submitting}
                className="border-zinc-700 text-zinc-300"
              >
                取消
              </Button>
              <Button
                type="submit"
                isLoading={submitting}
                className="bg-indigo-600 hover:bg-indigo-500"
              >
                {editing ? '保存' : '创建'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* 克隆模板 Dialog */}
      <Dialog open={cloneOpen} onOpenChange={setCloneOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-md">
          <DialogHeader>
            <DialogTitle>克隆模板策略</DialogTitle>
            <DialogDescription>从内置模板创建新的策略</DialogDescription>
          </DialogHeader>

          <form onSubmit={handleCloneSubmit(onCloneSubmit)} className="space-y-4 pt-2">
            <div className="space-y-2">
              <Label htmlFor="clone-template" className="text-zinc-300">
                基础模板 <span className="text-red-400">*</span>
              </Label>
              <Select
                id="clone-template"
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
                {...registerClone('base_template_code', { required: '请选择模板' })}
              >
                <option value="">请选择模板...</option>
                {templates.map((t) => (
                  <option key={t.id} value={t.code}>
                    {t.name} ({t.code})
                  </option>
                ))}
              </Select>
              {cloneErrors.base_template_code && (
                <p className="text-sm text-red-400">{cloneErrors.base_template_code.message}</p>
              )}
              {templates.length === 0 && (
                <p className="text-xs text-amber-400">当前无可用模板</p>
              )}
            </div>

            <div className="space-y-2">
              <Label htmlFor="clone-code" className="text-zinc-300">
                新策略编码 <span className="text-red-400">*</span>
              </Label>
              <Input
                id="clone-code"
                placeholder="如：cn-policy-v2"
                className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500 focus:border-indigo-500 font-mono"
                {...registerClone('code', { required: '请输入编码' })}
              />
              {cloneErrors.code && (
                <p className="text-sm text-red-400">{cloneErrors.code.message}</p>
              )}
            </div>

            <div className="space-y-2">
              <Label htmlFor="clone-name" className="text-zinc-300">
                新策略名称 <span className="text-red-400">*</span>
              </Label>
              <Input
                id="clone-name"
                placeholder="如：国内分流策略 v2"
                className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500 focus:border-indigo-500"
                {...registerClone('name', { required: '请输入名称' })}
              />
              {cloneErrors.name && (
                <p className="text-sm text-red-400">{cloneErrors.name.message}</p>
              )}
            </div>

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setCloneOpen(false)}
                disabled={cloneSubmitting}
                className="border-zinc-700 text-zinc-300"
              >
                取消
              </Button>
              <Button
                type="submit"
                isLoading={cloneSubmitting}
                className="bg-indigo-600 hover:bg-indigo-500"
              >
                克隆
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* 添加规则 Dialog */}
      <Dialog open={ruleDialogOpen} onOpenChange={setRuleDialogOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-md max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>添加规则</DialogTitle>
            <DialogDescription>为策略添加一条路由规则</DialogDescription>
          </DialogHeader>

          <form onSubmit={handleRuleSubmit(onRuleSubmit)} className="space-y-4 pt-2">
            <div className="space-y-2">
              <Label className="text-zinc-300">规则来源</Label>
              <Select
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
                {...registerRule('rule_source')}
              >
                <option value="rule_set">规则集</option>
                <option value="inline">内联</option>
              </Select>
              <p className="text-xs text-zinc-500">
                选择规则集则从已配置的规则集加载，内联则手动输入匹配值
              </p>
            </div>

            {watchRuleSource === 'rule_set' ? (
              <div className="space-y-2">
                <Label htmlFor="rule-set" className="text-zinc-300">
                  规则集 <span className="text-red-400">*</span>
                </Label>
                <Select
                  id="rule-set"
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                  {...registerRule('rule_set_id')}
                >
                  <option value="">请选择规则集...</option>
                  {ruleSets
                    .filter((rs) => rs.status === 'active')
                    .map((rs) => (
                      <option key={rs.id} value={rs.id}>
                        {rs.name} ({rs.code})
                      </option>
                    ))}
                </Select>
                {ruleErrors.rule_set_id && (
                  <p className="text-sm text-red-400">{ruleErrors.rule_set_id.message}</p>
                )}
                {ruleSets.length === 0 && (
                  <p className="text-xs text-amber-400">
                    暂无可用规则集，请先在「路由规则集」页面创建
                  </p>
                )}
              </div>
            ) : (
              <div className="space-y-2">
                <Label htmlFor="inline-values" className="text-zinc-300">
                  内联规则值 <span className="text-red-400">*</span>
                </Label>
                <Textarea
                  id="inline-values"
                  rows={4}
                  placeholder={'每行一条规则，例如：\ndomain:example.com\ndomain-suffix:google.com\ngeosite:cn'}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500 focus:border-indigo-500 font-mono text-xs"
                  {...registerRule('inline_values')}
                />
                <p className="text-xs text-zinc-500">每行一条规则</p>
              </div>
            )}

            <div className="space-y-2">
              <Label htmlFor="outbound-action" className="text-zinc-300">
                出站动作 <span className="text-red-400">*</span>
              </Label>
              <Select
                id="outbound-action"
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
                {...registerRule('outbound_action')}
              >
                <option value="proxy">代理（走代理出站）</option>
                <option value="direct">直连（不走代理）</option>
                <option value="block">阻断（拒绝连接）</option>
              </Select>
            </div>

            <div className="space-y-2">
              <Label htmlFor="outbound-tag" className="text-zinc-300">
                出站标签
              </Label>
              <Input
                id="outbound-tag"
                placeholder="如：proxy-hk-01（仅代理动作需要）"
                className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500 focus:border-indigo-500 font-mono"
                {...registerRule('outbound_tag')}
              />
              <p className="text-xs text-zinc-500">指定使用的出站节点标签</p>
            </div>

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setRuleDialogOpen(false)}
                disabled={ruleSubmitting}
                className="border-zinc-700 text-zinc-300"
              >
                取消
              </Button>
              <Button
                type="submit"
                isLoading={ruleSubmitting}
                className="bg-indigo-600 hover:bg-indigo-500"
              >
                添加
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}
