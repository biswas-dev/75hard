import { useEffect, useState } from 'react'
import { PhotoUpload } from '../components/PhotoUpload'
import { ApiError, api } from '../lib/api'
import type { AIStatus, Photo, Plan, Recipe } from '../lib/types'

type Tab = 'recipes' | 'plan'

export function Coach() {
  const [status, setStatus] = useState<AIStatus | null>(null)
  const [tab, setTab] = useState<Tab>('recipes')

  useEffect(() => {
    api.aiStatus().then(setStatus).catch(() => setStatus(null))
  }, [])

  if (status && !status.enabled) {
    return (
      <div className="space-y-5">
        <Header status={status} />
        <div className="card p-8 text-center text-ink-500">
          <p className="text-ink-300">AI features are switched off on this server.</p>
          <p className="mt-2 text-sm">
            Set <code className="rounded bg-ink-850 px-1.5 py-0.5 text-ink-400">AI_1_PROVIDER</code>,{' '}
            <code className="rounded bg-ink-850 px-1.5 py-0.5 text-ink-400">AI_1_MODEL</code> and{' '}
            <code className="rounded bg-ink-850 px-1.5 py-0.5 text-ink-400">AI_1_API_KEY</code> to enable
            them.
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-5 pb-4">
      <Header status={status} />

      <div className="grid grid-cols-2 gap-2">
        {(['recipes', 'plan'] as Tab[]).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`rounded-xl border py-2.5 text-sm capitalize transition ${
              tab === t
                ? 'border-flame-500 bg-flame-500/15 text-flame-400'
                : 'border-ink-800 bg-ink-900 text-ink-400'
            }`}
          >
            {t === 'recipes' ? 'Recipes' : 'Weekly plan'}
          </button>
        ))}
      </div>

      {tab === 'recipes' ? <Recipes /> : <WeeklyPlan />}
    </div>
  )
}

function Header({ status }: { status: AIStatus | null }) {
  return (
    <header className="animate-slide-up">
      <h1 className="text-2xl font-semibold text-ink-100">Coach</h1>
      <p className="mt-1 text-sm text-ink-500">
        {status?.enabled
          ? `${status.used_today} of ${status.daily_limit} AI requests used today`
          : 'Recipes and plans built from your own logged record.'}
      </p>
    </header>
  )
}

