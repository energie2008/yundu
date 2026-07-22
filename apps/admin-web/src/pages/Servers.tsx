import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Server,
  Cpu,
  HardDrive,
  Activity,
  Zap,
  Shield,
  RefreshCw,
  Copy,
  Eye,
  EyeOff,
  Key,
  Terminal,
  CheckCircle,
  AlertCircle,
  XCircle,
  ChevronLeft,
  Plus,
  ArrowRight,
  FileText,
  Download,
  Upload,
  Check,
} from 'lucide-react'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
  Button,
  Badge,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  Skeleton,
  EmptyState,
  Switch,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  Input,
  Label,
  Select,
  useToast,
} from '@airport/ui'
import { api, ApiError } from '@/lib/api'
import { EP } from '@/lib/endpoints'

type ServerStatus = 'provisioning' | 'active' | 'maintenance' | 'offline' | 'retired'
type ServerRole = 'node' | 'edge' | 'relay' | 'balancer'
type RuntimeStatus = 'inactive' | 'active' | 'error'
type NodeStatus = 'unknown' | 'healthy' | 'degraded' | 'offline' | 'disabled'
type KernelStatus = 'running' | 'stopped' | 'restarting'
type CheckStatus = 'pass' | 'warn' | 'fail'

interface KernelInfo {
  name: string
  version: string
  status: KernelStatus
  restartCount: number
  memoryMB: number
  apiPort: number
  uptimeSeconds?: number
}

interface PreflightCheck {
  name: string
  status: CheckStatus
  description?: string
}

interface AssociatedNode {
  id: string
  code: string
  name: string
  protocol: string
  port: number
  apiPort?: number
  status: NodeStatus
  address: string
}

interface RuntimeMetrics {
  cpuPercent: number
  memoryPercent: number
  memoryUsedMB: number
  memoryTotalMB: number
  diskPercent: number
  diskUsedGB: number
  diskTotalGB: number
  networkInKBps: number
  networkOutKBps: number
  uptimeSeconds: number
  onlineUsers: number
}

interface ServerDetail {
  id: string
  code: string
  sid: number
  name: string
  host: string
  ipv4: string
  ipv6?: string
  sshPort: number
  osName?: string
  osVersion?: string
  arch?: string
  status: ServerStatus
  role: ServerRole
  provider?: string
  region?: string
  lastHeartbeatAt?: string
  createdAt: string
  nodeCount: number
  metrics: RuntimeMetrics
  agentToken: string
  installCmd: string
  kernels: KernelInfo[]
  preflightChecks: PreflightCheck[]
  nodes: AssociatedNode[]
  lastDiagnosis?: {
    category: string
    suggestion: string
  }
}

const panelUrl = 'https://panel.example.com'

function defaultMetrics(): RuntimeMetrics {
  return {
    cpuPercent: 0,
    memoryPercent: 0,
    memoryUsedMB: 0,
    memoryTotalMB: 0,
    diskPercent: 0,
    diskUsedGB: 0,
    diskTotalGB: 0,
    networkInKBps: 0,
    networkOutKBps: 0,
    uptimeSeconds: 0,
    onlineUsers: 0,
  }
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

// 后端 ServerSystemMetrics 结构
interface ServerMetricsResponse {
  cpu_percent: number
  mem_percent: number
  mem_total_mb: number
  mem_used_mb: number
  disk_percent: number
  disk_total_gb: number
  disk_used_gb: number
  network_in_kbps: number
  network_out_kbps: number
  uptime_seconds: number
  online_users?: number
}

// 后端 ServerResponse 结构（来自 node-service internal/model/dto.go）
interface RuntimeInfoItem {
  id: string
  runtime_type: string
  display_name?: string
  runtime_version: string
  status: string
  api_port?: number
  memory_mb?: number
  restart_count?: number
  uptime_seconds?: number
  last_heartbeat_at?: string
}

interface ServerResponseItem {
  id: string
  code: string
  name: string
  host: string
  ipv4?: string
  ipv6?: string
  ssh_port?: number
  os_name?: string
  os_version?: string
  arch?: string
  status: string
  role: string
  provider?: string
  region_id?: string
  labels?: Record<string, string>
  last_heartbeat_at?: string
  created_at: string
  node_count?: number
  metrics?: ServerMetricsResponse
  runtimes?: RuntimeInfoItem[]
  nodes?: AssociatedNodeResponse[]
}

// 后端 AssociatedNodeInfo 结构（GET /admin/servers/:id 返回）
interface AssociatedNodeResponse {
  id: string
  code: string
  name: string
  protocol_type: string
  port: number
  server_port?: number
  address: string
  is_enabled: boolean
  health_status: string
}

// 后端 ServerDetailResponse 结构（含 agent_token 和 install_cmd）
interface ServerDetailResponseItem extends ServerResponseItem {
  agent_token?: string
  install_cmd?: string
}

// 将后端 ServerResponse 映射为前端 ServerDetail
function mapServerResponse(s: ServerResponseItem): ServerDetail {
  const runtimes = s.runtimes || []
  const m = s.metrics
  return {
    id: s.id,
    code: s.code,
    sid: 0,
    name: s.name,
    host: s.host,
    ipv4: s.ipv4 || '',
    ipv6: s.ipv6,
    sshPort: s.ssh_port || 22,
    osName: s.os_name,
    osVersion: s.os_version,
    arch: s.arch,
    status: (s.status as ServerStatus) || 'offline',
    role: (s.role as ServerRole) || 'node',
    provider: s.provider,
    region: s.region_id,
    lastHeartbeatAt: s.last_heartbeat_at,
    createdAt: s.created_at,
    nodeCount: s.node_count || 0,
    metrics: m ? {
      cpuPercent: m.cpu_percent || 0,
      memoryPercent: m.mem_percent || 0,
      memoryUsedMB: m.mem_used_mb || 0,
      memoryTotalMB: m.mem_total_mb || 0,
      diskPercent: m.disk_percent || 0,
      diskUsedGB: m.disk_used_gb || 0,
      diskTotalGB: m.disk_total_gb || 0,
      networkInKBps: m.network_in_kbps || 0,
      networkOutKBps: m.network_out_kbps || 0,
      uptimeSeconds: m.uptime_seconds || 0,
      onlineUsers: m.online_users || 0,
    } : defaultMetrics(),
    agentToken: '',
    installCmd: '',
    kernels: runtimes.map(rt => ({
      name: rt.display_name || rt.runtime_type,
      version: rt.runtime_version || '',
      status: (rt.status === 'active' ? 'running' : 'stopped') as KernelStatus,
      restartCount: rt.restart_count || 0,
      memoryMB: rt.memory_mb || 0,
      apiPort: rt.api_port || 0,
      uptimeSeconds: rt.uptime_seconds || 0,
    })),
    preflightChecks: [],
    nodes: (s.nodes || []).map(n => ({
      id: n.id,
      code: n.code,
      name: n.name,
      protocol: n.protocol_type,
      port: n.port,
      apiPort: n.server_port,
      status: mapNodeHealthStatus(n.health_status, n.is_enabled),
      address: n.address,
    })),
  }
}

// 将后端 health_status 映射为前端 NodeStatus
function mapNodeHealthStatus(health: string, isEnabled: boolean): NodeStatus {
  if (!isEnabled) return 'disabled'
  switch (health) {
    case 'healthy':
      return 'healthy'
    case 'degraded':
      return 'degraded'
    case 'offline':
      return 'offline'
    default:
      return 'unknown'
  }
}

function generateSparklineData(points: number = 30) {
  // 返回全 0 的初始化数组，等待真实数据填充
  return Array.from({ length: points }, () => 0)
}

function formatUptime(seconds: number): string {
  if (seconds === 0) return '离线'
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (days > 0) return `${days}天 ${hours}小时`
  if (hours > 0) return `${hours}小时 ${minutes}分钟`
  return `${minutes}分钟`
}

function formatHeartbeat(isoString?: string): string {
  if (!isoString) return '从未'
  const diff = Date.now() - new Date(isoString).getTime()
  const seconds = Math.floor(diff / 1000)
  if (seconds < 60) return `${seconds}秒前`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}分钟前`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}小时前`
  return `${Math.floor(hours / 24)}天前`
}

