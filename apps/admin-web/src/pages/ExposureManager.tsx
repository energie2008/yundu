import { useState, useEffect } from 'react'
import { Plus, Pencil, Trash2, Globe, Server, ShieldAlert, FileCode2, Network } from 'lucide-react'
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
  Separator,
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
type ExposureMode = 'direct_public_ip' | 'nginx_reverse_proxy' | 'cloudflare_tunnel_fixed'
type ExposureStatus = 'active' | 'inactive' | 'error' | 'configuring'

interface EdgeExposure {
  id: string
  node_id: string
  node_name?: string
  exposure_mode: ExposureMode
  public_port?: number
  origin_port?: number
  domain?: string
  nginx_config_id?: string
  status: ExposureStatus
  created_at?: string
  updated_at?: string
}

interface NginxConfig {
  id: string
  name?: string
  server_name?: string
  listen_port?: number
  config_content?: string
  created_at?: string
  updated_at?: string
}

interface CompatRule {
  id: string
  protocol_type?: string
  transport_type?: string
  exposure_mode: ExposureMode
  supported: boolean
  note?: string
  created_at?: string
}

// ===== 工具函数 =====
function getStatusBadge(status: ExposureStatus) {
  const map: Record<ExposureStatus, { label: string; variant: 'success' | 'secondary' | 'destructive' | 'warning' }> = {
    active: { label: '运行中', variant: 'success' },
    inactive: { label: '未启用', variant: 'secondary' },
    error: { label: '异常', variant: 'destructive' },
    configuring: { label: '配置中', variant: 'warning' },
  }
  const v = map[status] || map.inactive
  return <Badge variant={v.variant}>{v.label}</Badge>
}

const exposureModeLabel: Record<ExposureMode, string> = {
  direct_public_ip: '直连公网 IP',
  nginx_reverse_proxy: 'Nginx 反代',
  cloudflare_tunnel_fixed: 'CF 隧道(固定)',
}

const exposureModeIcon: Record<ExposureMode, JSX.Element> = {
  direct_public_ip: <Globe className="w-3.5 h-3.5" />,
  nginx_reverse_proxy: <Network className="w-3.5 h-3.5" />,
  cloudflare_tunnel_fixed: <Server className="w-3.5 h-3.5" />,
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
  id: '',
  node_id: '',
  exposure_mode: 'direct_public_ip' as ExposureMode,
  public_port: '',
  origin_port: '',
  domain: '',
  nginx_config_id: '',
}

const emptyNginxForm = {
  id: '',
  name: '',
  server_name: '',
  listen_port: '',
  config_content: '',
}

const emptyCompatForm = {
  id: '',
  protocol_type: '',
  transport_type: '',
  exposure_mode: 'nginx_reverse_proxy' as ExposureMode,
  supported: true,
  note: '',
}

