import * as React from 'react'
import { Button, Input, Label, Card, CardContent, Tabs, TabsList, TabsTrigger, TabsContent, Badge, useToast, Switch } from '@airport/ui'
import {
  Plus,
  Trash2,
  Settings,
  Code2,
  ChevronUp,
  ChevronDown,
  Tag,
  Network,
  Save,
  AlertCircle,
} from 'lucide-react'
import { YamlEditor, EditorMode, tryParseJson, tryParseYamlSimple } from '@/components/common/YamlEditor'

export type RuleType = 'geosite' | 'geoip' | 'domain_suffix' | 'domain_keyword' | 'domain' | 'port' | 'ip_cidr' | 'process_name'
export type RuleAction = 'direct' | 'proxy' | 'block' | string

export interface Rule {
  id: string
  type: RuleType
  value: string
  action: RuleAction
  remark?: string
}

export interface NodeGroup {
  id: string
  name: string
  tags: string[]
  node_ids: string[]
}

export interface RuleSetConfig {
  id?: string
  name: string
  rules: Rule[]
  groups: NodeGroup[]
  final_action: 'direct' | 'proxy' | 'block'
  tags: string[]
  enable_log?: boolean
}

interface RuleSetEditorProps {
  initialRuleSet?: Partial<RuleSetConfig>
  availableNodes?: Array<{ id: string; name: string; region: string; tags?: string[] }>
  onChange?: (ruleSet: RuleSetConfig) => void
  onSave?: (ruleSet: RuleSetConfig) => void
  height?: number
}

const RULE_TYPES: Array<{ value: RuleType; label: string; description: string }> = [
  { value: 'geosite', label: 'GeoSite', description: '站点类别 (如: cn, google)' },
  { value: 'geoip', label: 'GeoIP', description: 'IP地理位置 (如: cn, private)' },
  { value: 'domain_suffix', label: '域名后缀', description: '如: example.com' },
  { value: 'domain_keyword', label: '域名关键字', description: '域名中包含关键词' },
  { value: 'domain', label: '完整域名', description: '精确匹配域名' },
  { value: 'port', label: '端口', description: '端口号 (如: 22, 80, 443)' },
  { value: 'ip_cidr', label: 'IP CIDR', description: '如: 192.168.0.0/16' },
  { value: 'process_name', label: '进程名', description: '应用进程名称' },
]

const DEFAULT_ACTIONS: Array<{ value: RuleAction; label: string; color: string }> = [
  { value: 'proxy', label: '代理', color: 'text-indigo-400 border-indigo-700 bg-indigo-950/30' },
  { value: 'direct', label: '直连', color: 'text-emerald-400 border-emerald-700 bg-emerald-950/30' },
  { value: 'block', label: '拒绝', color: 'text-red-400 border-red-700 bg-red-950/30' },
]

const FINAL_ACTIONS = [
  { value: 'proxy', label: '代理', description: '未匹配规则的流量走代理' },
  { value: 'direct', label: '直连', description: '未匹配规则的流量直连' },
  { value: 'block', label: '拒绝', description: '未匹配规则的流量拒绝' },
]

function generateId(prefix: string = ''): string {
  return `${prefix}${Date.now()}_${Math.random().toString(36).slice(2, 8)}`
}

function createDefaultRule(): Rule {
  return {
    id: generateId('rule_'),
    type: 'geosite',
    value: '',
    action: 'proxy',
  }
}

function createDefaultGroup(name: string = '默认组'): NodeGroup {
  return {
    id: generateId('group_'),
    name,
    tags: [],
    node_ids: [],
  }
}

const DEFAULT_RULESET: RuleSetConfig = {
  name: '',
  rules: [
    { id: generateId('rule_'), type: 'geosite', value: 'cn', action: 'direct' },
    { id: generateId('rule_'), type: 'geoip', value: 'cn', action: 'direct' },
    { id: generateId('rule_'), type: 'geoip', value: 'private', action: 'direct' },
  ],
  groups: [createDefaultGroup('代理节点')],
  final_action: 'proxy',
  tags: [],
  enable_log: false,
}

