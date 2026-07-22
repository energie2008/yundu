import * as React from 'react'
import { Button, Input, Label, Card, CardContent, Badge, useToast } from '@airport/ui'
import {
  ChevronUp,
  ChevronDown,
  Plus,
  Trash2,
  ArrowRight,
  Globe,
  Server,
  Zap,
  Info,
  Link2,
} from 'lucide-react'

export interface ChainHop {
  id: string
  type: 'existing' | 'custom'
  nodeId?: string
  nodeName?: string
  customConfig?: {
    address: string
    port: number
    protocol: string
  }
}

export interface ChainConfig {
  id?: string
  name: string
  landing_node: ChainHop
  relay_nodes: ChainHop[]
  billing_strategy: 'bill_at_landing' | 'bill_at_entry'
  tags: string[]
}

interface ChainEditorProps {
  initialChain?: Partial<ChainConfig>
  availableNodes?: Array<{ id: string; name: string; protocol: string; region: string }>
  onChange?: (chain: ChainConfig) => void
  onSave?: (chain: ChainConfig) => void
}

const BILLING_OPTIONS = [
  { value: 'bill_at_landing', label: '落地节点计费 (推荐)', description: '流量在最终落地节点计费' },
  { value: 'bill_at_entry', label: '入口节点计费', description: '流量在入口节点计费' },
]

function generateHopId(): string {
  return `hop_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`
}

function createDefaultHop(type: 'existing' | 'custom' = 'existing'): ChainHop {
  return {
    id: generateHopId(),
    type,
    nodeId: undefined,
    nodeName: undefined,
    customConfig: type === 'custom' ? { address: '', port: 443, protocol: 'vless' } : undefined,
  }
}

const DEFAULT_CHAIN: ChainConfig = {
  name: '',
  landing_node: createDefaultHop(),
  relay_nodes: [],
  billing_strategy: 'bill_at_landing',
  tags: [],
}

