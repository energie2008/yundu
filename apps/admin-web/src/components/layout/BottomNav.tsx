import { NavLink, useLocation } from 'react-router-dom'
import { cn } from '@airport/ui'
import {
  TAB_GROUPS,
  TAB_GROUP_LABELS,
  TAB_GROUP_COLORS,
  getTabGroup,
} from '@/lib/nav'

export function BottomNav() {
  const location = useLocation()
  const group = getTabGroup(location.pathname)
  const tabs = TAB_GROUPS[group]
  const groupLabel = TAB_GROUP_LABELS[group]
  const groupColor = TAB_GROUP_COLORS[group]

  return (
    <nav
      className="fixed bottom-0 left-0 right-0 z-50 backdrop-blur-sm safe-bottom tablet:hidden transition-colors duration-300"
      style={{
        borderTop: '1px solid var(--border)',
        backgroundColor: 'var(--header-bg)',
      }}
    >
      {/* Context indicator bar */}
      <div className={cn('h-0.5', groupColor)} />
      <div className="flex items-center justify-between px-3 pt-1 pb-0.5">
        <span className="text-[10px] uppercase tracking-wider" style={{ color: 'var(--muted-foreground)' }}>{groupLabel}</span>
      </div>

      <div className="flex items-center justify-around h-14 px-1">
        {tabs.map((item) => (
          <NavLink
            key={item.path}
            to={item.path}
            end={item.path === '/dashboard'}
            className={({ isActive }) =>
              cn(
                'flex flex-col items-center justify-center gap-0.5 px-2 py-1 rounded-lg transition-all duration-150 min-w-0 flex-1',
              )
            }
            style={({ isActive }) => ({
              color: isActive ? 'var(--primary)' : 'var(--muted-foreground)',
            })}
          >
            {({ isActive }) => {
              const Icon = item.icon
              return (
                <>
                  {Icon && <Icon className={cn('w-5 h-5')} style={{ color: isActive ? 'var(--primary)' : 'var(--muted-foreground)' }} />}
                  <span className="text-[10px] truncate">{item.label}</span>
                </>
              )
            }}
          </NavLink>
        ))}
      </div>
    </nav>
  )
}
