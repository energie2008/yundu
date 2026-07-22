import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Wallet, Check, X, Loader2, RefreshCw } from 'lucide-react'
import {
  Card,
  CardContent,
  Button,
  Select,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  Skeleton,
  EmptyState,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  Label,
  Textarea,
  useToast,
  Badge,
} from '@airport/ui'
import { api } from '@/lib/api'
import { EP } from '@/lib/endpoints'

interface Withdrawal {
  id: string
  user_id: string
  amount: number
  method: string
  account: string
  real_name?: string
  status: number
  remark?: string
  handled_by?: string
  handled_at?: string
  created_at: string
  updated_at: string
}

interface WithdrawalsResp {
  items: Withdrawal[]
  total: number
}

const STATUS_MAP: Record<number, { label: string; color: string }> = {
  0: { label: '待处理', color: 'bg-amber-500/20 text-amber-300 border-amber-500/30' },
  1: { label: '已打款', color: 'bg-emerald-500/20 text-emerald-300 border-emerald-500/30' },
  2: { label: '已拒绝', color: 'bg-red-500/20 text-red-300 border-red-500/30' },
}

const METHOD_MAP: Record<string, string> = {
  alipay: '支付宝',
  usdt_trc20: 'USDT-TRC20',
}

function formatTime(s: string): string {
  if (!s) return '-'
  try {
    return new Date(s).toLocaleString('zh-CN', { hour12: false })
  } catch {
    return s
  }
}

