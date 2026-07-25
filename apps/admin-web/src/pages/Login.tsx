import { useEffect, useState } from 'react'
import { useNavigate, Navigate } from 'react-router-dom'
import { useForm } from 'react-hook-form'
import { Shield, Mail, Lock, Eye, EyeOff } from 'lucide-react'
import { useToast } from '@airport/ui'
import { useAuthStore } from '@/lib/auth'
import { useTheme } from '@/lib/theme-provider'

interface LoginFormData {
  email: string
  password: string
}

export default function Login() {
  const navigate = useNavigate()
  const { toast } = useToast()
  const { login, isAuthenticated, isLoading, init } = useAuthStore()
  const { theme, toggleTheme } = useTheme()
  const [showPassword, setShowPassword] = useState(false)

  useEffect(() => {
    init()
  }, [init])

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<LoginFormData>({
    defaultValues: {
      email: '',
      password: '',
    },
  })

  const validateEmail = (email: string) => {
    return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email) || '请输入有效的邮箱地址'
  }

  const onSubmit = async (data: LoginFormData) => {
    if (!validateEmail(data.email)) {
      toast({
        title: '输入错误',
        description: '请输入有效的邮箱地址',
        variant: 'destructive',
      })
      return
    }
    if (data.password.length < 6) {
      toast({
        title: '输入错误',
        description: '密码至少 6 个字符',
        variant: 'destructive',
      })
      return
    }

    try {
      await login(data.email, data.password)
      toast({
        title: '登录成功',
        description: '欢迎回来',
        variant: 'success',
      })
      navigate('/dashboard', { replace: true })
    } catch (err) {
      toast({
        title: '登录失败',
        description: err instanceof Error ? err.message : '请检查邮箱和密码',
        variant: 'destructive',
      })
    }
  }

  if (isLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center" style={{ backgroundColor: 'var(--background)' }}>
        <div className="animate-spin rounded-full h-8 w-8 border-b-2" style={{ borderColor: 'var(--primary)' }} />
      </div>
    )
  }

  if (isAuthenticated) {
    return <Navigate to="/dashboard" replace />
  }

  return (
    <div
      className="min-h-screen flex items-center justify-center p-4 relative overflow-hidden"
      style={{ background: 'var(--gradient-soft)', minHeight: '100vh' }}
    >
      {/* Decorative background elements */}
      <div className="absolute top-0 left-0 w-96 h-96 rounded-full opacity-10" style={{ background: 'var(--gradient-primary)', filter: 'blur(80px)', transform: 'translate(-30%, -30%)' }} />
      <div className="absolute bottom-0 right-0 w-96 h-96 rounded-full opacity-10" style={{ background: 'var(--gradient-info)', filter: 'blur(80px)', transform: 'translate(30%, 30%)' }} />

      {/* Theme toggle - top right */}
      <button
        onClick={toggleTheme}
        className="absolute top-4 right-4 p-2.5 rounded-xl transition-all hover:scale-105 z-10"
        style={{
          color: theme === 'light' ? 'var(--warning)' : 'var(--primary)',
          backgroundColor: theme === 'light' ? 'var(--accent-amber)' : 'var(--accent)',
        }}
        title={theme === 'light' ? '切换黑夜模式' : '切换白天模式'}
      >
        {theme === 'light' ? <EyeOff className="w-5 h-5" /> : <Eye className="w-5 h-5" />}
      </button>

      <div className="w-full max-w-md relative z-10">
        {/* Brand header */}
        <div className="text-center mb-6">
          <div
            className="mx-auto w-14 h-14 rounded-xl flex items-center justify-center mb-3 shadow-lg"
            style={{ background: 'var(--gradient-primary)', boxShadow: 'var(--shadow-glow)' }}
          >
            <Shield className="w-7 h-7 text-white" />
          </div>
          <h1 className="text-2xl font-bold" style={{ color: 'var(--foreground)' }}>Airport Panel</h1>
          <p className="text-sm mt-1" style={{ color: 'var(--muted-foreground)' }}>云渡 YunDu 管理后台</p>
        </div>

        {/* Login Card */}
        <div
          className="p-8 rounded-2xl"
          style={{
            backgroundColor: 'var(--card)',
            border: '1px solid var(--border)',
            boxShadow: 'var(--shadow-lg)',
          }}
        >
          <form onSubmit={handleSubmit(onSubmit)} className="space-y-5">
            {/* Email */}
            <div className="space-y-2">
              <label className="text-sm font-medium" style={{ color: 'var(--foreground)' }}>邮箱</label>
              <div className="relative">
                <Mail className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5" style={{ color: 'var(--muted-foreground)' }} />
                <input
                  type="email"
                  placeholder="a****@***********"
                  className="w-full h-11 pl-10 pr-3 rounded-lg text-sm outline-none transition-all focus:ring-2"
                  style={{
                    backgroundColor: 'var(--background)',
                    border: '1px solid var(--border)',
                    color: 'var(--foreground)',
                  }}
                  {...register('email', { required: '请输入邮箱' })}
                />
              </div>
              {errors.email && <p className="text-sm" style={{ color: 'var(--destructive)' }}>{errors.email.message}</p>}
            </div>

            {/* Password */}
            <div className="space-y-2">
              <label className="text-sm font-medium" style={{ color: 'var(--foreground)' }}>密码</label>
              <div className="relative">
                <Lock className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5" style={{ color: 'var(--muted-foreground)' }} />
                <input
                  type={showPassword ? 'text' : 'password'}
                  placeholder="••••••••"
                  className="w-full h-11 pl-10 pr-10 rounded-lg text-sm outline-none transition-all focus:ring-2"
                  style={{
                    backgroundColor: 'var(--background)',
                    border: '1px solid var(--border)',
                    color: 'var(--foreground)',
                  }}
                  {...register('password', { required: '请输入密码' })}
                />
                <button
                  type="button"
                  onClick={() => setShowPassword(!showPassword)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 transition-colors"
                  style={{ color: 'var(--muted-foreground)' }}
                >
                  {showPassword ? <EyeOff className="w-5 h-5" /> : <Eye className="w-5 h-5" />}
                </button>
              </div>
              {errors.password && <p className="text-sm" style={{ color: 'var(--destructive)' }}>{errors.password.message}</p>}
            </div>

            {/* Submit */}
            <button
              type="submit"
              disabled={isSubmitting}
              className="w-full h-11 text-base text-white rounded-lg transition-all hover:scale-[1.01] disabled:opacity-60"
              style={{
                background: 'var(--gradient-primary)',
                boxShadow: 'var(--shadow-md)',
              }}
            >
              {isSubmitting ? '登录中...' : '登录'}
            </button>
          </form>
        </div>

        {/* Footer */}
        <p className="text-center text-xs mt-6" style={{ color: 'var(--muted-foreground)' }}>
          © 2026 YunDu Cloud · 管理控制台
        </p>
      </div>
    </div>
  )
}
