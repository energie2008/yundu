import * as React from 'react'
import {
  Button,
  Input,
  Label,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
  Badge,
  Switch,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  Separator,
  Textarea,
  useToast,
} from '@airport/ui'
import {
  Server,
  Shield,
  GitBranch,
  HardDrive,
  Code,
  Key,
  RefreshCw,
  Copy,
  Check,
  X,
  Link2,
  Zap,
  Activity,
  Globe,
  Plus,
  Lock,
  Unlock,
  Settings2,
  Network,
  Layers,
  Route,
  FileCode,
  ChevronDown,
  Eye,
  EyeOff,
  ArrowRight,
  AlertTriangle,
  Hash,
  Sparkles,
  Users,
  Save,
  Trash2,
  Pencil,
} from 'lucide-react'
import { YamlEditor, EditorMode, tryParseJson, tryParseYamlSimple } from '@/components/common/YamlEditor'
import { dump as yamlDump } from 'js-yaml'
import { PresetCard, CompatibilityIndicator, PresetDiffViewer } from './PresetCard'
import { DEFAULT_PRESETS } from '@/data/presets'
import { applyPresetToSpec, diffFromPreset, getModifiedFields, type NodeSpecForm } from './preset-utils'
import type { PresetTemplate } from '@/types/preset'
import { useProtocolPresets, useCreatePreset, useDeletePreset, useUpdatePreset } from '@/lib/hooks'
import { api, ApiError } from '@/lib/api'
import { EP } from '@/lib/endpoints'

export interface MuxConfig {
  enabled: boolean
  protocol: 'yamux' | 'h2mux' | 'smux' | 'xmux' | ''
  max_connections: number
  max_streams: number
  padding: boolean
  keep_alive_period: number
  // XMUX 专用字段（Xray xhttpSettings.extra.xmux），支持范围值如 "16-32"
  max_concurrency: string
  c_max_reuse_times: string
  h_max_request_times: string
  h_max_reusable_secs: string
}

export interface TCPBrutalConfig {
  enabled: boolean
  up_mbps: number
  down_mbps: number
}

export interface PortHoppingConfig {
  enabled: boolean
  port_range: string
  interval: number
}

export interface ECHConfig {
  enabled: boolean
  config: string
  priority: 'auto' | 'high'
  enable_dhps: boolean
}

export interface DownloadSettings {
  enabled: boolean
  address: string
  port: number
  server_port?: number
  network: 'tcp' | 'xhttp'
  security: 'tls' | 'reality' | 'none'
  mode: 'packet-up' | 'stream-up' | 'stream-one'
  sni: string
  host: string
  path: string
  public_key: string
  private_key: string
  short_id: string
  // server_name 是下行 REALITY 的 SNI 域名（与 sni 字段同义，用于兼容旧表单）
  server_name: string
  // dest 是下行 REALITY 的回落目标（IP:Port，如 127.0.0.1:9454 或 www.microsoft.com:443）
  dest: string
  fingerprint: string
  alpn: string
  no_grpc_header?: boolean
  allow_insecure?: boolean
}

export interface TLSAdvancedConfig {
  cert_mode: 'none' | 'file' | 'paste' | 'acme'
  cert_file: string
  key_file: string
  cert_pem: string
  key_pem: string
  acme_domains: string
  acme_email: string
  server_name: string
}

export interface AdvancedConfig {
  tls: TLSAdvancedConfig
  mux: MuxConfig
  tcp_brutal: TCPBrutalConfig
  port_hopping: PortHoppingConfig
  ech: ECHConfig
  custom_outbounds: string
  custom_routes: string
}

export interface NodeSpec {
  numeric_id: number
  id: string
  code: string
  protocol: string
  name: string
  multiplier: number
  traffic_limit: number
  // P2-J: 节点流量限额相关字段
  device_limit: number
  speed_limit_mbps: number
  transfer_enable_bytes: number
  transfer_enable_unit: 'GB' | 'MB'
  padding_scheme: string
  tags: string[]
  permission_groups: string[]
  // P1-1: 绑定的套餐 ID 列表
  plan_ids: string[]
  // P1-3: 绑定的证书包 ID
  cert_bundle_id?: string
  address: string
  port: number
  client_port: number
  server_port: number
  transport: string
  security: string
  flow: string
  host?: string
  path?: string
  service_name?: string
  sni?: string
  alpn?: string
  allow_insecure?: boolean
  utls_fingerprint?: string
  reality_dest?: string
  public_key?: string
  private_key?: string
  short_id?: string
  reality_utls_enabled?: boolean
  reality_utls_fingerprint?: string
  spider_x?: string
  parent_node_id?: string
  parent_numeric_id?: number
  server_bindings: Array<{ id: string; name: string; sid: number; auto_manage: boolean; runtime_id?: string }>
  route_groups: string[]
  is_visible: boolean
  priority: number
  region?: string
  raw_settings?: string
  uuid?: string
  password?: string
  method?: string
  username?: string
  advanced: AdvancedConfig
  xhttp_mode?: string
  preset_id?: string
  download_settings?: DownloadSettings
  raw_config_json?: Record<string, unknown>
  // P-Chain: 链式套娃出站 URI（节点入站流量经此代理出站）
  // 支持 socks5:// http:// trojan:// vless:// vmess:// ss:// hysteria2:// tuic://
  chain_outbound_uri?: string
  [key: string]: unknown
}

interface NodeConfigEditorProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  mode: 'create' | 'edit'
  initialSpec?: Partial<NodeSpec>
  onSave: (spec: NodeSpec) => Promise<boolean> | boolean
}

interface ValidationState {
  xrayValid: boolean
  xrayError?: string
  singboxValid: boolean
  singboxError?: string
}

const selectClass = "flex h-9 w-full rounded-lg border border-zinc-800 bg-zinc-950 px-3 py-1 text-sm text-zinc-100 focus:outline-none focus:border-indigo-500"

const PROTOCOL_OPTIONS = [
  { value: 'vless', label: 'VLESS', color: 'bg-emerald-500' },
  { value: 'vmess', label: 'VMess', color: 'bg-blue-500' },
  { value: 'trojan', label: 'Trojan', color: 'bg-purple-500' },
  { value: 'ss', label: 'Shadowsocks', color: 'bg-amber-500' },
  { value: 'hysteria2', label: 'Hysteria2', color: 'bg-rose-500' },
  { value: 'tuic', label: 'TUIC', color: 'bg-cyan-500' },
  { value: 'anytls', label: 'AnyTLS', color: 'bg-violet-500' },
  { value: 'mieru', label: 'Mieru', color: 'bg-lime-500' },
]

const TRANSPORT_OPTIONS: Record<string, Array<{ value: string; label: string }>> = {
  vless: [
    { value: 'tcp', label: 'TCP' }, { value: 'ws', label: 'WebSocket' },
    { value: 'grpc', label: 'gRPC' }, { value: 'h2', label: 'HTTP/2' },
    { value: 'httpupgrade', label: 'HTTPUpgrade' }, { value: 'kcp', label: 'KCP' },
    { value: 'quic', label: 'QUIC' }, { value: 'xhttp', label: 'XHTTP' },
  ],
  vmess: [
    { value: 'tcp', label: 'TCP' }, { value: 'ws', label: 'WebSocket' },
    { value: 'grpc', label: 'gRPC' }, { value: 'h2', label: 'HTTP/2' },
    { value: 'kcp', label: 'KCP' }, { value: 'quic', label: 'QUIC' },
  ],
  trojan: [
    { value: 'tcp', label: 'TCP' }, { value: 'ws', label: 'WebSocket' },
    { value: 'grpc', label: 'gRPC' },
  ],
  ss: [{ value: 'tcp', label: 'TCP' }, { value: 'udp', label: 'UDP' }],
  hysteria2: [{ value: 'udp', label: 'UDP (QUIC)' }],
  tuic: [{ value: 'udp', label: 'UDP (QUIC)' }],
  anytls: [{ value: 'tcp', label: 'TCP' }],
  mieru: [{ value: 'tcp', label: 'TCP' }, { value: 'udp', label: 'UDP' }],
}

const SECURITY_OPTIONS: Record<string, Array<{ value: string; label: string }>> = {
  vless: [{ value: 'none', label: 'None' }, { value: 'tls', label: 'TLS' }, { value: 'reality', label: 'REALITY' }],
  vmess: [{ value: 'none', label: 'None' }, { value: 'tls', label: 'TLS' }],
  trojan: [{ value: 'tls', label: 'TLS' }],
  ss: [{ value: 'none', label: 'None' }],
  hysteria2: [{ value: 'tls', label: 'TLS' }],
  tuic: [{ value: 'tls', label: 'TLS' }],
  anytls: [{ value: 'tls', label: 'TLS' }],
  mieru: [{ value: 'none', label: 'None' }],
}

const FLOW_OPTIONS = [
  { value: '', label: '无' },
  { value: 'xtls-rprx-vision', label: 'xtls-rprx-vision (推荐)' },
  { value: 'xtls-rprx-direct', label: 'xtls-rprx-direct' },
  { value: 'xtls-rprx-splice', label: 'xtls-rprx-splice' },
]
const ALPN_OPTIONS = ['h2', 'http/1.1', 'h2,http/1.1', 'h3']
const UTLS_FINGERPRINTS = ['chrome', 'firefox', 'safari', 'ios', 'android', 'edge', '360', 'qq']
const ENCRYPTION_METHODS = ['aes-256-gcm', 'aes-128-gcm', 'chacha20-ietf-poly1305', '2022-blake3-aes-256-gcm', '2022-blake3-aes-128-gcm']
const XHTTP_MODES = ['auto', 'packet-up', 'stream-up', 'stream-one', 'packet-up-padding']

// 真实服务器列表从后端 GET /admin/servers 动态加载（见 NodeConfigEditor 内 useEffect）
interface ServerOption {
  id: string
  name: string
  sid: number
  online: boolean
  ip: string
  runtime_id?: string
}

// 会员分组（替代 MOCK_PERMISSION_GROUPS）：从后端 GET /admin/node-groups/all 拉取
interface PermissionGroupOption {
  value: string  // UUID
  label: string  // 显示名（code 或 name）
}

// 路由组（替代 MOCK_ROUTE_GROUPS）：从后端 GET /admin/proxy-chains 拉取
interface RouteGroupOption {
  value: string  // UUID
  label: string  // 显示名
}

// 父节点（替代 MOCK_PARENT_NODES）：从后端 GET /admin/nodes 拉取
interface ParentNodeOption {
  id: string  // YunDu 使用 UUID，无 numeric_id 字段
  name: string
  protocol: string
  transport: string
}

// 套餐选项：从后端 GET /admin/plans 拉取
interface PlanOption {
  id: string
  name: string
  code?: string
}

// 证书包选项：从后端 GET /admin/cert-bundles 拉取（如果接口存在）
interface CertBundleOption {
  id: string
  name: string
  domain?: string
}

const DEFAULT_ADVANCED: AdvancedConfig = {
  tls: { cert_mode: 'none', cert_file: '', key_file: '', cert_pem: '', key_pem: '', acme_domains: '', acme_email: '', server_name: '' },
  mux: { enabled: false, protocol: 'yamux', max_connections: 8, max_streams: 32, padding: false, keep_alive_period: 30, max_concurrency: '', c_max_reuse_times: '', h_max_request_times: '', h_max_reusable_secs: '' },
  tcp_brutal: { enabled: false, up_mbps: 50, down_mbps: 100 },
  port_hopping: { enabled: false, port_range: '', interval: 0 },
  ech: { enabled: false, config: '', priority: 'auto', enable_dhps: false },
  custom_outbounds: '', custom_routes: '',
}

const DEFAULT_SPEC: NodeSpec = {
  numeric_id: 0, id: '', code: '', protocol: 'vless', name: '',
  multiplier: 1.0, traffic_limit: 0, tags: [], permission_groups: [],
  plan_ids: [], cert_bundle_id: '',
  device_limit: 0, speed_limit_mbps: 0, transfer_enable_bytes: 0, transfer_enable_unit: 'GB', padding_scheme: '',
  address: '', port: 443, client_port: 443, server_port: 443,
  transport: 'tcp', security: 'reality', flow: 'xtls-rprx-vision',
  // REALITY 默认值：以编辑保存优先，不预设默认伪装域名，避免覆盖用户清空操作
  sni: '', alpn: 'h2,http/1.1', allow_insecure: false,
  utls_fingerprint: 'chrome', reality_dest: '',
  public_key: '', private_key: '', short_id: '',
  reality_utls_enabled: true, reality_utls_fingerprint: 'chrome', spider_x: '',
  parent_node_id: '', parent_numeric_id: 0,
  server_bindings: [], route_groups: [],
  is_visible: true, priority: 0,
  uuid: '', path: '/', host: '', service_name: '',
  advanced: DEFAULT_ADVANCED, xhttp_mode: 'auto', preset_id: '',
  download_settings: { enabled: false, address: '', port: 443, network: 'xhttp', security: 'tls', mode: 'stream-up', sni: '', host: '', path: '', public_key: '', private_key: '', short_id: '', server_name: '', dest: '', fingerprint: 'chrome', alpn: '', no_grpc_header: false, allow_insecure: false },
  chain_outbound_uri: '',
}

// UUID v4 格式校验正则（与后端 google/uuid 包兼容，大小写不敏感）
// 用于在提交前拦截非法 runtime_id，避免后端返回 "invalid UUID length: N" 这类晦涩错误
const UUID_V4_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

function generateUUID(): string {
  if (typeof crypto !== 'undefined' && crypto.randomUUID) return crypto.randomUUID()
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0
    const v = c === 'x' ? r : (r & 0x3) | 0x8
    return v.toString(16)
  })
}
// SS2022 key 是 base64 编码的随机字节 PSK，长度必须匹配 cipher：
//   2022-blake3-aes-128-gcm = 16 字节，2022-blake3-aes-256-gcm / chacha20-poly1305 = 32 字节
function generateSS2022Key(method: string): string {
  const bytes = method.startsWith('2022-blake3-aes-128') ? 16 : 32
  const arr = new Uint8Array(bytes)
  if (typeof crypto !== 'undefined' && crypto.getRandomValues) {
    crypto.getRandomValues(arr)
  } else {
    for (let i = 0; i < bytes; i++) arr[i] = Math.floor(Math.random() * 256)
  }
  return btoa(String.fromCharCode(...Array.from(arr)))
}
// 根据协议和加密方式生成合适的密码
function generatePassword(protocol: string, method?: string): string {
  if (protocol === 'ss' && method && method.startsWith('2022-blake3-')) {
    return generateSS2022Key(method)
  }
  return generateUUID()
}
function generateShortId(): string {
  const hex = '0123456789abcdef'; let r = ''
  for (let i = 0; i < 16; i++) r += hex[Math.floor(Math.random() * 16)]
  return r
}
function generateKeyPair(): { publicKey: string; privateKey: string } {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/'
  let priv = '', pub = ''
  for (let i = 0; i < 43; i++) { priv += chars[Math.floor(Math.random() * chars.length)]; pub += chars[Math.floor(Math.random() * chars.length)] }
  return { privateKey: priv, publicKey: pub }
}

function specToYaml(spec: Partial<NodeSpec>, mode: EditorMode): string {
  const cleanSpec: Record<string, unknown> = {}
  Object.entries(spec).forEach(([k, v]) => {
    if (v !== undefined && v !== '' && v !== null) {
      if (Array.isArray(v) && v.length === 0) return
      if (k === 'advanced') return
      cleanSpec[k] = v
    }
  })
  if (mode === 'json') return JSON.stringify(cleanSpec, null, 2)
  return yamlDump(cleanSpec, { indent: 2, lineWidth: -1, noRefs: true })
}
function yamlToSpec(text: string, mode: EditorMode): { spec: Partial<NodeSpec>; valid: boolean; error?: string } {
  if (!text.trim()) return { spec: {}, valid: true }
  const result = mode === 'json' ? tryParseJson(text) : tryParseYamlSimple(text)
  if (!result.valid || !result.data) return { spec: {}, valid: false, error: result.error }
  return { spec: result.data as Partial<NodeSpec>, valid: true }
}

