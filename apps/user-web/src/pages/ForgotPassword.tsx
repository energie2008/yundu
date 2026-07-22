import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { ArrowLeft, Mail, Send, CheckCircle2 } from 'lucide-react'
import { useToast } from '@/lib/toast'
import { useAuthStore } from '@/lib/auth'

const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
const forgotSchema = z.object({
  email: z.string().regex(emailRegex, '请输入有效的邮箱地址'),
})

type ForgotFormData = z.infer<typeof forgotSchema>

export default function ForgotPassword() {
  const { toast } = useToast()
  const { forgotPassword } = useAuthStore()
  const [sent, setSent] = useState(false)

  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<ForgotFormData>({
    defaultValues: { email: '' },
  })

  const onSubmit = async (data: ForgotFormData) => {
    const result = forgotSchema.safeParse(data)
    if (!result.success) {
      result.error.issues.forEach((issue) => {
        setError(issue.path[0] as keyof ForgotFormData, {
          type: 'manual',
          message: issue.message,
        })
      })
      return
    }

    try {
      await forgotPassword(data.email)
      setSent(true)
      toast({
        title: '邮件已发送',
        description: '如果该邮箱已注册，您将收到重置密码邮件',
        variant: 'success',
      })
    } catch (err) {
      toast({
        title: '发送失败',
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
          {sent ? (
            <div className="text-center py-4">
              <div className="mx-auto w-20 h-20 rounded-full flex items-center justify-center mb-6" style={{ background: 'rgba(34,197,94,0.1)' }}>
                <CheckCircle2 className="w-10 h-10" style={{ color: 'var(--success)' }} />
              </div>
              <h2 className="text-2xl font-bold mb-2" style={{ color: 'var(--foreground)' }}>检查您的邮箱</h2>
              <p className="text-sm mb-8 leading-relaxed" style={{ color: 'var(--muted-foreground)' }}>
                我们已向您的邮箱发送了重置密码链接，请点击邮件中的链接完成重置。
              </p>
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
          ) : (
            <>
              <div className="mb-6">
                <h2 className="text-xl font-bold" style={{ color: 'var(--foreground)' }}>找回密码</h2>
                <p className="text-sm mt-1" style={{ color: 'var(--muted-foreground)' }}>输入注册邮箱，我们将发送重置链接</p>
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

                <button
                  type="submit"
                  disabled={isSubmitting}
                  className="w-full h-11 rounded-lg text-base font-medium text-white shadow-sm border-0 flex items-center justify-center gap-2 transition-opacity"
                  style={{ background: 'var(--primary)' }}
                  onMouseEnter={e => (e.currentTarget.style.opacity = '0.9')}
                  onMouseLeave={e => (e.currentTarget.style.opacity = '1')}
                >
                  <Send className="w-4 h-4" />
                  {isSubmitting ? '发送中...' : '发送重置邮件'}
                </button>

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
