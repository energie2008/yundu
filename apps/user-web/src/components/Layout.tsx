import { NavLink, Outlet, useNavigate } from 'react-router-dom';
import { useState } from 'react';
import { useAuth } from '../lib/auth';
import { useTheme } from '../lib/theme';
import { LanguageSelector } from './LanguageSelector';

const navItems = [
  { to: '/dashboard', icon: '🏠', label: '仪表盘' },
  { to: '/dashboard/announcements', icon: '📢', label: '系统公告' },
  { to: '/dashboard/plans', icon: '💳', label: '购买订阅' },
  { to: '/dashboard/orders', icon: '📋', label: '我的订单' },
  { to: '/dashboard/tickets', icon: '💬', label: '工单支持' },
  { to: '/dashboard/knowledge', icon: '📖', label: '文档中心' },
  { to: '/dashboard/invite', icon: '🎁', label: '邀请好友' },
  { to: '/dashboard/profile', icon: '👤', label: '个人资料' },
];

export function Layout() {
  const { user, logout } = useAuth();
  const { theme, toggleTheme } = useTheme();
  const navigate = useNavigate();
  const [sidebarOpen, setSidebarOpen] = useState(false);

  const handleLogout = () => {
    logout();
    navigate('/login');
  };

  const userInitial = user?.email?.charAt(0).toUpperCase() || 'U';

  const closeSidebar = () => setSidebarOpen(false);

  return (
    <div className="flex h-screen" style={{ background: 'var(--background)' }}>
      {/* Sidebar - desktop visible, mobile hidden by default */}
      <aside
        className={
          'fixed md:static inset-y-0 left-0 z-50 w-64 md:w-52 flex-shrink-0 flex flex-col border-r transition-transform duration-200 ' +
          (sidebarOpen ? 'translate-x-0' : '-translate-x-full md:translate-x-0')
        }
        style={{ background: 'var(--sidebar-bg)', borderColor: 'var(--sidebar-border)' }}
      >
        {/* Logo */}
        <div className="h-14 flex items-center px-4 border-b" style={{ borderColor: 'var(--sidebar-border)' }}>
          <div className="w-7 h-7 rounded-lg flex items-center justify-center text-white font-bold text-sm mr-2" style={{ background: 'var(--primary)' }}>
            Y
          </div>
          <span className="font-bold text-lg" style={{ color: 'var(--foreground)' }}>YunDu</span>
          <button
            onClick={closeSidebar}
            className="md:hidden ml-auto w-8 h-8 rounded-lg flex items-center justify-center text-lg transition-colors"
            style={{ color: 'var(--muted-foreground)' }}
            onMouseEnter={e => (e.currentTarget.style.background = 'var(--muted)')}
            onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
            aria-label="关闭菜单"
          >
            ×
          </button>
        </div>

        {/* Navigation */}
        <nav className="flex-1 py-3 px-2 space-y-0.5 overflow-y-auto">
          {navItems.map(item => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === '/dashboard'}
              onClick={closeSidebar}
              className={({ isActive }) =>
                `flex items-center gap-2.5 px-3 py-2 rounded-lg text-sm transition-all duration-150 ` +
                (isActive
                  ? 'text-white font-medium shadow-sm'
                  : 'hover:bg-opacity-50')
              }
              style={({ isActive }) => ({
                background: isActive ? 'var(--primary)' : 'transparent',
                color: isActive ? 'white' : 'var(--muted-foreground)',
              })}
            >
              <span className="text-base">{item.icon}</span>
              <span>{item.label}</span>
            </NavLink>
          ))}
        </nav>

        {/* User section */}
        <div className="p-3 border-t" style={{ borderColor: 'var(--sidebar-border)' }}>
          <div className="flex items-center gap-2 mb-2 px-1">
            <div className="w-8 h-8 rounded-full flex items-center justify-center text-white text-sm font-medium" style={{ background: 'var(--primary)' }}>
              {userInitial}
            </div>
            <div className="flex-1 min-w-0">
              <div className="text-xs font-medium truncate" style={{ color: 'var(--foreground)' }}>
                {user?.email}
              </div>
              <div className="text-xs" style={{ color: 'var(--muted-foreground)' }}>
                {user?.active_plan_name || 'Free'}
              </div>
            </div>
          </div>
          <button
            onClick={handleLogout}
            className="w-full text-left px-3 py-1.5 rounded-lg text-xs transition-colors flex items-center gap-2"
            style={{ color: 'var(--muted-foreground)' }}
            onMouseEnter={e => (e.currentTarget.style.background = 'var(--muted)')}
            onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
          >
            <span>🚪</span> 退出登录
          </button>
        </div>
      </aside>

      {/* Mobile overlay */}
      {sidebarOpen && (
        <div
          className="fixed inset-0 z-40 bg-black/40 md:hidden"
          onClick={closeSidebar}
          aria-hidden="true"
        />
      )}

      {/* Main content area */}
      <div className="flex-1 flex flex-col min-w-0 overflow-hidden">
        {/* Top bar */}
        <header className="h-14 flex items-center justify-between px-4 md:px-6 border-b flex-shrink-0" style={{ background: 'var(--header-bg)', borderColor: 'var(--border)' }}>
          <div className="flex items-center gap-2">
            {/* Hamburger button - mobile only */}
            <button
              onClick={() => setSidebarOpen(true)}
              className="md:hidden w-9 h-9 rounded-lg flex items-center justify-center text-lg transition-colors"
              style={{ color: 'var(--muted-foreground)' }}
              onMouseEnter={e => (e.currentTarget.style.background = 'var(--muted)')}
              onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
              aria-label="打开菜单"
            >
              ☰
            </button>
            <div className="text-sm hidden sm:block" style={{ color: 'var(--muted-foreground)' }}>
              {/* Breadcrumb or title handled by pages */}
            </div>
          </div>
          <div className="flex items-center gap-2 md:gap-3">
            {/* Language selector */}
            <LanguageSelector />
            {/* Theme toggle */}
            <button
              onClick={toggleTheme}
              className="w-8 h-8 rounded-lg flex items-center justify-center transition-colors text-sm"
              style={{ color: 'var(--muted-foreground)' }}
              onMouseEnter={e => (e.currentTarget.style.background = 'var(--muted)')}
              onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
              title={theme === 'light' ? '切换深色模式' : '切换浅色模式'}
            >
              {theme === 'light' ? '🌙' : '☀️'}
            </button>
            {/* Notifications */}
            <button
              onClick={() => navigate('/dashboard/notifications')}
              className="w-8 h-8 rounded-lg flex items-center justify-center transition-colors text-sm"
              style={{ color: 'var(--muted-foreground)' }}
              onMouseEnter={e => (e.currentTarget.style.background = 'var(--muted)')}
              onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
            >
              🔔
            </button>
          </div>
        </header>

        {/* Page content */}
        <main className="flex-1 overflow-y-auto">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
