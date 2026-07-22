import { useState, useEffect, useRef } from 'react'
import { Puzzle, Upload, Trash2, Settings, Power, PowerOff, Download, ArrowUpCircle, Package } from 'lucide-react'
import {
  Card,
  CardContent,
  Button,
  Input,
  Label,
  Badge,
  Switch,
  Skeleton,
  EmptyState,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  useToast,
  Select,
} from '@airport/ui'
import { xbAdminApi } from '@/lib/api'

interface PluginType {
  id: string
  name: string
  description?: string
}

interface Plugin {
  id: string | number
  name: string
  version?: string
  description?: string
  author?: string
  type?: string
  status?: number | boolean
  enabled?: number | boolean
  installed?: number | boolean
  is_upgrade?: number | boolean
  config?: Record<string, unknown>
  config_fields?: PluginConfigField[]
  icon?: string
  [key: string]: unknown
}

interface PluginConfigField {
  name: string
  label?: string
  type?: string
  placeholder?: string
  default?: string
  required?: boolean
  options?: { label: string; value: string }[]
}

function extractList<T>(resp: unknown): T[] {
  if (Array.isArray(resp)) return resp as T[]
  if (!resp || typeof resp !== 'object') return []
  const obj = resp as Record<string, unknown>
  const dataField = obj.data
  if (dataField && typeof dataField === 'object') {
    if (Array.isArray(dataField)) return dataField as T[]
    const dataObj = dataField as Record<string, unknown>
    if (Array.isArray(dataObj.plugins)) return dataObj.plugins as T[]
    if (Array.isArray(dataObj.data)) return dataObj.data as T[]
    if (Array.isArray(dataObj.items)) return dataObj.items as T[]
    if (Array.isArray(dataObj.list)) return dataObj.list as T[]
  }
  if (Array.isArray(obj.plugins)) return obj.plugins as T[]
  if (Array.isArray(obj.data)) return obj.data as T[]
  if (Array.isArray(obj.items)) return obj.items as T[]
  return []
}

function extractObject<T>(resp: unknown): T | null {
  if (!resp || typeof resp !== 'object') return null
  const obj = resp as Record<string, unknown>
  const dataField = obj.data
  if (dataField && typeof dataField === 'object' && !Array.isArray(dataField)) {
    return dataField as T
  }
  return obj as T
}

function toBool(v: unknown): boolean {
  if (typeof v === 'boolean') return v
  if (typeof v === 'number') return v === 1
  if (typeof v === 'string') return v === '1' || v === 'true'
  return false
}

const TOKEN_KEY = 'airport_admin_token'

import { ComingSoon } from '@/components/common/ComingSoon'

