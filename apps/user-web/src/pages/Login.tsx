import { useEffect, useState } from 'react'
import { Link, useNavigate, Navigate, useSearchParams } from 'react-router-dom'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { Mail, Lock, Eye, EyeOff } from 'lucide-react'
import { useToast } from '../lib/toast'
import { useAuthStore } from '../lib/auth'

const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
const loginSchema = z.object({
  email: z.string().regex(emailRegex, '请输入有效的邮箱地址'),
  password: z.string().min(6, '密码至少 6 个字符'),
  rememberMe: z.boolean().optional(),
})

type LoginFormData = z.infer<typeof loginSchema>

function Button({
  children,
  type = 'button',
  className = '',
  disabled,
  isLoading,
  onClick,
  style,
  onMouseEnter,
  onMouseLeave,
}: {
  children: React.ReactNode
  type?: 'button' | 'submit' | 'reset'
  className?: string
  disabled?: boolean
  isLoading?: boolean
  onClick?: (e: React.MouseEvent) => void
  style?: React.CSSProperties
  onMouseEnter?: (e: React.MouseEvent<HTMLButtonElement>) => void
  onMouseLeave?: (e: React.MouseEvent<HTMLButtonElement>) => void
}) {
  return (
    <button
      type={type}
      disabled={disabled || isLoading}
      onClick={onClick}
      className={className}
      style={{
        cursor: disabled || isLoading ? 'not-allowed' : 'pointer',
        opacity: disabled || isLoading ? 0.6 : 1,
        ...style,
      }}
      onMouseEnter={onMouseEnter}
      onMouseLeave={onMouseLeave}
    >
      {isLoading ? '处理中...' : children}
    </button>
  )
}

export function Login() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { toast } = useToast()
  const { login, isAuthenticated, isLoading, init } = useAuthStore()
  const [showPassword, setShowPassword] = useState(false)

  useEffect(() => {
    init()
  }, [init])

  useEffect(() => {
    if (searchParams.get('registered') === '1') {
      toast({
        title: '注册成功',
        description: '请查收验证邮件，验证后即可登录',
      })
    }
    if (searchParams.get('verified') === '1') {
      toast({
        title: '邮箱验证成功',
        description: '现在可以登录了',
      })
    }
    if (searchParams.get('reset') === '1') {
      toast({
        title: '密码重置成功',
        description: '请使用新密码登录',
      })
    }
  }, [searchParams, toast])

  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<LoginFormData>({
    defaultValues: { email: '', password: '', rememberMe: false },
  })

  const onSubmit = async (data: LoginFormData) => {
    const result = loginSchema.safeParse(data)
    if (!result.success) {
      result.error.issues.forEach((issue) => {
        setError(issue.path[0] as keyof LoginFormData, {
          type: 'manual',
          message: issue.message,
        })
      })
      return
    }

    try {
      await login(data.email, data.password)
      toast({ title: '登录成功', description: '欢迎回来' })
      const returnUrl = searchParams.get('returnUrl')
      navigate(returnUrl ? decodeURIComponent(returnUrl) : '/dashboard', { replace: true })
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
      <div className="min-h-screen flex items-center justify-center" style={{ background: 'var(--background)' }}>
        <div className="animate-spin rounded-full h-8 w-8 border-b-2" style={{ borderColor: 'var(--primary)' }} />
      </div>
    )
  }

  if (isAuthenticated) {
    return <Navigate to="/dashboard" replace />
  }

  return (
    <div
      className="min-h-screen flex items-center justify-center p-4"
      style={{ background: 'linear-gradient(135deg, #f0edff 0%, #f5f7fb 100%)', minHeight: '100vh' }}
    >
      <div className="w-full max-w-md">
        {/* Brand header */}
        <div className="text-center mb-6">
          <div className="mx-auto w-14 h-14 rounded-xl flex items-center justify-center mb-3 shadow-md" style={{ background: 'var(--primary)' }}>
            <span className="text-white font-bold text-xl">Y</span>
          </div>
          <h1 className="text-2xl font-bold" style={{ color: 'var(--foreground)' }}>YunDu 云渡</h1>
          <p className="text-sm mt-1" style={{ color: 'var(--muted-foreground)' }}>全球化网络加速服务</p>
        </div>

        {/* Card */}
        <div className="xboard-card p-8">
          <div className="mb-6">
            <h2 className="text-xl font-bold" style={{ color: 'var(--foreground)' }}>欢迎回来</h2>
            <p className="text-sm mt-1" style={{ color: 'var(--muted-foreground)' }}>登录您的 YunDu 账户</p>
          </div>

          <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
            <div className="space-y-1.5">
              <label className="text-sm font-medium" style={{ color: 'var(--foreground)' }}>邮箱</label>
              <div className="relative">
                <Mail className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5" style={{ color: 'var(--muted-foreground)' }} />
                <input
                  type="email"
                  placeholder="a****@***********"
                  className="w-full h-11 pl-10 pr-3 rounded-lg text-sm outline-none transition-colors focus:ring-2"
                  style={{ background: 'var(--card)', border: '1px solid var(--border)', color: 'var(--foreground)' }}
                  {...register('email', { required: '请输入邮箱' })}
                />
              </div>
              {errors.email && <p className="text-sm" style={{ color: 'var(--destructive)' }}>{errors.email.message}</p>}
            </div>

            <div className="space-y-1.5">
              <label className="text-sm font-medium" style={{ color: 'var(--foreground)' }}>密码</label>
              <div className="relative">
                <Lock className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5" style={{ color: 'var(--muted-foreground)' }} />
                <input
                  type={showPassword ? 'text' : 'password'}
                  placeholder="••••••••"
                  className="w-full h-11 pl-10 pr-10 rounded-lg text-sm outline-none transition-colors focus:ring-2"
                  style={{ background: 'var(--card)', border: '1px solid var(--border)', color: 'var(--foreground)' }}
                  {...register('password', { required: '请输入密码' })}
                />
                <button
                  type="button"
                  onClick={() => setShowPassword(!showPassword)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 transition-colors"
                  style={{ color: 'var(--muted-foreground)' }}
                  onMouseEnter={e => (e.currentTarget.style.color = 'var(--foreground)')}
                  onMouseLeave={e => (e.currentTarget.style.color = 'var(--muted-foreground)')}
                >
                  {showPassword ? <EyeOff className="w-5 h-5" /> : <Eye className="w-5 h-5" />}
                </button>
              </div>
              {errors.password && <p className="text-sm" style={{ color: 'var(--destructive)' }}>{errors.password.message}</p>}
            </div>

            <div className="flex items-center justify-between">
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  className="w-4 h-4 rounded"
                  style={{ accentColor: 'var(--primary)' }}
                  {...register('rememberMe')}
                />
                <span className="text-sm" style={{ color: 'var(--muted-foreground)' }}>记住我</span>
              </label>
              <Link to="/forgot-password" className="text-sm transition-colors" style={{ color: 'var(--primary)' }}>
                忘记密码？
              </Link>
            </div>

            <Button
              type="submit"
              className="w-full h-11 text-base text-white border-0 shadow-sm rounded-lg"
              style={{ background: 'var(--primary)' }}
              isLoading={isSubmitting}
            >
              登录
            </Button>
          </form>

          <div className="mt-6 text-center">
            <p className="text-sm" style={{ color: 'var(--muted-foreground)' }}>
              还没有账号？
              <Link to="/register" className="ml-1 font-medium transition-colors" style={{ color: 'var(--primary)' }}>
                立即注册
              </Link>
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}
