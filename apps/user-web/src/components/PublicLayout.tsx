import { Outlet, useLocation, Link } from 'react-router-dom'
import { useEffect } from 'react'
import { Zap } from 'lucide-react'
import { useAuthStore } from '@/lib/auth'
import { Button } from '@airport/ui'

export function PublicLayout() {
  const location = useLocation()
  const { isAuthenticated } = useAuthStore()

  useEffect(() => {
    window.scrollTo(0, 0)
  }, [location.pathname])

  return (
    <div className="min-h-screen bg-[#F8F9FC]">
      <header className="sticky top-0 z-40 border-b border-gray-200 bg-white/90 backdrop-blur-sm">
        <div className="flex items-center justify-between h-14 px-4 md:px-8 max-w-7xl mx-auto">
          <Link to="/" className="flex items-center gap-2">
            <div className="w-7 h-7 rounded-lg bg-[#8B73B5] flex items-center justify-center">
              <Zap className="w-4 h-4 text-white fill-white" />
            </div>
            <span className="text-lg font-bold text-gray-900">YunDu</span>
          </Link>
          <div>
            {isAuthenticated ? (
              <Link to="/dashboard">
                <Button className="bg-[#8B73B5] hover:bg-[#7A63A3] text-white h-9">
                  控制台
                </Button>
              </Link>
            ) : (
              <div className="flex items-center gap-2">
                <Link to="/login">
                  <Button variant="outline" className="border-gray-300 text-gray-700 hover:bg-gray-50 h-9">
                    登录
                  </Button>
                </Link>
                <Link to="/register">
                  <Button className="bg-[#8B73B5] hover:bg-[#7A63A3] text-white h-9">
                    注册
                  </Button>
                </Link>
              </div>
            )}
          </div>
        </div>
      </header>

      <main className="pb-8">
        <Outlet />
      </main>
    </div>
  )
}
