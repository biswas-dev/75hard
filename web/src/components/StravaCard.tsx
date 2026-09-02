import { useCallback, useEffect, useState } from 'react'
import { api } from '../lib/api'
import type { StravaStatus } from '../lib/types'

/**
 * Connect, sync and disconnect a Strava account.
 *
 * An imported activity becomes a workout, which is the same row a manual entry
 * writes — so a synced session completes a day through exactly the same path.
 * Outdoor or indoor is decided from Strava's own signals; the trainer flag and
 * the sport type between them cover almost everything, and anything ambiguous
 * defaults to outdoor because that is what it usually is.
 */
export function StravaCard() {
  const [status, setStatus] = useState<StravaStatus | null>(null)
  const [busy, setBusy] = useState(false)
  const [note, setNote] = useState('')
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    try {
      setStatus(await api.stravaStatus())
    } catch {
      setStatus(null)
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  // The callback redirects back with ?strava=connected; pick it up, report it,
  // and clean the URL so a refresh does not repeat the message.
  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const result = params.get('strava')
    if (!result) return

    if (result === 'connected') setNote('Strava connected — recent activities imported.')
    if (result === 'error') setError('Could not connect to Strava. Try again.')

    params.delete('strava')
    const query = params.toString()
    window.history.replaceState({}, '', window.location.pathname + (query ? `?${query}` : ''))
    load()
  }, [load])

  async function connect() {
    setBusy(true)
    setError('')
    try {
      const { url } = await api.stravaConnect()
      window.location.href = url
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not start the connection')
      setBusy(false)
    }
  }

  async function sync() {
    setBusy(true)
    setError('')
    setNote('')
    try {
      const { imported } = await api.stravaSync()
      setNote(
        imported > 0
          ? `Synced ${imported} ${imported === 1 ? 'activity' : 'activities'}.`
          : 'Already up to date.',
      )
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Sync failed')
    } finally {
      setBusy(false)
    }
  }

  async function disconnect() {
    setBusy(true)
    try {
      await api.stravaDisconnect()
      setNote('Strava disconnected. Workouts already logged were kept.')
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not disconnect')
    } finally {
      setBusy(false)
    }
  }

  if (!status) return null

  return (
    <section className="card space-y-3 p-4">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-medium uppercase tracking-wide text-ink-500">Strava</h2>
        {status.connected && (
          <span className="rounded-full bg-moss-500/15 px-2.5 py-0.5 text-xs text-moss-400">
            Connected
          </span>
        )}
      </div>

      {!status.configured && (
        <p className="text-sm text-ink-500">
          This server has no Strava application configured, so the connection is unavailable.
        </p>
      )}

      {status.configured && !status.connected && (
        <>
          <p className="text-sm text-ink-400">
            Import your activities automatically. A session outdoors ticks the outdoor workout;
            a treadmill or gym session ticks the indoor one.
          </p>
          <button className="btn-ghost w-full" onClick={connect} disabled={busy}>
            {busy ? 'Opening Strava…' : 'Connect Strava'}
          </button>
        </>
      )}

      {status.configured && status.connected && (
        <>
          <dl className="space-y-1 text-sm">
            <Row label="Athlete" value={status.athlete || `#${status.athlete_id}`} />
            <Row label="Activities" value={String(status.activities)} />
            <Row
              label="Last sync"
              value={status.last_sync_at ? new Date(status.last_sync_at).toLocaleString() : 'never'}
            />
          </dl>

          {status.last_error && (
            <p className="rounded-lg bg-red-500/10 px-3 py-2 text-xs text-red-400">
              Last sync failed: {status.last_error}
            </p>
          )}

          <div className="flex gap-2">
            <button className="btn-ghost flex-1 py-2 text-sm" onClick={sync} disabled={busy}>
              {busy ? 'Syncing…' : 'Sync now'}
            </button>
            <button
              className="btn-ghost flex-1 border-red-500/30 py-2 text-sm text-red-400"
              onClick={disconnect}
              disabled={busy}
            >
              Disconnect
            </button>
          </div>
          <p className="text-xs text-ink-600">
            Strava does not expose resting heart rate — enter that on the main page. Average heart
            rate per activity is imported and charted there.
          </p>
        </>
      )}

      {note && <p className="text-sm text-moss-400">{note}</p>}
      {error && <p className="text-sm text-red-400">{error}</p>}
    </section>
  )
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between">
      <dt className="text-ink-500">{label}</dt>
      <dd className="font-mono text-ink-300">{value}</dd>
    </div>
  )
}
