import { useState, useEffect, useMemo, useCallback } from 'react'
import {
  Plus,
  Pencil,
  Trash2,
  Layers,
  Search,
  HardDrive,
  Users,
  Check,
  RefreshCw,
} from 'lucide-react'
import {
  Card,
  CardContent,
  Button,
  Input,
  Label,
  Skeleton,
  Badge,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  useToast,
  Separator,
} from '@airport/ui'
import { api } from '@/lib/api'
import { EP } from '@/lib/endpoints'

// ===== 类型定义 =====
interface NodeGroup {
  id: string
  code: string
  name: string
  description?: string
  visibility: string
  sort_order: number
  node_count?: number
  created_at?: string
  updated_at?: string
}

interface NodeItem {
  id: string
  code: string
  name: string
  protocol_type: string
  transport_type: string
  address: string
  port: number
  is_enabled: boolean
  is_visible: boolean
  group_id?: string | null
  // 多对多分组：节点所属的所有分组 ID 列表
  group_ids?: string[] | null
}

// ===== 工具函数 =====
function extractList<T>(resp: unknown, fallbackKey: string = 'items'): T[] {
  if (Array.isArray(resp)) return resp as T[]
  if (!resp || typeof resp !== 'object') return []
  const obj = resp as Record<string, unknown>
  if (Array.isArray(obj[fallbackKey])) return obj[fallbackKey] as T[]
  if (Array.isArray(obj.data)) return obj.data as T[]
  return []
}

const VISIBILITY_OPTIONS = [
  { value: 'public', label: '公开', color: 'bg-emerald-500/20 text-emerald-400 border-emerald-500/30' },
  { value: 'private', label: '私有', color: 'bg-amber-500/20 text-amber-400 border-amber-500/30' },
  { value: 'hidden', label: '隐藏', color: 'bg-zinc-500/20 text-zinc-400 border-zinc-500/30' },
]

const PROTOCOL_COLORS: Record<string, string> = {
  vless: 'bg-emerald-500/20 text-emerald-400 border-emerald-500/30',
  vmess: 'bg-blue-500/20 text-blue-400 border-blue-500/30',
  trojan: 'bg-purple-500/20 text-purple-400 border-purple-500/30',
  ss: 'bg-amber-500/20 text-amber-400 border-amber-500/30',
  shadowsocks: 'bg-amber-500/20 text-amber-400 border-amber-500/30',
  hysteria2: 'bg-rose-500/20 text-rose-400 border-rose-500/30',
  tuic: 'bg-cyan-500/20 text-cyan-400 border-cyan-500/30',
}

function protocolBadge(protocol: string): string {
  return PROTOCOL_COLORS[protocol.toLowerCase()] || 'bg-zinc-500/20 text-zinc-400 border-zinc-500/30'
}

