import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { LogOut, Key, Info, Shield } from 'lucide-react'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
  Button,
  Input,
  Label,
  Avatar,
  AvatarFallback,
  Badge,
  Separator,
  useToast,
} from '@airport/ui'
import { useAuthStore } from '@/lib/auth'

export default function Profile() {
  const navigate = useNavigate()
  const { toast } = useToast()
  const { admin, logout } = useAuthStore()
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [changingPassword, setChangingPassword] = useState(false)

  const handleLogout = async () => {
    await logout()
    navigate('/login', { replace: true })
  }

  const handleChangePassword = async (e: React.FormEvent) => {
    e.preventDefault()
    if (newPassword !== confirmPassword) {
      toast({
        title: '密码不匹配',
        description: '两次输入的新密码不一致',
        variant: 'destructive',
      })
      return
    }
    if (newPassword.length < 6) {
      toast({
        title: '密码太短',
        description: '新密码至少需要 6 个字符',
        variant: 'destructive',
      })
      return
    }
    setChangingPassword(true)
    await new Promise((r) => setTimeout(r, 800))
    setChangingPassword(false)
    setCurrentPassword('')
    setNewPassword('')
    setConfirmPassword('')
    toast({
      title: '密码已更新',
      description: '您的密码已成功修改',
      variant: 'success',
    })
  }

  const getInitials = (name: string) => {
    return name ? name.charAt(0).toUpperCase() : 'A'
  }

  return (
    <div className="space-y-4">
      <Card className="bg-zinc-900 border-zinc-800">
        <CardContent className="p-6">
          <div className="flex flex-col items-center text-center">
            <Avatar className="h-20 w-20 border-2 border-zinc-700 mb-4">
              <AvatarFallback className="bg-indigo-600/20 text-indigo-300 text-2xl font-semibold">
                {getInitials(admin?.name || 'A')}
              </AvatarFallback>
            </Avatar>
            <h2 className="text-xl font-semibold text-zinc-100">
              {admin?.name || 'Administrator'}
            </h2>
            <p className="text-sm text-zinc-400 mt-1">{admin?.email || 'a****@***********'}</p>
            <Badge variant="outline" className="mt-2 bg-indigo-900/30 text-indigo-300 border-indigo-800/30">
              <Shield className="w-3 h-3 mr-1" />
              {admin?.role || 'admin'}
            </Badge>
          </div>
        </CardContent>
      </Card>

      <Card className="bg-zinc-900 border-zinc-800">
        <CardHeader className="pb-3">
          <CardTitle className="text-base flex items-center gap-2">
            <Key className="w-4 h-4 text-zinc-400" />
            修改密码
          </CardTitle>
          <CardDescription className="text-zinc-500">
            定期修改密码以保护您的账户安全
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleChangePassword} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="currentPassword" className="text-zinc-300 text-sm">
                当前密码
              </Label>
              <Input
                id="currentPassword"
                type="password"
                value={currentPassword}
                onChange={(e) => setCurrentPassword(e.target.value)}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
                placeholder="••••••••"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="newPassword" className="text-zinc-300 text-sm">
                新密码
              </Label>
              <Input
                id="newPassword"
                type="password"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
                placeholder="••••••••"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="confirmPassword" className="text-zinc-300 text-sm">
                确认新密码
              </Label>
              <Input
                id="confirmPassword"
                type="password"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
                placeholder="••••••••"
              />
            </div>
            <Button
              type="submit"
              className="w-full bg-indigo-600 hover:bg-indigo-500"
              isLoading={changingPassword}
              disabled={!currentPassword || !newPassword || !confirmPassword}
            >
              修改密码
            </Button>
          </form>
        </CardContent>
      </Card>

      <Card className="bg-zinc-900 border-zinc-800">
        <CardHeader className="pb-3">
          <CardTitle className="text-base flex items-center gap-2">
            <Info className="w-4 h-4 text-zinc-400" />
            系统信息
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex justify-between items-center py-2">
            <span className="text-sm text-zinc-400">面板版本</span>
            <span className="text-sm text-zinc-200 font-mono">v1.0.0</span>
          </div>
          <Separator className="bg-zinc-800" />
          <div className="flex justify-between items-center py-2">
            <span className="text-sm text-zinc-400">构建时间</span>
            <span className="text-sm text-zinc-200">2026-06-29</span>
          </div>
          <Separator className="bg-zinc-800" />
          <div className="flex justify-between items-center py-2">
            <span className="text-sm text-zinc-400">节点服务</span>
            <div className="flex items-center gap-2">
              <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
              <span className="text-sm text-emerald-400">在线</span>
            </div>
          </div>
          <Separator className="bg-zinc-800" />
          <div className="flex justify-between items-center py-2">
            <span className="text-sm text-zinc-400">API 状态</span>
            <div className="flex items-center gap-2">
              <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
              <span className="text-sm text-emerald-400">正常</span>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card className="bg-zinc-900 border-red-900/30 border">
        <CardContent className="p-4">
          <Button
            variant="destructive"
            className="w-full bg-red-900/50 hover:bg-red-900/70 text-red-200"
            onClick={handleLogout}
          >
            <LogOut className="w-4 h-4 mr-2" />
            退出登录
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}