function getStatusBadgeClass(status: ServerStatus): string {
  switch (status) {
    case 'active':
      return 'bg-emerald-900/50 text-emerald-300 border-emerald-800/50'
    case 'provisioning':
      return 'bg-blue-900/50 text-blue-300 border-blue-800/50'
    case 'maintenance':
      return 'bg-amber-900/50 text-amber-300 border-amber-800/50'
    case 'offline':
      return 'bg-red-900/50 text-red-300 border-red-800/50'
    case 'retired':
      return 'bg-zinc-800 text-zinc-500 border-zinc-700'
    default:
      return 'bg-zinc-800 text-zinc-400'
  }
}

function getStatusText(status: ServerStatus): string {
  switch (status) {
    case 'active': return '在线'
    case 'provisioning': return '部署中'
    case 'maintenance': return '维护中'
    case 'offline': return '离线'
    case 'retired': return '已退役'
    default: return status
  }
}

function getNodeStatusBadgeClass(status: NodeStatus): string {
  switch (status) {
    case 'healthy':
      return 'bg-emerald-900/50 text-emerald-300 border-emerald-800/50'
    case 'degraded':
      return 'bg-amber-900/50 text-amber-300 border-amber-800/50'
    case 'offline':
      return 'bg-red-900/50 text-red-300 border-red-800/50'
    case 'disabled':
      return 'bg-zinc-800 text-zinc-500 border-zinc-700'
    default:
      return 'bg-zinc-800 text-zinc-400'
  }
}

function getNodeStatusText(status: NodeStatus): string {
  switch (status) {
    case 'healthy': return '正常'
    case 'degraded': return '降级'
    case 'offline': return '离线'
    case 'disabled': return '已禁用'
    case 'unknown': return '未知'
    default: return status
  }
}

function ProgressBar({ value, color = 'emerald', showLabel = false, label = '' }: { value: number; color?: string; showLabel?: boolean; label?: string }) {
  const colorMap: Record<string, string> = {
    emerald: 'bg-emerald-500',
    indigo: 'bg-indigo-500',
    amber: 'bg-amber-500',
    red: 'bg-red-500',
    violet: 'bg-violet-500',
    blue: 'bg-blue-500',
    cyan: 'bg-cyan-500',
  }
  const barColor = value > 80 ? colorMap.red : value > 60 ? colorMap.amber : colorMap[color] || colorMap.emerald
  return (
    <div className="flex items-center gap-2">
      <div className="flex-1 bg-zinc-800 rounded-full h-2">
        <div className={`h-2 rounded-full transition-all duration-500 ${barColor}`} style={{ width: `${Math.min(value, 100)}%` }} />
      </div>
      {showLabel && <span className="text-xs text-zinc-400 w-10 text-right">{label || `${value}%`}</span>}
    </div>
  )
}

function StatusDot({ status }: { status: ServerStatus }) {
  const colorMap: Record<ServerStatus, string> = {
    active: 'bg-emerald-500',
    provisioning: 'bg-blue-500',
    maintenance: 'bg-amber-500',
    offline: 'bg-red-500',
    retired: 'bg-zinc-500',
  }
  const animate = status === 'active' || status === 'provisioning'
  return <span className={`inline-block w-2 h-2 rounded-full ${colorMap[status]} ${animate ? 'animate-pulse' : ''}`} />
}

function NodeStatusDot({ status }: { status: NodeStatus }) {
  const colorMap: Record<NodeStatus, string> = {
    healthy: 'bg-emerald-500',
    degraded: 'bg-amber-500',
    offline: 'bg-red-500',
    disabled: 'bg-zinc-500',
    unknown: 'bg-zinc-400',
  }
  const animate = status === 'healthy'
  return <span className={`inline-block w-2 h-2 rounded-full ${colorMap[status]} ${animate ? 'animate-pulse' : ''}`} />
}

function KernelStatusDot({ status }: { status: KernelStatus }) {
  if (status === 'running') return <span className="inline-block w-2 h-2 rounded-full bg-emerald-500" />
  if (status === 'restarting') return <span className="inline-block w-2 h-2 rounded-full bg-amber-500 animate-pulse" />
  return <span className="inline-block w-2 h-2 rounded-full bg-red-500" />
}

function CheckIcon({ status }: { status: CheckStatus }) {
  if (status === 'pass') return <CheckCircle className="w-5 h-5 text-emerald-400 flex-shrink-0" />
  if (status === 'warn') return <AlertCircle className="w-5 h-5 text-amber-400 flex-shrink-0" />
  return <XCircle className="w-5 h-5 text-red-400 flex-shrink-0" />
}

function MetricCard({ icon: Icon, label, value, subValue, color, progress }: {
  icon: any
  label: string
  value: string | number
  subValue?: string
  color: string
  progress?: number
}) {
  return (
    <div className="bg-zinc-800/50 rounded-lg p-4 border border-zinc-800">
      <div className="flex items-center gap-2 mb-2">
        <Icon className={`w-4 h-4 ${color}`} />
        <span className="text-xs text-zinc-500">{label}</span>
      </div>
      <div className="flex items-baseline gap-2">
        <span className="text-xl font-bold text-zinc-100">{value}</span>
        {subValue && <span className="text-xs text-zinc-500">{subValue}</span>}
      </div>
      {progress !== undefined && (
        <div className="mt-2">
          <ProgressBar value={progress} color={color.includes('emerald') ? 'emerald' : color.includes('blue') ? 'blue' : color.includes('amber') ? 'amber' : 'cyan'} />
        </div>
      )}
    </div>
  )
}

