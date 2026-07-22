import { useState, useEffect } from 'react'
import {
  Plus,
  Pencil,
  Trash2,
  Server,
  Search,
  Key,
  Terminal,
  Copy,
  Check,
  RefreshCw,
  AlertCircle,
} from 'lucide-react'
import {
  Card,
  CardContent,
  Button,
  Input,
  Label,
  Badge,
  Skeleton,
  EmptyState,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  useToast,
} from '@airport/ui'
import { api } from '@/lib/api'
import { EP } from '@/lib/endpoints'

interface Machine {
  id: string
  name: string
  host: string
  port: number
  is_enabled: boolean
  region?: string
  tags?: string[]
  config_json?: Record<string, unknown>
  created_at: string
  updated_at: string
  agent_token?: string
  install_cmd?: string
  [key: string]: unknown
}

interface ServerResponseItem {
  id: string
  name: string
  host: string
  port?: number
  is_enabled?: boolean
  region?: string
  tags?: string[]
  config_json?: Record<string, unknown>
  created_at: string
  updated_at: string
}

interface ServerDetailResponse extends ServerResponseItem {
  agent_token?: string
  install_cmd?: string
}

function extractList<T>(resp: unknown): T[] {
  if (Array.isArray(resp)) return resp as T[]
  if (!resp || typeof resp !== 'object') return []
  const obj = resp as Record<string, unknown>
  if (Array.isArray(obj.data)) return obj.data as T[]
  if (obj.data && typeof obj.data === 'object') {
    const dataObj = obj.data as Record<string, unknown>
    if (Array.isArray(dataObj.items)) return dataObj.items as T[]
    if (Array.isArray(dataObj.list)) return dataObj.list as T[]
  }
  if (Array.isArray(obj.items)) return obj.items as T[]
  if (Array.isArray(obj.list)) return obj.list as T[]
  return []
}

function mapServerResponse(s: ServerResponseItem): Machine {
  return {
    id: s.id,
    name: s.name,
    host: s.host,
    port: s.port || 22,
    is_enabled: s.is_enabled ?? true,
    region: s.region,
    tags: s.tags,
    config_json: s.config_json,
    created_at: s.created_at,
    updated_at: s.updated_at,
    ip: s.host,
    status: s.is_enabled ? 1 : 0,
  }
}

function getStatusBadge(isEnabled?: boolean) {
  const isOnline = isEnabled === true
  if (isOnline) {
    return (
      <Badge variant="outline" className="bg-emerald-900/50 text-emerald-300 border-emerald-800/50 text-xs">
        <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 mr-1.5 animate-pulse" />
        在线
      </Badge>
    )
  }
  return (
    <Badge variant="outline" className="bg-zinc-800 text-zinc-400 border-zinc-700 text-xs">
      <span className="w-1.5 h-1.5 rounded-full bg-zinc-500 mr-1.5" />
      离线
    </Badge>
  )
}

