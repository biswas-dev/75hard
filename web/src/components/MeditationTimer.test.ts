import { describe, expect, it } from 'vitest'
import { formatClock, loggedMinutes } from './MeditationTimer'

describe('meditation timer', () => {
  it('formats the clock', () => {
    expect(formatClock(0)).toBe('0:00')
    expect(formatClock(9)).toBe('0:09')
    expect(formatClock(60)).toBe('1:00')
    expect(formatClock(600)).toBe('10:00')
    expect(formatClock(1259)).toBe('20:59')
  })

  it('rounds a sitting rather than truncating it', () => {
    // Nine minutes forty is a ten minute sitting to anyone who just sat it.
    // Flooring would quietly shave time off every single session.
    expect(loggedMinutes(580)).toBe(10)
    expect(loggedMinutes(600)).toBe(10)
    expect(loggedMinutes(620)).toBe(10)
    // And it rounds down when it should.
    expect(loggedMinutes(500)).toBe(8)
  })

  it('never records a sitting as zero', () => {
    // Somebody who sat for forty seconds did sit; recording nothing is worse
    // than recording a minute.
    expect(loggedMinutes(1)).toBe(1)
    expect(loggedMinutes(20)).toBe(1)
    expect(loggedMinutes(40)).toBe(1)
  })

  it('handles a long sitting', () => {
    expect(loggedMinutes(3600)).toBe(60)
    expect(formatClock(3600)).toBe('60:00')
  })
})
