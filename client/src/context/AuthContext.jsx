import { useCallback, useEffect, useMemo, useState } from 'react'
import { authApi, loadSession, onSessionChange, saveSession } from '../api/client'
import { AuthContext } from './auth'

export function AuthProvider({ children }) {
  const [session, setSession] = useState(loadSession)

  // Keep React state in sync with token refreshes and other tabs.
  useEffect(() => {
    const unsubscribe = onSessionChange(setSession)
    const onStorage = (e) => e.key === 'swatch.auth' && setSession(loadSession())
    window.addEventListener('storage', onStorage)
    return () => {
      unsubscribe()
      window.removeEventListener('storage', onStorage)
    }
  }, [])

  const login = useCallback(async (email, password) => {
    saveSession(await authApi.login(email, password))
  }, [])

  const register = useCallback(async (payload) => {
    saveSession(await authApi.register(payload))
  }, [])

  const logout = useCallback(async () => {
    try {
      await authApi.logout()
    } catch {
      // token may already be expired; clearing locally is enough
    }
    saveSession(null)
  }, [])

  const updateUser = useCallback((user) => {
    const current = loadSession()
    if (current) saveSession({ ...current, user })
  }, [])

  const value = useMemo(
    () => ({
      user: session?.user ?? null,
      isAuthenticated: Boolean(session?.access_token),
      isAdmin: session?.user?.role === 'ADMIN',
      login,
      register,
      logout,
      updateUser,
    }),
    [session, login, register, logout, updateUser],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
