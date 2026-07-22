import * as React from 'react'
import { useCreateNode, useProtocolPresets } from '@/lib/hooks'
import { Button, Input, Label, Textarea, Select, Badge, Card, CardContent } from '@airport/ui'
import { DynamicForm, type SchemaField } from '../common/DynamicForm'
import { Loader2, Plus, X, Check, AlertTriangle, RefreshCw, Copy, CheckCircle2, FileCode, Zap } from 'lucide-react'
import { DEFAULT_PRESETS } from '@/data/presets'
import type { PresetTemplate, KernelCompatLevel } from '@/types/preset'
import { BADGE_STYLE } from '@/types/preset'
import { applyPresetToSpec } from './preset-utils'
import { load as yamlLoad, dump as yamlDump } from 'js-yaml'

interface CreateNodeWizardProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess?: () => void
}

function randomString(len: number): string {
  const chars = 'abcdefghijklmnopqrstuvwxyz0123456789'
  let r = ''
  for (let i = 0; i < len; i++) r += chars.charAt(Math.floor(Math.random() * chars.length))
  return r
}

function genUUID(): string {
  if (typeof crypto.randomUUID === 'function') return crypto.randomUUID()
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0
    return (c === 'x' ? r : (r & 0x3) | 0x8).toString(16)
  })
}

function genShortId(): string {
  return Math.random() < 0.5 ? '' : randomString(2 + Math.floor(Math.random() * 12)).padEnd(2, '0')
}

interface NodeForm {
  numeric_id: number
  name: string
  protocol: string
  type: string
  address: string
  port: number
  client_port: number
  server_port: number
  transport: string
  security: string
  runtime: 'xray' | 'singbox'
  path: string
  host: string
  service_name: string
  sni: string
  alpn: string
  utls_fingerprint: string
  short_id: string
  public_key: string
  private_key: string
  spider_x: string
  xhttp_mode: string
  uuid: string
  password: string
  method: string
  is_udp: boolean
  is_enabled: boolean
  is_visible: boolean
  traffic_rate: number
  preset_id: string | null
  advanced: Record<string, unknown>
}

function makeDefaultForm(): NodeForm {
  return {
    numeric_id: Math.floor(Math.random() * 90000) + 10000,
    name: '',
    protocol: 'vless',
    type: 'vless',
    address: '',
    port: 443,
    client_port: 443,
    server_port: 443,
    transport: 'tcp',
    security: 'reality',
    runtime: 'xray',
    path: '/' + randomString(20),
    host: '',
    service_name: randomString(12),
    sni: 'www.microsoft.com',
    alpn: 'h2,http/1.1',
    utls_fingerprint: 'chrome',
    short_id: genShortId(),
    public_key: '',
    private_key: '',
    spider_x: '/',
    xhttp_mode: 'auto',
    uuid: genUUID(),
    password: genUUID().replace(/-/g, ''),
    method: 'chacha20-ietf-poly1305',
    is_udp: true,
    is_enabled: true,
    is_visible: true,
    traffic_rate: 1.0,
    preset_id: null,
    advanced: {}
  }
}

