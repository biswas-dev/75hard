import { useEffect, useState } from 'react'
import { api } from '../lib/api'
import { PhotoUpload } from './PhotoUpload'
import type { Day, Entry, Meal } from '../lib/types'

interface Props {
  entry: Entry
  day: Day
  onChanged: () => void
  onLogMeal: (meal?: Meal) => void
}

/**
 * The optional panel behind a task.
 *
 * "Follow the diet" is the motivating case: some days you want to record every
 * meal and watch the calorie total, most days you ate on plan and want to say
 * so and move on. The tick alone always completes the task — this is additive,
 * and never gates anything.
 */
export function TaskTracker({ entry, day, onChanged, onLogMeal }: Props) {
  if (entry.tracker === 'nutrition') {
    return <NutritionTracker day={day} onChanged={onChanged} onLogMeal={onLogMeal} color={entry.color} />
  }
  if (entry.tracker === 'workout') {
    return <WorkoutTracker entry={entry} day={day} onChanged={onChanged} />
  }
  if (entry.tracker === 'meditation') {
    return <MeditationTracker entry={entry} day={day} onChanged={onChanged} />
  }
  return null
}

function NutritionTracker({
  day,
  onChanged,
  onLogMeal,
  color,
}: {
  day: Day
  onChanged: () => void
  onLogMeal: (meal?: Meal) => void
  color: string
}) {
  const { totals, meals } = day
  const target = totals.kcal_target ?? 0
  const pct = target > 0 ? Math.min(100, (totals.kcal / target) * 100) : 0

  // A background estimate has no way to push, so poll while one is running.
  // Stops as soon as nothing is pending, so an idle panel makes no requests.
  const pending = meals.some((m) => m.estimate_status === 'pending')
  useEffect(() => {
    if (!pending) return
    const id = window.setInterval(onChanged, 4000)
    return () => window.clearInterval(id)
  }, [pending, onChanged])

  return (
    <div className="space-y-4">
      <div>
        <div className="mb-1.5 flex items-baseline justify-between">
          <span className="text-sm text-ink-400">Today</span>
          <span className="font-mono text-sm text-ink-200">
            {Math.round(totals.kcal)}
            {target > 0 && <span className="text-ink-600"> / {target}</span>}
            <span className="text-ink-600"> kcal</span>
          </span>
        </div>
        {target > 0 && (
          <div className="h-1.5 overflow-hidden rounded-full bg-ink-800">
            <div
              className="h-full rounded-full transition-all duration-500"
              style={{ width: `${pct}%`, backgroundColor: color }}
            />
          </div>
        )}
      </div>

      <div className="grid grid-cols-3 gap-2 text-center">
        <Macro label="Protein" value={totals.protein_g} />
        <Macro label="Carbs" value={totals.carbs_g} />
        <Macro label="Fat" value={totals.fat_g} />
      </div>

      {meals.length > 0 && (
        <ul className="divide-y divide-ink-800">
          {meals.map((meal) => (
            <li key={meal.id} className="flex items-center gap-3 py-2">
              <button
                type="button"
                onClick={() => onLogMeal(meal)}
                className="min-w-0 flex-1 text-left"
                title="Edit this meal"
              >
                <span className="block truncate text-sm text-ink-200">
                  {meal.name || (meal.estimate_status === 'pending' ? 'Reading the photo…' : meal.slot)}
                </span>
                <span className="text-xs capitalize text-ink-600">
                  {meal.slot}
                  {meal.estimate_status === 'done' && ' · AI estimate'}
                  {meal.estimate_status === 'failed' && (
                    <>
                      {' · '}
                      <button
                        className="text-flame-400 hover:underline"
                        title={meal.estimate_error}
                        onClick={async () => {
                          await api.retryEstimate(meal.id)
                          onChanged()
                        }}
                      >
                        estimate failed — retry
                      </button>
                    </>
                  )}
                  {meal.estimate_status === '' && meal.source === 'ai' && ' · AI estimate'}
                </span>
              </button>
              {/* A pending estimate must not render as a real zero. */}
              <span className="shrink-0 font-mono text-sm text-ink-400">
                {meal.estimate_status === 'pending' ? <PulseDots /> : Math.round(meal.kcal)}
              </span>
              <button
                aria-label={`Remove ${meal.name || 'meal'}`}
                className="shrink-0 px-1 text-ink-600 hover:text-red-400"
                onClick={async () => {
                  await api.deleteMeal(meal.id)
                  onChanged()
                }}
              >
                ×
              </button>
            </li>
          ))}
        </ul>
      )}

      {/* The fast path: one photo, tagged by the clock, priced in the
          background. The meal sheet below is for when you want to be exact. */}
      <PhotoUpload
        kind="food"
        dayNumber={day.day_number}
        label="Snap a meal"
        withSlot
        autolog
        onUploaded={onChanged}
      />

      <button className="btn-ghost w-full py-2 text-sm" onClick={() => onLogMeal()}>
        + Log a meal in detail
      </button>
      <p className="text-center text-xs text-ink-600">
        Optional. Ticking the task is enough on its own.
      </p>
    </div>
  )
}

