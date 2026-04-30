import { computed, reactive } from 'vue'
import { AUTH_TOKEN_STORAGE_KEY, getCurrentUser, signOut } from './api'

const AUTH_USER_STORAGE_KEY = 'kipup-auth-user'

export const permissionOptions = [
  { value: 'upload', label: 'Upload' },
  { value: 'download', label: 'Download' },
  { value: 'create', label: 'Create bucket' },
  { value: 'delete', label: 'Delete' },
  { value: 'move', label: 'Move' },
  { value: 'rename', label: 'Rename' },
  { value: 'search', label: 'Search' },
  { value: 'cleanup', label: 'Cleanup' },
  { value: 'webhook', label: 'Webhook' },
  { value: 'presign', label: 'Presign' }
]

const state = reactive({
  token: readStoredToken(),
  user: readStoredUser(),
  ready: false,
  loading: false
})

export function useAuth() {
  const currentUser = computed(() => state.user)
  const isAuthenticated = computed(() => Boolean(state.token && state.user))
  const isAdmin = computed(() => state.user?.role === 'admin')
  const hasPermission = (permission) => isAdmin.value || (state.user?.permissions || []).includes(permission)

  return {
    state,
    currentUser,
    isAuthenticated,
    isAdmin,
    hasPermission
  }
}

export function hasStoredToken() {
  return Boolean(readStoredToken())
}

export async function initializeAuth() {
  if (state.loading) return
  if (state.ready && (!state.token || state.user)) return

  state.loading = true
  try {
    if (!state.token) {
      state.user = null
      state.ready = true
      return
    }
    await refreshCurrentUser()
    state.ready = true
  } catch {
    clearAuthSession()
  } finally {
    state.loading = false
  }
}

export async function refreshCurrentUser() {
  if (!state.token) {
    state.user = null
    state.ready = true
    return null
  }
  const { data } = await getCurrentUser()
  state.user = data.user || null
  persistStoredUser(state.user)
  state.ready = true
  return state.user
}

export function setAuthSession(token, user) {
  state.token = token
  state.user = user || null
  state.ready = true
  persistStoredToken(token)
  persistStoredUser(user)
}

export function clearAuthSession() {
  state.token = ''
  state.user = null
  state.ready = true
  persistStoredToken('')
  persistStoredUser(null)
}

export async function signOutSession() {
  try {
    if (state.token) {
      await signOut()
    }
  } finally {
    clearAuthSession()
  }
}

function readStoredToken() {
  if (typeof window === 'undefined') return ''
  return window.localStorage.getItem(AUTH_TOKEN_STORAGE_KEY) || ''
}

function readStoredUser() {
  if (typeof window === 'undefined') return null
  const raw = window.localStorage.getItem(AUTH_USER_STORAGE_KEY)
  if (!raw) return null
  try {
    return JSON.parse(raw)
  } catch {
    return null
  }
}

function persistStoredToken(token) {
  if (typeof window === 'undefined') return
  if (token) window.localStorage.setItem(AUTH_TOKEN_STORAGE_KEY, token)
  else window.localStorage.removeItem(AUTH_TOKEN_STORAGE_KEY)
}

function persistStoredUser(user) {
  if (typeof window === 'undefined') return
  if (user) window.localStorage.setItem(AUTH_USER_STORAGE_KEY, JSON.stringify(user))
  else window.localStorage.removeItem(AUTH_USER_STORAGE_KEY)
}
