import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../lib/api'
import type { Day, Entry, GridTask } from '../lib/types'
import { TaskIcon } from './TaskIcon'

interface Props {
  programId: number
  task: GridTask
  dayNumber: number
  isFuture: boolean
  onClose: () => void
  onSaved: () => void
}

/**
 * The detail behind a grid cell: one activity on one day.
 *
 * Opened by a long press or a right-click, so it is where the occasional
 * things live — logging an exact amount, leaving a note, checking what was
 * recorded — rather than the daily tick.
 */
export function CellDetailSheet({ programId, task, dayNumber, isFuture, onClose, onSaved }: Props) {
  const [day, setDay] = useState<Day | null>(null)
  const [entry, setEntry] = useState<Entry | null>(null)
  const [value, setValue] = useState('')
  const [note, setNote] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    api
      .getDay(programId, dayNumber)
      .then((d) => {
        if (cancelled) return
        setDay(d)
        const e = d.entries.find((x) => x.task_id === task.task_id) ?? null
        setEntry(e)
        setValue(e?.value != null ? String(e.value) : '')
        setNote(e?.note ?? '')
      })
      .catch((err) => setError(err instanceof Error ? err.message : 'Could not load that day'))
    return () => {
      cancelled = true
    }
  }, [programId, dayNumber, task.task_id])

  const metered = task.kind === 'number' || task.kind === 'duration'

  async function save(done?: boolean) {
    setBusy(true)
    setError('')
    try {
      const body: { done?: boolean; value?: number; note?: string } = { note }
      if (metered && value.trim() !== '') body.value = Number(value)
      if (done !== undefined) body.done = done
      await api.toggleTask(programId, dayNumber, task.task_id, body)
      onSaved()
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not save')
      setBusy(false)
    }
  }

  const dateLabel = day
    ? new Date(`${day.date}T12:00:00`).toLocaleDateString(undefined, {
        weekday: 'long',
        month: 'long',
        day: 'numeric',
      })
    : ''

  return (
    <div className="fixed inset-0 z-50 flex items-end justify-center">
      <button className="absolute inset-0 bg-black/70 backdrop-blur-sm" onClick={onClose} aria-label="Close" />

      <div className="animate-slide-up relative max-h-[85vh] w-full max-w-lg overflow-y-auto rounded-t-3xl border-t border-ink-800 bg-ink-900 p-5 pb-8">
        <div className="mx-auto mb-5 h-1 w-10 rounded-full bg-ink-700" />

        <header className="mb-4 flex items-center gap-3">
          <span
            className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl"
            style={{ backgroundColor: `${task.color}22`, color: task.color }}
          >
            <TaskIcon name={task.icon} size={20} />
          </span>
          <div className="min-w-0 flex-1">
            <h2 className="truncate text-lg font-semibold text-ink-100">{task.title}</h2>
            <p className="text-sm text-ink-500">
              Day {dayNumber}
              {dateLabel && ` · ${dateLabel}`}
            </p>
          </div>
        </header>

        {isFuture ? (
          <div className="rounded-xl bg-ink-850 p-4 text-center text-sm text-ink-400">
            This day hasn&apos;t arrived yet.
          </div>
        ) : !day ? (
          <div className="h-32 animate-pulse rounded-xl bg-ink-850" />
        ) : (
          <div className="space-y-4">
            {metered && (
              <div>
                <label className="label" htmlFor="cell-value">
                  Amount {task.unit && <span className="text-ink-600">({task.unit})</span>}
                </label>
                <div className="flex items-center gap-2">
                  <input
                    id="cell-value"
                    type="number"
                    inputMode="decimal"
                    className="field text-center"
                    value={value}
                    onChange={(e) => setValue(e.target.value)}
                  />
                  {task.target_num != null && (
                    <button
                      className="btn-ghost shrink-0 px-3 py-3 text-sm"
                      onClick={() => setValue(String(task.target_num))}
                    >
                      Target
                    </button>
                  )}
                </div>
                {task.target_num != null && (
                  <p className="mt-1.5 text-xs text-ink-600">
                    Completes at {task.target_num} {task.unit}
                  </p>
                )}
              </div>
            )}

            {task.kind === 'photo' && (
              <div className="rounded-xl bg-ink-850 p-4 text-sm text-ink-400">
                {day.photos.length > 0
                  ? `${day.photos.length} photo${day.photos.length === 1 ? '' : 's'} on this day.`
                  : 'No photo yet.'}{' '}
                <Link
                  to={`/app/day/${dayNumber}`}
                  className="text-flame-500 hover:text-flame-400"
                  onClick={onClose}
                >
                  Open the day to add one
                </Link>
                .
              </div>
            )}

            <div>
              <label className="label" htmlFor="cell-note">Note</label>
              <textarea
                id="cell-note"
                rows={3}
                className="field resize-none"
                placeholder="Anything worth remembering about this one"
                value={note}
                onChange={(e) => setNote(e.target.value)}
              />
            </div>

            {error && <p className="text-sm text-red-400">{error}</p>}

            <div className="flex gap-3">
              {entry?.done ? (
                <button className="btn-ghost flex-1" disabled={busy} onClick={() => save(false)}>
                  Mark not done
                </button>
              ) : (
                <button className="btn-ghost flex-1" disabled={busy} onClick={onClose}>
                  Cancel
                </button>
              )}
              <button
                className="btn-primary flex-1"
                disabled={busy || task.kind === 'photo'}
                onClick={() => save(metered ? undefined : true)}
              >
                {busy ? 'Saving…' : entry?.done ? 'Save' : 'Mark done'}
              </button>
            </div>

            <Link
              to={`/app/day/${dayNumber}`}
              onClick={onClose}
              className="block pt-1 text-center text-sm text-ink-500 hover:text-ink-300"
            >
              Open the full day
            </Link>
          </div>
        )}
      </div>
    </div>
  )
}
