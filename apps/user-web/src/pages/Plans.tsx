import { useQuery } from '@tanstack/react-query';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { api } from '../lib/api';
import { EP, adaptPlan, bytesToGB, getPeriodLabel } from '../lib/endpoints';
import type { PlanResponse, NodeInfo } from '../lib/endpoints';
import { useState } from 'react';

function formatTraffic(bytes: number): string {
  if (bytes <= 0) return '不限流量';
  const gb = bytesToGB(bytes);
  return gb >= 1024 ? `${(gb / 1024).toFixed(0)} TB` : `${gb.toFixed(0)} GB`;
}

function NodeModal({ plan, onClose }: { plan: PlanResponse; onClose: () => void }) {
  const nodesQuery = useQuery<NodeInfo[]>({
    queryKey: ['plan-nodes', plan.id],
    queryFn: () => api.get(EP.PLAN_NODES(plan.id)),
    retry: 1,
  });

  const nodes = nodesQuery.data || [];
  const onlineCount = nodes.filter(n => n.is_online).length;

  return (
    <div className="modal-overlay animate-fade-in" onClick={onClose}>
      <div
        className="bg-white rounded-xl shadow-2xl w-full max-w-lg max-h-[80vh] flex flex-col animate-slide-up overflow-hidden"
        onClick={e => e.stopPropagation()}
      >
        {/* Purple header like xboard */}
        <div className="px-5 py-4 flex items-center justify-between text-white" style={{ background: 'var(--primary)' }}>
          <div className="flex items-center gap-2">
            <span className="text-lg">🖥️</span>
            <h3 className="font-semibold text-sm">{plan.name} - 可用节点 ({nodes.length})</h3>
          </div>
          <button onClick={onClose} className="w-7 h-7 rounded-full flex items-center justify-center hover:bg-white hover:bg-opacity-20 transition-colors text-lg leading-none">×</button>
        </div>
        {/* Node list */}
        <div className="flex-1 overflow-y-auto">
          {nodesQuery.isLoading ? (
            <div className="p-8 text-center text-sm" style={{ color: 'var(--muted-foreground)' }}>加载节点中...</div>
          ) : nodes.length === 0 ? (
            <div className="p-8 text-center text-sm" style={{ color: 'var(--muted-foreground)' }}>暂无节点</div>
          ) : (
            nodes.map(node => (
              <div
                key={node.id}
                className="flex items-center gap-2 px-5 py-2.5 border-b text-sm"
                style={{ borderColor: 'var(--border)' }}
              >
                <span className={`status-dot ${node.is_online ? 'online' : 'offline'}`} />
                <div className="flex-1 min-w-0">
                  <div className="flex items-center flex-wrap gap-1">
                    <span className="font-medium truncate" style={{ color: 'var(--foreground)' }}>
                      {node.country_flag ? `${node.country_flag} ` : ''}{node.name}
                    </span>
                    {node.protocol && (
                      <span className="proto-tag">{node.protocol}</span>
                    )}
                    {node.tags?.slice(0, 2).map(tag => (
                      <span key={tag} className="proto-tag" style={{ background: 'rgba(100,116,139,0.1)', color: 'var(--muted-foreground)' }}>{tag}</span>
                    ))}
                  </div>
                </div>
                <div className="flex items-center gap-2 flex-shrink-0">
                  {node.rate !== undefined && node.rate !== 1 && (
                    <span className="mult-badge">{node.rate}x</span>
                  )}
                  <span className={node.is_online ? 'available-badge' : 'text-xs'} style={!node.is_online ? { color: 'var(--destructive)' } : {}}>
                    {node.is_online ? '可用' : '离线'}
                  </span>
                </div>
              </div>
            ))
          )}
        </div>
        {/* Footer */}
        <div className="px-5 py-3 border-t flex justify-end" style={{ borderColor: 'var(--border)' }}>
          <button
            onClick={onClose}
            className="px-4 py-1.5 rounded-lg text-sm border transition-colors"
            style={{ borderColor: 'var(--border)', color: 'var(--foreground)' }}
          >
            关闭
          </button>
        </div>
      </div>
    </div>
  );
}

