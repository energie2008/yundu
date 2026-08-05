import { useState, useMemo, useEffect } from 'react';
import { useSearchParams, useNavigate } from 'react-router-dom';
import { useQuery, useMutation } from '@tanstack/react-query';
import { api } from '../lib/api';
import { EP, adaptPlan, getPeriodLabel, bytesToGB } from '../lib/endpoints';
import type { PlanResponse, PaymentMethod, PaymentMethodsResponse, OrderResponse, CouponValidateResponse } from '../lib/endpoints';
import { PaymentMethodIcon } from '../components/PaymentIcons';
import { useToast } from '../lib/toast';

const EVM_NETWORK_LABELS: Record<string, string> = {
  polygon: 'Polygon',
  arbitrum: 'Arbitrum One',
  bsc: 'BSC',
}

function formatTraffic(bytes: number): string {
  if (bytes <= 0) return '不限流量';
  const gb = bytesToGB(bytes);
  return gb >= 1024 ? `${(gb / 1024).toFixed(0)} TB` : `${gb.toFixed(0)} GB`;
}

function ConfirmPayCard({
  open,
  planName,
  periodLabel,
  basePrice,
  discount,
  totalPrice,
  method,
  networkLabel,
  rate,
  submitting,
  onConfirm,
  onCancel,
}: {
  open: boolean
  planName: string
  periodLabel: string
  basePrice: number
  discount: number
  totalPrice: number
  method?: PaymentMethod
  networkLabel?: string
  rate: number
  submitting: boolean
  onConfirm: () => void
  onCancel: () => void
}) {
  if (!open) return null
  const isUsdt = !!method && (method.method === 'usdt_trc20' || method.method === 'usdt_erc20' || method.method === 'usdt_bep20')
  const usdtTotal = totalPrice > 0 && rate > 0 ? totalPrice / rate : 0

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      style={{ background: 'rgba(0,0,0,0.55)' }}
      onClick={onCancel}
    >
      <div
        className="xboard-card w-full max-w-md p-5 sm:p-6"
        style={{ background: 'var(--card)', border: '1px solid var(--border)' }}
        onClick={e => e.stopPropagation()}
      >
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-bold" style={{ color: 'var(--foreground)' }}>确认支付</h2>
          <button
            onClick={onCancel}
            className="text-2xl leading-none px-2"
            style={{ color: 'var(--muted-foreground)' }}
            aria-label="关闭"
          >
            ×
          </button>
        </div>

        <div className="rounded-lg p-4 mb-4" style={{ background: 'var(--muted)' }}>
          <div className="flex items-start justify-between gap-3">
            <div>
              <p className="text-base font-semibold" style={{ color: 'var(--foreground)' }}>{planName}</p>
              <p className="text-xs mt-1" style={{ color: 'var(--muted-foreground)' }}>{periodLabel}</p>
            </div>
            <span className="text-lg font-bold whitespace-nowrap" style={{ color: 'var(--primary)' }}>
              ¥{totalPrice.toFixed(2)}
            </span>
          </div>
        </div>

        <div className="space-y-2.5 text-sm mb-4">
          <div className="flex justify-between">
            <span style={{ color: 'var(--muted-foreground)' }}>原价</span>
            <span style={{ color: 'var(--foreground)' }}>¥{basePrice.toFixed(2)}</span>
          </div>
          {discount > 0 && (
            <div className="flex justify-between">
              <span style={{ color: 'var(--success)' }}>优惠</span>
              <span style={{ color: 'var(--success)' }}>-¥{discount.toFixed(2)}</span>
            </div>
          )}
          <div className="flex justify-between pt-2 border-t" style={{ borderColor: 'var(--border)' }}>
            <span className="font-semibold" style={{ color: 'var(--foreground)' }}>支付方式</span>
            <span className="font-medium flex items-center gap-2" style={{ color: 'var(--foreground)' }}>
              {method && <PaymentMethodIcon method={method.method} size={20} />}
              {totalPrice === 0
                ? '优惠券全额抵扣'
                : networkLabel
                  ? `${method?.name || 'USDT'} · ${networkLabel}`
                  : method?.name || '-'}
            </span>
          </div>
          {method?.hint && (
            <div className="rounded-md p-2.5 text-xs mt-2" style={{ background: 'rgba(217,119,87,0.08)', border: '1px solid rgba(217,119,87,0.2)' }}>
              <p style={{ color: 'var(--primary)' }}>⚠️ {method.hint}</p>
            </div>
          )}
          {totalPrice > 0 && isUsdt && usdtTotal > 0 && (
            <div className="flex justify-between">
              <span style={{ color: 'var(--muted-foreground)' }}>应付等值</span>
              <span className="font-bold" style={{ color: 'var(--primary)' }}>≈ {usdtTotal.toFixed(2)} USDT</span>
            </div>
          )}
        </div>

        <div className="flex gap-3">
          <button
            onClick={onCancel}
            disabled={submitting}
            className="flex-1 h-10 rounded-lg text-sm font-medium border transition-colors disabled:opacity-50"
            style={{ borderColor: 'var(--border)', color: 'var(--muted-foreground)' }}
          >
            返回修改
          </button>
          <button
            onClick={onConfirm}
            disabled={submitting}
            className="flex-[2] h-10 rounded-lg text-sm font-medium text-white transition-colors shadow-sm disabled:opacity-50"
            style={{ background: 'var(--primary)' }}
          >
            {submitting ? '处理中...' : totalPrice === 0 ? '确认获取' : '确认并支付'}
          </button>
        </div>
      </div>
    </div>
  )
}

