import { Link } from 'react-router-dom';
import { useTheme } from '../lib/theme';
import { useQuery } from '@tanstack/react-query';
import { api } from '../lib/api';
import { EP, adaptPlan, bytesToGB, getPeriodLabel, type PlanResponse } from '../lib/endpoints';
import {
  Zap,
  Shield,
  Globe,
  Smartphone,
  Gauge,
  Headphones,
  Moon,
  Sun,
  ArrowRight,
  Check,
  Sparkles,
  Quote,
  MessageSquare,
  Users,
  Activity,
} from 'lucide-react';

function formatTraffic(bytes: number): string {
  if (bytes <= 0) return '不限流量';
  const gb = bytesToGB(bytes);
  return gb >= 1024 ? `${(gb / 1024).toFixed(0)} TB` : `${gb.toFixed(0)} GB`;
}

const featureAccents = [
  { bg: 'var(--accent)', fg: 'var(--accent-foreground)' },
  { bg: 'var(--accent-emerald)', fg: 'var(--accent-emerald-foreground)' },
  { bg: 'var(--accent-sky)', fg: 'var(--accent-sky-foreground)' },
  { bg: 'var(--accent-amber)', fg: 'var(--accent-amber-foreground)' },
  { bg: 'var(--accent-pink)', fg: 'var(--accent-pink-foreground)' },
  { bg: 'var(--accent-rose)', fg: 'var(--accent-rose-foreground)' },
];

const features = [
  {
    icon: Zap,
    title: '极速连接',
    desc: '全球优质 BGP 线路，智能路由选优，延迟低至毫秒级',
  },
  {
    icon: Shield,
    title: '安全加密',
    desc: '端到端 TLS / REALITY 加密，全方位保护您的隐私',
  },
  {
    icon: Globe,
    title: '全球节点',
    desc: '覆盖亚太、欧美多个地区，持续扩展节点覆盖',
  },
  {
    icon: Smartphone,
    title: '多端支持',
    desc: '支持 Clash、Shadowrocket、v2rayN、Sing-box 等主流客户端',
  },
  {
    icon: Gauge,
    title: '智能负载',
    desc: '自动负载均衡，高峰期无感切换最优节点',
  },
  {
    icon: Headphones,
    title: '专业支持',
    desc: '工单系统快速响应，专业团队 7×24 小时为您服务',
  },
];

const testimonials = [
  {
    name: '陈先生',
    location: '北京',
    avatar: '北',
    comment: '用了三年了，稳得一批，晚高峰看 4K 都不带缓冲的',
  },
  {
    name: '李同学',
    location: '上海',
    avatar: '沪',
    comment: '节点延迟真的低，打外服游戏延迟稳在 30ms 以内，爱了',
  },
  {
    name: '王工',
    location: '深圳',
    avatar: '粤',
    comment: '做外贸查资料必备，专线稳定，客服响应也快',
  },
  {
    name: '张小姐',
    location: '成都',
    avatar: '川',
    comment: '之前换了好几个，这个是唯一用了超过一年没跑路的',
  },
  {
    name: '刘老师',
    location: '杭州',
    avatar: '浙',
    comment: '看奈飞迪士尼速度飞起，流媒体解锁全得很',
  },
  {
    name: '赵同学',
    location: '武汉',
    avatar: '鄂',
    comment: '宿舍党福音，连教育网都不卡，开黑查资料两不误',
  },
];

const faqs = [
  {
    q: '如何开始使用？',
    a: '注册账号后选择合适的套餐，完成支付即可获得订阅链接，导入支持的客户端即刻使用。',
  },
  {
    q: '支持哪些客户端？',
    a: '支持 Clash Meta、Shadowrocket、v2rayN、Sing-box、Stash、Surge 等所有主流代理客户端。',
  },
  {
    q: '流量如何计算？',
    a: '上传与下载流量合并计算，套餐周期结束后自动重置流量配额。',
  },
  {
    q: '可以退款吗？',
    a: '未使用的套餐支持 7 天无理由退款，请通过工单系统联系客服处理。',
  },
];

