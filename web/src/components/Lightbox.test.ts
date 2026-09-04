import { describe, expect, it } from 'vitest'
import { IDENTITY, clamp, clampPan, spread, zoomOut } from './Lightbox'

/**
 * The viewer's gestures are arithmetic, and arithmetic is worth pinning down:
 * a sign wrong in the pan clamp throws the photo off screen with no way back
 * except closing and reopening.
 */
describe('clamp', () => {
  it('holds a value inside its bounds', () => {
    expect(clamp(5, 1, 3)).toBe(3)
    expect(clamp(-5, 1, 3)).toBe(1)
    expect(clamp(2, 1, 3)).toBe(2)
  })
})

describe('pinch distance', () => {
  it('measures between two pointers', () => {
    const p = new Map([
      [1, { x: 0, y: 0 }],
      [2, { x: 3, y: 4 }],
    ])
    expect(spread(p)).toBe(5)
  })

  it('is zero with fewer than two fingers down', () => {
    expect(spread(new Map([[1, { x: 0, y: 0 }]]))).toBe(0)
    expect(spread(new Map())).toBe(0)
  })
})

describe('pan clamping', () => {
  const frame = { clientWidth: 400, clientHeight: 800 }

  it('keeps a zoomed photo from leaving the frame', () => {
    // At 2x the picture is one frame wider than the window, so it may travel
    // half a frame in either direction and no further.
    const out = clampPan({ scale: 2, x: 9999, y: -9999 }, frame)
    expect(out.x).toBe(200)
    expect(out.y).toBe(-400)
  })

  it('leaves a pan that is already inside alone', () => {
    const out = clampPan({ scale: 2, x: 50, y: -100 }, frame)
    expect(out).toEqual({ scale: 2, x: 50, y: -100 })
  })

  it('recentres when not zoomed', () => {
    // An unzoomed photo fits, so any offset is a leftover from a gesture.
    expect(clampPan({ scale: 1, x: 120, y: 80 }, frame)).toEqual({ scale: 1, x: 0, y: 0 })
  })

  it('recentres when there is no frame to measure', () => {
    expect(clampPan({ scale: 3, x: 120, y: 80 }, null)).toEqual({ scale: 3, x: 0, y: 0 })
  })
})

describe('zooming out', () => {
  it('snaps back to centre once it reaches natural size', () => {
    // Otherwise a photo that fits sits off-centre with nothing to drag it back.
    expect(zoomOut({ scale: 1.1, x: 40, y: 40 })).toEqual(IDENTITY)
  })

  it('stays zoomed and keeps its offset above natural size', () => {
    const out = zoomOut({ scale: 4, x: 40, y: 40 })
    expect(out.scale).toBeLessThan(4)
    expect(out.scale).toBeGreaterThan(1)
    expect(out.x).toBe(40)
  })

  it('never goes below natural size', () => {
    let t = { scale: 2, x: 0, y: 0 }
    for (let i = 0; i < 50; i++) t = zoomOut(t)
    expect(t.scale).toBe(1)
  })
})
