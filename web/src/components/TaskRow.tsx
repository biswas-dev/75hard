import { useEffect, useRef, useState } from 'react'
import type { Day, Entry, Meal } from '../lib/types'
import { TaskIcon } from './TaskIcon'
import { TaskTracker } from './TaskTracker'

interface Props {
  entry: Entry
  disabled?: boolean
  onToggle: (entry: Entry, next: { done?: boolean; value?: number }) => void
  /** Supplying a day enables the task's optional tracker panel. */
  day?: Day
  onChanged?: () => void
  onLogMeal?: (meal?: Meal) => void
}

/**
 * One task on the day screen.
 *
 * The whole row is the tap target. Check-kind tasks flip on tap; number and
 * duration tasks expand a stepper so partial progress can be logged, and only
 * complete once they reach their target.
 */
export function TaskRow({ entry, disabled, onToggle, day, onChanged, onLogMeal }: Props) {
  const [rippling, setRippling] = useState(false)
  const [expanded, setExpanded] = useState(false)
  const [draft, setDraft] = useState(entry.value ?? 0)
  const rippleTimer = useRef<number>()

  const isMetered = entry.kind === 'number' || entry.kind === 'duration'
  // A tracker is a richer panel behind the task. It never gates completion, so
  // the row still ticks with one tap whether or not it is open.
  const hasTracker = Boolean(entry.tracker) && Boolean(day)
  const expandable = isMetered || hasTracker
  const target = entry.target_num ?? 0
  const progress = isMetered && target > 0 ? Math.min(1, (entry.value ?? 0) / target) : entry.done ? 1 : 0

  useEffect(() => setDraft(entry.value ?? 0), [entry.value])
  useEffect(() => () => window.clearTimeout(rippleTimer.current), [])

  function celebrate() {
    setRippling(true)
    rippleTimer.current = window.setTimeout(() => setRippling(false), 600)
    // A short pulse on the tap makes the check land physically. Silently
    // absent on desktop and on iOS, which is fine — it's a bonus, not the
    // feedback itself.
    navigator.vibrate?.(12)
  }

  function handlePrimary() {
    if (disabled) return
    if (entry.kind === 'photo') {
      // Photo tasks are completed by uploading, not by ticking.
      return
    }
    if (isMetered) {
      setExpanded((v) => !v)
      return
    }
    if (!entry.done) celebrate()
    onToggle(entry, { done: !entry.done })
  }

  function commit(value: number) {
    const clamped = Math.max(0, value)
    setDraft(clamped)
    if (target > 0 && clamped >= target && !entry.done) celebrate()
    onToggle(entry, { value: clamped, done: target > 0 ? clamped >= target : true })
  }

  return (
    <div
      className={`card overflow-hidden transition-colors duration-300 ${
        entry.done ? 'border-moss-500/30 bg-moss-500/[0.06]' : ''
      }`}
    >
      <div className="flex w-full items-center">
      <button
        type="button"
        onClick={handlePrimary}
        disabled={disabled}
        aria-pressed={entry.done}
        className="flex min-w-0 flex-1 items-center gap-4 p-4 text-left transition active:bg-ink-850/60 disabled:opacity-60"
      >
        {/* The check target: a ring that fills as the task progresses. */}
        <span className="relative flex h-12 w-12 shrink-0 items-center justify-center">
          <svg viewBox="0 0 48 48" className="absolute inset-0 h-12 w-12 -rotate-90">
            <circle cx="24" cy="24" r="21" className="fill-none stroke-ink-800" strokeWidth="3" />
            <circle
              cx="24"
              cy="24"
              r="21"
              className={`fill-none transition-all duration-500 ease-out ${
                entry.done ? 'stroke-moss-500' : 'stroke-flame-500'
              }`}
              strokeWidth="3"
              strokeLinecap="round"
              strokeDasharray={2 * Math.PI * 21}
              strokeDashoffset={2 * Math.PI * 21 * (1 - progress)}
            />
          </svg>

          {rippling && (
            <span className="absolute inset-0 animate-ripple rounded-full bg-moss-500/40" aria-hidden />
          )}

          <span
            className={`relative transition-all duration-200 ${
              entry.done ? 'animate-pop text-moss-400' : 'text-ink-400'
            }`}
          >
            {entry.done ? <CheckMark /> : <TaskIcon name={entry.icon} />}
          </span>
        </span>

        <span className="min-w-0 flex-1">
          <span
            className={`block font-medium transition-colors ${
              entry.done ? 'text-moss-300' : 'text-ink-100'
            }`}
          >
            {entry.title}
          </span>
          <span className="mt-0.5 block truncate text-sm text-ink-500">
            {isMetered && target > 0
              ? `${formatNumber(entry.value ?? 0)} of ${formatNumber(target)} ${entry.unit}`
              : entry.kind === 'photo'
                ? entry.done
                  ? 'Photo added'
                  : 'Add a photo below'
                : entry.detail}
          </span>
        </span>

        {!entry.required && (
          <span className="shrink-0 rounded-full bg-ink-800 px-2 py-0.5 text-[11px] text-ink-400">
            optional
          </span>
        )}
      </button>

      {expandable && (
        <button
          type="button"
          aria-expanded={expanded}
          aria-label={expanded ? `Hide ${entry.title} detail` : `Show ${entry.title} detail`}
          className={`shrink-0 self-stretch px-4 text-ink-500 transition-transform hover:text-ink-300 ${
            expanded ? 'rotate-180' : ''
          }`}
          onClick={() => setExpanded((v) => !v)}
        >
          <Chevron />
        </button>
      )}
      </div>

      {hasTracker && expanded && !isMetered && (
        <div className="animate-slide-up border-t border-ink-800 p-4">
          <TaskTracker
            entry={entry}
            day={day!}
            onChanged={() => onChanged?.()}
            onLogMeal={(meal) => onLogMeal?.(meal)}
          />
        </div>
      )}

      {isMetered && expanded && (
        <div className="animate-slide-up border-t border-ink-800 p-4">
          <div className="flex items-center gap-2">
            <button type="button" className="btn-ghost px-4" onClick={() => commit(draft - step(target))}>
              −
            </button>
            <input
              type="number"
              inputMode="decimal"
              value={draft}
              onChange={(e) => setDraft(Number(e.target.value))}
              onBlur={() => commit(draft)}
              className="field text-center"
              aria-label={`${entry.title} amount in ${entry.unit}`}
            />
            <button type="button" className="btn-ghost px-4" onClick={() => commit(draft + step(target))}>
              +
            </button>
          </div>
          <div className="mt-3 flex gap-2">
            <button type="button" className="btn-ghost flex-1 py-2 text-sm" onClick={() => commit(0)}>
              Reset
            </button>
            <button type="button" className="btn-primary flex-1 py-2 text-sm" onClick={() => commit(target)}>
              Mark {formatNumber(target)} {entry.unit}
            </button>
          </div>

          {hasTracker && (
            <div className="mt-4 border-t border-ink-800 pt-4">
              <TaskTracker
                entry={entry}
                day={day!}
                onChanged={() => onChanged?.()}
                onLogMeal={(meal) => onLogMeal?.(meal)}
              />
            </div>
          )}
        </div>
      )}
    </div>
  )
}

/** A sensible increment for the target's magnitude: 1 page, 5 minutes, 8 oz. */
function step(target: number): number {
  if (target >= 100) return 8
  if (target >= 30) return 5
  return 1
}

function formatNumber(n: number): string {
  return Number.isInteger(n) ? String(n) : n.toFixed(1)
}

function CheckMark() {
  return (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3">
      <path d="M20 6 9 17l-5-5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function Chevron() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
      <path d="m6 9 6 6 6-6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}
