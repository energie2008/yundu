import { Outlet, useLocation } from 'react-router-dom'
import { useEffect } from 'react'
import { Sidebar } from './Sidebar'
import { BottomNav } from './BottomNav'
import { Header } from './Header'
import { useAuthStore } from '@/lib/auth'
import { ADMIN_BG } from '@/lib/theme'

export function Layout() {
  const location = useLocation()
  const { fetchMe, admin } = useAuthStore()

  useEffect(() => {
    if (!admin) {
      fetchMe()
    }
  }, [admin, fetchMe])

  useEffect(() => {
    window.scrollTo(0, 0)
  }, [location.pathname])

  return (
    <div className="min-h-screen text-slate-200" style={{ backgroundColor: ADMIN_BG }}>
      <Sidebar />

      <div className="tablet:pl-60">
        <Header />

        <main className="pb-20 tablet:pb-6">
          <div className="p-4 tablet:p-6 max-w-7xl mx-auto">
            <Outlet />
          </div>
        </main>
      </div>

      <BottomNav />
    </div>
  )
}
