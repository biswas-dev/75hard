import { useEffect, useState } from 'react'
import { api } from '../lib/api'
import type { AIStatus, ProviderBalance } from '../lib/types'

/**
 * What the AI features are costing, and what is left to spend.
 *
 * Only some providers publish a balance — DeepSeek does, NVIDIA and Anthropic
 * do not — so a provider that is absent here is unknown rather than empty.
 * Those are very different things and the card does not conflate them.
 */
export function AiBalanceCard() {
  const [status, setStatus] = useState<AIStatus | null>(null)
  const [balances, setBalances] = useState<ProviderBalance[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false
    Promise.all([
      api.aiStatus().catch(() => null),
      api.aiBalance().catch(() => ({ balances: [] as ProviderBalance[], cached: false })),
    ]).then(([s, b]) => {
      if (cancelled) return
      setStatus(s)
      setBalances(b.balances)
      setLoading(false)
    })
    return () => {
      cancelled = true
    }
  }, [])

  if (loading || !status?.enabled) return null

  return (
    <section className="card space-y-3 p-4">
      <h2 className="text-sm font-medium uppercase tracking-wide text-ink-500">AI</h2>

      <dl className="space-y-1.5 text-sm">
        <div className="flex justify-between">
          <dt className="text-ink-500">Providers</dt>
          <dd className="font-mono text-ink-300">{(status.providers ?? []).join(' → ') || '—'}</dd>
        </div>
        <div className="flex justify-between">
          <dt className="text-ink-500">Used today</dt>
          <dd className="font-mono text-ink-300">
            {status.used_today}
            <span className="text-ink-600"> / {status.daily_limit}</span>
          </dd>
        </div>
      </dl>

      {balances.length > 0 && (
        <div className="border-t border-ink-800 pt-3">
          <p className="mb-2 text-xs text-ink-500">Credit remaining</p>
          <ul className="space-y-1.5">
            {balances.map((b) => (
              <li key={b.provider} className="flex items-baseline justify-between text-sm">
                <span className="capitalize text-ink-400">{b.provider}</span>
                {b.error ? (
                  <span className="text-xs text-ink-600">{b.error}</span>
                ) : (
                  <span
                    className={`font-mono ${
                      // Under a dollar is worth noticing before it stops working.
                      !b.available || b.amount < 1 ? 'text-flame-400' : 'text-moss-400'
                    }`}
                  >
                    {b.currency === 'USD' ? '$' : ''}
                    {b.amount.toFixed(2)}
                    {b.currency !== 'USD' && ` ${b.currency}`}
                  </span>
                )}
              </li>
            ))}
          </ul>
          <p className="mt-2 text-xs text-ink-600">
            Only some providers publish a balance; the rest are not shown rather than shown as
            zero. Updated every ten minutes, and after each call.
          </p>
        </div>
      )}
    </section>
  )
}
