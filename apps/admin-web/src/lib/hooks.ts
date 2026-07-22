import { useQuery, useMutation, useQueryClient, UseQueryOptions, UseMutationOptions } from '@tanstack/react-query'
import { api } from './api'
import { EP } from './endpoints'
import type { PresetTemplate } from '@/types/preset'

// ===== Types =====

export interface Server {
  id: string
  name: string
  host?: string
  ip?: string
  region?: string
  status?: string
}

export interface Runtime {
  id: string
  name: string
  version?: string
  type?: string
  status?: string
}

export interface Protocol {
  id: string
  name: string
  type: string
  transport?: string
  security?: string
  config_schema?: Record<string, unknown>
}

export interface ProtocolPreset extends PresetTemplate {
  // 以下为后端额外字段（PresetTemplate 之外的）
  code?: string
  protocol_type?: string
  transport_type?: string
  security_type?: string
  is_recommended?: boolean
  is_enabled?: boolean
  is_builtin?: boolean
  sort_order?: number
  recommended_port?: number
  created_at?: string
  updated_at?: string
}

export interface ImportPreviewItem {
  index: number
  name: string
  protocol_type: string
  address: string
  port: number
  valid: boolean
  warning?: string
  raw_uri: string
  parsed: Record<string, unknown>
}

export interface ImportPreviewResult {
  items: ImportPreviewItem[]
  valid_count: number
  invalid_count: number
}

export interface Node {
  id: string
  name: string
  code?: string
  server_id?: string
  server_name?: string
  runtime_id?: string
  address?: string
  protocol_type: string
  transport_type?: string
  security_type?: string
  port: number
  priority?: number
  is_enabled?: boolean
  is_visible?: boolean
  node_type?: string
  region?: string
  group?: string
  multiplier?: number
  status?: string
  last_heartbeat?: string
  last_heartbeat_at?: string
  cpu?: number
  current_cpu_percent?: number
  memory?: number
  current_mem_percent?: number
  bandwidth?: number
  config?: Record<string, unknown>
  listen_host?: string
  enable_udp?: boolean
  created_at?: string
  updated_at?: string
}

export interface ConfigVersion {
  id: string
  version: number
  created_at: string
  is_active?: boolean
  note?: string
}

export interface User {
  id: string
  email: string
  name?: string
  username?: string
  status?: string
  plan?: string
  plan_id?: string
  plan_name?: string
  traffic_used?: number
  traffic_limit?: number
  expires_at?: string
  created_at?: string
  updated_at?: string
  devices_online?: number
  ban_reason?: string
  banned_at?: string
  note?: string
  tags?: string[]
  subscription_token?: string
  subscription_url?: string
}

export interface UserDetail extends User {
  orders?: Order[]
  subscriptions?: Subscription[]
  traffic_logs?: TrafficLog[]
  audit_logs?: AuditLog[]
}

export interface Order {
  id: string
  plan_name: string
  amount: number
  status: string
  created_at: string
}

export interface Subscription {
  id: string
  token: string
  name?: string
  created_at: string
  last_used_at?: string
  is_revoked?: boolean
}

export interface TrafficLog {
  id: string
  date: string
  upload: number
  download: number
  total: number
}

export interface AuditLog {
  id: string
  action: string
  ip?: string
  user_agent?: string
  created_at: string
}

export interface Plan {
  id: string
  name: string
}

// ===== Helper to extract list from response =====

function extractList<T>(resp: unknown): T[] {
  if (Array.isArray(resp)) return resp as T[]
  if (!resp || typeof resp !== 'object') return []
  const obj = resp as Record<string, unknown>
  const dataField = obj.data
  if (dataField && typeof dataField === 'object') {
    if (Array.isArray(dataField)) return dataField as T[]
    const dataObj = dataField as Record<string, unknown>
    if (Array.isArray(dataObj.items)) return dataObj.items as T[]
    if (Array.isArray(dataObj.list)) return dataObj.list as T[]
  }
  if (Array.isArray(obj.items)) return obj.items as T[]
  if (Array.isArray(obj.list)) return obj.list as T[]
  if (Array.isArray(obj.data)) return obj.data as T[]
  return []
}

function extractData<T>(resp: unknown): T | null {
  if (!resp || typeof resp !== 'object') return null
  const obj = resp as Record<string, unknown>
  if (obj.data !== undefined) return obj.data as T
  return resp as T
}

