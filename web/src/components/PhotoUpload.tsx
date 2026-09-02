import { useRef, useState } from 'react'
import { api } from '../lib/api'
import { compressImage, formatBytes } from '../lib/compress'
import { CameraCapture } from './CameraCapture'
import type { Photo, Pose } from '../lib/types'

interface Props {
  kind: 'progress' | 'food' | 'ingredients'
  dayNumber?: number
  label: string
  onUploaded: (photo: Photo) => void
  /** Offer an angle picker. Only meaningful for progress shots. */
  withPose?: boolean
  /** Offer a meal picker. Only meaningful for food shots. */
  withSlot?: boolean
  /**
   * Food shots only: have the server create the meal and estimate it in the
   * background. Off for the meal sheet, which builds the meal from its form.
   */
  autolog?: boolean
}

const SLOTS = ['breakfast', 'lunch', 'dinner', 'snack'] as const

/** The rear camera is the only way to shoot your own back or side. */
const REAR_POSES: Pose[] = ['back', 'side']

/**
 * Camera-first upload button. Compresses in the browser before sending, and
 * reports the saving so the size reduction is visible rather than implied.
 *
 * Prefers the in-app camera, which is the only one that can offer a self-timer
 * on the rear camera — without that a back shot is impossible to take alone.
 * Falls back to the native picker where getUserMedia is unavailable.
 */
export function PhotoUpload({
  kind,
  dayNumber,
  label,
  onUploaded,
  withPose,
  withSlot,
  autolog,
}: Props) {
  const inputRef = useRef<HTMLInputElement>(null)
  const [busy, setBusy] = useState(false)
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const [cameraOpen, setCameraOpen] = useState(false)
  // Front by default: it is the shot people take without thinking, and an
  // untagged photo still counts, so this only saves a tap.
  const [pose, setPose] = useState<Pose>('front')
  // Left unset so the server can infer the meal from the time of day; picking
  // one overrides that.
  const [slot, setSlot] = useState<string>('')

  const canUseCamera =
    typeof navigator !== 'undefined' && !!navigator.mediaDevices?.getUserMedia

  async function upload(source: Blob) {
    setBusy(true)
    setError('')
    setStatus('Compressing…')

    try {
      const result = await compressImage(source)
      setStatus(`Uploading ${formatBytes(result.bytes)}…`)

      const photo = await api.uploadPhoto(result.blob, {
        kind,
        dayNumber,
        pose: withPose ? pose : undefined,
        slot: withSlot && slot ? slot : undefined,
        autolog,
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

  async function handleFile(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return
    // Reset immediately so picking the same file twice still fires a change.
    e.target.value = ''
    await upload(file)
  }

  return (
    <div>
      {cameraOpen && (
        <CameraCapture
          label={label}
          // A back or side shot needs the rear camera and a timer; opening on
          // the right one saves a tap at exactly the moment it is awkward.
          initialFacing={withPose && REAR_POSES.includes(pose) ? 'environment' : 'user'}
          onClose={() => setCameraOpen(false)}
          onCapture={(blob) => {
            setCameraOpen(false)
            void upload(blob)
          }}
        />
      )}

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

      {withSlot && (
        <div className="mb-2 grid grid-cols-5 gap-1.5">
          <button
            type="button"
            onClick={() => setSlot('')}
            className={`rounded-xl border py-2 text-xs transition ${
              slot === ''
                ? 'border-flame-500 bg-flame-500/15 text-flame-400'
                : 'border-ink-800 bg-ink-850 text-ink-400'
            }`}
            title="Tagged from the time of day"
          >
            Auto
          </button>
          {SLOTS.map((s) => (
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
        onClick={() => (canUseCamera ? setCameraOpen(true) : inputRef.current?.click())}
      >
        {busy ? <Spinner /> : <CameraIcon />}
        {busy ? status || 'Working…' : label}
      </button>

      {canUseCamera && !busy && (
        // The in-app camera cannot reach the photo library, so keep a way in.
        <button
          type="button"
          className="mt-2 w-full text-center text-xs text-ink-500 hover:text-ink-300"
          onClick={() => inputRef.current?.click()}
        >
          Choose an existing photo
        </button>
      )}

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