export default function Commissions() {
  const { toast } = useToast()
  const qc = useQueryClient()
  const [statusFilter, setStatusFilter] = useState<string>('')
  const [actionDialog, setActionDialog] = useState<{ open: boolean; id: string; approve: boolean }>({
    open: false,
    id: '',
    approve: true,
  })
  const [remark, setRemark] = useState('')

  const { data, isLoading, isFetching, refetch } = useQuery<WithdrawalsResp>({
    queryKey: ['admin-commission-withdrawals', statusFilter],
    queryFn: () =>
      api.get<WithdrawalsResp>(EP.COMMISSION_WITHDRAWALS, {
        params: { status: statusFilter || undefined },
      }),
    retry: false,
  })

  const processMutation = useMutation({
    mutationFn: async ({ id, approve }: { id: string; approve: boolean }) => {
      const endpoint = approve
        ? EP.COMMISSION_WITHDRAWAL_APPROVE(id)
        : EP.COMMISSION_WITHDRAWAL_REJECT(id)
      return api.post<Withdrawal>(endpoint, { remark })
    },
    onSuccess: (_data, vars) => {
      toast({
        title: vars.approve ? '已打款' : '已拒绝',
        variant: 'success',
      })
      setActionDialog({ open: false, id: '', approve: true })
      setRemark('')
      qc.invalidateQueries({ queryKey: ['admin-commission-withdrawals'] })
    },
    onError: (err: unknown) => {
      toast({
        title: '操作失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    },
  })

  const items = data?.items ?? []

  const openApprove = (id: string) => {
    setRemark('')
    setActionDialog({ open: true, id, approve: true })
  }
  const openReject = (id: string) => {
    setRemark('')
    setActionDialog({ open: true, id, approve: false })
  }

  const submitAction = () => {
    if (!actionDialog.id) return
    processMutation.mutate({ id: actionDialog.id, approve: actionDialog.approve })
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between flex-wrap gap-3">
        <h2 className="text-lg font-semibold text-zinc-100 flex items-center gap-2">
          <Wallet className="w-5 h-5 text-indigo-400" />
          返利提现管理
        </h2>
        <div className="flex items-center gap-2">
          <Select
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value)}
            className="bg-zinc-800 border-zinc-700 text-zinc-100 w-40"
          >
            <option value="">全部状态</option>
            <option value="0">待处理</option>
            <option value="1">已打款</option>
            <option value="2">已拒绝</option>
          </Select>
          <Button
            variant="outline"
            size="sm"
            onClick={() => refetch()}
            className="border-zinc-700 text-zinc-300"
          >
            <RefreshCw className={`w-4 h-4 mr-1 ${isFetching ? 'animate-spin' : ''}`} />
            刷新
          </Button>
        </div>
      </div>

      <Card className="bg-zinc-900 border-zinc-800">
        <CardContent className="p-0">
          {isLoading ? (
            <div className="p-4 space-y-3">
              <Skeleton className="h-8 w-full bg-zinc-800 rounded" />
              <Skeleton className="h-8 w-full bg-zinc-800 rounded" />
              <Skeleton className="h-8 w-full bg-zinc-800 rounded" />
            </div>
          ) : items.length === 0 ? (
            <EmptyState title="暂无提现记录" description="用户发起提现后将显示在此处" className="py-12" />
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="border-zinc-800 hover:bg-transparent">
                    <TableHead className="text-zinc-400 text-xs font-medium">用户UID</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium">金额</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium">方式</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium">收款账号</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium hidden md:table-cell">真实姓名</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium">状态</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium hidden lg:table-cell">备注</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium hidden md:table-cell">申请时间</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((w) => {
                    const st = STATUS_MAP[w.status] ?? STATUS_MAP[0]
                    return (
                      <TableRow key={w.id} className="border-zinc-800 hover:bg-zinc-800/50">
                        <TableCell className="py-3 text-xs text-zinc-400 font-mono truncate max-w-[120px]" title={w.user_id}>
                          {w.user_id.slice(0, 8)}...
                        </TableCell>
                        <TableCell className="py-3 text-sm font-semibold text-emerald-300">
                          ${Number(w.amount).toFixed(2)}
                        </TableCell>
                        <TableCell className="py-3 text-sm text-zinc-300">
                          {METHOD_MAP[w.method] ?? w.method}
                        </TableCell>
                        <TableCell className="py-3 text-sm text-zinc-300 font-mono truncate max-w-[160px]" title={w.account}>
                          {w.account}
                        </TableCell>
                        <TableCell className="py-3 text-sm text-zinc-300 hidden md:table-cell">
                          {w.real_name || '-'}
                        </TableCell>
                        <TableCell className="py-3">
                          <Badge variant="secondary" className={`text-xs border ${st.color}`}>
                            {st.label}
                          </Badge>
                        </TableCell>
                        <TableCell className="py-3 text-xs text-zinc-400 hidden lg:table-cell max-w-[160px] truncate" title={w.remark || ''}>
                          {w.remark || '-'}
                        </TableCell>
                        <TableCell className="py-3 text-xs text-zinc-400 hidden md:table-cell">
                          {formatTime(w.created_at)}
                        </TableCell>
                        <TableCell className="py-3">
                          {w.status === 0 ? (
                            <div className="flex items-center gap-1">
                              <Button
                                variant="ghost"
                                size="icon"
                                className="h-8 w-8 text-emerald-400 hover:text-emerald-300"
                                onClick={() => openApprove(w.id)}
                                title="批准打款"
                              >
                                <Check className="w-4 h-4" />
                              </Button>
                              <Button
                                variant="ghost"
                                size="icon"
                                className="h-8 w-8 text-red-400 hover:text-red-300"
                                onClick={() => openReject(w.id)}
                                title="拒绝并退款"
                              >
                                <X className="w-4 h-4" />
                              </Button>
                            </div>
                          ) : (
                            <span className="text-xs text-zinc-500">已处理</span>
                          )}
                        </TableCell>
                      </TableRow>
                    )
                  })}
                </TableBody>
              </Table>
            </div>
          )}
          {data && (
            <div className="px-4 py-3 text-xs text-zinc-500 border-t border-zinc-800">
              共 {data.total} 条记录
            </div>
          )}
        </CardContent>
      </Card>

      <Dialog
        open={actionDialog.open}
        onOpenChange={(open) => setActionDialog({ ...actionDialog, open })}
      >
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              {actionDialog.approve ? (
                <Check className="w-5 h-5 text-emerald-400" />
              ) : (
                <X className="w-5 h-5 text-red-400" />
              )}
              <span>{actionDialog.approve ? '确认打款' : '拒绝提现'}</span>
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-3 pt-1">
            <p className="text-sm text-zinc-400">
              {actionDialog.approve
                ? '确认已通过线下渠道向用户打款。此操作不可撤销。'
                : '拒绝后，提现金额将退还到用户的佣金余额。此操作不可撤销。'}
            </p>
            <div className="space-y-2">
              <Label htmlFor="remark" className="text-zinc-300">
                备注 {actionDialog.approve ? '(可选)' : '(建议填写原因)'}
              </Label>
              <Textarea
                id="remark"
                rows={3}
                placeholder={actionDialog.approve ? '如：已通过支付宝打款' : '如：收款信息有误'}
                value={remark}
                onChange={(e) => setRemark(e.target.value)}
                className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500"
              />
            </div>
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setActionDialog({ ...actionDialog, open: false })}
              className="border-zinc-700 text-zinc-300"
            >
              取消
            </Button>
            <Button
              className={
                actionDialog.approve
                  ? 'bg-emerald-600 hover:bg-emerald-500'
                  : 'bg-red-600 hover:bg-red-500'
              }
              onClick={submitAction}
              disabled={processMutation.isPending}
            >
              {processMutation.isPending ? (
                <Loader2 className="w-4 h-4 mr-1 animate-spin" />
              ) : actionDialog.approve ? (
                <Check className="w-4 h-4 mr-1" />
              ) : (
                <X className="w-4 h-4 mr-1" />
              )}
              {actionDialog.approve ? '确认打款' : '拒绝并退款'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
