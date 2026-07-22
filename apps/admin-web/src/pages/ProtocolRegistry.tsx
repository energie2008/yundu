import { useState, useEffect } from 'react'
import { Plus, Pencil, Play, FileCode2, Layers } from 'lucide-react'
import {
  Card,
  CardContent,
  Button,
  Input,
  Badge,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  Skeleton,
  EmptyState,
  Textarea,
  Select,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  useToast,
} from '@airport/ui'
import { api, ApiError } from '@/lib/api'
import { EP } from '@/lib/endpoints'

// ===== 类型定义 =====
type ProtocolStatus = 'active' | 'draft' | 'deprecated'

interface ProtocolRegistry {
  id: string
  protocol_type: string
  transport_type: string
  security_type: string
  schema_version: string
  schema_json: string
  status: ProtocolStatus
  created_at?: string
  updated_at?: string
}

interface ConfigTemplate {
  id: string
  template_code: string
  render_engine: string
  template_content: string
  description?: string
  created_at?: string
  updated_at?: string
}

interface RenderResult {
  rendered?: string
  output?: string
  error?: string
}

// ===== 工具函数 =====
function getStatusBadge(status: ProtocolStatus) {
  const variants: Record<ProtocolStatus, { label: string; variant: 'success' | 'secondary' | 'destructive' }> = {
    active: { label: '已启用', variant: 'success' },
    draft: { label: '草稿', variant: 'secondary' },
    deprecated: { label: '已废弃', variant: 'destructive' },
  }
  const v = variants[status] || variants.draft
  return <Badge variant={v.variant}>{v.label}</Badge>
}

