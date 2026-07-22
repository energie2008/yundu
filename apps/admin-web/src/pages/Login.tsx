import { useEffect } from 'react'
import { useNavigate, Navigate } from 'react-router-dom'
import { useForm } from 'react-hook-form'
import { Shield } from 'lucide-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Button,
  Input,
  Label,
  useToast,
} from '@airport/ui'
import { useAuthStore } from '@/lib/auth'

interface LoginFormData {
  email: string
  password: string
}

export default function Login() {
  const navigate = useNavigate()
  const { toast } = useToast()
  const { login, isAuthenticated, isLoading, init } = useAuthStore()

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
      <div className="min-h-screen flex items-center justify-center bg-zinc-950">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-indigo-500" />
      </div>
    )
  }

  if (isAuthenticated) {
    return <Navigate to="/dashboard" replace />
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-zinc-950 p-4">
      <Card className="w-full max-w-md bg-zinc-900 border-zinc-800">
        <CardHeader className="text-center pb-2">
          <div className="mx-auto w-12 h-12 rounded-xl bg-indigo-600/20 flex items-center justify-center mb-4">
            <Shield className="w-6 h-6 text-indigo-400" />
          </div>
          <CardTitle className="text-xl text-zinc-100">Airport Panel</CardTitle>
          <CardDescription className="text-zinc-400">
            管理后台登录
          </CardDescription>
        </CardHeader>
        <CardContent className="pt-4">
          <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="email" className="text-zinc-300">
                邮箱
              </Label>
              <Input
                id="email"
                type="email"
                placeholder="a****@***********"
                className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500 focus:border-indigo-500"
                {...register('email', {
                  required: '请输入邮箱',
                })}
              />
              {errors.email && (
                <p className="text-sm text-red-400">{errors.email.message}</p>
              )}
            </div>

            <div className="space-y-2">
              <Label htmlFor="password" className="text-zinc-300">
                密码
              </Label>
              <Input
                id="password"
                type="password"
                placeholder="••••••••"
                className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500 focus:border-indigo-500"
                {...register('password', {
                  required: '请输入密码',
                })}
              />
              {errors.password && (
                <p className="text-sm text-red-400">{errors.password.message}</p>
              )}
            </div>

            <Button
              type="submit"
              className="w-full bg-indigo-600 hover:bg-indigo-500 text-white"
              isLoading={isSubmitting}
            >
              登录
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