function buildConfigSchema(protocol: string, transport: string, security: string): SchemaField {
  const props: Record<string, SchemaField> = {}

  if (['vless', 'vmess', 'tuic'].includes(protocol)) {
    props.uuid = { type: 'string', title: 'UUID', description: '用户唯一标识', autoGenerate: 'uuid' }
  }
  if (['trojan', 'shadowsocks', 'hysteria2', 'tuic'].includes(protocol)) {
    props.password = { type: 'string', title: '密码', description: '认证密码', autoGenerate: 'password', secret: true }
  }
  if (protocol === 'shadowsocks') {
    props.method = {
      type: 'string', title: '加密方式', enum: ['aes-256-gcm', 'chacha20-ietf-poly1305', '2022-blake3-aes-128-gcm', '2022-blake3-aes-256-gcm', '2022-blake3-chacha20-poly1305'],
      enumNames: { 'aes-256-gcm': 'AES-256-GCM', 'chacha20-ietf-poly1305': 'ChaCha20-Poly1305', '2022-blake3-aes-128-gcm': '2022-Blake3-AES-128', '2022-blake3-aes-256-gcm': '2022-Blake3-AES-256', '2022-blake3-chacha20-poly1305': '2022-Blake3-ChaCha20' },
      default: 'chacha20-ietf-poly1305'
    }
  }
  if (protocol === 'vless' && security === 'none') {
    props.flow = { type: 'string', title: 'Flow控制', enum: ['', 'xtls-rprx-vision'], enumNames: { '': '无', 'xtls-rprx-vision': 'XTLS Vision' }, default: '' }
  }
  if (tuicOrH3(protocol)) {
    props.congestion_control = { type: 'string', title: '拥塞控制', enum: ['bbr', 'cubic', 'reno'], default: 'bbr' }
  }

  if (['ws', 'xhttp', 'httpupgrade', 'h2', 'http'].includes(transport)) {
    props.path = { type: 'string', title: '路径 (Path)', description: 'WebSocket/HTTP路径', autoGenerate: 'path', default: '/' }
    props.host = { type: 'string', title: 'Host', description: '自定义Host头（可选）', default: '' }
  }
  if (transport === 'grpc') {
    props.service_name = { type: 'string', title: 'Service Name', description: 'gRPC服务名', default: randomString(12), pattern: '^[a-zA-Z0-9._-]+$' }
  }
  if (transport === 'xhttp') {
    props.xhttp_mode = { type: 'string', title: 'XHTTP模式', enum: ['auto', 'packet-up', 'stream-up', 'stream-one'], enumNames: { 'auto': 'Auto (推荐)', 'packet-up': 'Packet-Up', 'stream-up': 'Stream-Up', 'stream-one': 'Stream-One' }, default: 'auto' }
  }

  if (security === 'tls') {
    props.sni = { type: 'string', title: 'SNI', description: 'TLS服务器名称指示', default: '' }
    props.alpn = { type: 'string', title: 'ALPN', description: '逗号分隔，如: h2,http/1.1', default: 'h2,http/1.1' }
    props.utls_fingerprint = { type: 'string', title: 'uTLS指纹', enum: ['', 'chrome', 'firefox', 'safari', 'edge', 'random'], enumNames: { '': '无指纹', 'chrome': 'Chrome', 'firefox': 'Firefox', 'safari': 'Safari', 'edge': 'Edge', 'random': '随机' }, default: 'chrome' }
    props.allow_insecure = { type: 'boolean', title: '允许不安全证书', description: '跳过证书验证（仅测试用）', default: false }
  }
  if (security === 'reality') {
    props.sni = { type: 'string', title: 'SNI / 伪装站点', description: 'REALITY伪装站点域名', default: 'www.microsoft.com' }
    props.utls_fingerprint = { type: 'string', title: 'uTLS指纹', enum: ['chrome', 'firefox', 'safari', 'edge', 'random', 'ios'], default: 'chrome' }
    props.public_key = { type: 'string', title: '公钥 (Public Key)', description: 'REALITY公钥，部署VPS后自动获取', autoGenerate: 'base64key', sensitive: true }
    props.private_key = { type: 'string', title: '私钥 (Private Key)', description: 'REALITY私钥，VPS端自动生成', sensitive: true, default: '' }
    props.short_id = { type: 'string', title: 'Short ID', description: '可留空，建议随机2-16位十六进制', default: '', pattern: '^[0-9a-fA-F]{0,16}$' }
    props.spider_x = { type: 'string', title: 'Spider X', description: '爬虫路径（可选）', default: '/' }
  }

  if (protocol === 'hysteria2') {
    props.up_mbps = { type: 'integer', title: '上行带宽(Mbps)', default: 100, minimum: 1 }
    props.down_mbps = { type: 'integer', title: '下行带宽(Mbps)', default: 100, minimum: 1 }
    props.obfs_password = { type: 'string', title: '混淆密码', autoGenerate: 'password', secret: true, default: '' }
  }

  return { type: 'object', properties: props }
}

function tuicOrH3(p: string) {
  return p === 'tuic' || p === 'hysteria2'
}

