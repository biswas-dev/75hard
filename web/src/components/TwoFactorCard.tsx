import { useEffect, useState } from 'react'
import QRCode from 'qrcode'
import { api } from '../lib/api'
import type { TwoFactorSetup, TwoFactorStatus } from '../lib/types'
import { useAuth } from '../state/AuthContext'

/**
 * Enrolling and un-enrolling an authenticator app.
 *
 * Enrolment is two steps on purpose: the secret is stored the moment it is
 * generated but does nothing until a code from the phone proves it was scanned
 * correctly. A QR misread that switched two-factor on would lock the account
 * out of itself, which is a far worse failure than having to scan again.
 */
export function TwoFactorCard() {
  const { user } = useAuth()
  const [status, setStatus] = useState<TwoFactorStatus | null>(null)
  const [setup, setSetup] = useState<TwoFactorSetup | null>(null)
  const [qr, setQr] = useState('')
  const [code, setCode] = useState('')
  const [recovery, setRecovery] = useState<string[]>([])
  const [disabling, setDisabling] = useState(false)
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function load() {
    try {
      setStatus(await api.twoFactorStatus())
    } catch {
      setStatus(null)
    }
  }

  useEffect(() => {
    load()
  }, [])

  // Render the otpauth URI locally. Sending a secret to somebody's QR service
  // to be drawn would be handing them the key.
  useEffect(() => {
    if (!setup) {
      setQr('')
      return
    }
    QRCode.toDataURL(setup.uri, { width: 320, margin: 1 })
      .then(setQr)
      .catch(() => setQr(''))
  }, [setup])

  async function begin() {
    setBusy(true)
    setError('')
    try {
      setSetup(await api.twoFactorSetup())
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not start enrolment')
    } finally {
      setBusy(false)
    }
  }

  async function confirm() {
    setBusy(true)
    setError('')
    try {
      const res = await api.twoFactorConfirm(code.trim())
      setRecovery(res.recovery_codes)
      setSetup(null)
      setCode('')
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'That code did not match')
    } finally {
      setBusy(false)
    }
  }

  async function disable() {
    setBusy(true)
    setError('')
    try {
      await api.twoFactorDisable(
        user?.auth_provider === 'password' ? { password } : { code: code.trim() },
      )
      setDisabling(false)
      setPassword('')
      setCode('')
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not turn it off')
    } finally {
      setBusy(false)
    }
  }

  if (!status) return null

  return (
    <section className="card space-y-4 p-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-sm font-medium uppercase tracking-wide text-ink-500">
            Two-factor
          </h2>
          <p className="mt-1 text-sm text-ink-400">
            {status.enabled
              ? 'A code from your authenticator app is needed to sign in.'
              : 'A six-digit code from your phone, on top of your password.'}
          </p>
        </div>
        {status.enabled && (
          <span className="shrink-0 rounded-lg bg-moss-500/15 px-2 py-1 text-xs text-moss-400">
            On
          </span>
        )}
      </div>

      {!status.configurable && (
        <p className="rounded-xl border border-ink-800 bg-ink-850 px-3 py-2 text-sm text-ink-500">
          This server has no encryption key set, so a secret cannot be stored.
        </p>
      )}

      {/* The codes, shown once. They are stored hashed; there is no second
          chance to read them, so this stays on screen until dismissed. */}
      {recovery.length > 0 && (
        <div className="rounded-xl border border-flame-500/30 bg-flame-500/5 p-3">
          <p className="text-sm font-medium text-flame-400">Save your recovery codes</p>
          <p className="mt-1 text-xs text-ink-400">
            Each one works once, and only these will get you in if you lose your phone. They are
            stored hashed — this is the only time they can be shown.
          </p>
          <ul className="mt-3 grid grid-cols-2 gap-1 font-mono text-sm text-ink-200">
            {recovery.map((c) => (
              <li key={c}>{c}</li>
            ))}
          </ul>
          <div className="mt-3 flex gap-2">
            <button
              className="btn-ghost flex-1 py-2 text-sm"
              onClick={() => navigator.clipboard?.writeText(recovery.join('\n'))}
            >
              Copy
            </button>
            <button
              className="btn-ghost flex-1 py-2 text-sm"
              onClick={() => setRecovery([])}
            >
              I have saved them
            </button>
          </div>
        </div>
      )}

      {setup && (
        <div className="space-y-3">
          {qr && (
            <img
              src={qr}
              alt="Scan this with your authenticator app"
              className="mx-auto w-48 rounded-xl bg-white p-2"
            />
          )}
          <div>
            <p className="text-xs uppercase tracking-wide text-ink-500">Or type this in</p>
            <p className="mt-1 select-all break-all font-mono text-sm text-ink-300">
              {setup.secret}
            </p>
          </div>
          <div>
            <label className="label" htmlFor="totp-code">Code from the app</label>
            <input
              id="totp-code"
              inputMode="numeric"
              autoComplete="one-time-code"
              className="field text-center font-mono text-xl tracking-[0.3em]"
              placeholder="000000"
              value={code}
              onChange={(e) => setCode(e.target.value)}
            />
          </div>
          <div className="flex gap-2">
            <button
              className="btn-ghost flex-1"
              onClick={() => {
                setSetup(null)
                setCode('')
              }}
            >
              Cancel
            </button>
            <button
              className="btn-primary flex-1"
              onClick={confirm}
              disabled={busy || code.trim().length < 6}
            >
              {busy ? 'Checking…' : 'Turn on'}
            </button>
          </div>
        </div>
      )}

      {!setup && !status.enabled && status.configurable && (
        <button className="btn-ghost w-full" onClick={begin} disabled={busy}>
          {status.pending ? 'Start again' : 'Set up two-factor'}
        </button>
      )}

      {status.enabled && !disabling && (
        <>
          <p className="text-xs text-ink-600">
            {status.recovery_codes_left} recovery{' '}
            {status.recovery_codes_left === 1 ? 'code' : 'codes'} left.
          </p>
          <button className="btn-ghost w-full" onClick={() => setDisabling(true)}>
            Turn off two-factor
          </button>
        </>
      )}

      {status.enabled && disabling && (
        <div className="space-y-3 rounded-xl border border-red-500/25 bg-red-500/10 p-3">
          <p className="text-sm text-red-300">
            Your account will be protected by your password alone.
          </p>
          {user?.auth_provider === 'password' ? (
            <input
              type="password"
              autoComplete="current-password"
              className="field"
              placeholder="Your password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          ) : (
            <input
              inputMode="numeric"
              className="field text-center font-mono tracking-[0.3em]"
              placeholder="000000"
              value={code}
              onChange={(e) => setCode(e.target.value)}
            />
          )}
          <div className="flex gap-2">
            <button
              className="btn-ghost flex-1 py-2 text-sm"
              onClick={() => {
                setDisabling(false)
                setPassword('')
                setCode('')
                setError('')
              }}
            >
              Keep it on
            </button>
            <button className="btn-danger flex-1 py-2 text-sm" onClick={disable} disabled={busy}>
              Turn off
            </button>
          </div>
        </div>
      )}

      {error && <p className="text-sm text-red-400">{error}</p>}
    </section>
  )
}
