import { useEffect, useRef, useState } from 'react'
import type { GridTask } from '../lib/types'
import { TaskIcon } from './TaskIcon'

export type CellState = '' | 'd' | 'p' | 'm' | 'f'

interface Props {
  task: GridTask
  currentDay: number
  onOpenDay: (dayNumber: number) => void
  onQuickToggle: (dayNumber: number, done: boolean) => void
  onChangeColor: (color: string) => void
}

/** The palette offered when recolouring an activity. */
const PALETTE = [
  '#ff6b35', '#37d67a', '#4a9eff', '#ffd166', '#b47aea', '#ff5d8f',
  '#2dd4bf', '#f97316', '#a3e635', '#60a5fa',
]

/** How long a press must be held before it counts as "open details". */
const LONG_PRESS_MS = 450

/**
 * A contribution-style heatmap for one activity across the whole program.
 *
 * Tap toggles the day. Press and hold — or right-click on a desktop — opens
 * that day's detail, which is where a value, a note or a photo goes. The two
 * gestures are deliberately different weights: ticking is the thing you do
 * every day, detail is the thing you do occasionally.
 */
export function ActivityGrid({ task, currentDay, onOpenDay, onQuickToggle, onChangeColor }: Props) {
  const [menuOpen, setMenuOpen] = useState(false)
  const pressTimer = useRef<number>()
  const longFired = useRef(false)

  useEffect(() => () => window.clearTimeout(pressTimer.current), [])

  function startPress(day: number) {
    longFired.current = false
    pressTimer.current = window.setTimeout(() => {
      longFired.current = true
      navigator.vibrate?.(14)
      onOpenDay(day)
    }, LONG_PRESS_MS)
  }

  function endPress(day: number, state: CellState) {
    window.clearTimeout(pressTimer.current)
    // The long press already opened the detail; don't also toggle.
    if (longFired.current) return
    if (state === 'f') {
      // A future day can be inspected but not ticked.
      onOpenDay(day)
      return
    }
    if (task.kind === 'photo') {
      // A photo task is satisfied by uploading, so a tap opens the day.
      onOpenDay(day)
      return
    }
    onQuickToggle(day, state !== 'd')
  }

  return (
    <section className="card p-4">
      <header className="mb-3 flex items-start gap-3">
        <span
          className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl"
          style={{ backgroundColor: `${task.color}22`, color: task.color }}
        >
          <TaskIcon name={task.icon} size={18} />
        </span>

        <div className="min-w-0 flex-1">
          <h3 className="truncate font-medium text-ink-100">{task.title}</h3>
          <p className="text-xs text-ink-500">
            {task.completed} done
            {task.streak > 0 && <> · {task.streak} day streak</>}
            {task.best_streak > 0 && <> · best {task.best_streak}</>}
          </p>
        </div>

        <div className="relative shrink-0">
          <button
            aria-label={`Options for ${task.title}`}
            className="rounded-lg px-2 py-1 text-ink-500 hover:bg-ink-850 hover:text-ink-300"
            onClick={() => setMenuOpen((v) => !v)}
          >
            •••
          </button>

          {menuOpen && (
            <>
              {/* Click-away target, behind the menu. */}
              <button
                className="fixed inset-0 z-10 cursor-default"
                aria-label="Close menu"
                onClick={() => setMenuOpen(false)}
              />
              <div className="absolute right-0 z-20 mt-1 w-44 animate-fade-in rounded-xl border border-ink-800 bg-ink-850 p-3 shadow-xl">
                <p className="mb-2 text-xs text-ink-500">Colour</p>
                <div className="flex flex-wrap gap-2">
                  {PALETTE.map((c) => (
                    <button
                      key={c}
                      aria-label={`Set colour ${c}`}
                      onClick={() => {
                        onChangeColor(c)
                        setMenuOpen(false)
                      }}
                      className={`h-6 w-6 rounded-full transition ${
                        task.color === c ? 'ring-2 ring-white ring-offset-2 ring-offset-ink-850' : ''
                      }`}
                      style={{ backgroundColor: c }}
                    />
                  ))}
                </div>
              </div>
            </>
          )}
        </div>
      </header>

      <div className="grid grid-cols-[repeat(15,minmax(0,1fr))] gap-1">
        {task.cells.map((state, i) => {
          const day = i + 1
          const isToday = day === currentDay
          return (
            <button
              key={day}
              title={`Day ${day}`}
              aria-label={`${task.title}, day ${day}, ${describe(state as CellState)}`}
              className={`relative aspect-square rounded-[3px] transition-transform active:scale-90 ${
                isToday ? 'ring-1 ring-white/70' : ''
              }`}
              style={cellStyle(state as CellState, task.color)}
              onContextMenu={(e) => {
                e.preventDefault()
                onOpenDay(day)
              }}
              onPointerDown={() => startPress(day)}
              onPointerUp={() => endPress(day, state as CellState)}
              onPointerLeave={() => window.clearTimeout(pressTimer.current)}
            />
          )
        })}
      </div>

      <div className="mt-2 flex justify-between text-[10px] text-ink-600">
        <span>Day 1</span>
        <span>Day {task.cells.length}</span>
      </div>
    </section>
  )
}

/** Cell colours: the activity's own hue, dimmed by how far along the day got. */
function cellStyle(state: CellState, color: string): React.CSSProperties {
  switch (state) {
    case 'd':
      return { backgroundColor: color }
    case 'p':
      return { backgroundColor: `${color}66`, border: `1px solid ${color}` }
    case 'm':
      return { backgroundColor: 'rgba(239,68,68,0.16)', border: '1px solid rgba(239,68,68,0.35)' }
    case 'f':
      return { backgroundColor: 'rgba(255,255,255,0.03)' }
    default:
      return { backgroundColor: 'rgba(255,255,255,0.07)' }
  }
}

function describe(state: CellState): string {
  switch (state) {
    case 'd':
      return 'done'
    case 'p':
      return 'partly done'
    case 'm':
      return 'missed'
    case 'f':
      return 'upcoming'
    default:
      return 'not done yet'
  }
}
