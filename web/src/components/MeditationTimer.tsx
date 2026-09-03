import { useCallback, useEffect, useRef, useState } from 'react'

const STORAGE_KEY = '75hard.meditation.session'

interface Running {
  startedAt: number
  targetMinutes: number
}

interface Props {
  /** Called with the elapsed minutes when a sitting is finished. */
  onFinish: (minutes: number) => void
  busy?: boolean
}

/**
 * A timer for a sitting, which logs itself when it ends.
 *
 * Two things shape the mechanics. Elapsed time is computed from a stored start
 * timestamp rather than counted by an interval, because a phone put face down
 * throttles timers and a counted tick would drift or stop entirely. And that
 * timestamp is written to storage, because the whole point is to put the phone
 * down — a locked screen, a discarded tab or an accidental reload must not cost
 * somebody the sitting they just did.
 *
 * The running state takes over the screen. Anything else on it is a thing to
 * look at instead of sitting, and a countdown competing with a progress ring
 * and a food log is not a timer anybody wants to meditate to.
 */
export function MeditationTimer({ onFinish, busy }: Props) {
  const [running, setRunning] = useState<Running | null>(null)
  const [now, setNow] = useState(() => Date.now())
  const chimed = useRef(false)

  // Restore a sitting that outlived the page.
  useEffect(() => {
    try {
      const raw = window.localStorage.getItem(STORAGE_KEY)
      if (!raw) return
      const parsed = JSON.parse(raw) as Running
      if (typeof parsed.startedAt === 'number' && parsed.startedAt > 0) {
        setRunning(parsed)
      }
    } catch {
      // A corrupt entry is not worth reporting; it just means no sitting.
    }
  }, [])

  // Re-render every second. Everything shown derives from the timestamps, so
  // a throttled or missed tick changes nothing but the smoothness.
  useEffect(() => {
    if (!running) return
    const id = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(id)
  }, [running])

  const elapsedSec = running ? Math.max(0, Math.floor((now - running.startedAt) / 1000)) : 0
  const targetSec = running ? running.targetMinutes * 60 : 0
  const remaining = targetSec > 0 ? Math.max(0, targetSec - elapsedSec) : 0
  const reached = targetSec > 0 && elapsedSec >= targetSec

  // One quiet buzz at the bell. It does not stop the timer: sitting on past it
  // is normal, and the app has no business interrupting that.
  useEffect(() => {
    if (reached && !chimed.current) {
      chimed.current = true
      navigator.vibrate?.([120, 80, 120])
    }
  }, [reached])

  const start = useCallback((targetMinutes: number) => {
    const session = { startedAt: Date.now(), targetMinutes }
    chimed.current = false
    setRunning(session)
    setNow(Date.now())
    try {
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(session))
    } catch {
      // Private browsing can refuse storage. The timer still works while the
      // page stays open, which is the common case anyway.
    }
  }, [])

  const clear = useCallback(() => {
    setRunning(null)
    chimed.current = false
    try {
      window.localStorage.removeItem(STORAGE_KEY)
    } catch {
      // The in-memory state is already cleared; nothing else to do.
    }
  }, [])

  function finish() {
    if (!running) return
    const minutes = loggedMinutes(elapsedSec)
    clear()
    onFinish(minutes)
  }

  if (!running) {
    return (
      <div>
        <p className="mb-1.5 text-xs text-ink-500">Sit now</p>
        <div className="flex flex-wrap gap-1.5">
          {[5, 10, 15, 20].map((m) => (
            <button
              key={m}
              type="button"
              onClick={() => start(m)}
              className="rounded-lg border border-ink-800 bg-ink-850 px-3 py-1.5 text-sm text-ink-300 transition hover:border-ink-700 hover:text-ink-100"
            >
              {m} min
            </button>
          ))}
          <button
            type="button"
            onClick={() => start(0)}
            className="rounded-lg border border-ink-800 bg-ink-850 px-3 py-1.5 text-sm text-ink-400 transition hover:border-ink-700 hover:text-ink-100"
            title="No bell — finish when you are done"
          >
            Open ended
          </button>
        </div>
      </div>
    )
  }

  // Progress fills as the sitting goes. Open ended has nothing to fill toward,
  // so the ring breathes instead of measuring.
  const progress = targetSec > 0 ? Math.min(1, elapsedSec / targetSec) : 0

  const size = 280
  const stroke = 3
  const radius = (size - stroke) / 2
  const circumference = 2 * Math.PI * radius

  return (
    <div
      className="fixed inset-0 z-50 flex flex-col items-center justify-center bg-ink-950"
      role="dialog"
      aria-modal="true"
      aria-label="Meditation timer"
    >
      {/* A slow wash of colour behind the ring, so the screen is not a flat
          black rectangle for twenty minutes. */}
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0 opacity-70"
        style={{
          background:
            'radial-gradient(circle at 50% 45%, rgba(125,211,252,0.10), transparent 55%),' +
            'radial-gradient(circle at 50% 55%, rgba(255,107,53,0.07), transparent 60%)',
        }}
      />

      <div className="relative flex flex-col items-center">
        <svg width={size} height={size} className="-rotate-90" aria-hidden>
          <circle
            cx={size / 2}
            cy={size / 2}
            r={radius}
            fill="none"
            stroke="#1b1b22"
            strokeWidth={stroke}
          />
          {targetSec > 0 && (
            <circle
              cx={size / 2}
              cy={size / 2}
              r={radius}
              fill="none"
              stroke="#7dd3fc"
              strokeWidth={stroke}
              strokeLinecap="round"
              strokeDasharray={circumference}
              strokeDashoffset={circumference * (1 - progress)}
              style={{ transition: 'stroke-dashoffset 1s linear' }}
            />
          )}
        </svg>

        <div className="absolute inset-0 flex flex-col items-center justify-center">
          {/* The breathing mark: four seconds out, four back. Something to
              settle on without instructing anybody how to breathe. */}
          <span
            aria-hidden
            className="mb-5 block h-2 w-2 rounded-full bg-sky-300/70"
            style={{ animation: 'breathe 8s ease-in-out infinite' }}
          />
          <p className="font-mono text-6xl tabular-nums text-ink-100">
            {targetSec > 0 && !reached ? formatClock(remaining) : formatClock(elapsedSec)}
          </p>
          <p className="mt-3 text-sm text-ink-500">
            {targetSec === 0
              ? 'Open ended'
              : reached
                ? `${running.targetMinutes} minutes done`
                : `of ${running.targetMinutes} minutes`}
          </p>
        </div>
      </div>

      <div className="relative mt-14 flex w-full max-w-xs flex-col gap-2 px-6">
        <button
          className="rounded-xl bg-flame-500 py-3 font-medium text-ink-950 transition hover:bg-flame-400 disabled:opacity-50"
          onClick={finish}
          disabled={busy || elapsedSec < 1}
        >
          {busy ? 'Saving…' : 'Finish and log'}
        </button>
        <button
          className="rounded-xl py-2.5 text-sm text-ink-600 transition hover:text-ink-400"
          onClick={clear}
        >
          Discard
        </button>
      </div>

      <p className="relative mt-6 px-8 text-center text-xs text-ink-700">
        Keeps running if you lock the phone or close the tab.
      </p>

      <style>{`
        @keyframes breathe {
          0%, 100% { transform: scale(1); opacity: 0.45; }
          50% { transform: scale(3.2); opacity: 0.9; }
        }
      `}</style>
    </div>
  )
}

export function formatClock(totalSeconds: number) {
  const m = Math.floor(totalSeconds / 60)
  const s = totalSeconds % 60
  return `${m}:${String(s).padStart(2, '0')}`
}

/**
 * The minutes a sitting is recorded as.
 *
 * Rounded, not truncated: nine minutes and forty seconds is a ten minute
 * sitting to anybody who just sat it, and flooring would quietly shave time
 * off every session. Never below one, because a sitting that happened should
 * not record as zero.
 */
export function loggedMinutes(elapsedSeconds: number) {
  return Math.max(1, Math.round(elapsedSeconds / 60))
}
