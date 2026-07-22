import { useState, useEffect, useMemo, useCallback } from 'react'
import {
  Server,
  Activity,
  Plus,
  Search,
  Upload,
  Eye,
  EyeOff,
  Edit,
  Trash2,
  Copy,
  RefreshCw,
  Globe,
  Zap,
  Shield,
  Check,
  X,
  Wifi,
  WifiOff,
  Link2,
  Layers,
  HardDrive,
  Loader2,
  Send,
  Route,
  Users,
  Gauge,
  Smartphone,
  AlertTriangle,
} from 'lucide-react'
import {
  Card,
  CardContent,
  Badge,
  Button,
  Input,
  Label,
  Skeleton,
  Switch,
  Select,
  useToast,
  Separator,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@airport/ui'
import { api, ApiError } from '@/lib/api'
import { EP } from '@/lib/endpoints'
import { Checkbox } from '@/components/common/Checkbox'
import { NodeConfigEditor, type NodeSpec } from '@/components/nodes/NodeConfigEditor'
import { useSearchParams } from 'react-router-dom'

// UUID v4 格式校验正则（与后端 google/uuid 包兼容，大小写不敏感）
// 用于在提交前拦截非法 runtime_id，避免后端返回 "invalid UUID length: N" 这类晦涩错误
const UUID_V4_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

interface NodeTlsSettings {
  server_name?: string
  sni?: string
  allow_insecure?: boolean | number
  alpn?: string | string[]
  ech?: { enabled?: boolean; [k: string]: unknown }
  cert_file?: string
  key_file?: string
  fingerprint?: string
}

interface NodeRealitySettings {
  server_name?: string
  server_port?: number
  public_key?: string
  private_key?: string
  short_id?: string
  allow_insecure?: boolean | number
  spider_x?: string
  reality_dest?: string
  fingerprint?: string
}

interface NodeNetworkSettings {
  header?: { type?: string }
  path?: string
  host?: string | string[]
  serviceName?: string
  acceptProxyProtocol?: boolean
}

interface NodeMultiplex {
  enabled?: boolean | number
  protocol?: string
  max_connections?: number
  padding?: boolean | number
  brutal?: {
    enabled?: boolean | number
    up_mbps?: number
    down_mbps?: number
  }
}

interface NodeUtls {
  enabled?: boolean | number
  fingerprint?: string
}

interface NodeConfigData {
  tls?: 0 | 1 | 2
  network?: 'tcp' | 'ws' | 'grpc' | 'h2' | 'httpupgrade' | 'xhttp' | 'kcp' | 'quic' | 'udp'
  flow?: string
  encryption?: { enabled?: boolean; encryption?: string; decryption?: string }
  tls_settings?: NodeTlsSettings
  reality_settings?: NodeRealitySettings
  network_settings?: NodeNetworkSettings
  multiplex?: NodeMultiplex
  utls?: NodeUtls
  custom_outbounds?: unknown[]
  custom_routes?: unknown[]
  uuid?: string
  password?: string
  method?: string
  username?: string
  client_port?: number
  server_port?: number
  preset_id?: string
  runtime?: string
  security?: string
  security_type?: string
  [k: string]: unknown
}

interface Node {
  id: string
  code: string
  name: string
  protocol_type: string
  transport_type: string
  address: string
  port: number
  is_enabled: boolean
  is_visible: boolean
  traffic_rate: number
  health_status?: 'healthy' | 'degraded' | 'unhealthy' | string
  // P2-I: 在线连接数（后端 NodeResponse 暂未返回，通过 CHANNEL_HEALTH 列表聚合填充）
  online_count?: number
  // P2-J: 节点流量限额相关字段（后端 NodeResponse 已返回 speed_limit_mbps/device_limit/padding_scheme）
  speed_limit_mbps?: number
  device_limit?: number
  padding_scheme?: string
  // transfer_enable_bytes: 后端 Node 模型有此字段，预留读取（NodeResponse 暂未返回）
  transfer_enable_bytes?: number
  tags?: string[]
  region_id?: string
  group_id?: string
  runtime_id?: string
  // DB 列：gRPC serviceName / WS path / XHTTP path（后端 NodeResponse 返回，用于编辑回显 fallback）
  path?: string
  // DB 列：服务端监听端口（后端 NodeResponse 返回，用于编辑回显，是唯一真相源）
  server_port?: number
  config?: string | NodeConfigData | null
  settings?: string | Record<string, unknown> | null
  stream_settings?: string | Record<string, unknown> | null
  plan_codes?: string[]
  // P1-1: 绑定的套餐 ID 列表（后端返回，用于编辑回显）
  plan_ids?: string[]
  // P1-3: 绑定的证书包 ID
  cert_bundle_id?: string
  // P1: 保存成功后的警告信息
  warnings?: string[]
  created_at?: string
  updated_at?: string
  // 后端 NodeResponse 扩展字段（用于编辑回显）
  // server_info: 当前 runtime 绑定的服务器摘要 { id, code, name, host }
  server_info?: { id: string; code: string; name: string; host: string }
  // chain_ids: 节点绑定的代理链 UUID 数组（对应 route_groups）
  chain_ids?: string[]
  // group_info: 节点所属会员分组摘要 { id, code, name }（主分组，向后兼容）
  group_info?: { id: string; code: string; name: string }
  // group_ids: 节点所属的所有分组 ID 列表（多对多，用于编辑回显）
  group_ids?: string[]
  // 上下行分离（split mode）：P2 exposure_mode 架构改造
  is_split_mode?: boolean
  downstream_exposure_mode?: string
  // groups: 节点所属的所有分组简要信息（多对多）
  groups?: { id: string; code: string; name: string }[]
  // 配置下发状态
  dispatch_status?: 'pending' | 'pushed' | 'applied' | 'failed' | string
  dispatch_version?: number
  dispatch_time?: string
  dispatch_error?: string
  security_type?: string
  sni?: string
  // 修复：DB 列字段，用于编辑回显 fallback
  reality_server_name?: string
  priority?: number
}

// 节点路由绑定（对齐后端 routing/model.go BindingResponse）
interface RouteBinding {
  node_id: string
  policy_id: string
  bind_scope: string
  inbound_tag?: string
  created_at: string
}

// 路由策略摘要（用于绑定下拉，对齐后端 PolicyResponse 子集）
interface RoutePolicyOption {
  id: string
  code: string
  name: string
  description?: string
  status: string
}

const PROTOCOL_COLORS: Record<string, string> = {
  vless: 'bg-emerald-500/20 text-emerald-400 border-emerald-500/30',
  vmess: 'bg-blue-500/20 text-blue-400 border-blue-500/30',
  trojan: 'bg-purple-500/20 text-purple-400 border-purple-500/30',
  shadowsocks: 'bg-amber-500/20 text-amber-400 border-amber-500/30',
  ss: 'bg-amber-500/20 text-amber-400 border-amber-500/30',
  hysteria2: 'bg-rose-500/20 text-rose-400 border-rose-500/30',
  tuic: 'bg-cyan-500/20 text-cyan-400 border-cyan-500/30',
  anytls: 'bg-violet-500/20 text-violet-400 border-violet-500/30',
  mieru: 'bg-lime-500/20 text-lime-400 border-lime-500/30',
}

const PROTOCOL_DOT_COLORS: Record<string, string> = {
  vless: 'bg-emerald-500',
  vmess: 'bg-blue-500',
  trojan: 'bg-purple-500',
  shadowsocks: 'bg-amber-500',
  ss: 'bg-amber-500',
  hysteria2: 'bg-rose-500',
  tuic: 'bg-cyan-500',
  anytls: 'bg-violet-500',
  mieru: 'bg-lime-500',
}

const PROTOCOL_FILTERS = [
  { value: 'all', label: '全部协议' },
  { value: 'vless', label: 'VLESS' },
  { value: 'vmess', label: 'VMess' },
  { value: 'trojan', label: 'Trojan' },
  { value: 'shadowsocks', label: 'Shadowsocks' },
  { value: 'hysteria2', label: 'Hysteria2' },
  { value: 'tuic', label: 'TUIC' },
]

const STATUS_FILTERS = [
  { value: 'all', label: '全部状态' },
  { value: 'online', label: '在线' },
  { value: 'pending', label: '待激活' },
  { value: 'offline', label: '离线' },
  { value: 'hidden', label: '已隐藏' },
  { value: 'disabled', label: '已禁用' },
]

const SORT_OPTIONS = [
  { value: 'sort', label: '默认排序' },
  { value: 'name', label: '按名称' },
  { value: 'traffic', label: '按流量' },
  { value: 'online', label: '按在线状态' },
]

const DISPATCH_STATUS_CONFIG: Record<string, { label: string; badgeClass: string; dotClass: string }> = {
  pending: {
    label: '待下发',
    badgeClass: 'bg-amber-500/20 text-amber-400 border-amber-500/30',
    dotClass: 'bg-amber-500',
  },
  pushed: {
    label: '已推送',
    badgeClass: 'bg-blue-500/20 text-blue-400 border-blue-500/30',
    dotClass: 'bg-blue-500',
  },
  applied: {
    label: '已生效',
    badgeClass: 'bg-emerald-500/20 text-emerald-400 border-emerald-500/30',
    dotClass: 'bg-emerald-500',
  },
  failed: {
    label: '下发失败',
    badgeClass: 'bg-red-500/20 text-red-400 border-red-500/30',
    dotClass: 'bg-red-500',
  },
}

function parseTags(tags: string | string[] | null | undefined): string[] {
  if (Array.isArray(tags)) return tags
  if (typeof tags === 'string') {
    if (!tags) return []
    try {
      const parsed = JSON.parse(tags)
      if (Array.isArray(parsed)) return parsed as string[]
    } catch {
      return tags.split(',').map(s => s.trim()).filter(Boolean)
    }
  }
  return []
}

function parseConfig(config: string | NodeConfigData | null | undefined): NodeConfigData {
  if (!config) return {}
  if (typeof config === 'string') {
    try { return JSON.parse(config) as NodeConfigData } catch { return {} }
  }
  return config
}

function cfgStr(cfg: NodeConfigData, key: string): string | undefined {
  const v = cfg[key]
  return typeof v === 'string' ? v : undefined
}

function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return `${(bytes / Math.pow(1024, i)).toFixed(2)} ${units[i]}`
}

function formatDispatchTime(ts: string | undefined): string {
  if (!ts) return ''
  try {
    const d = new Date(ts)
    if (isNaN(d.getTime())) return ts
    return d.toLocaleString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    })
  } catch {
    return ts
  }
}

