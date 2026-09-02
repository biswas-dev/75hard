import { TrendChart } from './TrendChart'
import { roundWeight, toDisplay, unitFor } from '../lib/units'
import { useAuth } from '../state/AuthContext'
import type { Summary, Trend } from '../lib/types'

/**
 * The whole picture, on the page you open every day.
 *
 * Ordered by how often the answer is wanted: where the streak stands, then the
 * volume behind it, then the two measurements that only mean anything as a
 * trend.
 */
export function SummaryPanel({ summary }: { summary: Summary }) {
  const { user } = useAuth()
  const unit = unitFor(user)

  // The series arrives in kilograms; convert once, here, so the chart, the
  // delta and the summary line all agree.
  const weightPoints = summary.vitals
    .filter((v) => v.weight_kg != null)
    .map((v) => ({ x: v.day_number, y: toDisplay(v.weight_kg as number, unit) }))
  const hrPoints = summary.vitals
    .filter((v) => v.resting_hr != null)
    .map((v) => ({ x: v.day_number, y: v.resting_hr as number }))
  const activityHrPoints = summary.heart_rate.map((p) => ({ x: p.day_number, y: p.average_hr }))

  const hours = Math.floor(summary.total_minutes / 60)
  const minutes = summary.total_minutes % 60

  return (
    <section className="space-y-4">
      <div className="card p-4">
        <div className="mb-3 flex items-baseline justify-between">
          <h2 className="text-sm font-medium uppercase tracking-wide text-ink-500">Progress</h2>
          <span className="font-mono text-sm text-ink-300">
            day {summary.current_day}
            <span className="text-ink-600"> / {summary.length_days}</span>
          </span>
        </div>

        <div className="mb-4 h-2 overflow-hidden rounded-full bg-ink-800">
          <div
            className="h-full rounded-full bg-flame-500 transition-all duration-700"
            style={{ width: `${Math.min(100, summary.percent_done)}%` }}
          />
        </div>

        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <Stat label="Streak" value={summary.streak} suffix="days" accent />
          <Stat label="Best streak" value={summary.best_streak} suffix="days" />
          <Stat label="Complete" value={summary.days_complete} suffix={`of ${summary.length_days}`} />
          <Stat label="Missed" value={summary.days_missed} suffix="days" />
        </div>
      </div>

      <div className="card p-4">
        <h2 className="mb-3 text-sm font-medium uppercase tracking-wide text-ink-500">Volume</h2>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <Stat label="Training" value={hours} suffix={`h ${minutes}m`} />
          <Stat label="Outdoors" value={Math.round(summary.outdoor_minutes / 60)} suffix="hours" />
          <Stat label="Sessions" value={summary.total_workouts} suffix="logged" />
          <Stat label="Photos" value={summary.total_photos} suffix="taken" />
        </div>
        {(summary.meditation_minutes > 0 || summary.avg_kcal > 0) && (
          <div className="mt-3 grid grid-cols-2 gap-3 border-t border-ink-800 pt-3">
            {summary.avg_kcal > 0 && (
              <Stat label="Average intake" value={Math.round(summary.avg_kcal)} suffix="kcal/day" />
            )}
            {summary.meditation_minutes > 0 && (
              <Stat label="Meditation" value={summary.meditation_minutes} suffix="minutes" />
            )}
          </div>
        )}
      </div>

      {(weightPoints.length > 0 || hrPoints.length > 0 || activityHrPoints.length > 0) && (
        <div className="card space-y-5 p-4">
          <h2 className="text-sm font-medium uppercase tracking-wide text-ink-500">Trends</h2>

          {weightPoints.length > 0 && (
            <div>
              <TrendChart points={weightPoints} color="#ff6b35" unit={unit} label="Weight" />
              <TrendSummary trend={summary.weight} unit={unit} convert />
            </div>
          )}

          {hrPoints.length > 0 && (
            <div>
              <TrendChart points={hrPoints} color="#4a9eff" unit="bpm" label="Resting pulse" />
              <TrendSummary trend={summary.resting_hr} unit="bpm" />
            </div>
          )}

          {activityHrPoints.length > 0 && (
            <div>
              <TrendChart
                points={activityHrPoints}
                color="#b47aea"
                unit="bpm"
                label="Training heart rate"
              />
              <p className="mt-1 text-xs text-ink-600">
                Average heart rate while training, from Strava. Falling at the same effort means
                fitness improving.
              </p>
            </div>
          )}
        </div>
      )}
    </section>
  )
}

/**
 * `convert` marks a trend whose numbers are kilograms and must be shown in the
 * reader's unit. Resting pulse is already in its own unit and passes through.
 */
function TrendSummary({
  trend,
  unit,
  convert,
}: {
  trend: Trend
  unit: string
  convert?: boolean
}) {
  if (trend.count < 2) return null

  const show = (value?: number) => {
    if (value == null) return '—'
    return convert
      ? roundWeight(toDisplay(value, unit as 'kg' | 'lb'), unit as 'kg' | 'lb')
      : value
  }

  return (
    <p className="mt-1 font-mono text-xs text-ink-600">
      best {show(trend.best)} {unit} · average {show(trend.average)} {unit} · {trend.count} readings
    </p>
  )
}

function Stat({
  label,
  value,
  suffix,
  accent,
}: {
  label: string
  value: number
  suffix?: string
  accent?: boolean
}) {
  return (
    <div>
      <p className="text-xs text-ink-500">{label}</p>
      <p className={`font-mono text-xl ${accent ? 'text-flame-400' : 'text-ink-200'}`}>
        {value}
        {suffix && <span className="ml-1 text-xs text-ink-600">{suffix}</span>}
      </p>
    </div>
  )
}
