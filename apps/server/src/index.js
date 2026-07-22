import dotenv from 'dotenv'
import { fileURLToPath } from 'url'
import { dirname, resolve } from 'path'
import express from 'express'
import cors from 'cors'
import morgan from 'morgan'
import crypto from 'crypto'
import { createProxyMiddleware } from 'http-proxy-middleware'

// 优先加载仓库根目录的 .env（apps/server/src/index.js 向上 3 级到 air/）
// dotenv 不覆盖已存在的环境变量，所以系统环境变量优先级最高
const __dirname = dirname(fileURLToPath(import.meta.url))
dotenv.config({ path: resolve(__dirname, '../../../.env') })
dotenv.config()

const LOCAL_NO_PROXY = '127.0.0.1,localhost,::1,127.0.0.0/8,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16'
const existingNoProxy = process.env.NO_PROXY || process.env.no_proxy || ''
process.env.NO_PROXY = existingNoProxy ? `${existingNoProxy},${LOCAL_NO_PROXY}` : LOCAL_NO_PROXY
process.env.no_proxy = process.env.NO_PROXY

const app = express()
const PORT = process.env.PORT || 8080
const XBOARD_URL = process.env.XBOARD_URL || 'http://127.0.0.1:7001'
const XB_ADMIN_PATH = process.env.XB_ADMIN_PATH || 'adminifanr520'
const DEEPSEEK_API_KEY = process.env.DEEPSEEK_API_KEY || 'sk-ac9314aaa6994f79924305133f2369bf'
const DEEPSEEK_API_URL = 'https://api.deepseek.com/chat/completions'

let adminAuthCache = { token: null, expiresAt: 0 }