function nodeToSpec(node: Node): Partial<NodeSpec> {
  const cfg = parseConfig(node.config)
  // TLS 分离回显（仅 argo_tunnel/CF 隧道节点需要）：
  // - argo_tunnel：DB security_type="none"（xray 无TLS），config_json.security_type="tls"（客户端 TLS）
  //   回显时需读 config_json（客户端面向），否则表单会显示 "none" 看起来像没开 TLS
  // - cdn/cdn_saas/direct/reality：DB 和 config_json 一致（均为 tls/reality/none），直接读 DB 即可
  //   CDN 节点的 xray inbound TLS 剥离在后端渲染层由 shouldStripTLSForNginxVhost 动态完成，
  //   不在 DB 字段层面做分离
  const exposureMode = cfgStr(cfg, 'exposure_mode')
  const isTLSSplit = exposureMode === 'argo_tunnel'
  const cfgSec = isTLSSplit
    ? (cfgStr(cfg, 'security_type') || cfgStr(cfg, 'security') || 'tls')
    : (node.security_type || cfgStr(cfg, 'security') || cfgStr(cfg, 'security_type') || 'none')
  const tls = cfg.tls ?? (cfgSec === 'reality' ? 2 : cfgSec === 'tls' ? 1 : 0)
  const network = (node.transport_type || cfg.network || 'tcp') as NodeConfigData['network']
  const netHost = cfg.network_settings?.host
  const netHostStr = Array.isArray(netHost) ? netHost.join(',') : (typeof netHost === 'string' ? netHost : '')
  const alpnVal = cfg.tls_settings?.alpn || cfg.alpn
  const alpnStr = Array.isArray(alpnVal) ? alpnVal.join(',') : (typeof alpnVal === 'string' ? alpnVal : '')
  const tags = parseTags(node.tags)

  // 安全层推断：TLS 分离节点（仅 argo_tunnel）读 config_json（客户端），其余节点读 DB
  const secVal = isTLSSplit
    ? (cfgStr(cfg, 'security_type') || cfgStr(cfg, 'security'))
    : (node.security_type || cfgStr(cfg, 'security') || cfgStr(cfg, 'security_type'))
  let security = 'none'
  if (tls === 1) security = 'tls'
  else if (tls === 2) security = 'reality'
  else if (secVal) security = secVal

  const muxEnabled = !!cfg.multiplex?.enabled
  const brutalEnabled = !!cfg.multiplex?.brutal?.enabled

  const numericId = parseInt(node.id, 10) || 0

  // P2-J: 流量限额相关字段从节点数据读取（替代硬编码 0）
  const teBytes = Number(node.transfer_enable_bytes) || 0
  const GB = 1024 * 1024 * 1024
  // 单位推断：能被 GB 整除用 GB，否则用 MB
  const teUnit: 'GB' | 'MB' = teBytes > 0 && teBytes % GB !== 0 ? 'MB' : 'GB'

  return {
    numeric_id: numericId,
    id: node.id,
    code: node.code,
    protocol: node.protocol_type,
    name: node.name,
    multiplier: Number(node.traffic_rate) || 1.0,
    traffic_limit: teBytes > 0 ? teBytes / GB : 0,
    device_limit: Number(node.device_limit) || 0,
    speed_limit_mbps: Number(node.speed_limit_mbps) || 0,
    transfer_enable_bytes: teBytes,
    transfer_enable_unit: teUnit,
    padding_scheme: node.padding_scheme || '',
    tags,
    // 多对多分组：优先使用后端返回的 group_ids 数组，回退到 group_id 单值
    permission_groups: Array.isArray(node.group_ids) && node.group_ids.length > 0
      ? [...node.group_ids]
      : (node.group_id ? [node.group_id] : []),
    address: node.address,
    port: Number(node.port) || 443,
    // 标准化回显：client_port 优先从 DB 列 node.port 读取（standardizeNodeFields 已同步到 config_json.client_port）
    // server_port 优先从 DB 列 node.server_port 读取（后端 NodeResponse 返回）
    client_port: Number(node.port || cfg.client_port || cfg.server_port) || 443,
    server_port: Number(node.server_port || cfg.server_port || cfg.client_port || node.port) || 443,
    transport: network || 'tcp',
    security,
    // XHTTP 协议不使用 flow（flow=xtls-rprx-vision 仅适用于 TCP+REALITY Vision）
    flow: network === 'xhttp' ? '' : (cfgStr(cfg, 'flow') || ''),
    host: netHostStr || cfgStr(cfg, 'host') || (cfg as any)?.xhttp?.host || (cfg as any)?.ws?.host || node.host_header || undefined,
    path: cfgStr(cfg, 'path') || (cfg as any)?.xhttp?.path || (cfg as any)?.ws?.path || cfg.network_settings?.path || node.path || undefined,
    // gRPC serviceName 回退顺序：config_json.service_name → network_settings.serviceName → node.path（DB 列）
    // 修复 gRPC 节点编辑回显：normalizer 之前会删除 config_json.service_name，需从 node.path 回退
    service_name: cfgStr(cfg, 'service_name') || cfg.network_settings?.serviceName || (network === 'grpc' && node.path ? node.path : undefined),
    // SNI: 标准化回显——优先从 DB 列 node.sni 读取（唯一真相源），回退到 config_json
    sni: node.sni || cfgStr(cfg, 'sni') || cfgStr(cfg, 'server_name') || (cfg as any)?.reality?.sni || cfg.tls_settings?.sni || cfg.tls_settings?.server_name || cfg.reality_settings?.server_name || undefined,
    alpn: alpnStr || undefined,
    allow_insecure: !!cfg.allow_insecure || !!cfg.tls_settings?.allow_insecure,
    utls_fingerprint: cfgStr(cfg, 'utls_fingerprint') || cfgStr(cfg, 'fingerprint') || cfg.utls?.fingerprint || undefined,
    // 修复：reality_dest 回显——优先从顶层 reality_dest 读取（normalizer 拍平后的位置）
    // 不再从 server_name + 443 拼接（server_name 是 SNI 域名，不是 dest）
    reality_dest: cfgStr(cfg, 'reality_dest') || (cfg as any)?.reality?.reality_dest || cfg.reality_settings?.reality_dest || undefined,
    // P-Chain: 链式套娃出站 URI 回显（从 config_json.chain_outbound_uri 读取）
    chain_outbound_uri: cfgStr(cfg, 'chain_outbound_uri') || undefined,
    public_key: cfgStr(cfg, 'public_key') || (cfg as any)?.reality?.public_key || cfg.reality_settings?.public_key || undefined,
    private_key: cfgStr(cfg, 'private_key') || (cfg as any)?.reality?.private_key || cfg.reality_settings?.private_key || undefined,
    short_id: cfgStr(cfg, 'short_id') || (cfg as any)?.reality?.short_id || cfg.reality_settings?.short_id || undefined,
    spider_x: cfgStr(cfg, 'spider_x') || (cfg as any)?.reality?.spider_x || cfg.reality_settings?.spider_x || undefined,
    parent_node_id: cfgStr(cfg, 'parent_node_id') || '',
    parent_numeric_id: 0,
    // 编辑回显：优先使用后端返回的 server_info（含 id/name/host）
    // 兜底使用 runtime_id（不含 id/name，UI 会显示空值但 runtime_id 仍可保存）
    server_bindings: node.server_info
      ? [{ id: node.server_info.id, name: node.server_info.name || node.server_info.code || node.server_info.host, sid: 0, auto_manage: true, runtime_id: node.runtime_id }]
      : node.runtime_id
        ? [{ id: '', name: '', sid: 0, auto_manage: true, runtime_id: node.runtime_id }]
        : [],
    // 编辑回显：使用后端返回的 chain_ids（UUID 数组，对应 route_groups）
    route_groups: Array.isArray(node.chain_ids) ? node.chain_ids : [],
    // P1-1: 编辑回显已绑定的套餐（优先使用 plan_ids，回退到 plan_codes）
    plan_ids: Array.isArray(node.plan_ids) && node.plan_ids.length > 0
      ? [...node.plan_ids]
      : Array.isArray(node.plan_codes) ? [...node.plan_codes] : [],
    // P1-3: 编辑回显证书包
    cert_bundle_id: node.cert_bundle_id || '',
    is_visible: !!node.is_visible,
    // 修复：priority 优先读 DB 列 node.priority（normalizer 之前会删除 cfg.priority）
    priority: Number(node.priority) || Number(cfg.priority) || 0,
    uuid: cfgStr(cfg, 'uuid'),
    password: cfgStr(cfg, 'password'),
    method: cfgStr(cfg, 'method'),
    username: cfgStr(cfg, 'username'),
    xhttp_mode: cfgStr(cfg, 'xhttp_mode') || cfgStr(cfg, 'mode') || (cfg as any)?.xhttp?.mode || undefined,
    region: cfgStr(cfg, 'region') || undefined,
    reality_utls_enabled: !!(cfg.reality_utls_enabled) || (!!cfgStr(cfg, 'reality_utls_fingerprint')),
    reality_utls_fingerprint: cfgStr(cfg, 'reality_utls_fingerprint') || undefined,
    download_settings: (() => {
      // P0 修复：优先从正确存储路径 xhttp.extra.downloadSettings 读取（兼容顶层 download_settings 老数据）
      const ds = ((cfg as any)?.xhttp?.extra?.downloadSettings as Record<string, unknown> | undefined)
        || (cfg.download_settings as Record<string, unknown> | undefined)
      if (!ds || typeof ds !== 'object') return undefined
      const dlReality = (ds.reality as Record<string, unknown> | undefined)
        || (ds.realitySettings as Record<string, unknown> | undefined) || {}
      const dlTls = (ds.tls as Record<string, unknown> | undefined)
        || (ds.tlsSettings as Record<string, unknown> | undefined) || {}
      const dlXhttp = (ds.xhttpSettings as Record<string, unknown> | undefined) || {}
      return {
        enabled: true,
        address: String(ds.address ?? ''),
        port: Number(ds.port) || 443,
        network: (ds.network as 'tcp' | 'xhttp') || 'xhttp',
        security: (ds.security as 'tls' | 'reality' | 'none') || 'tls',
        mode: (() => {
          const sec = (ds.security as string) || 'tls'
          const defaultMode = sec === 'reality' ? 'stream-up' : 'packet-up'
          return (String(dlXhttp.mode ?? ds.mode ?? defaultMode) as 'packet-up' | 'stream-up' | 'stream-one')
        })(),
        // 修复：sni 读取优先级——ds.sni > dlReality.sni > dlTls.serverName（仅当不是 IP:Port 时）
        // 避免历史数据中 serverName 被误填为 dest（IP:Port）导致 SNI 错误
        sni: (() => {
          const candidates = [
            ds.sni,
            dlReality.sni,
            dlReality.serverName,
            dlReality.server_name,
            dlTls.serverName,
            dlTls.server_name,
          ]
          for (const c of candidates) {
            if (typeof c !== 'string' || !c) continue
            // 排除 IP:Port 格式（dest 误填）
            if (/^\d+\.\d+\.\d+\.\d+:\d+$/.test(c) || /^127\.0\.0\.1:\d+$/.test(c)) continue
            return c
          }
          return ''
        })(),
        host: String(dlXhttp.host ?? ds.host ?? ''),
        path: String(dlXhttp.path ?? ds.path ?? ''),
        public_key: String(dlReality.publicKey ?? dlReality.public_key ?? ''),
        private_key: String(dlReality.privateKey ?? dlReality.private_key ?? ''),
        short_id: String(dlReality.shortId ?? dlReality.short_id ?? ''),
        // 修复：server_name 是 SNI 域名，排除 IP:Port 格式（历史数据可能误填为 dest）
        server_name: (() => {
          const candidates = [
            dlReality.serverName,
            dlReality.server_name,
            ds.serverName,
            ds.server_name,
          ]
          for (const c of candidates) {
            if (typeof c !== 'string' || !c) continue
            if (/^\d+\.\d+\.\d+\.\d+:\d+$/.test(c) || /^127\.0\.0\.1:\d+$/.test(c)) continue
            return c
          }
          return ''
        })(),
        // 修复：dest 单独存储，从 realitySettings.dest 或 顶层 reality_dest 或 误填的 serverName(IP:Port) 读取
        dest: (() => {
          // 优先从正确的 dest 字段读取
          if (typeof dlReality.dest === 'string' && dlReality.dest) return dlReality.dest
          if (typeof dlReality.reality_dest === 'string' && dlReality.reality_dest) return dlReality.reality_dest
          if (typeof ds.reality_dest === 'string' && ds.reality_dest) return ds.reality_dest
          // 容错：历史数据中 dest 被误填到 serverName 字段（IP:Port 格式）
          const misplaced = [
            dlReality.serverName,
            dlReality.server_name,
            ds.serverName,
            ds.server_name,
          ]
          for (const c of misplaced) {
            if (typeof c === 'string' && c && /^\d+\.\d+\.\d+\.\d+:\d+$/.test(c)) return c
          }
          return ''
        })(),
        fingerprint: String(dlReality.fingerprint ?? dlTls.fingerprint ?? ds.fingerprint ?? 'chrome'),
        alpn: (() => {
          const alpnRaw = dlTls.alpn ?? dlReality.alpn ?? ds.alpn
          if (Array.isArray(alpnRaw)) return alpnRaw.join(',')
          return String(alpnRaw ?? '')
        })(),
        no_grpc_header: !!(dlXhttp.noGRPCHeader ?? dlXhttp.no_grpc_header ?? ds.no_grpc_header ?? false),
        allow_insecure: !!(dlTls.allowInsecure ?? dlTls.allow_insecure ?? ds.allow_insecure ?? false),
      }
    })(),
    preset_id: cfgStr(cfg, 'preset_id'),
    // 修复：CDN 字段回显（之前 nodeToSpec 无读取路径，手动设置的 CDN 参数回显为空）
    cdn_address: cfgStr(cfg, 'cdn_address') || undefined,
    cdn_port: Number(cfg.cdn_port) || undefined,
    cdn_path: cfgStr(cfg, 'cdn_path') || undefined,
    // 节点暴露方式：区分 CF CDN (cdn_saas) 和 CF Tunnel (argo_tunnel) 和直连(direct)
    // 后端 standardizeNodeFields 会自动设置默认值，前端读取用于编辑回显
    exposure_mode: cfgStr(cfg, 'exposure_mode') || undefined,
    // 上下行分离模式：split mode 开关 + 下行暴露方式
    is_split_mode: !!(cfg.is_split_mode ?? node.is_split_mode),
    downstream_exposure_mode: cfgStr(cfg, 'downstream_exposure_mode') || node.downstream_exposure_mode || undefined,
    advanced: {
      // 修复：cert_file/key_file 回显（之前硬编码空值，TLS file 模式不可用）
      tls: { cert_mode: cfgStr(cfg, 'cert_file') ? 'file' : 'none', cert_file: cfgStr(cfg, 'cert_file') || '', key_file: cfgStr(cfg, 'key_file') || '', cert_pem: cfgStr(cfg, 'cert_pem') || '', key_pem: cfgStr(cfg, 'key_pem') || '', acme_domains: '', acme_email: '', server_name: cfgStr(cfg, 'server_name') || cfg.tls_settings?.server_name || '' },
      mux: (() => {
        // XHTTP XMUX：从 xhttp.extra.xmux 读取
        const xmux = (cfg as any)?.xhttp?.extra?.xmux
        if (xmux && typeof xmux === 'object') {
          return {
            enabled: true,
            protocol: 'xmux' as const,
            max_connections: Number(xmux.maxConnection) || Number(xmux.max_connections) || 8,
            max_streams: 32,
            padding: false,
            keep_alive_period: 30,
            max_concurrency: String(xmux.maxConcurrency ?? ''),
            c_max_reuse_times: String(xmux.cMaxReuseTimes ?? ''),
            h_max_request_times: String(xmux.hMaxRequestTimes ?? ''),
            h_max_reusable_secs: String(xmux.hMaxReusableSecs ?? ''),
          }
        }
        // 标准 multiplex
        return {
          enabled: muxEnabled,
          protocol: (cfg.multiplex?.protocol as NodeSpec['advanced']['mux']['protocol']) || 'yamux',
          max_connections: Number(cfg.multiplex?.max_connections) || 8,
          max_streams: 32,
          padding: !!cfg.multiplex?.padding,
          keep_alive_period: 30,
          max_concurrency: '',
          c_max_reuse_times: '',
          h_max_request_times: '',
          h_max_reusable_secs: '',
        }
      })(),
      tcp_brutal: {
        enabled: brutalEnabled,
        // 修复：优先从 multiplex.brutal 读取，回退到顶层 up_mbps/down_mbps（Hysteria2/TUIC 协议）
        up_mbps: Number(cfg.multiplex?.brutal?.up_mbps) || Number(cfg.up_mbps) || 50,
        down_mbps: Number(cfg.multiplex?.brutal?.down_mbps) || Number(cfg.down_mbps) || 100,
      },
      // 修复：从 config_json.port_hopping 读取（之前硬编码默认值导致面板设置无效）
      // 字段用于 Hysteria2/TUIC 等 UDP 协议的客户端 URI mport 参数渲染 + 服务端持久化
      port_hopping: {
        enabled: !!(cfg as any).port_hopping?.enabled,
        port_range: (cfg as any).port_hopping?.port_range || '',
        interval: Number((cfg as any).port_hopping?.interval) || 0,
      },
      ech: { enabled: false, config: '', priority: 'auto', enable_dhps: false },
      custom_outbounds: Array.isArray(cfg.custom_outbounds) ? JSON.stringify(cfg.custom_outbounds, null, 2) : '',
      custom_routes: Array.isArray(cfg.custom_routes) ? JSON.stringify(cfg.custom_routes, null, 2) : '',
    },
  }
}

