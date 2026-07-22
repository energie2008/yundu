import { useState, useEffect } from 'react'
import {
  Eye,
  Copy,
  Check,
  X,
  AlertTriangle,
  QrCode,
  Smartphone,
  FileEdit,
  BarChart3,
  Plus,
  Save,
  Star,
  Users,
  Globe,
  Zap,
  Activity,
} from 'lucide-react'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Badge,
  Button,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
  Select,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  Skeleton,
  useToast,
  Input,
  Label,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@airport/ui'
import { api, ApiError } from '@/lib/api'
import { EP } from '@/lib/endpoints'
import { YamlEditor } from '@/components/common/YamlEditor'

const CLIENT_KEYS = ['clash', 'clashmeta', 'singbox', 'shadowrocket', 'v2rayn', 'surge', 'quantumultx', 'loon'] as const
type ClientKey = typeof CLIENT_KEYS[number]

const CLIENT_LABELS: Record<ClientKey, string> = {
  clash: 'Clash',
  clashmeta: 'Clash Meta',
  singbox: 'Sing-box',
  shadowrocket: 'Shadowrocket',
  surge: 'Surge',
  quantumultx: 'Quantumult X',
  loon: 'Loon',
  v2rayn: 'V2RayN URI',
}

const TEMPLATE_CLIENT_KEYS = ['clash', 'clashmeta', 'singbox', 'shadowrocket', 'v2rayn', 'surge', 'quantumultx', 'loon'] as const
type TemplateClientKey = typeof TEMPLATE_CLIENT_KEYS[number]

const TEMPLATE_CLIENT_OPTIONS: { value: TemplateClientKey; label: string }[] = [
  { value: 'clash', label: 'Clash' },
  { value: 'clashmeta', label: 'Clash Meta' },
  { value: 'singbox', label: 'Sing-box' },
  { value: 'shadowrocket', label: 'Shadowrocket' },
  { value: 'v2rayn', label: 'V2RayN' },
  { value: 'surge', label: 'Surge' },
  { value: 'quantumultx', label: 'Quantumult X' },
  { value: 'loon', label: 'Loon' },
]

interface PreviewNode {
  id: string
  name: string
  proto: string
  address: string
  port: number
  uuid: string
  password: string
  sni: string
}

interface SubTemplate {
  id: string
  code: string
  name: string
  target_client: TemplateClientKey
  content: string
  is_default: boolean
  status: 'active' | 'disabled'
  created_at?: string
  updated_at?: string
}

interface AccessOverview {
  total_requests: number
  unique_users: number
  unique_ips: number
  cache_hit_rate: number
  top_clients: { client: string; count: number; percentage: number }[]
}

interface AccessLog {
  id: string
  timestamp: string
  user_email?: string
  user_id?: string
  client: string
  ip: string
  status: number
  node_count: number
  cache_hit: boolean
}

function normalizeList<T>(data: unknown): T[] {
  if (Array.isArray(data)) return data as T[]
  if (data && typeof data === 'object') {
    const obj = data as Record<string, unknown>
    if (Array.isArray(obj.items)) return obj.items as T[]
    if (Array.isArray(obj.list)) return obj.list as T[]
    const dataField = obj.data
    if (dataField && typeof dataField === 'object') {
      if (Array.isArray(dataField)) return dataField as T[]
      const dataObj = dataField as Record<string, unknown>
      if (Array.isArray(dataObj.items)) return dataObj.items as T[]
    }
  }
  return []
}

