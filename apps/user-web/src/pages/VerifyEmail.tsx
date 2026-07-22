import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { Mail, CheckCircle2, XCircle, Loader2 } from 'lucide-react'
import { api } from '@/lib/api'
import { EP } from '@/lib/endpoints'

type Status = 'loading' | 'success' | 'error' | 'no-token'

export default function VerifyEmail() {
  const [searchParams] = useSearchParams()
  const token = searchParams.get('token')
  const [status, setStatus] = useState<Status>(token ? 'loading' : 'no-token')
  const [message, setMessage] = useState<string>('')

  useEffect(() => {
    if (!token) {
      setStatus('no-token')
      return
    }

    let cancelled = false
    const verify = async () => {
      try {
        const res = await api.get<{ success?: boolean; message?: string }>(
          EP.EMAIL_VERIFY,
          { params: { token } }
        )
        if (cancelled) return
        if (res && typeof res === 'object' && res.success === false) {
          setStatus('error')
          setMessage(res.message || '邮箱验证失败，请重试')
        } else {
          setStatus('success')
          setMessage(res?.message || '您的邮箱已验证成功')
        }
      } catch (err: any) {
        if (cancelled) return
        setStatus('error')
        setMessage(err?.message || '验证链接无效或已过期')
      }
    }
    verify()

    return () => {
      cancelled = true
    }
  }, [token])

  const config = {
    loading: {
      icon: Loader2,
      color: 'var(--primary)',
      title: '正在验证邮箱...',
      desc: '请稍候，我们正在处理您的请求',
      spin: true,
    },
    success: {
      icon: CheckCircle2,
      color: 'var(--success)',
      title: '验证成功',
      desc: message,
      spin: false,
    },
    error: {
      icon: XCircle,
      color: 'var(--destructive)',
      title: '验证失败',
      desc: message,
      spin: false,
    },
    'no-token': {
      icon: Mail,
      color: 'var(--muted-foreground)',
      title: '无效的验证链接',
      desc: '未检测到验证令牌，请通过邮件中的链接重新访问',
      spin: false,
    },
  }[status]

  const Icon = config.icon

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
        <div className="xboard-card p-8 text-center">
          <div
            className="w-20 h-20 rounded-2xl flex items-center justify-center mx-auto mb-6"
            style={{ backgroundColor: `${config.color}1a` }}
          >
            <Icon
              className={`w-10 h-10 ${config.spin ? 'animate-spin' : ''}`}
              style={{ color: config.color }}
            />
          </div>
          <h2 className="text-xl font-bold mb-3" style={{ color: 'var(--foreground)' }}>
            {config.title}
          </h2>
          <p className="text-sm mb-8" style={{ color: 'var(--muted-foreground)' }}>
            {config.desc}
          </p>

          {status === 'success' && (
            <Link to="/dashboard">
              <button
                className="w-full h-11 rounded-lg text-base font-medium text-white shadow-sm border-0 transition-opacity"
                style={{ background: 'var(--primary)' }}
                onMouseEnter={e => (e.currentTarget.style.opacity = '0.9')}
                onMouseLeave={e => (e.currentTarget.style.opacity = '1')}
              >
                进入控制台
              </button>
            </Link>
          )}
          {(status === 'error' || status === 'no-token') && (
            <Link to="/login">
              <button
                className="w-full h-11 rounded-lg text-base font-medium border transition-colors"
                style={{ borderColor: 'var(--border)', color: 'var(--foreground)', background: 'transparent' }}
                onMouseEnter={e => (e.currentTarget.style.background = 'var(--muted)')}
                onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
              >
                返回登录
              </button>
            </Link>
          )}
        </div>
      </div>
    </div>
  )
}
