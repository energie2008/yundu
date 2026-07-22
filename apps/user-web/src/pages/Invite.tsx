import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useAuth } from '../lib/auth'
import { useToast } from '../lib/toast'
import { api } from '../lib/api'
import {
  EP, formatCNY, formatDateTime, CommissionSummary, WithdrawResponse,
  CommissionDetail, InvitationItem, PaginatedResponse,
} from '../lib/endpoints'

// 提现方式选项（对齐 xboard Dict::WITHDRAW_METHOD_WHITELIST_DEFAULT）
const WITHDRAW_METHODS = [
  { value: 'alipay', label: '支付宝', needRealName: true, placeholder: '请输入支付宝账号（邮箱或手机号）' },
  { value: 'usdt', label: 'USDT', needRealName: false, placeholder: '请输入 USDT TRC20 钱包地址' },
  { value: 'paypal', label: 'Paypal', needRealName: false, placeholder: '请输入 Paypal 邮箱' },
] as const

function CopyButton({ text, label = '复制' }: { text: string; label?: string }) {
  const [copied, setCopied] = useState(false)
  const { toast } = useToast()

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      toast({ title: '已复制', variant: 'success' })
      setTimeout(() => setCopied(false), 2000)
    } catch {
      toast({ title: '复制失败', variant: 'destructive' })
    }
  }

  return (
    <button
      onClick={copy}
      className="px-3 py-1.5 rounded-lg text-xs font-medium transition-colors"
      style={{ background: 'var(--primary)', color: 'white' }}
    >
      {copied ? '✓ 已复制' : label}
    </button>
  )
}

function StatCard({
  icon, label, value, subValue,
}: {
  icon: string; label: string; value: string | number; subValue?: string
}) {
  return (
    <div className="xboard-card p-5">
      <div className="flex items-center gap-3 mb-3">
        <div className="w-10 h-10 rounded-lg flex items-center justify-center text-lg" style={{ background: 'rgba(124,92,252,0.1)' }}>
          <span style={{ color: 'var(--primary)' }}>{icon}</span>
        </div>
        <span className="text-sm" style={{ color: 'var(--muted-foreground)' }}>{label}</span>
      </div>
      <p className="text-2xl font-bold" style={{ color: 'var(--foreground)' }}>{value}</p>
      {subValue && <p className="text-xs mt-1" style={{ color: 'var(--muted-foreground)' }}>{subValue}</p>}
    </div>
  )
}

// 提现状态映射
function getWithdrawStatusLabel(status: number): { text: string; color: string } {
  switch (status) {
    case 0: return { text: '审核中', color: '#f59e0b' }
    case 1: return { text: '已打款', color: '#22c55e' }
    case 2: return { text: '已拒绝', color: '#ef4444' }
    default: return { text: '未知', color: 'var(--muted-foreground)' }
  }
}

// 佣金状态映射
function getCommissionStatusLabel(status: number): { text: string; color: string } {
  switch (status) {
    case 0: return { text: '待结算', color: '#f59e0b' }
    case 1: return { text: '已结算', color: '#22c55e' }
    case 2: return { text: '已取消', color: '#ef4444' }
    default: return { text: '未知', color: 'var(--muted-foreground)' }
  }
}

function getMethodLabel(method: string): string {
  return WITHDRAW_METHODS.find(m => m.value === method)?.label || method
}

