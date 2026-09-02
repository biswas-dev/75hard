import type { User } from './types'

export type WeightUnit = 'kg' | 'lb'

/** One kilogram in pounds. */
const LB_PER_KG = 2.2046226218

/**
 * Weight is stored and sent in kilograms everywhere; the unit is a display
 * preference only.
 *
 * Converting at the edges rather than storing per-user units keeps every
 * chart, average and comparison on one scale — otherwise each of them would
 * have to know which rows were recorded in which unit, and a person who
 * switched halfway through would corrupt their own history.
 */
export function toDisplay(kg: number, unit: WeightUnit): number {
  return unit === 'lb' ? kg * LB_PER_KG : kg
}

/** Converts a number the person typed back to kilograms for storage. */
export function toKg(value: number, unit: WeightUnit): number {
  return unit === 'lb' ? value / LB_PER_KG : value
}

/** Rounds for display: one decimal in kg, none in lb where it is noise. */
export function roundWeight(value: number, unit: WeightUnit): number {
  return unit === 'lb' ? Math.round(value * 10) / 10 : Math.round(value * 10) / 10
}

/** A weight in kilograms, rendered in the reader's preferred unit. */
export function formatWeight(kg: number, unit: WeightUnit): string {
  return `${roundWeight(toDisplay(kg, unit), unit)} ${unit}`
}

/** A difference in kilograms, rendered with an explicit sign. */
export function formatWeightDelta(deltaKg: number, unit: WeightUnit): string {
  const value = roundWeight(toDisplay(deltaKg, unit), unit)
  return `${value > 0 ? '+' : ''}${value} ${unit}`
}

/** The unit to use for a user, defaulting to kilograms. */
export function unitFor(user: User | null | undefined): WeightUnit {
  return user?.weight_unit === 'lb' ? 'lb' : 'kg'
}

/** Sensible input bounds in the chosen unit, so the field guards typos. */
export function weightBounds(unit: WeightUnit) {
  return unit === 'lb' ? { min: 44, max: 880, step: 0.2 } : { min: 20, max: 400, step: 0.1 }
}