function buildXrayOutbound(f: NodeForm): Record<string, unknown> {
  const outbound: Record<string, unknown> = {
    protocol: f.protocol,
    tag: f.name || 'proxy',
  }

  const settings: Record<string, unknown> = {}
  if (['vless', 'vmess'].includes(f.protocol)) {
    const user: Record<string, unknown> = { id: f.uuid, flow: (f as unknown as Record<string, unknown>).flow || '' }
    if (f.protocol === 'vmess') {
      user.v = 'auto'
      user.scy = 'auto'
    }
    settings.vnext = [{ address: f.address || 'YOUR_SERVER_IP', port: f.port, users: [user] }]
  } else if (f.protocol === 'trojan') {
    settings.servers = [{ address: f.address || 'YOUR_SERVER_IP', port: f.port, password: f.password }]
  } else if (f.protocol === 'shadowsocks') {
    settings.servers = [{ address: f.address || 'YOUR_SERVER_IP', port: f.port, method: f.method, password: f.password }]
  } else if (f.protocol === 'tuic') {
    settings.servers = [{ address: f.address || 'YOUR_SERVER_IP', port: f.port, uuid: f.uuid, password: f.password }]
  } else if (f.protocol === 'hysteria2') {
    settings.servers = [{ address: f.address || 'YOUR_SERVER_IP', port: f.port, password: f.password }]
  }
  outbound.settings = settings

  const streamSettings: Record<string, unknown> = { network: f.transport }
  if (f.security !== 'none') {
    streamSettings.security = f.security
    const secObj: Record<string, unknown> = {}
    if (f.security === 'tls') {
      secObj.serverName = f.sni
      secObj.alpn = f.alpn.split(',').map(s => s.trim()).filter(Boolean)
      if (f.utls_fingerprint) secObj.fingerprint = f.utls_fingerprint
      secObj.allowInsecure = !!(f.advanced.allow_insecure)
    } else if (f.security === 'reality') {
      secObj.serverName = f.sni
      secObj.fingerprint = f.utls_fingerprint
      secObj.publicKey = f.public_key
      secObj.shortId = f.short_id
      secObj.spiderX = f.spider_x
    }
    streamSettings[f.security] = secObj
  } else {
    streamSettings.security = 'none'
  }

  if (f.transport === 'ws') {
    streamSettings.ws = { path: f.path, headers: f.host ? { Host: f.host } : undefined }
  } else if (f.transport === 'grpc') {
    streamSettings.grpc = { serviceName: f.service_name, multiMode: true }
  } else if (f.transport === 'xhttp') {
    streamSettings.xhttp = { path: f.path, host: f.host || undefined, mode: f.xhttp_mode }
  } else if (f.transport === 'httpupgrade') {
    streamSettings.httpupgrade = { path: f.path, host: f.host || undefined }
  } else if (f.transport === 'h2') {
    streamSettings.http = { path: f.path, host: f.host ? [f.host] : undefined }
  }

  outbound.streamSettings = streamSettings
  return { outbounds: [outbound] }
}

const steps = [
  { id: 'basic', label: '基础信息' },
  { id: 'protocol', label: '协议参数' },
  { id: 'preview', label: '预览确认' }
]

const compatMetaMap: Record<KernelCompatLevel, { xray: boolean; singbox: boolean; label: string; color: string }> = {
  both: { xray: true, singbox: true, label: '双内核', color: 'text-emerald-400' },
  xray_only: { xray: true, singbox: false, label: '仅Xray', color: 'text-blue-400' },
  singbox_only: { xray: false, singbox: true, label: '仅Sing-box', color: 'text-violet-400' },
  experimental: { xray: true, singbox: true, label: '实验性', color: 'text-amber-400' },
}

