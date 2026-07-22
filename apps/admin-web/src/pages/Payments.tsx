import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  CreditCard,
  RefreshCw,
  Save,
  Power,
  CheckCircle,
  XCircle,
} from 'lucide-react'
import {
  Card,
  CardContent,
  Button,
  Input,
  Badge,
  Skeleton,
} from '@airport/ui'
import {
  ADMIN_CARD,
  ADMIN_BORDER,
  ADMIN_TEXT,
  ADMIN_TEXT_SECONDARY,
  ADMIN_TEXT_MUTED,
  ADMIN_INPUT_BG,
  ADMIN_INPUT_BORDER,
} from '@/lib/theme'
import { api } from '@/lib/api'
import { EP } from '@/lib/endpoints'

interface PaymentMethod {
  method: string
  name: string
  enabled: boolean
  address: string
  amount_tolerance: number
  confirmations: number
  network: string
  auto_activate: boolean
}

interface PaymentMethodsResponse {
  methods: PaymentMethod[]
}

export default function Payments() {
  const queryClient = useQueryClient()
  const [editMethod, setEditMethod] = useState<string | null>(null)
  const [editForm, setEditForm] = useState<Partial<PaymentMethod>>({})
  const [rateInput, setRateInput] = useState('')

  const { data, isLoading, isFetching } = useQuery<PaymentMethodsResponse>({
    queryKey: ['payment-methods'],
    queryFn: async () => {
      const raw = await api.get<PaymentMethodsResponse>(EP.PAYMENT_METHODS)
      return raw
    },
    retry: false,
  })

  const { data: rateData } = useQuery<{ usdt_to_cny: number; auto_update?: boolean; last_updated?: string }>({
    queryKey: ['exchange-rate'],
    queryFn: async () => {
      return api.get(EP.PAYMENT_EXCHANGE_RATE)
    },
    retry: false,
  })

  const updateRate = useMutation({
    mutationFn: async (rate: number) => {
      return api.put(EP.PAYMENT_EXCHANGE_RATE, { usdt_to_cny: rate })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['exchange-rate'] })
    },
  })

  const updateMethod = useMutation({
    mutationFn: async ({ method, config }: { method: string; config: Partial<PaymentMethod> }) => {
      return api.put(EP.PAYMENT_METHOD_DETAIL(method), {
        enabled: config.enabled,
        address: config.address,
        amount_tolerance: config.amount_tolerance,
        confirmations: config.confirmations,
        auto_activate: config.auto_activate,
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['payment-methods'] })
      setEditMethod(null)
    },
  })

  const toggleMethod = useMutation({
    mutationFn: async (method: string) => {
      return api.post(EP.PAYMENT_METHOD_TOGGLE(method), {})
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['payment-methods'] })
    },
  })

  const handleRefresh = () => {
    queryClient.invalidateQueries({ queryKey: ['payment-methods'] })
  }

  const handleEdit = (m: PaymentMethod) => {
    setEditMethod(m.method)
    setEditForm({
      address: m.address,
      amount_tolerance: m.amount_tolerance,
      confirmations: m.confirmations,
      auto_activate: m.auto_activate,
      enabled: m.enabled,
    })
  }

  const handleSave = () => {
    if (!editMethod) return
    updateMethod.mutate({ method: editMethod, config: editForm })
  }

  const methods = data?.methods ?? []

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold" style={{ color: ADMIN_TEXT }}>支付配置</h1>
          <p className="text-sm mt-1" style={{ color: ADMIN_TEXT_MUTED }}>管理虚拟货币支付方式（USDT TRC20/ERC20）</p>
        </div>
        <Button variant="outline" size="sm" onClick={handleRefresh} disabled={isFetching}>
          <RefreshCw className={`w-4 h-4 mr-2 ${isFetching ? 'animate-spin' : ''}`} />
          刷新
        </Button>
      </div>

      {/* 汇率配置卡片 */}
      <Card style={{ background: ADMIN_CARD, borderColor: ADMIN_BORDER }}>
        <CardContent className="p-5">
          <div className="flex items-center gap-3 mb-4">
            <div className="w-10 h-10 rounded-full flex items-center justify-center bg-blue-900/30">
              <RefreshCw className="w-5 h-5 text-blue-400" />
            </div>
            <div>
              <h3 className="text-base font-semibold" style={{ color: ADMIN_TEXT }}>USDT → CNY 汇率配置</h3>
              <p className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>
                用于套餐 CNY 价格展示和订单 CNY 金额计算（下单时锁定汇率）
              </p>
            </div>
          </div>
          <div className="flex items-end gap-3">
            <div className="space-y-1 flex-1 max-w-xs">
              <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>1 USDT = ? CNY</label>
              <Input
                type="number"
                step="0.0001"
                min="0"
                value={rateInput !== '' ? rateInput : (rateData?.usdt_to_cny ?? 7.2)}
                onChange={(e) => setRateInput(e.target.value)}
                placeholder="7.2"
                style={{ background: ADMIN_INPUT_BG, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }}
              />
            </div>
            <Button
              size="sm"
              onClick={() => {
                const v = parseFloat(rateInput)
                if (v > 0) updateRate.mutate(v)
              }}
              disabled={updateRate.isPending || (rateInput !== '' && parseFloat(rateInput) <= 0)}
            >
              <Save className="w-4 h-4 mr-1" />
              {updateRate.isPending ? '保存中...' : '保存汇率'}
            </Button>
          </div>
          {rateData?.last_updated && (
            <p className="text-xs mt-3" style={{ color: ADMIN_TEXT_MUTED }}>
              最后更新: {new Date(rateData.last_updated).toLocaleString('zh-CN')}
            </p>
          )}
          {updateRate.isError && (
            <p className="text-xs mt-2 text-red-400">保存失败，请重试</p>
          )}
          {updateRate.isSuccess && rateInput === '' && (
            <p className="text-xs mt-2 text-green-400">汇率已更新</p>
          )}
        </CardContent>
      </Card>

      {isLoading ? (
        <Card style={{ background: ADMIN_CARD, borderColor: ADMIN_BORDER }}>
          <CardContent className="p-6">
            <Skeleton className="h-32 w-full" />
          </CardContent>
        </Card>
      ) : methods.length === 0 ? (
        <Card style={{ background: ADMIN_CARD, borderColor: ADMIN_BORDER }}>
          <CardContent className="p-12 text-center">
            <CreditCard className="w-12 h-12 mx-auto mb-3 opacity-30" />
            <p className="text-sm" style={{ color: ADMIN_TEXT_MUTED }}>暂无支付方式配置</p>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4 md:grid-cols-2">
          {methods.map((m) => (
            <Card key={m.method} style={{ background: ADMIN_CARD, borderColor: ADMIN_BORDER }}>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-4">
                  <div className="flex items-center gap-3">
                    <div className={`w-10 h-10 rounded-full flex items-center justify-center ${m.enabled ? 'bg-green-900/30' : 'bg-zinc-800'}`}>
                      <CreditCard className={`w-5 h-5 ${m.enabled ? 'text-green-400' : 'text-zinc-500'}`} />
                    </div>
                    <div>
                      <h3 className="text-base font-semibold" style={{ color: ADMIN_TEXT }}>{m.name}</h3>
                      <p className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>{m.network}</p>
                    </div>
                  </div>
                  <Badge variant="outline" className={m.enabled ? 'bg-green-900/50 text-green-300 border-green-800/50' : 'bg-zinc-800/50 text-zinc-400 border-zinc-700/50'}>
                    {m.enabled ? '已启用' : '已禁用'}
                  </Badge>
                </div>

                {editMethod === m.method ? (
                  <div className="space-y-3">
                    <div className="space-y-1">
                      <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>收款钱包地址</label>
                      <Input
                        value={editForm.address || ''}
                        onChange={(e) => setEditForm({ ...editForm, address: e.target.value })}
                        placeholder="输入 TRC20/ERC20 钱包地址"
                        style={{ background: ADMIN_INPUT_BG, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }}
                      />
                    </div>
                    <div className="grid grid-cols-2 gap-3">
                      <div className="space-y-1">
                        <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>金额容差</label>
                        <Input
                          type="number"
                          step="0.01"
                          value={editForm.amount_tolerance ?? 0}
                          onChange={(e) => setEditForm({ ...editForm, amount_tolerance: parseFloat(e.target.value) || 0 })}
                          style={{ background: ADMIN_INPUT_BG, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }}
                        />
                      </div>
                      <div className="space-y-1">
                        <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>最小确认数</label>
                        <Input
                          type="number"
                          value={editForm.confirmations ?? 1}
                          onChange={(e) => setEditForm({ ...editForm, confirmations: parseInt(e.target.value) || 1 })}
                          style={{ background: ADMIN_INPUT_BG, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }}
                        />
                      </div>
                    </div>
                    <label className="flex items-center gap-2 text-sm" style={{ color: ADMIN_TEXT_SECONDARY }}>
                      <input
                        type="checkbox"
                        checked={editForm.auto_activate ?? false}
                        onChange={(e) => setEditForm({ ...editForm, auto_activate: e.target.checked })}
                      />
                      支付成功后自动激活订阅
                    </label>
                    <div className="flex items-center gap-2 pt-2">
                      <Button size="sm" onClick={handleSave} disabled={updateMethod.isPending}>
                        <Save className="w-4 h-4 mr-1" /> 保存
                      </Button>
                      <Button size="sm" variant="outline" onClick={() => setEditMethod(null)}>取消</Button>
                    </div>
                  </div>
                ) : (
                  <div className="space-y-2">
                    <div className="flex items-center justify-between text-sm">
                      <span style={{ color: ADMIN_TEXT_MUTED }}>收款地址</span>
                      <code className="text-xs font-mono" style={{ color: ADMIN_TEXT_SECONDARY }}>
                        {m.address ? `${m.address.slice(0, 8)}...${m.address.slice(-6)}` : '未配置'}
                      </code>
                    </div>
                    <div className="flex items-center justify-between text-sm">
                      <span style={{ color: ADMIN_TEXT_MUTED }}>金额容差</span>
                      <span style={{ color: ADMIN_TEXT_SECONDARY }}>{m.amount_tolerance ?? 0}</span>
                    </div>
                    <div className="flex items-center justify-between text-sm">
                      <span style={{ color: ADMIN_TEXT_MUTED }}>最小确认数</span>
                      <span style={{ color: ADMIN_TEXT_SECONDARY }}>{m.confirmations ?? 1}</span>
                    </div>
                    <div className="flex items-center justify-between text-sm">
                      <span style={{ color: ADMIN_TEXT_MUTED }}>自动激活</span>
                      {m.auto_activate ? (
                        <CheckCircle className="w-4 h-4 text-green-400" />
                      ) : (
                        <XCircle className="w-4 h-4 text-zinc-500" />
                      )}
                    </div>
                    <div className="flex items-center gap-2 pt-3">
                      <Button size="sm" variant="outline" onClick={() => handleEdit(m)}>编辑</Button>
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() => toggleMethod.mutate(m.method)}
                        disabled={toggleMethod.isPending}
                      >
                        <Power className="w-4 h-4 mr-1" />
                        {m.enabled ? '禁用' : '启用'}
                      </Button>
                    </div>
                  </div>
                )}
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      {/* 说明卡片 */}
      <Card style={{ background: ADMIN_CARD, borderColor: ADMIN_BORDER }}>
        <CardContent className="p-5">
          <h3 className="text-sm font-semibold mb-2" style={{ color: ADMIN_TEXT }}>支付说明</h3>
          <ul className="space-y-1 text-xs" style={{ color: ADMIN_TEXT_MUTED }}>
            <li>• 系统支持 USDT TRC20（波场网络）和 ERC20（以太坊网络）两种虚拟货币支付方式</li>
            <li>• 配置收款地址后，用户购买套餐将生成对应网络的支付订单</li>
            <li>• 金额容差：允许用户支付的金额与应付金额的差值（用于处理精度问题）</li>
            <li>• 最小确认数：区块确认数达到此值后订单自动标记为已支付</li>
            <li>• 自动激活：支付成功后自动激活用户订阅，无需手动操作</li>
            <li>• 用户端购买流程：选择套餐 → 输入优惠码（可选）→ 生成支付地址 → 区块链转账 → 自动激活</li>
          </ul>
        </CardContent>
      </Card>
    </div>
  )
}
