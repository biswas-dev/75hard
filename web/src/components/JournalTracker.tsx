import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '../lib/api'
import type { Day, Entry, JournalEntry } from '../lib/types'

/**
 * Journaling: type an entry, or upload a page you wrote by hand.
 *
 * Optional, like meditation, and never able to fail a run — the panel says so,
 * because a tracker that looks mandatory changes how the whole challenge feels.
 *
 * A handwritten upload is transcribed in the background so the page becomes
 * searchable, and the transcription is shown as clearly separate from anything
 * typed: it is a machine's reading of somebody's handwriting, not their words.
 */
export function JournalTracker({
  day,
  onChanged,
}: {
  entry: Entry
  day: Day
  onChanged: () => void
}) {
  const [entries, setEntries] = useState<JournalEntry[]>([])
  const [body, setBody] = useState('')
  const [title, setTitle] = useState('')
  const [query, setQuery] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const fileRef = useRef<HTMLInputElement>(null)

  const load = useCallback(async (q?: string) => {
    try {
      setEntries(await api.journal(q))
    } catch {
      setEntries([])
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  // Search as you type, but not on every keystroke.
  useEffect(() => {
    const t = window.setTimeout(() => load(query.trim() || undefined), 300)
    return () => window.clearTimeout(t)
  }, [query, load])

  // A transcription arrives behind the upload with no way to push, so poll
  // while one is running and stop the moment nothing is pending.
  const pending = entries.some((e) => e.parse_status === 'pending')
  useEffect(() => {
    if (!pending) return
    const id = window.setInterval(() => load(query.trim() || undefined), 5000)
    return () => window.clearInterval(id)
  }, [pending, query, load])

  async function save() {
    if (busy || !body.trim()) return
    setBusy(true)
    setError('')
    try {
      await api.createJournal({
        day_number: day.day_number,
        title: title.trim(),
        body: body.trim(),
      })
      setBody('')
      setTitle('')
      await load()
      onChanged()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not save the entry')
    } finally {
      setBusy(false)
    }
  }

  async function upload(file: File) {
    setBusy(true)
    setError('')
    try {
      await api.uploadJournal(file, { dayNumber: day.day_number, title: title.trim() })
      setTitle('')
      await load()
      onChanged()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not upload that page')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <input
          className="field"
          placeholder="Title (optional)"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
        />
        <textarea
          className="field min-h-28 resize-y"
          placeholder="Write it down."
          value={body}
          onChange={(e) => setBody(e.target.value)}
        />
        <div className="flex gap-2">
          <button
            className="btn-ghost flex-1 py-2 text-sm"
            onClick={save}
            disabled={busy || !body.trim()}
          >
            {busy ? 'Saving…' : 'Save entry'}
          </button>
          <button
            className="btn-ghost flex-1 py-2 text-sm"
            onClick={() => fileRef.current?.click()}
            disabled={busy}
          >
            Upload a page
          </button>
        </div>
        <input
          ref={fileRef}
          type="file"
          accept="application/pdf"
          className="hidden"
          onChange={(e) => {
            const f = e.target.files?.[0]
            e.target.value = ''
            if (f) void upload(f)
          }}
        />
        <p className="text-xs text-ink-600">
          A PDF you wrote by hand is read in the background so you can search it later. The
          original page is always kept.
        </p>
      </div>

      {(entries.length > 0 || query) && (
        <input
          className="field"
          placeholder="Search everything you have written"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      )}

      {entries.length > 0 && (
        <ul className="divide-y divide-ink-800">
          {entries.map((e) => (
            <li key={e.id} className="py-3">
              <div className="flex items-baseline gap-2">
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm text-ink-200">
                    {e.title || (e.kind === 'pdf' ? e.file_name : 'Untitled')}
                  </span>
                  <span className="font-mono text-xs text-ink-600">
                    {e.day_number ? `day ${e.day_number} · ` : ''}
                    {new Date(e.created_at).toLocaleDateString()}
                    {e.kind === 'pdf' && ' · page'}
                    {e.parse_status === 'pending' && ' · reading the page…'}
                    {e.parse_status === 'failed' && ' · could not be read'}
                  </span>
                </span>
                {e.has_file && (
                  <a
                    href={`/api/journal/${e.id}/file`}
                    target="_blank"
                    rel="noreferrer"
                    className="shrink-0 text-xs text-flame-400 hover:underline"
                  >
                    open
                  </a>
                )}
                <button
                  aria-label="Delete entry"
                  className="shrink-0 px-1 text-ink-600 hover:text-red-400"
                  onClick={async () => {
                    await api.deleteJournal(e.id)
                    await load(query.trim() || undefined)
                    onChanged()
                  }}
                >
                  ×
                </button>
              </div>

              {e.snippet ? (
                <p className="mt-1.5 text-xs leading-relaxed text-ink-400">{e.snippet}</p>
              ) : (
                e.body && (
                  <p className="mt-1.5 line-clamp-3 text-xs leading-relaxed text-ink-400">
                    {e.body}
                  </p>
                )
              )}

              {e.parse_status === 'done' && e.parsed_text && !e.snippet && (
                <details className="mt-1.5">
                  <summary className="cursor-pointer text-xs text-ink-600">
                    Show what was read from the page
                  </summary>
                  <p className="mt-1 whitespace-pre-wrap text-xs leading-relaxed text-ink-500">
                    {e.parsed_text}
                  </p>
                  <p className="mt-1 text-[11px] text-ink-700">
                    A machine's reading of your handwriting, kept separate from the page itself.
                  </p>
                </details>
              )}

              {e.parse_status === 'failed' && e.parse_error && (
                <p className="mt-1 text-xs text-ink-600">{e.parse_error}</p>
              )}
            </li>
          ))}
        </ul>
      )}

      {error && <p className="text-sm text-red-400">{error}</p>}

      <p className="text-center text-xs text-ink-600">
        Optional — journaling is not one of the six rules, and skipping it never fails your run.
      </p>
    </div>
  )
}