export default function Invite() {
  const { user } = useAuth()
  const { toast } = useToast()
  const qc = useQueryClient()

  // 邀请码
  const { data: inviteCodeData } = useQuery<{ code: string }>({
    queryKey: ['my-invite-code'],
    queryFn: () => api.get<{ code: string }>(EP.INVITE_CODE),
    retry: false,
  })
  const inviteCode = inviteCodeData?.code || '--------'
  const inviteLink = `${window.location.origin}/register?invite=${inviteCode}`

  // 佣金概览
  const { data: summary } = useQuery<CommissionSummary>({
    queryKey: ['commission-summary'],
    queryFn: async () => {
      try {
        return await api.get<CommissionSummary>(EP.COMMISSION_SUMMARY)
      } catch {
        return {
          available_balance: user?.commission_balance ?? 0,
          total_earned: user?.commission_total ?? 0,
          pending_settlement: 0,
          invited_count: 0,
          withdrawn_total: 0,
          rate: 20,
          min_withdraw: 10,
          withdraw_enabled: true,
        }
      }
    },
    retry: false,
  })

  // 提现记录
  const { data: withdrawalsResp } = useQuery<PaginatedResponse<WithdrawResponse>>({
    queryKey: ['commission-withdrawals'],
    queryFn: () => api.get<PaginatedResponse<WithdrawResponse>>(EP.COMMISSION_WITHDRAWALS),
    retry: false,
  })
  const withdrawals = withdrawalsResp?.items || []

  // 佣金明细
  const { data: detailsResp } = useQuery<PaginatedResponse<CommissionDetail>>({
    queryKey: ['commission-details'],
    queryFn: () => api.get<PaginatedResponse<CommissionDetail>>(EP.COMMISSION_DETAILS),
    retry: false,
  })
  const commissionDetails = detailsResp?.items || []

  // 邀请明细（被邀请用户列表）
  const { data: invitationsResp } = useQuery<PaginatedResponse<InvitationItem>>({
    queryKey: ['my-invitations'],
    queryFn: () => api.get<PaginatedResponse<InvitationItem>>(EP.INVITATIONS),
    retry: false,
  })
  const invitations = invitationsResp?.items || []

  // 提现表单状态
  const [withdrawForm, setWithdrawForm] = useState({
    amount: '',
    method: 'alipay' as string,
    account: '',
    real_name: '',
  })
  const [withdrawOpen, setWithdrawOpen] = useState(false)

  const withdrawMut = useMutation({
    mutationFn: () => api.post(EP.COMMISSION_WITHDRAW, {
      amount: parseFloat(withdrawForm.amount),
      method: withdrawForm.method,
      account: withdrawForm.account,
      real_name: withdrawForm.real_name,
    }),
    onSuccess: () => {
      toast({ title: '提现申请已提交', variant: 'success' })
      setWithdrawForm({ amount: '', method: 'alipay', account: '', real_name: '' })
      setWithdrawOpen(false)
      qc.invalidateQueries({ queryKey: ['commission-withdrawals'] })
      qc.invalidateQueries({ queryKey: ['commission-summary'] })
    },
    onError: (err: any) => {
      toast({ title: err?.message || '提现失败', variant: 'destructive' })
    },
  })

  const invitedCount = summary?.invited_count ?? 0
  const totalEarned = summary?.total_earned ?? user?.commission_total ?? 0
  const availableBalance = summary?.available_balance ?? user?.commission_balance ?? 0
  const pendingSettlement = summary?.pending_settlement ?? 0
  const withdrawnTotal = summary?.withdrawn_total ?? 0
  const rate = summary?.rate ?? 20
  const minWithdraw = summary?.min_withdraw ?? 10
  const withdrawEnabled = summary?.withdraw_enabled ?? true

  const handleShare = async () => {
    if (navigator.share) {
      try {
        await navigator.share({
          title: '邀请好友加入',
          text: `使用我的邀请码 ${inviteCode} 注册，双方均可获得奖励！佣金比例 ${rate}%`,
          url: inviteLink,
        })
      } catch {
        toast({ title: '已复制邀请链接' })
      }
    } else {
      await navigator.clipboard.writeText(inviteLink)
      toast({ title: '邀请链接已复制', variant: 'success' })
    }
  }

  const handleWithdrawSubmit = () => {
    const amount = parseFloat(withdrawForm.amount)
    if (!amount || amount <= 0) {
      toast({ title: '请输入有效金额', variant: 'destructive' })
      return
    }
    if (amount < minWithdraw) {
      toast({ title: `最低提现 ${formatCNY(minWithdraw)}`, variant: 'destructive' })
      return
    }
    if (amount > availableBalance) {
      toast({ title: '余额不足', variant: 'destructive' })
      return
    }
    if (!withdrawForm.account.trim()) {
      toast({ title: '请填写提现账号', variant: 'destructive' })
      return
    }
    const methodCfg = WITHDRAW_METHODS.find(m => m.value === withdrawForm.method)
    if (methodCfg?.needRealName && !withdrawForm.real_name.trim()) {
      toast({ title: '请填写真实姓名', variant: 'destructive' })
      return
    }
    withdrawMut.mutate()
  }

  // 当前 tab
  const [tab, setTab] = useState<'invitees' | 'commissions' | 'withdrawals'>('invitees')

  return (
    <div className="p-6 max-w-5xl mx-auto">
      {/* Page title */}
      <div className="mb-6">
        <h1 className="text-xl font-bold mb-1" style={{ color: 'var(--foreground)' }}>
          🎁 邀请返利
        </h1>
        <p className="text-sm" style={{ color: 'var(--muted-foreground)' }}>
          邀请好友注册，好友消费后您可获得 {rate}% 佣金奖励，支持人民币提现
        </p>
      </div>

      {/* Stats cards */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
        <StatCard
          icon="👥"
          label="已邀请人数"
          value={invitedCount}
          subValue="好友成功注册后统计"
        />
        <StatCard
          icon="💰"
          label="累计获得佣金"
          value={formatCNY(totalEarned)}
          subValue="好友消费后发放"
        />
        <StatCard
          icon="💳"
          label="可提现余额"
          value={formatCNY(availableBalance)}
          subValue={`最低提现 ${formatCNY(minWithdraw)}`}
        />
        <StatCard
          icon="📊"
          label="佣金比例"
          value={`${rate}%`}
          subValue={`已提现 ${formatCNY(withdrawnTotal)}`}
        />
      </div>

      {/* Invite code & link */}
      <div className="xboard-card p-6 mb-6">
        <h2 className="text-base font-semibold mb-4" style={{ color: 'var(--foreground)' }}>
          🔗 我的邀请码
        </h2>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-5">
          <div className="rounded-lg p-4" style={{ backgroundColor: 'var(--muted)' }}>
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs" style={{ color: 'var(--muted-foreground)' }}>邀请码</span>
              <CopyButton text={inviteCode} />
            </div>
            <p className="text-xl font-bold font-mono tracking-widest" style={{ color: 'var(--primary)' }}>
              {inviteCode}
            </p>
          </div>
          <div className="rounded-lg p-4" style={{ backgroundColor: 'var(--muted)' }}>
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs" style={{ color: 'var(--muted-foreground)' }}>邀请链接</span>
              <CopyButton text={inviteLink} />
            </div>
            <p className="text-sm font-mono truncate" style={{ color: 'var(--foreground)' }}>
              {inviteLink}
            </p>
          </div>
        </div>
        <button
          onClick={handleShare}
          className="w-full py-3 rounded-lg text-sm font-medium text-white transition-opacity"
          style={{ background: 'var(--primary)' }}
          onMouseEnter={e => (e.currentTarget.style.opacity = '0.9')}
          onMouseLeave={e => (e.currentTarget.style.opacity = '1')}
        >
          📤 分享邀请链接
        </button>
      </div>

      {/* Withdrawal section */}
      <div className="xboard-card p-6 mb-6">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-base font-semibold" style={{ color: 'var(--foreground)' }}>
            💸 佣金提现
          </h2>
          {withdrawEnabled && (
            <button
              onClick={() => setWithdrawOpen(!withdrawOpen)}
              className="px-4 py-2 rounded-lg text-sm font-medium text-white transition-opacity"
              style={{ background: 'var(--primary)' }}
            >
              {withdrawOpen ? '取消提现' : '申请提现'}
            </button>
          )}
        </div>

        {!withdrawEnabled ? (
          <p className="text-sm text-center py-4" style={{ color: 'var(--muted-foreground)' }}>
            提现功能暂未开启
          </p>
        ) : withdrawOpen ? (
          <div className="space-y-4">
            {/* 提现表单 */}
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label className="text-xs mb-1 block" style={{ color: 'var(--muted-foreground)' }}>
                  提现金额（人民币 ¥）
                </label>
                <input
                  type="number"
                  value={withdrawForm.amount}
                  onChange={e => setWithdrawForm({ ...withdrawForm, amount: e.target.value })}
                  placeholder={`最低 ${formatCNY(minWithdraw)}`}
                  className="w-full px-3 py-2.5 rounded-lg text-sm border outline-none"
                  style={{ background: 'var(--background)', borderColor: 'var(--border)', color: 'var(--foreground)' }}
                />
                <p className="text-xs mt-1" style={{ color: 'var(--muted-foreground)' }}>
                  可用余额：{formatCNY(availableBalance)}
                </p>
              </div>
              <div>
                <label className="text-xs mb-1 block" style={{ color: 'var(--muted-foreground)' }}>
                  提现方式
                </label>
                <select
                  value={withdrawForm.method}
                  onChange={e => setWithdrawForm({ ...withdrawForm, method: e.target.value, real_name: '' })}
                  className="w-full px-3 py-2.5 rounded-lg text-sm border outline-none"
                  style={{ background: 'var(--background)', borderColor: 'var(--border)', color: 'var(--foreground)' }}
                >
                  {WITHDRAW_METHODS.map(m => (
                    <option key={m.value} value={m.value}>{m.label}</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="text-xs mb-1 block" style={{ color: 'var(--muted-foreground)' }}>
                  提现账号
                </label>
                <input
                  type="text"
                  value={withdrawForm.account}
                  onChange={e => setWithdrawForm({ ...withdrawForm, account: e.target.value })}
                  placeholder={WITHDRAW_METHODS.find(m => m.value === withdrawForm.method)?.placeholder || ''}
                  className="w-full px-3 py-2.5 rounded-lg text-sm border outline-none"
                  style={{ background: 'var(--background)', borderColor: 'var(--border)', color: 'var(--foreground)' }}
                />
              </div>
              {WITHDRAW_METHODS.find(m => m.value === withdrawForm.method)?.needRealName && (
                <div>
                  <label className="text-xs mb-1 block" style={{ color: 'var(--muted-foreground)' }}>
                    真实姓名
                  </label>
                  <input
                    type="text"
                    value={withdrawForm.real_name}
                    onChange={e => setWithdrawForm({ ...withdrawForm, real_name: e.target.value })}
                    placeholder="请输入支付宝实名认证姓名"
                    className="w-full px-3 py-2.5 rounded-lg text-sm border outline-none"
                    style={{ background: 'var(--background)', borderColor: 'var(--border)', color: 'var(--foreground)' }}
                  />
                </div>
              )}
            </div>
            <div className="flex items-center gap-3">
              <button
                onClick={handleWithdrawSubmit}
                disabled={withdrawMut.isPending}
                className="px-6 py-2.5 rounded-lg text-sm font-medium text-white transition-opacity disabled:opacity-50"
                style={{ background: 'var(--primary)' }}
              >
                {withdrawMut.isPending ? '提交中...' : '确认提现'}
              </button>
              <span className="text-xs" style={{ color: 'var(--muted-foreground)' }}>
                提现申请提交后，管理员将在 1-3 个工作日内审核打款
              </span>
            </div>
          </div>
        ) : (
          <div className="grid grid-cols-3 gap-4">
            <div className="text-center">
              <p className="text-xs mb-1" style={{ color: 'var(--muted-foreground)' }}>可提现</p>
              <p className="text-lg font-bold" style={{ color: 'var(--primary)' }}>{formatCNY(availableBalance)}</p>
            </div>
            <div className="text-center">
              <p className="text-xs mb-1" style={{ color: 'var(--muted-foreground)' }}>待结算</p>
              <p className="text-lg font-bold" style={{ color: '#f59e0b' }}>{formatCNY(pendingSettlement)}</p>
            </div>
            <div className="text-center">
              <p className="text-xs mb-1" style={{ color: 'var(--muted-foreground)' }}>已提现</p>
              <p className="text-lg font-bold" style={{ color: 'var(--foreground)' }}>{formatCNY(withdrawnTotal)}</p>
            </div>
          </div>
        )}
      </div>

      {/* Details tabs: invited users / commission logs / withdrawal history */}
      <div className="xboard-card overflow-hidden mb-6">
        {/* Tab bar */}
        <div className="flex border-b" style={{ borderColor: 'var(--border)' }}>
          {([
            { key: 'invitees', label: `邀请用户 (${invitations.length})` },
            { key: 'commissions', label: `佣金明细 (${commissionDetails.length})` },
            { key: 'withdrawals', label: `提现记录 (${withdrawals.length})` },
          ] as const).map(t => (
            <button
              key={t.key}
              onClick={() => setTab(t.key)}
              className="px-5 py-3 text-sm font-medium transition-colors"
              style={{
                color: tab === t.key ? 'var(--primary)' : 'var(--muted-foreground)',
                borderBottom: tab === t.key ? '2px solid var(--primary)' : '2px solid transparent',
              }}
            >
              {t.label}
            </button>
          ))}
        </div>

        {/* Tab content */}
        <div className="max-h-[400px] overflow-y-auto">
          {tab === 'invitees' && (
            invitations.length === 0 ? (
              <div className="p-8 text-center text-sm" style={{ color: 'var(--muted-foreground)' }}>
                暂无邀请记录，快去分享邀请链接吧
              </div>
            ) : (
              invitations.map(inv => (
                <div
                  key={inv.id}
                  className="flex items-center gap-3 px-5 py-3 border-b text-sm"
                  style={{ borderColor: 'var(--border)' }}
                >
                  <div className="w-8 h-8 rounded-full flex items-center justify-center text-xs font-medium"
                    style={{ background: 'rgba(124,92,252,0.1)', color: 'var(--primary)' }}>
                    {inv.email[0]?.toUpperCase()}
                  </div>
                  <div className="flex-1 min-w-0">
                    <p className="font-medium truncate" style={{ color: 'var(--foreground)' }}>{inv.email}</p>
                    <p className="text-xs" style={{ color: 'var(--muted-foreground)' }}>
                      {inv.registered_at ? `注册于 ${formatDateTime(inv.registered_at)}` : formatDateTime(inv.created_at)}
                    </p>
                  </div>
                  <span
                    className="text-xs px-2 py-0.5 rounded-full"
                    style={{
                      background: inv.email_verified ? 'rgba(34,197,94,0.1)' : 'rgba(245,158,11,0.1)',
                      color: inv.email_verified ? '#22c55e' : '#f59e0b',
                    }}
                  >
                    {inv.email_verified ? '已验证' : '待验证'}
                  </span>
                </div>
              ))
            )
          )}

          {tab === 'commissions' && (
            commissionDetails.length === 0 ? (
              <div className="p-8 text-center text-sm" style={{ color: 'var(--muted-foreground)' }}>
                暂无佣金记录，好友消费后将在此显示
              </div>
            ) : (
              <table className="w-full text-sm">
                <thead>
                  <tr style={{ background: 'var(--muted)' }}>
                    <th className="text-left px-5 py-2 text-xs font-medium" style={{ color: 'var(--muted-foreground)' }}>时间</th>
                    <th className="text-right px-5 py-2 text-xs font-medium" style={{ color: 'var(--muted-foreground)' }}>订单金额</th>
                    <th className="text-right px-5 py-2 text-xs font-medium" style={{ color: 'var(--muted-foreground)' }}>获得佣金</th>
                    <th className="text-center px-5 py-2 text-xs font-medium" style={{ color: 'var(--muted-foreground)' }}>状态</th>
                  </tr>
                </thead>
                <tbody>
                  {commissionDetails.map(d => {
                    const st = getCommissionStatusLabel(d.status)
                    return (
                      <tr key={d.id} className="border-b" style={{ borderColor: 'var(--border)' }}>
                        <td className="px-5 py-3 text-xs" style={{ color: 'var(--muted-foreground)' }}>
                          {formatDateTime(d.created_at)}
                        </td>
                        <td className="px-5 py-3 text-right" style={{ color: 'var(--foreground)' }}>
                          {formatCNY(d.order_amount)}
                        </td>
                        <td className="px-5 py-3 text-right font-medium" style={{ color: 'var(--primary)' }}>
                          +{formatCNY(d.get_amount)}
                        </td>
                        <td className="px-5 py-3 text-center">
                          <span className="text-xs px-2 py-0.5 rounded-full" style={{ background: `${st.color}15`, color: st.color }}>
                            {st.text}
                          </span>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            )
          )}

          {tab === 'withdrawals' && (
            withdrawals.length === 0 ? (
              <div className="p-8 text-center text-sm" style={{ color: 'var(--muted-foreground)' }}>
                暂无提现记录
              </div>
            ) : (
              <table className="w-full text-sm">
                <thead>
                  <tr style={{ background: 'var(--muted)' }}>
                    <th className="text-left px-5 py-2 text-xs font-medium" style={{ color: 'var(--muted-foreground)' }}>时间</th>
                    <th className="text-right px-5 py-2 text-xs font-medium" style={{ color: 'var(--muted-foreground)' }}>金额</th>
                    <th className="text-center px-5 py-2 text-xs font-medium" style={{ color: 'var(--muted-foreground)' }}>方式</th>
                    <th className="text-center px-5 py-2 text-xs font-medium" style={{ color: 'var(--muted-foreground)' }}>状态</th>
                  </tr>
                </thead>
                <tbody>
                  {withdrawals.map(w => {
                    const st = getWithdrawStatusLabel(w.status)
                    return (
                      <tr key={w.id} className="border-b" style={{ borderColor: 'var(--border)' }}>
                        <td className="px-5 py-3 text-xs" style={{ color: 'var(--muted-foreground)' }}>
                          {formatDateTime(w.created_at)}
                        </td>
                        <td className="px-5 py-3 text-right font-medium" style={{ color: 'var(--foreground)' }}>
                          {formatCNY(w.amount)}
                        </td>
                        <td className="px-5 py-3 text-center" style={{ color: 'var(--foreground)' }}>
                          {getMethodLabel(w.method)}
                        </td>
                        <td className="px-5 py-3 text-center">
                          <span className="text-xs px-2 py-0.5 rounded-full" style={{ background: `${st.color}15`, color: st.color }}>
                            {st.text}
                          </span>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            )
          )}
        </div>
      </div>

      {/* Invite rules */}
      <div className="xboard-card p-6">
        <h2 className="text-base font-semibold mb-4" style={{ color: 'var(--foreground)' }}>
          📋 邀请规则
        </h2>
        <div className="space-y-3">
          {[
            { step: '1', title: '分享邀请', desc: '将邀请码或邀请链接分享给好友' },
            { step: '2', title: '好友注册', desc: '好友使用您的邀请码成功注册' },
            { step: '3', title: '获得佣金', desc: `好友完成消费后，您将获得 ${rate}% 佣金奖励，佣金以人民币计算` },
            { step: '4', title: '申请提现', desc: `佣金余额达到 ${formatCNY(minWithdraw)} 即可申请提现，支持支付宝/USDT/Paypal` },
          ].map(item => (
            <div key={item.step} className="flex items-start gap-3 p-3 rounded-lg" style={{ backgroundColor: 'rgba(124,92,252,0.06)' }}>
              <div className="w-6 h-6 rounded-full flex items-center justify-center flex-shrink-0 mt-0.5"
                style={{ background: 'rgba(124,92,252,0.1)' }}>
                <span className="text-xs font-bold" style={{ color: 'var(--primary)' }}>{item.step}</span>
              </div>
              <div>
                <p className="text-sm font-medium" style={{ color: 'var(--foreground)' }}>{item.title}</p>
                <p className="text-xs mt-0.5" style={{ color: 'var(--muted-foreground)' }}>{item.desc}</p>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
