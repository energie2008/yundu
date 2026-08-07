import { useState } from 'react'
import {
  FileJson, Cpu, Code2, CheckCircle2, XCircle, Play,
  Copy, Layers
} from 'lucide-react'
import {
  Card, CardContent, CardHeader, CardTitle,
  Badge, Button, Textarea,
  Separator, useToast
} from '@airport/ui'
import { api, ApiError } from '@/lib/api'
import { EP } from '@/lib/endpoints'

interface ValidationIssue {
  level: 'error' | 'warning' | 'info'
  kernel: string
  message: string
  field?: string
}

const SAMPLE_SPEC = `{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "code": "P01",
  "name": "vless-tcp-node",
  "protocol": "vless",
  "transport": { "type": "tcp" },
  "security": "none",
  "port": 443,
  "address": "cdn.example.com",
  "server_port": 8445,
  "speed_limit_mbps": 100,
  "device_limit": 3
}`

function Panel({ title, icon, children, actions }: {
  title: string
  icon: React.ReactNode
  children: React.ReactNode
  actions?: React.ReactNode
}) {
  return (
    <Card className="flex flex-col h-full min-h-[500px]">
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between">
          <CardTitle className="text-sm flex items-center gap-2">
            {icon}
            {title}
          </CardTitle>
          {actions}
        </div>
      </CardHeader>
      <CardContent className="flex-1 overflow-auto">
        {children}
      </CardContent>
    </Card>
  )
}

function ConfigViewer({ config, loading }: { config: string; loading?: boolean }) {
  if (loading) {
    return <div className="animate-pulse space-y-2">{Array.from({ length: 8 }).map((_, i) => <div key={i} className="h-4 bg-muted rounded" />)}</div>
  }
  if (!config) {
    return <div className="text-center py-8 text-muted-foreground text-sm">点击「渲染」生成配置</div>
  }
  return (
    <pre className="text-xs font-mono whitespace-pre-wrap break-all bg-muted/30 p-3 rounded-lg border border-border">
      {config}
    </pre>
  )
}

function IssueBadge({ level }: { level: string }) {
  const colors: Record<string, string> = {
    error: 'bg-red-100 text-red-700 border-red-200',
    warning: 'bg-yellow-100 text-yellow-700 border-yellow-200',
    info: 'bg-blue-100 text-blue-700 border-blue-200',
  }
  return <Badge variant="outline" className={`text-xs ${colors[level] || colors.info}`}>{level}</Badge>
}

