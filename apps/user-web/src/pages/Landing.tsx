import { Link } from 'react-router-dom';
import { useTheme } from '../lib/theme';

export function Landing() {
  const { theme, toggleTheme } = useTheme();

  return (
    <div className="min-h-screen" style={{ background: '#f5f7fb' }}>
      {/* Header - xboard style white header */}
      <header className="sticky top-0 z-40 border-b" style={{ background: '#fff', borderColor: '#e2e8f0' }}>
        <div className="max-w-7xl mx-auto px-6 h-16 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <div className="w-8 h-8 rounded-lg flex items-center justify-center text-white font-bold text-sm" style={{ background: '#7c5cfc' }}>Y</div>
            <span className="font-bold text-xl" style={{ color: '#1e293b' }}>YunDu</span>
          </div>
          <nav className="hidden md:flex items-center gap-6 text-sm" style={{ color: '#64748b' }}>
            <a href="#features" className="hover:text-purple-600 transition-colors">功能特性</a>
            <a href="#pricing" className="hover:text-purple-600 transition-colors">套餐价格</a>
            <a href="#faq" className="hover:text-purple-600 transition-colors">常见问题</a>
          </nav>
          <div className="flex items-center gap-3">
            <button onClick={toggleTheme} className="w-9 h-9 rounded-lg flex items-center justify-center text-sm hover:bg-gray-100 transition-colors" title="切换主题">
              {theme === 'light' ? '🌙' : '☀️'}
            </button>
            <Link to="/login" className="px-4 py-2 text-sm font-medium rounded-lg transition-colors hover:bg-gray-100" style={{ color: '#334155' }}>登录</Link>
            <Link to="/register" className="px-4 py-2 text-sm font-medium rounded-lg text-white transition-colors shadow-sm" style={{ background: '#7c5cfc' }}>
              免费注册
            </Link>
          </div>
        </div>
      </header>

      {/* Hero */}
      <section className="py-20 px-6" style={{ background: 'linear-gradient(180deg, #f0edff 0%, #f5f7fb 100%)' }}>
        <div className="max-w-4xl mx-auto text-center">
          <div className="inline-block px-4 py-1.5 rounded-full text-xs font-medium mb-6" style={{ background: 'rgba(124,92,252,0.1)', color: '#7c5cfc' }}>
            ✨ 云原生高性能代理平台
          </div>
          <h1 className="text-4xl md:text-5xl font-bold mb-5 leading-tight" style={{ color: '#1e293b' }}>
            极速、安全、稳定的<br />全球网络加速服务
          </h1>
          <p className="text-lg mb-8 max-w-2xl mx-auto" style={{ color: '#64748b' }}>
            覆盖全球优质节点，支持多协议接入，智能路由自动选优，为您提供流畅的网络体验
          </p>
          <div className="flex items-center justify-center gap-4 flex-wrap">
            <Link to="/register" className="px-6 py-3 rounded-lg text-white font-medium text-sm shadow-md transition-all hover:shadow-lg" style={{ background: '#7c5cfc' }}>
              立即开始 →
            </Link>
            <Link to="/login" className="px-6 py-3 rounded-lg font-medium text-sm border transition-colors hover:bg-gray-50" style={{ borderColor: '#e2e8f0', color: '#334155', background: '#fff' }}>
              已有账号，登录
            </Link>
          </div>
        </div>
      </section>

      {/* Features */}
      <section id="features" className="py-16 px-6">
        <div className="max-w-6xl mx-auto">
          <h2 className="text-2xl font-bold text-center mb-3" style={{ color: '#1e293b' }}>为什么选择 YunDu</h2>
          <p className="text-center mb-12 text-sm" style={{ color: '#64748b' }}>我们致力于提供最优质的网络加速服务</p>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
            {[
              { icon: '🚀', title: '极速连接', desc: '全球优质BGP线路，智能路由选优，延迟低至毫秒级' },
              { icon: '🔒', title: '安全加密', desc: '端到端TLS/REALITY加密，保护您的隐私安全' },
              { icon: '🌍', title: '全球节点', desc: '覆盖亚太、欧美多个地区，持续扩展节点覆盖' },
              { icon: '📱', title: '多端支持', desc: '支持Clash、Shadowrocket、v2rayN等主流客户端' },
              { icon: '⚡', title: '智能负载', desc: '自动负载均衡，高峰期自动切换最优节点' },
              { icon: '💬', title: '7×24支持', desc: '工单系统快速响应，专业团队随时为您服务' },
            ].map((f, i) => (
              <div key={i} className="bg-white rounded-xl p-6 border transition-shadow hover:shadow-md" style={{ borderColor: '#e2e8f0' }}>
                <div className="text-3xl mb-3">{f.icon}</div>
                <h3 className="font-semibold mb-2" style={{ color: '#1e293b' }}>{f.title}</h3>
                <p className="text-sm" style={{ color: '#64748b' }}>{f.desc}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* Pricing preview */}
      <section id="pricing" className="py-16 px-6" style={{ background: '#fff' }}>
        <div className="max-w-4xl mx-auto text-center">
          <h2 className="text-2xl font-bold mb-3" style={{ color: '#1e293b' }}>灵活的套餐方案</h2>
          <p className="mb-10 text-sm" style={{ color: '#64748b' }}>从免费体验到高端套餐，总有一款适合您</p>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-5">
            {[
              { name: '轻量套餐', traffic: '66GB/月', price: '¥6', features: ['1Gbps网络速率', '全线路流媒体解锁', '入口BGP三网优化', '不限制设备数量'] },
              { name: '增强套餐', traffic: '156GB/月', price: '¥43', popular: true, features: ['1.2Gbps网络速率', '全线路流媒体解锁+原生IP', '全线IEPL BGP专线', '更多地区节点'] },
              { name: '轻奢套餐', traffic: '不限时', price: '¥180', features: ['2Gbps网络速率', '解锁全部地区', 'BGP三网优化+专线', '包含轻量+增强节点'] },
            ].map((p, i) => (
              <div key={i} className={`rounded-xl p-6 border-2 text-left relative ${p.popular ? 'shadow-lg' : ''}`} style={{ borderColor: p.popular ? '#7c5cfc' : '#e2e8f0', background: p.popular ? 'rgba(124,92,252,0.03)' : '#fff' }}>
                {p.popular && (
                  <span className="absolute -top-3 left-6 px-3 py-0.5 rounded-full text-xs font-medium text-white" style={{ background: '#7c5cfc' }}>最受欢迎</span>
                )}
                <h3 className="font-semibold text-base mb-1" style={{ color: '#1e293b' }}>{p.name}</h3>
                <div className="text-2xl font-bold mb-1" style={{ color: '#7c5cfc' }}>{p.price}<span className="text-sm font-normal" style={{ color: '#64748b' }}>/月起</span></div>
                <div className="text-sm mb-4" style={{ color: '#ef4444' }}>{p.traffic}</div>
                <div className="space-y-1.5 mb-5">
                  {p.features.map((f, j) => (
                    <div key={j} className="flex items-start gap-2 text-sm">
                      <span className="text-green-500 mt-0.5">✅</span>
                      <span style={{ color: '#334155' }}>{f}</span>
                    </div>
                  ))}
                </div>
                <Link to="/register" className="block w-full py-2 rounded-lg text-center text-sm font-medium transition-colors text-white" style={{ background: p.popular ? '#7c5cfc' : '#7c5cfc' }}>
                  立即购买
                </Link>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* FAQ */}
      <section id="faq" className="py-16 px-6">
        <div className="max-w-3xl mx-auto">
          <h2 className="text-2xl font-bold text-center mb-10" style={{ color: '#1e293b' }}>常见问题</h2>
          <div className="space-y-3">
            {[
              { q: '如何开始使用？', a: '注册账号后选择合适的套餐，完成支付后即可获得订阅链接，导入到支持的客户端即可使用。' },
              { q: '支持哪些客户端？', a: '支持Clash Meta、Shadowrocket、v2rayN、Sing-box、Stash、Surge等主流代理客户端。' },
              { q: '流量如何计算？', a: '上传和下载流量合并计算，套餐周期结束后自动重置流量配额。' },
              { q: '可以退款吗？', a: '未使用的套餐支持7天无理由退款，请通过工单系统联系客服处理。' },
            ].map((item, i) => (
              <div key={i} className="bg-white rounded-xl p-5 border" style={{ borderColor: '#e2e8f0' }}>
                <h3 className="font-medium mb-1" style={{ color: '#1e293b' }}>{item.q}</h3>
                <p className="text-sm" style={{ color: '#64748b' }}>{item.a}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* Footer */}
      <footer className="py-10 px-6 border-t" style={{ background: '#fff', borderColor: '#e2e8f0' }}>
        <div className="max-w-6xl mx-auto flex flex-col md:flex-row items-center justify-between gap-4 text-sm" style={{ color: '#94a3b8' }}>
          <div className="flex items-center gap-2">
            <div className="w-6 h-6 rounded flex items-center justify-center text-white font-bold text-xs" style={{ background: '#7c5cfc' }}>Y</div>
            <span style={{ color: '#334155' }}>YunDu</span>
            <span>© 2026 All rights reserved.</span>
          </div>
          <div className="flex items-center gap-4">
            <a href="#" className="hover:text-purple-600 transition-colors">服务条款</a>
            <a href="#" className="hover:text-purple-600 transition-colors">隐私政策</a>
            <Link to="/login" className="hover:text-purple-600 transition-colors">用户登录</Link>
          </div>
        </div>
      </footer>
    </div>
  );
}
