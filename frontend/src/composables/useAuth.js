import { ref, computed } from 'vue'

const TOKEN_KEY = 'agent-chat-auth-token'

export function useAuth(apiBase) {
  const token = ref(localStorage.getItem(TOKEN_KEY) || '')
  const authError = ref('')
  const checking = ref(true)
  const needsAuth = ref(false)

  const isAuthenticated = computed(() => !!token.value)

  function authHeaders(extra = {}) {
    const headers = { ...extra }
    if (token.value) {
      headers['Authorization'] = `Bearer ${token.value}`
    }
    return headers
  }

  async function login(password) {
    authError.value = ''
    try {
      const res = await fetch(`${apiBase}/api/auth/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ password })
      })
      if (!res.ok) {
        const data = await res.json().catch(() => ({}))
        authError.value = data.error || 'Login failed'
        return false
      }
      const data = await res.json()
      token.value = data.token
      localStorage.setItem(TOKEN_KEY, data.token)
      return true
    } catch (err) {
      authError.value = err.message || 'Network error'
      return false
    }
  }

  function logout() {
    token.value = ''
    localStorage.removeItem(TOKEN_KEY)
  }

  // 通过调用受保护端点判断是否需要认证以及 token 是否有效
  async function checkAuth() {
    checking.value = true
    try {
      const res = await fetch(`${apiBase}/api/backends`, {
        headers: authHeaders()
      })
      if (res.status === 401) {
        needsAuth.value = true
        logout()
      } else {
        // 200 = 认证通过或认证未启用
        needsAuth.value = false
      }
    } catch {
      // 网络错误, 保持当前状态
    } finally {
      checking.value = false
    }
  }

  return { token, isAuthenticated, needsAuth, authError, checking, authHeaders, login, logout, checkAuth }
}
