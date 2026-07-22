import { useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { ArrowLeft, CheckCircle2, Lock, Eye, EyeOff, KeyRound } from 'lucide-react'
import { useToast } from '@/lib/toast'
import { useAuthStore } from '@/lib/auth'

const resetSchema = z
  .object({
    password: z.string().min(8, '密码至少 8 个字符'),
    confirmPassword: z.string().min(8, '请确认密码'),
  })
  .refine((data) => data.password === data.confirmPassword, {
    message: '两次输入的密码不一致',
    path: ['confirmPassword'],
  })

type ResetFormData = z.infer<typeof resetSchema>

export default function ResetPassword() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { toast } = useToast()
  const { resetPassword } = useAuthStore()
  const [success, setSuccess] = useState(false)
  const [showPassword, setShowPassword] = useState(false)
  const [showConfirmPassword, setShowConfirmPassword] = useState(false)

  const token = searchParams.get('token') || ''

  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<ResetFormData>({
    defaultValues: { password: '', confirmPassword: '' },
  })

  const onSubmit = async (data: ResetFormData) => {
    const result = resetSchema.safeParse(data)
    if (!result.success) {
      result.error.issues.forEach((issue) => {
        setError(issue.path[0] as keyof ResetFormData, {
          type: 'manual',
          message: issue.message,
        })
      })
      return
    }

    if (!token) {
      toast({
        title: '无效链接',
        description: '重置链接无效或已过期',
        variant: 'destructive',
      })
      return
    }

    try {
      await resetPassword(token, data.password)
      setSuccess(true)
      toast({
        title: '密码重置成功',
        description: '请使用新密码登录',
        variant: 'success',
      })
      setTimeout(() => {
        navigate('/login?reset=1', { replace: true })
      }, 2000)
    } catch (err) {
      toast({
        title: '重置失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    }
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
          {success ? (
            <div className="text-center py-4">
              <div className="mx-auto w-20 h-20 rounded-full flex items-center justify-center mb-6" style={{ background: 'rgba(34,197,94,0.1)' }}>
                <CheckCircle2 className="w-10 h-10" style={{ color: 'var(--success)' }} />
              </div>
              <h2 className="text-2xl font-bold mb-2" style={{ color: 'var(--foreground)' }}>密码已重置</h2>
              <p className="text-sm" style={{ color: 'var(--muted-foreground)' }}>正在跳转至登录页面...</p>
            </div>
          ) : (
            <>
              <div className="mb-6">
                <div className="w-12 h-12 rounded-xl flex items-center justify-center mb-4" style={{ background: 'rgba(124,92,252,0.1)' }}>
                  <KeyRound className="w-6 h-6" style={{ color: 'var(--primary)' }} />
                </div>
                <h2 className="text-xl font-bold" style={{ color: 'var(--foreground)' }}>重置密码</h2>
                <p className="text-sm mt-1" style={{ color: 'var(--muted-foreground)' }}>请输入您的新密码</p>
              </div>

              <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
                <div className="space-y-1.5">
                  <label className="text-sm font-medium" style={{ color: 'var(--foreground)' }}>新密码</label>
                  <div className="relative">
                    <Lock className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5" style={{ color: 'var(--muted-foreground)' }} />
                    <input
                      type={showPassword ? 'text' : 'password'}
                      placeholder="至少 8 位密码"
                      className="w-full h-11 pl-10 pr-10 rounded-lg text-sm outline-none transition-colors"
                      style={{ background: 'var(--card)', border: '1px solid var(--border)', color: 'var(--foreground)' }}
                      {...register('password', { required: '请输入新密码' })}
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
                  <label className="text-sm font-medium" style={{ color: 'var(--foreground)' }}>确认新密码</label>
                  <div className="relative">
                    <Lock className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5" style={{ color: 'var(--muted-foreground)' }} />
                    <input
                      type={showConfirmPassword ? 'text' : 'password'}
                      placeholder="再次输入新密码"
                      className="w-full h-11 pl-10 pr-10 rounded-lg text-sm outline-none transition-colors"
                      style={{ background: 'var(--card)', border: '1px solid var(--border)', color: 'var(--foreground)' }}
                      {...register('confirmPassword', { required: '请确认新密码' })}
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

                <button
                  type="submit"
                  disabled={isSubmitting || !token}
                  className="w-full h-11 rounded-lg text-base font-medium text-white shadow-sm border-0 transition-opacity"
                  style={{ background: 'var(--primary)' }}
                  onMouseEnter={e => { if (!e.currentTarget.disabled) e.currentTarget.style.opacity = '0.9'; }}
                  onMouseLeave={e => (e.currentTarget.style.opacity = '1')}
                >
                  {isSubmitting ? '提交中...' : '重置密码'}
                </button>

                {!token && (
                  <p className="text-sm text-center" style={{ color: 'var(--destructive)' }}>
                    无效的重置链接，请重新申请重置密码
                  </p>
                )}

                <div className="text-center pt-2">
                  <Link
                    to="/login"
                    className="inline-flex items-center text-sm transition-colors"
                    style={{ color: 'var(--primary)' }}
                    onMouseEnter={e => (e.currentTarget.style.opacity = '0.8')}
                    onMouseLeave={e => (e.currentTarget.style.opacity = '1')}
                  >
                    <ArrowLeft className="w-4 h-4 mr-2" />
                    返回登录
                  </Link>
                </div>
              </form>
            </>
          )}
        </div>
      </div>
    </div>
  )
}