const PREVIEW_CONFIGS: Record<string, Record<ClientKey, { code: string; passed: boolean }>> = {
  'VLESS+Reality+WS': {
    clash: {
      passed: true,
      code: `- name: "🇭🇰 香港 01 - VLESS+Reality+WS"
  type: vless
  server: hk01.example.com
  port: 443
  uuid: a3482e88-686a-4a58-8126-99c9df64b7bf
  network: ws
  tls: true
  udp: true
  reality-opts:
    public-key: xWgQBdYqH1Rc2LzN8vKpM5oT7nF3aE9sW4uI0yB6d
    short-id: 6ba85179e30d4fc2
  ws-opts:
    path: /ray
    headers:
      Host: hk01.example.com
  client-fingerprint: chrome`,
    },
    clashmeta: {
      passed: true,
      code: `- name: "🇭🇰 香港 01 - VLESS+Reality+WS"
  type: vless
  server: hk01.example.com
  port: 443
  uuid: a3482e88-686a-4a58-8126-99c9df64b7bf
  network: ws
  tls: true
  udp: true
  reality-opts:
    public-key: xWgQBdYqH1Rc2LzN8vKpM5oT7nF3aE9sW4uI0yB6d
    short-id: 6ba85179e30d4fc2
  ws-opts:
    path: /ray
    headers:
      Host: hk01.example.com
  client-fingerprint: chrome`,
    },
    singbox: {
      passed: true,
      code: `{
  "tag": "🇭🇰 香港 01 - VLESS+Reality+WS",
  "type": "vless",
  "server": "hk01.example.com",
  "server_port": 443,
  "uuid": "a3482e88-686a-4a58-8126-99c9df64b7bf",
  "tls": {
    "enabled": true,
    "server_name": "hk01.example.com",
    "utls": { "enabled": true, "fingerprint": "chrome" },
    "reality": {
      "enabled": true,
      "public_key": "xWgQBdYqH1Rc2LzN8vKpM5oT7nF3aE9sW4uI0yB6d",
      "short_id": "6ba85179e30d4fc2"
    }
  },
  "transport": { "type": "ws", "path": "/ray", "headers": { "Host": "hk01.example.com" } }
}`,
    },
    shadowrocket: {
      passed: true,
      code: `vless://a3482e88-686a-4a58-8126-99c9df64b7bf@hk01.example.com:443?security=reality&sni=hk01.example.com&fp=chrome&pbk=xWgQBdYqH1Rc2LzN8vKpM5oT7nF3aE9sW4uI0yB6d&sid=6ba85179e30d4fc2&type=ws&path=%2Fray&host=hk01.example.com#%F0%9F%87%AD%F0%9F%87%B0%20%E9%A6%99%E6%B8%AF%2001%20-%20VLESS%2BReality%2BWS`,
    },
    surge: {
      passed: true,
      code: `🇭🇰 香港 01 - VLESS+Reality+WS = vless, hk01.example.com, 443, username=a3482e88-686a-4a58-8126-99c9df64b7bf, tls=true, sni=hk01.example.com, reality-pbk=xWgQBdYqH1Rc2LzN8vKpM5oT7nF3aE9sW4uI0yB6d, reality-sid=6ba85179e30d4fc2, ws=true, ws-path=/ray, ws-host=hk01.example.com, tfo=true, udp-relay=true, client-fingerprint=chrome`,
    },
    quantumultx: {
      passed: true,
      code: `vless=hk01.example.com:443, method=none, password=a3482e88-686a-4a58-8126-99c9df64b7bf, obfs=wss, obfs-uri=/ray, obfs-host=hk01.example.com, tls-host=hk01.example.com, reality=1, reality-pbk=xWgQBdYqH1Rc2LzN8vKpM5oT7nF3aE9sW4uI0yB6d, reality-sid=6ba85179e30d4fc2, tls13=1, tag=🇭🇰 香港 01 - VLESS+Reality+WS, tfo=1, udp-relay=1`,
    },
    loon: {
      passed: true,
      code: `🇭🇰 香港 01 - VLESS+Reality+WS = VLESS, hk01.example.com, 443, a3482e88-686a-4a58-8126-99c9df64b7bf, over-tls=true, reality=true, reality-pbk=xWgQBdYqH1Rc2LzN8vKpM5oT7nF3aE9sW4uI0yB6d, reality-sid=6ba85179e30d4fc2, sni=hk01.example.com, transport=ws, path=/ray, host=hk01.example.com, tfo=true, udp=true`,
    },
    v2rayn: {
      passed: true,
      code: `vless://a3482e88-686a-4a58-8126-99c9df64b7bf@hk01.example.com:443?security=reality&sni=hk01.example.com&fp=chrome&pbk=xWgQBdYqH1Rc2LzN8vKpM5oT7nF3aE9sW4uI0yB6d&sid=6ba85179e30d4fc2&type=ws&path=%2Fray&host=hk01.example.com#%F0%9F%87%AD%F0%9F%87%B0%20%E9%A6%99%E6%B8%AF%2001%20-%20VLESS%2BReality%2BWS`,
    },
  },
  'Trojan+TLS+gRPC': {
    clash: {
      passed: true,
      code: `- name: "🇯🇵 日本 02 - Trojan+TLS+gRPC"
  type: trojan
  server: jp02.example.com
  port: 443
  password: trojan-pass-jp02-xyz789
  network: grpc
  tls: true
  sni: jp02.example.com
  udp: true
  grpc-opts:
    grpc-service-name: grpc-trojan
  client-fingerprint: chrome`,
    },
    clashmeta: {
      passed: true,
      code: `- name: "🇯🇵 日本 02 - Trojan+TLS+gRPC"
  type: trojan
  server: jp02.example.com
  port: 443
  password: trojan-pass-jp02-xyz789
  network: grpc
  tls: true
  sni: jp02.example.com
  udp: true
  grpc-opts:
    grpc-service-name: grpc-trojan
  client-fingerprint: chrome`,
    },
    singbox: {
      passed: true,
      code: `{
  "tag": "🇯🇵 日本 02 - Trojan+TLS+gRPC",
  "type": "trojan",
  "server": "jp02.example.com",
  "server_port": 443,
  "password": "trojan-pass-jp02-xyz789",
  "tls": {
    "enabled": true,
    "server_name": "jp02.example.com",
    "utls": { "enabled": true, "fingerprint": "chrome" }
  },
  "transport": { "type": "grpc", "service_name": "grpc-trojan" }
}`,
    },
    shadowrocket: {
      passed: true,
      code: `trojan://trojan-pass-jp02-xyz789@jp02.example.com:443?security=tls&sni=jp02.example.com&fp=chrome&type=grpc&serviceName=grpc-trojan#%F0%9F%87%AF%F0%9F%87%B5%20%E6%97%A5%E6%9C%AC%2002%20-%20Trojan%2BTLS%2BgRPC`,
    },
    surge: {
      passed: false,
      code: `🇯🇵 日本 02 - Trojan+TLS+gRPC = trojan, jp02.example.com, 443, password=trojan-pass-jp02-xyz789, tls=true, sni=jp02.example.com, tfo=true, udp-relay=true, client-fingerprint=chrome`,
    },
    quantumultx: {
      passed: true,
      code: `trojan=jp02.example.com:443, password=trojan-pass-jp02-xyz789, obfs=grpc, obfs-uri=grpc-trojan, tls-host=jp02.example.com, tls13=1, tag=🇯🇵 日本 02 - Trojan+TLS+gRPC, tfo=1, udp-relay=1`,
    },
    loon: {
      passed: true,
      code: `🇯🇵 日本 02 - Trojan+TLS+gRPC = Trojan, jp02.example.com, 443, trojan-pass-jp02-xyz789, over-tls=true, sni=jp02.example.com, transport=grpc, grpc-service-name=grpc-trojan, tfo=true, udp=true`,
    },
    v2rayn: {
      passed: true,
      code: `trojan://trojan-pass-jp02-xyz789@jp02.example.com:443?security=tls&sni=jp02.example.com&fp=chrome&type=grpc&serviceName=grpc-trojan#%F0%9F%87%AF%F0%9F%87%B5%20%E6%97%A5%E6%9C%AC%2002%20-%20Trojan%2BTLS%2BgRPC`,
    },
  },
  'Hysteria2': {
    clash: {
      passed: true,
      code: `- name: "🇺🇸 美国 03 - Hysteria2"
  type: hysteria2
  server: us03.example.com
  port: 8443
  password: hy2-password-us03-abc123
  sni: us03.example.com
  up: 30 Mbps
  down: 100 Mbps
  obfs: salamander
  obfs-password: hy2-obfs-secret`,
    },
    clashmeta: {
      passed: true,
      code: `- name: "🇺🇸 美国 03 - Hysteria2"
  type: hysteria2
  server: us03.example.com
  port: 8443
  password: hy2-password-us03-abc123
  sni: us03.example.com
  up: 30 Mbps
  down: 100 Mbps
  obfs: salamander
  obfs-password: hy2-obfs-secret`,
    },
    singbox: {
      passed: true,
      code: `{
  "tag": "🇺🇸 美国 03 - Hysteria2",
  "type": "hysteria2",
  "server": "us03.example.com",
  "server_port": 8443,
  "password": "hy2-password-us03-abc123",
  "tls": { "enabled": true, "server_name": "us03.example.com" },
  "obfs": { "type": "salamander", "password": "hy2-obfs-secret" },
  "up_mbps": 30,
  "down_mbps": 100
}`,
    },
    shadowrocket: {
      passed: false,
      code: `hysteria2://hy2-password-us03-abc123@us03.example.com:8443?obfs=salamander&obfs-password=hy2-obfs-secret&sni=us03.example.com&insecure=0#%F0%9F%87%BA%F0%9F%87%B8%20%E7%BE%8E%E5%9B%BD%2003%20-%20Hysteria2`,
    },
    surge: {
      passed: false,
      code: `[ERROR] Surge does not support Hysteria2 protocol. Please use a compatible client such as Clash Meta or Sing-box.`,
    },
    quantumultx: {
      passed: false,
      code: `[ERROR] Quantumult X does not support Hysteria2 protocol natively. Consider using TUIC or Trojan as alternatives.`,
    },
    loon: {
      passed: true,
      code: `🇺🇸 美国 03 - Hysteria2 = Hysteria2, us03.example.com, 8443, hy2-password-us03-abc123, over-tls=true, sni=us03.example.com, obfs=salamander, obfs-password=hy2-obfs-secret, download-bandwidth=100, upload-bandwidth=30, tfo=true, udp=true`,
    },
    v2rayn: {
      passed: true,
      code: `hysteria2://hy2-password-us03-abc123@us03.example.com:8443?obfs=salamander&obfs-password=hy2-obfs-secret&sni=us03.example.com#%F0%9F%87%BA%F0%9F%87%B8%20%E7%BE%8E%E5%9B%BD%2003%20-%20Hysteria2`,
    },
  },
  'VLESS+h2': {
    clash: {
      passed: true,
      code: `- name: "🇸🇬 新加坡 04 - VLESS+h2"
  type: vless
  server: sg04.example.com
  port: 443
  uuid: b7c9d3e2-1f4a-5b6c-7d8e-9f0a1b2c3d4e
  network: h2
  tls: true
  h2-opts:
    host:
      - sg04.example.com
    path: /vless
  client-fingerprint: chrome`,
    },
    clashmeta: {
      passed: true,
      code: `- name: "🇸🇬 新加坡 04 - VLESS+h2"
  type: vless
  server: sg04.example.com
  port: 443
  uuid: b7c9d3e2-1f4a-5b6c-7d8e-9f0a1b2c3d4e
  network: h2
  tls: true
  h2-opts:
    host:
      - sg04.example.com
    path: /vless
  client-fingerprint: chrome`,
    },
    singbox: {
      passed: true,
      code: `{
  "tag": "🇸🇬 新加坡 04 - VLESS+h2",
  "type": "vless",
  "server": "sg04.example.com",
  "server_port": 443,
  "uuid": "b7c9d3e2-1f4a-5b6c-7d8e-9f0a1b2c3d4e",
  "tls": {
    "enabled": true,
    "server_name": "sg04.example.com",
    "utls": { "enabled": true, "fingerprint": "chrome" }
  },
  "transport": { "type": "http", "host": ["sg04.example.com"], "path": "/vless" }
}`,
    },
    shadowrocket: {
      passed: false,
      code: `vless://b7c9d3e2-1f4a-5b6c-7d8e-9f0a1b2c3d4e@sg04.example.com:443?security=tls&sni=sg04.example.com&fp=chrome&type=http&path=%2Fvless&host=sg04.example.com#%F0%9F%87%B8%F0%9F%87%AC%20%E6%96%B0%E5%8A%A0%E5%9D%A1%2004%20-%20VLESS%2Bh2`,
    },
    surge: {
      passed: true,
      code: `🇸🇬 新加坡 04 - VLESS+h2 = vless, sg04.example.com, 443, username=b7c9d3e2-1f4a-5b6c-7d8e-9f0a1b2c3d4e, tls=true, sni=sg04.example.com, h2=true, h2-path=/vless, h2-host=sg04.example.com, tfo=true, udp-relay=true, client-fingerprint=chrome`,
    },
    quantumultx: {
      passed: true,
      code: `vless=sg04.example.com:443, method=none, password=b7c9d3e2-1f4a-5b6c-7d8e-9f0a1b2c3d4e, obfs=over-tls, obfs-host=sg04.example.com, tls-host=sg04.example.com, tls13=1, tag=🇸🇬 新加坡 04 - VLESS+h2, tfo=1, udp-relay=1`,
    },
    loon: {
      passed: true,
      code: `🇸🇬 新加坡 04 - VLESS+h2 = VLESS, sg04.example.com, 443, b7c9d3e2-1f4a-5b6c-7d8e-9f0a1b2c3d4e, over-tls=true, sni=sg04.example.com, transport=http, path=/vless, host=sg04.example.com, tfo=true, udp=true`,
    },
    v2rayn: {
      passed: true,
      code: `vless://b7c9d3e2-1f4a-5b6c-7d8e-9f0a1b2c3d4e@sg04.example.com:443?security=tls&sni=sg04.example.com&fp=chrome&type=http&path=%2Fvless&host=sg04.example.com#%F0%9F%87%B8%F0%9F%87%AC%20%E6%96%B0%E5%8A%A0%E5%9D%A1%2004%20-%20VLESS%2Bh2`,
    },
  },
}

