import { useCallback, useEffect, useRef, useState } from 'react'
import { AuthImage } from './AuthImage'
import { api } from '../lib/api'
import type { Photo } from '../lib/types'

/** How far a flick has to travel before it turns the page. */
export const SWIPE_PX = 60

/** Zoom bounds. Below 1 the image would float in the middle of nothing. */
const MIN_SCALE = 1
const MAX_SCALE = 5

/** What a double-tap zooms to, and what a wheel notch multiplies by. */
const DOUBLE_TAP_SCALE = 2.5
const WHEEL_STEP = 1.15

/** Two taps closer together than this are a double-tap. */
const DOUBLE_TAP_MS = 300

export interface Transform {
  scale: number
  x: number
  y: number
}

export const IDENTITY: Transform = { scale: 1, x: 0, y: 0 }

interface Props {
  photos: Photo[]
  index: number
  onIndex: (next: number) => void
  onClose: () => void
  /** Actions for the photo on screen: retag, delete, whatever the caller has. */
  footer?: (photo: Photo) => React.ReactNode
  /** A line under the counter — the day, the meal, the angle. */
  caption?: (photo: Photo) => string
}

/**
 * Full-screen photo viewer.
 *
 * Written by hand and carrying no dependency: pinch, pan, double-tap, wheel
 * zoom, swipe and keyboard all come out of the pointer events the browser
 * already sends. A gesture library would be several times the size of this
 * file for behaviour that is a few dozen lines of arithmetic.
 *
 * Pointer events rather than touch events because they are one API for a
 * finger, a mouse and a stylus — the alternative is writing every gesture
 * twice and having them disagree.
 */
