import { useCallback, useEffect, useState } from 'react'
import { api } from '../lib/api'
import type { Program, ProgramTask } from '../lib/types'

/**
 * Show or hide the optional tasks.
 *
 * Hidden rather than deleted: somebody who turns journaling off in March
 * should not lose February. A hidden task leaves the day screen, the grid and
 * every count, and everything it recorded comes back intact if it is switched
 * on again.
 *
 * Only optional tasks appear here. The six canonical rules are the challenge,
 * and offering to hide one would be offering to change what completing a day
 * means.
 */
export function OptionalTasksCard() {
  const [program, setProgram] = useState<Program | null>(null)
  const [busy, setBusy] = useState<number | null>(null)

  const load = useCallback(async () => {
    try {
      setProgram(await api.activeProgram())
    } catch {
      setProgram(null)
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  if (!program) return null

  const optional = program.tasks.filter((t) => !t.required)
  if (optional.length === 0) return null

  async function toggle(task: ProgramTask) {
    if (busy !== null || !program) return
    setBusy(task.id)
    try {
      await api.updateTask(program.id, task.id, { hidden: !task.hidden })
      await load()
    } finally {
      setBusy(null)
    }
  }

  return (
    <section className="card space-y-3 p-4">
      <div>
        <h2 className="text-sm font-medium uppercase tracking-wide text-ink-500">Extras</h2>
        <p className="mt-1 text-xs text-ink-600">
          Optional trackers. None of them can fail your run — turn one off and it leaves the day
          screen, but everything it has recorded is kept.
        </p>
      </div>

      <ul className="divide-y divide-ink-800">
        {optional.map((task) => (
          <li key={task.id} className="flex items-center gap-3 py-2.5">
            <span className="min-w-0 flex-1">
              <span className="block truncate text-sm text-ink-200">{task.title}</span>
              <span className="text-xs text-ink-600">{task.detail}</span>
            </span>
            <button
              type="button"
              role="switch"
              aria-checked={!task.hidden}
              aria-label={`${task.hidden ? 'Show' : 'Hide'} ${task.title}`}
              disabled={busy === task.id}
              onClick={() => toggle(task)}
              className={`relative h-6 w-11 shrink-0 rounded-full transition ${
                task.hidden ? 'bg-ink-800' : 'bg-flame-500'
              } disabled:opacity-50`}
            >
              <span
                className={`absolute top-0.5 h-5 w-5 rounded-full bg-white transition-all ${
                  task.hidden ? 'left-0.5' : 'left-[22px]'
                }`}
              />
            </button>
          </li>
        ))}
      </ul>
    </section>
  )
}
