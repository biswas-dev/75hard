import { useCallback, useEffect, useState } from 'react'
import { api } from '../lib/api'
import type { Passkey } from '../lib/types'
import {
  decodeCreationOptions,
  encodeCreatedCredential,
  passkeysSupported,
  platformAuthenticatorAvailable,
  wasCancelled,
} from '../lib/webauthn'

/**
 * Registering and removing passkeys.
 *
 * A passkey signs in on its own and does not ask for a code afterwards: the
 * authenticator has already checked a fingerprint, a face or a PIN before it
 * would sign anything, so the assertion is already two factors.
 */
export function PasskeysCard() {
  const [keys, setKeys] = useState<Passkey[]>([])
  const [supported, setSupported] = useState(false)
  const [platform, setPlatform] = useState(false)
  const [name, setName] = useState('')
  const [adding, setAdding] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [removing, setRemoving] = useState<Passkey | null>(null)

  const load = useCallback(async () => {
    try {
      setKeys(await api.listPasskeys())
    } catch {
      setKeys([])
    }
  }, [])

  useEffect(() => {
    setSupported(passkeysSupported())
    platformAuthenticatorAvailable().then(setPlatform)
    load()
  }, [load])

  async function add() {
    setBusy(true)
    setError('')
    try {
      const { session_id, options } = await api.passkeyRegisterBegin()
      const publicKey = decodeCreationOptions(
        ((options as { publicKey?: Record<string, unknown> }).publicKey ?? options) as Record<
          string,
          unknown
        >,
      )
      const cred = (await navigator.credentials.create({ publicKey })) as PublicKeyCredential | null
      if (!cred) return

      await api.passkeyRegisterFinish({
        session_id,
        name: name.trim() || defaultName(platform),
        credential: encodeCreatedCredential(cred),
      })
      setName('')
      setAdding(false)
      await load()
    } catch (err) {
      // A dismissed prompt is a change of mind, not a failure.
      if (wasCancelled(err)) return
      setError(err instanceof Error ? err.message : 'Could not add that passkey')
    } finally {
      setBusy(false)
    }
  }

  async function remove(pk: Passkey) {
    setBusy(true)
    setError('')
    try {
      await api.deletePasskey(pk.id)
      setRemoving(null)
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not remove that passkey')
    } finally {
      setBusy(false)
    }
  }

  if (!supported) return null

  return (
    <section className="card space-y-4 p-4">
      <div>
        <h2 className="text-sm font-medium uppercase tracking-wide text-ink-500">Passkeys</h2>
        <p className="mt-1 text-sm text-ink-400">
          Sign in with {platform ? 'this device' : 'a phone or security key'} instead of a
          password. Nothing to remember and nothing to phish.
        </p>
      </div>

      {keys.length > 0 && (
        <ul className="divide-y divide-ink-800">
          {keys.map((pk) => (
            <li key={pk.id} className="flex items-center gap-3 py-2.5">
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm text-ink-200">{pk.name}</p>
                <p className="text-xs text-ink-600">
                  {pk.last_used_at
                    ? `Last used ${new Date(pk.last_used_at).toLocaleDateString()}`
                    : 'Never used'}
                  {pk.backed_up ? ' · synced' : ' · this device only'}
                </p>
              </div>
              <button
                className="shrink-0 px-2 text-ink-600 hover:text-red-400"
                aria-label={`Remove ${pk.name}`}
                onClick={() => setRemoving(pk)}
              >
                ×
              </button>
            </li>
          ))}
        </ul>
      )}

      {removing && (
        <div className="space-y-3 rounded-xl border border-red-500/25 bg-red-500/10 p-3">
          <p className="text-sm text-red-300">
            Remove {removing.name}? You will not be able to sign in with it again.
            {keys.length === 1 && ' It is your only passkey.'}
          </p>
          <div className="flex gap-2">
            <button className="btn-ghost flex-1 py-2 text-sm" onClick={() => setRemoving(null)}>
              Keep
            </button>
            <button
              className="btn-danger flex-1 py-2 text-sm"
              onClick={() => remove(removing)}
              disabled={busy}
            >
              Remove
            </button>
          </div>
        </div>
      )}

      {adding ? (
        <div className="space-y-3">
          <div>
            <label className="label" htmlFor="passkey-name">Name it</label>
            <input
              id="passkey-name"
              className="field"
              placeholder={defaultName(platform)}
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
            <p className="mt-1.5 text-xs text-ink-600">
              So you can tell it apart later when you have more than one.
            </p>
          </div>
          <div className="flex gap-2">
            <button
              className="btn-ghost flex-1"
              onClick={() => {
                setAdding(false)
                setName('')
              }}
            >
              Cancel
            </button>
            <button className="btn-primary flex-1" onClick={add} disabled={busy}>
              {busy ? 'Waiting…' : 'Continue'}
            </button>
          </div>
        </div>
      ) : (
        <button className="btn-ghost w-full" onClick={() => setAdding(true)}>
          Add a passkey
        </button>
      )}

      {error && <p className="text-sm text-red-400">{error}</p>}
    </section>
  )
}

/** A name that means something before the browser has said which key it was. */
function defaultName(platform: boolean): string {
  return platform ? 'This device' : 'Security key'
}
