interface Point {
  x: number
  y: number
}

interface Props {
  points: Point[]
  color: string
  /** Unit shown beside the first and last readings. */
  unit: string
  label: string
  /**
   * Lower is better for everything charted here — weight, resting pulse and
   * training heart rate — so a fall is drawn as progress.
   */
  lowerIsBetter?: boolean
  height?: number
}

/**
 * A small line chart for one measurement over the program.
 *
 * The y-axis is scaled to the data rather than to zero: a weight series spanning
 * 80-83kg plotted from zero is a flat line, which hides exactly the change the
 * chart exists to show.
 */
export function TrendChart({ points, color, unit, label, lowerIsBetter = true, height = 120 }: Props) {
  if (points.length === 0) return null

  const w = 320
  const h = height
  const pad = 10

  const ys = points.map((p) => p.y)
  const min = Math.min(...ys)
  const max = Math.max(...ys)
  // A flat series would divide by zero; centring it is more honest than
  // stretching a single value across the whole axis.
  const range = max - min || 1

  const xs = points.map((p) => p.x)
  const xMin = Math.min(...xs)
  const xRange = Math.max(...xs) - xMin || 1

  const coords = points.map((p) => ({
    x: pad + ((p.x - xMin) / xRange) * (w - pad * 2),
    y: pad + (1 - (p.y - min) / range) * (h - pad * 2),
  }))

  const line = coords
    .map((c, i) => `${i === 0 ? 'M' : 'L'}${c.x.toFixed(1)},${c.y.toFixed(1)}`)
    .join(' ')
  // Closed under the line, for the soft fill.
  const area = `${line} L${coords[coords.length - 1].x.toFixed(1)},${h - pad} L${coords[0].x.toFixed(1)},${h - pad} Z`

  const first = points[0]
  const last = points[points.length - 1]
  const delta = last.y - first.y
  const improved = lowerIsBetter ? delta <= 0 : delta >= 0
  const gradientId = `grad-${label.replace(/\W/g, '')}`

  return (
    <div>
      <div className="mb-2 flex items-baseline justify-between">
        <h3 className="text-xs font-medium uppercase tracking-wide text-ink-500">{label}</h3>
        {points.length > 1 && (
          <span className={`font-mono text-xs ${improved ? 'text-moss-400' : 'text-flame-400'}`}>
            {delta > 0 ? '+' : ''}
            {round(delta)} {unit}
          </span>
        )}
      </div>

      <svg viewBox={`0 0 ${w} ${h}`} className="w-full" style={{ height }} role="img" aria-label={label}>
        <defs>
          <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={color} stopOpacity="0.28" />
            <stop offset="100%" stopColor={color} stopOpacity="0" />
          </linearGradient>
        </defs>
        {points.length > 1 && <path d={area} fill={`url(#${gradientId})`} />}
        <path
          d={line}
          fill="none"
          stroke={color}
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        {/* Endpoints only when the series is short enough for dots to read. */}
        {coords.length <= 40 &&
          coords.map((c, i) => <circle key={i} cx={c.x} cy={c.y} r="2.5" fill={color} />)}
      </svg>

      <div className="mt-1 flex justify-between font-mono text-[11px] text-ink-600">
        <span>
          {round(first.y)} {unit} · day {first.x}
        </span>
        <span>
          {round(last.y)} {unit} · day {last.x}
        </span>
      </div>
    </div>
  )
}

function round(n: number) {
  return Math.round(n * 10) / 10
}