function formatTime(dateStr?: string) {
  if (!dateStr) return '-'
  return new Date(dateStr).toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

// 规范化列表响应：兼容数组、{items:[]}、{data:[]}
function normalizeList<T>(data: unknown): T[] {
  if (Array.isArray(data)) return data as T[]
  if (data && typeof data === 'object') {
    const obj = data as Record<string, unknown>
    if (Array.isArray(obj.items)) return obj.items as T[]
    if (Array.isArray(obj.list)) return obj.list as T[]
    // 处理嵌套结构：{code:0, data:{items:[...]}}
    const dataField = obj.data
    if (dataField && typeof dataField === 'object') {
      if (Array.isArray(dataField)) return dataField as T[]
      const dataObj = dataField as Record<string, unknown>
      if (Array.isArray(dataObj.items)) return dataObj.items as T[]
      if (Array.isArray(dataObj.list)) return dataObj.list as T[]
    }
  }
  return []
}

const emptyForm = {
  protocol_type: '',
  transport_type: 'tcp',
  security_type: 'none',
  schema_version: '1',
  schema_json: '{}',
  status: 'draft' as ProtocolStatus,
}

const emptyTemplateForm = {
  id: '',
  template_code: '',
  render_engine: 'mustache',
  template_content: '',
  description: '',
}

// ===== 主组件 =====
export default function ProtocolRegistry() {
  const { toast } = useToast()
  const [tab, setTab] = useState('protocols')

  // 协议注册状态
  const [loading, setLoading] = useState(true)
  const [protocols, setProtocols] = useState<ProtocolRegistry[]>([])
  const [search, setSearch] = useState('')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [form, setForm] = useState(emptyForm)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [submitting, setSubmitting] = useState(false)

  // 配置模板状态
  const [tplLoading, setTplLoading] = useState(false)
  const [templates, setTemplates] = useState<ConfigTemplate[]>([])
  const [tplDialogOpen, setTplDialogOpen] = useState(false)
  const [tplForm, setTplForm] = useState(emptyTemplateForm)
  const [tplErrors, setTplErrors] = useState<Record<string, string>>({})
  const [tplSubmitting, setTplSubmitting] = useState(false)
  const [renderDialogOpen, setRenderDialogOpen] = useState(false)
  const [renderTemplate, setRenderTemplate] = useState<ConfigTemplate | null>(null)
  const [sampleData, setSampleData] = useState('{}')
  const [renderResult, setRenderResult] = useState('')
  const [rendering, setRendering] = useState(false)

  // 加载协议列表
  const loadProtocols = async () => {
    setLoading(true)
    try {
      const data = await api.get(EP.PROTOCOL_REGISTRY)
      setProtocols(normalizeList<ProtocolRegistry>(data))
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '加载协议列表失败'
      toast({ title: '加载失败', description: msg, variant: 'destructive' })
    } finally {
      setLoading(false)
    }
  }

  // 加载配置模板
  const loadTemplates = async () => {
    setTplLoading(true)
    try {
      const data = await api.get(EP.CONFIG_TEMPLATES)
      setTemplates(normalizeList<ConfigTemplate>(data))
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '加载配置模板失败'
      toast({ title: '加载失败', description: msg, variant: 'destructive' })
    } finally {
      setTplLoading(false)
    }
  }

  useEffect(() => {
    loadProtocols()
  }, [])

  const onTabChange = (value: string) => {
    setTab(value)
    if (value === 'templates' && templates.length === 0 && !tplLoading) {
      loadTemplates()
    }
  }

  // ===== 协议表单 =====
  const openCreate = () => {
    setEditingId(null)
    setForm(emptyForm)
    setErrors({})
    setDialogOpen(true)
  }

  const openEdit = (p: ProtocolRegistry) => {
    setEditingId(p.id)
    setForm({
      protocol_type: p.protocol_type || '',
      transport_type: p.transport_type || 'tcp',
      security_type: p.security_type || 'none',
      schema_version: p.schema_version || '1',
      schema_json: p.schema_json || '{}',
      status: p.status || 'draft',
    })
    setErrors({})
    setDialogOpen(true)
  }

  const validateForm = () => {
    const e: Record<string, string> = {}
    if (!form.protocol_type.trim()) e.protocol_type = '请输入协议类型'
    if (!form.transport_type.trim()) e.transport_type = '请选择传输类型'
    if (!form.security_type.trim()) e.security_type = '请选择安全类型'
    if (!form.schema_version.trim()) e.schema_version = '请输入 Schema 版本'
    try {
      JSON.parse(form.schema_json)
    } catch {
      e.schema_json = 'Schema 必须是合法 JSON'
    }
    setErrors(e)
    return Object.keys(e).length === 0
  }

  const submitProtocol = async () => {
    if (!validateForm()) return
    setSubmitting(true)
    try {
      if (editingId) {
        await api.patch(EP.PROTOCOL_REGISTRY_ITEM(editingId), form)
        toast({ title: '更新成功', description: `协议 ${form.protocol_type} 已更新`, variant: 'success' })
      } else {
        await api.post(EP.PROTOCOL_REGISTRY, form)
        toast({ title: '创建成功', description: `协议 ${form.protocol_type} 已注册`, variant: 'success' })
      }
      setDialogOpen(false)
      loadProtocols()
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '保存失败'
      toast({ title: '保存失败', description: msg, variant: 'destructive' })
    } finally {
      setSubmitting(false)
    }
  }

  const filteredProtocols = protocols.filter((p) =>
    p.protocol_type.toLowerCase().includes(search.toLowerCase()) ||
    p.transport_type.toLowerCase().includes(search.toLowerCase())
  )

  // ===== 模板表单 =====
  const openTplCreate = () => {
    setTplForm(emptyTemplateForm)
    setTplErrors({})
    setTplDialogOpen(true)
  }

  const openTplEdit = (t: ConfigTemplate) => {
    setTplForm({
      id: t.id,
      template_code: t.template_code,
      render_engine: t.render_engine || 'mustache',
      template_content: t.template_content || '',
      description: t.description || '',
    })
    setTplErrors({})
    setTplDialogOpen(true)
  }

  const validateTpl = () => {
    const e: Record<string, string> = {}
    if (!tplForm.template_code.trim()) e.template_code = '请输入模板编码'
    if (!tplForm.render_engine.trim()) e.render_engine = '请选择渲染引擎'
    if (!tplForm.template_content.trim()) e.template_content = '请输入模板内容'
    setTplErrors(e)
    return Object.keys(e).length === 0
  }

  const submitTemplate = async () => {
    if (!validateTpl()) return
    setTplSubmitting(true)
    try {
      const payload = {
        template_code: tplForm.template_code,
        render_engine: tplForm.render_engine,
        template_content: tplForm.template_content,
        description: tplForm.description,
      }
      // 后端使用 PUT /admin/config-templates/:code 做 upsert（创建+更新均用 PUT）
      await api.put(EP.CONFIG_TEMPLATE_UPSERT(tplForm.template_code), payload)
      if (tplForm.id) {
        toast({ title: '保存成功', description: `模板 ${tplForm.template_code} 已更新`, variant: 'success' })
      } else {
        toast({ title: '创建成功', description: `模板 ${tplForm.template_code} 已创建`, variant: 'success' })
      }
      setTplDialogOpen(false)
      loadTemplates()
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '保存失败'
      toast({ title: '保存失败', description: msg, variant: 'destructive' })
    } finally {
      setTplSubmitting(false)
    }
  }

  // ===== 渲染测试 =====
  const openRender = (t: ConfigTemplate) => {
    setRenderTemplate(t)
    setSampleData('{}')
    setRenderResult('')
    setRenderDialogOpen(true)
  }

  const runRender = async () => {
    if (!renderTemplate) return
    let parsed: unknown = {}
    try {
      parsed = JSON.parse(sampleData)
    } catch {
      toast({ title: '参数错误', description: '示例数据必须是合法 JSON', variant: 'destructive' })
      return
    }
    setRendering(true)
    setRenderResult('')
    try {
      const res = await api.post<RenderResult>(
        EP.CONFIG_TEMPLATE_RENDER(renderTemplate.template_code),
        { sample_data: parsed }
      )
      const out = (res && (res.rendered || res.output)) || JSON.stringify(res, null, 2)
      setRenderResult(typeof out === 'string' ? out : JSON.stringify(out, null, 2))
      toast({ title: '渲染完成', variant: 'success' })
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '渲染失败'
      setRenderResult(`渲染失败: ${msg}`)
      toast({ title: '渲染失败', description: msg, variant: 'destructive' })
    } finally {
      setRendering(false)
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-zinc-100">协议注册中心</h2>
      </div>

      <Tabs value={tab} onValueChange={onTabChange}>
        <TabsList>
          <TabsTrigger value="protocols">
            <Layers className="w-3.5 h-3.5 mr-1.5" />
            协议注册
          </TabsTrigger>
          <TabsTrigger value="templates">
            <FileCode2 className="w-3.5 h-3.5 mr-1.5" />
            配置模板
          </TabsTrigger>
        </TabsList>

        {/* ===== 协议注册 Tab ===== */}
        <TabsContent value="protocols">
          <div className="space-y-4">
            <div className="flex items-center justify-between gap-2">
              <Input
                placeholder="搜索协议类型或传输类型..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="bg-zinc-900 border-zinc-800 text-zinc-100 placeholder:text-zinc-500 max-w-xs"
              />
              <Button size="sm" className="bg-indigo-600 hover:bg-indigo-500" onClick={openCreate}>
                <Plus className="w-4 h-4 mr-1" />
                注册协议
              </Button>
            </div>

            <Card className="bg-zinc-900 border-zinc-800 overflow-hidden">
              <CardContent className="p-0">
                {loading ? (
                  <div className="p-4 space-y-3">
                    {[1, 2, 3].map((i) => (
                      <Skeleton key={i} className="h-14 w-full bg-zinc-800 rounded-lg" />
                    ))}
                  </div>
                ) : filteredProtocols.length === 0 ? (
                  <EmptyState
                    title="暂无协议"
                    description="注册第一个协议以开始管理"
                    className="py-12"
                  />
                ) : (
                  <div className="overflow-x-auto">
                    <Table>
                      <TableHeader>
                        <TableRow className="border-zinc-800 hover:bg-transparent">
                          <TableHead className="text-zinc-400 text-xs font-medium">协议类型</TableHead>
                          <TableHead className="text-zinc-400 text-xs font-medium hidden sm:table-cell">传输</TableHead>
                          <TableHead className="text-zinc-400 text-xs font-medium hidden md:table-cell">安全</TableHead>
                          <TableHead className="text-zinc-400 text-xs font-medium">Schema 版本</TableHead>
                          <TableHead className="text-zinc-400 text-xs font-medium">状态</TableHead>
                          <TableHead className="text-zinc-400 text-xs font-medium hidden lg:table-cell">更新时间</TableHead>
                          <TableHead className="text-zinc-400 text-xs font-medium w-10"></TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {filteredProtocols.map((p) => (
                          <TableRow key={p.id} className="border-zinc-800 hover:bg-zinc-800/50">
                            <TableCell className="py-3">
                              <div className="font-medium text-zinc-200 text-sm uppercase">{p.protocol_type}</div>
                              <div className="text-xs text-zinc-500 sm:hidden mt-0.5">
                                {p.transport_type} · {p.security_type}
                              </div>
                            </TableCell>
                            <TableCell className="py-3 hidden sm:table-cell text-sm text-zinc-300">{p.transport_type}</TableCell>
                            <TableCell className="py-3 hidden md:table-cell text-sm text-zinc-300">{p.security_type}</TableCell>
                            <TableCell className="py-3 text-sm text-zinc-400">v{p.schema_version}</TableCell>
                            <TableCell className="py-3">{getStatusBadge(p.status)}</TableCell>
                            <TableCell className="py-3 hidden lg:table-cell text-sm text-zinc-500">{formatTime(p.updated_at)}</TableCell>
                            <TableCell className="py-3">
                              <Button
                                variant="ghost"
                                size="icon"
                                className="h-8 w-8 text-zinc-400 hover:text-zinc-200"
                                onClick={() => openEdit(p)}
                              >
                                <Pencil className="w-4 h-4" />
                              </Button>
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </div>
                )}
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        {/* ===== 配置模板 Tab ===== */}
        <TabsContent value="templates">
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <p className="text-sm text-zinc-500">管理协议配置渲染模板</p>
              <Button size="sm" className="bg-indigo-600 hover:bg-indigo-500" onClick={openTplCreate}>
                <Plus className="w-4 h-4 mr-1" />
                新建模板
              </Button>
            </div>

            {tplLoading ? (
              <div className="space-y-3">
                {[1, 2, 3].map((i) => (
                  <Skeleton key={i} className="h-28 w-full bg-zinc-800 rounded-lg" />
                ))}
              </div>
            ) : templates.length === 0 ? (
              <Card className="bg-zinc-900 border-zinc-800">
                <CardContent>
                  <EmptyState
                    title="暂无配置模板"
                    description="创建模板来渲染节点配置"
                    className="py-12"
                  />
                </CardContent>
              </Card>
            ) : (
              <div className="space-y-3">
                {templates.map((t) => (
                  <Card key={t.id} className="bg-zinc-900 border-zinc-800">
                    <CardContent className="p-4">
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0 flex-1">
                          <div className="flex items-center gap-2 flex-wrap">
                            <FileCode2 className="w-4 h-4 text-indigo-400 flex-shrink-0" />
                            <span className="font-mono text-sm text-zinc-100 truncate">{t.template_code}</span>
                            <Badge variant="secondary" className="bg-zinc-800 text-zinc-300 text-xs">
                              {t.render_engine}
                            </Badge>
                          </div>
                          {t.description && (
                            <p className="text-xs text-zinc-500 mt-1.5">{t.description}</p>
                          )}
                          <pre className="mt-2 text-xs text-zinc-400 bg-zinc-950/60 border border-zinc-800 rounded-md p-2 max-h-24 overflow-auto whitespace-pre-wrap break-all">
                            {t.template_content}
                          </pre>
                          <p className="text-xs text-zinc-600 mt-2">更新于 {formatTime(t.updated_at)}</p>
                        </div>
                        <div className="flex items-center gap-1 flex-shrink-0">
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-8 w-8 text-zinc-400 hover:text-emerald-400"
                            onClick={() => openRender(t)}
                            title="渲染测试"
                          >
                            <Play className="w-4 h-4" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-8 w-8 text-zinc-400 hover:text-zinc-200"
                            onClick={() => openTplEdit(t)}
                          >
                            <Pencil className="w-4 h-4" />
                          </Button>
                        </div>
                      </div>
                    </CardContent>
                  </Card>
                ))}
              </div>
            )}
          </div>
        </TabsContent>
      </Tabs>

      {/* ===== 协议注册 Dialog ===== */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-lg">
          <DialogHeader>
            <DialogTitle>{editingId ? '编辑协议' : '注册协议'}</DialogTitle>
            <DialogDescription>定义协议的传输、安全与 Schema 配置</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400">协议类型 *</label>
              <Input
                placeholder="如 vmess / vless / trojan"
                value={form.protocol_type}
                onChange={(e) => setForm({ ...form, protocol_type: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              />
              {errors.protocol_type && <p className="text-xs text-red-400">{errors.protocol_type}</p>}
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <label className="text-xs text-zinc-400">传输类型 *</label>
                <Select
                  value={form.transport_type}
                  onChange={(e) => setForm({ ...form, transport_type: e.target.value })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                >
                  <option value="tcp">TCP</option>
                  <option value="ws">WebSocket</option>
                  <option value="grpc">gRPC</option>
                  <option value="http2">HTTP/2</option>
                  <option value="kcp">mKCP</option>
                  <option value="quic">QUIC</option>
                </Select>
                {errors.transport_type && <p className="text-xs text-red-400">{errors.transport_type}</p>}
              </div>
              <div className="space-y-1.5">
                <label className="text-xs text-zinc-400">安全类型 *</label>
                <Select
                  value={form.security_type}
                  onChange={(e) => setForm({ ...form, security_type: e.target.value })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                >
                  <option value="none">None</option>
                  <option value="tls">TLS</option>
                  <option value="reality">Reality</option>
                  <option value="xtls">XTLS</option>
                  <option value="shadowtls">ShadowTLS</option>
                </Select>
                {errors.security_type && <p className="text-xs text-red-400">{errors.security_type}</p>}
              </div>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <label className="text-xs text-zinc-400">Schema 版本 *</label>
                <Input
                  placeholder="如 1"
                  value={form.schema_version}
                  onChange={(e) => setForm({ ...form, schema_version: e.target.value })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
                {errors.schema_version && <p className="text-xs text-red-400">{errors.schema_version}</p>}
              </div>
              <div className="space-y-1.5">
                <label className="text-xs text-zinc-400">状态</label>
                <Select
                  value={form.status}
                  onChange={(e) => setForm({ ...form, status: e.target.value as ProtocolStatus })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                >
                  <option value="draft">草稿</option>
                  <option value="active">已启用</option>
                  <option value="deprecated">已废弃</option>
                </Select>
              </div>
            </div>
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400">Schema JSON *</label>
              <Textarea
                rows={6}
                placeholder='{"fields":[...]}'
                value={form.schema_json}
                onChange={(e) => setForm({ ...form, schema_json: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100 font-mono text-xs"
              />
              {errors.schema_json && <p className="text-xs text-red-400">{errors.schema_json}</p>}
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={() => setDialogOpen(false)}>
              取消
            </Button>
            <Button className="bg-indigo-600 hover:bg-indigo-500" disabled={submitting} onClick={submitProtocol}>
              {submitting ? '保存中...' : '保存'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* ===== 配置模板 Dialog ===== */}
      <Dialog open={tplDialogOpen} onOpenChange={setTplDialogOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-2xl">
          <DialogHeader>
            <DialogTitle>{tplForm.id ? '编辑模板' : '新建模板'}</DialogTitle>
            <DialogDescription>配置渲染引擎与模板内容</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <label className="text-xs text-zinc-400">模板编码 *</label>
                <Input
                  placeholder="如 vmess_ws_tls"
                  value={tplForm.template_code}
                  onChange={(e) => setTplForm({ ...tplForm, template_code: e.target.value })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100 font-mono"
                  disabled={!!tplForm.id}
                />
                {tplErrors.template_code && <p className="text-xs text-red-400">{tplErrors.template_code}</p>}
              </div>
              <div className="space-y-1.5">
                <label className="text-xs text-zinc-400">渲染引擎 *</label>
                <Select
                  value={tplForm.render_engine}
                  onChange={(e) => setTplForm({ ...tplForm, render_engine: e.target.value })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                >
                  <option value="mustache">Mustache</option>
                  <option value="handlebars">Handlebars</option>
                  <option value="eta">Eta</option>
                  <option value="json_template">JSON Template</option>
                </Select>
                {tplErrors.render_engine && <p className="text-xs text-red-400">{tplErrors.render_engine}</p>}
              </div>
            </div>
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400">描述</label>
              <Input
                placeholder="模板用途说明"
                value={tplForm.description}
                onChange={(e) => setTplForm({ ...tplForm, description: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              />
            </div>
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400">模板内容 *</label>
              <Textarea
                rows={10}
                placeholder='{{#fields}}...'
                value={tplForm.template_content}
                onChange={(e) => setTplForm({ ...tplForm, template_content: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100 font-mono text-xs"
              />
              {tplErrors.template_content && <p className="text-xs text-red-400">{tplErrors.template_content}</p>}
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={() => setTplDialogOpen(false)}>
              取消
            </Button>
            <Button className="bg-indigo-600 hover:bg-indigo-500" disabled={tplSubmitting} onClick={submitTemplate}>
              {tplSubmitting ? '保存中...' : '保存'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* ===== 渲染测试 Dialog ===== */}
      <Dialog open={renderDialogOpen} onOpenChange={setRenderDialogOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-2xl">
          <DialogHeader>
            <DialogTitle>渲染测试 · {renderTemplate?.template_code}</DialogTitle>
            <DialogDescription>使用示例数据渲染模板，预览输出结果</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400">示例数据 (JSON)</label>
              <Textarea
                rows={5}
                value={sampleData}
                onChange={(e) => setSampleData(e.target.value)}
                className="bg-zinc-800 border-zinc-700 text-zinc-100 font-mono text-xs"
              />
            </div>
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400">渲染结果</label>
              <pre className="text-xs text-zinc-300 bg-zinc-950/60 border border-zinc-800 rounded-md p-3 min-h-[120px] max-h-64 overflow-auto whitespace-pre-wrap break-all">
                {rendering ? '渲染中...' : renderResult || '点击下方按钮执行渲染'}
              </pre>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={() => setRenderDialogOpen(false)}>
              关闭
            </Button>
            <Button className="bg-indigo-600 hover:bg-indigo-500" disabled={rendering} onClick={runRender}>
              {rendering ? '渲染中...' : '执行渲染'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