function WorkoutTracker({ entry, day, onChanged }: { entry: Entry; day: Day; onChanged: () => void }) {
  // The outdoor task tracks outdoor sessions; anything else tracks indoor.
  const kind: 'indoor' | 'outdoor' = entry.key === 'workout_outdoor' ? 'outdoor' : 'indoor'
  const sessions = day.workouts.filter((w) => w.kind === kind)
  const logged = sessions.reduce((n, w) => n + w.minutes, 0)

  const [activity, setActivity] = useState('')
  const [minutes, setMinutes] = useState('')
  const [busy, setBusy] = useState(false)

  async function add() {
    const mins = Number(minutes)
    if (!Number.isFinite(mins) || mins <= 0) return
    setBusy(true)
    try {
      await api.createWorkout({
        day_number: day.day_number,
        kind,
        activity: activity.trim(),
        minutes: mins,
        // Crediting the task means the logged minutes drive completion, so
        // two short sessions can add up to the target.
        task_id: entry.task_id,
      })
      setActivity('')
      setMinutes('')
      onChanged()
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="space-y-4">
      <p className="text-sm text-ink-400">
        {logged} min logged {kind}
        {entry.target_num != null && <span className="text-ink-600"> of {entry.target_num}</span>}
      </p>

      {sessions.length > 0 && (
        <ul className="divide-y divide-ink-800">
          {sessions.map((w) => (
            <li key={w.id} className="flex items-center gap-3 py-2">
              <span className="min-w-0 flex-1 truncate text-sm text-ink-200">
                {w.activity || 'Session'}
              </span>
              <span className="shrink-0 font-mono text-sm text-ink-400">{w.minutes} min</span>
              <button
                aria-label="Remove session"
                className="shrink-0 px-1 text-ink-600 hover:text-red-400"
                onClick={async () => {
                  await api.deleteWorkout(w.id)
                  onChanged()
                }}
              >
                ×
              </button>
            </li>
          ))}
        </ul>
      )}

      <div className="flex gap-2">
        <input
          className="field flex-1 py-2"
          placeholder={kind === 'outdoor' ? 'Walk, run, ride…' : 'Lift, rows, yoga…'}
          value={activity}
          onChange={(e) => setActivity(e.target.value)}
        />
        <input
          type="number"
          inputMode="numeric"
          className="field w-24 py-2 text-center"
          placeholder="min"
          value={minutes}
          onChange={(e) => setMinutes(e.target.value)}
        />
      </div>
      <button className="btn-ghost w-full py-2 text-sm" onClick={add} disabled={busy || !minutes}>
        + Log session
      </button>
      <p className="text-center text-xs text-ink-600">
        Optional. Ticking the task is enough on its own.
      </p>
    </div>
  )
}

function Macro({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-xl bg-ink-850 py-2">
      <p className="font-mono text-sm text-ink-200">
        {Math.round(value)}
        <span className="text-xs text-ink-600">g</span>
      </p>
      <p className="text-[11px] text-ink-600">{label}</p>
    </div>
  )
}

/** The apps people actually use, offered as shortcuts. Anything can be typed. */
const MEDITATION_SOURCES = ['Calm', 'Headspace', 'Waking Up', 'Muse', 'Insight Timer', 'Unguided']

const MEDITATION_STYLES = [
  { value: 'guided', label: 'Guided' },
  { value: 'unguided', label: 'Unguided' },
  { value: 'breathwork', label: 'Breathwork' },
  { value: 'body_scan', label: 'Body scan' },
  { value: 'walking', label: 'Walking' },
  { value: 'other', label: 'Other' },
]

/**
 * Meditation is the one tracker behind an optional task.
 *
 * It is not one of the six rules, so missing it never fails a run — the panel
 * says so plainly, because a tracker that looks mandatory changes how the
 * whole challenge feels.
 */
function MeditationTracker({ entry, day, onChanged }: { entry: Entry; day: Day; onChanged: () => void }) {
  const [minutes, setMinutes] = useState(10)
  const [source, setSource] = useState('')
  const [style, setStyle] = useState('guided')
  const [busy, setBusy] = useState(false)

  const sessions = day.meditations ?? []
  const total = day.totals.meditation_minutes ?? 0

  async function add() {
    if (busy || minutes <= 0) return
    setBusy(true)
    try {
      await api.createMeditation({
        day_number: day.day_number,
        minutes,
        source: source.trim(),
        style,
        task_id: entry.task_id,
      })
      setSource('')
      onChanged()
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-baseline justify-between">
        <span className="text-sm text-ink-400">Today</span>
        <span className="font-mono text-sm text-ink-200">
          {total}
          <span className="text-ink-600"> min</span>
        </span>
      </div>

      {sessions.length > 0 && (
        <ul className="divide-y divide-ink-800">
          {sessions.map((m) => (
            <li key={m.id} className="flex items-center gap-3 py-2">
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm text-ink-200">
                  {m.source || MEDITATION_STYLES.find((s) => s.value === m.style)?.label || 'Session'}
                </span>
                <span className="text-xs text-ink-600">
                  {MEDITATION_STYLES.find((s) => s.value === m.style)?.label ?? m.style}
                </span>
              </span>
              <span className="shrink-0 font-mono text-sm text-ink-400">{m.minutes} min</span>
              <button
                aria-label="Remove session"
                className="shrink-0 px-1 text-ink-600 hover:text-red-400"
                onClick={async () => {
                  await api.deleteMeditation(m.id)
                  onChanged()
                }}
              >
                ×
              </button>
            </li>
          ))}
        </ul>
      )}

      <div>
        <label className="mb-1.5 block text-xs text-ink-500" htmlFor="meditation-minutes">
          How long
        </label>
        <div className="flex flex-wrap gap-1.5">
          {[5, 10, 15, 20, 30].map((m) => (
            <button
              key={m}
              type="button"
              onClick={() => setMinutes(m)}
              className={`rounded-lg border px-3 py-1.5 text-sm transition ${
                minutes === m
                  ? 'border-flame-500 bg-flame-500/15 text-flame-400'
                  : 'border-ink-800 bg-ink-850 text-ink-400'
              }`}
            >
              {m}
            </button>
          ))}
          <input
            id="meditation-minutes"
            type="number"
            min={1}
            max={1440}
            value={minutes}
            onChange={(e) => setMinutes(Number(e.target.value))}
            aria-label="Minutes meditated"
            className="w-20 rounded-lg border border-ink-800 bg-ink-850 px-2 py-1.5 text-sm text-ink-200"
          />
        </div>
      </div>

      <div>
        <label className="mb-1.5 block text-xs text-ink-500" htmlFor="meditation-source">
          Where
        </label>
        <div className="mb-2 flex flex-wrap gap-1.5">
          {MEDITATION_SOURCES.map((s) => (
            <button
              key={s}
              type="button"
              onClick={() => setSource(source === s ? '' : s)}
              className={`rounded-lg border px-2.5 py-1 text-xs transition ${
                source === s
                  ? 'border-flame-500 bg-flame-500/15 text-flame-400'
                  : 'border-ink-800 bg-ink-850 text-ink-400'
              }`}
            >
              {s}
            </button>
          ))}
        </div>
        <input
          id="meditation-source"
          type="text"
          value={source}
          onChange={(e) => setSource(e.target.value)}
          placeholder="Or type anywhere else"
          className="w-full rounded-lg border border-ink-800 bg-ink-850 px-3 py-2 text-sm text-ink-200 placeholder:text-ink-600"
        />
      </div>

      <div className="flex flex-wrap gap-1.5">
        {MEDITATION_STYLES.map((s) => (
          <button
            key={s.value}
            type="button"
            onClick={() => setStyle(s.value)}
            className={`rounded-lg border px-2.5 py-1 text-xs transition ${
              style === s.value
                ? 'border-flame-500 bg-flame-500/15 text-flame-400'
                : 'border-ink-800 bg-ink-850 text-ink-400'
            }`}
          >
            {s.label}
          </button>
        ))}
      </div>

      <button className="btn-ghost w-full py-2 text-sm" onClick={add} disabled={busy || minutes <= 0}>
        {busy ? 'Saving…' : '+ Log a sitting'}
      </button>
      <p className="text-center text-xs text-ink-600">
        Optional — meditation is not one of the six rules, and skipping it never fails your run.
      </p>
    </div>
  )
}

/** Three dots, for a number that is still being worked out. */
function PulseDots() {
  return (
    <span className="inline-flex gap-0.5 align-middle" aria-label="Estimating">
      {[0, 150, 300].map((delay) => (
        <span
          key={delay}
          className="h-1 w-1 animate-pulse rounded-full bg-ink-500"
          style={{ animationDelay: `${delay}ms` }}
        />
      ))}
    </span>
  )
}
