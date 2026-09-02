import { useState } from 'react'
import { api } from '../lib/api'
import type { Day, Entry } from '../lib/types'

interface Props {
  entry: Entry
  day: Day
  onChanged: () => void
  onLogMeal: () => void
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
  onLogMeal: () => void
  color: string
}) {
  const { totals, meals } = day
  const target = totals.kcal_target ?? 0
  const pct = target > 0 ? Math.min(100, (totals.kcal / target) * 100) : 0

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
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm text-ink-200">{meal.name || meal.slot}</span>
                <span className="text-xs capitalize text-ink-600">
                  {meal.slot}
                  {meal.source === 'ai' && ' · AI estimate'}
                </span>
              </span>
              <span className="shrink-0 font-mono text-sm text-ink-400">{Math.round(meal.kcal)}</span>
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

      <button className="btn-ghost w-full py-2 text-sm" onClick={onLogMeal}>
        + Log a meal
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
