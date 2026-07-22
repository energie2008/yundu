import { useState, useEffect, useRef } from 'react'
import { Palette, Upload, Trash2, Settings, Check, Package } from 'lucide-react'
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
import { xbAdminApi } from '@/lib/api'

interface Theme {
  name: string
  version?: string
  description?: string
  author?: string
  status?: string
  is_active?: boolean | number
  config?: Record<string, unknown>
  [key: string]: unknown
}

function extractList<T>(resp: unknown): T[] {
  if (Array.isArray(resp)) return resp as T[]
  if (!resp || typeof resp !== 'object') return []
  const obj = resp as Record<string, unknown>
  const dataField = obj.data
  if (dataField && typeof dataField === 'object') {
    if (Array.isArray(dataField)) return dataField as T[]
    const dataObj = dataField as Record<string, unknown>
    if (Array.isArray(dataObj.themes)) return dataObj.themes as T[]
    if (Array.isArray(dataObj.data)) return dataObj.data as T[]
    if (Array.isArray(dataObj.items)) return dataObj.items as T[]
    if (Array.isArray(dataObj.list)) return dataObj.list as T[]
  }
  if (Array.isArray(obj.themes)) return obj.themes as T[]
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

export default function SystemTheme() {
  return <ComingSoon title="主题设置" reason="该模块后端 API 正在迁移到 YunDu Go 微服务，暂不可用。" />

  const { toast } = useToast()
  const [loading, setLoading] = useState(true)
  const [themes, setThemes] = useState<Theme[]>([])
  const [uploading, setUploading] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const [configOpen, setConfigOpen] = useState(false)
  const [configLoading, setConfigLoading] = useState(false)
  const [configSaving, setConfigSaving] = useState(false)
  const [selectedTheme, setSelectedTheme] = useState<Theme | null>(null)
  const [themeConfig, setThemeConfig] = useState<Record<string, string>>({})

  useEffect(() => {
    loadThemes()
  }, [])

  const loadThemes = async () => {
    setLoading(true)
    try {
      const resp = await xbAdminApi.get<unknown>('/theme/getThemes')
      setThemes(extractList<Theme>(resp))
    } catch (err) {
      toast({
        title: '加载失败',
        description: err instanceof Error ? err.message : '无法获取主题列表',
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
      toast({ title: '文件格式错误', description: '请上传 .zip 格式的主题包', variant: 'destructive' })
      return
    }
    setUploading(true)
    try {
      const token = localStorage.getItem(TOKEN_KEY)
      const formData = new FormData()
      formData.append('file', file)
      const res = await fetch('/api/v1/admin/theme/upload', {
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
      toast({ title: '上传成功', description: `主题「${file.name}」已上传`, variant: 'success' })
      if (fileInputRef.current) fileInputRef.current.value = ''
      await loadThemes()
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

  const deleteTheme = async (t: Theme) => {
    if (toBool(t.is_active)) {
      toast({ title: '无法删除', description: '当前激活的主题不能删除，请先切换到其他主题', variant: 'destructive' })
      return
    }
    if (!window.confirm(`确定删除主题「${t.name}」吗？`)) return
    try {
      await xbAdminApi.post('/theme/delete', { name: t.name })
      toast({ title: '已删除', variant: 'success' })
      await loadThemes()
    } catch (err) {
      toast({
        title: '删除失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    }
  }

  const openConfig = async (t: Theme) => {
    setSelectedTheme(t)
    setConfigOpen(true)
    setConfigLoading(true)
    setThemeConfig({})
    try {
      const resp = await xbAdminApi.post<unknown>('/theme/getThemeConfig', { name: t.name })
      const data = extractObject<Record<string, unknown>>(resp)
      const cfg = (data?.config as Record<string, unknown>) || data || {}
      const stringCfg: Record<string, string> = {}
      Object.entries(cfg).forEach(([k, v]) => {
        stringCfg[k] = typeof v === 'string' ? v : v !== null && v !== undefined ? String(v) : ''
      })
      setThemeConfig(stringCfg)
    } catch (err) {
      setThemeConfig({})
    } finally {
      setConfigLoading(false)
    }
  }

  const saveThemeConfig = async () => {
    if (!selectedTheme) return
    setConfigSaving(true)
    try {
      await xbAdminApi.post('/theme/saveThemeConfig', { name: selectedTheme.name, config: themeConfig })
      toast({ title: '配置已保存', variant: 'success' })
      setConfigOpen(false)
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

  const activeTheme = themes.find((t) => toBool(t.is_active))

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <h2 className="text-lg font-semibold text-zinc-100">主题管理</h2>
          {activeTheme && (
            <Badge variant="success" className="flex items-center gap-1">
              <Check className="w-3 h-3" />
              当前：{activeTheme?.name}
            </Badge>
          )}
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
            上传主题
          </Button>
        </div>
      </div>

      <Card className="bg-zinc-900 border-zinc-800 overflow-hidden">
        <CardContent className="p-0">
          {loading ? (
            <div className="p-4 grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
              {[1, 2, 3].map((i) => (
                <Skeleton key={i} className="h-40 w-full bg-zinc-800 rounded-lg" />
              ))}
            </div>
          ) : themes.length === 0 ? (
            <EmptyState
              title="暂无主题"
              description="上传 .zip 主题包开始使用"
              className="py-12"
            />
          ) : (
            <div className="p-4 grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
              {themes.map((t) => {
                const isActive = toBool(t.is_active)
                return (
                  <div
                    key={t.name}
                    className={`rounded-lg border p-4 transition-colors ${
                      isActive
                        ? 'border-indigo-600/50 bg-indigo-950/20'
                        : 'border-zinc-800 bg-zinc-950/30 hover:border-zinc-700'
                    }`}
                  >
                    <div className="flex items-start justify-between mb-3">
                      <div className="flex items-center gap-2">
                        <div className={`p-2 rounded-lg ${isActive ? 'bg-indigo-600/20 text-indigo-400' : 'bg-zinc-800 text-zinc-400'}`}>
                          <Palette className="w-5 h-5" />
                        </div>
                        <div>
                          <div className="font-medium text-zinc-100 text-sm flex items-center gap-2">
                            {t.name}
                            {isActive && (
                              <Badge variant="success" className="text-xs">激活</Badge>
                            )}
                          </div>
                          <div className="text-xs text-zinc-500">v{t.version || '1.0.0'}</div>
                        </div>
                      </div>
                    </div>

                    {t.description && (
                      <p className="text-xs text-zinc-400 mb-3 line-clamp-2">{t.description}</p>
                    )}

                    <div className="flex items-center justify-between pt-2 border-t border-zinc-800">
                      <div className="text-xs text-zinc-500">
                        {t.author && <span>作者：{t.author}</span>}
                      </div>
                      <div className="flex items-center gap-1">
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-8 w-8 text-zinc-400"
                          onClick={() => openConfig(t)}
                          title="主题配置"
                        >
                          <Settings className="w-4 h-4" />
                        </Button>
                        {!isActive && (
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-8 w-8 text-red-400 hover:text-red-300"
                            onClick={() => deleteTheme(t)}
                            title="删除"
                          >
                            <Trash2 className="w-4 h-4" />
                          </Button>
                        )}
                      </div>
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
              <span>主题配置 - {selectedTheme?.name}</span>
            </DialogTitle>
          </DialogHeader>
          {configLoading ? (
            <div className="space-y-3 py-4">
              <Skeleton className="h-10 w-full bg-zinc-800 rounded" />
              <Skeleton className="h-10 w-full bg-zinc-800 rounded" />
              <Skeleton className="h-10 w-full bg-zinc-800 rounded" />
            </div>
          ) : Object.keys(themeConfig).length === 0 ? (
            <div className="text-center py-8 text-sm text-zinc-500">该主题无可配置项</div>
          ) : (
            <div className="space-y-4 pt-1">
              {Object.entries(themeConfig).map(([key, value]) => (
                <div key={key} className="space-y-2">
                  <Label htmlFor={`cfg-${key}`} className="text-zinc-300 text-sm capitalize">
                    {key.replace(/_/g, ' ')}
                  </Label>
                  <Input
                    id={`cfg-${key}`}
                    value={value}
                    onChange={(e) => setThemeConfig((prev) => ({ ...prev, [key]: e.target.value }))}
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
              onClick={saveThemeConfig}
              disabled={configSaving || Object.keys(themeConfig).length === 0}
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
