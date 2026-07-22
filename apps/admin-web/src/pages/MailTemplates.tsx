import { useState, useEffect, useCallback, useMemo } from 'react'
import {
  Mail,
  Pencil,
  Search,
  RefreshCw,
  Send,
  RotateCw,
  Eye,
  Code2,
  ChevronLeft,
  ChevronRight,
} from 'lucide-react'
import {
  Card,
  CardContent,
  Button,
  Input,
  Badge,
  Skeleton,
  EmptyState,
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

// ===== 类型定义 =====
// 对齐后端 model.MailTemplateResponse
interface MailTemplate {
  id: string
  // 模板标识码（slug），如 verify_email / reset_password / payment_success ...
  name: string
  // 邮件主题（支持变量占位符）
  subject: string
  // 邮件正文（HTML，支持变量占位符）
  body: string
  // 内置模板不可删除
  is_builtin: boolean
  enabled: boolean
  updated_at?: string
  created_at?: string
}

// 内置模板的中文名映射，便于在列表中展示
const TEMPLATE_NAME_LABELS: Record<string, string> = {
  verify_email: '邮箱验证',
  reset_password: '重置密码',
  payment_success: '支付成功',
  ticket_reply: '工单回复',
  subscription_expired: '订阅过期',
  traffic_warning: '流量预警',
}

const PAGE_SIZE = 10

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

export default function MailTemplates() {
  const { toast } = useToast()
  const [templates, setTemplates] = useState<MailTemplate[]>([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)

  // 编辑对话框
  const [editOpen, setEditOpen] = useState(false)
  const [editing, setEditing] = useState<MailTemplate | null>(null)
  const [formSubject, setFormSubject] = useState('')
  const [formBody, setFormBody] = useState('')
  const [saving, setSaving] = useState(false)

  // 测试发送对话框
  const [testOpen, setTestOpen] = useState(false)
  const [testing, setTesting] = useState<MailTemplate | null>(null)
  const [testEmail, setTestEmail] = useState('')
  const [sending, setSending] = useState(false)

  // 预览对话框
  const [previewOpen, setPreviewOpen] = useState(false)
  const [previewing, setPreviewing] = useState<MailTemplate | null>(null)

  const [reloading, setReloading] = useState(false)

  // ===== 拉取列表 =====
  const fetchTemplates = useCallback(async () => {
    try {
      setLoading(true)
      const data = await api.get<unknown>(EP.MAIL_TEMPLATES)
      setTemplates(normalizeList<MailTemplate>(data))
    } catch (err) {
      toast({
        title: '加载失败',
        description: err instanceof ApiError ? err.message : '无法获取邮件模板列表',
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

  const filtered = useMemo(() => {
    const kw = search.trim().toLowerCase()
    if (!kw) return templates
    return templates.filter(
      (t) =>
        t.name.toLowerCase().includes(kw) ||
        (t.subject || '').toLowerCase().includes(kw) ||
        (TEMPLATE_NAME_LABELS[t.name] || '').toLowerCase().includes(kw),
    )
  }, [templates, search])

  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE))
  const currentPage = Math.min(page, totalPages)
  const paged = filtered.slice((currentPage - 1) * PAGE_SIZE, currentPage * PAGE_SIZE)

  // ===== 编辑 =====
  const openEdit = (tpl: MailTemplate) => {
    setEditing(tpl)
    setFormSubject(tpl.subject || '')
    setFormBody(tpl.body || '')
    setEditOpen(true)
  }

  const closeEdit = () => {
    setEditOpen(false)
    setEditing(null)
    setFormSubject('')
    setFormBody('')
  }

  const handleSubmit = async () => {
    if (!editing) return
    if (!formSubject.trim()) {
      toast({ title: '请填写邮件主题', variant: 'destructive' })
      return
    }
    if (!formBody.trim()) {
      toast({ title: '请填写邮件正文', variant: 'destructive' })
      return
    }
    try {
      setSaving(true)
      await api.put(EP.MAIL_TEMPLATE_DETAIL(editing.id), {
        subject: formSubject,
        body: formBody,
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

  // ===== 测试发送 =====
  const openTest = (tpl: MailTemplate) => {
    setTesting(tpl)
    setTestEmail('')
    setTestOpen(true)
  }

  const closeTest = () => {
    setTestOpen(false)
    setTesting(null)
    setTestEmail('')
  }

  const handleTestSend = async () => {
    if (!testing) return
    if (!testEmail.trim()) {
      toast({ title: '请输入收件邮箱', variant: 'destructive' })
      return
    }
    // 简单邮箱格式校验
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(testEmail.trim())) {
      toast({ title: '邮箱格式不正确', variant: 'destructive' })
      return
    }
    try {
      setSending(true)
      await api.post(EP.MAIL_TEST_SEND, {
        to: testEmail.trim(),
        subject: testing.subject,
        body: testing.body,
      })
      toast({
        title: '测试邮件已发送',
        description: `已发送至 ${testEmail.trim()}`,
        variant: 'success',
      })
      closeTest()
    } catch (err) {
      toast({
        title: '发送失败',
        description: err instanceof ApiError ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setSending(false)
    }
  }

  // ===== 重载缓存 =====
  const handleReload = async () => {
    try {
      setReloading(true)
      await api.post(EP.MAIL_TEMPLATES_RELOAD)
      toast({ title: '模板缓存已重载', description: '已从数据库重新加载所有模板', variant: 'success' })
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
  const openPreview = (tpl: MailTemplate) => {
    setPreviewing(tpl)
    setPreviewOpen(true)
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-zinc-100 flex items-center gap-2">
            <Mail className="w-5 h-5 text-sky-400" />
            邮件模板管理
          </h1>
          <p className="text-sm text-zinc-500 mt-1">
            管理系统邮件模板（验证码、重置密码、支付成功等），支持编辑主题与正文、测试发送
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
              placeholder="搜索标识码、主题或名称..."
              value={search}
              onChange={(e) => {
                setSearch(e.target.value)
                setPage(1)
              }}
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
          ) : paged.length === 0 ? (
            <EmptyState
              title="暂无邮件模板"
              description={search ? '没有匹配的模板，试试调整搜索条件' : '后端尚未预置任何邮件模板'}
              className="py-12"
            />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-zinc-800">
                    <th className="text-left p-3 text-xs font-medium text-zinc-400">标识码</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400">名称</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400">主题</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400 hidden md:table-cell">类型</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400 hidden lg:table-cell">最后更新</th>
                    <th className="text-left p-3 text-xs font-medium text-zinc-400">状态</th>
                    <th className="text-right p-3 text-xs font-medium text-zinc-400">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {paged.map((tpl) => (
                    <tr
                      key={tpl.id}
                      className="border-b border-zinc-800 last:border-0 hover:bg-zinc-800/50 transition-colors"
                    >
                      <td className="p-3">
                        <code className="text-sm font-mono text-sky-400 bg-sky-950/50 px-2 py-0.5 rounded">
                          {tpl.name}
                        </code>
                      </td>
                      <td className="p-3 text-sm text-zinc-200">
                        {TEMPLATE_NAME_LABELS[tpl.name] || '-'}
                      </td>
                      <td className="p-3 text-sm text-zinc-300 max-w-xs truncate" title={tpl.subject}>
                        {tpl.subject || <span className="text-zinc-600">（空）</span>}
                      </td>
                      <td className="p-3 hidden md:table-cell">
                        {tpl.is_builtin ? (
                          <Badge variant="outline" className="bg-violet-900/40 text-violet-300 border-violet-800/50 text-xs">
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
                        {tpl.enabled ? (
                          <Badge variant="outline" className="bg-emerald-900/40 text-emerald-300 border-emerald-800/50 text-xs">
                            启用
                          </Badge>
                        ) : (
                          <Badge variant="outline" className="bg-zinc-800 text-zinc-500 border-zinc-700 text-xs">
                            禁用
                          </Badge>
                        )}
                      </td>
                      <td className="p-3">
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-8 w-8 p-0"
                            onClick={() => openPreview(tpl)}
                            title="预览"
                          >
                            <Eye className="w-4 h-4 text-zinc-400" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-8 w-8 p-0"
                            onClick={() => openTest(tpl)}
                            title="测试发送"
                          >
                            <Send className="w-4 h-4 text-emerald-400" />
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

      {/* 分页 */}
      {!loading && filtered.length > PAGE_SIZE && (
        <div className="flex items-center justify-between text-sm text-zinc-400">
          <span>
            共 {filtered.length} 个模板，第 {currentPage}/{totalPages} 页
          </span>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 h-8 w-8 p-0"
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              disabled={currentPage <= 1}
            >
              <ChevronLeft className="w-4 h-4" />
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 h-8 w-8 p-0"
              onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
              disabled={currentPage >= totalPages}
            >
              <ChevronRight className="w-4 h-4" />
            </Button>
          </div>
        </div>
      )}

      {/* 编辑对话框 */}
      <Dialog open={editOpen} onOpenChange={setEditOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-3xl max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>
              编辑邮件模板
              {editing && (
                <span className="ml-2 text-sm font-normal text-zinc-500">
                  ({TEMPLATE_NAME_LABELS[editing.name] || editing.name})
                </span>
              )}
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label className="text-zinc-300 text-sm">标识码</Label>
              <Input
                value={editing?.name || ''}
                disabled
                className="bg-zinc-800 border-zinc-700 text-zinc-500 font-mono"
              />
              <p className="text-xs text-zinc-500">标识码不可修改，由系统预置</p>
            </div>
            <div className="space-y-2">
              <Label className="text-zinc-300 text-sm">邮件主题 *</Label>
              <Input
                value={formSubject}
                onChange={(e) => setFormSubject(e.target.value)}
                placeholder="邮件主题，支持 {{.Var}} 变量占位符"
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              />
            </div>
            <div className="space-y-2">
              <Label className="text-zinc-300 text-sm flex items-center gap-1">
                <Code2 className="w-3.5 h-3.5" />
                邮件正文（HTML）*
              </Label>
              <textarea
                value={formBody}
                onChange={(e) => setFormBody(e.target.value)}
                placeholder="<html><body>邮件正文，支持变量占位符...</body></html>"
                spellCheck={false}
                className="w-full bg-zinc-950 border border-zinc-700 text-zinc-300 font-mono text-xs leading-6 p-3 rounded-md resize-y focus:outline-none min-h-[280px]"
              />
              <p className="text-xs text-zinc-500">
                支持 HTML 语法与变量占位符，编辑后点击保存将立即生效
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
              className="bg-sky-600 hover:bg-sky-500"
              onClick={handleSubmit}
              disabled={saving}
            >
              {saving ? '保存中...' : '保存'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 测试发送对话框 */}
      <Dialog open={testOpen} onOpenChange={setTestOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-md">
          <DialogHeader>
            <DialogTitle>测试发送邮件</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label className="text-zinc-300 text-sm">收件邮箱 *</Label>
              <Input
                type="email"
                value={testEmail}
                onChange={(e) => setTestEmail(e.target.value)}
                placeholder="example@domain.com"
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              />
            </div>
            {testing && (
              <div className="text-xs text-zinc-500 bg-zinc-800/50 border border-zinc-800 rounded-md p-3 space-y-1">
                <div>将使用以下内容发送测试邮件：</div>
                <div className="text-zinc-400">模板: <span className="font-mono text-sky-400">{testing.name}</span></div>
                <div className="text-zinc-400 truncate">主题: {testing.subject || '（空）'}</div>
              </div>
            )}
            <p className="text-xs text-amber-500/80">
              测试邮件将使用该模板当前的 subject 和 body 发送，便于预览实际渲染效果
            </p>
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              className="border-zinc-700 text-zinc-300 hover:bg-zinc-800"
              onClick={closeTest}
            >
              取消
            </Button>
            <Button
              className="bg-emerald-600 hover:bg-emerald-500"
              onClick={handleTestSend}
              disabled={sending}
            >
              {sending ? '发送中...' : '发送测试邮件'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 预览对话框 */}
      <Dialog open={previewOpen} onOpenChange={setPreviewOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-3xl max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>
              模板预览
              {previewing && (
                <span className="ml-2 text-sm font-normal text-zinc-500">
                  ({TEMPLATE_NAME_LABELS[previewing.name] || previewing.name})
                </span>
              )}
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-3 py-2">
            {previewing && (
              <>
                <div>
                  <div className="text-xs text-zinc-500 mb-1">主题</div>
                  <div className="text-sm text-zinc-200 bg-zinc-800/50 border border-zinc-800 rounded-md p-2">
                    {previewing.subject || '（空）'}
                  </div>
                </div>
                <div>
                  <div className="text-xs text-zinc-500 mb-1">HTML 正文（渲染预览）</div>
                  <div
                    className="bg-white border border-zinc-800 rounded-md p-4 text-sm text-zinc-900 overflow-auto max-h-[50vh]"
                    // eslint-disable-next-line react/no-danger
                    dangerouslySetInnerHTML={{ __html: previewing.body || '<p style="color:#999">（空）</p>' }}
                  />
                </div>
                <div>
                  <div className="text-xs text-zinc-500 mb-1">HTML 源码</div>
                  <pre className="bg-zinc-950 border border-zinc-800 rounded-md p-3 text-xs font-mono text-zinc-300 overflow-auto max-h-60 whitespace-pre-wrap break-all">
                    {previewing.body || '（空）'}
                  </pre>
                </div>
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
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
