import { useState, useEffect } from 'react'
import { Plus, Pencil, Eye, RefreshCw, Trash2, ShieldCheck, FileKey, ListChecks, Download } from 'lucide-react'
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
  Switch,
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
type CertStatus = 'active' | 'expired' | 'revoked' | 'pending'
type CertType = 'acme' | 'self_signed' | 'upload' | 'cloudflare_origin'

interface TLSCertificate {
  id: string
  name: string
  type: CertType
  domains: string | string[]
  cert_pem?: string
  key_pem?: string
  issuer?: string
  not_before?: string
  not_after?: string
  status: CertStatus
  created_at?: string
  updated_at?: string
}

interface TLSProfile {
  id: string
  name: string
  min_tls_version: string
  cipher_suites: string | string[]
  hsts_enabled: boolean
  hsts_max_age: number
  created_at?: string
  updated_at?: string
}

interface CertDeployRecord {
  id: string
  node_id: string
  node_name?: string
  certificate_id: string
  certificate_name?: string
  profile_id?: string
  status: CertStatus | 'deploying' | 'failed'
  message?: string
  created_at?: string
}

// ===== 工具函数 =====
function getCertStatusBadge(status: CertStatus | 'deploying' | 'failed') {
  const map: Record<string, { label: string; variant: 'success' | 'secondary' | 'destructive' | 'warning' }> = {
    active: { label: '有效', variant: 'success' },
    pending: { label: '处理中', variant: 'warning' },
    expired: { label: '已过期', variant: 'secondary' },
    revoked: { label: '已吊销', variant: 'destructive' },
    deploying: { label: '部署中', variant: 'warning' },
    failed: { label: '失败', variant: 'destructive' },
  }
  const v = map[status] || map.pending
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

function formatDate(dateStr?: string) {
  if (!dateStr) return '-'
  return new Date(dateStr).toLocaleDateString('zh-CN')
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

function toDomainArray(domains: string | string[] | undefined): string[] {
  if (!domains) return []
  if (Array.isArray(domains)) return domains
  try {
    const parsed = JSON.parse(domains)
    if (Array.isArray(parsed)) return parsed
  } catch {
    // not json
  }
  return String(domains).split(',').map((s) => s.trim()).filter(Boolean)
}

const certTypeLabel: Record<CertType, string> = {
  acme: 'ACME 自动',
  self_signed: '自签名',
  upload: '手动上传',
  cloudflare_origin: 'CF Origin',
}

const emptyCertForm = {
  id: '',
  name: '',
  type: 'acme' as CertType,
  domains: '',
  cert_pem: '',
  key_pem: '',
}

const emptyProfileForm = {
  id: '',
  name: '',
  min_tls_version: '1.2',
  cipher_suites: 'TLS_AES_128_GCM_SHA256,TLS_AES_256_GCM_SHA384',
  hsts_enabled: true,
  hsts_max_age: 15552000,
}

// ===== 主组件 =====
export default function TLSCertificates() {
  const { toast } = useToast()
  const [tab, setTab] = useState('certs')

  // 证书
  const [certsLoading, setCertsLoading] = useState(true)
  const [certs, setCerts] = useState<TLSCertificate[]>([])
  const [certSearch, setCertSearch] = useState('')
  const [certDialogOpen, setCertDialogOpen] = useState(false)
  const [certForm, setCertForm] = useState(emptyCertForm)
  const [certErrors, setCertErrors] = useState<Record<string, string>>({})
  const [certSubmitting, setCertSubmitting] = useState(false)
  const [detailCert, setDetailCert] = useState<TLSCertificate | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)
  const [renewingId, setRenewingId] = useState<string | null>(null)

  // TLS Profile
  const [profilesLoading, setProfilesLoading] = useState(false)
  const [profiles, setProfiles] = useState<TLSProfile[]>([])
  const [profileDialogOpen, setProfileDialogOpen] = useState(false)
  const [profileForm, setProfileForm] = useState(emptyProfileForm)
  const [profileErrors, setProfileErrors] = useState<Record<string, string>>({})
  const [profileSubmitting, setProfileSubmitting] = useState(false)

  // 部署记录
  const [recordsLoading, setRecordsLoading] = useState(false)
  const [records, setRecords] = useState<CertDeployRecord[]>([])

  const loadCerts = async () => {
    setCertsLoading(true)
    try {
      const data = await api.get(EP.TLS_CERTIFICATES)
      setCerts(normalizeList<TLSCertificate>(data))
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '加载证书失败'
      toast({ title: '加载失败', description: msg, variant: 'destructive' })
    } finally {
      setCertsLoading(false)
    }
  }

  const loadProfiles = async () => {
    setProfilesLoading(true)
    try {
      const data = await api.get(EP.TLS_PROFILES)
      setProfiles(normalizeList<TLSProfile>(data))
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '加载 TLS Profile 失败'
      toast({ title: '加载失败', description: msg, variant: 'destructive' })
    } finally {
      setProfilesLoading(false)
    }
  }

  // 部署记录：后端无全局列表接口，改为逐证书聚合 deploy-status
  const loadRecords = async () => {
    if (certs.length === 0) {
      setRecords([])
      return
    }
    setRecordsLoading(true)
    try {
      const results: CertDeployRecord[] = []
      await Promise.all(
        certs.map(async (c) => {
          try {
            const data = await api.get(EP.TLS_CERTIFICATE_DEPLOY_STATUS(c.id))
            const list = normalizeList<CertDeployRecord>(data)
            for (const r of list) {
              if (!r.certificate_id) r.certificate_id = c.id
              if (!r.certificate_name) r.certificate_name = c.name
              results.push(r)
            }
          } catch {
            // 单证书查询失败时跳过，不影响整体
          }
        })
      )
      results.sort((a, b) => (b.created_at || '').localeCompare(a.created_at || ''))
      setRecords(results)
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '加载部署记录失败'
      toast({ title: '加载失败', description: msg, variant: 'destructive' })
    } finally {
      setRecordsLoading(false)
    }
  }

  useEffect(() => {
    loadCerts()
  }, [])

  const onTabChange = (value: string) => {
    setTab(value)
    if (value === 'profiles' && profiles.length === 0 && !profilesLoading) loadProfiles()
    if (value === 'records' && records.length === 0 && !recordsLoading) loadRecords()
  }

  // ===== 证书操作 =====
  const openCertCreate = () => {
    setCertForm(emptyCertForm)
    setCertErrors({})
    setCertDialogOpen(true)
  }

  const validateCert = () => {
    const e: Record<string, string> = {}
    if (!certForm.name.trim()) e.name = '请输入证书名称'
    if (!certForm.type) e.type = '请选择证书类型'
    if (!certForm.domains.trim()) e.domains = '请输入域名'
    if (certForm.type === 'upload' || certForm.type === 'self_signed') {
      if (!certForm.cert_pem.trim()) e.cert_pem = '请输入证书 PEM'
      if (!certForm.key_pem.trim()) e.key_pem = '请输入私钥 PEM'
    }
    setCertErrors(e)
    return Object.keys(e).length === 0
  }

  const submitCert = async () => {
    if (!validateCert()) return
    setCertSubmitting(true)
    try {
      const domainsArr = certForm.domains.split(',').map((s) => s.trim()).filter(Boolean)
      const payload: Record<string, unknown> = {
        name: certForm.name,
        type: certForm.type,
        domains: domainsArr,
      }
      if (certForm.type === 'upload' || certForm.type === 'self_signed') {
        payload.cert_pem = certForm.cert_pem
        payload.key_pem = certForm.key_pem
      }
      if (certForm.id) {
        await api.patch(EP.TLS_CERTIFICATE(certForm.id), payload)
        toast({ title: '更新成功', description: `证书 ${certForm.name} 已更新`, variant: 'success' })
      } else {
        await api.post(EP.TLS_CERTIFICATES, payload)
        toast({ title: '创建成功', description: `证书 ${certForm.name} 已添加`, variant: 'success' })
      }
      setCertDialogOpen(false)
      loadCerts()
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '保存失败'
      toast({ title: '保存失败', description: msg, variant: 'destructive' })
    } finally {
      setCertSubmitting(false)
    }
  }

  const viewCert = (c: TLSCertificate) => {
    setDetailCert(c)
    setDetailOpen(true)
  }

  const renewCert = async (c: TLSCertificate) => {
    setRenewingId(c.id)
    try {
      await api.post(EP.TLS_CERTIFICATE_RENEW(c.id))
      toast({ title: '续期已触发', description: `证书 ${c.name} 正在续期`, variant: 'success' })
      loadCerts()
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '续期失败'
      toast({ title: '续期失败', description: msg, variant: 'destructive' })
    } finally {
      setRenewingId(null)
    }
  }

  const deleteCert = async (c: TLSCertificate) => {
    if (!window.confirm(`确认删除证书 ${c.name}？`)) return
    try {
      await api.delete(EP.TLS_CERTIFICATE(c.id))
      toast({ title: '已删除', description: `证书 ${c.name} 已删除`, variant: 'success' })
      loadCerts()
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '删除失败'
      toast({ title: '删除失败', description: msg, variant: 'destructive' })
    }
  }

  const filteredCerts = certs.filter((c) =>
    c.name.toLowerCase().includes(certSearch.toLowerCase()) ||
    toDomainArray(c.domains).join(' ').toLowerCase().includes(certSearch.toLowerCase())
  )

  // ===== Profile 操作 =====
  const openProfileCreate = () => {
    setProfileForm(emptyProfileForm)
    setProfileErrors({})
    setProfileDialogOpen(true)
  }

  const openProfileEdit = (p: TLSProfile) => {
    setProfileForm({
      id: p.id,
      name: p.name,
      min_tls_version: p.min_tls_version || '1.2',
      cipher_suites: Array.isArray(p.cipher_suites) ? p.cipher_suites.join(',') : (p.cipher_suites || ''),
      hsts_enabled: !!p.hsts_enabled,
      hsts_max_age: p.hsts_max_age ?? 15552000,
    })
    setProfileErrors({})
    setProfileDialogOpen(true)
  }

  const validateProfile = () => {
    const e: Record<string, string> = {}
    if (!profileForm.name.trim()) e.name = '请输入 Profile 名称'
    if (!profileForm.min_tls_version.trim()) e.min_tls_version = '请选择最低 TLS 版本'
    if (!profileForm.cipher_suites.trim()) e.cipher_suites = '请输入加密套件'
    if (profileForm.hsts_enabled && (profileForm.hsts_max_age <= 0 || !Number.isFinite(profileForm.hsts_max_age))) {
      e.hsts_max_age = 'HSTS 最大年龄必须为正整数'
    }
    setProfileErrors(e)
    return Object.keys(e).length === 0
  }

  const submitProfile = async () => {
    if (!validateProfile()) return
    setProfileSubmitting(true)
    try {
      const payload = {
        name: profileForm.name,
        min_tls_version: profileForm.min_tls_version,
        cipher_suites: profileForm.cipher_suites.split(',').map((s) => s.trim()).filter(Boolean),
        hsts_enabled: profileForm.hsts_enabled,
        hsts_max_age: Number(profileForm.hsts_max_age),
      }
      if (profileForm.id) {
        await api.patch(EP.TLS_PROFILE(profileForm.id), payload)
        toast({ title: '更新成功', description: `Profile ${profileForm.name} 已更新`, variant: 'success' })
      } else {
        await api.post(EP.TLS_PROFILES, payload)
        toast({ title: '创建成功', description: `Profile ${profileForm.name} 已创建`, variant: 'success' })
      }
      setProfileDialogOpen(false)
      loadProfiles()
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '保存失败'
      toast({ title: '保存失败', description: msg, variant: 'destructive' })
    } finally {
      setProfileSubmitting(false)
    }
  }

  const deleteProfile = async (p: TLSProfile) => {
    if (!window.confirm(`确认删除 Profile ${p.name}？`)) return
    try {
      await api.delete(EP.TLS_PROFILE(p.id))
      toast({ title: '已删除', description: `Profile ${p.name} 已删除`, variant: 'success' })
      loadProfiles()
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '删除失败'
      toast({ title: '删除失败', description: msg, variant: 'destructive' })
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-zinc-100">TLS 证书中心</h2>
      </div>

      <Tabs value={tab} onValueChange={onTabChange}>
        <TabsList>
          <TabsTrigger value="certs">
            <FileKey className="w-3.5 h-3.5 mr-1.5" />
            证书
          </TabsTrigger>
          <TabsTrigger value="profiles">
            <ShieldCheck className="w-3.5 h-3.5 mr-1.5" />
            TLS Profile
          </TabsTrigger>
          <TabsTrigger value="records">
            <ListChecks className="w-3.5 h-3.5 mr-1.5" />
            部署记录
          </TabsTrigger>
        </TabsList>

        {/* ===== 证书 Tab ===== */}
        <TabsContent value="certs">
          <div className="space-y-4">
            <div className="flex items-center justify-between gap-2">
              <Input
                placeholder="搜索证书名称或域名..."
                value={certSearch}
                onChange={(e) => setCertSearch(e.target.value)}
                className="bg-zinc-900 border-zinc-800 text-zinc-100 placeholder:text-zinc-500 max-w-xs"
              />
              <Button size="sm" className="bg-indigo-600 hover:bg-indigo-500" onClick={openCertCreate}>
                <Plus className="w-4 h-4 mr-1" />
                添加证书
              </Button>
            </div>

            <Card className="bg-zinc-900 border-zinc-800 overflow-hidden">
              <CardContent className="p-0">
                {certsLoading ? (
                  <div className="p-4 space-y-3">
                    {[1, 2, 3].map((i) => (
                      <Skeleton key={i} className="h-16 w-full bg-zinc-800 rounded-lg" />
                    ))}
                  </div>
                ) : filteredCerts.length === 0 ? (
                  <EmptyState title="暂无证书" description="添加第一个 TLS 证书" className="py-12" />
                ) : (
                  <div className="overflow-x-auto">
                    <Table>
                      <TableHeader>
                        <TableRow className="border-zinc-800 hover:bg-transparent">
                          <TableHead className="text-zinc-400 text-xs font-medium">名称</TableHead>
                          <TableHead className="text-zinc-400 text-xs font-medium hidden sm:table-cell">类型</TableHead>
                          <TableHead className="text-zinc-400 text-xs font-medium">域名</TableHead>
                          <TableHead className="text-zinc-400 text-xs font-medium hidden md:table-cell">到期</TableHead>
                          <TableHead className="text-zinc-400 text-xs font-medium">状态</TableHead>
                          <TableHead className="text-zinc-400 text-xs font-medium w-24"></TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {filteredCerts.map((c) => (
                          <TableRow key={c.id} className="border-zinc-800 hover:bg-zinc-800/50">
                            <TableCell className="py-3">
                              <div className="font-medium text-zinc-200 text-sm">{c.name}</div>
                              <div className="text-xs text-zinc-500 sm:hidden mt-0.5">{certTypeLabel[c.type]}</div>
                            </TableCell>
                            <TableCell className="py-3 hidden sm:table-cell">
                              <Badge variant="secondary" className="bg-zinc-800 text-zinc-300 text-xs">
                                {certTypeLabel[c.type] || c.type}
                              </Badge>
                            </TableCell>
                            <TableCell className="py-3 text-sm text-zinc-400">
                              <div className="flex flex-col gap-0.5">
                                {toDomainArray(c.domains).slice(0, 2).map((d) => (
                                  <span key={d} className="truncate max-w-[180px]">{d}</span>
                                ))}
                                {toDomainArray(c.domains).length > 2 && (
                                  <span className="text-xs text-zinc-600">+{toDomainArray(c.domains).length - 2}</span>
                                )}
                              </div>
                            </TableCell>
                            <TableCell className="py-3 hidden md:table-cell text-sm text-zinc-400">{formatDate(c.not_after)}</TableCell>
                            <TableCell className="py-3">{getCertStatusBadge(c.status)}</TableCell>
                            <TableCell className="py-3">
                              <div className="flex items-center gap-1">
                                <Button variant="ghost" size="icon" className="h-8 w-8 text-zinc-400 hover:text-zinc-200" onClick={() => viewCert(c)} title="查看">
                                  <Eye className="w-4 h-4" />
                                </Button>
                                <Button
                                  variant="ghost"
                                  size="icon"
                                  className="h-8 w-8 text-zinc-400 hover:text-emerald-400"
                                  onClick={() => renewCert(c)}
                                  title="续期"
                                  disabled={renewingId === c.id}
                                >
                                  <RefreshCw className={`w-4 h-4 ${renewingId === c.id ? 'animate-spin' : ''}`} />
                                </Button>
                                <Button variant="ghost" size="icon" className="h-8 w-8 text-zinc-400 hover:text-red-400" onClick={() => deleteCert(c)} title="删除">
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

        {/* ===== TLS Profile Tab ===== */}
        <TabsContent value="profiles">
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <p className="text-sm text-zinc-500">管理 TLS 安全策略与 HSTS 配置</p>
              <Button size="sm" className="bg-indigo-600 hover:bg-indigo-500" onClick={openProfileCreate}>
                <Plus className="w-4 h-4 mr-1" />
                新建 Profile
              </Button>
            </div>

            {profilesLoading ? (
              <div className="space-y-3">
                {[1, 2, 3].map((i) => (
                  <Skeleton key={i} className="h-24 w-full bg-zinc-800 rounded-lg" />
                ))}
              </div>
            ) : profiles.length === 0 ? (
              <Card className="bg-zinc-900 border-zinc-800">
                <CardContent>
                  <EmptyState title="暂无 TLS Profile" description="创建 Profile 定义 TLS 安全策略" className="py-12" />
                </CardContent>
              </Card>
            ) : (
              <div className="space-y-3">
                {profiles.map((p) => {
                  const ciphers = Array.isArray(p.cipher_suites) ? p.cipher_suites : (p.cipher_suites || '').split(',').filter(Boolean)
                  return (
                    <Card key={p.id} className="bg-zinc-900 border-zinc-800">
                      <CardContent className="p-4">
                        <div className="flex items-start justify-between gap-3">
                          <div className="min-w-0 flex-1">
                            <div className="flex items-center gap-2 flex-wrap">
                              <ShieldCheck className="w-4 h-4 text-indigo-400 flex-shrink-0" />
                              <span className="font-medium text-zinc-100 text-sm">{p.name}</span>
                              <Badge variant="secondary" className="bg-zinc-800 text-zinc-300 text-xs">
                                TLS {p.min_tls_version}
                              </Badge>
                              {p.hsts_enabled && (
                                <Badge variant="success" className="text-xs">HSTS</Badge>
                              )}
                            </div>
                            <div className="mt-2 space-y-1">
                              <div className="flex flex-wrap gap-1">
                                {ciphers.slice(0, 4).map((c) => (
                                  <span key={c} className="px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-400 text-[10px] font-mono">
                                    {c}
                                  </span>
                                ))}
                                {ciphers.length > 4 && (
                                  <span className="px-1.5 py-0.5 text-zinc-600 text-[10px]">+{ciphers.length - 4}</span>
                                )}
                              </div>
                              {p.hsts_enabled && (
                                <p className="text-xs text-zinc-500">HSTS 最大年龄: {p.hsts_max_age}s</p>
                              )}
                            </div>
                          </div>
                          <div className="flex items-center gap-1 flex-shrink-0">
                            <Button variant="ghost" size="icon" className="h-8 w-8 text-zinc-400 hover:text-zinc-200" onClick={() => openProfileEdit(p)}>
                              <Pencil className="w-4 h-4" />
                            </Button>
                            <Button variant="ghost" size="icon" className="h-8 w-8 text-zinc-400 hover:text-red-400" onClick={() => deleteProfile(p)}>
                              <Trash2 className="w-4 h-4" />
                            </Button>
                          </div>
                        </div>
                      </CardContent>
                    </Card>
                  )
                })}
              </div>
            )}
          </div>
        </TabsContent>

        {/* ===== 部署记录 Tab ===== */}
        <TabsContent value="records">
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <p className="text-sm text-zinc-500">证书部署到节点的历史记录</p>
              <Button size="sm" variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={loadRecords}>
                <RefreshCw className="w-3.5 h-3.5 mr-1.5" />
                刷新
              </Button>
            </div>

            <Card className="bg-zinc-900 border-zinc-800 overflow-hidden">
              <CardContent className="p-0">
                {recordsLoading ? (
                  <div className="p-4 space-y-3">
                    {[1, 2, 3].map((i) => (
                      <Skeleton key={i} className="h-14 w-full bg-zinc-800 rounded-lg" />
                    ))}
                  </div>
                ) : records.length === 0 ? (
                  <EmptyState title="暂无部署记录" description="证书部署后将显示在此" className="py-12" />
                ) : (
                  <div className="overflow-x-auto">
                    <Table>
                      <TableHeader>
                        <TableRow className="border-zinc-800 hover:bg-transparent">
                          <TableHead className="text-zinc-400 text-xs font-medium">节点</TableHead>
                          <TableHead className="text-zinc-400 text-xs font-medium hidden sm:table-cell">证书</TableHead>
                          <TableHead className="text-zinc-400 text-xs font-medium">状态</TableHead>
                          <TableHead className="text-zinc-400 text-xs font-medium hidden md:table-cell">消息</TableHead>
                          <TableHead className="text-zinc-400 text-xs font-medium">时间</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {records.map((r) => (
                          <TableRow key={r.id} className="border-zinc-800 hover:bg-zinc-800/50">
                            <TableCell className="py-3">
                              <div className="font-medium text-zinc-200 text-sm">{r.node_name || r.node_id}</div>
                              <div className="text-xs text-zinc-500 sm:hidden mt-0.5">{r.certificate_name || r.certificate_id}</div>
                            </TableCell>
                            <TableCell className="py-3 hidden sm:table-cell text-sm text-zinc-300">{r.certificate_name || r.certificate_id}</TableCell>
                            <TableCell className="py-3">{getCertStatusBadge(r.status)}</TableCell>
                            <TableCell className="py-3 hidden md:table-cell text-sm text-zinc-500 max-w-[200px] truncate">{r.message || '-'}</TableCell>
                            <TableCell className="py-3 text-sm text-zinc-500">{formatTime(r.created_at)}</TableCell>
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
      </Tabs>

      {/* ===== 证书 Dialog ===== */}
      <Dialog open={certDialogOpen} onOpenChange={setCertDialogOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-lg">
          <DialogHeader>
            <DialogTitle>{certForm.id ? '编辑证书' : '添加证书'}</DialogTitle>
            <DialogDescription>支持 ACME 自动签发、自签名、手动上传与 Cloudflare Origin</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400">证书名称 *</label>
              <Input
                placeholder="如 example.com-main"
                value={certForm.name}
                onChange={(e) => setCertForm({ ...certForm, name: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              />
              {certErrors.name && <p className="text-xs text-red-400">{certErrors.name}</p>}
            </div>
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400">证书类型 *</label>
              <Select
                value={certForm.type}
                onChange={(e) => setCertForm({ ...certForm, type: e.target.value as CertType })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              >
                <option value="acme">ACME 自动</option>
                <option value="self_signed">自签名</option>
                <option value="upload">手动上传</option>
                <option value="cloudflare_origin">Cloudflare Origin</option>
              </Select>
              {certErrors.type && <p className="text-xs text-red-400">{certErrors.type}</p>}
            </div>
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400">域名 *</label>
              <Input
                placeholder="多个域名用英文逗号分隔"
                value={certForm.domains}
                onChange={(e) => setCertForm({ ...certForm, domains: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              />
              {certErrors.domains && <p className="text-xs text-red-400">{certErrors.domains}</p>}
            </div>
            {(certForm.type === 'upload' || certForm.type === 'self_signed') && (
              <>
                <div className="space-y-1.5">
                  <label className="text-xs text-zinc-400">证书 PEM *</label>
                  <Textarea
                    rows={5}
                    placeholder="-----BEGIN CERTIFICATE-----"
                    value={certForm.cert_pem}
                    onChange={(e) => setCertForm({ ...certForm, cert_pem: e.target.value })}
                    className="bg-zinc-800 border-zinc-700 text-zinc-100 font-mono text-xs"
                  />
                  {certErrors.cert_pem && <p className="text-xs text-red-400">{certErrors.cert_pem}</p>}
                </div>
                <div className="space-y-1.5">
                  <label className="text-xs text-zinc-400">私钥 PEM *</label>
                  <Textarea
                    rows={5}
                    placeholder="-----BEGIN PRIVATE KEY-----"
                    value={certForm.key_pem}
                    onChange={(e) => setCertForm({ ...certForm, key_pem: e.target.value })}
                    className="bg-zinc-800 border-zinc-700 text-zinc-100 font-mono text-xs"
                  />
                  {certErrors.key_pem && <p className="text-xs text-red-400">{certErrors.key_pem}</p>}
                </div>
              </>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={() => setCertDialogOpen(false)}>
              取消
            </Button>
            <Button className="bg-indigo-600 hover:bg-indigo-500" disabled={certSubmitting} onClick={submitCert}>
              {certSubmitting ? '保存中...' : '保存'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* ===== 证书详情 Dialog ===== */}
      <Dialog open={detailOpen} onOpenChange={setDetailOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-lg">
          {detailCert && (
            <>
              <DialogHeader>
                <DialogTitle className="flex items-center gap-2">
                  {detailCert.name}
                  {getCertStatusBadge(detailCert.status)}
                </DialogTitle>
                <DialogDescription>{certTypeLabel[detailCert.type] || detailCert.type}</DialogDescription>
              </DialogHeader>
              <div className="space-y-3 pt-2">
                <div>
                  <div className="text-xs text-zinc-500 mb-1">域名</div>
                  <div className="flex flex-wrap gap-1">
                    {toDomainArray(detailCert.domains).map((d) => (
                      <Badge key={d} variant="secondary" className="bg-zinc-800 text-zinc-300 text-xs">{d}</Badge>
                    ))}
                  </div>
                </div>
                <Separator className="bg-zinc-800" />
                <div className="grid grid-cols-2 gap-3 text-xs">
                  <div>
                    <div className="text-zinc-500">签发者</div>
                    <div className="text-zinc-300 mt-0.5">{detailCert.issuer || '-'}</div>
                  </div>
                  <div>
                    <div className="text-zinc-500">签发时间</div>
                    <div className="text-zinc-300 mt-0.5">{formatDate(detailCert.not_before)}</div>
                  </div>
                  <div>
                    <div className="text-zinc-500">到期时间</div>
                    <div className="text-zinc-300 mt-0.5">{formatDate(detailCert.not_after)}</div>
                  </div>
                  <div>
                    <div className="text-zinc-500">更新时间</div>
                    <div className="text-zinc-300 mt-0.5">{formatTime(detailCert.updated_at)}</div>
                  </div>
                </div>
                {detailCert.cert_pem && (
                  <>
                    <Separator className="bg-zinc-800" />
                    <div>
                      <div className="flex items-center justify-between mb-1">
                        <span className="text-xs text-zinc-500">证书内容</span>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-7 w-7 text-zinc-400 hover:text-zinc-200"
                          onClick={() => {
                            navigator.clipboard?.writeText(detailCert.cert_pem || '')
                            toast({ title: '已复制', variant: 'success' })
                          }}
                          title="复制"
                        >
                          <Download className="w-3.5 h-3.5" />
                        </Button>
                      </div>
                      <pre className="text-[10px] text-zinc-400 bg-zinc-950/60 border border-zinc-800 rounded-md p-2 max-h-32 overflow-auto whitespace-pre-wrap break-all">
                        {detailCert.cert_pem}
                      </pre>
                    </div>
                  </>
                )}
              </div>
              <DialogFooter>
                <Button
                  className="bg-indigo-600 hover:bg-indigo-500"
                  disabled={renewingId === detailCert.id}
                  onClick={() => renewCert(detailCert)}
                >
                  <RefreshCw className={`w-4 h-4 mr-1.5 ${renewingId === detailCert.id ? 'animate-spin' : ''}`} />
                  {renewingId === detailCert.id ? '续期中...' : '续期'}
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>

      {/* ===== Profile Dialog ===== */}
      <Dialog open={profileDialogOpen} onOpenChange={setProfileDialogOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-lg">
          <DialogHeader>
            <DialogTitle>{profileForm.id ? '编辑 TLS Profile' : '新建 TLS Profile'}</DialogTitle>
            <DialogDescription>配置最低 TLS 版本、加密套件与 HSTS</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400">Profile 名称 *</label>
              <Input
                placeholder="如 modern-tls"
                value={profileForm.name}
                onChange={(e) => setProfileForm({ ...profileForm, name: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              />
              {profileErrors.name && <p className="text-xs text-red-400">{profileErrors.name}</p>}
            </div>
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400">最低 TLS 版本 *</label>
              <Select
                value={profileForm.min_tls_version}
                onChange={(e) => setProfileForm({ ...profileForm, min_tls_version: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              >
                <option value="1.0">TLS 1.0</option>
                <option value="1.1">TLS 1.1</option>
                <option value="1.2">TLS 1.2</option>
                <option value="1.3">TLS 1.3</option>
              </Select>
              {profileErrors.min_tls_version && <p className="text-xs text-red-400">{profileErrors.min_tls_version}</p>}
            </div>
            <div className="space-y-1.5">
              <label className="text-xs text-zinc-400">加密套件 *</label>
              <Textarea
                rows={3}
                placeholder="逗号分隔，如 TLS_AES_128_GCM_SHA256"
                value={profileForm.cipher_suites}
                onChange={(e) => setProfileForm({ ...profileForm, cipher_suites: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100 font-mono text-xs"
              />
              {profileErrors.cipher_suites && <p className="text-xs text-red-400">{profileErrors.cipher_suites}</p>}
            </div>
            <div className="flex items-center justify-between rounded-lg border border-zinc-800 bg-zinc-950/40 p-3">
              <div>
                <div className="text-sm text-zinc-200">启用 HSTS</div>
                <div className="text-xs text-zinc-500">强制浏览器使用 HTTPS</div>
              </div>
              <Switch
                checked={profileForm.hsts_enabled}
                onChange={(e) => setProfileForm({ ...profileForm, hsts_enabled: e.target.checked })}
              />
            </div>
            {profileForm.hsts_enabled && (
              <div className="space-y-1.5">
                <label className="text-xs text-zinc-400">HSTS 最大年龄 (秒)</label>
                <Input
                  type="number"
                  value={profileForm.hsts_max_age}
                  onChange={(e) => setProfileForm({ ...profileForm, hsts_max_age: Number(e.target.value) })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
                {profileErrors.hsts_max_age && <p className="text-xs text-red-400">{profileErrors.hsts_max_age}</p>}
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" className="border-zinc-700 text-zinc-300 hover:bg-zinc-800" onClick={() => setProfileDialogOpen(false)}>
              取消
            </Button>
            <Button className="bg-indigo-600 hover:bg-indigo-500" disabled={profileSubmitting} onClick={submitProfile}>
              {profileSubmitting ? '保存中...' : '保存'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