// ===== Query Keys =====

export const queryKeys = {
  nodes: ['nodes'] as const,
  servers: ['servers'] as const,
  runtimes: ['runtimes'] as const,
  protocols: ['protocols'] as const,
  protocolPresets: ['protocol-presets'] as const,
  users: ['users'] as const,
  user: (id: string) => ['users', id] as const,
  node: (id: string) => ['nodes', id] as const,
  nodeConfigVersions: (id: string) => ['nodes', id, 'config-versions'] as const,
  plans: ['plans'] as const,
}

// ===== Server & Runtime queries =====

export function useServers(options?: Partial<UseQueryOptions<Server[]>>) {
  return useQuery({
    queryKey: queryKeys.servers,
    queryFn: async () => {
      const resp = await api.get<unknown>(EP.SERVERS)
      return extractList<Server>(resp)
    },
    ...options,
  })
}

export function useRuntimes(options?: Partial<UseQueryOptions<Runtime[]>>) {
  return useQuery({
    queryKey: queryKeys.runtimes,
    queryFn: async () => {
      const resp = await api.get<unknown>(EP.RUNTIMES)
      return extractList<Runtime>(resp)
    },
    ...options,
  })
}

// ===== Protocol queries =====

export function useProtocols(options?: Partial<UseQueryOptions<Protocol[]>>) {
  return useQuery({
    queryKey: queryKeys.protocols,
    queryFn: async () => {
      const resp = await api.get<unknown>(EP.PROTOCOL_REGISTRY)
      return extractList<Protocol>(resp)
    },
    ...options,
  })
}

export function useProtocolPresets(options?: Partial<UseQueryOptions<ProtocolPreset[]>>) {
  return useQuery({
    queryKey: queryKeys.protocolPresets,
    queryFn: async () => {
      const resp = await api.get<unknown>(EP.PROTOCOL_PRESETS)
      return extractList<ProtocolPreset>(resp)
    },
    ...options,
  })
}

export function useCreatePreset(options?: UseMutationOptions<ProtocolPreset, Error, Record<string, unknown>>) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (data: Record<string, unknown>) => {
      const resp = await api.post<unknown>(EP.PROTOCOL_PRESETS, data)
      return extractData<ProtocolPreset>(resp) as ProtocolPreset
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.protocolPresets })
    },
    ...options,
  })
}

export function useUpdatePreset(options?: UseMutationOptions<ProtocolPreset, Error, { id: string; data: Record<string, unknown> }>) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, data }: { id: string; data: Record<string, unknown> }) => {
      const resp = await api.patch<unknown>(EP.PROTOCOL_PRESET_DETAIL(id), data)
      return extractData<ProtocolPreset>(resp) as ProtocolPreset
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.protocolPresets })
    },
    ...options,
  })
}

export function useDeletePreset(options?: UseMutationOptions<void, Error, string>) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: string) => {
      await api.delete(EP.PROTOCOL_PRESET_DETAIL(id))
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.protocolPresets })
    },
    ...options,
  })
}

// Fork 内置预设为自定义预设（可编辑）
export function useForkPreset(options?: UseMutationOptions<ProtocolPreset, Error, { id: string; data?: { name?: string; code?: string } }>) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, data }: { id: string; data?: { name?: string; code?: string } }) => {
      const resp = await api.post<unknown>(EP.PROTOCOL_PRESET_FORK(id), data || {})
      return extractData<ProtocolPreset>(resp) as ProtocolPreset
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.protocolPresets })
    },
    ...options,
  })
}

// ===== Node queries & mutations =====

export function useNodes(options?: Partial<UseQueryOptions<Node[]>>) {
  return useQuery({
    queryKey: queryKeys.nodes,
    queryFn: async () => {
      const resp = await api.get<unknown>(EP.NODES)
      return extractList<Node>(resp)
    },
    ...options,
  })
}

export function useNode(id: string, options?: Partial<UseQueryOptions<Node | null>>) {
  return useQuery({
    queryKey: queryKeys.node(id),
    queryFn: async () => {
      const resp = await api.get<unknown>(EP.NODE_DETAIL(id))
      return extractData<Node>(resp)
    },
    enabled: !!id,
    ...options,
  })
}

