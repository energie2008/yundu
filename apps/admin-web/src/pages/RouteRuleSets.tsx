import { useState, useEffect, useCallback } from 'react'
import {
  Route,
  Plus,
  Search,
  Trash2,
  Pencil,
  RefreshCw,
  Layers,
} from 'lucide-react'
import {
  Card,
  CardContent,
  Button,
  Input,
  Label,
  Badge,
  Select,
  Switch,
  Skeleton,
  EmptyState,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  Textarea,
  useToast,
} from '@airport/ui'
import { api } from '@/lib/api'
import { EP } from '@/lib/endpoints'

// ===== 类型定义（对齐后端 routing/model.go RuleSetResponse）=====

type RuleType = 'builtin' | 'custom'
type SourceType = 'inline' | 'geosite' | 'geoip' | 'remote_url'

interface RuleSet {
  id: string
  code: string
  name: string
  description?: string
  rule_type: string
  source_type: string
  source_url?: string
  content: string[]
  auto_update: boolean
  last_synced_at?: string
  status: string
  created_at: string
  updated_at: string
}

interface ListResponse {
  page: number
  page_size: number
  total: number
  items: RuleSet[]
}

const RULE_TYPE_OPTIONS: { value: RuleType; label: string }[] = [
  { value: 'custom', label: '自定义 (custom)' },
  { value: 'builtin', label: '内置 (builtin)' },
]

const SOURCE_TYPE_OPTIONS: { value: SourceType; label: string }[] = [
  { value: 'inline', label: '内联 (inline)' },
  { value: 'geosite', label: 'geosite' },
  { value: 'geoip', label: 'geoip' },
  { value: 'remote_url', label: '远程 URL (remote_url)' },
]

const RULE_TYPE_BADGE: Record<string, string> = {
  builtin: 'bg-indigo-900/50 text-indigo-300 border-indigo-800/50',
  custom: 'bg-zinc-800 text-zinc-300 border-zinc-700',
}

const SOURCE_TYPE_BADGE: Record<string, string> = {
  inline: 'bg-emerald-900/50 text-emerald-300 border-emerald-800/50',
  geosite: 'bg-cyan-900/50 text-cyan-300 border-cyan-800/50',
  geoip: 'bg-amber-900/50 text-amber-300 border-amber-800/50',
  remote_url: 'bg-violet-900/50 text-violet-300 border-violet-800/50',
}

function statusBadge(status?: string) {
  if (!status) return <Badge variant="secondary" className="bg-zinc-800 text-zinc-400 text-xs">-</Badge>
  const ok = status === 'active' || status === 'enabled'
  const cls = ok
    ? 'bg-emerald-900/50 text-emerald-300 border-emerald-800/50'
    : 'bg-zinc-800 text-zinc-400 border-zinc-700'
  return <Badge variant="outline" className={`${cls} text-xs`}>{status}</Badge>
}

interface FormState {
  code: string
  name: string
  description: string
  rule_type: RuleType
  source_type: SourceType
  source_url: string
  content: string // textarea 文本，按行拆分为数组
  auto_update: boolean
}

const EMPTY_FORM: FormState = {
  code: '',
  name: '',
  description: '',
  rule_type: 'custom',
  source_type: 'inline',
  source_url: '',
  content: '',
  auto_update: false,
}

function contentToText(content: string[] | undefined): string {
  if (!content || !Array.isArray(content)) return ''
  return content.join('\n')
}

function textToContent(text: string): string[] {
  return text
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean)
}

