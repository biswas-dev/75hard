import { describe, expect, it } from 'vitest'
import { lensFor } from './PhotoUpload'

/**
 * Which lens opens is a small thing that is very annoying to get wrong: the
 * wrong one means fumbling with a flip button while holding a plate, or while
 * standing in position for a timed shot.
 */
describe('camera lens defaults', () => {
  it('opens the front camera for a front progress photo', () => {
    expect(lensFor('progress', 'front')).toBe('user')
    expect(lensFor('progress', '')).toBe('user')
  })

  it('opens the rear camera for a back or side shot', () => {
    // You cannot see the screen to frame these, so they need the rear lens
    // and the timer.
    expect(lensFor('progress', 'back')).toBe('environment')
    expect(lensFor('progress', 'side')).toBe('environment')
  })

  it('always opens the rear camera for food', () => {
    // The meal is on the table, never in the selfie camera.
    for (const pose of ['', 'front', 'side', 'back'] as const) {
      expect(lensFor('food', pose)).toBe('environment')
      expect(lensFor('ingredients', pose)).toBe('environment')
    }
  })
})
