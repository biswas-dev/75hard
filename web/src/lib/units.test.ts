import { describe, expect, it } from 'vitest'
import {
  formatWeight,
  formatWeightDelta,
  toDisplay,
  toKg,
  unitFor,
  weightBounds,
} from './units'
import type { User } from './types'

describe('weight units', () => {
  it('leaves kilograms untouched', () => {
    expect(toDisplay(80, 'kg')).toBe(80)
    expect(toKg(80, 'kg')).toBe(80)
  })

  it('converts kilograms to pounds', () => {
    expect(toDisplay(100, 'lb')).toBeCloseTo(220.46, 1)
    expect(toKg(220.46, 'lb')).toBeCloseTo(100, 2)
  })

  it('round-trips without drift', () => {
    // Someone switching units and back must not see their weight move. This is
    // the property that lets the toggle be a display preference rather than a
    // migration of their history.
    for (const kg of [55.4, 80, 92.75, 120.1, 47.3]) {
      expect(toKg(toDisplay(kg, 'lb'), 'lb')).toBeCloseTo(kg, 6)
      expect(toKg(toDisplay(kg, 'kg'), 'kg')).toBeCloseTo(kg, 6)
    }
  })

  it('formats with the unit attached', () => {
    expect(formatWeight(80, 'kg')).toBe('80 kg')
    expect(formatWeight(100, 'lb')).toBe('220.5 lb')
  })

  it('signs a delta explicitly', () => {
    // Losing weight is the common case and has to read as negative, or the
    // chart says the opposite of what happened.
    expect(formatWeightDelta(-2.1, 'kg')).toBe('-2.1 kg')
    expect(formatWeightDelta(1.5, 'kg')).toBe('+1.5 kg')
    expect(formatWeightDelta(0, 'kg')).toBe('0 kg')
    expect(formatWeightDelta(-1, 'lb')).toBe('-2.2 lb')
  })

  it('defaults to kilograms for anyone without a preference', () => {
    expect(unitFor(null)).toBe('kg')
    expect(unitFor(undefined)).toBe('kg')
    expect(unitFor({ weight_unit: 'lb' } as User)).toBe('lb')
    expect(unitFor({ weight_unit: 'kg' } as User)).toBe('kg')
    // An unrecognised stored value must not produce a broken unit.
    expect(unitFor({ weight_unit: 'stone' } as unknown as User)).toBe('kg')
  })

  it('bounds the input for the chosen unit', () => {
    const kg = weightBounds('kg')
    const lb = weightBounds('lb')
    // The pound bounds must cover the same real range as the kilogram ones,
    // or a legitimate weight is rejected by the field.
    expect(toKg(lb.min, 'lb')).toBeLessThanOrEqual(kg.min + 1)
    expect(toKg(lb.max, 'lb')).toBeGreaterThanOrEqual(kg.max - 1)
  })
})
