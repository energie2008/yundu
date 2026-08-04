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
import { PaymentMethodIcon } from '@/components/common/PaymentIcons'

const EVM_NETWORK_OPTIONS = [
  { key: 'polygon', label: 'Polygon' },
  { key: 'arbitrum', label: 'Arbitrum One' },
]

interface EpayConfig {
  pid?: string
  key?: string
  gateway_url?: string
  pay_type?: string
  notify_url?: string
  return_url?: string
  key_configured?: boolean
}

interface PaymentMethod {
  method: string
  name: string
  enabled: boolean
  address?: string
  amount_tolerance?: number
  confirmations?: number
  network?: string
  auto_activate?: boolean
  api_key_configured?: boolean
  api_key?: string
  networks?: string[]
  epay?: EpayConfig
  epay_configured?: boolean
  rpc_nodes?: string[]
  available?: boolean
  unavailable_reason?: string
}

interface PaymentMethodsResponse {
  methods: PaymentMethod[]
}

export default function Payments() {
  const queryClient = useQueryClient()
  const [editMethod, setEditMethod] = useState<string | null>(null)
  const [editForm, setEditForm] = useState<Partial<PaymentMethod>>({})
  const [rateInput, setRateInput] = useState('')
  const [saveError, setSaveError] = useState('')

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
        network: config.network,
        api_key: config.api_key,
        networks: config.networks,
        epay: config.epay,
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
    setSaveError('')
    setEditForm({
      address: m.address,
      amount_tolerance: m.amount_tolerance,
      confirmations: m.confirmations,
      auto_activate: m.auto_activate,
      enabled: m.enabled,
      network: m.network,
      networks: m.networks,
      epay: m.epay,
    })
  }

  const handleSave = () => {
    if (!editMethod) return
    const config: Partial<PaymentMethod> = { ...editForm }
    if (editMethod === 'usdt_erc20' && !(config.networks || []).length) {
      setSaveError('请至少选择一个收款网络')
      return
    }
    if (config.api_key === '') {
      delete config.api_key
    }
    if (config.epay?.key === '') {
      delete config.epay.key
    }
    updateMethod.mutate({ method: editMethod, config })
  }

  const toggleNetwork = (key: string) => {
    const cur = editForm.networks || []
    setEditForm({
      ...editForm,
      networks: cur.includes(key) ? cur.filter(k => k !== key) : [...cur, key],
    })
  }

  const setEpayField = (field: keyof EpayConfig, value: string) => {
    setEditForm({
      ...editForm,
      epay: { ...(editForm.epay || {}), [field]: value },
    })
  }

  const methods = data?.methods ?? []

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold" style={{ color: ADMIN_TEXT }}>支付配置</h1>
          <p className="text-sm mt-1" style={{ color: ADMIN_TEXT_MUTED }}>管理 USDT 支付（TRC20 / EVM Polygon / Arbitrum One 双通道）与支付宝/微信</p>
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
                    <PaymentMethodIcon method={m.method} size={40} />
                    <div>
                      <h3 className="text-base font-semibold" style={{ color: ADMIN_TEXT }}>{m.name}</h3>
                      <p className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>{m.network}</p>
                    </div>
                  </div>
                  <Badge variant="outline" className={m.enabled ? 'bg-green-900/50 text-green-300 border-green-800/50' : 'bg-zinc-800/50 text-zinc-400 border-zinc-700/50'}>
                    {m.enabled ? '已启用' : '已禁用'}
                  </Badge>
                </div>

                {(m.enabled && (m.method === 'usdt_trc20' || m.method === 'usdt_erc20' || m.method === 'usdt_bep20') && !m.address) && (
                  <div className="rounded-lg p-2.5 mb-3 text-xs" style={{ background: 'rgba(205,92,77,0.08)', border: '1px solid rgba(205,92,77,0.25)', color: '#cd5c4d' }}>
                    已启用但未配置收款地址，用户端不会显示该支付方式。
                  </div>
                )}
                {(m.enabled && (m.method === 'wechat' || m.method === 'alipay') && !m.epay_configured) && (
                  <div className="rounded-lg p-2.5 mb-3 text-xs" style={{ background: 'rgba(205,92,77,0.08)', border: '1px solid rgba(205,92,77,0.25)', color: '#cd5c4d' }}>
                    已启用但易支付未配置（需商户ID + 密钥 + 网关地址），用户端不会显示该支付方式。
                  </div>
                )}
                {m.method === 'usdt_bep20' && (
                  <div className="rounded-lg p-2.5 mb-3 text-xs" style={{ background: 'rgba(232,163,61,0.08)', border: '1px solid rgba(232,163,61,0.25)', color: '#e8a33d' }}>
                    BSC 自动到账依赖可用 RPC 节点（已内置可用节点）；若无法自动到账，请在编辑里更换/增加 RPC。
                  </div>
                )}

                {editMethod === m.method ? (
                  <div className="space-y-3">
                    {(m.method === 'usdt_trc20' || m.method === 'usdt_erc20' || m.method === 'usdt_bep20') && (
                      <>
                        <div className="space-y-1">
                          <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>收款钱包地址</label>
                          <Input
                            value={editForm.address || ''}
                            onChange={(e) => setEditForm({ ...editForm, address: e.target.value })}
                            placeholder={m.method === 'usdt_bep20' ? '输入 BEP20 (BSC) 钱包地址' : '输入 TRC20/ERC20 钱包地址'}
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
                      </>
                    )}
                    {m.method === 'usdt_bep20' && (
                      <div className="rounded-lg p-3 space-y-2" style={{ background: 'rgba(243,186,47,0.08)', border: '1px solid rgba(243,186,47,0.2)' }}>
                        <p className="text-xs font-medium" style={{ color: '#d4a72c' }}>BSC RPC 节点（自动到账依赖此查询）</p>
                        <p className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>
                          BSC 公共 RPC 对 USDT 转账日志查询全部限流（日志量超限），默认已停用。如需启用请填写可用的 RPC（每行一个，如自建节点/付费 RPC）。
                        </p>
                        <textarea
                          value={(editForm.rpc_nodes || []).join('\n')}
                          onChange={(e) => setEditForm({ ...editForm, rpc_nodes: e.target.value.split('\n').map((s: string) => s.trim()).filter(Boolean) })}
                          rows={4}
                          placeholder={'https://...（每行一个 RPC）'}
                          style={{ width: '100%', background: ADMIN_INPUT_BG, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT, borderRadius: 8, padding: 8, fontSize: 12, fontFamily: 'monospace' }}
                        />
                      </div>
                    )}
                    {m.method === 'usdt_erc20' && (
                      <>
                        <div className="space-y-1">
                          <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>收款网络（可多选，共用同一收款地址）</label>
                          <div className="flex flex-wrap gap-2">
                            {EVM_NETWORK_OPTIONS.map(opt => {
                              const active = (editForm.networks || []).includes(opt.key)
                              return (
                                <button
                                  key={opt.key}
                                  type="button"
                                  onClick={() => toggleNetwork(opt.key)}
                                  className="px-3 py-1.5 rounded-lg border text-xs font-medium transition-colors"
                                  style={{
                                    borderColor: active ? '#26a17b' : ADMIN_INPUT_BORDER,
                                    color: active ? '#26a17b' : ADMIN_TEXT_SECONDARY,
                                    background: active ? 'rgba(38,161,123,0.12)' : ADMIN_INPUT_BG,
                                  }}
                                >
                                  {active ? '✓ ' : ''}{opt.label}
                                </button>
                              )
                            })}
                          </div>
                          <p className="text-xs mt-1" style={{ color: ADMIN_TEXT_MUTED }}>
                            网络共用同一地址，订单按所选网络独立查链匹配
                          </p>
                        </div>
                        <div className="space-y-1">
                          <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>
                            Etherscan V2 API Key {m.api_key_configured ? '（已配置）' : '（未配置）'}
                          </label>
                          <Input
                            type="password"
                            value={editForm.api_key || ''}
                            onChange={(e) => setEditForm({ ...editForm, api_key: e.target.value })}
                            placeholder="留空保持不变，用于 EVM 链上转账查询"
                            style={{ background: ADMIN_INPUT_BG, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }}
                          />
                          <p className="text-xs mt-1" style={{ color: ADMIN_TEXT_MUTED }}>
                            免费注册于 etherscan.io/apidashboard，Polygon/Arbitrum 自动到账需此 Key
                          </p>
                        </div>
                      </>
                    )}
                    {(m.method === 'wechat' || m.method === 'alipay') && (
                      <>
                        <div className="space-y-1">
                          <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>
                            易支付商户状态：{m.epay_configured ? '已配置' : '未配置'}
                          </label>
                          <div className="rounded-lg p-3 space-y-3" style={{ background: ADMIN_INPUT_BG, border: `1px solid ${ADMIN_INPUT_BORDER}` }}>
                            <div className="grid grid-cols-2 gap-3">
                              <div className="space-y-1">
                                <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>商户ID (pid)</label>
                                <Input
                                  value={editForm.epay?.pid || ''}
                                  onChange={(e) => setEpayField('pid', e.target.value)}
                                  placeholder="1001"
                                  style={{ background: ADMIN_CARD, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }}
                                />
                              </div>
                              <div className="space-y-1">
                                <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>
                                  商户密钥 (key) {editForm.epay?.key_configured ? '（已配置）' : ''}
                                </label>
                                <Input
                                  type="password"
                                  value={editForm.epay?.key || ''}
                                  onChange={(e) => setEpayField('key', e.target.value)}
                                  placeholder="留空保持不变"
                                  style={{ background: ADMIN_CARD, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }}
                                />
                              </div>
                            </div>
                            <div className="space-y-1">
                              <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>易支付网关地址</label>
                              <Input
                                value={editForm.epay?.gateway_url || ''}
                                onChange={(e) => setEpayField('gateway_url', e.target.value)}
                                placeholder="https://pay.example.com"
                                style={{ background: ADMIN_CARD, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }}
                              />
                            </div>
                            <div className="grid grid-cols-2 gap-3">
                              <div className="space-y-1">
                                <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>下单类型</label>
                                <select
                                  value={editForm.epay?.pay_type || (m.method === 'wechat' ? 'wxpay' : 'alipay')}
                                  onChange={(e) => setEpayField('pay_type', e.target.value)}
                                  className="w-full px-3 py-2 rounded-lg border text-sm"
                                  style={{ background: ADMIN_CARD, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }}
                                >
                                  <option value="alipay">支付宝 alipay</option>
                                  <option value="wxpay">微信 wxpay</option>
                                </select>
                              </div>
                              <div className="space-y-1">
                                <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>通知地址 (notify_url)</label>
                                <Input
                                  value={editForm.epay?.notify_url || ''}
                                  onChange={(e) => setEpayField('notify_url', e.target.value)}
                                  placeholder="https://6.tiktokplay.na.am/api/v1/payment/notify/alipay"
                                  style={{ background: ADMIN_CARD, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }}
                                />
                              </div>
                            </div>
                            <div className="space-y-1">
                              <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>回跳地址 (return_url)</label>
                              <Input
                                value={editForm.epay?.return_url || ''}
                                onChange={(e) => setEpayField('return_url', e.target.value)}
                                placeholder="https://7.tiktokplay.na.am/dashboard/orders"
                                style={{ background: ADMIN_CARD, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }}
                              />
                            </div>
                          </div>
                        </div>
                      </>
                    )}
                    {saveError && (
                      <p className="text-xs" style={{ color: '#f87171' }}>{saveError}</p>
                    )}
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
                    {(m.method === 'usdt_trc20' || m.method === 'usdt_erc20' || m.method === 'usdt_bep20') && (
                      <>
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
                      </>
                    )}
                    {(m.method === 'wechat' || m.method === 'alipay') && (
                      <>
                        <div className="flex items-center justify-between text-sm">
                          <span style={{ color: ADMIN_TEXT_MUTED }}>易支付商户</span>
                          <span style={{ color: m.epay_configured ? '#26a17b' : ADMIN_TEXT_SECONDARY }}>
                            {m.epay_configured ? `已配置 (PID ${m.epay?.pid})` : '未配置'}
                          </span>
                        </div>
                        <div className="flex items-center justify-between text-sm">
                          <span style={{ color: ADMIN_TEXT_MUTED }}>网关地址</span>
                          <span className="text-xs font-mono" style={{ color: ADMIN_TEXT_SECONDARY }}>
                            {m.epay?.gateway_url || '-'}
                          </span>
                        </div>
                      </>
                    )}
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
            <li>• USDT 通道：TRC20（波场网络）、EVM 双通道（Polygon / Arbitrum One，共用同一收款地址）</li>
            <li>• BEP20(BSC) 因公共 RPC 无法查询 USDT 转账日志（日志量超限），自动到账不可用，默认停用</li>
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
