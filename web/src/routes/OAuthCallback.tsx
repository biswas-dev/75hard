import { useEffect, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { api } from '../lib/api'
import { useAuth } from '../state/AuthContext'

/**
 * Landing point for go-login's success redirect, which arrives as
 * /oauth/callback?token=… — the token is exchanged for a session and the
 * query string is dropped from history so it isn't left in the URL bar.
 */
export function OAuthCallback() {
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const { loginWithToken } = useAuth()
  const [error, setError] = useState('')
  const ran = useRef(false)

  useEffect(() => {
    // StrictMode double-invokes effects in development; only run once.
    if (ran.current) return
    ran.current = true

    const token = params.get('token')
    if (!token) {
      navigate('/login?error=Sign+in+failed', { replace: true })
      return
    }

    loginWithToken(token)
      .then(async () => {
        // Send a first-time user to setup, a returning one to today.
        try {
          await api.activeProgram()
          navigate('/app', { replace: true })
        } catch {
          navigate('/start', { replace: true })
        }
      })
      .catch(() => setError('That sign-in link was not valid.'))
  }, [params, loginWithToken, navigate])

  return (
    <div className="flex h-full flex-col items-center justify-center gap-4 px-4">
      {error ? (
        <>
          <p className="text-center text-red-400">{error}</p>
          <button className="btn-ghost" onClick={() => navigate('/login', { replace: true })}>
            Back to sign in
          </button>
        </>
      ) : (
        <>
          <div className="h-8 w-8 animate-spin rounded-full border-2 border-ink-700 border-t-flame-500" />
          <p className="text-sm text-ink-500">Signing you in…</p>
        </>
      )}
    </div>
  )
}
