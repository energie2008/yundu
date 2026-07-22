import { TOKEN_KEYS, EP } from './endpoints'

interface ApiOptions extends RequestInit {
  params?: Record<string, string | number | boolean | undefined>
}

class ApiError extends Error {
  status: number
  data: unknown

  constructor(message: string, status: number, data?: unknown) {
    super(message)
    this.status = status
    this.data = data
  }
}

function getAccessToken(): string | null {
  return localStorage.getItem(TOKEN_KEYS.ACCESS)
}

function setTokens(access: string, refresh: string = '') {
  localStorage.setItem(TOKEN_KEYS.ACCESS, access)
  if (refresh) {
    localStorage.setItem(TOKEN_KEYS.REFRESH, refresh)
  }
}

function clearTokens() {
  localStorage.removeItem(TOKEN_KEYS.ACCESS)
  localStorage.removeItem(TOKEN_KEYS.REFRESH)
}

function redirectToLogin() {
  clearTokens()
  const returnUrl = encodeURIComponent(window.location.pathname + window.location.search)
  if (window.location.pathname !== '/login' && window.location.pathname !== '/register') {
    window.location.href = `/login?returnUrl=${returnUrl}`
  }
}

let isRefreshing = false
let refreshPromise: Promise<string | null> | null = null

async function tryRefreshToken(): Promise<string | null> {
  if (isRefreshing && refreshPromise) {
    return refreshPromise
  }
  const refreshToken = localStorage.getItem(TOKEN_KEYS.REFRESH)
  if (!refreshToken) return null

  isRefreshing = true
  refreshPromise = (async () => {
    try {
      const resp = await fetch('/api/v1/auth/refresh', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refresh_token: refreshToken }),
      })
      if (!resp.ok) return null
      const body = await resp.json()
      const tokenData = body?.data?.token || body?.token
      if (!tokenData?.access_token) return null
      localStorage.setItem(TOKEN_KEYS.ACCESS, tokenData.access_token)
      if (tokenData.refresh_token) {
        localStorage.setItem(TOKEN_KEYS.REFRESH, tokenData.refresh_token)
      }
      return tokenData.access_token
    } catch {
      return null
    } finally {
      isRefreshing = false
      refreshPromise = null
    }
  })()

  return refreshPromise
}

async function request<T>(url: string, options: ApiOptions = {}): Promise<T> {
  const { params, headers: customHeaders, ...restOptions } = options

  let fullUrl = url
  if (params) {
    const searchParams = new URLSearchParams()
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined) {
        searchParams.append(key, String(value))
      }
    })
    const queryString = searchParams.toString()
    if (queryString) {
      fullUrl += `?${queryString}`
    }
  }

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(customHeaders as Record<string, string>),
  }
  const token = getAccessToken()
  if (token) {
    headers['Authorization'] = token.startsWith('Bearer ') ? token : `Bearer ${token}`
  }

  let response: Response = await fetch(fullUrl, { ...restOptions, headers })

  if (response.status === 401) {
    const newToken = await tryRefreshToken()
    if (newToken) {
      headers['Authorization'] = newToken.startsWith('Bearer ') ? newToken : `Bearer ${newToken}`
      const retryResponse = await fetch(fullUrl, { ...restOptions, headers })
      if (retryResponse.status !== 401) {
        response = retryResponse
      } else {
        redirectToLogin()
        throw new ApiError('未授权，请重新登录', 401)
      }
    } else {
      redirectToLogin()
      throw new ApiError('未授权，请重新登录', 401)
    }
  }

  let data: unknown = null
  const contentType = response.headers.get('content-type')
  if (contentType && contentType.includes('application/json')) {
    data = await response.json()
  } else {
    data = await response.text()
  }

  if (!response.ok) {
    const body = data as { code?: number; message?: string; error?: string; msg?: string }
    const message = body?.message || body?.error || body?.msg || `请求失败: ${response.status}`
    throw new ApiError(message, response.status, data)
  }

  const body = data as { code?: number; data?: T; message?: string; status?: string }
  if (body && typeof body === 'object') {
    if ('status' in body && body.status === 'fail') {
      throw new ApiError(body.message || '请求失败', response.status, data)
    }
    if ('code' in body && body.code !== undefined && body.code !== 0) {
      throw new ApiError(body.message || `请求失败(code=${body.code})`, response.status, data)
    }
    if ('data' in body) {
      return body.data as T
    }
  }

  return data as T
}

export const api = {
  get: <T>(url: string, options?: Omit<ApiOptions, 'method' | 'body'>) =>
    request<T>(url, { ...options, method: 'GET' }),

  post: <T>(url: string, body?: unknown, options?: Omit<ApiOptions, 'method' | 'body'>) =>
    request<T>(url, {
      ...options,
      method: 'POST',
      body: body ? JSON.stringify(body) : undefined,
    }),

  put: <T>(url: string, body?: unknown, options?: Omit<ApiOptions, 'method' | 'body'>) =>
    request<T>(url, {
      ...options,
      method: 'PUT',
      body: body ? JSON.stringify(body) : undefined,
    }),

  patch: <T>(url: string, body?: unknown, options?: Omit<ApiOptions, 'method' | 'body'>) =>
    request<T>(url, {
      ...options,
      method: 'PATCH',
      body: body ? JSON.stringify(body) : undefined,
    }),

  delete: <T>(url: string, options?: Omit<ApiOptions, 'method' | 'body'>) =>
    request<T>(url, { ...options, method: 'DELETE' }),

  getGuestConfig: <T>() => request<T>(EP.GUEST_CONFIG, { method: 'GET' }),
}

export { ApiError, setTokens, clearTokens, redirectToLogin }
