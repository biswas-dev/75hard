import { useEffect, useState } from 'react'
import { api } from '../lib/api'

/**
 * A short daily note on the Today screen.
 *
 * Fetched lazily and failing silently: this is a nicety on top of the day, and
 * a provider being down or the quota being spent must never leave an error
 * banner on the screen someone opens every morning.
 */
export function CoachNote() {
  const [note, setNote] = useState<string | null>(null)
  const [dismissed, setDismissed] = useState(false)

  useEffect(() => {
    let cancelled = false

    api
      .aiStatus()
      .then((status) => {
        if (!status.enabled || cancelled) return
        return api.coachNote().then((res) => {
          if (!cancelled) setNote(res.note.note)
        })
      })
      .catch(() => {
        // Deliberately silent.
      })

    return () => {
      cancelled = true
    }
  }, [])

  if (!note || dismissed) return null

  return (
    <div className="card animate-slide-up border-flame-500/20 bg-flame-500/[0.05] p-4">
      <div className="flex items-start gap-3">
        <span className="mt-0.5 shrink-0 text-flame-500">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8">
            <path
              d="M12 3v4M12 17v4M3 12h4M17 12h4M5.6 5.6l2.8 2.8M15.6 15.6l2.8 2.8M18.4 5.6l-2.8 2.8M8.4 15.6l-2.8 2.8"
              strokeLinecap="round"
            />
          </svg>
        </span>
        <p className="min-w-0 flex-1 text-sm leading-relaxed text-ink-300">{note}</p>
        <button
          aria-label="Dismiss"
          className="-mt-1 shrink-0 p-1 text-lg leading-none text-ink-600 hover:text-ink-400"
          onClick={() => setDismissed(true)}
        >
          ×
        </button>
      </div>
    </div>
  )
}
