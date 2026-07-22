import { useState, useEffect } from 'react'
import {
  Plus,
  Pencil,
  Trash2,
  RefreshCw,
  Download,
  Key,
  Power,
  CreditCard,
  History,
  Layers,
} from 'lucide-react'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Button,
  Input,
  Badge,
  Skeleton,
  EmptyState,
  Label,
  Select,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
  useToast,
} from '@airport/ui'
import { xbAdminApi } from '@/lib/api'

interface Plan {
  id: number
  name: string
}

interface GiftCardTemplate {
  id: number
  name: string
  type: number
  value: number
  plan_id: number | null
  created_at?: string
  [key: string]: unknown
}

interface GiftCardCode {
  id: number
  template_id: number
  code: string
  status: number
  user_id?: number | null
  user?: { email?: string; [key: string]: unknown }
  used_at?: number | null
  created_at: number
  [key: string]: unknown
}

interface GiftCardUsage {
  id: number
  code: string
  template_id: number
  template?: { name?: string; [key: string]: unknown }
  user_id: number
  user?: { email?: string; [key: string]: unknown }
  used_at: number
  [key: string]: unknown
}

const TEMPLATE_TYPE_MAP: Record<number, { label: string; color: string }> = {
  1: { label: '余额充值', color: 'bg-emerald-900/50 text-emerald-300 border-emerald-800/50' },
  2: { label: '套餐赠送', color: 'bg-indigo-900/50 text-indigo-300 border-indigo-800/50' },
}

const CODE_STATUS_MAP: Record<number, { label: string; color: string }> = {
  0: { label: '未使用', color: 'bg-zinc-800 text-zinc-400 border-zinc-700' },
  1: { label: '已使用', color: 'bg-emerald-900/50 text-emerald-300 border-emerald-800/50' },
  2: { label: '已禁用', color: 'bg-red-900/50 text-red-300 border-red-800/50' },
}

import { ComingSoon } from '@/components/common/ComingSoon'