function specToNodePayload(spec: NodeSpec, isEdit: boolean): Record<string, unknown> {
  // flow 字段规则：
  // - 仅 VLESS + REALITY 才使用 flow=xtls-rprx-vision
  // - Trojan/VMess/SS 等协议不支持 flow，必须清空
  // - XHTTP 传输不使用 flow（即使 VLESS+REALITY+XHTTP 也不用）
  const isFlowSupported = spec.protocol === 'vless' && spec.security === 'reality' && spec.transport !== 'xhttp'
  const effectiveFlow = isFlowSupported ? (spec.flow || undefined) : ''

  // 传输相关字段规则：TCP 传输不需要 path/host/service_name
  const isTcp = spec.transport === 'tcp'
  // uuid 仅 VLESS/VMess/TUIC 需要；Trojan/SS/Hysteria2 用 password
  const needsUuid = ['vless', 'vmess', 'tuic'].includes(spec.protocol)

  const configData: NodeConfigData = {
    network: spec.transport as NodeConfigData['network'],
    flow: effectiveFlow,
    uuid: needsUuid ? spec.uuid : undefined,
    password: spec.password,
    method: spec.method,
    username: spec.username,
    // TCP 传输清空 path/host/service_name（避免残留字段污染配置）
    path: isTcp ? undefined : spec.path,
    host: isTcp ? undefined : spec.host,
    service_name: isTcp ? undefined : spec.service_name,
    sni: spec.sni,
    alpn: spec.alpn ? spec.alpn.split(',').map(s => s.trim()).filter(Boolean) : undefined,
    utls_fingerprint: spec.utls_fingerprint,
    allow_insecure: spec.allow_insecure,
    client_port: spec.client_port || spec.port,
    server_port: spec.server_port || spec.port,
    preset_id: spec.preset_id || undefined,
    // xhttp_mode 仅 XHTTP 传输才输出（TCP/WS/gRPC 不需要）
    xhttp_mode: spec.transport === 'xhttp' ? spec.xhttp_mode : undefined,
    region: spec.region || undefined,
    parent_node_id: spec.parent_node_id || undefined,
    priority: spec.priority || 0,
    reality_utls_enabled: spec.reality_utls_enabled,
    reality_utls_fingerprint: spec.reality_utls_fingerprint,
    // P1: fingerprint 顶层写入（后端 pickString 第一优先级），统一 TLS/REALITY 指纹路径
    fingerprint: spec.security === 'reality'
      ? (spec.reality_utls_fingerprint || spec.utls_fingerprint || undefined)
      : (spec.utls_fingerprint || undefined),
  }

  const tls: 0 | 1 | 2 = spec.security === 'reality' ? 2 : spec.security === 'tls' ? 1 : 0
  configData.tls = tls
  configData.security = spec.security
  configData.security_type = spec.security
  // P-Chain: 链式套娃出站 URI 透传（后端 validateChainOutboundURI 三重校验 + ParseChainURI 解析）
  configData.chain_outbound_uri = spec.chain_outbound_uri || undefined

  // P2-2: 移除 network_settings 双写（后端 buildTransportConfig 从 model.Node.Path/HostHeader 读取，
  // 不读 config_json.network_settings；顶层 path/host/service_name 已写入，nodeToSpec 保留回退兼容历史数据）

  if (tls >= 1) {
    const ts: NodeTlsSettings = { allow_insecure: spec.allow_insecure ? 1 : 0 }
    if (spec.sni) { ts.server_name = spec.sni; ts.sni = spec.sni }
    if (spec.alpn) {
      ts.alpn = spec.alpn.split(',').map(s => s.trim()).filter(Boolean)
    }
    // P0 修复：TLS 证书路径写入 config_json + tls_settings（对齐后端 pickStringNested 读取路径）
    // 后端 xray_config.go 从 config_json.cert_file / tls_settings.cert_file 三级回退读取
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
    const rs: NodeRealitySettings = {}
    // 修复：reality_dest 必须原样保留到顶层和 reality_settings（normalizer 会拍平）
    // 不再拆分为 server_name + server_port（server_name 应为 SNI 域名，不是 dest IP）
    if (spec.reality_dest) {
      configData.reality_dest = spec.reality_dest
      rs.reality_dest = spec.reality_dest
    }
    // server_name 来自 SNI（realitySettings.serverNames），不是 dest
    if (spec.sni) rs.server_name = spec.sni
    if (spec.public_key) rs.public_key = spec.public_key
    if (spec.private_key) rs.private_key = spec.private_key
    if (spec.short_id) rs.short_id = spec.short_id
    if (spec.spider_x) rs.spider_x = spec.spider_x
    // P2-1: 移除 rs.allow_insecure = 0（REALITY 无此字段，写入 reality_settings 属于无效字段污染）
    configData.reality_settings = rs
  }

  const muxEnabled = spec.advanced?.mux?.enabled
  const brutalEnabled = spec.advanced?.tcp_brutal?.enabled
  if (muxEnabled || brutalEnabled) {
    const mx: NodeMultiplex = {}
    if (muxEnabled) {
      mx.enabled = 1
      mx.protocol = spec.advanced.mux.protocol || 'yamux'
      mx.max_connections = spec.advanced.mux.max_connections || 8
      mx.padding = spec.advanced.mux.padding ? 1 : 0
    }
    if (brutalEnabled) {
      mx.brutal = {
        enabled: 1,
        up_mbps: spec.advanced.tcp_brutal.up_mbps || 50,
        down_mbps: spec.advanced.tcp_brutal.down_mbps || 100,
      }
    }
    configData.multiplex = mx
    // 修复：tcp_brutal 启用时同步写顶层 up_mbps/down_mbps
    // 原因：normalizer 白名单包含 up_mbps/down_mbps 顶层键，但 multiplex 对象内的 brutal.up_mbps 不会被拍平
    // 同时 Hysteria2/TUIC 协议也读顶层 up_mbps/down_mbps（不在 multiplex 内）
    if (brutalEnabled) {
      ;(configData as any).up_mbps = spec.advanced.tcp_brutal.up_mbps || 50
      ;(configData as any).down_mbps = spec.advanced.tcp_brutal.down_mbps || 100
    }
  }

  // XHTTP XMUX 保存：写入 xhttp.extra.xmux（对应 Xray xhttpSettings.extra.xmux）
  if (spec.advanced?.mux?.enabled && spec.advanced.mux.protocol === 'xmux') {
    const xmux: Record<string, unknown> = {}
    if (spec.advanced.mux.max_concurrency) xmux.maxConcurrency = spec.advanced.mux.max_concurrency
    if (spec.advanced.mux.max_connections > 0) xmux.maxConnection = spec.advanced.mux.max_connections
    if (spec.advanced.mux.c_max_reuse_times) xmux.cMaxReuseTimes = spec.advanced.mux.c_max_reuse_times
    if (spec.advanced.mux.h_max_request_times) xmux.hMaxRequestTimes = spec.advanced.mux.h_max_request_times
    if (spec.advanced.mux.h_max_reusable_secs) xmux.hMaxReusableSecs = spec.advanced.mux.h_max_reusable_secs
    if (Object.keys(xmux).length > 0) {
      ;(configData as any).xhttp = {
        ...((configData as any).xhttp || {}),
        extra: { xmux },
      }
    }
  }

  // XHTTP mode 保存：写入 xhttp.mode（如果 transport 是 xhttp）
  if (spec.transport === 'xhttp' && spec.xhttp_mode) {
    ;(configData as any).xhttp = {
      ...((configData as any).xhttp || {}),
      mode: spec.xhttp_mode,
    }
  }

  // XHTTP path/host 保存：写入 xhttp.path/xhttp.host
  if (spec.transport === 'xhttp') {
    const xhttpExisting = (configData as any).xhttp || {}
    if (spec.path) xhttpExisting.path = spec.path
    if (spec.host) xhttpExisting.host = spec.host
    if (Object.keys(xhttpExisting).length > 0) {
      ;(configData as any).xhttp = xhttpExisting
    }
  }

  // downloadSettings 保存：写入 xhttp.extra.downloadSettings（Xray 标准嵌套结构）
  // 下行参数必须包裹在 xhttpSettings/tlsSettings/realitySettings 中（标准 StreamSettings 递归）
  if (spec.download_settings?.enabled && spec.transport === 'xhttp') {
    const xhttpExisting = (configData as any).xhttp || {}
    const extraExisting = xhttpExisting.extra || {}
    const ds: Record<string, unknown> = {
      address: spec.download_settings.address,
      port: spec.download_settings.port || 443,
      network: spec.download_settings.network || 'xhttp',
      security: spec.download_settings.security || 'tls',
    }
    // ✅ 正确：xhttp 传输参数包裹在 xhttpSettings 中
    const xhttpSettings: Record<string, unknown> = {
      mode: spec.download_settings.mode || (spec.download_settings.security === 'reality' ? 'stream-up' : 'packet-up'),
    }
    if (spec.download_settings.host) xhttpSettings.host = spec.download_settings.host
    if (spec.download_settings.path) xhttpSettings.path = spec.download_settings.path
    if (spec.download_settings.no_grpc_header) xhttpSettings.noGRPCHeader = true
    ds.xhttpSettings = xhttpSettings
    // ✅ 正确：REALITY 参数包裹在 realitySettings 中（下行 REALITY 直连，独立密钥对）
    if (spec.download_settings.security === 'reality') {
      const realitySettings: Record<string, unknown> = {}
      if (spec.download_settings.public_key) realitySettings.publicKey = spec.download_settings.public_key
      if (spec.download_settings.private_key) realitySettings.privateKey = spec.download_settings.private_key
      if (spec.download_settings.short_id) realitySettings.shortId = spec.download_settings.short_id
      // 修复：serverName 是 SNI 域名，dest 是回落目标（IP:Port），两个字段独立存储
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
    }
    // ✅ 正确：TLS 参数包裹在 tlsSettings 中（下行 TLS CDN）
    if (spec.download_settings.security === 'tls') {
      const tlsSettings: Record<string, unknown> = {}
      const tlsSni = spec.download_settings.sni || spec.download_settings.server_name || spec.sni
      if (tlsSni) tlsSettings.serverName = tlsSni
      if (spec.download_settings.fingerprint) tlsSettings.fingerprint = spec.download_settings.fingerprint
      if (spec.download_settings.alpn) {
        tlsSettings.alpn = spec.download_settings.alpn.split(',').map((s: string) => s.trim()).filter(Boolean)
      }
      if (spec.download_settings.allow_insecure) tlsSettings.allowInsecure = true
      if (Object.keys(tlsSettings).length > 0) ds.tlsSettings = tlsSettings
    }
    extraExisting.downloadSettings = ds
    xhttpExisting.extra = extraExisting
    ;(configData as any).xhttp = xhttpExisting
  }

  // P2-1: 移除 reality 嵌套对象双写（与 reality_settings 重复）
  // REALITY 字段统一写入 reality_settings（xboard 标准路径），后端 pickStringNested 三级回退兼容读取
  // fingerprint 已由 P1-1 写入顶层 configData.fingerprint，alpn 已在顶层 configData.alpn

  if (spec.advanced?.custom_outbounds) {
    try { configData.custom_outbounds = JSON.parse(spec.advanced.custom_outbounds) } catch {}
  }
  if (spec.advanced?.custom_routes) {
    try { configData.custom_routes = JSON.parse(spec.advanced.custom_routes) } catch {}
  }

  // P0-3: 如果 raw_config_json 存在（用户通过 Raw JSON tab 手动编辑），用它覆盖表单生成的 configData
  // 顶层 DB 字段（name/port/sni 等）仍从表单获取，确保数据库列正确更新
  let finalConfigJson: Record<string, unknown> = configData
  if (spec.raw_config_json && typeof spec.raw_config_json === 'object' && Object.keys(spec.raw_config_json).length > 0) {
    finalConfigJson = { ...configData, ...spec.raw_config_json }
  }

  // P1-3: KCP/QUIC 字段（从 spec 或 raw_config_json 读取，后端 buildTransportConfig 需要这些字段）
  if (spec.transport === 'kcp' && (spec as any).seed) {
    finalConfigJson.seed = (spec as any).seed
  }
  if (spec.transport === 'quic') {
    if ((spec as any).quic_security) finalConfigJson.quic_security = (spec as any).quic_security
    if ((spec as any).quic_key) finalConfigJson.quic_key = (spec as any).quic_key
  }

  // P1-4: CDN 端口/路径映射（后端 buildNginxVhosts 读取 cdn_address/cdn_port/cdn_path）
  // XHTTP+TLS 且启用下行分离（上行CDN+下行直连）场景：自动设置 cdn_address=host（CDN域名）
  if (spec.transport === 'xhttp' && spec.security === 'tls' && spec.download_settings?.enabled && spec.host) {
    finalConfigJson.cdn_address = spec.host
  } else if ((spec as any).cdn_address) {
    finalConfigJson.cdn_address = (spec as any).cdn_address
  }
  if ((spec as any).cdn_port) finalConfigJson.cdn_port = (spec as any).cdn_port
  if ((spec as any).cdn_path) finalConfigJson.cdn_path = (spec as any).cdn_path
  if ((spec as any).cdn_up_path) finalConfigJson.cdn_up_path = (spec as any).cdn_up_path
  if ((spec as any).cdn_down_path) finalConfigJson.cdn_down_path = (spec as any).cdn_down_path
  // 节点暴露方式：前端明确选择时发送给后端（后端 standardizeNodeFields 会保留用户设置的值）
  if ((spec as any).exposure_mode) finalConfigJson.exposure_mode = (spec as any).exposure_mode
  // 上下行分离（split mode）字段：顶层字段 + config_json 双写
  if ((spec as any).is_split_mode) {
    finalConfigJson.is_split_mode = true
    if ((spec as any).downstream_exposure_mode) {
      finalConfigJson.downstream_exposure_mode = (spec as any).downstream_exposure_mode
    }
  } else {
    finalConfigJson.is_split_mode = false
  }

  // 端口跳跃（port_hopping）：Hysteria2/TUIC 等 UDP 协议支持
  // 写入 config_json.port_hopping 用于：
  //   1) 服务端持久化（面板编辑回显）
  //   2) 客户端 URI 的 mport 参数渲染（见 NodeConfigEditor.tsx 的 hysteria2 URI 构造）
  // 显式写入 false 确保关闭操作被持久化（避免旧值残留）
  if (spec.advanced?.port_hopping?.enabled) {
    finalConfigJson.port_hopping = {
      enabled: true,
      port_range: spec.advanced.port_hopping.port_range || '',
      interval: Number(spec.advanced.port_hopping.interval) || 0,
    }
  } else {
    finalConfigJson.port_hopping = { enabled: false, port_range: '', interval: 0 }
  }

  const payload: Record<string, unknown> = {
    name: spec.name,
    protocol_type: spec.protocol,
    transport_type: spec.transport,
    address: spec.address,
    // 服务端监听端口同步：server_port 优先（xray inbound 真实监听端口）
    // 设计原因：后端 xray_config.go:257 与 deployment_service.go:793 均读 node.Port，
    // 面板编辑器只暴露 server_port 字段，故需让 server_port 同步到顶层 port 字段，
    // 否则面板填写的"服务端监听端口"不生效，破坏零 SSH 闭环。
    port: (typeof spec.server_port === 'number' && spec.server_port > 0) ? spec.server_port : spec.port,
    // 修复：同时发送 server_port 顶层字段，让后端 UpdateNode 更新 node.ServerPort DB 列
    // 否则 standardizeNodeFields 会用旧 node.ServerPort 覆盖 config_json.server_port，导致编辑回显为旧值
    server_port: (typeof spec.server_port === 'number' && spec.server_port > 0) ? spec.server_port : undefined,
    // 修复：编辑模式不强制启用，新建模式默认启用
    // 后端 UpdateNodeRequest.IsEnabled 为 *bool 指针类型，编辑模式不发送则保留原值
    is_enabled: isEdit ? undefined : true,
    is_visible: spec.is_visible,
    // 修复：priority 发送顶层，让后端 nodes.priority DB 列正确更新（影响节点排序）
    priority: spec.priority ?? 0,
    traffic_rate: spec.multiplier || 1.0,
    // P2-J: 节点流量限额相关字段发送到后端
    speed_limit_mbps: Number(spec.speed_limit_mbps) || 0,
    device_limit: Number(spec.device_limit) || 0,
    transfer_enable_bytes: Number(spec.transfer_enable_bytes) || 0,
    padding_scheme: spec.padding_scheme || undefined,
    tags: Array.isArray(spec.tags) ? spec.tags : [],
    config_json: finalConfigJson,
    // 同步顶层 DB 字段（后端 UpdateNode 会独立更新这些列）
    sni: spec.sni || '',
    alpn: spec.alpn ? spec.alpn.split(',').map(s => s.trim()).filter(Boolean) : [],
    // gRPC 节点的 ServiceName 存储在 node.Path DB 列（后端 buildTransportConfig 从 n.Path 读取），
    // 必须把 service_name 映射到 payload.path，否则后端渲染时 ServiceName 为空导致 Validate 失败。
    // TCP 传输（Trojan/VLESS+TCP 等）不需要 path，发送空字符串避免后端 path 唯一性校验误报冲突
    // WS/XHTTP 节点：spec.path 为空时发送 undefined（不更新独立列），避免编辑保存时清空已有 path
    path: spec.transport === 'grpc' ? (spec.service_name || spec.path || '') : (isTcp ? '' : (spec.path || undefined)),
    // host_header 为空时发送 undefined（不更新独立列），避免编辑保存时清空已有 host_header
    host_header: spec.host || undefined,
    flow: effectiveFlow || '',
    security_type: spec.security || 'none',
    // 上下行分离（split mode）：顶层 DB 字段
    is_split_mode: (spec as any).is_split_mode === true,
    downstream_exposure_mode: ((spec as any).is_split_mode && (spec as any).downstream_exposure_mode) ? (spec as any).downstream_exposure_mode : undefined,
  }

  // P0-3 修复：Raw JSON 中编辑的 security_type/security 同步到 payload 顶层 DB 列
  // 否则后端 UpdateNode 从 UpdateNodeRequest.SecurityType（顶层）读取，Raw JSON 编辑不生效
  if (spec.raw_config_json && typeof spec.raw_config_json === 'object') {
    const rawCfg = spec.raw_config_json as Record<string, unknown>
    if (typeof rawCfg.security_type === 'string' && rawCfg.security_type) {
      payload.security_type = rawCfg.security_type
    } else if (typeof rawCfg.security === 'string' && rawCfg.security) {
      payload.security_type = rawCfg.security
    }
  }
  // 移除 undefined 字段，避免 JSON 序列化发送 "is_enabled": null
  if (payload.is_enabled === undefined) {
    delete payload.is_enabled
  }

  // runtime_id: 后端 CreateNodeRequest binding:"required"，从绑定的服务器获取
  // 必须是有效的 UUID v4，否则后端 google/uuid 包会返回 "invalid UUID length: N"
  const rid = spec.server_bindings?.[0]?.runtime_id
  if (rid) {
    if (!UUID_V4_RE.test(rid)) {
      throw new Error(`所选服务器未注册有效的 runtime（收到 ${rid.length} 字符，期望 36 字符 UUID）。请先在该服务器上安装 node-agent，或选择其他服务器。`)
    }
    payload.runtime_id = rid
  }

  if (spec.code) payload.code = spec.code
  // 多对多分组：发送 group_ids 数组，后端整体覆盖关联表
  // 始终发送（包括空数组），后端会整体覆盖；空数组表示清空所有分组关联
  // 过滤非 UUID 值（如 'default' 占位符），避免后端 []uuid.UUID 解析失败
  const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i
  payload.group_ids = (Array.isArray(spec.permission_groups) ? spec.permission_groups : [])
    .filter((id: unknown): id is string => typeof id === 'string' && UUID_RE.test(id))

  // route_groups 对应后端的 chain_ids（代理链 UUID 列表）
  // 始终发送（包括空数组），后端会整体覆盖绑定关系
  payload.chain_ids = (Array.isArray(spec.route_groups) ? spec.route_groups : [])
    .filter((id: unknown): id is string => typeof id === 'string' && UUID_RE.test(id))

  // P1-1: 发送绑定的套餐 ID 列表
  payload.plan_ids = Array.isArray(spec.plan_ids) ? spec.plan_ids : []

  // P1-3: 发送证书包 ID（仅当有值时发送）
  if (spec.cert_bundle_id) {
    payload.cert_bundle_id = spec.cert_bundle_id
  }

  return payload
}