export default function CompilerWorkbench() {
  const [spec, setSpec] = useState(SAMPLE_SPEC)
  const [kernel, setKernel] = useState('both')
  const [xrayConfig, setXrayConfig] = useState('')
  const [singboxConfig, setSingboxConfig] = useState('')
  const [validation, setValidation] = useState<ValidationIssue[]>([])
  const [rendering, setRendering] = useState(false)
  const [validating, setValidating] = useState(false)
  const { toast } = useToast()

  const handleRender = async () => {
    setRendering(true)
    setXrayConfig('')
    setSingboxConfig('')
    try {
      const parsed = JSON.parse(spec)
      // 后端 NodeValidationRequest 接受 raw_yaml 或 spec（单个 NodeSpec 对象）。
      // 后端 DualKernelValidator 总是双核都渲染校验，前端 kernel 选择仅用于本地展示过滤。
      const resp = await api.post(EP.NODE_VALIDATE, {
        spec: parsed,
      })
      const data = (resp as any)?.data || resp
      if (data?.xray_config) {
        setXrayConfig(JSON.stringify(data.xray_config, null, 2))
      }
      if (data?.sing_box_config) {
        setSingboxConfig(JSON.stringify(data.sing_box_config, null, 2))
      }
      if (data?.errors) {
        setValidation(data.errors)
      }
      toast({ title: '渲染完成' })
    } catch (err) {
      if (err instanceof SyntaxError) {
        toast({ title: 'JSON 解析错误', description: err.message, variant: 'destructive' })
      } else if (err instanceof ApiError) {
        toast({ title: '渲染失败', description: err.message, variant: 'destructive' })
        setValidation([{
          level: 'error',
          kernel: 'both',
          message: err.message,
        }])
      }
    } finally {
      setRendering(false)
    }
  }

  const handleValidate = async () => {
    setValidating(true)
    try {
      const parsed = JSON.parse(spec)
      // 后端 ValidateNode 默认就包含 dry-run 渲染校验，无需额外 dry_run 标志
      const resp = await api.post(EP.NODE_VALIDATE, {
        spec: parsed,
      })
      const data = (resp as any)?.data || resp
      setValidation(data?.errors || [])
      toast({ title: '验证完成', description: `${data?.errors?.length || 0} 个问题` })
    } catch (err) {
      if (err instanceof ApiError) {
        toast({ title: '验证失败', description: err.message, variant: 'destructive' })
      }
    } finally {
      setValidating(false)
    }
  }

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text)
    toast({ title: '已复制到剪贴板' })
  }

  const errorCount = validation.filter(v => v.level === 'error').length
  const warnCount = validation.filter(v => v.level === 'warning').length

  return (
    <div className="space-y-4 p-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <Layers className="w-6 h-6" />
            编译器工作台
          </h1>
          <p className="text-sm text-muted-foreground mt-1">
            NodeSpec IR → 双内核配置渲染可视化 · 支持 dry-run 校验
          </p>
        </div>
        <div className="flex items-center gap-2">
          <select
            value={kernel}
            onChange={(e) => setKernel(e.target.value)}
            className="h-9 w-32 rounded-md border border-input bg-background px-3 text-sm focus:outline-none focus:ring-1 focus:ring-ring"
          >
            <option value="both">双内核</option>
            <option value="xray">仅 Xray</option>
            <option value="sing_box">仅 Sing-box</option>
          </select>
          <Button variant="outline" size="sm" onClick={handleValidate} disabled={validating}>
            <CheckCircle2 className="w-4 h-4 mr-2" />
            {validating ? '验证中...' : '验证'}
          </Button>
          <Button size="sm" onClick={handleRender} disabled={rendering}>
            <Play className="w-4 h-4 mr-2" />
            {rendering ? '渲染中...' : '渲染'}
          </Button>
        </div>
      </div>

      {/* 5-Panel Grid */}
      <div className="grid grid-cols-1 lg:grid-cols-5 gap-4">
        {/* Panel 1: NodeSpec IR Input */}
        <Panel
          title="NodeSpec IR"
          icon={<FileJson className="w-4 h-4" />}
          actions={<Button variant="ghost" size="sm" onClick={() => copyToClipboard(spec)}><Copy className="w-3 h-3" /></Button>}
        >
          <Textarea
            value={spec}
            onChange={(e) => setSpec(e.target.value)}
            className="font-mono text-xs h-full min-h-[400px] resize-none"
            placeholder="输入节点规格 JSON..."
          />
        </Panel>

        {/* Panel 2: Render Parameters */}
        <Panel
          title="渲染参数"
          icon={<Cpu className="w-4 h-4" />}
        >
          <div className="space-y-3 text-sm">
            <div>
              <label className="text-xs text-muted-foreground">内核选择</label>
              <div className="mt-1"><Badge variant="outline">{kernel}</Badge></div>
            </div>
            <Separator />
            <div>
              <label className="text-xs text-muted-foreground">协议</label>
              <div className="mt-1">
                {(() => {
                  try { return <Badge variant="outline">{JSON.parse(spec).protocol || '-'}</Badge> }
                  catch { return <Badge variant="outline">-</Badge> }
                })()}
              </div>
            </div>
            <div>
              <label className="text-xs text-muted-foreground">传输方式</label>
              <div className="mt-1">
                {(() => {
                  try { return <Badge variant="outline">{(JSON.parse(spec).transport?.type) || JSON.parse(spec).transport || '-'}</Badge> }
                  catch { return <Badge variant="outline">-</Badge> }
                })()}
              </div>
            </div>
            <div>
              <label className="text-xs text-muted-foreground">暴露模式</label>
              <div className="mt-1">
                {(() => {
                  try { return <Badge variant="outline">{JSON.parse(spec).exposure_mode || '-'}</Badge> }
                  catch { return <Badge variant="outline">-</Badge> }
                })()}
              </div>
            </div>
            <Separator />
            <div className="text-xs text-muted-foreground">
              <p className="mb-1">渲染流程:</p>
              <p>1. 解析 NodeSpec IR</p>
              <p>2. 选择内核 (xray/sing-box)</p>
              <p>3. 应用 exposure policy</p>
              <p>4. 生成 inbound/outbound</p>
              <p>5. 注入限速/审计规则</p>
              <p>6. dry-run 校验</p>
            </div>
          </div>
        </Panel>

        {/* Panel 3: Xray Config */}
        <Panel
          title="Xray 配置"
          icon={<Code2 className="w-4 h-4 text-blue-500" />}
          actions={xrayConfig ? <Button variant="ghost" size="sm" onClick={() => copyToClipboard(xrayConfig)}><Copy className="w-3 h-3" /></Button> : null}
        >
          <ConfigViewer config={xrayConfig} loading={rendering} />
        </Panel>

        {/* Panel 4: Sing-box Config */}
        <Panel
          title="Sing-box 配置"
          icon={<Code2 className="w-4 h-4 text-purple-500" />}
          actions={singboxConfig ? <Button variant="ghost" size="sm" onClick={() => copyToClipboard(singboxConfig)}><Copy className="w-3 h-3" /></Button> : null}
        >
          <ConfigViewer config={singboxConfig} loading={rendering} />
        </Panel>

        {/* Panel 5: Validation Results */}
        <Panel
          title="校验结果"
          icon={errorCount > 0 ? <XCircle className="w-4 h-4 text-red-500" /> : <CheckCircle2 className="w-4 h-4 text-green-500" />}
          actions={
            <div className="flex items-center gap-1">
              {errorCount > 0 && <Badge variant="outline" className="text-xs bg-red-50 text-red-700">{errorCount} 错误</Badge>}
              {warnCount > 0 && <Badge variant="outline" className="text-xs bg-yellow-50 text-yellow-700">{warnCount} 警告</Badge>}
              {errorCount === 0 && warnCount === 0 && validation.length === 0 && (
                <Badge variant="outline" className="text-xs bg-green-50 text-green-700">通过</Badge>
              )}
            </div>
          }
        >
          {validation.length === 0 ? (
            <div className="text-center py-8 text-muted-foreground text-sm">
              点击「验证」或「渲染」查看校验结果
            </div>
          ) : (
            <div className="space-y-2">
              {validation.map((issue, i) => (
                <div key={i} className="p-2 rounded border border-border text-xs">
                  <div className="flex items-center justify-between mb-1">
                    <IssueBadge level={issue.level} />
                    <Badge variant="outline" className="text-xs">{issue.kernel}</Badge>
                  </div>
                  <p className="text-muted-foreground">{issue.message}</p>
                  {issue.field && <p className="text-muted-foreground/70 mt-1">字段: {issue.field}</p>}
                </div>
              ))}
            </div>
          )}
        </Panel>
      </div>
    </div>
  )
}
