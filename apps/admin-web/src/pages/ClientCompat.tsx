import { useState, useEffect } from 'react'
import {
  Plus,
  Pencil,
  Trash2,
  Smartphone,
  LayoutGrid,
  Code2,
  RefreshCw,
  CheckCircle2,
  XCircle,
  AlertCircle,
  Minus,
} from 'lucide-react'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
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
type Platform = 'ios' | 'android' | 'windows' | 'macos' | 'linux'
type ClientStatus = 'active' | 'deprecated' | 'beta'
type CompatLevel = 'supported' | 'partial' | 'unsupported'
type PatchType = 'config_override' | 'route_patch' | 'dns_patch' | 'subscription_patch'

interface ClientProfile {
  id: string
  name: string
  platform: Platform
  version?: string
  features?: string[] | string
  status: ClientStatus
  created_at?: string
  updated_at?: string
}

interface CompatMatrix {
  features: string[]
  rows: Array<{
    client_id: string
    client_name: string
    platform: Platform
    cells: Record<string, CompatLevel>
  }>
}

interface AdvancedPatchProfile {
  id: string
  client_id: string
  client_name?: string
  patch_type: PatchType
  patch_content: string
  status: 'active' | 'inactive'
  note?: string
  created_at?: string
  updated_at?: string
}

