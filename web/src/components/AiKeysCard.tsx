import { useCallback, useEffect, useState } from 'react'
import { api } from '../lib/api'
import type { AIKeys, AISlot, ProviderInfo } from '../lib/types'

/**
 * Bring-your-own provider keys.
 *
 * The AI features cost money, and the instance's own keys are somebody's
 * personal credit — so anyone other than the administrator supplies their own
 * or goes without. The card leads with the two facts that actually decide the
 * choice: whether a provider is free, and whether it will tell you what is
 * left.
 */
export function AiKeysCard() {
  const [data, setData] = useState<AIKeys | null>(null)
  const [editing, setEditing] = useState<number | null>(null)
  const [provider, setProvider] = useState('')
  const [model, setModel] = useState('')
  const [apiKey, setApiKey] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    try {
      setData(await api.aiKeys())
    } catch {
      setData(null)
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  function startEdit(slot: number, existing?: AISlot) {
    setEditing(slot)
    setError('')
    setApiKey('')
    const p = existing?.provider || data?.providers[0]?.name || ''
    setProvider(p)
    setModel(existing?.model || suggestedFor(p, data?.providers) || '')
  }

  function suggestedFor(name: string, providers?: ProviderInfo[]) {
    return providers?.find((p) => p.name === name)?.suggested_model ?? ''
  }

  async function save(slot: number) {
    if (busy) return
    setBusy(true)
    setError('')
    try {
      await api.saveAIKey(slot, {
        provider,
        model: model.trim(),
        // Empty keeps whatever is already stored, so the model can be changed
        // without pasting the key again.
        api_key: apiKey.trim() || undefined,
      })
      setEditing(null)
      setApiKey('')
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not save')
    } finally {
      setBusy(false)
    }
  }

  if (!data) return null

  const slotNumbers = [1, 2, 3]
  const byslot = new Map(data.slots.map((s) => [s.slot, s]))

  return (
    <section className="card space-y-4 p-4">
      <div>
        <h2 className="text-sm font-medium uppercase tracking-wide text-ink-500">AI providers</h2>
        <p className="mt-1 text-xs text-ink-600">
          {data.using_own_keys
            ? 'Running on your own keys. Slot 1 is tried first, then 2, then 3.'
            : data.server_fallback
              ? "Using this server's keys. Add your own below to use them instead."
              : 'Add a provider key to turn on photo calorie estimates, recipes and the coach.'}
        </p>
      </div>

      {!data.configurable && (
        <p className="rounded-lg bg-ink-850 px-3 py-2 text-xs text-ink-500">
          This server has no encryption key configured, so provider keys cannot be stored safely
          here.
        </p>
      )}

      {data.configurable && (
        <ul className="space-y-2">
          {slotNumbers.map((n) => {
            const slot = byslot.get(n)
            const info = data.providers.find((p) => p.name === slot?.provider)
            return (
              <li key={n} className="rounded-xl border border-ink-800 bg-ink-850/40 p-3">
                {editing === n ? (
                  <div className="space-y-2">
                    <select
                      className="field"
                      value={provider}
                      onChange={(e) => {
                        setProvider(e.target.value)
                        setModel(suggestedFor(e.target.value, data.providers))
                      }}
                    >
                      {data.providers.map((p) => (
                        <option key={p.name} value={p.name}>
                          {p.label}
                          {p.free ? ' — free tier' : ''}
                        </option>
                      ))}
                    </select>
                    <input
                      className="field"
                      value={model}
                      onChange={(e) => setModel(e.target.value)}
                      placeholder="Model"
                    />
                    <input
                      className="field"
                      type="password"
                      autoComplete="off"
                      value={apiKey}
                      onChange={(e) => setApiKey(e.target.value)}
                      placeholder={slot?.has_key ? 'Leave blank to keep the stored key' : 'API key'}
                    />
                    {(() => {
                      const chosen = data.providers.find((p) => p.name === provider)
                      return chosen ? (
                        <p className="text-xs text-ink-600">
                          {chosen.free
                            ? 'Free tier available. '
                            : 'Paid — you are billed by the provider. '}
                          {chosen.publishes_balance
                            ? 'Remaining credit is shown here.'
                            : 'This provider does not publish a balance.'}{' '}
                          <a
                            href={chosen.signup_url}
                            target="_blank"
                            rel="noreferrer"
                            className="text-flame-400 hover:underline"
                          >
                            Get a key
                          </a>
                        </p>
                      ) : null
                    })()}
                    <div className="flex gap-2">
                      <button
                        className="btn-ghost flex-1 py-2 text-sm"
                        onClick={() => save(n)}
                        disabled={busy}
                      >
                        {busy ? 'Saving…' : 'Save'}
                      </button>
                      <button
                        className="btn-ghost flex-1 py-2 text-sm"
                        onClick={() => setEditing(null)}
                      >
                        Cancel
                      </button>
                    </div>
                  </div>
                ) : (
                  <div className="flex items-center gap-3">
                    <span className="font-mono text-xs text-ink-600">{n}</span>
                    <span className="min-w-0 flex-1">
                      {slot ? (
                        <>
                          <span className="block truncate text-sm text-ink-200">
                            {info?.label ?? slot.provider}
                            {info?.free && (
                              <span className="ml-2 text-xs text-moss-400">free</span>
                            )}
                          </span>
                          <span className="font-mono text-xs text-ink-600">
                            {slot.model || 'default model'} · {slot.key_hint}
                          </span>
                        </>
                      ) : (
                        <span className="text-sm text-ink-600">
                          {n === 1 ? 'Primary — not set' : `Backup ${n - 1} — not set`}
                        </span>
                      )}
                    </span>
                    <button
                      className="shrink-0 px-2 text-xs text-ink-500 hover:text-ink-200"
                      onClick={() => startEdit(n, slot)}
                    >
                      {slot ? 'Change' : 'Add'}
                    </button>
                    {slot && (
                      <button
                        className="shrink-0 px-2 text-xs text-ink-600 hover:text-red-400"
                        onClick={async () => {
                          await api.deleteAIKey(n)
                          await load()
                        }}
                      >
                        Remove
                      </button>
                    )}
                  </div>
                )}
              </li>
            )
          })}
        </ul>
      )}

      <p className="text-xs text-ink-600">
        Keys are encrypted before they are stored and are never sent back to the browser — only
        the last four characters are shown, so you can tell which key is which.
      </p>

      {error && <p className="text-sm text-red-400">{error}</p>}
    </section>
  )
}
