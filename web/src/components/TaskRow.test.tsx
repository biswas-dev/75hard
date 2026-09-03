import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { TaskRow } from './TaskRow'
import type { Day, Entry } from '../lib/types'

const day = {
  id: 1,
  program_id: 1,
  day_number: 1,
  date: '2026-09-03',
  status: 'pending',
  note: '',
  weight_kg: null,
  resting_hr: null,
  completed_at: null,
  is_today: true,
  tasks_done: 0,
  tasks_total: 6,
  entries: [],
  photos: [],
  meals: [],
  workouts: [],
  meditations: [],
  totals: {
    kcal: 0,
    protein_g: 0,
    carbs_g: 0,
    fat_g: 0,
    kcal_target: null,
    workout_minutes: 0,
    outdoor_minutes: 0,
    meditation_minutes: 0,
  },
} as unknown as Day

function entry(overrides: Partial<Entry>): Entry {
  return {
    task_id: 9,
    key: 'meditation',
    title: 'Meditate',
    detail: 'Optional.',
    icon: 'lotus',
    kind: 'check',
    unit: '',
    target_num: null,
    required: false,
    sort_order: 7,
    value: null,
    note: '',
    done: false,
    completed_at: null,
    tracker: 'meditation',
    color: '#7dd3fc',
    ...overrides,
  } as Entry
}

describe('TaskRow tracker panel', () => {
  it('opens the meditation panel from the chevron', () => {
    render(<TaskRow entry={entry({})} day={day} onToggle={vi.fn()} onChanged={vi.fn()} />)

    // Closed to begin with.
    expect(screen.queryByText(/Sit now/)).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /Show Meditate detail/i }))

    // The timer is the point of the panel, so its presence is the check.
    expect(screen.getByText(/Sit now/)).toBeInTheDocument()
  })

  it('opens the journal panel from the chevron', () => {
    render(
      <TaskRow
        entry={entry({ key: 'journal', title: 'Journal', tracker: 'journal', icon: 'pen' })}
        day={day}
        onToggle={vi.fn()}
        onChanged={vi.fn()}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: /Show Journal detail/i }))
    expect(screen.getByPlaceholderText(/Write it down/)).toBeInTheDocument()
  })

  it('shows an optional badge only on optional tasks', () => {
    const { rerender } = render(
      <TaskRow entry={entry({})} day={day} onToggle={vi.fn()} onChanged={vi.fn()} />,
    )
    expect(screen.getByText('optional')).toBeInTheDocument()

    rerender(
      <TaskRow
        entry={entry({ required: true, tracker: '' })}
        day={day}
        onToggle={vi.fn()}
        onChanged={vi.fn()}
      />,
    )
    expect(screen.queryByText('optional')).not.toBeInTheDocument()
  })
})