async function getAdminToken() {
  if (adminAuthCache.token && Date.now() < adminAuthCache.expiresAt) {
    return adminAuthCache.token
  }
  const email = process.env.XB_ADMIN_EMAIL || 'a***@example.com'
  const password = process.env.XB_ADMIN_PASSWORD || 'admin12345'
  try {
    const loginRes = await fetch(`${XBOARD_URL}/api/v1/passport/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, password })
    })
    const loginData = await loginRes.json()
    if (loginData.status !== 'success' || !loginData.data?.is_admin) {
      adminAuthCache = { token: null, expiresAt: 0 }
      throw new Error(`XBoard admin login failed: ${loginData.message || 'unknown error'}`)
    }
    const rawToken = loginData.data.auth_data || loginData.data.token
    const token = rawToken.replace(/^Bearer\s+/i, '')
    adminAuthCache = { token, expiresAt: Date.now() + 3600 * 1000 }
    return token
  } catch (err) {
    adminAuthCache = { token: null, expiresAt: 0 }
    throw err
  }
}

app.use(cors({
  origin: ['http://localhost:5177', 'http://localhost:5178', 'http://localhost:5179'],
  credentials: true
}))
app.use(morgan('dev'))

async function ensureAdminAuth(req, res, next) {
  const authHeader = req.headers['authorization']
  if (!authHeader || authHeader === '' || authHeader === 'Bearer null' || authHeader === 'Bearer undefined') {
    try {
      const token = await getAdminToken()
      req.headers['authorization'] = `Bearer ${token}`
    } catch (e) {
      console.error('Failed to get admin token for proxy:', e.message)
    }
  }
  next()
}

const xbAdminProxy = createProxyMiddleware({
  target: XBOARD_URL,
  changeOrigin: true,
  // proxyTimeout: 控制与上游 XBoard 的连接最长存活时间
  // 30s 足够覆盖 XBoard 99% 接口；超过则主动断开返 504（语义清晰）
  proxyTimeout: 30000,
  // timeout: 整个代理请求的最长时间（含响应体传输）
  // 设 60s，留 30s 给响应体传输
  timeout: 60000,
  pathRewrite: (path) => {
    const stripped = path.replace(/^\/api\/v1\/admin/, '')
    return `/api/v2/${XB_ADMIN_PATH}${stripped || '/'}`
  },
  onError: (err, req, res) => {
    // 区分错误类型，给前端清晰的可观测信号
    if (!res.headersSent) {
      const isConnRefused = err.code === 'ECONNREFUSED' || err.code === 'ECONNRESET'
      const isTimeout = err.code === 'ETIMEDOUT' || err.message?.includes('timeout')
      const status = isConnRefused ? 502 : (isTimeout ? 504 : 502)
      const reason = isConnRefused ? '上游服务不可达（SSH 隧道可能断了）'
                    : isTimeout ? '上游响应超时'
                    : '上游代理错误'
      res.writeHead(status, { 'Content-Type': 'application/json' })
      res.end(JSON.stringify({
        status: 'fail',
        code: status,
        message: reason,
        upstream: XBOARD_URL,
        hint: status === 502 ? '请检查 SSH 隧道是否在运行' : undefined
      }))
    }
  }
})

// ===== node-service 代理（P0）=====
// node-service 是新的节点管理微服务（端口 8082），拥有协议预设、节点 CRUD、
// 双核渲染器、部署编排、AI 诊断等能力。Express 作为前端唯一入口，将 node-service
// 专有路由代理到 8082，其余 admin 路由仍走 XBoard(7001)。
//
// 鉴权说明：node-service 使用独立的 JWT（由 JWT_SECRET 签发，与 XBoard token 不兼容）。
// Express 作为可信网关，为 admin 路由生成内部 super_admin JWT 并注入 Authorization 头。
// 前端不需要任何改动，仍发送 XBoard token，Express 会替换为 node-service token。
const NODE_SERVICE_URL = process.env.NODE_SERVICE_URL || 'http://127.0.0.1:8082'
const NODE_SERVICE_JWT_SECRET = process.env.JWT_SECRET || ''

let nodeServiceTokenCache = { token: null, expiresAt: 0 }

function generateNodeServiceToken() {
  if (!NODE_SERVICE_JWT_SECRET) {
    throw new Error('JWT_SECRET 未配置，无法生成 node-service token')
  }
  // node-service 的 Claims 结构体字段：
  //   user_id / token_type / session_id / is_admin / permissions / admin_id
  // ValidateAdminToken 要求：token_type="access" 且 is_admin=true
  // RBAC RequirePermission 要求：permissions 包含对应 code 或 "*"
  // admin_id 不能为 uuid.Nil（否则 RequireSuperAdmin/RequirePermission 拒绝）
  const header = { alg: 'HS256', typ: 'JWT' }
  const now = Math.floor(Date.now() / 1000)
  const payload = {
    user_id: '00000000-0000-0000-0000-000000000001',
    token_type: 'access',
    session_id: '00000000-0000-0000-0000-000000000002',
    is_admin: true,
    permissions: ['*'],
    admin_id: '00000000-0000-0000-0000-000000000001',
    iat: now,
    exp: now + 3600,
    sub: 'express-gateway-internal'
  }
  const encHeader = Buffer.from(JSON.stringify(header)).toString('base64url')
  const encPayload = Buffer.from(JSON.stringify(payload)).toString('base64url')
  const signingInput = `${encHeader}.${encPayload}`
  const signature = crypto.createHmac('sha256', NODE_SERVICE_JWT_SECRET).update(signingInput).digest('base64url')
  return `${signingInput}.${signature}`
}

function getNodeServiceToken() {
  // 提前 5 分钟刷新，避免请求途中过期
  if (nodeServiceTokenCache.token && Date.now() < nodeServiceTokenCache.expiresAt - 300000) {
    return nodeServiceTokenCache.token
  }
  const token = generateNodeServiceToken()
  nodeServiceTokenCache = { token, expiresAt: Date.now() + 3600 * 1000 }
  return token
}

const nodeServiceProxy = createProxyMiddleware({
  target: NODE_SERVICE_URL,
  changeOrigin: true,
  proxyTimeout: 30000,
  timeout: 60000,
  // pathFilter 决定哪些请求走 node-service 代理（不匹配的放行到下一个中间件）
  // 注意：不能用 app.use(path, proxy) 挂载，因为 Express 会剥掉 mount path，
  // 导致 node-service 收到 path=/ 返回 404。pathFilter 在原始 url 上匹配。
  pathFilter: (pathname, req) => {
    // public 路由
    if (pathname === '/api/v1/public/protocol-presets' || pathname.startsWith('/api/v1/public/protocol-presets/')) {
      return true
    }
    // admin 路由
    for (const route of nodeServiceAdminRoutes) {
      if (pathname === route || pathname.startsWith(route + '/')) {
        return true
      }
    }
    return false
  },
  onError: (err, req, res) => {
    if (!res.headersSent) {
      const isConnRefused = err.code === 'ECONNREFUSED' || err.code === 'ECONNRESET'
      const isTimeout = err.code === 'ETIMEDOUT' || err.message?.includes('timeout')
      const status = isConnRefused ? 502 : (isTimeout ? 504 : 502)
      const reason = isConnRefused ? 'node-service 不可达（未启动？）'
                    : isTimeout ? 'node-service 响应超时'
                    : 'node-service 代理错误'
      res.writeHead(status, { 'Content-Type': 'application/json' })
      res.end(JSON.stringify({
        status: 'fail',
        code: status,
        message: reason,
        upstream: NODE_SERVICE_URL,
        hint: status === 502 ? '请确认 node-service 在 8082 端口运行' : undefined
      }))
    }
  }
})

// node-service 专有路由（必须在 XBoard admin 代理之前注册，否则会被 /api/v1/admin/* 吞掉）
// 路由前缀来自 node-service/internal 各 handler 的 admin.Group(...) 注册
const nodeServiceAdminRoutes = [
  '/api/v1/admin/protocol-presets',
  '/api/v1/admin/protocol-registry',
  '/api/v1/admin/config-templates',
  '/api/v1/admin/nodes',
  '/api/v1/admin/servers',
  '/api/v1/admin/deployments',
  '/api/v1/admin/proxy-chains',
  '/api/v1/admin/health',
  '/api/v1/admin/tls-certificates',
  '/api/v1/admin/tls-profiles',
  '/api/v1/admin/config-import',
  '/api/v1/admin/runtime-upgrades',
  '/api/v1/admin/warp-profiles',
  '/api/v1/admin/channels',
  '/api/v1/admin/route-rule-sets',
  '/api/v1/admin/route-policies',
  '/api/v1/admin/route-policy-rules',
  '/api/v1/admin/node-groups',
  '/api/v1/admin/diagnosis',
  '/api/v1/admin/experience',
]

// public 路由无需认证，admin 路由注入内部 super_admin JWT
// 注意：不传 path 给 app.use（pathFilter 已在 proxy 内部匹配），
// 否则 Express 会剥掉 mount path 导致 node-service 收到 path=/
app.use((req, res, next) => {
  // 判断是否是 admin 路由（需要注入 JWT）
  const isAdminRoute = nodeServiceAdminRoutes.some(r => req.path === r || req.path.startsWith(r + '/'))
  if (isAdminRoute) {
    try {
      const token = getNodeServiceToken()
      req.headers['authorization'] = `Bearer ${token}`
    } catch (e) {
      console.error('[node-service] 生成内部 token 失败:', e.message)
    }
  }
  // 交给 nodeServiceProxy（pathFilter 会决定是否代理，不匹配则 next）
  nodeServiceProxy(req, res, next)
})

// ===== XBoard admin 代理（node-service 未匹配的 admin 路由走这里）=====
app.use('/api/v1/admin', ensureAdminAuth, xbAdminProxy)

const xboardUserProxy = createProxyMiddleware({
  target: XBOARD_URL,
  changeOrigin: true,
  ws: true,
  proxyTimeout: 30000,
  timeout: 60000,
  pathFilter: (pathname) => {
    if (pathname.startsWith('/api/v1/yundu/')) return false
    if (pathname.startsWith('/api/v1/admin')) return false
    if (pathname.startsWith('/api/v1/user/auth/')) return false
    if (pathname.startsWith('/api/v1/user/me')) return false
    if (pathname.startsWith('/api/v1/user/subscription')) return false
    if (pathname.startsWith('/api/v1/me/')) return false
    if (pathname === '/api/v1/plans' || pathname.startsWith('/api/v1/plans/')) return false
    if (pathname.startsWith('/api/v1/user/orders')) return false
    if (pathname.startsWith('/api/v1/me/tickets')) return false
    if (pathname.startsWith('/api/v1/user/tickets')) return false
    if (pathname.startsWith('/api/v1/me/notifications')) return false
    if (pathname.startsWith('/api/v1/user/preferences')) return false
    if (pathname.startsWith('/api/v1/user/commissions')) return false
    if (pathname.startsWith('/api/v1/guest/')) return false
    if (pathname === '/api/v1/health') return false
    return pathname.startsWith('/api') || pathname.startsWith('/sub') || pathname.startsWith('/link')
  },
  onError: (err, req, res) => {
    if (!res.headersSent) {
      const isConnRefused = err.code === 'ECONNREFUSED' || err.code === 'ECONNRESET'
      const isTimeout = err.code === 'ETIMEDOUT' || err.message?.includes('timeout')
      const status = isConnRefused ? 502 : (isTimeout ? 504 : 502)
      const reason = isConnRefused ? '上游服务不可达（SSH 隧道可能断了）'
                    : isTimeout ? '上游响应超时'
                    : '上游代理错误'
      res.writeHead(status, { 'Content-Type': 'application/json' })
      res.end(JSON.stringify({
        status: 'fail',
        code: status,
        message: reason,
        upstream: XBOARD_URL,
        hint: status === 502 ? '请检查 SSH 隧道是否在运行' : undefined
      }))
    }
  }
})

app.use(xboardUserProxy)

app.use(express.json({ limit: '50mb' }))
app.use(express.urlencoded({ extended: true, limit: '50mb' }))

app.post('/api/v1/yundu/admin/login', async (req, res) => {
  try {
    const { email, password } = req.body || {}
    if (!email || !password) {
      return res.status(400).json({ status: 'fail', message: '邮箱和密码不能为空' })
    }
    const loginRes = await fetch(`${XBOARD_URL}/api/v1/passport/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, password })
    })
    const loginData = await loginRes.json()
    if (loginData.status !== 'success' || !loginData.data) {
      return res.status(401).json({ status: 'fail', message: loginData.message || '登录失败' })
    }
    if (!loginData.data.is_admin) {
      return res.status(403).json({ status: 'fail', message: '该账号不是管理员' })
    }
    const rawToken = loginData.data.auth_data || loginData.data.token
    const cleanToken = rawToken.replace(/^Bearer\s+/i, '')
    adminAuthCache = { token: cleanToken, expiresAt: Date.now() + 3600 * 1000 }
    return res.json({
      status: 'success',
      data: {
        token: cleanToken,
        auth_data: `Bearer ${cleanToken}`,
        is_admin: true,
        email,
        admin_path: XB_ADMIN_PATH
      }
    })
  } catch (err) {
    console.error('Admin login error:', err.message)
    res.status(500).json({ status: 'error', message: '登录服务异常' })
  }
})