export function CreateNodeWizard({ open, onOpenChange, onSuccess }: CreateNodeWizardProps) {
  const createNode = useCreateNode()
  // 从后端获取预设（失败时回退到本地 DEFAULT_PRESETS，保证 UI 可用）
  const presetsQuery = useProtocolPresets()
  const presets = React.useMemo<PresetTemplate[]>(
    () => (presetsQuery.data && presetsQuery.data.length > 0 ? presetsQuery.data : DEFAULT_PRESETS),
    [presetsQuery.data]
  )
  const [step, setStep] = React.useState(0)
  const [yamlMode, setYamlMode] = React.useState(false)
  const [yamlText, setYamlText] = React.useState('')
  const [yamlError, setYamlError] = React.useState<string | null>(null)
  const [copied, setCopied] = React.useState(false)
  const [form, setForm] = React.useState<NodeForm>(makeDefaultForm)

  const configSchema = React.useMemo(
    () => buildConfigSchema(form.protocol, form.transport, form.security),
    [form.protocol, form.transport, form.security]
  )

  const formConfig: Record<string, unknown> = React.useMemo(() => {
    const c: Record<string, unknown> = { ...form.advanced }
    if ('uuid' in configSchema.properties!) c.uuid = form.uuid
    if ('password' in configSchema.properties!) c.password = form.password
    if ('method' in configSchema.properties!) c.method = form.method
    if ('path' in configSchema.properties!) c.path = form.path
    if ('host' in configSchema.properties!) c.host = form.host
    if ('service_name' in configSchema.properties!) c.service_name = form.service_name
    if ('sni' in configSchema.properties!) c.sni = form.sni
    if ('alpn' in configSchema.properties!) c.alpn = form.alpn
    if ('utls_fingerprint' in configSchema.properties!) c.utls_fingerprint = form.utls_fingerprint
    if ('short_id' in configSchema.properties!) c.short_id = form.short_id
    if ('public_key' in configSchema.properties!) c.public_key = form.public_key
    if ('private_key' in configSchema.properties!) c.private_key = form.private_key
    if ('spider_x' in configSchema.properties!) c.spider_x = form.spider_x
    if ('xhttp_mode' in configSchema.properties!) c.xhttp_mode = form.xhttp_mode
    return c
  }, [form, configSchema])

  const fullConfig = React.useMemo(() => buildXrayOutbound(form), [form])
  const configJson = React.useMemo(() => JSON.stringify(fullConfig, null, 2), [fullConfig])

  React.useEffect(() => {
    if (yamlMode && open) {
      try {
        setYamlText(yamlDump({ ...form, config: fullConfig }, { indent: 2, lineWidth: 120 }))
        setYamlError(null)
      } catch (e) {
        setYamlError('YAML序列化失败: ' + String(e))
      }
    }
  }, [yamlMode, form, fullConfig, open])

  const update = <K extends keyof NodeForm>(key: K, val: NodeForm[K]) => {
    setForm(prev => ({ ...prev, [key]: val }))
  }

  const handleConfigChange = (val: Record<string, unknown>) => {
    setForm(prev => {
      const next = { ...prev, advanced: { ...prev.advanced } }
      for (const [k, v] of Object.entries(val)) {
        if (k === 'uuid') next.uuid = String(v ?? '')
        else if (k === 'password') next.password = String(v ?? '')
        else if (k === 'method') next.method = String(v ?? next.method)
        else if (k === 'path') next.path = String(v ?? '/')
        else if (k === 'host') next.host = String(v ?? '')
        else if (k === 'service_name') next.service_name = String(v ?? '')
        else if (k === 'sni') next.sni = String(v ?? '')
        else if (k === 'alpn') next.alpn = String(v ?? '')
        else if (k === 'utls_fingerprint') next.utls_fingerprint = String(v ?? 'chrome')
        else if (k === 'short_id') next.short_id = String(v ?? '')
        else if (k === 'public_key') next.public_key = String(v ?? '')
        else if (k === 'spider_x') next.spider_x = String(v ?? '/')
        else if (k === 'xhttp_mode') next.xhttp_mode = String(v ?? 'auto')
        else next.advanced[k] = v
      }
      return next
    })
  }

  const applyPreset = (preset: PresetTemplate) => {
    const spec = applyPresetToSpec(preset)
    setForm(prev => {
      const nf = { ...prev }
      nf.protocol = spec.protocol || preset.protocol
      nf.transport = spec.transport || preset.transport
      nf.security = spec.security || preset.security
      nf.type = nf.protocol
      nf.preset_id = preset.id
      nf.traffic_rate = spec.multiplier || 1.0
      nf.is_visible = spec.is_visible !== false
      if (spec.port) nf.port = spec.port
      if (spec.client_port) nf.client_port = spec.client_port
      if (spec.server_port) nf.server_port = spec.server_port
      if (spec.path !== undefined) nf.path = spec.path || '/' + randomString(20)
      if (spec.host !== undefined) nf.host = spec.host || ''
      if (spec.service_name) nf.service_name = spec.service_name
      if (spec.sni !== undefined) nf.sni = spec.sni
      if (spec.alpn !== undefined) nf.alpn = spec.alpn
      if (spec.utls_fingerprint) nf.utls_fingerprint = spec.utls_fingerprint
      if (spec.reality_utls_fingerprint) nf.utls_fingerprint = spec.reality_utls_fingerprint
      if (spec.short_id !== undefined) nf.short_id = spec.short_id || genShortId()
      if (spec.public_key) nf.public_key = spec.public_key
      if (spec.private_key) nf.private_key = spec.private_key
      if (spec.spider_x) nf.spider_x = spec.spider_x
      if (spec.xhttp_mode) nf.xhttp_mode = spec.xhttp_mode
      if (spec.uuid) nf.uuid = spec.uuid; else nf.uuid = genUUID()
      if (spec.password) nf.password = spec.password; else if (['trojan', 'shadowsocks', 'hysteria2'].includes(nf.protocol)) nf.password = genUUID().replace(/-/g, '')
      if (spec.method) nf.method = spec.method

      if (preset.kernel_compat === 'xray_only') nf.runtime = 'xray'
      else if (preset.kernel_compat === 'singbox_only') nf.runtime = 'singbox'

      if (!nf.uuid && ['vless', 'vmess', 'tuic'].includes(nf.protocol)) nf.uuid = genUUID()
      if (!nf.path) nf.path = '/' + randomString(20)
      if (!nf.service_name && nf.transport === 'grpc') nf.service_name = randomString(12)
      if (!nf.short_id && nf.security === 'reality') nf.short_id = genShortId()
      return nf
    })
    setStep(1)
  }

  const regenerateAutoFields = () => {
    setForm(prev => {
      const nf = { ...prev }
      nf.numeric_id = Math.floor(Math.random() * 90000) + 10000
      nf.uuid = genUUID()
      nf.password = genUUID().replace(/-/g, '')
      nf.path = '/' + randomString(20 + Math.floor(Math.random() * 12))
      nf.service_name = randomString(12)
      nf.short_id = genShortId()
      nf.spider_x = '/' + randomString(6)
      return nf
    })
  }

  const handleYamlChange = (value: string) => {
    setYamlText(value)
    try {
      const parsed = yamlLoad(value) as Record<string, unknown>
      setYamlError(null)
      if (parsed && typeof parsed === 'object') {
        setForm(prev => ({
          ...prev,
          name: String(parsed.name || prev.name),
          address: String(parsed.address || prev.address),
          port: Number(parsed.port || prev.port),
          protocol: String(parsed.protocol || prev.protocol),
          transport: String(parsed.transport || prev.transport),
          security: String(parsed.security || prev.security),
          numeric_id: Number(parsed.numeric_id || prev.numeric_id)
        }))
      }
    } catch (e) {
      setYamlError('YAML解析错误: ' + String(e))
    }
  }

  const handleCopyConfig = async () => {
    try {
      await navigator.clipboard.writeText(configJson)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {}
  }

  const handleClose = () => {
    onOpenChange(false)
    setStep(0)
    setYamlMode(false)
    setTimeout(() => setForm(makeDefaultForm()), 200)
  }

  const handleSubmit = async () => {
    try {
      const payload: Record<string, unknown> = {
        name: form.name,
        protocol_type: form.protocol,
        type: form.type,
        address: form.address || '0.0.0.0',
        port: form.port,
        transport_type: form.transport,
        security_type: form.security,
        is_enabled: form.is_enabled,
        enable_udp: form.is_udp,
        is_visible: form.is_visible,
        multiplier: form.traffic_rate,
        config: JSON.stringify({
          uuid: form.uuid,
          password: form.password,
          method: form.method,
          path: form.path,
          host: form.host,
          service_name: form.service_name,
          sni: form.sni,
          alpn: form.alpn,
          utls_fingerprint: form.utls_fingerprint,
          short_id: form.short_id,
          public_key: form.public_key,
          private_key: form.private_key,
          spider_x: form.spider_x,
          xhttp_mode: form.xhttp_mode,
          client_port: form.client_port,
          server_port: form.server_port,
          preset_id: form.preset_id,
          runtime: form.runtime,
          ...form.advanced
        }),
        settings: JSON.stringify({}),
        stream_settings: JSON.stringify({}),
        tags: JSON.stringify([])
      }
      await createNode.mutateAsync(payload)
      onSuccess?.()
      handleClose()
    } catch (error) {
      console.error('Failed to create node:', error)
    }
  }

  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4">
      <div className="bg-zinc-900 border border-zinc-800 rounded-xl w-full max-w-5xl max-h-[90vh] overflow-hidden flex flex-col">
        <div className="flex items-center justify-between px-6 py-4 border-b border-zinc-800">
          <div>
            <h2 className="text-lg font-semibold text-zinc-100">新建节点</h2>
            <p className="text-sm text-zinc-500 mt-0.5">步骤 {step + 1}/{steps.length}：{steps[step].label}</p>
          </div>
          <button onClick={handleClose} className="p-2 text-zinc-400 hover:text-zinc-100 hover:bg-zinc-800 rounded-lg">
            <X className="w-5 h-5" />
          </button>
        </div>

        <div className="flex gap-1 px-6 py-3 border-b border-zinc-800 bg-zinc-900/50">
          {steps.map((s, i) => (
            <React.Fragment key={s.id}>
              <button
                onClick={() => i <= step && setStep(i)}
                className={`flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium transition-colors ${
                  i === step ? 'bg-indigo-500/20 text-indigo-400' : i < step ? 'text-emerald-400 hover:bg-zinc-800 cursor-pointer' : 'text-zinc-500 cursor-default'
                }`}
              >
                <span className={`w-6 h-6 rounded-full flex items-center justify-center text-xs ${
                  i === step ? 'bg-indigo-500 text-white' : i < step ? 'bg-emerald-500/20 text-emerald-400' : 'bg-zinc-800 text-zinc-500'
                }`}>
                  {i < step ? <Check className="w-3 h-3" /> : i + 1}
                </span>
                {s.label}
              </button>
              {i < steps.length - 1 && <div className={`flex-1 h-px my-auto ${i < step ? 'bg-emerald-800' : 'bg-zinc-800'}`} />}
            </React.Fragment>
          ))}
        </div>

        <div className="flex-1 overflow-y-auto p-6 scrollbar-thin">
          {step === 0 && (
            <div className="space-y-6">
              <div>
                <h3 className="text-sm font-medium text-zinc-200 mb-3 flex items-center gap-2">
                  <Zap className="w-4 h-4 text-indigo-400" />
                  快速预设（推荐）
                </h3>
                <p className="text-xs text-zinc-500 mb-4">选择预设将自动填充协议参数、端口、UUID、密钥等所有字段，VPS部署后自动回填服务器IP，无需手动配置</p>
                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
                  {presets.map(preset => {
                    const compat = compatMetaMap[preset.kernel_compat]
                    const badgeStyle = preset.badge ? BADGE_STYLE[preset.badge] : null
                    return (
                      <button
                        key={preset.id}
                        onClick={() => applyPreset(preset)}
                        className={`text-left p-4 rounded-lg border transition-all hover:scale-[1.02] ${
                          form.preset_id === preset.id ? 'border-indigo-500 bg-indigo-500/10' : 'border-zinc-800 bg-zinc-800/50 hover:border-zinc-700'
                        }`}
                      >
                        <div className="flex items-center gap-2 mb-2 flex-wrap">
                          <span className="font-medium text-zinc-100">{preset.name}</span>
                          {badgeStyle && (
                            <Badge className={`${badgeStyle.bg} ${badgeStyle.color} border text-[10px] px-1.5 py-0`}>
                              {preset.badge}
                            </Badge>
                          )}
                        </div>
                        <p className="text-xs text-zinc-400 mb-3 line-clamp-2">{preset.description}</p>
                        <div className="flex items-center gap-1.5 flex-wrap">
                          <Badge variant="outline" className="text-[10px]">{preset.protocol.toUpperCase()}</Badge>
                          <Badge variant="outline" className="text-[10px]">{preset.transport}</Badge>
                          <Badge variant="outline" className="text-[10px]">{preset.security}</Badge>
                          <Badge variant="outline" className={`text-[10px] ${compat.color}`}>{compat.label}</Badge>
                        </div>
                        {preset.warnings && preset.warnings.length > 0 && (
                          <div className="mt-2 flex items-start gap-1 text-[10px] text-amber-400">
                            <AlertTriangle className="w-3 h-3 mt-0.5 flex-shrink-0" />
                            <span>{preset.warnings[0]}</span>
                          </div>
                        )}
                      </button>
                    )
                  })}
                </div>
              </div>

              <div className="relative flex items-center gap-4">
                <div className="flex-1 h-px bg-zinc-800" />
                <span className="text-xs text-zinc-600">或手动配置</span>
                <div className="flex-1 h-px bg-zinc-800" />
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div className="space-y-1.5">
                  <Label className="text-zinc-300 text-sm">节点名称 <span className="text-red-400">*</span></Label>
                  <Input value={form.name} onChange={e => update('name', e.target.value)} placeholder="例如：香港BGP-01" className="bg-zinc-800 border-zinc-700 text-zinc-100" />
                </div>
                <div className="space-y-1.5">
                  <Label className="text-zinc-300 text-sm">数字ID (5位)</Label>
                  <div className="flex gap-2">
                    <Input type="number" value={form.numeric_id} onChange={e => update('numeric_id', Number(e.target.value))} className="bg-zinc-800 border-zinc-700 text-zinc-100 flex-1" min={10000} max={99999} />
                    <Button type="button" variant="ghost" size="sm" onClick={regenerateAutoFields} className="px-2 text-zinc-400 hover:text-indigo-400" title="重新生成所有自动字段">
                      <RefreshCw className="w-4 h-4" />
                    </Button>
                  </div>
                  <p className="text-[10px] text-zinc-500">与XBoard Node交互使用，已自动生成</p>
                </div>
                <div className="space-y-1.5">
                  <Label className="text-zinc-300 text-sm">协议类型</Label>
                  <Select value={form.protocol} onChange={e => { update('protocol', e.target.value); update('type', e.target.value); update('preset_id', null) }}>
                    {['vless', 'vmess', 'trojan', 'shadowsocks', 'tuic', 'hysteria2'].map(p =>
                      <option key={p} value={p}>{p.toUpperCase()}{p === 'ss' ? ' (Shadowsocks)' : ''}</option>
                    )}
                  </Select>
                </div>
                <div className="space-y-1.5">
                  <Label className="text-zinc-300 text-sm">内核运行时</Label>
                  <Select value={form.runtime} onChange={e => update('runtime', e.target.value as 'xray' | 'singbox')}>
                    <option value="xray">Xray-core</option>
                    <option value="singbox">Sing-box</option>
                  </Select>
                </div>
                <div className="space-y-1.5">
                  <Label className="text-zinc-300 text-sm">传输协议</Label>
                  <Select value={form.transport} onChange={e => { update('transport', e.target.value); update('preset_id', null) }}>
                    {['tcp', 'ws', 'grpc', 'httpupgrade', 'xhttp', 'h2', 'quic'].map(t => <option key={t} value={t}>{t.toUpperCase()}</option>)}
                  </Select>
                </div>
                <div className="space-y-1.5">
                  <Label className="text-zinc-300 text-sm">传输层安全</Label>
                  <Select value={form.security} onChange={e => { update('security', e.target.value); update('preset_id', null) }}>
                    <option value="none">无 (None)</option>
                    <option value="tls">TLS</option>
                    <option value="reality">REALITY</option>
                  </Select>
                </div>
                <div className="space-y-1.5">
                  <Label className="text-zinc-300 text-sm">端口</Label>
                  <Input type="number" value={form.port} onChange={e => update('port', Number(e.target.value))} className="bg-zinc-800 border-zinc-700 text-zinc-100" />
                </div>
                <div className="space-y-1.5">
                  <Label className="text-zinc-300 text-sm">UDP转发</Label>
                  <div className="flex items-center gap-3 pt-2">
                    <label className="flex items-center gap-2 text-sm text-zinc-300 cursor-pointer">
                      <input type="checkbox" checked={form.is_udp} onChange={e => update('is_udp', e.target.checked)} className="rounded border-zinc-600 bg-zinc-800" />
                      启用UDP
                    </label>
                  </div>
                </div>
              </div>

              <div className="p-3 rounded-lg bg-indigo-500/10 border border-indigo-500/20 flex items-start gap-2">
                <CheckCircle2 className="w-4 h-4 text-indigo-400 mt-0.5 flex-shrink-0" />
                <div className="text-xs text-indigo-300">
                  <p className="font-medium mb-1">全自动填写说明</p>
                  <p className="text-zinc-400">UUID、密码、WS路径、gRPC服务名、REALITY ShortID等所有非VPS相关字段将在下一步自动生成，无需手动填写。REALITY公钥/私钥、服务器IP会在VPS部署完成后自动回填。</p>
                </div>
              </div>
            </div>
          )}

          {step === 1 && (
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <div>
                  <h3 className="text-sm font-medium text-zinc-200">协议参数配置</h3>
                  <p className="text-xs text-zinc-500 mt-0.5">所有字段已自动生成，可按需调整；标记为"可自动生成"的字段可点刷新按钮重新生成</p>
                </div>
                <div className="flex items-center gap-2">
                  <Button type="button" variant="ghost" size="sm" onClick={regenerateAutoFields} className="text-zinc-400 hover:text-indigo-400">
                    <RefreshCw className="w-4 h-4 mr-1" />重新生成密钥
                  </Button>
                  <Button type="button" variant="ghost" size="sm" onClick={() => setYamlMode(!yamlMode)} className="text-zinc-400 hover:text-indigo-400">
                    <FileCode className="w-4 h-4 mr-1" />{yamlMode ? '表单模式' : 'YAML编辑'}
                  </Button>
                </div>
              </div>

              {yamlMode ? (
                <div className="space-y-3">
                  <div className="p-3 rounded-lg bg-amber-500/10 border border-amber-500/20 flex items-start gap-2">
                    <AlertTriangle className="w-4 h-4 text-amber-400 mt-0.5 flex-shrink-0" />
                    <p className="text-xs text-amber-300">YAML模式下可直接编辑完整节点配置，修改基础字段（名称/端口/协议）会同步回表单；其他字段建议在表单模式下调整。</p>
                  </div>
                  {yamlError && (
                    <div className="p-3 rounded-lg bg-red-500/10 border border-red-500/20 text-xs text-red-400">{yamlError}</div>
                  )}
                  <Textarea
                    value={yamlText}
                    onChange={e => handleYamlChange(e.target.value)}
                    className="bg-zinc-950 border-zinc-800 text-zinc-300 font-mono text-xs min-h-[420px] resize-y"
                    placeholder="# YAML配置"
                  />
                </div>
              ) : (
                <Card className="bg-zinc-800/30 border-zinc-700/50">
                  <CardContent className="p-4">
                    <DynamicForm schema={configSchema} value={formConfig} onChange={handleConfigChange} />
                  </CardContent>
                </Card>
              )}
            </div>
          )}

          {step === 2 && (
            <div className="space-y-4">
              <div>
                <h3 className="text-sm font-medium text-zinc-200 mb-2">配置预览</h3>
                <p className="text-xs text-zinc-500 mb-3">以下是将创建的节点完整配置（Xray-core JSON格式），确认无误后点击创建。VPS IP地址显示为 YOUR_SERVER_IP，部署后系统自动回填。</p>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-4">
                <div className="p-3 rounded-lg bg-zinc-800/50 border border-zinc-700/50 space-y-1">
                  <div className="flex justify-between"><span className="text-xs text-zinc-500">节点名称</span><span className="text-xs text-zinc-200">{form.name || '(未命名)'}</span></div>
                  <div className="flex justify-between"><span className="text-xs text-zinc-500">数字ID</span><span className="text-xs text-zinc-200 font-mono">{form.numeric_id}</span></div>
                  <div className="flex justify-between"><span className="text-xs text-zinc-500">协议栈</span><span className="text-xs text-zinc-200">{form.protocol.toUpperCase()}/{form.transport}/{form.security}</span></div>
                  <div className="flex justify-between"><span className="text-xs text-zinc-500">端口 (client/server)</span><span className="text-xs text-zinc-200 font-mono">{form.client_port}/{form.server_port}</span></div>
                  <div className="flex justify-between"><span className="text-xs text-zinc-500">内核</span><span className="text-xs text-zinc-200">{form.runtime === 'xray' ? 'Xray-core' : 'Sing-box'}</span></div>
                </div>
                <div className="p-3 rounded-lg bg-zinc-800/50 border border-zinc-700/50 space-y-1">
                  <div className="flex justify-between"><span className="text-xs text-zinc-500">UUID</span><span className="text-xs text-zinc-400 font-mono truncate max-w-[220px]">{form.uuid || '-'}</span></div>
                  <div className="flex justify-between"><span className="text-xs text-zinc-500">密码</span><span className="text-xs text-zinc-400 font-mono truncate max-w-[220px]">{form.password || '-'}</span></div>
                  <div className="flex justify-between"><span className="text-xs text-zinc-500">路径/ServiceName</span><span className="text-xs text-zinc-400 font-mono truncate max-w-[220px]">{form.service_name || form.path || '/'}</span></div>
                  <div className="flex justify-between"><span className="text-xs text-zinc-500">SNI</span><span className="text-xs text-zinc-400 font-mono truncate max-w-[220px]">{form.sni || '-'}</span></div>
                  <div className="flex justify-between"><span className="text-xs text-zinc-500">预设</span><span className="text-xs text-indigo-400">{form.preset_id ? (presets.find(p => p.id === form.preset_id)?.name || form.preset_id) : '手动配置'}</span></div>
                </div>
              </div>

              <div className="relative">
                <div className="flex items-center justify-between mb-2">
                  <Label className="text-zinc-300 text-sm">Xray JSON 配置预览</Label>
                  <Button type="button" variant="ghost" size="sm" onClick={handleCopyConfig} className="text-zinc-400 hover:text-indigo-400">
                    {copied ? <CheckCircle2 className="w-4 h-4 mr-1 text-emerald-400" /> : <Copy className="w-4 h-4 mr-1" />}
                    {copied ? '已复制' : '复制JSON'}
                  </Button>
                </div>
                <pre className="bg-zinc-950 border border-zinc-800 rounded-lg p-4 text-xs text-emerald-400 font-mono overflow-auto max-h-[320px] scrollbar-thin">
                  {configJson}
                </pre>
              </div>

              <div className="p-3 rounded-lg bg-blue-500/10 border border-blue-500/20">
                <p className="text-xs text-blue-300 font-medium mb-1">全自动后续流程</p>
                <ol className="text-xs text-zinc-400 space-y-0.5 list-decimal list-inside">
                  <li>点击"创建节点"——数据库创建节点记录，所有字段自动就绪</li>
                  <li>进入节点详情页，关联VPS服务器（或直接前往自动化页面）</li>
                  <li>系统自动执行VPS脚本：安装Xray/Sing-box、生成REALITY密钥对、回填公钥和IP</li>
                  <li>配置自动下发，节点上线，订阅实时更新</li>
                </ol>
              </div>
            </div>
          )}
        </div>

        <div className="flex items-center justify-between px-6 py-4 border-t border-zinc-800 bg-zinc-900/50">
          <Button type="button" variant="ghost" onClick={handleClose} className="text-zinc-400 hover:text-zinc-100">取消</Button>
          <div className="flex items-center gap-3">
            {step > 0 && (
              <Button type="button" variant="ghost" onClick={() => setStep(step - 1)} className="text-zinc-400 hover:text-zinc-100">上一步</Button>
            )}
            {step < steps.length - 1 ? (
              <Button onClick={() => setStep(step + 1)} disabled={step === 0 && !form.name.trim()} className="bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50">
                下一步
              </Button>
            ) : (
              <Button onClick={handleSubmit} disabled={createNode.isPending || !form.name.trim()} className="bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50">
                {createNode.isPending ? <><Loader2 className="w-4 h-4 mr-2 animate-spin" />创建中...</> : <><Plus className="w-4 h-4 mr-2" />创建节点</>}
              </Button>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