export default function GiftCards() {
  return <ComingSoon title="礼品卡管理" reason="该模块后端 API 正在迁移到 YunDu Go identity-service，暂不可用。" />

  const { toast } = useToast()
  const [activeTab, setActiveTab] = useState('templates')
  const [plans, setPlans] = useState<Plan[]>([])

  const [templates, setTemplates] = useState<GiftCardTemplate[]>([])
  const [templatesLoading, setTemplatesLoading] = useState(true)
  const [templateDialogOpen, setTemplateDialogOpen] = useState(false)
  const [editingTemplate, setEditingTemplate] = useState<GiftCardTemplate | null>(null)
  const [templateForm, setTemplateForm] = useState({ name: '', type: 1, value: 0, plan_id: '' })
  const [templateSubmitting, setTemplateSubmitting] = useState(false)

  const [codes, setCodes] = useState<GiftCardCode[]>([])
  const [codesLoading, setCodesLoading] = useState(false)
  const [selectedTemplateId, setSelectedTemplateId] = useState<string>('')
  const [generateDialogOpen, setGenerateDialogOpen] = useState(false)
  const [generateCount, setGenerateCount] = useState(10)
  const [generateSubmitting, setGenerateSubmitting] = useState(false)

  const [usages, setUsages] = useState<GiftCardUsage[]>([])
  const [usagesLoading, setUsagesLoading] = useState(false)

  const fetchPlans = async () => {
    try {
      const data = await xbAdminApi.get<Plan[]>('/plan/fetch')
      setPlans(Array.isArray(data) ? data : [])
    } catch {
      setPlans([])
    }
  }

  const fetchTemplates = async () => {
    try {
      setTemplatesLoading(true)
      const data = await xbAdminApi.get<GiftCardTemplate[]>('/gift-card/templates')
      setTemplates(Array.isArray(data) ? data : [])
    } catch (err) {
      const message = err instanceof Error ? err.message : '获取模板列表失败'
      toast({ title: '获取失败', description: message, variant: 'destructive' })
    } finally {
      setTemplatesLoading(false)
    }
  }

  const fetchCodes = async (templateId?: string) => {
    try {
      setCodesLoading(true)
      const body: Record<string, unknown> = {}
      if (templateId || selectedTemplateId) {
        body.template_id = Number(templateId || selectedTemplateId)
      }
      const data = await xbAdminApi.post<GiftCardCode[]>('/gift-card/codes', body)
      setCodes(Array.isArray(data) ? data : [])
    } catch (err) {
      const message = err instanceof Error ? err.message : '获取卡密列表失败'
      toast({ title: '获取失败', description: message, variant: 'destructive' })
    } finally {
      setCodesLoading(false)
    }
  }

  const fetchUsages = async () => {
    try {
      setUsagesLoading(true)
      const data = await xbAdminApi.post<GiftCardUsage[]>('/gift-card/usages', {})
      setUsages(Array.isArray(data) ? data : [])
    } catch (err) {
      const message = err instanceof Error ? err.message : '获取使用记录失败'
      toast({ title: '获取失败', description: message, variant: 'destructive' })
    } finally {
      setUsagesLoading(false)
    }
  }

  useEffect(() => {
    fetchPlans()
    fetchTemplates()
  }, [])

  useEffect(() => {
    if (activeTab === 'codes') {
      fetchCodes()
    } else if (activeTab === 'usages') {
      fetchUsages()
    }
  }, [activeTab])

  const openCreateTemplateDialog = () => {
    setEditingTemplate(null)
    setTemplateForm({ name: '', type: 1, value: 0, plan_id: '' })
    setTemplateDialogOpen(true)
  }

  const openEditTemplateDialog = (template: GiftCardTemplate) => {
    setEditingTemplate(template)
    setTemplateForm({
      name: template.name,
      type: template.type,
      value: template.value,
      plan_id: template.plan_id ? String(template.plan_id) : '',
    })
    setTemplateDialogOpen(true)
  }

  const closeTemplateDialog = () => {
    setTemplateDialogOpen(false)
    setEditingTemplate(null)
  }

  const handleTemplateSubmit = async () => {
    if (!templateForm.name.trim()) {
      toast({ title: '请输入模板名称', variant: 'destructive' })
      return
    }
    if (templateForm.type === 1 && (!templateForm.value || templateForm.value <= 0)) {
      toast({ title: '请输入充值金额', variant: 'destructive' })
      return
    }
    if (templateForm.type === 2 && !templateForm.plan_id) {
      toast({ title: '请选择套餐', variant: 'destructive' })
      return
    }

    try {
      setTemplateSubmitting(true)
      const submitData = {
        ...(editingTemplate ? { id: editingTemplate.id } : {}),
        name: templateForm.name,
        type: Number(templateForm.type),
        value: templateForm.type === 1 ? Number(templateForm.value) : Number(templateForm.plan_id),
        plan_id: templateForm.type === 2 ? Number(templateForm.plan_id) : null,
      }

      if (editingTemplate) {
        await xbAdminApi.post('/gift-card/update-template', submitData)
        toast({ title: '模板已更新', variant: 'success' })
      } else {
        await xbAdminApi.post('/gift-card/create-template', submitData)
        toast({ title: '模板已创建', variant: 'success' })
      }
      closeTemplateDialog()
      fetchTemplates()
    } catch (err) {
      const message = err instanceof Error ? err.message : '操作失败'
      toast({ title: '操作失败', description: message, variant: 'destructive' })
    } finally {
      setTemplateSubmitting(false)
    }
  }

  const handleDeleteTemplate = async (template: GiftCardTemplate) => {
    if (!confirm(`确定要删除模板 "${template.name}" 吗？`)) return
    try {
      await xbAdminApi.post('/gift-card/delete-template', { id: template.id })
      toast({ title: '模板已删除', variant: 'success' })
      fetchTemplates()
    } catch (err) {
      const message = err instanceof Error ? err.message : '删除失败'
      toast({ title: '删除失败', description: message, variant: 'destructive' })
    }
  }

  const handleGenerateCodes = async () => {
    if (!selectedTemplateId) {
      toast({ title: '请先选择卡密模板', variant: 'destructive' })
      return
    }
    if (generateCount <= 0 || generateCount > 1000) {
      toast({ title: '生成数量需在1-1000之间', variant: 'destructive' })
      return
    }
    try {
      setGenerateSubmitting(true)
      await xbAdminApi.post('/gift-card/generate-codes', {
        template_id: Number(selectedTemplateId),
        count: generateCount,
      })
      toast({ title: `已生成 ${generateCount} 个卡密`, variant: 'success' })
      setGenerateDialogOpen(false)
      fetchCodes()
    } catch (err) {
      const message = err instanceof Error ? err.message : '生成失败'
      toast({ title: '生成失败', description: message, variant: 'destructive' })
    } finally {
      setGenerateSubmitting(false)
    }
  }

  const toggleCode = async (code: GiftCardCode) => {
    try {
      await xbAdminApi.post('/gift-card/toggle-code', { id: code.id })
      toast({ title: '状态已更新', variant: 'success' })
      fetchCodes()
    } catch (err) {
      const message = err instanceof Error ? err.message : '操作失败'
      toast({ title: '操作失败', description: message, variant: 'destructive' })
    }
  }

  const deleteCode = async (code: GiftCardCode) => {
    if (!confirm(`确定要删除卡密 "${code.code}" 吗？`)) return
    try {
      await xbAdminApi.post('/gift-card/delete-code', { id: code.id })
      toast({ title: '卡密已删除', variant: 'success' })
      fetchCodes()
    } catch (err) {
      const message = err instanceof Error ? err.message : '删除失败'
      toast({ title: '删除失败', description: message, variant: 'destructive' })
    }
  }

  const getExportUrl = () => {
    const baseUrl = '/api/v1/admin/gift-card/export-codes'
    const params = selectedTemplateId ? `?template_id=${selectedTemplateId}` : ''
    return baseUrl + params
  }

  const formatDate = (timestamp?: number | null) => {
    if (!timestamp) return '-'
    try {
      return new Date(timestamp * 1000).toLocaleString('zh-CN')
    } catch { return '-' }
  }

  const getTemplateName = (templateId: number) => {
    const t = templates.find(t => t.id === templateId)
    return t ? t.name : `模板 #${templateId}`
  }

  const getUserEmail = (user?: { email?: string } | null, userId?: number) => {
    if (user && user.email) return user.email
    if (userId) return `用户 #${userId}`
    return '-'
  }

  const getValueDisplay = (template: GiftCardTemplate) => {
    if (template.type === 1) return `¥${template.value}`
    const plan = plans.find(p => p.id === template.plan_id)
    return plan ? plan.name : `套餐 #${template.value}`
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-zinc-100">礼品卡管理</h1>
          <p className="text-sm text-zinc-500 mt-1">管理卡密模板、生成和使用记录</p>
        </div>
      </div>

      <Tabs value={activeTab} onValueChange={setActiveTab} className="space-y-4">
        <TabsList className="bg-zinc-800 border border-zinc-700">
          <TabsTrigger value="templates" className="data-[state=active]:bg-zinc-700">
            <Layers className="w-4 h-4 mr-1" />卡密模板
          </TabsTrigger>
          <TabsTrigger value="codes" className="data-[state=active]:bg-zinc-700">
            <Key className="w-4 h-4 mr-1" />卡密管理
          </TabsTrigger>
          <TabsTrigger value="usages" className="data-[state=active]:bg-zinc-700">
            <History className="w-4 h-4 mr-1" />使用记录
          </TabsTrigger>
        </TabsList>

        <TabsContent value="templates" className="space-y-4">
          <div className="flex items-center justify-between">
            <div className="text-sm text-zinc-500">共 {templates.length} 个模板</div>
            <div className="flex items-center gap-2">
              <Button variant="outline" size="sm" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={fetchTemplates}>
                <RefreshCw className="w-4 h-4 mr-1" />刷新
              </Button>
              <Button size="sm" className="bg-indigo-600 hover:bg-indigo-500" onClick={openCreateTemplateDialog}>
                <Plus className="w-4 h-4 mr-1" />创建模板
              </Button>
            </div>
          </div>

          <Card className="bg-zinc-900 border-zinc-800 overflow-hidden">
            <CardContent className="p-0">
              {templatesLoading ? (
                <div className="p-4 space-y-3">
                  {[1, 2, 3].map((i) => (
                    <Skeleton key={i} className="h-14 w-full bg-zinc-800 rounded-lg" />
                  ))}
                </div>
              ) : templates.length === 0 ? (
                <EmptyState title="暂无模板" description="点击右上角创建按钮创建第一个卡密模板" className="py-12" />
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full">
                    <thead>
                      <tr className="border-b border-zinc-800">
                        <th className="text-left p-3 text-xs font-medium text-zinc-400">ID</th>
                        <th className="text-left p-3 text-xs font-medium text-zinc-400">名称</th>
                        <th className="text-left p-3 text-xs font-medium text-zinc-400">类型</th>
                        <th className="text-left p-3 text-xs font-medium text-zinc-400">值</th>
                        <th className="text-left p-3 text-xs font-medium text-zinc-400 hidden md:table-cell">套餐</th>
                        <th className="text-left p-3 text-xs font-medium text-zinc-400 w-10"></th>
                      </tr>
                    </thead>
                    <tbody>
                      {templates.map((template) => {
                        const typeInfo = TEMPLATE_TYPE_MAP[template.type] || { label: '未知', color: 'bg-zinc-800 text-zinc-400 border-zinc-700' }
                        return (
                          <tr key={template.id} className="border-b border-zinc-800 last:border-0 hover:bg-zinc-800/50 transition-colors">
                            <td className="p-3 text-sm text-zinc-500 font-mono">{template.id}</td>
                            <td className="p-3 text-sm font-medium text-zinc-200">{template.name}</td>
                            <td className="p-3">
                              <Badge variant="outline" className={typeInfo.color}>{typeInfo.label}</Badge>
                            </td>
                            <td className="p-3 text-sm text-zinc-100 font-medium">{getValueDisplay(template)}</td>
                            <td className="p-3 text-sm text-zinc-400 hidden md:table-cell">
                              {template.type === 2 && template.plan_id ? plans.find(p => p.id === template.plan_id)?.name || `#${template.plan_id}` : '-'}
                            </td>
                            <td className="p-3">
                              <div className="flex items-center gap-1">
                                <Button variant="ghost" size="sm" className="h-8 w-8 p-0" onClick={() => openEditTemplateDialog(template)}>
                                  <Pencil className="w-4 h-4 text-zinc-400" />
                                </Button>
                                <Button variant="ghost" size="sm" className="h-8 w-8 p-0" onClick={() => handleDeleteTemplate(template)}>
                                  <Trash2 className="w-4 h-4 text-red-400" />
                                </Button>
                              </div>
                            </td>
                          </tr>
                        )
                      })}
                    </tbody>
                  </table>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="codes" className="space-y-4">
          <div className="flex items-center justify-between flex-wrap gap-2">
            <div className="flex items-center gap-2">
              <Select value={selectedTemplateId} onChange={(e) => { setSelectedTemplateId(e.target.value); fetchCodes(e.target.value) }} className="w-48 bg-zinc-800 border-zinc-700 text-zinc-100 h-9">
                <option value="">全部模板</option>
                {templates.map((t) => (
                  <option key={t.id} value={String(t.id)}>{t.name}</option>
                ))}
              </Select>
              <Button variant="outline" size="sm" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={() => fetchCodes()}>
                <RefreshCw className="w-4 h-4 mr-1" />刷新
              </Button>
              <a href={getExportUrl()} target="_blank" rel="noopener">
                <Button variant="outline" size="sm" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800">
                  <Download className="w-4 h-4 mr-1" />导出
                </Button>
              </a>
            </div>
            <Button size="sm" className="bg-indigo-600 hover:bg-indigo-500" onClick={() => setGenerateDialogOpen(true)}>
              <Plus className="w-4 h-4 mr-1" />生成卡密
            </Button>
          </div>

          <Card className="bg-zinc-900 border-zinc-800 overflow-hidden">
            <CardContent className="p-0">
              {codesLoading ? (
                <div className="p-4 space-y-3">
                  {[1, 2, 3, 4, 5].map((i) => (
                    <Skeleton key={i} className="h-14 w-full bg-zinc-800 rounded-lg" />
                  ))}
                </div>
              ) : codes.length === 0 ? (
                <EmptyState title="暂无卡密" description="选择模板后点击生成按钮创建卡密" className="py-12" />
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full">
                    <thead>
                      <tr className="border-b border-zinc-800">
                        <th className="text-left p-3 text-xs font-medium text-zinc-400">卡密</th>
                        <th className="text-left p-3 text-xs font-medium text-zinc-400">模板</th>
                        <th className="text-left p-3 text-xs font-medium text-zinc-400">状态</th>
                        <th className="text-left p-3 text-xs font-medium text-zinc-400 hidden md:table-cell">使用者</th>
                        <th className="text-left p-3 text-xs font-medium text-zinc-400 hidden lg:table-cell">使用时间</th>
                        <th className="text-left p-3 text-xs font-medium text-zinc-400 w-10"></th>
                      </tr>
                    </thead>
                    <tbody>
                      {codes.map((code) => {
                        const statusInfo = CODE_STATUS_MAP[code.status] || { label: '未知', color: 'bg-zinc-800 text-zinc-400 border-zinc-700' }
                        return (
                          <tr key={code.id} className="border-b border-zinc-800 last:border-0 hover:bg-zinc-800/50 transition-colors">
                            <td className="p-3">
                              <code className="text-sm font-mono text-indigo-400 bg-indigo-950/50 px-2 py-0.5 rounded">{code.code}</code>
                            </td>
                            <td className="p-3 text-sm text-zinc-400">{getTemplateName(code.template_id)}</td>
                            <td className="p-3">
                              <Badge variant="outline" className={statusInfo.color}>{statusInfo.label}</Badge>
                            </td>
                            <td className="p-3 text-sm text-zinc-300 hidden md:table-cell">
                              {getUserEmail(code.user as { email?: string } | undefined, code.user_id as number)}
                            </td>
                            <td className="p-3 text-sm text-zinc-500 hidden lg:table-cell">{formatDate(code.used_at)}</td>
                            <td className="p-3">
                              <div className="flex items-center gap-1">
                                {code.status !== 1 && (
                                  <Button variant="ghost" size="sm" className="h-8 w-8 p-0" onClick={() => toggleCode(code)}>
                                    <Power className={`w-4 h-4 ${code.status === 2 ? 'text-emerald-400' : 'text-yellow-400'}`} />
                                  </Button>
                                )}
                                <Button variant="ghost" size="sm" className="h-8 w-8 p-0" onClick={() => deleteCode(code)}>
                                  <Trash2 className="w-4 h-4 text-red-400" />
                                </Button>
                              </div>
                            </td>
                          </tr>
                        )
                      })}
                    </tbody>
                  </table>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="usages" className="space-y-4">
          <div className="flex items-center justify-between">
            <div className="text-sm text-zinc-500">共 {usages.length} 条记录</div>
            <Button variant="outline" size="sm" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={fetchUsages}>
              <RefreshCw className="w-4 h-4 mr-1" />刷新
            </Button>
          </div>

          <Card className="bg-zinc-900 border-zinc-800 overflow-hidden">
            <CardContent className="p-0">
              {usagesLoading ? (
                <div className="p-4 space-y-3">
                  {[1, 2, 3, 4, 5].map((i) => (
                    <Skeleton key={i} className="h-14 w-full bg-zinc-800 rounded-lg" />
                  ))}
                </div>
              ) : usages.length === 0 ? (
                <EmptyState title="暂无使用记录" description="还没有卡密被使用" className="py-12" />
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full">
                    <thead>
                      <tr className="border-b border-zinc-800">
                        <th className="text-left p-3 text-xs font-medium text-zinc-400">卡密</th>
                        <th className="text-left p-3 text-xs font-medium text-zinc-400 hidden sm:table-cell">模板</th>
                        <th className="text-left p-3 text-xs font-medium text-zinc-400">用户</th>
                        <th className="text-left p-3 text-xs font-medium text-zinc-400 hidden md:table-cell">使用时间</th>
                      </tr>
                    </thead>
                    <tbody>
                      {usages.map((usage) => (
                        <tr key={usage.id} className="border-b border-zinc-800 last:border-0 hover:bg-zinc-800/50 transition-colors">
                          <td className="p-3">
                            <code className="text-sm font-mono text-zinc-300 bg-zinc-800 px-2 py-0.5 rounded">{usage.code}</code>
                          </td>
                          <td className="p-3 text-sm text-zinc-400 hidden sm:table-cell">
                            {usage.template?.name || getTemplateName(usage.template_id)}
                          </td>
                          <td className="p-3 text-sm text-zinc-300">
                            {getUserEmail(usage.user as { email?: string } | undefined, usage.user_id)}
                          </td>
                          <td className="p-3 text-sm text-zinc-500 hidden md:table-cell">{formatDate(usage.used_at)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      <Dialog open={templateDialogOpen} onOpenChange={setTemplateDialogOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-md">
          <DialogHeader>
            <DialogTitle>{editingTemplate ? '编辑模板' : '创建模板'}</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label className="text-zinc-300 text-sm">模板名称 *</Label>
              <Input
                value={templateForm.name}
                onChange={(e) => setTemplateForm({ ...templateForm, name: e.target.value })}
                placeholder="如: 10元充值卡"
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              />
            </div>
            <div className="space-y-2">
              <Label className="text-zinc-300 text-sm">类型</Label>
              <Select value={String(templateForm.type)} onChange={(e) => setTemplateForm({ ...templateForm, type: Number(e.target.value), plan_id: '' })} className="bg-zinc-800 border-zinc-700 text-zinc-100">
                <option value="1">余额充值</option>
                <option value="2">套餐赠送</option>
              </Select>
            </div>
            {templateForm.type === 1 ? (
              <div className="space-y-2">
                <Label className="text-zinc-300 text-sm">充值金额 (元) *</Label>
                <Input
                  type="number"
                  value={templateForm.value}
                  onChange={(e) => setTemplateForm({ ...templateForm, value: Number(e.target.value) })}
                  placeholder="如: 10"
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
              </div>
            ) : (
              <div className="space-y-2">
                <Label className="text-zinc-300 text-sm">选择套餐 *</Label>
                <Select value={templateForm.plan_id} onChange={(e) => setTemplateForm({ ...templateForm, plan_id: e.target.value })} className="bg-zinc-800 border-zinc-700 text-zinc-100">
                  <option value="" disabled>选择套餐</option>
                  {plans.map((p) => (
                    <option key={p.id} value={String(p.id)}>{p.name}</option>
                  ))}
                </Select>
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={closeTemplateDialog}>
              取消
            </Button>
            <Button className="bg-indigo-600 hover:bg-indigo-500" onClick={handleTemplateSubmit} disabled={templateSubmitting}>
              {templateSubmitting ? '保存中...' : editingTemplate ? '更新' : '创建'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={generateDialogOpen} onOpenChange={setGenerateDialogOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-md">
          <DialogHeader>
            <DialogTitle>生成卡密</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label className="text-zinc-300 text-sm">选择模板</Label>
              <Select value={selectedTemplateId} onChange={(e) => setSelectedTemplateId(e.target.value)} className="bg-zinc-800 border-zinc-700 text-zinc-100">
                <option value="" disabled>选择模板</option>
                {templates.map((t) => (
                  <option key={t.id} value={String(t.id)}>{t.name}</option>
                ))}
              </Select>
            </div>
            <div className="space-y-2">
              <Label className="text-zinc-300 text-sm">生成数量</Label>
              <Input
                type="number"
                min="1"
                max="1000"
                value={generateCount}
                onChange={(e) => setGenerateCount(Number(e.target.value))}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              />
              <p className="text-xs text-zinc-500">单次最多生成1000个</p>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={() => setGenerateDialogOpen(false)}>
              取消
            </Button>
            <Button className="bg-indigo-600 hover:bg-indigo-500" onClick={handleGenerateCodes} disabled={generateSubmitting}>
              {generateSubmitting ? '生成中...' : '生成'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
