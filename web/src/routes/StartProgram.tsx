import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { ICON_NAMES, TaskIcon } from '../components/TaskIcon'
import { api } from '../lib/api'
import type { ProgramTask, TaskKind } from '../lib/types'

type DraftTask = Pick<ProgramTask, 'title' | 'detail' | 'icon' | 'kind' | 'unit' | 'required'> & {
  key: string
  target_num: number | null
}

// The canonical six, mirrored from the server's DefaultTasks(). Shown up front
// so the template can be edited before day 1 rather than after.
const DEFAULT_TASKS: DraftTask[] = [
  { key: 'workout_indoor', title: '45-minute workout', detail: 'Any training you like, at least 45 minutes.', icon: 'dumbbell', kind: 'duration', target_num: 45, unit: 'min', required: true },
  { key: 'workout_outdoor', title: '45-minute outdoor workout', detail: 'A second session, outside, whatever the weather.', icon: 'tree', kind: 'duration', target_num: 45, unit: 'min', required: true },
  { key: 'diet', title: 'Follow the diet', detail: 'No cheat meals, no alcohol.', icon: 'salad', kind: 'check', target_num: null, unit: '', required: true },
  { key: 'water', title: 'Drink 1 gallon of water', detail: '128 oz over the day.', icon: 'droplet', kind: 'number', target_num: 128, unit: 'oz', required: true },
  { key: 'reading', title: 'Read 10 pages', detail: 'Non-fiction, personal development.', icon: 'book', kind: 'number', target_num: 10, unit: 'pages', required: true },
  { key: 'progress_photo', title: 'Take a progress photo', detail: 'One photo, every day.', icon: 'camera', kind: 'photo', target_num: null, unit: '', required: true },
]

