import type { PresetTemplate, PresetBaseSpec, PresetDiff } from '@/types/preset'

export interface NodeSpecForm {
  numeric_id?: number
  id?: string
  code?: string
  name?: string
  protocol: string
  address?: string
  port?: number
  client_port?: number
  server_port?: number
  transport: string
  transport_type?: string
  security: string
  flow?: string
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
  server_bindings?: Array<{ id: string; name: string; sid: number; auto_manage: boolean }>
  route_groups?: string[]
  is_visible?: boolean
  priority?: number
  region?: string
  multiplier?: number
  traffic_limit?: number
  tags?: string[]
  permission_groups?: string[]
  uuid?: string
  password?: string
  method?: string
  username?: string
  xhttp_mode?: string
  preset_id?: string
  raw_settings?: string
  advanced?: Record<string, unknown>
  download_settings?: {
    enabled: boolean
    address?: string
    port?: number
    network?: string
    security?: string
    mode?: string
    host?: string
    path?: string
    sni?: string
    public_key?: string
    private_key?: string
    short_id?: string
    // server_name 是 REALITY 下行 SNI 域名（如 sub6.dannelblog.na.am）
    server_name?: string
    // dest 是 REALITY 下行回落目标（IP:Port，如 127.0.0.1:9454）
    dest?: string
    fingerprint?: string
    alpn?: string
    no_grpc_header?: boolean
    allow_insecure?: boolean
  }
  [key: string]: unknown
}

export function applyPresetToSpec(preset: PresetTemplate): Partial<NodeSpecForm> {
  const bs = preset.base_spec
  const result: Partial<NodeSpecForm> = {
    protocol: bs.protocol,
    transport: bs.transport.type,
    security: bs.security,
    preset_id: preset.id,
    multiplier: bs.traffic_rate || 1.0,
    is_visible: bs.is_visible !== false,
    allow_insecure: false,
  }
  if (bs.client_port) result.client_port = bs.client_port
  if (bs.server_port) result.server_port = bs.server_port
  if (bs.port) result.port = bs.port
  if (bs.address) result.address = bs.address
  if (bs.allow_udp !== undefined) {
  }
  if (bs.transport.ws) {
    if (bs.transport.ws.path !== undefined) result.path = bs.transport.ws.path
    if (bs.transport.ws.host !== undefined) result.host = bs.transport.ws.host
  }
  if (bs.transport.grpc?.service_name) {
    result.service_name = bs.transport.grpc.service_name
  }
  if (bs.transport.xhttp) {
    if (bs.transport.xhttp.path !== undefined) result.path = bs.transport.xhttp.path
    if (bs.transport.xhttp.host !== undefined) result.host = bs.transport.xhttp.host
    if (bs.transport.xhttp.mode) result.xhttp_mode = bs.transport.xhttp.mode
    if (bs.transport.xhttp.download_settings) {
      const ds = bs.transport.xhttp.download_settings
      const dlReality = ds.reality || {}
      const dlTls = ds.tls || {}
      const dlSec = (ds.security as string) || 'tls'
      const defaultMode = dlSec === 'reality' ? 'stream-up' : 'packet-up'
      result.download_settings = {
        enabled: true,
        address: ds.address || '',
        port: ds.port || 443,
        network: (ds.network as any) || 'xhttp',
        security: (dlSec as any),
        mode: ds.mode || defaultMode,
        host: ds.host || '',
        path: ds.path || '',
        sni: ds.sni || dlReality.sni || dlTls.sni || '',
        public_key: dlReality.public_key || '',
        private_key: dlReality.private_key || '',
        short_id: dlReality.short_id || '',
        // 修复：server_name 是 SNI 域名（不是 dest）
        server_name: dlReality.sni || ds.sni || '',
        // 修复：dest 单独存储（IP:Port 格式）
        dest: dlReality.reality_dest || '',
        fingerprint: dlReality.fingerprint || dlTls.fingerprint || 'chrome',
        alpn: Array.isArray(dlTls.alpn) ? dlTls.alpn.join(',') : (dlTls.alpn || ''),
        no_grpc_header: ds.no_grpc_header || bs.transport.xhttp.no_grpc_header || false,
        allow_insecure: dlTls.allow_insecure || false,
      }
    }
  }
  if (bs.tls) {
    if (bs.tls.sni !== undefined) result.sni = bs.tls.sni
    if (bs.tls.fingerprint) result.utls_fingerprint = bs.tls.fingerprint
    if (bs.tls.alpn) result.alpn = (bs.tls.alpn || []).join(',')
    if (bs.tls.cert_mode) {
      result.advanced = { ...(result.advanced || {}), tls: { cert_mode: bs.tls.cert_mode } }
    }
  }
  if (bs.reality) {
    if (bs.reality.sni) result.sni = bs.reality.sni
    if (bs.reality.fingerprint) {
      result.reality_utls_enabled = true
      result.reality_utls_fingerprint = bs.reality.fingerprint
      result.utls_fingerprint = bs.reality.fingerprint
    }
    if (bs.reality.reality_dest) result.reality_dest = bs.reality.reality_dest
    if (bs.reality.short_id !== undefined) result.short_id = bs.reality.short_id
    if (bs.reality.spider_x) result.spider_x = bs.reality.spider_x
    if (bs.reality.public_key) result.public_key = bs.reality.public_key
    if (bs.reality.private_key) result.private_key = bs.reality.private_key
  }
  if (bs.credentials) {
    if (bs.credentials.uuid) result.uuid = bs.credentials.uuid
    if (bs.credentials.password) result.password = bs.credentials.password
    if (bs.credentials.method) result.method = bs.credentials.method
    if (bs.credentials.flow !== undefined) result.flow = bs.credentials.flow
  }

  // === 根据协议/传输/安全类型清空不适用字段（防止 DEFAULT_SPEC 残留） ===
  const proto = bs.protocol
  const tp = bs.transport.type
  const sec = bs.security
  const needsFlow = proto === 'vless' && sec === 'reality' && tp !== 'xhttp'
  const needsUuid = ['vless', 'vmess', 'tuic'].includes(proto)
  if (!needsFlow) result.flow = ''
  if (!needsUuid) result.uuid = ''
  // TCP 传输清空 path/host/service_name
  if (tp === 'tcp') {
    result.path = ''
    result.host = ''
    result.service_name = ''
  }
  // 非 gRPC 清空 service_name
  if (tp !== 'grpc') {
    result.service_name = ''
  }
  // 非 XHTTP 清空 xhttp_mode
  if (tp !== 'xhttp') {
    result.xhttp_mode = ''
  }
  // 非 REALITY 清空 reality 相关字段
  if (sec !== 'reality') {
    result.reality_utls_enabled = false
    result.reality_utls_fingerprint = undefined
    result.reality_dest = ''
    result.short_id = ''
    result.public_key = ''
    result.private_key = ''
  }
  // security=none 清空 alpn
  if (sec === 'none') {
    result.alpn = ''
  }

  return result
}

