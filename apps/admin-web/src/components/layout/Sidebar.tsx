import { NavLink, useLocation } from 'react-router-dom'
import {
  ChevronDown,
  ChevronRight,
  LogOut,
  Shield,
  UserCog,
} from 'lucide-react'
import { useState } from 'react'
import { cn } from '@airport/ui'
import { useAuthStore } from '@/lib/auth'
import { sidebarGroups } from '@/lib/nav'

function SidebarLink({
  item,
}: {
  item: { label: string; path: string; icon?: React.ComponentType<{ className?: string }>; badge?: string; badgeColor?: string }
}) {
  return (
    <NavLink
      to={item.path}
      className={({ isActive }) =>
        cn(
          'group flex items-center gap-3 px-3 py-2.5 rounded-xl text-sm transition-all duration-200',
          isActive
            ? 'bg-gradient-to-r from-indigo-500/15 to-purple-500/10 text-indigo-300 font-medium shadow-sm border border-indigo-500/20'
            : 'text-slate-400 hover:bg-white/[0.03] hover:text-slate-200 border border-transparent'
        )
      }
    >
      {item.icon && <item.icon className="w-4 h-4 shrink-0" />}
      <span className="flex-1 truncate">{item.label}</span>
      {item.badge && (
        <span
          className={cn(
            'text-[10px] font-semibold px-1.5 py-0.5 rounded-md text-white shrink-0',
            item.badgeColor || 'bg-slate-600'
          )}
        >
          {item.badge}
        </span>
      )}
    </NavLink>
  )
}

function SidebarGroup({
  group,
}: {
  group: { label: string; items: any[]; collapsible?: boolean }
}) {
  const location = useLocation()
  const hasActive = group.items.some((i) => location.pathname.startsWith(i.path))
  const [open, setOpen] = useState(hasActive || !group.collapsible)

  return (
    <div>
      {group.collapsible ? (
        <button
          onClick={() => setOpen(!open)}
          className="w-full flex items-center justify-between px-3 mb-1 text-[11px] font-semibold text-slate-500 uppercase tracking-wider hover:text-slate-300 transition-colors"
        >
          <span>{group.label}</span>
          {open ? <ChevronDown className="w-3 h-3" /> : <ChevronRight className="w-3 h-3" />}
        </button>
      ) : (
        <div className="px-3 mb-2 text-[11px] font-semibold text-slate-500 uppercase tracking-wider">
          {group.label}
        </div>
      )}
      {(open || !group.collapsible) && (
        <div className="space-y-0.5">
          {group.items.map((item) => (
            <SidebarLink key={item.path} item={item} />
          ))}
        </div>
      )}
    </div>
  )
}

export function Sidebar() {
  const { admin, logout } = useAuthStore()

  return (
    <aside className="hidden tablet:flex fixed left-0 top-0 bottom-0 w-60 flex-col z-40" style={{ backgroundColor: '#0d1326', borderRight: '1px solid rgba(99,102,241,0.1)' }}>
      <div className="flex items-center gap-3 px-5 h-16" style={{ borderBottom: '1px solid rgba(99,102,241,0.1)' }}>
        <div className="w-9 h-9 rounded-xl flex items-center justify-center shadow-lg shadow-indigo-500/20" style={{ background: 'linear-gradient(135deg, #6366f1, #a855f7)' }}>
          <Shield className="w-5 h-5 text-white" />
        </div>
        <div className="flex flex-col">
          <span className="font-bold text-slate-100 text-[15px] leading-tight">云渡 YunDu</span>
          <span className="text-[10px] text-slate-500 leading-tight">Admin 管理后台</span>
        </div>
      </div>

      <nav className="flex-1 overflow-y-auto p-3 space-y-4 scrollbar-thin">
        {sidebarGroups.map((group) => (
          <SidebarGroup key={group.label} group={group} />
        ))}
      </nav>

      <div className="p-3" style={{ borderTop: '1px solid rgba(99,102,241,0.1)' }}>
        <NavLink
          to="/profile"
          className={({ isActive }) =>
            cn(
              'flex items-center gap-3 px-3 py-2.5 rounded-xl text-sm transition-all duration-200',
              isActive
                ? 'bg-gradient-to-r from-indigo-500/15 to-purple-500/10 text-indigo-300 font-medium border border-indigo-500/20'
                : 'text-slate-400 hover:bg-white/[0.03] hover:text-slate-200 border border-transparent'
            )
          }
        >
          <UserCog className="w-4 h-4 shrink-0" />
          <div className="flex-1 min-w-0">
            <div className="truncate font-medium">{admin?.name || '管理员'}</div>
            <div className="text-[10px] text-slate-500 truncate">{admin?.email || ''}</div>
          </div>
        </NavLink>
        <button
          onClick={() => logout()}
          className="w-full flex items-center gap-3 px-3 py-2.5 rounded-xl text-sm text-slate-500 hover:bg-red-500/5 hover:text-red-400 transition-all duration-200 mt-1"
        >
          <LogOut className="w-4 h-4 shrink-0" />
          <span>退出登录</span>
        </button>
      </div>
    </aside>
  )
}
