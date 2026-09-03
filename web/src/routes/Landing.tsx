import { useMemo } from 'react'
import { Link } from 'react-router-dom'

/**
 * The public front page.
 *
 * 75 Hard is a challenge about unglamorous repetition, so the page leads with
 * the thing that actually represents it — a 75-cell grid — rather than a stock
 * hero photograph of someone running at sunrise. The copy states the rules
 * plainly and says what the app does and does not do, because anyone reading
 * this is deciding whether to commit 75 days, and overselling it would be a
 * poor way to start.
 */
export function Landing() {
  // A fixed, deterministic pattern: mostly done, a couple of gaps late on.
  // Deliberately not random — a grid that reshuffles on every render reads as
  // decoration, and this is meant to read as somebody's actual run.
  const cells = useMemo(() => {
    const missed = new Set([38, 52])
    return Array.from({ length: 75 }, (_, i) => {
      const day = i + 1
      if (missed.has(day)) return 'missed'
      if (day <= 61) return 'done'
      return 'future'
    })
  }, [])

  return (
    <div className="min-h-dvh bg-ink-950 text-ink-200">
      <header className="mx-auto flex max-w-5xl items-center justify-between px-5 py-5">
        <span className="flex items-center gap-2 font-semibold tracking-tight text-ink-100">
          <Flame />
          75 Hard
        </span>
        <nav className="flex items-center gap-1 text-sm">
          <Link to="/login" className="rounded-lg px-3 py-2 text-ink-400 hover:text-ink-200">
            Sign in
          </Link>
          <Link
            to="/signup"
            className="rounded-lg bg-flame-500 px-4 py-2 font-medium text-ink-950 hover:bg-flame-400"
          >
            Start
          </Link>
        </nav>
      </header>

      <main className="mx-auto max-w-5xl px-5 pb-24">
        {/* Hero */}
        <section className="border-b border-ink-800 py-14 sm:py-20">
          <div className="grid gap-12 lg:grid-cols-[1.1fr_1fr] lg:items-center">
            <div>
              <p className="font-mono text-xs uppercase tracking-[0.2em] text-flame-500">
                Seventy-five days. No substitutions.
              </p>
              <h1 className="mt-4 text-4xl font-semibold leading-[1.1] tracking-tight text-ink-100 sm:text-5xl">
                Six things, every day,
                <br />
                for seventy-five days.
              </h1>
              <p className="mt-5 max-w-xl text-base leading-relaxed text-ink-400">
                Miss one and you start again at day one. That rule is the whole point, and it is
                also why the tracking matters — you need to know exactly where you stand, on a
                phone, in ten seconds, at six in the morning.
              </p>
              <p className="mt-3 max-w-xl text-base leading-relaxed text-ink-400">
                This is the tracker I built to do my own run. It is free, it has no advertising,
                and nobody is selling your progress photos.
              </p>

              <div className="mt-8 flex flex-wrap items-center gap-3">
                <Link
                  to="/signup"
                  className="rounded-xl bg-flame-500 px-6 py-3 font-medium text-ink-950 transition hover:bg-flame-400"
                >
                  Create an account
                </Link>
                <Link
                  to="/login"
                  className="rounded-xl border border-ink-800 px-6 py-3 font-medium text-ink-300 transition hover:border-ink-700 hover:text-ink-100"
                >
                  I already have one
                </Link>
              </div>
            </div>

            {/* The grid: one cell per day. */}
            <div className="rounded-2xl border border-ink-800 bg-ink-900 p-5">
              <div className="mb-3 flex items-baseline justify-between">
                <span className="font-mono text-xs uppercase tracking-wider text-ink-500">
                  Attempt 2
                </span>
                <span className="font-mono text-xs text-ink-500">day 61 / 75</span>
              </div>
              <div className="grid grid-cols-[repeat(15,minmax(0,1fr))] gap-1.5">
                {cells.map((state, i) => (
                  <span
                    key={i}
                    title={`Day ${i + 1}`}
                    className={`aspect-square rounded-[3px] ${
                      state === 'done'
                        ? 'bg-flame-500'
                        : state === 'missed'
                          ? 'bg-ink-700'
                          : 'bg-ink-850'
                    }`}
                  />
                ))}
              </div>
              <p className="mt-4 text-xs leading-relaxed text-ink-600">
                Every task gets its own grid like this. The two grey squares are the mornings
                that ended somebody's first attempt — which is exactly the kind of thing you want
                to be able to see at a glance.
              </p>
            </div>
          </div>
        </section>

        {/* The rules */}
        <section className="border-b border-ink-800 py-14">
          <h2 className="text-2xl font-semibold tracking-tight text-ink-100">The six rules</h2>
          <p className="mt-2 max-w-2xl text-ink-400">
            These are not mine — they are the challenge as Andy Frisella wrote it. The app tracks
            them as written, and lets you change any of them if your run looks different.
          </p>

          <ol className="mt-8 grid gap-px overflow-hidden rounded-2xl border border-ink-800 bg-ink-800 sm:grid-cols-2">
            {RULES.map((rule, i) => (
              <li key={rule.title} className="bg-ink-900 p-5">
                <div className="flex items-baseline gap-3">
                  <span className="font-mono text-xs text-flame-500">
                    {String(i + 1).padStart(2, '0')}
                  </span>
                  <h3 className="font-medium text-ink-100">{rule.title}</h3>
                </div>
                <p className="mt-2 text-sm leading-relaxed text-ink-400">{rule.detail}</p>
              </li>
            ))}
          </ol>

          <p className="mt-6 text-sm text-ink-500">
            No alcohol, no cheat meals, and the workouts have to be two separate sessions. If you
            miss anything, the day does not count and the run restarts.
          </p>
        </section>

        {/* What the app does */}
        <section className="border-b border-ink-800 py-14">
          <h2 className="text-2xl font-semibold tracking-tight text-ink-100">
            What the tracker actually does
          </h2>

          <div className="mt-8 grid gap-8 sm:grid-cols-2">
            {FEATURES.map((f) => (
              <div key={f.title}>
                <h3 className="font-medium text-ink-100">{f.title}</h3>
                <p className="mt-2 text-sm leading-relaxed text-ink-400">{f.detail}</p>
              </div>
            ))}
          </div>
        </section>

        {/* Honest limits */}
        <section className="border-b border-ink-800 py-14">
          <h2 className="text-2xl font-semibold tracking-tight text-ink-100">
            Things worth knowing first
          </h2>
          <ul className="mt-6 space-y-4">
            {CAVEATS.map((c) => (
              <li key={c} className="flex gap-3 text-sm leading-relaxed text-ink-400">
                <span aria-hidden className="mt-2 h-1 w-1 shrink-0 rounded-full bg-ink-600" />
                <span>{c}</span>
              </li>
            ))}
          </ul>
        </section>

        {/* Close */}
        <section className="py-14">
          <div className="rounded-2xl border border-ink-800 bg-gradient-to-b from-ink-900 to-ink-950 p-8 sm:p-10">
            <h2 className="text-2xl font-semibold tracking-tight text-ink-100">
              Day one is the easy one.
            </h2>
            <p className="mt-3 max-w-xl text-ink-400">
              Pick a start date, take the first photo, and get on with it. The account takes about
              twenty seconds to make and you can delete it whenever you want.
            </p>
            <Link
              to="/signup"
              className="mt-7 inline-block rounded-xl bg-flame-500 px-6 py-3 font-medium text-ink-950 transition hover:bg-flame-400"
            >
              Start day one
            </Link>
          </div>
        </section>
      </main>

      <footer className="border-t border-ink-800">
        <div className="mx-auto flex max-w-5xl flex-col gap-2 px-5 py-8 text-sm text-ink-600 sm:flex-row sm:items-center sm:justify-between">
          <span>
            Built by{' '}
            <a href="https://biswas.me" className="text-ink-400 hover:text-ink-200">
              Anshuman Biswas
            </a>
          </span>
          <span className="flex gap-4">
            <a href="/api/openapi.yaml" className="hover:text-ink-400">
              API
            </a>
            <a
              href="https://github.com/biswas-dev/75hard"
              className="hover:text-ink-400"
              target="_blank"
              rel="noreferrer"
            >
              Source
            </a>
          </span>
        </div>
      </footer>
    </div>
  )
}

