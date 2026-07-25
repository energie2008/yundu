import { useLocation } from 'react-router-dom'
import { LogOut, Menu, Bell, Search, Sun, Moon } from 'lucide-react'
import { cn, Avatar, AvatarFallback, Button } from '@airport/ui'
import { useAuthStore } from '@/lib/auth'
import { getPageTitle } from '@/lib/nav'
import { useTheme } from '@/lib/theme-provider'

interface HeaderProps {
  onMenuClick?: () => void
}

export function Header({ onMenuClick }: HeaderProps) {
  const location = useLocation()
  const { admin, logout } = useAuthStore()
  const { theme, toggleTheme } = useTheme()
  const title = getPageTitle(location.pathname)

  const getInitials = (name: string) => {
    return name ? name.charAt(0).toUpperCase() : 'A'
  }

  return (
    <header
      className="sticky top-0 z-30 backdrop-blur-xl safe-top transition-colors duration-300"
      style={{ backgroundColor: 'var(--header-bg)', borderBottom: '1px solid var(--header-border)' }}
    >
      <div className="flex items-center justify-between h-16 px-4 tablet:px-6">
        <div className="flex items-center gap-3">
          <button
            onClick={onMenuClick}
            className="tablet:hidden p-2 -ml-2 rounded-xl transition-all hover:bg-opacity-50"
            style={{ color: 'var(--muted-foreground)' }}
          >
            <Menu className="w-5 h-5" />
          </button>
          <div>
            <h1 className="text-lg font-bold" style={{ color: 'var(--foreground)' }}>{title}</h1>
            <p className="text-[11px] hidden sm:block" style={{ color: 'var(--muted-foreground)' }}>
              云渡 YunDu 管理控制台
            </p>
          </div>
        </div>

        <div className="flex items-center gap-3">
          {/* Search */}
          <div className="hidden md:flex items-center gap-2 px-3 py-1.5 rounded-xl w-64 transition-colors" style={{ backgroundColor: 'var(--muted)', border: '1px solid var(--border)' }}>
            <Search className="w-4 h-4" style={{ color: 'var(--muted-foreground)' }} />
            <input
              type="text"
              placeholder="搜索..."
              className="bg-transparent border-0 outline-none text-sm flex-1"
              style={{ color: 'var(--foreground)' }}
            />
          </div>

          {/* Theme Toggle */}
          <button
            onClick={toggleTheme}
            className="p-2 rounded-xl relative transition-all hover:scale-105"
            style={{
              color: theme === 'light' ? 'var(--warning)' : 'var(--primary)',
              backgroundColor: theme === 'light' ? 'var(--accent-amber)' : 'var(--accent)',
            }}
            title={theme === 'light' ? '切换黑夜模式' : '切换白天模式'}
          >
            {theme === 'light' ? <Moon className="w-5 h-5" /> : <Sun className="w-5 h-5" />}
          </button>

          {/* Notifications */}
          <button
            className="p-2 rounded-xl relative transition-all"
            style={{ color: 'var(--muted-foreground)' }}
            onMouseEnter={e => { e.currentTarget.style.backgroundColor = 'var(--muted)' }}
            onMouseLeave={e => { e.currentTarget.style.backgroundColor = 'transparent' }}
          >
            <Bell className="w-5 h-5" />
            <span className="absolute top-1.5 right-1.5 w-2 h-2 rounded-full" style={{ backgroundColor: 'var(--destructive)', boxShadow: '0 0 0 2px var(--header-bg)' }} />
          </button>

          {/* User Info */}
          <div className="hidden tablet:flex items-center gap-3 pl-3" style={{ borderLeft: '1px solid var(--border)' }}>
            <div className="text-right">
              <p className="text-sm font-medium" style={{ color: 'var(--foreground)' }}>
                {admin?.name || 'Admin'}
              </p>
              <p className="text-xs" style={{ color: 'var(--muted-foreground)' }}>{admin?.email || 'a****@***********'}</p>
            </div>
            <Avatar className="h-9 w-9" style={{ border: '2px solid var(--primary-glow)' }}>
              <AvatarFallback className="text-sm font-semibold text-white" style={{ background: 'var(--gradient-primary)' }}>
                {getInitials(admin?.name || 'A')}
              </AvatarFallback>
            </Avatar>
          </div>

          {/* Logout */}
          <Button
            variant="ghost"
            size="icon"
            onClick={() => logout()}
            className="hover:bg-opacity-50"
            style={{ color: 'var(--muted-foreground)' }}
            title="退出登录"
          >
            <LogOut className="w-4 h-4" />
          </Button>
        </div>
      </div>
    </header>
  )
}
