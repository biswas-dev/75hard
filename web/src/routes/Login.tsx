import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api } from '../lib/api'
import type { AuthConfig } from '../lib/types'
import { useAuth } from '../state/AuthContext'
import { AuthShell, OAuthButtons } from './AuthShell'

export function Login() {
  const { login } = useAuth()
  const navigate = useNavigate()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [config, setConfig] = useState<AuthConfig | null>(null)

  useEffect(() => {
    api.authConfig().then(setConfig).catch(() => setConfig(null))

    // go-login redirects failures back here with a human-readable reason.
    const reason = new URLSearchParams(window.location.search).get('error')
    if (reason) setError(reason)
  }, [])

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await login(email, password)
      navigate('/app', { replace: true })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not sign in')
    } finally {
      setBusy(false)
    }
  }

  return (
    <AuthShell title="Welcome back" subtitle="Pick up where you left off.">
      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <label className="label" htmlFor="email">Email</label>
          <input
            id="email"
            type="email"
            className="field"
            autoComplete="email"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        </div>
        <div>
          <label className="label" htmlFor="password">Password</label>
          <input
            id="password"
            type="password"
            className="field"
            autoComplete="current-password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </div>

        {error && (
          <p className="rounded-xl border border-red-500/25 bg-red-500/10 px-4 py-3 text-sm text-red-400">
            {error}
          </p>
        )}

        <button type="submit" className="btn-primary w-full" disabled={busy}>
          {busy ? 'Signing in…' : 'Sign in'}
        </button>

        <p className="text-center text-sm">
          <Link to="/forgot-password" className="text-ink-500 hover:text-ink-300">
            Forgot your password?
          </Link>
        </p>
      </form>

      <OAuthButtons config={config} />

      {config?.allow_signup !== false && (
        <p className="mt-6 text-center text-sm text-ink-500">
          No account?{' '}
          <Link to="/signup" className="text-flame-500 hover:text-flame-400">
            Create one
          </Link>
        </p>
      )}
    </AuthShell>
  )
}