export function useNodeConfigVersions(id: string, options?: Partial<UseQueryOptions<ConfigVersion[]>>) {
  return useQuery({
    queryKey: queryKeys.nodeConfigVersions(id),
    queryFn: async () => {
      const resp = await api.get<unknown>(EP.NODE_CONFIG_VERSIONS(id))
      return extractList<ConfigVersion>(resp)
    },
    enabled: !!id,
    ...options,
  })
}

export function useCreateNode(options?: UseMutationOptions<Node, Error, Record<string, unknown>>) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (data: Record<string, unknown>) => {
      const resp = await api.post<unknown>(EP.NODES, data)
      return extractData<Node>(resp) as Node
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.nodes })
    },
    ...options,
  })
}

export function useImportUriPreview(options?: UseMutationOptions<ImportPreviewResult, Error, { uris: string; server_id?: string; runtime_id?: string; region?: string; multiplier?: number }>) {
  return useMutation({
    mutationFn: async (data: { uris: string; server_id?: string; runtime_id?: string; region?: string; multiplier?: number }) => {
      const resp = await api.post<unknown>(EP.NODES_IMPORT_URI_PREVIEW, data)
      return extractData<ImportPreviewResult>(resp) as ImportPreviewResult
    },
    ...options,
  })
}

export function useImportUriConfirm(options?: UseMutationOptions<{ created: number }, Error, { items: ImportPreviewItem[]; selected_indices: number[]; server_id?: string; runtime_id?: string; region?: string; multiplier?: number }>) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (data: { items: ImportPreviewItem[]; selected_indices: number[]; server_id?: string; runtime_id?: string; region?: string; multiplier?: number }) => {
      const resp = await api.post<unknown>(EP.NODES_IMPORT_URI_CONFIRM, data)
      return extractData<{ created: number }>(resp) as { created: number }
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.nodes })
    },
    ...options,
  })
}

export function useDeployNode(options?: UseMutationOptions<unknown, Error, string>) {
  return useMutation({
    mutationFn: async (id: string) => {
      return api.post<unknown>(EP.NODE_DEPLOY(id))
    },
    ...options,
  })
}

// 发布配置：按 scope 刷新部署配置（scope_type=node + scope_id=节点ID）
export function useRefreshNodeConfig(options?: UseMutationOptions<unknown, Error, { scope_type: string; scope_id: string }>) {
  return useMutation({
    mutationFn: async (payload: { scope_type: string; scope_id: string }) => {
      return api.post<unknown>(EP.DEPLOYMENT_REFRESH, payload)
    },
    ...options,
  })
}

export function useRollbackNode(options?: UseMutationOptions<unknown, Error, { id: string; version?: number }>) {
  return useMutation({
    mutationFn: async ({ id, version }: { id: string; version?: number }) => {
      return api.post<unknown>(EP.NODE_ROLLBACK(id), version ? { version } : undefined)
    },
    ...options,
  })
}

// ===== User queries & mutations =====

export function useUsers(options?: Partial<UseQueryOptions<User[]>>) {
  return useQuery({
    queryKey: queryKeys.users,
    queryFn: async () => {
      const resp = await api.get<unknown>(EP.USERS)
      return extractList<User>(resp)
    },
    ...options,
  })
}

export function useUser(id: string, options?: Partial<UseQueryOptions<UserDetail | null>>) {
  return useQuery({
    queryKey: queryKeys.user(id),
    queryFn: async () => {
      const resp = await api.get<unknown>(EP.USER_DETAIL(id))
      return extractData<UserDetail>(resp)
    },
    enabled: !!id,
    ...options,
  })
}

export function useCreateUser(options?: UseMutationOptions<User, Error, Record<string, unknown>>) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (data: Record<string, unknown>) => {
      const resp = await api.post<unknown>(EP.USERS, data)
      return extractData<User>(resp) as User
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.users })
    },
    ...options,
  })
}

export function useBanUser(options?: UseMutationOptions<unknown, Error, { id: string; reason?: string }>) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, reason }: { id: string; reason?: string }) => {
      return api.post<unknown>(EP.USER_BAN(id), { reason })
    },
    onSuccess: (_, variables) => {
      qc.invalidateQueries({ queryKey: queryKeys.users })
      qc.invalidateQueries({ queryKey: queryKeys.user(variables.id) })
    },
    ...options,
  })
}

export function useUnbanUser(options?: UseMutationOptions<unknown, Error, string>) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: string) => {
      return api.post<unknown>(EP.USER_UNBAN(id))
    },
    onSuccess: (_, id) => {
      qc.invalidateQueries({ queryKey: queryKeys.users })
      qc.invalidateQueries({ queryKey: queryKeys.user(id) })
    },
    ...options,
  })
}

