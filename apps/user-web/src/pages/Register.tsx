import { useEffect, useState } from 'react'
import { Link, useNavigate, Navigate } from 'react-router-dom'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { Mail, Lock, Eye, EyeOff, Ticket } from 'lucide-react'
import { useToast } from '../lib/toast'
import { useAuthStore } from '../lib/auth'

const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
const registerSchema = z
  .object({
    email: z.string().regex(emailRegex, '请输入有效的邮箱地址'),
    emailCode: z.string().min(6, '请输入邮箱验证码').max(6, '验证码为6位数字'),
    password: z.string().min(6, '密码至少 6 个字符'),
    confirmPassword: z.string().min(6, '请确认密码'),
    inviteCode: z.string().optional(),
    agreeTerms: z.boolean().refine((val) => val === true, {
      message: '请同意服务条款和隐私政策',
    }),
  })
  .refine((data) => data.password === data.confirmPassword, {
    message: '两次输入的密码不一致',
    path: ['confirmPassword'],
  })

type RegisterFormData = z.infer<typeof registerSchema>

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

export function Register() {
  const navigate = useNavigate()
  const { toast } = useToast()
  const { register: registerUser, sendEmailCode, isAuthenticated, isLoading, init } = useAuthStore()
  const [showPassword, setShowPassword] = useState(false)
  const [showConfirmPassword, setShowConfirmPassword] = useState(false)
  const [countdown, setCountdown] = useState(0)
  const [sendingCode, setSendingCode] = useState(false)

  useEffect(() => {
    init()
  }, [init])

  // 验证码发送冷却倒计时（60s，参考 Xboard email_verify）
  useEffect(() => {
    if (countdown <= 0) return
    const timer = setInterval(() => {
      setCountdown((prev) => prev - 1)
    }, 1000)
    return () => clearInterval(timer)
  }, [countdown])

  const {
    register,
    handleSubmit,
    setError,
    getValues,
    formState: { errors, isSubmitting },
  } = useForm<RegisterFormData>({
    defaultValues: { email: '', emailCode: '', password: '', confirmPassword: '', inviteCode: '', agreeTerms: false },
  })

  // 发送注册验证码：先校验邮箱格式，再调用后端 send-email-code 接口
  const handleSendCode = async () => {
    const email = getValues('email')
    if (!email || !emailRegex.test(email)) {
      setError('email', { type: 'manual', message: '请先输入有效的邮箱地址' })
      return
    }
    try {
      setSendingCode(true)
      await sendEmailCode(email)
      setCountdown(60)
      toast({ title: '验证码已发送', description: '请查收邮箱', variant: 'success' })
    } catch (err) {
      toast({
        title: '发送失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setSendingCode(false)
    }
  }

  const onSubmit = async (data: RegisterFormData) => {
    const result = registerSchema.safeParse(data)
    if (!result.success) {
      result.error.issues.forEach((issue) => {
        setError(issue.path[0] as keyof RegisterFormData, {
          type: 'manual',
          message: issue.message,
        })
      })
      return
    }

    try {
      const res = await registerUser(data.email, data.password, data.inviteCode, data.emailCode)
      if (res.requiresVerification) {
        toast({
          title: '注册成功',
          description: '请查收验证邮件，验证后即可登录',
          variant: 'success',
        })
        navigate('/login?registered=1', { replace: true })
      } else {
        toast({ title: '注册成功', description: '欢迎加入', variant: 'success' })
        navigate('/dashboard', { replace: true })
      }
    } catch (err) {
      toast({
        title: '注册失败',
        description: err instanceof Error ? err.message : '请稍后重试',
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
            <h2 className="text-xl font-bold" style={{ color: 'var(--foreground)' }}>创建账号</h2>
            <p className="text-sm mt-1" style={{ color: 'var(--muted-foreground)' }}>加入我们，享受高速网络服务</p>
          </div>

          <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
            <div className="space-y-1.5">
              <label className="text-sm font-medium" style={{ color: 'var(--foreground)' }}>邮箱</label>
              <div className="relative">
                <Mail className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5" style={{ color: 'var(--muted-foreground)' }} />
                <input
                  type="email"
                  placeholder="a****@***********"
                  className="w-full h-11 pl-10 pr-3 rounded-lg text-sm outline-none transition-colors"
                  style={{ background: 'var(--card)', border: '1px solid var(--border)', color: 'var(--foreground)' }}
                  {...register('email', { required: '请输入邮箱' })}
                />
              </div>
              {errors.email && <p className="text-sm" style={{ color: 'var(--destructive)' }}>{errors.email.message}</p>}
            </div>

            <div className="space-y-1.5">
              <label className="text-sm font-medium" style={{ color: 'var(--foreground)' }}>邮箱验证码</label>
              <div className="flex gap-2">
                <div className="relative flex-1">
                  <Mail className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5" style={{ color: 'var(--muted-foreground)' }} />
                  <input
                    type="text"
                    inputMode="numeric"
                    maxLength={6}
                    placeholder="请输入6位验证码"
                    className="w-full h-11 pl-10 pr-3 rounded-lg text-sm outline-none transition-colors"
                    style={{ background: 'var(--card)', border: '1px solid var(--border)', color: 'var(--foreground)' }}
                    {...register('emailCode', { required: '请输入验证码' })}
                  />
                </div>
                <button
                  type="button"
                  onClick={handleSendCode}
                  disabled={countdown > 0 || sendingCode}
                  className="h-11 px-4 rounded-lg text-sm font-medium whitespace-nowrap transition-opacity"
                  style={{
                    background: countdown > 0 || sendingCode ? 'var(--muted)' : 'var(--primary)',
                    color: '#fff',
                    border: '1px solid var(--border)',
                    cursor: countdown > 0 || sendingCode ? 'not-allowed' : 'pointer',
                    opacity: countdown > 0 || sendingCode ? 0.6 : 1,
                  }}
                >
                  {sendingCode ? '发送中...' : countdown > 0 ? `${countdown}s 后重发` : '获取验证码'}
                </button>
              </div>
              {errors.emailCode && <p className="text-sm" style={{ color: 'var(--destructive)' }}>{errors.emailCode.message}</p>}
            </div>

            <div className="space-y-1.5">
              <label className="text-sm font-medium" style={{ color: 'var(--foreground)' }}>密码</label>
              <div className="relative">
                <Lock className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5" style={{ color: 'var(--muted-foreground)' }} />
                <input
                  type={showPassword ? 'text' : 'password'}
                  placeholder="至少 6 位密码"
                  className="w-full h-11 pl-10 pr-10 rounded-lg text-sm outline-none transition-colors"
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

            <div className="space-y-1.5">
              <label className="text-sm font-medium" style={{ color: 'var(--foreground)' }}>确认密码</label>
              <div className="relative">
                <Lock className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5" style={{ color: 'var(--muted-foreground)' }} />
                <input
                  type={showConfirmPassword ? 'text' : 'password'}
                  placeholder="再次输入密码"
                  className="w-full h-11 pl-10 pr-10 rounded-lg text-sm outline-none transition-colors"
                  style={{ background: 'var(--card)', border: '1px solid var(--border)', color: 'var(--foreground)' }}
                  {...register('confirmPassword', { required: '请确认密码' })}
                />
                <button
                  type="button"
                  onClick={() => setShowConfirmPassword(!showConfirmPassword)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 transition-colors"
                  style={{ color: 'var(--muted-foreground)' }}
                  onMouseEnter={e => (e.currentTarget.style.color = 'var(--foreground)')}
                  onMouseLeave={e => (e.currentTarget.style.color = 'var(--muted-foreground)')}
                >
                  {showConfirmPassword ? <EyeOff className="w-5 h-5" /> : <Eye className="w-5 h-5" />}
                </button>
              </div>
              {errors.confirmPassword && (
                <p className="text-sm" style={{ color: 'var(--destructive)' }}>{errors.confirmPassword.message}</p>
              )}
            </div>

            <div className="space-y-1.5">
              <label className="text-sm font-medium" style={{ color: 'var(--foreground)' }}>邀请码（选填）</label>
              <div className="relative">
                <Ticket className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5" style={{ color: 'var(--muted-foreground)' }} />
                <input
                  type="text"
                  placeholder="输入邀请码"
                  className="w-full h-11 pl-10 pr-3 rounded-lg text-sm outline-none transition-colors"
                  style={{ background: 'var(--card)', border: '1px solid var(--border)', color: 'var(--foreground)' }}
                  {...register('inviteCode')}
                />
              </div>
            </div>

            <div className="space-y-1.5">
              <label className="flex items-start gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  className="w-4 h-4 mt-0.5 rounded"
                  style={{ accentColor: 'var(--primary)' }}
                  {...register('agreeTerms')}
                />
                <span className="text-sm" style={{ color: 'var(--muted-foreground)' }}>
                  我已阅读并同意
                  <a href="#" className="ml-1" style={{ color: 'var(--primary)' }}>服务条款</a>
                  和
                  <a href="#" className="ml-1" style={{ color: 'var(--primary)' }}>隐私政策</a>
                </span>
              </label>
              {errors.agreeTerms && <p className="text-sm" style={{ color: 'var(--destructive)' }}>{errors.agreeTerms.message}</p>}
            </div>

            <Button
              type="submit"
              className="w-full h-11 text-base text-white border-0 shadow-sm rounded-lg"
              style={{ background: 'var(--primary)' }}
              isLoading={isSubmitting}
            >
              注册
            </Button>
          </form>

          <div className="mt-6 text-center">
            <p className="text-sm" style={{ color: 'var(--muted-foreground)' }}>
              已有账号？
              <Link to="/login" className="ml-1 font-medium transition-colors" style={{ color: 'var(--primary)' }}>
                立即登录
              </Link>
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}
