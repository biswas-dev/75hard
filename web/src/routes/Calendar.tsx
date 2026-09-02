import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { ActivityGrid } from '../components/ActivityGrid'
import { CellDetailSheet } from '../components/CellDetailSheet'
import { ApiError, api } from '../lib/api'
import type { DaySummary, Grid, GridTask, Program } from '../lib/types'

type View = 'grid' | 'days'

export function Calendar() {
  const navigate = useNavigate()
  const [program, setProgram] = useState<Program | null>(null)
  const [days, setDays] = useState<DaySummary[]>([])
  const [grid, setGrid] = useState<Grid | null>(null)
  const [view, setView] = useState<View>('grid')
  const [loading, setLoading] = useState(true)
  const [open, setOpen] = useState<{ task: GridTask; day: number } | null>(null)

  const load = useCallback(async () => {
    try {
      const p = await api.activeProgram()
      setProgram(p)
      const [d, g] = await Promise.all([api.listDays(p.id), api.grid(p.id)])
      setDays(d)
      setGrid(g)
    } catch (err) {
      if (err instanceof ApiError && err.code === 'no_active_program') {
        navigate('/start', { replace: true })
      }
    } finally {
      setLoading(false)
    }
  }, [navigate])

  useEffect(() => {
    load()
  }, [load])

  async function quickToggle(task: GridTask, dayNumber: number, done: boolean) {
    if (!program || !grid) return

    // Optimistic: recolour the cell immediately, reconcile from the server's
    // recomputed grid a moment later.
    setGrid({
      ...grid,
      tasks: grid.tasks.map((t) =>
        t.task_id === task.task_id
          ? { ...t, cells: t.cells.map((c, i) => (i === dayNumber - 1 ? (done ? 'd' : '') : c)) }
          : t,
      ),
    })

    try {
      const body: { done: boolean; value?: number } = { done }
      // A metered task has no meaning as a bare tick, so a quick tap fills it
      // to the target (or clears it).
      if ((task.kind === 'number' || task.kind === 'duration') && task.target_num != null) {
        body.value = done ? task.target_num : 0
      }
      await api.toggleTask(program.id, dayNumber, task.task_id, body)
      navigator.vibrate?.(10)
    } catch {
      // Fall through to the refetch below, which restores the truth.
    }
    setGrid(await api.grid(program.id))
  }

  async function changeColor(task: GridTask, color: string) {
    if (!program || !grid) return
    setGrid({
      ...grid,
      tasks: grid.tasks.map((t) => (t.task_id === task.task_id ? { ...t, color } : t)),
    })
    await api.updateTask(program.id, task.task_id, { color })
  }

  if (loading) {
    return (
      <div className="space-y-3">
        {[0, 1, 2].map((i) => (
          <div key={i} className="h-40 animate-pulse rounded-2xl bg-ink-900" />
        ))}
      </div>
    )
  }
  if (!program) return null

  return (
    <div className="space-y-5 pb-4">
      <header className="animate-slide-up">
        <h1 className="text-2xl font-semibold text-ink-100">{program.name}</h1>
        <p className="mt-1 text-sm text-ink-500">
          Started{' '}
          {new Date(`${program.start_date}T12:00:00`).toLocaleDateString(undefined, {
            month: 'long',
            day: 'numeric',
            year: 'numeric',
          })}
          {program.attempt_number > 1 && ` · attempt ${program.attempt_number}`}
        </p>
      </header>

      <div className="grid grid-cols-2 gap-2">
        {(['grid', 'days'] as View[]).map((v) => (
          <button
            key={v}
            onClick={() => setView(v)}
            className={`rounded-xl border py-2.5 text-sm transition ${
              view === v
                ? 'border-flame-500 bg-flame-500/15 text-flame-400'
                : 'border-ink-800 bg-ink-900 text-ink-400'
            }`}
          >
            {v === 'grid' ? 'By activity' : 'By day'}
          </button>
        ))}
      </div>

      {view === 'grid' ? (
        <>
          <p className="px-1 text-xs text-ink-600">
            Tap a square to tick it off. Press and hold — or right-click — to add detail.
          </p>

          {grid?.tasks.map((task) => (
            <ActivityGrid
              key={task.task_id}
              task={task}
              currentDay={grid.current_day}
              onOpenDay={(day) => setOpen({ task, day })}
              onQuickToggle={(day, done) => quickToggle(task, day, done)}
              onChangeColor={(color) => changeColor(task, color)}
            />
          ))}

          <div className="flex flex-wrap gap-3 px-1 text-xs text-ink-500">
            <Legend swatch={{ backgroundColor: '#ff6b35' }} label="Done" />
            <Legend swatch={{ backgroundColor: '#ff6b3566', border: '1px solid #ff6b35' }} label="Partial" />
            <Legend
              swatch={{ backgroundColor: 'rgba(239,68,68,0.16)', border: '1px solid rgba(239,68,68,0.35)' }}
              label="Missed"
            />
            <Legend swatch={{ backgroundColor: 'rgba(255,255,255,0.07)' }} label="Not yet" />
          </div>
        </>
      ) : (
        <>
          <div className="grid grid-cols-7 gap-2">
            {days.map((d) => {
              const isFuture = d.day_number > program.current_day
              const isToday = d.day_number === program.current_day
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
            <Legend swatch={{ backgroundColor: 'rgba(55,214,122,0.2)', border: '1px solid rgba(55,214,122,0.4)' }} label="Complete" />
            <Legend swatch={{ backgroundColor: 'rgba(239,68,68,0.15)', border: '1px solid rgba(239,68,68,0.3)' }} label="Missed" />
            <Legend swatch={{ backgroundColor: '#0e0e11', border: '1px solid #1b1b22' }} label="Pending" />
          </div>
        </>
      )}

      {open && grid && (
        <CellDetailSheet
          programId={program.id}
          task={open.task}
          dayNumber={open.day}
          isFuture={grid.current_day >= 1 && open.day > grid.current_day}
          onClose={() => setOpen(null)}
          onSaved={load}
        />
      )}
    </div>
  )
}

function Legend({ swatch, label }: { swatch: React.CSSProperties; label: string }) {
  return (
    <span className="flex items-center gap-2">
      <span className="h-3.5 w-3.5 rounded" style={swatch} />
      {label}
    </span>
  )
}
