import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { AuthImage } from '../components/AuthImage'
import { CoachNote } from '../components/CoachNote'
import { Confetti } from '../components/Confetti'
import { DailyVitals } from '../components/DailyVitals'
import { DeletablePhotos, POSE_LABEL } from '../components/PhotoTile'
import { PhotoUpload } from '../components/PhotoUpload'
import { SummaryPanel } from '../components/SummaryPanel'
import { TaskRow } from '../components/TaskRow'
import { ApiError, api } from '../lib/api'
import type { Day, Entry, Meal, Program, Summary } from '../lib/types'
import { MealSheet } from './MealSheet'

export function Today() {
  const navigate = useNavigate()
  const [program, setProgram] = useState<Program | null>(null)
  const [day, setDay] = useState<Day | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [confetti, setConfetti] = useState(false)
  const [mealOpen, setMealOpen] = useState(false)
  // The meal being edited, or null for a new one.
  const [editingMeal, setEditingMeal] = useState<Meal | null>(null)
  const [summary, setSummary] = useState<Summary | null>(null)
  // Which day is on screen. Null means today, and stays null so the page keeps
  // following the date over midnight rather than pinning to whatever day it
  // was when it loaded.
  const [viewDay, setViewDay] = useState<number | null>(null)

  // Tracks whether the day was already complete, so the celebration fires on
  // the transition rather than on every refetch of a finished day.
  const wasComplete = useRef(false)
  // Opening the app on an already-finished day should not celebrate; only a
  // transition that happens while you are looking at it should.
  const firstLoad = useRef(true)

  const load = useCallback(async () => {
    try {
      const p = await api.activeProgram()
      // Past days are fetched by number; today comes from its own endpoint so
      // it stays correct as the date rolls over.
      const d =
        viewDay === null || viewDay === p.current_day
          ? await api.today()
          : await api.getDay(p.id, viewDay)
      setProgram(p)

      // The summary is a nice-to-have beside the day itself: if it fails, the
      // page still works, so it is fetched separately and its error swallowed.
      api.getSummary().then(setSummary).catch(() => setSummary(null))

      // A refetch is how the day completes when the last task is a photo
      // upload, so the transition has to be detected here too — not only in
      // the check-off path.
      if (!firstLoad.current && d.status === 'complete' && !wasComplete.current) {
        setConfetti(true)
        navigator.vibrate?.([18, 60, 18])
      }
      firstLoad.current = false
      wasComplete.current = d.status === 'complete'
      setDay(d)
    } catch (err) {
      if (err instanceof ApiError && err.code === 'no_active_program') {
        navigate('/start', { replace: true })
        return
      }
      setError(err instanceof Error ? err.message : 'Could not load today')
    } finally {
      setLoading(false)
    }
  }, [navigate, viewDay])

  useEffect(() => {
    load()
  }, [load])

  function applyDay(next: Day) {
    if (next.status === 'complete' && !wasComplete.current) {
      setConfetti(true)
      navigator.vibrate?.([18, 60, 18])
    }
    wasComplete.current = next.status === 'complete'
    setDay(next)
  }

  async function toggle(entry: Entry, next: { done?: boolean; value?: number }) {
    if (!program || !day) return

    // Optimistic: the tap must feel instant, so the row flips before the
    // round trip and the server's answer reconciles it a moment later.
    const optimisticDone =
      next.done ?? (entry.target_num ? (next.value ?? 0) >= entry.target_num : false)
    const snapshot = day
    setDay({
      ...day,
      entries: day.entries.map((e) =>
        e.task_id === entry.task_id
          ? { ...e, done: optimisticDone, value: next.value ?? e.value }
          : e,
      ),
      tasks_done: day.tasks_done + (optimisticDone === entry.done ? 0 : optimisticDone ? 1 : -1),
    })

    try {
      applyDay(await api.toggleTask(program.id, day.day_number, entry.task_id, next))
    } catch (err) {
      setDay(snapshot)
      setError(err instanceof Error ? err.message : 'Could not save that')
    }
  }

  if (loading) {
    return (
      <div className="space-y-3">
        <div className="h-28 animate-pulse rounded-2xl bg-ink-900" />
        {[0, 1, 2, 3, 4, 5].map((i) => (
          <div key={i} className="h-20 animate-pulse rounded-2xl bg-ink-900" />
        ))}
      </div>
    )
  }

  if (!program || !day) {
    return <p className="text-center text-red-400">{error || 'Something went wrong.'}</p>
  }

  const pct = day.tasks_total > 0 ? day.tasks_done / day.tasks_total : 0
  const photoTask = day.entries.find((e) => e.kind === 'photo')
  // current_day is 0 until the start date arrives, so the program is scheduled
  // but not running yet. Ticking tasks now would log them against day 1 early.
  const notStarted = program.current_day < 1

  return (
    <div className="space-y-5 pb-4">
      {confetti && <Confetti onDone={() => setConfetti(false)} />}

      <header className="animate-slide-up">
        <div className="flex items-end justify-between">
          <div className="flex items-end gap-3">
            {/* Stepping back through the run. Forward only exists once you
                have gone back — there is nothing ahead to look at, and a day
                that has not happened cannot be filled in. */}
            {!notStarted && (
              <div className="flex items-center gap-1 pb-1">
                <button
                  type="button"
                  onClick={() => setViewDay(Math.max(1, day.day_number - 1))}
                  disabled={day.day_number <= 1}
                  aria-label="Previous day"
                  className="rounded-lg p-1.5 text-ink-500 transition hover:bg-ink-850 hover:text-ink-200 disabled:pointer-events-none disabled:opacity-25"
                >
                  <Chevron dir="left" />
                </button>
                <button
                  type="button"
                  onClick={() =>
                    setViewDay(
                      day.day_number + 1 >= program.current_day ? null : day.day_number + 1,
                    )
                  }
                  disabled={day.day_number >= program.current_day}
                  aria-label="Next day"
                  className="rounded-lg p-1.5 text-ink-500 transition hover:bg-ink-850 hover:text-ink-200 disabled:pointer-events-none disabled:opacity-25"
                >
                  <Chevron dir="right" />
                </button>
              </div>
            )}
          <div>
            <p className="text-sm text-ink-500">
              {new Date(`${notStarted ? program.start_date : day.date}T12:00:00`).toLocaleDateString(
                undefined,
                { weekday: 'long', month: 'long', day: 'numeric' },
              )}
            </p>
            <h1 className="mt-1 text-3xl font-semibold text-ink-100">
              {notStarted ? (
                <>Starts <span className="text-flame-500">soon</span></>
              ) : (
                <>
                  Day <span className="text-flame-500">{day.day_number}</span>
                  <span className="text-ink-600"> / {program.length_days}</span>
                </>
              )}
            </h1>
            </div>
          </div>
          <DayRing progress={pct} complete={day.status === 'complete'} />
        </div>

        <div className={`mt-4 flex gap-2 ${notStarted ? 'hidden' : ''}`}>
          <Pill label="Streak" value={`${program.streak} day${program.streak === 1 ? '' : 's'}`} />
          <Pill label="Done" value={`${program.days_complete} / ${program.length_days}`} />
          {day.status === 'complete' && <Pill label="" value="Day complete" tone="good" />}
          {day.status === 'missed' && <Pill label="" value="Missed" tone="bad" />}
        </div>
      </header>

      {notStarted && (
        <div className="card animate-slide-up border-flame-500/30 bg-flame-500/[0.07] p-4">
          <p className="font-medium text-flame-400">Your program hasn&apos;t started yet.</p>
          <p className="mt-1 text-sm text-ink-400">
            Day 1 is{' '}
            {new Date(`${program.start_date}T12:00:00`).toLocaleDateString(undefined, {
              weekday: 'long',
              month: 'long',
              day: 'numeric',
            })}
            . Everything below is your plan for that day — come back then to start ticking it off.
          </p>
        </div>
      )}

      {program.status !== 'active' && (
        <div className="card border-flame-500/30 bg-flame-500/[0.07] p-4">
          <p className="font-medium text-flame-400">
            {program.status === 'completed' ? 'You finished the challenge.' : 'This attempt has ended.'}
          </p>
          <p className="mt-1 text-sm text-ink-400">
            {program.status === 'completed'
              ? 'All 75 days logged. Start another whenever you are ready.'
              : 'A required task was missed on a past day.'}
          </p>
          <button
            className="btn-primary mt-3 w-full py-2.5 text-sm"
            onClick={async () => {
              await api.restartProgram(program.id)
              await load()
            }}
          >
            Start a new attempt
          </button>
        </div>
      )}

      <CoachNote />

      <section className="space-y-3">
        {day.entries.map((entry) => (
          <TaskRow
            key={entry.task_id}
            entry={entry}
            disabled={program.status !== 'active' || notStarted}
            onToggle={toggle}
            day={day}
            onChanged={load}
            onLogMeal={(meal) => {
              setEditingMeal(meal ?? null)
              setMealOpen(true)
            }}
          />
        ))}
      </section>

      {photoTask && (
        <section className="card p-4">
          <h2 className="mb-3 text-sm font-medium uppercase tracking-wide text-ink-500">
            Progress photo
          </h2>
          {day.photos.filter((p) => p.kind === 'progress').length > 0 && (
            <DeletablePhotos
              photos={day.photos.filter((p) => p.kind === 'progress')}
              ratio="h-24 w-24 shrink-0"
              className="mb-3 flex gap-2 overflow-x-auto pb-1"
              altFor={() => `Progress photo, day ${day.day_number}`}
              badgeFor={(p) => (p.pose ? POSE_LABEL[p.pose] : '')}
              onDeleted={() => load()}
            />
          )}
          <PhotoUpload
            kind="progress"
            dayNumber={day.day_number}
            label="Take today's photo"
            withPose
            onUploaded={() => load()}
          />
          <p className="mt-2 text-center text-xs text-ink-600">
            One photo is enough. Three angles from the same spot tells you more.
          </p>
        </section>
      )}

      {program.status === 'active' && !notStarted && (
        <DailyVitals programId={program.id} day={day} onSaved={load} />
      )}

      {summary && <SummaryPanel summary={summary} />}

      <section className="card p-4">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-sm font-medium uppercase tracking-wide text-ink-500">Food</h2>
          <span className="font-mono text-sm text-ink-300">
            {Math.round(day.totals.kcal)}
            {day.totals.kcal_target ? (
              <span className="text-ink-600"> / {day.totals.kcal_target}</span>
            ) : null}
            <span className="text-ink-600"> kcal</span>
          </span>
        </div>

        {day.totals.kcal_target ? (
          <div className="mb-3 h-1.5 overflow-hidden rounded-full bg-ink-800">
            <div
              className="h-full rounded-full bg-flame-500 transition-all duration-500"
              style={{ width: `${Math.min(100, (day.totals.kcal / day.totals.kcal_target) * 100)}%` }}
            />
          </div>
        ) : null}

        {day.meals.length > 0 && (
          <ul className="mb-3 divide-y divide-ink-800">
            {day.meals.map((meal) => (
              <li key={meal.id} className="flex items-center gap-3 py-2.5">
                {/* The whole row opens the meal, photo included — the
                    thumbnail is the most obvious thing to reach for, and a
                    meal saved without calories, which is what a failed
                    estimate leaves, was otherwise only deletable. */}
                <button
                  type="button"
                  onClick={() => {
                    setEditingMeal(meal)
                    setMealOpen(true)
                  }}
                  className="flex min-w-0 flex-1 items-center gap-3 text-left"
                  title="Open this meal to edit it or estimate its calories"
                >
                  {meal.photo_url && (
                    <AuthImage
                      src={`${meal.photo_url}?size=thumb`}
                      alt={meal.name || 'Meal'}
                      className="h-10 w-10 shrink-0 rounded-lg object-cover"
                    />
                  )}
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-ink-200">{meal.name || meal.slot}</span>
                    <span className="text-xs capitalize text-ink-600">
                      {meal.slot}
                      {meal.items.length > 0 && ` · ${meal.items.length} items`}
                      {meal.kcal === 0 && ' · tap to estimate'}
                    </span>
                  </span>
                  <span className="shrink-0 font-mono text-sm text-ink-400">
                    {Math.round(meal.kcal)}
                  </span>
                </button>
                <button
                  aria-label={`Delete ${meal.name || 'meal'}`}
                  className="shrink-0 p-1 text-ink-600 hover:text-red-400"
                  onClick={async () => {
                    await api.deleteMeal(meal.id)
                    await load()
                  }}
                >
                  ×
                </button>
              </li>
            ))}
          </ul>
        )}

        <div className="grid grid-cols-4 gap-2 pb-3 text-center">
          <Macro label="Protein" value={day.totals.protein_g} unit="g" />
          <Macro label="Carbs" value={day.totals.carbs_g} unit="g" />
          <Macro label="Fat" value={day.totals.fat_g} unit="g" />
          <Macro label="Training" value={day.totals.workout_minutes} unit="min" />
        </div>

        <button className="btn-ghost w-full" onClick={() => setMealOpen(true)}>
          + Log a meal
        </button>
      </section>

      {mealOpen && (
        <MealSheet
          key={editingMeal?.id ?? 'new'}
          dayNumber={day.day_number}
          meal={editingMeal}
          onClose={() => {
            setMealOpen(false)
            setEditingMeal(null)
          }}
          onSaved={async () => {
            setMealOpen(false)
            setEditingMeal(null)
            await load()
          }}
          onChanged={() => {
            void load()
          }}
        />
      )}

      {error && <p className="text-center text-sm text-red-400">{error}</p>}
    </div>
  )
}

