import { useState, useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { Mail, Calendar, LogOut, Key, Loader2, Check, Shield, Eye, EyeOff, User, Bell, RefreshCw, AlertTriangle, Wallet, Banknote, History } from 'lucide-react'
import { Button, Input, Tabs, TabsList, TabsTrigger, TabsContent, Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter, Select } from '@airport/ui'
import { useToast } from '@/lib/toast'
import { api } from '@/lib/api'
import { EP, UserDetailResponse, SubscriptionTokenResponse, NotificationPreferences, WithdrawResponse, adaptUser, adaptSubscriptionToken, formatDateTime, formatCNY, formatUSDT } from '@/lib/endpoints'
import { useAuthStore } from '@/lib/auth'

const passwordSchema = z
  .object({
    old_password: z.string().min(6, '原密码至少6位'),
    new_password: z.string().min(6, '新密码至少6位'),
    confirm_password: z.string(),
  })
  .refine((data) => data.new_password === data.confirm_password, {
    message: '两次密码输入不一致',
    path: ['confirm_password'],
  })

type PasswordFormValues = z.infer<typeof passwordSchema>

function InfoCard({
  icon: Icon,
  label,
  value,
  valueColor,
}: {
  icon: any
  label: string
  value: string
  valueColor?: string
}) {
  return (
    <div className="flex items-center gap-4 p-4 rounded-xl" style={{ backgroundColor: 'var(--muted)' }}>
      <div className="w-10 h-10 rounded-lg flex items-center justify-center" style={{ background: 'rgba(124,92,252,0.1)' }}>
        <Icon className="w-5 h-5" style={{ color: 'var(--primary)' }} />
      </div>
      <div>
        <p className="text-xs mb-0.5" style={{ color: 'var(--muted-foreground)' }}>{label}</p>
        <p className="text-sm font-medium" style={{ color: valueColor || 'var(--foreground)' }}>{value}</p>
      </div>
    </div>
  )
}

function Toggle({ checked, onChange, disabled }: { checked: boolean; onChange: (v: boolean) => void; disabled?: boolean }) {
  return (
    <button
      onClick={() => !disabled && onChange(!checked)}
      disabled={disabled}
      className={`relative w-12 h-6 rounded-full transition-colors ${disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'}`}
      style={{ backgroundColor: checked ? 'var(--primary)' : 'var(--border)' }}
    >
      <span
        className={`absolute top-1 w-4 h-4 bg-white rounded-full transition-transform ${checked ? 'translate-x-7' : 'translate-x-1'}`}
      />
    </button>
  )
}

function getWithdrawStatusLabel(status: number): { label: string; color: string } {
  switch (status) {
    case 0: return { label: '待审核', color: '#f59e0b' }
    case 1: return { label: '已通过', color: 'var(--success)' }
    case 2: return { label: '已拒绝', color: 'var(--destructive)' }
    case 3: return { label: '已打款', color: 'var(--success)' }
    default: return { label: String(status), color: 'var(--muted-foreground)' }
  }
}

function getWithdrawMethodLabel(method: string): string {
  const labels: Record<string, string> = {
    alipay: '支付宝',
    usdt_trc20: 'USDT-TRC20',
    usdt_erc20: 'USDT-ERC20',
  }
  return labels[method] || method
}

export default function Profile() {
  const { user, logout } = useAuthStore()
  const { toast } = useToast()
  const queryClient = useQueryClient()
  const [changingPassword, setChangingPassword] = useState(false)
  const [showOldPassword, setShowOldPassword] = useState(false)
  const [showNewPassword, setShowNewPassword] = useState(false)
  const [showConfirmPassword, setShowConfirmPassword] = useState(false)
  const [activeTab, setActiveTab] = useState('profile')
  const [showResetDialog, setShowResetDialog] = useState(false)
  const [resetting, setResetting] = useState(false)
  const [withdrawDialogOpen, setWithdrawDialogOpen] = useState(false)
  const [withdrawMethod, setWithdrawMethod] = useState('alipay')
  const [withdrawAmount, setWithdrawAmount] = useState('')
  const [withdrawAccount, setWithdrawAccount] = useState('')
  const [withdrawRealName, setWithdrawRealName] = useState('')
  const [withdrawing, setWithdrawing] = useState(false)
  const [preferencesLoaded, setPreferencesLoaded] = useState(false)
  const [savingPrefs, setSavingPrefs] = useState(false)
  const [notifyExpiry, setNotifyExpiry] = useState(true)
  const [notifyTraffic, setNotifyTraffic] = useState(true)
  const [notifyTicketReply, setNotifyTicketReply] = useState(true)

  const { data: profile, isLoading } = useQuery<UserDetailResponse>({
    queryKey: ['profile'],
    queryFn: async () => {
      const raw = await api.get<UserDetailResponse>(EP.ME)
      return adaptUser(raw) as UserDetailResponse
    },
  })

  const { data: tokens = [] } = useQuery<SubscriptionTokenResponse[]>({
    queryKey: ['subscription-tokens'],
    queryFn: async () => {
      const rawTokens = await api.get<SubscriptionTokenResponse[]>(EP.SUBSCRIPTION_TOKENS)
      return rawTokens.map(adaptSubscriptionToken)
    },
  })

  const { data: notificationPrefs } = useQuery<NotificationPreferences>({
    queryKey: ['notification-preferences'],
    queryFn: () => api.get<NotificationPreferences>(EP.PREFERENCES),
    retry: false,
  })

  useEffect(() => {
    if (notificationPrefs && !preferencesLoaded) {
      setNotifyExpiry(notificationPrefs.notify_expiry ?? true)
      setNotifyTraffic(notificationPrefs.notify_traffic ?? true)
      setNotifyTicketReply(notificationPrefs.notify_ticket_reply ?? true)
      setPreferencesLoaded(true)
    }
  }, [notificationPrefs, preferencesLoaded])

  useEffect(() => {
    if (profile && !preferencesLoaded) {
      setNotifyExpiry(profile.notify_expiry ?? true)
      setNotifyTraffic(profile.notify_traffic ?? true)
      setNotifyTicketReply(profile.notify_ticket_reply ?? true)
    }
  }, [profile, preferencesLoaded])

  const { data: withdrawals = [] } = useQuery<WithdrawResponse[]>({
    queryKey: ['commission-withdrawals'],
    queryFn: async () => {
      try {
        const res = await api.get<{ items: WithdrawResponse[] } | WithdrawResponse[]>(EP.COMMISSION_WITHDRAWALS)
        return Array.isArray(res) ? res : (res.items || [])
      } catch { return [] }
    },
    retry: false,
  })

  const {
    register,
    handleSubmit,
    reset,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<PasswordFormValues>({
    defaultValues: { old_password: '', new_password: '', confirm_password: '' },
  })

  const savePrefs = async () => {
    setSavingPrefs(true)
    try {
      await api.put(EP.PREFERENCES, {
        notify_expiry: notifyExpiry,
        notify_traffic: notifyTraffic,
        notify_ticket_reply: notifyTicketReply,
      })
      toast({ title: '设置已保存', variant: 'success' })
      queryClient.invalidateQueries({ queryKey: ['profile'] })
      queryClient.invalidateQueries({ queryKey: ['me'] })
    } catch (e: any) {
      toast({ title: '保存失败', description: e.message, variant: 'destructive' })
    } finally {
      setSavingPrefs(false)
    }
  }

  const onSubmitPassword = async (data: PasswordFormValues) => {
    const result = passwordSchema.safeParse(data)
    if (!result.success) {
      result.error.issues.forEach((issue) => {
        setError(issue.path[0] as keyof PasswordFormValues, {
          type: 'manual',
          message: issue.message,
        })
      })
      return
    }

    try {
      await api.post(EP.AUTH_CHANGE_PASSWORD, {
        old_password: data.old_password,
        new_password: data.new_password,
      })
      toast({ title: '密码修改成功', variant: 'success' })
      setChangingPassword(false)
      reset()
    } catch (e: any) {
      toast({ title: '密码修改失败', description: e.message || '请检查原密码是否正确', variant: 'destructive' })
    }
  }

  const handleLogout = () => {
    logout()
    toast({ title: '已退出登录', variant: 'success' })
    window.location.href = '/login'
  }

  const handleResetSubscription = async () => {
    try {
      setResetting(true)
      if (tokens.length > 0) {
        for (const token of tokens) {
          await api.post(EP.SUBSCRIPTION_RESET_TOKEN(token.id))
        }
      } else {
        await api.post(EP.SUBSCRIPTION_RESET)
      }
      toast({ title: '重置成功', description: '订阅UUID已变更，请重新导入订阅链接', variant: 'success' })
      setShowResetDialog(false)
      queryClient.invalidateQueries({ queryKey: ['subscription-tokens'] })
      queryClient.invalidateQueries({ queryKey: ['profile'] })
      queryClient.invalidateQueries({ queryKey: ['me'] })
    } catch (e: any) {
      toast({ title: '重置失败', description: e.message || '请稍后重试', variant: 'destructive' })
    } finally {
      setResetting(false)
    }
  }

  const handleWithdraw = async () => {
    const amount = parseFloat(withdrawAmount)
    if (!withdrawAccount.trim()) {
      toast({ title: '请填写收款账户', variant: 'destructive' })
      return
    }
    if (withdrawMethod === 'alipay' && !withdrawRealName.trim()) {
      toast({ title: '请填写真实姓名', variant: 'destructive' })
      return
    }
    if (isNaN(amount) || amount <= 0) {
      toast({ title: '请输入有效金额', variant: 'destructive' })
      return
    }

    setWithdrawing(true)
    try {
      await api.post(EP.COMMISSION_WITHDRAW, {
        method: withdrawMethod,
        amount,
        account: withdrawAccount.trim(),
        real_name: withdrawMethod === 'alipay' ? withdrawRealName.trim() : undefined,
      })
      toast({ title: '提现申请已提交', description: '请等待审核', variant: 'success' })
      setWithdrawDialogOpen(false)
      setWithdrawAmount('')
      setWithdrawAccount('')
      setWithdrawRealName('')
      queryClient.invalidateQueries({ queryKey: ['commission-withdrawals'] })
      queryClient.invalidateQueries({ queryKey: ['profile'] })
      queryClient.invalidateQueries({ queryKey: ['me'] })
    } catch (e: any) {
      toast({ title: '提现失败', description: e.message, variant: 'destructive' })
    } finally {
      setWithdrawing(false)
    }
  }

  const displayUser = profile || user
  const balance = displayUser?.balance ?? 0
  const commissionBalance = displayUser?.commission_balance ?? 0

  if (isLoading || !displayUser) {
    return (
      <div className="p-6 max-w-3xl mx-auto">
        <div className="h-8 w-32 mb-5 rounded animate-pulse" style={{ backgroundColor: 'var(--muted)' }} />
        <div className="xboard-card p-6 mb-5">
          <div className="flex items-center gap-5">
            <div className="w-20 h-20 rounded-full animate-pulse" style={{ backgroundColor: 'var(--muted)' }} />
            <div className="flex-1">
              <div className="h-6 w-32 mb-2 rounded animate-pulse" style={{ backgroundColor: 'var(--muted)' }} />
              <div className="h-4 w-48 rounded animate-pulse" style={{ backgroundColor: 'var(--muted)' }} />
            </div>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="p-6 max-w-3xl mx-auto">
      <h1 className="text-xl font-bold mb-6" style={{ color: 'var(--foreground)' }}>个人中心</h1>

      <div className="xboard-card p-6 mb-5">
        <div className="flex items-center gap-5">
          <div
            className="w-20 h-20 rounded-full flex items-center justify-center text-white text-3xl font-bold shadow-sm"
            style={{ background: 'var(--primary)' }}
          >
            {displayUser.email.charAt(0).toUpperCase()}
          </div>
          <div className="flex-1">
            <h2 className="text-xl font-bold mb-1" style={{ color: 'var(--foreground)' }}>{displayUser.email}</h2>
            <p className="text-sm" style={{ color: 'var(--muted-foreground)' }}>
              ID: {displayUser.id} · 注册于 {formatDateTime(displayUser.created_at)}
            </p>
          </div>
        </div>
      </div>

      <Tabs value={activeTab} onValueChange={setActiveTab} className="mb-6">
        <TabsList className="border-b" style={{ borderColor: 'var(--border)', backgroundColor: 'transparent' }}>
          <TabsTrigger value="profile" className="data-[state=active]:border-b-2" style={{ color: activeTab === 'profile' ? 'var(--primary)' : 'var(--muted-foreground)', borderBottomColor: activeTab === 'profile' ? 'var(--primary)' : 'transparent' }}>
            <User className="w-4 h-4 mr-2" />
            个人资料
          </TabsTrigger>
          <TabsTrigger value="security" className="data-[state=active]:border-b-2" style={{ color: activeTab === 'security' ? 'var(--primary)' : 'var(--muted-foreground)', borderBottomColor: activeTab === 'security' ? 'var(--primary)' : 'transparent' }}>
            <Shield className="w-4 h-4 mr-2" />
            安全设置
          </TabsTrigger>
        </TabsList>

        <TabsContent value="profile">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-5">
            <InfoCard icon={Mail} label="邮箱" value={displayUser.email} />
            <InfoCard icon={Shield} label="邮箱验证" value={displayUser.email_verified ? '已验证' : '未验证'} />
            <InfoCard icon={Calendar} label="注册时间" value={formatDateTime(displayUser.created_at)} />
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-5">
            <InfoCard icon={Wallet} label="账户余额" value={formatCNY(balance)} />
            <InfoCard icon={Banknote} label="佣金余额" value={`${formatUSDT(commissionBalance)} USDT`} valueColor="var(--success)" />
          </div>

          <div className="xboard-card p-5 mb-5">
            <h3 className="text-base font-semibold mb-4 flex items-center gap-2" style={{ color: 'var(--foreground)' }}>
              <Bell className="w-4 h-4" style={{ color: 'var(--primary)' }} />
              通知设置
            </h3>
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm font-medium" style={{ color: 'var(--foreground)' }}>到期前提醒</p>
                  <p className="text-xs mt-0.5" style={{ color: 'var(--muted-foreground)' }}>订阅到期前发送邮件提醒</p>
                </div>
                <Toggle checked={notifyExpiry} onChange={setNotifyExpiry} />
              </div>
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm font-medium" style={{ color: 'var(--foreground)' }}>流量不足提醒</p>
                  <p className="text-xs mt-0.5" style={{ color: 'var(--muted-foreground)' }}>流量剩余不足时发送提醒</p>
                </div>
                <Toggle checked={notifyTraffic} onChange={setNotifyTraffic} />
              </div>
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm font-medium" style={{ color: 'var(--foreground)' }}>工单回复提醒</p>
                  <p className="text-xs mt-0.5" style={{ color: 'var(--muted-foreground)' }}>收到工单回复时发送提醒</p>
                </div>
                <Toggle checked={notifyTicketReply} onChange={setNotifyTicketReply} />
              </div>
            </div>
            <div className="mt-4 flex justify-end">
              <Button
                className="h-9 px-4 text-white border-0 text-sm shadow-sm"
                style={{ background: 'var(--primary)' }}
                onClick={savePrefs}
                disabled={savingPrefs}
                onMouseEnter={e => (e.currentTarget.style.opacity = '0.9')}
                onMouseLeave={e => (e.currentTarget.style.opacity = '1')}
              >
                {savingPrefs ? <Loader2 className="w-4 h-4 mr-1.5 animate-spin" /> : <Check className="w-4 h-4 mr-1.5" />}
                保存设置
              </Button>
            </div>
          </div>

          <div className="xboard-card p-5 mb-5">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-base font-semibold flex items-center gap-2" style={{ color: 'var(--foreground)' }}>
                <Banknote className="w-4 h-4" style={{ color: 'var(--primary)' }} />
                佣金提现
              </h3>
              <Button
                className="h-9 px-4 text-white border-0 text-sm shadow-sm"
                style={{ background: 'var(--primary)' }}
                onClick={() => setWithdrawDialogOpen(true)}
                disabled={commissionBalance <= 0}
                onMouseEnter={e => (e.currentTarget.style.opacity = '0.9')}
                onMouseLeave={e => (e.currentTarget.style.opacity = '1')}
              >
                <Wallet className="w-4 h-4 mr-1.5" />
                申请提现
              </Button>
            </div>
            <p className="text-sm mb-2" style={{ color: 'var(--muted-foreground)' }}>
              邀请好友获得的佣金可在此提现，支持支付宝和USDT-TRC20。
            </p>

            {withdrawals.length > 0 && (
              <div className="mt-4">
                <div className="flex items-center gap-2 mb-3">
                  <History className="w-4 h-4" style={{ color: 'var(--muted-foreground)' }} />
                  <span className="text-sm font-medium" style={{ color: 'var(--foreground)' }}>提现记录</span>
                </div>
                <div className="space-y-2">
                  {withdrawals.slice(0, 10).map(w => {
                    const statusInfo = getWithdrawStatusLabel(w.status)
                    return (
                      <div key={w.id} className="flex items-center justify-between p-3 rounded-lg" style={{ backgroundColor: 'var(--muted)' }}>
                        <div className="min-w-0">
                          <div className="flex items-center gap-2">
                            <span className="text-sm font-medium" style={{ color: 'var(--foreground)' }}>{formatUSDT(w.amount)} USDT</span>
                            <span className="text-xs px-1.5 py-0.5 rounded" style={{ backgroundColor: 'var(--card)', color: 'var(--muted-foreground)', border: '1px solid var(--border)' }}>
                              {getWithdrawMethodLabel(w.method)}
                            </span>
                          </div>
                          <p className="text-xs mt-0.5" style={{ color: 'var(--muted-foreground)' }}>{formatDateTime(w.created_at)}</p>
                        </div>
                        <span className="text-xs font-medium" style={{ color: statusInfo.color }}>{statusInfo.label}</span>
                      </div>
                    )
                  })}
                </div>
              </div>
            )}
          </div>
        </TabsContent>

        <TabsContent value="security">
          <div className="xboard-card p-5 mb-5">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-base font-semibold flex items-center gap-2" style={{ color: 'var(--foreground)' }}>
                <Key className="w-4 h-4" style={{ color: 'var(--primary)' }} />
                修改密码
              </h3>
              {!changingPassword && (
                <Button
                  variant="ghost"
                  onClick={() => setChangingPassword(true)}
                  className="text-sm h-8 px-3"
                  style={{ color: 'var(--primary)' }}
                  onMouseEnter={e => (e.currentTarget.style.background = 'rgba(124,92,252,0.1)')}
                  onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
                >
                  修改
                </Button>
              )}
            </div>

            {changingPassword ? (
              <form onSubmit={handleSubmit(onSubmitPassword)} className="space-y-4">
                <div>
                  <label className="block text-sm font-medium mb-1" style={{ color: 'var(--foreground)' }}>原密码</label>
                  <div className="relative">
                    <Input
                      type={showOldPassword ? 'text' : 'password'}
                      placeholder="请输入原密码"
                      className="h-10"
                      style={{ background: 'var(--card)', borderColor: 'var(--border)', color: 'var(--foreground)' }}
                      {...register('old_password', { required: '请输入原密码' })}
                    />
                    <button
                      type="button"
                      onClick={() => setShowOldPassword(!showOldPassword)}
                      className="absolute right-3 top-1/2 -translate-y-1/2 transition-colors"
                      style={{ color: 'var(--muted-foreground)' }}
                      onMouseEnter={e => (e.currentTarget.style.color = 'var(--foreground)')}
                      onMouseLeave={e => (e.currentTarget.style.color = 'var(--muted-foreground)')}
                    >
                      {showOldPassword ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                    </button>
                  </div>
                  {errors.old_password && <p className="text-sm mt-1" style={{ color: 'var(--destructive)' }}>{errors.old_password.message}</p>}
                </div>
                <div>
                  <label className="block text-sm font-medium mb-1" style={{ color: 'var(--foreground)' }}>新密码</label>
                  <div className="relative">
                    <Input
                      type={showNewPassword ? 'text' : 'password'}
                      placeholder="请输入新密码（至少6位）"
                      className="h-10"
                      style={{ background: 'var(--card)', borderColor: 'var(--border)', color: 'var(--foreground)' }}
                      {...register('new_password', { required: '请输入新密码' })}
                    />
                    <button
                      type="button"
                      onClick={() => setShowNewPassword(!showNewPassword)}
                      className="absolute right-3 top-1/2 -translate-y-1/2 transition-colors"
                      style={{ color: 'var(--muted-foreground)' }}
                      onMouseEnter={e => (e.currentTarget.style.color = 'var(--foreground)')}
                      onMouseLeave={e => (e.currentTarget.style.color = 'var(--muted-foreground)')}
                    >
                      {showNewPassword ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                    </button>
                  </div>
                  {errors.new_password && <p className="text-sm mt-1" style={{ color: 'var(--destructive)' }}>{errors.new_password.message}</p>}
                </div>
                <div>
                  <label className="block text-sm font-medium mb-1" style={{ color: 'var(--foreground)' }}>确认新密码</label>
                  <div className="relative">
                    <Input
                      type={showConfirmPassword ? 'text' : 'password'}
                      placeholder="请再次输入新密码"
                      className="h-10"
                      style={{ background: 'var(--card)', borderColor: 'var(--border)', color: 'var(--foreground)' }}
                      {...register('confirm_password', { required: '请确认密码' })}
                    />
                    <button
                      type="button"
                      onClick={() => setShowConfirmPassword(!showConfirmPassword)}
                      className="absolute right-3 top-1/2 -translate-y-1/2 transition-colors"
                      style={{ color: 'var(--muted-foreground)' }}
                      onMouseEnter={e => (e.currentTarget.style.color = 'var(--foreground)')}
                      onMouseLeave={e => (e.currentTarget.style.color = 'var(--muted-foreground)')}
                    >
                      {showConfirmPassword ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                    </button>
                  </div>
                  {errors.confirm_password && <p className="text-sm mt-1" style={{ color: 'var(--destructive)' }}>{errors.confirm_password.message}</p>}
                </div>
                <div className="flex gap-3">
                  <Button
                    type="submit"
                    disabled={isSubmitting}
                    className="text-white h-10 px-5 rounded-lg border-0 shadow-sm"
                    style={{ background: 'var(--primary)' }}
                    onMouseEnter={e => (e.currentTarget.style.opacity = '0.9')}
                    onMouseLeave={e => (e.currentTarget.style.opacity = '1')}
                  >
                    {isSubmitting ? (
                      <>
                        <Loader2 className="w-4 h-4 mr-2 animate-spin" />
                        提交中
                      </>
                    ) : (
                      <>
                        <Check className="w-4 h-4 mr-1.5" />
                        确认修改
                      </>
                    )}
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    onClick={() => {
                      setChangingPassword(false)
                      reset()
                    }}
                    className="h-10 px-5"
                    style={{ color: 'var(--muted-foreground)' }}
                    onMouseEnter={e => (e.currentTarget.style.background = 'var(--muted)')}
                    onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
                  >
                    取消
                  </Button>
                </div>
              </form>
            ) : (
              <p className="text-sm" style={{ color: 'var(--muted-foreground)' }}>定期修改密码可以提高账户安全性</p>
            )}
          </div>

          <div className="rounded-xl border p-5 mb-5" style={{ backgroundColor: 'rgba(239, 68, 68, 0.04)', borderColor: 'rgba(239, 68, 68, 0.2)' }}>
            <div className="flex items-start gap-3 mb-4">
              <div className="w-10 h-10 rounded-lg flex items-center justify-center flex-shrink-0" style={{ backgroundColor: 'rgba(239, 68, 68, 0.1)' }}>
                <AlertTriangle className="w-5 h-5" style={{ color: 'var(--destructive)' }} />
              </div>
              <div>
                <h3 className="text-base font-bold" style={{ color: 'var(--foreground)' }}>重置订阅</h3>
                <p className="text-sm mt-1" style={{ color: 'var(--muted-foreground)' }}>
                  如果您的账户信息或订阅已泄露，此选项用于重置您的UUID。重置后，原订阅链接将失效，需要重新导入新链接。
                </p>
              </div>
            </div>
            <Button
              className="h-10 px-5 text-white border-0"
              style={{ background: 'var(--destructive)' }}
              onClick={() => setShowResetDialog(true)}
              onMouseEnter={e => (e.currentTarget.style.opacity = '0.9')}
              onMouseLeave={e => (e.currentTarget.style.opacity = '1')}
            >
              <RefreshCw className="w-4 h-4 mr-2" />
              重置订阅
            </Button>
          </div>

          <div className="xboard-card p-5">
            <h3 className="text-base font-semibold mb-4" style={{ color: 'var(--foreground)' }}>账户操作</h3>
            <Button
              variant="ghost"
              onClick={handleLogout}
              className="h-10 px-5"
              style={{ color: 'var(--destructive)' }}
              onMouseEnter={e => (e.currentTarget.style.background = 'rgba(239,68,68,0.1)')}
              onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
            >
              <LogOut className="w-4 h-4 mr-1.5" />
              退出登录
            </Button>
          </div>
        </TabsContent>
      </Tabs>

      <Dialog open={showResetDialog} onOpenChange={setShowResetDialog}>
        <DialogContent className="xboard-card" style={{ background: 'var(--card)', borderColor: 'var(--border)' }}>
          <DialogHeader>
            <DialogTitle style={{ color: 'var(--foreground)' }}>确认重置订阅？</DialogTitle>
            <DialogDescription style={{ color: 'var(--muted-foreground)' }}>
              重置后您的订阅UUID将变更，所有客户端需要重新配置。此操作不可撤销。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="ghost"
              onClick={() => setShowResetDialog(false)}
              className="h-10 px-5"
              style={{ color: 'var(--muted-foreground)' }}
              onMouseEnter={e => (e.currentTarget.style.background = 'var(--muted)')}
              onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
            >
              取消
            </Button>
            <Button
              className="h-10 px-5 text-white border-0"
              style={{ background: 'var(--destructive)' }}
              onClick={handleResetSubscription}
              disabled={resetting}
              onMouseEnter={e => (e.currentTarget.style.opacity = '0.9')}
              onMouseLeave={e => (e.currentTarget.style.opacity = '1')}
            >
              {resetting ? (
                <>
                  <Loader2 className="w-4 h-4 mr-2 animate-spin" />
                  重置中
                </>
              ) : (
                '确认重置'
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={withdrawDialogOpen} onOpenChange={setWithdrawDialogOpen}>
        <DialogContent className="xboard-card" style={{ background: 'var(--card)', borderColor: 'var(--border)' }}>
          <DialogHeader>
            <DialogTitle style={{ color: 'var(--foreground)' }}>申请佣金提现</DialogTitle>
            <DialogDescription style={{ color: 'var(--muted-foreground)' }}>
              当前可提现余额：{formatUSDT(commissionBalance)} USDT
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 mt-4">
            <div>
              <label className="text-sm mb-1.5 block" style={{ color: 'var(--muted-foreground)' }}>提现方式</label>
              <Select
                value={withdrawMethod}
                onChange={(e) => setWithdrawMethod(e.target.value)}
                style={{ background: 'var(--card)', border: '1px solid var(--border)', color: 'var(--foreground)' }}
              >
                <option value="alipay">支付宝</option>
                <option value="usdt_trc20">USDT-TRC20</option>
              </Select>
            </div>
            <div>
              <label className="text-sm mb-1.5 block" style={{ color: 'var(--muted-foreground)' }}>提现金额 (USDT)</label>
              <Input
                type="number"
                placeholder="请输入金额"
                value={withdrawAmount}
                onChange={(e) => setWithdrawAmount(e.target.value)}
                style={{ background: 'var(--card)', borderColor: 'var(--border)', color: 'var(--foreground)' }}
              />
            </div>
            <div>
              <label className="text-sm mb-1.5 block" style={{ color: 'var(--muted-foreground)' }}>
                {withdrawMethod === 'alipay' ? '支付宝账号' : 'USDT钱包地址 (TRC20)'}
              </label>
              <Input
                placeholder={withdrawMethod === 'alipay' ? '请输入支付宝账号' : '请输入TRC20钱包地址'}
                value={withdrawAccount}
                onChange={(e) => setWithdrawAccount(e.target.value)}
                style={{ background: 'var(--card)', borderColor: 'var(--border)', color: 'var(--foreground)' }}
              />
            </div>
            {withdrawMethod === 'alipay' && (
              <div>
                <label className="text-sm mb-1.5 block" style={{ color: 'var(--muted-foreground)' }}>真实姓名</label>
                <Input
                  placeholder="请输入支付宝实名"
                  value={withdrawRealName}
                  onChange={(e) => setWithdrawRealName(e.target.value)}
                  style={{ background: 'var(--card)', borderColor: 'var(--border)', color: 'var(--foreground)' }}
                />
              </div>
            )}
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              className="h-10"
              style={{ borderColor: 'var(--border)', color: 'var(--muted-foreground)', backgroundColor: 'transparent' }}
              onClick={() => setWithdrawDialogOpen(false)}
            >
              取消
            </Button>
            <Button
              className="h-10 text-white border-0 shadow-sm"
              style={{ background: 'var(--primary)' }}
              onClick={handleWithdraw}
              disabled={withdrawing}
              onMouseEnter={e => (e.currentTarget.style.opacity = '0.9')}
              onMouseLeave={e => (e.currentTarget.style.opacity = '1')}
            >
              {withdrawing ? <Loader2 className="w-4 h-4 mr-2 animate-spin" /> : null}
              提交申请
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