export function diffFromPreset(current: Record<string, unknown>, preset: PresetTemplate): PresetDiff {
  const diff: PresetDiff = {}
  const bs = preset.base_spec
  const get = (k: string): unknown => current[k]
  const getStr = (k: string): string => (current[k] as string) || ''

  compareField(diff, 'protocol', bs.protocol, get('protocol'))
  compareField(diff, 'transport.type', bs.transport.type, get('transport'))
  compareField(diff, 'security', bs.security, get('security'))

  if (bs.client_port !== undefined) compareField(diff, 'client_port', bs.client_port, get('client_port'))
  if (bs.server_port !== undefined) compareField(diff, 'server_port', bs.server_port, get('server_port'))

  if (bs.transport.ws) {
    compareField(diff, 'transport.ws.path', bs.transport.ws.path, get('path'))
    compareField(diff, 'transport.ws.host', bs.transport.ws.host, getStr('host'))
  }
  if (bs.transport.grpc?.service_name) {
    compareField(diff, 'transport.grpc.service_name', bs.transport.grpc.service_name, get('service_name'))
  }
  if (bs.transport.xhttp) {
    compareField(diff, 'transport.xhttp.mode', bs.transport.xhttp.mode || 'auto', get('xhttp_mode') || 'auto')
  }

  if (bs.tls) {
    compareField(diff, 'tls.sni', bs.tls.sni || '', getStr('sni'))
    compareField(diff, 'tls.fingerprint', bs.tls.fingerprint || 'chrome', getStr('utls_fingerprint') || 'chrome')
    compareField(diff, 'tls.alpn', (bs.tls.alpn || ['h2', 'http/1.1']).join(','), getStr('alpn'))
  }

  if (bs.reality) {
    compareField(diff, 'reality.sni', bs.reality.sni || '', getStr('sni'))
    compareField(diff, 'reality.fingerprint', bs.reality.fingerprint || 'chrome', getStr('reality_utls_fingerprint') || 'chrome')
    compareField(diff, 'reality.short_id', bs.reality.short_id || '', getStr('short_id'))
  }

  return diff
}

function compareField(diff: PresetDiff, field: string, presetVal: unknown, currentVal: unknown) {
  const modified = !deepEqual(presetVal, currentVal)
  if (modified) {
    diff[field] = { field, preset_value: presetVal, current_value: currentVal, modified: true }
  }
}

function deepEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true
  if (a === undefined || b === undefined) return a === b
  if (a === null || b === null) return a === b
  if (typeof a !== typeof b) return false
  if (Array.isArray(a) && Array.isArray(b)) {
    if (a.length !== b.length) return false
    return a.every((v, i) => deepEqual(v, b[i]))
  }
  if (typeof a === 'object' && typeof b === 'object') {
    const aObj = a as Record<string, unknown>
    const bObj = b as Record<string, unknown>
    const aKeys = Object.keys(aObj)
    const bKeys = Object.keys(bObj)
    if (aKeys.length !== bKeys.length) return false
    return aKeys.every(k => deepEqual(aObj[k], bObj[k]))
  }
  return String(a) === String(b)
}

export function getModifiedFields(diff: PresetDiff): string[] {
  return Object.entries(diff).filter(([, v]) => v.modified).map(([k]) => k)
}

export function getPresetRecommendations(preset: PresetTemplate): string[] {
  return preset.recommendations || []
}

export function getPresetWarnings(preset: PresetTemplate): string[] {
  return preset.warnings || []
}
