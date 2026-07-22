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
    <nav className="fixed bottom-0 left-0 right-0 z-50 border-t border-zinc-800 bg-zinc-950/95 backdrop-blur-sm safe-bottom tablet:hidden">
      {/* 上下文指示条：颜色 + 分组名，提示当前底部 Tab 所属上下文 */}
      <div className={cn('h-0.5', groupColor)} />
      <div className="flex items-center justify-between px-3 pt-1 pb-0.5">
        <span className="text-[10px] text-zinc-500 uppercase tracking-wider">{groupLabel}</span>
      </div>

      <div className="flex items-center justify-around h-14 px-1">
        {tabs.map((item) => (
          <NavLink
            key={item.path}
            to={item.path}
            // end 属性：仅当 path 完全匹配时高亮（避免 /nodes 下的子路由也高亮 /nodes）
            end={item.path === '/dashboard'}
            className={({ isActive }) =>
              cn(
                'flex flex-col items-center justify-center gap-0.5 px-2 py-1 rounded-lg transition-all duration-150 min-w-0 flex-1',
                isActive
                  ? 'text-indigo-400'
                  : 'text-zinc-500 hover:text-zinc-300'
              )
            }
          >
            {({ isActive }) => {
              const Icon = item.icon
              return (
                <>
                  {Icon && <Icon className={cn('w-5 h-5', isActive && 'text-indigo-400')} />}
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