function Recipes() {
  const [ingredients, setIngredients] = useState('')
  const [preferences, setPreferences] = useState('')
  const [slot, setSlot] = useState('dinner')
  const [photo, setPhoto] = useState<Photo | null>(null)
  const [recipes, setRecipes] = useState<Recipe[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [expanded, setExpanded] = useState<number | null>(0)

  async function suggest() {
    setBusy(true)
    setError('')
    try {
      const { recipes } = await api.suggestRecipes({
        ingredients: ingredients
          .split(',')
          .map((s) => s.trim())
          .filter(Boolean),
        preferences,
        meal_slot: slot,
        photo_id: photo?.id ?? null,
      })
      setRecipes(recipes)
      setExpanded(0)
      if (recipes.length === 0) setError('No suggestions came back. Try again with more detail.')
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="space-y-4">
      <section className="card space-y-4 p-4">
        <div>
          <label className="label" htmlFor="ingredients">What have you got?</label>
          <input
            id="ingredients"
            className="field"
            placeholder="chicken, rice, spinach"
            value={ingredients}
            onChange={(e) => setIngredients(e.target.value)}
          />
          <p className="mt-1.5 text-xs text-ink-600">Comma separated, or photograph the fridge below.</p>
        </div>

        {photo ? (
          <div className="flex items-center gap-3 rounded-xl bg-ink-850 p-3">
            <span className="flex-1 text-sm text-ink-300">Ingredient photo attached</span>
            <button className="text-sm text-ink-500 hover:text-red-400" onClick={() => setPhoto(null)}>
              Remove
            </button>
          </div>
        ) : (
          <PhotoUpload
            kind="ingredients"
            label="Photograph your ingredients"
            onUploaded={setPhoto}
          />
        )}

        <div>
          <label className="label">Meal</label>
          <div className="grid grid-cols-4 gap-2">
            {['breakfast', 'lunch', 'dinner', 'snack'].map((s) => (
              <button
                key={s}
                type="button"
                onClick={() => setSlot(s)}
                className={`rounded-xl border py-2 text-xs capitalize transition ${
                  slot === s
                    ? 'border-flame-500 bg-flame-500/15 text-flame-400'
                    : 'border-ink-800 bg-ink-850 text-ink-400'
                }`}
              >
                {s}
              </button>
            ))}
          </div>
        </div>

        <div>
          <label className="label" htmlFor="prefs">Anything to avoid?</label>
          <input
            id="prefs"
            className="field"
            placeholder="no dairy, quick to cook"
            value={preferences}
            onChange={(e) => setPreferences(e.target.value)}
          />
        </div>

        <button className="btn-primary w-full" onClick={suggest} disabled={busy}>
          {busy ? 'Thinking…' : 'Suggest meals for what’s left today'}
        </button>
        <p className="text-center text-xs text-ink-600">
          Suggestions fit your remaining calories for today.
        </p>
      </section>

      {error && (
        <p className="rounded-xl border border-red-500/25 bg-red-500/10 px-4 py-3 text-sm text-red-400">
          {error}
        </p>
      )}

      {recipes.map((r, i) => (
        <article key={r.name + i} className="card animate-slide-up overflow-hidden">
          <button
            className="w-full p-4 text-left"
            onClick={() => setExpanded(expanded === i ? null : i)}
          >
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <h3 className="font-medium text-ink-100">{r.name}</h3>
                <p className="mt-0.5 text-sm text-ink-500">{r.summary}</p>
              </div>
              <span className="shrink-0 font-mono text-sm text-flame-400">
                {Math.round(r.kcal_per_serving)}
                <span className="text-xs text-ink-600"> kcal</span>
              </span>
            </div>
            <div className="mt-3 flex flex-wrap gap-2 text-xs text-ink-500">
              <Chip>{r.minutes} min</Chip>
              <Chip>{r.servings} serving{r.servings === 1 ? '' : 's'}</Chip>
              <Chip>{Math.round(r.protein_g)}g protein</Chip>
            </div>
          </button>

          {expanded === i && (
            <div className="animate-slide-up space-y-4 border-t border-ink-800 p-4">
              <div>
                <h4 className="mb-2 text-xs uppercase tracking-wide text-ink-500">Ingredients</h4>
                <ul className="space-y-1 text-sm text-ink-300">
                  {r.ingredients.map((ing, j) => (
                    <li key={j} className="flex gap-2">
                      <span className="text-ink-600">•</span>
                      {ing}
                    </li>
                  ))}
                </ul>
              </div>
              <div>
                <h4 className="mb-2 text-xs uppercase tracking-wide text-ink-500">Method</h4>
                <ol className="space-y-2 text-sm text-ink-300">
                  {r.steps.map((step, j) => (
                    <li key={j} className="flex gap-3">
                      <span className="font-mono text-xs text-flame-500">{j + 1}</span>
                      {step}
                    </li>
                  ))}
                </ol>
              </div>
            </div>
          )}
        </article>
      ))}
    </div>
  )
}

function WeeklyPlan() {
  const [goals, setGoals] = useState('')
  const [plan, setPlan] = useState<Plan | null>(null)
  const [cached, setCached] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function build(force = false) {
    setBusy(true)
    setError('')
    try {
      const res = await api.buildPlan(goals, force)
      setPlan(res.plan)
      setCached(res.cached)
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="space-y-4">
      <section className="card space-y-4 p-4">
        <div>
          <label className="label" htmlFor="goals">What are you working toward?</label>
          <textarea
            id="goals"
            rows={3}
            className="field resize-none"
            placeholder="Lose fat while keeping strength. Knee is sensitive to running."
            value={goals}
            onChange={(e) => setGoals(e.target.value)}
          />
        </div>
        <button className="btn-primary w-full" onClick={() => build(false)} disabled={busy}>
          {busy ? 'Writing your week…' : plan ? 'Rebuild the plan' : 'Build my week'}
        </button>
        <p className="text-center text-xs text-ink-600">
          Built from your actual completion record, not a template.
        </p>
      </section>

      {error && (
        <p className="rounded-xl border border-red-500/25 bg-red-500/10 px-4 py-3 text-sm text-red-400">
          {error}
        </p>
      )}

      {plan && (
        <div className="animate-slide-up space-y-4">
          <section className="card p-4">
            <p className="text-ink-200">{plan.summary}</p>
            {plan.focus && (
              <p className="mt-3 inline-block rounded-full bg-flame-500/15 px-3 py-1 text-sm text-flame-400">
                {plan.focus}
              </p>
            )}
            {cached && (
              <button
                className="mt-3 block text-xs text-ink-600 underline hover:text-ink-400"
                onClick={() => build(true)}
              >
                This is your last plan — generate a fresh one
              </button>
            )}
          </section>

          {plan.days.map((d) => (
            <section key={d.day} className="card p-4">
              <h3 className="mb-3 text-sm font-medium text-flame-500">Day {d.day}</h3>
              <dl className="space-y-2 text-sm">
                <Line term="Indoor" value={d.indoor} />
                <Line term="Outdoor" value={d.outdoor} />
                <Line term="Food" value={d.nutrition} />
              </dl>
              {d.note && <p className="mt-3 border-l-2 border-ink-700 pl-3 text-sm text-ink-500">{d.note}</p>}
            </section>
          ))}

          {plan.tips.length > 0 && (
            <section className="card p-4">
              <h3 className="mb-3 text-xs uppercase tracking-wide text-ink-500">Worth remembering</h3>
              <ul className="space-y-2 text-sm text-ink-300">
                {plan.tips.map((tip, i) => (
                  <li key={i} className="flex gap-2">
                    <span className="text-flame-500">•</span>
                    {tip}
                  </li>
                ))}
              </ul>
            </section>
          )}

          <p className="px-2 text-center text-xs text-ink-600">
            General fitness guidance, not medical advice.
          </p>
        </div>
      )}
    </div>
  )
}

function Line({ term, value }: { term: string; value: string }) {
  return (
    <div className="flex gap-3">
      <dt className="w-16 shrink-0 text-ink-600">{term}</dt>
      <dd className="min-w-0 flex-1 text-ink-300">{value}</dd>
    </div>
  )
}

function Chip({ children }: { children: React.ReactNode }) {
  return <span className="rounded-full bg-ink-850 px-2.5 py-1">{children}</span>
}

function errorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === 'ai_quota') return "You've used today's AI requests. The limit resets on a rolling 24 hours."
    if (err.code === 'ai_disabled') return 'AI features are not configured on this server.'
    if (err.code === 'no_active_program') return 'Start a program first.'
  }
  return err instanceof Error ? err.message : 'Something went wrong'
}
