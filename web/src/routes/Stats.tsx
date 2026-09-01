import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { TaskIcon } from '../components/TaskIcon'
import { ApiError, api } from '../lib/api'
import type { Stats } from '../lib/types'

export function StatsPage() {
  const navigate = useNavigate()
  const [stats, setStats] = useState<Stats | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api
      .stats()
      .then(setStats)
      .catch((err) => {
        if (err instanceof ApiError && err.code === 'no_active_program') navigate('/start', { replace: true })
      })
      .finally(() => setLoading(false))
  }, [navigate])

  if (loading) return <div className="h-96 animate-pulse rounded-2xl bg-ink-900" />
  if (!stats) return null

  return (
    <div className="space-y-5">
      <header className="animate-slide-up">
        <h1 className="text-2xl font-semibold text-ink-100">Progress</h1>
        <p className="mt-1 text-sm text-ink-500">
          Day {stats.current_day} of {stats.length_days}
        </p>
      </header>

      <div className="card p-5">
        <div className="mb-2 flex items-baseline justify-between">
          <span className="text-3xl font-semibold text-ink-100">
            {Math.round(stats.percent_done)}
            <span className="text-lg text-ink-500">%</span>
          </span>
          <span className="text-sm text-ink-500">
            {stats.days_complete} of {stats.length_days} days
          </span>
        </div>
        <div className="h-2 overflow-hidden rounded-full bg-ink-800">
          <div
            className="h-full rounded-full bg-gradient-to-r from-flame-500 to-moss-500 transition-all duration-700"
            style={{ width: `${stats.percent_done}%` }}
          />
        </div>
      </div>

      <div className="grid grid-cols-2 gap-3">
        <Tile label="Current streak" value={stats.streak} unit="days" accent />
        <Tile label="Best streak" value={stats.best_streak} unit="days" />
        <Tile label="Days missed" value={stats.days_missed} unit="" />
        <Tile label="Photos" value={stats.total_photos} unit="" />
        <Tile label="Workouts" value={stats.total_workouts} unit="" />
        <Tile label="Training" value={Math.round(stats.total_minutes / 60)} unit="hrs" />
      </div>

      {stats.avg_kcal > 0 && (
        <div className="card p-4">
          <p className="text-sm text-ink-500">Average intake on logged days</p>
          <p className="mt-1 font-mono text-2xl text-ink-100">
            {Math.round(stats.avg_kcal)}
            <span className="ml-1 text-sm text-ink-500">kcal</span>
          </p>
        </div>
      )}

      {stats.task_completion.length > 0 && (
        <section className="card p-4">
          <h2 className="mb-4 text-sm font-medium uppercase tracking-wide text-ink-500">
            Task consistency
          </h2>
          <div className="space-y-4">
            {stats.task_completion.map((t) => (
              <div key={t.task_id}>
                <div className="mb-1.5 flex items-center gap-2">
                  <span className="text-ink-500">
                    <TaskIcon name={t.icon} size={16} />
                  </span>
                  <span className="min-w-0 flex-1 truncate text-sm text-ink-300">{t.title}</span>
                  <span className="shrink-0 font-mono text-sm text-ink-400">{Math.round(t.rate)}%</span>
                </div>
                <div className="h-1.5 overflow-hidden rounded-full bg-ink-800">
                  <div
                    className={`h-full rounded-full transition-all duration-700 ${
                      t.rate >= 90 ? 'bg-moss-500' : t.rate >= 60 ? 'bg-flame-500' : 'bg-red-500'
                    }`}
                    style={{ width: `${t.rate}%` }}
                  />
                </div>
              </div>
            ))}
          </div>
        </section>
      )}

      {stats.weight_series.length > 1 && <WeightChart points={stats.weight_series} />}
    </div>
  )
}

function Tile({
  label,
  value,
  unit,
  accent,
}: {
  label: string
  value: number
  unit: string
  accent?: boolean
}) {
  return (
    <div className="card p-4">
      <p className="text-xs text-ink-500">{label}</p>
      <p className={`mt-1 font-mono text-2xl ${accent ? 'text-flame-500' : 'text-ink-100'}`}>
        {value}
        {unit && <span className="ml-1 text-sm text-ink-600">{unit}</span>}
      </p>
    </div>
  )
}

/** A minimal inline sparkline — no charting library for a single series. */
function WeightChart({ points }: { points: Stats['weight_series'] }) {
  const w = 320
  const h = 120
  const pad = 8

  const weights = points.map((p) => p.weight_kg)
  const min = Math.min(...weights)
  const max = Math.max(...weights)
  const range = max - min || 1
  const days = points.map((p) => p.day_number)
  const dayMin = Math.min(...days)
  const dayRange = Math.max(...days) - dayMin || 1

  const coords = points.map((p) => ({
    x: pad + ((p.day_number - dayMin) / dayRange) * (w - pad * 2),
    y: pad + (1 - (p.weight_kg - min) / range) * (h - pad * 2),
  }))
  const path = coords.map((c, i) => `${i === 0 ? 'M' : 'L'}${c.x.toFixed(1)},${c.y.toFixed(1)}`).join(' ')
  const first = points[0]
  const last = points[points.length - 1]
  const delta = last.weight_kg - first.weight_kg

  return (
    <section className="card p-4">
      <div className="mb-3 flex items-baseline justify-between">
        <h2 className="text-sm font-medium uppercase tracking-wide text-ink-500">Weight</h2>
        <span className={`font-mono text-sm ${delta <= 0 ? 'text-moss-400' : 'text-flame-400'}`}>
          {delta > 0 ? '+' : ''}
          {delta.toFixed(1)} kg
        </span>
      </div>
      <svg viewBox={`0 0 ${w} ${h}`} className="h-32 w-full overflow-visible">
        <path d={path} fill="none" stroke="#ff6b35" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
        {coords.map((c, i) => (
          <circle key={i} cx={c.x} cy={c.y} r="2.5" fill="#ff6b35" />
        ))}
      </svg>
      <div className="mt-1 flex justify-between font-mono text-xs text-ink-600">
        <span>
          {first.weight_kg} kg · day {first.day_number}
        </span>
        <span>
          {last.weight_kg} kg · day {last.day_number}
        </span>
      </div>
    </section>
  )
}