export function Plans() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const [nodeModalPlan, setNodeModalPlan] = useState<PlanResponse | null>(null);

  const plansQuery = useQuery<PlanResponse[]>({
    queryKey: ['plans'],
    queryFn: async () => {
      const data = await api.get<PlanResponse[]>(EP.PLANS);
      return (data || []).filter(p => p.status === 'active').map(adaptPlan);
    },
  });

  const plans = plansQuery.data || [];

  const handleBuy = (plan: PlanResponse, periodCode?: string) => {
    const params = new URLSearchParams();
    params.set('plan_id', plan.id);
    if (periodCode) params.set('period', periodCode);
    navigate(`/dashboard/checkout?${params.toString()}`);
  };

  // Default period: month if available, else first price
  const getDefaultPeriod = (plan: PlanResponse) => {
    const month = plan.prices?.find(p => p.period_code === 'month');
    return month || plan.prices?.[0];
  };

  return (
    <div className="p-6 max-w-7xl mx-auto">
      <div className="mb-6">
        <h1 className="text-xl font-bold mb-1" style={{ color: 'var(--foreground)' }}>购买订阅</h1>
        <p className="text-sm" style={{ color: 'var(--muted-foreground)' }}>选择适合您的订阅套餐</p>
      </div>

      {plansQuery.isLoading ? (
        <div className="text-center py-12 text-sm" style={{ color: 'var(--muted-foreground)' }}>加载套餐中...</div>
      ) : plans.length === 0 ? (
        <div className="text-center py-12 xboard-card">
          <p className="text-sm mb-4" style={{ color: 'var(--muted-foreground)' }}>暂无可用套餐</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
          {plans.map(plan => {
            const defaultPrice = getDefaultPeriod(plan);
            const priceCNY = defaultPrice?.price_cny || 0;
            const nodeCount = plan.node_count || 0;

            return (
              <div key={plan.id} className="xboard-card p-5 flex flex-col hover:shadow-md transition-shadow">
                {/* Plan name + traffic highlight */}
                <h3 className="text-base font-semibold mb-1" style={{ color: 'var(--foreground)' }}>{plan.name}</h3>
                <div className="text-base font-bold mb-3" style={{ color: '#ef4444' }}>
                  {formatTraffic(plan.traffic_bytes)} 每月重置
                </div>

                {/* Features */}
                <div className="space-y-1.5 mb-4 flex-1">
                  {(plan.features?.length ?? 0) > 0 ? (
                    plan.features!.map((feat, i) => {
                      const clean = feat.replace(/^[✅✔️✓☑️\s]+/, '').trim();
                      return (
                        <div key={i} className="flex items-start gap-2 text-sm">
                          <span className="text-green-500 mt-0.5 flex-shrink-0">✅</span>
                          <span style={{ color: 'var(--foreground)' }}>{clean || feat}</span>
                        </div>
                      );
                    })
                  ) : (
                    <>
                      {plan.speed_limit_mbps && plan.speed_limit_mbps > 0 && (
                        <div className="flex items-start gap-2 text-sm">
                          <span className="text-green-500 mt-0.5">✅</span>
                          <span>{plan.speed_limit_mbps}Mbps 网络速率</span>
                        </div>
                      )}
                      {plan.device_limit && plan.device_limit > 0 && (
                        <div className="flex items-start gap-2 text-sm">
                          <span className="text-green-500 mt-0.5">✅</span>
                          <span>最多 {plan.device_limit} 台设备</span>
                        </div>
                      )}
                      <div className="flex items-start gap-2 text-sm">
                        <span className="text-green-500 mt-0.5">✅</span>
                        <span>全线路流媒体解锁</span>
                      </div>
                    </>
                  )}
                </div>

                {/* Specs summary */}
                <div className="space-y-1 mb-4 text-xs" style={{ color: 'var(--muted-foreground)' }}>
                  <div className="flex items-center gap-2">
                    <span className="text-green-500">✓</span>
                    <span>{formatTraffic(plan.traffic_bytes)}</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-green-500">✓</span>
                    <span>{plan.speed_limit_mbps && plan.speed_limit_mbps > 0 ? `${plan.speed_limit_mbps}Mbps` : '不限速'}</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-green-500">✓</span>
                    <span>{plan.device_limit && plan.device_limit > 0 ? `最多${plan.device_limit}台设备` : '不限制设备数量'}</span>
                  </div>
                </div>

                {/* 查看可用节点 button - light gray, xboard style */}
                <button
                  onClick={() => setNodeModalPlan(plan)}
                  className="w-full py-2 rounded-lg text-sm font-medium mb-2 transition-colors flex items-center justify-center gap-1"
                  style={{ background: 'var(--muted)', color: 'var(--muted-foreground)' }}
                  onMouseEnter={e => (e.currentTarget.style.background = 'var(--border)')}
                  onMouseLeave={e => (e.currentTarget.style.background = 'var(--muted)')}
                >
                  <span>🖥️</span> 查看可用节点 ({nodeCount})
                </button>

                {/* 立即购买 button - purple, xboard style */}
                <button
                  onClick={() => handleBuy(plan, defaultPrice?.period_code)}
                  className="w-full py-2.5 rounded-lg text-sm font-medium text-white transition-colors shadow-sm"
                  style={{ background: 'var(--primary)' }}
                  onMouseEnter={e => (e.currentTarget.style.opacity = '0.9')}
                  onMouseLeave={e => (e.currentTarget.style.opacity = '1')}
                >
                  立即购买
                  {defaultPrice && (
                    <span className="ml-1 opacity-90">¥{priceCNY.toFixed(2)}/{getPeriodLabel(defaultPrice.period_code)}</span>
                  )}
                </button>
              </div>
            );
          })}
        </div>
      )}

      {nodeModalPlan && <NodeModal plan={nodeModalPlan} onClose={() => setNodeModalPlan(null)} />}
    </div>
  );
}
