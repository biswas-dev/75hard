import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../lib/api'
import type { Program } from '../lib/types'

function displayDate(value: string | null) {
  if (!value) return 'In progress'
  const date = value.slice(0, 10)
  return new Date(`${date}T12:00:00`).toLocaleDateString(undefined, {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}

export function ChallengeHistory() {
  const navigate = useNavigate()
  const [programs, setPrograms] = useState<Program[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    api.listPrograms()
      .then((items) => setPrograms(items.filter((item) => item.status !== 'active')))
      .catch((err) => setError(err instanceof Error ? err.message : 'Could not load challenge history'))
      .finally(() => setLoading(false))
  }, [])

  return (
    <div className="mx-auto min-h-dvh w-full max-w-lg px-4 py-8">
      <header className="mb-8 animate-slide-up">
        <button type="button" className="mb-4 text-sm text-ink-400 hover:text-ink-200" onClick={() => navigate(-1)}>
          ← Back
        </button>
        <h1 className="text-3xl font-semibold text-ink-100">Past challenges</h1>
        <p className="mt-2 text-ink-500">Every finished attempt stays here, whether it succeeded or failed.</p>
      </header>

      {loading && <div className="h-32 animate-pulse rounded-2xl bg-ink-900" />}
      {error && <p className="rounded-xl border border-red-500/25 bg-red-500/10 p-4 text-sm text-red-400">{error}</p>}
      {!loading && !error && programs.length === 0 && (
        <section className="card p-5 text-center text-ink-500">No past challenges yet.</section>
      )}

      <div className="space-y-3">
        {programs.map((program) => (
          <article key={program.id} className="card p-5">
            <div className="flex items-start justify-between gap-4">
              <div>
                <p className="text-xs font-medium uppercase tracking-wide text-ink-500">Attempt {program.attempt_number}</p>
                <h2 className="mt-1 text-lg font-semibold text-ink-100">{program.name}</h2>
              </div>
              <span className={`rounded-full px-2.5 py-1 text-xs font-medium capitalize ${
                program.status === 'completed'
                  ? 'bg-moss-500/15 text-moss-400'
                  : 'bg-red-500/10 text-red-300'
              }`}>
                {program.status}
              </span>
            </div>
            <dl className="mt-5 grid grid-cols-3 gap-3 border-t border-ink-800 pt-4 text-sm">
              <div>
                <dt className="text-xs text-ink-600">Started</dt>
                <dd className="mt-1 text-ink-300">{displayDate(program.start_date)}</dd>
              </div>
              <div>
                <dt className="text-xs text-ink-600">Ended</dt>
                <dd className="mt-1 text-ink-300">{displayDate(program.ended_at)}</dd>
              </div>
              <div>
                <dt className="text-xs text-ink-600">Completed</dt>
                <dd className="mt-1 text-ink-300">{program.days_complete} {program.days_complete === 1 ? 'day' : 'days'}</dd>
              </div>
            </dl>
          </article>
        ))}
      </div>

      <button type="button" className="btn-primary mt-6 w-full" onClick={() => navigate('/start')}>
        Start a new challenge
      </button>
    </div>
  )
}
