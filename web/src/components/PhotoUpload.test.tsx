import { describe, expect, it } from 'vitest'
import { lensFor, timerFor } from './PhotoUpload'

/**
 * Which lens opens is a small thing that is very annoying to get wrong: the
 * wrong one means fumbling with a flip button while holding a plate, or while
 * standing in position for a timed shot.
 */
describe('camera lens defaults', () => {
  it('opens the front camera for every progress photo', () => {
    // Side and back shots included. The rear lens takes the better picture,
    // but you cannot see what it is pointing at, so framing one alone means
    // guessing and re-taking.
    for (const pose of ['front', 'side', 'back', ''] as const) {
      expect(lensFor('progress', pose)).toBe('user')
    }
  })

  it('always opens the rear camera for food', () => {
    // The meal is on the table, never in the selfie camera.
    for (const pose of ['', 'front', 'side', 'back'] as const) {
      expect(lensFor('food', pose)).toBe('environment')
      expect(lensFor('ingredients', pose)).toBe('environment')
    }
  })

  it('starts the timer at three seconds for a progress photo', () => {
    // Taken of yourself from across the room; a meal is shot with the phone
    // in your hand and wants no delay.
    expect(timerFor('progress')).toBe(3)
    expect(timerFor('food')).toBe(0)
    expect(timerFor('ingredients')).toBe(0)
  })
})