export default function NodeGroups() {
  const { toast } = useToast()
  const [loading, setLoading] = useState(true)
  const [groups, setGroups] = useState<NodeGroup[]>([])
  const [search, setSearch] = useState('')

  // 分组编辑对话框
  const [editDialogOpen, setEditDialogOpen] = useState(false)
  const [editingGroup, setEditingGroup] = useState<NodeGroup | null>(null)
  const [saving, setSaving] = useState(false)
  const [formCode, setFormCode] = useState('')
  const [formName, setFormName] = useState('')
  const [formDesc, setFormDesc] = useState('')
  const [formVisibility, setFormVisibility] = useState('public')
  const [formSortOrder, setFormSortOrder] = useState(0)

  // 节点管理对话框
  const [nodesDialogOpen, setNodesDialogOpen] = useState(false)
  const [managingGroup, setManagingGroup] = useState<NodeGroup | null>(null)
  const [allNodes, setAllNodes] = useState<NodeItem[]>([])
  const [nodesLoading, setNodesLoading] = useState(false)
  const [selectedBoundIds, setSelectedBoundIds] = useState<Set<string>>(new Set())
  const [selectedUnboundIds, setSelectedUnboundIds] = useState<Set<string>>(new Set())
  const [nodeSearch, setNodeSearch] = useState('')

  // ===== 加载分组列表 =====
  const loadGroups = useCallback(async () => {
    setLoading(true)
    try {
      const data = await api.get<unknown>(EP.NODE_GROUPS, { params: { page: 1, page_size: 100 } })
      const list = extractList<NodeGroup>(data)
      setGroups(list)
    } catch (err) {
      toast({
        title: '加载失败',
        description: err instanceof Error ? err.message : '无法获取分组列表',
        variant: 'destructive',
      })
      setGroups([])
    } finally {
      setLoading(false)
    }
  }, [toast])

  useEffect(() => {
    loadGroups()
  }, [loadGroups])

  const filteredGroups = useMemo(() => {
    const kw = search.trim().toLowerCase()
    if (!kw) return groups
    return groups.filter(g =>
      g.name.toLowerCase().includes(kw) ||
      g.code.toLowerCase().includes(kw) ||
      (g.description || '').toLowerCase().includes(kw)
    )
  }, [groups, search])

  // ===== 分组 CRUD =====
  const openCreate = useCallback(() => {
    setEditingGroup(null)
    setFormCode('')
    setFormName('')
    setFormDesc('')
    setFormVisibility('public')
    setFormSortOrder(0)
    setEditDialogOpen(true)
  }, [])

  const openEdit = useCallback((g: NodeGroup) => {
    setEditingGroup(g)
    setFormCode(g.code)
    setFormName(g.name)
    setFormDesc(g.description || '')
    setFormVisibility(g.visibility || 'public')
    setFormSortOrder(g.sort_order || 0)
    setEditDialogOpen(true)
  }, [])

  const handleSaveGroup = useCallback(async () => {
    if (!formName.trim()) {
      toast({ title: '校验失败', description: '请输入分组名称', variant: 'destructive' })
      return
    }
    if (!formCode.trim()) {
      toast({ title: '校验失败', description: '请输入分组编码', variant: 'destructive' })
      return
    }
    if (!/^[a-zA-Z0-9_-]{2,64}$/.test(formCode.trim())) {
      toast({ title: '校验失败', description: '编码只能包含字母、数字、下划线、连字符（2-64字符）', variant: 'destructive' })
      return
    }
    setSaving(true)
    try {
      const payload = {
        code: formCode.trim(),
        name: formName.trim(),
        description: formDesc.trim() || undefined,
        visibility: formVisibility,
        sort_order: formSortOrder,
      }
      if (editingGroup) {
        await api.patch(EP.NODE_GROUP_DETAIL(editingGroup.id), payload)
        toast({ title: '分组已更新', variant: 'success' })
      } else {
        await api.post(EP.NODE_GROUPS, payload)
        toast({ title: '分组已创建', variant: 'success' })
      }
      setEditDialogOpen(false)
      await loadGroups()
    } catch (err) {
      toast({
        title: '保存失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setSaving(false)
    }
  }, [formCode, formName, formDesc, formVisibility, formSortOrder, editingGroup, toast, loadGroups])

  const handleDelete = useCallback(async (g: NodeGroup) => {
    if (!window.confirm(`确定删除分组「${g.name}」吗？此操作不可撤销。\n\n如果该分组下还有节点，请先在「管理节点」中移除所有节点。`)) return
    try {
      await api.delete(EP.NODE_GROUP_DETAIL(g.id))
      toast({ title: '分组已删除', variant: 'success' })
      await loadGroups()
    } catch (err) {
      toast({
        title: '删除失败',
        description: err instanceof Error ? err.message : '可能仍有节点关联此分组，请先移除节点',
        variant: 'destructive',
      })
    }
  }, [toast, loadGroups])

  // ===== 节点管理 =====
  const openManageNodes = useCallback(async (g: NodeGroup) => {
    setManagingGroup(g)
    setNodesDialogOpen(true)
    setNodeSearch('')
    setSelectedBoundIds(new Set())
    setSelectedUnboundIds(new Set())
    setNodesLoading(true)
    try {
      const data = await api.get<{ items: NodeItem[] } | NodeItem[]>(EP.NODES, {
        params: { page: 1, page_size: 500 },
      })
      const list = extractList<NodeItem>(data)
      setAllNodes(list)
    } catch (err) {
      toast({
        title: '加载节点失败',
        description: err instanceof Error ? err.message : '无法获取节点列表',
        variant: 'destructive',
      })
      setAllNodes([])
    } finally {
      setNodesLoading(false)
    }
  }, [toast])

  // 判断节点是否属于指定分组（多对多：优先用 group_ids 数组，回退 group_id 单值）
  const isNodeInGroup = (n: NodeItem, groupId: string): boolean => {
    if (Array.isArray(n.group_ids) && n.group_ids.length > 0) {
      return n.group_ids.includes(groupId)
    }
    return n.group_id === groupId
  }

  const boundNodes = useMemo(() => {
    if (!managingGroup) return []
    return allNodes.filter(n => isNodeInGroup(n, managingGroup.id))
  }, [allNodes, managingGroup])

  const unboundNodes = useMemo(() => {
    if (!managingGroup) return []
    const kw = nodeSearch.trim().toLowerCase()
    return allNodes.filter(n => {
      if (isNodeInGroup(n, managingGroup.id)) return false
      if (!kw) return true
      return n.name.toLowerCase().includes(kw) ||
        n.code.toLowerCase().includes(kw) ||
        (n.address || '').toLowerCase().includes(kw)
    })
  }, [allNodes, managingGroup, nodeSearch])

  const toggleBoundSelect = useCallback((id: string) => {
    setSelectedBoundIds(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }, [])

  const toggleUnboundSelect = useCallback((id: string) => {
    setSelectedUnboundIds(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }, [])

  const handleBindNodes = useCallback(async () => {
    if (!managingGroup || selectedUnboundIds.size === 0) return
    try {
      const ids = Array.from(selectedUnboundIds)
      const resp = await api.post<{ bound: number }>(EP.NODE_GROUP_BIND_NODES(managingGroup.id), { node_ids: ids })
      toast({
        title: '节点已添加',
        description: `成功绑定 ${resp.bound || ids.length} 个节点到「${managingGroup.name}」`,
        variant: 'success',
      })
      // 更新本地状态：将绑定的节点加入当前分组的 group_ids 数组（多对多，不覆盖其它分组）
      setAllNodes(prev => prev.map(n => {
        if (!ids.includes(n.id)) return n
        const existing = Array.isArray(n.group_ids) ? [...n.group_ids] : (n.group_id ? [n.group_id] : [])
        if (!existing.includes(managingGroup.id)) existing.push(managingGroup.id)
        return { ...n, group_ids: existing, group_id: n.group_id || managingGroup.id }
      }))
      setSelectedUnboundIds(new Set())
      // 同步分组列表的 node_count
      await loadGroups()
    } catch (err) {
      toast({
        title: '绑定失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    }
  }, [managingGroup, selectedUnboundIds, toast, loadGroups])

  const handleUnbindNodes = useCallback(async () => {
    if (!managingGroup || selectedBoundIds.size === 0) return
    if (!window.confirm(`确定从「${managingGroup.name}」移除选中的 ${selectedBoundIds.size} 个节点吗？`)) return
    try {
      const ids = Array.from(selectedBoundIds)
      const resp = await api.post<{ unbound: number }>(EP.NODE_GROUP_UNBIND_NODES(managingGroup.id), { node_ids: ids })
      toast({
        title: '节点已移除',
        description: `成功解绑 ${resp.unbound || ids.length} 个节点`,
        variant: 'success',
      })
      // 更新本地状态：从解绑节点的 group_ids 数组中移除当前分组（多对多，不影响其它分组）
      setAllNodes(prev => prev.map(n => {
        if (!ids.includes(n.id)) return n
        const existing = Array.isArray(n.group_ids) ? [...n.group_ids] : (n.group_id ? [n.group_id] : [])
        const updated = existing.filter(gid => gid !== managingGroup.id)
        return { ...n, group_ids: updated, group_id: updated[0] || null }
      }))
      setSelectedBoundIds(new Set())
      await loadGroups()
    } catch (err) {
      toast({
        title: '解绑失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    }
  }, [managingGroup, selectedBoundIds, toast, loadGroups])

  // ===== 渲染 =====
  return (
    <div className="space-y-4 pb-20 sm:pb-4">
      {/* 顶部工具栏 */}
      <div className="flex items-center justify-between flex-wrap gap-2">
        <div>
          <h2 className="text-lg font-semibold text-zinc-100 flex items-center gap-2">
            <Layers className="w-5 h-5 text-indigo-400" />
            会员分组管理
          </h2>
          <p className="text-xs text-zinc-500 mt-0.5">
            管理节点分组（如免费测速组、VIP1组、VIP2组），每组可绑定不同节点，对应不同套餐可见性
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={loadGroups} className="border-zinc-700 text-zinc-300">
            <RefreshCw className="w-4 h-4 mr-1" />刷新
          </Button>
          <Button size="sm" className="bg-indigo-600 hover:bg-indigo-500" onClick={openCreate}>
            <Plus className="w-4 h-4 mr-1" />添加分组
          </Button>
        </div>
      </div>

      {/* 搜索栏 */}
      <Card className="bg-zinc-900 border-zinc-800">
        <CardContent className="p-3">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-zinc-500" />
            <Input
              placeholder="搜索分组名称、编码或描述..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="pl-9 bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500 h-9"
            />
          </div>
        </CardContent>
      </Card>

      {/* 分组列表 */}
      {loading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
          {[1, 2, 3, 4, 5, 6].map(i => (
            <Skeleton key={i} className="h-40 w-full bg-zinc-800 rounded-lg" />
          ))}
        </div>
      ) : filteredGroups.length === 0 ? (
        <Card className="bg-zinc-900 border-zinc-800">
          <CardContent className="py-16 text-center">
            <Layers className="w-12 h-12 mx-auto text-zinc-700 mb-3" />
            <p className="text-zinc-400 text-sm">{search ? '没有匹配的分组' : '暂无分组'}</p>
            <p className="text-zinc-600 text-xs mt-1">
              {search ? '尝试其他关键词' : '点击「添加分组」创建第一个会员分组'}
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
          {filteredGroups.map(g => {
            const visOpt = VISIBILITY_OPTIONS.find(v => v.value === g.visibility) || VISIBILITY_OPTIONS[0]
            return (
              <Card key={g.id} className="bg-zinc-900 border-zinc-800 hover:border-zinc-700 transition-colors">
                <CardContent className="p-4 space-y-3">
                  {/* 头部：图标+名称+可见性 */}
                  <div className="flex items-start justify-between gap-2">
                    <div className="flex items-center gap-2 min-w-0 flex-1">
                      <div className="p-1.5 rounded-md bg-indigo-900/30 flex-shrink-0">
                        <Layers className="w-4 h-4 text-indigo-400" />
                      </div>
                      <div className="min-w-0 flex-1">
                        <div className="text-sm font-medium text-zinc-100 truncate">{g.name}</div>
                        <div className="text-[10px] text-zinc-500 font-mono">{g.code}</div>
                      </div>
                    </div>
                    <Badge variant="outline" className={`${visOpt.color} text-[10px] flex-shrink-0`}>
                      {visOpt.label}
                    </Badge>
                  </div>

                  {/* 描述 */}
                  {g.description && (
                    <p className="text-xs text-zinc-400 line-clamp-2">{g.description}</p>
                  )}

                  {/* 统计 */}
                  <div className="flex items-center gap-3 text-xs text-zinc-500">
                    <span className="flex items-center gap-1">
                      <HardDrive className="w-3.5 h-3.5" />
                      {g.node_count || 0} 个节点
                    </span>
                    <span className="flex items-center gap-1">
                      <Users className="w-3.5 h-3.5" />
                      排序: {g.sort_order || 0}
                    </span>
                  </div>

                  {/* 操作按钮 */}
                  <Separator className="bg-zinc-800" />
                  <div className="flex items-center gap-1">
                    <Button
                      variant="outline"
                      size="sm"
                      className="flex-1 h-8 border-zinc-700 text-zinc-300 hover:bg-zinc-800"
                      onClick={() => openManageNodes(g)}
                    >
                      <HardDrive className="w-3.5 h-3.5 mr-1" />管理节点
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8 text-zinc-400 hover:text-indigo-300"
                      onClick={() => openEdit(g)}
                      title="编辑"
                    >
                      <Pencil className="w-3.5 h-3.5" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8 text-zinc-400 hover:text-red-300"
                      onClick={() => handleDelete(g)}
                      title="删除"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </Button>
                  </div>
                </CardContent>
              </Card>
            )
          })}
        </div>
      )}

      {/* 编辑/创建分组对话框 */}
      <Dialog open={editDialogOpen} onOpenChange={setEditDialogOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Layers className="w-5 h-5 text-indigo-400" />
              <span>{editingGroup ? '编辑分组' : '添加分组'}</span>
            </DialogTitle>
            <DialogDescription className="text-zinc-400 text-xs">
              会员分组用于组织节点，对应不同套餐的可见性
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-3 pt-2">
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label className="text-zinc-300 text-sm">分组编码 *</Label>
                <Input
                  value={formCode}
                  onChange={(e) => setFormCode(e.target.value)}
                  placeholder="如: free / vip1 / vip2"
                  className="bg-zinc-800 border-zinc-700 text-zinc-100 h-9 font-mono text-xs"
                  disabled={!!editingGroup}
                  autoFocus
                />
                <p className="text-[10px] text-zinc-500">字母/数字/下划线/连字符，2-64字符</p>
              </div>
              <div className="space-y-1.5">
                <Label className="text-zinc-300 text-sm">分组名称 *</Label>
                <Input
                  value={formName}
                  onChange={(e) => setFormName(e.target.value)}
                  placeholder="如: 免费测速组"
                  className="bg-zinc-800 border-zinc-700 text-zinc-100 h-9"
                />
              </div>
            </div>

            <div className="space-y-1.5">
              <Label className="text-zinc-300 text-sm">描述</Label>
              <Input
                value={formDesc}
                onChange={(e) => setFormDesc(e.target.value)}
                placeholder="可选，分组说明"
                className="bg-zinc-800 border-zinc-700 text-zinc-100 h-9"
              />
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label className="text-zinc-300 text-sm">可见性</Label>
                <select
                  value={formVisibility}
                  onChange={(e) => setFormVisibility(e.target.value)}
                  className="flex h-9 w-full rounded-lg border border-zinc-800 bg-zinc-950 px-3 py-1 text-sm text-zinc-100 focus:outline-none focus:border-indigo-500"
                >
                  {VISIBILITY_OPTIONS.map(v => (
                    <option key={v.value} value={v.value} className="bg-zinc-800">{v.label}</option>
                  ))}
                </select>
              </div>
              <div className="space-y-1.5">
                <Label className="text-zinc-300 text-sm">排序权重</Label>
                <Input
                  type="number"
                  value={formSortOrder}
                  onChange={(e) => setFormSortOrder(Number(e.target.value) || 0)}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100 h-9"
                />
                <p className="text-[10px] text-zinc-500">数字越小越靠前</p>
              </div>
            </div>
          </div>

          <DialogFooter className="pt-2">
            <Button
              variant="outline"
              onClick={() => setEditDialogOpen(false)}
              className="border-zinc-700 text-zinc-300"
            >
              取消
            </Button>
            <Button
              className="bg-indigo-600 hover:bg-indigo-500"
              onClick={handleSaveGroup}
              disabled={saving}
              isLoading={saving}
            >
              {editingGroup ? '保存' : '创建'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 节点管理对话框 */}
      <Dialog open={nodesDialogOpen} onOpenChange={setNodesDialogOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-4xl max-h-[90vh] overflow-hidden flex flex-col">
          <DialogHeader className="flex-shrink-0">
            <DialogTitle className="flex items-center gap-2">
              <HardDrive className="w-5 h-5 text-indigo-400" />
              <span>管理节点 - {managingGroup?.name}</span>
            </DialogTitle>
            <DialogDescription className="text-zinc-400 text-xs">
              左侧为已绑定节点（可勾选后移除），右侧为可添加节点（可勾选后添加）
            </DialogDescription>
          </DialogHeader>

          <div className="flex-1 overflow-hidden grid grid-cols-2 gap-3 min-h-0">
            {/* 左侧：已绑定节点 */}
            <div className="flex flex-col min-h-0 rounded-lg border border-zinc-800 bg-zinc-950/30">
              <div className="flex items-center justify-between p-3 border-b border-zinc-800 flex-shrink-0">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium text-zinc-200">已绑定节点</span>
                  <Badge variant="outline" className="bg-emerald-950/40 text-emerald-400 border-emerald-800/50 text-[10px]">
                    {boundNodes.length}
                  </Badge>
                </div>
                {selectedBoundIds.size > 0 && (
                  <Button
                    size="sm"
                    variant="outline"
                    className="h-7 text-xs border-red-800 text-red-300 hover:bg-red-950/30"
                    onClick={handleUnbindNodes}
                  >
                    <Trash2 className="w-3 h-3 mr-1" />移除({selectedBoundIds.size})
                  </Button>
                )}
              </div>
              <div className="flex-1 overflow-y-auto p-2 space-y-1">
                {nodesLoading ? (
                  <div className="text-center py-8 text-xs text-zinc-500">加载中...</div>
                ) : boundNodes.length === 0 ? (
                  <div className="text-center py-8 text-xs text-zinc-500">
                    <HardDrive className="w-8 h-8 mx-auto mb-2 text-zinc-700" />
                    该分组暂无节点
                  </div>
                ) : (
                  boundNodes.map(n => {
                    const selected = selectedBoundIds.has(n.id)
                    return (
                      <button
                        key={n.id}
                        type="button"
                        onClick={() => toggleBoundSelect(n.id)}
                        className={`w-full flex items-center gap-2 p-2 rounded-md border transition-colors text-left ${selected ? 'bg-red-950/30 border-red-800/50' : 'bg-zinc-900/50 border-zinc-800 hover:border-zinc-700'}`}
                      >
                        <div className={`w-4 h-4 rounded border flex-shrink-0 flex items-center justify-center ${selected ? 'bg-red-600 border-red-500' : 'border-zinc-600'}`}>
                          {selected && <Check className="w-3 h-3 text-white" />}
                        </div>
                        <div className="flex-1 min-w-0">
                          <div className="text-xs text-zinc-200 truncate">{n.name}</div>
                          <div className="text-[10px] text-zinc-500 font-mono truncate">{n.code} · {n.address}:{n.port}</div>
                        </div>
                        <Badge variant="outline" className={`${protocolBadge(n.protocol_type)} text-[9px] flex-shrink-0`}>
                          {n.protocol_type}
                        </Badge>
                      </button>
                    )
                  })
                )}
              </div>
            </div>

            {/* 右侧：可添加节点 */}
            <div className="flex flex-col min-h-0 rounded-lg border border-zinc-800 bg-zinc-950/30">
              <div className="flex items-center justify-between p-3 border-b border-zinc-800 flex-shrink-0 gap-2">
                <div className="flex items-center gap-2 min-w-0">
                  <span className="text-sm font-medium text-zinc-200 flex-shrink-0">可添加节点</span>
                  <Badge variant="outline" className="bg-zinc-800 text-zinc-400 border-zinc-700 text-[10px]">
                    {unboundNodes.length}
                  </Badge>
                </div>
                {selectedUnboundIds.size > 0 && (
                  <Button
                    size="sm"
                    className="h-7 text-xs bg-indigo-600 hover:bg-indigo-500"
                    onClick={handleBindNodes}
                  >
                    <Plus className="w-3 h-3 mr-1" />添加({selectedUnboundIds.size})
                  </Button>
                )}
              </div>
              <div className="p-2 border-b border-zinc-800 flex-shrink-0">
                <div className="relative">
                  <Search className="absolute left-2 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-zinc-500" />
                  <Input
                    placeholder="搜索节点..."
                    value={nodeSearch}
                    onChange={(e) => setNodeSearch(e.target.value)}
                    className="pl-7 bg-zinc-800 border-zinc-700 text-zinc-100 h-8 text-xs"
                  />
                </div>
              </div>
              <div className="flex-1 overflow-y-auto p-2 space-y-1">
                {nodesLoading ? (
                  <div className="text-center py-8 text-xs text-zinc-500">加载中...</div>
                ) : unboundNodes.length === 0 ? (
                  <div className="text-center py-8 text-xs text-zinc-500">
                    <Check className="w-8 h-8 mx-auto mb-2 text-zinc-700" />
                    {nodeSearch ? '无匹配节点' : '所有节点已绑定'}
                  </div>
                ) : (
                  unboundNodes.map(n => {
                    const selected = selectedUnboundIds.has(n.id)
                    // 多对多：显示节点已属于的其它分组（排除当前管理分组）
                    const mgId = managingGroup?.id
                    const nodeGroupIds = Array.isArray(n.group_ids) && n.group_ids.length > 0
                      ? n.group_ids
                      : (n.group_id ? [n.group_id] : [])
                    const otherGroupNames = mgId
                      ? nodeGroupIds.filter(gid => gid !== mgId)
                          .map(gid => groups.find(g => g.id === gid)?.name)
                          .filter(Boolean)
                      : []
                    return (
                      <button
                        key={n.id}
                        type="button"
                        onClick={() => toggleUnboundSelect(n.id)}
                        className={`w-full flex items-center gap-2 p-2 rounded-md border transition-colors text-left ${selected ? 'bg-indigo-950/30 border-indigo-800/50' : 'bg-zinc-900/50 border-zinc-800 hover:border-zinc-700'}`}
                      >
                        <div className={`w-4 h-4 rounded border flex-shrink-0 flex items-center justify-center ${selected ? 'bg-indigo-600 border-indigo-500' : 'border-zinc-600'}`}>
                          {selected && <Check className="w-3 h-3 text-white" />}
                        </div>
                        <div className="flex-1 min-w-0">
                          <div className="text-xs text-zinc-200 truncate">{n.name}</div>
                          <div className="text-[10px] text-zinc-500 font-mono truncate">
                            {n.code} · {n.address}:{n.port}
                            {otherGroupNames.length > 0 && <span className="text-amber-500 ml-1">[也在: {otherGroupNames.join(', ')}]</span>}
                          </div>
                        </div>
                        <Badge variant="outline" className={`${protocolBadge(n.protocol_type)} text-[9px] flex-shrink-0`}>
                          {n.protocol_type}
                        </Badge>
                      </button>
                    )
                  })
                )}
              </div>
            </div>
          </div>

          <DialogFooter className="flex-shrink-0 pt-2">
            <Button
              variant="outline"
              onClick={() => setNodesDialogOpen(false)}
              className="border-zinc-700 text-zinc-300"
            >
              完成
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