function PlanCard({ plan }: { plan: PlanResponse }) {
  const monthPrice = plan.prices?.find(p => p.period_code === 'month' || p.period_code === 'monthly');
  const defaultPrice = monthPrice || plan.prices?.[0];
  const priceCNY = defaultPrice?.price_cny || 0;
  const featureList = plan.features?.slice(0, 4) || [];
  const isPopular = plan.prices && plan.prices.length > 0 && priceCNY >= 30 && priceCNY <= 80;

  return (
    <div
      className={`relative flex flex-col rounded-2xl p-7 transition-all duration-300 ${
        isPopular
          ? 'bg-[var(--card)] shadow-xl shadow-purple-500/10 ring-2 ring-[rgb(217_119_87_/_0.3)] scale-[1.02]'
          : 'bg-[rgb(255_253_250_/_0.6)] backdrop-blur-sm border border-[var(--border)] hover:shadow-lg hover:border-[rgb(217_119_87_/_0.2)]'
      }`}
    >
      {isPopular && (
        <div className="absolute -top-3 left-1/2 -translate-x-1/2">
          <span className="inline-flex items-center gap-1 px-4 py-1 rounded-full text-xs font-medium text-white bg-gradient-to-r from-[var(--primary)] to-[var(--primary-soft)] shadow-md shadow-purple-500/25">
            <Sparkles className="w-3 h-3" />
            最受欢迎
          </span>
        </div>
      )}

      <h3 className="text-lg font-semibold text-[var(--foreground)] mb-1">{plan.name}</h3>
      <p className="text-sm font-medium mb-4" style={{ color: 'var(--secondary-foreground)' }}>
        {formatTraffic(plan.traffic_bytes)} · 每月重置
      </p>

      <div className="mb-6">
        <span className="text-4xl font-bold text-[var(--primary)]">¥{priceCNY.toFixed(0)}</span>
        <span className="text-[var(--secondary-foreground)] text-sm ml-1 font-medium">
          /{defaultPrice ? getPeriodLabel(defaultPrice.period_code) : '月'}
        </span>
      </div>

      <div className="space-y-2.5 mb-8 flex-1">
        {featureList.length > 0 ? (
          featureList.map((feat, i) => {
            const clean = feat.replace(/^[✅✔️✓☑️\s]+/, '').trim();
            return (
              <div key={i} className="flex items-start gap-2.5 text-sm">
                <div className="mt-0.5 w-4 h-4 rounded-full bg-[var(--accent)] flex items-center justify-center flex-shrink-0">
                  <Check className="w-2.5 h-2.5 text-[var(--accent-foreground)]" strokeWidth={3} />
                </div>
                <span className="text-[var(--foreground)]">{clean || feat}</span>
              </div>
            );
          })
        ) : (
          <>
            {plan.speed_limit_mbps && plan.speed_limit_mbps > 0 && (
              <div className="flex items-start gap-2.5 text-sm">
                <div className="mt-0.5 w-4 h-4 rounded-full bg-[var(--accent)] flex items-center justify-center flex-shrink-0">
                  <Check className="w-2.5 h-2.5 text-[var(--accent-foreground)]" strokeWidth={3} />
                </div>
                <span>{plan.speed_limit_mbps}Mbps 网络速率</span>
              </div>
            )}
            <div className="flex items-start gap-2.5 text-sm">
              <div className="mt-0.5 w-4 h-4 rounded-full bg-[var(--accent)] flex items-center justify-center flex-shrink-0">
                <Check className="w-2.5 h-2.5 text-[var(--accent-foreground)]" strokeWidth={3} />
              </div>
              <span>全线路流媒体解锁</span>
            </div>
            <div className="flex items-start gap-2.5 text-sm">
              <div className="mt-0.5 w-4 h-4 rounded-full bg-[var(--accent)] flex items-center justify-center flex-shrink-0">
                <Check className="w-2.5 h-2.5 text-[var(--accent-foreground)]" strokeWidth={3} />
              </div>
              <span>{plan.device_limit && plan.device_limit > 0 ? `最多 ${plan.device_limit} 台设备` : '不限制设备数量'}</span>
            </div>
          </>
        )}
      </div>

      <Link
        to="/register"
        className={`block w-full py-3 rounded-xl text-center text-sm font-medium transition-all duration-200 ${
          isPopular
            ? 'bg-gradient-to-r from-[var(--primary)] to-[var(--primary-soft)] text-white shadow-lg shadow-purple-500/25 hover:shadow-xl hover:shadow-purple-500/30 hover:-translate-y-0.5'
            : 'bg-[var(--accent)] text-[var(--accent-foreground)] hover:bg-[var(--primary)] hover:text-white'
        }`}
      >
        立即选购
        <ArrowRight className="inline w-4 h-4 ml-1" />
      </Link>
    </div>
  );
}