app.post('/api/v1/yundu/ai/chat', async (req, res) => {
  try {
    const { messages, model = 'deepseek-chat' } = req.body || {}
    if (!messages || !Array.isArray(messages) || messages.length === 0) {
      return res.status(400).json({ status: 'fail', message: 'messages参数不能为空' })
    }
    const systemPrompt = {
      role: 'system',
      content: '你是Yundu云渡机场管理系统的AI助手，精通Xray/Sing-box/V2Board/XBoard配置，擅长节点配置、故障诊断、协议优化、网络分析。回答请简洁专业，代码使用yaml/json格式。'
    }
    const aiRes = await fetch(DEEPSEEK_API_URL, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${DEEPSEEK_API_KEY}`
      },
      body: JSON.stringify({ model, messages: [systemPrompt, ...messages], stream: false, temperature: 0.7, max_tokens: 2048 })
    })
    if (!aiRes.ok) {
      const errText = await aiRes.text()
      console.error('DeepSeek API error:', aiRes.status, errText)
      return res.status(502).json({ status: 'fail', message: 'AI服务请求失败' })
    }
    const aiData = await aiRes.json()
    const reply = aiData.choices?.[0]?.message?.content || ''
    res.json({ status: 'success', data: { reply, model, usage: aiData.usage } })
  } catch (err) {
    console.error('AI chat error:', err.message)
    res.status(500).json({ status: 'error', message: 'AI服务异常: ' + err.message })
  }
})

app.post('/api/v1/yundu/ai/diagnose', async (req, res) => {
  try {
    const { node_config, error_log, protocol } = req.body || {}
    const prompt = `请诊断以下节点配置问题：\n协议: ${protocol || 'unknown'}\n配置: ${JSON.stringify(node_config || {})}\n错误日志: ${error_log || '无'}\n请给出问题分析和修复建议，格式为JSON: {issues:[{severity,description,fix}],recommended_config:{...}}`
    const aiRes = await fetch(DEEPSEEK_API_URL, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${DEEPSEEK_API_KEY}` },
      body: JSON.stringify({
        model: 'deepseek-chat',
        messages: [
          { role: 'system', content: '你是Yundu云渡机场管理系统的AI节点诊断专家。只返回有效JSON，不要其他解释文字。' },
          { role: 'user', content: prompt }
        ],
        stream: false, temperature: 0.3, max_tokens: 2048
      })
    })
    const aiData = await aiRes.json()
    const reply = aiData.choices?.[0]?.message?.content || '{}'
    let parsed = {}
    try { parsed = JSON.parse(reply.replace(/```json|```/g, '').trim()) } catch {}
    res.json({ status: 'success', data: parsed })
  } catch (err) {
    res.status(500).json({ status: 'error', message: 'AI诊断失败: ' + err.message })
  }
})

