import { useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
  Badge,
  Button,
  Input,
  Label,
  Skeleton,
  useToast,
} from '@airport/ui'
import {
  User,
  CreditCard,
  Link2,
  Receipt,
  Activity,
  FileText,
  Copy,
  RefreshCw,
  Ban,
  CheckCircle,
  Calendar,
  Monitor,
  Tag,
  Edit3,
} from 'lucide-react'
import {
  useUser,
  User as UserT,
  UserDetail,
} from '@/lib/hooks'

interface UserDetailDialogProps {
  userId: string | null
  onOpenChange: (open: boolean) => void
}

function formatDate(dateStr?: string) {
  if (!dateStr) return '-'
  try {
    return new Date(dateStr).toLocaleString('zh-CN', {
      year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit',
    })
  } catch { return dateStr }
}

function TrafficBar({ used, limit }: { used?: number; limit?: number }) {
  if (typeof used !== 'number' || typeof limit !== 'number' || limit <= 0) {
    return <span className="text-zinc-600 text-xs">-</span>
  }
  const pct = Math.min((used / limit) * 100, 100)
  const barColor = pct > 90 ? 'bg-red-500' : pct > 70 ? 'bg-amber-500' : 'bg-emerald-500'
  return (
    <div className="w-full">
      <div className="flex justify-between text-xs text-zinc-400 mb-1">
        <span>{used.toFixed(2)} GB</span>
        <span>/ {limit} GB</span>
      </div>
      <div className="w-full bg-zinc-800 rounded-full h-2">
        <div className={`h-2 rounded-full ${barColor}`} style={{ width: `${pct}%` }} />
      </div>
    </div>
  )
}

function getStatusBadge(status?: string) {
  if (!status) return <Badge variant="outline" className="bg-zinc-800 text-zinc-400 border-zinc-700">未知</Badge>
  const variants: Record<string, { label: string; className: string }> = {
    active: { label: '正常', className: 'bg-emerald-900/50 text-emerald-300 border-emerald-800/50' },
    banned: { label: '封禁', className: 'bg-red-900/50 text-red-300 border-red-800/50' },
    suspended: { label: '停用', className: 'bg-red-900/50 text-red-300 border-red-800/50' },
    expired: { label: '过期', className: 'bg-orange-900/50 text-orange-300 border-orange-800/50' },
    disabled: { label: '禁用', className: 'bg-yellow-900/50 text-yellow-300 border-yellow-800/50' },
    pending: { label: '待激活', className: 'bg-zinc-800 text-zinc-400 border-zinc-700' },
  }
  const v = variants[status] || { label: status, className: 'bg-zinc-800 text-zinc-400 border-zinc-700' }
  return <Badge variant="outline" className={v.className}>{v.label}</Badge>
}

