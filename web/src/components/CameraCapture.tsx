import { useCallback, useEffect, useRef, useState } from 'react'

type Facing = 'user' | 'environment'

interface Props {
  /** Called with the captured frame. The caller compresses and uploads. */
  onCapture: (blob: Blob) => void
  onClose: () => void
  /** Which camera to open with. Rear for a back shot, front for a selfie. */
  initialFacing?: Facing
  /** Seconds the self-timer starts on. Zero shoots immediately. */
  initialTimer?: number
  label?: string
}

const TIMERS = [0, 3, 10] as const

/**
 * In-app camera with a self-timer and a working front/rear switch.
 *
 * The native file input (`capture="environment"`) cannot do this. It hands off
 * to the phone's camera app, which has no timer we can drive — so photographing
 * your own back is impossible: you cannot see the screen or reach the shutter
 * while standing in position. A live preview we control is the only way to
 * offer a countdown on *either* camera.
 */
export function CameraCapture({
  onCapture,
  onClose,
  initialFacing = 'user',
  initialTimer = 0,
  label,
}: Props) {
  const videoRef = useRef<HTMLVideoElement>(null)
  const streamRef = useRef<MediaStream | null>(null)
  // Held in a ref as well as state so the countdown's timeouts can be cleared
  // from the unmount cleanup, which does not see the latest state.
  const countdownRef = useRef<number | null>(null)

  const [facing, setFacing] = useState<Facing>(initialFacing)
  const [timer, setTimer] = useState<number>(initialTimer)
  const [counting, setCounting] = useState<number | null>(null)
  const [error, setError] = useState('')
  const [ready, setReady] = useState(false)
  const [flash, setFlash] = useState(false)
  const [multiCamera, setMultiCamera] = useState(true)

  const stopStream = useCallback(() => {
    streamRef.current?.getTracks().forEach((t) => t.stop())
    streamRef.current = null
  }, [])

  // Open the requested camera, replacing whatever was running.
  useEffect(() => {
    let cancelled = false

    async function open() {
      setReady(false)
      setError('')
      stopStream()

      if (!navigator.mediaDevices?.getUserMedia) {
        setError('This browser cannot open the camera directly.')
        return
      }

      try {
        const stream = await navigator.mediaDevices.getUserMedia({
          // `ideal` rather than `exact`: a laptop with one camera should still
          // work rather than throwing OverconstrainedError.
          video: { facingMode: { ideal: facing }, width: { ideal: 1920 }, height: { ideal: 1920 } },
          audio: false,
        })
        if (cancelled) {
          stream.getTracks().forEach((t) => t.stop())
          return
        }
        streamRef.current = stream
        if (videoRef.current) {
          videoRef.current.srcObject = stream
          await videoRef.current.play().catch(() => {})
        }
        setReady(true)
      } catch (err) {
        if (cancelled) return
        setError(
          err instanceof DOMException && err.name === 'NotAllowedError'
            ? 'Camera access was blocked. Allow it in your browser settings and try again.'
            : 'Could not open the camera.',
        )
      }
    }

    open()
    return () => {
      cancelled = true
    }
  }, [facing, stopStream])

  // Tear everything down on unmount: a live camera left running is both a
  // battery drain and a privacy problem.
  useEffect(() => {
    return () => {
      if (countdownRef.current) window.clearTimeout(countdownRef.current)
      stopStream()
    }
  }, [stopStream])

  // Report whether there is more than one camera, so the switch is not offered
  // on a device where it does nothing.
  useEffect(() => {
    navigator.mediaDevices
      ?.enumerateDevices?.()
      .then((devices) => setMultiCamera(devices.filter((d) => d.kind === 'videoinput').length > 1))
      .catch(() => setMultiCamera(true))
  }, [ready])

  const shoot = useCallback(() => {
    const video = videoRef.current
    if (!video || !video.videoWidth) return

    const canvas = document.createElement('canvas')
    canvas.width = video.videoWidth
    canvas.height = video.videoHeight
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    // The preview is mirrored for the front camera so it reads like a mirror,
    // but the saved photo must not be — a mirrored progress shot flips every
    // asymmetry and makes a side-by-side comparison misleading.
    ctx.drawImage(video, 0, 0)

    setFlash(true)
    window.setTimeout(() => setFlash(false), 150)

    canvas.toBlob(
      (blob) => {
        if (blob) onCapture(blob)
      },
      'image/jpeg',
      0.92,
    )
  }, [onCapture])

  const start = useCallback(() => {
    if (!ready || counting !== null) return
    if (timer === 0) {
      shoot()
      return
    }

    let remaining = timer
    setCounting(remaining)

    const tick = () => {
      remaining -= 1
      if (remaining <= 0) {
        setCounting(null)
        countdownRef.current = null
        shoot()
        return
      }
      setCounting(remaining)
      countdownRef.current = window.setTimeout(tick, 1000)
    }
    countdownRef.current = window.setTimeout(tick, 1000)
  }, [ready, counting, timer, shoot])

  const cancelCountdown = useCallback(() => {
    if (countdownRef.current) window.clearTimeout(countdownRef.current)
    countdownRef.current = null
    setCounting(null)
  }, [])

  // Escape closes, space shoots — useful on a laptop, harmless on a phone.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        counting !== null ? cancelCountdown() : onClose()
      }
      if (e.key === ' ' || e.key === 'Enter') {
        e.preventDefault()
        counting !== null ? cancelCountdown() : start()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [counting, cancelCountdown, onClose, start])

  return (
    <div className="fixed inset-0 z-50 flex flex-col bg-black" role="dialog" aria-modal="true" aria-label="Camera">
      <div className="flex items-center justify-between px-4 py-3 text-white">
        <button type="button" onClick={onClose} className="rounded-lg px-3 py-1.5 text-sm text-white/80">
          Cancel
        </button>
        <p className="text-sm font-medium text-white/90">{label ?? 'Take a photo'}</p>
        {multiCamera ? (
          <button
            type="button"
            onClick={() => {
              cancelCountdown()
              setFacing((f) => (f === 'user' ? 'environment' : 'user'))
            }}
            className="rounded-lg px-3 py-1.5 text-sm text-white/80"
            aria-label={facing === 'user' ? 'Switch to the rear camera' : 'Switch to the front camera'}
          >
            <FlipIcon />
          </button>
        ) : (
          <span className="w-16" />
        )}
      </div>

      <div className="relative flex-1 overflow-hidden">
        <video
          ref={videoRef}
          playsInline
          muted
          autoPlay
          className="h-full w-full object-cover"
          // Mirrored preview only; the capture above is deliberately not.
          style={{ transform: facing === 'user' ? 'scaleX(-1)' : undefined }}
        />

        {counting !== null && (
          <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
            <span className="text-[8rem] font-bold leading-none text-white drop-shadow-lg tabular-nums">
              {counting}
            </span>
          </div>
        )}

        {flash && <div className="pointer-events-none absolute inset-0 bg-white" />}

        {error && (
          <div className="absolute inset-0 flex items-center justify-center p-8">
            <p className="text-center text-sm text-white/90">{error}</p>
          </div>
        )}
      </div>

      <div className="space-y-4 px-4 pb-8 pt-4">
        <div className="flex items-center justify-center gap-2">
          {TIMERS.map((t) => (
            <button
              key={t}
              type="button"
              onClick={() => setTimer(t)}
              className={`rounded-full px-4 py-1.5 text-sm transition ${
                timer === t ? 'bg-white text-black' : 'bg-white/15 text-white/80'
              }`}
            >
              {t === 0 ? 'Off' : `${t}s`}
            </button>
          ))}
        </div>

        {timer > 0 && facing === 'environment' && (
          <p className="text-center text-xs text-white/60">
            Prop the phone up, then get into position — you have {timer} seconds.
          </p>
        )}

        <div className="flex justify-center">
          <button
            type="button"
            onClick={counting !== null ? cancelCountdown : start}
            disabled={!ready && counting === null}
            aria-label={counting !== null ? 'Cancel the timer' : 'Take the photo'}
            className="flex h-20 w-20 items-center justify-center rounded-full border-4 border-white disabled:opacity-40"
          >
            <span
              className={`transition-all ${
                counting !== null ? 'h-7 w-7 rounded bg-red-500' : 'h-16 w-16 rounded-full bg-white'
              }`}
            />
          </button>
        </div>
      </div>
    </div>
  )
}

function FlipIcon() {
  return (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8">
      <path d="M3 8h3l2-3h8l2 3h3v11H3z" strokeLinejoin="round" />
      <path d="M9.5 13.5a2.5 2.5 0 0 1 4.6-1.4M14.5 13a2.5 2.5 0 0 1-4.6 1.4" strokeLinecap="round" />
      <path d="M14 10.5h1.6V12M10 15.5H8.4V14" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}
