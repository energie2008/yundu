import { create } from 'zustand'
import { api, setTokens, clearTokens } from './api'
import { EP, TOKEN_KEYS, UserResponse, LoginResponse, RegisterResponse, UserDetailResponse, adaptUser } from './endpoints'

export interface User extends UserResponse {}

interface AuthState {
  user: User | null
  accessToken: string | null
  refreshToken: string | null
  isAuthenticated: boolean
  isLoading: boolean
  login: (email: string, password: string) => Promise<void>
  register: (email: string, password: string, inviteCode?: string) => Promise<{ requiresVerification: boolean }>
  logout: () => Promise<void>
  fetchMe: () => Promise<void>
  init: () => void
  forgotPassword: (email: string) => Promise<void>
  resetPassword: (token: string, newPassword: string) => Promise<void>
}

export const useAuthStore = create<AuthState>((set, get) => ({
  user: null,
  accessToken: null,
  refreshToken: null,
  isAuthenticated: false,
  isLoading: true,

  init: () => {
    const accessToken = localStorage.getItem(TOKEN_KEYS.ACCESS)
    const refreshToken = localStorage.getItem(TOKEN_KEYS.REFRESH)
    if (accessToken) {
      set({ accessToken, refreshToken, isAuthenticated: true, isLoading: false })
    } else {
      set({ isLoading: false })
    }
  },

  login: async (email: string, password: string) => {
    const data = await api.post<LoginResponse>(EP.AUTH_LOGIN, { email, password })
    const { token: tokenResp, user: userData } = data
    if (!tokenResp?.access_token) {
      throw new Error('登录响应缺少 access_token')
    }
    setTokens(tokenResp.access_token, tokenResp.refresh_token || '')
    set({
      accessToken: tokenResp.access_token,
      refreshToken: tokenResp.refresh_token || '',
      isAuthenticated: true,
      user: adaptUser({ ...userData, profile: null, subscription: userData.subscription ?? null } as UserDetailResponse),
    })
    await get().fetchMe()
  },

  register: async (email: string, password: string, inviteCode?: string) => {
    const data = await api.post<RegisterResponse>(EP.AUTH_REGISTER, {
      email,
      password,
      invite_code: inviteCode || '',
    })
    if (!data.requires_verification) {
      try {
        await get().login(email, password)
        return { requiresVerification: false }
      } catch (loginErr) {
        return { requiresVerification: false }
      }
    }
    return { requiresVerification: true }
  },

  logout: async () => {
    try {
      await api.post(EP.AUTH_LOGOUT)
    } catch {
    }
    clearTokens()
    set({ accessToken: null, refreshToken: null, user: null, isAuthenticated: false })
  },

  fetchMe: async () => {
    const { accessToken } = get()
    if (!accessToken) return
    try {
      const raw = await api.get<UserDetailResponse>(EP.ME)
      const user = adaptUser(raw)
      set({ user })
    } catch {
      clearTokens()
      set({ accessToken: null, refreshToken: null, user: null, isAuthenticated: false })
    }
  },

  forgotPassword: async (email: string) => {
    await api.post(EP.AUTH_FORGOT_PASSWORD, { email })
  },

  resetPassword: async (token: string, newPassword: string) => {
    await api.post(EP.AUTH_RESET_PASSWORD, {
      token,
      password: newPassword,
      password_confirmation: newPassword,
    })
  },
}))

// Alias for convenience
export const useAuth = useAuthStore
