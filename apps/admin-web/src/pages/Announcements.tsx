import { useState, useEffect } from 'react'
import { Megaphone, Plus, Pencil, Trash2, Eye, EyeOff } from 'lucide-react'
import {
  Card,
  CardContent,
  Button,
  Input,
  Label,
  Badge,
  Switch,
  Textarea,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  Skeleton,
  EmptyState,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  useToast,
} from '@airport/ui'
import { api } from '@/lib/api'
import { EP } from '@/lib/endpoints'

interface Announcement {
  id: number
  title: string
  content: string
  type: 'notice' | 'update' | 'maintenance' | 'alert'
  status: 'draft' | 'published' | 'archived'
  published_at?: string
  created_at: string
  updated_at: string
  sort?: number
  img_url?: string
}

function toBool(v: unknown): boolean {
  if (typeof v === 'boolean') return v
  if (typeof v === 'number') return v === 1
  if (typeof v === 'string') return v === '1' || v === 'true'
  return false
}

function truncate(str: string, len: number): string {
  if (!str) return '-'
  return str.length > len ? str.slice(0, len) + '...' : str
}

export default function Announcements() {
  const { toast } = useToast()
  const [loading, setLoading] = useState(true)
  const [announcements, setAnnouncements] = useState<Announcement[]>([])

  const [editOpen, setEditOpen] = useState(false)
  const [editLoading, setEditLoading] = useState(false)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [form, setForm] = useState({
    title: '',
    content: '',
    show: true,
    sort: 0,
    img_url: '',
  })

  useEffect(() => {
    loadAnnouncements()
  }, [])

  const loadAnnouncements = async () => {
    setLoading(true)
    try {
      const data = await api.get<{ items: Announcement[]; total: number; page: number; page_size: number }>(EP.ANNOUNCEMENTS, {
        params: { page: 1, page_size: 100 },
      })
      setAnnouncements(data.items.map((item, idx) => ({
        ...item,
        sort: (data.items.length - idx),
      })))
    } catch (err) {
      toast({
        title: '加载失败',
        description: err instanceof Error ? err.message : '无法获取公告列表',
        variant: 'destructive',
      })
    } finally {
      setLoading(false)
    }
  }

  const openCreate = () => {
    setEditingId(null)
    setForm({ title: '', content: '', show: true, sort: 0, img_url: '' })
    setEditOpen(true)
  }

  const openEdit = (n: Announcement) => {
    setEditingId(n.id)
    setForm({
      title: n.title || '',
      content: n.content || '',
      show: n.status === 'published',
      sort: typeof n.sort === 'number' ? n.sort : Number(n.sort) || 0,
      img_url: n.img_url || '',
    })
    setEditOpen(true)
  }

  const submitForm = async () => {
    if (!form.title.trim()) {
      toast({ title: '校验失败', description: '请输入公告标题', variant: 'destructive' })
      return
    }
    if (!form.content.trim()) {
      toast({ title: '校验失败', description: '请输入公告内容', variant: 'destructive' })
      return
    }
    setEditLoading(true)
    try {
      const payload = {
        title: form.title.trim(),
        content: form.content.trim(),
        type: 'notice' as const,
      }
      if (editingId) {
        await api.patch(EP.ANNOUNCEMENT_DETAIL(String(editingId)), payload)
        if (form.show) {
          await api.post(EP.ANNOUNCEMENT_PUBLISH(String(editingId)))
        }
        toast({ title: '更新成功', variant: 'success' })
      } else {
        // 创建后若勾选"显示"，立即调用 publish 让用户端可见
        const created = await api.post<{ id: string }>(EP.ANNOUNCEMENTS, payload)
        if (form.show && created?.id) {
          await api.post(EP.ANNOUNCEMENT_PUBLISH(String(created.id)))
        }
        toast({ title: '创建成功', variant: 'success' })
      }
      setEditOpen(false)
      await loadAnnouncements()
    } catch (err) {
      toast({
        title: '保存失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setEditLoading(false)
    }
  }

  const toggleShow = async (n: Announcement) => {
    try {
      if (n.status === 'draft' || n.status === 'archived') {
        await api.post(EP.ANNOUNCEMENT_PUBLISH(String(n.id)))
        toast({ title: '已显示', variant: 'success' })
      } else {
        await api.post(EP.ANNOUNCEMENT_ARCHIVE(String(n.id)))
        toast({ title: '已隐藏', variant: 'success' })
      }
      await loadAnnouncements()
    } catch (err) {
      toast({
        title: '操作失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    }
  }

  const deleteAnnouncement = async (n: Announcement) => {
    if (!window.confirm(`确定删除公告「${n.title}」吗？此操作不可撤销。`)) return
    try {
      await api.delete(EP.ANNOUNCEMENT_DETAIL(String(n.id)))
      toast({ title: '已删除', variant: 'success' })
      await loadAnnouncements()
    } catch (err) {
      toast({
        title: '删除失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    }
  }

  const updateSort = async (n: Announcement, newSort: number) => {
    setAnnouncements((prev) => prev.map((x) => (x.id === n.id ? { ...x, sort: newSort } : x)))
  }

  const sortedAnnouncements = [...announcements].sort((a, b) => (Number(b.sort) || 0) - (Number(a.sort) || 0))

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-zinc-100">公告管理</h2>
        <Button size="sm" className="bg-indigo-600 hover:bg-indigo-500" onClick={openCreate}>
          <Plus className="w-4 h-4 mr-1" />
          新建公告
        </Button>
      </div>

      <Card className="bg-zinc-900 border-zinc-800 overflow-hidden">
        <CardContent className="p-0">
          {loading ? (
            <div className="p-4 space-y-3">
              {[1, 2, 3, 4, 5].map((i) => (
                <Skeleton key={i} className="h-14 w-full bg-zinc-800 rounded-lg" />
              ))}
            </div>
          ) : sortedAnnouncements.length === 0 ? (
            <EmptyState title="暂无公告" description="点击右上角按钮创建第一条公告" className="py-12" />
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="border-zinc-800 hover:bg-transparent">
                    <TableHead className="text-zinc-400 text-xs font-medium w-12">ID</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium">标题</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium hidden md:table-cell">内容预览</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium w-20">显示</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium w-24">排序</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium w-28">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {sortedAnnouncements.map((n) => (
                    <TableRow key={n.id} className="border-zinc-800 hover:bg-zinc-800/50">
                      <TableCell className="py-3 text-sm text-zinc-400 font-mono">{n.id}</TableCell>
                      <TableCell className="py-3">
                        <div className="flex items-center gap-2">
                          <div className="p-1.5 rounded-md bg-zinc-800">
                            <Megaphone className="w-4 h-4 text-zinc-400" />
                          </div>
                          <div className="font-medium text-zinc-200 text-sm truncate max-w-[200px]">
                            {n.title || '(无标题)'}
                          </div>
                        </div>
                      </TableCell>
                      <TableCell className="py-3 text-sm text-zinc-400 hidden md:table-cell max-w-[300px] truncate">
                        {truncate(n.content, 60)}
                      </TableCell>
                      <TableCell className="py-3">
                        <Button
                          variant="ghost"
                          size="icon"
                          className={`h-8 w-8 ${n.status === 'published' ? 'text-emerald-400' : 'text-zinc-500'}`}
                          onClick={() => toggleShow(n)}
                        >
                          {n.status === 'published' ? <Eye className="w-4 h-4" /> : <EyeOff className="w-4 h-4" />}
                        </Button>
                      </TableCell>
                      <TableCell className="py-3">
                        <Input
                          type="number"
                          value={n.sort ?? 0}
                          onChange={(e) => {
                            const val = Number(e.target.value) || 0
                            setAnnouncements((prev) => prev.map((x) => (x.id === n.id ? { ...x, sort: val } : x)))
                          }}
                          onBlur={(e) => updateSort(n, Number(e.target.value) || 0)}
                          className="bg-zinc-800 border-zinc-700 text-zinc-100 w-16 h-8 text-center text-sm"
                        />
                      </TableCell>
                      <TableCell className="py-3">
                        <div className="flex items-center gap-1">
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-8 w-8 text-zinc-400"
                            onClick={() => openEdit(n)}
                          >
                            <Pencil className="w-4 h-4" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-8 w-8 text-red-400 hover:text-red-300"
                            onClick={() => deleteAnnouncement(n)}
                          >
                            <Trash2 className="w-4 h-4" />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>

      <Dialog open={editOpen} onOpenChange={setEditOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-2xl max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Megaphone className="w-5 h-5 text-zinc-400" />
              <span>{editingId ? '编辑公告' : '新建公告'}</span>
            </DialogTitle>
          </DialogHeader>

          <div className="space-y-4 pt-1">
            <div className="space-y-2">
              <Label htmlFor="notice-title" className="text-zinc-300">标题</Label>
              <Input
                id="notice-title"
                placeholder="请输入公告标题..."
                value={form.title}
                onChange={(e) => setForm({ ...form, title: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500"
              />
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-2">
                <Label htmlFor="notice-sort" className="text-zinc-300">排序（越大越靠前）</Label>
                <Input
                  id="notice-sort"
                  type="number"
                  value={form.sort}
                  onChange={(e) => setForm({ ...form, sort: Number(e.target.value) || 0 })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
              </div>
              <div className="space-y-2">
                <Label className="text-zinc-300">图片URL（可选）</Label>
                <Input
                  placeholder="https://..."
                  value={form.img_url}
                  onChange={(e) => setForm({ ...form, img_url: e.target.value })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500"
                />
              </div>
            </div>

            <div className="space-y-2">
              <Label htmlFor="notice-content" className="text-zinc-300">内容</Label>
              <Textarea
                id="notice-content"
                rows={6}
                placeholder="请输入公告内容，支持 Markdown 格式..."
                value={form.content}
                onChange={(e) => setForm({ ...form, content: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500 focus:border-indigo-500 min-h-[160px]"
              />
              <p className="text-xs text-zinc-500">支持 Markdown 语法</p>
            </div>

            <div className="flex items-center justify-between rounded-lg border border-zinc-800 bg-zinc-950/30 p-3">
              <div>
                <Label className="text-zinc-300 text-sm">显示公告</Label>
                <p className="text-xs text-zinc-500 mt-0.5">关闭后用户端将看不到此公告</p>
              </div>
              <Switch checked={form.show} onChange={(e) => setForm({ ...form, show: e.target.checked })} />
            </div>
          </div>

          <DialogFooter className="pt-2">
            <Button
              variant="outline"
              onClick={() => setEditOpen(false)}
              className="border-zinc-700 text-zinc-300"
            >
              取消
            </Button>
            <Button
              className="bg-indigo-600 hover:bg-indigo-500"
              onClick={submitForm}
              disabled={editLoading}
              isLoading={editLoading}
            >
              保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
