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

// FiatChannel 第三方法币易支付渠道（可随时添加/更换：第三方易支付稳定性差、常需换平台）
interface FiatChannel {
  id: string
  name: string
  protocol: string          // v1（彩虹标准 MD5）| v2（RSA，api/pay/* 端点）
  sign_type?: string        // v2 下：RSA（默认）| MD5（V2 平台的 V1 兼容端点）
  gateway_url: string
  pid: string
  md5_key?: string
  merchant_private_key?: string
  platform_public_key?: string
  pay_type?: string
  notify_url?: string
  return_url?: string
  mapi_path?: string
  submit_path?: string
  query_path?: string
  method?: string
  device?: string
  // 只读展示字段（后端脱敏，不回传密钥）
  configured?: boolean
  md5_key_configured?: boolean
  private_key_configured?: boolean
  platform_key_set?: boolean
}

interface FiatChannelsResponse {
  channels: FiatChannel[]
  alipay_channel: string
  wechat_channel: string
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
  channel_id?: string
  channel_name?: string
  channel_configured?: boolean
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

  // 法币渠道池（第三方易支付可随时添加/更换）
  const { data: fiatData, isLoading: fiatLoading } = useQuery<FiatChannelsResponse>({
    queryKey: ['fiat-channels'],
    queryFn: async () => {
      return api.get<FiatChannelsResponse>(`${EP.PAYMENT_METHODS}/fiat-channels`)
    },
    retry: false,
  })
  const channels = fiatData?.channels ?? []

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
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['payment-methods'] })
      setEditMethod(null)
    },
  })

  // 渠道编辑状态：null=列表态，对象=编辑态（含 isNew 标记）
  const [editingChannel, setEditingChannel] = useState<Partial<FiatChannel> & { isNew?: boolean } | null>(null)
  const [channelError, setChannelError] = useState('')

  const saveChannels = useMutation({
    mutationFn: async (payload: { channels: FiatChannel[]; alipay_channel: string; wechat_channel: string }) => {
      return api.put(`${EP.PAYMENT_METHODS}/fiat-channels`, payload)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['fiat-channels'] })
      queryClient.invalidateQueries({ queryKey: ['payment-methods'] })
      setEditingChannel(null)
      setChannelError('')
    },
    onError: (e: Error) => {
      setChannelError(e.message || '保存失败')
    },
  })

  const setChannelField = (field: keyof FiatChannel, value: string) => {
    setEditingChannel((prev) => ({ ...(prev || {}), [field]: value }))
  }

  // 保存单个渠道编辑：合并进渠道列表 + 保留当前绑定
  const handleSaveChannel = () => {
    if (!editingChannel) return
    const id = (editingChannel.id || '').trim()
    const name = (editingChannel.name || '').trim()
    const gatewayURL = (editingChannel.gateway_url || '').trim()
    const pid = (editingChannel.pid || '').trim()
    if (!id || !name || !gatewayURL || !pid) {
      setChannelError('渠道ID、名称、接口地址、商户ID 均必填')
      return
    }
    if (!/^[a-zA-Z0-9_-]+$/.test(id)) {
      setChannelError('渠道ID 只能包含字母、数字、下划线、连字符')
      return
    }
    const others = channels.filter((c) => c.id !== id)
    const next: FiatChannel = {
      ...(editingChannel as FiatChannel),
      id, name, gateway_url: gatewayURL.replace(/\/+$/, ''), pid,
      protocol: editingChannel.protocol || 'v1',
      sign_type: editingChannel.protocol === 'v2' ? (editingChannel.sign_type || 'RSA') : '',
    }
    const list = [...others, next].sort((a, b) => a.id.localeCompare(b.id))
    saveChannels.mutate({
      channels: list,
      alipay_channel: fiatData?.alipay_channel && list.some((c) => c.id === fiatData.alipay_channel) ? fiatData.alipay_channel : (list[0]?.id || ''),
      wechat_channel: fiatData?.wechat_channel && list.some((c) => c.id === fiatData.wechat_channel) ? fiatData.wechat_channel : (list[0]?.id || ''),
    })
  }

  const handleDeleteChannel = (ch: FiatChannel) => {
    if (!window.confirm(`确定删除渠道「${ch.name || ch.id}」吗？绑定该渠道的支付方式将回退到第一个可用渠道。`)) return
    const list = channels.filter((c) => c.id !== ch.id)
    saveChannels.mutate({
      channels: list,
      alipay_channel: fiatData?.alipay_channel === ch.id ? (list[0]?.id || '') : (fiatData?.alipay_channel || ''),
      wechat_channel: fiatData?.wechat_channel === ch.id ? (list[0]?.id || '') : (fiatData?.wechat_channel || ''),
    })
  }

  const handleBindChannel = (method: 'alipay' | 'wechat', channelID: string) => {
    saveChannels.mutate({
      channels,
      alipay_channel: method === 'alipay' ? channelID : (fiatData?.alipay_channel || ''),
      wechat_channel: method === 'wechat' ? channelID : (fiatData?.wechat_channel || ''),
    })
  }

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
    updateMethod.mutate({ method: editMethod, config })
  }

  const toggleNetwork = (key: string) => {
    const cur = editForm.networks || []
    setEditForm({
      ...editForm,
      networks: cur.includes(key) ? cur.filter(k => k !== key) : [...cur, key],
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

      {/* 法币渠道池卡片：第三方易支付可随时添加/更换 */}
      <Card style={{ background: ADMIN_CARD, borderColor: ADMIN_BORDER }}>
        <CardContent className="p-5">
          <div className="flex items-center justify-between mb-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 rounded-full flex items-center justify-center bg-indigo-900/30">
                <CreditCard className="w-5 h-5 text-indigo-400" />
              </div>
              <div>
                <h3 className="text-base font-semibold" style={{ color: ADMIN_TEXT }}>法币支付渠道（第三方易支付）</h3>
                <p className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>
                  第三方易支付平台稳定性参差，可在此维护多个渠道随时换绑；支持 V1（MD5）与 V2（RSA/MD5）两种协议
                </p>
              </div>
            </div>
            {editingChannel?.isNew !== true && !editingChannel && (
              <Button size="sm" onClick={() => { setChannelError(''); setEditingChannel({ isNew: true, protocol: 'v1', sign_type: 'RSA', pay_type: 'alipay' }) }}>
                添加渠道
              </Button>
            )}
          </div>

          {editingChannel ? (
            <div className="rounded-lg p-4 space-y-3" style={{ background: ADMIN_INPUT_BG, border: `1px solid ${ADMIN_INPUT_BORDER}` }}>
              <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
                <div className="space-y-1">
                  <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>渠道ID（唯一标识）</label>
                  <Input
                    value={editingChannel.id || ''}
                    onChange={(e) => setChannelField('id', e.target.value)}
                    placeholder="如 ifz / qiupay"
                    disabled={!editingChannel.isNew}
                    style={{ background: ADMIN_CARD, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }}
                  />
                </div>
                <div className="space-y-1">
                  <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>渠道名称</label>
                  <Input
                    value={editingChannel.name || ''}
                    onChange={(e) => setChannelField('name', e.target.value)}
                    placeholder="如 ifz V2 易支付"
                    style={{ background: ADMIN_CARD, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }}
                  />
                </div>
                <div className="space-y-1">
                  <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>协议版本</label>
                  <select
                    value={editingChannel.protocol || 'v1'}
                    onChange={(e) => setChannelField('protocol', e.target.value)}
                    className="w-full px-3 py-2 rounded-lg border text-sm"
                    style={{ background: ADMIN_CARD, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }}
                  >
                    <option value="v1">V1 彩虹标准（MD5，mapi.php）</option>
                    <option value="v2">V2（api/pay/*，RSA 或 MD5）</option>
                  </select>
                </div>
                <div className="space-y-1">
                  <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>签名方式{editingChannel.protocol === 'v2' ? '' : '（仅 V2 可选）'}</label>
                  <select
                    value={editingChannel.sign_type || 'RSA'}
                    onChange={(e) => setChannelField('sign_type', e.target.value)}
                    disabled={editingChannel.protocol !== 'v2'}
                    className="w-full px-3 py-2 rounded-lg border text-sm"
                    style={{ background: ADMIN_CARD, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }}
                  >
                    <option value="RSA">RSA（SHA256WithRSA）</option>
                    <option value="MD5">MD5（V2 平台 V1 端点兼容）</option>
                  </select>
                </div>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                <div className="space-y-1 md:col-span-2">
                  <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>接口地址（gateway_url）</label>
                  <Input
                    value={editingChannel.gateway_url || ''}
                    onChange={(e) => setChannelField('gateway_url', e.target.value)}
                    placeholder="https://pay.example.com 或 https://xx/xpay/epay/"
                    style={{ background: ADMIN_CARD, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }}
                  />
                </div>
                <div className="space-y-1">
                  <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>商户ID (pid)</label>
                  <Input
                    value={editingChannel.pid || ''}
                    onChange={(e) => setChannelField('pid', e.target.value)}
                    placeholder="1001"
                    style={{ background: ADMIN_CARD, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }}
                  />
                </div>
              </div>

              {/* 密钥组：v1/v2-md5 用 MD5 密钥；v2-rsa 用商户私钥+平台公钥 */}
              {(editingChannel.protocol !== 'v2' || (editingChannel.sign_type || 'RSA') === 'MD5') && (
                <div className="space-y-1">
                  <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>
                    MD5 商户密钥 {editingChannel.md5_key_configured && !editingChannel.isNew ? '（已配置，留空保持不变）' : ''}
                  </label>
                  <Input
                    type="password"
                    value={editingChannel.md5_key || ''}
                    onChange={(e) => setChannelField('md5_key', e.target.value)}
                    placeholder="留空保持不变"
                    style={{ background: ADMIN_CARD, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }}
                  />
                </div>
              )}
              {editingChannel.protocol === 'v2' && (editingChannel.sign_type || 'RSA') === 'RSA' && (
                <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                  <div className="space-y-1">
                    <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>
                      商户私钥（PKCS#8）{editingChannel.private_key_configured && !editingChannel.isNew ? '（已配置，留空保持不变）' : ''}
                    </label>
                    <textarea
                      value={editingChannel.merchant_private_key || ''}
                      onChange={(e) => setChannelField('merchant_private_key', e.target.value)}
                      rows={3}
                      placeholder="MIIEvQ...（粘贴带/不带 PEM 头尾均可）"
                      style={{ width: '100%', background: ADMIN_CARD, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT, borderRadius: 8, padding: 8, fontSize: 11, fontFamily: 'monospace' }}
                    />
                  </div>
                  <div className="space-y-1">
                    <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>
                      平台公钥（X.509）{editingChannel.platform_key_set && !editingChannel.isNew ? '（已配置，留空保持不变）' : ''}
                    </label>
                    <textarea
                      value={editingChannel.platform_public_key || ''}
                      onChange={(e) => setChannelField('platform_public_key', e.target.value)}
                      rows={3}
                      placeholder="MIIBIjAN...（粘贴带/不带 PEM 头尾均可）"
                      style={{ width: '100%', background: ADMIN_CARD, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT, borderRadius: 8, padding: 8, fontSize: 11, fontFamily: 'monospace' }}
                    />
                  </div>
                </div>
              )}

              <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
                <div className="space-y-1">
                  <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>下单类型</label>
                  <select
                    value={editingChannel.pay_type || 'alipay'}
                    onChange={(e) => setChannelField('pay_type', e.target.value)}
                    className="w-full px-3 py-2 rounded-lg border text-sm"
                    style={{ background: ADMIN_CARD, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }}
                  >
                    <option value="alipay">支付宝 alipay</option>
                    <option value="wxpay">微信 wxpay</option>
                  </select>
                </div>
                <div className="space-y-1 md:col-span-2">
                  <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>通知地址（留空自动按面板补齐）</label>
                  <Input
                    value={editingChannel.notify_url || ''}
                    onChange={(e) => setChannelField('notify_url', e.target.value)}
                    placeholder="自动：https://面板/api/v1/payment/notify/{method}"
                    style={{ background: ADMIN_CARD, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }}
                  />
                </div>
                <div className="space-y-1">
                  <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>回跳地址（留空自动）</label>
                  <Input
                    value={editingChannel.return_url || ''}
                    onChange={(e) => setChannelField('return_url', e.target.value)}
                    placeholder="自动：用户订单列表"
                    style={{ background: ADMIN_CARD, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }}
                  />
                </div>
              </div>

              {editingChannel.protocol === 'v2' && (editingChannel.sign_type || 'RSA') === 'RSA' && (
                <div className="grid grid-cols-2 gap-3">
                  <div className="space-y-1">
                    <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>接口类型 method（web 自适应/仅跳转）</label>
                    <select
                      value={editingChannel.method || 'web'}
                      onChange={(e) => setChannelField('method', e.target.value)}
                      className="w-full px-3 py-2 rounded-lg border text-sm"
                      style={{ background: ADMIN_CARD, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }}
                    >
                      <option value="web">web（自适应二维码/跳转）</option>
                      <option value="jump">jump（仅跳转链接）</option>
                    </select>
                  </div>
                  <div className="space-y-1">
                    <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>设备类型 device</label>
                    <select
                      value={editingChannel.device || 'pc'}
                      onChange={(e) => setChannelField('device', e.target.value)}
                      className="w-full px-3 py-2 rounded-lg border text-sm"
                      style={{ background: ADMIN_CARD, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }}
                    >
                      <option value="pc">pc</option>
                      <option value="mobile">mobile</option>
                    </select>
                  </div>
                </div>
              )}

              {editingChannel.protocol === 'v1' && (
                <div className="rounded-lg p-3" style={{ background: 'rgba(59,130,246,0.06)', border: '1px solid rgba(59,130,246,0.18)' }}>
                  <p className="text-xs font-medium mb-2" style={{ color: '#60a5fa' }}>接口路径（高级，默认彩虹标准，换平台一般无需改）</p>
                  <div className="grid grid-cols-3 gap-2">
                    <div className="space-y-1">
                      <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>JSON下单 (mapi)</label>
                      <Input value={editingChannel.mapi_path || 'mapi.php'} onChange={(e) => setChannelField('mapi_path', e.target.value)} placeholder="mapi.php" style={{ background: ADMIN_CARD, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }} />
                    </div>
                    <div className="space-y-1">
                      <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>表单下单 (submit)</label>
                      <Input value={editingChannel.submit_path || 'submit.php'} onChange={(e) => setChannelField('submit_path', e.target.value)} placeholder="submit.php" style={{ background: ADMIN_CARD, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }} />
                    </div>
                    <div className="space-y-1">
                      <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>查单 (query)</label>
                      <Input value={editingChannel.query_path || 'api.php'} onChange={(e) => setChannelField('query_path', e.target.value)} placeholder="api.php" style={{ background: ADMIN_CARD, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }} />
                    </div>
                  </div>
                </div>
              )}

              {channelError && <p className="text-xs" style={{ color: '#f87171' }}>{channelError}</p>}
              <div className="flex items-center gap-2">
                <Button size="sm" onClick={handleSaveChannel} disabled={saveChannels.isPending}>
                  <Save className="w-4 h-4 mr-1" /> {saveChannels.isPending ? '保存中...' : '保存渠道'}
                </Button>
                <Button size="sm" variant="outline" onClick={() => { setEditingChannel(null); setChannelError('') }}>取消</Button>
              </div>
            </div>
          ) : (
            <div className="space-y-2">
              {fiatLoading ? (
                <Skeleton className="h-16 w-full" />
              ) : channels.length === 0 ? (
                <p className="text-sm py-3" style={{ color: ADMIN_TEXT_MUTED }}>暂无渠道，点击「添加渠道」接入第一个第三方易支付</p>
              ) : (
                channels.map((c) => (
                  <div key={c.id} className="flex items-center justify-between rounded-lg px-3 py-2.5" style={{ background: ADMIN_INPUT_BG, border: `1px solid ${ADMIN_INPUT_BORDER}` }}>
                    <div className="min-w-0">
                      <div className="flex items-center gap-2 flex-wrap">
                        <span className="text-sm font-medium" style={{ color: ADMIN_TEXT }}>{c.name || c.id}</span>
                        <span className="text-xs font-mono" style={{ color: ADMIN_TEXT_MUTED }}>{c.id}</span>
                        <Badge variant="outline" className="text-xs">
                          {c.protocol === 'v2' ? `V2/${c.sign_type || 'RSA'}` : 'V1/MD5'}
                        </Badge>
                        <Badge variant="outline" className={c.configured ? 'bg-green-900/50 text-green-300 border-green-800/50' : 'bg-amber-900/50 text-amber-300 border-amber-800/50'}>
                          {c.configured ? '已配置' : '配置不完整'}
                        </Badge>
                        {fiatData?.alipay_channel === c.id && <Badge variant="outline" className="bg-blue-900/50 text-blue-300 border-blue-800/50 text-xs">支付宝</Badge>}
                        {fiatData?.wechat_channel === c.id && <Badge variant="outline" className="bg-emerald-900/50 text-emerald-300 border-emerald-800/50 text-xs">微信</Badge>}
                      </div>
                      <p className="text-xs mt-1 truncate font-mono" style={{ color: ADMIN_TEXT_MUTED }}>{c.gateway_url} · pid {c.pid}</p>
                    </div>
                    <div className="flex items-center gap-1 flex-shrink-0 ml-2">
                      <Button size="sm" variant="ghost" className="h-8 px-2" onClick={() => { setChannelError(''); setEditingChannel({ ...c, md5_key: '', merchant_private_key: '', platform_public_key: '' }) }}>
                        编辑
                      </Button>
                      <Button size="sm" variant="ghost" className="h-8 px-2 text-red-400 hover:text-red-300" onClick={() => handleDeleteChannel(c)}>
                        删除
                      </Button>
                    </div>
                  </div>
                ))
              )}
            </div>
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
                {(m.enabled && (m.method === 'wechat' || m.method === 'alipay') && !m.channel_configured) && (
                  <div className="rounded-lg p-2.5 mb-3 text-xs" style={{ background: 'rgba(205,92,77,0.08)', border: '1px solid rgba(205,92,77,0.25)', color: '#cd5c4d' }}>
                    已启用但绑定的渠道未配置完整，用户端不会显示该支付方式。
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
                      <div className="rounded-lg p-3 space-y-2" style={{ background: ADMIN_INPUT_BG, border: `1px solid ${ADMIN_INPUT_BORDER}` }}>
                        <p className="text-xs font-medium" style={{ color: ADMIN_TEXT_MUTED }}>
                          当前渠道：{m.channel_name ? `${m.channel_name}（${m.channel_id}）` : '未绑定'} {m.channel_configured ? '✓ 已配置' : '⚠ 配置不完整'}
                        </p>
                        <div className="space-y-1">
                          <label className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>切换渠道（第三方易支付出问题时可随时换绑）</label>
                          <select
                            value={m.channel_id || ''}
                            onChange={(e) => handleBindChannel(m.method as 'alipay' | 'wechat', e.target.value)}
                            disabled={saveChannels.isPending || channels.length === 0}
                            className="w-full px-3 py-2 rounded-lg border text-sm"
                            style={{ background: ADMIN_CARD, borderColor: ADMIN_INPUT_BORDER, color: ADMIN_TEXT }}
                          >
                            {channels.length === 0 && <option value="">暂无渠道（请在下方渠道管理添加）</option>}
                            {channels.map((c) => (
                              <option key={c.id} value={c.id}>
                                {c.name}（{c.id}·{c.protocol}{c.protocol === 'v2' ? `/${c.sign_type}` : ''}）{c.configured ? '' : ' ⚠未配置完整'}
                              </option>
                            ))}
                          </select>
                        </div>
                      </div>
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
                          <span style={{ color: ADMIN_TEXT_MUTED }}>绑定渠道</span>
                          <span style={{ color: m.channel_configured ? '#26a17b' : ADMIN_TEXT_SECONDARY }}>
                            {m.channel_name ? `${m.channel_name}${m.channel_configured ? '' : '（配置不完整）'}` : '未绑定'}
                          </span>
                        </div>
                        <div className="flex items-center justify-between text-sm">
                          <span style={{ color: ADMIN_TEXT_MUTED }}>渠道ID</span>
                          <span className="text-xs font-mono" style={{ color: ADMIN_TEXT_SECONDARY }}>
                            {m.channel_id || '-'}
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
