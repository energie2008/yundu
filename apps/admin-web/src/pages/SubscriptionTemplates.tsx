import { useState, useEffect, useCallback } from 'react'
import {
  FileCode2,
  Pencil,
  Search,
  RefreshCw,
  RotateCw,
  Eye,
  Star,
  Save,
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
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  useToast,
} from '@airport/ui'
import { api, ApiError } from '@/lib/api'
import { EP } from '@/lib/endpoints'
import { YamlEditor } from '@/components/common/YamlEditor'

// ===== 类型定义 =====
// 对齐后端 model.SubscribeTemplate（按名称索引，name 即客户端内核名）
interface SubscribeTemplate {
  id: string
  // 客户端内核名：clash / clashmeta / singbox / surge / surfboard ...
  name: string
  // 模板内容（YAML / JSON / conf）
  content: string
  is_builtin: boolean
  enabled: boolean
  created_at?: string
  updated_at?: string
}

// 客户端内核名 → 展示标签
const CLIENT_LABELS: Record<string, string> = {
  clash: 'Clash',
  clashmeta: 'Clash Meta',
  singbox: 'Sing-box',
  surge: 'Surge',
  surfboard: 'Surfboard',
  shadowrocket: 'Shadowrocket',
  v2rayn: 'V2RayN',
  quantumultx: 'Quantumult X',
  loon: 'Loon',
}

function clientLabel(name: string): string {
  return CLIENT_LABELS[name] || name
}

// 根据 name 推断编辑器模式（singbox 多为 JSON，其余多为 YAML）
function inferMode(name: string): 'yaml' | 'json' {
  return name === 'singbox' ? 'json' : 'yaml'
}

function formatDate(iso?: string | null): string {
  if (!iso) return '-'
  try {
    return new Date(iso).toLocaleString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    })
  } catch {
    return '-'
  }
}

function normalizeList<T>(data: unknown): T[] {
  if (Array.isArray(data)) return data as T[]
  if (data && typeof data === 'object') {
    const obj = data as Record<string, unknown>
    if (Array.isArray(obj.items)) return obj.items as T[]
    if (Array.isArray(obj.list)) return obj.list as T[]
  }
  return []
}