// ===== 工具函数 =====
function formatTime(dateStr?: string) {
  if (!dateStr) return '-'
  return new Date(dateStr).toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

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

function toArray(val: string[] | string | undefined): string[] {
  if (!val) return []
  if (Array.isArray(val)) return val
  try {
    const parsed = JSON.parse(val)
    if (Array.isArray(parsed)) return parsed
  } catch {
    // not json
  }
  return String(val).split(',').map((s) => s.trim()).filter(Boolean)
}

const platformLabel: Record<Platform, string> = {
  ios: 'iOS',
  android: 'Android',
  windows: 'Windows',
  macos: 'macOS',
  linux: 'Linux',
}

const clientStatusLabel: Record<ClientStatus, { label: string; variant: 'success' | 'secondary' | 'warning' }> = {
  active: { label: '活跃', variant: 'success' },
  deprecated: { label: '已弃用', variant: 'secondary' },
  beta: { label: 'Beta', variant: 'warning' },
}

function getClientStatusBadge(status: ClientStatus) {
  const cfg = clientStatusLabel[status] || clientStatusLabel.active
  return <Badge variant={cfg.variant}>{cfg.label}</Badge>
}

const patchTypeLabel: Record<PatchType, string> = {
  config_override: '配置覆盖',
  route_patch: '路由补丁',
  dns_patch: 'DNS 补丁',
  subscription_patch: '订阅补丁',
}

const compatLevelConfig: Record<
  CompatLevel,
  { label: string; variant: 'success' | 'secondary' | 'destructive' | 'warning' }
> = {
  supported: { label: '支持', variant: 'success' },
  partial: { label: '部分支持', variant: 'warning' },
  unsupported: { label: '不支持', variant: 'destructive' },
}

function getCompatBadge(level: CompatLevel) {
  const cfg = compatLevelConfig[level] || { label: '-', variant: 'secondary' as const }
  return <Badge variant={cfg.variant} className="text-xs">{cfg.label}</Badge>
}

const emptyClientForm = {
  id: '',
  name: '',
  platform: 'ios' as Platform,
  version: '',
  features: '',
  status: 'active' as ClientStatus,
}

const emptyPatchForm = {
  id: '',
  client_id: '',
  patch_type: 'config_override' as PatchType,
  patch_content: '',
  status: 'active' as 'active' | 'inactive',
  note: '',
}

const emptyCompatForm = {
  client_id: '',
  feature: '',
  level: 'supported' as CompatLevel,
  note: '',
}

// ===== 主组件 =====
export default function ClientCompat() {
  const { toast } = useToast()
  const [tab, setTab] = useState('clients')

  // 客户端列表
  const [clients, setClients] = useState<ClientProfile[]>([])
  const [clientsLoading, setClientsLoading] = useState(true)
  const [clientDialogOpen, setClientDialogOpen] = useState(false)
  const [clientForm, setClientForm] = useState(emptyClientForm)
  const [clientErrors, setClientErrors] = useState<Record<string, string>>({})
  const [clientSubmitting, setClientSubmitting] = useState(false)

  // 兼容矩阵
  const [matrix, setMatrix] = useState<CompatMatrix>({ features: [], rows: [] })
  const [matrixLoading, setMatrixLoading] = useState(false)
  const [compatDialogOpen, setCompatDialogOpen] = useState(false)
  const [compatForm, setCompatForm] = useState(emptyCompatForm)
  const [compatErrors, setCompatErrors] = useState<Record<string, string>>({})
  const [compatSubmitting, setCompatSubmitting] = useState(false)

  // 高级补丁
  const [patches, setPatches] = useState<AdvancedPatchProfile[]>([])
  const [patchesLoading, setPatchesLoading] = useState(false)
  const [patchDialogOpen, setPatchDialogOpen] = useState(false)
  const [patchForm, setPatchForm] = useState(emptyPatchForm)
  const [patchErrors, setPatchErrors] = useState<Record<string, string>>({})
  const [patchSubmitting, setPatchSubmitting] = useState(false)

  const loadClients = async () => {
    setClientsLoading(true)
    try {
      const data = await api.get(EP.CLIENT_PROFILES)
      setClients(normalizeList<ClientProfile>(data))
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '加载客户端失败'
      toast({ title: '加载失败', description: msg, variant: 'destructive' })
    } finally {
      setClientsLoading(false)
    }
  }

  const loadMatrix = async () => {
    setMatrixLoading(true)
    try {
      const data = await api.get(EP.CLIENT_COMPAT_MATRIX)
      if (data && typeof data === 'object') {
        const obj = data as Partial<CompatMatrix>
        setMatrix({ features: obj.features || [], rows: obj.rows || [] })
      } else {
        setMatrix({ features: [], rows: [] })
      }
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '加载兼容矩阵失败'
      toast({ title: '加载失败', description: msg, variant: 'destructive' })
    } finally {
      setMatrixLoading(false)
    }
  }

  // 后端无 /admin/advanced-patch-profiles 接口；补丁功能暂不可用
  const loadPatches = async () => {
    setPatches([])
  }

  useEffect(() => {
    loadClients()
  }, [])

  const onTabChange = (value: string) => {
    setTab(value)
    if (value === 'matrix' && matrix.features.length === 0 && !matrixLoading) loadMatrix()
    if (value === 'patches' && patches.length === 0 && !patchesLoading) loadPatches()
  }

  // ===== 客户端操作 =====
  const openClientCreate = () => {
    setClientForm(emptyClientForm)
    setClientErrors({})
    setClientDialogOpen(true)
  }

  const openClientEdit = (c: ClientProfile) => {
    setClientForm({
      id: c.id,
      name: c.name,
      platform: c.platform,
      version: c.version || '',
      features: toArray(c.features).join(', '),
      status: c.status,
    })
    setClientErrors({})
    setClientDialogOpen(true)
  }

  const validateClient = () => {
    const e: Record<string, string> = {}
    if (!clientForm.name.trim()) e.name = '请输入客户端名称'
    if (!clientForm.platform) e.platform = '请选择平台'
    setClientErrors(e)
    return Object.keys(e).length === 0
  }

  const submitClient = async () => {
    if (!validateClient()) return
    // 后端仅提供 GET /admin/client-profiles，暂不支持创建/更新
    toast({ title: '暂不可用', description: '后端尚未实现客户端创建/更新接口', variant: 'destructive' })
    setClientDialogOpen(false)
  }

  const deleteClient = async (_c: ClientProfile) => {
    // 后端仅提供 GET /admin/client-profiles，暂不支持删除
    toast({ title: '暂不可用', description: '后端尚未实现客户端删除接口', variant: 'destructive' })
  }

  // ===== 兼容条目操作 =====
  const openCompatCreate = () => {
    setCompatForm({ ...emptyCompatForm, client_id: clients[0]?.id || '' })
    setCompatErrors({})
    setCompatDialogOpen(true)
  }

  const validateCompat = () => {
    const e: Record<string, string> = {}
    if (!compatForm.client_id) e.client_id = '请选择客户端'
    if (!compatForm.feature.trim()) e.feature = '请输入功能/协议名称'
    setCompatErrors(e)
    return Object.keys(e).length === 0
  }

  const submitCompat = async () => {
    if (!validateCompat()) return
    setCompatSubmitting(true)
    try {
      const payload = {
        client_id: compatForm.client_id,
        feature: compatForm.feature,
        level: compatForm.level,
        note: compatForm.note || undefined,
      }
      // 后端使用 PATCH 更新兼容矩阵（非 POST）
      await api.patch(EP.CLIENT_COMPAT_MATRIX, payload)
      toast({ title: '已添加', description: '兼容条目已添加', variant: 'success' })
      setCompatDialogOpen(false)
      loadMatrix()
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '保存失败'
      toast({ title: '保存失败', description: msg, variant: 'destructive' })
    } finally {
      setCompatSubmitting(false)
    }
  }

  // ===== 补丁操作 =====
  const openPatchCreate = () => {
    setPatchForm({ ...emptyPatchForm, client_id: clients[0]?.id || '' })
    setPatchErrors({})
    setPatchDialogOpen(true)
  }

  const openPatchEdit = (p: AdvancedPatchProfile) => {
    setPatchForm({
      id: p.id,
      client_id: p.client_id,
      patch_type: p.patch_type,
      patch_content: p.patch_content,
      status: p.status,
      note: p.note || '',
    })
    setPatchErrors({})
    setPatchDialogOpen(true)
  }

  const validatePatch = () => {
    const e: Record<string, string> = {}
    if (!patchForm.client_id) e.client_id = '请选择客户端'
    if (!patchForm.patch_type) e.patch_type = '请选择补丁类型'
    if (!patchForm.patch_content.trim()) e.patch_content = '请输入补丁内容'
    setPatchErrors(e)
    return Object.keys(e).length === 0
  }

  const submitPatch = async () => {
    if (!validatePatch()) return
    // 后端暂无 /admin/advanced-patch-profiles 接口
    toast({ title: '暂不可用', description: '后端尚未实现补丁接口', variant: 'destructive' })
    setPatchDialogOpen(false)
  }

  const deletePatch = async (_p: AdvancedPatchProfile) => {
    // 后端暂无 /admin/advanced-patch-profiles 接口
    toast({ title: '暂不可用', description: '后端尚未实现补丁接口', variant: 'destructive' })
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-zinc-100">客户端兼容矩阵</h2>
      </div>

      <Tabs value={tab} onValueChange={onTabChange}>
        <TabsList>
          <TabsTrigger value="clients">
            <Smartphone className="w-3.5 h-3.5 mr-1.5" />
            客户端列表
          </TabsTrigger>
          <TabsTrigger value="matrix">
            <LayoutGrid className="w-3.5 h-3.5 mr-1.5" />
            兼容矩阵
          </TabsTrigger>
          <TabsTrigger value="patches">
            <Code2 className="w-3.5 h-3.5 mr-1.5" />
            高级补丁
          </TabsTrigger>
        </TabsList>

        {/* ===== 客户端列表 Tab ===== */}
        <TabsContent value="clients">
          <div className="space-y-4">
            <div className="flex items-center justify-between gap-2">
              <Button size="sm" variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={loadClients}>
                <RefreshCw className="w-3.5 h-3.5 mr-1.5" />
                刷新
              </Button>
              <Button size="sm" className="bg-indigo-600 hover:bg-indigo-500" onClick={openClientCreate}>
                <Plus className="w-4 h-4 mr-1" />
                添加客户端
              </Button>
            </div>

            <Card className="bg-zinc-900 border-zinc-800 overflow-hidden">
              <CardContent className="p-0">
                {clientsLoading ? (
                  <div className="p-4 space-y-3">
                    {[1, 2, 3].map((i) => (
                      <Skeleton key={i} className="h-14 w-full bg-zinc-800 rounded-lg" />
                    ))}
                  </div>
                ) : clients.length === 0 ? (
                  <EmptyState title="暂无客户端" description="添加第一个客户端配置" className="py-12" />
                ) : (
                  <div className="overflow-x-auto">
                    <Table>
                      <TableHeader>
                        <TableRow className="border-zinc-800 hover:bg-transparent">
                          <TableHead className="text-zinc-400 text-xs font-medium">名称</TableHead>
                          <TableHead className="text-zinc-400 text-xs font-medium hidden sm:table-cell">平台</TableHead>
                          <TableHead className="text-zinc-400 text-xs font-medium hidden md:table-cell">版本</TableHead>
                          <TableHead className="text-zinc-400 text-xs font-medium hidden lg:table-cell">特性</TableHead>
                          <TableHead className="text-zinc-400 text-xs font-medium">状态</TableHead>
                          <TableHead className="text-zinc-400 text-xs font-medium w-24"></TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {clients.map((c) => {
                          const feats = toArray(c.features)
                          return (
                            <TableRow key={c.id} className="border-zinc-800 hover:bg-zinc-800/50">
                              <TableCell className="py-3">
                                <div className="font-medium text-zinc-200 text-sm">{c.name}</div>
                                <div className="text-xs text-zinc-500 sm:hidden mt-0.5">{platformLabel[c.platform]}</div>
                              </TableCell>
                              <TableCell className="py-3 hidden sm:table-cell">
                                <Badge variant="secondary" className="bg-zinc-800 text-zinc-300 text-xs">
                                  {platformLabel[c.platform] || c.platform}
                                </Badge>
                              </TableCell>
                              <TableCell className="py-3 hidden md:table-cell text-sm text-zinc-400">{c.version || '-'}</TableCell>
                              <TableCell className="py-3 hidden lg:table-cell text-sm text-zinc-400">
                                <div className="flex flex-wrap gap-1 max-w-[200px]">
                                  {feats.slice(0, 3).map((f) => (
                                    <span key={f} className="px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-400 text-[10px]">{f}</span>
                                  ))}
                                  {feats.length > 3 && <span className="text-[10px] text-zinc-600">+{feats.length - 3}</span>}
                                </div>
                              </TableCell>
                              <TableCell className="py-3">{getClientStatusBadge(c.status)}</TableCell>
                              <TableCell className="py-3">
                                <div className="flex items-center gap-1">
                                  <Button variant="ghost" size="icon" className="h-8 w-8 text-zinc-400 hover:text-zinc-200" onClick={() => openClientEdit(c)} title="编辑">
                                    <Pencil className="w-4 h-4" />
                                  </Button>
                                  <Button variant="ghost" size="icon" className="h-8 w-8 text-zinc-400 hover:text-red-400" onClick={() => deleteClient(c)} title="删除">
                                    <Trash2 className="w-4 h-4" />
                                  </Button>
                                </div>
                              </TableCell>
                            </TableRow>
                          )
                        })}
                      </TableBody>
                    </Table>
                  </div>
                )}
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        {/* ===== 兼容矩阵 Tab ===== */}
        <TabsContent value="matrix">
          <div className="space-y-4">
            <div className="flex items-center justify-between gap-2">
              <div className="flex items-center gap-3 text-xs text-zinc-500">
                <span className="flex items-center gap-1"><CheckCircle2 className="w-3 h-3 text-emerald-400" />支持</span>
                <span className="flex items-center gap-1"><AlertCircle className="w-3 h-3 text-amber-400" />部分支持</span>
                <span className="flex items-center gap-1"><XCircle className="w-3 h-3 text-red-400" />不支持</span>
              </div>
              <div className="flex gap-2">
                <Button size="sm" variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={loadMatrix}>
                  <RefreshCw className="w-3.5 h-3.5 mr-1.5" />
                  刷新
                </Button>
                <Button size="sm" className="bg-indigo-600 hover:bg-indigo-500" onClick={openCompatCreate} disabled={clients.length === 0}>
                  <Plus className="w-4 h-4 mr-1" />
                  添加条目
                </Button>
              </div>
            </div>

            <Card className="bg-zinc-900 border-zinc-800 overflow-hidden">
              <CardContent className="p-0">
                {matrixLoading ? (
                  <div className="p-4 space-y-3">
                    {[1, 2, 3].map((i) => (
                      <Skeleton key={i} className="h-12 w-full bg-zinc-800 rounded-lg" />
                    ))}
                  </div>
                ) : matrix.rows.length === 0 ? (
                  <EmptyState title="暂无兼容矩阵" description="添加客户端与功能/协议的兼容关系" className="py-12" />
                ) : (
                  <div className="overflow-x-auto">
                    <Table>
                      <TableHeader>
                        <TableRow className="border-zinc-800 hover:bg-transparent">
                          <TableHead className="text-zinc-400 text-xs font-medium sticky left-0 bg-zinc-900">客户端</TableHead>
                          <TableHead className="text-zinc-400 text-xs font-medium hidden sm:table-cell">平台</TableHead>
                          {matrix.features.map((f) => (
                            <TableHead key={f} className="text-zinc-400 text-xs font-medium whitespace-nowrap">{f}</TableHead>
                          ))}
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {matrix.rows.map((row) => (
                          <TableRow key={row.client_id} className="border-zinc-800 hover:bg-zinc-800/50">
                            <TableCell className="py-3 font-medium text-zinc-200 text-sm sticky left-0 bg-zinc-900">{row.client_name}</TableCell>
                            <TableCell className="py-3 hidden sm:table-cell">
                              <Badge variant="secondary" className="bg-zinc-800 text-zinc-300 text-xs">
                                {platformLabel[row.platform] || row.platform}
                              </Badge>
                            </TableCell>
                            {matrix.features.map((f) => {
                              const level = row.cells?.[f]
                              return (
                                <TableCell key={f} className="py-3">
                                  {level ? getCompatBadge(level) : <Minus className="w-3.5 h-3.5 text-zinc-700" />}
                                </TableCell>
                              )
                            })}
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

        {/* ===== 高级补丁 Tab ===== */}
        <TabsContent value="patches">
          <div className="space-y-4">
            <div className="flex items-center justify-between gap-2">
              <p className="text-sm text-zinc-500">针对特定客户端的高级补丁与覆盖配置</p>
              <div className="flex gap-2">
                <Button size="sm" variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={loadPatches}>
                  <RefreshCw className="w-3.5 h-3.5 mr-1.5" />
                  刷新
                </Button>
                <Button size="sm" className="bg-indigo-600 hover:bg-indigo-500" onClick={openPatchCreate} disabled={clients.length === 0}>
                  <Plus className="w-4 h-4 mr-1" />
                  添加补丁
                </Button>
              </div>
            </div>

            {patchesLoading ? (
              <div className="space-y-3">
                {[1, 2, 3].map((i) => (
                  <Skeleton key={i} className="h-24 w-full bg-zinc-800 rounded-lg" />
                ))}
              </div>
            ) : patches.length === 0 ? (
              <Card className="bg-zinc-900 border-zinc-800">
                <CardContent>
                  <EmptyState title="暂无补丁" description="添加针对客户端的高级补丁" className="py-12" />
                </CardContent>
              </Card>
            ) : (
              <div className="space-y-3">
                {patches.map((p) => (
                  <Card key={p.id} className="bg-zinc-900 border-zinc-800">
                    <CardContent className="p-4">
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0 flex-1">
                          <div className="flex items-center gap-2 flex-wrap">
                            <Code2 className="w-4 h-4 text-indigo-400 flex-shrink-0" />
                            <span className="font-medium text-zinc-100 text-sm">{p.client_name || p.client_id}</span>
                            <Badge variant="secondary" className="bg-zinc-800 text-zinc-300 text-xs">
                              {patchTypeLabel[p.patch_type] || p.patch_type}
                            </Badge>
                            {p.status === 'active' ? (
                              <Badge variant="success" className="text-xs">启用</Badge>
                            ) : (
                              <Badge variant="secondary" className="text-xs">停用</Badge>
                            )}
                          </div>
                          {p.note && <p className="text-xs text-zinc-500 mt-1">{p.note}</p>}
                          <pre className="mt-2 text-[10px] text-zinc-400 bg-zinc-950/60 border border-zinc-800 rounded-md p-2 max-h-32 overflow-auto whitespace-pre-wrap break-all">
                            {p.patch_content}
                          </pre>
                        </div>
                        <div className="flex items-center gap-1 flex-shrink-0">
                          <Button variant="ghost" size="icon" className="h-8 w-8 text-zinc-400 hover:text-zinc-200" onClick={() => openPatchEdit(p)} title="编辑">
                            <Pencil className="w-4 h-4" />
                          </Button>
                          <Button variant="ghost" size="icon" className="h-8 w-8 text-zinc-400 hover:text-red-400" onClick={() => deletePatch(p)} title="删除">
                            <Trash2 className="w-4 h-4" />
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

      {/* ===== 客户端 Dialog ===== */}
      <Dialog open={clientDialogOpen} onOpenChange={setClientDialogOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-lg">
          <DialogHeader>
            <DialogTitle>{clientForm.id ? '编辑客户端' : '添加客户端'}</DialogTitle>
            <DialogDescription>管理客户端配置文件</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400">客户端名称 *</label>
              <Input
                placeholder="如 v2rayN / Shadowrocket"
                value={clientForm.name}
                onChange={(e) => setClientForm({ ...clientForm, name: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              />
              {clientErrors.name && <p className="text-xs text-red-400">{clientErrors.name}</p>}
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <label className="text-xs text-zinc-400">平台 *</label>
                <Select
                  value={clientForm.platform}
                  onChange={(e) => setClientForm({ ...clientForm, platform: e.target.value as Platform })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                >
                  <option value="ios">iOS</option>
                  <option value="android">Android</option>
                  <option value="windows">Windows</option>
                  <option value="macos">macOS</option>
                  <option value="linux">Linux</option>
                </Select>
                {clientErrors.platform && <p className="text-xs text-red-400">{clientErrors.platform}</p>}
              </div>
              <div className="space-y-1.5">
                <label className="text-xs text-zinc-400">版本</label>
                <Input
                  placeholder="如 6.2.0"
                  value={clientForm.version}
                  onChange={(e) => setClientForm({ ...clientForm, version: e.target.value })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
              </div>
            </div>
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400">特性（逗号分隔）</label>
              <Input
                placeholder="如 vmess, vless, trojan"
                value={clientForm.features}
                onChange={(e) => setClientForm({ ...clientForm, features: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              />
            </div>
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400">状态</label>
              <Select
                value={clientForm.status}
                onChange={(e) => setClientForm({ ...clientForm, status: e.target.value as ClientStatus })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              >
                <option value="active">活跃</option>
                <option value="beta">Beta</option>
                <option value="deprecated">已弃用</option>
              </Select>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={() => setClientDialogOpen(false)}>
              取消
            </Button>
            <Button className="bg-indigo-600 hover:bg-indigo-500" disabled={clientSubmitting} onClick={submitClient}>
              {clientSubmitting ? '保存中...' : '保存'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* ===== 兼容条目 Dialog ===== */}
      <Dialog open={compatDialogOpen} onOpenChange={setCompatDialogOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-lg">
          <DialogHeader>
            <DialogTitle>添加兼容条目</DialogTitle>
            <DialogDescription>设置客户端对协议/功能的支持情况</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400">客户端 *</label>
              <Select
                value={compatForm.client_id}
                onChange={(e) => setCompatForm({ ...compatForm, client_id: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              >
                <option value="">请选择</option>
                {clients.map((c) => (
                  <option key={c.id} value={c.id}>{c.name}</option>
                ))}
              </Select>
              {compatErrors.client_id && <p className="text-xs text-red-400">{compatErrors.client_id}</p>}
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <label className="text-xs text-zinc-400">功能/协议 *</label>
                <Input
                  placeholder="如 vmess, vless"
                  value={compatForm.feature}
                  onChange={(e) => setCompatForm({ ...compatForm, feature: e.target.value })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
                {compatErrors.feature && <p className="text-xs text-red-400">{compatErrors.feature}</p>}
              </div>
              <div className="space-y-1.5">
                <label className="text-xs text-zinc-400">支持级别 *</label>
                <Select
                  value={compatForm.level}
                  onChange={(e) => setCompatForm({ ...compatForm, level: e.target.value as CompatLevel })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                >
                  <option value="supported">支持</option>
                  <option value="partial">部分支持</option>
                  <option value="unsupported">不支持</option>
                </Select>
              </div>
            </div>
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400">备注</label>
              <Input
                placeholder="可选说明"
                value={compatForm.note}
                onChange={(e) => setCompatForm({ ...compatForm, note: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={() => setCompatDialogOpen(false)}>
              取消
            </Button>
            <Button className="bg-indigo-600 hover:bg-indigo-500" disabled={compatSubmitting} onClick={submitCompat}>
              {compatSubmitting ? '保存中...' : '保存'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* ===== 补丁 Dialog ===== */}
      <Dialog open={patchDialogOpen} onOpenChange={setPatchDialogOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-lg">
          <DialogHeader>
            <DialogTitle>{patchForm.id ? '编辑补丁' : '添加补丁'}</DialogTitle>
            <DialogDescription>针对客户端的高级补丁配置</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400">客户端 *</label>
              <Select
                value={patchForm.client_id}
                onChange={(e) => setPatchForm({ ...patchForm, client_id: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              >
                <option value="">请选择</option>
                {clients.map((c) => (
                  <option key={c.id} value={c.id}>{c.name}</option>
                ))}
              </Select>
              {patchErrors.client_id && <p className="text-xs text-red-400">{patchErrors.client_id}</p>}
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <label className="text-xs text-zinc-400">补丁类型 *</label>
                <Select
                  value={patchForm.patch_type}
                  onChange={(e) => setPatchForm({ ...patchForm, patch_type: e.target.value as PatchType })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                >
                  <option value="config_override">配置覆盖</option>
                  <option value="route_patch">路由补丁</option>
                  <option value="dns_patch">DNS 补丁</option>
                  <option value="subscription_patch">订阅补丁</option>
                </Select>
                {patchErrors.patch_type && <p className="text-xs text-red-400">{patchErrors.patch_type}</p>}
              </div>
              <div className="space-y-1.5">
                <label className="text-xs text-zinc-400">状态</label>
                <Select
                  value={patchForm.status}
                  onChange={(e) => setPatchForm({ ...patchForm, status: e.target.value as 'active' | 'inactive' })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                >
                  <option value="active">启用</option>
                  <option value="inactive">停用</option>
                </Select>
              </div>
            </div>
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400">补丁内容 *</label>
              <Textarea
                rows={6}
                placeholder="JSON / YAML 补丁内容"
                value={patchForm.patch_content}
                onChange={(e) => setPatchForm({ ...patchForm, patch_content: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100 font-mono text-xs"
              />
              {patchErrors.patch_content && <p className="text-xs text-red-400">{patchErrors.patch_content}</p>}
            </div>
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400">备注</label>
              <Input
                placeholder="可选"
                value={patchForm.note}
                onChange={(e) => setPatchForm({ ...patchForm, note: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={() => setPatchDialogOpen(false)}>
              取消
            </Button>
            <Button className="bg-indigo-600 hover:bg-indigo-500" disabled={patchSubmitting} onClick={submitPatch}>
              {patchSubmitting ? '保存中...' : '保存'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
