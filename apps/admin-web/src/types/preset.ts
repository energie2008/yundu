export type KernelCompatLevel = 'both' | 'xray_only' | 'singbox_only' | 'experimental'

export type PresetBadge = '主推' | '均衡' | 'CDN友好' | '实验性' | '已弃用' | '新协议'

export interface PresetTLSConfig {
  sni?: string
  alpn?: string[]
  fingerprint?: string
  cert_mode?: string
  allow_insecure?: boolean
}

export interface PresetRealityConfig {
  sni?: string
  fingerprint?: string
  short_id?: string
  spider_x?: string
  public_key?: string
  private_key?: string
  reality_dest?: string
}

export interface PresetWSConfig {
  path?: string
  host?: string
}

export interface PresetGRPCConfig {
  service_name?: string
}

export interface PresetXHTTPDownloadConfig {
  address?: string
  address_ipv6?: string
  port?: number
  network?: string
  security?: string
  path?: string
  host?: string
  mode?: string
  sni?: string
  server_port?: number
  no_grpc_header?: boolean
  reality?: PresetRealityConfig
  tls?: PresetTLSConfig
}

export interface PresetXHTTPConfig {
  path?: string
  host?: string
  mode?: string
  no_grpc_header?: boolean
  headers?: Record<string, string>
  download_settings?: PresetXHTTPDownloadConfig
}

export interface PresetMuxConfig {
  enabled?: boolean
  protocol?: string
  max_connections?: number
  max_streams?: number
  max_concurrency?: string
  c_max_reuse_times?: string
  h_max_request_times?: string
  h_max_reusable_secs?: string
  padding?: boolean
  keep_alive_period?: number
}

export interface PresetSockoptConfig {
  tcp_fast_open?: boolean
  tcp_multipath?: boolean
  congestion?: string
  tcp_keep_alive?: number
}

export interface PresetTCPBrutalConfig {
  enabled?: boolean
  up_mbps?: number
  down_mbps?: number
}

export interface PresetTransportConfig {
  type: string
  ws?: PresetWSConfig
  grpc?: PresetGRPCConfig
  xhttp?: PresetXHTTPConfig
  mux?: PresetMuxConfig
  sockopt?: PresetSockoptConfig
  tcp_brutal?: PresetTCPBrutalConfig
  headers?: Record<string, string>
}

export interface PresetCredentials {
  uuid?: string
  password?: string
  method?: string
  flow?: string
}

export interface PresetBaseSpec {
  protocol: string
  transport: PresetTransportConfig
  security: string
  port?: number
  client_port?: number
  server_port?: number
  address?: string
  allow_udp?: boolean
  traffic_rate?: number
  is_visible?: boolean
  tls?: PresetTLSConfig
  reality?: PresetRealityConfig
  credentials?: PresetCredentials
}

export interface PresetTemplate {
  id: string
  name: string
  badge?: PresetBadge
  description: string
  protocol: string
  transport: string
  security: string
  min_xray_version?: string
  min_singbox_version?: string
  client_support: string[]
  kernel_compat: KernelCompatLevel
  base_spec: PresetBaseSpec
  recommendations?: string[]
  warnings?: string[]
  deprecated_at?: string
  updated_from_upstream: string
}

export interface PresetDiffEntry {
  field: string
  preset_value: unknown
  current_value: unknown
  modified: boolean
}

export type PresetDiff = Record<string, PresetDiffEntry>

export const COMPAT_META: Record<KernelCompatLevel, {
  color: string
  bgColor: string
  borderColor: string
  label: string
  desc: string
}> = {
  both: {
    color: '#00D9C0',
    bgColor: 'bg-emerald-950/30',
    borderColor: 'border-emerald-800/50',
    label: '双内核兼容',
    desc: 'Xray & Sing-box 均支持',
  },
  xray_only: {
    color: '#4A9EFF',
    bgColor: 'bg-blue-950/30',
    borderColor: 'border-blue-800/50',
    label: '仅 Xray',
    desc: '仅 Xray-core 支持',
  },
  singbox_only: {
    color: '#8B7FFF',
    bgColor: 'bg-violet-950/30',
    borderColor: 'border-violet-800/50',
    label: '仅 Sing-box',
    desc: '仅 Sing-box 支持',
  },
  experimental: {
    color: '#F5A623',
    bgColor: 'bg-amber-950/30',
    borderColor: 'border-amber-800/50',
    label: '实验性',
    desc: '部分客户端验证通过',
  },
}

export const BADGE_STYLE: Record<PresetBadge, { color: string; bg: string }> = {
  '主推': { color: 'text-emerald-400', bg: 'bg-emerald-950/40 border-emerald-800/50' },
  '均衡': { color: 'text-blue-400', bg: 'bg-blue-950/40 border-blue-800/50' },
  'CDN友好': { color: 'text-purple-400', bg: 'bg-purple-950/40 border-purple-800/50' },
  '实验性': { color: 'text-amber-400', bg: 'bg-amber-950/40 border-amber-800/50' },
  '已弃用': { color: 'text-red-400', bg: 'bg-red-950/40 border-red-800/50' },
  '新协议': { color: 'text-cyan-400', bg: 'bg-cyan-950/40 border-cyan-800/50' },
}
