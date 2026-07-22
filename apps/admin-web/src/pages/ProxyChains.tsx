import { useState, useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { Plus, Trash2, GitBranch, ArrowRight, Power, PowerOff } from 'lucide-react'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Button,
  Input,
  Label,
  Badge,
  Skeleton,
  EmptyState,
  Separator,
  Switch,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  useToast,
} from '@airport/ui'
import { api } from '@/lib/api'
import { EP } from '@/lib/endpoints'

// ===== 类型定义 =====

interface ProxyChain {
  id: string
  name: string
  description?: string
  enabled: boolean
  hops?: number
  nodes?: string[]
  created_at?: string
  createdAt?: string
}

interface ChainFormData {
  name: string
  description: string
  enabled: boolean
}

const DEFAULT_FORM: ChainFormData = {
  name: '',
  description: '',
  enabled: true,
}

// ===== 工具函数 =====

function extractList<T>(resp: unknown): T[] {
  if (Array.isArray(resp)) return resp as T[]
  if (!resp || typeof resp !== 'object') return []
  const obj = resp as Record<string, unknown>
  // 处理嵌套结构：{code:0, data:{items:[...]}}
  const dataField = obj.data
  if (dataField && typeof dataField === 'object') {
    if (Array.isArray(dataField)) return dataField as T[]
    const dataObj = dataField as Record<string, unknown>
    if (Array.isArray(dataObj.items)) return dataObj.items as T[]
    if (Array.isArray(dataObj.list)) return dataObj.list as T[]
  }
  if (Array.isArray(obj.items)) return obj.items as T[]
  if (Array.isArray(obj.list)) return obj.list as T[]
  if (Array.isArray(obj.data)) return obj.data as T[]
  return []
}

function getCreatedAt(chain: ProxyChain): string {
  return chain.created_at || chain.createdAt || ''
}

// ===== 主组件 =====