function generateNodeUri(node: Node): string {
  try {
    const cfg = parseConfig(node.config)
    const address = node.address
    const port = Number(node.port) || 443
    const cfgSec = cfgStr(cfg, 'security') || cfgStr(cfg, 'security_type')
    const tls = cfg.tls ?? (cfgSec === 'reality' ? 2 : cfgSec === 'tls' ? 1 : 0)
    const net = (cfg.network as string) || node.transport_type || 'tcp'
    const name = encodeURIComponent(node.name)
    const params = new URLSearchParams()
    params.set('type', net)
    params.set('security', tls === 2 ? 'reality' : tls === 1 ? 'tls' : 'none')

    const sni = cfg.tls_settings?.sni || cfgStr(cfg, 'sni')
    const pbk = cfg.reality_settings?.public_key || cfgStr(cfg, 'public_key')
    const sid = cfg.reality_settings?.short_id || cfgStr(cfg, 'short_id')
    const flow = cfgStr(cfg, 'flow')
    const wsPath = cfg.network_settings?.path || cfgStr(cfg, 'path')
    const grpcSvc = cfg.network_settings?.serviceName || cfgStr(cfg, 'service_name')
    const uuid = cfgStr(cfg, 'uuid') || 'uuid'
    const password = cfgStr(cfg, 'password') || 'password'

    if (tls >= 1 && sni) params.set('sni', sni)
    if (tls === 2 && pbk) params.set('pbk', pbk)
    if (tls === 2 && sid) params.set('sid', sid)
    if (flow) params.set('flow', flow)
    if (net === 'ws' && wsPath) params.set('path', wsPath)
    if (net === 'grpc' && grpcSvc) params.set('serviceName', grpcSvc)

    const type = node.protocol_type.toLowerCase()
    if (type === 'vless') return `vless://${uuid}@${address}:${port}?${params.toString()}#${name}`
    if (type === 'trojan') return `trojan://${password}@${address}:${port}?${params.toString()}#${name}`
    if (type === 'ss' || type === 'shadowsocks') return `ss://${address}:${port}#${name}`
    return `${type}://${address}:${port}#${name}`
  } catch { return '' }
}