app.get('/api/v1/yundu/dashboard', async (req, res) => {
  try {
    const token = await getAdminToken()
    const headers = { 'Authorization': `Bearer ${token}` }
    const [statRes, nodesRes, plansRes] = await Promise.all([
      fetch(`${XBOARD_URL}/api/v2/${XB_ADMIN_PATH}/stat/getOverride`, { headers }),
      fetch(`${XBOARD_URL}/api/v2/${XB_ADMIN_PATH}/server/manage/getNodes`, { headers }),
      fetch(`${XBOARD_URL}/api/v2/${XB_ADMIN_PATH}/plan/fetch`, { headers })
    ])
    const statData = await statRes.json()
    const nodesData = await nodesRes.json()
    const plansData = await plansRes.json()
    const nodes = Array.isArray(nodesData.data) ? nodesData.data : []
    const onlineCount = nodes.filter(n => n.show === 1 || n.show === true).length
    res.json({
      status: 'success',
      data: {
        stats: statData.data || {},
        nodes_total: nodes.length,
        nodes_online: onlineCount,
        plans: Array.isArray(plansData.data) ? plansData.data : [],
        servers: [{ id: 'vps1', name: 'VPS-190', host: '43.135.147.190', status: 'online' }],
      }
    })
  } catch (err) {
    console.error('Dashboard fetch error:', err.message)
    res.status(500).json({ status: 'error', message: '获取面板数据失败' })
  }
})

