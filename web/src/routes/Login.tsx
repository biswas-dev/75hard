import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api } from '../lib/api'
import type { AuthConfig } from '../lib/types'
import {
  decodeRequestOptions,
  encodeAssertion,
  passkeysSupported,
  wasCancelled,
} from '../lib/webauthn'
import { useAuth } from '../state/AuthContext'
import { AuthShell, OAuthButtons } from './AuthShell'

export function Login() {
  const { login, completeTwoFactor, loginWithToken } = useAuth()
  const navigate = useNavigate()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [config, setConfig] = useState<AuthConfig | null>(null)
  // Set when the password was right but the account wants a code. Holding it
  // in state rather than storing anything means a reload starts over, which is
  // the safe direction.
  const [challenge, setChallenge] = useState('')
  const [code, setCode] = useState('')

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
      const pending = await login(email, password)
      if (pending) {
        setChallenge(pending)
        return
      }
      navigate('/app', { replace: true })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not sign in')
    } finally {
      setBusy(false)
    }
  }

  async function handleCode(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await completeTwoFactor(challenge, code.trim())
      navigate('/app', { replace: true })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'That code did not work')
      setCode('')
    } finally {
      setBusy(false)
    }
  }

  async function handlePasskey() {
    setBusy(true)
    setError('')
    try {
      const { session_id, options } = await api.passkeyLoginBegin()
      const publicKey = decodeRequestOptions(
        ((options as { publicKey?: Record<string, unknown> }).publicKey ?? options) as Record<
          string,
          unknown
        >,
      )
      const cred = (await navigator.credentials.get({ publicKey })) as PublicKeyCredential | null
      if (!cred) {
        return
      }
      const res = await api.passkeyLoginFinish({
        session_id,
        credential: encodeAssertion(cred),
      })
      await loginWithToken(res.token)
      navigate('/app', { replace: true })
    } catch (err) {
      // Dismissing the browser prompt throws the same error a refusal does.
      // Putting a red message on screen for a change of mind is noise.
      if (wasCancelled(err)) return
      setError(err instanceof Error ? err.message : 'That passkey did not work')
    } finally {
      setBusy(false)
    }
  }

  if (challenge) {
    return (
      <AuthShell
        title="One more step"
        subtitle="Enter the six-digit code from your authenticator app."
      >
        <form onSubmit={handleCode} className="space-y-4">
          <div>
            <label className="label" htmlFor="code">Code</label>
            <input
              id="code"
              inputMode="numeric"
              autoComplete="one-time-code"
              autoFocus
              className="field text-center font-mono text-2xl tracking-[0.4em]"
              placeholder="000000"
              value={code}
              onChange={(e) => setCode(e.target.value)}
            />
            <p className="mt-2 text-xs text-ink-600">
              Lost your phone? A recovery code works here too.
            </p>
          </div>

          {error && (
            <p className="rounded-xl border border-red-500/25 bg-red-500/10 px-4 py-3 text-sm text-red-400">
              {error}
            </p>
          )}

          <button type="submit" className="btn-primary w-full" disabled={busy || code.length < 6}>
            {busy ? 'Checking…' : 'Sign in'}
          </button>

          <button
            type="button"
            className="btn-ghost w-full"
            onClick={() => {
              setChallenge('')
              setCode('')
              setError('')
            }}
          >
            Back
          </button>
        </form>
      </AuthShell>
    )
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

        {config?.passkeys && passkeysSupported() && (
          <button type="button" className="btn-ghost w-full" onClick={handlePasskey} disabled={busy}>
            Sign in with a passkey
          </button>
        )}

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