function ruleSetToYaml(ruleSet: RuleSetConfig, mode: EditorMode): string {
  const cleanData: Record<string, unknown> = {
    name: ruleSet.name,
    final_action: ruleSet.final_action,
    rules: ruleSet.rules.map(r => ({
      type: r.type,
      value: r.value,
      action: r.action,
      ...(r.remark ? { remark: r.remark } : {}),
    })),
    groups: ruleSet.groups.map(g => ({
      name: g.name,
      tags: g.tags,
      node_ids: g.node_ids,
    })),
    tags: ruleSet.tags,
    enable_log: ruleSet.enable_log,
  }

  if (mode === 'json') {
    return JSON.stringify(cleanData, null, 2)
  }
  return jsonToYaml(cleanData)
}

function jsonToYaml(obj: unknown, indent: number = 0): string {
  const spaces = '  '.repeat(indent)

  if (obj === null || obj === undefined) return 'null'
  if (typeof obj === 'boolean' || typeof obj === 'number') return String(obj)
  if (typeof obj === 'string') {
    if (obj === '' || /[:#\-\[\]{}&*!|>'"%@`,]/.test(obj)) {
      return JSON.stringify(obj)
    }
    return obj
  }

  if (Array.isArray(obj)) {
    if (obj.length === 0) return '[]'
    return '\n' + obj.map(item => spaces + '- ' + jsonToYaml(item, indent + 1).trimStart()).join('\n')
  }

  if (typeof obj === 'object') {
    const entries = Object.entries(obj as Record<string, unknown>)
    if (entries.length === 0) return '{}'
    return entries.map(([k, v]) => {
      if (typeof v === 'object' && v !== null) {
        return `${spaces}${k}:${jsonToYaml(v, indent + 1).startsWith('\n') ? '' : ' '}${jsonToYaml(v, indent + 1)}`
      }
      return `${spaces}${k}: ${jsonToYaml(v, indent + 1)}`
    }).join('\n')
  }

  return String(obj)
}

function yamlToRuleSet(text: string, mode: EditorMode): { ruleSet: Partial<RuleSetConfig>; valid: boolean; error?: string } {
  if (!text.trim()) return { ruleSet: {}, valid: true }

  let result
  if (mode === 'json') {
    result = tryParseJson(text)
  } else {
    result = tryParseYamlSimple(text)
  }

  if (!result.valid || !result.data) {
    return { ruleSet: {}, valid: false, error: result.error }
  }

  const data = result.data as Record<string, unknown>
  const ruleSet: Partial<RuleSetConfig> = {}

  if (typeof data.name === 'string') ruleSet.name = data.name
  if (typeof data.final_action === 'string') ruleSet.final_action = data.final_action as RuleSetConfig['final_action']
  if (Array.isArray(data.tags)) ruleSet.tags = data.tags.filter(t => typeof t === 'string') as string[]
  if (typeof data.enable_log === 'boolean') ruleSet.enable_log = data.enable_log

  if (Array.isArray(data.rules)) {
    ruleSet.rules = data.rules.map((r: Record<string, unknown>, i: number) => ({
      id: generateId('rule_'),
      type: (r.type as RuleType) || 'geosite',
      value: String(r.value || ''),
      action: (r.action as RuleAction) || 'proxy',
      remark: typeof r.remark === 'string' ? r.remark : undefined,
    }))
  }

  if (Array.isArray(data.groups)) {
    ruleSet.groups = data.groups.map((g: Record<string, unknown>) => ({
      id: generateId('group_'),
      name: String(g.name || '未命名组'),
      tags: Array.isArray(g.tags) ? g.tags.filter(t => typeof t === 'string') as string[] : [],
      node_ids: Array.isArray(g.node_ids) ? g.node_ids.filter(n => typeof n === 'string') as string[] : [],
    }))
  }

  return { ruleSet, valid: true }
}

export function RuleSetEditor({
  initialRuleSet,
  availableNodes = [],
  onChange,
  onSave,
  height = 500,
}: RuleSetEditorProps) {
  const { toast } = useToast()
  const [activeTab, setActiveTab] = React.useState('form')
  const [editorMode, setEditorMode] = React.useState<EditorMode>('json')
  const [ruleSet, setRuleSet] = React.useState<RuleSetConfig>({ ...DEFAULT_RULESET, ...initialRuleSet })
  const [yamlText, setYamlText] = React.useState(() => ruleSetToYaml({ ...DEFAULT_RULESET, ...initialRuleSet }, 'json'))
  const [isSyncing, setIsSyncing] = React.useState(false)
  const [newTag, setNewTag] = React.useState('')
  const [expandedGroup, setExpandedGroup] = React.useState<string | null>(null)

  React.useEffect(() => {
    if (!isSyncing && activeTab === 'yaml') {
      const result = yamlToRuleSet(yamlText, editorMode)
      if (result.valid && result.ruleSet) {
        const newRuleSet = {
          ...DEFAULT_RULESET,
          ...ruleSet,
          ...result.ruleSet,
        } as RuleSetConfig
        const hasChanges = JSON.stringify(newRuleSet) !== JSON.stringify(ruleSet)
        if (hasChanges) {
          setRuleSet(newRuleSet)
          onChange?.(newRuleSet)
        }
      }
    }
  }, [yamlText, editorMode, activeTab])

  React.useEffect(() => {
    if (!isSyncing && activeTab === 'form') {
      setIsSyncing(true)
      setYamlText(ruleSetToYaml(ruleSet, editorMode))
      onChange?.(ruleSet)
      setTimeout(() => setIsSyncing(false), 0)
    }
  }, [ruleSet, activeTab, editorMode])

  const updateRuleSet = React.useCallback(<K extends keyof RuleSetConfig>(key: K, value: RuleSetConfig[K]) => {
    setRuleSet(prev => ({ ...prev, [key]: value }))
  }, [])

  const handleTabChange = React.useCallback((tab: string) => {
    if (tab === activeTab) return

    if (tab === 'yaml') {
      setIsSyncing(true)
      setYamlText(ruleSetToYaml(ruleSet, editorMode))
      setTimeout(() => setIsSyncing(false), 0)
    } else {
      const result = yamlToRuleSet(yamlText, editorMode)
      if (result.valid && result.ruleSet) {
        setRuleSet(prev => ({ ...DEFAULT_RULESET, ...prev, ...result.ruleSet } as RuleSetConfig))
      } else {
        toast({
          title: 'YAML解析错误',
          description: result.error || '请修正语法后再切换',
          variant: 'destructive',
        })
        return
      }
    }
    setActiveTab(tab)
  }, [activeTab, ruleSet, yamlText, editorMode, toast])

  const addRule = React.useCallback(() => {
    setRuleSet(prev => ({
      ...prev,
      rules: [...prev.rules, createDefaultRule()],
    }))
  }, [])

  const updateRule = React.useCallback((index: number, updates: Partial<Rule>) => {
    setRuleSet(prev => ({
      ...prev,
      rules: prev.rules.map((r, i) => i === index ? { ...r, ...updates } : r),
    }))
  }, [])

  const removeRule = React.useCallback((index: number) => {
    setRuleSet(prev => ({
      ...prev,
      rules: prev.rules.filter((_, i) => i !== index),
    }))
  }, [])

  const moveRule = React.useCallback((index: number, direction: 'up' | 'down') => {
    setRuleSet(prev => {
      const newRules = [...prev.rules]
      const newIndex = direction === 'up' ? index - 1 : index + 1
      if (newIndex < 0 || newIndex >= newRules.length) return prev
      ;[newRules[index], newRules[newIndex]] = [newRules[newIndex], newRules[index]]
      return { ...prev, rules: newRules }
    })
  }, [])

  const addGroup = React.useCallback(() => {
    const newGroup = createDefaultGroup(`组${ruleSet.groups.length + 1}`)
    setRuleSet(prev => ({
      ...prev,
      groups: [...prev.groups, newGroup],
    }))
    setExpandedGroup(newGroup.id)
  }, [ruleSet.groups.length])

  const updateGroup = React.useCallback((groupId: string, updates: Partial<NodeGroup>) => {
    setRuleSet(prev => ({
      ...prev,
      groups: prev.groups.map(g => g.id === groupId ? { ...g, ...updates } : g),
    }))
  }, [])

  const removeGroup = React.useCallback((groupId: string) => {
    if (ruleSet.groups.length <= 1) {
      toast({ title: '至少保留一个节点组', variant: 'destructive' })
      return
    }
    setRuleSet(prev => ({
      ...prev,
      groups: prev.groups.filter(g => g.id !== groupId),
    }))
  }, [ruleSet.groups.length, toast])

  const toggleNodeInGroup = React.useCallback((groupId: string, nodeId: string) => {
    setRuleSet(prev => ({
      ...prev,
      groups: prev.groups.map(g => {
        if (g.id !== groupId) return g
        const hasNode = g.node_ids.includes(nodeId)
        return {
          ...g,
          node_ids: hasNode
            ? g.node_ids.filter(id => id !== nodeId)
            : [...g.node_ids, nodeId],
        }
      }),
    }))
  }, [])

  const addTag = React.useCallback(() => {
    const tag = newTag.trim()
    if (tag && !ruleSet.tags.includes(tag)) {
      setRuleSet(prev => ({ ...prev, tags: [...prev.tags, tag] }))
      setNewTag('')
    }
  }, [newTag, ruleSet.tags])

  const removeTag = React.useCallback((tag: string) => {
    setRuleSet(prev => ({ ...prev, tags: prev.tags.filter(t => t !== tag) }))
  }, [])

  const handleSave = React.useCallback(() => {
    if (!ruleSet.name.trim()) {
      toast({ title: '请输入规则集名称', variant: 'destructive' })
      return
    }
    const invalidRules = ruleSet.rules.filter(r => !r.value.trim())
    if (invalidRules.length > 0) {
      toast({ title: `有 ${invalidRules.length} 条规则未填写值`, variant: 'destructive' })
      return
    }
    onSave?.(ruleSet)
  }, [ruleSet, onSave, toast])

  const getActionBadgeClass = (action: RuleAction): string => {
    const preset = DEFAULT_ACTIONS.find(a => a.value === action)
    if (preset) return preset.color
    return 'text-zinc-400 border-zinc-700 bg-zinc-800/50'
  }

  const getAvailableActions = (): Array<{ value: RuleAction; label: string }> => {
    const groupActions = ruleSet.groups.map(g => ({
      value: `group:${g.id}`,
      label: g.name,
    }))
    return [...DEFAULT_ACTIONS.map(a => ({ value: a.value, label: a.label })), ...groupActions]
  }

  return (
    <div className="space-y-4">
      <Tabs value={activeTab} onValueChange={handleTabChange}>
        <TabsList className="w-full grid grid-cols-2">
          <TabsTrigger value="form" className="gap-2">
            <Settings className="w-4 h-4" />
            可视化编辑
          </TabsTrigger>
          <TabsTrigger value="yaml" className="gap-2">
            <Code2 className="w-4 h-4" />
            YAML/JSON编辑
          </TabsTrigger>
        </TabsList>

        <TabsContent value="form" className="space-y-4 mt-4">
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <Label className="text-zinc-300 text-sm">规则集名称</Label>
              <Input
                value={ruleSet.name}
                onChange={(e) => updateRuleSet('name', e.target.value)}
                placeholder="如：全局分流"
                className="bg-zinc-800 border-zinc-700 text-zinc-100 h-9"
              />
            </div>
            <div className="space-y-1.5">
              <Label className="text-zinc-300 text-sm">默认动作</Label>
              <select
                value={ruleSet.final_action}
                onChange={(e) => updateRuleSet('final_action', e.target.value as RuleSetConfig['final_action'])}
                className="flex h-9 w-full rounded-lg border border-zinc-700 bg-zinc-800 px-3 py-1 text-sm text-zinc-100 focus:outline-none focus:border-indigo-500"
              >
                {FINAL_ACTIONS.map(a => (
                  <option key={a.value} value={a.value}>{a.label} - {a.description}</option>
                ))}
              </select>
            </div>
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
            {ruleSet.tags.length > 0 && (
              <div className="flex flex-wrap gap-1 mt-2">
                {ruleSet.tags.map(tag => (
                  <Badge
                    key={tag}
                    variant="outline"
                    className="gap-1 cursor-pointer border-zinc-700 text-zinc-400 hover:text-red-400 hover:border-red-900"
                    onClick={() => removeTag(tag)}
                  >
                    <Tag className="w-3 h-3" />
                    {tag}
                    <span className="text-xs">×</span>
                  </Badge>
                ))}
              </div>
            )}
          </div>

          <div className="flex items-center justify-between p-3 rounded-lg bg-zinc-800/50 border border-zinc-700/50">
            <div>
              <div className="text-sm text-zinc-200">启用规则日志</div>
              <div className="text-xs text-zinc-500">记录规则匹配日志用于调试</div>
            </div>
            <Switch
              checked={ruleSet.enable_log || false}
              onChange={(e) => updateRuleSet('enable_log', e.target.checked)}
            />
          </div>

          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <Label className="text-zinc-300 text-sm">节点组管理</Label>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={addGroup}
                className="border-zinc-700 text-zinc-300 h-8"
              >
                <Plus className="w-4 h-4 mr-1" />
                添加组
              </Button>
            </div>

            <div className="space-y-2">
              {ruleSet.groups.map(group => (
                <Card key={group.id} className="bg-zinc-800/50 border-zinc-700">
                  <CardContent className="p-3">
                    <div className="flex items-center justify-between mb-2">
                      <div className="flex items-center gap-2 flex-1">
                        <Input
                          value={group.name}
                          onChange={(e) => updateGroup(group.id, { name: e.target.value })}
                          className="bg-zinc-900 border-zinc-700 text-zinc-100 h-8 text-sm flex-1 max-w-48"
                        />
                        <Badge variant="outline" className="text-xs border-zinc-700 text-zinc-400">
                          {group.node_ids.length} 节点
                        </Badge>
                      </div>
                      <div className="flex items-center gap-1">
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          className="h-7 w-7 p-0 text-zinc-400 hover:text-zinc-200"
                          onClick={() => setExpandedGroup(expandedGroup === group.id ? null : group.id)}
                        >
                          {expandedGroup === group.id ? <ChevronUp className="w-4 h-4" /> : <ChevronDown className="w-4 h-4" />}
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          className="h-7 w-7 p-0 text-red-400 hover:text-red-300"
                          onClick={() => removeGroup(group.id)}
                        >
                          <Trash2 className="w-4 h-4" />
                        </Button>
                      </div>
                    </div>

                    {expandedGroup === group.id && (
                      <div className="space-y-2 pt-2 border-t border-zinc-700/50">
                        {availableNodes.length === 0 ? (
                          <p className="text-xs text-zinc-500 py-2 text-center">暂无可用节点</p>
                        ) : (
                          <div className="grid grid-cols-2 gap-1 max-h-48 overflow-y-auto">
                            {availableNodes.map(node => {
                              const selected = group.node_ids.includes(node.id)
                              return (
                                <button
                                  key={node.id}
                                  type="button"
                                  onClick={() => toggleNodeInGroup(group.id, node.id)}
                                  className={`flex items-center gap-2 px-2 py-1.5 rounded text-left text-xs transition-colors ${
                                    selected
                                      ? 'bg-indigo-950/50 text-indigo-300 border border-indigo-800/50'
                                      : 'bg-zinc-900/50 text-zinc-400 border border-transparent hover:border-zinc-700'
                                  }`}
                                >
                                  <div className={`w-4 h-4 rounded border flex items-center justify-center flex-shrink-0 ${
                                    selected ? 'bg-indigo-600 border-indigo-500' : 'border-zinc-600'
                                  }`}>
                                    {selected && <span className="text-white text-[10px]">✓</span>}
                                  </div>
                                  <div className="min-w-0">
                                    <div className="truncate">{node.name}</div>
                                    <div className="text-[10px] text-zinc-500">[{node.region}]</div>
                                  </div>
                                </button>
                              )
                            })}
                          </div>
                        )}
                      </div>
                    )}
                  </CardContent>
                </Card>
              ))}
            </div>
          </div>

          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <Label className="text-zinc-300 text-sm">规则列表 ({ruleSet.rules.length} 条)</Label>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={addRule}
                className="border-zinc-700 text-zinc-300 h-8"
              >
                <Plus className="w-4 h-4 mr-1" />
                添加规则
              </Button>
            </div>

            <div className="space-y-2 max-h-96 overflow-y-auto pr-1">
              {ruleSet.rules.map((rule, index) => (
                <Card key={rule.id} className="bg-zinc-800/50 border-zinc-700">
                  <CardContent className="p-3">
                    <div className="flex items-start gap-2">
                      <div className="flex flex-col gap-1 pt-1.5">
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          className="h-6 w-6 p-0 text-zinc-500 hover:text-zinc-300 disabled:opacity-30"
                          onClick={() => moveRule(index, 'up')}
                          disabled={index === 0}
                        >
                          <ChevronUp className="w-3 h-3" />
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          className="h-6 w-6 p-0 text-zinc-500 hover:text-zinc-300 disabled:opacity-30"
                          onClick={() => moveRule(index, 'down')}
                          disabled={index === ruleSet.rules.length - 1}
                        >
                          <ChevronDown className="w-3 h-3" />
                        </Button>
                      </div>

                      <div className="flex-1 grid grid-cols-12 gap-2">
                        <select
                          value={rule.type}
                          onChange={(e) => updateRule(index, { type: e.target.value as RuleType })}
                          className="col-span-3 flex h-8 rounded-lg border border-zinc-700 bg-zinc-900 px-2 py-1 text-xs text-zinc-100 focus:outline-none focus:border-indigo-500"
                        >
                          {RULE_TYPES.map(t => (
                            <option key={t.value} value={t.value}>{t.label}</option>
                          ))}
                        </select>

                        <Input
                          value={rule.value}
                          onChange={(e) => updateRule(index, { value: e.target.value })}
                          placeholder={RULE_TYPES.find(t => t.value === rule.type)?.description || '值'}
                          className="col-span-5 bg-zinc-900 border-zinc-700 text-zinc-100 h-8 text-xs"
                        />

                        <select
                          value={rule.action}
                          onChange={(e) => updateRule(index, { action: e.target.value })}
                          className={`col-span-3 flex h-8 rounded-lg border px-2 py-1 text-xs focus:outline-none focus:border-indigo-500 bg-zinc-900 ${getActionBadgeClass(rule.action)}`}
                        >
                          {getAvailableActions().map(a => (
                            <option key={a.value} value={a.value}>{a.label}</option>
                          ))}
                        </select>
                      </div>

                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        className="h-8 w-8 p-0 text-red-400 hover:text-red-300 flex-shrink-0"
                        onClick={() => removeRule(index)}
                      >
                        <Trash2 className="w-4 h-4" />
                      </Button>
                    </div>

                    <Input
                      value={rule.remark || ''}
                      onChange={(e) => updateRule(index, { remark: e.target.value })}
                      placeholder="备注（可选）"
                      className="mt-2 bg-zinc-900 border-zinc-700 text-zinc-400 h-7 text-xs"
                    />
                  </CardContent>
                </Card>
              ))}

              {ruleSet.rules.length === 0 && (
                <div className="text-center py-8 text-zinc-500 text-sm">
                  <AlertCircle className="w-8 h-8 mx-auto mb-2 opacity-50" />
                  暂无规则，点击"添加规则"开始配置
                </div>
              )}
            </div>
          </div>
        </TabsContent>

        <TabsContent value="yaml" className="mt-4">
          <div className="mb-3 p-3 rounded-lg bg-amber-950/30 border border-amber-900/50">
            <div className="flex items-start gap-2">
              <AlertCircle className="w-4 h-4 text-amber-400 flex-shrink-0 mt-0.5" />
              <div className="text-xs text-amber-300">
                YAML/JSON模式支持直接编辑完整配置。切换回可视化模式时会自动解析并同步。组ID请使用 <code className="bg-amber-950/50 px-1 rounded">group:xxx</code> 格式引用节点组。
              </div>
            </div>
          </div>
          <YamlEditor
            value={yamlText}
            onChange={setYamlText}
            mode={editorMode}
            onModeChange={setEditorMode}
            height={height}
            placeholder="输入分流规则配置..."
          />
        </TabsContent>
      </Tabs>

      <div className="flex items-center gap-2 pt-2">
        {onSave && (
          <Button
            type="button"
            onClick={handleSave}
            className="bg-indigo-600 hover:bg-indigo-500"
          >
            <Save className="w-4 h-4 mr-2" />
            保存规则集
          </Button>
        )}
      </div>
    </div>
  )
}