// generateConfigJsonPreview 生成 config_json 预览，用于 raw config_json 编辑模式
// 与 Nodes.tsx specToNodePayload 中的 configData 逻辑保持一致
function generateConfigJsonPreview(spec: NodeSpec): Record<string, unknown> {
  // flow 字段规则：仅 VLESS+REALITY（非 XHTTP）才使用 flow
  const isFlowSupported = spec.protocol === 'vless' && spec.security === 'reality' && spec.transport !== 'xhttp'
  const effectiveFlow = isFlowSupported ? (spec.flow || undefined) : undefined
  // TCP 传输清空 path/host/service_name
  const isTcp = spec.transport === 'tcp'
  // uuid 仅 VLESS/VMess/TUIC 需要
  const needsUuid = ['vless', 'vmess', 'tuic'].includes(spec.protocol)
  const configData: Record<string, unknown> = {
    network: spec.transport,
    flow: effectiveFlow,
    uuid: needsUuid ? spec.uuid : undefined,
    password: spec.password,
    method: spec.method,
    username: spec.username,
    path: isTcp ? undefined : spec.path,
    host: isTcp ? undefined : spec.host,
    service_name: isTcp ? undefined : spec.service_name,
    sni: spec.sni,
    alpn: spec.alpn ? spec.alpn.split(',').map(s => s.trim()).filter(Boolean) : undefined,
    utls_fingerprint: spec.utls_fingerprint,
    allow_insecure: spec.allow_insecure,
    client_port: spec.client_port || spec.port,
    server_port: spec.server_port || spec.port,
    xhttp_mode: spec.transport === 'xhttp' ? spec.xhttp_mode : undefined,
    region: spec.region || undefined,
    parent_node_id: spec.parent_node_id || undefined,
    priority: spec.priority || 0,
    fingerprint: spec.utls_fingerprint || spec.reality_utls_fingerprint || undefined,
  }
  const tls: number = spec.security === 'reality' ? 2 : spec.security === 'tls' ? 1 : 0
  configData.tls = tls
  configData.security = spec.security
  configData.security_type = spec.security
  if (tls >= 1) {
    const ts: Record<string, unknown> = { allow_insecure: spec.allow_insecure ? 1 : 0 }
    if (spec.sni) { ts.server_name = spec.sni; ts.sni = spec.sni }
    if (spec.alpn) ts.alpn = spec.alpn.split(',').map(s => s.trim()).filter(Boolean)
    if (spec.advanced?.tls?.cert_mode === 'file' && spec.advanced.tls.cert_file) {
      configData.cert_file = spec.advanced.tls.cert_file
      ts.cert_file = spec.advanced.tls.cert_file
    }
    if (spec.advanced?.tls?.cert_mode === 'file' && spec.advanced.tls.key_file) {
      configData.key_file = spec.advanced.tls.key_file
      ts.key_file = spec.advanced.tls.key_file
    }
    configData.tls_settings = ts
  }
  if (tls === 2) {
    const rs: Record<string, unknown> = {}
    if (spec.reality_dest) {
      const parts = spec.reality_dest.split(':')
      rs.server_name = parts[0]
      rs.server_port = parts[1] ? Number(parts[1]) : 443
    }
    if (spec.public_key) rs.public_key = spec.public_key
    if (spec.private_key) rs.private_key = spec.private_key
    if (spec.short_id) rs.short_id = spec.short_id
    if (spec.spider_x) rs.spider_x = spec.spider_x
    if (spec.reality_utls_fingerprint) rs.fingerprint = spec.reality_utls_fingerprint
    configData.reality_settings = rs
    const realityObj: Record<string, unknown> = {}
    if (spec.sni) realityObj.sni = spec.sni
    if (spec.public_key) realityObj.public_key = spec.public_key
    if (spec.private_key) realityObj.private_key = spec.private_key
    if (spec.short_id) realityObj.short_id = spec.short_id
    if (spec.spider_x) realityObj.spider_x = spec.spider_x
    if (spec.reality_utls_fingerprint) realityObj.fingerprint = spec.reality_utls_fingerprint
    configData.reality = realityObj
  }
  // XHTTP
  if (spec.transport === 'xhttp') {
    const xhttp: Record<string, unknown> = {}
    if (spec.xhttp_mode) xhttp.mode = spec.xhttp_mode
    if (spec.path) xhttp.path = spec.path
    if (spec.host) xhttp.host = spec.host
    if (spec.advanced?.mux?.enabled && spec.advanced.mux.protocol === 'xmux') {
      const xmux: Record<string, unknown> = {}
      if (spec.advanced.mux.max_concurrency) xmux.maxConcurrency = spec.advanced.mux.max_concurrency
      if (spec.advanced.mux.max_connections > 0) xmux.maxConnection = spec.advanced.mux.max_connections
      if (spec.advanced.mux.c_max_reuse_times) xmux.cMaxReuseTimes = spec.advanced.mux.c_max_reuse_times
      if (spec.advanced.mux.h_max_request_times) xmux.hMaxRequestTimes = spec.advanced.mux.h_max_request_times
      if (spec.advanced.mux.h_max_reusable_secs) xmux.hMaxReusableSecs = spec.advanced.mux.h_max_reusable_secs
      if (Object.keys(xmux).length > 0) xhttp.extra = { xmux }
    }
    if (spec.download_settings?.enabled) {
      const ds: Record<string, unknown> = {
        address: spec.download_settings.address,
        port: spec.download_settings.port || 443,
        network: spec.download_settings.network,
        security: spec.download_settings.security,
      }
      const xhttpSettings: Record<string, unknown> = {
        mode: spec.download_settings.mode || 'stream-up',
      }
      if (spec.download_settings.host) xhttpSettings.host = spec.download_settings.host
      if (spec.download_settings.path) xhttpSettings.path = spec.download_settings.path
      if (spec.download_settings.no_grpc_header) xhttpSettings.noGRPCHeader = true
      ds.xhttpSettings = xhttpSettings
      if (spec.download_settings.security === 'reality') {
        const realitySettings: Record<string, unknown> = {}
        if (spec.download_settings.public_key) realitySettings.publicKey = spec.download_settings.public_key
        if (spec.download_settings.private_key) realitySettings.privateKey = spec.download_settings.private_key
        if (spec.download_settings.short_id) realitySettings.shortId = spec.download_settings.short_id
        // 修复：serverName 是 SNI 域名（不是 dest），dest 单独存储
        const dlSni = spec.download_settings.server_name || spec.download_settings.sni
        if (dlSni) realitySettings.serverName = dlSni
        if (spec.download_settings.dest) {
          realitySettings.dest = spec.download_settings.dest
        }
        if (spec.download_settings.fingerprint) realitySettings.fingerprint = spec.download_settings.fingerprint
        if (spec.download_settings.alpn) {
          realitySettings.alpn = spec.download_settings.alpn.split(',').map((s: string) => s.trim()).filter(Boolean)
        }
        if (Object.keys(realitySettings).length > 0) ds.realitySettings = realitySettings
      } else if (spec.download_settings.security === 'tls') {
        const tlsSettings: Record<string, unknown> = {}
        if (spec.download_settings.sni || spec.download_settings.server_name) {
          tlsSettings.serverName = spec.download_settings.sni || spec.download_settings.server_name
        }
        if (spec.download_settings.fingerprint) tlsSettings.fingerprint = spec.download_settings.fingerprint
        if (spec.download_settings.alpn) {
          tlsSettings.alpn = spec.download_settings.alpn.split(',').map((s: string) => s.trim()).filter(Boolean)
        }
        if (spec.download_settings.allow_insecure) tlsSettings.allowInsecure = true
        if (Object.keys(tlsSettings).length > 0) ds.tlsSettings = tlsSettings
      }
      const extra = (xhttp.extra as Record<string, unknown>) || {}
      extra.downloadSettings = ds
      xhttp.extra = extra
    }
    if (Object.keys(xhttp).length > 0) configData.xhttp = xhttp
  }
  // Multiplex
  if (spec.advanced?.mux?.enabled && spec.advanced.mux.protocol !== 'xmux') {
    configData.multiplex = {
      enabled: 1,
      protocol: spec.advanced.mux.protocol || 'yamux',
      max_connections: spec.advanced.mux.max_connections || 8,
      padding: spec.advanced.mux.padding ? 1 : 0,
    }
  }
  if (spec.advanced?.tcp_brutal?.enabled) {
    configData.multiplex = { ...(configData.multiplex as object || {}), brutal: { enabled: 1, up_mbps: spec.advanced.tcp_brutal.up_mbps || 50, down_mbps: spec.advanced.tcp_brutal.down_mbps || 100 } }
  }
  // P-Chain: 链式套娃出站 URI（透传到 config_json，由后端 ParseChainURI 解析+校验）
  if (spec.chain_outbound_uri) configData.chain_outbound_uri = spec.chain_outbound_uri
  // 移除 undefined 值
  Object.keys(configData).forEach(k => { if (configData[k] === undefined) delete configData[k] })
  return configData
}

function generatePreviewLink(spec: NodeSpec): string {
  if (!spec.address) return ''
  try {
    const port = spec.client_port || spec.server_port || spec.port
    switch (spec.protocol) {
      case 'vless': {
        const params = new URLSearchParams()
        params.set('type', spec.transport)
        params.set('security', spec.security)
        if (spec.flow) params.set('flow', spec.flow)
        if (spec.security === 'reality') {
          if (spec.public_key) params.set('pbk', spec.public_key)
          if (spec.short_id) params.set('sid', spec.short_id)
          if (spec.sni) params.set('sni', spec.sni)
          if (spec.spider_x) params.set('spx', spec.spider_x)
          params.set('fp', spec.reality_utls_fingerprint || 'chrome')
        } else if (spec.security === 'tls') {
          if (spec.sni) params.set('sni', spec.sni)
          if (spec.alpn) params.set('alpn', spec.alpn)
          params.set('fp', spec.utls_fingerprint || 'chrome')
          if (spec.allow_insecure) params.set('allowInsecure', '1')
        }
        if (spec.transport === 'ws') { if (spec.path) params.set('path', spec.path); if (spec.host) params.set('host', spec.host) }
        else if (spec.transport === 'grpc') { if (spec.service_name) params.set('serviceName', spec.service_name) }
        else if (spec.transport === 'xhttp') {
          if (spec.path) params.set('path', spec.path); if (spec.host) params.set('host', spec.host)
          if (spec.xhttp_mode) params.set('mode', spec.xhttp_mode)
        }
        if (spec.advanced?.mux?.enabled) params.set('multiplex', '1')
        return `vless://${spec.uuid || ''}@${spec.address}:${port}?${params.toString()}#${encodeURIComponent(spec.name || spec.address)}`
      }
      case 'vmess': {
        const config = { v: '2', ps: spec.name || spec.address, add: spec.address, port: String(port), id: spec.uuid || '', aid: '0', scy: 'auto', net: spec.transport, type: 'none', host: spec.host || '', path: spec.path || '/', tls: spec.security === 'tls' ? 'tls' : '', sni: spec.sni || '' }
        return 'vmess://' + btoa(JSON.stringify(config))
      }
      case 'trojan': {
        const params = new URLSearchParams()
        params.set('type', spec.transport); params.set('security', 'tls')
        if (spec.sni) params.set('sni', spec.sni)
        params.set('fp', spec.utls_fingerprint || 'chrome')
        if (spec.transport === 'ws') { if (spec.path) params.set('path', spec.path); if (spec.host) params.set('host', spec.host) }
        return `trojan://${spec.password || ''}@${spec.address}:${port}?${params.toString()}#${encodeURIComponent(spec.name || spec.address)}`
      }
      case 'ss': {
        const method = spec.method || 'aes-256-gcm'
        const userInfo = btoa(`${method}:${spec.password || ''}`)
        return `ss://${userInfo}@${spec.address}:${port}#${encodeURIComponent(spec.name || spec.address)}`
      }
      case 'hysteria2': {
        const params = new URLSearchParams(); params.set('sni', spec.sni || spec.address)
        if (spec.advanced?.port_hopping?.enabled && spec.advanced.port_hopping.port_range) params.set('mport', spec.advanced.port_hopping.port_range)
        return `hysteria2://${spec.password || ''}@${spec.address}:${port}?${params.toString()}#${encodeURIComponent(spec.name || spec.address)}`
      }
      default: return `${spec.protocol}://${spec.address}:${port}`
    }
  } catch { return '' }
}

function DownloadSettingsPanel({ spec, updateSpec }: {
  spec: NodeSpec
  updateSpec: <K extends keyof NodeSpec>(key: K, value: NodeSpec[K]) => void
}) {
  const [expanded, setExpanded] = React.useState(false)
  const [isGeneratingDlKey, setIsGeneratingDlKey] = React.useState(false)
  const dl: DownloadSettings = (spec.download_settings as DownloadSettings) || {
    enabled: false, address: '', port: 443, network: 'xhttp', security: 'tls',
    mode: 'stream-up', sni: '', host: '', path: '', public_key: '', private_key: '', short_id: '',
    server_name: '', dest: '', fingerprint: 'chrome', alpn: '', no_grpc_header: false, allow_insecure: false,
  }
  const uDL = <K extends keyof DownloadSettings>(k: K, v: DownloadSettings[K]) => {
    updateSpec('download_settings', { ...dl, [k]: v })
  }

  const generateDlKeyPairAction = React.useCallback(async () => {
    setIsGeneratingDlKey(true)
    try {
      const kp = generateKeyPair()
      updateSpec('download_settings', { ...dl, public_key: kp.publicKey, private_key: kp.privateKey })
    } finally {
      setIsGeneratingDlKey(false)
    }
  }, [dl, updateSpec])

  const generateDlShortIdAction = React.useCallback(() => {
    updateSpec('download_settings', { ...dl, short_id: generateShortId() })
  }, [dl, updateSpec])

  const FINGERPRINT_OPTIONS = ['chrome', 'firefox', 'safari', 'ios', 'android', 'edge', '360', 'qq', 'random']

  return (
    <div className="rounded-lg border border-zinc-700/60 bg-zinc-900/40 overflow-hidden">
      <button
        type="button"
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center justify-between p-3 hover:bg-zinc-800/30 transition-colors"
      >
        <div className="flex items-center gap-2">
          <ChevronDown className={`w-4 h-4 text-zinc-400 transition-transform ${expanded ? 'rotate-180' : ''}`} />
          <span className="text-sm text-zinc-200 font-medium flex items-center gap-2">
            <Network className="w-4 h-4 text-indigo-400" />
            XHTTP 下行通道 (downloadSettings)
            {dl.enabled && <Badge className="bg-emerald-950/40 text-emerald-400 border-emerald-800/50 ml-1">已启用</Badge>}
          </span>
        </div>
        <span className="text-xs text-zinc-500">{dl.enabled ? `${dl.address}:${dl.port}` : '未配置'}</span>
      </button>

      {!expanded && (
        <div className="px-3 pb-3">
          <div className="flex items-center justify-between p-2 rounded-md bg-zinc-800/30 border border-zinc-700/40">
            <span className="text-xs text-zinc-500">启用下行分流（上行/下行走不同通道，抗封锁 + CDN加速）</span>
            <Switch checked={dl.enabled} onChange={(e) => uDL('enabled', e.target.checked)} />
          </div>
        </div>
      )}

      {expanded && (
        <div className="p-3 pt-0 space-y-4">
          <div className="flex items-center justify-between p-3 rounded-md bg-zinc-800/40 border border-zinc-700/40">
            <div>
              <div className="text-sm text-zinc-200 font-medium">启用下行分流</div>
              <div className="text-xs text-zinc-500">上行走当前节点，下行走 downloadSettings 指定的通道（CDN/IPv6/REALITY直连）</div>
            </div>
            <Switch checked={dl.enabled} onChange={(e) => uDL('enabled', e.target.checked)} />
          </div>

          {dl.enabled && (<>
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label className="text-zinc-300 text-xs">下行地址 (Address)</Label>
                <Input
                  value={dl.address}
                  onChange={(e) => uDL('address', e.target.value)}
                  placeholder="cdn.example.com 或 2603:c020:..."
                  className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 font-mono text-xs"
                />
                <p className="text-[10px] text-zinc-500">CDN域名 / IPv4 / IPv6</p>
              </div>
              <div className="space-y-1.5">
                <Label className="text-zinc-300 text-xs">下行端口 (Port)</Label>
                <Input
                  type="number"
                  min="1"
                  max="65535"
                  value={dl.port}
                  onChange={(e) => uDL('port', Number(e.target.value) || 443)}
                  className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9"
                />
                <p className="text-[10px] text-zinc-500">客户端连接端口，CDN通常443，直连用节点端口</p>
              </div>
            </div>

            <div className="grid grid-cols-3 gap-3">
              <div className="space-y-1.5">
                <Label className="text-zinc-300 text-xs">下行网络</Label>
                <select
                  value={dl.network}
                  onChange={(e) => uDL('network', e.target.value as 'tcp' | 'xhttp')}
                  className={selectClass}
                >
                  <option value="xhttp" className="bg-zinc-800">xhttp</option>
                  <option value="tcp" className="bg-zinc-800">tcp (raw)</option>
                </select>
              </div>
              <div className="space-y-1.5">
                <Label className="text-zinc-300 text-xs">下行安全层</Label>
                <select
                  value={dl.security}
                  onChange={(e) => uDL('security', e.target.value as 'tls' | 'reality' | 'none')}
                  className={selectClass}
                >
                  <option value="tls" className="bg-zinc-800">TLS (CDN/标准证书)</option>
                  <option value="reality" className="bg-zinc-800">REALITY (直连抗封锁)</option>
                  <option value="none" className="bg-zinc-800">None (明文，不推荐)</option>
                </select>
              </div>
              <div className="space-y-1.5">
                <Label className="text-zinc-300 text-xs">下行 XHTTP 模式</Label>
                <select
                  value={dl.mode}
                  onChange={(e) => uDL('mode', e.target.value as 'packet-up' | 'stream-up' | 'stream-one')}
                  className={selectClass}
                >
                  <option value="stream-up" className="bg-zinc-800">stream-up (推荐：直连回传大流量)</option>
                  <option value="packet-up" className="bg-zinc-800">packet-up</option>
                  <option value="stream-one" className="bg-zinc-800">stream-one</option>
                </select>
                <p className="text-[10px] text-zinc-500">下行直连推荐stream-up；CDN中转推荐packet-up</p>
              </div>
            </div>

            {dl.network === 'xhttp' && (
              <div className="grid grid-cols-3 gap-3">
                <div className="space-y-1.5">
                  <Label className="text-zinc-300 text-xs">下行 Host</Label>
                  <Input
                    value={dl.host}
                    onChange={(e) => uDL('host', e.target.value)}
                    placeholder="cdn.example.com"
                    className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 text-xs"
                  />
                </div>
                <div className="space-y-1.5">
                  <Label className="text-zinc-300 text-xs">下行 Path</Label>
                  <Input
                    value={dl.path}
                    onChange={(e) => uDL('path', e.target.value)}
                    placeholder="/xhb4cc53b6"
                    className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 font-mono text-xs"
                  />
                </div>
                <div className="space-y-1.5">
                  <Label className="text-zinc-300 text-xs">下行 SNI</Label>
                  <Input
                    value={dl.sni}
                    onChange={(e) => uDL('sni', e.target.value)}
                    placeholder="cdn.example.com"
                    className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 text-xs"
                  />
                </div>
              </div>
            )}

            {dl.security === 'reality' && (
              <div className="p-3 rounded-md bg-emerald-950/20 border border-emerald-900/40 space-y-3">
                <div className="text-xs font-medium text-emerald-300 flex items-center gap-1.5">
                  <Shield className="w-3.5 h-3.5" />下行 REALITY 配置
                  <span className="text-emerald-500/60 ml-1">（独立密钥对，与主入站REALITY不共用）</span>
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <div className="space-y-1.5">
                    <div className="flex items-center justify-between">
                      <Label className="text-zinc-300 text-xs">下行 PublicKey</Label>
                      <Button type="button" variant="ghost" size="sm" onClick={generateDlKeyPairAction} disabled={isGeneratingDlKey} className="h-7 px-2 text-xs text-emerald-400 hover:text-emerald-300 disabled:opacity-50">
                        <Key className="w-3 h-3 mr-1" />{isGeneratingDlKey ? '生成中...' : '生成密钥对'}
                      </Button>
                    </div>
                    <Input
                      value={dl.public_key}
                      onChange={(e) => uDL('public_key', e.target.value)}
                      placeholder="x25519 public key"
                      className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 font-mono text-xs"
                    />
                  </div>
                  <div className="space-y-1.5">
                    <div className="flex items-center justify-between">
                      <Label className="text-zinc-300 text-xs">下行 Short ID</Label>
                      <Button type="button" variant="ghost" size="sm" onClick={generateDlShortIdAction} className="h-7 px-2 text-xs text-emerald-400 hover:text-emerald-300">
                        <RefreshCw className="w-3 h-3 mr-1" />随机
                      </Button>
                    </div>
                    <Input
                      value={dl.short_id}
                      onChange={(e) => uDL('short_id', e.target.value)}
                      placeholder="16位hex"
                      className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 font-mono text-xs"
                    />
                  </div>
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <div className="space-y-1.5">
                    <Label className="text-zinc-300 text-xs">下行 REALITY SNI (握手域名)</Label>
                    <Input
                      value={dl.server_name || dl.sni}
                      onChange={(e) => uDL('server_name', e.target.value)}
                      placeholder="sub6.dannelblog.na.am"
                      className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 font-mono text-xs"
                    />
                    <p className="text-[10px] text-zinc-500">REALITY 握手 SNI 域名（与客户端连接的域名一致）</p>
                  </div>
                  <div className="space-y-1.5">
                    <Label className="text-zinc-300 text-xs">下行 REALITY Dest (回落目标 IP:Port)</Label>
                    <Input
                      value={dl.dest}
                      onChange={(e) => uDL('dest', e.target.value)}
                      placeholder="127.0.0.1:9454 或 www.microsoft.com:443"
                      className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 font-mono text-xs"
                    />
                    <p className="text-[10px] text-zinc-500">回落目标（host:port），推荐本地反代端口如 127.0.0.1:9454</p>
                  </div>
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <div className="space-y-1.5">
                    <Label className="text-zinc-300 text-xs">下行 uTLS 指纹 <span className="text-emerald-400">(重要)</span></Label>
                    <select value={dl.fingerprint || 'chrome'} onChange={(e) => uDL('fingerprint', e.target.value)} className={selectClass}>
                      {UTLS_FINGERPRINTS.map(f => <option key={f} value={f} className="bg-zinc-800">{f}</option>)}
                    </select>
                    <p className="text-[10px] text-zinc-500">推荐 chrome，模拟Chrome浏览器TLS指纹</p>
                  </div>
                  <div className="space-y-1.5" />
                </div>
                <div className="space-y-1.5">
                  <Label className="text-zinc-300 text-xs">下行 PrivateKey（仅保存到面板，用于服务端下行inbound配置）</Label>
                  <Input
                    value={dl.private_key}
                    onChange={(e) => uDL('private_key', e.target.value)}
                    placeholder="x25519 private key（点上方'生成密钥对'自动填充）"
                    className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 font-mono text-xs"
                  />
                </div>
              </div>
            )}

            {dl.security === 'tls' && (
              <div className="p-3 rounded-md bg-blue-950/20 border border-blue-900/40 space-y-3">
                <div className="text-xs font-medium text-blue-300 flex items-center gap-1.5">
                  <Shield className="w-3.5 h-3.5" />下行 TLS 配置（CDN/标准证书模式）
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <div className="space-y-1.5">
                    <Label className="text-zinc-300 text-xs">下行 SNI</Label>
                    <Input
                      value={dl.sni}
                      onChange={(e) => uDL('sni', e.target.value)}
                      placeholder="cdn.example.com"
                      className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 font-mono text-xs"
                    />
                    <p className="text-[10px] text-zinc-500">TLS 证书域名，留空则使用下行地址</p>
                  </div>
                  <div className="space-y-1.5">
                    <Label className="text-zinc-300 text-xs">ALPN（逗号分隔）</Label>
                    <Input
                      value={dl.alpn}
                      onChange={(e) => uDL('alpn', e.target.value)}
                      placeholder="h2,http/1.1 或 h3"
                      className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 font-mono text-xs"
                    />
                    <p className="text-[10px] text-zinc-500">UDP QOS不严重时可填 h3</p>
                  </div>
                </div>
              </div>
            )}

            <div className="p-3 rounded-md bg-zinc-800/40 border border-zinc-700/50">
              <p className="text-[11px] text-zinc-400">
                <strong className="text-zinc-300">端口说明：</strong>
                Address/Port 是客户端连接的<strong>公网地址和端口</strong>（通常 443），服务端下行监听端口（高位端口）由系统自动分配，无需在此填写。
              </p>
            </div>

            <div className="p-3 rounded-md bg-amber-950/20 border border-amber-900/40">
              <p className="text-xs text-amber-300 flex items-start gap-1.5">
                <AlertTriangle className="w-3.5 h-3.5 flex-shrink-0 mt-0.5" />
                <span>
                  <strong>注意事项：</strong>
                  ① 下行通道必须可达且正确反代到服务端的下行 inbound。
                  ② CDN 模式下行 address 用域名（如 y3.example.com），直连用 IP/IPv6。
                  ③ 下行与上行可使用不同SNI/安全层（如上行 TLS+CDN、下行 Reality 直连）。
                </span>
              </p>
            </div>
          </>)}
        </div>
      )}
    </div>
  )
}