function isNodeOnline(node: Node): boolean {
  const s = node.health_status
  return s === 'healthy' || s === 'degraded'
}

function isNodePending(node: Node): boolean {
  const s = node.health_status
  return s === 'unknown' || s === undefined || s === ''
}

export default function Nodes() {
  const { toast } = useToast()
  const [loading, setLoading] = useState(true)
  const [nodes, setNodes] = useState<Node[]>([])
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())

  const [search, setSearch] = useState('')
  const [protocolFilter, setProtocolFilter] = useState('all')
  const [statusFilter, setStatusFilter] = useState('all')
  const [sortBy, setSortBy] = useState('sort')

  const [editorOpen, setEditorOpen] = useState(false)
  const [editorMode, setEditorMode] = useState<'create' | 'edit'>('create')
  const [editingSpec, setEditingSpec] = useState<Partial<NodeSpec> | undefined>(undefined)
  const [copiedId, setCopiedId] = useState<string | null>(null)
  const [deployingId, setDeployingId] = useState<string | null>(null)
  const [searchParams, setSearchParams] = useSearchParams()

  // 路由策略绑定对话框状态
  const [routeDialogNode, setRouteDialogNode] = useState<Node | null>(null)
  const [routeBindings, setRouteBindings] = useState<RouteBinding[]>([])
  const [routePolicies, setRoutePolicies] = useState<RoutePolicyOption[]>([])
  const [selectedPolicyId, setSelectedPolicyId] = useState('')
  const [routeLoading, setRouteLoading] = useState(false)
  const [routeBinding, setRouteBinding] = useState(false)
  const [routeConfigPreview, setRouteConfigPreview] = useState<string | null>(null)
  const [routeConfigLoading, setRouteConfigLoading] = useState(false)

  // 支持 URL 参数 ?action=create 自动打开创建对话框（从服务器详情页跳转）
  useEffect(() => {
    const action = searchParams.get('action')
    if (action === 'create') {
      setEditingSpec(undefined)
      setEditorMode('create')
      setEditorOpen(true)
      // 清除 URL 参数，避免刷新时重复打开
      searchParams.delete('action')
      setSearchParams(searchParams, { replace: true })
    }
  }, [searchParams, setSearchParams])

  const loadNodes = async () => {
    setLoading(true)
    try {
      const data = await api.get<{ items: Node[]; total: number; page: number; page_size: number }>(EP.NODES, {
        params: { page: 1, page_size: 200 },
      })
      const items = data.items || []
      // P2-I: 获取在线连接数 — 通过通道健康列表 (server_id -> online_users) 聚合
      // 后端 NodeResponse 暂未返回 online_count，使用 CHANNEL_HEALTH 列表按 server_info.id 映射
      try {
        const healthResp = await api.get<{
          items: Array<{ server_id: string; online_users: number }>
          total: number
        }>(EP.CHANNEL_HEALTH, { params: { page: 1, page_size: 500 } })
        const onlineMap = new Map<string, number>()
        for (const it of healthResp.items || []) {
          onlineMap.set(it.server_id, it.online_users || 0)
        }
        items.forEach(n => {
          const sid = n.server_info?.id
          if (sid && onlineMap.has(sid)) {
            n.online_count = onlineMap.get(sid)
          }
        })
      } catch {
        // 通道健康接口不可用时静默降级，在线连接数显示占位 "-"
      }
      setNodes(items)
    } catch (err) {
      toast({
        title: '加载失败',
        description: err instanceof Error ? err.message : '无法获取节点列表',
        variant: 'destructive',
      })
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadNodes()
  }, [])

  const stats = useMemo(() => {
    const total = nodes.length
    const online = nodes.filter(n => n.is_visible && isNodeOnline(n)).length
    const pending = nodes.filter(n => n.is_visible && isNodePending(n)).length
    const totalTraffic = 0
    return { total, online, pending, offline: total - online - pending, totalTraffic }
  }, [nodes])

  const filteredNodes = useMemo(() => {
    let result = nodes.filter(n => {
      const kw = search.trim().toLowerCase()
      const matchSearch = !kw ||
        n.name.toLowerCase().includes(kw) ||
        n.id.toLowerCase().includes(kw) ||
        (n.address || '').toLowerCase().includes(kw) ||
        (n.code || '').toLowerCase().includes(kw)

      const nodeType = n.protocol_type.toLowerCase()
      let matchProto = protocolFilter === 'all'
      if (protocolFilter === 'ss') matchProto = nodeType === 'shadowsocks' || nodeType === 'ss'
      else if (protocolFilter !== 'all') matchProto = nodeType === protocolFilter

      const isVisible = !!n.is_visible
      const isOnline = isNodeOnline(n)
      const isPending = isNodePending(n)
      const isEnabled = !!n.is_enabled
      let matchStatus = statusFilter === 'all'
      if (statusFilter === 'online') matchStatus = isVisible && isOnline
      else if (statusFilter === 'pending') matchStatus = isVisible && isPending
      else if (statusFilter === 'offline') matchStatus = isVisible && !isOnline && !isPending
      else if (statusFilter === 'hidden') matchStatus = !isVisible
      else if (statusFilter === 'disabled') matchStatus = !isEnabled

      return matchSearch && matchProto && matchStatus
    })

    result = [...result].sort((a, b) => {
      switch (sortBy) {
        case 'name': return a.name.localeCompare(b.name)
        case 'traffic': {
          return 0
        }
        case 'online': {
          const oa = isNodeOnline(a) ? 1 : 0
          const ob = isNodeOnline(b) ? 1 : 0
          return ob - oa
        }
        default: return a.name.localeCompare(b.name)
      }
    })

    return result
  }, [nodes, search, protocolFilter, statusFilter, sortBy])

  const allSelected = filteredNodes.length > 0 && filteredNodes.every(n => selectedIds.has(n.id))

  const toggleSelectAll = useCallback(() => {
    if (allSelected) {
      setSelectedIds(new Set())
    } else {
      setSelectedIds(new Set(filteredNodes.map(n => n.id)))
    }
  }, [allSelected, filteredNodes])

  const toggleSelect = useCallback((id: string) => {
    setSelectedIds(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }, [])

  const openCreate = useCallback(() => {
    setEditingSpec(undefined)
    setEditorMode('create')
    setEditorOpen(true)
  }, [])

  const openEdit = useCallback(async (node: Node) => {
    try {
      const fullNode = await api.get<Node>(EP.NODE_DETAIL(node.id))
      const spec = nodeToSpec(fullNode)
      setEditingSpec(spec)
    } catch {
      const spec = nodeToSpec(node)
      setEditingSpec(spec)
    }
    setEditorMode('edit')
    setEditorOpen(true)
  }, [])

  const handleSaveNode = useCallback(async (spec: NodeSpec): Promise<boolean> => {
    try {
      const isEdit = editorMode === 'edit'
      const payload = specToNodePayload(spec, isEdit)
      let response: any
      if (isEdit && spec.id) {
        response = await api.patch(EP.NODE_DETAIL(spec.id), payload)
      } else {
        response = await api.post(EP.NODES, payload)
      }

      // 检查并显示 warnings 警告信息
      const warnings: string[] | undefined = response?.warnings
      if (warnings && warnings.length > 0) {
        const warningText = warnings.join('；')
        toast({
          title: isEdit ? '节点已更新，但存在警告' : '节点已创建，但存在警告',
          description: `⚠️ ${warningText}。节点已保存，agent重连后将自动拉取配置`,
          variant: 'destructive',
          duration: 8000,
        })
      } else {
        toast({ title: isEdit ? '节点已更新' : '节点已创建', variant: 'success' })
      }

      // 注意：不在此处关闭对话框。由 NodeConfigEditor 在 onSave 返回 true 后统一调用 onOpenChange(false)。
      // 这样可确保保存失败时（返回 false）对话框保持打开，保留用户填写的表单数据。
      setSelectedIds(new Set())
      await loadNodes()
      return true
    } catch (err) {
      // 专门处理 409 路径冲突错误
      if (err instanceof ApiError && err.status === 409) {
        const msg = err.message || ''
        toast({
          title: '路径冲突',
          description: msg.includes('path') ? msg : `路径已被其他节点使用：${msg}`,
          variant: 'destructive',
        })
      } else {
        toast({
          title: '保存失败',
          description: err instanceof Error ? err.message : '请稍后重试',
          variant: 'destructive',
        })
      }
      return false
    }
  }, [editorMode, toast, loadNodes])

  const handleDelete = useCallback(async (node: Node) => {
    if (!window.confirm(`确定删除节点「${node.name}」吗？此操作不可撤销。`)) return
    try {
      await api.delete(EP.NODE_DETAIL(node.id))
      toast({ title: '节点已删除', variant: 'success' })
      await loadNodes()
    } catch (err) {
      toast({
        title: '删除失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    }
  }, [toast, loadNodes])

  const handleToggleVisibility = useCallback(async (node: Node) => {
    try {
      await api.patch(EP.NODE_DETAIL(node.id), { is_visible: !node.is_visible })
      await loadNodes()
    } catch (err) {
      toast({
        title: '操作失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    }
  }, [toast, loadNodes])

  const handleCopyUri = useCallback((node: Node) => {
    const uri = generateNodeUri(node)
    navigator.clipboard.writeText(uri).then(() => {
      setCopiedId(node.id)
      toast({ title: '节点链接已复制', variant: 'success' })
      setTimeout(() => setCopiedId(null), 2000)
    }).catch(() => {
      toast({ title: '复制失败', variant: 'destructive' })
    })
  }, [toast])

  const handleCopyAddress = useCallback((node: Node) => {
    const addr = `${node.address}:${node.port}`
    navigator.clipboard.writeText(addr).then(() => {
      setCopiedId(node.id)
      toast({ title: '地址已复制', variant: 'success' })
      setTimeout(() => setCopiedId(null), 2000)
    })
  }, [toast])

  // 发布配置：调用 POST /admin/deployments/refresh，按节点 scope 刷新部署配置
  const handleRefreshConfig = useCallback(async (node: Node) => {
    setDeployingId(node.id)
    try {
      await api.post(EP.DEPLOYMENT_REFRESH, { scope_type: 'node', scope_id: node.id })
      toast({ title: '发布配置成功', description: `节点「${node.name}」配置已发布`, variant: 'success' })
    } catch (err) {
      toast({
        title: '发布配置失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setDeployingId(null)
    }
  }, [toast])

  const handleBulkDelete = useCallback(async () => {
    if (selectedIds.size === 0) return
    if (!window.confirm(`确定删除选中的 ${selectedIds.size} 个节点吗？此操作不可撤销。`)) return
    let success = 0
    for (const id of selectedIds) {
      try {
        await api.delete(EP.NODE_DETAIL(id))
        success++
      } catch {}
    }
    toast({ title: `已删除 ${success}/${selectedIds.size} 个节点`, variant: success === selectedIds.size ? 'success' : 'default' })
    setSelectedIds(new Set())
    await loadNodes()
  }, [selectedIds, toast, loadNodes])

  const handleBulkToggle = useCallback(async (visible: boolean) => {
    if (selectedIds.size === 0) return
    let success = 0
    for (const id of selectedIds) {
      try {
        await api.patch(EP.NODE_DETAIL(id), { is_visible: visible })
        success++
      } catch {}
    }
    toast({ title: `已${visible ? '显示' : '隐藏'} ${success}/${selectedIds.size} 个节点`, variant: 'success' })
    await loadNodes()
  }, [selectedIds, toast, loadNodes])

  const handleImportUri = useCallback(() => {
    toast({ title: 'URI导入功能准备中', description: '该功能即将上线，敬请期待', variant: 'default' })
  }, [toast])

  // ===== 路由策略绑定 =====
  // 打开节点路由策略对话框，并行加载已绑定策略与可选策略列表
  const openRouteDialog = async (node: Node) => {
    setRouteDialogNode(node)
    setSelectedPolicyId('')
    setRouteConfigPreview(null)
    setRouteBindings([])
    setRoutePolicies([])
    setRouteLoading(true)
    try {
      const [bindResp, policyResp] = await Promise.all([
        api.get<{ items: RouteBinding[]; total: number }>(EP.NODE_ROUTE_BINDINGS(node.id)).catch(() => ({ items: [] as RouteBinding[], total: 0 })),
        api.get<{ items: RoutePolicyOption[]; total: number }>(EP.ROUTE_POLICIES, {
          params: { page: 1, page_size: 200 },
        }).catch(() => ({ items: [] as RoutePolicyOption[], total: 0 })),
      ])
      setRouteBindings(bindResp.items || [])
      setRoutePolicies(policyResp.items || [])
    } catch (err) {
      toast({
        title: '加载路由数据失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setRouteLoading(false)
    }
  }

  const reloadRouteBindings = async (nodeId: string) => {
    try {
      const resp = await api.get<{ items: RouteBinding[]; total: number }>(EP.NODE_ROUTE_BINDINGS(nodeId))
      setRouteBindings(resp.items || [])
    } catch {
      // 静默失败，已绑定列表保持原样
    }
  }

  const handleRouteBind = async () => {
    if (!routeDialogNode || !selectedPolicyId) return
    setRouteBinding(true)
    try {
      await api.post(EP.NODE_ROUTE_BINDINGS(routeDialogNode.id), { policy_id: selectedPolicyId })
      toast({ title: '策略已绑定', variant: 'success' })
      setSelectedPolicyId('')
      await reloadRouteBindings(routeDialogNode.id)
    } catch (err) {
      toast({
        title: '绑定失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setRouteBinding(false)
    }
  }

  const handleRouteUnbind = async (policyId: string) => {
    if (!routeDialogNode) return
    if (!window.confirm('确定解绑该路由策略吗？')) return
    try {
      await api.delete(EP.NODE_ROUTE_BINDING(routeDialogNode.id, policyId))
      toast({ title: '策略已解绑', variant: 'success' })
      await reloadRouteBindings(routeDialogNode.id)
    } catch (err) {
      toast({
        title: '解绑失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    }
  }

  // 预览节点路由配置渲染结果（GET /admin/nodes/:id/routing-config）
  const handleRouteConfigPreview = async () => {
    if (!routeDialogNode) return
    setRouteConfigLoading(true)
    setRouteConfigPreview(null)
    try {
      const result = await api.get<unknown>(EP.NODE_ROUTING_CONFIG(routeDialogNode.id))
      setRouteConfigPreview(JSON.stringify(result, null, 2))
    } catch (err) {
      toast({
        title: '渲染失败',
        description: err instanceof Error ? err.message : '请稍后重试',
        variant: 'destructive',
      })
    } finally {
      setRouteConfigLoading(false)
    }
  }

  // 根据 policy_id 查找策略名称，回退到截断的 ID
  const policyName = (policyId: string) => {
    const p = routePolicies.find((x) => x.id === policyId)
    return p ? `${p.name} (${p.code})` : policyId.substring(0, 8)
  }

  return (
    <div className="space-y-5 pb-20 sm:pb-4">
      <div className="flex flex-col sm:flex-row sm:items-start justify-between gap-3">
        <div>
          <h2 className="text-xl font-semibold text-zinc-100 flex items-center gap-2">
            <Server className="w-6 h-6 text-indigo-400" />节点管理
          </h2>
          <p className="text-sm text-zinc-400 mt-1">管理和配置代理节点</p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={handleImportUri} className="border-zinc-700 text-zinc-300 hover:bg-zinc-800">
            <Upload className="w-4 h-4 mr-1.5" />导入URI
          </Button>
          <Button size="sm" onClick={openCreate} className="bg-indigo-600 hover:bg-indigo-500">
            <Plus className="w-4 h-4 mr-1.5" />添加节点
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
        <Card className="bg-zinc-900 border-zinc-800">
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs text-zinc-500">节点总数</p>
                <p className="text-2xl font-bold text-zinc-100 mt-1">{stats.total}</p>
              </div>
              <div className="w-10 h-10 rounded-lg bg-zinc-800 flex items-center justify-center">
                <Layers className="w-5 h-5 text-zinc-400" />
              </div>
            </div>
          </CardContent>
        </Card>
        <Card className="bg-zinc-900 border-zinc-800">
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs text-zinc-500">在线节点</p>
                <p className="text-2xl font-bold text-emerald-400 mt-1">{stats.online}</p>
              </div>
              <div className="w-10 h-10 rounded-lg bg-emerald-500/10 flex items-center justify-center">
                <Wifi className="w-5 h-5 text-emerald-400" />
              </div>
            </div>
          </CardContent>
        </Card>
        <Card className="bg-zinc-900 border-zinc-800">
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs text-zinc-500">待激活</p>
                <p className="text-2xl font-bold text-amber-400 mt-1">{stats.pending}</p>
              </div>
              <div className="w-10 h-10 rounded-lg bg-amber-500/10 flex items-center justify-center">
                <Activity className="w-5 h-5 text-amber-400" />
              </div>
            </div>
          </CardContent>
        </Card>
        <Card className="bg-zinc-900 border-zinc-800">
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs text-zinc-500">离线节点</p>
                <p className="text-2xl font-bold text-red-400 mt-1">{stats.offline}</p>
              </div>
              <div className="w-10 h-10 rounded-lg bg-red-500/10 flex items-center justify-center">
                <WifiOff className="w-5 h-5 text-red-400" />
              </div>
            </div>
          </CardContent>
        </Card>
        <Card className="bg-zinc-900 border-zinc-800">
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs text-zinc-500">今日总流量</p>
                <p className="text-2xl font-bold text-zinc-100 mt-1">{formatBytes(stats.totalTraffic)}</p>
              </div>
              <div className="w-10 h-10 rounded-lg bg-blue-500/10 flex items-center justify-center">
                <Activity className="w-5 h-5 text-blue-400" />
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      <Card className="bg-zinc-900 border-zinc-800">
        <CardContent className="p-3">
          <div className="flex flex-col lg:flex-row gap-2">
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-zinc-500" />
              <Input
                placeholder="搜索节点名称、ID或地址..."
                value={search}
                onChange={e => setSearch(e.target.value)}
                className="pl-9 bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500 h-9"
              />
            </div>
            <div className="flex gap-2 flex-wrap">
              <select
                value={protocolFilter}
                onChange={e => setProtocolFilter(e.target.value)}
                className="h-9 rounded-lg border border-zinc-700 bg-zinc-800 px-3 text-sm text-zinc-100 focus:outline-none focus:border-indigo-500"
              >
                {PROTOCOL_FILTERS.map(f => (
                  <option key={f.value} value={f.value} className="bg-zinc-800">{f.label}</option>
                ))}
              </select>
              <select
                value={statusFilter}
                onChange={e => setStatusFilter(e.target.value)}
                className="h-9 rounded-lg border border-zinc-700 bg-zinc-800 px-3 text-sm text-zinc-100 focus:outline-none focus:border-indigo-500"
              >
                {STATUS_FILTERS.map(f => (
                  <option key={f.value} value={f.value} className="bg-zinc-800">{f.label}</option>
                ))}
              </select>
              <select
                value={sortBy}
                onChange={e => setSortBy(e.target.value)}
                className="h-9 rounded-lg border border-zinc-700 bg-zinc-800 px-3 text-sm text-zinc-100 focus:outline-none focus:border-indigo-500"
              >
                {SORT_OPTIONS.map(f => (
                  <option key={f.value} value={f.value} className="bg-zinc-800">{f.label}</option>
                ))}
              </select>
              <Button variant="ghost" size="sm" onClick={loadNodes} className="text-zinc-400 hover:text-zinc-200 h-9">
                <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      {selectedIds.size > 0 && (
        <Card className="bg-indigo-950/30 border-indigo-800/50">
          <CardContent className="p-3">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <Checkbox checked={allSelected} onChange={toggleSelectAll} />
                <span className="text-sm text-indigo-300">已选择 {selectedIds.size} 个节点</span>
              </div>
              <div className="flex items-center gap-2">
                <Button variant="ghost" size="sm" onClick={() => handleBulkToggle(true)} className="text-emerald-400 hover:text-emerald-300 h-8">
                  <Eye className="w-4 h-4 mr-1" />批量显示
                </Button>
                <Button variant="ghost" size="sm" onClick={() => handleBulkToggle(false)} className="text-zinc-400 hover:text-zinc-200 h-8">
                  <EyeOff className="w-4 h-4 mr-1" />批量隐藏
                </Button>
                <Button variant="ghost" size="sm" onClick={handleBulkDelete} className="text-red-400 hover:text-red-300 h-8">
                  <Trash2 className="w-4 h-4 mr-1" />批量删除
                </Button>
                <Button variant="ghost" size="sm" onClick={() => setSelectedIds(new Set())} className="text-zinc-500 h-8">
                  <X className="w-4 h-4" />
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {loading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {Array.from({ length: 6 }).map((_, i) => (
            <Card key={i} className="bg-zinc-900 border-zinc-800">
              <CardContent className="p-4 space-y-3">
                <div className="flex items-center justify-between">
                  <Skeleton className="h-5 w-40 bg-zinc-800" />
                  <Skeleton className="h-5 w-16 bg-zinc-800" />
                </div>
                <Skeleton className="h-4 w-full bg-zinc-800" />
                <Skeleton className="h-4 w-3/4 bg-zinc-800" />
                <div className="flex gap-2">
                  <Skeleton className="h-6 w-16 bg-zinc-800" />
                  <Skeleton className="h-6 w-20 bg-zinc-800" />
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      ) : filteredNodes.length === 0 ? (
        <Card className="bg-zinc-900 border-zinc-800">
          <CardContent className="py-16 text-center">
            <Server className="w-12 h-12 text-zinc-600 mx-auto mb-3" />
            <p className="text-zinc-400 text-sm">
              {nodes.length === 0 ? '暂无节点，点击右上角添加节点' : '没有匹配的节点，请调整筛选条件'}
            </p>
            {nodes.length === 0 && (
              <Button size="sm" onClick={openCreate} className="mt-4 bg-indigo-600 hover:bg-indigo-500">
                <Plus className="w-4 h-4 mr-1.5" />添加第一个节点
              </Button>
            )}
          </CardContent>
        </Card>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {filteredNodes.map(node => {
            const cfg = parseConfig(node.config)
            const isVisible = !!node.is_visible
            const isOnline = isNodeOnline(node)
            const isPending = isNodePending(node)
            const isEnabled = !!node.is_enabled
            const statusColor = !isVisible ? 'bg-zinc-500' : !isEnabled ? 'bg-zinc-500' : isOnline ? 'bg-emerald-500' : isPending ? 'bg-amber-500' : 'bg-red-500'
            const protoKey = node.protocol_type.toLowerCase()
            const badgeColor = PROTOCOL_COLORS[protoKey] || 'bg-zinc-800 text-zinc-400 border-zinc-700'
            const dotColor = PROTOCOL_DOT_COLORS[protoKey] || 'bg-zinc-500'
            const protoLabel = protoKey === 'ss' ? 'SS' : node.protocol_type.toUpperCase()
            const transport = node.transport_type || (cfg.network as string) || ''
            const cfgSec = cfgStr(cfg, 'security') || cfgStr(cfg, 'security_type')
            const tlsLevel = cfg.tls ?? (cfgSec === 'reality' ? 2 : cfgSec === 'tls' ? 1 : 0)
            const rateVal = Number(node.traffic_rate) || 1
            const isSelected = selectedIds.has(node.id)
            const dispatchCfg = node.dispatch_status ? DISPATCH_STATUS_CONFIG[node.dispatch_status] : null
            const dispatchTooltip = dispatchCfg ? [
              `配置状态: ${dispatchCfg.label}`,
              node.dispatch_version ? `版本: v${node.dispatch_version}` : '',
              node.dispatch_time ? `时间: ${formatDispatchTime(node.dispatch_time)}` : '',
              node.dispatch_error ? `错误: ${node.dispatch_error}` : '',
            ].filter(Boolean).join('\n') : ''

            return (
              <Card
                key={node.id}
                className={`bg-zinc-900 border-zinc-800 hover:border-zinc-700 transition-colors ${isSelected ? 'ring-2 ring-indigo-500/50 border-indigo-700' : ''}`}
              >
                <CardContent className="p-4">
                  <div className="flex items-start justify-between gap-2 mb-3">
                    <div className="flex items-start gap-2.5 min-w-0">
                      <Checkbox
                        checked={isSelected}
                        onChange={() => toggleSelect(node.id)}
                        className="mt-0.5"
                      />
                      <div className="min-w-0">
                        <div className="flex items-center gap-2 flex-wrap">
                          <div className={`w-2.5 h-2.5 rounded-full ${statusColor} ${isOnline && isVisible && isEnabled ? 'animate-pulse' : ''}`} />
                          <span className={`font-medium text-sm truncate ${isVisible ? 'text-zinc-100' : 'text-zinc-500'}`}>
                            {node.name}
                          </span>
                        </div>
                        <div className="flex items-center gap-1.5 mt-1.5 flex-wrap">
                          <Badge variant="outline" className={`text-[10px] px-1.5 py-0 border ${badgeColor}`}>
                            <span className={`w-1.5 h-1.5 rounded-full ${dotColor} mr-1`} />{protoLabel}
                          </Badge>
                          {transport && transport !== 'tcp' && (
                            <Badge variant="outline" className="text-[10px] px-1.5 py-0 bg-zinc-800/50 text-zinc-400 border-zinc-700">
                              {transport.toUpperCase()}
                            </Badge>
                          )}
                          {tlsLevel === 2 && (
                            <Badge variant="outline" className="text-[10px] px-1.5 py-0 bg-indigo-950/30 text-indigo-400 border-indigo-800/50">
                              <Shield className="w-2.5 h-2.5 mr-0.5" />REALITY
                            </Badge>
                          )}
                          {tlsLevel === 1 && (
                            <Badge variant="outline" className="text-[10px] px-1.5 py-0 bg-blue-950/30 text-blue-400 border-blue-800/50">
                              <Shield className="w-2.5 h-2.5 mr-0.5" />TLS
                            </Badge>
                          )}
                          <Badge variant="outline" className="text-[10px] px-1.5 py-0 bg-zinc-800/50 text-zinc-500 border-zinc-700 font-mono">
                            #{node.id.substring(0, 8)}
                          </Badge>
                          {rateVal !== 1 && (
                            <Badge variant="outline" className="text-[10px] px-1.5 py-0 bg-amber-950/30 text-amber-400 border-amber-800/50">
                              <Zap className="w-2.5 h-2.5 mr-0.5" />{rateVal.toFixed(1)}x
                            </Badge>
                          )}
                          {node.plan_codes && node.plan_codes.length > 0 && (
                            node.plan_codes.map(code => (
                              <Badge key={code} variant="outline" className="text-[10px] px-1.5 py-0 bg-violet-950/30 text-violet-400 border-violet-800/50">
                                {code}
                              </Badge>
                            ))
                          )}
                          {dispatchCfg && (
                            <Badge
                              variant="outline"
                              className={`text-[10px] px-1.5 py-0 border ${dispatchCfg.badgeClass}`}
                              title={dispatchTooltip}
                            >
                              <span className={`w-1.5 h-1.5 rounded-full ${dispatchCfg.dotClass} mr-1 ${node.dispatch_status === 'pending' ? 'animate-pulse' : ''}`} />
                              {dispatchCfg.label}
                              {node.dispatch_version && <span className="ml-1 opacity-70">v{node.dispatch_version}</span>}
                            </Badge>
                          )}
                        </div>
                      </div>
                    </div>
                    <Switch
                      checked={isVisible}
                      onChange={() => handleToggleVisibility(node)}
                      className="flex-shrink-0"
                    />
                  </div>

                  <div className="space-y-2.5 mb-3">
                    <div className="flex items-center gap-2 group cursor-pointer" onClick={() => handleCopyAddress(node)}>
                      <Globe className="w-3.5 h-3.5 text-zinc-500 flex-shrink-0" />
                      <span className="text-xs font-mono text-zinc-400 truncate flex-1">
                        {node.address}:{node.port}
                      </span>
                      {copiedId === node.id ? (
                        <Check className="w-3.5 h-3.5 text-emerald-400 flex-shrink-0" />
                      ) : (
                        <Copy className="w-3.5 h-3.5 text-zinc-600 group-hover:text-zinc-400 flex-shrink-0 opacity-0 group-hover:opacity-100 transition-opacity" />
                      )}
                    </div>

                    <div className="flex items-center gap-3 text-[11px] text-zinc-500 flex-wrap">
                      {node.tags && parseTags(node.tags).length > 0 && (
                        <span className="flex items-center gap-1">
                          <HardDrive className="w-3 h-3" />
                          <span className="truncate">{parseTags(node.tags).slice(0, 2).join(', ')}</span>
                        </span>
                      )}
                      {/* P2-I: 在线连接数 */}
                      <span className="flex items-center gap-1" title="在线连接数">
                        <Users className="w-3 h-3" />
                        {typeof node.online_count === 'number' ? `${node.online_count}人在线` : '-'}
                      </span>
                      {/* P2-J: 速度限制（0 显示"不限"） */}
                      <span className="flex items-center gap-1" title="速度限制">
                        <Gauge className="w-3 h-3" />
                        {Number(node.speed_limit_mbps) > 0 ? `${node.speed_limit_mbps}Mbps` : '不限'}
                      </span>
                      {/* P2-J: 设备数限制（0 显示"不限"） */}
                      <span className="flex items-center gap-1" title="设备数限制">
                        <Smartphone className="w-3 h-3" />
                        {Number(node.device_limit) > 0 ? `${node.device_limit}台` : '不限'}
                      </span>
                      {node.health_status && (
                        <span className="flex items-center gap-1">
                          <Activity className="w-3 h-3" />{node.health_status}
                        </span>
                      )}
                    </div>
                  </div>

                  <Separator className="bg-zinc-800 my-2" />

                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-1">
                      <div className={`flex items-center gap-1 text-[11px] px-2 py-0.5 rounded ${isOnline && isVisible && isEnabled ? 'bg-emerald-500/10 text-emerald-400' : isPending && isVisible && isEnabled ? 'bg-amber-500/10 text-amber-400' : !isVisible ? 'bg-zinc-800 text-zinc-500' : !isEnabled ? 'bg-zinc-800 text-zinc-500' : 'bg-red-500/10 text-red-400'}`}>
                        {isOnline && isVisible && isEnabled ? <Wifi className="w-3 h-3" /> : isPending && isVisible && isEnabled ? <Activity className="w-3 h-3" /> : !isVisible ? <EyeOff className="w-3 h-3" /> : !isEnabled ? <Shield className="w-3 h-3" /> : <WifiOff className="w-3 h-3" />}
                        {isOnline && isVisible && isEnabled ? '在线' : isPending && isVisible && isEnabled ? '待激活' : !isVisible ? '已隐藏' : !isEnabled ? '已禁用' : '离线'}
                      </div>
                    </div>
                    <div className="flex items-center gap-0.5">
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-7 px-2 text-xs text-zinc-400 hover:text-emerald-400"
                        onClick={() => handleRefreshConfig(node)}
                        disabled={deployingId === node.id}
                        title="发布配置"
                      >
                        {deployingId === node.id
                          ? <Loader2 className="w-3 h-3 mr-1 animate-spin" />
                          : <Send className="w-3 h-3 mr-1" />}
                        发布配置
                      </Button>
                      <Button variant="ghost" size="icon" className="h-7 w-7 text-zinc-500 hover:text-indigo-400" onClick={() => openEdit(node)}>
                        <Edit className="w-3.5 h-3.5" />
                      </Button>
                      <Button variant="ghost" size="icon" className="h-7 w-7 text-zinc-500 hover:text-cyan-400" onClick={() => openRouteDialog(node)} title="路由策略">
                        <Route className="w-3.5 h-3.5" />
                      </Button>
                      <Button variant="ghost" size="icon" className="h-7 w-7 text-zinc-500 hover:text-blue-400" onClick={() => handleCopyUri(node)}>
                        <Link2 className="w-3.5 h-3.5" />
                      </Button>
                      <Button variant="ghost" size="icon" className="h-7 w-7 text-zinc-500 hover:text-red-400" onClick={() => handleDelete(node)}>
                        <Trash2 className="w-3.5 h-3.5" />
                      </Button>
                    </div>
                  </div>
                </CardContent>
              </Card>
            )
          })}
        </div>
      )}

      <NodeConfigEditor
        open={editorOpen}
        onOpenChange={setEditorOpen}
        mode={editorMode}
        initialSpec={editingSpec}
        onSave={handleSaveNode}
      />

      {/* 路由策略绑定对话框 */}
      <Dialog open={!!routeDialogNode} onOpenChange={(o) => { if (!o) setRouteDialogNode(null) }}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-2xl">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Route className="w-5 h-5 text-cyan-400" />
              <span>路由策略绑定 · {routeDialogNode?.name}</span>
            </DialogTitle>
          </DialogHeader>

          <div className="space-y-4 pt-2 max-h-[70vh] overflow-y-auto pr-1">
            {/* 绑定新策略 */}
            <div className="flex items-end gap-2">
              <div className="flex-1 space-y-1.5">
                <Label className="text-zinc-300 text-sm">选择路由策略</Label>
                <Select
                  value={selectedPolicyId}
                  onChange={(e) => setSelectedPolicyId(e.target.value)}
                  className="bg-zinc-800 border-zinc-700 text-zinc-100 h-9"
                >
                  <option value="">选择策略...</option>
                  {routePolicies.map((p) => (
                    <option key={p.id} value={p.id}>{p.name} ({p.code}){p.status ? ` · ${p.status}` : ''}</option>
                  ))}
                </Select>
              </div>
              <Button
                className="bg-indigo-600 hover:bg-indigo-500 h-9"
                onClick={handleRouteBind}
                disabled={!selectedPolicyId || routeBinding}
                isLoading={routeBinding}
              >
                绑定
              </Button>
            </div>

            {/* 已绑定策略列表 */}
            <div className="space-y-2">
              <div className="text-sm text-zinc-400">已绑定策略</div>
              {routeLoading ? (
                <div className="space-y-2">
                  {[1, 2].map((i) => (
                    <Skeleton key={i} className="h-12 w-full bg-zinc-800 rounded-lg" />
                  ))}
                </div>
              ) : routeBindings.length === 0 ? (
                <div className="rounded-lg border border-dashed border-zinc-800 px-4 py-6 text-center">
                  <Route className="w-6 h-6 text-zinc-600 mx-auto mb-2" />
                  <p className="text-sm text-zinc-500">该节点尚未绑定路由策略</p>
                </div>
              ) : (
                <div className="space-y-2">
                  {routeBindings.map((b) => (
                    <div
                      key={b.policy_id}
                      className="flex items-center justify-between gap-2 rounded-lg border border-zinc-800 bg-zinc-950/40 px-3 py-2"
                    >
                      <div className="min-w-0 flex-1">
                        <div className="text-sm font-medium text-zinc-200 truncate">{policyName(b.policy_id)}</div>
                        <div className="flex items-center gap-2 mt-0.5">
                          {b.bind_scope && (
                            <Badge variant="outline" className="text-[10px] px-1.5 py-0 bg-zinc-800 text-zinc-400 border-zinc-700">
                              scope: {b.bind_scope}
                            </Badge>
                          )}
                          {b.inbound_tag && (
                            <Badge variant="outline" className="text-[10px] px-1.5 py-0 bg-zinc-800 text-zinc-400 border-zinc-700">
                              inbound: {b.inbound_tag}
                            </Badge>
                          )}
                          <code className="text-[10px] text-zinc-600 font-mono">{b.policy_id.substring(0, 8)}</code>
                        </div>
                      </div>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-7 text-xs text-zinc-400 hover:text-red-400"
                        onClick={() => handleRouteUnbind(b.policy_id)}
                      >
                        解绑
                      </Button>
                    </div>
                  ))}
                </div>
              )}
            </div>

            {/* 路由配置预览 */}
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-sm text-zinc-400">路由配置预览</span>
                <Button
                  size="sm"
                  variant="outline"
                  className="border-zinc-700 text-zinc-300 h-8"
                  onClick={handleRouteConfigPreview}
                  disabled={routeConfigLoading || !routeDialogNode}
                  isLoading={routeConfigLoading}
                >
                  {!routeConfigLoading && <Eye className="w-3.5 h-3.5 mr-1" />}
                  预览渲染结果
                </Button>
              </div>
              {routeConfigPreview !== null && (
                <pre className="max-h-64 overflow-auto rounded-lg border border-zinc-800 bg-zinc-950 p-3 text-xs font-mono text-zinc-300 whitespace-pre-wrap break-all">
                  {routeConfigPreview || '(空)'}
                </pre>
              )}
            </div>
          </div>

          <DialogFooter className="pt-2">
            <Button
              variant="outline"
              onClick={() => setRouteDialogNode(null)}
              className="border-zinc-700 text-zinc-300"
            >
              关闭
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
