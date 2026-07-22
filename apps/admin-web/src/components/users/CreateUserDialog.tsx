import { useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  Button,
  Input,
  Label,
  Select,
  useToast,
} from '@airport/ui'
import { useCreateUser } from '@/lib/hooks'

interface CreateUserDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess?: () => void
}

interface FormData {
  email: string
  password: string
  plan_id: string
}

const DEFAULT_FORM: FormData = {
  email: '',
  password: '',
  plan_id: '',
}

function generatePassword(): string {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!@#$%^&*'
  let result = ''
  for (let i = 0; i < 12; i++) {
    result += chars.charAt(Math.floor(Math.random() * chars.length))
  }
  return result
}

export function CreateUserDialog({ open, onOpenChange, onSuccess }: CreateUserDialogProps) {
  const { toast } = useToast()
  const [form, setForm] = useState<FormData>({ ...DEFAULT_FORM })
  const [errors, setErrors] = useState<Partial<Record<keyof FormData, string>>>({})

  const createMutation = useCreateUser({
    onSuccess: () => {
      toast({ title: '创建成功', description: `用户 ${form.email} 已创建`, variant: 'success' })
      reset()
      onOpenChange(false)
      onSuccess?.()
    },
    onError: (err) => {
      toast({ title: '创建失败', description: err.message, variant: 'destructive' })
    },
  })

  function reset() {
    setForm({ ...DEFAULT_FORM })
    setErrors({})
  }

  function updateField<K extends keyof FormData>(key: K, value: FormData[K]) {
    setForm((prev) => ({ ...prev, [key]: value }))
    if (errors[key]) {
      setErrors((prev) => ({ ...prev, [key]: undefined }))
    }
  }

  function validate(): boolean {
    const newErrors: Partial<Record<keyof FormData, string>> = {}
    if (!form.email.trim()) newErrors.email = '请输入邮箱'
    else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(form.email.trim())) newErrors.email = '请输入有效的邮箱地址'
    if (!form.password || form.password.length < 6) newErrors.password = '密码至少 6 位'
    setErrors(newErrors)
    return Object.keys(newErrors).length === 0
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!validate()) return

    const payload: Record<string, unknown> = {
      email: form.email.trim(),
      password: form.password,
    }
    if (form.plan_id.trim()) {
      payload.plan_id = form.plan_id.trim()
    }
    createMutation.mutate(payload)
  }

  function handleOpenChange(v: boolean) {
    if (!v) reset()
    onOpenChange(v)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-md">
        <DialogHeader>
          <DialogTitle>添加用户</DialogTitle>
          <DialogDescription className="text-zinc-400">创建一个新的用户账户</DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4 pt-2">
          <div className="space-y-1.5">
            <Label htmlFor="email" className="text-zinc-300 text-sm">
              邮箱 <span className="text-red-400">*</span>
            </Label>
            <Input
              id="email"
              type="email"
              placeholder="user@example.com"
              value={form.email}
              onChange={(e) => updateField('email', e.target.value)}
              className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500 h-9"
            />
            {errors.email && <p className="text-xs text-red-400">{errors.email}</p>}
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="password" className="text-zinc-300 text-sm">
              密码 <span className="text-red-400">*</span>
            </Label>
            <div className="flex gap-2">
              <Input
                id="password"
                type="text"
                placeholder="至少 6 位"
                value={form.password}
                onChange={(e) => updateField('password', e.target.value)}
                className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500 h-9 flex-1"
              />
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="h-9 border-zinc-700 text-zinc-300 whitespace-nowrap"
                onClick={() => updateField('password', generatePassword())}
              >
                随机生成
              </Button>
            </div>
            {errors.password && <p className="text-xs text-red-400">{errors.password}</p>}
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="plan_id" className="text-zinc-300 text-sm">套餐</Label>
            <Input
              id="plan_id"
              placeholder="套餐 ID（可选）"
              value={form.plan_id}
              onChange={(e) => updateField('plan_id', e.target.value)}
              className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500 h-9"
            />
            <p className="text-xs text-zinc-500">可选，关联已有套餐的 ID</p>
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => handleOpenChange(false)}
              disabled={createMutation.isPending}
              className="border-zinc-700 text-zinc-300"
            >
              取消
            </Button>
            <Button
              type="submit"
              isLoading={createMutation.isPending}
              className="bg-indigo-600 hover:bg-indigo-500"
            >
              创建
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