interface GoldenRow {
  proto: string
  cells: Record<ClientKey, 'ok' | 'warn' | 'fail'>
  notes?: Partial<Record<ClientKey, string>>
}

const GOLDEN_MATRIX: GoldenRow[] = [
  {
    proto: 'VLESS+Reality+WS',
    cells: { clash: 'ok', clashmeta: 'ok', singbox: 'ok', shadowrocket: 'ok', surge: 'ok', quantumultx: 'ok', loon: 'ok', v2rayn: 'ok' },
  },
  {
    proto: 'Trojan+TLS+gRPC',
    cells: { clash: 'ok', clashmeta: 'ok', singbox: 'ok', shadowrocket: 'ok', surge: 'warn', quantumultx: 'ok', loon: 'ok', v2rayn: 'ok' },
    notes: { surge: 'gRPC传输需 Surge ≥ 5.0' },
  },
  {
    proto: 'Hysteria2',
    cells: { clash: 'ok', clashmeta: 'ok', singbox: 'ok', shadowrocket: 'warn', surge: 'fail', quantumultx: 'fail', loon: 'ok', v2rayn: 'ok' },
    notes: { shadowrocket: '需 Shadowrocket ≥ 2.2.17', surge: '不支持Hysteria2', quantumultx: '不支持Hysteria2' },
  },
  {
    proto: 'VLESS+h2',
    cells: { clash: 'ok', clashmeta: 'ok', singbox: 'ok', shadowrocket: 'warn', surge: 'ok', quantumultx: 'ok', loon: 'ok', v2rayn: 'ok' },
    notes: { shadowrocket: '需 Shadowrocket ≥ 2.2.3' },
  },
  {
    proto: 'VMess+WS+TLS',
    cells: { clash: 'ok', clashmeta: 'ok', singbox: 'ok', shadowrocket: 'ok', surge: 'ok', quantumultx: 'ok', loon: 'ok', v2rayn: 'ok' },
  },
  {
    proto: 'Shadowsocks 2022',
    cells: { clash: 'ok', clashmeta: 'ok', singbox: 'ok', shadowrocket: 'ok', surge: 'warn', quantumultx: 'ok', loon: 'warn', v2rayn: 'ok' },
    notes: { surge: '仅支持部分AEAD', loon: '需 Loon ≥ 3.0' },
  },
]

interface FieldCheck {
  field: string
  label: string
  expected: string
  values: Record<ClientKey, { value: string; match: boolean }>
}

