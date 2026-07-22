import { useLocation } from 'react-router-dom'
import { LogOut, Menu, Bell, Search } from 'lucide-react'
import { cn, Avatar, AvatarFallback, Button, Input } from '@airport/ui'
import { useAuthStore } from '@/lib/auth'
import { getPageTitle } from '@/lib/nav'
import { ADMIN_INPUT_BG, ADMIN_INPUT_BORDER, ADMIN_TEXT, ADMIN_TEXT_MUTED, ADMIN_BORDER } from '@/lib/theme'

interface HeaderProps {
  onMenuClick?: () => void
}

export function Header({ onMenuClick }: HeaderProps) {
  const location = useLocation()
  const { admin, logout } = useAuthStore()
  const title = getPageTitle(location.pathname)

  const getInitials = (name: string) => {
    return name ? name.charAt(0).toUpperCase() : 'A'
  }

  return (
    <header
      className="sticky top-0 z-30 backdrop-blur-xl safe-top"
      style={{ backgroundColor: 'rgba(10,14,26,0.8)', borderBottom: `1px solid ${ADMIN_BORDER}` }}
    >
      <div className="flex items-center justify-between h-16 px-4 tablet:px-6">
        <div className="flex items-center gap-3">
          <button
            onClick={onMenuClick}
            className="tablet:hidden p-2 -ml-2 rounded-xl transition-all"
            style={{ color: ADMIN_TEXT_MUTED }}
          >
            <Menu className="w-5 h-5" />
          </button>
          <div>
            <h1 className="text-lg font-bold" style={{ color: ADMIN_TEXT }}>{title}</h1>
            <p className="text-[11px] hidden sm:block" style={{ color: ADMIN_TEXT_MUTED }}>
              云渡 YunDu 管理控制台
            </p>
          </div>
        </div>

        <div className="flex items-center gap-3">
          <div className="hidden md:flex items-center gap-2 px-3 py-1.5 rounded-xl w-64" style={{ backgroundColor: ADMIN_INPUT_BG, border: `1px solid ${ADMIN_INPUT_BORDER}` }}>
            <Search className="w-4 h-4" style={{ color: ADMIN_TEXT_MUTED }} />
            <input
              type="text"
              placeholder="搜索..."
              className="bg-transparent border-0 outline-none text-sm flex-1 placeholder:text-slate-600"
              style={{ color: ADMIN_TEXT }}
            />
          </div>
          <button
            className="p-2 rounded-xl relative transition-all hover:bg-white/5"
            style={{ color: ADMIN_TEXT_MUTED }}
          >
            <Bell className="w-5 h-5" />
            <span className="absolute top-1.5 right-1.5 w-2 h-2 rounded-full bg-indigo-500 ring-2 ring-[#0a0e1a]" />
          </button>
          <div className="hidden tablet:flex items-center gap-3 pl-3" style={{ borderLeft: `1px solid ${ADMIN_BORDER}` }}>
            <div className="text-right">
              <p className="text-sm font-medium" style={{ color: ADMIN_TEXT }}>
                {admin?.name || 'Admin'}
              </p>
              <p className="text-xs" style={{ color: ADMIN_TEXT_MUTED }}>{admin?.email || 'a****@***********'}</p>
            </div>
            <Avatar className="h-9 w-9" style={{ border: '2px solid rgba(99,102,241,0.3)' }}>
              <AvatarFallback className="text-sm font-semibold text-white" style={{ background: 'linear-gradient(135deg, #6366f1, #a855f7)' }}>
                {getInitials(admin?.name || 'A')}
              </AvatarFallback>
            </Avatar>
          </div>
          <Button
            variant="ghost"
            size="icon"
            onClick={() => logout()}
            className="text-slate-500 hover:text-red-400 hover:bg-red-500/5"
            title="退出登录"
          >
            <LogOut className="w-4 h-4" />
          </Button>
        </div>
      </div>
    </header>
  )
}
