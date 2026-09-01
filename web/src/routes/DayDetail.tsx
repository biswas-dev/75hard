import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { AuthImage } from '../components/AuthImage'
import { PhotoUpload } from '../components/PhotoUpload'
import { TaskRow } from '../components/TaskRow'
import { api } from '../lib/api'
import type { Day, Entry, Program } from '../lib/types'

/** A past (or future) day, reachable from the calendar. */
export function DayDetail() {
  const { dayNumber } = useParams()
  const navigate = useNavigate()
  const n = Number(dayNumber)

  const [program, setProgram] = useState<Program | null>(null)
  const [day, setDay] = useState<Day | null>(null)
  const [note, setNote] = useState('')
  const [weight, setWeight] = useState('')
  const [saved, setSaved] = useState(false)
  const [loading, setLoading] = useState(true)

  const load = useCallback(async () => {
    const p = await api.activeProgram()
    setProgram(p)
    const d = await api.getDay(p.id, n)
    setDay(d)
    setNote(d.note)
    setWeight(d.weight_kg != null ? String(d.weight_kg) : '')
    setLoading(false)
  }, [n])

  useEffect(() => {
    if (!Number.isFinite(n) || n < 1) {
      navigate('/app/calendar', { replace: true })
      return
    }
    load().catch(() => setLoading(false))
  }, [n, load, navigate])

  async function toggle(entry: Entry, next: { done?: boolean; value?: number }) {
    if (!program || !day) return
    setDay(await api.toggleTask(program.id, day.day_number, entry.task_id, next))
  }

  async function saveJournal() {
    if (!program || !day) return
    const body: { note?: string; weight_kg?: number } = { note }
    if (weight.trim() !== '') body.weight_kg = Number(weight)
    setDay(await api.updateDay(program.id, day.day_number, body))
    setSaved(true)
    window.setTimeout(() => setSaved(false), 2500)
  }

  if (loading) return <div className="h-96 animate-pulse rounded-2xl bg-ink-900" />
  if (!program || !day) return <p className="text-center text-ink-500">Day not found.</p>

  const isFuture = day.day_number > program.current_day

  return (
    <div className="space-y-5 pb-4">
      <header className="animate-slide-up flex items-center gap-3">
        <Link to="/app/calendar" className="btn-ghost h-10 w-10 rounded-xl p-0" aria-label="Back to calendar">
          ←
        </Link>
        <div className="min-w-0 flex-1">
          <h1 className="text-2xl font-semibold text-ink-100">Day {day.day_number}</h1>
          <p className="text-sm text-ink-500">
            {new Date(`${day.date}T12:00:00`).toLocaleDateString(undefined, {
              weekday: 'long',
              month: 'long',
              day: 'numeric',
            })}
          </p>
        </div>
        <StatusBadge status={day.status} />
      </header>

      {isFuture ? (
        <div className="card p-6 text-center text-ink-500">
          <p>This day hasn&apos;t started yet.</p>
          <p className="mt-1 text-sm">Come back on {day.date}.</p>
        </div>
      ) : (
        <>
          <section className="space-y-3">
            {day.entries.map((entry) => (
              <TaskRow key={entry.task_id} entry={entry} onToggle={toggle} />
            ))}
          </section>

          <section className="card p-4">
            <h2 className="mb-3 text-sm font-medium uppercase tracking-wide text-ink-500">Photos</h2>
            {day.photos.length > 0 && (
              <div className="mb-3 grid grid-cols-3 gap-2">
                {day.photos.map((p) => (
                  <AuthImage
                    key={p.id}
                    src={p.thumb_url}
                    alt={`Photo from day ${day.day_number}`}
                    className="aspect-square w-full rounded-xl object-cover"
                  />
                ))}
              </div>
            )}
            <PhotoUpload
              kind="progress"
              dayNumber={day.day_number}
              label="Add a photo"
              onUploaded={() => load()}
            />
          </section>

          <section className="card space-y-4 p-4">
            <h2 className="text-sm font-medium uppercase tracking-wide text-ink-500">Journal</h2>
            <div>
              <label className="label" htmlFor="note">How did it go?</label>
              <textarea
                id="note"
                rows={4}
                className="field resize-none"
                placeholder="Notes on the day…"
                value={note}
                onChange={(e) => setNote(e.target.value)}
              />
            </div>
            <div>
              <label className="label" htmlFor="weight">Weight (kg)</label>
              <input
                id="weight"
                type="number"
                inputMode="decimal"
                step="0.1"
                className="field"
                placeholder="—"
                value={weight}
                onChange={(e) => setWeight(e.target.value)}
              />
            </div>
            <button className="btn-primary w-full" onClick={saveJournal}>
              {saved ? 'Saved' : 'Save'}
            </button>
          </section>
        </>
      )}
    </div>
  )
}

function StatusBadge({ status }: { status: Day['status'] }) {
  const map = {
    complete: 'bg-moss-500/15 text-moss-400 border-moss-500/25',
    missed: 'bg-red-500/15 text-red-400 border-red-500/25',
    pending: 'bg-ink-850 text-ink-400 border-ink-800',
  } as const
  return (
    <span className={`shrink-0 rounded-full border px-3 py-1 text-xs capitalize ${map[status]}`}>
      {status}
    </span>
  )
}
