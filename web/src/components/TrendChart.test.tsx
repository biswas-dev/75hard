import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { TrendChart } from './TrendChart'

describe('TrendChart', () => {
  const falling = [
    { x: 1, y: 82.5 },
    { x: 5, y: 81.2 },
    { x: 9, y: 80.4 },
  ]

  it('renders nothing without data', () => {
    const { container } = render(
      <TrendChart points={[]} color="#ff6b35" unit="kg" label="Weight" />,
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('labels the chart and shows the endpoints', () => {
    render(<TrendChart points={falling} color="#ff6b35" unit="kg" label="Weight" />)

    expect(screen.getByRole('img', { name: 'Weight' })).toBeInTheDocument()
    expect(screen.getByText(/82.5 kg · day 1/)).toBeInTheDocument()
    expect(screen.getByText(/80.4 kg · day 9/)).toBeInTheDocument()
  })

  it('shows a fall as an improvement when lower is better', () => {
    render(<TrendChart points={falling} color="#ff6b35" unit="kg" label="Weight" />)

    // -2.1 over the series, and it must read as progress rather than a warning.
    const delta = screen.getByText(/-2.1 kg/)
    expect(delta).toBeInTheDocument()
    expect(delta.className).toContain('moss')
  })

  it('shows a rise as a regression when lower is better', () => {
    // A resting pulse going the wrong way must be visible, not hidden.
    render(
      <TrendChart
        points={[
          { x: 1, y: 52 },
          { x: 6, y: 57 },
        ]}
        color="#4a9eff"
        unit="bpm"
        label="Resting pulse"
      />,
    )
    const delta = screen.getByText(/\+5 bpm/)
    expect(delta.className).toContain('flame')
  })

  it('does not divide by zero on a flat series', () => {
    // Every reading identical gives a zero range; the chart must still render.
    expect(() =>
      render(
        <TrendChart
          points={[
            { x: 1, y: 80 },
            { x: 2, y: 80 },
          ]}
          color="#ff6b35"
          unit="kg"
          label="Flat"
        />,
      ),
    ).not.toThrow()
    expect(screen.getByRole('img', { name: 'Flat' })).toBeInTheDocument()
  })

  it('renders a single reading without a delta', () => {
    // One point is a start, not a trend: the reading shows, the change does not.
    render(<TrendChart points={[{ x: 3, y: 75 }]} color="#ff6b35" unit="kg" label="One" />)

    expect(screen.getByRole('img', { name: 'One' })).toBeInTheDocument()
    // Both endpoints are the same reading.
    expect(screen.getAllByText(/75 kg · day 3/)).toHaveLength(2)
    // No delta, because there is nothing to compare against.
    expect(screen.queryByText(/^[+-]/)).toBeNull()
  })
})