const RULES = [
  {
    title: 'Two 45-minute workouts',
    detail: 'At least one outdoors, whatever the weather, and the second starting two hours after the first.',
  },
  {
    title: 'Follow a diet',
    detail: 'Any diet you choose, followed properly. No cheat meals and no alcohol.',
  },
  {
    title: 'Drink a gallon of water',
    detail: 'Roughly 3.8 litres, or 128 ounces, over the course of the day.',
  },
  {
    title: 'Read ten pages',
    detail: 'Non-fiction, and actual pages — audiobooks do not count under the original rules.',
  },
  {
    title: 'Take a progress photo',
    detail: 'One a day. The point is the comparison at the end, not any single picture.',
  },
  {
    title: 'No compromises',
    detail: 'Miss any of the above and the day does not count. You go back to day one.',
  },
]

const FEATURES = [
  {
    title: 'One tap per task',
    detail:
      'The day screen is six things and a progress ring. Tick them off and it celebrates when the last one lands. That is the whole daily interaction, and it is meant to take seconds.',
  },
  {
    title: 'A grid for every activity',
    detail:
      'Seventy-five cells per task, coloured however you like. Long-press a cell to see or edit that day. It is the fastest way to spot the habit that keeps slipping.',
  },
  {
    title: 'Photos that stay yours',
    detail:
      'The in-app camera has a self-timer and switches to the rear lens, so you can actually take a back shot alone. Images are compressed in the browser and re-encoded on the server, which strips the EXIF — including the GPS coordinates of your bedroom.',
  },
  {
    title: 'Food photos, priced in the background',
    detail:
      'Photograph a meal and it is logged and tagged by time of day immediately. An AI estimate of the calories fills in behind it, so you never wait on a model. Or type the meal in yourself. Or just tick the box and move on.',
  },
  {
    title: 'Strava, if you use it',
    detail:
      'Connect once and your activities import themselves. Sessions are grouped by when they started, so a morning walk and an evening swim count as two workouts, and your average training heart rate is charted across the run.',
  },
  {
    title: 'Optional extras that cannot fail you',
    detail:
      'Meditation, weight and resting pulse are all tracked if you want them and ignored if you do not. None of them are part of the six rules, so missing one never ends your run.',
  },
]

const CAVEATS = [
  'This is a personal project, not a company. It works well and it is maintained, but there is no support desk behind it.',
  'The challenge itself is genuinely demanding. It is not a fitness programme and it was never designed by a doctor — if you have any medical reason to be careful, talk to someone qualified before starting.',
  'Progress photos and meal photos are stored on the server behind your account. They are never public and never shared, but they do leave your phone.',
  'The AI features are optional and rate limited. Everything else works with them switched off entirely.',
]

function Flame() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden>
      <path
        d="M12 2c3 4 6 6 6 10a6 6 0 1 1-12 0c0-2 1-3 2-4 0 2 1 3 2 3 0-3 1-6 2-9Z"
        fill="#ff6b35"
      />
    </svg>
  )
}
