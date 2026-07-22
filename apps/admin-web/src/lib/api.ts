const TOKEN_KEY = 'airport_admin_token'
const REFRESH_TOKEN_KEY = 'airport_admin_refresh_token'
const baseURL = '/api/v1'

interface ApiOptions extends RequestInit {
  params?: Record<string, string | number | boolean | undefined | null | string[]>
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

function clearAuthAndRedirect() {
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(REFRESH_TOKEN_KEY)
  if (window.location.pathname !== '/login') {
    window.location.href = '/login'
  }
}

let isRefreshing = false
let refreshPromise: Promise<string | null> | null = null

async function tryRefreshToken(): Promise<string | null> {
  if (isRefreshing && refreshPromise) {
    return refreshPromise
  }
  const refreshToken = localStorage.getItem(REFRESH_TOKEN_KEY)
  if (!refreshToken) return null

  isRefreshing = true
  refreshPromise = (async () => {
    try {
      const resp = await fetch(`${baseURL}/auth/refresh`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refresh_token: refreshToken }),
      })
      if (!resp.ok) return null
      const body = await resp.json()
      const tokenData = body?.data?.token || body?.token
      if (!tokenData?.access_token) return null
      const newAccess = tokenData.access_token.startsWith('Bearer ')
        ? tokenData.access_token.substring(7)
        : tokenData.access_token
      localStorage.setItem(TOKEN_KEY, newAccess)
      if (tokenData.refresh_token) {
        localStorage.setItem(REFRESH_TOKEN_KEY, tokenData.refresh_token)
      }
      return newAccess
    } catch {
      return null
    } finally {
      isRefreshing = false
      refreshPromise = null
    }
  })()

  return refreshPromise
}

async function request<T>(endpoint: string, options: ApiOptions = {}): Promise<T> {
  const { params, headers: customHeaders, ...restOptions } = options
  const token = localStorage.getItem(TOKEN_KEY)

  let url = `${baseURL}${endpoint}`
  if (params) {
    const searchParams = new URLSearchParams()
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined && value !== null) {
        if (Array.isArray(value)) {
          value.forEach(v => searchParams.append(key + '[]', String(v)))
        } else {
          searchParams.append(key, String(value))
        }
      }
    })
    const queryString = searchParams.toString()
    if (queryString) {
      url += `?${queryString}`
    }
  }

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(customHeaders as Record<string, string>),
  }

  if (token) {
    headers['Authorization'] = token.startsWith('Bearer ') ? token : `Bearer ${token}`
  }

  let response = await fetch(url, {
    ...restOptions,
    headers,
  })

  if (response.status === 401) {
    const newToken = await tryRefreshToken()
    if (newToken) {
      headers['Authorization'] = `Bearer ${newToken}`
      const retryResponse = await fetch(url, { ...restOptions, headers })
      if (retryResponse.status !== 401) {
        response = retryResponse
      } else {
        clearAuthAndRedirect()
        throw new ApiError('未授权，请重新登录', 401)
      }
    } else {
      clearAuthAndRedirect()
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
    const message =
      (data as { message?: string })?.message ||
      (data as { error?: string })?.error ||
      `请求失败: ${response.status}`
    throw new ApiError(message, response.status, data)
  }

  const body = data as { code?: number; message?: string; data?: T }
  if (body && typeof body === 'object' && 'code' in body && body.code !== 0) {
    throw new ApiError(body.message || '请求失败', response.status, data)
  }
  if (body && typeof body === 'object' && 'data' in body && body.data !== undefined) {
    return body.data as T
  }

  return data as T
}

export const api = {
  get: <T>(endpoint: string, options?: Omit<ApiOptions, 'method' | 'body'>) =>
    request<T>(endpoint, { ...options, method: 'GET' }),

  post: <T>(endpoint: string, body?: unknown, options?: Omit<ApiOptions, 'method' | 'body'>) =>
    request<T>(endpoint, {
      ...options,
      method: 'POST',
      body: body !== undefined ? JSON.stringify(body) : undefined,
    }),

  put: <T>(endpoint: string, body?: unknown, options?: Omit<ApiOptions, 'method' | 'body'>) =>
    request<T>(endpoint, {
      ...options,
      method: 'PUT',
      body: body ? JSON.stringify(body) : undefined,
    }),

  patch: <T>(endpoint: string, body?: unknown, options?: Omit<ApiOptions, 'method' | 'body'>) =>
    request<T>(endpoint, {
      ...options,
      method: 'PATCH',
      body: body ? JSON.stringify(body) : undefined,
    }),

  delete: <T>(endpoint: string, options?: Omit<ApiOptions, 'method' | 'body'>) =>
    request<T>(endpoint, { ...options, method: 'DELETE' }),
}

export const xbAdminApi = {
  get: <T>(path: string, params?: Record<string, unknown>) => api.get<T>(`/admin${path}`, { params: params as Record<string, string | number | boolean | undefined> }),
  post: <T>(path: string, body?: unknown) => api.post<T>(`/admin${path}`, body),
}

export const yunduApi = {
  get: <T>(path: string, params?: Record<string, unknown>) => api.get<T>(`/yundu${path}`, { params: params as Record<string, string | number | boolean | undefined> }),
  post: <T>(path: string, body?: unknown) => api.post<T>(`/yundu${path}`, body),
}

export { ApiError }