export function ChainEditor({
  initialChain,
  availableNodes = [],
  onChange,
  onSave,
}: ChainEditorProps) {
  const { toast } = useToast()
  const [chain, setChain] = React.useState<ChainConfig>({ ...DEFAULT_CHAIN, ...initialChain })
  const [newTag, setNewTag] = React.useState('')

  React.useEffect(() => {
    onChange?.(chain)
  }, [chain, onChange])

  const updateChain = React.useCallback(<K extends keyof ChainConfig>(key: K, value: ChainConfig[K]) => {
    setChain(prev => ({ ...prev, [key]: value }))
  }, [])

  const updateLandingNode = React.useCallback((updates: Partial<ChainHop>) => {
    setChain(prev => ({
      ...prev,
      landing_node: { ...prev.landing_node, ...updates },
    }))
  }, [])

  const addRelayNode = React.useCallback(() => {
    setChain(prev => ({
      ...prev,
      relay_nodes: [...prev.relay_nodes, createDefaultHop()],
    }))
  }, [])

  const updateRelayNode = React.useCallback((index: number, updates: Partial<ChainHop>) => {
    setChain(prev => ({
      ...prev,
      relay_nodes: prev.relay_nodes.map((hop, i) =>
        i === index ? { ...hop, ...updates } : hop
      ),
    }))
  }, [])

  const removeRelayNode = React.useCallback((index: number) => {
    setChain(prev => ({
      ...prev,
      relay_nodes: prev.relay_nodes.filter((_, i) => i !== index),
    }))
  }, [])

  const moveRelayNode = React.useCallback((index: number, direction: 'up' | 'down') => {
    setChain(prev => {
      const newRelays = [...prev.relay_nodes]
      const newIndex = direction === 'up' ? index - 1 : index + 1
      if (newIndex < 0 || newIndex >= newRelays.length) return prev
      ;[newRelays[index], newRelays[newIndex]] = [newRelays[newIndex], newRelays[index]]
      return { ...prev, relay_nodes: newRelays }
    })
  }, [])

  const addTag = React.useCallback(() => {
    const tag = newTag.trim()
    if (tag && !chain.tags.includes(tag)) {
      setChain(prev => ({ ...prev, tags: [...prev.tags, tag] }))
      setNewTag('')
    }
  }, [newTag, chain.tags])

  const removeTag = React.useCallback((tag: string) => {
    setChain(prev => ({ ...prev, tags: prev.tags.filter(t => t !== tag) }))
  }, [])

  const handleSave = React.useCallback(() => {
    if (!chain.name.trim()) {
      toast({ title: '请输入链路名称', variant: 'destructive' })
      return
    }
    onSave?.(chain)
  }, [chain, onSave, toast])

  const renderHopCard = (
    hop: ChainHop,
    index: number,
    isLanding: boolean,
    onUpdate: (updates: Partial<ChainHop>) => void,
    onRemove?: () => void,
    onMoveUp?: () => void,
    onMoveDown?: () => void,
  ) => {
    const isFirstRelay = index === 0
    const isLastRelay = index === chain.relay_nodes.length - 1

    return (
      <Card key={hop.id} className={`bg-zinc-800/50 border-zinc-700 ${isLanding ? 'border-emerald-700/50' : ''}`}>
        <CardContent className="p-4">
          <div className="flex items-center justify-between mb-3">
            <div className="flex items-center gap-2">
              {isLanding ? (
                <Badge className="bg-emerald-600/20 text-emerald-400 border-emerald-700 gap-1">
                  <Globe className="w-3 h-3" />
                  落地节点
                </Badge>
              ) : (
                <Badge className="bg-indigo-600/20 text-indigo-400 border-indigo-700 gap-1">
                  <Server className="w-3 h-3" />
                  中转 {index + 1}
                </Badge>
              )}
            </div>
            <div className="flex items-center gap-1">
              {!isLanding && (
                <>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="h-7 w-7 p-0 text-zinc-400 hover:text-zinc-200 disabled:opacity-30"
                    onClick={onMoveUp}
                    disabled={isFirstRelay}
                  >
                    <ChevronUp className="w-4 h-4" />
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="h-7 w-7 p-0 text-zinc-400 hover:text-zinc-200 disabled:opacity-30"
                    onClick={onMoveDown}
                    disabled={isLastRelay}
                  >
                    <ChevronDown className="w-4 h-4" />
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="h-7 w-7 p-0 text-red-400 hover:text-red-300 hover:bg-red-950/30"
                    onClick={onRemove}
                  >
                    <Trash2 className="w-4 h-4" />
                  </Button>
                </>
              )}
            </div>
          </div>

          <div className="space-y-3">
            <div className="flex gap-2">
              <Button
                type="button"
                variant={hop.type === 'existing' ? 'secondary' : 'ghost'}
                size="sm"
                className="h-8 text-xs"
                onClick={() => onUpdate({ type: 'existing', customConfig: undefined })}
              >
                选择已有节点
              </Button>
              <Button
                type="button"
                variant={hop.type === 'custom' ? 'secondary' : 'ghost'}
                size="sm"
                className="h-8 text-xs"
                onClick={() => onUpdate({ type: 'custom', nodeId: undefined, nodeName: undefined, customConfig: { address: '', port: 443, protocol: 'vless' } })}
              >
                手动配置
              </Button>
            </div>

            {hop.type === 'existing' ? (
              <div className="space-y-1.5">
                <Label className="text-zinc-400 text-xs">选择节点</Label>
                <select
                  value={hop.nodeId || ''}
                  onChange={(e) => {
                    const node = availableNodes.find(n => n.id === e.target.value)
                    onUpdate({
                      nodeId: e.target.value,
                      nodeName: node?.name,
                    })
                  }}
                  className="flex h-9 w-full rounded-lg border border-zinc-700 bg-zinc-800 px-3 py-1 text-sm text-zinc-100 focus:outline-none focus:border-indigo-500"
                >
                  <option value="">-- 选择节点 --</option>
                  {availableNodes.map(n => (
                    <option key={n.id} value={n.id}>
                      [{n.region}] {n.name} ({n.protocol})
                    </option>
                  ))}
                </select>
                {availableNodes.length === 0 && (
                  <p className="text-xs text-zinc-500">暂无可用节点，请先创建节点</p>
                )}
              </div>
            ) : (
              <div className="space-y-3 p-3 rounded-lg bg-zinc-900/50 border border-zinc-700/50">
                <div className="grid grid-cols-2 gap-2">
                  <div className="space-y-1">
                    <Label className="text-zinc-400 text-xs">地址</Label>
                    <Input
                      value={hop.customConfig?.address || ''}
                      onChange={(e) => onUpdate({
                        customConfig: { ...hop.customConfig!, address: e.target.value }
                      })}
                      placeholder="example.com"
                      className="bg-zinc-800 border-zinc-700 text-zinc-100 h-8 text-sm"
                    />
                  </div>
                  <div className="space-y-1">
                    <Label className="text-zinc-400 text-xs">端口</Label>
                    <Input
                      type="number"
                      value={hop.customConfig?.port || 443}
                      onChange={(e) => onUpdate({
                        customConfig: { ...hop.customConfig!, port: Number(e.target.value) || 443 }
                      })}
                      className="bg-zinc-800 border-zinc-700 text-zinc-100 h-8 text-sm"
                    />
                  </div>
                </div>
                <div className="space-y-1">
                  <Label className="text-zinc-400 text-xs">协议</Label>
                  <select
                    value={hop.customConfig?.protocol || 'vless'}
                    onChange={(e) => onUpdate({
                      customConfig: { ...hop.customConfig!, protocol: e.target.value }
                    })}
                    className="flex h-8 w-full rounded-lg border border-zinc-700 bg-zinc-800 px-3 py-1 text-sm text-zinc-100 focus:outline-none focus:border-indigo-500"
                  >
                    <option value="vless">VLESS</option>
                    <option value="vmess">VMess</option>
                    <option value="trojan">Trojan</option>
                    <option value="ss">Shadowsocks</option>
                  </select>
                </div>
              </div>
            )}
          </div>
        </CardContent>
      </Card>
    )
  }

  const totalHops = chain.relay_nodes.length + 1

  return (
    <div className="space-y-4">
      <div className="space-y-3">
        <div className="space-y-1.5">
          <Label className="text-zinc-300 text-sm">链路名称</Label>
          <Input
            value={chain.name}
            onChange={(e) => updateChain('name', e.target.value)}
            placeholder="如：日本中转-香港落地"
            className="bg-zinc-800 border-zinc-700 text-zinc-100 h-9"
          />
        </div>

        <div className="space-y-1.5">
          <Label className="text-zinc-300 text-sm">标签</Label>
          <div className="flex gap-2">
            <Input
              value={newTag}
              onChange={(e) => setNewTag(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && (e.preventDefault(), addTag())}
              placeholder="添加标签..."
              className="bg-zinc-800 border-zinc-700 text-zinc-100 h-9 flex-1"
            />
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={addTag}
              className="border-zinc-700 text-zinc-300 h-9"
            >
              <Plus className="w-4 h-4 mr-1" />
              添加
            </Button>
          </div>
          {chain.tags.length > 0 && (
            <div className="flex flex-wrap gap-1 mt-2">
              {chain.tags.map(tag => (
                <Badge
                  key={tag}
                  variant="outline"
                  className="gap-1 cursor-pointer border-zinc-700 text-zinc-400 hover:text-red-400 hover:border-red-900"
                  onClick={() => removeTag(tag)}
                >
                  {tag}
                  <span className="text-xs">×</span>
                </Badge>
              ))}
            </div>
          )}
        </div>
      </div>

      <div className="p-3 rounded-lg bg-indigo-950/20 border border-indigo-900/50">
        <div className="flex items-start gap-2">
          <Zap className="w-4 h-4 text-indigo-400 flex-shrink-0 mt-0.5" />
          <div className="text-xs text-indigo-300">
            <strong>内核自动适配</strong>：系统将根据节点协议和传输方式自动选择最佳链式代理实现。Xray使用Dokodemo-door+Outbound链，Sing-box使用Dialer链。
          </div>
        </div>
      </div>

      <div className="space-y-2">
        <Label className="text-zinc-300 text-sm">计费策略</Label>
        <div className="grid grid-cols-2 gap-2">
          {BILLING_OPTIONS.map(opt => (
            <button
              key={opt.value}
              type="button"
              onClick={() => updateChain('billing_strategy', opt.value as ChainConfig['billing_strategy'])}
              className={`p-3 rounded-lg border text-left transition-all ${
                chain.billing_strategy === opt.value
                  ? 'border-indigo-500 bg-indigo-950/30'
                  : 'border-zinc-700 bg-zinc-800/30 hover:border-zinc-600'
              }`}
            >
              <div className="text-sm font-medium text-zinc-100">{opt.label}</div>
              <div className="text-xs text-zinc-500 mt-0.5">{opt.description}</div>
            </button>
          ))}
        </div>
      </div>

      <div className="space-y-4">
        <Label className="text-zinc-300 text-sm">链路拓扑 ({totalHops} 跳)</Label>

        <div className="p-4 rounded-lg bg-zinc-950 border border-zinc-800 overflow-x-auto">
          <div className="flex items-center gap-2 min-w-max">
            <div className="flex items-center gap-2 px-3 py-2 rounded-lg bg-zinc-800 border border-zinc-700">
              <div className="w-8 h-8 rounded-full bg-zinc-700 flex items-center justify-center">
                <span className="text-xs text-zinc-400 font-bold">入口</span>
              </div>
              <span className="text-xs text-zinc-400">用户</span>
            </div>
            <ArrowRight className="w-5 h-5 text-zinc-600 flex-shrink-0" />

            {chain.relay_nodes.map((hop, idx) => (
              <React.Fragment key={hop.id}>
                <div className={`flex items-center gap-2 px-3 py-2 rounded-lg border ${
                  hop.nodeId || (hop.customConfig?.address)
                    ? 'bg-indigo-950/30 border-indigo-800/50'
                    : 'bg-zinc-800/50 border-zinc-700/50'
                }`}>
                  <div className={`w-8 h-8 rounded-full flex items-center justify-center ${
                    hop.nodeId || (hop.customConfig?.address)
                      ? 'bg-indigo-600'
                      : 'bg-zinc-700'
                  }`}>
                    <span className="text-xs text-white font-bold">{idx + 1}</span>
                  </div>
                  <div className="min-w-0">
                    <div className="text-xs text-zinc-200 font-medium truncate max-w-24">
                      {hop.nodeName || hop.customConfig?.address || `中转${idx + 1}`}
                    </div>
                    <div className="text-[10px] text-zinc-500">
                      {hop.type === 'existing' ? '已有' : '自定义'}
                    </div>
                  </div>
                </div>
                <ArrowRight className="w-5 h-5 text-zinc-600 flex-shrink-0" />
              </React.Fragment>
            ))}

            <div className={`flex items-center gap-2 px-3 py-2 rounded-lg border ${
              chain.landing_node.nodeId || chain.landing_node.customConfig?.address
                ? 'bg-emerald-950/30 border-emerald-800/50'
                : 'bg-zinc-800/50 border-zinc-700/50'
            }`}>
              <div className={`w-8 h-8 rounded-full flex items-center justify-center ${
                chain.landing_node.nodeId || chain.landing_node.customConfig?.address
                  ? 'bg-emerald-600'
                  : 'bg-zinc-700'
              }`}>
                <Globe className="w-4 h-4 text-white" />
              </div>
              <div className="min-w-0">
                <div className="text-xs text-zinc-200 font-medium truncate max-w-24">
                  {chain.landing_node.nodeName || chain.landing_node.customConfig?.address || '落地'}
                </div>
                <div className="text-[10px] text-emerald-400">目标网络</div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div className="space-y-3">
        <Label className="text-zinc-300 text-sm">节点配置</Label>

        {chain.relay_nodes.length > 0 && (
          <div className="space-y-2">
            {chain.relay_nodes.map((hop, idx) =>
              renderHopCard(
                hop,
                idx,
                false,
                (updates) => updateRelayNode(idx, updates),
                () => removeRelayNode(idx),
                () => moveRelayNode(idx, 'up'),
                () => moveRelayNode(idx, 'down'),
              )
            )}
          </div>
        )}

        <Button
          type="button"
          variant="outline"
          onClick={addRelayNode}
          className="w-full border-dashed border-zinc-700 text-zinc-400 hover:text-zinc-200 py-6"
        >
          <Plus className="w-4 h-4 mr-2" />
          添加中转节点
        </Button>

        {renderHopCard(
          chain.landing_node,
          -1,
          true,
          updateLandingNode,
        )}
      </div>

      {needsAdvancedConfig(chain) && (
        <div className="flex items-start gap-2 p-3 rounded-lg bg-amber-950/30 border border-amber-900/50">
          <Info className="w-4 h-4 text-amber-400 flex-shrink-0 mt-0.5" />
          <div className="text-xs text-amber-300">
            多跳链路中如果包含混合协议或特殊传输配置，请确保所有节点版本兼容。建议使用相同内核版本的节点构建链路。
          </div>
        </div>
      )}

      <div className="flex items-center gap-2 pt-2">
        {onSave && (
          <Button
            type="button"
            onClick={handleSave}
            className="bg-indigo-600 hover:bg-indigo-500"
          >
            <Link2 className="w-4 h-4 mr-2" />
            保存链路配置
          </Button>
        )}
      </div>
    </div>
  )
}

function needsAdvancedConfig(chain: ChainConfig): boolean {
  return chain.relay_nodes.length >= 2
}
