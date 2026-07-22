import { useState, useMemo } from 'react'
import { useProtocolPresets, useDeletePreset, useForkPreset, useUpdatePreset } from '@/lib/hooks'
import type { ProtocolPreset } from '@/lib/hooks'
import { useToast } from '@airport/ui'

// 预设管理页：支持分组、搜索、过滤、fork 内置预设、编辑自定义预设、删除
export default function Presets() {
  const { data: presets, isLoading } = useProtocolPresets()
  const deleteMut = useDeletePreset()
  const forkMut = useForkPreset()
  const updateMut = useUpdatePreset()
  const { toast } = useToast()

  const [search, setSearch] = useState('')
  const [filterProtocol, setFilterProtocol] = useState('')
  const [filterTransport, setFilterTransport] = useState('')
  const [filterBuiltin, setFilterBuiltin] = useState<'all' | 'builtin' | 'custom'>('all')
  const [editing, setEditing] = useState<ProtocolPreset | null>(null)
  const [editName, setEditName] = useState('')
  const [editDesc, setEditDesc] = useState('')

  // 过滤后的预设列表
  const filtered = useMemo(() => {
    if (!presets) return []
    return presets.filter(p => {
      if (search) {
        const q = search.toLowerCase()
        const name = (p.name || '').toLowerCase()
        const code = (p.code || '').toLowerCase()
        const desc = (p.description || '').toLowerCase()
        if (!name.includes(q) && !code.includes(q) && !desc.includes(q)) return false
      }
      if (filterProtocol && p.protocol_type !== filterProtocol) return false
      if (filterTransport && p.transport_type !== filterTransport) return false
      if (filterBuiltin === 'builtin' && !p.is_builtin) return false
      if (filterBuiltin === 'custom' && p.is_builtin) return false
      return true
    })
  }, [presets, search, filterProtocol, filterTransport, filterBuiltin])

  // 按协议分组
  const grouped = useMemo(() => {
    const m = new Map<string, ProtocolPreset[]>()
    for (const p of filtered) {
      const key = p.protocol_type || 'other'
      if (!m.has(key)) m.set(key, [])
      m.get(key)!.push(p)
    }
    return Array.from(m.entries()).sort((a, b) => a[0].localeCompare(b[0]))
  }, [filtered])

  const protocols = useMemo(() => {
    const s = new Set<string>()
    presets?.forEach(p => p.protocol_type && s.add(p.protocol_type))
    return Array.from(s).sort()
  }, [presets])

  const transports = useMemo(() => {
    const s = new Set<string>()
    presets?.forEach(p => p.transport_type && s.add(p.transport_type))
    return Array.from(s).sort()
  }, [presets])

  const handleFork = (p: ProtocolPreset) => {
    forkMut.mutate({ id: p.id }, {
      onSuccess: () => toast({ title: '已复制为自定义预设', variant: 'success' }),
      onError: (e) => toast({ title: '复制失败', description: String(e), variant: 'destructive' }),
    })
  }

  const handleDelete = (p: ProtocolPreset) => {
    if (!confirm(`确认删除预设 "${p.name}"？`)) return
    deleteMut.mutate(p.id, {
      onSuccess: () => toast({ title: '已删除', variant: 'success' }),
      onError: (e) => toast({ title: '删除失败', description: String(e), variant: 'destructive' }),
    })
  }

  const handleToggleEnabled = (p: ProtocolPreset) => {
    updateMut.mutate({ id: p.id, data: { is_enabled: !p.is_enabled } }, {
      onSuccess: () => toast({ title: p.is_enabled ? '已禁用' : '已启用', variant: 'success' }),
      onError: (e) => toast({ title: '操作失败', description: String(e), variant: 'destructive' }),
    })
  }

  const handleEdit = (p: ProtocolPreset) => {
    setEditing(p)
    setEditName(p.name || '')
    setEditDesc(p.description || '')
  }

  const handleSaveEdit = () => {
    if (!editing) return
    updateMut.mutate({ id: editing.id, data: { name: editName, description: editDesc } }, {
      onSuccess: () => {
        toast({ title: '已保存', variant: 'success' })
        setEditing(null)
      },
      onError: (e) => toast({ title: '保存失败', description: String(e), variant: 'destructive' }),
    })
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">协议预设管理</h1>
        <div className="text-sm text-muted-foreground">
          共 {presets?.length || 0} 个预设（内置 {presets?.filter(p => p.is_builtin).length || 0} + 自定义 {presets?.filter(p => !p.is_builtin).length || 0}）
        </div>
      </div>

      {/* 工具栏 */}
      <div className="flex flex-wrap gap-2 items-center">
        <input
          type="text"
          placeholder="搜索名称/描述/代码..."
          value={search}
          onChange={e => setSearch(e.target.value)}
          className="px-3 py-1.5 text-sm border rounded-md min-w-[200px]"
        />
        <select value={filterProtocol} onChange={e => setFilterProtocol(e.target.value)} className="px-3 py-1.5 text-sm border rounded-md">
          <option value="">全部协议</option>
          {protocols.map(p => <option key={p} value={p}>{p}</option>)}
        </select>
        <select value={filterTransport} onChange={e => setFilterTransport(e.target.value)} className="px-3 py-1.5 text-sm border rounded-md">
          <option value="">全部传输</option>
          {transports.map(t => <option key={t} value={t}>{t}</option>)}
        </select>
        <select value={filterBuiltin} onChange={e => setFilterBuiltin(e.target.value as any)} className="px-3 py-1.5 text-sm border rounded-md">
          <option value="all">全部类型</option>
          <option value="builtin">内置</option>
          <option value="custom">自定义</option>
        </select>
      </div>

      {isLoading && <div className="text-center py-8 text-muted-foreground">加载中...</div>}

      {/* 分组展示 */}
      {!isLoading && grouped.map(([proto, items]) => (
        <div key={proto} className="space-y-2">
          <h2 className="text-lg font-semibold text-muted-foreground uppercase">{proto} ({items.length})</h2>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
            {items.map(p => (
              <div key={p.id} className={`border rounded-lg p-4 space-y-2 ${p.is_builtin ? 'bg-blue-50/30 border-blue-200' : 'bg-green-50/30 border-green-200'}`}>
                <div className="flex items-start justify-between">
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="font-medium truncate">{p.name}</span>
                      {p.is_builtin ? (
                        <span className="text-xs px-1.5 py-0.5 bg-blue-100 text-blue-700 rounded">内置</span>
                      ) : (
                        <span className="text-xs px-1.5 py-0.5 bg-green-100 text-green-700 rounded">自定义</span>
                      )}
                      {p.is_recommended && <span className="text-xs px-1.5 py-0.5 bg-yellow-100 text-yellow-700 rounded">推荐</span>}
                    </div>
                    <div className="text-xs text-muted-foreground mt-0.5">
                      {p.protocol_type} / {p.transport_type} / {p.security_type}
                    </div>
                  </div>
                </div>
                {p.description && <p className="text-sm text-muted-foreground line-clamp-2">{p.description}</p>}
                <div className="flex flex-wrap gap-1.5 pt-1">
                  {p.is_builtin ? (
                    <button
                      onClick={() => handleFork(p)}
                      disabled={forkMut.isPending}
                      className="text-xs px-2 py-1 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
                    >
                      复制为可编辑
                    </button>
                  ) : (
                    <>
                      <button
                        onClick={() => handleEdit(p)}
                        className="text-xs px-2 py-1 bg-gray-600 text-white rounded hover:bg-gray-700"
                      >
                        编辑
                      </button>
                      <button
                        onClick={() => handleToggleEnabled(p)}
                        className={`text-xs px-2 py-1 rounded text-white ${p.is_enabled ? 'bg-orange-600 hover:bg-orange-700' : 'bg-green-600 hover:bg-green-700'}`}
                      >
                        {p.is_enabled ? '禁用' : '启用'}
                      </button>
                      <button
                        onClick={() => handleDelete(p)}
                        disabled={deleteMut.isPending}
                        className="text-xs px-2 py-1 bg-red-600 text-white rounded hover:bg-red-700 disabled:opacity-50"
                      >
                        删除
                      </button>
                    </>
                  )}
                </div>
              </div>
            ))}
          </div>
        </div>
      ))}

      {!isLoading && filtered.length === 0 && (
        <div className="text-center py-12 text-muted-foreground">无匹配预设</div>
      )}

      {/* 编辑弹窗 */}
      {editing && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={() => setEditing(null)}>
          <div className="bg-background border rounded-lg p-6 w-[480px] space-y-4" onClick={e => e.stopPropagation()}>
            <h3 className="text-lg font-semibold">编辑预设</h3>
            <div className="space-y-3">
              <div>
                <label className="text-sm font-medium">名称</label>
                <input value={editName} onChange={e => setEditName(e.target.value)} className="w-full px-3 py-1.5 text-sm border rounded-md mt-1" />
              </div>
              <div>
                <label className="text-sm font-medium">描述</label>
                <textarea value={editDesc} onChange={e => setEditDesc(e.target.value)} rows={3} className="w-full px-3 py-1.5 text-sm border rounded-md mt-1" />
              </div>
            </div>
            <div className="flex justify-end gap-2">
              <button onClick={() => setEditing(null)} className="px-3 py-1.5 text-sm border rounded-md">取消</button>
              <button onClick={handleSaveEdit} disabled={updateMut.isPending} className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded-md hover:bg-blue-700 disabled:opacity-50">保存</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