function AdvancedSettingsDialog({ open, onOpenChange, config, onConfigChange }: {
  open: boolean; onOpenChange: (open: boolean) => void; config: AdvancedConfig; onConfigChange: (c: AdvancedConfig) => void
}) {
  const updateConfig = <K extends keyof AdvancedConfig>(k: K, v: AdvancedConfig[K]) => onConfigChange({ ...config, [k]: v })
  const uTLS = <K extends keyof TLSAdvancedConfig>(k: K, v: TLSAdvancedConfig[K]) => onConfigChange({ ...config, tls: { ...config.tls, [k]: v } })
  const uMux = <K extends keyof MuxConfig>(k: K, v: MuxConfig[K]) => onConfigChange({ ...config, mux: { ...config.mux, [k]: v } })
  const uBrut = <K extends keyof TCPBrutalConfig>(k: K, v: TCPBrutalConfig[K]) => onConfigChange({ ...config, tcp_brutal: { ...config.tcp_brutal, [k]: v } })
  const uHop = <K extends keyof PortHoppingConfig>(k: K, v: PortHoppingConfig[K]) => onConfigChange({ ...config, port_hopping: { ...config.port_hopping, [k]: v } })
  const uECH = <K extends keyof ECHConfig>(k: K, v: ECHConfig[K]) => onConfigChange({ ...config, ech: { ...config.ech, [k]: v } })
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-3xl max-h-[90vh] p-0 flex flex-col overflow-hidden">
        <DialogHeader className="p-6 pb-4 border-b border-zinc-800 flex-shrink-0">
          <DialogTitle className="text-lg font-semibold flex items-center gap-2">
            <Settings2 className="w-5 h-5 text-indigo-400" />高级协议设置
          </DialogTitle>
          <DialogDescription className="text-zinc-400 mt-1 text-sm">TLS证书、多路复用、TCP Brutal、端口跳跃、自定义路由等高级配置</DialogDescription>
        </DialogHeader>
        <Tabs defaultValue="tls" className="flex-1 flex flex-col overflow-hidden">
          <div className="px-6 border-b border-zinc-800 flex-shrink-0">
            <TabsList className="h-11 bg-transparent p-0 gap-1">
              <TabsTrigger value="tls" className="h-11 px-3 rounded-none border-b-2 border-transparent data-[state=active]:border-indigo-500 data-[state=active]:text-indigo-400 data-[state=active]:bg-transparent text-zinc-400 hover:text-zinc-200 text-sm gap-1.5"><Shield className="w-4 h-4" />TLS证书</TabsTrigger>
              <TabsTrigger value="mux" className="h-11 px-3 rounded-none border-b-2 border-transparent data-[state=active]:border-indigo-500 data-[state=active]:text-indigo-400 data-[state=active]:bg-transparent text-zinc-400 hover:text-zinc-200 text-sm gap-1.5"><Layers className="w-4 h-4" />多路复用</TabsTrigger>
              <TabsTrigger value="brutal" className="h-11 px-3 rounded-none border-b-2 border-transparent data-[state=active]:border-indigo-500 data-[state=active]:text-indigo-400 data-[state=active]:bg-transparent text-zinc-400 hover:text-zinc-200 text-sm gap-1.5"><Zap className="w-4 h-4" />TCP Brutal</TabsTrigger>
              <TabsTrigger value="hopping" className="h-11 px-3 rounded-none border-b-2 border-transparent data-[state=active]:border-indigo-500 data-[state=active]:text-indigo-400 data-[state=active]:bg-transparent text-zinc-400 hover:text-zinc-200 text-sm gap-1.5"><Network className="w-4 h-4" />端口跳跃</TabsTrigger>
              <TabsTrigger value="enhancement" className="h-11 px-3 rounded-none border-b-2 border-transparent data-[state=active]:border-indigo-500 data-[state=active]:text-indigo-400 data-[state=active]:bg-transparent text-zinc-400 hover:text-zinc-200 text-sm gap-1.5"><Sparkles className="w-4 h-4" />增强 (ECH)</TabsTrigger>
              <TabsTrigger value="custom" className="h-11 px-3 rounded-none border-b-2 border-transparent data-[state=active]:border-indigo-500 data-[state=active]:text-indigo-400 data-[state=active]:bg-transparent text-zinc-400 hover:text-zinc-200 text-sm gap-1.5"><FileCode className="w-4 h-4" />自定义</TabsTrigger>
            </TabsList>
          </div>
          <div className="flex-1 overflow-y-auto p-6 space-y-5">
            <TabsContent value="tls" className="mt-0 space-y-5">
              <div className="space-y-2"><Label className="text-zinc-300 text-sm">证书模式</Label>
                <select value={config.tls.cert_mode} onChange={(e) => uTLS('cert_mode', e.target.value as any)} className={selectClass}>
                  <option value="none" className="bg-zinc-800">无证书 (CDN/Reality场景，证书由上层提供)</option>
                  <option value="file" className="bg-zinc-800">文件路径 (服务器上已有证书文件)</option>
                  <option value="paste" className="bg-zinc-800">粘贴内容 (直接粘贴证书和私钥)</option>
                  <option value="acme" className="bg-zinc-800">ACME自动申请 (Let's Encrypt自动签发)</option>
                </select>
              </div>
              <div className="space-y-2"><Label className="text-zinc-300 text-sm">证书域名 (SNI)</Label>
                <Input value={config.tls.server_name} onChange={(e) => uTLS('server_name', e.target.value)} placeholder="example.com" className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9" />
              </div>
              {config.tls.cert_mode === 'file' && (<>
                <div className="space-y-2"><Label className="text-zinc-300 text-sm">证书文件路径 (Full Chain)</Label><Input value={config.tls.cert_file} onChange={(e) => uTLS('cert_file', e.target.value)} placeholder="/etc/ssl/certs/example.com.pem" className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 font-mono text-xs" /></div>
                <div className="space-y-2"><Label className="text-zinc-300 text-sm">私钥文件路径 (Private Key)</Label><Input value={config.tls.key_file} onChange={(e) => uTLS('key_file', e.target.value)} placeholder="/etc/ssl/private/example.com.key" className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 font-mono text-xs" /></div>
              </>)}
              {config.tls.cert_mode === 'paste' && (<>
                <div className="space-y-2"><Label className="text-zinc-300 text-sm">证书内容 (Public Key / Full Chain PEM)</Label><Textarea value={config.tls.cert_pem} onChange={(e) => uTLS('cert_pem', e.target.value)} placeholder="-----BEGIN CERTIFICATE-----&#10;...&#10;-----END CERTIFICATE-----" className="bg-zinc-950 border-zinc-800 text-zinc-100 min-h-24 font-mono text-xs" /></div>
                <div className="space-y-2"><Label className="text-zinc-300 text-sm">私钥内容 (Private Key PEM)</Label><Textarea value={config.tls.key_pem} onChange={(e) => uTLS('key_pem', e.target.value)} placeholder="-----BEGIN PRIVATE KEY-----&#10;...&#10;-----END PRIVATE KEY-----" className="bg-zinc-950 border-zinc-800 text-zinc-100 min-h-24 font-mono text-xs" /></div>
              </>)}
              {config.tls.cert_mode === 'acme' && (<>
                <div className="space-y-2"><Label className="text-zinc-300 text-sm">域名列表 (逗号分隔)</Label><Input value={config.tls.acme_domains} onChange={(e) => uTLS('acme_domains', e.target.value)} placeholder="example.com,www.example.com" className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9" /></div>
                <div className="space-y-2"><Label className="text-zinc-300 text-sm">联系邮箱</Label><Input value={config.tls.acme_email} onChange={(e) => uTLS('acme_email', e.target.value)} placeholder="admin@example.com" className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9" /></div>
                <div className="p-3 rounded-lg bg-amber-950/30 border border-amber-900/50"><p className="text-xs text-amber-300">ACME模式需要服务器80端口可访问用于HTTP-01验证，或使用DNS-01验证需配置DNS API。</p></div>
              </>)}
            </TabsContent>
            <TabsContent value="mux" className="mt-0 space-y-5">
              <div className="flex items-center justify-between p-3 rounded-lg bg-zinc-800/50 border border-zinc-700/50">
                <div><div className="text-sm text-zinc-200 font-medium">启用多路复用 (Multiplexing)</div><div className="text-xs text-zinc-500">在单个TCP连接上复用多个TCP流，减少握手延迟</div></div>
                <Switch checked={config.mux.enabled} onChange={(e) => uMux('enabled', e.target.checked)} />
              </div>
              {config.mux.enabled && (<>
                <div className="space-y-2"><Label className="text-zinc-300 text-sm">复用协议</Label>
                  <select value={config.mux.protocol} onChange={(e) => uMux('protocol', e.target.value as any)} className={selectClass}>
                    <option value="" className="bg-zinc-800">不使用</option><option value="yamux" className="bg-zinc-800">YAMUX (Xray原生)</option>
                    <option value="h2mux" className="bg-zinc-800">H2Mux</option><option value="smux" className="bg-zinc-800">SMUX</option><option value="xmux" className="bg-zinc-800">XMux (XHTTP专用)</option>
                  </select>
                </div>
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2"><Label className="text-zinc-300 text-sm">最大连接数</Label><Input type="number" min="1" max="256" value={config.mux.max_connections} onChange={(e) => uMux('max_connections', Number(e.target.value) || 8)} className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9" /></div>
                  <div className="space-y-2"><Label className="text-zinc-300 text-sm">单连接最大流数</Label><Input type="number" min="1" max="256" value={config.mux.max_streams} onChange={(e) => uMux('max_streams', Number(e.target.value) || 32)} className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9" /></div>
                </div>
                <div className="flex items-center justify-between p-3 rounded-lg bg-zinc-800/30 border border-zinc-700/30">
                  <div><div className="text-sm text-zinc-200">启用填充 (Padding)</div><div className="text-xs text-zinc-500">添加随机长度填充包，对抗流量特征分析</div></div>
                  <Switch checked={config.mux.padding} onChange={(e) => uMux('padding', e.target.checked)} />
                </div>
                <div className="space-y-2"><Label className="text-zinc-300 text-sm">KeepAlive 间隔 (秒)</Label><Input type="number" min="0" value={config.mux.keep_alive_period} onChange={(e) => uMux('keep_alive_period', Number(e.target.value) || 30)} className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9" /></div>
                {config.mux.protocol === 'xmux' && (<div className="space-y-3 p-3 rounded-lg bg-indigo-900/20 border border-indigo-700/30">
                  <div className="text-xs text-indigo-300 font-medium">XMUX 专用参数 (XHTTP extra.xmux)</div>
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2"><Label className="text-zinc-300 text-sm">maxConcurrency</Label><Input value={config.mux.max_concurrency} onChange={(e) => uMux('max_concurrency', e.target.value)} placeholder="16 或 16-32" className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 font-mono text-xs" /><p className="text-[10px] text-zinc-500">最大并发请求数，支持范围值</p></div>
                    <div className="space-y-2"><Label className="text-zinc-300 text-sm">maxConnection</Label><Input type="number" min="0" value={config.mux.max_connections} onChange={(e) => uMux('max_connections', Number(e.target.value) || 0)} className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 font-mono text-xs" /><p className="text-[10px] text-zinc-500">最大连接数（复用 max_connections 字段）</p></div>
                    <div className="space-y-2"><Label className="text-zinc-300 text-sm">cMaxReuseTimes</Label><Input value={config.mux.c_max_reuse_times} onChange={(e) => uMux('c_max_reuse_times', e.target.value)} placeholder="256 或 64-128" className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 font-mono text-xs" /><p className="text-[10px] text-zinc-500">单连接最大复用次数</p></div>
                    <div className="space-y-2"><Label className="text-zinc-300 text-sm">hMaxReusableSecs</Label><Input value={config.mux.h_max_reusable_secs} onChange={(e) => uMux('h_max_reusable_secs', e.target.value)} placeholder="1800-3600" className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 font-mono text-xs" /><p className="text-[10px] text-zinc-500">连接最大复用时长（秒，范围值）</p></div>
                  </div>
                </div>)}
              </>)}
            </TabsContent>
            <TabsContent value="brutal" className="mt-0 space-y-5">
              <div className="flex items-center justify-between p-3 rounded-lg bg-zinc-800/50 border border-zinc-700/50">
                <div><div className="text-sm text-zinc-200 font-medium">启用 TCP Brutal</div><div className="text-xs text-zinc-500">激进拥塞控制算法，丢包环境下仍能打满带宽 (需内核模块支持)</div></div>
                <Switch checked={config.tcp_brutal.enabled} onChange={(e) => uBrut('enabled', e.target.checked)} />
              </div>
              {config.tcp_brutal.enabled && (<div className="grid grid-cols-2 gap-4">
                <div className="space-y-2"><Label className="text-zinc-300 text-sm">上行带宽 (Mbps)</Label><Input type="number" min="1" value={config.tcp_brutal.up_mbps} onChange={(e) => uBrut('up_mbps', Number(e.target.value) || 50)} className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9" /></div>
                <div className="space-y-2"><Label className="text-zinc-300 text-sm">下行带宽 (Mbps)</Label><Input type="number" min="1" value={config.tcp_brutal.down_mbps} onChange={(e) => uBrut('down_mbps', Number(e.target.value) || 100)} className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9" /></div>
              </div>)}
              <div className="p-3 rounded-lg bg-blue-950/30 border border-blue-900/50"><p className="text-xs text-blue-300">TCP Brutal 需要服务端安装内核模块</p></div>
            </TabsContent>
            <TabsContent value="hopping" className="mt-0 space-y-5">
              <div className="flex items-center justify-between p-3 rounded-lg bg-zinc-800/50 border border-zinc-700/50">
                <div><div className="text-sm text-zinc-200 font-medium">启用端口跳跃 (Port Hopping)</div><div className="text-xs text-zinc-500">Hysteria2等UDP协议支持，客户端在端口范围内随机连接，抗端口封锁</div></div>
                <Switch checked={config.port_hopping.enabled} onChange={(e) => uHop('enabled', e.target.checked)} />
              </div>
              {config.port_hopping.enabled && (<>
                <div className="space-y-2"><Label className="text-zinc-300 text-sm">端口范围</Label><Input value={config.port_hopping.port_range} onChange={(e) => uHop('port_range', e.target.value)} placeholder="例如: 40020-40200 (单端口用逗号分隔: 443,8443,2053)" className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 font-mono" /></div>
                <div className="p-3 rounded-lg bg-amber-950/30 border border-amber-900/50">
                  <p className="text-xs text-amber-300 font-medium mb-1">⚠️ 端口跳跃配置说明</p>
                  <div className="text-xs text-amber-200/80 space-y-1">
                    <p>• <strong>客户端口和服务端口</strong>都填主端口（如 40020），不是范围</p>
                    <p>• <strong>端口范围</strong>在此处填写（如 40020-40200）</p>
                    <p>• 订阅链接自动渲染为 <code className="text-amber-100">mport=40020-40200</code></p>
                    <p>• <strong>Hysteria2 是 UDP 协议，不能走 CDN/nginx 443</strong>，必须直连</p>
                  </div>
                </div>
                <div className="p-3 rounded-lg bg-blue-950/30 border border-blue-900/50">
                  <p className="text-xs text-blue-300 font-medium mb-1">服务端 iptables 规则（需手动配置）</p>
                  <code className="text-xs text-blue-200 block bg-blue-950/50 p-2 rounded font-mono">iptables -t nat -A PREROUTING -p udp --dport 40020:40200 -j REDIRECT --to-ports 40020</code>
                  <p className="text-xs text-blue-400/60 mt-1">将端口范围内的 UDP 流量重定向到主端口（xray 监听端口）</p>
                </div>
              </>)}
            </TabsContent>
            <TabsContent value="enhancement" className="mt-0 space-y-5">
              <div className="p-3 rounded-lg bg-indigo-950/20 border border-indigo-900/50 flex items-start gap-2">
                <Sparkles className="w-4 h-4 text-indigo-400 flex-shrink-0 mt-0.5" />
                <div className="text-xs text-indigo-300">
                  <strong>Enhancement 增强配置</strong>：聚合 uTLS 指纹、ECH 加密客户端问候、Mux 多路复用三项抗封锁增强能力。
                  uTLS 指纹在「安全层」Tab 配置；Mux 在本对话框「多路复用」Tab 配置；ECH 配置在下方。
                </div>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div className="p-3 rounded-lg bg-zinc-800/40 border border-zinc-700/50">
                  <div className="flex items-center gap-2 mb-1"><Shield className="w-3.5 h-3.5 text-emerald-400" /><span className="text-xs text-zinc-400">uTLS 指纹</span></div>
                  <div className="text-sm text-zinc-200">{config.tls.server_name ? '已配置' : '未配置'}</div>
                  <div className="text-xs text-zinc-500 mt-1">在「安全层」Tab 设置 fingerprint</div>
                </div>
                <div className="p-3 rounded-lg bg-zinc-800/40 border border-zinc-700/50">
                  <div className="flex items-center gap-2 mb-1"><Layers className="w-3.5 h-3.5 text-blue-400" /><span className="text-xs text-zinc-400">多路复用 (Mux)</span></div>
                  <div className="text-sm text-zinc-200">{config.mux.enabled ? `已启用 (${config.mux.protocol || 'yamux'})` : '未启用'}</div>
                  <div className="text-xs text-zinc-500 mt-1">在「多路复用」Tab 配置</div>
                </div>
              </div>

              <Separator className="bg-zinc-800" />

              <div className="flex items-center justify-between p-3 rounded-lg bg-zinc-800/50 border border-zinc-700/50">
                <div>
                  <div className="text-sm text-zinc-200 font-medium flex items-center gap-2"><Sparkles className="w-4 h-4 text-indigo-400" />启用 ECH (Encrypted Client Hello)</div>
                  <div className="text-xs text-zinc-500">加密 TLS ClientHello 的 SNI 字段，对抗基于 SNI 的主动探测和封锁（需服务端支持）</div>
                </div>
                <Switch checked={config.ech.enabled} onChange={(e) => uECH('enabled', e.target.checked)} />
              </div>

              {config.ech.enabled && (<>
                <div className="space-y-2">
                  <Label className="text-zinc-300 text-sm">ECH Config 列表</Label>
                  <Textarea value={config.ech.config} onChange={(e) => uECH('config', e.target.value)} placeholder="每行一个 ECH config (base64)&#10;例如:&#10;AEP+DQA...&#10;AEP+DQB..." className="bg-zinc-950 border-zinc-800 text-zinc-100 min-h-24 font-mono text-xs" />
                  <p className="text-xs text-zinc-500">从 DNS HTTPS 记录获取 ECH config，每行一个 base64 字符串。可用 <code className="text-indigo-400">dig TYPE65 _dns.example.com</code> 查询。</p>
                </div>

                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <Label className="text-zinc-300 text-sm">优先级 (Priority)</Label>
                    <select value={config.ech.priority} onChange={(e) => uECH('priority', e.target.value as 'auto' | 'high')} className={selectClass}>
                      <option value="auto" className="bg-zinc-800">auto (优先 ECH，失败回退)</option>
                      <option value="high" className="bg-zinc-800">high (强制 ECH，无回退)</option>
                    </select>
                  </div>
                  <div className="flex items-center justify-between p-3 rounded-lg bg-zinc-800/30 border border-zinc-700/30">
                    <div>
                      <div className="text-sm text-zinc-200">启用 DHPS</div>
                      <div className="text-xs text-zinc-500">Xray 专用：Decoy Hello + Padding</div>
                    </div>
                    <Switch checked={config.ech.enable_dhps} onChange={(e) => uECH('enable_dhps', e.target.checked)} />
                  </div>
                </div>

                <div className="p-3 rounded-lg bg-amber-950/30 border border-amber-900/50">
                  <p className="text-xs text-amber-300">
                    <strong>注意</strong>：ECH 需要服务端配置 ECH 密钥对（可使用 <code className="text-amber-200">xray tls ech</code> 生成）。
                    Sing-box 用 <code className="text-amber-200">ech.config</code>，Xray 用 <code className="text-amber-200">echConfigList</code> + <code className="text-amber-200">enableDHPS</code>。
                    客户端 config 字段在双内核中均为 base64 数组。
                  </p>
                </div>
              </>)}
            </TabsContent>
            <TabsContent value="custom" className="mt-0 space-y-5">
              <div className="space-y-2"><Label className="text-zinc-300 text-sm">自定义 Outbounds (JSON数组)</Label><Textarea value={config.custom_outbounds} onChange={(e) => updateConfig('custom_outbounds', e.target.value)} placeholder='[ WARP出口示例 ]' className="bg-zinc-950 border-zinc-800 text-zinc-100 min-h-32 font-mono text-xs" /><p className="text-xs text-zinc-500">JSON数组格式，将被合并到内核配置的outbounds中，用于WARP出口、自定义代理链等</p></div>
              <div className="space-y-2"><Label className="text-zinc-300 text-sm">自定义 Routes/Routing (JSON数组)</Label><Textarea value={config.custom_routes} onChange={(e) => updateConfig('custom_routes', e.target.value)} className="bg-zinc-950 border-zinc-800 text-zinc-100 min-h-32 font-mono text-xs" /><p className="text-xs text-zinc-500">JSON数组格式，将被合并到路由规则中，用于域名分流、IP规则等</p></div>
            </TabsContent>
          </div>
        </Tabs>
        <DialogFooter className="gap-2 p-4 border-t border-zinc-800 bg-zinc-900">
          <Button variant="outline" onClick={() => onOpenChange(false)} className="border-zinc-700 text-zinc-300 hover:bg-zinc-800">关闭</Button>
          <Button onClick={() => onOpenChange(false)} className="bg-indigo-600 hover:bg-indigo-500"><Check className="w-4 h-4 mr-2" />确认设置</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function NodeConfigEditor({ open, onOpenChange, mode, initialSpec, onSave }: NodeConfigEditorProps) {
  const { toast } = useToast()
  // 从后端获取预设（失败时回退到本地 DEFAULT_PRESETS，保证 UI 可用）
  const presetsQuery = useProtocolPresets()
  const presets = React.useMemo<PresetTemplate[]>(
    () => (presetsQuery.data && presetsQuery.data.length > 0 ? presetsQuery.data : DEFAULT_PRESETS),
    [presetsQuery.data]
  )
  const createPreset = useCreatePreset()
  const deletePreset = useDeletePreset()
  const updatePreset = useUpdatePreset()
  const [savePresetDialog, setSavePresetDialog] = React.useState(false)
  const [newPresetName, setNewPresetName] = React.useState('')
  const [newPresetDesc, setNewPresetDesc] = React.useState('')
  // 从后端加载真实服务器列表（替换原 MOCK_SERVERS）
  const [servers, setServers] = React.useState<ServerOption[]>([])
  React.useEffect(() => {
    if (!open) return
    let cancelled = false
    ;(async () => {
      try {
        const data = await api.get<unknown>(EP.SERVERS, { params: { page: 1, page_size: 100 } })
        if (cancelled) return
        // 后端返回 { items: [...], total: N } 或数组
        const list: any[] = Array.isArray(data) ? data : (((data as any)?.items) || [])
        setServers(list.map((s: any) => ({
          id: s.id,
          name: s.name || s.code,
          sid: s.sid ?? 0,
          online: s.status === 'active',
          ip: s.ipv4 || s.host || '',
          runtime_id: Array.isArray(s.runtimes) && s.runtimes.length > 0 ? s.runtimes[0].id : undefined,
        })))
      } catch (e) {
        if (!cancelled) {
          setServers([])
          // 静默失败：不弹 toast，避免干扰用户编辑流程
        }
      }
    })()
    return () => { cancelled = true }
  }, [open])

  // 从后端加载会员分组列表（替换 MOCK_PERMISSION_GROUPS）
  // 后端 GET /admin/node-groups/all 返回不分页全量数组，供下拉框使用
  const [permissionGroups, setPermissionGroups] = React.useState<PermissionGroupOption[]>([])
  React.useEffect(() => {
    if (!open) return
    let cancelled = false
    ;(async () => {
      try {
        const data = await api.get<unknown>(EP.NODE_GROUPS_ALL)
        if (cancelled) return
        const list: any[] = Array.isArray(data) ? data : (((data as any)?.items) || [])
        setPermissionGroups(list.map((g: any) => ({
          value: String(g.id || ''),
          label: g.name || g.code || g.id,
        })))
      } catch {
        if (!cancelled) setPermissionGroups([])
      }
    })()
    return () => { cancelled = true }
  }, [open])

  // 从后端加载路由组（代理链）列表（替换 MOCK_ROUTE_GROUPS）
  // 后端 GET /admin/proxy-chains 返回分页数组
  const [routeGroups, setRouteGroups] = React.useState<RouteGroupOption[]>([])
  React.useEffect(() => {
    if (!open) return
    let cancelled = false
    ;(async () => {
      try {
        const data = await api.get<unknown>(EP.PROXY_CHAINS, { params: { page: 1, page_size: 100 } })
        if (cancelled) return
        const list: any[] = Array.isArray(data) ? data : (((data as any)?.items) || [])
        setRouteGroups(list.map((c: any) => ({
          value: String(c.id || ''),
          label: c.name || c.code || c.id,
        })))
      } catch {
        if (!cancelled) setRouteGroups([])
      }
    })()
    return () => { cancelled = true }
  }, [open])

  // 从后端加载节点列表作为父节点选项（替换 MOCK_PARENT_NODES）
  // 仅取前 100 条用于下拉框；忽略错误，失败时父节点选项为空
  const [parentNodes, setParentNodes] = React.useState<ParentNodeOption[]>([])
  React.useEffect(() => {
    if (!open) return
    let cancelled = false
    ;(async () => {
      try {
        const data = await api.get<{ items: any[] } | any[]>(EP.NODES, { params: { page: 1, page_size: 100 } })
        if (cancelled) return
        const list: any[] = Array.isArray(data) ? data : (((data as any)?.items) || [])
        setParentNodes(list.map((n: any) => ({
          id: String(n.id || ''),
          name: n.name || n.code || n.id,
          protocol: n.protocol_type || '',
          transport: n.transport_type || '',
        })))
      } catch {
        if (!cancelled) setParentNodes([])
      }
    })()
    return () => { cancelled = true }
  }, [open])

  // P1-1: 从后端加载套餐列表（用于 plan_ids 多选绑定）
  const [plans, setPlans] = React.useState<PlanOption[]>([])
  React.useEffect(() => {
    if (!open) return
    let cancelled = false
    ;(async () => {
      try {
        const data = await api.get<{ items: any[] } | any[]>(EP.PLANS, { params: { page: 1, page_size: 200 } })
        if (cancelled) return
        const list: any[] = Array.isArray(data) ? data : (((data as any)?.items) || [])
        setPlans(list.map((p: any) => ({
          id: String(p.id || ''),
          name: p.name || p.code || p.id,
          code: p.code,
        })))
      } catch {
        if (!cancelled) setPlans([])
      }
    })()
    return () => { cancelled = true }
  }, [open])

  // P1-3: 从后端加载证书包列表（用于 cert_bundle_id 选择，接口可能不存在，静默失败）
  const [certBundles, setCertBundles] = React.useState<CertBundleOption[]>([])
  React.useEffect(() => {
    if (!open) return
    let cancelled = false
    ;(async () => {
      try {
        const data = await api.get<{ items: any[] } | any[]>('/admin/cert-bundles', { params: { page: 1, page_size: 100 } })
        if (cancelled) return
        const list: any[] = Array.isArray(data) ? data : (((data as any)?.items) || [])
        setCertBundles(list.map((c: any) => ({
          id: String(c.id || ''),
          name: c.name || c.domain || c.id,
          domain: c.domain,
        })))
      } catch {
        if (!cancelled) setCertBundles([])
      }
    })()
    return () => { cancelled = true }
  }, [open])

  const [activeTab, setActiveTab] = React.useState('preset')
  const [editorMode, setEditorMode] = React.useState<EditorMode>('yaml')
  const [spec, setSpec] = React.useState<NodeSpec>(() => ({ ...DEFAULT_SPEC, ...initialSpec, advanced: { ...DEFAULT_ADVANCED, ...(initialSpec?.advanced || {}) }, id: initialSpec?.id || '', code: initialSpec?.code || '', numeric_id: initialSpec?.numeric_id || 0 }))
  const [yamlText, setYamlText] = React.useState(() => specToYaml({ ...DEFAULT_SPEC, ...initialSpec }, 'yaml'))
  const [validation, setValidation] = React.useState<ValidationState>({ xrayValid: true, singboxValid: true })
  const [isValidating, setIsValidating] = React.useState(false)
  const [copiedLink, setCopiedLink] = React.useState(false)
  const [newTag, setNewTag] = React.useState('')
  const [yamlEditorError, setYamlEditorError] = React.useState<string | null>(null)
  const [advancedOpen, setAdvancedOpen] = React.useState(false)
  const [showFullUuid, setShowFullUuid] = React.useState(false)
  const [selectedPreset, setSelectedPreset] = React.useState<PresetTemplate | null>(null)
  const [rawConfigText, setRawConfigText] = React.useState('')
  const [rawConfigDirty, setRawConfigDirty] = React.useState(false)
  const [rawConfigError, setRawConfigError] = React.useState<string | null>(null)
  // Path 唯一性检查相关状态
  const [pathChecking, setPathChecking] = React.useState(false)
  const [pathConflict, setPathConflict] = React.useState<{ exists: boolean; conflictingNodeName?: string; suggestedPath?: string } | null>(null)
  const validationTimerRef = React.useRef<ReturnType<typeof setTimeout> | null>(null)
  const isSyncingRef = React.useRef(false)
  const isEdit = mode === 'edit'
  const availableTransports = React.useMemo(() => TRANSPORT_OPTIONS[spec.protocol] || TRANSPORT_OPTIONS.vless, [spec.protocol])
  const availableSecurities = React.useMemo(() => SECURITY_OPTIONS[spec.protocol] || SECURITY_OPTIONS.vless, [spec.protocol])
  const previewLink = React.useMemo(() => generatePreviewLink(spec), [spec])
  const currentProtocol = PROTOCOL_OPTIONS.find(p => p.value === spec.protocol)
  const presetDiff = React.useMemo(() => selectedPreset ? diffFromPreset(spec, selectedPreset) : {}, [spec, selectedPreset])
  const modifiedFields = React.useMemo(() => getModifiedFields(presetDiff), [presetDiff])
  const isPresetModified = modifiedFields.length > 0 && !!selectedPreset

  // 修复：使用后端返回的 presets（含用户自定义预设），而非硬编码 DEFAULT_PRESETS
  // 此前硬编码导致用户保存的自定义预设在 NodeConfigEditor 预设 Tab 不显示
  const filteredPresets = React.useMemo(() => presets, [presets])

  const formValid = React.useMemo(() => !!spec.name && !!spec.address && spec.client_port > 0 && spec.client_port <= 65535 && spec.server_port > 0 && spec.server_port <= 65535 && validation.xrayValid && validation.singboxValid, [spec.name, spec.address, spec.client_port, spec.server_port, validation])
  const showTlsFields = spec.security === 'tls' || spec.security === 'reality'
  const showRealityFields = spec.security === 'reality'
  const needsTransportPath = ['ws', 'h2', 'httpupgrade', 'xhttp'].includes(spec.transport)
  const needsTransportHost = ['ws', 'h2', 'httpupgrade', 'xhttp'].includes(spec.transport)
  const needsServiceName = ['grpc'].includes(spec.transport)
  const showsAuth = ['vless', 'vmess', 'tuic'].includes(spec.protocol)
  const showsPassword = ['trojan', 'ss', 'hysteria2', 'tuic', 'anytls', 'mieru'].includes(spec.protocol)
  const showsFlow = spec.protocol === 'vless' && spec.security === 'reality'
  const showsXHTTP = spec.transport === 'xhttp'
  // argo_tunnel 节点识别：仅 CF 隧道节点需要 DB 字段层面的 TLS 分离
  // argo_tunnel: cloudflared 明文 HTTP 回源，DB security_type=none，客户端必须 TLS
  // cdn/cdn_saas 节点不做 DB 字段分离（DB security_type=tls），TLS 剥离在渲染层动态完成
  const isArgoTunnelSpec = (spec as { exposure_mode?: string }).exposure_mode === 'argo_tunnel'

  const runValidation = React.useCallback(async () => {
    const hasAddress = !!spec.address, hasClientPort = spec.client_port > 0 && spec.client_port <= 65535
    const hasServerPort = spec.server_port > 0 && spec.server_port <= 65535, hasName = !!spec.name
    if (!hasAddress || !hasClientPort || !hasServerPort || !hasName) {
      const msg = !hasAddress ? '缺少地址' : !hasClientPort || !hasServerPort ? '端口无效' : '缺少节点名称'
      setValidation({ xrayValid: false, xrayError: msg, singboxValid: false, singboxError: msg })
      return
    }
    if ((spec as any).is_split_mode && !(spec as any).downstream_exposure_mode) {
      const msg = '启用上下行分离时必须选择下行暴露方式'
      setValidation({ xrayValid: false, xrayError: msg, singboxValid: false, singboxError: msg })
      return
    }
    setIsValidating(true)
    try {
      const resp = await api.post<{
        status: string; xray_valid: boolean; singbox_valid: boolean;
        xray_error?: string; singbox_error?: string; error?: string;
      }>(EP.NODE_VALIDATE, { spec })
      setValidation({
        xrayValid: resp.xray_valid, xrayError: resp.xray_error,
        singboxValid: resp.singbox_valid, singboxError: resp.singbox_error,
      })
    } catch {
      setValidation({ xrayValid: true, singboxValid: true })
    } finally {
      setIsValidating(false)
    }
  }, [spec])

  React.useEffect(() => {
    if (validationTimerRef.current) clearTimeout(validationTimerRef.current)
    validationTimerRef.current = setTimeout(runValidation, 800)
    return () => { if (validationTimerRef.current) clearTimeout(validationTimerRef.current) }
  }, [spec.name, spec.address, spec.client_port, spec.server_port, spec.protocol, spec.uuid, spec.password, runValidation])

  React.useEffect(() => {
    if (!isSyncingRef.current && activeTab === 'yaml') {
      const result = yamlToSpec(yamlText, editorMode)
      if (result.valid && result.spec) {
        setYamlEditorError(null)
        setSpec(prev => {
          const newSpec = { ...DEFAULT_SPEC, ...prev, ...result.spec, advanced: { ...DEFAULT_ADVANCED, ...prev.advanced, ...(result.spec.advanced || {}) } } as NodeSpec
          if (JSON.stringify(newSpec) !== JSON.stringify(prev)) return newSpec
          return prev
        })
      } else { setYamlEditorError(result.error || '解析错误') }
    }
  }, [yamlText, editorMode, activeTab])

  React.useEffect(() => {
    if (!isSyncingRef.current && activeTab !== 'yaml') {
      isSyncingRef.current = true
      setYamlText(specToYaml(spec, editorMode))
      setTimeout(() => { isSyncingRef.current = false }, 0)
    }
  }, [spec, activeTab, editorMode])

  const updateSpec = React.useCallback(<K extends keyof NodeSpec>(key: K, value: NodeSpec[K]) => {
    setSpec(prev => {
      const next = { ...prev, [key]: value }
      if (key === 'protocol') {
        const proto = value as string
        const transports = TRANSPORT_OPTIONS[proto] || TRANSPORT_OPTIONS.vless
        const securities = SECURITY_OPTIONS[proto] || SECURITY_OPTIONS.vless
        next.transport = transports[0].value
        next.security = securities.find(s => s.value === 'reality')?.value || securities[0].value
        next.client_port = next.security === 'reality' ? 443 : proto === 'ss' ? 8388 : proto === 'mieru' ? 4000 : 443
        next.server_port = next.client_port
        next.uuid = ''; next.password = ''; next.flow = ''
        next.method = proto === 'ss' ? 'aes-256-gcm' : undefined
        next.advanced = { ...DEFAULT_ADVANCED }
        setSelectedPreset(null)
      }
      if (key === 'security') {
        const sec = value as string
        // argo_tunnel 节点客户端口必须为 443（CF Edge 入口端口）
        // cdn/cdn_saas 节点也使用 443，但安全层不锁定
        const isArgoTunnel = (next as { exposure_mode?: string }).exposure_mode === 'argo_tunnel'
        const hasCDNAddr = !!(next as { cdn_address?: unknown }).cdn_address
        if (isArgoTunnel || hasCDNAddr) {
          // argo_tunnel / CDN 节点：端口保持 443，仅清理 flow
          if (sec === 'reality') { next.flow = 'xtls-rprx-vision' }
          else { next.flow = '' }
        } else {
          if (sec === 'reality') { next.client_port = 443; next.flow = 'xtls-rprx-vision' }
          else if (sec === 'none') { next.client_port = next.protocol === 'ss' ? 8388 : next.protocol === 'mieru' ? 4000 : 8080; next.flow = '' }
          else { next.client_port = 443; next.flow = '' }
          next.server_port = next.client_port
        }
      }
      if (key === 'transport') {
        // 传输类型切换时清理残留字段，避免旧传输的字段污染新配置
        const tp = value as string
        if (tp === 'tcp') {
          // TCP 传输不需要 path/host/service_name/xhttp_mode/flow（flow 仅 VLESS+REALITY 才用）
          next.path = ''; next.host = ''; next.service_name = ''; next.xhttp_mode = ''
          const isFlowStillValid = next.protocol === 'vless' && next.security === 'reality'
          if (!isFlowStillValid) next.flow = ''
        } else if (tp === 'ws' || tp === 'httpupgrade') {
          // WS/HTTPUpgrade 需要 path/host，不需要 service_name/xhttp_mode
          next.service_name = ''; next.xhttp_mode = ''
          next.flow = '' // WS/HTTPUpgrade 不支持 flow
        } else if (tp === 'grpc') {
          // gRPC 需要 service_name（映射到 path），不需要 xhttp_mode
          next.xhttp_mode = ''
          next.flow = '' // gRPC 不支持 flow
        } else if (tp === 'xhttp') {
          // XHTTP 需要 path/host/xhttp_mode，不需要 service_name/flow
          next.service_name = ''
          next.flow = '' // XHTTP 不支持 flow（即使 VLESS+REALITY+XHTTP 也不用）
          if (!next.xhttp_mode) next.xhttp_mode = 'packet-up'
        }
      }
      if (key === 'client_port') next.server_port = value as number
      if (key === 'parent_node_id') {
        // YunDu 使用 UUID，没有 numeric_id 字段；保留 parent_node_id 即可
        next.parent_numeric_id = 0
      }
      return next
    })
  }, [])

  const updateAdvanced = React.useCallback((advanced: AdvancedConfig) => setSpec(prev => ({ ...prev, advanced })), [])

  const handlePresetSelect = React.useCallback((preset: PresetTemplate) => {
    if (selectedPreset?.id === preset.id && !isPresetModified) { setSelectedPreset(null); return }
    setSelectedPreset(preset)
    const patch = applyPresetToSpec(preset) as Partial<NodeSpec>
    setSpec(prev => ({
      ...prev,
      protocol: patch.protocol || prev.protocol,
      transport: patch.transport || prev.transport,
      security: patch.security || prev.security,
      client_port: patch.client_port ?? prev.client_port,
      server_port: patch.server_port ?? prev.server_port,
      path: patch.path ?? prev.path,
      host: patch.host ?? prev.host,
      service_name: patch.service_name ?? prev.service_name,
      xhttp_mode: patch.xhttp_mode ?? prev.xhttp_mode,
      sni: patch.sni ?? prev.sni,
      alpn: patch.alpn ?? prev.alpn,
      utls_fingerprint: patch.utls_fingerprint ?? prev.utls_fingerprint,
      reality_utls_enabled: patch.reality_utls_enabled ?? prev.reality_utls_enabled,
      reality_utls_fingerprint: patch.reality_utls_fingerprint ?? prev.reality_utls_fingerprint,
      short_id: patch.short_id ?? prev.short_id,
      spider_x: patch.spider_x ?? prev.spider_x,
      reality_dest: patch.reality_dest ?? prev.reality_dest,
      public_key: patch.public_key ?? prev.public_key,
      private_key: patch.private_key ?? prev.private_key,
      multiplier: patch.multiplier ?? prev.multiplier,
      is_visible: patch.is_visible ?? prev.is_visible,
      method: patch.method ?? prev.method,
      password: patch.password ?? prev.password,
      uuid: patch.uuid ?? prev.uuid,
      preset_id: preset.id,
      advanced: { ...prev.advanced, tls: { ...prev.advanced.tls, cert_mode: (patch.advanced?.tls as any)?.cert_mode || prev.advanced.tls.cert_mode } },
      download_settings: (patch as any).download_settings ?? prev.download_settings,
      chain_outbound_uri: (patch as any).chain_outbound_uri ?? prev.chain_outbound_uri,
    }))
    setActiveTab('basic')
  }, [selectedPreset, isPresetModified])

  const handleTabChange = React.useCallback((tab: string) => {
    if (tab === activeTab) return
    if (tab === 'yaml') {
      isSyncingRef.current = true
      setYamlText(specToYaml(spec, editorMode))
      setTimeout(() => { isSyncingRef.current = false }, 0)
    }
    else if (activeTab === 'yaml') {
      const result = yamlToSpec(yamlText, editorMode)
      if (result.valid && result.spec) { setSpec(prev => ({ ...DEFAULT_SPEC, ...prev, ...result.spec, advanced: { ...DEFAULT_ADVANCED, ...prev.advanced, ...(result.spec.advanced || {}) } } as NodeSpec)); setYamlEditorError(null) }
      else { toast({ title: '配置解析错误', description: result.error || '请修正语法错误', variant: 'destructive' }); return }
    }
    setActiveTab(tab)
  }, [activeTab, spec, yamlText, editorMode, toast])

  const addTag = React.useCallback(() => { const tag = newTag.trim(); if (tag && !spec.tags.includes(tag)) { updateSpec('tags', [...spec.tags, tag]); setNewTag('') } }, [newTag, spec.tags, updateSpec])
  const removeTag = React.useCallback((tag: string) => updateSpec('tags', spec.tags.filter(t => t !== tag)), [spec.tags, updateSpec])
  const togglePermissionGroup = React.useCallback((g: string) => updateSpec('permission_groups', spec.permission_groups.includes(g) ? spec.permission_groups.filter(x => x !== g) : [...spec.permission_groups, g]), [spec.permission_groups, updateSpec])
  const toggleServerBinding = React.useCallback((serverId: string) => {
    const server = servers.find(s => s.id === serverId); if (!server) return
    const existing = spec.server_bindings.find(b => b.id === serverId)
    updateSpec('server_bindings', existing ? spec.server_bindings.filter(b => b.id !== serverId) : [...spec.server_bindings, { id: server.id, name: server.name, sid: server.sid, auto_manage: true, runtime_id: server.runtime_id }])
  }, [servers, spec.server_bindings, updateSpec])
  const toggleRouteGroup = React.useCallback((g: string) => updateSpec('route_groups', spec.route_groups.includes(g) ? spec.route_groups.filter(x => x !== g) : [...spec.route_groups, g]), [spec.route_groups, updateSpec])
  const togglePlan = React.useCallback((planId: string) => updateSpec('plan_ids', spec.plan_ids.includes(planId) ? spec.plan_ids.filter(x => x !== planId) : [...spec.plan_ids, planId]), [spec.plan_ids, updateSpec])
  const [isGeneratingKey, setIsGeneratingKey] = React.useState(false)
  const generateKeyPairAction = React.useCallback(async () => {
    setIsGeneratingKey(true)
    try {
      const resp = await api.post<{ private_key: string; public_key: string; short_id: string }>(EP.NODE_REALITY_KEYPAIR)
      setSpec(prev => ({ ...prev, public_key: resp.public_key, private_key: resp.private_key, short_id: resp.short_id }))
      toast({ title: 'REALITY密钥已生成', description: 'x25519密钥对+short_id 已从后端生成', variant: 'success' })
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : '后端不可用，请检查node-service状态'
      toast({ title: '密钥生成失败', description: msg, variant: 'destructive' })
    } finally {
      setIsGeneratingKey(false)
    }
  }, [toast])
  const generateShortIdAction = React.useCallback(() => updateSpec('short_id', generateShortId()), [updateSpec])
  const generateUuidAction = React.useCallback(() => updateSpec('uuid', generateUUID()), [updateSpec])

  // 下行 REALITY 独立密钥对生成（不与主入站共用）
  const [isGeneratingDlKey, setIsGeneratingDlKey] = React.useState(false)
  const generateDlKeyPairAction = React.useCallback(async () => {
    setIsGeneratingDlKey(true)
    try {
      const resp = await api.post<{ private_key: string; public_key: string; short_id: string }>(EP.NODE_REALITY_KEYPAIR)
      setSpec(prev => {
        const prevDl = prev.download_settings || { enabled: false, address: '', port: 443, network: 'xhttp' as const, security: 'reality' as const, mode: 'stream-up', sni: '', host: '', path: '', public_key: '', private_key: '', short_id: '', server_name: '', dest: '', fingerprint: 'chrome', alpn: '', no_grpc_header: false, allow_insecure: false }
        return {
          ...prev,
          download_settings: {
            ...prevDl,
            enabled: true,
            security: 'reality',
            public_key: resp.public_key,
            private_key: resp.private_key,
            short_id: resp.short_id,
          }
        }
      })
      toast({ title: '下行REALITY密钥已生成', description: '独立x25519密钥对+short_id 已生成，与主入站不共用', variant: 'success' })
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : '后端不可用，请检查node-service状态'
      toast({ title: '密钥生成失败', description: msg, variant: 'destructive' })
    } finally {
      setIsGeneratingDlKey(false)
    }
  }, [toast, setSpec])
  const generateDlShortIdAction = React.useCallback(() => {
    setSpec(prev => {
      const prevDl = prev.download_settings || { enabled: false, address: '', port: 443, network: 'xhttp' as const, security: 'reality' as const, mode: 'stream-up', sni: '', host: '', path: '', public_key: '', private_key: '', short_id: '', server_name: '', dest: '', fingerprint: 'chrome', alpn: '' }
      return { ...prev, download_settings: { ...prevDl, short_id: generateShortId() } }
    })
  }, [setSpec])

  // Path 唯一性检查：搜索同 path 的节点，排除当前编辑节点自身
  const checkPathUnique = React.useCallback(async () => {
    const pathValue = (spec.path || '/').trim()
    if (!pathValue) { setPathConflict(null); return }
    setPathChecking(true)
    setPathConflict(null)
    try {
      const data = await api.get<unknown>(EP.NODES, { params: { search: pathValue, page: 1, page_size: 50 } })
      const list: any[] = Array.isArray(data) ? data : (((data as any)?.items) || [])
      // 过滤：同 path 且同 runtime 的节点（排除自身）
      const currentRuntimeId = spec.server_bindings?.[0]?.runtime_id
      const conflicts = list.filter((n: any) => {
        if (isEdit && n.id === spec.id) return false // 排除自身
        const nodePath = n.path || n.transport_path || '/'
        if (nodePath !== pathValue) return false
        // 检查是否同 runtime：节点的 server_bindings 中任一 runtime_id 与当前节点相同
        const bindings: any[] = n.server_bindings || []
        if (currentRuntimeId && bindings.some((b: any) => b.runtime_id === currentRuntimeId)) return true
        // 如果没有 runtime 信息，仅凭 path 相同即视为潜在冲突
        if (!currentRuntimeId) return true
        return false
      })
      if (conflicts.length > 0) {
        const conflictName = conflicts[0].name || conflicts[0].code || conflicts[0].id
        // 生成建议的唯一 path
        let suggested = pathValue
        const base = pathValue.replace(/-\d+$/, '')
        let suffix = 2
        const existingPaths = new Set(list.map((n: any) => n.path || n.transport_path || '/'))
        while (existingPaths.has(`${base}-${suffix}`)) suffix++
        suggested = `${base}-${suffix}`
        setPathConflict({ exists: true, conflictingNodeName: conflictName, suggestedPath: suggested })
      } else {
        setPathConflict({ exists: false })
      }
    } catch {
      setPathConflict(null)
    } finally {
      setPathChecking(false)
    }
  }, [spec.path, spec.id, spec.server_bindings, isEdit])

  // 当 path 变化时重置冲突状态
  React.useEffect(() => { setPathConflict(null) }, [spec.path])

  const handleCopyLink = React.useCallback(() => { if (previewLink) { navigator.clipboard.writeText(previewLink); setCopiedLink(true); toast({ title: '节点链接已复制', variant: 'success' }); setTimeout(() => setCopiedLink(false), 2000) } }, [previewLink, toast])
  const handleSave = React.useCallback(async () => {
    if (!formValid) { toast({ title: '请完善表单', description: '请填写所有必填字段并通过校验', variant: 'destructive' }); return }
    const currentSpec = activeTab === 'yaml' ? ({ ...DEFAULT_SPEC, ...yamlToSpec(yamlText, editorMode).spec, advanced: spec.advanced } as NodeSpec) : spec
    // 创建模式下必须绑定至少一个服务器（后端 CreateNodeRequest.runtime_id required）
    if (!isEdit && (!currentSpec.server_bindings || currentSpec.server_bindings.length === 0 || !currentSpec.server_bindings[0].runtime_id)) {
      toast({ title: '请绑定服务器', description: '创建节点时必须选择至少一个服务器（用于确定运行时）', variant: 'destructive' }); return
    }
    // 校验 runtime_id 是否为有效的 UUID 格式（后端 uuid.UUID 字段会拒绝非法字符串）
    const rid = currentSpec.server_bindings?.[0]?.runtime_id
    if (rid && !UUID_V4_RE.test(rid)) {
      toast({ title: '服务器运行时无效', description: `所选服务器未注册有效的 runtime（${rid.length <= 10 ? `收到 ${rid.length} 字符` : '格式错误'}）。请先在该服务器上安装 node-agent 以注册 runtime，或选择其他服务器。`, variant: 'destructive' }); return
    }
    // P0-3: 如果 raw config_json 被修改且有效，覆盖 spec.raw_config_json
    if (rawConfigDirty && rawConfigText.trim()) {
      const parsed = tryParseJson(rawConfigText)
      if (!parsed.valid) {
        toast({ title: 'Raw config_json 无效', description: parsed.error || 'JSON 解析错误', variant: 'destructive' }); return
      }
      currentSpec.raw_config_json = parsed.data as Record<string, unknown>
    }
    if (!currentSpec.id) currentSpec.id = generateUUID()
    if (!currentSpec.code) currentSpec.code = `node${Date.now().toString(36)}`
    if (!currentSpec.numeric_id) currentSpec.numeric_id = Math.floor(Math.random() * 90000) + 10000
    // 等待 onSave 完成：保存失败时保持对话框打开，保留用户填写的表单数据
    const ok = await onSave(currentSpec)
    if (ok !== false) {
      onOpenChange(false)
    }
  }, [formValid, activeTab, yamlText, editorMode, spec, rawConfigDirty, rawConfigText, onSave, onOpenChange, toast, isEdit])
  const handleReset = React.useCallback(() => { setSpec({ ...DEFAULT_SPEC, ...initialSpec, advanced: { ...DEFAULT_ADVANCED, ...(initialSpec?.advanced || {}) } }); setActiveTab('preset'); setSelectedPreset(null) }, [initialSpec])

  const detectKernelCompat = React.useCallback((proto: string, transport: string, security: string, ds?: DownloadSettings | null): 'both' | 'xray_only' | 'singbox_only' | 'experimental' => {
    if (proto === 'anytls') return 'singbox_only'
    if (proto === 'mieru') return 'experimental'
    if (ds?.enabled) return 'xray_only'
    if (transport === 'xhttp' && security === 'reality') return 'xray_only'
    if (transport === 'xhttp') return 'experimental'
    return 'both'
  }, [])

  const specToPresetBaseSpec = React.useCallback((s: NodeSpec) => {
    const baseSpec: Record<string, unknown> = {
      protocol: s.protocol,
      transport: { type: s.transport },
      security: s.security,
      client_port: s.client_port,
      server_port: s.server_port,
      traffic_rate: s.multiplier || 1.0,
      is_visible: s.is_visible !== false,
    }
    if (s.transport === 'ws') {
      const ws: Record<string, unknown> = {}
      if (s.path) ws.path = s.path
      if (s.host) ws.host = s.host
      if (Object.keys(ws).length > 0) baseSpec.transport = { type: 'ws', ws }
    } else if (s.transport === 'grpc') {
      if (s.service_name) baseSpec.transport = { type: 'grpc', grpc: { service_name: s.service_name } }
    } else if (s.transport === 'xhttp') {
      const xhttp: Record<string, unknown> = {}
      if (s.path) xhttp.path = s.path
      if (s.host) xhttp.host = s.host
      if (s.xhttp_mode && s.xhttp_mode !== 'auto') xhttp.mode = s.xhttp_mode
      if (s.download_settings?.enabled) {
        const ds: Record<string, unknown> = {
          address: s.download_settings.address,
          port: s.download_settings.port || 443,
          network: s.download_settings.network,
          security: s.download_settings.security,
          mode: s.download_settings.mode,
        }
        if (s.download_settings.host) ds.host = s.download_settings.host
        if (s.download_settings.path) ds.path = s.download_settings.path
        if (s.download_settings.sni) ds.sni = s.download_settings.sni
        if (s.download_settings.security === 'reality') {
          const reality: Record<string, unknown> = {}
          if (s.download_settings.public_key) reality.public_key = s.download_settings.public_key
          if (s.download_settings.short_id) reality.short_id = s.download_settings.short_id
          // 修复：reality_dest 是 dest（IP:Port），不是 server_name（SNI 域名）
          if (s.download_settings.dest) reality.reality_dest = s.download_settings.dest
          // server_name 单独存为 SNI
          const dlSni = s.download_settings.server_name || s.download_settings.sni
          if (dlSni) reality.sni = dlSni
          if (s.download_settings.fingerprint) reality.fingerprint = s.download_settings.fingerprint
          if (Object.keys(reality).length > 0) ds.reality = reality
        } else if (s.download_settings.security === 'tls') {
          const tls: Record<string, unknown> = {}
          if (s.download_settings.sni) tls.sni = s.download_settings.sni
          if (s.download_settings.fingerprint) tls.fingerprint = s.download_settings.fingerprint
          if (s.download_settings.alpn) tls.alpn = s.download_settings.alpn.split(',').map(x => x.trim()).filter(Boolean)
          if (Object.keys(tls).length > 0) ds.tls = tls
        }
        xhttp.download_settings = ds
      }
      if (Object.keys(xhttp).length > 0) baseSpec.transport = { type: 'xhttp', xhttp }
      else baseSpec.transport = { type: 'xhttp' }
    }
    if (s.security === 'tls') {
      const tls: Record<string, unknown> = {}
      if (s.sni) tls.sni = s.sni
      if (s.utls_fingerprint) tls.fingerprint = s.utls_fingerprint
      if (s.alpn) tls.alpn = s.alpn.split(',').map(x => x.trim()).filter(Boolean)
      if (s.advanced?.tls?.cert_mode && s.advanced.tls.cert_mode !== 'none') tls.cert_mode = s.advanced.tls.cert_mode
      if (Object.keys(tls).length > 0) baseSpec.tls = tls
    }
    if (s.security === 'reality') {
      const reality: Record<string, unknown> = {}
      if (s.sni) reality.sni = s.sni
      if (s.public_key) reality.public_key = s.public_key
      if (s.short_id) reality.short_id = s.short_id
      if (s.reality_dest || s.sni) reality.reality_dest = s.reality_dest || s.sni
      if (s.reality_utls_fingerprint || s.utls_fingerprint) reality.fingerprint = s.reality_utls_fingerprint || s.utls_fingerprint
      if (s.spider_x) reality.spider_x = s.spider_x
      if (Object.keys(reality).length > 0) baseSpec.reality = reality
    }
    const creds: Record<string, unknown> = {}
    if (s.flow) creds.flow = s.flow
    if (Object.keys(creds).length > 0) baseSpec.credentials = creds
    return baseSpec
  }, [])

  const handleSaveAsPreset = React.useCallback(() => {
    if (!newPresetName.trim()) {
      toast({ title: '请输入预设名称', variant: 'destructive' }); return
    }
    const baseSpec = specToPresetBaseSpec(spec)
    const code = 'custom-' + Date.now().toString(36)
    createPreset.mutate({
      code,
      name: newPresetName.trim(),
      description: newPresetDesc.trim() || '用户自定义预设',
      protocol_type: spec.protocol,
      transport_type: spec.transport,
      security_type: spec.security,
      kernel_compat: detectKernelCompat(spec.protocol, spec.transport, spec.security, spec.download_settings),
      base_spec: baseSpec,
      recommended_port: spec.client_port || 443,
      sort_order: 50,
      is_recommended: false,
      is_enabled: true,
      client_support: ['xray', 'sing-box'],
    }, {
      onSuccess: () => {
        toast({ title: '预设已保存', description: `「${newPresetName.trim()}」已保存为自定义预设` })
        setSavePresetDialog(false)
        setNewPresetName('')
        setNewPresetDesc('')
      },
      onError: (err) => {
        toast({ title: '保存失败', description: String(err), variant: 'destructive' })
      },
    })
  }, [newPresetName, newPresetDesc, spec, specToPresetBaseSpec, detectKernelCompat, createPreset, toast])

  const handleUpdatePreset = React.useCallback((presetId: string, presetName: string) => {
    if (!confirm(`确定要用当前配置更新自定义预设「${presetName}」吗？`)) return
    const baseSpec = specToPresetBaseSpec(spec)
    updatePreset.mutate({
      id: presetId,
      data: {
        name: presetName,
        protocol_type: spec.protocol,
        transport_type: spec.transport,
        security_type: spec.security,
        kernel_compat: detectKernelCompat(spec.protocol, spec.transport, spec.security, spec.download_settings),
        base_spec: baseSpec,
        recommended_port: spec.client_port || 443,
      },
    }, {
      onSuccess: () => {
        toast({ title: '预设已更新', description: `「${presetName}」已更新为当前配置` })
      },
      onError: (err) => {
        toast({ title: '更新失败', description: String(err), variant: 'destructive' })
      },
    })
  }, [spec, specToPresetBaseSpec, detectKernelCompat, updatePreset, toast])

  React.useEffect(() => {
    if (open) {
      setSpec({ ...DEFAULT_SPEC, ...initialSpec, advanced: { ...DEFAULT_ADVANCED, ...(initialSpec?.advanced || {}) }, id: initialSpec?.id || '', code: initialSpec?.code || '', numeric_id: initialSpec?.numeric_id || 0 })
      setYamlText(specToYaml({ ...DEFAULT_SPEC, ...initialSpec }, 'yaml'))
      setEditorMode('yaml')
      if (initialSpec?.preset_id) {
        const preset = presets.find(p => p.id === initialSpec.preset_id)
        setSelectedPreset(preset || null)
      } else { setSelectedPreset(null) }
      setActiveTab(isEdit ? 'basic' : 'preset')
      setYamlEditorError(null)
      // P0-3: 初始化 raw config_json 预览
      setRawConfigText(JSON.stringify(generateConfigJsonPreview({ ...DEFAULT_SPEC, ...initialSpec }), null, 2))
      setRawConfigDirty(false)
      setRawConfigError(null)
    }
  }, [open, initialSpec, isEdit])

  // P0-3: 切换到 raw_config tab 时，如果未手动修改，自动从表单重新生成预览
  React.useEffect(() => {
    if (activeTab === 'raw_config' && !rawConfigDirty) {
      setRawConfigText(JSON.stringify(generateConfigJsonPreview(spec), null, 2))
    }
  }, [activeTab, spec, rawConfigDirty])

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-5xl max-h-[95vh] p-0 flex flex-col overflow-hidden">
          <DialogHeader className="p-6 pb-4 border-b border-zinc-800 flex-shrink-0">
            <div className="flex items-start justify-between gap-4">
              <div>
                <DialogTitle className="text-lg font-semibold flex items-center gap-3 flex-wrap">
                  <Server className="w-5 h-5 text-indigo-400" />
                  {isEdit ? '编辑节点' : '新增节点'}
                  <Badge variant="secondary" className="bg-zinc-800 text-zinc-400 font-mono text-xs flex items-center gap-1">
                    <Hash className="w-3 h-3" />{spec.numeric_id || '自动分配'}
                  </Badge>
                  {isEdit && spec.id && <Badge variant="secondary" className="bg-zinc-800 text-zinc-500 font-mono text-[10px]">{spec.id.substring(0, 8)}...</Badge>}
                  {spec.preset_id && selectedPreset && (<Badge className="bg-indigo-900/50 text-indigo-300 border-indigo-800/50 text-xs">预设: {selectedPreset.name}</Badge>)}
                </DialogTitle>
                <DialogDescription className="text-zinc-400 mt-1 text-sm">{isEdit ? '修改节点配置，保存后将自动部署到绑定服务器' : '选择预设快速创建，或手动配置每个参数'}</DialogDescription>
              </div>
              <div className="flex items-center gap-2">
                <CompatibilityIndicator spec={{ protocol: spec.protocol, transport: spec.transport, security: spec.security }} />
                <div className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-zinc-800 border border-zinc-700">
                  <div className={`w-2.5 h-2.5 rounded-full ${currentProtocol?.color || 'bg-zinc-500'}`} />
                  <select value={spec.protocol} onChange={(e) => updateSpec('protocol', e.target.value)}
                    className="h-7 border-0 bg-transparent text-sm font-medium text-zinc-200 focus:outline-none cursor-pointer appearance-none pr-5"
                    style={{ backgroundImage: `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='%2371717a' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpolyline points='6 9 12 15 18 9'%3E%3C/polyline%3E%3C/svg%3E")`, backgroundRepeat: 'no-repeat', backgroundPosition: 'right 0 center' }}>
                    {PROTOCOL_OPTIONS.map(p => <option key={p.value} value={p.value} className="bg-zinc-800 text-zinc-200">{p.label}</option>)}
                  </select>
                </div>
              </div>
            </div>
          </DialogHeader>

          <Tabs value={activeTab} onValueChange={handleTabChange} className="flex-1 flex flex-col overflow-hidden">
            <div className="px-6 border-b border-zinc-800 flex-shrink-0 overflow-x-auto">
              <TabsList className="h-11 bg-transparent p-0 gap-1">
                {!isEdit && (<TabsTrigger value="preset" className="h-11 px-3 rounded-none border-b-2 border-transparent data-[state=active]:border-indigo-500 data-[state=active]:text-indigo-400 data-[state=active]:bg-transparent text-zinc-400 hover:text-zinc-200 text-sm gap-1.5 whitespace-nowrap"><Zap className="w-4 h-4" />推荐预设</TabsTrigger>)}
                <TabsTrigger value="basic" className="h-11 px-3 rounded-none border-b-2 border-transparent data-[state=active]:border-indigo-500 data-[state=active]:text-indigo-400 data-[state=active]:bg-transparent text-zinc-400 hover:text-zinc-200 text-sm gap-1.5 whitespace-nowrap"><Server className="w-4 h-4" />基础信息</TabsTrigger>
                <TabsTrigger value="connection" className="h-11 px-3 rounded-none border-b-2 border-transparent data-[state=active]:border-indigo-500 data-[state=active]:text-indigo-400 data-[state=active]:bg-transparent text-zinc-400 hover:text-zinc-200 text-sm gap-1.5 whitespace-nowrap"><Globe className="w-4 h-4" />连接配置</TabsTrigger>
                <TabsTrigger value="transport" className="h-11 px-3 rounded-none border-b-2 border-transparent data-[state=active]:border-indigo-500 data-[state=active]:text-indigo-400 data-[state=active]:bg-transparent text-zinc-400 hover:text-zinc-200 text-sm gap-1.5 whitespace-nowrap"><Activity className="w-4 h-4" />传输</TabsTrigger>
                <TabsTrigger value="security" className="h-11 px-3 rounded-none border-b-2 border-transparent data-[state=active]:border-indigo-500 data-[state=active]:text-indigo-400 data-[state=active]:bg-transparent text-zinc-400 hover:text-zinc-200 text-sm gap-1.5 whitespace-nowrap"><Shield className="w-4 h-4" />安全</TabsTrigger>
                <TabsTrigger value="relay" className="h-11 px-3 rounded-none border-b-2 border-transparent data-[state=active]:border-indigo-500 data-[state=active]:text-indigo-400 data-[state=active]:bg-transparent text-zinc-400 hover:text-zinc-200 text-sm gap-1.5 whitespace-nowrap"><GitBranch className="w-4 h-4" />中转/父节点</TabsTrigger>
                <TabsTrigger value="binding" className="h-11 px-3 rounded-none border-b-2 border-transparent data-[state=active]:border-indigo-500 data-[state=active]:text-indigo-400 data-[state=active]:bg-transparent text-zinc-400 hover:text-zinc-200 text-sm gap-1.5 whitespace-nowrap"><HardDrive className="w-4 h-4" />绑定/路由</TabsTrigger>
                <TabsTrigger value="yaml" className="h-11 px-3 rounded-none border-b-2 border-transparent data-[state=active]:border-indigo-500 data-[state=active]:text-indigo-400 data-[state=active]:bg-transparent text-zinc-400 hover:text-zinc-200 text-sm gap-1.5 whitespace-nowrap"><Code className="w-4 h-4" />YAML/JSON</TabsTrigger>
                <TabsTrigger value="raw_config" className="h-11 px-3 rounded-none border-b-2 border-transparent data-[state=active]:border-indigo-500 data-[state=active]:text-indigo-400 data-[state=active]:bg-transparent text-zinc-400 hover:text-zinc-200 text-sm gap-1.5 whitespace-nowrap"><FileCode className="w-4 h-4" />Raw JSON</TabsTrigger>
              </TabsList>
            </div>

            <div className="flex-1 overflow-y-auto p-6">
              {!isEdit && (<TabsContent value="preset" className="mt-0 space-y-4">
                <div className="flex items-center justify-between">
                  <div><h3 className="text-sm font-semibold text-zinc-200">选择一个预设快速开始</h3><p className="text-xs text-zinc-500 mt-0.5">选择预设后会自动填充表单参数，你仍可以任意修改每个字段，不会锁定</p></div>
                  <div className="flex items-center gap-2">
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => { setNewPresetName(spec.name || '自定义预设'); setNewPresetDesc(''); setSavePresetDialog(true) }}
                      className="border-zinc-700 text-zinc-300 h-7 text-xs hover:border-indigo-500 hover:text-indigo-300"
                    >
                      <Save className="w-3.5 h-3.5 mr-1" />保存为自定义预设
                    </Button>
                    {selectedPreset && (<Button type="button" variant="ghost" size="sm" onClick={() => { setSelectedPreset(null); updateSpec('preset_id', ''); }} className="text-zinc-400 h-7 text-xs">清除预设</Button>)}
                  </div>
                </div>
                {isPresetModified && selectedPreset && (<PresetDiffViewer modifiedFields={modifiedFields} presetName={selectedPreset.name} />)}
                {selectedPreset && selectedPreset.warnings && selectedPreset.warnings.length > 0 && (<div className="p-3 rounded-lg bg-amber-950/20 border border-amber-900/30 space-y-1">
                  {selectedPreset.warnings.map((w, i) => (<div key={i} className="flex items-start gap-2 text-xs text-amber-300/80"><AlertTriangle className="w-3.5 h-3.5 text-amber-500 flex-shrink-0 mt-0.5" />{w}</div>))}
                </div>)}
                <div className="grid grid-cols-3 gap-3">
                  {filteredPresets.map(p => (
                    <div key={p.id} className="relative group">
                      <PresetCard preset={p} selected={selectedPreset?.id === p.id} modified={selectedPreset?.id === p.id && isPresetModified} onClick={() => handlePresetSelect(p)} />
                      {(p as any).is_builtin === false && (
                        <>
                          <button
                            type="button"
                            onClick={(e) => {
                              e.stopPropagation()
                              handleUpdatePreset(p.id, p.name)
                            }}
                            className="absolute top-2 right-10 w-6 h-6 rounded bg-blue-900/60 text-blue-300 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center hover:bg-blue-800"
                            title="更新自定义预设（用当前配置覆盖）"
                          >
                            <Pencil className="w-3 h-3" />
                          </button>
                          <button
                            type="button"
                            onClick={(e) => {
                              e.stopPropagation()
                              if (confirm(`确定删除自定义预设「${p.name}」吗？`)) {
                                deletePreset.mutate(p.id, {
                                  onSuccess: () => toast({ title: '预设已删除' }),
                                  onError: (err) => toast({ title: '删除失败', description: String(err), variant: 'destructive' }),
                                })
                              }
                            }}
                            className="absolute top-2 right-2 w-6 h-6 rounded bg-red-900/60 text-red-300 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center hover:bg-red-800"
                            title="删除自定义预设"
                          >
                            <Trash2 className="w-3 h-3" />
                          </button>
                        </>
                      )}
                    </div>
                  ))}
                </div>
                <div className="flex items-center justify-center pt-2">
                  <Button type="button" variant="ghost" onClick={() => setActiveTab('basic')} className="text-indigo-400 hover:text-indigo-300 text-sm">从零开始手动配置<ArrowRight className="w-4 h-4 ml-1" /></Button>
                </div>
              </TabsContent>)}

              <TabsContent value="basic" className="mt-0 space-y-5">
                {selectedPreset && isPresetModified && (<PresetDiffViewer modifiedFields={modifiedFields} presetName={selectedPreset.name} />)}
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2"><Label className="text-zinc-300 text-sm">节点名称 <span className="text-red-400">*</span></Label><Input value={spec.name} onChange={(e) => updateSpec('name', e.target.value)} placeholder="如：香港01-CN2 GIA" className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 focus:border-indigo-500" /></div>
                  <div className="space-y-2"><Label className="text-zinc-300 text-sm">节点编码 (Code)</Label><Input value={spec.code} onChange={(e) => updateSpec('code', e.target.value)} placeholder="如：HK-01" className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 focus:border-indigo-500 font-mono" /></div>
                </div>
                <div className="grid grid-cols-3 gap-4">
                  <div className="space-y-2"><Label className="text-zinc-300 text-sm">基础倍率</Label><div className="flex items-center gap-2"><Input type="number" step="0.1" min="0.1" value={spec.multiplier} onChange={(e) => updateSpec('multiplier', Number(e.target.value) || 1)} className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9" /><span className="text-zinc-400 text-sm">x</span></div></div>
                  <div className="space-y-2"><Label className="text-zinc-300 text-sm">优先级</Label><Input type="number" value={spec.priority} onChange={(e) => updateSpec('priority', Number(e.target.value) || 0)} className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9" /></div>
                  <div className="space-y-2"><Label className="text-zinc-300 text-sm">设备数限制</Label><Input type="number" min="0" value={spec.device_limit} onChange={(e) => updateSpec('device_limit', Number(e.target.value) || 0)} className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9" /><p className="text-xs text-zinc-500">0为不限</p></div>
                </div>
                <div className="grid grid-cols-3 gap-4">
                  <div className="space-y-2"><Label className="text-zinc-300 text-sm">速度限制</Label><div className="flex items-center gap-2"><Input type="number" min="0" value={spec.speed_limit_mbps} onChange={(e) => updateSpec('speed_limit_mbps', Number(e.target.value) || 0)} className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9" /><span className="text-zinc-400 text-sm">Mbps</span></div><p className="text-xs text-zinc-500">0为不限</p></div>
                  <div className="space-y-2"><Label className="text-zinc-300 text-sm">流量限额</Label><div className="flex items-center gap-2"><Input type="number" min="0" step="0.01" value={spec.transfer_enable_bytes > 0 ? spec.transfer_enable_bytes / (spec.transfer_enable_unit === 'GB' ? 1024 * 1024 * 1024 : 1024 * 1024) : 0} onChange={(e) => updateSpec('transfer_enable_bytes', (Number(e.target.value) || 0) * (spec.transfer_enable_unit === 'GB' ? 1024 * 1024 * 1024 : 1024 * 1024))} className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9" /><select value={spec.transfer_enable_unit} onChange={(e) => updateSpec('transfer_enable_unit', e.target.value as 'GB' | 'MB')} className={selectClass + ' w-20'}><option value="GB" className="bg-zinc-800">GB</option><option value="MB" className="bg-zinc-800">MB</option></select></div><p className="text-xs text-zinc-500">0为不限</p></div>
                  {spec.protocol === 'anytls' && (
                    <div className="space-y-2"><Label className="text-zinc-300 text-sm">AnyTLS 填充方案</Label><select value={spec.padding_scheme || ''} onChange={(e) => updateSpec('padding_scheme', e.target.value)} className={selectClass}><option value="" className="bg-zinc-800">默认</option>{['max-0', 'max-1', 'max-2', 'max-3', 'max-4', 'max-5', 'max-6', 'max-7', 'max-8'].map(s => <option key={s} value={s} className="bg-zinc-800">{s}</option>)}</select><p className="text-xs text-zinc-500">仅 AnyTLS 协议</p></div>
                  )}
                </div>
                <div className="space-y-2"><Label className="text-zinc-300 text-sm">区域/线路</Label><Input value={spec.region || ''} onChange={(e) => updateSpec('region', e.target.value)} placeholder="如：香港/CN2 GIA" className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9" /></div>
                <div className="space-y-2"><Label className="text-zinc-300 text-sm">节点标签</Label>
                  <div className="flex gap-2"><Input value={newTag} onChange={(e) => setNewTag(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && (e.preventDefault(), addTag())} placeholder="输入标签按回车添加" className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 flex-1" /><Button type="button" variant="outline" size="sm" onClick={addTag} className="border-zinc-700 text-zinc-300 h-9"><Plus className="w-4 h-4 mr-1" />添加</Button></div>
                  {spec.tags.length > 0 && (<div className="flex flex-wrap gap-1.5 mt-2">{spec.tags.map(tag => (<Badge key={tag} variant="outline" className="gap-1 cursor-pointer border-zinc-700 bg-zinc-800 text-zinc-300 hover:text-red-400 hover:border-red-900 hover:bg-red-950/30" onClick={() => removeTag(tag)}>{tag}<X className="w-3 h-3" /></Badge>))}</div>)}
                </div>
                <div className="space-y-2"><Label className="text-zinc-300 text-sm">权限组（哪些套餐可见）</Label>
                  {permissionGroups.length === 0 ? (
                    <p className="text-xs text-zinc-500">暂无会员分组，请先在「节点分组」页面创建</p>
                  ) : (
                    <div className="flex flex-wrap gap-2">{permissionGroups.map(g => (<button key={g.value} type="button" onClick={() => togglePermissionGroup(g.value)} className={`px-3 py-1.5 rounded-lg text-sm border transition-colors ${spec.permission_groups.includes(g.value) ? 'bg-indigo-600/20 border-indigo-500 text-indigo-300' : 'bg-zinc-800/50 border-zinc-700 text-zinc-400 hover:border-zinc-600'}`}>{g.label}</button>))}</div>
                  )}
                </div>
                <div className="flex items-center justify-between p-3 rounded-lg bg-zinc-800/50 border border-zinc-700/50">
                  <div><div className="text-sm text-zinc-200">节点可见</div><div className="text-xs text-zinc-500">关闭后用户订阅中不显示此节点</div></div>
                  <Switch checked={spec.is_visible} onChange={(e) => updateSpec('is_visible', e.target.checked)} />
                </div>
              </TabsContent>

              <TabsContent value="connection" className="mt-0 space-y-5">
                <div className="space-y-2"><Label className="text-zinc-300 text-sm">节点地址 <span className="text-red-400">*</span></Label><Input value={spec.address} onChange={(e) => updateSpec('address', e.target.value)} placeholder="example.com 或 IP地址" className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9" /></div>
                <div className="grid grid-cols-[1fr,auto,1fr] gap-3 items-end">
                  <div className="space-y-2"><Label className="text-zinc-300 text-sm">客户端口 <span className="text-red-400">*</span><span className="text-xs text-zinc-500 ml-2">(TCP协议填443，UDP填高位端口)</span></Label><Input type="number" min="1" max="65535" value={spec.client_port} onChange={(e) => updateSpec('client_port', Number(e.target.value) || 0)} className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 font-mono" /></div>
                  <div className="pb-2"><button type="button" onClick={() => updateSpec('server_port', spec.client_port)} className="p-2 rounded-lg bg-zinc-800 border border-zinc-700 text-zinc-400 hover:text-indigo-400 hover:border-indigo-500 transition-colors" title="同步端口（直连节点用）"><Link2 className="w-4 h-4" /></button></div>
                  <div className="space-y-2"><Label className="text-zinc-300 text-sm">服务端监听端口 <span className="text-red-400">*</span><span className="text-xs text-zinc-500 ml-2">(Xray实际监听端口)</span></Label><Input type="number" min="1" max="65535" value={spec.server_port} onChange={(e) => updateSpec('server_port', Number(e.target.value) || 0)} className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 font-mono" /></div>
                </div>
                <div className="p-3 rounded-lg bg-zinc-800/30 border border-zinc-700/30">
                  <div className="text-xs text-zinc-400 space-y-1.5">
                    <p className="text-zinc-300 font-medium mb-1">标准架构端口填写规则（nginx 443 + SNI 分流）：</p>
                    <p><strong className="text-emerald-400">CDN节点</strong>（TCP协议，经CF CDN）：客户端口=<strong className="text-zinc-200">443</strong>，服务端口=高位端口（如9445）</p>
                    <p><strong className="text-blue-400">直连节点</strong>（REALITY/TCP，经nginx 443 SNI default）：客户端口=<strong className="text-zinc-200">443</strong>，服务端口=高位端口（如9450）</p>
                    <p><strong className="text-rose-400">UDP节点</strong>（Hysteria2/TUIC，不能走nginx 443）：客户端口=服务端口=高位端口（如40020），点同步按钮</p>
                    <p className="text-zinc-500 mt-1.5 pt-1.5 border-t border-zinc-700/50">注：标准架构下所有TCP协议客户端口都是443（nginx stream监听443）。UDP协议绕过nginx直接连xray端口。</p>
                  </div>
                </div>
                {showsAuth && (<div className="space-y-2">
                  <div className="flex items-center justify-between"><Label className="text-zinc-300 text-sm">UUID <span className="text-red-400">*</span></Label><Button type="button" variant="ghost" size="sm" onClick={generateUuidAction} className="h-7 px-2 text-xs text-indigo-400 hover:text-indigo-300"><RefreshCw className="w-3 h-3 mr-1" />生成UUID</Button></div>
                  <div className="flex gap-2"><Input value={spec.uuid || ''} onChange={(e) => updateSpec('uuid', e.target.value)} placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 font-mono text-xs flex-1" type={showFullUuid ? 'text' : 'password'} /><Button type="button" variant="ghost" size="icon" className="h-9 w-9 text-zinc-400" onClick={() => setShowFullUuid(!showFullUuid)}>{showFullUuid ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}</Button></div>
                </div>)}
                {showsPassword && (<div className="space-y-2">
                  <div className="flex items-center justify-between"><Label className="text-zinc-300 text-sm">密码 <span className="text-red-400">*</span></Label><Button type="button" variant="ghost" size="sm" onClick={() => updateSpec('password', generatePassword(spec.protocol, spec.method))} className="h-7 px-2 text-xs text-indigo-400 hover:text-indigo-300"><RefreshCw className="w-3 h-3 mr-1" />生成密码</Button></div>
                  <Input type="text" value={spec.password || ''} onChange={(e) => updateSpec('password', e.target.value)} placeholder="password" className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 font-mono text-xs" />
                </div>)}
                {spec.protocol === 'ss' && (<div className="space-y-2"><Label className="text-zinc-300 text-sm">加密方式</Label><select value={spec.method || 'aes-256-gcm'} onChange={(e) => updateSpec('method', e.target.value)} className={selectClass}>{ENCRYPTION_METHODS.map(m => <option key={m} value={m} className="bg-zinc-800">{m}</option>)}</select></div>)}
                {showsFlow && (<div className="space-y-2"><Label className="text-zinc-300 text-sm">流控 (Flow Control)</Label><select value={spec.flow || ''} onChange={(e) => updateSpec('flow', e.target.value)} className={selectClass}>{FLOW_OPTIONS.map(f => <option key={f.value} value={f.value} className="bg-zinc-800">{f.label}</option>)}</select></div>)}
                {spec.protocol === 'mieru' && (<div className="space-y-2"><Label className="text-zinc-300 text-sm">用户名 (可选)</Label><Input value={spec.username || ''} onChange={(e) => updateSpec('username', e.target.value)} placeholder="Mieru用户名" className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9" /></div>)}
              </TabsContent>

              <TabsContent value="transport" className="mt-0 space-y-5">
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2"><Label className="text-zinc-300 text-sm">传输协议</Label><select value={spec.transport} onChange={(e) => updateSpec('transport', e.target.value)} className={selectClass}>{availableTransports.map(t => <option key={t.value} value={t.value} className="bg-zinc-800">{t.label}</option>)}</select></div>
                </div>
                {needsTransportPath && (<div className="space-y-2">
                  <div className="flex items-center justify-between"><Label className="text-zinc-300 text-sm">路径 (Path)</Label><Button type="button" variant="ghost" size="sm" onClick={checkPathUnique} disabled={pathChecking} className="h-7 px-2 text-xs text-indigo-400 hover:text-indigo-300 disabled:opacity-50"><Route className="w-3 h-3 mr-1" />{pathChecking ? '检查中...' : '检查唯一性'}</Button></div>
                  <Input value={spec.path || '/'} onChange={(e) => updateSpec('path', e.target.value)} placeholder="/your-random-path" className={`bg-zinc-950 border-zinc-800 text-zinc-100 h-9 font-mono ${pathConflict?.exists ? 'border-red-500 focus:border-red-500' : pathConflict && !pathConflict.exists ? 'border-emerald-500 focus:border-emerald-500' : ''}`} />
                  {pathConflict?.exists && (<div className="flex items-start gap-2 p-2 rounded-lg bg-red-950/30 border border-red-900/50">
                    <AlertTriangle className="w-4 h-4 text-red-400 flex-shrink-0 mt-0.5" />
                    <div className="text-xs text-red-300 space-y-1">
                      <p>路径 <code className="text-red-200 bg-red-950 px-1 rounded">{spec.path}</code> 已被节点「<strong>{pathConflict.conflictingNodeName}</strong>」使用，同一运行时下路径必须唯一。</p>
                      {pathConflict.suggestedPath && (<p>建议路径：<button type="button" onClick={() => updateSpec('path', pathConflict.suggestedPath!)} className="text-indigo-400 hover:text-indigo-300 underline underline-offset-2 font-mono">{pathConflict.suggestedPath}</button></p>)}
                    </div>
                  </div>)}
                  {pathConflict && !pathConflict.exists && (<div className="flex items-center gap-2 p-2 rounded-lg bg-emerald-950/30 border border-emerald-900/50"><Check className="w-4 h-4 text-emerald-400" /><p className="text-xs text-emerald-300">路径可用，未发现冲突</p></div>)}
                </div>)}
                {needsTransportHost && (<div className="space-y-2"><Label className="text-zinc-300 text-sm">Host / SNI</Label><Input value={spec.host || ''} onChange={(e) => updateSpec('host', e.target.value)} placeholder="cdn.example.com" className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9" /></div>)}
                {needsServiceName && (<div className="space-y-2"><Label className="text-zinc-300 text-sm">gRPC Service Name</Label><Input value={spec.service_name || ''} onChange={(e) => updateSpec('service_name', e.target.value)} placeholder="grpc" className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9" /></div>)}
                {showsXHTTP && (<>
                  <div className="space-y-2"><Label className="text-zinc-300 text-sm">XHTTP 模式</Label><select value={spec.xhttp_mode || 'auto'} onChange={(e) => updateSpec('xhttp_mode', e.target.value)} className={selectClass}>{XHTTP_MODES.map(m => <option key={m} value={m} className="bg-zinc-800">{m}</option>)}</select></div>
                  <div className="p-3 rounded-lg bg-amber-950/30 border border-amber-900/50 flex items-start gap-2"><AlertTriangle className="w-4 h-4 text-amber-400 flex-shrink-0 mt-0.5" /><p className="text-xs text-amber-300"><strong>注意：</strong>stream-up/packet-up为实验性模式，仅v2rayNG验证通过，其他客户端可能失败</p></div>

                  <DownloadSettingsPanel spec={spec} updateSpec={updateSpec} />
                </>)}
                <Button type="button" variant="outline" onClick={() => setAdvancedOpen(true)} className="w-full border-dashed border-zinc-700 text-zinc-400 hover:border-indigo-500 hover:text-indigo-400 hover:bg-indigo-950/20 py-5"><Settings2 className="w-4 h-4 mr-2" />高级传输设置 (Mux多路复用/TCP Brutal/端口跳跃)<ChevronDown className="w-4 h-4 ml-auto" /></Button>
              </TabsContent>

              <TabsContent value="security" className="mt-0 space-y-5">
                <div className="space-y-2"><Label className="text-zinc-300 text-sm">安全层</Label>
                  {/* argo_tunnel 节点（CF 隧道）客户端必须 TLS：
                      cloudflared 明文 HTTP 回源，CF Edge 终止 TLS，DB security_type=none。
                      仅 argo_tunnel 需要锁定安全层，CDN 节点不锁定（TLS 剥离在渲染层动态完成） */}
                  {isArgoTunnelSpec && (
                    <div className="p-3 rounded-lg bg-amber-900/20 border border-amber-700/40 text-xs text-amber-300 flex items-start gap-2">
                      <AlertTriangle className="w-4 h-4 mt-0.5 flex-shrink-0" />
                      <div>CF 隧道节点（argo_tunnel）客户端必须开启 TLS（CF Edge 终止 TLS）。服务端 xray inbound 无 TLS（cloudflared 明文回源），二者为标准分离架构，安全层已锁定。</div>
                    </div>
                  )}
                  <div className="grid grid-cols-3 gap-2">{availableSecurities.map(s => {
                    // argo_tunnel 节点：禁用非 tls 选项（只有 tls 可选，其他变灰且禁止点击）
                    const isArgoTunnelForbidden = isArgoTunnelSpec && s.value !== 'tls'
                    return (
                      <button
                        key={s.value}
                        type="button"
                        onClick={() => { if (!isArgoTunnelForbidden) updateSpec('security', s.value) }}
                        disabled={isArgoTunnelForbidden}
                        className={`p-3 rounded-lg border text-left transition-all ${spec.security === s.value ? 'border-indigo-500 bg-indigo-950/30' : isArgoTunnelForbidden ? 'border-zinc-800 bg-zinc-900/30 opacity-40 cursor-not-allowed' : 'border-zinc-700 bg-zinc-800/30 hover:border-zinc-600'}`}
                      >
                        <div className="flex items-center gap-2 mb-1">{s.value === 'none' ? <Unlock className="w-4 h-4 text-zinc-500" /> : <Lock className="w-4 h-4 text-emerald-400" />}<span className="font-medium text-sm text-zinc-100">{s.label}</span></div>
                      </button>
                    )
                  })}</div>
                </div>
                {spec.security === 'tls' && (<div className="p-4 rounded-lg bg-zinc-800/50 border border-zinc-700/50 space-y-4">
                  <div className="text-sm font-medium text-zinc-200 flex items-center gap-2"><Shield className="w-4 h-4 text-blue-400" />TLS 设置</div>
                  <div className="space-y-2"><Label className="text-zinc-400 text-xs">SNI / 服务器名称</Label><Input value={spec.sni || ''} onChange={(e) => updateSpec('sni', e.target.value)} placeholder="example.com" className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9" /></div>
                  <div className="space-y-2"><Label className="text-zinc-400 text-xs">ALPN</Label><select value={spec.alpn || 'h2,http/1.1'} onChange={(e) => updateSpec('alpn', e.target.value)} className={selectClass}>{ALPN_OPTIONS.map(a => <option key={a} value={a} className="bg-zinc-800">{a}</option>)}</select></div>
                  <div className="space-y-2"><Label className="text-zinc-400 text-xs">uTLS 指纹 <span className="text-indigo-400">(重要：模拟浏览器TLS握手特征)</span></Label>
                    <select value={spec.utls_fingerprint || 'chrome'} onChange={(e) => updateSpec('utls_fingerprint', e.target.value)} className={selectClass}>{UTLS_FINGERPRINTS.map(f => <option key={f} value={f} className="bg-zinc-800">{f}</option>)}</select>
                    <p className="text-xs text-zinc-500">chrome为推荐值，模拟Chrome浏览器TLS指纹，对抗主动探测</p>
                  </div>
                  <div className="flex items-center justify-between p-3 rounded-lg bg-zinc-900/50 border border-zinc-800"><div><div className="text-sm text-zinc-200">允许不安全连接</div><div className="text-xs text-zinc-500">跳过证书验证（仅测试用）</div></div><Switch checked={spec.allow_insecure || false} onChange={(e) => updateSpec('allow_insecure', e.target.checked)} /></div>
                </div>)}
                {showRealityFields && (<div className="p-4 rounded-lg bg-zinc-800/50 border border-zinc-700/50 space-y-4">
                  <div className="text-sm font-medium text-zinc-200 flex items-center gap-2"><Shield className="w-4 h-4 text-emerald-400" />REALITY 设置</div>
                  <div className="space-y-2"><Label className="text-zinc-400 text-xs">伪装站点 (dest:port) <span className="text-rose-400">*</span></Label><Input value={spec.reality_dest || ''} onChange={(e) => updateSpec('reality_dest', e.target.value)} placeholder="127.0.0.1:9454（本地反代）或 oyc.yale.edu:443（伪装域名）" className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9" /><p className="text-[10px] text-zinc-500">必填。本地反代推荐（nginx vhost 回落真实站点），伪装域名用于直连模式</p></div>
                  <div className="space-y-2"><Label className="text-zinc-400 text-xs">SNI (Server Name)</Label><Input value={spec.sni || ''} onChange={(e) => updateSpec('sni', e.target.value)} placeholder="www.microsoft.com" className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9" /></div>
                  <div className="space-y-2">
                    <div className="flex items-center justify-between"><Label className="text-zinc-400 text-xs">公钥 (PublicKey)</Label><Button type="button" variant="ghost" size="sm" onClick={generateKeyPairAction} disabled={isGeneratingKey} className="h-7 px-2 text-xs text-indigo-400 hover:text-indigo-300 disabled:opacity-50"><Key className="w-3 h-3 mr-1" />{isGeneratingKey ? '生成中...' : '生成密钥对'}</Button></div>
                    <Input value={spec.public_key || ''} onChange={(e) => updateSpec('public_key', e.target.value)} placeholder="x25519 public key" className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 font-mono text-xs" />
                  </div>
                  <div className="space-y-2"><Label className="text-zinc-400 text-xs">私钥 (PrivateKey)</Label><Input value={spec.private_key || ''} onChange={(e) => updateSpec('private_key', e.target.value)} placeholder="x25519 private key" className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 font-mono text-xs" /></div>
                  <div className="space-y-2">
                    <div className="flex items-center justify-between"><Label className="text-zinc-400 text-xs">Short ID</Label><Button type="button" variant="ghost" size="sm" onClick={generateShortIdAction} className="h-7 px-2 text-xs text-indigo-400 hover:text-indigo-300"><RefreshCw className="w-3 h-3 mr-1" />随机生成</Button></div>
                    <Input value={spec.short_id || ''} onChange={(e) => updateSpec('short_id', e.target.value)} placeholder="16位hex，留空则为任意" className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 font-mono text-xs" />
                  </div>
                  <div className="space-y-2"><Label className="text-zinc-400 text-xs">uTLS指纹伪装 <span className="text-indigo-400">(重要)</span></Label>
                    <select value={spec.reality_utls_fingerprint || 'chrome'} onChange={(e) => updateSpec('reality_utls_fingerprint', e.target.value)} className={selectClass}>{UTLS_FINGERPRINTS.map(f => <option key={f} value={f} className="bg-zinc-800">{f}</option>)}</select>
                  </div>
                  <div className="space-y-2"><Label className="text-zinc-400 text-xs">SpiderX (可选)</Label><Input value={spec.spider_x || ''} onChange={(e) => updateSpec('spider_x', e.target.value)} placeholder="/" className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9" /></div>
                </div>)}
                {spec.security === 'none' && (<div className="p-3 rounded-lg bg-zinc-800/50 border border-zinc-700/50 flex items-start gap-2"><Unlock className="w-4 h-4 text-zinc-500 flex-shrink-0 mt-0.5" /><p className="text-xs text-zinc-400">无安全层：传输数据不加密，仅用于内部网络或已通过TLS隧道封装的场景。</p></div>)}
                {/* P-Chain: 链式套娃出站 —— 节点入站流量经此代理出站 */}
                <div className="space-y-2 p-3 rounded-lg bg-zinc-800/30 border border-zinc-700/50">
                  <Label className="text-zinc-300 text-sm">链式套娃出站 URI（可选）</Label>
                  <Input value={spec.chain_outbound_uri || ''} onChange={(e) => updateSpec('chain_outbound_uri', e.target.value)} placeholder="socks5://user:pass@host:port 或 trojan://pass@host:port?security=tls&sni=xxx" className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9 font-mono text-xs" />
                  <p className="text-[10px] text-zinc-500">填入后该节点入站流量将经此代理出站（套娃）。支持 socks5/http/trojan/vless/vmess/ss/hysteria2/tuic。留空=直连。上游失效自动降级 direct。</p>
                </div>
                {/* 节点暴露方式选择：direct / CF CDN / CF Tunnel / 上下行分离 */}
                <div className="space-y-3">
                  <Label className="text-zinc-300 text-sm">暴露方式（流量路径）</Label>
                  <div className="grid grid-cols-3 gap-2">
                    <button
                      key="direct"
                      type="button"
                      onClick={() => { updateSpec('exposure_mode', 'direct'); updateSpec('cdn_address', undefined); }}
                      className={`p-3 rounded-lg border text-left transition-all ${(spec as any).exposure_mode === 'direct' || !(spec as any).exposure_mode ? 'border-indigo-500 bg-indigo-950/30' : 'border-zinc-700 bg-zinc-800/30 hover:border-zinc-600'}`}
                    >
                      <div className="flex items-center gap-2 mb-1"><Server className="w-4 h-4 text-zinc-300" /><span className="font-medium text-sm text-zinc-100">直连</span></div>
                      <div className="text-xs text-zinc-500">客户端→VPS IP:端口（直连，无CDN）</div>
                    </button>
                    <button
                      key="cdn_saas"
                      type="button"
                      onClick={() => { updateSpec('exposure_mode', 'cdn_saas'); if (spec.host) updateSpec('cdn_address', spec.host); }}
                      className={`p-3 rounded-lg border text-left transition-all ${(spec as any).exposure_mode === 'cdn_saas' ? 'border-indigo-500 bg-indigo-950/30' : 'border-zinc-700 bg-zinc-800/30 hover:border-zinc-600'}`}
                    >
                      <div className="flex items-center gap-2 mb-1"><Globe className="w-4 h-4 text-blue-400" /><span className="font-medium text-sm text-zinc-100">CF CDN</span></div>
                      <div className="text-xs text-zinc-500">客户端→CF CDN→nginx 443 SSL→内核（需证书）</div>
                    </button>
                    <button
                      key="argo_tunnel"
                      type="button"
                      onClick={() => { updateSpec('exposure_mode', 'argo_tunnel'); }}
                      className={`p-3 rounded-lg border text-left transition-all ${(spec as any).exposure_mode === 'argo_tunnel' ? 'border-indigo-500 bg-indigo-950/30' : 'border-zinc-700 bg-zinc-800/30 hover:border-zinc-600'}`}
                    >
                      <div className="flex items-center gap-2 mb-1"><Route className="w-4 h-4 text-emerald-400" /><span className="font-medium text-sm text-zinc-100">CF Tunnel</span></div>
                      <div className="text-xs text-zinc-500">客户端→CF边缘→cloudflared→内核（HTTP，无需证书）</div>
                    </button>
                  </div>
                  <p className="text-xs text-zinc-500">
                    {(spec as any).exposure_mode === 'argo_tunnel'
                      ? 'CF Tunnel 模式：cloudflared 用 HTTP 直连内核 inbound（xray security=none，TLS 剥离），绕过 nginx，无需 SSL 证书。VPS 上需运行 cloudflared 进程（token 模式）。'
                      : (spec as any).exposure_mode === 'cdn_saas'
                        ? 'CF CDN 模式：CF CDN 回源到 nginx 443（SSL），nginx proxy_pass 到内核。需要为域名申请有效证书（不支持自签证书）。'
                        : '直连模式：客户端直接连接 VPS IP 或域名，通过端口访问。支持自签证书（allowInsecure=true）。'}
                  </p>

                  {/* 上下行分离开关 */}
                  {showsXHTTP && (
                    <div className="mt-3 pt-3 border-t border-zinc-800 space-y-3">
                      <div className="flex items-center justify-between p-3 rounded-lg bg-zinc-900/50 border border-zinc-800">
                        <div>
                          <div className="text-sm text-zinc-200 flex items-center gap-2"><GitBranch className="w-4 h-4 text-violet-400" />上下行分离（Split Mode）</div>
                          <div className="text-xs text-zinc-500 mt-0.5">上行走主暴露方式，下行走独立暴露方式（用于CDN→直连/REALITY等混合架构）</div>
                        </div>
                        <Switch
                          checked={(spec as any).is_split_mode === true}
                          onChange={(e: any) => {
                            const checked = e.target.checked;
                            updateSpec('is_split_mode', checked);
                            if (!checked) updateSpec('downstream_exposure_mode', undefined);
                          }}
                        />
                      </div>

                      {/* 下行暴露方式选择 */}
                      {(spec as any).is_split_mode && (
                        <div className="space-y-2 pl-4 border-l-2 border-violet-500/30">
                          <Label className="text-zinc-300 text-xs">下行暴露方式 <span className="text-rose-400">*</span></Label>
                          <select
                            value={(spec as any).downstream_exposure_mode || ''}
                            onChange={(e) => updateSpec('downstream_exposure_mode', e.target.value || undefined)}
                            className={selectClass}
                          >
                            <option value="" className="bg-zinc-800">-- 请选择下行暴露方式 --</option>
                            <option value="direct" className="bg-zinc-800">直连 (direct) - 客户端直连VPS IP:端口</option>
                            <option value="reality" className="bg-zinc-800">REALITY - 下行REALITY握手（无需证书）</option>
                            <option value="cdn_saas" className="bg-zinc-800">CF CDN - 下行经CDN（需有效证书）</option>
                          </select>

                          {/* 约束提示 */}
                          {(spec as any).downstream_exposure_mode === 'cdn_saas' && (
                            <div className="p-2 rounded bg-amber-950/30 border border-amber-800/50 flex items-start gap-2">
                              <AlertTriangle className="w-3.5 h-3.5 text-amber-400 flex-shrink-0 mt-0.5" />
                              <p className="text-xs text-amber-300">CDN模式要求为下行域名配置有效CA证书，自签证书不可用。请确保下行域名已解析并签发证书。</p>
                            </div>
                          )}
                          {(spec as any).downstream_exposure_mode === 'direct' && spec.security === 'tls' && (spec.allow_insecure === false || spec.allow_insecure === undefined) && (
                            <div className="p-2 rounded bg-sky-950/30 border border-sky-800/50 flex items-start gap-2">
                              <Shield className="w-3.5 h-3.5 text-sky-400 flex-shrink-0 mt-0.5" />
                              <p className="text-xs text-sky-300">直连模式可用自签证书，请在TLS设置中勾选"允许不安全连接"（仅测试用），或为下行域名配置有效CA证书。</p>
                            </div>
                          )}
                          {(spec as any).downstream_exposure_mode === 'reality' && (
                            <div className="p-2 rounded bg-emerald-950/30 border border-emerald-800/50 flex items-start gap-2">
                              <Shield className="w-3.5 h-3.5 text-emerald-400 flex-shrink-0 mt-0.5" />
                              <p className="text-xs text-emerald-300">REALITY模式无需证书，请在下方"XHTTP下行通道"中配置REALITY公钥/ShortID/SNI等参数。</p>
                            </div>
                          )}
                        </div>
                      )}
                    </div>
                  )}
                </div>
              </TabsContent>

              <TabsContent value="relay" className="mt-0 space-y-5">
                <div className="space-y-2"><Label className="text-zinc-300 text-sm">父节点 / 入口节点（单跳中转）</Label>
                  <select value={spec.parent_node_id || ''} onChange={(e) => updateSpec('parent_node_id', e.target.value)} className={selectClass}>
                    <option value="" className="bg-zinc-800">-- 无（落地节点，用户直连）--</option>
                    {parentNodes.map(n => (<option key={n.id} value={n.id} className="bg-zinc-800">{n.name} ({n.protocol}+{n.transport})</option>))}
                  </select>
                  <p className="text-xs text-zinc-500">选择父节点后，此节点作为落地节点，用户通过父节点中转到此节点。Xray用proxySettings.tag串联，Sing-box用detour字段。</p>
                </div>
                <Separator className="bg-zinc-800" />
                <div className="p-3 rounded-lg bg-indigo-950/20 border border-indigo-900/50 flex items-start gap-2"><GitBranch className="w-4 h-4 text-indigo-400 flex-shrink-0 mt-0.5" /><p className="text-xs text-indigo-300"><strong>多跳链式中转</strong>：当前版本通过父节点+自定义Outbounds实现多跳，最多支持5跳。可在"高级协议设置-自定义"中配置。</p></div>
              </TabsContent>

              <TabsContent value="binding" className="mt-0 space-y-5">
                <div className="space-y-3"><Label className="text-zinc-300 text-sm flex items-center gap-2"><HardDrive className="w-4 h-4 text-zinc-400" />绑定服务器<span className="text-xs text-zinc-500">SID很重要，对应YAML中的server_sid字段，与node-agent交互使用</span></Label>
                  <div className="space-y-2">{servers.length === 0 ? (
                    <div className="text-center py-6 text-xs text-zinc-500">
                      <HardDrive className="w-8 h-8 mx-auto mb-2 text-zinc-600" />
                      暂无可用服务器，请先在「服务器管理」页面添加
                    </div>
                  ) : servers.map(srv => {
                    const bound = spec.server_bindings.find(b => b.id === srv.id)
                    return (<button key={srv.id} type="button" onClick={() => toggleServerBinding(srv.id)} className={`w-full flex items-center justify-between p-3 rounded-lg border transition-all ${bound ? 'bg-emerald-950/20 border-emerald-800/50' : 'bg-zinc-800/30 border-zinc-700 hover:border-zinc-600'}`}>
                      <div className="flex items-center gap-3"><div className={`w-2.5 h-2.5 rounded-full ${srv.online ? 'bg-emerald-500' : 'bg-red-500'}`} /><div className="text-left"><div className="text-sm text-zinc-200 font-medium">{srv.name}</div><div className="text-xs text-zinc-500">SID: <span className="font-mono text-indigo-400">{srv.sid}</span> · {srv.ip}</div></div></div>
                      <div className="flex items-center gap-3">{bound && (<div className="flex items-center gap-1 text-xs text-emerald-400"><Check className="w-3.5 h-3.5" />由服务器管理</div>)}<div className={`w-5 h-5 rounded border flex items-center justify-center ${bound ? 'bg-emerald-600 border-emerald-500' : 'border-zinc-600'}`}>{bound && <Check className="w-3.5 h-3.5 text-white" />}</div></div>
                    </button>)
                  })}</div>
                </div>
                <Separator className="bg-zinc-800" />
                <div className="space-y-3"><Label className="text-zinc-300 text-sm flex items-center gap-2"><Route className="w-4 h-4 text-zinc-400" />路由组选择</Label>
                  {routeGroups.length === 0 ? (
                    <p className="text-xs text-zinc-500">暂无路由组（代理链），请先在「代理链」页面创建</p>
                  ) : (
                    <div className="flex flex-wrap gap-2">{routeGroups.map(g => (<button key={g.value} type="button" onClick={() => toggleRouteGroup(g.value)} className={`px-3 py-1.5 rounded-lg text-sm border transition-colors ${spec.route_groups.includes(g.value) ? 'bg-emerald-600/20 border-emerald-500 text-emerald-300' : 'bg-zinc-800/50 border-zinc-700 text-zinc-400 hover:border-zinc-600'}`}>{g.label}</button>))}</div>
                  )}
                  <p className="text-xs text-zinc-500">路由组决定此节点的分流策略</p>
                </div>
                <Separator className="bg-zinc-800" />
                <div className="space-y-3"><Label className="text-zinc-300 text-sm flex items-center gap-2"><Users className="w-4 h-4 text-zinc-400" />绑定套餐<span className="text-xs text-zinc-500">（不选=所有套餐可用）</span></Label>
                  {plans.length === 0 ? (
                    <p className="text-xs text-zinc-500">暂无套餐数据，请先在「套餐管理」页面创建</p>
                  ) : (
                    <div className="grid grid-cols-2 gap-2 max-h-40 overflow-y-auto p-2 bg-zinc-800/50 border border-zinc-700 rounded-lg">
                      {plans.map(p => {
                        const checked = spec.plan_ids.includes(p.id)
                        return (
                          <label
                            key={p.id}
                            className={`flex items-center gap-2 px-2 py-1.5 rounded cursor-pointer text-sm transition-colors ${
                              checked ? 'bg-indigo-900/40 text-indigo-300' : 'text-zinc-300 hover:bg-zinc-700/50'
                            }`}
                          >
                            <input
                              type="checkbox"
                              checked={checked}
                              onChange={() => togglePlan(p.id)}
                              className="rounded border-zinc-600 bg-zinc-800"
                            />
                            <span className="truncate">{p.name}</span>
                            {p.code && <span className="text-xs text-zinc-500">({p.code})</span>}
                          </label>
                        )
                      })}
                    </div>
                  )}
                  <p className="text-xs text-zinc-500">选择哪些套餐用户可以看到并使用此节点</p>
                </div>
                {certBundles.length > 0 && (
                  <>
                    <Separator className="bg-zinc-800" />
                    <div className="space-y-3">
                      <Label className="text-zinc-300 text-sm flex items-center gap-2"><Shield className="w-4 h-4 text-zinc-400" />TLS 证书包</Label>
                      <select
                        value={spec.cert_bundle_id || ''}
                        onChange={(e) => updateSpec('cert_bundle_id', e.target.value)}
                        className={selectClass}
                      >
                        <option value="" className="bg-zinc-800">不使用证书包</option>
                        {certBundles.map(c => (
                          <option key={c.id} value={c.id} className="bg-zinc-800">
                            {c.name}{c.domain ? ` (${c.domain})` : ''}
                          </option>
                        ))}
                      </select>
                      <p className="text-xs text-zinc-500">选择证书包后，节点将自动使用该证书进行 TLS 配置</p>
                    </div>
                  </>
                )}
                <Separator className="bg-zinc-800" />
                <Button type="button" variant="outline" onClick={() => setAdvancedOpen(true)} className="w-full border-dashed border-zinc-700 text-zinc-400 hover:border-indigo-500 hover:text-indigo-400 hover:bg-indigo-950/20 py-5"><Settings2 className="w-5 h-5 mr-2" /><div className="text-left"><div className="font-medium">高级协议设置</div><div className="text-xs text-zinc-500">TLS证书模式、多路复用、TCP Brutal、端口跳跃、自定义Outbounds/Routes</div></div><ChevronDown className="w-4 h-4 ml-auto" /></Button>
              </TabsContent>

              <TabsContent value="yaml" className="mt-0 space-y-4">
                <div className="p-3 rounded-lg bg-zinc-800/50 border border-zinc-700/50 flex items-start gap-2"><Code className="w-4 h-4 text-indigo-400 flex-shrink-0 mt-0.5" /><div className="text-xs text-zinc-400"><strong className="text-zinc-300">高级模式</strong>：直接编辑原始YAML/JSON配置。表单字段双向同步。</div></div>
                <YamlEditor value={yamlText} onChange={setYamlText} mode={editorMode} onModeChange={setEditorMode} height={450} placeholder="输入节点配置..." error={yamlEditorError} />
              </TabsContent>

              <TabsContent value="raw_config" className="mt-0 space-y-4">
                <div className="p-3 rounded-lg bg-amber-950/20 border border-amber-900/50 flex items-start gap-2">
                  <FileCode className="w-4 h-4 text-amber-400 flex-shrink-0 mt-0.5" />
                  <div className="text-xs text-zinc-400">
                    <strong className="text-amber-300">Raw config_json 编辑器</strong>：直接编辑发送给后端的 config_json 字段。
                    表单中已配置的字段会自动同步到此；你可以在此添加表单不支持的字段（如 cdn_port、cdn_path、seed、quic_key 等）。
                    {rawConfigDirty && <span className="text-amber-400 ml-1">（有未保存的修改）</span>}
                  </div>
                </div>
                <YamlEditor
                  value={rawConfigText}
                  onChange={(v) => { setRawConfigText(v); setRawConfigDirty(true); const r = tryParseJson(v); setRawConfigError(r.valid ? null : r.error || 'JSON 解析错误') }}
                  mode="json"
                  height={450}
                  placeholder="{}"
                  error={rawConfigError}
                  showModeToggle={false}
                />
                <div className="flex items-center gap-2">
                  <Button type="button" variant="outline" size="sm" onClick={() => { setRawConfigText(JSON.stringify(generateConfigJsonPreview(spec), null, 2)); setRawConfigDirty(false); setRawConfigError(null) }} className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 text-xs">
                    <RefreshCw className="w-3 h-3 mr-1" />从表单重新生成
                  </Button>
                  {rawConfigDirty && (
                    <span className="text-xs text-amber-400">修改后将覆盖表单生成的 config_json</span>
                  )}
                </div>
              </TabsContent>
            </div>
          </Tabs>

          <div className="border-t border-zinc-800 p-4 flex-shrink-0 bg-zinc-900">
            <div className="flex items-center justify-between gap-4 mb-3">
              <div className="flex items-center gap-4">
                <div className="flex items-center gap-2">{isValidating ? <div className="w-2 h-2 rounded-full bg-amber-500 animate-pulse" /> : validation.xrayValid ? <Check className="w-4 h-4 text-emerald-400" /> : <X className="w-4 h-4 text-red-400" />}<span className="text-xs text-zinc-400">{validation.xrayValid ? <span className="text-emerald-400">Xray校验通过</span> : <span className="text-red-400">Xray: {validation.xrayError}</span>}</span></div>
                <div className="flex items-center gap-2">{isValidating ? <div className="w-2 h-2 rounded-full bg-amber-500 animate-pulse" /> : validation.singboxValid ? <Check className="w-4 h-4 text-emerald-400" /> : <X className="w-4 h-4 text-red-400" />}<span className="text-xs text-zinc-400">{validation.singboxValid ? <span className="text-emerald-400">Sing-box校验通过</span> : <span className="text-red-400">Sing-box: {validation.singboxError}</span>}</span></div>
              </div>
              <div className="flex items-center gap-2">{previewLink && (<div className="hidden lg:flex items-center gap-2 px-3 py-1.5 rounded-lg bg-zinc-800 border border-zinc-700 max-w-md"><div className="min-w-0 flex-1"><div className="flex items-center justify-between"><span className="text-[10px] text-zinc-500">URI预览（可直接导入客户端）</span><Button type="button" variant="ghost" size="sm" onClick={handleCopyLink} className="h-6 w-6 p-0 text-zinc-400 hover:text-zinc-200">{copiedLink ? <Check className="w-3 h-3 text-emerald-400" /> : <Copy className="w-3 h-3" />}</Button></div><p className="text-[9px] text-zinc-400 font-mono truncate">{previewLink}</p></div></div>)}</div>
            </div>
            <DialogFooter className="gap-2 p-0">
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)} className="border-zinc-700 text-zinc-300 hover:bg-zinc-800">取消</Button>
              {isEdit && (<Button type="button" variant="outline" onClick={handleReset} className="border-zinc-700 text-zinc-400 hover:bg-zinc-800">重置</Button>)}
              <Button type="button" onClick={handleSave} disabled={!formValid} className="bg-indigo-600 hover:bg-indigo-500 disabled:bg-zinc-700 disabled:text-zinc-500 disabled:cursor-not-allowed"><Zap className="w-4 h-4 mr-2" />{isEdit ? '保存并部署' : '创建节点'}</Button>
            </DialogFooter>
          </div>
        </DialogContent>
      </Dialog>
      <Dialog open={savePresetDialog} onOpenChange={setSavePresetDialog}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2"><Save className="w-5 h-5 text-indigo-400" />保存为自定义预设</DialogTitle>
            <DialogDescription className="text-zinc-400">将当前表单的协议/传输/安全层配置保存为可复用的预设模板（不保存节点名称/地址/密钥等实例信息）</DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-1.5">
              <Label className="text-zinc-300 text-sm">预设名称 <span className="text-red-400">*</span></Label>
              <Input value={newPresetName} onChange={(e) => setNewPresetName(e.target.value)} placeholder="如：XHTTP-Reality-日本专线" className="bg-zinc-950 border-zinc-800 text-zinc-100 h-9" />
            </div>
            <div className="space-y-1.5">
              <Label className="text-zinc-300 text-sm">描述（可选）</Label>
              <Textarea value={newPresetDesc} onChange={(e) => setNewPresetDesc(e.target.value)} placeholder="说明该预设的适用场景..." className="bg-zinc-950 border-zinc-800 text-zinc-100 min-h-[60px]" />
            </div>
            <div className="p-3 rounded-lg bg-zinc-800/50 border border-zinc-700/50 text-xs text-zinc-400">
              <p className="text-zinc-300 font-medium mb-1">将保存以下协议参数：</p>
              <p>协议: <span className="text-zinc-200 font-mono">{spec.protocol}</span> · 传输: <span className="text-zinc-200 font-mono">{spec.transport}</span> · 安全: <span className="text-zinc-200 font-mono">{spec.security}</span></p>
              {spec.download_settings?.enabled && <p className="text-indigo-300 mt-1">含上下行分离（XHTTP downloadSettings）配置</p>}
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setSavePresetDialog(false)} className="border-zinc-700 text-zinc-300">取消</Button>
            <Button type="button" onClick={handleSaveAsPreset} disabled={createPreset.isPending} className="bg-indigo-600 hover:bg-indigo-500"><Save className="w-4 h-4 mr-1" />{createPreset.isPending ? '保存中...' : '保存预设'}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <AdvancedSettingsDialog open={advancedOpen} onOpenChange={setAdvancedOpen} config={spec.advanced} onConfigChange={updateAdvanced} />
    </>
  )
}
