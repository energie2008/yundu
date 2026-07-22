import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { api } from '../lib/api';
import { EP, formatBytes, getTrafficPercentage, getDaysRemaining, formatDate,
  getSubscriptionUrl, detectClientFromUA, getOneClickImportUrl,
  bytesToGB,
} from '../lib/endpoints';
import { useAuth } from '../lib/auth';
import type {
  SubscriptionResponse, SubscriptionTokenResponse, NodeInfo, TrafficLog,
} from '../lib/endpoints';
import { useState, useMemo } from 'react';

// Platform client icon/name mapping
const CLIENT_GROUPS: Record<string, { id: string; name: string; icon: string }[]> = {
  Windows: [
    { id: 'clash', name: 'Clash', icon: '⬢' },
    { id: 'singbox', name: 'Singbox', icon: '🛡️' },
    { id: 'v2rayn', name: 'v2rayN', icon: '📦' },
  ],
  macOS: [
    { id: 'clashx', name: 'ClashX', icon: '⬢' },
    { id: 'singbox', name: 'Singbox', icon: '🛡️' },
  ],
  Android: [
    { id: 'clashforandroid', name: 'ClashForAndroid', icon: '⬢' },
    { id: 'singbox', name: 'Singbox', icon: '🛡️' },
  ],
  iOS: [
    { id: 'shadowrocket', name: 'Shadowrocket', icon: '🚀' },
    { id: 'singbox', name: 'Singbox', icon: '🛡️' },
  ],
};

function CopyButton({ text, label = '复制' }: { text: string; label?: string }) {
  const [copied, setCopied] = useState(false);
  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {}
  };
  return (
    <button
      onClick={handleCopy}
      className="px-3 py-1.5 rounded-lg text-xs font-medium transition-colors"
      style={{ background: 'var(--primary)', color: 'white' }}
    >
      {copied ? '✓ 已复制' : label}
    </button>
  );
}

// Simple QR code placeholder using an online service (we'll use a data approach)
function QRCodeDisplay({ url }: { url: string }) {
  // Use a free QR code generator API
  const qrSrc = `https://api.qrserver.com/v1/create-qr-code/?size=200x200&data=${encodeURIComponent(url)}`;
  return (
    <div className="flex justify-center py-4">
      <img src={qrSrc} alt="订阅二维码" width={200} height={200} style={{ borderRadius: 8 }} />
    </div>
  );
}

// Collapsible panel component
function CollapsiblePanel({
  title,
  badge,
  defaultOpen = false,
  children,
}: {
  title: string;
  badge?: React.ReactNode;
  defaultOpen?: boolean;
  children: React.ReactNode;
}) {
  const [open, setOpen] = useState(defaultOpen);
  return (
    <div className="xboard-card overflow-hidden">
      <button
        onClick={() => setOpen(!open)}
        className="w-full flex items-center justify-between px-5 py-3.5 text-left transition-colors"
        style={{ color: 'var(--foreground)' }}
        onMouseEnter={e => (e.currentTarget.style.background = 'var(--muted)')}
        onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
      >
        <div className="flex items-center gap-2">
          <span className="text-sm font-semibold">{title}</span>
          {badge}
        </div>
        <span
          className="text-sm transition-transform"
          style={{
            color: 'var(--muted-foreground)',
            transform: open ? 'rotate(180deg)' : 'rotate(0deg)',
            display: 'inline-block',
          }}
        >
          ⌄
        </span>
      </button>
      {open && <div>{children}</div>}
    </div>
  );
}