export default function Machines() {
  const { toast } = useToast()
  const [loading, setLoading] = useState(true)
  const [machines, setMachines] = useState<Machine[]>([])
  const [search, setSearch] = useState('')

  const [dialogOpen, setDialogOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [form, setForm] = useState({
    name: '',
    ip: '',
    port: 22,
  })

  const [installDialogOpen, setInstallDialogOpen] = useState(false)
  const [installMachine, setInstallMachine] = useState<Machine | null>(null)
  const [copiedField, setCopiedField] = useState<'token' | 'cmd' | null>(null)
  const [resetLoading, setResetLoading] = useState(false)

  const loadData = async () => {
    setLoading(true)
    try {
      const data = await api.get<unknown>(EP.SERVERS, {
        params: { page: 1, page_size: 200 },
      })
      const list = extractList<ServerResponseItem>(data)
      setMachines(list.map(mapServerResponse))
    } catch (err) {
      toast({
        title: '加载失败',
        description: err instanceof Error ? err.message : '无法获取机器数据',
        variant: 'destructive',
      })
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadData()
  }, [])

  const filtered = machines.filter(m => {
    const kw = search.trim().toLowerCase()
    return !kw ||
      (m.name || '').toLowerCase().includes(kw) ||
      (m.host || '').toLowerCase().includes(kw) ||
      String(m.id).includes(kw)
  })

  function openCreate() {
    setEditingId(null)
    setForm({ name: '', ip: '', port: 22 })
    setDialogOpen(true)
  }

  function openEdit(m: Machine) {
    setEditingId(m.id)
    setForm({
      name: m.name || '',
      ip: m.host || '',
      port: Number(m.port) || 22,
    })
    setDialogOpen(true)
  }

  async function handleSave() {
    if (!form.name.trim()) {
      toast({ title: '校验失败', description: '请输入机器名称', variant: 'destructive' })
      return
    }
    if (!form.ip.trim()) {
      toast({ title: '校验失败', description: '请输入IP地址', variant: 'destructive' })
      return
    }
    setSaving(true)
    try {
      const payload: Record<string, unknown> = {
        name: form.name.trim(),
        host: form.ip.trim(),
        port: Number(form.port) || 22,
      }
      if (editingId) {
        await api.patch(EP.SERVER_DETAIL(editingId), payload)
        toast({ title: '机器已更新', variant: 'success' })
      } else {
        await api.post(EP.SERVERS, payload)
        toast({ title: '机器已创建', variant: 'success' })
      }
      setDialogOpen(false)
      await loadData()
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

  async function handleDelete(m: Machine) {
    if (!window.confirm(`确定删除机器「${m.name}」吗？此操作不可撤销。`)) return
    try {
      await api.delete(EP.SERVER_DETAIL(m.id))
      toast({ title: '机器已删除', variant: 'success' })
      await loadData()
    } catch (err) {
      toast({
        title: '删除失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    }
  }

  async function handleResetToken(m: Machine) {
    toast({
      title: '功能开发中',
      description: '该功能API开发中',
      variant: 'destructive',
    })
  }

  async function openInstallDialog(m: Machine) {
    setInstallMachine(m)
    setCopiedField(null)
    setInstallDialogOpen(true)
    try {
      const detail = await api.get<ServerDetailResponse>(EP.SERVER_DETAIL(m.id))
      const updated: Machine = {
        ...m,
        agent_token: detail.agent_token,
        install_cmd: detail.install_cmd,
      }
      setMachines(prev => prev.map(machine => {
        if (machine.id === m.id) {
          return updated
        }
        return machine
      }))
      setInstallMachine(updated)
    } catch {
    }
  }

  async function copyToClipboard(text: string, field: 'token' | 'cmd') {
    try {
      await navigator.clipboard.writeText(text)
      setCopiedField(field)
      toast({ title: '已复制到剪贴板', variant: 'success' })
      setTimeout(() => setCopiedField(null), 2000)
    } catch {
      toast({ title: '复制失败', variant: 'destructive' })
    }
  }

  return (
    <div className="space-y-4 pb-20 sm:pb-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-zinc-100">机器管理</h2>
        <Button size="sm" className="bg-indigo-600 hover:bg-indigo-500" onClick={openCreate}>
          <Plus className="w-4 h-4 mr-1" />添加机器
        </Button>
      </div>

      <Card className="bg-zinc-900 border-zinc-800">
        <CardContent className="p-3">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-zinc-500" />
            <Input
              placeholder="搜索机器名称或IP..."
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
            <EmptyState title="暂无机器" description="添加机器来部署和管理节点服务" className="py-12" />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-zinc-800">
                    <th className="text-left p-3 text-xs font-medium text-zinc-400 w-20">ID</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400">名称</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400 hidden md:table-cell">IP地址</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400 w-20 hidden md:table-cell">端口</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400 w-24">状态</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400 w-48">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {filtered.map((m) => (
                    <tr key={m.id} className="border-b border-zinc-800 last:border-0 hover:bg-zinc-800/50 transition-colors">
                      <td className="p-3">
                        <span className="font-mono text-xs text-zinc-500">{m.id}</span>
                      </td>
                      <td className="p-3">
                        <div className="flex items-center gap-2">
                          <div className="p-1.5 rounded-md bg-amber-900/30">
                            <Server className="w-4 h-4 text-amber-400" />
                          </div>
                          <span className="text-sm font-medium text-zinc-200">{m.name}</span>
                        </div>
                      </td>
                      <td className="p-3 hidden md:table-cell">
                        <span className="font-mono text-xs text-zinc-400">{m.host || '-'}</span>
                      </td>
                      <td className="p-3 hidden md:table-cell">
                        <span className="font-mono text-xs text-zinc-500">{m.port || 22}</span>
                      </td>
                      <td className="p-3">{getStatusBadge(m.is_enabled)}</td>
                      <td className="p-3">
                        <div className="flex items-center gap-1 flex-wrap">
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-7 text-xs text-cyan-400 hover:text-cyan-300 px-2"
                            onClick={() => openInstallDialog(m)}
                          >
                            <Terminal className="w-3.5 h-3.5 mr-1" />安装
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-7 text-xs text-amber-400 hover:text-amber-300 px-2"
                            onClick={() => handleResetToken(m)}
                            disabled={resetLoading}
                          >
                            <RefreshCw className={`w-3.5 h-3.5 mr-1 ${resetLoading ? 'animate-spin' : ''}`} />重置
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-7 w-7 text-zinc-400 hover:text-indigo-300"
                            onClick={() => openEdit(m)}
                          >
                            <Pencil className="w-3.5 h-3.5" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-7 w-7 text-zinc-400 hover:text-red-300"
                            onClick={() => handleDelete(m)}
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
          )}
        </CardContent>
      </Card>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Server className="w-5 h-5 text-amber-400" />
              <span>{editingId ? '编辑机器' : '添加机器'}</span>
            </DialogTitle>
          </DialogHeader>

          <div className="space-y-3 pt-2">
            <div className="space-y-1.5">
              <Label className="text-zinc-300 text-sm">机器名称 *</Label>
              <Input
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                placeholder="如: 美国硅谷01"
                className="bg-zinc-800 border-zinc-700 text-zinc-100 h-9"
              />
            </div>
            <div className="space-y-1.5">
              <Label className="text-zinc-300 text-sm">IP地址 *</Label>
              <Input
                value={form.ip}
                onChange={(e) => setForm({ ...form, ip: e.target.value })}
                placeholder="如: 104.xx.xx.21"
                className="bg-zinc-800 border-zinc-700 text-zinc-100 h-9 font-mono"
              />
            </div>
            <div className="space-y-1.5">
              <Label className="text-zinc-300 text-sm">SSH端口</Label>
              <Input
                type="number"
                value={form.port}
                onChange={(e) => setForm({ ...form, port: Number(e.target.value) })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100 h-9"
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
              {editingId ? '保存' : '创建'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={installDialogOpen} onOpenChange={setInstallDialogOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-xl">
          {installMachine && (
            <>
              <DialogHeader>
                <DialogTitle className="flex items-center gap-2">
                  <Terminal className="w-5 h-5 text-cyan-400" />
                  <span>部署命令 - {installMachine.name}</span>
                </DialogTitle>
              </DialogHeader>

              <div className="space-y-4 pt-2">
                <div className="space-y-2">
                  <Label className="text-zinc-300 text-sm flex items-center gap-2">
                    <Key className="w-4 h-4 text-amber-400" />机器 Token
                  </Label>
                  <div className="bg-zinc-950 rounded-lg p-3 font-mono text-xs flex items-center gap-2">
                    <code className="text-zinc-300 flex-1 break-all">
                      {installMachine.agent_token || '(Token尚未生成，请先点击重置Token)'}
                    </code>
                    {installMachine.agent_token && (
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-7 w-7 p-0 text-zinc-500 hover:text-zinc-300 hover:bg-zinc-800 flex-shrink-0"
                        onClick={() => copyToClipboard(installMachine.agent_token as string, 'token')}
                      >
                        {copiedField === 'token' ? <Check className="w-4 h-4 text-emerald-400" /> : <Copy className="w-4 h-4" />}
                      </Button>
                    )}
                  </div>
                </div>

                <div className="space-y-2">
                  <Label className="text-zinc-300 text-sm flex items-center gap-2">
                    <Terminal className="w-4 h-4 text-cyan-400" />一键安装命令
                  </Label>
                  <div className="bg-zinc-950 rounded-lg p-3 relative group">
                    <pre className="font-mono text-xs text-zinc-300 overflow-x-auto whitespace-pre-wrap break-all pr-8 max-h-40 overflow-y-auto">
                      {installMachine.install_cmd || '(请先生成Token后获取安装命令)'}
                    </pre>
                    {installMachine.install_cmd && (
                      <Button
                        size="sm"
                        className={`absolute top-2 right-2 h-7 w-7 p-0 ${
                          copiedField === 'cmd'
                            ? 'bg-emerald-600 hover:bg-emerald-500'
                            : 'bg-cyan-600 hover:bg-cyan-500'
                        }`}
                        onClick={() => copyToClipboard(installMachine.install_cmd as string, 'cmd')}
                      >
                        {copiedField === 'cmd' ? <Check className="w-4 h-4" /> : <Copy className="w-4 h-4" />}
                      </Button>
                    )}
                  </div>
                </div>

                <div className="bg-amber-950/30 border border-amber-900/50 rounded-lg p-3">
                  <div className="flex items-start gap-2">
                    <AlertCircle className="w-4 h-4 text-amber-400 mt-0.5 flex-shrink-0" />
                    <div className="text-xs text-amber-200/80 space-y-1">
                      <p>1. 请以 root 用户登录目标服务器</p>
                      <p>2. 复制上方安装命令并在服务器上执行</p>
                      <p>3. 安装完成后机器将自动连接到面板</p>
                      <p>4. 如果Token泄露，请立即点击重置Token</p>
                    </div>
                  </div>
                </div>
              </div>

              <DialogFooter className="pt-2">
                <Button
                  variant="outline"
                  onClick={() => setInstallDialogOpen(false)}
                  className="border-zinc-700 text-zinc-300"
                >
                  关闭
                </Button>
                {installMachine.install_cmd && (
                  <Button
                    className="bg-cyan-600 hover:bg-cyan-500"
                    onClick={() => copyToClipboard(installMachine.install_cmd as string, 'cmd')}
                  >
                    {copiedField === 'cmd' ? (
                      <><Check className="w-4 h-4 mr-1" />已复制</>
                    ) : (
                      <><Copy className="w-4 h-4 mr-1" />复制命令</>
                    )}
                  </Button>
                )}
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}
