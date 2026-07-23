import { useState, useEffect, useRef } from 'react'
import { Save, RefreshCw, Mail } from 'lucide-react'
import {
  Card,
  CardContent,
  Button,
  Input,
  Label,
  Switch,
  Textarea,
  Select,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
  useToast,
  Skeleton,
  Separator,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@airport/ui'
import { api } from '@/lib/api'
import { EP } from '@/lib/endpoints'

interface SystemConfig {
  app_name?: string
  app_description?: string
  app_url?: string
  subscribe_url?: string
  is_https?: boolean | number
  try_plan_plan_id?: number | string
  try_plan_time?: number | string
  try_plan_reset_traffic?: boolean | number
  subscribe_path?: string
  subscribe_domain?: string
  subscribe_key?: string
  show_info_to_server?: boolean | number
  show_method?: number | string
  is_rand_sub?: boolean | number
  rand_sub_start?: number | string
  rand_sub_end?: number | string
  email_driver?: string
  smtp_host?: string
  smtp_username?: string
  smtp_password?: string
  smtp_port?: number | string
  smtp_encryption?: string
  email_from_name?: string
  email_from_address?: string
  [key: string]: unknown
}

// SMTP 配置通过专用接口 /admin/mail/smtp-config 管理（与扁平系统设置分离）
interface SmtpConfig {
  enabled: boolean
  host: string
  port: number
  username: string
  password: string
  from: string
  password_configured: boolean
}

// 旧的 SMTP 扁平键（从 system_settings 加载时跳过，改由专用接口管理）
const SMTP_LEGACY_KEYS = [
  'smtp_host', 'smtp_port', 'smtp_username', 'smtp_password',
  'smtp_encryption', 'email_from_name', 'email_from_address', 'email_driver',
]

function toBool(v: unknown): boolean {
  if (typeof v === 'boolean') return v
  if (typeof v === 'number') return v === 1
  if (typeof v === 'string') return v === '1' || v === 'true'
  return false
}

function toNum(v: unknown, def = 0): number {
  if (typeof v === 'number') return v
  if (typeof v === 'string') {
    const n = parseInt(v, 10)
    return isNaN(n) ? def : n
  }
  return def
}

export default function Settings() {
  const { toast } = useToast()
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [settings, setSettings] = useState<SystemConfig>({})
  const originalSettings = useRef<SystemConfig>({})
  const keyToGroup = useRef<Record<string, string>>({})
  const [activeTab, setActiveTab] = useState('general')

  const [testMailOpen, setTestMailOpen] = useState(false)
  const [testMailTo, setTestMailTo] = useState('')
  const [testMailSending, setTestMailSending] = useState(false)

  // SMTP 配置独立状态（不混入扁平 system_settings）
  const [smtpConfig, setSmtpConfig] = useState<SmtpConfig>({
    enabled: false, host: '', port: 465, username: '', password: '', from: '', password_configured: false,
  })
  const [savingSmtp, setSavingSmtp] = useState(false)

  useEffect(() => {
    loadSettings()
  }, [])

  const loadSettings = async () => {
    setLoading(true)
    try {
      const data = await api.get<Record<string, Record<string, unknown>>>(EP.SYSTEM_SETTINGS)
      const flat: SystemConfig = {}
      const groupMap: Record<string, string> = {}
      for (const [group, groupSettings] of Object.entries(data || {})) {
        if (groupSettings && typeof groupSettings === 'object') {
          for (const [key, value] of Object.entries(groupSettings)) {
            // 跳过旧的 SMTP 扁平键，改由专用接口 /admin/mail/smtp-config 管理
            if (SMTP_LEGACY_KEYS.includes(key)) continue
            flat[key as keyof SystemConfig] = value as SystemConfig[keyof SystemConfig]
            groupMap[key] = group
          }
        }
      }
      setSettings(flat)
      originalSettings.current = { ...flat }
      keyToGroup.current = groupMap

      // SMTP 配置通过专用接口加载（与扁平系统设置分离）
      try {
        const smtpResp = await api.get<SmtpConfig>(EP.MAIL_SMTP_CONFIG)
        setSmtpConfig(smtpResp)
      } catch {
        // SMTP 配置未初始化，使用默认值
      }
    } catch (err) {
      toast({
        title: '加载失败',
        description: err instanceof Error ? err.message : '无法获取系统配置',
        variant: 'destructive',
      })
    } finally {
      setLoading(false)
    }
  }

  const update = <K extends keyof SystemConfig>(key: K, value: SystemConfig[K]) => {
    setSettings((prev) => ({ ...prev, [key]: value }))
  }

  const handleSave = async () => {
    setSaving(true)
    try {
      const updates: Promise<unknown>[] = []
      for (const [key, value] of Object.entries(settings)) {
        const original = originalSettings.current[key]
        if (JSON.stringify(value) !== JSON.stringify(original)) {
          const group = keyToGroup.current[key] || 'site'
          updates.push(api.put(EP.SYSTEM_SETTING_UPDATE(group, key), { value }))
        }
      }
      if (updates.length > 0) {
        await Promise.all(updates)
      }
      originalSettings.current = { ...settings }
      toast({ title: '保存成功', description: '系统配置已更新', variant: 'success' })
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

  const regenerateKey = () => {
    const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789'
    let key = ''
    for (let i = 0; i < 32; i++) {
      key += chars.charAt(Math.floor(Math.random() * chars.length))
    }
    update('subscribe_key', key)
    toast({ title: '已生成新密钥', description: '请记得保存配置以生效', variant: 'default' })
  }

  // SMTP 配置独立保存（通过专用接口，保存后即时刷新内存，无需重启服务）
  const handleSaveSmtp = async () => {
    setSavingSmtp(true)
    try {
      const resp = await api.put<SmtpConfig>(EP.MAIL_SMTP_CONFIG, {
        enabled: smtpConfig.enabled,
        host: smtpConfig.host,
        port: smtpConfig.port,
        username: smtpConfig.username,
        password: smtpConfig.password, // 留空则后端保持现有密码不变
        from: smtpConfig.from,
      })
      setSmtpConfig(resp)
      toast({ title: '保存成功', description: 'SMTP 配置已更新并即时生效', variant: 'success' })
    } catch (err) {
      toast({
        title: '保存失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setSavingSmtp(false)
    }
  }

  const handleTestMail = async () => {
    if (!testMailTo.trim() || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(testMailTo)) {
      toast({ title: '校验失败', description: '请输入有效的邮箱地址', variant: 'destructive' })
      return
    }
    setTestMailSending(true)
    try {
      await api.post(EP.MAIL_TEST_SEND, {
        to: testMailTo.trim(),
        subject: 'YunDu 测试邮件',
        body: '<p>这是一封来自 YunDu 系统的测试邮件，收到此邮件说明 SMTP 配置正常。</p>',
      })
      toast({ title: '发送成功', description: `测试邮件已发送至 ${testMailTo}`, variant: 'success' })
      setTestMailOpen(false)
      setTestMailTo('')
    } catch (err) {
      toast({
        title: '发送失败',
        description: err instanceof Error ? err.message : '请检查 SMTP 配置是否正确',
        variant: 'destructive',
      })
    } finally {
      setTestMailSending(false)
    }
  }

  if (loading) {
    return (
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <Skeleton className="h-7 w-32 bg-zinc-800 rounded" />
          <Skeleton className="h-9 w-24 bg-zinc-800 rounded" />
        </div>
        <Card className="bg-zinc-900 border-zinc-800">
          <CardContent className="p-4 space-y-4">
            <Skeleton className="h-10 w-full bg-zinc-800 rounded" />
            {[1, 2, 3, 4, 5].map((i) => (
              <Skeleton key={i} className="h-12 w-full bg-zinc-800 rounded" />
            ))}
          </CardContent>
        </Card>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-zinc-100">系统设置</h2>
        <Button
          size="sm"
          className="bg-indigo-600 hover:bg-indigo-500"
          onClick={handleSave}
          disabled={saving}
          isLoading={saving}
        >
          <Save className="w-4 h-4 mr-1" />
          保存设置
        </Button>
      </div>

      <Card className="bg-zinc-900 border-zinc-800">
        <CardContent className="p-0">
          <Tabs value={activeTab} onValueChange={setActiveTab} className="w-full">
            <TabsList className="w-full justify-start rounded-none border-b border-zinc-800 bg-transparent p-0 h-auto overflow-x-auto">
              {[
                { v: 'general', l: '基础设置' },
                { v: 'subscribe', l: '订阅设置' },
                { v: 'email', l: '邮件设置' },
                { v: 'payment', l: '支付设置' },
                { v: 'other', l: '其他设置' },
              ].map((t) => (
                <TabsTrigger
                  key={t.v}
                  value={t.v}
                  className="rounded-none border-b-2 border-transparent data-[state=active]:border-indigo-500 data-[state=active]:bg-transparent data-[state=active]:text-indigo-400 px-4 py-3 text-sm text-zinc-400"
                >
                  {t.l}
                </TabsTrigger>
              ))}
            </TabsList>

            <TabsContent value="general" className="p-4 space-y-5 mt-0">
              <div className="space-y-2">
                <Label htmlFor="app_name" className="text-zinc-300 text-sm">站点名称</Label>
                <Input
                  id="app_name"
                  value={settings.app_name || ''}
                  onChange={(e) => update('app_name', e.target.value)}
                  placeholder="XBoard"
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="app_description" className="text-zinc-300 text-sm">站点描述</Label>
                <Textarea
                  id="app_description"
                  value={settings.app_description || ''}
                  onChange={(e) => update('app_description', e.target.value)}
                  placeholder="站点描述信息"
                  className="bg-zinc-800 border-zinc-700 text-zinc-100 min-h-[80px]"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="app_url" className="text-zinc-300 text-sm">站点地址</Label>
                <Input
                  id="app_url"
                  value={settings.app_url || ''}
                  onChange={(e) => update('app_url', e.target.value)}
                  placeholder="https://example.com"
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="subscribe_url" className="text-zinc-300 text-sm">订阅地址</Label>
                <Input
                  id="subscribe_url"
                  value={settings.subscribe_url || ''}
                  onChange={(e) => update('subscribe_url', e.target.value)}
                  placeholder="https://sub.example.com"
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
              </div>

              <Separator className="bg-zinc-800" />

              <div className="flex items-center justify-between">
                <div>
                  <Label className="text-zinc-300 text-sm">启用 HTTPS</Label>
                  <p className="text-xs text-zinc-500 mt-0.5">强制使用 HTTPS 协议访问</p>
                </div>
                <Switch
                  checked={toBool(settings.is_https)}
                  onChange={(e) => update('is_https', e.target.checked ? 1 : 0)}
                />
              </div>

              <Separator className="bg-zinc-800" />

              <div className="space-y-2">
                <Label htmlFor="try_plan_plan_id" className="text-zinc-300 text-sm">试用套餐 ID</Label>
                <Input
                  id="try_plan_plan_id"
                  type="number"
                  value={settings.try_plan_plan_id ?? ''}
                  onChange={(e) => update('try_plan_plan_id', e.target.value)}
                  placeholder="0"
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
                <p className="text-xs text-zinc-500">新用户注册时自动分配的套餐，0 为不分配</p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="try_plan_time" className="text-zinc-300 text-sm">试用时长（小时）</Label>
                <Input
                  id="try_plan_time"
                  type="number"
                  value={settings.try_plan_time ?? ''}
                  onChange={(e) => update('try_plan_time', e.target.value)}
                  placeholder="1"
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
              </div>

              <div className="flex items-center justify-between">
                <div>
                  <Label className="text-zinc-300 text-sm">试用重置流量</Label>
                  <p className="text-xs text-zinc-500 mt-0.5">试用到期后重置用户流量</p>
                </div>
                <Switch
                  checked={toBool(settings.try_plan_reset_traffic)}
                  onChange={(e) => update('try_plan_reset_traffic', e.target.checked ? 1 : 0)}
                />
              </div>
            </TabsContent>

            <TabsContent value="subscribe" className="p-4 space-y-5 mt-0">
              <div className="space-y-2">
                <Label htmlFor="sub_url" className="text-zinc-300 text-sm">订阅地址</Label>
                <Input
                  id="sub_url"
                  value={settings.subscribe_url || ''}
                  onChange={(e) => update('subscribe_url', e.target.value)}
                  placeholder="https://sub.example.com"
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="subscribe_path" className="text-zinc-300 text-sm">订阅路径</Label>
                <Input
                  id="subscribe_path"
                  value={settings.subscribe_path || ''}
                  onChange={(e) => update('subscribe_path', e.target.value)}
                  placeholder="/api/v1/client/subscribe"
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="subscribe_domain" className="text-zinc-300 text-sm">订阅域名</Label>
                <Input
                  id="subscribe_domain"
                  value={settings.subscribe_domain || ''}
                  onChange={(e) => update('subscribe_domain', e.target.value)}
                  placeholder="sub.example.com"
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="subscribe_key" className="text-zinc-300 text-sm">订阅密钥</Label>
                <div className="flex gap-2">
                  <Input
                    id="subscribe_key"
                    value={settings.subscribe_key || ''}
                    onChange={(e) => update('subscribe_key', e.target.value)}
                    className="bg-zinc-800 border-zinc-700 text-zinc-100 font-mono text-xs"
                  />
                  <Button
                    variant="outline"
                    size="sm"
                    className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 shrink-0"
                    onClick={regenerateKey}
                  >
                    <RefreshCw className="w-4 h-4 mr-1" />
                    重新生成
                  </Button>
                </div>
                <p className="text-xs text-zinc-500">用于生成订阅链接的加密密钥</p>
              </div>

              <Separator className="bg-zinc-800" />

              <div className="flex items-center justify-between">
                <div>
                  <Label className="text-zinc-300 text-sm">显示节点信息</Label>
                  <p className="text-xs text-zinc-500 mt-0.5">在订阅中显示节点信息给客户端</p>
                </div>
                <Switch
                  checked={toBool(settings.show_info_to_server)}
                  onChange={(e) => update('show_info_to_server', e.target.checked ? 1 : 0)}
                />
              </div>

              <div className="space-y-2">
                <Label className="text-zinc-300 text-sm">订阅显示方式</Label>
                <Select
                  value={String(settings.show_method ?? 0)}
                  onChange={(e) => update('show_method', toNum(e.target.value))}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                >
                  <option value="0">仅显示节点</option>
                  <option value="1">显示节点+剩余流量</option>
                  <option value="2">显示节点+到期时间</option>
                </Select>
              </div>

              <Separator className="bg-zinc-800" />

              <div className="flex items-center justify-between">
                <div>
                  <Label className="text-zinc-300 text-sm">随机订阅</Label>
                  <p className="text-xs text-zinc-500 mt-0.5">随机返回订阅节点</p>
                </div>
                <Switch
                  checked={toBool(settings.is_rand_sub)}
                  onChange={(e) => update('is_rand_sub', e.target.checked ? 1 : 0)}
                />
              </div>

              {toBool(settings.is_rand_sub) && (
                <div className="grid grid-cols-2 gap-3">
                  <div className="space-y-2">
                    <Label htmlFor="rand_sub_start" className="text-zinc-300 text-sm">随机起始位置</Label>
                    <Input
                      id="rand_sub_start"
                      type="number"
                      value={settings.rand_sub_start ?? ''}
                      onChange={(e) => update('rand_sub_start', e.target.value)}
                      placeholder="0"
                      className="bg-zinc-800 border-zinc-700 text-zinc-100"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="rand_sub_end" className="text-zinc-300 text-sm">随机结束位置</Label>
                    <Input
                      id="rand_sub_end"
                      type="number"
                      value={settings.rand_sub_end ?? ''}
                      onChange={(e) => update('rand_sub_end', e.target.value)}
                      placeholder="10"
                      className="bg-zinc-800 border-zinc-700 text-zinc-100"
                    />
                  </div>
                </div>
              )}
            </TabsContent>

            <TabsContent value="email" className="p-4 space-y-5 mt-0">
              {/* 启用/禁用开关 */}
              <div className="flex items-center justify-between">
                <div>
                  <Label className="text-zinc-300 text-sm">启用邮件服务</Label>
                  <p className="text-xs text-zinc-500 mt-0.5">开启后注册验证码、密码重置等邮件功能将可用</p>
                </div>
                <Switch
                  checked={smtpConfig.enabled}
                  onChange={(e) => setSmtpConfig((prev) => ({ ...prev, enabled: e.target.checked }))}
                />
              </div>

              <Separator className="bg-zinc-800" />

              {/* SMTP 服务器 */}
              <div className="space-y-2">
                <Label htmlFor="smtp_host" className="text-zinc-300 text-sm">SMTP 服务器</Label>
                <Input
                  id="smtp_host"
                  value={smtpConfig.host}
                  onChange={(e) => setSmtpConfig((prev) => ({ ...prev, host: e.target.value }))}
                  placeholder="smtp.example.com"
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
              </div>

              {/* 端口 + 加密方式提示 */}
              <div className="space-y-2">
                <Label htmlFor="smtp_port" className="text-zinc-300 text-sm">SMTP 端口</Label>
                <Input
                  id="smtp_port"
                  type="number"
                  value={smtpConfig.port}
                  onChange={(e) => setSmtpConfig((prev) => ({ ...prev, port: parseInt(e.target.value) || 0 }))}
                  placeholder="465"
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
                <p className="text-xs text-zinc-500">
                  465 = SSL（隐式 TLS）&nbsp;|&nbsp;587 = STARTTLS&nbsp;|&nbsp;25 = 无加密（不推荐）
                </p>
              </div>

              {/* 用户名 */}
              <div className="space-y-2">
                <Label htmlFor="smtp_username" className="text-zinc-300 text-sm">SMTP 用户名</Label>
                <Input
                  id="smtp_username"
                  value={smtpConfig.username}
                  onChange={(e) => setSmtpConfig((prev) => ({ ...prev, username: e.target.value }))}
                  placeholder="noreply@example.com"
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
              </div>

              {/* 密码 */}
              <div className="space-y-2">
                <Label htmlFor="smtp_password" className="text-zinc-300 text-sm">SMTP 密码</Label>
                <Input
                  id="smtp_password"
                  type="password"
                  value={smtpConfig.password}
                  onChange={(e) => setSmtpConfig((prev) => ({ ...prev, password: e.target.value }))}
                  placeholder={smtpConfig.password_configured ? '已设置（留空保持不变）' : '请输入密码'}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
                {smtpConfig.password_configured && (
                  <p className="text-xs text-zinc-500">密码已配置，留空保存将保持现有密码不变</p>
                )}
              </div>

              <Separator className="bg-zinc-800" />

              {/* 发件人地址 */}
              <div className="space-y-2">
                <Label htmlFor="email_from" className="text-zinc-300 text-sm">发件人地址</Label>
                <Input
                  id="email_from"
                  value={smtpConfig.from}
                  onChange={(e) => setSmtpConfig((prev) => ({ ...prev, from: e.target.value }))}
                  placeholder="noreply@example.com"
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
                <p className="text-xs text-zinc-500">邮件显示的发件人地址，留空则使用 SMTP 用户名</p>
              </div>

              {/* 操作按钮 */}
              <div className="flex items-center gap-3 pt-2">
                <Button
                  size="sm"
                  className="bg-indigo-600 hover:bg-indigo-500"
                  onClick={handleSaveSmtp}
                  disabled={savingSmtp}
                  isLoading={savingSmtp}
                >
                  <Save className="w-4 h-4 mr-1" />
                  保存邮件配置
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  className="border-zinc-700 text-zinc-300 hover:bg-zinc-800"
                  onClick={() => setTestMailOpen(true)}
                >
                  <Mail className="w-4 h-4 mr-1" />
                  发送测试邮件
                </Button>
              </div>
              <p className="text-xs text-zinc-500">
                提示：邮件配置独立保存，保存后即时生效无需重启服务。顶部「保存设置」按钮仅保存其他系统配置。
              </p>
            </TabsContent>

            <TabsContent value="payment" className="p-4 space-y-5 mt-0">
              <div className="rounded-lg border border-zinc-800 bg-zinc-950/30 p-4">
                <p className="text-sm text-zinc-400">支付设置请在「支付管理」页面配置各支付渠道参数。</p>
              </div>
            </TabsContent>

            <TabsContent value="other" className="p-4 space-y-5 mt-0">
              <div className="rounded-lg border border-zinc-800 bg-zinc-950/30 p-4">
                <p className="text-sm text-zinc-400">其他系统配置项将在后续版本中提供。</p>
              </div>
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>

      <Dialog open={testMailOpen} onOpenChange={setTestMailOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Mail className="w-5 h-5 text-zinc-400" />
              发送测试邮件
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-2 pt-1">
            <Label htmlFor="testmail" className="text-zinc-300 text-sm">收件人邮箱</Label>
            <Input
              id="testmail"
              type="email"
              value={testMailTo}
              onChange={(e) => setTestMailTo(e.target.value)}
              placeholder="test@example.com"
              className="bg-zinc-800 border-zinc-700 text-zinc-100"
            />
            <p className="text-xs text-zinc-500">请确保已正确配置邮件参数后再发送测试邮件。</p>
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setTestMailOpen(false)}
              className="border-zinc-700 text-zinc-300"
            >
              取消
            </Button>
            <Button
              className="bg-indigo-600 hover:bg-indigo-500"
              onClick={handleTestMail}
              disabled={testMailSending}
              isLoading={testMailSending}
            >
              发送
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
