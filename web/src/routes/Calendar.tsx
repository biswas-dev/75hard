import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { ApiError, api } from '../lib/api'
import type { DaySummary, Program } from '../lib/types'

export function Calendar() {
  const navigate = useNavigate()
  const [program, setProgram] = useState<Program | null>(null)
  const [days, setDays] = useState<DaySummary[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api
      .activeProgram()
      .then(async (p) => {
        setProgram(p)
        setDays(await api.listDays(p.id))
      })
      .catch((err) => {
        if (err instanceof ApiError && err.code === 'no_active_program') navigate('/start', { replace: true })
      })
      .finally(() => setLoading(false))
  }, [navigate])

  if (loading) {
    return <div className="h-96 animate-pulse rounded-2xl bg-ink-900" />
  }
  if (!program) return null

  const currentDay = program.current_day

  return (
    <div className="space-y-5">
      <header className="animate-slide-up">
        <h1 className="text-2xl font-semibold text-ink-100">{program.name}</h1>
        <p className="mt-1 text-sm text-ink-500">
          Started {new Date(`${program.start_date}T12:00:00`).toLocaleDateString(undefined, {
            month: 'long',
            day: 'numeric',
            year: 'numeric',
          })}
          {program.attempt_number > 1 && ` · attempt ${program.attempt_number}`}
        </p>
      </header>

      <div className="grid grid-cols-7 gap-2">
        {days.map((d) => {
          const isFuture = d.day_number > currentDay
          const isToday = d.day_number === currentDay
          const pct = d.tasks_total > 0 ? d.tasks_done / d.tasks_total : 0

          return (
            <Link
              key={d.day_number}
              to={`/app/day/${d.day_number}`}
              aria-label={`Day ${d.day_number}, ${d.status}`}
              className={`relative flex aspect-square items-center justify-center rounded-xl border text-sm transition active:scale-95 ${
                d.status === 'complete'
                  ? 'border-moss-500/40 bg-moss-500/15 text-moss-300'
                  : d.status === 'missed'
                    ? 'border-red-500/30 bg-red-500/10 text-red-400'
                    : isFuture
                      ? 'border-ink-850 bg-ink-900/50 text-ink-700'
                      : 'border-ink-800 bg-ink-900 text-ink-400'
              } ${isToday ? 'ring-2 ring-flame-500 ring-offset-2 ring-offset-ink-950' : ''}`}
            >
              {d.day_number}

              {/* Partial progress on a day that isn't finished yet. */}
              {d.status === 'pending' && pct > 0 && (
                <span className="absolute inset-x-1.5 bottom-1.5 h-0.5 overflow-hidden rounded-full bg-ink-800">
                  <span className="block h-full bg-flame-500" style={{ width: `${pct * 100}%` }} />
                </span>
              )}
            </Link>
          )
        })}
      </div>

      <div className="flex flex-wrap gap-4 pt-2 text-xs text-ink-500">
        <Legend className="bg-moss-500/20 border-moss-500/40" label="Complete" />
        <Legend className="bg-red-500/15 border-red-500/30" label="Missed" />
        <Legend className="bg-ink-900 border-ink-800" label="Pending" />
        <Legend className="bg-ink-900 border-flame-500" label="Today" />
      </div>
    </div>
  )
}

function Legend({ className, label }: { className: string; label: string }) {
  return (
    <span className="flex items-center gap-2">
      <span className={`h-3.5 w-3.5 rounded border ${className}`} />
      {label}
    </span>
  )
}