function TrafficChart({ data }: { data?: TrafficLog[] }) {
  const logs = data || [];
  const maxGB = useMemo(() => {
    let max = 0.01;
    for (const l of logs) {
      const gb = bytesToGB(l.total);
      if (gb > max) max = gb;
    }
    return Math.max(max, 0.05);
  }, [logs]);

  const yLabels = useMemo(() => {
    const steps = 5;
    const labels: number[] = [];
    for (let i = 0; i <= steps; i++) {
      labels.push((maxGB / steps) * (steps - i));
    }
    return labels;
  }, [maxGB]);

  const formatDateShort = (dateStr: string) => {
    const d = new Date(dateStr);
    return `${d.getMonth() + 1}/${d.getDate()}`;
  };

  return (
    <div className="px-5 pb-5">
      <div className="flex items-center gap-2 mb-2">
        <span className="text-xs" style={{ color: 'var(--muted-foreground)' }}>最近7天流量使用</span>
        <div className="ml-auto flex items-center gap-3">
          <div className="flex items-center gap-1">
            <span className="w-2.5 h-2.5 rounded-sm" style={{ background: 'var(--primary)' }} />
            <span className="text-xs" style={{ color: 'var(--muted-foreground)' }}>下载</span>
          </div>
          <div className="flex items-center gap-1">
            <span className="w-2.5 h-2.5 rounded-sm" style={{ background: 'var(--primary)', opacity: 0.4 }} />
            <span className="text-xs" style={{ color: 'var(--muted-foreground)' }}>上传</span>
          </div>
        </div>
      </div>
      <div className="relative h-48">
        <div className="absolute left-0 top-0 bottom-6 w-14 flex flex-col justify-between text-[10px]" style={{ color: 'var(--muted-foreground)' }}>
          {yLabels.map((v, i) => (
            <span key={i}>{v.toFixed(2)} GB</span>
          ))}
        </div>
        <div className="ml-14 h-full relative border-l border-b" style={{ borderColor: 'var(--border)' }}>
          {[0, 1, 2, 3, 4].map(i => (
            <div
              key={i}
              className="absolute left-0 right-0 border-t border-dashed"
              style={{ top: `${i * 20}%`, borderColor: 'var(--border)' }}
            />
          ))}
          <div className="absolute inset-0 flex items-end justify-around px-2 pb-0">
            {logs.map((l) => {
              const downGB = bytesToGB(l.download);
              const upGB = bytesToGB(l.upload);
              const totalGB = bytesToGB(l.total);
              const hTotal = Math.max((totalGB / maxGB) * 80, totalGB > 0 ? 3 : 0);
              const hDown = totalGB > 0 ? (downGB / totalGB) * hTotal : 0;
              const hUp = totalGB > 0 ? (upGB / totalGB) * hTotal : 0;
              return (
                <div key={l.date} className="flex flex-col items-center gap-1" style={{ width: '10%' }}>
                  <div
                    className="w-full flex flex-col justify-end rounded-t overflow-hidden"
                    style={{ height: `${hTotal}%` }}
                  >
                    {hUp > 0 && (
                      <div
                        className="w-full transition-all"
                        style={{ height: `${(hUp / hTotal) * 100}%`, background: 'var(--primary)', opacity: 0.4 }}
                      />
                    )}
                    {hDown > 0 && (
                      <div
                        className="w-full transition-all"
                        style={{ height: `${(hDown / hTotal) * 100}%`, background: 'var(--primary)', opacity: 0.8 }}
                      />
                    )}
                  </div>
                </div>
              );
            })}
          </div>
          <div className="absolute -bottom-5 left-0 right-0 flex justify-around text-[10px]" style={{ color: 'var(--muted-foreground)' }}>
            {logs.map(l => <span key={l.date}>{formatDateShort(l.date)}</span>)}
          </div>
        </div>
      </div>
    </div>
  );
}

