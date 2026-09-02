import { useCallback, useEffect, useState } from 'react'
import { api } from '../lib/api'
import type { APIToken, CreatedToken } from '../lib/types'

type Scope = 'read' | 'write'

/**
 * Personal API tokens.
 *
 * The secret is shown once, in this component, and never again — the server
 * keeps only its hash. So the created state is deliberately sticky: it stays
 * on screen until dismissed, rather than disappearing on the next re-render
 * and taking an unrecoverable credential with it.
 */
export function ApiTokensCard() {
  const [tokens, setTokens] = useState<APIToken[]>([])
  const [created, setCreated] = useState<CreatedToken | null>(null)
  const [name, setName] = useState('')
  const [scope, setScope] = useState<Scope>('read')
  const [expiry, setExpiry] = useState(0)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [copied, setCopied] = useState('')

  const load = useCallback(async () => {
    try {
      setTokens(await api.listTokens())
    } catch {
      setTokens([])
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  async function create() {
    if (busy) return
    setBusy(true)
    setError('')
    try {
      const result = await api.createToken({
        name: name.trim() || 'API token',
        // Write implies read, so a write token carries both.
        scopes: scope === 'write' ? ['read', 'write'] : ['read'],
        expires_in_days: expiry,
      })
      setCreated(result)
      setName('')
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not create the token')
    } finally {
      setBusy(false)
    }
  }

  async function copy(text: string, what: string) {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(what)
      window.setTimeout(() => setCopied(''), 2000)
    } catch {
      // Clipboard access can be refused; the value is on screen to select.
      setCopied('')
    }
  }

  return (
    <section className="card space-y-4 p-4">
      <div>
        <h2 className="text-sm font-medium uppercase tracking-wide text-ink-500">API tokens</h2>
        <p className="mt-1 text-xs text-ink-600">
          For scripts and assistants. Pair one with the{' '}
          <a href="/api/openapi.yaml" className="text-flame-400 hover:underline">
            OpenAPI document
          </a>{' '}
          to let a tool work out the calls on its own.
        </p>
      </div>

      {created && (
        <div className="space-y-3 rounded-xl border border-flame-500/30 bg-flame-500/5 p-3">
          <p className="text-xs text-flame-400">
            Copy this now — it is shown once and cannot be recovered.
          </p>

          <div className="flex items-center gap-2">
            <code className="min-w-0 flex-1 overflow-x-auto rounded-lg bg-ink-900 px-2 py-1.5 font-mono text-xs text-ink-200">
              {created.secret}
            </code>
            <button
              className="btn-ghost shrink-0 px-3 py-1.5 text-xs"
              onClick={() => copy(created.secret, 'token')}
            >
              {copied === 'token' ? 'Copied' : 'Copy'}
            </button>
          </div>

          <div>
            <p className="mb-1 text-xs text-ink-500">Try it</p>
            <div className="flex items-center gap-2">
              <code className="min-w-0 flex-1 overflow-x-auto rounded-lg bg-ink-900 px-2 py-1.5 font-mono text-[11px] text-ink-400">
                {created.discovery.example}
              </code>
              <button
                className="btn-ghost shrink-0 px-3 py-1.5 text-xs"
                onClick={() => copy(created.discovery.example, 'example')}
              >
                {copied === 'example' ? 'Copied' : 'Copy'}
              </button>
            </div>
          </div>

          <p className="text-xs text-ink-600">
            API description:{' '}
            <a
              href={created.discovery.spec_url}
              className="text-flame-400 hover:underline"
              target="_blank"
              rel="noreferrer"
            >
              {created.discovery.spec_url}
            </a>
          </p>

          <button className="btn-ghost w-full py-2 text-sm" onClick={() => setCreated(null)}>
            I've saved it
          </button>
        </div>
      )}

      {tokens.length > 0 && (
        <ul className="divide-y divide-ink-800">
          {tokens.map((t) => (
            <li key={t.id} className="flex items-center gap-3 py-2.5">
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm text-ink-200">{t.name}</span>
                <span className="font-mono text-xs text-ink-600">
                  {t.prefix}… · {t.scopes.join(', ')}
                  {t.last_used_at
                    ? ` · used ${new Date(t.last_used_at).toLocaleDateString()}`
                    : ' · never used'}
                  {t.expires_at && ` · expires ${new Date(t.expires_at).toLocaleDateString()}`}
                </span>
              </span>
              <button
                className="shrink-0 px-2 text-xs text-ink-600 hover:text-red-400"
                onClick={async () => {
                  await api.revokeToken(t.id)
                  await load()
                }}
              >
                Revoke
              </button>
            </li>
          ))}
        </ul>
      )}

      <div className="space-y-2">
        <input
          className="field"
          placeholder="What is it for?"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />

        <div className="grid grid-cols-2 gap-2">
          {(['read', 'write'] as Scope[]).map((s) => (
            <button
              key={s}
              type="button"
              onClick={() => setScope(s)}
              className={`rounded-xl border py-2 text-sm transition ${
                scope === s
                  ? 'border-flame-500 bg-flame-500/15 text-flame-400'
                  : 'border-ink-800 bg-ink-850 text-ink-400'
              }`}
            >
              {s === 'read' ? 'Read only' : 'Read and write'}
            </button>
          ))}
        </div>

        <select
          className="field"
          value={expiry}
          onChange={(e) => setExpiry(Number(e.target.value))}
        >
          <option value={0}>Never expires</option>
          <option value={30}>Expires in 30 days</option>
          <option value={90}>Expires in 90 days</option>
          <option value={365}>Expires in a year</option>
        </select>

        <button className="btn-ghost w-full" onClick={create} disabled={busy}>
          {busy ? 'Creating…' : 'Create a token'}
        </button>

        <p className="text-xs text-ink-600">
          A read-only token can look at everything and change nothing — the safer choice for
          anything you do not fully control.
        </p>
      </div>

      {error && <p className="text-sm text-red-400">{error}</p>}
    </section>
  )
}
