import { useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { api } from '../lib/api'
import { useAuth } from '../state/AuthContext'
import { AuthShell } from './AuthShell'

/** Requests a reset link. */
export function ForgotPassword() {
  const [email, setEmail] = useState('')
  const [busy, setBusy] = useState(false)
  const [sent, setSent] = useState(false)
  const [resetURL, setResetURL] = useState('')
  const [error, setError] = useState('')

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      const res = await api.forgotPassword(email)
      setSent(true)
      // On a single-operator instance the server hands the link straight back
      // rather than mailing it. When it doesn't, the link is in the logs.
      if (res.reset_url) setResetURL(res.reset_url)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong')
    } finally {
      setBusy(false)
    }
  }

  if (sent) {
    return (
      <AuthShell title="Check your email" subtitle="If that address has an account, a link is on its way.">
        <p className="text-sm text-ink-400">
          The link is good for one hour and can be used once.
        </p>

        {resetURL && (
          <div className="mt-4 rounded-xl border border-flame-500/25 bg-flame-500/[0.07] p-4">
            <p className="text-xs text-ink-400">
              This server has no mail sender configured, so here is the link directly:
            </p>
            <a
              href={resetURL}
              className="mt-2 block break-all text-sm text-flame-400 hover:text-flame-300"
            >
              {resetURL}
            </a>
          </div>
        )}

        <Link to="/login" className="btn-ghost mt-6 w-full">
          Back to sign in
        </Link>
      </AuthShell>
    )
  }

  return (
    <AuthShell title="Reset your password" subtitle="We'll send a link to set a new one.">
      <form onSubmit={submit} className="space-y-4">
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

        {error && (
          <p className="rounded-xl border border-red-500/25 bg-red-500/10 px-4 py-3 text-sm text-red-400">
            {error}
          </p>
        )}

        <button type="submit" className="btn-primary w-full" disabled={busy}>
          {busy ? 'Sending…' : 'Send reset link'}
        </button>
      </form>

      <p className="mt-6 text-center text-sm text-ink-500">
        Remembered it?{' '}
        <Link to="/login" className="text-flame-500 hover:text-flame-400">
          Sign in
        </Link>
      </p>
    </AuthShell>
  )
}

/** Sets a new password from a reset link. */
export function ResetPassword() {
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const { loginWithToken } = useAuth()

  const token = params.get('token') ?? ''
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    if (password.length < 8) {
      setError('Password must be at least 8 characters')
      return
    }
    if (password !== confirm) {
      setError('Those two passwords do not match')
      return
    }
    setBusy(true)
    setError('')
    try {
      const res = await api.resetPassword(token, password)
      // The server signs them in on success, so there is no reason to make
      // them retype what they just set.
      await loginWithToken(res.token)
      navigate('/app', { replace: true })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not set the password')
      setBusy(false)
    }
  }

  if (!token) {
    return (
      <AuthShell title="That link is incomplete" subtitle="It is missing its token.">
        <Link to="/forgot-password" className="btn-primary w-full">
          Request a new link
        </Link>
      </AuthShell>
    )
  }

  return (
    <AuthShell title="Set a new password" subtitle="Then you'll be signed straight in.">
      <form onSubmit={submit} className="space-y-4">
        <div>
          <label className="label" htmlFor="new">New password</label>
          <input
            id="new"
            type="password"
            className="field"
            autoComplete="new-password"
            required
            minLength={8}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
          <p className="mt-1.5 text-xs text-ink-600">At least 8 characters.</p>
        </div>
        <div>
          <label className="label" htmlFor="confirm">Confirm</label>
          <input
            id="confirm"
            type="password"
            className="field"
            autoComplete="new-password"
            required
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
          />
        </div>

        {error && (
          <p className="rounded-xl border border-red-500/25 bg-red-500/10 px-4 py-3 text-sm text-red-400">
            {error}
          </p>
        )}

        <button type="submit" className="btn-primary w-full" disabled={busy}>
          {busy ? 'Setting…' : 'Set password and sign in'}
        </button>
      </form>

      <p className="mt-6 text-center text-sm text-ink-500">
        <Link to="/forgot-password" className="text-flame-500 hover:text-flame-400">
          Request a new link
        </Link>
      </p>
    </AuthShell>
  )
}