function RealtimeMetricsPanel({ server }: { server: ServerDetail }) {
  const [metrics, setMetrics] = useState(server.metrics)
  const [timeRange, setTimeRange] = useState<'1m' | '5m' | '15m' | '1h'>('5m')
  const [cpuData] = useState<number[]>(generateSparklineData())
  const [memData] = useState<number[]>(generateSparklineData())
  const [netInData] = useState<number[]>(generateSparklineData())
  const [netOutData] = useState<number[]>(generateSparklineData())

  useEffect(() => {
    setMetrics(server.metrics)
  }, [server.metrics])

  // TODO: 当 agent 上报 metrics 后，通过 SSE/WebSocket 推送真实数据更新 cpuData/memData/netInData/netOutData
  // 当前仅展示 server.metrics（来自后端），不再用 Math.random() 模拟

  const renderLine = (data: number[], color: string) => {
    const max = Math.max(...data, 1)
    const points = data.map((val, i) => {
      const x = (i / (data.length - 1)) * 100
      const y = 100 - (val / max) * 100
      return `${x},${y}`
    }).join(' ')

    return (
      <svg className="w-full h-20" viewBox="0 0 100 100" preserveAspectRatio="none">
        <polyline
          fill="none"
          stroke={color}
          strokeWidth="1.5"
          points={points}
          className="transition-all duration-300"
        />
        <polygon
          fill={color}
          fillOpacity="0.1"
          points={`0,100 ${points} 100,100`}
        />
      </svg>
    )
  }

  if (server.status === 'offline') {
    return (
      <Card className="bg-zinc-900 border-zinc-800">
        <CardHeader className="pb-3">
          <CardTitle className="text-base flex items-center gap-2">
            <Activity className="w-4 h-4 text-red-400" />
            实时指标
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex flex-col items-center justify-center py-12 text-zinc-500">
            <XCircle className="w-12 h-12 mb-3 text-red-500/50" />
            <p>服务器离线，无法获取实时指标</p>
            <p className="text-xs mt-1">最后心跳: {formatHeartbeat(server.lastHeartbeatAt)}</p>
          </div>
        </CardContent>
      </Card>
    )
  }

  return (
    <Card className="bg-zinc-900 border-zinc-800">
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <CardTitle className="text-base flex items-center gap-2">
            <Activity className="w-4 h-4 text-emerald-400" />
            实时指标
            <span className="flex items-center gap-1 text-xs font-normal text-emerald-400">
              <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse" />
              实时更新中
            </span>
          </CardTitle>
          <div className="flex gap-1">
            {(['1m', '5m', '15m', '1h'] as const).map((range) => (
              <button
                key={range}
                onClick={() => setTimeRange(range)}
                className={`px-2 py-1 text-xs rounded-md transition-colors ${
                  timeRange === range
                    ? 'bg-indigo-600 text-white'
                    : 'bg-zinc-800 text-zinc-400 hover:bg-zinc-700'
                }`}
              >
                {range}
              </button>
            ))}
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
          <MetricCard
            icon={Cpu}
            label="CPU"
            value={`${Math.round(metrics.cpuPercent)}%`}
            color="text-emerald-400"
            progress={metrics.cpuPercent}
          />
          <MetricCard
            icon={HardDrive}
            label="内存"
            value={`${Math.round(metrics.memoryPercent)}%`}
            subValue={`${metrics.memoryUsedMB}MB`}
            color="text-blue-400"
            progress={metrics.memoryPercent}
          />
          <MetricCard
            icon={Activity}
            label="在线用户"
            value={metrics.onlineUsers}
            subValue="人"
            color="text-violet-400"
          />
          <MetricCard
            icon={Zap}
            label="运行时间"
            value={formatUptime(metrics.uptimeSeconds)}
            color="text-amber-400"
          />
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4 pt-2">
          <div className="bg-zinc-800/30 rounded-lg p-3">
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs text-zinc-500 flex items-center gap-1">
                <span className="w-2 h-2 rounded-full bg-emerald-500" /> CPU 使用率
              </span>
              <span className="text-xs text-emerald-400 font-mono">{Math.round(metrics.cpuPercent)}%</span>
            </div>
            {renderLine(cpuData, '#10b981')}
          </div>
          <div className="bg-zinc-800/30 rounded-lg p-3">
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs text-zinc-500 flex items-center gap-1">
                <span className="w-2 h-2 rounded-full bg-blue-500" /> 内存使用率
              </span>
              <span className="text-xs text-blue-400 font-mono">{Math.round(metrics.memoryPercent)}%</span>
            </div>
            {renderLine(memData, '#3b82f6')}
          </div>
          <div className="bg-zinc-800/30 rounded-lg p-3">
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs text-zinc-500 flex items-center gap-1">
                <Download className="w-3 h-3 text-cyan-400" /> 入站流量
              </span>
              <span className="text-xs text-cyan-400 font-mono">{Math.round(metrics.networkInKBps)} KB/s</span>
            </div>
            {renderLine(netInData, '#06b6d4')}
          </div>
          <div className="bg-zinc-800/30 rounded-lg p-3">
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs text-zinc-500 flex items-center gap-1">
                <Upload className="w-3 h-3 text-violet-400" /> 出站流量
              </span>
              <span className="text-xs text-violet-400 font-mono">{Math.round(metrics.networkOutKBps)} KB/s</span>
            </div>
            {renderLine(netOutData, '#8b5cf6')}
          </div>
        </div>

        <div className="grid grid-cols-2 md:grid-cols-4 gap-3 pt-2 border-t border-zinc-800">
          <div className="text-center">
            <div className="text-xs text-zinc-500 mb-1">磁盘使用</div>
            <div className="text-sm font-semibold text-zinc-200">
              {metrics.diskUsedGB}GB <span className="text-zinc-500">/ {metrics.diskTotalGB}GB</span>
            </div>
            <div className="mt-1">
              <ProgressBar value={metrics.diskPercent} color="amber" />
            </div>
          </div>
          <div className="text-center">
            <div className="text-xs text-zinc-500 mb-1">入站速率</div>
            <div className="text-sm font-semibold text-cyan-400 flex items-center justify-center gap-1">
              <Download className="w-3 h-3" />
              {Math.round(metrics.networkInKBps)} KB/s
            </div>
          </div>
          <div className="text-center">
            <div className="text-xs text-zinc-500 mb-1">出站速率</div>
            <div className="text-sm font-semibold text-violet-400 flex items-center justify-center gap-1">
              <Upload className="w-3 h-3" />
              {Math.round(metrics.networkOutKBps)} KB/s
            </div>
          </div>
          <div className="text-center">
            <div className="text-xs text-zinc-500 mb-1">最后心跳</div>
            <div className="text-sm font-semibold text-zinc-200">{formatHeartbeat(server.lastHeartbeatAt)}</div>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

function RealtimeLogs({ serverId }: { serverId: string }) {
  const [autoScroll, setAutoScroll] = useState(true)
  const [logs, setLogs] = useState<{timestamp: string; level: string; source: string; message: string}[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fetchLogs = useCallback(async () => {
    try {
      const resp = await api.get<{logs: {timestamp: string; level: string; source: string; message: string; labels?: Record<string,string>}[]; total: number}>(
        EP.SERVER_LOGS(serverId), { params: { limit: 100 } }
      )
      setLogs(resp.logs || [])
      setError(null)
    } catch (e) {
      setError(e instanceof ApiError ? e.message : '加载日志失败')
    } finally {
      setLoading(false)
    }
  }, [serverId])

  useEffect(() => {
    fetchLogs()
    // 每 5 秒刷新一次日志（轮询）
    const interval = setInterval(fetchLogs, 5000)
    return () => clearInterval(interval)
  }, [fetchLogs])

  const getLogColor = (level: string) => {
    const upper = level.toUpperCase()
    if (upper === 'ERROR' || upper === 'FATAL') return 'text-red-400'
    if (upper === 'WARN' || upper === 'WARNING') return 'text-amber-400'
    if (upper === 'DEBUG') return 'text-zinc-500'
    return 'text-zinc-400'
  }

  const formatLogLine = (log: {timestamp: string; level: string; source: string; message: string}) => {
    const level = log.level.toUpperCase().padEnd(5)
    const source = log.source ? ` [${log.source}]` : ''
    return `[${log.timestamp}] ${level}${source} ${log.message}`
  }

  return (
    <Card className="bg-zinc-900 border-zinc-800">
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <CardTitle className="text-base flex items-center gap-2">
            <FileText className="w-4 h-4 text-zinc-400" />
            实时日志
            <span className="flex items-center gap-1 text-xs font-normal text-emerald-400">
              <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse" />
              实时
            </span>
          </CardTitle>
          <div className="flex items-center gap-4">
            <div className="flex items-center gap-2">
              <span className="text-xs text-zinc-500">自动滚动</span>
              <Switch checked={autoScroll} onChange={(e) => setAutoScroll(e.target.checked)} />
            </div>
            <Button variant="ghost" size="sm" onClick={fetchLogs} className="text-indigo-400 hover:text-indigo-300 h-7 px-2">
              <RefreshCw className="w-3 h-3 mr-1" /> 刷新
            </Button>
          </div>
        </div>
      </CardHeader>
      <CardContent>
        <div className="bg-black/50 rounded-lg p-3 h-52 overflow-auto font-mono text-xs" ref={(el) => {
          if (el && autoScroll) {
            el.scrollTop = el.scrollHeight
          }
        }}>
          {loading ? (
            <div className="text-zinc-500">加载中...</div>
          ) : error ? (
            <div className="text-red-400">{error}</div>
          ) : logs.length === 0 ? (
            <div className="text-zinc-500">暂无日志（agent 未连接或未上报日志）</div>
          ) : (
            logs.map((log, i) => (
              <div key={i} className={getLogColor(log.level)}>
                {formatLogLine(log)}
              </div>
            ))
          )}
        </div>
      </CardContent>
    </Card>
  )
}

function ServerDetailView({ server, onBack }: { server: ServerDetail; onBack: () => void }) {
  const navigate = useNavigate()
  const { toast } = useToast()
  const [showToken, setShowToken] = useState(false)
  const [copied, setCopied] = useState<'token' | 'install' | null>(null)
  const [currentServer, setCurrentServer] = useState(server)

  useEffect(() => {
    setCurrentServer(server)
  }, [server])

  const copyToClipboard = useCallback(async (text: string, type: 'token' | 'install') => {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(type)
      toast({
        title: type === 'token' ? 'Token已复制到剪贴板' : '安装命令已复制到剪贴板',
        variant: 'success',
      })
      setTimeout(() => setCopied(null), 2000)
    } catch {
      toast({ title: '复制失败', variant: 'destructive' })
    }
  }, [toast])

  const resetToken = useCallback(async () => {
    try {
      const resp = await api.get<{ agent_token?: string; install_cmd?: string } & ServerDetailResponseItem>(
        EP.SERVER_TOKEN(currentServer.id)
      )
      const newToken = resp.agent_token || ''
      const newInstallCmd = resp.install_cmd || ''
      setCurrentServer((prev) => ({
        ...prev,
        agentToken: newToken,
        installCmd: newInstallCmd,
      }))
      toast({ title: 'Token 已从后端重新获取', variant: 'success' })
    } catch (e) {
      toast({
        title: '获取 Token 失败',
        description: e instanceof ApiError ? e.message : '未知错误',
        variant: 'destructive',
      })
    }
  }, [currentServer.id, toast])

  const restartKernel = (kernelName: string) => {
    setCurrentServer((prev) => ({
      ...prev,
      kernels: prev.kernels.map((k) =>
        k.name === kernelName ? { ...k, status: 'restarting' as KernelStatus } : k
      ),
    }))
    toast({ title: `正在重启 ${kernelName}...`, variant: 'default' })
    setTimeout(() => {
      setCurrentServer((prev) => ({
        ...prev,
        kernels: prev.kernels.map((k) =>
          k.name === kernelName
            ? { ...k, status: 'running' as KernelStatus, restartCount: k.restartCount + 1 }
            : k
        ),
      }))
      toast({ title: `${kernelName} 重启成功`, variant: 'success' })
    }, 3000)
  }

  const maskedToken = currentServer.agentToken.slice(0, 12) + '••••••••••••••••••••' + currentServer.agentToken.slice(-4)

  return (
    <div className="space-y-6">
      <Button variant="ghost" onClick={onBack} className="text-zinc-400 hover:text-zinc-200 -ml-2">
        <ChevronLeft className="w-4 h-4 mr-1" />
        返回服务器列表
      </Button>

      <Card className="bg-zinc-900 border-zinc-800">
        <CardContent className="p-6">
          <div className="flex items-start justify-between">
            <div className="flex items-center gap-4">
              <div className="p-3 rounded-xl bg-zinc-800">
                <Server className="w-8 h-8 text-indigo-400" />
              </div>
              <div>
                <div className="flex items-center gap-3 flex-wrap">
                  <h1 className="text-xl font-bold text-zinc-100">{currentServer.name}</h1>
                  <Badge variant="outline" className="bg-indigo-900/30 text-indigo-300 border-indigo-800/50 font-mono text-xs">
                    SID: {currentServer.code}
                  </Badge>
                  <Badge variant="outline" className={`border ${getStatusBadgeClass(currentServer.status)}`}>
                    <StatusDot status={currentServer.status} />
                    <span className="ml-1.5">{getStatusText(currentServer.status)}</span>
                  </Badge>
                  {currentServer.region && (
                    <Badge variant="secondary" className="bg-zinc-800 text-zinc-400 text-xs">
                      {currentServer.region}
                    </Badge>
                  )}
                </div>
                <div className="flex items-center gap-4 text-sm text-zinc-500 mt-1">
                  <span className="font-mono">{currentServer.ipv4}</span>
                  {currentServer.ipv6 && <span className="font-mono text-xs">{currentServer.ipv6}</span>}
                  <span>角色: {currentServer.role}</span>
                  <span>节点数: {currentServer.nodeCount}</span>
                </div>
              </div>
            </div>
            <div className="flex gap-2">
              <Button
                size="sm"
                className="bg-indigo-600 hover:bg-indigo-500"
                onClick={() => navigate(`/nodes?server_id=${currentServer.id}&action=create`)}
              >
                <Plus className="w-4 h-4 mr-1" />
                新增节点
              </Button>
              <Button
                size="sm"
                variant="outline"
                className="border-zinc-700 text-zinc-300 hover:bg-zinc-800"
                onClick={() => navigate(`/nodes?server_id=${currentServer.id}`)}
              >
                节点管理
                <ArrowRight className="w-4 h-4 ml-1" />
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      <RealtimeMetricsPanel server={currentServer} />

      <Card className="bg-zinc-900 border-zinc-800">
        <CardHeader className="pb-3">
          <CardTitle className="text-base flex items-center gap-2">
            <Shield className="w-4 h-4 text-violet-400" />
            内核运行状态
          </CardTitle>
          <CardDescription className="text-zinc-500">
            双进程常驻（Xray + Sing-box），协议按路由规则分发到对应内核
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {currentServer.kernels.map((kernel) => (
              <div key={kernel.name} className="bg-zinc-800/50 rounded-lg p-4 border border-zinc-800">
                <div className="flex items-center justify-between mb-3">
                  <div className="flex items-center gap-2">
                    <KernelStatusDot status={kernel.status} />
                    <span className="font-semibold text-zinc-100">{kernel.name}</span>
                    <span className="text-sm text-zinc-500">
                      {kernel.status === 'running' ? '运行中' : kernel.status === 'restarting' ? '重启中' : '已停止'}
                    </span>
                  </div>
                </div>
                <div className="grid grid-cols-4 gap-2 text-sm mb-3">
                  <div>
                    <div className="text-zinc-500 text-xs">版本</div>
                    <div className="text-zinc-300 font-mono text-xs">{kernel.version}</div>
                  </div>
                  <div>
                    <div className="text-zinc-500 text-xs">API端口</div>
                    <div className="text-zinc-300 font-mono text-xs">{kernel.apiPort}</div>
                  </div>
                  <div>
                    <div className="text-zinc-500 text-xs">重启</div>
                    <div className="text-zinc-300">{kernel.restartCount}次</div>
                  </div>
                  <div>
                    <div className="text-zinc-500 text-xs">内存</div>
                    <div className="text-zinc-300">{kernel.memoryMB}MB</div>
                  </div>
                </div>
                <div className="flex gap-2">
                  <Button
                    variant="ghost"
                    size="sm"
                    className="flex-1 h-8 text-zinc-400 hover:text-zinc-200 hover:bg-zinc-700"
                  >
                    <FileText className="w-3 h-3 mr-1" />
                    查看日志
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="flex-1 h-8 text-zinc-400 hover:text-zinc-200 hover:bg-zinc-700"
                    onClick={() => restartKernel(kernel.name)}
                    disabled={kernel.status === 'restarting'}
                  >
                    <RefreshCw className={`w-3 h-3 mr-1 ${kernel.status === 'restarting' ? 'animate-spin' : ''}`} />
                    {kernel.status === 'restarting' ? '重启中' : '重启'}
                  </Button>
                </div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <Card className="bg-gradient-to-br from-violet-900/50 to-indigo-900/50 border-violet-500/30">
          <CardContent className="p-6">
            <button
              onClick={() => navigate(`/diagnostics/ai?server=${currentServer.id}`)}
              className="w-full text-left"
            >
              <div className="flex items-start gap-4">
                <div className="p-3 rounded-xl bg-violet-600/30">
                  <Zap className="w-8 h-8 text-violet-300" />
                </div>
                <div className="flex-1">
                  <h3 className="text-lg font-bold text-zinc-100 mb-1">🧠 AI 智能诊断</h3>
                  <p className="text-sm text-zinc-400 mb-3">
                    自动分析Agent日志，定位配置/网络/证书/内核兼容问题
                  </p>
                  {currentServer.lastDiagnosis && (
                    <div className="bg-black/20 rounded-lg p-3">
                      <div className="flex items-center gap-2 mb-1">
                        <Badge variant="destructive" className="bg-red-900/50 text-red-300 text-xs">
                          {currentServer.lastDiagnosis.category}
                        </Badge>
                        <span className="text-xs text-zinc-500">上次诊断结果</span>
                      </div>
                      <p className="text-xs text-zinc-400">{currentServer.lastDiagnosis.suggestion}</p>
                    </div>
                  )}
                </div>
                <ArrowRight className="w-5 h-5 text-violet-400 mt-2" />
              </div>
            </button>
          </CardContent>
        </Card>

        <Card className="bg-zinc-900 border-zinc-800">
          <CardHeader className="pb-3">
            <CardTitle className="text-base flex items-center gap-2">
              <CheckCircle className="w-4 h-4 text-emerald-400" />
              部署前置检查
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-3">
              {currentServer.preflightChecks.map((check, i) => (
                <div key={i} className="flex items-center gap-3">
                  <CheckIcon status={check.status} />
                  <div className="flex-1">
                    <span
                      className={`text-sm ${
                        check.status === 'pass'
                          ? 'text-zinc-200'
                          : check.status === 'warn'
                          ? 'text-amber-200'
                          : 'text-red-200'
                      }`}
                    >
                      {check.name}
                    </span>
                    {check.description && (
                      <span className="text-xs text-zinc-500 ml-2">({check.description})</span>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <Card className="bg-zinc-900 border-zinc-800">
          <CardHeader className="pb-3">
            <CardTitle className="text-base flex items-center gap-2">
              <Key className="w-4 h-4 text-amber-400" />
              服务器 Token
            </CardTitle>
            <CardDescription className="text-zinc-500">
              此Token用于node-agent向面板认证，请妥善保管
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="bg-zinc-950 rounded-lg p-3 font-mono text-sm mb-3 flex items-center justify-between gap-2">
              <span className="text-zinc-300 truncate">
                {showToken ? currentServer.agentToken : maskedToken}
              </span>
              <Button
                variant="ghost"
                size="sm"
                className="h-7 w-7 p-0 text-zinc-500 hover:text-zinc-300 hover:bg-zinc-800 flex-shrink-0"
                onClick={() => copyToClipboard(currentServer.agentToken, 'token')}
              >
                {copied === 'token' ? <Check className="w-4 h-4 text-emerald-400" /> : <Copy className="w-4 h-4" />}
              </Button>
            </div>
            <div className="flex gap-2">
              <Button
                variant="outline"
                size="sm"
                className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 flex-1"
                onClick={() => setShowToken(!showToken)}
              >
                {showToken ? (
                  <><EyeOff className="w-4 h-4 mr-1" /> 隐藏Token</>
                ) : (
                  <><Eye className="w-4 h-4 mr-1" /> 查看Token</>
                )}
              </Button>
              <Button
                variant="outline"
                size="sm"
                className="border-zinc-700 text-zinc-300 hover:bg-zinc-800 flex-1"
                onClick={resetToken}
              >
                <RefreshCw className="w-4 h-4 mr-1" />
                重置Token
              </Button>
            </div>
          </CardContent>
        </Card>

        <Card className="bg-zinc-900 border-zinc-800">
          <CardHeader className="pb-3">
            <CardTitle className="text-base flex items-center gap-2">
              <Terminal className="w-4 h-4 text-cyan-400" />
              一键安装命令
            </CardTitle>
            <CardDescription className="text-zinc-500">
              在目标服务器上执行此命令，即可自动接入面板
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="bg-zinc-950 rounded-lg p-3 mb-3 relative group">
              <pre className="font-mono text-xs text-zinc-300 overflow-x-auto whitespace-pre-wrap break-all pr-8">
                {currentServer.installCmd}
              </pre>
              <Button
                size="sm"
                className={`absolute top-2 right-2 h-7 w-7 p-0 ${
                  copied === 'install'
                    ? 'bg-emerald-600 hover:bg-emerald-500'
                    : 'bg-cyan-600 hover:bg-cyan-500 opacity-0 group-hover:opacity-100 transition-opacity'
                }`}
                onClick={() => copyToClipboard(currentServer.installCmd, 'install')}
              >
                {copied === 'install' ? <Check className="w-4 h-4" /> : <Copy className="w-4 h-4" />}
              </Button>
            </div>
            <Button
              size="sm"
              className="bg-cyan-600 hover:bg-cyan-500 w-full"
              onClick={() => copyToClipboard(currentServer.installCmd, 'install')}
            >
              {copied === 'install' ? (
                <><Check className="w-4 h-4 mr-1" /> 已复制!</>
              ) : (
                <><Copy className="w-4 h-4 mr-1" /> 复制安装命令</>
              )}
            </Button>
          </CardContent>
        </Card>
      </div>

      <RealtimeLogs serverId={currentServer.id} />

      <Card className="bg-zinc-900 border-zinc-800">
        <CardHeader className="pb-3">
          <CardTitle className="text-base flex items-center gap-2">
            <Server className="w-4 h-4 text-indigo-400" />
            关联节点
            <Badge variant="secondary" className="bg-zinc-800 text-zinc-400 ml-2">
              {currentServer.nodes.length}个节点 · {currentServer.nodes.filter(n => n.status === 'healthy').length}个正常
            </Badge>
          </CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {currentServer.nodes.length === 0 ? (
            <div className="p-8 text-center">
              <EmptyState title="暂无关联节点" description="添加节点到此服务器开始使用" />
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow className="border-zinc-800 hover:bg-transparent">
                  <TableHead className="text-zinc-400 text-xs font-medium">节点ID</TableHead>
                  <TableHead className="text-zinc-400 text-xs font-medium">节点名称</TableHead>
                  <TableHead className="text-zinc-400 text-xs font-medium">协议</TableHead>
                  <TableHead className="text-zinc-400 text-xs font-medium">双端口</TableHead>
                  <TableHead className="text-zinc-400 text-xs font-medium">地址</TableHead>
                  <TableHead className="text-zinc-400 text-xs font-medium">状态</TableHead>
                  <TableHead className="text-zinc-400 text-xs font-medium w-20">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {currentServer.nodes.map((node) => (
                  <TableRow key={node.id} className="border-zinc-800 hover:bg-zinc-800/50">
                    <TableCell className="py-3">
                      <span className="font-mono text-xs text-zinc-500">{node.code}</span>
                    </TableCell>
                    <TableCell className="py-3">
                      <span className="text-sm text-zinc-200 font-medium">{node.name}</span>
                    </TableCell>
                    <TableCell className="py-3">
                      <Badge variant="outline" className="bg-indigo-900/30 text-indigo-300 border-indigo-800/50 text-xs">
                        {node.protocol}
                      </Badge>
                    </TableCell>
                    <TableCell className="py-3">
                      <div className="flex items-center gap-2">
                        <Badge variant="secondary" className="bg-zinc-800 text-zinc-300 font-mono text-xs">
                          :{node.port}
                        </Badge>
                        {node.apiPort && (
                          <>
                            <span className="text-zinc-600">/</span>
                            <Badge variant="secondary" className="bg-zinc-800/50 text-zinc-500 font-mono text-xs">
                              api:{node.apiPort}
                            </Badge>
                          </>
                        )}
                      </div>
                    </TableCell>
                    <TableCell className="py-3">
                      <span className="text-sm text-zinc-400 font-mono">{node.address}</span>
                    </TableCell>
                    <TableCell className="py-3">
                      <Badge variant="outline" className={`border ${getNodeStatusBadgeClass(node.status)}`}>
                        <NodeStatusDot status={node.status} />
                        <span className="ml-1.5">{getNodeStatusText(node.status)}</span>
                      </Badge>
                    </TableCell>
                    <TableCell className="py-3">
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-8 text-indigo-400 hover:text-indigo-300 hover:bg-indigo-950/30 px-2"
                      >
                        管理
                        <ArrowRight className="w-3 h-3 ml-1" />
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  )
}

// 添加服务器对话框
interface AddServerFormData {
  code: string
  name: string
  host: string
  ipv4: string
  ssh_port: string
  provider: string
  role: ServerRole
}

const DEFAULT_ADD_SERVER_FORM: AddServerFormData = {
  code: '',
  name: '',
  host: '',
  ipv4: '',
  ssh_port: '22',
  provider: '',
  role: 'node',
}

function AddServerDialog({
  open,
  onOpenChange,
  onSuccess,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess?: () => void
}) {
  const { toast } = useToast()
  const [form, setForm] = useState<AddServerFormData>({ ...DEFAULT_ADD_SERVER_FORM })
  const [errors, setErrors] = useState<Partial<Record<keyof AddServerFormData, string>>>({})
  const [submitting, setSubmitting] = useState(false)

  function updateField<K extends keyof AddServerFormData>(key: K, value: AddServerFormData[K]) {
    setForm((prev) => ({ ...prev, [key]: value }))
    if (errors[key]) {
      setErrors((prev) => ({ ...prev, [key]: undefined }))
    }
  }

  function validate(): boolean {
    const newErrors: Partial<Record<keyof AddServerFormData, string>> = {}
    if (!form.code.trim()) {
      newErrors.code = '服务器代号必填'
    } else if (!/^[a-zA-Z0-9]{2,64}$/.test(form.code.trim())) {
      newErrors.code = '代号只能含字母数字，2-64 字符'
    }
    if (!form.name.trim()) newErrors.name = '服务器名称必填'
    if (!form.host.trim()) newErrors.host = '主机地址必填（IP 或域名）'
    if (form.ssh_port && (parseInt(form.ssh_port, 10) < 1 || parseInt(form.ssh_port, 10) > 65535)) {
      newErrors.ssh_port = 'SSH 端口范围 1-65535'
    }
    setErrors(newErrors)
    return Object.keys(newErrors).length === 0
  }

  async function handleSubmit() {
    if (!validate()) return
    setSubmitting(true)
    try {
      const payload: Record<string, unknown> = {
        code: form.code.trim(),
        name: form.name.trim(),
        host: form.host.trim(),
        role: form.role,
      }
      if (form.ipv4.trim()) payload.ipv4 = form.ipv4.trim()
      if (form.ssh_port) payload.ssh_port = parseInt(form.ssh_port, 10)
      if (form.provider.trim()) payload.provider = form.provider.trim()

      await api.post(EP.SERVERS, payload)
      toast({ title: '创建成功', description: `服务器 ${form.name} 已添加`, variant: 'success' })
      setForm({ ...DEFAULT_ADD_SERVER_FORM })
      setErrors({})
      onOpenChange(false)
      onSuccess?.()
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '创建服务器失败'
      toast({ title: '创建失败', description: msg, variant: 'destructive' })
    } finally {
      setSubmitting(false)
    }
  }

  function handleOpenChange(next: boolean) {
    if (!next) {
      setForm({ ...DEFAULT_ADD_SERVER_FORM })
      setErrors({})
    }
    onOpenChange(next)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="bg-zinc-900 border-zinc-800 max-w-lg">
        <DialogHeader>
          <DialogTitle className="text-zinc-100">添加服务器</DialogTitle>
          <DialogDescription className="text-zinc-500">
            填写服务器基本信息，创建后会生成 agent token 和一键安装命令
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="srv-code" className="text-zinc-300">服务器代号 *</Label>
              <Input
                id="srv-code"
                value={form.code}
                onChange={(e) => updateField('code', e.target.value)}
                placeholder="如 VPS206"
                className="bg-zinc-950 border-zinc-700 text-zinc-100"
              />
              {errors.code && <p className="text-xs text-red-400">{errors.code}</p>}
            </div>
            <div className="space-y-2">
              <Label htmlFor="srv-name" className="text-zinc-300">服务器名称 *</Label>
              <Input
                id="srv-name"
                value={form.name}
                onChange={(e) => updateField('name', e.target.value)}
                placeholder="如 东京节点服务器"
                className="bg-zinc-950 border-zinc-700 text-zinc-100"
              />
              {errors.name && <p className="text-xs text-red-400">{errors.name}</p>}
            </div>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="srv-host" className="text-zinc-300">主机地址 *</Label>
              <Input
                id="srv-host"
                value={form.host}
                onChange={(e) => updateField('host', e.target.value)}
                placeholder="IP 或域名"
                className="bg-zinc-950 border-zinc-700 text-zinc-100"
              />
              {errors.host && <p className="text-xs text-red-400">{errors.host}</p>}
            </div>
            <div className="space-y-2">
              <Label htmlFor="srv-ipv4" className="text-zinc-300">IPv4（可选）</Label>
              <Input
                id="srv-ipv4"
                value={form.ipv4}
                onChange={(e) => updateField('ipv4', e.target.value)}
                placeholder="留空则从 host 推断"
                className="bg-zinc-950 border-zinc-700 text-zinc-100"
              />
            </div>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="srv-ssh" className="text-zinc-300">SSH 端口</Label>
              <Input
                id="srv-ssh"
                value={form.ssh_port}
                onChange={(e) => updateField('ssh_port', e.target.value)}
                placeholder="22"
                className="bg-zinc-950 border-zinc-700 text-zinc-100"
              />
              {errors.ssh_port && <p className="text-xs text-red-400">{errors.ssh_port}</p>}
            </div>
            <div className="space-y-2">
              <Label htmlFor="srv-provider" className="text-zinc-300">云厂商（可选）</Label>
              <Input
                id="srv-provider"
                value={form.provider}
                onChange={(e) => updateField('provider', e.target.value)}
                placeholder="如 Oracle/Tencent/Contabo"
                className="bg-zinc-950 border-zinc-700 text-zinc-100"
              />
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="srv-role" className="text-zinc-300">角色</Label>
            <Select
              id="srv-role"
              value={form.role}
              onChange={(e) => updateField('role', e.target.value as ServerRole)}
              className="bg-zinc-950 border-zinc-700 text-zinc-100"
            >
              <option value="node">node（节点服务器）</option>
              <option value="edge">edge（边缘入口）</option>
              <option value="relay">relay（中继）</option>
              <option value="balancer">balancer（负载均衡）</option>
            </Select>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => handleOpenChange(false)} disabled={submitting}>
            取消
          </Button>
          <Button onClick={handleSubmit} disabled={submitting} className="bg-indigo-600 hover:bg-indigo-500">
            {submitting ? '创建中...' : '创建服务器'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function ServerListView({
  servers,
  loading,
  onViewDetail,
  onAddServer,
}: {
  servers: ServerDetail[]
  loading: boolean
  onViewDetail: (serverId: string) => void
  onAddServer: () => void
}) {
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold text-zinc-100">服务器管理</h2>
          <p className="text-sm text-zinc-500 mt-1">管理和监控所有节点服务器</p>
        </div>
        <Button size="sm" className="bg-indigo-600 hover:bg-indigo-500" onClick={onAddServer}>
          <Plus className="w-4 h-4 mr-1" />
          添加服务器
        </Button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        <Card className="bg-zinc-900 border-zinc-800">
          <CardContent className="p-4">
            <div className="text-2xl font-bold text-emerald-400">{servers.filter(s => s.status === 'active').length}</div>
            <div className="text-xs text-zinc-500 mt-1">在线服务器</div>
          </CardContent>
        </Card>
        <Card className="bg-zinc-900 border-zinc-800">
          <CardContent className="p-4">
            <div className="text-2xl font-bold text-red-400">{servers.filter(s => s.status === 'offline').length}</div>
            <div className="text-xs text-zinc-500 mt-1">离线服务器</div>
          </CardContent>
        </Card>
        <Card className="bg-zinc-900 border-zinc-800">
          <CardContent className="p-4">
            <div className="text-2xl font-bold text-indigo-400">{servers.reduce((sum, s) => sum + s.nodeCount, 0)}</div>
            <div className="text-xs text-zinc-500 mt-1">总节点数</div>
          </CardContent>
        </Card>
        <Card className="bg-zinc-900 border-zinc-800">
          <CardContent className="p-4">
            <div className="text-2xl font-bold text-violet-400">
              {servers.reduce((sum, s) => sum + s.metrics.onlineUsers, 0)}
            </div>
            <div className="text-xs text-zinc-500 mt-1">当前在线用户</div>
          </CardContent>
        </Card>
      </div>

      <Card className="bg-zinc-900 border-zinc-800 overflow-hidden">
        <CardContent className="p-0">
          {loading ? (
            <div className="p-4 space-y-3">
              {[1, 2, 3].map((i) => (
                <Skeleton key={i} className="h-16 w-full bg-zinc-800 rounded-lg" />
              ))}
            </div>
          ) : servers.length === 0 ? (
            <EmptyState title="暂无服务器" description="添加服务器来管理您的节点" className="py-12" />
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="border-zinc-800 hover:bg-transparent">
                    <TableHead className="text-zinc-400 text-xs font-medium">服务器</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium">SID</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium">状态</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium">IP地址</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium">节点</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium hidden md:table-cell">负载</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium">心跳</TableHead>
                    <TableHead className="text-zinc-400 text-xs font-medium w-20">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {servers.map((server) => (
                    <TableRow key={server.id} className="border-zinc-800 hover:bg-zinc-800/50 cursor-pointer" onClick={() => onViewDetail(server.id)}>
                      <TableCell className="py-3">
                        <div className="flex items-center gap-2">
                          <div className="p-1.5 rounded-md bg-zinc-800">
                            <Server className="w-4 h-4 text-zinc-400" />
                          </div>
                          <div>
                            <div className="font-medium text-zinc-200 text-sm">{server.name}</div>
                            <div className="text-xs text-zinc-500">{server.region} · {server.provider}</div>
                          </div>
                        </div>
                      </TableCell>
                      <TableCell className="py-3">
                        <span className="text-sm text-indigo-400 font-mono">{server.code}</span>
                      </TableCell>
                      <TableCell className="py-3">
                        <Badge variant="outline" className={`border ${getStatusBadgeClass(server.status)}`}>
                          <StatusDot status={server.status} />
                          <span className="ml-1.5">{getStatusText(server.status)}</span>
                        </Badge>
                      </TableCell>
                      <TableCell className="py-3">
                        <span className="text-sm text-zinc-300 font-mono">{server.ipv4}</span>
                      </TableCell>
                      <TableCell className="py-3">
                        <Badge variant="secondary" className="bg-zinc-800 text-zinc-300">
                          {server.nodeCount}
                        </Badge>
                      </TableCell>
                      <TableCell className="py-3 hidden md:table-cell">
                        <div className="space-y-1 w-36">
                          <div className="flex items-center gap-2">
                            <span className="text-xs text-zinc-500 w-8">CPU</span>
                            <div className="flex-1">
                              <ProgressBar value={server.metrics.cpuPercent} color="emerald" />
                            </div>
                            <span className="text-xs text-zinc-500 w-8 text-right">{Math.round(server.metrics.cpuPercent)}%</span>
                          </div>
                          <div className="flex items-center gap-2">
                            <span className="text-xs text-zinc-500 w-8">MEM</span>
                            <div className="flex-1">
                              <ProgressBar value={server.metrics.memoryPercent} color="blue" />
                            </div>
                            <span className="text-xs text-zinc-500 w-8 text-right">{Math.round(server.metrics.memoryPercent)}%</span>
                          </div>
                        </div>
                      </TableCell>
                      <TableCell className="py-3 text-sm text-zinc-400">{formatHeartbeat(server.lastHeartbeatAt)}</TableCell>
                      <TableCell className="py-3">
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-8 text-indigo-400 hover:text-indigo-300 hover:bg-indigo-950/30"
                          onClick={(e) => { e.stopPropagation(); onViewDetail(server.id); }}
                        >
                          详情
                          <ArrowRight className="w-3 h-3 ml-1" />
                        </Button>
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

export default function Servers() {
  const [loading, setLoading] = useState(true)
  const [view, setView] = useState<'list' | 'detail'>('list')
  const [selectedServerId, setSelectedServerId] = useState<string>('')
  const [servers, setServers] = useState<ServerDetail[]>([])
  const [addDialogOpen, setAddDialogOpen] = useState(false)
  const { toast } = useToast()

  const loadServers = useCallback(async () => {
    setLoading(true)
    try {
      const data = await api.get<unknown>(EP.SERVERS, {
        params: { page: 1, page_size: 100 },
      })
      const list = normalizeList<ServerResponseItem>(data)
      setServers(list.map(mapServerResponse))
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '加载服务器列表失败'
      toast({ title: '加载失败', description: msg, variant: 'destructive' })
      setServers([])
    } finally {
      setLoading(false)
    }
  }, [toast])

  useEffect(() => {
    loadServers()
  }, [loadServers])

  // 进入详情视图时加载 agent_token、install_cmd 和关联节点（后端 GET /admin/servers/:id 返回 ServerDetailResponse）
  const loadServerDetail = useCallback(async (serverId: string) => {
    if (!serverId) return
    try {
      const data = await api.get<ServerDetailResponseItem>(EP.SERVER_DETAIL(serverId))
      setServers((prev) =>
        prev.map((s) => {
          if (s.id !== serverId) return s
          // 重新映射完整响应（含 nodes、runtimes、metrics 等详情字段）
          const remapped = mapServerResponse(data)
          return {
            ...s,
            ...remapped,
            agentToken: data.agent_token || '',
            installCmd: data.install_cmd || '',
          }
        }),
      )
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '加载服务器详情失败'
      toast({ title: '详情加载失败', description: msg, variant: 'destructive' })
    }
  }, [toast])

  useEffect(() => {
    if (view === 'detail' && selectedServerId) {
      loadServerDetail(selectedServerId)
    }
  }, [view, selectedServerId, loadServerDetail])

  const selectedServer = servers.find((s) => s.id === selectedServerId)

  const handleViewDetail = (serverId: string) => {
    setSelectedServerId(serverId)
    setView('detail')
  }

  const handleBack = () => {
    setView('list')
    setSelectedServerId('')
  }

  return (
    <div className="space-y-4">
      {view === 'list' ? (
        <ServerListView
          servers={servers}
          loading={loading}
          onViewDetail={handleViewDetail}
          onAddServer={() => setAddDialogOpen(true)}
        />
      ) : selectedServer ? (
        <ServerDetailView server={selectedServer} onBack={handleBack} />
      ) : null}
      <AddServerDialog
        open={addDialogOpen}
        onOpenChange={setAddDialogOpen}
        onSuccess={loadServers}
      />
    </div>
  )
}