export function UserDetailDialog({ userId, onOpenChange }: UserDetailDialogProps) {
  const { toast } = useToast()
  const [editingNote, setEditingNote] = useState(false)
  const [noteText, setNoteText] = useState('')
  const [activeTab, setActiveTab] = useState('overview')

  const { data: user, isLoading } = useUser(userId || '', {
    enabled: !!userId,
  })

  const copyText = (text: string, label: string) => {
    navigator.clipboard.writeText(text)
    toast({ title: '已复制', description: `${label}已复制到剪贴板`, variant: 'success' })
  }

  const isOpen = !!userId

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-3xl max-h-[90vh] overflow-hidden p-0 flex flex-col">
        {isLoading || !user ? (
          <div className="p-6 space-y-4">
            <Skeleton className="h-8 w-48 bg-zinc-800" />
            <Skeleton className="h-4 w-full bg-zinc-800" />
            <Skeleton className="h-32 w-full bg-zinc-800" />
          </div>
        ) : (
          <>
            <DialogHeader className="p-6 pb-2">
              <DialogTitle className="flex items-center gap-3">
                <div className="w-10 h-10 rounded-full bg-gradient-to-br from-indigo-500 to-purple-600 flex items-center justify-center text-white font-bold text-lg">
                  {(user.name || user.email || '?')[0].toUpperCase()}
                </div>
                <div>
                  <div className="flex items-center gap-2">
                    {user.name || user.username || user.email}
                    {getStatusBadge(user.status)}
                  </div>
                  <div className="text-sm text-zinc-400 font-normal mt-0.5">{user.email}</div>
                </div>
              </DialogTitle>
            </DialogHeader>

            <Tabs value={activeTab} onValueChange={setActiveTab} className="flex-1 flex flex-col overflow-hidden">
              <div className="px-6 border-b border-zinc-800">
                <TabsList className="bg-transparent p-0 h-auto gap-1">
                  <TabsTrigger value="overview" className="data-[state=active]:bg-zinc-800 px-3 py-2 h-auto text-xs">
                    <User className="w-3.5 h-3.5 mr-1" />概览
                  </TabsTrigger>
                  <TabsTrigger value="subscription" className="data-[state=active]:bg-zinc-800 px-3 py-2 h-auto text-xs">
                    <Link2 className="w-3.5 h-3.5 mr-1" />订阅
                  </TabsTrigger>
                  <TabsTrigger value="orders" className="data-[state=active]:bg-zinc-800 px-3 py-2 h-auto text-xs">
                    <Receipt className="w-3.5 h-3.5 mr-1" />订单
                  </TabsTrigger>
                  <TabsTrigger value="traffic" className="data-[state=active]:bg-zinc-800 px-3 py-2 h-auto text-xs">
                    <Activity className="w-3.5 h-3.5 mr-1" />流量
                  </TabsTrigger>
                  <TabsTrigger value="logs" className="data-[state=active]:bg-zinc-800 px-3 py-2 h-auto text-xs">
                    <FileText className="w-3.5 h-3.5 mr-1" />日志
                  </TabsTrigger>
                </TabsList>
              </div>

              <div className="flex-1 overflow-y-auto p-6">
                {/* Overview Tab */}
                <TabsContent value="overview" className="mt-0 space-y-4">
                  <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
                    <div className="bg-zinc-800/50 rounded-lg p-3 border border-zinc-700/50">
                      <div className="text-xs text-zinc-500 mb-1">套餐</div>
                      <div className="text-sm font-medium text-zinc-200">{user.plan_name || user.plan || '-'}</div>
                    </div>
                    <div className="bg-zinc-800/50 rounded-lg p-3 border border-zinc-700/50">
                      <div className="text-xs text-zinc-500 mb-1 flex items-center gap-1"><Monitor className="w-3 h-3" />在线设备</div>
                      <div className="text-sm font-medium text-zinc-200">{user.devices_online ?? 0}</div>
                    </div>
                    <div className="bg-zinc-800/50 rounded-lg p-3 border border-zinc-700/50">
                      <div className="text-xs text-zinc-500 mb-1">到期时间</div>
                      <div className="text-sm font-medium text-zinc-200">{formatDate(user.expires_at)}</div>
                    </div>
                    <div className="bg-zinc-800/50 rounded-lg p-3 border border-zinc-700/50">
                      <div className="text-xs text-zinc-500 mb-1">注册时间</div>
                      <div className="text-sm font-medium text-zinc-200">{formatDate(user.created_at)}</div>
                    </div>
                  </div>

                  <div className="bg-zinc-800/50 rounded-lg p-4 border border-zinc-700/50">
                    <div className="flex items-center justify-between mb-2">
                      <span className="text-sm text-zinc-400">流量使用</span>
                      <span className="text-xs text-zinc-500">
                        {user.traffic_used?.toFixed(2) || 0} / {user.traffic_limit || 0} GB
                      </span>
                    </div>
                    <TrafficBar used={user.traffic_used} limit={user.traffic_limit} />
                  </div>

                  {user.status === 'banned' && user.ban_reason && (
                    <div className="bg-red-950/30 border border-red-900/50 rounded-lg p-3 flex items-start gap-2">
                      <Ban className="w-4 h-4 text-red-400 mt-0.5 flex-shrink-0" />
                      <div>
                        <div className="text-sm text-red-300 font-medium">封禁原因</div>
                        <div className="text-xs text-red-400 mt-0.5">{user.ban_reason}</div>
                        {user.banned_at && <div className="text-xs text-red-500 mt-1">{formatDate(user.banned_at)}</div>}
                      </div>
                    </div>
                  )}

                  <div className="bg-zinc-800/50 rounded-lg p-4 border border-zinc-700/50 space-y-3">
                    <div className="flex items-center justify-between">
                      <span className="text-sm text-zinc-400 flex items-center gap-1"><Tag className="w-3.5 h-3.5" />标签 / 备注</span>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        className="h-7 text-xs text-zinc-400 hover:text-zinc-200"
                        onClick={() => { setNoteText(user.note || ''); setEditingNote(!editingNote) }}
                      >
                        <Edit3 className="w-3 h-3 mr-1" />{editingNote ? '取消' : '编辑'}
                      </Button>
                    </div>
                    {editingNote ? (
                      <div className="space-y-2">
                        <Input
                          value={noteText}
                          onChange={(e) => setNoteText(e.target.value)}
                          placeholder="输入备注..."
                          className="bg-zinc-900 border-zinc-700 text-zinc-100"
                        />
                        <Button size="sm" className="bg-indigo-600 hover:bg-indigo-500" onClick={() => { setEditingNote(false); toast({ title: '备注已更新', variant: 'success' }) }}>保存</Button>
                      </div>
                    ) : (
                      <p className="text-sm text-zinc-300">{user.note || '暂无备注'}</p>
                    )}
                    {user.tags && user.tags.length > 0 && (
                      <div className="flex flex-wrap gap-1 pt-1">
                        {user.tags.map((tag, i) => (
                          <Badge key={i} variant="outline" className="bg-zinc-700/50 text-zinc-300 border-zinc-600 text-xs">{tag}</Badge>
                        ))}
                      </div>
                    )}
                  </div>

                  <div className="grid grid-cols-3 gap-2 pt-2">
                    <div className="text-center p-2 rounded bg-zinc-800/30">
                      <div className="text-xs text-zinc-500">ID</div>
                      <code className="text-xs text-zinc-400 font-mono truncate block">{user.id.slice(0, 8)}...</code>
                    </div>
                    {user.username && (
                      <div className="text-center p-2 rounded bg-zinc-800/30">
                        <div className="text-xs text-zinc-500">用户名</div>
                        <div className="text-xs text-zinc-300 truncate">{user.username}</div>
                      </div>
                    )}
                    <div className="text-center p-2 rounded bg-zinc-800/30">
                      <div className="text-xs text-zinc-500">更新时间</div>
                      <div className="text-xs text-zinc-400">{formatDate(user.updated_at)}</div>
                    </div>
                  </div>
                </TabsContent>

                {/* Subscription Tab */}
                <TabsContent value="subscription" className="mt-0 space-y-4">
                  <div className="bg-zinc-800/50 rounded-lg p-4 border border-zinc-700/50 space-y-3">
                    <div className="flex items-center justify-between">
                      <span className="text-sm font-medium text-zinc-200">订阅链接</span>
                      <div className="flex gap-1">
                        <Button variant="ghost" size="sm" className="h-7 text-xs text-zinc-400"
                          onClick={() => copyText(user.subscription_url || '', '订阅链接')}>
                          <Copy className="w-3 h-3 mr-1" />复制
                        </Button>
                        <Button variant="ghost" size="sm" className="h-7 text-xs text-amber-400">
                          <RefreshCw className="w-3 h-3 mr-1" />重置
                        </Button>
                      </div>
                    </div>
                    <code className="block bg-zinc-950 border border-zinc-800 rounded p-2 text-xs text-zinc-400 font-mono break-all">
                      {user.subscription_url || `${window.location.origin}/api/v1/sub/${user.subscription_token || '...'}`}
                    </code>
                  </div>

                  <div>
                    <h4 className="text-sm font-medium text-zinc-300 mb-2">Token 列表</h4>
                    {user.subscriptions && user.subscriptions.length > 0 ? (
                      <div className="space-y-2">
                        {user.subscriptions.map((sub) => (
                          <div key={sub.id} className="flex items-center justify-between p-3 bg-zinc-800/50 rounded-lg border border-zinc-700/50">
                            <div className="flex-1 min-w-0">
                              <div className="flex items-center gap-2">
                                <code className="text-xs text-zinc-300 font-mono truncate">{sub.token.slice(0, 16)}...</code>
                                {sub.is_revoked ? (
                                  <Badge variant="outline" className="bg-red-900/30 text-red-400 border-red-800/50 text-[10px]">已吊销</Badge>
                                ) : (
                                  <Badge variant="outline" className="bg-emerald-900/30 text-emerald-400 border-emerald-800/50 text-[10px]">活跃</Badge>
                                )}
                              </div>
                              <div className="flex gap-3 text-xs text-zinc-500 mt-1">
                                <span>创建: {formatDate(sub.created_at)}</span>
                                {sub.last_used_at && <span>最后使用: {formatDate(sub.last_used_at)}</span>}
                              </div>
                            </div>
                            {!sub.is_revoked && (
                              <Button variant="ghost" size="sm" className="h-7 text-xs text-red-400">吊销</Button>
                            )}
                          </div>
                        ))}
                      </div>
                    ) : (
                      <div className="text-center py-6 text-sm text-zinc-500">暂无订阅Token</div>
                    )}
                  </div>
                </TabsContent>

                {/* Orders Tab */}
                <TabsContent value="orders" className="mt-0">
                  {user.orders && user.orders.length > 0 ? (
                    <div className="space-y-2">
                      {user.orders.map((order) => (
                        <div key={order.id} className="flex items-center justify-between p-3 bg-zinc-800/50 rounded-lg border border-zinc-700/50">
                          <div>
                            <div className="text-sm text-zinc-200">{order.plan_name}</div>
                            <div className="text-xs text-zinc-500">{formatDate(order.created_at)}</div>
                          </div>
                          <div className="text-right">
                            <div className="text-sm font-medium text-zinc-200">¥{order.amount}</div>
                            <Badge variant="outline" className={`text-[10px] ${
                              order.status === 'paid' ? 'bg-emerald-900/30 text-emerald-400 border-emerald-800/50' :
                              order.status === 'pending' ? 'bg-amber-900/30 text-amber-400 border-amber-800/50' :
                              'bg-red-900/30 text-red-400 border-red-800/50'
                            }`}>{order.status}</Badge>
                          </div>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <div className="text-center py-8 text-sm text-zinc-500">暂无订单记录</div>
                  )}
                </TabsContent>

                {/* Traffic Tab */}
                <TabsContent value="traffic" className="mt-0">
                  {user.traffic_logs && user.traffic_logs.length > 0 ? (
                    <div className="space-y-2">
                      <div className="grid grid-cols-4 gap-2 text-xs text-zinc-500 pb-2 border-b border-zinc-800 px-2">
                        <span>日期</span>
                        <span className="text-right">上传</span>
                        <span className="text-right">下载</span>
                        <span className="text-right">合计</span>
                      </div>
                      {user.traffic_logs.map((log) => (
                        <div key={log.id} className="grid grid-cols-4 gap-2 p-2 text-sm hover:bg-zinc-800/50 rounded">
                          <span className="text-zinc-400 text-xs">{log.date}</span>
                          <span className="text-right text-zinc-500 text-xs">{(log.upload / 1024 / 1024 / 1024).toFixed(2)} GB</span>
                          <span className="text-right text-zinc-500 text-xs">{(log.download / 1024 / 1024 / 1024).toFixed(2)} GB</span>
                          <span className="text-right text-zinc-300 text-xs font-medium">{(log.total / 1024 / 1024 / 1024).toFixed(2)} GB</span>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <div className="text-center py-8 text-sm text-zinc-500">暂无流量记录</div>
                  )}
                </TabsContent>

                {/* Logs Tab */}
                <TabsContent value="logs" className="mt-0">
                  {user.audit_logs && user.audit_logs.length > 0 ? (
                    <div className="space-y-2">
                      {user.audit_logs.map((log) => (
                        <div key={log.id} className="p-3 bg-zinc-800/50 rounded-lg border border-zinc-700/50">
                          <div className="flex items-center justify-between">
                            <span className="text-sm text-zinc-200">{log.action}</span>
                            <span className="text-xs text-zinc-500">{formatDate(log.created_at)}</span>
                          </div>
                          {(log.ip || log.user_agent) && (
                            <div className="flex gap-3 text-xs text-zinc-500 mt-1">
                              {log.ip && <span>IP: {log.ip}</span>}
                              {log.user_agent && <span className="truncate max-w-[300px]">{log.user_agent}</span>}
                            </div>
                          )}
                        </div>
                      ))}
                    </div>
                  ) : (
                    <div className="text-center py-8 text-sm text-zinc-500">暂无操作日志</div>
                  )}
                </TabsContent>
              </div>
            </Tabs>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