export default function RouteRuleSets() {
  const { toast } = useToast()
  const [loading, setLoading] = useState(true)
  const [ruleSets, setRuleSets] = useState<RuleSet[]>([])
  const [search, setSearch] = useState('')

  const [dialogOpen, setDialogOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [form, setForm] = useState<FormState>(EMPTY_FORM)

  const [syncingId, setSyncingId] = useState<string | null>(null)

  const loadRuleSets = useCallback(async () => {
    setLoading(true)
    try {
      const data = await api.get<ListResponse>(EP.ROUTE_RULE_SETS, {
        params: { page: 1, page_size: 200 },
      })
      setRuleSets(data.items || [])
    } catch (err) {
      toast({
        title: '加载失败',
        description: err instanceof Error ? err.message : '无法获取规则集列表',
        variant: 'destructive',
      })
    } finally {
      setLoading(false)
    }
  }, [toast])

  useEffect(() => {
    loadRuleSets()
  }, [loadRuleSets])

  const filtered = ruleSets.filter((rs) => {
    const kw = search.trim().toLowerCase()
    if (!kw) return true
    return (
      (rs.name || '').toLowerCase().includes(kw) ||
      (rs.code || '').toLowerCase().includes(kw) ||
      (rs.description || '').toLowerCase().includes(kw)
    )
  })

  function openCreate() {
    setEditingId(null)
    setForm(EMPTY_FORM)
    setDialogOpen(true)
  }

  function openEdit(rs: RuleSet) {
    setEditingId(rs.id)
    setForm({
      code: rs.code || '',
      name: rs.name || '',
      description: rs.description || '',
      rule_type: (rs.rule_type as RuleType) || 'custom',
      source_type: (rs.source_type as SourceType) || 'inline',
      source_url: rs.source_url || '',
      content: contentToText(rs.content),
      auto_update: !!rs.auto_update,
    })
    setDialogOpen(true)
  }

  async function handleSave() {
    if (!form.name.trim()) {
      toast({ title: '校验失败', description: '请输入规则集名称', variant: 'destructive' })
      return
    }
    if (!editingId && !form.code.trim()) {
      toast({ title: '校验失败', description: '请输入规则集编码 (code)', variant: 'destructive' })
      return
    }

    setSaving(true)
    try {
      if (editingId) {
        // PATCH：后端 UpdateRuleSetRequest 仅允许更新 name/description/content/auto_update/status
        const payload: Record<string, unknown> = {
          name: form.name.trim(),
          description: form.description.trim(),
          content: textToContent(form.content),
          auto_update: form.auto_update,
        }
        await api.patch(EP.ROUTE_RULE_SET(editingId), payload)
        toast({ title: '规则集已更新', variant: 'success' })
      } else {
        // POST：后端 CreateRuleSetRequest
        const payload: Record<string, unknown> = {
          code: form.code.trim(),
          name: form.name.trim(),
          description: form.description.trim(),
          rule_type: form.rule_type,
          source_type: form.source_type,
          content: textToContent(form.content),
          auto_update: form.auto_update,
        }
        if (form.source_type === 'remote_url' && form.source_url.trim()) {
          payload.source_url = form.source_url.trim()
        }
        await api.post(EP.ROUTE_RULE_SETS, payload)
        toast({ title: '规则集已创建', variant: 'success' })
      }
      setDialogOpen(false)
      await loadRuleSets()
    } catch (err) {
      toast({
        title: '保存失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete(rs: RuleSet) {
    if (!window.confirm(`确定删除规则集「${rs.name}」吗？此操作不可撤销。`)) return
    try {
      await api.delete(EP.ROUTE_RULE_SET(rs.id))
      toast({ title: '规则集已删除', variant: 'success' })
      await loadRuleSets()
    } catch (err) {
      toast({
        title: '删除失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    }
  }

  async function handleSync(rs: RuleSet) {
    setSyncingId(rs.id)
    try {
      await api.post(EP.ROUTE_RULE_SET_SYNC(rs.id), {})
      toast({ title: '同步成功', description: `规则集「${rs.name}」已从远端同步`, variant: 'success' })
      await loadRuleSets()
    } catch (err) {
      toast({
        title: '同步失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setSyncingId(null)
    }
  }

  const isEdit = !!editingId

  return (
    <div className="space-y-4 pb-20 sm:pb-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold text-zinc-100 flex items-center gap-2">
            <Route className="w-5 h-5 text-indigo-400" />
            分流规则
          </h2>
          <p className="text-sm text-zinc-400 mt-0.5">
            管理全局 RuleSet 规则集，规则统一下发到所有客户端
          </p>
        </div>
        <Button size="sm" className="bg-indigo-600 hover:bg-indigo-500" onClick={openCreate}>
          <Plus className="w-4 h-4 mr-1" />新建规则集
        </Button>
      </div>

      <Card className="bg-zinc-900 border-zinc-800">
        <CardContent className="p-3">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-zinc-500" />
            <Input
              placeholder="搜索规则集名称、编码或描述..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="pl-9 bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500 h-9"
            />
          </div>
        </CardContent>
      </Card>

      <Card className="bg-zinc-900 border-zinc-800 overflow-hidden">
        <CardContent className="p-0">
          {loading ? (
            <div className="p-4 space-y-3">
              {[1, 2, 3, 4].map((i) => (
                <Skeleton key={i} className="h-14 w-full bg-zinc-800 rounded-lg" />
              ))}
            </div>
          ) : filtered.length === 0 ? (
            <EmptyState
              title="暂无规则集"
              description="新建规则集来管理流量分流"
              className="py-12"
            />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-zinc-800">
                    <th className="text-left p-3 text-xs font-medium text-zinc-400">名称 / 编码</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400 hidden md:table-cell">描述</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400 w-24">类型</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400 w-28">来源</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400 w-20">条目</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400 w-20">状态</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400 w-28">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {filtered.map((rs) => (
                    <tr
                      key={rs.id}
                      className="border-b border-zinc-800 last:border-0 hover:bg-zinc-800/50 transition-colors"
                    >
                      <td className="p-3">
                        <div className="flex items-center gap-2">
                          <div className="p-1.5 rounded-md bg-indigo-900/30">
                            <Layers className="w-4 h-4 text-indigo-400" />
                          </div>
                          <div className="min-w-0">
                            <div className="text-sm font-medium text-zinc-200 truncate">{rs.name}</div>
                            <code className="text-[11px] text-zinc-500 font-mono">{rs.code}</code>
                          </div>
                        </div>
                      </td>
                      <td className="p-3 hidden md:table-cell">
                        <span className="text-sm text-zinc-400 line-clamp-2">{rs.description || '-'}</span>
                      </td>
                      <td className="p-3">
                        <Badge variant="outline" className={`${RULE_TYPE_BADGE[rs.rule_type] || RULE_TYPE_BADGE.custom} text-xs`}>
                          {rs.rule_type}
                        </Badge>
                      </td>
                      <td className="p-3">
                        <Badge variant="outline" className={`${SOURCE_TYPE_BADGE[rs.source_type] || 'bg-zinc-800 text-zinc-300 border-zinc-700'} text-xs`}>
                          {rs.source_type}
                        </Badge>
                      </td>
                      <td className="p-3">
                        <span className="text-sm text-zinc-300 font-mono">{rs.content?.length || 0}</span>
                      </td>
                      <td className="p-3">{statusBadge(rs.status)}</td>
                      <td className="p-3">
                        <div className="flex items-center gap-1">
                          {rs.source_type === 'remote_url' && (
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-8 w-8 text-zinc-400 hover:text-cyan-300"
                              onClick={() => handleSync(rs)}
                              disabled={syncingId === rs.id}
                              title="从远端同步"
                            >
                              <RefreshCw className={`w-4 h-4 ${syncingId === rs.id ? 'animate-spin' : ''}`} />
                            </Button>
                          )}
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-8 w-8 text-zinc-400 hover:text-indigo-300"
                            onClick={() => openEdit(rs)}
                            title="编辑"
                          >
                            <Pencil className="w-4 h-4" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-8 w-8 text-zinc-400 hover:text-red-300"
                            onClick={() => handleDelete(rs)}
                            title="删除"
                          >
                            <Trash2 className="w-4 h-4" />
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-lg">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Route className="w-5 h-5 text-indigo-400" />
              <span>{isEdit ? '编辑规则集' : '新建规则集'}</span>
            </DialogTitle>
          </DialogHeader>

          <div className="space-y-3 pt-2 max-h-[65vh] overflow-y-auto pr-1">
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label className="text-zinc-300 text-sm">编码 (code) *</Label>
                <Input
                  value={form.code}
                  onChange={(e) => setForm({ ...form, code: e.target.value })}
                  placeholder="如: cn-direct"
                  disabled={isEdit}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100 h-9 font-mono text-xs disabled:opacity-60"
                />
              </div>
              <div className="space-y-1.5">
                <Label className="text-zinc-300 text-sm">名称 *</Label>
                <Input
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  placeholder="如: 国内直连"
                  className="bg-zinc-800 border-zinc-700 text-zinc-100 h-9"
                />
              </div>
            </div>

            <div className="space-y-1.5">
              <Label className="text-zinc-300 text-sm">描述</Label>
              <Input
                value={form.description}
                onChange={(e) => setForm({ ...form, description: e.target.value })}
                placeholder="规则集说明"
                className="bg-zinc-800 border-zinc-700 text-zinc-100 h-9"
              />
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label className="text-zinc-300 text-sm">规则类型</Label>
                <Select
                  value={form.rule_type}
                  onChange={(e) => setForm({ ...form, rule_type: e.target.value as RuleType })}
                  disabled={isEdit}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100 h-9 disabled:opacity-60"
                >
                  {RULE_TYPE_OPTIONS.map((t) => (
                    <option key={t.value} value={t.value}>{t.label}</option>
                  ))}
                </Select>
              </div>
              <div className="space-y-1.5">
                <Label className="text-zinc-300 text-sm">来源类型</Label>
                <Select
                  value={form.source_type}
                  onChange={(e) => setForm({ ...form, source_type: e.target.value as SourceType })}
                  disabled={isEdit}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100 h-9 disabled:opacity-60"
                >
                  {SOURCE_TYPE_OPTIONS.map((s) => (
                    <option key={s.value} value={s.value}>{s.label}</option>
                  ))}
                </Select>
              </div>
            </div>

            {form.source_type === 'remote_url' && (
              <div className="space-y-1.5">
                <Label className="text-zinc-300 text-sm">远端 URL</Label>
                <Input
                  value={form.source_url}
                  onChange={(e) => setForm({ ...form, source_url: e.target.value })}
                  placeholder="https://example.com/ruleset.json"
                  disabled={isEdit}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100 h-9 font-mono text-xs disabled:opacity-60"
                />
              </div>
            )}

            <div className="space-y-1.5">
              <Label className="text-zinc-300 text-sm">
                内容条目 <span className="text-zinc-500">(每行一条)</span>
              </Label>
              <Textarea
                value={form.content}
                onChange={(e) => setForm({ ...form, content: e.target.value })}
                placeholder={'如:\ngeosite:cn\ngeoip:cn\ndomain-suffix:cn'}
                rows={6}
                className="bg-zinc-800 border-zinc-700 text-zinc-100 font-mono text-xs"
              />
              <p className="text-xs text-zinc-500">
                {form.source_type === 'inline' && '内联规则，每行一条匹配值'}
                {form.source_type === 'geosite' && 'geosite 分类名，每行一条（如 cn、google）'}
                {form.source_type === 'geoip' && 'geoip 分类名，每行一条（如 cn、private）'}
                {form.source_type === 'remote_url' && '远端规则集，内容可选作本地缓存'}
              </p>
            </div>

            <div className="flex items-center justify-between rounded-lg border border-zinc-800 bg-zinc-950/40 px-3 py-2">
              <div>
                <Label className="text-zinc-300 text-sm cursor-pointer">自动更新</Label>
                <p className="text-xs text-zinc-500 mt-0.5">定期从远端 URL 同步规则内容</p>
              </div>
              <Switch
                checked={form.auto_update}
                onChange={(e) => setForm({ ...form, auto_update: e.target.checked })}
              />
            </div>
          </div>

          <DialogFooter className="pt-2">
            <Button
              variant="outline"
              onClick={() => setDialogOpen(false)}
              className="border-zinc-700 text-zinc-300"
            >
              取消
            </Button>
            <Button
              className="bg-indigo-600 hover:bg-indigo-500"
              onClick={handleSave}
              disabled={saving}
              isLoading={saving}
            >
              {isEdit ? '保存' : '创建'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
