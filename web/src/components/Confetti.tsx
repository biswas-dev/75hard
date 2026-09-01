import { useEffect, useRef } from 'react'

interface Particle {
  x: number
  y: number
  vx: number
  vy: number
  size: number
  rotation: number
  spin: number
  color: string
}

const COLORS = ['#ff6b35', '#37d67a', '#ffd166', '#5ee094', '#ff8659']

/**
 * A one-shot confetti burst for the moment the last required task of the day
 * lands. Hand-rolled on a canvas — a library for six seconds of particles a
 * day would be more bytes than the rest of the screen.
 */
export function Confetti({ onDone }: { onDone?: () => void }) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const doneRef = useRef(onDone)
  doneRef.current = onDone

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return

    // Respect a reduced-motion preference: finish immediately, draw nothing.
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
      doneRef.current?.()
      return
    }

    const ctx = canvas.getContext('2d')
    if (!ctx) return

    const dpr = Math.min(window.devicePixelRatio || 1, 2)
    const width = canvas.clientWidth
    const height = canvas.clientHeight
    canvas.width = width * dpr
    canvas.height = height * dpr
    ctx.scale(dpr, dpr)

    const particles: Particle[] = Array.from({ length: 90 }, () => {
      const angle = -Math.PI / 2 + (Math.random() - 0.5) * 1.6
      const speed = 6 + Math.random() * 9
      return {
        x: width / 2,
        y: height * 0.42,
        vx: Math.cos(angle) * speed * (0.6 + Math.random() * 0.8),
        vy: Math.sin(angle) * speed,
        size: 5 + Math.random() * 6,
        rotation: Math.random() * Math.PI,
        spin: (Math.random() - 0.5) * 0.3,
        color: COLORS[Math.floor(Math.random() * COLORS.length)],
      }
    })

    let frame = 0
    let raf = 0

    const tick = () => {
      frame++
      ctx.clearRect(0, 0, width, height)

      for (const p of particles) {
        p.vy += 0.32 // gravity
        p.vx *= 0.99 // drag
        p.x += p.vx
        p.y += p.vy
        p.rotation += p.spin

        ctx.save()
        ctx.translate(p.x, p.y)
        ctx.rotate(p.rotation)
        ctx.globalAlpha = Math.max(0, 1 - frame / 110)
        ctx.fillStyle = p.color
        ctx.fillRect(-p.size / 2, -p.size / 4, p.size, p.size / 2)
        ctx.restore()
      }

      if (frame < 110) {
        raf = requestAnimationFrame(tick)
      } else {
        ctx.clearRect(0, 0, width, height)
        doneRef.current?.()
      }
    }
    raf = requestAnimationFrame(tick)

    return () => cancelAnimationFrame(raf)
  }, [])

  return (
    <canvas
      ref={canvasRef}
      className="pointer-events-none fixed inset-0 z-50 h-full w-full"
      aria-hidden
    />
  )
}