function DayRing({ progress, complete }: { progress: number; complete: boolean }) {
  const r = 30
  const c = 2 * Math.PI * r
  return (
    <div className="relative h-20 w-20">
      <svg viewBox="0 0 72 72" className="h-20 w-20 -rotate-90">
        <circle cx="36" cy="36" r={r} className="fill-none stroke-ink-800" strokeWidth="5" />
        <circle
          cx="36"
          cy="36"
          r={r}
          className={`fill-none transition-all duration-700 ease-out ${
            complete ? 'stroke-moss-500' : 'stroke-flame-500'
          }`}
          strokeWidth="5"
          strokeLinecap="round"
          strokeDasharray={c}
          strokeDashoffset={c * (1 - progress)}
        />
      </svg>
      <span className="absolute inset-0 flex items-center justify-center text-lg font-semibold text-ink-100">
        {Math.round(progress * 100)}
        <span className="text-xs text-ink-500">%</span>
      </span>
    </div>
  )
}

function Pill({ label, value, tone }: { label: string; value: string; tone?: 'good' | 'bad' }) {
  const toneClass =
    tone === 'good'
      ? 'bg-moss-500/15 text-moss-400 border-moss-500/25'
      : tone === 'bad'
        ? 'bg-red-500/15 text-red-400 border-red-500/25'
        : 'bg-ink-900 text-ink-300 border-ink-800'
  return (
    <span className={`rounded-full border px-3 py-1.5 text-xs ${toneClass}`}>
      {label && <span className="text-ink-500">{label} </span>}
      {value}
    </span>
  )
}

function Macro({ label, value, unit }: { label: string; value: number; unit: string }) {
  return (
    <div className="rounded-xl bg-ink-850 py-2">
      <p className="font-mono text-sm text-ink-200">
        {Math.round(value)}
        <span className="text-xs text-ink-600">{unit}</span>
      </p>
      <p className="text-[11px] text-ink-600">{label}</p>
    </div>
  )
}

function Chevron({ dir }: { dir: 'left' | 'right' }) {
  return (
    <svg
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
      style={dir === 'right' ? { transform: 'rotate(180deg)' } : undefined}
    >
      <path d="M15 5l-7 7 7 7" />
    </svg>
  )
}
