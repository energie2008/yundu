import { create } from 'zustand'
import { api } from './api'

const TOKEN_KEY = 'airport_admin_token'
const REFRESH_TOKEN_KEY = 'airport_admin_refresh_token'

export interface Admin {
  id: string
  email: string
  name: string
  role: string
  is_admin: boolean
}

interface AuthState {
  token: string | null
  admin: Admin | null
  isAuthenticated: boolean
  isLoading: boolean
  login: (email: string, password: string) => Promise<void>
  logout: () => Promise<void>
  fetchMe: () => Promise<void>
  init: () => void
}

export const useAuthStore = create<AuthState>((set, get) => ({
  token: null,
  admin: null,
  isAuthenticated: false,
  isLoading: true,

  init: () => {
    const storedToken = localStorage.getItem(TOKEN_KEY)
    if (storedToken) {
      set({ token: storedToken, isAuthenticated: true, isLoading: false })
    } else {
      set({ isLoading: false })
    }
  },

  login: async (email: string, password: string) => {
    const response = await fetch('/api/v1/admin/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, password }),
    })

    const data = await response.json()

    if (!response.ok || data.code !== 0) {
      const message = data?.message || '登录失败'
      throw new Error(message)
    }

    const { token: accessToken, admin } = data.data
    if (!accessToken?.access_token) {
      throw new Error('登录响应缺少token')
    }

    const cleanToken = accessToken.access_token.startsWith('Bearer ') ? accessToken.access_token.substring(7) : accessToken.access_token
    localStorage.setItem(TOKEN_KEY, cleanToken)
    if (accessToken.refresh_token) {
      localStorage.setItem(REFRESH_TOKEN_KEY, accessToken.refresh_token)
    }

    const adminInfo: Admin = {
      id: admin?.id || '1',
      email: admin?.email || email,
      name: admin?.display_name || email.split('@')[0],
      role: admin?.is_super_admin ? 'super_admin' : 'admin',
      is_admin: true,
    }

    set({ token: cleanToken, admin: adminInfo, isAuthenticated: true })
  },

  logout: async () => {
    localStorage.removeItem(TOKEN_KEY)
    localStorage.removeItem(REFRESH_TOKEN_KEY)
    set({ token: null, admin: null, isAuthenticated: false })
  },

  fetchMe: async () => {
    const { token } = get()
    if (!token) return
    try {
      const me = await api.get<any>('/admin/me')
      const adminData = me?.admin || me?.data?.admin || me
      if (adminData && adminData.id) {
        set({
          admin: {
            id: adminData.id,
            email: adminData.email || '',
            name: adminData.display_name || adminData.name || adminData.email?.split('@')[0] || 'Admin',
            role: adminData.is_super_admin ? 'super_admin' : (adminData.role || 'admin'),
            is_admin: true,
          },
        })
      }
    } catch {
      // Keep existing admin info
    }
  },
}))