// ===== 主组件 =====
export default function ExposureManager() {
  const { toast } = useToast()
  const [tab, setTab] = useState('exposures')

  // 服务器选择器（后端按 server 维度管理 exposure）
  interface ServerOption {
    id: string
    name?: string
    hostname?: string
    status?: string
  }
  const [servers, setServers] = useState<ServerOption[]>([])
  const [serversLoading, setServersLoading] = useState(true)
  const [selectedServerId, setSelectedServerId] = useState('')

  // 暴露列表（按选中服务器）
  const [loading, setLoading] = useState(false)
  const [exposures, setExposures] = useState<EdgeExposure[]>([])
  const [search, setSearch] = useState('')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [form, setForm] = useState(emptyForm)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [submitting, setSubmitting] = useState(false)

  // nginx configs — 后端无独立接口，暂不可用
  const [nginxLoading, setNginxLoading] = useState(false)
  const [nginxConfigs, setNginxConfigs] = useState<NginxConfig[]>([])
  const [nginxDialogOpen, setNginxDialogOpen] = useState(false)
  const [nginxForm, setNginxForm] = useState(emptyNginxForm)
  const [nginxErrors, setNginxErrors] = useState<Record<string, string>>({})
  const [nginxSubmitting, setNginxSubmitting] = useState(false)

  // 兼容规则 — 后端无独立接口，暂不可用
  const [compatLoading, setCompatLoading] = useState(false)
  const [compatRules, setCompatRules] = useState<CompatRule[]>([])
  const [compatDialogOpen, setCompatDialogOpen] = useState(false)
  const [compatForm, setCompatForm] = useState(emptyCompatForm)
  const [compatErrors, setCompatErrors] = useState<Record<string, string>>({})
  const [compatSubmitting, setCompatSubmitting] = useState(false)

  const loadServers = async () => {
    setServersLoading(true)
    try {
      const data = await api.get(EP.SERVERS)
      const list = normalizeList<ServerOption>(data)
      setServers(list)
      if (list.length > 0 && !selectedServerId) {
        setSelectedServerId(list[0].id)
      }
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '加载服务器列表失败'
      toast({ title: '加载失败', description: msg, variant: 'destructive' })
    } finally {
      setServersLoading(false)
    }
  }

  // 后端按 server 维度管理 exposure：GET /admin/servers/:id/exposure
  const loadExposures = async (serverId?: string) => {
    const sid = serverId || selectedServerId
    if (!sid) {
      setExposures([])
      return
    }
    setLoading(true)
    try {
      const data = await api.get(EP.SERVER_EXPOSURE(sid))
      // 后端返回单个 exposure 对象或数组，统一处理
      const list = normalizeList<EdgeExposure>(data)
      setExposures(list)
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '加载边缘暴露配置失败'
      toast({ title: '加载失败', description: msg, variant: 'destructive' })
      setExposures([])
    } finally {
      setLoading(false)
    }
  }

  // 后端无 /admin/nginx-configs 接口
  const loadNginxConfigs = async () => {
    setNginxConfigs([])
  }

  // 后端无 /admin/exposure-compat-rules 接口
  const loadCompatRules = async () => {
    setCompatRules([])
  }

  useEffect(() => {
    loadServers()
  }, [])

  useEffect(() => {
    if (selectedServerId) loadExposures(selectedServerId)
  }, [selectedServerId])

  const onTabChange = (value: string) => {
    setTab(value)
    if (value === 'nginx' && nginxConfigs.length === 0 && !nginxLoading) loadNginxConfigs()
    if (value === 'compat' && compatRules.length === 0 && !compatLoading) loadCompatRules()
  }

  // ===== 暴露表单 =====
  const openCreate = () => {
    if (!selectedServerId) {
      toast({ title: '操作失败', description: '请先选择服务器', variant: 'destructive' })
      return
    }
    setForm({ ...emptyForm, node_id: selectedServerId })
    setErrors({})
    setDialogOpen(true)
  }

  const openEdit = (e: EdgeExposure) => {
    setForm({
      id: e.id,
      node_id: e.node_id || selectedServerId,
      exposure_mode: e.exposure_mode,
      public_port: e.public_port ? String(e.public_port) : '',
      origin_port: e.origin_port ? String(e.origin_port) : '',
      domain: e.domain || '',
      nginx_config_id: e.nginx_config_id || '',
    })
    setErrors({})
    setDialogOpen(true)
  }

  const validate = () => {
    const e: Record<string, string> = {}
    // node_id 由服务器选择器自动填充，无需手动校验
    if (!form.exposure_mode) e.exposure_mode = '请选择暴露模式'
    if (form.public_port && (Number(form.public_port) < 1 || Number(form.public_port) > 65535)) {
      e.public_port = '公网端口需在 1-65535'
    }
    if (form.origin_port && (Number(form.origin_port) < 1 || Number(form.origin_port) > 65535)) {
      e.origin_port = '源端口需在 1-65535'
    }
    if (form.exposure_mode === 'nginx_reverse_proxy' && !form.origin_port) {
      e.origin_port = 'Nginx 反代需填写源端口'
    }
    if (form.exposure_mode === 'cloudflare_tunnel_fixed' && !form.domain) {
      e.domain = 'CF 隧道需填写域名'
    }
    setErrors(e)
    return Object.keys(e).length === 0
  }

  const submit = async () => {
    if (!validate()) return
    if (!selectedServerId) {
      toast({ title: '校验失败', description: '请先选择服务器', variant: 'destructive' })
      return
    }
    setSubmitting(true)
    try {
      // 后端按服务器维度管理 exposure：POST/PATCH /admin/servers/:id/exposure
      const payload = {
        node_id: selectedServerId,
        exposure_mode: form.exposure_mode,
        public_port: form.public_port ? Number(form.public_port) : undefined,
        origin_port: form.origin_port ? Number(form.origin_port) : undefined,
        domain: form.domain || undefined,
        nginx_config_id: form.nginx_config_id || undefined,
      }
      if (form.id) {
        await api.patch(EP.SERVER_EXPOSURE(selectedServerId), payload)
        toast({ title: '更新成功', description: '暴露配置已更新', variant: 'success' })
      } else {
        await api.post(EP.SERVER_EXPOSURE(selectedServerId), payload)
        toast({ title: '创建成功', description: '暴露配置已创建', variant: 'success' })
      }
      setDialogOpen(false)
      loadExposures(selectedServerId)
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '保存失败'
      toast({ title: '保存失败', description: msg, variant: 'destructive' })
    } finally {
      setSubmitting(false)
    }
  }

  const deleteExposure = async (e: EdgeExposure) => {
    if (!selectedServerId) {
      toast({ title: '操作失败', description: '请先选择服务器', variant: 'destructive' })
      return
    }
    if (!window.confirm(`确认删除服务器「${selectedServerId}」的暴露配置？`)) return
    try {
      await api.delete(EP.SERVER_EXPOSURE(selectedServerId))
      toast({ title: '已删除', description: '暴露配置已删除', variant: 'success' })
      loadExposures(selectedServerId)
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '删除失败'
      toast({ title: '删除失败', description: msg, variant: 'destructive' })
    }
  }

  const filteredExposures = exposures.filter((e) =>
    e.node_id.toLowerCase().includes(search.toLowerCase()) ||
    (e.domain || '').toLowerCase().includes(search.toLowerCase()) ||
    (e.node_name || '').toLowerCase().includes(search.toLowerCase())
  )

  const getNginxName = (id?: string) => nginxConfigs.find((n) => n.id === id)?.name || id || '-'

  // ===== Nginx 配置表单 =====
  const openNginxCreate = () => {
    setNginxForm(emptyNginxForm)
    setNginxErrors({})
    setNginxDialogOpen(true)
  }

  const openNginxEdit = (n: NginxConfig) => {
    setNginxForm({
      id: n.id,
      name: n.name || '',
      server_name: n.server_name || '',
      listen_port: n.listen_port ? String(n.listen_port) : '',
      config_content: n.config_content || '',
    })
    setNginxErrors({})
    setNginxDialogOpen(true)
  }

  const validateNginx = () => {
    const e: Record<string, string> = {}
    if (!nginxForm.name.trim()) e.name = '请输入配置名称'
    if (!nginxForm.config_content.trim()) e.config_content = '请输入配置内容'
    setNginxErrors(e)
    return Object.keys(e).length === 0
  }

  // 后端无 /admin/nginx-configs 接口，Nginx 配置由 exposure apply 时自动生成
  const submitNginx = async () => {
    if (!validateNginx()) return
    toast({
      title: '暂不可用',
      description: '后端尚未实现独立 Nginx 配置管理接口，配置由暴露应用时自动生成',
      variant: 'destructive',
    })
    setNginxDialogOpen(false)
  }

  // ===== 兼容规则表单 =====
  const openCompatCreate = () => {
    setCompatForm(emptyCompatForm)
    setCompatErrors({})
    setCompatDialogOpen(true)
  }

  const validateCompat = () => {
    const e: Record<string, string> = {}
    if (!compatForm.exposure_mode) e.exposure_mode = '请选择暴露模式'
    setCompatErrors(e)
    return Object.keys(e).length === 0
  }

  // 后端无 /admin/exposure-compat-rules 接口
  const submitCompat = async () => {
    if (!validateCompat()) return
    toast({
      title: '暂不可用',
      description: '后端尚未实现兼容规则独立管理接口',
      variant: 'destructive',
    })
    setCompatDialogOpen(false)
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between flex-wrap gap-3">
        <h2 className="text-lg font-semibold text-zinc-100">边缘暴露管理</h2>
        {/* 服务器选择器：后端按 server 维度管理 exposure */}
        <div className="flex items-center gap-2 min-w-0 flex-1 sm:flex-initial sm:w-80">
          <Server className="w-4 h-4 text-zinc-400 flex-shrink-0" />
          {serversLoading ? (
            <Skeleton className="h-9 flex-1 bg-zinc-800 rounded-md" />
          ) : servers.length === 0 ? (
            <span className="text-xs text-zinc-500">暂无服务器</span>
          ) : (
            <Select
              value={selectedServerId}
              onChange={(e) => setSelectedServerId(e.target.value)}
              className="bg-zinc-900 border-zinc-800 text-zinc-100 flex-1"
            >
              {servers.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name || s.hostname || s.id}
                  {s.status ? ` (${s.status})` : ''}
                </option>
              ))}
            </Select>
          )}
        </div>
      </div>

      <Tabs value={tab} onValueChange={onTabChange}>
        <TabsList>
          <TabsTrigger value="exposures">
            <Globe className="w-3.5 h-3.5 mr-1.5" />
            暴露配置
          </TabsTrigger>
          <TabsTrigger value="nginx">
            <FileCode2 className="w-3.5 h-3.5 mr-1.5" />
            Nginx 配置
          </TabsTrigger>
          <TabsTrigger value="compat">
            <ShieldAlert className="w-3.5 h-3.5 mr-1.5" />
            兼容规则
          </TabsTrigger>
        </TabsList>

        {/* ===== 暴露配置 Tab ===== */}
        <TabsContent value="exposures">
          <div className="space-y-4">
            <div className="flex items-center justify-between gap-2">
              <Input
                placeholder="搜索节点 ID 或域名..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="bg-zinc-900 border-zinc-800 text-zinc-100 placeholder:text-zinc-500 max-w-xs"
              />
              <Button size="sm" className="bg-indigo-600 hover:bg-indigo-500" onClick={openCreate}>
                <Plus className="w-4 h-4 mr-1" />
                新建暴露
              </Button>
            </div>

            <Card className="bg-zinc-900 border-zinc-800 overflow-hidden">
              <CardContent className="p-0">
                {loading ? (
                  <div className="p-4 space-y-3">
                    {[1, 2, 3].map((i) => (
                      <Skeleton key={i} className="h-16 w-full bg-zinc-800 rounded-lg" />
                    ))}
                  </div>
                ) : !selectedServerId ? (
                  <EmptyState title="请先选择服务器" description="在顶部下拉框选择一台服务器以查看其暴露配置" className="py-12" />
                ) : filteredExposures.length === 0 ? (
                  <EmptyState title="暂无暴露配置" description="新建暴露以将节点对外提供服务" className="py-12" />
                ) : (
                  <div className="overflow-x-auto">
                    <Table>
                      <TableHeader>
                        <TableRow className="border-zinc-800 hover:bg-transparent">
                          <TableHead className="text-zinc-400 text-xs font-medium">节点</TableHead>
                          <TableHead className="text-zinc-400 text-xs font-medium">暴露模式</TableHead>
                          <TableHead className="text-zinc-400 text-xs font-medium hidden sm:table-cell">公网端口</TableHead>
                          <TableHead className="text-zinc-400 text-xs font-medium hidden md:table-cell">源端口</TableHead>
                          <TableHead className="text-zinc-400 text-xs font-medium">域名</TableHead>
                          <TableHead className="text-zinc-400 text-xs font-medium">状态</TableHead>
                          <TableHead className="text-zinc-400 text-xs font-medium w-20"></TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {filteredExposures.map((e) => (
                          <TableRow key={e.id} className="border-zinc-800 hover:bg-zinc-800/50">
                            <TableCell className="py-3">
                              <div className="font-medium text-zinc-200 text-sm">{e.node_name || e.node_id}</div>
                              <div className="text-xs text-zinc-500 sm:hidden mt-0.5 flex items-center gap-1">
                                {exposureModeIcon[e.exposure_mode]}
                                {exposureModeLabel[e.exposure_mode]}
                              </div>
                            </TableCell>
                            <TableCell className="py-3">
                              <Badge variant="secondary" className="bg-zinc-800 text-zinc-300 text-xs inline-flex items-center gap-1">
                                {exposureModeIcon[e.exposure_mode]}
                                {exposureModeLabel[e.exposure_mode]}
                              </Badge>
                            </TableCell>
                            <TableCell className="py-3 hidden sm:table-cell text-sm text-zinc-300">{e.public_port || '-'}</TableCell>
                            <TableCell className="py-3 hidden md:table-cell text-sm text-zinc-300">{e.origin_port || '-'}</TableCell>
                            <TableCell className="py-3 text-sm text-zinc-400">
                              <span className="truncate inline-block max-w-[160px] align-bottom">{e.domain || '-'}</span>
                            </TableCell>
                            <TableCell className="py-3">{getStatusBadge(e.status)}</TableCell>
                            <TableCell className="py-3">
                              <div className="flex items-center gap-1">
                                <Button variant="ghost" size="icon" className="h-8 w-8 text-zinc-400 hover:text-zinc-200" onClick={() => openEdit(e)}>
                                  <Pencil className="w-4 h-4" />
                                </Button>
                                <Button variant="ghost" size="icon" className="h-8 w-8 text-zinc-400 hover:text-red-400" onClick={() => deleteExposure(e)}>
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
          </div>
        </TabsContent>

        {/* ===== Nginx 配置 Tab ===== */}
        <TabsContent value="nginx">
          <div className="space-y-4">
            <Card className="bg-zinc-900 border-zinc-800 border-amber-800/40">
              <CardContent className="p-4">
                <div className="flex items-start gap-3">
                  <FileCode2 className="w-5 h-5 text-amber-400 flex-shrink-0 mt-0.5" />
                  <div className="min-w-0">
                    <p className="text-sm text-zinc-200 font-medium">该模块暂不可用</p>
                    <p className="text-xs text-zinc-500 mt-1">
                      后端尚未实现独立 Nginx 配置管理接口（/admin/nginx-configs）。
                      Nginx 反代配置将在「暴露配置」应用时由后端自动生成，无需手工维护。
                    </p>
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        {/* ===== 兼容规则 Tab ===== */}
        <TabsContent value="compat">
          <div className="space-y-4">
            <Card className="bg-zinc-900 border-zinc-800 border-amber-800/40">
              <CardContent className="p-4">
                <div className="flex items-start gap-3">
                  <ShieldAlert className="w-5 h-5 text-amber-400 flex-shrink-0 mt-0.5" />
                  <div className="min-w-0">
                    <p className="text-sm text-zinc-200 font-medium">该模块暂不可用</p>
                    <p className="text-xs text-zinc-500 mt-1">
                      后端尚未实现兼容规则独立管理接口（/admin/exposure-compat-rules）。
                    </p>
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>
        </TabsContent>
      </Tabs>

      {/* ===== 暴露配置 Dialog ===== */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-lg">
          <DialogHeader>
            <DialogTitle>{form.id ? '编辑暴露配置' : '新建暴露配置'}</DialogTitle>
            <DialogDescription>配置节点对外的暴露模式与端口</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400">目标服务器</label>
              <div className="h-9 px-3 flex items-center rounded-md border border-zinc-700 bg-zinc-950/60 text-sm text-zinc-300">
                {servers.find((s) => s.id === form.node_id)?.name
                  || servers.find((s) => s.id === form.node_id)?.hostname
                  || form.node_id
                  || '未选择服务器'}
              </div>
            </div>
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400">暴露模式 *</label>
              <Select
                value={form.exposure_mode}
                onChange={(e) => setForm({ ...form, exposure_mode: e.target.value as ExposureMode })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              >
                <option value="direct_public_ip">直连公网 IP</option>
                <option value="nginx_reverse_proxy">Nginx 反向代理</option>
                <option value="cloudflare_tunnel_fixed">Cloudflare 隧道(固定)</option>
              </Select>
              {errors.exposure_mode && <p className="text-xs text-red-400">{errors.exposure_mode}</p>}
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <label className="text-xs text-zinc-400">公网端口</label>
                <Input
                  type="number"
                  placeholder="如 443"
                  value={form.public_port}
                  onChange={(e) => setForm({ ...form, public_port: e.target.value })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
                {errors.public_port && <p className="text-xs text-red-400">{errors.public_port}</p>}
              </div>
              <div className="space-y-1.5">
                <label className="text-xs text-zinc-400">源端口</label>
                <Input
                  type="number"
                  placeholder="如 8080"
                  value={form.origin_port}
                  onChange={(e) => setForm({ ...form, origin_port: e.target.value })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
                {errors.origin_port && <p className="text-xs text-red-400">{errors.origin_port}</p>}
              </div>
            </div>
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400">域名</label>
              <Input
                placeholder="如 example.com"
                value={form.domain}
                onChange={(e) => setForm({ ...form, domain: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              />
              {errors.domain && <p className="text-xs text-red-400">{errors.domain}</p>}
            </div>
            {form.exposure_mode === 'nginx_reverse_proxy' && (
              <div className="space-y-1.5">
                <label className="text-xs text-zinc-400">关联 Nginx 配置</label>
                <Select
                  value={form.nginx_config_id}
                  onChange={(e) => setForm({ ...form, nginx_config_id: e.target.value })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                >
                  <option value="">不关联</option>
                  {nginxConfigs.map((n) => (
                    <option key={n.id} value={n.id}>{n.name}</option>
                  ))}
                </Select>
                {nginxConfigs.length === 0 && (
                  <p className="text-xs text-zinc-500">提示: 暂无可关联的 Nginx 配置，请先在 Nginx 配置 Tab 创建</p>
                )}
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={() => setDialogOpen(false)}>
              取消
            </Button>
            <Button className="bg-indigo-600 hover:bg-indigo-500" disabled={submitting} onClick={submit}>
              {submitting ? '保存中...' : '保存'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* ===== Nginx 配置 Dialog ===== */}
      <Dialog open={nginxDialogOpen} onOpenChange={setNginxDialogOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-2xl">
          <DialogHeader>
            <DialogTitle>{nginxForm.id ? '编辑 Nginx 配置' : '新建 Nginx 配置'}</DialogTitle>
            <DialogDescription>填写反向代理配置内容</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <label className="text-xs text-zinc-400">配置名称 *</label>
                <Input
                  placeholder="如 ws-tls-proxy"
                  value={nginxForm.name}
                  onChange={(e) => setNginxForm({ ...nginxForm, name: e.target.value })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
                {nginxErrors.name && <p className="text-xs text-red-400">{nginxErrors.name}</p>}
              </div>
              <div className="space-y-1.5">
                <label className="text-xs text-zinc-400">server_name</label>
                <Input
                  placeholder="如 example.com"
                  value={nginxForm.server_name}
                  onChange={(e) => setNginxForm({ ...nginxForm, server_name: e.target.value })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
              </div>
            </div>
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400">监听端口</label>
              <Input
                type="number"
                placeholder="如 443"
                value={nginxForm.listen_port}
                onChange={(e) => setNginxForm({ ...nginxForm, listen_port: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              />
            </div>
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400">配置内容 *</label>
              <Textarea
                rows={12}
                placeholder={'server {\n  listen 443 ssl;\n  ...'}
                value={nginxForm.config_content}
                onChange={(e) => setNginxForm({ ...nginxForm, config_content: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100 font-mono text-xs"
              />
              {nginxErrors.config_content && <p className="text-xs text-red-400">{nginxErrors.config_content}</p>}
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={() => setNginxDialogOpen(false)}>
              取消
            </Button>
            <Button className="bg-indigo-600 hover:bg-indigo-500" disabled={nginxSubmitting} onClick={submitNginx}>
              {nginxSubmitting ? '保存中...' : '保存'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* ===== 兼容规则 Dialog ===== */}
      <Dialog open={compatDialogOpen} onOpenChange={setCompatDialogOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-lg">
          <DialogHeader>
            <DialogTitle>{compatForm.id ? '编辑兼容规则' : '新建兼容规则'}</DialogTitle>
            <DialogDescription>定义协议在特定暴露模式下的兼容性</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <label className="text-xs text-zinc-400">协议类型</label>
                <Input
                  placeholder="留空表示通配"
                  value={compatForm.protocol_type}
                  onChange={(e) => setCompatForm({ ...compatForm, protocol_type: e.target.value })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
              </div>
              <div className="space-y-1.5">
                <label className="text-xs text-zinc-400">传输类型</label>
                <Input
                  placeholder="留空表示通配"
                  value={compatForm.transport_type}
                  onChange={(e) => setCompatForm({ ...compatForm, transport_type: e.target.value })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
              </div>
            </div>
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400">暴露模式 *</label>
              <Select
                value={compatForm.exposure_mode}
                onChange={(e) => setCompatForm({ ...compatForm, exposure_mode: e.target.value as ExposureMode })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              >
                <option value="direct_public_ip">直连公网 IP</option>
                <option value="nginx_reverse_proxy">Nginx 反向代理</option>
                <option value="cloudflare_tunnel_fixed">Cloudflare 隧道(固定)</option>
              </Select>
              {compatErrors.exposure_mode && <p className="text-xs text-red-400">{compatErrors.exposure_mode}</p>}
            </div>
            <div className="flex items-center justify-between rounded-lg border border-zinc-800 bg-zinc-950/40 p-3">
              <div>
                <div className="text-sm text-zinc-200">是否兼容</div>
                <div className="text-xs text-zinc-500">关闭表示该组合不兼容</div>
              </div>
              <Select
                value={compatForm.supported ? 'true' : 'false'}
                onChange={(e) => setCompatForm({ ...compatForm, supported: e.target.value === 'true' })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100 w-28"
              >
                <option value="true">兼容</option>
                <option value="false">不兼容</option>
              </Select>
            </div>
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400">备注</label>
              <Textarea
                rows={2}
                placeholder="规则说明"
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
    </div>
  )
}
