import { useRef, useState } from 'react'
import { api } from '../lib/api'
import { compressImage, formatBytes } from '../lib/compress'
import type { Photo, Pose } from '../lib/types'

interface Props {
  kind: 'progress' | 'food' | 'ingredients'
  dayNumber?: number
  label: string
  onUploaded: (photo: Photo) => void
  /** Offer an angle picker. Only meaningful for progress shots. */
  withPose?: boolean
}

/**
 * Camera-first upload button. Compresses in the browser before sending, and
 * reports the saving so the size reduction is visible rather than implied.
 */
export function PhotoUpload({ kind, dayNumber, label, onUploaded, withPose }: Props) {
  const inputRef = useRef<HTMLInputElement>(null)
  const [busy, setBusy] = useState(false)
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  // Front by default: it is the shot people take without thinking, and an
  // untagged photo still counts, so this only saves a tap.
  const [pose, setPose] = useState<Pose>('front')

  async function handleFile(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return
    // Reset immediately so picking the same file twice still fires a change.
    e.target.value = ''

    setBusy(true)
    setError('')
    setStatus('Compressing…')

    try {
      const result = await compressImage(file)
      setStatus(`Uploading ${formatBytes(result.bytes)}…`)

      const photo = await api.uploadPhoto(result.blob, {
        kind,
        dayNumber,
        pose: withPose ? pose : undefined,
      })

      const saved = result.originalBytes - result.bytes
      setStatus(
        saved > 0
          ? `Saved — ${formatBytes(result.originalBytes)} → ${formatBytes(result.bytes)}`
          : 'Saved',
      )
      onUploaded(photo)
      window.setTimeout(() => setStatus(''), 4000)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Upload failed')
      setStatus('')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div>
      {withPose && (
        <div className="mb-2 grid grid-cols-3 gap-2">
          {(['front', 'side', 'back'] as Pose[]).map((p) => (
            <button
              key={p}
              type="button"
              onClick={() => setPose(p)}
              className={`rounded-xl border py-2 text-sm capitalize transition ${
                pose === p
                  ? 'border-flame-500 bg-flame-500/15 text-flame-400'
                  : 'border-ink-800 bg-ink-850 text-ink-400'
              }`}
            >
              {p}
            </button>
          ))}
        </div>
      )}

      <input
        ref={inputRef}
        type="file"
        accept="image/*"
        // Opens the camera directly on a phone rather than the file browser.
        capture="environment"
        onChange={handleFile}
        className="hidden"
      />
      <button
        type="button"
        className="btn-ghost w-full"
        disabled={busy}
        onClick={() => inputRef.current?.click()}
      >
        {busy ? <Spinner /> : <CameraIcon />}
        {busy ? status || 'Working…' : label}
      </button>

      {status && !busy && <p className="mt-2 text-center text-sm text-moss-400">{status}</p>}
      {error && <p className="mt-2 text-center text-sm text-red-400">{error}</p>}
    </div>
  )
}

function CameraIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8">
      <path d="M3 8h3l2-3h8l2 3h3v11H3z" strokeLinejoin="round" />
      <circle cx="12" cy="13" r="3.5" />
    </svg>
  )
}

function Spinner() {
  return (
    <svg className="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none">
      <circle cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="3" className="opacity-25" />
      <path d="M12 2a10 10 0 0 1 10 10" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
    </svg>
  )
}
