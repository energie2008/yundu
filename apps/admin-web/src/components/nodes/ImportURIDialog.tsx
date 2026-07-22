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
  Textarea,
  Badge,
  useToast,
} from '@airport/ui'
import { Link2, AlertTriangle, CheckCircle2, XCircle, Copy } from 'lucide-react'
import { Checkbox } from '@/components/common/Checkbox'
import {
  useServers,
  useRuntimes,
  useImportUriPreview,
  useImportUriConfirm,
  ImportPreviewItem,
} from '@/lib/hooks'

interface ImportURIDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess?: () => void
}

export function ImportURIDialog({ open, onOpenChange, onSuccess }: ImportURIDialogProps) {
  const { toast } = useToast()
  const [uris, setUris] = useState('')
  const [serverId, setServerId] = useState('')
  const [runtimeId, setRuntimeId] = useState('')
  const [region, setRegion] = useState('')
  const [multiplier, setMultiplier] = useState('1')
  const [previewItems, setPreviewItems] = useState<ImportPreviewItem[]>([])
  const [selectedIndices, setSelectedIndices] = useState<Set<number>>(new Set())
  const [previewed, setPreviewed] = useState(false)

  const { data: servers = [] } = useServers()
  const { data: runtimes = [] } = useRuntimes()

  const previewMutation = useImportUriPreview({
    onSuccess: (result) => {
      setPreviewItems(result.items)
      setSelectedIndices(new Set(result.items.filter((i) => i.valid).map((i) => i.index)))
      setPreviewed(true)
      toast({
        title: '解析完成',
        description: `有效: ${result.valid_count} 条, 无效: ${result.invalid_count} 条`,
        variant: result.invalid_count > 0 ? 'default' : 'success',
      })
    },
    onError: (err) => {
      toast({
        title: '解析失败',
        description: err.message,
        variant: 'destructive',
      })
    },
  })

  const confirmMutation = useImportUriConfirm({
    onSuccess: (result) => {
      toast({
        title: '导入成功',
        description: `成功创建 ${result.created} 个节点`,
        variant: 'success',
      })
      reset()
      onOpenChange(false)
      onSuccess?.()
    },
    onError: (err) => {
      toast({
        title: '导入失败',
        description: err.message,
        variant: 'destructive',
      })
    },
  })

  function reset() {
    setUris('')
    setServerId('')
    setRuntimeId('')
    setRegion('')
    setMultiplier('1')
    setPreviewItems([])
    setSelectedIndices(new Set())
    setPreviewed(false)
  }

  function handlePreview() {
    const uriList = uris.split('\n').map((u) => u.trim()).filter(Boolean)
    if (uriList.length === 0) {
      toast({ title: '请输入URI', description: '请粘贴至少一条节点URI链接', variant: 'destructive' })
      return
    }
    previewMutation.mutate({
      uris: uriList.join('\n'),
      server_id: serverId || undefined,
      runtime_id: runtimeId || undefined,
      region: region || undefined,
      multiplier: multiplier ? Number(multiplier) : undefined,
    })
  }

  function handleConfirm() {
    if (selectedIndices.size === 0) {
      toast({ title: '未选择节点', description: '请至少选择一个有效节点导入', variant: 'destructive' })
      return
    }
    confirmMutation.mutate({
      items: previewItems,
      selected_indices: Array.from(selectedIndices),
      server_id: serverId || undefined,
      runtime_id: runtimeId || undefined,
      region: region || undefined,
      multiplier: multiplier ? Number(multiplier) : undefined,
    })
  }

  function toggleSelect(index: number) {
    setSelectedIndices((prev) => {
      const next = new Set(prev)
      if (next.has(index)) {
        next.delete(index)
      } else {
        next.add(index)
      }
      return next
    })
  }

  function toggleSelectAll() {
    const validItems = previewItems.filter((i) => i.valid)
    const allSelected = validItems.every((i) => selectedIndices.has(i.index))
    if (allSelected) {
      setSelectedIndices(new Set())
    } else {
      setSelectedIndices(new Set(validItems.map((i) => i.index)))
    }
  }

  const validCount = previewItems.filter((i) => i.valid).length
  const allValidSelected = validCount > 0 && previewItems.filter((i) => i.valid).every((i) => selectedIndices.has(i.index))

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!v) reset(); onOpenChange(v) }}>
      <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-3xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Link2 className="w-5 h-5 text-indigo-400" />
            导入节点链接
          </DialogTitle>
          <DialogDescription className="text-zinc-400">
            支持 vless://, vmess://, trojan://, ss://, hysteria2://, tuic:// 等链接，每行一条
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 pt-2">
          {/* URI 输入区 */}
          <div className="space-y-2">
            <Label className="text-zinc-300">节点URI链接</Label>
            <Textarea
              placeholder="粘贴节点链接，每行一条...&#10;vless://xxx&#10;trojan://yyy&#10;ss://zzz"
              value={uris}
              onChange={(e) => { setUris(e.target.value); setPreviewed(false) }}
              className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500 min-h-[120px] font-mono text-xs"
            />
          </div>

          {/* 批量设置 */}
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <Label className="text-zinc-300 text-sm">关联服务器</Label>
              <select
                value={serverId}
                onChange={(e) => setServerId(e.target.value)}
                className="flex h-9 w-full rounded-lg border border-zinc-700 bg-zinc-800 px-3 py-1 text-sm text-zinc-100 focus:outline-none focus:border-indigo-500"
              >
                <option value="">-- 选择服务器 --</option>
                {servers.map((s) => (
                  <option key={s.id} value={s.id}>{s.name}{s.host ? ` (${s.host})` : ''}</option>
                ))}
              </select>
            </div>
            <div className="space-y-1.5">
              <Label className="text-zinc-300 text-sm">运行时</Label>
              <select
                value={runtimeId}
                onChange={(e) => setRuntimeId(e.target.value)}
                className="flex h-9 w-full rounded-lg border border-zinc-700 bg-zinc-800 px-3 py-1 text-sm text-zinc-100 focus:outline-none focus:border-indigo-500"
              >
                <option value="">-- 选择运行时 --</option>
                {runtimes.map((r) => (
                  <option key={r.id} value={r.id}>{r.name}{r.version ? ` v${r.version}` : ''}</option>
                ))}
              </select>
            </div>
            <div className="space-y-1.5">
              <Label className="text-zinc-300 text-sm">区域</Label>
              <Input
                placeholder="如: HK, JP, US"
                value={region}
                onChange={(e) => setRegion(e.target.value)}
                className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500 h-9"
              />
            </div>
            <div className="space-y-1.5">
              <Label className="text-zinc-300 text-sm">流量倍率</Label>
              <Input
                type="number"
                step="0.1"
                min="0.1"
                value={multiplier}
                onChange={(e) => setMultiplier(e.target.value)}
                className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500 h-9"
              />
            </div>
          </div>

          <Button
            type="button"
            onClick={handlePreview}
            isLoading={previewMutation.isPending}
            className="w-full bg-indigo-600 hover:bg-indigo-500"
          >
            解析预览
          </Button>

          {/* 预览结果 */}
          {previewed && previewItems.length > 0 && (
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3 text-sm">
                  <button
                    type="button"
                    onClick={toggleSelectAll}
                    className="flex items-center gap-1.5 text-zinc-300 hover:text-zinc-100"
                  >
                    <Checkbox checked={allValidSelected} onChange={toggleSelectAll} />
                    <span>全选有效项</span>
                  </button>
                  <span className="text-zinc-500">
                    已选 <span className="text-indigo-400">{selectedIndices.size}</span> / {validCount} 个有效节点
                  </span>
                </div>
              </div>

              <div className="border border-zinc-700 rounded-lg overflow-hidden">
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="bg-zinc-800/80 border-b border-zinc-700">
                        <th className="w-8 p-2"></th>
                        <th className="text-left p-2 text-xs font-medium text-zinc-400">名称</th>
                        <th className="text-left p-2 text-xs font-medium text-zinc-400 hidden sm:table-cell">协议</th>
                        <th className="text-left p-2 text-xs font-medium text-zinc-400 hidden md:table-cell">地址</th>
                        <th className="text-left p-2 text-xs font-medium text-zinc-400 hidden sm:table-cell">端口</th>
                        <th className="text-left p-2 text-xs font-medium text-zinc-400">状态</th>
                      </tr>
                    </thead>
                    <tbody>
                      {previewItems.map((item) => (
                        <tr
                          key={item.index}
                          className={`border-b border-zinc-800 last:border-0 ${
                            item.valid ? 'hover:bg-zinc-800/50 cursor-pointer' : 'bg-red-950/10'
                          }`}
                          onClick={() => item.valid && toggleSelect(item.index)}
                        >
                          <td className="p-2">
                            {item.valid ? (
                              <Checkbox
                                checked={selectedIndices.has(item.index)}
                                onChange={() => toggleSelect(item.index)}
                                onClick={(e) => e.stopPropagation()}
                              />
                            ) : (
                              <XCircle className="w-4 h-4 text-red-400" />
                            )}
                          </td>
                          <td className="p-2">
                            <div className="text-zinc-200 text-xs truncate max-w-[150px]" title={item.name}>
                              {item.name || '(未命名)'}
                            </div>
                            {item.warning && (
                              <div className="text-xs text-amber-400 flex items-center gap-1 mt-0.5">
                                <AlertTriangle className="w-3 h-3" />
                                {item.warning}
                              </div>
                            )}
                          </td>
                          <td className="p-2 hidden sm:table-cell">
                            <Badge variant="outline" className="bg-zinc-800 text-zinc-300 text-[10px]">
                              {item.protocol_type.toUpperCase()}
                            </Badge>
                          </td>
                          <td className="p-2 hidden md:table-cell text-zinc-400 text-xs font-mono">
                            {item.address}
                          </td>
                          <td className="p-2 hidden sm:table-cell text-zinc-400 text-xs">
                            {item.port}
                          </td>
                          <td className="p-2">
                            {item.valid ? (
                              <span className="flex items-center gap-1 text-emerald-400 text-xs">
                                <CheckCircle2 className="w-3.5 h-3.5" />
                                有效
                              </span>
                            ) : (
                              <span className="flex items-center gap-1 text-red-400 text-xs">
                                <XCircle className="w-3.5 h-3.5" />
                                无效
                              </span>
                            )}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => { reset(); onOpenChange(false) }}
            disabled={confirmMutation.isPending}
            className="border-zinc-700 text-zinc-300"
          >
            取消
          </Button>
          {previewed && (
            <Button
              type="button"
              onClick={handleConfirm}
              isLoading={confirmMutation.isPending}
              disabled={selectedIndices.size === 0}
              className="bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50"
            >
              确认导入 ({selectedIndices.size})
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
