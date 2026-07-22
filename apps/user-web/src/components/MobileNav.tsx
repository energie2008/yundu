import { NavLink } from 'react-router-dom'
import { Home, ShoppingBag, FileText, User } from 'lucide-react'

const navItems = [
  { path: '/dashboard', label: '首页', icon: Home, end: true },
  { path: '/dashboard/plans', label: '套餐', icon: ShoppingBag },
  { path: '/dashboard/orders', label: '订单', icon: FileText },
  { path: '/dashboard/profile', label: '我的', icon: User },
]

export function MobileNav() {
  return (
    <nav
      className="fixed bottom-0 left-0 right-0 z-50 border-t backdrop-blur-sm safe-bottom"
      style={{ background: 'var(--card)', borderColor: 'var(--border)' }}
    >
      <div className="flex items-center justify-around h-16 px-2 max-w-lg mx-auto">
        {navItems.map((item) => (
          <NavLink
            key={item.path}
            to={item.path}
            end={item.end}
            className="flex flex-col items-center justify-center gap-1 px-3 py-2 rounded-xl transition-all duration-150 min-w-0 flex-1"
          >
            {({ isActive }) => (
              <>
                <item.icon
                  className="w-6 h-6"
                  style={{ color: isActive ? 'var(--primary)' : 'var(--muted-foreground)' }}
                />
                <span
                  className="text-xs font-medium"
                  style={{ color: isActive ? 'var(--primary)' : 'var(--muted-foreground)' }}
                >
                  {item.label}
                </span>
              </>
            )}
          </NavLink>
        ))}
      </div>
    </nav>
  )
}