function todayLocal(): string {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

export function StartProgram() {
  const navigate = useNavigate()
  const [tasks, setTasks] = useState<DraftTask[]>(DEFAULT_TASKS)
  const [startDate, setStartDate] = useState(todayLocal())
  const [lengthDays, setLengthDays] = useState(75)
  const [strictRestart, setStrictRestart] = useState(true)
  const [kcalTarget, setKcalTarget] = useState<string>('')
  const [editing, setEditing] = useState<number | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    // Someone who already has a program shouldn't be here.
    api.activeProgram().then(() => navigate('/app', { replace: true })).catch(() => {})
  }, [navigate])

  function update(index: number, patch: Partial<DraftTask>) {
    setTasks((prev) => prev.map((t, i) => (i === index ? { ...t, ...patch } : t)))
  }

  function remove(index: number) {
    setTasks((prev) => prev.filter((_, i) => i !== index))
    setEditing(null)
  }

  function add() {
    setTasks((prev) => [
      ...prev,
      {
        key: `custom_${prev.length}_${Date.now()}`,
        title: 'New task',
        detail: '',
        icon: 'check',
        kind: 'check',
        target_num: null,
        unit: '',
        required: true,
      },
    ])
    setEditing(tasks.length)
  }

  async function start() {
    if (tasks.length === 0) {
      setError('Add at least one task before starting.')
      return
    }
    setBusy(true)
    setError('')
    try {
      await api.createProgram({
        start_date: startDate,
        length_days: lengthDays,
        strict_restart: strictRestart,
        daily_kcal_target: kcalTarget ? Number(kcalTarget) : null,
        tasks,
      })
      navigate('/app', { replace: true })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not start the program')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="mx-auto w-full max-w-lg px-4 py-8 pb-32">
      <header className="mb-8 animate-slide-up">
        <h1 className="text-3xl font-semibold text-ink-100">Set up your challenge</h1>
        <p className="mt-2 text-ink-500">
          These are the six classic rules. Change anything you like — the program is yours.
        </p>
      </header>

      <section className="card mb-6 divide-y divide-ink-800">
        <Row label="Start date">
          <input
            type="date"
            className="field max-w-[10rem] py-2"
            value={startDate}
            onChange={(e) => setStartDate(e.target.value)}
          />
        </Row>
        <Row label="Length">
          <div className="flex items-center gap-2">
            <input
              type="number"
              min={1}
              max={365}
              className="field w-20 py-2 text-center"
              value={lengthDays}
              onChange={(e) => setLengthDays(Number(e.target.value))}
            />
            <span className="text-sm text-ink-500">days</span>
          </div>
        </Row>
        <Row label="Daily calorie target" hint="Optional — leave blank to skip calorie tracking targets.">
          <input
            type="number"
            placeholder="none"
            className="field w-28 py-2 text-center"
            value={kcalTarget}
            onChange={(e) => setKcalTarget(e.target.value)}
          />
        </Row>
        <Row
          label="Restart on a missed day"
          hint={
            strictRestart
              ? 'Canonical rules: miss anything and the attempt ends at day 1.'
              : 'Lenient: a miss is recorded but your attempt continues.'
          }
        >
          <Toggle checked={strictRestart} onChange={setStrictRestart} label="Restart on a missed day" />
        </Row>
      </section>

      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-sm font-medium uppercase tracking-wide text-ink-500">
          Daily tasks ({tasks.length})
        </h2>
        <button type="button" className="text-sm text-flame-500 hover:text-flame-400" onClick={add}>
          + Add task
        </button>
      </div>

      <div className="space-y-3">
        {tasks.map((task, i) => (
          <div key={task.key} className="card overflow-hidden">
            <button
              type="button"
              className="flex w-full items-center gap-3 p-4 text-left"
              onClick={() => setEditing(editing === i ? null : i)}
            >
              <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-ink-850 text-flame-500">
                <TaskIcon name={task.icon} size={20} />
              </span>
              <span className="min-w-0 flex-1">
                <span className="block truncate font-medium text-ink-100">{task.title}</span>
                <span className="block truncate text-sm text-ink-500">
                  {task.kind === 'number' || task.kind === 'duration'
                    ? `${task.target_num ?? 0} ${task.unit}`
                    : task.kind === 'photo'
                      ? 'Photo'
                      : task.detail || 'Tick to complete'}
                </span>
              </span>
              {!task.required && (
                <span className="rounded-full bg-ink-800 px-2 py-0.5 text-[11px] text-ink-400">optional</span>
              )}
            </button>

            {editing === i && (
              <div className="animate-slide-up space-y-4 border-t border-ink-800 p-4">
                <div>
                  <label className="label">Title</label>
                  <input
                    className="field"
                    value={task.title}
                    onChange={(e) => update(i, { title: e.target.value })}
                  />
                </div>
                <div>
                  <label className="label">Description</label>
                  <input
                    className="field"
                    value={task.detail}
                    onChange={(e) => update(i, { detail: e.target.value })}
                  />
                </div>
                <div>
                  <label className="label">Type</label>
                  <select
                    className="field"
                    value={task.kind}
                    onChange={(e) => {
                      const kind = e.target.value as TaskKind
                      const metered = kind === 'number' || kind === 'duration'
                      update(i, {
                        kind,
                        // A metered task needs a target to be completable, and
                        // a non-metered one must not carry a stale one.
                        target_num: metered ? (task.target_num ?? (kind === 'duration' ? 45 : 1)) : null,
                        unit: metered ? task.unit || (kind === 'duration' ? 'min' : '') : '',
                      })
                    }}
                  >
                    <option value="check">Tick to complete</option>
                    <option value="number">Count toward a target</option>
                    <option value="duration">Minutes toward a target</option>
                    <option value="photo">Photo</option>
                    <option value="text">Note</option>
                  </select>
                </div>

                {(task.kind === 'number' || task.kind === 'duration') && (
                  <div className="flex gap-3">
                    <div className="flex-1">
                      <label className="label">Target</label>
                      <input
                        type="number"
                        className="field"
                        value={task.target_num ?? 0}
                        onChange={(e) => update(i, { target_num: Number(e.target.value) })}
                      />
                    </div>
                    <div className="flex-1">
                      <label className="label">Unit</label>
                      <input
                        className="field"
                        placeholder="oz, pages, min"
                        value={task.unit}
                        onChange={(e) => update(i, { unit: e.target.value })}
                      />
                    </div>
                  </div>
                )}

                <div>
                  <label className="label">Icon</label>
                  <div className="flex flex-wrap gap-2">
                    {ICON_NAMES.map((name) => (
                      <button
                        key={name}
                        type="button"
                        onClick={() => update(i, { icon: name })}
                        aria-label={`Icon ${name}`}
                        className={`flex h-10 w-10 items-center justify-center rounded-xl border transition ${
                          task.icon === name
                            ? 'border-flame-500 bg-flame-500/15 text-flame-500'
                            : 'border-ink-800 bg-ink-850 text-ink-400 hover:text-ink-200'
                        }`}
                      >
                        <TaskIcon name={name} size={18} />
                      </button>
                    ))}
                  </div>
                </div>

                <div className="flex items-center justify-between pt-1">
                  <Toggle
                    checked={task.required}
                    onChange={(v) => update(i, { required: v })}
                    label="Required to complete the day"
                  />
                  <button type="button" className="btn-danger px-3 py-2 text-sm" onClick={() => remove(i)}>
                    Remove
                  </button>
                </div>
              </div>
            )}
          </div>
        ))}
      </div>

      {error && (
        <p className="mt-4 rounded-xl border border-red-500/25 bg-red-500/10 px-4 py-3 text-sm text-red-400">
          {error}
        </p>
      )}

      <div className="fixed inset-x-0 bottom-0 border-t border-ink-800 bg-ink-950/95 p-4 backdrop-blur">
        <div className="mx-auto max-w-lg">
          <button className="btn-primary w-full py-4 text-base" onClick={start} disabled={busy}>
            {busy ? 'Starting…' : `Begin day 1 of ${lengthDays}`}
          </button>
        </div>
      </div>
    </div>
  )
}

function Row({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 p-4">
      <div className="min-w-0">
        <p className="font-medium text-ink-200">{label}</p>
        {hint && <p className="mt-0.5 text-xs text-ink-500">{hint}</p>}
      </div>
      <div className="shrink-0">{children}</div>
    </div>
  )
}

function Toggle({
  checked,
  onChange,
  label,
}: {
  checked: boolean
  onChange: (v: boolean) => void
  label: string
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      onClick={() => onChange(!checked)}
      className={`relative h-7 w-12 shrink-0 rounded-full transition-colors ${
        checked ? 'bg-flame-500' : 'bg-ink-700'
      }`}
    >
      <span
        className={`absolute top-1 h-5 w-5 rounded-full bg-white transition-transform ${
          checked ? 'translate-x-6' : 'translate-x-1'
        }`}
      />
    </button>
  )
}