export function Lightbox({ photos, index, onIndex, onClose, footer, caption }: Props) {
  const photo = photos[index]
  const [t, setT] = useState<Transform>(IDENTITY)
  // Horizontal offset while a swipe is in progress, so the photo follows the
  // finger instead of jumping when it is released.
  const [drag, setDrag] = useState(0)

  const frame = useRef<HTMLDivElement>(null)
  // Live pointers by id. Two means a pinch; one means a pan or a swipe.
  const pointers = useRef(new Map<number, { x: number; y: number }>())
  const start = useRef({ x: 0, y: 0, t: IDENTITY, dist: 0, moved: false })
  const lastTap = useRef(0)

  const zoomed = t.scale > 1.01

  const reset = useCallback(() => {
    setT(IDENTITY)
    setDrag(0)
  }, [])

  // A new photo always arrives unzoomed and centred.
  useEffect(() => {
    reset()
  }, [index, reset])

  const go = useCallback(
    (delta: number) => {
      const next = index + delta
      if (next < 0 || next >= photos.length) return
      onIndex(next)
    },
    [index, photos.length, onIndex],
  )

  // Keyboard, and the scroll lock that stops the page moving underneath.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      switch (e.key) {
        case 'Escape':
          onClose()
          break
        case 'ArrowLeft':
          go(-1)
          break
        case 'ArrowRight':
          go(1)
          break
        case '0':
          reset()
          break
        case '+':
        case '=':
          setT((p) => ({ ...p, scale: Math.min(MAX_SCALE, p.scale * WHEEL_STEP) }))
          break
        case '-':
          setT((p) => zoomOut(p))
          break
      }
    }
    window.addEventListener('keydown', onKey)

    const previous = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      window.removeEventListener('keydown', onKey)
      document.body.style.overflow = previous
    }
  }, [go, onClose, reset])

  // Warm the neighbours so a swipe lands on a picture rather than a spinner.
  // The blob is thrown away immediately; it is the HTTP cache we are filling.
  useEffect(() => {
    for (const n of [index - 1, index + 1]) {
      const neighbour = photos[n]
      if (!neighbour) continue
      api
        .photoObjectURL(neighbour.url)
        .then(URL.revokeObjectURL)
        .catch(() => {})
    }
  }, [index, photos])

  function onPointerDown(e: React.PointerEvent) {
    ;(e.target as Element).setPointerCapture?.(e.pointerId)
    pointers.current.set(e.pointerId, { x: e.clientX, y: e.clientY })

    if (pointers.current.size === 2) {
      start.current.dist = spread(pointers.current)
      start.current.t = t
      return
    }
    start.current = { x: e.clientX, y: e.clientY, t, dist: 0, moved: false }
  }

  function onPointerMove(e: React.PointerEvent) {
    if (!pointers.current.has(e.pointerId)) return
    pointers.current.set(e.pointerId, { x: e.clientX, y: e.clientY })

    // Two fingers: scale about the midpoint between them, so the picture grows
    // where the hands are rather than towards the middle of the screen.
    if (pointers.current.size === 2) {
      const dist = spread(pointers.current)
      if (start.current.dist > 0) {
        const scale = clamp(
          (start.current.t.scale * dist) / start.current.dist,
          MIN_SCALE,
          MAX_SCALE,
        )
        setT((p) => ({ ...p, scale }))
      }
      return
    }

    const dx = e.clientX - start.current.x
    const dy = e.clientY - start.current.y
    if (Math.abs(dx) > 4 || Math.abs(dy) > 4) start.current.moved = true

    if (zoomed) {
      setT({ ...start.current.t, x: start.current.t.x + dx, y: start.current.t.y + dy })
      return
    }
    // Not zoomed: a horizontal drag is turning the page.
    setDrag(dx)
  }

  function onPointerUp(e: React.PointerEvent) {
    pointers.current.delete(e.pointerId)

    // Coming out of a pinch: settle back if it ended below its natural size.
    if (pointers.current.size === 1) {
      const [only] = [...pointers.current.values()]
      start.current = { x: only.x, y: only.y, t, dist: 0, moved: true }
      return
    }
    if (pointers.current.size > 0) return

    if (zoomed) {
      setT((p) => clampPan(p, frame.current))
      return
    }

    const moved = start.current.moved
    const dx = drag
    setDrag(0)

    if (Math.abs(dx) > SWIPE_PX) {
      go(dx < 0 ? 1 : -1)
      return
    }

    // A tap that went nowhere. Two in quick succession zoom in.
    if (!moved) {
      const now = Date.now()
      if (now - lastTap.current < DOUBLE_TAP_MS) {
        lastTap.current = 0
        setT({ scale: DOUBLE_TAP_SCALE, x: 0, y: 0 })
        return
      }
      lastTap.current = now
    }
  }

  function onWheel(e: React.WheelEvent) {
    // Trackpad pinch arrives as a wheel event with ctrlKey set.
    const factor = e.deltaY < 0 ? WHEEL_STEP : 1 / WHEEL_STEP
    setT((p) => {
      const scale = clamp(p.scale * factor, MIN_SCALE, MAX_SCALE)
      return scale <= 1.01 ? IDENTITY : clampPan({ ...p, scale }, frame.current)
    })
  }

  if (!photo) return null

  return (
    <div className="fixed inset-0 z-[70] flex flex-col bg-black/95 backdrop-blur-sm">
      <header className="flex items-center justify-between gap-3 px-4 py-3">
        <div className="min-w-0">
          {photos.length > 1 && (
            <p className="text-sm text-ink-300">
              {index + 1} of {photos.length}
            </p>
          )}
          {caption && <p className="truncate text-xs text-ink-500">{caption(photo)}</p>}
        </div>
        <div className="flex shrink-0 items-center gap-1">
          {zoomed && (
            <button
              className="rounded-lg px-2 py-1 text-xs text-ink-400 hover:text-ink-200"
              onClick={reset}
            >
              Reset
            </button>
          )}
          <button
            className="p-2 text-2xl leading-none text-ink-300 hover:text-white"
            onClick={onClose}
            aria-label="Close"
          >
            ×
          </button>
        </div>
      </header>

      <div
        ref={frame}
        className="relative flex flex-1 select-none items-center justify-center overflow-hidden"
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onPointerCancel={onPointerUp}
        onWheel={onWheel}
        onDoubleClick={() => setT((p) => (p.scale > 1.01 ? IDENTITY : { scale: DOUBLE_TAP_SCALE, x: 0, y: 0 }))}
        style={{ touchAction: 'none', cursor: zoomed ? 'grab' : 'auto' }}
      >
        <div
          className="max-h-full max-w-full"
          style={{
            transform: `translate3d(${t.x + drag}px, ${t.y}px, 0) scale(${t.scale})`,
            // Animate only when nothing is under the finger, or dragging lags.
            transition: pointers.current.size ? 'none' : 'transform 180ms ease-out',
          }}
        >
          <AuthImage
            src={photo.url}
            alt={photo.caption || 'Photo'}
            className="max-h-[80vh] max-w-full object-contain"
          />
        </div>

        {/* Arrows for a mouse. Touch has the swipe, and hiding them there
            keeps a finger from covering the picture with controls. */}
        {index > 0 && (
          <button
            onClick={() => go(-1)}
            aria-label="Previous photo"
            className="absolute left-2 hidden h-10 w-10 items-center justify-center rounded-full bg-black/50 text-xl text-white/80 hover:bg-black/70 md:flex"
          >
            ‹
          </button>
        )}
        {index < photos.length - 1 && (
          <button
            onClick={() => go(1)}
            aria-label="Next photo"
            className="absolute right-2 hidden h-10 w-10 items-center justify-center rounded-full bg-black/50 text-xl text-white/80 hover:bg-black/70 md:flex"
          >
            ›
          </button>
        )}
      </div>

      {footer && <div className="space-y-3 p-4">{footer(photo)}</div>}
    </div>
  )
}

export function clamp(v: number, lo: number, hi: number): number {
  return Math.min(hi, Math.max(lo, v))
}

/** Distance between the two live pointers. */
export function spread(pointers: Map<number, { x: number; y: number }>): number {
  const [a, b] = [...pointers.values()]
  if (!a || !b) return 0
  return Math.hypot(a.x - b.x, a.y - b.y)
}

export function zoomOut(p: Transform): Transform {
  const scale = clamp(p.scale / WHEEL_STEP, MIN_SCALE, MAX_SCALE)
  return scale <= 1.01 ? IDENTITY : { ...p, scale }
}

/**
 * Keep a zoomed photo's edges inside the frame.
 *
 * Without this a pan can throw the picture off screen entirely, and the only
 * way back is to guess which direction it went.
 */
export function clampPan(p: Transform, frame: { clientWidth: number; clientHeight: number } | null): Transform {
  if (!frame || p.scale <= 1) return { ...p, x: 0, y: 0 }
  const limitX = (frame.clientWidth * (p.scale - 1)) / 2
  const limitY = (frame.clientHeight * (p.scale - 1)) / 2
  return { ...p, x: clamp(p.x, -limitX, limitX), y: clamp(p.y, -limitY, limitY) }
}
