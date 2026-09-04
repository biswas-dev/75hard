import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { api } from '../lib/api'
import type { User } from '../lib/types'

interface AuthValue {
  user: User | null
  loading: boolean
  /**
   * Sign in with a password.
   *
   * Returns a challenge when the account needs a code, and null once signed
   * in — the caller has to look, rather than assume it is done.
   */
  login: (email: string, password: string) => Promise<string | null>
  /** Finish a sign-in that stopped for a code. */
  completeTwoFactor: (challenge: string, code: string) => Promise<void>
  signup: (email: string, password: string, name: string) => Promise<void>
  loginWithToken: (token: string) => Promise<void>
  logout: () => void
  refresh: () => Promise<void>
}

const AuthContext = createContext<AuthValue | null>(null)

/** The browser's timezone, sent at signup so day boundaries are right from day 1. */
function browserTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
  } catch {
    return 'UTC'
  }
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    // Restore the session from the stored token on first paint.
    if (!api.getToken()) {
      setLoading(false)
      return
    }
    api
      .me()
      .then(setUser)
      .catch(() => api.setToken(null))
      .finally(() => setLoading(false))
  }, [])

  const login = useCallback(async (email: string, password: string) => {
    const res = await api.login(email, password)
    if ('two_factor' in res) {
      // A correct password, but sign-in is not finished. No token is stored:
      // the challenge is not a session and must not be treated as one.
      return res.challenge
    }
    api.setToken(res.token)
    setUser(res.user)
    return null
  }, [])

  const completeTwoFactor = useCallback(async (challenge: string, code: string) => {
    const res = await api.verifyTwoFactor(challenge, code)
    api.setToken(res.token)
    setUser(res.user)
  }, [])

  const signup = useCallback(async (email: string, password: string, name: string) => {
    const res = await api.signup({ email, password, name, timezone: browserTimezone() })
    api.setToken(res.token)
    setUser(res.user)
  }, [])

  // Used by the OAuth callback route, which receives ?token= in the URL.
  const loginWithToken = useCallback(async (token: string) => {
    api.setToken(token)
    setUser(await api.me())
  }, [])

  const logout = useCallback(() => {
    api.setToken(null)
    setUser(null)
  }, [])

  const refresh = useCallback(async () => {
    setUser(await api.me())
  }, [])

  const value = useMemo(
    () => ({ user, loading, login, completeTwoFactor, signup, loginWithToken, logout, refresh }),
    [user, loading, login, completeTwoFactor, signup, loginWithToken, logout, refresh],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthValue {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used inside AuthProvider')
  return ctx
}