export function Landing() {
  const { theme, toggleTheme } = useTheme();

  const plansQuery = useQuery<PlanResponse[]>({
    queryKey: ['landing-plans'],
    queryFn: async () => {
      try {
        const data = await api.get<PlanResponse[]>(EP.PLANS_GUEST);
        return (data || []).filter(p => p.status === 'active').map(adaptPlan).slice(0, 3);
      } catch {
        return [];
      }
    },
    staleTime: 5 * 60 * 1000,
  });

  const plans = plansQuery.data || [];

  const fallbackPlans: PlanResponse[] = [
    {
      id: '1',
      code: 'light',
      name: '轻量套餐',
      description: '',
      content: '',
      status: 'active',
      billing_type: 'recurring',
      traffic_bytes: 66 * 1024 * 1024 * 1024,
      speed_limit_mbps: 1000,
      device_limit: 0,
      reset_cycle: 'monthly',
      features: ['1Gbps 网络速率', '全线路流媒体解锁', 'BGP 三网优化', '不限制设备数量'],
      feature_flags: {},
      prices: [{ period_code: 'month', price_usdt: 1, price_cny: 6 }],
      node_count: 12,
      created_at: new Date().toISOString(),
    },
    {
      id: '2',
      code: 'pro',
      name: '增强套餐',
      description: '',
      content: '',
      status: 'active',
      billing_type: 'recurring',
      traffic_bytes: 156 * 1024 * 1024 * 1024,
      speed_limit_mbps: 1200,
      device_limit: 0,
      reset_cycle: 'monthly',
      features: ['1.2Gbps 网络速率', '全线路流媒体解锁+原生IP', 'IEPL BGP 专线', '更多地区节点'],
      feature_flags: {},
      prices: [{ period_code: 'month', price_usdt: 6, price_cny: 43 }],
      node_count: 25,
      created_at: new Date().toISOString(),
    },
    {
      id: '3',
      code: 'luxury',
      name: '轻奢套餐',
      description: '',
      content: '',
      status: 'active',
      billing_type: 'recurring',
      traffic_bytes: 0,
      speed_limit_mbps: 2000,
      device_limit: 0,
      reset_cycle: 'monthly',
      features: ['2Gbps 网络速率', '解锁全部地区', 'BGP 三网优化+专线', '包含轻量+增强节点'],
      feature_flags: {},
      prices: [{ period_code: 'month', price_usdt: 25, price_cny: 180 }],
      node_count: 35,
      created_at: new Date().toISOString(),
    },
  ];

  const displayPlans = plans.length > 0 ? plans : fallbackPlans;

  return (
    <div className="min-h-screen bg-[var(--background)]">
      <header
        className="sticky top-0 z-50 backdrop-blur-xl border-b transition-colors"
        style={{
          background: 'var(--header-bg)',
          borderColor: 'var(--border)',
        }}
      >
        <div className="max-w-6xl mx-auto px-6 h-16 flex items-center justify-between">
          <Link to="/" className="flex items-center gap-2.5">
            <div
              className="w-8 h-8 rounded-xl flex items-center justify-center text-white font-bold text-sm shadow-lg shadow-purple-500/20"
              style={{
                background: 'linear-gradient(135deg, var(--primary) 0%, var(--primary-soft) 100%)',
              }}
            >
              Y
            </div>
            <span className="font-bold text-lg tracking-tight" style={{ color: 'var(--foreground)' }}>
              YunDu
            </span>
          </Link>
          <nav className="hidden md:flex items-center gap-8 text-sm">
            <a href="#features" className="text-[var(--muted-foreground)] hover:text-[var(--primary)] transition-colors">
              功能特性
            </a>
            <a href="#pricing" className="text-[var(--muted-foreground)] hover:text-[var(--primary)] transition-colors">
              套餐价格
            </a>
            <a href="#testimonials" className="text-[var(--muted-foreground)] hover:text-[var(--primary)] transition-colors">
              用户评价
            </a>
            <a href="#faq" className="text-[var(--muted-foreground)] hover:text-[var(--primary)] transition-colors">
              常见问题
            </a>
          </nav>
          <div className="flex items-center gap-2">
            <button
              onClick={toggleTheme}
              className="w-9 h-9 rounded-xl flex items-center justify-center hover:bg-[var(--muted)] transition-colors"
              title="切换主题"
            >
              {theme === 'light' ? (
                <Moon className="w-4 h-4 text-[var(--muted-foreground)]" />
              ) : (
                <Sun className="w-4 h-4 text-[var(--muted-foreground)]" />
              )}
            </button>
            <Link
              to="/login"
              className="px-4 py-2 text-sm font-medium rounded-xl transition-colors hover:bg-[var(--muted)] text-[var(--foreground)]"
            >
              登录
            </Link>
            <Link
              to="/register"
              className="px-4 py-2 text-sm font-medium rounded-xl text-white transition-all shadow-md hover:shadow-lg hover:-translate-y-0.5"
              style={{
                background: 'linear-gradient(135deg, var(--primary) 0%, var(--primary-soft) 100%)',
              }}
            >
              免费注册
            </Link>
          </div>
        </div>
      </header>

      <section className="relative pt-24 pb-32 px-6 overflow-hidden">
        <div className="absolute inset-0 overflow-hidden pointer-events-none">
          <div
            className="absolute -top-40 -right-40 w-[600px] h-[600px] rounded-full blur-3xl opacity-30"
            style={{ background: 'radial-gradient(circle, var(--primary) 0%, transparent 70%)' }}
          />
          <div
            className="absolute top-20 -left-20 w-[400px] h-[400px] rounded-full blur-3xl opacity-20"
            style={{ background: 'radial-gradient(circle, var(--primary-soft) 0%, transparent 70%)' }}
          />
          <div
            className="absolute bottom-0 left-1/2 -translate-x-1/2 w-[800px] h-[400px] rounded-t-full blur-3xl opacity-10"
            style={{ background: 'linear-gradient(to top, var(--primary), transparent)' }}
          />
        </div>

        <div className="relative max-w-4xl mx-auto text-center">
          <div
            className="inline-flex items-center gap-2 px-4 py-2 rounded-full text-xs font-medium mb-8"
            style={{ background: 'var(--accent)', color: 'var(--accent-foreground)' }}
          >
            <Sparkles className="w-3.5 h-3.5" />
            云原生高性能网络加速平台
          </div>

          <h1
            className="text-5xl md:text-6xl font-bold mb-6 leading-[1.1] tracking-tight"
            style={{ color: 'var(--foreground)' }}
          >
            极速、安全、
            <br />
            <span
              className="bg-clip-text text-transparent"
              style={{
                backgroundImage: 'linear-gradient(135deg, var(--primary) 0%, var(--primary-soft) 100%)',
              }}
            >
              稳定连接世界
            </span>
          </h1>

          <p
            className="text-lg md:text-xl mb-10 max-w-2xl mx-auto leading-relaxed"
            style={{ color: 'var(--muted-foreground)' }}
          >
            覆盖全球优质节点，智能路由自动选优
            <br className="hidden md:block" />
            为您提供流畅、安全的网络体验
          </p>

          <div className="flex items-center justify-center gap-4 flex-wrap">
            <Link
              to="/register"
              className="group inline-flex items-center gap-2 px-7 py-3.5 rounded-xl text-white font-medium text-sm transition-all shadow-lg shadow-purple-500/25 hover:shadow-xl hover:shadow-purple-500/30 hover:-translate-y-0.5"
              style={{
                background: 'linear-gradient(135deg, var(--primary) 0%, var(--primary-soft) 100%)',
              }}
            >
              立即开始
              <ArrowRight className="w-4 h-4 transition-transform group-hover:translate-x-0.5" />
            </Link>
            <a
              href="#pricing"
              className="inline-flex items-center gap-2 px-7 py-3.5 rounded-xl font-medium text-sm border transition-all hover:bg-[var(--muted)]"
              style={{
                borderColor: 'var(--border)',
                color: 'var(--foreground)',
                background: 'var(--card)',
              }}
            >
              查看套餐
            </a>
          </div>

          <div className="mt-20 flex items-center justify-center gap-10 md:gap-16 flex-wrap">
            {[
              { icon: Users, value: '50,000+', label: '活跃用户' },
              { icon: Globe, value: '50+', label: '全球节点' },
              { icon: Activity, value: '99.9%', label: '在线率' },
            ].map((stat, i) => (
              <div key={i} className="text-center">
                <div className="flex items-center justify-center gap-2 mb-1">
                  <stat.icon className="w-4 h-4 text-[var(--primary)]" />
                  <span className="text-2xl font-bold" style={{ color: 'var(--foreground)' }}>
                    {stat.value}
                  </span>
                </div>
                <span className="text-sm" style={{ color: 'var(--muted-foreground)' }}>{stat.label}</span>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section id="features" className="py-24 px-6">
        <div className="max-w-6xl mx-auto">
          <div className="text-center mb-16">
            <h2 className="text-3xl md:text-4xl font-bold mb-4 tracking-tight" style={{ color: 'var(--foreground)' }}>
              为什么选择 YunDu
            </h2>
            <p className="text-base max-w-lg mx-auto" style={{ color: 'var(--secondary-foreground)' }}>
              我们致力于提供最优质的网络加速服务，让每一次连接都稳定可靠
            </p>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
            {features.map((f, i) => (
              <div
                key={i}
                className="group p-7 rounded-2xl bg-[rgb(255_253_250_/_0.5)] border border-[var(--border)] backdrop-blur-sm transition-all duration-300 hover:bg-[var(--card)] hover:shadow-lg hover:shadow-purple-500/5 hover:-translate-y-1"
              >
                <div
                  className="w-11 h-11 rounded-xl flex items-center justify-center mb-5 transition-transform group-hover:scale-105"
                  style={{ background: featureAccents[i % featureAccents.length].bg }}
                >
                  <f.icon className="w-5 h-5" style={{ color: featureAccents[i % featureAccents.length].fg }} strokeWidth={1.8} />
                </div>
                <h3 className="font-semibold text-base mb-2" style={{ color: 'var(--foreground)' }}>{f.title}</h3>
                <p className="text-sm leading-relaxed" style={{ color: 'var(--secondary-foreground)' }}>{f.desc}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      <div className="max-w-4xl mx-auto px-6">
        <div className="h-px bg-gradient-to-r from-transparent via-[var(--border)] to-transparent" />
      </div>

      <section id="pricing" className="py-24 px-6">
        <div className="max-w-6xl mx-auto">
          <div className="text-center mb-16">
            <h2 className="text-3xl md:text-4xl font-bold mb-4 tracking-tight" style={{ color: 'var(--foreground)' }}>
              灵活的套餐方案
            </h2>
            <p className="text-base max-w-lg mx-auto" style={{ color: 'var(--secondary-foreground)' }}>
              从体验到专业，总有一款适合您
            </p>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-3 gap-6 max-w-5xl mx-auto items-start">
            {displayPlans.map((plan) => (
              <PlanCard key={plan.id} plan={plan} />
            ))}
          </div>

          <div className="text-center mt-10">
            <Link
              to="/register"
              className="inline-flex items-center gap-1.5 text-sm font-medium transition-colors"
              style={{ color: 'var(--primary)' }}
            >
              查看全部套餐
              <ArrowRight className="w-4 h-4" />
            </Link>
          </div>
        </div>
      </section>

      <div className="max-w-4xl mx-auto px-6">
        <div className="h-px bg-gradient-to-r from-transparent via-[var(--border)] to-transparent" />
      </div>

      <section id="testimonials" className="py-24 px-6">
        <div className="max-w-6xl mx-auto">
          <div className="text-center mb-16">
            <div
              className="inline-flex items-center gap-2 px-4 py-2 rounded-full text-xs font-medium mb-5"
              style={{ background: 'var(--accent)', color: 'var(--accent-foreground)' }}
            >
              <MessageSquare className="w-3.5 h-3.5" />
              真实用户评价
            </div>
            <h2 className="text-3xl md:text-4xl font-bold mb-4 tracking-tight" style={{ color: 'var(--foreground)' }}>
              来自全国各地的声音
            </h2>
            <p className="text-base max-w-lg mx-auto" style={{ color: 'var(--secondary-foreground)' }}>
              听听用户们怎么说
            </p>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
            {testimonials.map((t, i) => (
              <div
                key={i}
                className="group p-7 rounded-2xl bg-[rgb(255_253_250_/_0.5)] border border-[var(--border)] backdrop-blur-sm transition-all duration-300 hover:bg-[var(--card)] hover:shadow-lg hover:-translate-y-1"
              >
                <Quote
                  className="w-8 h-8 mb-4 opacity-20"
                  style={{ color: 'var(--primary)' }}
                />
                <p className="text-sm leading-relaxed mb-5" style={{ color: 'var(--foreground)' }}>
                  "{t.comment}"
                </p>
                <div className="flex items-center gap-3">
                  <div
                    className="w-9 h-9 rounded-full flex items-center justify-center text-white text-xs font-bold"
                    style={{
                      background: 'linear-gradient(135deg, var(--primary) 0%, var(--primary-soft) 100%)',
                    }}
                  >
                    {t.avatar}
                  </div>
                  <div>
                    <div className="text-sm font-medium" style={{ color: 'var(--foreground)' }}>{t.name}</div>
                    <div className="text-xs" style={{ color: 'var(--muted-foreground)' }}>{t.location}</div>
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>
      </section>

      <div className="max-w-4xl mx-auto px-6">
        <div className="h-px bg-gradient-to-r from-transparent via-[var(--border)] to-transparent" />
      </div>

      <section id="faq" className="py-24 px-6">
        <div className="max-w-3xl mx-auto">
          <div className="text-center mb-16">
            <h2 className="text-3xl md:text-4xl font-bold mb-4 tracking-tight" style={{ color: 'var(--foreground)' }}>
              常见问题
            </h2>
            <p className="text-base" style={{ color: 'var(--muted-foreground)' }}>
              还有疑问？随时联系我们的客服团队
            </p>
          </div>

          <div className="space-y-3">
            {faqs.map((item, i) => (
              <div
                key={i}
                className="p-6 rounded-2xl bg-[rgb(255_253_250_/_0.5)] border border-[var(--border)] backdrop-blur-sm transition-all hover:bg-[var(--card)] hover:border-[rgb(217_119_87_/_0.2)]"
              >
                <h3 className="font-medium text-base mb-2" style={{ color: 'var(--foreground)' }}>{item.q}</h3>
                <p className="text-sm leading-relaxed" style={{ color: 'var(--secondary-foreground)' }}>{item.a}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className="py-24 px-6">
        <div className="max-w-4xl mx-auto">
          <div
            className="relative rounded-3xl p-12 md:p-16 text-center overflow-hidden"
            style={{
              background: 'linear-gradient(135deg, var(--primary) 0%, var(--primary-soft) 100%)',
            }}
          >
            <div className="absolute inset-0 overflow-hidden pointer-events-none">
              <div className="absolute -top-20 -right-20 w-64 h-64 rounded-full bg-[rgb(255_253_250_/_0.1)] blur-2xl" />
              <div className="absolute -bottom-20 -left-20 w-64 h-64 rounded-full bg-[rgb(255_253_250_/_0.1)] blur-2xl" />
            </div>
            <div className="relative">
              <h2 className="text-3xl md:text-4xl font-bold text-white mb-4 tracking-tight">
                准备好开始了吗？
              </h2>
              <p className="text-white/80 text-base mb-8 max-w-md mx-auto">
                注册即送体验流量，立即感受极速网络
              </p>
              <Link
                to="/register"
                className="inline-flex items-center gap-2 px-8 py-4 rounded-xl bg-[var(--card)] font-medium text-sm transition-all hover:shadow-xl hover:-translate-y-0.5"
                style={{ color: 'var(--primary)' }}
              >
                免费注册
                <ArrowRight className="w-4 h-4" />
              </Link>
            </div>
          </div>
        </div>
      </section>

      <footer className="py-12 px-6 border-t" style={{ borderColor: 'var(--border)' }}>
        <div className="max-w-6xl mx-auto flex flex-col md:flex-row items-center justify-between gap-6 text-sm">
          <div className="flex items-center gap-2.5">
            <div
              className="w-7 h-7 rounded-lg flex items-center justify-center text-white font-bold text-xs"
              style={{
                background: 'linear-gradient(135deg, var(--primary) 0%, var(--primary-soft) 100%)',
              }}
            >
              Y
            </div>
            <span className="font-medium" style={{ color: 'var(--foreground)' }}>YunDu</span>
            <span style={{ color: 'var(--muted-foreground)' }}>© 2026 All rights reserved.</span>
          </div>
          <div className="flex items-center gap-6" style={{ color: 'var(--muted-foreground)' }}>
            <a href="#" className="hover:text-[var(--primary)] transition-colors">服务条款</a>
            <a href="#" className="hover:text-[var(--primary)] transition-colors">隐私政策</a>
            <Link to="/login" className="hover:text-[var(--primary)] transition-colors">用户登录</Link>
          </div>
        </div>
      </footer>
    </div>
  );
}