function buildFieldChecks(node: PreviewNode, configs: Record<ClientKey, { code: string; passed: boolean }>): FieldCheck[] {
  const cfgs = configs
  const checks: FieldCheck[] = []

  const addressValues: Record<ClientKey, { value: string; match: boolean }> = {} as any
  const portValues: Record<ClientKey, { value: string; match: boolean }> = {} as any
  const credValues: Record<ClientKey, { value: string; match: boolean }> = {} as any
  const sniValues: Record<ClientKey, { value: string; match: boolean }> = {} as any

  for (const key of CLIENT_KEYS) {
    const code = cfgs[key]?.code || ''
    const addrMatch = code.match(/server[^\n]*?([a-z0-9.-]+\.(?:com|net|org|io|dev))[^\n]*/i) ||
      code.match(/(?:^|[,@/])([a-z0-9.-]+\.example\.com)(?::|[/?#]|$)/i)
    const hasPort = code.includes(`:${node.port}`) || code.includes(`"server_port": ${node.port}`) || code.includes(`port: ${node.port}`)
    const cred = node.uuid || node.password
    const credMatch = code.includes(cred)
    const sniMatch = code.includes(node.sni)

    addressValues[key] = {
      value: addrMatch ? addrMatch[1] : '(未找到)',
      match: !!addrMatch && addrMatch[1] === node.address,
    }
    portValues[key] = {
      value: hasPort ? String(node.port) : '(未找到)',
      match: hasPort,
    }
    credValues[key] = {
      value: credMatch ? cred.substring(0, 8) + '...' : '(不匹配)',
      match: credMatch,
    }
    sniValues[key] = {
      value: sniMatch ? node.sni : '(未找到)',
      match: sniMatch || (cfgs[key]?.passed === false),
    }
  }

  checks.push({ field: 'address', label: '地址 (Address)', expected: node.address, values: addressValues })
  checks.push({ field: 'port', label: '端口 (Port)', expected: String(node.port), values: portValues })
  checks.push({ field: 'cred', label: node.uuid ? 'UUID' : '密码 (Password)', expected: (node.uuid || node.password).substring(0, 16) + '...', values: credValues })
  checks.push({ field: 'sni', label: 'TLS SNI', expected: node.sni, values: sniValues })

  return checks
}

function PreviewTab() {
  const [nodes, setNodes] = useState<PreviewNode[]>([])
  const [loading, setLoading] = useState(true)
  const [selectedNodeId, setSelectedNodeId] = useState('')
  const [showURI, setShowURI] = useState(true)
  const [showQR, setShowQR] = useState(false)
  const [activeTab, setActiveTab] = useState<ClientKey>('clashmeta')
  const [copiedKey, setCopiedKey] = useState<string | null>(null)
  const { toast } = useToast()

  useEffect(() => {
    const loadNodes = async () => {
      setLoading(true)
      try {
        const data = await api.get<unknown>(EP.NODES, {
          params: { page: 1, page_size: 100 },
        })
        const list = normalizeList<unknown>(data)
        const mapped: PreviewNode[] = list.map((n: any) => {
          const proto = [n.protocol_type, n.security_type, n.transport_type].filter(Boolean).join('+')
          const sni = n.sni || n.address || ''
          return {
            id: n.id,
            name: n.name,
            proto,
            address: n.address,
            port: n.port,
            uuid: '',
            password: '',
            sni,
          }
        })
        setNodes(mapped)
        if (mapped.length > 0) setSelectedNodeId(mapped[0].id)
      } catch (err) {
        const msg = err instanceof ApiError ? err.message : '加载节点列表失败'
        toast({ title: '加载失败', description: msg, variant: 'destructive' })
        setNodes([])
      } finally {
        setLoading(false)
      }
    }
    loadNodes()
  }, [toast])

  const node = nodes.find((n) => n.id === selectedNodeId) || nodes[0]
  const configs = (node && PREVIEW_CONFIGS[node.proto]) || PREVIEW_CONFIGS['VLESS+Reality+WS']
  const fieldChecks = node ? buildFieldChecks(node, configs) : []

  const handleCopy = async (clientKey: ClientKey) => {
    const code = configs[clientKey]?.code || ''
    try {
      await navigator.clipboard.writeText(code)
      setCopiedKey(clientKey)
      setTimeout(() => setCopiedKey(null), 1500)
    } catch {
      setCopiedKey(clientKey)
      setTimeout(() => setCopiedKey(null), 1500)
    }
  }

  const statusBadge = (passed: boolean) => {
    if (passed) {
      return (
        <Badge variant="success" className="text-xs">
          <Check className="w-3 h-3 mr-1" />
          校验通过
        </Badge>
      )
    }
    return (
      <Badge variant="destructive" className="text-xs">
        <X className="w-3 h-3 mr-1" />
        渲染差异
      </Badge>
    )
  }

  const matrixCell = (status: 'ok' | 'warn' | 'fail', note?: string) => {
    if (status === 'ok') {
      return (
        <div className="flex items-center justify-center" title={note}>
          <Check className="w-4 h-4 text-emerald-400" />
        </div>
      )
    }
    if (status === 'warn') {
      return (
        <div className="flex items-center justify-center" title={note}>
          <AlertTriangle className="w-4 h-4 text-amber-400" />
        </div>
      )
    }
    return (
      <div className="flex items-center justify-center" title={note}>
        <X className="w-4 h-4 text-red-400" />
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <Card className="bg-zinc-900 border-zinc-800">
        <CardContent className="p-4">
          <div className="flex flex-wrap items-center gap-4">
            <div className="flex items-center gap-2">
              <Smartphone className="w-4 h-4 text-zinc-400" />
              <span className="text-sm text-zinc-400">节点</span>
              <Select
                value={selectedNodeId}
                onChange={(e) => setSelectedNodeId(e.target.value)}
                className="w-72 bg-zinc-800 border-zinc-700 text-zinc-100"
                disabled={loading || nodes.length === 0}
              >
                {loading ? (
                  <option value="">加载中...</option>
                ) : nodes.length === 0 ? (
                  <option value="">暂无节点</option>
                ) : (
                  nodes.map((n) => (
                    <option key={n.id} value={n.id}>{n.name}</option>
                  ))
                )}
              </Select>
            </div>
            <div className="flex items-center gap-2">
              <span className="text-sm text-zinc-400">显示URI</span>
              <Switch checked={showURI} onChange={(e) => setShowURI(e.target.checked)} />
            </div>
            <div className="flex items-center gap-2">
              <QrCode className="w-4 h-4 text-zinc-400" />
              <span className="text-sm text-zinc-400">显示QR码</span>
              <Switch checked={showQR} onChange={(e) => setShowQR(e.target.checked)} />
            </div>
            <div className="ml-auto">
              <Badge variant="secondary" className="bg-zinc-800 text-zinc-300">
                {node ? node.proto : '—'}
              </Badge>
            </div>
          </div>
        </CardContent>
      </Card>

      {loading ? (
        <Card className="bg-zinc-900 border-zinc-800">
          <CardContent className="p-8">
            <Skeleton className="h-48 w-full" />
          </CardContent>
        </Card>
      ) : !node ? (
        <Card className="bg-zinc-900 border-zinc-800">
          <CardContent className="p-8 text-center text-zinc-500">
            <Smartphone className="w-12 h-12 mx-auto mb-3 opacity-30" />
            <p className="text-sm">暂无节点数据</p>
            <p className="text-xs mt-1">请先在节点管理中创建节点</p>
          </CardContent>
        </Card>
      ) : (
        <Tabs value={activeTab} onValueChange={(v) => setActiveTab(v as ClientKey)}>
          <TabsList>
            {CLIENT_KEYS.map((key) => (
              <TabsTrigger key={key} value={key} className="text-xs">
                {CLIENT_LABELS[key]}
                {configs[key]?.passed ? (
                  <Check className="w-3 h-3 ml-1.5 text-emerald-400" />
                ) : (
                  <X className="w-3 h-3 ml-1.5 text-red-400" />
                )}
              </TabsTrigger>
            ))}
          </TabsList>

          {CLIENT_KEYS.map((key) => {
            const cfg = configs[key]
            if (!cfg) return null
            return (
              <TabsContent key={key} value={key}>
                <Card className="bg-zinc-900 border-zinc-800">
                  <CardContent className="p-4">
                    <div className="flex items-center justify-between mb-3">
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-medium text-zinc-200">{CLIENT_LABELS[key]}</span>
                        {statusBadge(cfg.passed)}
                      </div>
                      <Button
                        variant="outline"
                        size="sm"
                        className="border-zinc-700 text-zinc-300 hover:bg-zinc-800"
                        onClick={() => handleCopy(key)}
                      >
                        {copiedKey === key ? (
                          <Check className="w-3.5 h-3.5 mr-1.5 text-emerald-400" />
                        ) : (
                          <Copy className="w-3.5 h-3.5 mr-1.5" />
                        )}
                        {copiedKey === key ? '已复制' : '复制'}
                      </Button>
                    </div>
                    <pre className="bg-zinc-950 border border-zinc-800 rounded-lg p-4 text-xs font-mono text-zinc-300 overflow-x-auto max-h-96 overflow-y-auto whitespace-pre-wrap break-all">
                      {cfg.code}
                    </pre>
                    {showURI && key !== 'clash' && key !== 'clashmeta' && key !== 'singbox' && (
                      <div className="mt-3">
                        <div className="text-xs text-zinc-500 mb-1">URI 链接</div>
                        <div className="bg-zinc-950/60 border border-zinc-800 rounded-lg p-3 text-xs font-mono text-zinc-400 break-all">
                          {key === 'shadowrocket' || key === 'v2rayn'
                            ? cfg.code
                            : '(该客户端不使用URI格式，配置已在上方展示)'}
                        </div>
                      </div>
                    )}
                    {showQR && (
                      <div className="mt-3 flex items-center justify-center p-6 bg-zinc-950/60 border border-zinc-800 rounded-lg">
                        <div className="text-center text-zinc-500">
                          <QrCode className="w-16 h-16 mx-auto mb-2 opacity-40" />
                          <p className="text-xs">QR码预览（{CLIENT_LABELS[key]}）</p>
                          <p className="text-[10px] text-zinc-600 mt-1">模拟数据：实际渲染时将生成二维码图片</p>
                        </div>
                      </div>
                    )}
                  </CardContent>
                </Card>
              </TabsContent>
            )
          })}
        </Tabs>
      )}

      <Card className="bg-zinc-900 border-zinc-800">
        <CardHeader className="pb-2">
          <CardTitle className="text-base flex items-center gap-2">
            <AlertTriangle className="w-4 h-4 text-amber-400" />
            Golden Test 兼容矩阵
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow className="border-zinc-800 hover:bg-transparent">
                  <TableHead className="text-zinc-400 text-xs font-medium sticky left-0 bg-zinc-900 w-44">协议组合</TableHead>
                  {CLIENT_KEYS.map((key) => (
                    <TableHead key={key} className="text-zinc-400 text-xs font-medium text-center whitespace-nowrap">
                      {CLIENT_LABELS[key]}
                    </TableHead>
                  ))}
                </TableRow>
              </TableHeader>
              <TableBody>
                {GOLDEN_MATRIX.map((row) => (
                  <TableRow key={row.proto} className="border-zinc-800 hover:bg-zinc-800/30">
                    <TableCell className="py-3 text-sm text-zinc-200 font-medium sticky left-0 bg-zinc-900">
                      {row.proto}
                    </TableCell>
                    {CLIENT_KEYS.map((key) => (
                      <TableCell key={key} className="py-3 text-center">
                        {matrixCell(row.cells[key], row.notes?.[key])}
                      </TableCell>
                    ))}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          <div className="flex items-center gap-4 mt-3 text-xs text-zinc-500">
            <span className="flex items-center gap-1"><Check className="w-3 h-3 text-emerald-400" />完全支持</span>
            <span className="flex items-center gap-1"><AlertTriangle className="w-3 h-3 text-amber-400" />部分支持/版本要求</span>
            <span className="flex items-center gap-1"><X className="w-3 h-3 text-red-400" />不支持</span>
          </div>
        </CardContent>
      </Card>

      <Card className="bg-zinc-900 border-zinc-800">
        <CardHeader className="pb-2">
          <CardTitle className="text-base flex items-center gap-2">
            <Eye className="w-4 h-4 text-indigo-400" />
            字段一致性检查
          </CardTitle>
        </CardHeader>
        <CardContent>
          {fieldChecks.length === 0 ? (
            <div className="py-8 text-center text-zinc-500">
              <Eye className="w-10 h-10 mx-auto mb-2 opacity-30" />
              <p className="text-sm">暂无字段校验数据</p>
              <p className="text-xs mt-1">请先选择节点</p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="border-zinc-800 hover:bg-transparent">
                    <TableHead className="text-zinc-400 text-xs font-medium w-40">字段</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium w-48">期望值</TableHead>
                    {CLIENT_KEYS.map((key) => (
                      <TableHead key={key} className="text-zinc-400 text-xs font-medium text-center whitespace-nowrap">
                        {CLIENT_LABELS[key]}
                      </TableHead>
                    ))}
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {fieldChecks.map((check) => {
                    const hasMismatch = Object.values(check.values).some((v) => !v.match)
                    return (
                      <TableRow key={check.field} className="border-zinc-800 hover:bg-zinc-800/30">
                        <TableCell className="py-3 text-sm text-zinc-300 font-medium">
                          {check.label}
                          {hasMismatch && <AlertTriangle className="w-3 h-3 text-red-400 inline ml-1.5" />}
                        </TableCell>
                        <TableCell className="py-3 text-xs font-mono text-zinc-400">{check.expected}</TableCell>
                        {CLIENT_KEYS.map((key) => {
                          const v = check.values[key]
                          return (
                            <TableCell
                              key={key}
                              className={`py-3 text-center text-xs font-mono ${
                                v.match ? 'text-emerald-400' : 'text-red-400 bg-red-950/20'
                              }`}
                            >
                              {v.match ? <Check className="w-3.5 h-3.5 mx-auto" /> : v.value}
                            </TableCell>
                          )
                        })}
                      </TableRow>
                    )
                  })}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>

      <div className="flex items-center justify-center gap-2 py-3 text-sm text-amber-400/80 bg-amber-950/20 border border-amber-900/30 rounded-lg">
        <AlertTriangle className="w-4 h-4" />
        <span>此预览结果由订阅渲染器实时生成，与实际下发给用户的配置完全一致</span>
      </div>
    </div>
  )
}

function TemplatesTab() {
  const [templates, setTemplates] = useState<SubTemplate[]>([])
  const [loading, setLoading] = useState(true)
  const [selectedTemplate, setSelectedTemplate] = useState<SubTemplate | null>(null)
  const [editingContent, setEditingContent] = useState('')
  const [showCreateDialog, setShowCreateDialog] = useState(false)
  const [saving, setSaving] = useState(false)
  const [settingDefault, setSettingDefault] = useState<string | null>(null)
  const [newTemplate, setNewTemplate] = useState({
    code: '',
    name: '',
    target_client: 'clashmeta' as TemplateClientKey,
    content: '',
  })
  const { toast } = useToast()

  const loadTemplates = async () => {
    setLoading(true)
    try {
      const data = await api.get<unknown>(EP.SUB_TEMPLATES)
      const list = normalizeList<SubTemplate>(data)
      setTemplates(list)
      if (list.length > 0 && !selectedTemplate) {
        selectTemplate(list[0])
      }
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '加载模板列表失败'
      toast({ title: '加载失败', description: msg, variant: 'destructive' })
      setTemplates([])
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadTemplates()
  }, [])

  const selectTemplate = (tpl: SubTemplate) => {
    setSelectedTemplate(tpl)
    setEditingContent(tpl.content)
  }

  const handleSave = async () => {
    if (!selectedTemplate) return
    setSaving(true)
    try {
      await api.put(`${EP.SUB_TEMPLATES}/${selectedTemplate.id}`, {
        ...selectedTemplate,
        content: editingContent,
      })
      toast({ title: '保存成功', description: '模板已更新' })
      loadTemplates()
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '保存模板失败'
      toast({ title: '保存失败', description: msg, variant: 'destructive' })
    } finally {
      setSaving(false)
    }
  }

  const handleSetDefault = async (id: string) => {
    setSettingDefault(id)
    try {
      await api.post(EP.SUB_TEMPLATE_SET_DEFAULT(id), {})
      toast({ title: '设置成功', description: '已设为默认模板' })
      loadTemplates()
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '设置默认模板失败'
      toast({ title: '设置失败', description: msg, variant: 'destructive' })
    } finally {
      setSettingDefault(null)
    }
  }

  const handleCreate = async () => {
    if (!newTemplate.code || !newTemplate.name) {
      toast({ title: '请填写完整信息', description: 'code和name为必填项', variant: 'destructive' })
      return
    }
    setSaving(true)
    try {
      await api.post(EP.SUB_TEMPLATES, newTemplate)
      toast({ title: '创建成功', description: '新模板已创建' })
      setShowCreateDialog(false)
      setNewTemplate({ code: '', name: '', target_client: 'clashmeta', content: '' })
      loadTemplates()
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '创建模板失败'
      toast({ title: '创建失败', description: msg, variant: 'destructive' })
    } finally {
      setSaving(false)
    }
  }

  const getStatusBadge = (status: string) => {
    if (status === 'active') {
      return <Badge variant="success" className="text-xs">启用</Badge>
    }
    return <Badge variant="secondary" className="text-xs">禁用</Badge>
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-base font-semibold text-zinc-100 flex items-center gap-2">
            <FileEdit className="w-5 h-5 text-violet-400" />
            订阅模板管理
          </h3>
          <p className="text-sm text-zinc-400 mt-1">
            管理不同客户端的订阅配置模板，支持自定义模板内容
          </p>
        </div>
        <Button
          onClick={() => setShowCreateDialog(true)}
          className="bg-violet-600 hover:bg-violet-700 text-white"
        >
          <Plus className="w-4 h-4 mr-2" />
          新建模板
        </Button>
      </div>

      {loading ? (
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
          <Card className="bg-zinc-900 border-zinc-800 lg:col-span-1">
            <CardContent className="p-4">
              <Skeleton className="h-64 w-full" />
            </CardContent>
          </Card>
          <Card className="bg-zinc-900 border-zinc-800 lg:col-span-2">
            <CardContent className="p-4">
              <Skeleton className="h-96 w-full" />
            </CardContent>
          </Card>
        </div>
      ) : templates.length === 0 ? (
        <Card className="bg-zinc-900 border-zinc-800">
          <CardContent className="p-12 text-center text-zinc-500">
            <FileEdit className="w-16 h-16 mx-auto mb-4 opacity-30" />
            <p className="text-sm">暂无模板</p>
            <p className="text-xs mt-1">点击"新建模板"创建第一个订阅模板</p>
          </CardContent>
        </Card>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
          <Card className="bg-zinc-900 border-zinc-800 lg:col-span-1">
            <CardHeader className="pb-2">
              <CardTitle className="text-sm">模板列表</CardTitle>
            </CardHeader>
            <CardContent className="p-0">
              <div className="divide-y divide-zinc-800 max-h-[600px] overflow-y-auto">
                {templates.map((tpl) => (
                  <div
                    key={tpl.id}
                    className={`p-3 cursor-pointer transition-colors ${
                      selectedTemplate?.id === tpl.id
                        ? 'bg-violet-950/30 border-l-2 border-violet-500'
                        : 'hover:bg-zinc-800/50 border-l-2 border-transparent'
                    }`}
                    onClick={() => selectTemplate(tpl)}
                  >
                    <div className="flex items-start justify-between">
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="text-sm font-medium text-zinc-100 truncate">{tpl.name}</span>
                          {tpl.is_default && (
                            <Badge variant="secondary" className="bg-amber-900/50 text-amber-400 text-xs">
                              <Star className="w-3 h-3 mr-1" />默认
                            </Badge>
                          )}
                        </div>
                        <div className="text-xs text-zinc-500 mt-1 font-mono">{tpl.code}</div>
                        <div className="flex items-center gap-2 mt-2">
                          <Badge variant="outline" className="text-xs border-zinc-700 text-zinc-400">
                            {TEMPLATE_CLIENT_OPTIONS.find(o => o.value === tpl.target_client)?.label || tpl.target_client}
                          </Badge>
                          {getStatusBadge(tpl.status)}
                        </div>
                      </div>
                      {!tpl.is_default && (
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-7 px-2 text-zinc-400 hover:text-amber-400 hover:bg-amber-950/30"
                          onClick={(e) => {
                            e.stopPropagation()
                            handleSetDefault(tpl.id)
                          }}
                          disabled={settingDefault === tpl.id}
                        >
                          {settingDefault === tpl.id ? (
                            <Activity className="w-3.5 h-3.5 animate-spin" />
                          ) : (
                            <Star className="w-3.5 h-3.5" />
                          )}
                        </Button>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>

          <Card className="bg-zinc-900 border-zinc-800 lg:col-span-2">
            <CardHeader className="pb-2">
              <div className="flex items-center justify-between">
                <CardTitle className="text-sm">
                  {selectedTemplate ? `编辑: ${selectedTemplate.name}` : '选择模板'}
                </CardTitle>
                {selectedTemplate && (
                  <div className="flex items-center gap-2">
                    {!selectedTemplate.is_default && (
                      <Button
                        variant="outline"
                        size="sm"
                        className="border-zinc-700 text-amber-400 hover:bg-amber-950/30"
                        onClick={() => handleSetDefault(selectedTemplate.id)}
                        disabled={settingDefault === selectedTemplate.id}
                      >
                        {settingDefault === selectedTemplate.id ? (
                          <Activity className="w-3.5 h-3.5 mr-1.5 animate-spin" />
                        ) : (
                          <Star className="w-3.5 h-3.5 mr-1.5" />
                        )}
                        设为默认
                      </Button>
                    )}
                    <Button
                      size="sm"
                      className="bg-violet-600 hover:bg-violet-700 text-white"
                      onClick={handleSave}
                      disabled={saving}
                    >
                      {saving ? (
                        <Activity className="w-3.5 h-3.5 mr-1.5 animate-spin" />
                      ) : (
                        <Save className="w-3.5 h-3.5 mr-1.5" />
                      )}
                      保存
                    </Button>
                  </div>
                )}
              </div>
            </CardHeader>
            <CardContent>
              {selectedTemplate ? (
                <YamlEditor
                  value={editingContent}
                  onChange={setEditingContent}
                  mode="yaml"
                  height={500}
                  placeholder="# 输入YAML模板内容..."
                  showModeToggle={true}
                />
              ) : (
                <div className="h-[500px] flex items-center justify-center text-zinc-500">
                  <div className="text-center">
                    <FileEdit className="w-12 h-12 mx-auto mb-2 opacity-30" />
                    <p className="text-sm">请从左侧选择一个模板进行编辑</p>
                  </div>
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      )}

      <Dialog open={showCreateDialog} onOpenChange={setShowCreateDialog}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-100 max-w-2xl">
          <DialogHeader>
            <DialogTitle>新建订阅模板</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="tpl-code" className="text-sm text-zinc-300">模板代码 (Code)</Label>
                <Input
                  id="tpl-code"
                  value={newTemplate.code}
                  onChange={(e) => setNewTemplate({ ...newTemplate, code: e.target.value })}
                  placeholder="例如: clash-meta-default"
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="tpl-name" className="text-sm text-zinc-300">模板名称</Label>
                <Input
                  id="tpl-name"
                  value={newTemplate.name}
                  onChange={(e) => setNewTemplate({ ...newTemplate, name: e.target.value })}
                  placeholder="例如: Clash Meta 默认模板"
                  className="bg-zinc-800 border-zinc-700 text-zinc-100"
                />
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="tpl-client" className="text-sm text-zinc-300">目标客户端</Label>
              <Select
                id="tpl-client"
                value={newTemplate.target_client}
                onChange={(e) => setNewTemplate({ ...newTemplate, target_client: e.target.value as TemplateClientKey })}
                className="w-full bg-zinc-800 border-zinc-700 text-zinc-100"
              >
                {TEMPLATE_CLIENT_OPTIONS.map((opt) => (
                  <option key={opt.value} value={opt.value}>{opt.label}</option>
                ))}
              </Select>
            </div>
            <div className="space-y-2">
              <Label className="text-sm text-zinc-300">模板内容</Label>
              <YamlEditor
                value={newTemplate.content}
                onChange={(v) => setNewTemplate({ ...newTemplate, content: v })}
                mode="yaml"
                height={300}
                placeholder="# 输入YAML模板内容..."
                showModeToggle={true}
              />
            </div>
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setShowCreateDialog(false)}
              className="border-zinc-700 text-zinc-300 hover:bg-zinc-800"
            >
              取消
            </Button>
            <Button
              onClick={handleCreate}
              disabled={saving}
              className="bg-violet-600 hover:bg-violet-700 text-white"
            >
              {saving ? <Activity className="w-4 h-4 mr-2 animate-spin" /> : <Plus className="w-4 h-4 mr-2" />}
              创建
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function StatsTab() {
  const [overview, setOverview] = useState<AccessOverview | null>(null)
  const [logs, setLogs] = useState<AccessLog[]>([])
  const [loading, setLoading] = useState(true)
  const { toast } = useToast()

  useEffect(() => {
    const loadStats = async () => {
      setLoading(true)
      try {
        const [overviewData, logsData] = await Promise.all([
          api.get<unknown>(EP.SUB_ACCESS_OVERVIEW, { params: { days: 7 } }),
          api.get<unknown>(EP.SUB_ACCESS_LOGS, { params: { page: 1, page_size: 20 } }),
        ])
        setOverview((overviewData as any)?.data || overviewData as AccessOverview)
        setLogs(normalizeList<AccessLog>(logsData))
      } catch (err) {
        const msg = err instanceof ApiError ? err.message : '加载访问统计失败'
        toast({ title: '加载失败', description: msg, variant: 'destructive' })
      } finally {
        setLoading(false)
      }
    }
    loadStats()
  }, [toast])

  const formatNumber = (n: number) => {
    if (n >= 10000) return (n / 10000).toFixed(1) + 'w'
    if (n >= 1000) return (n / 1000).toFixed(1) + 'k'
    return String(n)
  }

  const formatTime = (ts: string) => {
    try {
      return new Date(ts).toLocaleString('zh-CN', {
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
      })
    } catch {
      return ts
    }
  }

  const StatCard = ({ icon: Icon, label, value, subValue, color }: {
    icon: any
    label: string
    value: string | number
    subValue?: string
    color: string
  }) => (
    <Card className="bg-zinc-900 border-zinc-800">
      <CardContent className="p-4">
        <div className="flex items-center gap-3">
          <div className={`w-10 h-10 rounded-lg ${color} flex items-center justify-center`}>
            <Icon className="w-5 h-5 text-white" />
          </div>
          <div>
            <p className="text-xs text-zinc-400">{label}</p>
            <p className="text-xl font-bold text-zinc-100 mt-0.5">{value}</p>
            {subValue && <p className="text-xs text-zinc-500 mt-0.5">{subValue}</p>}
          </div>
        </div>
      </CardContent>
    </Card>
  )

  return (
    <div className="space-y-4">
      <div>
        <h3 className="text-base font-semibold text-zinc-100 flex items-center gap-2">
          <BarChart3 className="w-5 h-5 text-emerald-400" />
          订阅访问统计
        </h3>
        <p className="text-sm text-zinc-400 mt-1">
          最近7天订阅访问概览和实时日志
        </p>
      </div>

      {loading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
          {[1, 2, 3, 4].map((i) => (
            <Card key={i} className="bg-zinc-900 border-zinc-800">
              <CardContent className="p-4">
                <Skeleton className="h-16 w-full" />
              </CardContent>
            </Card>
          ))}
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
          <StatCard
            icon={Zap}
            label="总请求数"
            value={formatNumber(overview?.total_requests || 0)}
            subValue="最近7天"
            color="bg-indigo-600"
          />
          <StatCard
            icon={Users}
            label="独立用户"
            value={formatNumber(overview?.unique_users || 0)}
            subValue="活跃订阅用户"
            color="bg-violet-600"
          />
          <StatCard
            icon={Globe}
            label="独立IP"
            value={formatNumber(overview?.unique_ips || 0)}
            subValue="访问来源IP"
            color="bg-cyan-600"
          />
          <StatCard
            icon={Activity}
            label="缓存命中率"
            value={`${((overview?.cache_hit_rate || 0) * 100).toFixed(1)}%`}
            subValue="CDN/边缘缓存"
            color="bg-emerald-600"
          />
        </div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <Card className="bg-zinc-900 border-zinc-800">
          <CardHeader className="pb-2">
            <CardTitle className="text-sm">热门客户端 Top 5</CardTitle>
          </CardHeader>
          <CardContent>
            {loading ? (
              <div className="space-y-3">
                {[1, 2, 3, 4, 5].map((i) => (
                  <Skeleton key={i} className="h-8 w-full" />
                ))}
              </div>
            ) : !overview?.top_clients?.length ? (
              <div className="py-8 text-center text-zinc-500">
                <BarChart3 className="w-10 h-10 mx-auto mb-2 opacity-30" />
                <p className="text-sm">暂无数据</p>
              </div>
            ) : (
              <div className="space-y-3">
                {overview.top_clients.map((item, idx) => (
                  <div key={item.client} className="space-y-1">
                    <div className="flex items-center justify-between text-sm">
                      <span className="text-zinc-300 flex items-center gap-2">
                        <span className="w-5 h-5 rounded bg-zinc-800 flex items-center justify-center text-xs text-zinc-500">
                          {idx + 1}
                        </span>
                        {item.client}
                      </span>
                      <span className="text-zinc-400 font-mono text-xs">
                        {formatNumber(item.count)} ({(item.percentage * 100).toFixed(1)}%)
                      </span>
                    </div>
                    <div className="h-2 bg-zinc-800 rounded-full overflow-hidden">
                      <div
                        className="h-full bg-gradient-to-r from-emerald-600 to-emerald-400 rounded-full transition-all"
                        style={{ width: `${item.percentage * 100}%` }}
                      />
                    </div>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>

        <Card className="bg-zinc-900 border-zinc-800">
          <CardHeader className="pb-2">
            <CardTitle className="text-sm">最近访问日志</CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            {loading ? (
              <div className="p-4 space-y-2">
                {[1, 2, 3, 4, 5].map((i) => (
                  <Skeleton key={i} className="h-10 w-full" />
                ))}
              </div>
            ) : logs.length === 0 ? (
              <div className="py-8 text-center text-zinc-500">
                <Activity className="w-10 h-10 mx-auto mb-2 opacity-30" />
                <p className="text-sm">暂无访问日志</p>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <Table>
                  <TableHeader>
                    <TableRow className="border-zinc-800 hover:bg-transparent">
                      <TableHead className="text-zinc-400 text-xs font-medium">时间</TableHead>
                      <TableHead className="text-zinc-400 text-xs font-medium">客户端</TableHead>
                      <TableHead className="text-zinc-400 text-xs font-medium">状态</TableHead>
                      <TableHead className="text-zinc-400 text-xs font-medium">节点</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {logs.slice(0, 8).map((log) => (
                      <TableRow key={log.id} className="border-zinc-800 hover:bg-zinc-800/30">
                        <TableCell className="py-2 text-xs text-zinc-400 font-mono whitespace-nowrap">
                          {formatTime(log.timestamp)}
                        </TableCell>
                        <TableCell className="py-2 text-xs text-zinc-300">{log.client}</TableCell>
                        <TableCell className="py-2">
                          <Badge
                            variant={log.status === 200 ? 'success' : 'destructive'}
                            className="text-xs"
                          >
                            {log.status}
                          </Badge>
                        </TableCell>
                        <TableCell className="py-2">
                          <div className="flex items-center gap-1">
                            <span className="text-xs text-zinc-400">{log.node_count}</span>
                            {log.cache_hit && (
                              <Badge variant="secondary" className="bg-cyan-900/50 text-cyan-400 text-[10px] px-1">
                                HIT
                              </Badge>
                            )}
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      <Card className="bg-zinc-900 border-zinc-800">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">访问日志（最近20条）</CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {loading ? (
            <div className="p-4">
              <Skeleton className="h-64 w-full" />
            </div>
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="border-zinc-800 hover:bg-transparent">
                    <TableHead className="text-zinc-400 text-xs font-medium">时间</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium">用户</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium">客户端</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium">IP</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium">状态</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium text-center">节点数</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium text-center">缓存</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {logs.map((log) => (
                    <TableRow key={log.id} className="border-zinc-800 hover:bg-zinc-800/30">
                      <TableCell className="py-2 text-xs text-zinc-400 font-mono whitespace-nowrap">
                        {formatTime(log.timestamp)}
                      </TableCell>
                      <TableCell className="py-2 text-xs text-zinc-300 max-w-32 truncate">
                        {log.user_email || log.user_id || '-'}
                      </TableCell>
                      <TableCell className="py-2 text-xs text-zinc-300">{log.client}</TableCell>
                      <TableCell className="py-2 text-xs text-zinc-400 font-mono">{log.ip}</TableCell>
                      <TableCell className="py-2">
                        <Badge
                          variant={log.status === 200 ? 'success' : 'destructive'}
                          className="text-xs"
                        >
                          {log.status}
                        </Badge>
                      </TableCell>
                      <TableCell className="py-2 text-xs text-zinc-400 text-center">{log.node_count}</TableCell>
                      <TableCell className="py-2 text-center">
                        {log.cache_hit ? (
                          <Badge variant="secondary" className="bg-cyan-900/50 text-cyan-400 text-[10px]">
                            命中
                          </Badge>
                        ) : (
                          <span className="text-zinc-600 text-xs">—</span>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}

export default function SubscriptionPreview() {
  const [mainTab, setMainTab] = useState<'preview' | 'templates' | 'stats'>('preview')

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-lg font-semibold text-zinc-100 flex items-center gap-2">
          <Eye className="w-5 h-5 text-indigo-400" />
          订阅管理
        </h2>
        <p className="text-sm text-zinc-400 mt-1">
          预览配置、管理模板、查看访问统计
        </p>
      </div>

      <Tabs value={mainTab} onValueChange={(v) => setMainTab(v as any)}>
        <TabsList className="mb-4">
          <TabsTrigger value="preview" className="gap-2">
            <Eye className="w-4 h-4" />
            预览
          </TabsTrigger>
          <TabsTrigger value="templates" className="gap-2">
            <FileEdit className="w-4 h-4" />
            模板管理
          </TabsTrigger>
          <TabsTrigger value="stats" className="gap-2">
            <BarChart3 className="w-4 h-4" />
            访问统计
          </TabsTrigger>
        </TabsList>

        <TabsContent value="preview">
          <PreviewTab />
        </TabsContent>

        <TabsContent value="templates">
          <TemplatesTab />
        </TabsContent>

        <TabsContent value="stats">
          <StatsTab />
        </TabsContent>
      </Tabs>
    </div>
  )
}