export default function SubscriptionTemplates() {
  const { toast } = useToast()
  const [templates, setTemplates] = useState<SubscribeTemplate[]>([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState('')

  // 编辑对话框
  const [editOpen, setEditOpen] = useState(false)
  const [editing, setEditing] = useState<SubscribeTemplate | null>(null)
  const [formName, setFormName] = useState('')
  const [formContent, setFormContent] = useState('')
  const [formEnabled, setFormEnabled] = useState(true)
  const [editorMode, setEditorMode] = useState<'yaml' | 'json'>('yaml')
  const [saving, setSaving] = useState(false)

  // 预览对话框
  const [previewOpen, setPreviewOpen] = useState(false)
  const [previewing, setPreviewing] = useState<SubscribeTemplate | null>(null)

  const [reloading, setReloading] = useState(false)
  const [togglingId, setTogglingId] = useState<string | null>(null)

  // ===== 拉取列表 =====
  const fetchTemplates = useCallback(async () => {
    try {
      setLoading(true)
      const data = await api.get<unknown>(EP.SUBSCRIBE_TEMPLATES)
      setTemplates(normalizeList<SubscribeTemplate>(data))
    } catch (err) {
      toast({
        title: '加载失败',
        description: err instanceof ApiError ? err.message : '无法获取订阅模板列表',
        variant: 'destructive',
      })
      setTemplates([])
    } finally {
      setLoading(false)
    }
  }, [toast])

  useEffect(() => {
    fetchTemplates()
  }, [fetchTemplates])

  const filtered = templates.filter((t) => {
    const kw = search.trim().toLowerCase()
    if (!kw) return true
    return (
      t.name.toLowerCase().includes(kw) ||
      clientLabel(t.name).toLowerCase().includes(kw)
    )
  })

  // ===== 编辑 =====
  const openEdit = (tpl: SubscribeTemplate) => {
    setEditing(tpl)
    setFormName(tpl.name)
    setFormContent(tpl.content || '')
    setFormEnabled(tpl.enabled)
    setEditorMode(inferMode(tpl.name))
    setEditOpen(true)
  }

  const closeEdit = () => {
    setEditOpen(false)
    setEditing(null)
    setFormName('')
    setFormContent('')
    setFormEnabled(true)
  }

  const handleSubmit = async () => {
    if (!editing) return
    if (!formContent.trim()) {
      toast({ title: '请填写模板内容', variant: 'destructive' })
      return
    }
    try {
      setSaving(true)
      await api.put(EP.SUBSCRIBE_TEMPLATE_DETAIL(editing.id), {
        content: formContent,
        enabled: formEnabled,
      })
      toast({ title: '模板已更新', variant: 'success' })
      closeEdit()
      fetchTemplates()
    } catch (err) {
      toast({
        title: '保存失败',
        description: err instanceof ApiError ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setSaving(false)
    }
  }

  // ===== 启用/禁用切换（PUT content + enabled，content 必填）=====
  const toggleEnabled = async (tpl: SubscribeTemplate) => {
    try {
      setTogglingId(tpl.id)
      await api.put(EP.SUBSCRIBE_TEMPLATE_DETAIL(tpl.id), {
        content: tpl.content,
        enabled: !tpl.enabled,
      })
      toast({
        title: tpl.enabled ? '已禁用模板' : '已启用模板',
        variant: 'success',
      })
      fetchTemplates()
    } catch (err) {
      toast({
        title: '更新失败',
        description: err instanceof ApiError ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setTogglingId(null)
    }
  }

  // ===== 重载缓存 =====
  const handleReload = async () => {
    try {
      setReloading(true)
      await api.post(EP.SUBSCRIBE_TEMPLATES_RELOAD)
      toast({
        title: '模板缓存已重载',
        description: '已从数据库重新加载所有订阅模板',
        variant: 'success',
      })
      fetchTemplates()
    } catch (err) {
      toast({
        title: '重载失败',
        description: err instanceof ApiError ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setReloading(false)
    }
  }

  // ===== 预览 =====
  const openPreview = (tpl: SubscribeTemplate) => {
    setPreviewing(tpl)
    setPreviewOpen(true)
  }

  const contentPreview = (content: string): string => {
    if (!content) return '（空）'
    const firstLines = content.split('\n').slice(0, 2).join(' ')
    return firstLines.length > 60 ? firstLines.slice(0, 60) + '...' : firstLines
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-zinc-100 flex items-center gap-2">
            <FileCode2 className="w-5 h-5 text-violet-400" />
            订阅模板管理
          </h1>
          <p className="text-sm text-zinc-500 mt-1">
            管理按客户端内核索引的订阅配置模板（Clash / Sing-box / Surge 等），渲染器按内核名取模板内容注入节点
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            className="border-zinc-700 text-zinc-300 hover:bg-zinc-800"
            onClick={fetchTemplates}
          >
            <RefreshCw className="w-4 h-4 mr-1" />刷新
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="border-amber-800/50 text-amber-300 hover:bg-amber-950/30"
            onClick={handleReload}
            disabled={reloading}
          >
            <RotateCw className={`w-4 h-4 mr-1 ${reloading ? 'animate-spin' : ''}`} />
            {reloading ? '重载中...' : '重载模板'}
          </Button>
        </div>
      </div>

      <Card className="bg-zinc-900 border-zinc-800">
        <CardContent className="p-3">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-zinc-500" />
            <Input
              placeholder="搜索客户端类型..."
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
          ) : filtered.length === 0 ? (
            <EmptyState
              title="暂无订阅模板"
              description={search ? '没有匹配的模板，试试调整搜索条件' : '后端尚未预置任何订阅模板'}
              className="py-12"
            />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-zinc-800">
                    <th className="text-left p-3 text-xs font-medium text-zinc-400">客户端类型</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400 hidden md:table-cell">内容预览</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400">类型</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400 hidden lg:table-cell">最后更新</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400">启用</th>
                    <th className="text-right p-3 text-xs font-medium text-zinc-400">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {filtered.map((tpl) => (
                    <tr
                      key={tpl.id}
                      className="border-b border-zinc-800 last:border-0 hover:bg-zinc-800/50 transition-colors"
                    >
                      <td className="p-3">
                        <div className="flex items-center gap-2">
                          <Badge variant="outline" className="bg-violet-950/40 text-violet-300 border-violet-800/50 text-xs">
                            {clientLabel(tpl.name)}
                          </Badge>
                          <code className="text-xs font-mono text-zinc-500">{tpl.name}</code>
                        </div>
                      </td>
                      <td className="p-3 text-xs text-zinc-400 hidden md:table-cell max-w-sm truncate font-mono" title={tpl.content}>
                        {contentPreview(tpl.content)}
                      </td>
                      <td className="p-3">
                        {tpl.is_builtin ? (
                          <Badge variant="outline" className="bg-sky-900/40 text-sky-300 border-sky-800/50 text-xs">
                            内置
                          </Badge>
                        ) : (
                          <Badge variant="outline" className="bg-zinc-800 text-zinc-400 border-zinc-700 text-xs">
                            自定义
                          </Badge>
                        )}
                      </td>
                      <td className="p-3 text-sm text-zinc-400 hidden lg:table-cell">
                        {formatDate(tpl.updated_at)}
                      </td>
                      <td className="p-3">
                        <Switch
                          checked={tpl.enabled}
                          onChange={() => toggleEnabled(tpl)}
                          disabled={togglingId === tpl.id}
                        />
                      </td>
                      <td className="p-3">
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-8 w-8 p-0"
                            onClick={() => openPreview(tpl)}
                            title="预览内容"
                          >
                            <Eye className="w-4 h-4 text-zinc-400" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-8 w-8 p-0"
                            onClick={() => openEdit(tpl)}
                            title="编辑"
                          >
                            <Pencil className="w-4 h-4 text-zinc-400" />
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

      {/* 编辑对话框 */}
      <Dialog open={editOpen} onOpenChange={setEditOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-4xl max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>
              编辑订阅模板
              {editing && (
                <span className="ml-2 text-sm font-normal text-zinc-500">
                  ({clientLabel(editing.name)})
                </span>
              )}
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label className="text-zinc-300 text-sm">客户端类型</Label>
                <Input
                  value={clientLabel(formName)}
                  disabled
                  className="bg-zinc-800 border-zinc-700 text-zinc-500"
                />
                <p className="text-xs text-zinc-500">内核名不可修改</p>
              </div>
              <div className="space-y-2">
                <Label className="text-zinc-300 text-sm">启用状态</Label>
                <div className="flex items-center gap-2 h-9">
                  <Switch
                    checked={formEnabled}
                    onChange={(e) => setFormEnabled(e.target.checked)}
                  />
                  <span className="text-sm text-zinc-400">
                    {formEnabled ? '启用' : '禁用'}
                  </span>
                </div>
              </div>
            </div>
            <div className="space-y-2">
              <Label className="text-zinc-300 text-sm">模板内容</Label>
              <YamlEditor
                value={formContent}
                onChange={setFormContent}
                mode={editorMode}
                onModeChange={setEditorMode}
                height={420}
                placeholder="# 输入订阅模板内容（YAML/JSON），渲染器将注入节点列表到 proxies/outbounds 占位..."
                showModeToggle={true}
              />
              <p className="text-xs text-zinc-500">
                支持变量占位符，保存后立即生效并刷新渲染缓存
              </p>
            </div>
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              className="border-zinc-700 text-zinc-300 hover:bg-zinc-800"
              onClick={closeEdit}
            >
              取消
            </Button>
            <Button
              className="bg-violet-600 hover:bg-violet-500"
              onClick={handleSubmit}
              disabled={saving}
            >
              {saving ? (
                <Save className="w-4 h-4 mr-1.5 animate-pulse" />
              ) : (
                <Save className="w-4 h-4 mr-1.5" />
              )}
              {saving ? '保存中...' : '保存'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 预览对话框 */}
      <Dialog open={previewOpen} onOpenChange={setPreviewOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-3xl max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>
              模板内容预览
              {previewing && (
                <span className="ml-2 text-sm font-normal text-zinc-500">
                  ({clientLabel(previewing.name)})
                </span>
              )}
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-3 py-2">
            {previewing && (
              <>
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant="outline" className="bg-violet-950/40 text-violet-300 border-violet-800/50 text-xs">
                    {clientLabel(previewing.name)}
                  </Badge>
                  {previewing.is_builtin && (
                    <Badge variant="outline" className="bg-sky-900/40 text-sky-300 border-sky-800/50 text-xs">
                      内置
                    </Badge>
                  )}
                  {previewing.enabled ? (
                    <Badge variant="outline" className="bg-emerald-900/40 text-emerald-300 border-emerald-800/50 text-xs">
                      启用
                    </Badge>
                  ) : (
                    <Badge variant="outline" className="bg-zinc-800 text-zinc-500 border-zinc-700 text-xs">
                      禁用
                    </Badge>
                  )}
                  <span className="text-xs text-zinc-500 ml-auto">
                    最后更新: {formatDate(previewing.updated_at)}
                  </span>
                </div>
                <div>
                  <div className="text-xs text-zinc-500 mb-1">原始模板内容</div>
                  <pre className="bg-zinc-950 border border-zinc-800 rounded-md p-3 text-xs font-mono text-zinc-300 overflow-auto max-h-[55vh] whitespace-pre-wrap break-all">
                    {previewing.content || '（空）'}
                  </pre>
                </div>
                <p className="text-xs text-amber-500/80 flex items-center gap-1">
                  <Star className="w-3 h-3" />
                  此处展示模板原始内容；完整渲染后的订阅配置预览请前往「订阅预览」页面查看
                </p>
              </>
            )}
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              className="border-zinc-700 text-zinc-300 hover:bg-zinc-800"
              onClick={() => setPreviewOpen(false)}
            >
              关闭
            </Button>
            {previewing && (
              <Button
                className="bg-violet-600 hover:bg-violet-500"
                onClick={() => {
                  setPreviewOpen(false)
                  openEdit(previewing)
                }}
              >
                <Pencil className="w-4 h-4 mr-1.5" />
                编辑此模板
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