export function Checkout() {
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const { toast } = useToast();
  const planId = searchParams.get('plan_id') || '';
  const initialPeriod = searchParams.get('period') || 'month';

  const [selectedPeriod, setSelectedPeriod] = useState(initialPeriod);
  const [selectedMethod, setSelectedMethod] = useState<string>('');
  const [couponCode, setCouponCode] = useState('');
  const [couponResult, setCouponResult] = useState<CouponValidateResponse | null>(null);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [selectedNetwork, setSelectedNetwork] = useState<string>('');

  const planQuery = useQuery<PlanResponse>({
    queryKey: ['plan', planId],
    queryFn: async () => {
      const data = await api.get<PlanResponse>(EP.PLAN_DETAIL(planId));
      return adaptPlan(data);
    },
    enabled: !!planId,
  });

  const methodsQuery = useQuery<PaymentMethodsResponse>({
    queryKey: ['payment-methods'],
    queryFn: () => api.get<PaymentMethodsResponse>(EP.PAYMENT_METHODS),
    retry: 1,
  });

  const [couponError, setCouponError] = useState<string>('');

  const couponMut = useMutation({
    mutationFn: async () => {
      setCouponError('');
      if (!couponCode.trim() || !plan) return null;
      const price = getSelectedPrice();
      return api.post<CouponValidateResponse>(EP.COUPON_VALIDATE, {
        coupon_code: couponCode.trim(),
        plan_id: planId,
        period_code: selectedPeriod,
        amount_cny: price?.price_cny || 0,
      });
    },
    onSuccess: (data) => { setCouponResult(data); setCouponError(''); },
    onError: (err: Error) => { setCouponResult(null); setCouponError(err.message || '优惠码验证失败'); },
  });

  const orderMut = useMutation({
    mutationFn: async () => {
      // 金额为0（全额优惠）时不要求选择支付方式，直接创建免费订单
      if (!plan) return null;
      if (totalPrice > 0 && !selectedMethod) return null;
      return api.post<OrderResponse>(EP.ORDER_CREATE, {
        plan_id: planId,
        period_code: selectedPeriod,
        payment_method: totalPrice === 0 ? 'free' : selectedMethod,
        network: selectedMethod === 'usdt_erc20' ? selectedNetwork : undefined,
        coupon_code: couponResult?.valid ? couponCode.trim() : undefined,
      });
    },
    onSuccess: (order) => {
      setConfirmOpen(false);
      if (order?.id) {
        navigate(`/dashboard/orders/${order.id}`);
      }
    },
    onError: (err: Error) => {
      toast({ title: '下单失败', description: err.message || '创建订单失败，请稍后重试', variant: 'destructive' });
    },
  });

  const plan = planQuery.data;
  const methods = methodsQuery.data?.methods || [];
  const evmNetworks = methods.find(m => m.method === 'usdt_erc20')?.networks || [];

  // 自动选择第一个可用支付方式
  useEffect(() => {
    if (!selectedMethod && methods.length > 0) {
      setSelectedMethod(methods[0].method)
    }
  }, [methods, selectedMethod])

  useEffect(() => {
    if (selectedMethod === 'usdt_erc20') {
      setSelectedNetwork(prev => evmNetworks.includes(prev) ? prev : (evmNetworks[0] || ''))
    }
  }, [selectedMethod, evmNetworks]);

  const getSelectedPrice = () => {
    if (!plan) return null;
    return plan.prices?.find(p => p.period_code === selectedPeriod) || plan.prices?.[0];
  };

  const selectedPrice = getSelectedPrice();
  const basePrice = selectedPrice?.price_cny || 0;
  const methodObj = methods.find(m => m.method === selectedMethod);
  const discount = couponResult?.valid ? (couponResult.discount_amount || 0) : 0;
  const totalPrice = Math.max(0, basePrice - discount);
  const exchangeRate = methodsQuery.data?.exchange_rate || 7.2;

  const periods = useMemo(() => {
    if (!plan?.prices) return [];
    return plan.prices.map(p => ({
      code: p.period_code,
      label: getPeriodLabel(p.period_code),
      price: p.price_cny,
    }));
  }, [plan]);

  if (!planId) {
    return (
      <div className="p-4 md:p-6 max-w-3xl mx-auto">
        <div className="xboard-card p-6 md:p-8 text-center">
          <p className="mb-4" style={{ color: 'var(--muted-foreground)' }}>未选择套餐</p>
          <button onClick={() => navigate('/dashboard/plans')} className="px-4 py-2 rounded-lg text-sm text-white" style={{ background: 'var(--primary)' }}>
            选择套餐
          </button>
        </div>
      </div>
    );
  }

  if (planQuery.isLoading) {
    return <div className="p-4 md:p-6 text-center text-sm" style={{ color: 'var(--muted-foreground)' }}>加载中...</div>;
  }

  if (!plan) {
    return (
      <div className="p-4 md:p-6 max-w-3xl mx-auto">
        <div className="xboard-card p-6 md:p-8 text-center">
          <p className="mb-4" style={{ color: 'var(--destructive)' }}>套餐不存在</p>
          <button onClick={() => navigate('/dashboard/plans')} className="px-4 py-2 rounded-lg text-sm text-white" style={{ background: 'var(--primary)' }}>
            返回套餐列表
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="p-4 md:p-6 max-w-5xl mx-auto">
      {/* Back link */}
      <button
        onClick={() => navigate('/dashboard/plans')}
        className="text-sm mb-4 flex items-center gap-1 transition-colors"
        style={{ color: 'var(--muted-foreground)' }}
        onMouseEnter={e => (e.currentTarget.style.color = 'var(--primary)')}
        onMouseLeave={e => (e.currentTarget.style.color = 'var(--muted-foreground)')}
      >
        ← 返回套餐列表
      </button>

      <h1 className="text-xl font-bold mb-1" style={{ color: 'var(--foreground)' }}>查看并完成您的购买</h1>
      <p className="text-sm mb-5 md:mb-6" style={{ color: 'var(--muted-foreground)' }}>确认订单信息并选择支付方式</p>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-5">
        {/* Left column */}
        <div className="space-y-5">
          {/* 套餐摘要 */}
          <div className="xboard-card p-5">
            <h3 className="flex items-center gap-2 text-sm font-semibold mb-3" style={{ color: 'var(--foreground)' }}>
              <span>📦</span> 套餐摘要
            </h3>
            <div className="text-base font-bold mb-2" style={{ color: '#cd5c4d' }}>
              {formatTraffic(plan.traffic_bytes)} 购买日起每 30 天重置
            </div>
            <div className="space-y-1.5 mb-4">
              {(plan.features?.length ?? 0) > 0 ? plan.features!.map((feat, i) => {
                const clean = feat.replace(/^[✅✔️✓☑️\s]+/, '').trim();
                return (
                  <div key={i} className="flex items-start gap-2 text-sm">
                    <span className="text-green-500 mt-0.5 flex-shrink-0">✅</span>
                    <span style={{ color: 'var(--foreground)' }}>{clean || feat}</span>
                  </div>
                );
              }) : (
                <>
                  <div className="flex items-start gap-2 text-sm"><span className="text-green-500 mt-0.5">✅</span><span>{plan.speed_limit_mbps}Mbps 网络速率</span></div>
                  <div className="flex items-start gap-2 text-sm"><span className="text-green-500 mt-0.5">✅</span><span>全线路流媒体解锁</span></div>
                </>
              )}
            </div>
          </div>

          {/* 选择订阅周期 */}
          <div className="xboard-card p-5">
            <h3 className="flex items-center gap-2 text-sm font-semibold mb-3" style={{ color: 'var(--foreground)' }}>
              <span>⏱️</span> 选择订阅周期
            </h3>
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
              {periods.map(p => {
                const isSelected = selectedPeriod === p.code;
                return (
                  <button
                    key={p.code}
                    onClick={() => { setSelectedPeriod(p.code); setCouponResult(null); }}
                    className="p-3 rounded-lg border-2 text-center transition-all text-sm"
                    style={{
                      borderColor: isSelected ? 'var(--primary)' : 'var(--border)',
                      background: isSelected ? 'rgba(217,119,87,0.06)' : 'transparent',
                      color: isSelected ? 'var(--primary)' : 'var(--foreground)',
                    }}
                  >
                    <div className="font-medium">{p.label}</div>
                    <div className="text-base font-bold mt-1">¥{p.price.toFixed(0)}</div>
                    {p.code === 'month' && <div className="text-xs" style={{ color: 'var(--muted-foreground)' }}>≈ ${(p.price / 7.2).toFixed(2)}</div>}
                  </button>
                );
              })}
            </div>
          </div>
        </div>

        {/* Right column */}
        <div className="space-y-5">
          {/* 选择支付方式 */}
          <div className="xboard-card p-5">
            <h3 className="flex items-center gap-2 text-sm font-semibold mb-3" style={{ color: 'var(--foreground)' }}>
              <span>💳</span> 选择支付方式
            </h3>
            <div className="space-y-2">
              {methods.map(m => {
                const isSelected = selectedMethod === m.method;
                return (
                  <label
                    key={m.method}
                    className="flex flex-wrap items-center gap-3 p-3 rounded-lg border-2 cursor-pointer transition-all text-sm"
                    style={{
                      borderColor: isSelected ? 'var(--primary)' : 'var(--border)',
                      background: isSelected ? 'rgba(217,119,87,0.06)' : 'transparent',
                    }}
                  >
                    <input
                      type="radio"
                      name="payment"
                      value={m.method}
                      checked={isSelected}
                      onChange={() => setSelectedMethod(m.method)}
                      className="accent-teal-600"
                      style={{ accentColor: 'var(--primary)' }}
                    />
                    <PaymentMethodIcon method={m.method} size={26} />
                    <span className="flex-1 font-medium" style={{ color: 'var(--foreground)' }}>{m.name}</span>
                    <span className="text-xs" style={{ color: 'var(--muted-foreground)' }}>
                      {m.currency}
                    </span>
                    {isSelected && m.method === 'usdt_erc20' && evmNetworks.length > 0 && (
                      <div className="flex flex-wrap gap-2 mt-2 pl-8 w-full">
                        {evmNetworks.map(net => {
                          const active = selectedNetwork === net
                          return (
                            <button
                              key={net}
                              type="button"
                              onClick={(e) => { e.preventDefault(); setSelectedNetwork(net) }}
                              className="px-2.5 py-1 rounded-lg border text-xs font-medium transition-colors"
                              style={{
                                borderColor: active ? 'var(--primary)' : 'var(--border)',
                                color: active ? 'var(--primary)' : 'var(--muted-foreground)',
                                background: active ? 'rgba(217,119,87,0.08)' : 'transparent',
                              }}
                            >
                              {EVM_NETWORK_LABELS[net] || net}
                            </button>
                          )
                        })}
                      </div>
                    )}
                    {isSelected && m.hint && (
                      <div className="w-full mt-2 pl-8">
                        <div className="rounded-md p-2.5 text-xs" style={{ background: 'rgba(217,119,87,0.08)', border: '1px solid rgba(217,119,87,0.2)' }}>
                          <p style={{ color: 'var(--primary)' }}>⚠️ {m.hint}</p>
                        </div>
                      </div>
                    )}
                  </label>
                );
              })}
              {methodsQuery.isLoading && (
                <p className="text-sm text-center py-4" style={{ color: 'var(--muted-foreground)' }}>加载支付方式中...</p>
              )}
              {methodsQuery.isError && (
                <p className="text-sm text-center py-4 px-3 rounded-lg" style={{ color: 'var(--destructive)', background: 'rgba(205,92,77,0.08)' }}>
                  支付方式加载失败，请刷新页面重试
                </p>
              )}
              {!methodsQuery.isLoading && !methodsQuery.isError && methods.length === 0 && (
                <p className="text-sm text-center py-4" style={{ color: 'var(--muted-foreground)' }}>暂无可⽤支付方式</p>
              )}
            </div>
          </div>

          {/* 优惠码 + 订单摘要 */}
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-5">
            {/* 优惠码 */}
            <div className="xboard-card p-5">
              <h3 className="text-sm font-semibold mb-3" style={{ color: 'var(--foreground)' }}>优惠码</h3>
              <div className="flex gap-2 items-stretch">
                <input
                  type="text"
                  value={couponCode}
                  onChange={e => setCouponCode(e.target.value)}
                  placeholder="请输入优惠码"
                  className="flex-1 min-w-0 px-3 py-2 rounded-lg border text-sm h-10"
                  style={{ borderColor: 'var(--border)', background: 'var(--card)', color: 'var(--foreground)' }}
                />
                <button
                  onClick={() => couponMut.mutate()}
                  disabled={couponMut.isPending || !couponCode.trim()}
                  className="px-4 rounded-lg text-sm font-medium border whitespace-nowrap transition-colors hover:opacity-80 disabled:opacity-40 disabled:cursor-not-allowed flex-shrink-0 flex items-center justify-center"
                  style={{ borderColor: 'var(--primary)', color: 'var(--primary)', background: 'transparent', minWidth: '72px' }}
                >
                  {couponMut.isPending ? '验证中...' : '验证'}
                </button>
              </div>
              <p className="text-xs mt-2" style={{ color: 'var(--muted-foreground)' }}>输入优惠码可获得订单折扣</p>
              {couponResult && (
                <p className="text-xs mt-1" style={{ color: couponResult.valid ? 'var(--success)' : 'var(--destructive)' }}>
                  {couponResult.valid ? `✓ 优惠码有效，折扣 ¥${couponResult.discount_amount?.toFixed(2)}` : '✗ 优惠码无效'}
                </p>
              )}
              {couponError && (
                <p className="text-xs mt-1" style={{ color: 'var(--destructive)' }}>✗ {couponError}</p>
              )}
            </div>

            {/* 订单摘要 */}
            <div className="xboard-card p-5">
              <h3 className="text-sm font-semibold mb-3" style={{ color: 'var(--foreground)' }}>订单摘要</h3>
              <div className="space-y-2 text-sm">
                <div className="flex justify-between"><span style={{ color: 'var(--muted-foreground)' }}>原价</span><span style={{ color: 'var(--foreground)' }}>¥{basePrice.toFixed(2)}</span></div>
                {discount > 0 && <div className="flex justify-between"><span style={{ color: 'var(--success)' }}>优惠</span><span style={{ color: 'var(--success)' }}>-¥{discount.toFixed(2)}</span></div>}
                <div className="flex justify-between pt-2 border-t" style={{ borderColor: 'var(--border)' }}>
                  <span className="font-semibold" style={{ color: 'var(--foreground)' }}>总计</span>
                  <span className="text-lg font-bold" style={{ color: 'var(--primary)' }}>¥{totalPrice.toFixed(2)}</span>
                </div>
              </div>
              <button
                onClick={() => setConfirmOpen(true)}
                disabled={orderMut.isPending || (totalPrice > 0 && !selectedMethod) || (selectedMethod === 'usdt_erc20' && !selectedNetwork)}
                className="w-full mt-4 py-2.5 rounded-lg text-sm font-medium text-white transition-colors shadow-sm disabled:opacity-50"
                style={{ background: 'var(--primary)' }}
              >
                {orderMut.isPending ? '处理中...' : totalPrice === 0 ? '免费获取' : '去结算'}
              </button>
            </div>
          </div>
        </div>
      </div>

      <ConfirmPayCard
        open={confirmOpen}
        planName={plan.name}
        periodLabel={getPeriodLabel(selectedPeriod)}
        basePrice={basePrice}
        discount={discount}
        totalPrice={totalPrice}
        method={methodObj}
        networkLabel={selectedMethod === 'usdt_erc20' ? (EVM_NETWORK_LABELS[selectedNetwork] || selectedNetwork) : (selectedMethod === 'usdt_bep20' ? 'BSC (BEP20)' : undefined)}
        rate={exchangeRate}
        submitting={orderMut.isPending}
        onConfirm={() => orderMut.mutate()}
        onCancel={() => setConfirmOpen(false)}
      />

      {/* Footer links */}
      <div className="text-center mt-6 text-xs" style={{ color: 'var(--muted-foreground)' }}>
        完成购买即表示您同意我们的服务条款
        <a href="#" className="mx-2 underline" style={{ color: 'var(--primary)' }}>服务条款</a>·
        <a href="#" className="mx-2 underline" style={{ color: 'var(--primary)' }}>隐私政策</a>
      </div>
    </div>
  );
}

