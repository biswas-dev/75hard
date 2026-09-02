import { useEffect, useState } from 'react'
import { api } from '../lib/api'
import type { Day } from '../lib/types'

interface Props {
  programId: number
  day: Day
  onSaved: () => void
}

/**
 * The optional morning numbers: weight, and resting pulse.
 *
 * Neither is part of the challenge and neither gates anything — they are here
 * because 75 days is long enough for both to move, and a number you only log
 * on day 1 and day 75 tells you far less than one you log most mornings.
 *
 * Resting HR is typed in rather than imported because Strava does not have it:
 * its API gives average and max heart rate per activity, while true resting HR
 * lives in Garmin, Fitbit or Apple Health.
 */
export function DailyVitals({ programId, day, onSaved }: Props) {
  const [weight, setWeight] = useState('')
  const [hr, setHr] = useState('')
  const [busy, setBusy] = useState(false)
  const [saved, setSaved] = useState(false)
  const [error, setError] = useState('')

  // Re-seed when the day changes, so moving between days does not carry
  // yesterday's reading into today's empty field.
  useEffect(() => {
    setWeight(day.weight_kg != null ? String(day.weight_kg) : '')
    setHr(day.resting_hr != null ? String(day.resting_hr) : '')
    setSaved(false)
    setError('')
  }, [day.id, day.weight_kg, day.resting_hr])

  const dirty =
    weight !== (day.weight_kg != null ? String(day.weight_kg) : '') ||
    hr !== (day.resting_hr != null ? String(day.resting_hr) : '')

  async function save() {
    if (busy) return
    setBusy(true)
    setError('')

    try {
      const body: { weight_kg?: number; resting_hr?: number } = {}
      // Zero clears the stored value rather than recording a 0kg weigh-in.
      if (weight.trim() !== '') body.weight_kg = Number(weight)
      else if (day.weight_kg != null) body.weight_kg = 0

      if (hr.trim() !== '') body.resting_hr = Number(hr)
      else if (day.resting_hr != null) body.resting_hr = 0

      await api.updateDay(programId, day.day_number, body)
      setSaved(true)
      onSaved()
      window.setTimeout(() => setSaved(false), 2500)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not save')
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="card p-4">
      <div className="mb-3 flex items-baseline justify-between">
        <h2 className="text-sm font-medium uppercase tracking-wide text-ink-500">Today's numbers</h2>
        <span className="text-xs text-ink-600">Optional</span>
      </div>

      <div className="grid grid-cols-2 gap-3">
        <label className="block">
          <span className="mb-1 block text-xs text-ink-500">Weight (kg)</span>
          <input
            type="number"
            inputMode="decimal"
            step="0.1"
            min="20"
            max="400"
            value={weight}
            onChange={(e) => setWeight(e.target.value)}
            placeholder="—"
            className="w-full rounded-lg border border-ink-800 bg-ink-850 px-3 py-2 font-mono text-sm text-ink-200 placeholder:text-ink-700"
          />
        </label>

        <label className="block">
          <span className="mb-1 block text-xs text-ink-500">Resting pulse</span>
          <input
            type="number"
            inputMode="numeric"
            step="1"
            min="25"
            max="200"
            value={hr}
            onChange={(e) => setHr(e.target.value)}
            placeholder="—"
            className="w-full rounded-lg border border-ink-800 bg-ink-850 px-3 py-2 font-mono text-sm text-ink-200 placeholder:text-ink-700"
          />
        </label>
      </div>

      <p className="mt-2 text-xs text-ink-600">
        Take your pulse before getting up — that is the reading that shows fitness improving.
      </p>

      {dirty && (
        <button className="btn-ghost mt-3 w-full py-2 text-sm" onClick={save} disabled={busy}>
          {busy ? 'Saving…' : 'Save'}
        </button>
      )}
      {saved && !dirty && <p className="mt-2 text-center text-xs text-moss-400">Saved</p>}
      {error && <p className="mt-2 text-center text-xs text-red-400">{error}</p>}
    </section>
  )
}