export function useResetPassword(options?: UseMutationOptions<{ password?: string }, Error, string>) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: string) => {
      const resp = await api.post<unknown>(EP.USER_RESET_PASSWORD(id))
      return extractData<{ password?: string }>(resp) as { password?: string }
    },
    onSuccess: (_, id) => {
      qc.invalidateQueries({ queryKey: queryKeys.user(id) })
    },
    ...options,
  })
}

export function useResetTraffic(options?: UseMutationOptions<unknown, Error, string>) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: string) => {
      return api.post<unknown>(EP.USER_RESET_TRAFFIC(id))
    },
    onSuccess: (_, id) => {
      qc.invalidateQueries({ queryKey: queryKeys.users })
      qc.invalidateQueries({ queryKey: queryKeys.user(id) })
    },
    ...options,
  })
}

export function useAddTraffic(options?: UseMutationOptions<unknown, Error, { id: string; amount_gb: number }>) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, amount_gb }: { id: string; amount_gb: number }) => {
      return api.post<unknown>(EP.USER_ADD_TRAFFIC(id), { amount_gb })
    },
    onSuccess: (_, variables) => {
      qc.invalidateQueries({ queryKey: queryKeys.users })
      qc.invalidateQueries({ queryKey: queryKeys.user(variables.id) })
    },
    ...options,
  })
}

export function useExtendUser(options?: UseMutationOptions<unknown, Error, { id: string; days: number }>) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, days }: { id: string; days: number }) => {
      return api.post<unknown>(EP.USER_EXTEND(id), { days })
    },
    onSuccess: (_, variables) => {
      qc.invalidateQueries({ queryKey: queryKeys.users })
      qc.invalidateQueries({ queryKey: queryKeys.user(variables.id) })
    },
    ...options,
  })
}

export function useChangePlan(options?: UseMutationOptions<unknown, Error, { id: string; plan_id: string; immediate?: boolean }>) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, plan_id, immediate }: { id: string; plan_id: string; immediate?: boolean }) => {
      return api.post<unknown>(EP.USER_CHANGE_PLAN(id), { plan_id, immediate })
    },
    onSuccess: (_, variables) => {
      qc.invalidateQueries({ queryKey: queryKeys.users })
      qc.invalidateQueries({ queryKey: queryKeys.user(variables.id) })
    },
    ...options,
  })
}

export function useResetSubscription(options?: UseMutationOptions<{ token?: string }, Error, string>) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: string) => {
      const resp = await api.post<unknown>(EP.USER_RESET_SUB(id))
      return extractData<{ token?: string }>(resp) as { token?: string }
    },
    onSuccess: (_, id) => {
      qc.invalidateQueries({ queryKey: queryKeys.user(id) })
    },
    ...options,
  })
}

export function useImpersonateUser(options?: UseMutationOptions<{ token?: string; url?: string }, Error, string>) {
  return useMutation({
    mutationFn: async (id: string) => {
      const resp = await api.post<unknown>(EP.USER_IMPERSONATE(id))
      return extractData<{ token?: string; url?: string }>(resp) as { token?: string; url?: string }
    },
    ...options,
  })
}

export function useDeleteUser(options?: UseMutationOptions<unknown, Error, string>) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: string) => {
      return api.delete<unknown>(EP.USER_DETAIL(id))
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.users })
    },
    ...options,
  })
}

// ===== Batch operations =====

export function useBatchBanUsers(options?: UseMutationOptions<unknown, Error, { user_ids: string[]; reason?: string }>) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ user_ids, reason }: { user_ids: string[]; reason?: string }) => {
      return api.post<unknown>(EP.USERS_BATCH_BAN, { user_ids, reason })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.users })
    },
    ...options,
  })
}

export function useBatchUnbanUsers(options?: UseMutationOptions<unknown, Error, string[]>) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (user_ids: string[]) => {
      return api.post<unknown>(EP.USERS_BATCH_UNBAN, { user_ids })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.users })
    },
    ...options,
  })
}

export function useBatchResetTraffic(options?: UseMutationOptions<unknown, Error, string[]>) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (user_ids: string[]) => {
      return api.post<unknown>(EP.USERS_BATCH_RESET_TRAFFIC, { user_ids })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.users })
    },
    ...options,
  })
}