export default function SystemPlugin() {
  return <ComingSoon title="插件管理" reason="该模块后端 API 正在迁移到 YunDu Go 微服务，暂不可用。" />

  const { toast } = useToast()
  const [loading, setLoading] = useState(true)
  const [plugins, setPlugins] = useState<Plugin[]>([])
  const [pluginTypes, setPluginTypes] = useState<PluginType[]>([])
  const [typeFilter, setTypeFilter] = useState<string>('all')
  const [uploading, setUploading] = useState(false)
  const [actionLoading, setActionLoading] = useState<string | number | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const [configOpen, setConfigOpen] = useState(false)
  const [configSaving, setConfigSaving] = useState(false)
  const [selectedPlugin, setSelectedPlugin] = useState<Plugin | null>(null)
  const [pluginConfig, setPluginConfig] = useState<Record<string, string>>({})

  useEffect(() => {
    loadData()
  }, [])

  const loadData = async () => {
    setLoading(true)
    try {
      const [pluginsResp, typesResp] = await Promise.all([
        xbAdminApi.get<unknown>('/plugin/getPlugins'),
        xbAdminApi.get<unknown>('/plugin/types').catch(() => null),
      ])
      setPlugins(extractList<Plugin>(pluginsResp))
      if (typesResp) {
        setPluginTypes(extractList<PluginType>(typesResp))
      }
    } catch (err) {
      toast({
        title: '加载失败',
        description: err instanceof Error ? err.message : '无法获取插件列表',
        variant: 'destructive',
      })
    } finally {
      setLoading(false)
    }
  }

  const handleUploadClick = () => {
    fileInputRef.current?.click()
  }

  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    if (!file.name.endsWith('.zip')) {
      toast({ title: '文件格式错误', description: '请上传 .zip 格式的插件包', variant: 'destructive' })
      return
    }
    setUploading(true)
    try {
      const token = localStorage.getItem(TOKEN_KEY)
      const formData = new FormData()
      formData.append('file', file)
      const res = await fetch('/api/v1/admin/plugin/upload', {
        method: 'POST',
        headers: {
          Authorization: token ? (token.startsWith('Bearer ') ? token : `Bearer ${token}`) : '',
        },
        body: formData,
      })
      if (!res.ok) {
        let msg = `上传失败: ${res.status}`
        try {
          const errData = await res.json()
          msg = errData.message || msg
        } catch {}
        throw new Error(msg)
      }
      toast({ title: '上传成功', description: `插件「${file.name}」已上传`, variant: 'success' })
      if (fileInputRef.current) fileInputRef.current.value = ''
      await loadData()
    } catch (err) {
      toast({
        title: '上传失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setUploading(false)
    }
  }

  const toggleEnable = async (p: Plugin) => {
    const id = p.id
    const isEnabled = toBool(p.enabled) || toBool(p.status)
    setActionLoading(id)
    try {
      if (isEnabled) {
        await xbAdminApi.post('/plugin/disable', { id })
        toast({ title: '已禁用', variant: 'success' })
      } else {
        await xbAdminApi.post('/plugin/enable', { id })
        toast({ title: '已启用', variant: 'success' })
      }
      await loadData()
    } catch (err) {
      toast({
        title: '操作失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setActionLoading(null)
    }
  }

  const installPlugin = async (p: Plugin) => {
    setActionLoading(p.id)
    try {
      await xbAdminApi.post('/plugin/install', { id: p.id })
      toast({ title: '安装成功', variant: 'success' })
      await loadData()
    } catch (err) {
      toast({
        title: '安装失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setActionLoading(null)
    }
  }

  const uninstallPlugin = async (p: Plugin) => {
    if (!window.confirm(`确定卸载插件「${p.name}」吗？插件配置数据可能丢失。`)) return
    setActionLoading(p.id)
    try {
      await xbAdminApi.post('/plugin/uninstall', { id: p.id })
      toast({ title: '已卸载', variant: 'success' })
      await loadData()
    } catch (err) {
      toast({
        title: '卸载失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setActionLoading(null)
    }
  }

  const upgradePlugin = async (p: Plugin) => {
    setActionLoading(p.id)
    try {
      await xbAdminApi.post('/plugin/upgrade', { id: p.id })
      toast({ title: '升级成功', variant: 'success' })
      await loadData()
    } catch (err) {
      toast({
        title: '升级失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setActionLoading(null)
    }
  }

  const openConfig = async (p: Plugin) => {
    setSelectedPlugin(p)
    setConfigOpen(true)
    setPluginConfig({})
    try {
      const resp = await xbAdminApi.post<unknown>('/plugin/getConfig', { id: p.id })
      const data = extractObject<Record<string, unknown>>(resp)
      const cfg = (data?.config as Record<string, unknown>) || (data?.data as Record<string, unknown>) || {}
      const stringCfg: Record<string, string> = {}
      Object.entries(cfg).forEach(([k, v]) => {
        stringCfg[k] = typeof v === 'string' ? v : v !== null && v !== undefined ? String(v) : ''
      })
      if (Object.keys(stringCfg).length === 0 && p.config) {
        Object.entries(p.config).forEach(([k, v]) => {
          stringCfg[k] = typeof v === 'string' ? v : v !== null && v !== undefined ? String(v) : ''
        })
      }
      setPluginConfig(stringCfg)
    } catch {
      if (p.config) {
        const stringCfg: Record<string, string> = {}
        Object.entries(p.config).forEach(([k, v]) => {
          stringCfg[k] = typeof v === 'string' ? v : v !== null && v !== undefined ? String(v) : ''
        })
        setPluginConfig(stringCfg)
      } else {
        setPluginConfig({})
      }
    }
  }

  const savePluginConfig = async () => {
    if (!selectedPlugin) return
    setConfigSaving(true)
    try {
      await xbAdminApi.post('/plugin/config', { id: selectedPlugin.id, config: pluginConfig })
      toast({ title: '配置已保存', variant: 'success' })
      setConfigOpen(false)
      await loadData()
    } catch (err) {
      toast({
        title: '保存失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setConfigSaving(false)
    }
  }

  const isInstalled = (p: Plugin): boolean => toBool(p.installed) !== false ? true : toBool(p.installed)
  const isEnabled = (p: Plugin): boolean => toBool(p.enabled) || toBool(p.status)
  const canUpgrade = (p: Plugin): boolean => toBool(p.is_upgrade)

  const filteredPlugins = typeFilter === 'all'
    ? plugins
    : plugins.filter((p) => p.type === typeFilter)

  const enabledCount = plugins.filter((p) => isEnabled(p)).length

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between flex-wrap gap-2">
        <div className="flex items-center gap-3">
          <h2 className="text-lg font-semibold text-zinc-100">插件管理</h2>
          <Badge variant="default" className="bg-indigo-600">
            已启用 {enabledCount}/{plugins.length}
          </Badge>
        </div>
        <div className="flex items-center gap-2">
          <input
            ref={fileInputRef}
            type="file"
            accept=".zip"
            className="hidden"
            onChange={handleFileChange}
          />
          <Button
            size="sm"
            className="bg-indigo-600 hover:bg-indigo-500"
            onClick={handleUploadClick}
            disabled={uploading}
            isLoading={uploading}
          >
            <Upload className="w-4 h-4 mr-1" />
            上传插件
          </Button>
        </div>
      </div>

      {pluginTypes.length > 0 && (
        <Card className="bg-zinc-900 border-zinc-800">
          <CardContent className="p-3">
            <Select
              value={typeFilter}
              onChange={(e) => setTypeFilter(e.target.value)}
              className="bg-zinc-800 border-zinc-700 text-zinc-100 w-full md:w-48"
            >
              <option value="all">全部类型</option>
              {pluginTypes.map((t) => (
                <option key={t.id} value={t.id}>{t.name}</option>
              ))}
            </Select>
          </CardContent>
        </Card>
      )}

      <Card className="bg-zinc-900 border-zinc-800 overflow-hidden">
        <CardContent className="p-0">
          {loading ? (
            <div className="p-4 grid grid-cols-1 md:grid-cols-2 gap-3">
              {[1, 2, 3, 4].map((i) => (
                <Skeleton key={i} className="h-36 w-full bg-zinc-800 rounded-lg" />
              ))}
            </div>
          ) : filteredPlugins.length === 0 ? (
            <EmptyState
              title="暂无插件"
              description="上传 .zip 插件包或从市场安装插件"
              className="py-12"
            />
          ) : (
            <div className="p-4 grid grid-cols-1 md:grid-cols-2 gap-3">
              {filteredPlugins.map((p) => {
                const installed = isInstalled(p)
                const enabled = isEnabled(p)
                const upgrade = canUpgrade(p)
                const busy = actionLoading === p.id
                return (
                  <div
                    key={String(p.id)}
                    className={`rounded-lg border p-4 transition-colors ${
                      enabled
                        ? 'border-emerald-800/40 bg-emerald-950/10'
                        : 'border-zinc-800 bg-zinc-950/30'
                    }`}
                  >
                    <div className="flex items-start justify-between mb-2">
                      <div className="flex items-center gap-2">
                        <div className={`p-2 rounded-lg ${enabled ? 'bg-emerald-600/20 text-emerald-400' : 'bg-zinc-800 text-zinc-400'}`}>
                          <Puzzle className="w-5 h-5" />
                        </div>
                        <div>
                          <div className="font-medium text-zinc-100 text-sm flex items-center gap-2">
                            {p.name}
                            {enabled && <Badge variant="success" className="text-xs">运行中</Badge>}
                            {!installed && <Badge variant="secondary" className="text-xs">未安装</Badge>}
                            {upgrade && <Badge variant="warning" className="text-xs">可升级</Badge>}
                          </div>
                          <div className="text-xs text-zinc-500">
                            v{p.version || '1.0.0'}
                            {p.author && ` · ${p.author}`}
                          </div>
                        </div>
                      </div>
                      {installed && (
                        <Switch
                          checked={enabled}
                          onChange={() => toggleEnable(p)}
                          disabled={busy}
                        />
                      )}
                    </div>

                    {p.description && (
                      <p className="text-xs text-zinc-400 mb-3 line-clamp-2">{p.description}</p>
                    )}

                    <div className="flex items-center justify-end gap-1 pt-2 border-t border-zinc-800">
                      {!installed ? (
                        <Button
                          variant="outline"
                          size="sm"
                          className="border-indigo-700 text-indigo-300 hover:bg-indigo-900/20 h-7 text-xs"
                          onClick={() => installPlugin(p)}
                          disabled={busy}
                          isLoading={busy}
                        >
                          <Download className="w-3 h-3 mr-1" />
                          安装
                        </Button>
                      ) : (
                        <>
                          {upgrade && (
                            <Button
                              variant="outline"
                              size="sm"
                              className="border-amber-700 text-amber-300 hover:bg-amber-900/20 h-7 text-xs"
                              onClick={() => upgradePlugin(p)}
                              disabled={busy}
                              isLoading={busy}
                            >
                              <ArrowUpCircle className="w-3 h-3 mr-1" />
                              升级
                            </Button>
                          )}
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-7 w-7 text-zinc-400"
                            onClick={() => openConfig(p)}
                            disabled={busy}
                            title="配置"
                          >
                            <Settings className="w-3.5 h-3.5" />
                          </Button>
                          {enabled ? (
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-7 w-7 text-amber-400"
                              onClick={() => toggleEnable(p)}
                              disabled={busy}
                              title="禁用"
                            >
                              <PowerOff className="w-3.5 h-3.5" />
                            </Button>
                          ) : (
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-7 w-7 text-emerald-400"
                              onClick={() => toggleEnable(p)}
                              disabled={busy}
                              title="启用"
                            >
                              <Power className="w-3.5 h-3.5" />
                            </Button>
                          )}
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-7 w-7 text-red-400 hover:text-red-300"
                            onClick={() => uninstallPlugin(p)}
                            disabled={busy}
                            title="卸载"
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </Button>
                        </>
                      )}
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </CardContent>
      </Card>

      <Dialog open={configOpen} onOpenChange={setConfigOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-lg max-h-[80vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Package className="w-5 h-5 text-zinc-400" />
              <span>插件配置 - {selectedPlugin?.name}</span>
            </DialogTitle>
          </DialogHeader>
          {Object.keys(pluginConfig).length === 0 ? (
            <div className="text-center py-8 text-sm text-zinc-500">该插件无可配置项</div>
          ) : (
            <div className="space-y-4 pt-1">
              {Object.entries(pluginConfig).map(([key, value]) => (
                <div key={key} className="space-y-2">
                  <Label htmlFor={`pcfg-${key}`} className="text-zinc-300 text-sm capitalize">
                    {key.replace(/_/g, ' ')}
                  </Label>
                  <Input
                    id={`pcfg-${key}`}
                    value={value}
                    onChange={(e) => setPluginConfig((prev) => ({ ...prev, [key]: e.target.value }))}
                    className="bg-zinc-800 border-zinc-700 text-zinc-100"
                  />
                </div>
              ))}
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfigOpen(false)} className="border-zinc-700 text-zinc-300">
              取消
            </Button>
            <Button
              className="bg-indigo-600 hover:bg-indigo-500"
              onClick={savePluginConfig}
              disabled={configSaving || Object.keys(pluginConfig).length === 0}
              isLoading={configSaving}
            >
              保存配置
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