export function Dashboard() {
  const { user } = useAuth();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const detectedClient = detectClientFromUA();

  // Panel expand states
  const [showAddress, setShowAddress] = useState(false);
  const [importOpen, setImportOpen] = useState(false);
  const [importTab, setImportTab] = useState<'copy' | 'qrcode'>('copy');
  const [clientPlatform, setClientPlatform] = useState<keyof typeof CLIENT_GROUPS>('Windows');

  const subQuery = useQuery<SubscriptionResponse>({
    queryKey: ['subscription'],
    queryFn: () => api.get(EP.SUBSCRIPTION),
    retry: 1,
  });

  const nodesQuery = useQuery<NodeInfo[]>({
    queryKey: ['my-nodes'],
    queryFn: () => api.get(EP.MY_NODES),
    retry: 1,
  });

  const tokensQuery = useQuery<SubscriptionTokenResponse[]>({
    queryKey: ['subscription-tokens'],
    queryFn: () => api.get(EP.SUBSCRIPTION_TOKENS),
    retry: 1,
  });

  const trafficLogsQuery = useQuery<TrafficLog[]>({
    queryKey: ['traffic-logs'],
    queryFn: () => api.get(EP.TRAFFIC_LOGS),
    retry: 1,
  });

  // Ensure token - gets or creates a full token with the raw token value (POST endpoint)
  const ensureTokenQuery = useQuery<{ token: SubscriptionTokenResponse; is_new: boolean }>({
    queryKey: ['ensure-subscription-token'],
    queryFn: () => api.post(EP.ENSURE_TOKEN),
    retry: 1,
    enabled: !!subQuery.data, // only fetch after subscription is loaded
  });

  const resetTokenMut = useMutation({
    mutationFn: () => api.post(EP.SUBSCRIPTION_RESET),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['ensure-subscription-token'] });
      qc.invalidateQueries({ queryKey: ['subscription-tokens'] });
    },
  });

  const sub = subQuery.data;
  const nodes = nodesQuery.data || [];
  const tokens = tokensQuery.data || [];
  // Use the ensured token (has full token value), fall back to list token
  const ensuredToken = ensureTokenQuery.data?.token;
  const activeToken = ensuredToken || tokens.find(t => t.status === 'active') || tokens[0];

  const onlineCount = nodes.filter(n => n.is_online).length;
  const trafficPct = sub ? getTrafficPercentage(sub.traffic_used_bytes, sub.traffic_quota_bytes) : 0;
  const downloadGB = sub ? bytesToGB(sub.download_bytes || sub.traffic_used_bytes).toFixed(2) : '0';
  const uploadGB = sub ? bytesToGB(sub.upload_bytes || 0).toFixed(2) : '0';
  const quotaGB = sub && sub.traffic_quota_bytes > 0 ? bytesToGB(sub.traffic_quota_bytes).toFixed(2) : null;
  const remainingGB = sub && sub.traffic_quota_bytes > 0
    ? bytesToGB(Math.max(0, sub.traffic_quota_bytes - sub.traffic_used_bytes)).toFixed(2)
    : null;
  const daysLeft = sub ? getDaysRemaining(sub.expires_at) : null;
  const isUnlimited = !sub || !sub.traffic_quota_bytes || sub.traffic_quota_bytes === 0;
  const trafficLogs = trafficLogsQuery.data || [];

  const subscriptionUrl = activeToken?.token ? getSubscriptionUrl(activeToken.token) : '';

  const handleResetToken = () => {
    if (confirm('确定要重置订阅链接吗？重置后旧链接将失效。')) {
      resetTokenMut.mutate();
    }
  };

  const username = user?.email?.split('@')[0] || '用户';

  // Quick action items
  const quickActions = [
    { icon: '🛒', label: '购买订阅', desc: '购买订阅套餐', to: '/dashboard/plans', color: '#7c5cfc' },
    { icon: '📥', label: '客户端软件下载', desc: '下载各平台客户端', to: '/dashboard/knowledge', color: '#3b82f6' },
    { icon: '📋', label: '我的订单', desc: '查看订单历史', to: '/dashboard/orders', color: '#7c5cfc' },
    { icon: '💬', label: '工单支持', desc: '获取客服帮助', to: '/dashboard/tickets', color: '#f59e0b' },
    { icon: '🎁', label: '邀请好友', desc: '赚取佣金奖励', to: '/dashboard/invite', color: '#10b981' },
  ];

  return (
    <div className="p-6 max-w-6xl mx-auto">
      {/* Page title */}
      <div className="mb-6">
        <h1 className="text-2xl font-bold mb-1" style={{ color: 'var(--foreground)' }}>
          仪表盘, {username}!
        </h1>
        <p className="text-sm" style={{ color: 'var(--muted-foreground)' }}>
          欢迎回来，这是您的账户概览。
        </p>
      </div>

      {/* Two column layout: My Subscription + Quick Actions */}
      <div className="grid grid-cols-1 lg:grid-cols-5 gap-5 mb-5">
        {/* My Subscription - left wider */}
        <div className="lg:col-span-3">
          <div className="xboard-card p-5">
            {/* Header */}
            <div className="flex items-start justify-between mb-1">
              <h3 className="text-sm font-semibold" style={{ color: 'var(--foreground)' }}>我的订阅</h3>
              {isUnlimited && (
                <span className="text-xs px-2 py-0.5 rounded-full" style={{ background: 'rgba(124,92,252,0.1)', color: 'var(--primary)' }}>
                  无限制
                </span>
              )}
            </div>

            {sub ? (
              <>
                <div className="text-xs mb-3" style={{ color: 'var(--muted-foreground)' }}>
                  {sub.plan_name}
                  {sub.price ? ` · ¥${sub.price} / 一次性` : ''}
                </div>

                {/* Expiration */}
                <div className="flex items-center justify-between text-xs mb-4">
                  <span style={{ color: 'var(--muted-foreground)' }}>到期时间</span>
                  <span className="font-medium" style={{ color: 'var(--foreground)' }}>
                    {sub.expires_at ? (daysLeft !== null ? `剩余 ${daysLeft} 天` : formatDate(sub.expires_at)) : '无限期'}
                  </span>
                </div>

                {/* Total traffic */}
                <div className="mb-1 flex items-center justify-between text-xs">
                  <span style={{ color: 'var(--muted-foreground)' }}>总流量</span>
                  <span className="font-medium" style={{ color: 'var(--foreground)' }}>
                    {isUnlimited ? '∞' : `${quotaGB} GB`}
                  </span>
                </div>

                {/* Progress bar */}
                <div className="w-full h-2 rounded-full overflow-hidden mb-1" style={{ background: 'var(--muted)' }}>
                  <div
                    className="h-full rounded-full transition-all"
                    style={{
                      width: `${Math.min(trafficPct, 100)}%`,
                      background: trafficPct > 90 ? 'var(--destructive)' : 'var(--primary)',
                    }}
                  />
                </div>
                <div className="text-[10px] mb-3" style={{ color: 'var(--muted-foreground)' }}>
                  {trafficPct.toFixed(1)}% 已使用
                </div>

                {/* Upload / Download */}
                <div className="flex items-center gap-2 text-xs mb-2">
                  <span style={{ color: 'var(--muted-foreground)' }}>
                    ⏫ {uploadGB} GB
                  </span>
                  <span style={{ color: 'var(--muted-foreground)' }}>/</span>
                  <span style={{ color: 'var(--muted-foreground)' }}>
                    ⏬ {downloadGB} GB
                  </span>
                </div>

                {/* Remaining */}
                <div className="flex items-center justify-between text-sm mb-4">
                  <span style={{ color: 'var(--muted-foreground)' }}>剩余流量</span>
                  <span className="font-semibold" style={{ color: 'var(--success)' }}>
                    {isUnlimited ? '∞' : `${remainingGB} GB`}
                  </span>
                </div>

                {/* Show subscription address link */}
                <button
                  onClick={() => { setShowAddress(!showAddress); if (showAddress) setImportOpen(false); }}
                  className="text-xs mb-3 flex items-center gap-1 transition-colors"
                  style={{ color: 'var(--primary)' }}
                >
                  {showAddress ? '隐藏订阅地址' : '显示订阅地址'}
                </button>

                {/* Quick Import area - shown when showAddress is true */}
                {showAddress && (
                  <div className="mb-4 rounded-xl overflow-hidden" style={{ border: importOpen ? '1px solid var(--border)' : 'none' }}>
                    {/* Quick import button - light gray when collapsed, purple when expanded */}
                    <button
                      onClick={() => setImportOpen(!importOpen)}
                      className="w-full py-2.5 px-4 flex items-center justify-center gap-2 text-sm font-medium rounded-lg transition-colors"
                      style={{
                        background: importOpen ? '#9580e8' : 'var(--muted)',
                        color: importOpen ? 'white' : 'var(--foreground)',
                      }}
                      onMouseEnter={e => {
                        if (!importOpen) e.currentTarget.style.background = 'var(--border)';
                        else e.currentTarget.style.background = '#846ddb';
                      }}
                      onMouseLeave={e => {
                        if (!importOpen) e.currentTarget.style.background = 'var(--muted)';
                        else e.currentTarget.style.background = '#9580e8';
                      }}
                    >
                      <span>📋</span> 快速导入订阅 <span style={{ transition: 'transform 0.2s', transform: importOpen ? 'rotate(180deg)' : 'rotate(0deg)', display: 'inline-block' }}>⌄</span>
                    </button>

                    {/* Expanded content - only when importOpen */}
                    {importOpen && (
                      <div className="p-4">
                        {ensureTokenQuery.isLoading ? (
                          <div className="p-4 text-center text-sm" style={{ color: 'var(--muted-foreground)' }}>加载订阅地址中...</div>
                        ) : ensureTokenQuery.isError ? (
                          <div className="p-4 text-center text-sm" style={{ color: 'var(--destructive)' }}>订阅地址加载失败</div>
                        ) : !subscriptionUrl ? (
                          <div className="p-4 text-center text-sm" style={{ color: 'var(--muted-foreground)' }}>暂无订阅地址</div>
                        ) : (
                          <>
                        {/* Copy / QRCode tabs */}
                        <div className="grid grid-cols-2 gap-2 mb-4">
                          <button
                            onClick={() => setImportTab('copy')}
                            className="py-2.5 rounded-lg text-sm font-medium flex items-center justify-center gap-1.5 transition-colors"
                            style={{
                              background: importTab === 'copy' ? 'rgba(124,92,252,0.08)' : 'var(--muted)',
                              color: importTab === 'copy' ? 'var(--primary)' : 'var(--muted-foreground)',
                            }}
                          >
                            <span>📋</span> 复制
                          </button>
                          <button
                            onClick={() => setImportTab('qrcode')}
                            className="py-2.5 rounded-lg text-sm font-medium flex items-center justify-center gap-1.5 transition-colors"
                            style={{
                              background: importTab === 'qrcode' ? 'rgba(124,92,252,0.08)' : 'var(--muted)',
                              color: importTab === 'qrcode' ? 'var(--primary)' : 'var(--muted-foreground)',
                            }}
                          >
                            <span>📱</span> 二维码
                          </button>
                        </div>

                        {importTab === 'copy' ? (
                          <div className="flex items-center gap-2 p-2.5 rounded-lg mb-3" style={{ background: 'var(--muted)' }}>
                            <code className="flex-1 text-xs break-all" style={{ color: 'var(--foreground)' }}>
                              {subscriptionUrl}
                            </code>
                            <CopyButton text={subscriptionUrl} />
                          </div>
                        ) : (
                          <QRCodeDisplay url={subscriptionUrl} />
                        )}

                        {/* Platform selection */}
                        <div className="flex items-center justify-between text-xs mb-2">
                          <span className="font-medium" style={{ color: 'var(--foreground)' }}>{clientPlatform}</span>
                          <div className="flex gap-3">
                            {(Object.keys(CLIENT_GROUPS) as (keyof typeof CLIENT_GROUPS)[]).map(p => (
                              <button
                                key={p}
                                onClick={() => setClientPlatform(p)}
                                className="transition-colors text-xs"
                                style={{ color: clientPlatform === p ? 'var(--primary)' : 'var(--muted-foreground)' }}
                              >
                                {p === 'Windows' ? 'Windows' : p === 'macOS' ? 'macOS' : p === 'Android' ? 'Android' : '其他平台'}
                              </button>
                            ))}
                          </div>
                        </div>

                        {/* Client buttons */}
                        <div className="grid grid-cols-2 gap-2">
                          {CLIENT_GROUPS[clientPlatform].map(c => {
                            const link = getOneClickImportUrl(c.id, subscriptionUrl);
                            return (
                              <a
                                key={c.id}
                                href={link || '#'}
                                className="py-3 rounded-lg flex flex-col items-center gap-1 transition-colors"
                                style={{ background: 'var(--muted)', color: 'var(--foreground)' }}
                                onMouseEnter={e => (e.currentTarget.style.background = 'var(--border)')}
                                onMouseLeave={e => (e.currentTarget.style.background = 'var(--muted)')}
                              >
                                <span className="text-lg" style={{ color: 'var(--primary)' }}>{c.icon}</span>
                                <span className="text-sm font-medium">{c.name}</span>
                              </a>
                            );
                          })}
                        </div>
                          </>
                        )}
                      </div>
                    )}
                  </div>
                )}

                {/* Action buttons */}
                <div className="grid grid-cols-2 gap-2">
                  <button
                    onClick={() => navigate('/dashboard/plans')}
                    className="py-2.5 rounded-lg text-sm font-medium text-white transition-colors"
                    style={{ background: 'var(--primary)' }}
                    onMouseEnter={e => (e.currentTarget.style.background = '#6a4ce0')}
                    onMouseLeave={e => (e.currentTarget.style.background = 'var(--primary)')}
                  >
                    🔄 续费订阅
                  </button>
                  <button
                    onClick={handleResetToken}
                    disabled={resetTokenMut.isPending}
                    className="py-2.5 rounded-lg text-sm font-medium transition-colors"
                    style={{ background: 'var(--muted)', color: 'var(--muted-foreground)' }}
                    onMouseEnter={e => (e.currentTarget.style.background = 'var(--border)')}
                    onMouseLeave={e => (e.currentTarget.style.background = 'var(--muted)')}
                  >
                    🔄 重置流量
                  </button>
                </div>
              </>
            ) : (
              <div className="py-6 text-center">
                <p className="text-sm mb-4" style={{ color: 'var(--muted-foreground)' }}>
                  您还没有激活订阅
                </p>
                <button
                  onClick={() => navigate('/dashboard/plans')}
                  className="px-6 py-2.5 rounded-lg text-sm font-medium text-white"
                  style={{ background: 'var(--primary)' }}
                >
                  立即购买
                </button>
              </div>
            )}
          </div>
        </div>

        {/* Quick Actions - right narrower */}
        <div className="lg:col-span-2">
          <div className="xboard-card p-5">
            <h3 className="text-sm font-semibold mb-4" style={{ color: 'var(--foreground)' }}>快捷操作</h3>
            <div className="space-y-1">
              {quickActions.map(action => (
                <button
                  key={action.to}
                  onClick={() => navigate(action.to)}
                  className="w-full flex items-center gap-3 px-3 py-2.5 rounded-lg transition-colors text-left"
                  onMouseEnter={e => (e.currentTarget.style.background = 'var(--muted)')}
                  onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
                >
                  <div
                    className="w-8 h-8 rounded-lg flex items-center justify-center text-base flex-shrink-0"
                    style={{ background: `${action.color}15` }}
                  >
                    <span style={{ color: action.color }}>{action.icon}</span>
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="text-sm font-medium" style={{ color: 'var(--foreground)' }}>{action.label}</div>
                    <div className="text-xs" style={{ color: 'var(--muted-foreground)' }}>{action.desc}</div>
                  </div>
                  <span className="text-xs" style={{ color: 'var(--muted-foreground)' }}>›</span>
                </button>
              ))}
            </div>
          </div>
        </div>
      </div>

      {/* Traffic Usage Records - collapsible */}
      <div className="mb-5">
        <CollapsiblePanel title="流量使用记录">
          <div className="px-5 pb-5">
            <div className="rounded-xl overflow-hidden border" style={{ borderColor: 'var(--border)' }}>
              <TrafficChart data={trafficLogs} />
            </div>
          </div>
        </CollapsiblePanel>
      </div>

      {/* Node Status - collapsible, default OPEN */}
      <div className="mb-5">
        <CollapsiblePanel
          title="节点状态"
          defaultOpen={true}
        >
          {/* Inner card with its own header */}
          <div className="px-5 pb-5">
            <div className="rounded-xl overflow-hidden border" style={{ borderColor: 'var(--border)' }}>
              {/* Inner header */}
              <div className="flex items-center justify-between px-4 py-3" style={{ borderBottom: '1px solid var(--border)' }}>
                <span className="text-sm font-semibold" style={{ color: 'var(--foreground)' }}>节点状态</span>
                <span className="text-xs px-2 py-0.5 rounded-full" style={{ background: 'rgba(34,197,94,0.1)', color: '#22c55e' }}>
                  {onlineCount}/{nodes.length} 在线
                </span>
              </div>
              {/* Node list */}
              <div className="max-h-[500px] overflow-y-auto">
                {nodesQuery.isLoading ? (
                  <div className="p-4 text-center text-sm" style={{ color: 'var(--muted-foreground)' }}>加载中...</div>
                ) : nodesQuery.isError ? (
                  <div className="p-4 text-center text-sm" style={{ color: 'var(--destructive)' }}>加载失败</div>
                ) : nodes.length === 0 ? (
                  <div className="p-4 text-center text-sm" style={{ color: 'var(--muted-foreground)' }}>暂无可用节点</div>
                ) : (
                  nodes.map(node => (
                    <div
                      key={node.id}
                      className="flex items-center gap-2 px-4 py-3 border-b text-sm transition-colors"
                      style={{ borderColor: 'var(--border)' }}
                      onMouseEnter={e => (e.currentTarget.style.background = 'var(--muted)')}
                      onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
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
                        <span className="text-xs flex items-center gap-1" style={{ color: node.is_online ? 'var(--success)' : 'var(--destructive)' }}>
                          {node.is_online ? '✓ 可用' : '离线'}
                        </span>
                      </div>
                    </div>
                  ))
                )}
              </div>
            </div>
          </div>
        </CollapsiblePanel>
      </div>
    </div>
  );
}
