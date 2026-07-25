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
            ? 'font-medium shadow-sm'
            : 'hover:bg-opacity-50 border border-transparent'
        )
      }
      style={({ isActive }) => ({
        background: isActive ? 'var(--sidebar-active)' : 'transparent',
        color: isActive ? 'var(--sidebar-active-text)' : 'var(--sidebar-text)',
        border: isActive ? '1px solid var(--primary-glow)' : '1px solid transparent',
      })}
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
          className="w-full flex items-center justify-between px-3 mb-1 text-[11px] font-semibold uppercase tracking-wider transition-colors"
          style={{ color: 'var(--muted-foreground)' }}
        >
          <span>{group.label}</span>
          {open ? <ChevronDown className="w-3 h-3" /> : <ChevronRight className="w-3 h-3" />}
        </button>
      ) : (
        <div className="px-3 mb-2 text-[11px] font-semibold uppercase tracking-wider" style={{ color: 'var(--muted-foreground)' }}>
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
    <aside
      className="hidden tablet:flex fixed left-0 top-0 bottom-0 w-60 flex-col z-40 transition-colors duration-300"
      style={{ backgroundColor: 'var(--sidebar-bg)', borderRight: '1px solid var(--sidebar-border)' }}
    >
      {/* Logo Header */}
      <div
        className="flex items-center gap-3 px-5 h-16"
        style={{ borderBottom: '1px solid var(--sidebar-border)' }}
      >
        <div
          className="w-9 h-9 rounded-xl flex items-center justify-center shadow-lg"
          style={{ background: 'var(--gradient-primary)', boxShadow: 'var(--shadow-glow)' }}
        >
          <Shield className="w-5 h-5 text-white" />
        </div>
        <div className="flex flex-col">
          <span className="font-bold text-[15px] leading-tight" style={{ color: 'var(--foreground)' }}>云渡 YunDu</span>
          <span className="text-[10px] leading-tight" style={{ color: 'var(--muted-foreground)' }}>Admin 管理后台</span>
        </div>
      </div>

      {/* Navigation */}
      <nav className="flex-1 overflow-y-auto p-3 space-y-4 scrollbar-thin">
        {sidebarGroups.map((group) => (
          <SidebarGroup key={group.label} group={group} />
        ))}
      </nav>

      {/* User Section */}
      <div className="p-3" style={{ borderTop: '1px solid var(--sidebar-border)' }}>
        <NavLink
          to="/profile"
          className={({ isActive }) =>
            cn(
              'flex items-center gap-3 px-3 py-2.5 rounded-xl text-sm transition-all duration-200',
              isActive ? 'font-medium' : 'border border-transparent'
            )
          }
          style={({ isActive }) => ({
            background: isActive ? 'var(--sidebar-active)' : 'transparent',
            color: isActive ? 'var(--sidebar-active-text)' : 'var(--sidebar-text)',
            border: isActive ? '1px solid var(--primary-glow)' : '1px solid transparent',
          })}
        >
          <UserCog className="w-4 h-4 shrink-0" />
          <div className="flex-1 min-w-0">
            <div className="truncate font-medium">{admin?.name || '管理员'}</div>
            <div className="text-[10px] truncate" style={{ color: 'var(--muted-foreground)' }}>{admin?.email || ''}</div>
          </div>
        </NavLink>
        <button
          onClick={() => logout()}
          className="w-full flex items-center gap-3 px-3 py-2.5 rounded-xl text-sm transition-all duration-200 mt-1"
          style={{ color: 'var(--muted-foreground)' }}
          onMouseEnter={e => { e.currentTarget.style.backgroundColor = 'var(--accent-rose)'; e.currentTarget.style.color = 'var(--destructive)' }}
          onMouseLeave={e => { e.currentTarget.style.backgroundColor = 'transparent'; e.currentTarget.style.color = 'var(--muted-foreground)' }}
        >
          <LogOut className="w-4 h-4 shrink-0" />
          <span>退出登录</span>
        </button>
      </div>
    </aside>
  )
}