export default function ProxyChains() {
  const { toast } = useToast()
  const [loading, setLoading] = useState(true)
  const [chains, setChains] = useState<ProxyChain[]>([])
  const [dialogOpen, setDialogOpen] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  const {
    register,
    handleSubmit,
    reset,
    watch,
    setValue,
    formState: { errors },
  } = useForm<ChainFormData>({
    defaultValues: DEFAULT_FORM,
  })

  const watchEnabled = watch('enabled')

  // ===== 数据加载 =====

  const loadData = async () => {
    setLoading(true)
    try {
      const resp = await api.get<unknown>(EP.PROXY_CHAINS)
      setChains(extractList<ProxyChain>(resp))
    } catch (err) {
      toast({
        title: '加载失败',
        description: err instanceof Error ? err.message : '无法获取代理链列表',
        variant: 'destructive',
      })
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadData()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // ===== 启用/禁用 =====

  const toggleChain = async (chain: ProxyChain) => {
    try {
      await api.patch(EP.PROXY_CHAIN(chain.id), { enabled: !chain.enabled })
      toast({
        title: '更新成功',
        description: `代理链 ${chain.name} 已${chain.enabled ? '禁用' : '启用'}`,
        variant: 'success',
      })
      await loadData()
    } catch (err) {
      toast({
        title: '更新失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    }
  }

  // ===== 删除 =====

  const deleteChain = async (chain: ProxyChain) => {
    if (!window.confirm(`确定删除代理链「${chain.name}」吗？此操作不可恢复。`)) return
    try {
      await api.delete(EP.PROXY_CHAIN(chain.id))
      toast({ title: '删除成功', description: `代理链 ${chain.name} 已删除`, variant: 'success' })
      await loadData()
    } catch (err) {
      toast({
        title: '删除失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    }
  }

  // ===== 表单操作 =====

  const openCreate = () => {
    reset(DEFAULT_FORM)
    setDialogOpen(true)
  }

  const onSubmit = async (data: ChainFormData) => {
    if (!data.name.trim()) {
      toast({ title: '校验失败', description: '请填写代理链名称', variant: 'destructive' })
      return
    }

    const payload = {
      name: data.name.trim(),
      description: data.description.trim(),
      enabled: data.enabled,
    }

    setSubmitting(true)
    try {
      await api.post(EP.PROXY_CHAINS, payload)
      toast({ title: '创建成功', description: `代理链 ${payload.name} 已创建`, variant: 'success' })
      setDialogOpen(false)
      await loadData()
    } catch (err) {
      toast({
        title: '创建失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setSubmitting(false)
    }
  }

  const getNodeName = (nodeId: string) => nodeId

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-zinc-100">代理链管理</h2>
        <Button size="sm" className="bg-indigo-600 hover:bg-indigo-500" onClick={openCreate}>
          <Plus className="w-4 h-4 mr-1" />
          创建代理链
        </Button>
      </div>

      {loading ? (
        <div className="space-y-3">
          {[1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-32 w-full bg-zinc-800 rounded-lg" />
          ))}
        </div>
      ) : chains.length === 0 ? (
        <Card className="bg-zinc-900 border-zinc-800">
          <CardContent>
            <EmptyState
              title="暂无代理链"
              description="创建代理链来配置多跳转发"
              className="py-12"
            />
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-3">
          {chains.map((chain) => {
            const nodes = chain.nodes || []
            const hops = chain.hops ?? nodes.length
            const createdAt = getCreatedAt(chain)
            return (
              <Card key={chain.id} className="bg-zinc-900 border-zinc-800">
                <CardHeader className="pb-3">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3">
                      <div className={`p-2 rounded-lg ${chain.enabled ? 'bg-indigo-600/20' : 'bg-zinc-800'}`}>
                        <GitBranch className={`w-4 h-4 ${chain.enabled ? 'text-indigo-400' : 'text-zinc-500'}`} />
                      </div>
                      <div>
                        <CardTitle className="text-sm font-medium text-zinc-100 flex items-center gap-2">
                          {chain.name}
                          <Badge
                            variant={chain.enabled ? 'success' : 'secondary'}
                            className={chain.enabled ? '' : 'bg-zinc-800 text-zinc-500'}
                          >
                            {chain.enabled ? '启用' : '禁用'}
                          </Badge>
                        </CardTitle>
                        <p className="text-xs text-zinc-500 mt-0.5">
                          {hops} 跳{createdAt ? ` · 创建于 ${new Date(createdAt).toLocaleDateString('zh-CN')}` : ''}
                        </p>
                        {chain.description && (
                          <p className="text-xs text-zinc-400 mt-1 line-clamp-2">{chain.description}</p>
                        )}
                      </div>
                    </div>
                    <div className="flex items-center gap-1">
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8 text-zinc-400 hover:text-zinc-200"
                        onClick={() => toggleChain(chain)}
                      >
                        {chain.enabled ? (
                          <Power className="w-4 h-4 text-emerald-400" />
                        ) : (
                          <PowerOff className="w-4 h-4" />
                        )}
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8 text-zinc-400 hover:text-red-400"
                        onClick={() => deleteChain(chain)}
                      >
                        <Trash2 className="w-4 h-4" />
                      </Button>
                    </div>
                  </div>
                </CardHeader>
                <CardContent className="pt-0">
                  <Separator className="bg-zinc-800 mb-3" />
                  <div className="flex items-center flex-wrap gap-2">
                    {nodes.map((nodeId, idx) => (
                      <div key={`${nodeId}-${idx}`} className="flex items-center gap-2">
                        <div className="px-3 py-1.5 rounded-md bg-zinc-800 text-sm text-zinc-300 border border-zinc-700">
                          {getNodeName(nodeId)}
                        </div>
                        {idx < nodes.length - 1 && (
                          <ArrowRight className="w-4 h-4 text-zinc-600" />
                        )}
                      </div>
                    ))}
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-8 px-2 text-zinc-500 border border-dashed border-zinc-700 hover:border-zinc-600 hover:text-zinc-300"
                    >
                      <Plus className="w-3 h-3 mr-1" />
                      添加跳点
                    </Button>
                  </div>
                </CardContent>
              </Card>
            )
          })}
        </div>
      )}

      {/* 创建 Dialog */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-lg max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>创建代理链</DialogTitle>
            <DialogDescription>创建一个新的代理链用于多跳转发</DialogDescription>
          </DialogHeader>

          <form onSubmit={handleSubmit(onSubmit)} className="space-y-4 pt-2">
            <div className="space-y-2">
              <Label htmlFor="name" className="text-zinc-300">
                链名称 <span className="text-red-400">*</span>
              </Label>
              <Input
                id="name"
                placeholder="如：HK->JP->US"
                className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500 focus:border-indigo-500"
                {...register('name', { required: '请输入代理链名称' })}
              />
              {errors.name && <p className="text-sm text-red-400">{errors.name.message}</p>}
            </div>

            <div className="space-y-2">
              <Label htmlFor="description" className="text-zinc-300">
                描述
              </Label>
              <Input
                id="description"
                placeholder="代理链描述（可选）"
                className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500 focus:border-indigo-500"
                {...register('description')}
              />
            </div>

            <div className="flex items-center justify-between py-2 px-3 rounded-lg bg-zinc-800/50 border border-zinc-800">
              <div>
                <Label className="text-zinc-300 cursor-pointer">启用状态</Label>
                <p className="text-xs text-zinc-500 mt-0.5">禁用后代理链不会生效</p>
              </div>
              <Switch
                checked={!!watchEnabled}
                onChange={(e) => setValue('enabled', e.target.checked)}
              />
            </div>

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setDialogOpen(false)}
                disabled={submitting}
                className="border-zinc-700 text-zinc-300"
              >
                取消
              </Button>
              <Button
                type="submit"
                isLoading={submitting}
                className="bg-indigo-600 hover:bg-indigo-500"
              >
                创建
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}