app.get('/api/v1/yundu/servers', async (req, res) => {
  try {
    const token = await getAdminToken()
    const headers = { 'Authorization': `Bearer ${token}` }
    const [machinesRes, groupsRes] = await Promise.all([
      fetch(`${XBOARD_URL}/api/v2/${XB_ADMIN_PATH}/server/machine/fetch`, { headers }),
      fetch(`${XBOARD_URL}/api/v2/${XB_ADMIN_PATH}/server/group/fetch`, { headers })
    ])
    const machinesData = await machinesRes.json()
    const groupsData = await groupsRes.json()
    res.json({
      status: 'success',
      data: {
        machines: Array.isArray(machinesData.data) ? machinesData.data : [],
        groups: Array.isArray(groupsData.data) ? groupsData.data : []
      }
    })
  } catch (err) {
    console.error('Servers fetch error:', err.message)
    res.status(500).json({ status: 'error', message: '获取服务器列表失败' })
  }
})

app.get('/health', (req, res) => {
  res.json({ status: 'ok', time: new Date().toISOString(), version: '0.2.0', xboard_url: XBOARD_URL })
})

// 上游健康检测：前端 / dev-start-all.ps1 用来判断 SSH 隧道是否真的通
// 真实探测 XBoard，3s 超时，返回详细状态而非简单的 TCP 端口检测
app.get('/health/upstream', async (req, res) => {
  const startedAt = Date.now()
  try {
    const r = await fetch(`${XBOARD_URL}/`, {
      method: 'GET',
      signal: AbortSignal.timeout(3000)
    })
    const elapsed = Date.now() - startedAt
    const bodyText = await r.text()
    const isXboardPage = bodyText.includes('云渡 YunDu') || bodyText.includes('Xboard')
    res.json({
      status: r.ok ? 'ok' : 'fail',
      upstream_reachable: r.ok,
      upstream_status_code: r.status,
      upstream_is_xboard: isXboardPage,
      elapsed_ms: elapsed,
      xboard_url: XBOARD_URL,
      hint: r.ok && isXboardPage ? 'SSH 隧道正常，XBoard 可达' :
            r.ok ? '上游可达但内容不像是 XBoard 首页' :
            '上游不可达，SSH 隧道可能断了'
    })
  } catch (err) {
    const elapsed = Date.now() - startedAt
    const isConnRefused = err.code === 'ECONNREFUSED' || err.code === 'ECONNRESET'
    const isTimeout = err.name === 'TimeoutError' || err.name === 'AbortError'
    res.status(502).json({
      status: 'fail',
      upstream_reachable: false,
      error_code: err.code || err.name,
      error_message: err.message,
      elapsed_ms: elapsed,
      xboard_url: XBOARD_URL,
      hint: isConnRefused ? 'SSH 隧道断了，请用 dev-start-all.ps1 重启' :
            isTimeout ? '上游响应超时（>3s），VPS 可能负载高或网络差' :
            '未知错误'
    })
  }
})

app.use((err, req, res, next) => {
  console.error(err.stack)
  res.status(500).json({ status: 'error', message: 'Internal Server Error' })
})

const server = app.listen(PORT, () => {
  console.log(`Yundu server running on http://localhost:${PORT}`)
  console.log(`Proxying XBoard user API to ${XBOARD_URL}/api/v1/`)
  console.log(`XBoard admin API: /api/v1/admin/* -> ${XBOARD_URL}/api/v2/${XB_ADMIN_PATH}/*`)
  console.log(`node-service proxy: ${nodeServiceAdminRoutes.length} admin routes + /api/v1/public/protocol-presets -> ${NODE_SERVICE_URL}`)
  console.log(`  JWT_SECRET loaded: ${NODE_SERVICE_JWT_SECRET ? 'yes' : 'NO (admin routes will 401)'}`)
})

server.on('upgrade', xboardUserProxy.upgrade)
