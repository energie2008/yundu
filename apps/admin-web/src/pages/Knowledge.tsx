import { useState, useEffect } from 'react'
import { BookOpen, Plus, Pencil, Trash2, Eye, EyeOff, FolderPlus, ChevronRight, FileText } from 'lucide-react'
import {
  Card,
  CardContent,
  Button,
  Input,
  Label,
  Switch,
  Textarea,
  Select,
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
  Badge,
} from '@airport/ui'
import { xbAdminApi } from '@/lib/api'

interface KnowledgeCategory {
  id: number
  name: string
  sort?: number
  created_at?: number
  updated_at?: number
}

interface KnowledgeArticle {
  id: number
  category_id: number
  title: string
  body: string
  show: number | boolean
  sort: number
  created_at?: number
  updated_at?: number
  category?: KnowledgeCategory
}

function extractList<T>(resp: unknown): T[] {
  if (Array.isArray(resp)) return resp as T[]
  if (!resp || typeof resp !== 'object') return []
  const obj = resp as Record<string, unknown>
  const dataField = obj.data
  if (dataField && typeof dataField === 'object') {
    if (Array.isArray(dataField)) return dataField as T[]
    const dataObj = dataField as Record<string, unknown>
    if (Array.isArray(dataObj.data)) return dataObj.data as T[]
    if (Array.isArray(dataObj.categories)) return dataObj.categories as T[]
    if (Array.isArray(dataObj.items)) return dataObj.items as T[]
    if (Array.isArray(dataObj.list)) return dataObj.list as T[]
  }
  if (Array.isArray(obj.data)) return obj.data as T[]
  if (Array.isArray(obj.categories)) return obj.categories as T[]
  if (Array.isArray(obj.items)) return obj.items as T[]
  if (Array.isArray(obj.list)) return obj.list as T[]
  return []
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

export default function Knowledge() {
  const { toast } = useToast()
  const [loading, setLoading] = useState(true)
  const [categories, setCategories] = useState<KnowledgeCategory[]>([])
  const [articles, setArticles] = useState<KnowledgeArticle[]>([])
  const [selectedCategoryId, setSelectedCategoryId] = useState<number | null>(null)

  const [catDialogOpen, setCatDialogOpen] = useState(false)
  const [editingCatId, setEditingCatId] = useState<number | null>(null)
  const [catForm, setCatForm] = useState({ name: '', sort: 0 })
  const [catSaving, setCatSaving] = useState(false)

  const [artDialogOpen, setArtDialogOpen] = useState(false)
  const [editingArtId, setEditingArtId] = useState<number | null>(null)
  const [artForm, setArtForm] = useState({
    category_id: 0,
    title: '',
    body: '',
    show: true,
    sort: 0,
  })
  const [artSaving, setArtSaving] = useState(false)

  useEffect(() => {
    loadData()
  }, [])

  const loadData = async () => {
    setLoading(true)
    try {
      const [catsResp, artsResp] = await Promise.all([
        xbAdminApi.get<unknown>('/knowledge/getCategory'),
        xbAdminApi.get<unknown>('/knowledge/fetch'),
      ])
      const cats = extractList<KnowledgeCategory>(catsResp)
      const arts = extractList<KnowledgeArticle>(artsResp)
      setCategories(cats)
      setArticles(arts)
      if (cats.length > 0 && selectedCategoryId === null) {
        setSelectedCategoryId(cats[0].id)
      }
    } catch (err) {
      toast({
        title: '加载失败',
        description: err instanceof Error ? err.message : '无法获取知识库数据',
        variant: 'destructive',
      })
    } finally {
      setLoading(false)
    }
  }

  const openCreateCategory = () => {
    setEditingCatId(null)
    setCatForm({ name: '', sort: 0 })
    setCatDialogOpen(true)
  }

  const openEditCategory = (c: KnowledgeCategory) => {
    setEditingCatId(c.id)
    setCatForm({ name: c.name || '', sort: c.sort || 0 })
    setCatDialogOpen(true)
  }

  const saveCategory = async () => {
    if (!catForm.name.trim()) {
      toast({ title: '校验失败', description: '请输入分类名称', variant: 'destructive' })
      return
    }
    setCatSaving(true)
    try {
      if (editingCatId) {
        await xbAdminApi.post('/knowledge/save', { id: editingCatId, name: catForm.name.trim(), sort: Number(catForm.sort) || 0, type: 'category' })
        toast({ title: '更新成功', variant: 'success' })
      } else {
        await xbAdminApi.post('/knowledge/save', { name: catForm.name.trim(), sort: Number(catForm.sort) || 0, type: 'category' })
        toast({ title: '创建成功', variant: 'success' })
      }
      setCatDialogOpen(false)
      await loadData()
    } catch (err) {
      toast({
        title: '保存失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setCatSaving(false)
    }
  }

  const deleteCategory = async (c: KnowledgeCategory) => {
    if (!window.confirm(`确定删除分类「${c.name}」吗？该分类下的文章也会受影响。`)) return
    try {
      await xbAdminApi.post('/knowledge/drop', { id: c.id, type: 'category' })
      toast({ title: '已删除', variant: 'success' })
      if (selectedCategoryId === c.id) setSelectedCategoryId(null)
      await loadData()
    } catch (err) {
      toast({
        title: '删除失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    }
  }

  const openCreateArticle = () => {
    setEditingArtId(null)
    setArtForm({
      category_id: selectedCategoryId || (categories[0]?.id ?? 0),
      title: '',
      body: '',
      show: true,
      sort: 0,
    })
    setArtDialogOpen(true)
  }

  const openEditArticle = (a: KnowledgeArticle) => {
    setEditingArtId(a.id)
    setArtForm({
      category_id: a.category_id,
      title: a.title || '',
      body: a.body || '',
      show: toBool(a.show),
      sort: typeof a.sort === 'number' ? a.sort : Number(a.sort) || 0,
    })
    setArtDialogOpen(true)
  }

  const saveArticle = async () => {
    if (!artForm.title.trim()) {
      toast({ title: '校验失败', description: '请输入文章标题', variant: 'destructive' })
      return
    }
    if (!artForm.body.trim()) {
      toast({ title: '校验失败', description: '请输入文章内容', variant: 'destructive' })
      return
    }
    if (!artForm.category_id) {
      toast({ title: '校验失败', description: '请选择分类', variant: 'destructive' })
      return
    }
    setArtSaving(true)
    try {
      const payload = {
        category_id: Number(artForm.category_id),
        title: artForm.title.trim(),
        body: artForm.body.trim(),
        show: artForm.show ? 1 : 0,
        sort: Number(artForm.sort) || 0,
      }
      if (editingArtId) {
        await xbAdminApi.post('/knowledge/save', { id: editingArtId, ...payload })
        toast({ title: '更新成功', variant: 'success' })
      } else {
        await xbAdminApi.post('/knowledge/save', payload)
        toast({ title: '创建成功', variant: 'success' })
      }
      setArtDialogOpen(false)
      await loadData()
    } catch (err) {
      toast({
        title: '保存失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setArtSaving(false)
    }
  }

  const toggleArticleShow = async (a: KnowledgeArticle) => {
    try {
      const newShow = toBool(a.show) ? 0 : 1
      await xbAdminApi.post('/knowledge/show', { id: a.id, show: newShow })
      toast({ title: newShow ? '已显示' : '已隐藏', variant: 'success' })
      await loadData()
    } catch (err) {
      toast({
        title: '操作失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    }
  }

  const deleteArticle = async (a: KnowledgeArticle) => {
    if (!window.confirm(`确定删除文章「${a.title}」吗？`)) return
    try {
      await xbAdminApi.post('/knowledge/drop', { id: a.id })
      toast({ title: '已删除', variant: 'success' })
      await loadData()
    } catch (err) {
      toast({
        title: '删除失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    }
  }

  const filteredArticles = articles.filter((a) =>
    selectedCategoryId ? a.category_id === selectedCategoryId : true
  ).sort((a, b) => (Number(b.sort) || 0) - (Number(a.sort) || 0))

  const getCategoryName = (id: number): string => {
    return categories.find((c) => c.id === id)?.name || '未分类'
  }

  if (loading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-7 w-32 bg-zinc-800 rounded" />
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
          <Skeleton className="h-80 w-full bg-zinc-800 rounded-lg col-span-1" />
          <Skeleton className="h-80 w-full bg-zinc-800 rounded-lg col-span-3" />
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-zinc-100">知识库管理</h2>
        <Button size="sm" className="bg-indigo-600 hover:bg-indigo-500" onClick={openCreateArticle}>
          <Plus className="w-4 h-4 mr-1" />
          新建文章
        </Button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        <Card className="bg-zinc-900 border-zinc-800 col-span-1">
          <CardContent className="p-3">
            <div className="flex items-center justify-between mb-3">
              <div className="flex items-center gap-2">
                <BookOpen className="w-4 h-4 text-indigo-400" />
                <span className="text-sm font-medium text-zinc-200">分类</span>
              </div>
              <Button
                variant="ghost"
                size="icon"
                className="h-7 w-7 text-zinc-400"
                onClick={openCreateCategory}
              >
                <FolderPlus className="w-4 h-4" />
              </Button>
            </div>
            <div className="space-y-1">
              <button
                onClick={() => setSelectedCategoryId(null)}
                className={`w-full flex items-center justify-between px-3 py-2 rounded-lg text-sm transition-colors ${
                  selectedCategoryId === null
                    ? 'bg-indigo-600/20 text-indigo-300 border border-indigo-600/30'
                    : 'text-zinc-400 hover:bg-zinc-800 hover:text-zinc-200'
                }`}
              >
                <span className="flex items-center gap-2">
                  <FileText className="w-4 h-4" />
                  全部文章
                </span>
                <Badge variant="secondary" className="bg-zinc-800 text-zinc-400 text-xs">
                  {articles.length}
                </Badge>
              </button>
              {categories.map((c) => {
                const count = articles.filter((a) => a.category_id === c.id).length
                return (
                  <div key={c.id} className="group flex items-center gap-1">
                    <button
                      onClick={() => setSelectedCategoryId(c.id)}
                      className={`flex-1 flex items-center justify-between px-3 py-2 rounded-lg text-sm transition-colors ${
                        selectedCategoryId === c.id
                          ? 'bg-indigo-600/20 text-indigo-300 border border-indigo-600/30'
                          : 'text-zinc-400 hover:bg-zinc-800 hover:text-zinc-200'
                      }`}
                    >
                      <span className="flex items-center gap-2 truncate">
                        <ChevronRight className="w-3 h-3 shrink-0" />
                        <span className="truncate">{c.name}</span>
                      </span>
                      <Badge variant="secondary" className="bg-zinc-800 text-zinc-400 text-xs shrink-0">
                        {count}
                      </Badge>
                    </button>
                    <div className="hidden group-hover:flex items-center gap-0.5 shrink-0">
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-6 w-6 text-zinc-500 hover:text-zinc-300"
                        onClick={() => openEditCategory(c)}
                      >
                        <Pencil className="w-3 h-3" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-6 w-6 text-red-400 hover:text-red-300"
                        onClick={() => deleteCategory(c)}
                      >
                        <Trash2 className="w-3 h-3" />
                      </Button>
                    </div>
                  </div>
                )
              })}
              {categories.length === 0 && (
                <div className="text-center text-xs text-zinc-500 py-4">
                  暂无分类，点击 + 创建
                </div>
              )}
            </div>
          </CardContent>
        </Card>

        <Card className="bg-zinc-900 border-zinc-800 col-span-1 md:col-span-3 overflow-hidden">
          <CardContent className="p-0">
            {filteredArticles.length === 0 ? (
              <EmptyState
                title="暂无文章"
                description={categories.length === 0 ? '请先创建分类' : '点击右上角按钮创建文章'}
                className="py-12"
              />
            ) : (
              <div className="overflow-x-auto">
                <Table>
                  <TableHeader>
                    <TableRow className="border-zinc-800 hover:bg-transparent">
                      <TableHead className="text-zinc-400 text-xs font-medium w-12">ID</TableHead>
                      <TableHead className="text-zinc-400 text-xs font-medium">标题</TableHead>
                      <TableHead className="text-zinc-400 text-xs font-medium hidden md:table-cell">分类</TableHead>
                      <TableHead className="text-zinc-400 text-xs font-medium hidden lg:table-cell">内容预览</TableHead>
                      <TableHead className="text-zinc-400 text-xs font-medium w-16">显示</TableHead>
                      <TableHead className="text-zinc-400 text-xs font-medium w-20">排序</TableHead>
                      <TableHead className="text-zinc-400 text-xs font-medium w-20">操作</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {filteredArticles.map((a) => (
                      <TableRow key={a.id} className="border-zinc-800 hover:bg-zinc-800/50">
                        <TableCell className="py-3 text-sm text-zinc-400 font-mono">{a.id}</TableCell>
                        <TableCell className="py-3">
                          <div className="flex items-center gap-2">
                            <div className="p-1.5 rounded-md bg-zinc-800">
                              <FileText className="w-4 h-4 text-zinc-400" />
                            </div>
                            <div className="font-medium text-zinc-200 text-sm truncate max-w-[180px]">
                              {a.title || '(无标题)'}
                            </div>
                          </div>
                        </TableCell>
                        <TableCell className="py-3 hidden md:table-cell">
                          <Badge variant="secondary" className="bg-zinc-800 text-zinc-300 text-xs">
                            {getCategoryName(a.category_id)}
                          </Badge>
                        </TableCell>
                        <TableCell className="py-3 text-sm text-zinc-400 hidden lg:table-cell max-w-[200px] truncate">
                          {truncate(a.body, 50)}
                        </TableCell>
                        <TableCell className="py-3">
                          <Button
                            variant="ghost"
                            size="icon"
                            className={`h-8 w-8 ${toBool(a.show) ? 'text-emerald-400' : 'text-zinc-500'}`}
                            onClick={() => toggleArticleShow(a)}
                          >
                            {toBool(a.show) ? <Eye className="w-4 h-4" /> : <EyeOff className="w-4 h-4" />}
                          </Button>
                        </TableCell>
                        <TableCell className="py-3 text-sm text-zinc-400">{a.sort ?? 0}</TableCell>
                        <TableCell className="py-3">
                          <div className="flex items-center gap-1">
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-8 w-8 text-zinc-400"
                              onClick={() => openEditArticle(a)}
                            >
                              <Pencil className="w-4 h-4" />
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-8 w-8 text-red-400 hover:text-red-300"
                              onClick={() => deleteArticle(a)}
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
      </div>

      <Dialog open={catDialogOpen} onOpenChange={setCatDialogOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <FolderPlus className="w-5 h-5 text-zinc-400" />
              <span>{editingCatId ? '编辑分类' : '新建分类'}</span>
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-3 pt-1">
            <div className="space-y-2">
              <Label htmlFor="cat-name" className="text-zinc-300">分类名称</Label>
              <Input
                id="cat-name"
                placeholder="如：使用指南"
                value={catForm.name}
                onChange={(e) => setCatForm({ ...catForm, name: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="cat-sort" className="text-zinc-300">排序</Label>
              <Input
                id="cat-sort"
                type="number"
                value={catForm.sort}
                onChange={(e) => setCatForm({ ...catForm, sort: Number(e.target.value) || 0 })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCatDialogOpen(false)} className="border-zinc-700 text-zinc-300">
              取消
            </Button>
            <Button
              className="bg-indigo-600 hover:bg-indigo-500"
              onClick={saveCategory}
              disabled={catSaving}
              isLoading={catSaving}
            >
              保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={artDialogOpen} onOpenChange={setArtDialogOpen}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-2xl max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <FileText className="w-5 h-5 text-zinc-400" />
              <span>{editingArtId ? '编辑文章' : '新建文章'}</span>
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-4 pt-1">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
              <div className="space-y-2">
                <Label htmlFor="art-cat" className="text-zinc-300">所属分类</Label>
                <Select
                  value={String(artForm.category_id)}
                  onChange={(e) => setArtForm({ ...artForm, category_id: Number(e.target.value) })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                >
                  <option value="0">请选择分类</option>
                  {categories.map((c) => (
                    <option key={c.id} value={c.id}>{c.name}</option>
                  ))}
                </Select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="art-sort" className="text-zinc-300">排序</Label>
                <Input
                  id="art-sort"
                  type="number"
                  value={artForm.sort}
                  onChange={(e) => setArtForm({ ...artForm, sort: Number(e.target.value) || 0 })}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="art-title" className="text-zinc-300">标题</Label>
              <Input
                id="art-title"
                placeholder="请输入文章标题..."
                value={artForm.title}
                onChange={(e) => setArtForm({ ...artForm, title: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="art-body" className="text-zinc-300">内容</Label>
              <Textarea
                id="art-body"
                rows={8}
                placeholder="请输入文章内容..."
                value={artForm.body}
                onChange={(e) => setArtForm({ ...artForm, body: e.target.value })}
                className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500 focus:border-indigo-500 min-h-[200px]"
              />
            </div>
            <div className="flex items-center justify-between rounded-lg border border-zinc-800 bg-zinc-950/30 p-3">
              <div>
                <Label className="text-zinc-300 text-sm">显示文章</Label>
                <p className="text-xs text-zinc-500 mt-0.5">关闭后用户端将看不到此文章</p>
              </div>
              <Switch checked={artForm.show} onChange={(e) => setArtForm({ ...artForm, show: e.target.checked })} />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setArtDialogOpen(false)} className="border-zinc-700 text-zinc-300">
              取消
            </Button>
            <Button
              className="bg-indigo-600 hover:bg-indigo-500"
              onClick={saveArticle}
              disabled={artSaving}
              isLoading={artSaving}
            >
              保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
