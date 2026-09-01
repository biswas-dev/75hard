import { useState } from 'react'
import { PhotoUpload } from '../components/PhotoUpload'
import { AuthImage } from '../components/AuthImage'
import { api } from '../lib/api'
import type { Photo } from '../lib/types'

interface Props {
  dayNumber: number
  onClose: () => void
  onSaved: () => void
}

interface DraftItem {
  name: string
  qty: string
  unit: string
  kcal: string
  protein_g: string
  carbs_g: string
  fat_g: string
}

const emptyItem: DraftItem = { name: '', qty: '1', unit: 'serving', kcal: '', protein_g: '', carbs_g: '', fat_g: '' }

/**
 * Bottom sheet for logging a meal: a photo, a name, and either a single
 * calorie figure or an itemised breakdown.
 *
 * When items are present the server sums them, so the totals shown here are a
 * preview of what it will store rather than a second source of truth.
 */
export function MealSheet({ dayNumber, onClose, onSaved }: Props) {
  const [name, setName] = useState('')
  const [slot, setSlot] = useState('lunch')
  const [photo, setPhoto] = useState<Photo | null>(null)
  const [itemised, setItemised] = useState(false)
  const [kcal, setKcal] = useState('')
  const [protein, setProtein] = useState('')
  const [carbs, setCarbs] = useState('')
  const [fat, setFat] = useState('')
  const [items, setItems] = useState<DraftItem[]>([{ ...emptyItem }])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const itemTotals = items.reduce(
    (acc, it) => ({
      kcal: acc.kcal + num(it.kcal),
      protein: acc.protein + num(it.protein_g),
      carbs: acc.carbs + num(it.carbs_g),
      fat: acc.fat + num(it.fat_g),
    }),
    { kcal: 0, protein: 0, carbs: 0, fat: 0 },
  )

  async function save() {
    setBusy(true)
    setError('')
    try {
      await api.createMeal({
        day_number: dayNumber,
        photo_id: photo?.id ?? null,
        name: name.trim(),
        slot,
        ...(itemised
          ? {
              items: items
                .filter((it) => it.name.trim() !== '')
                .map((it) => ({
                  name: it.name.trim(),
                  qty: num(it.qty) || 1,
                  unit: it.unit || 'serving',
                  kcal: num(it.kcal),
                  protein_g: num(it.protein_g),
                  carbs_g: num(it.carbs_g),
                  fat_g: num(it.fat_g),
                })),
            }
          : {
              kcal: num(kcal),
              protein_g: num(protein),
              carbs_g: num(carbs),
              fat_g: num(fat),
            }),
      })
      onSaved()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not save the meal')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-end justify-center">
      <button
        className="absolute inset-0 bg-black/70 backdrop-blur-sm"
        onClick={onClose}
        aria-label="Close"
      />
      <div className="animate-slide-up relative max-h-[90vh] w-full max-w-lg overflow-y-auto rounded-t-3xl border-t border-ink-800 bg-ink-900 p-5 pb-8">
        <div className="mx-auto mb-5 h-1 w-10 rounded-full bg-ink-700" />

        <h2 className="mb-4 text-lg font-semibold text-ink-100">Log a meal</h2>

        <div className="space-y-4">
          {photo ? (
            <div className="relative">
              <AuthImage
                src={photo.thumb_url}
                alt="Meal"
                className="h-40 w-full rounded-xl object-cover"
              />
              <button
                className="absolute right-2 top-2 rounded-full bg-black/60 px-2.5 py-1 text-sm text-white"
                onClick={() => setPhoto(null)}
              >
                Remove
              </button>
            </div>
          ) : (
            <PhotoUpload kind="food" dayNumber={dayNumber} label="Add a food photo" onUploaded={setPhoto} />
          )}

          <div>
            <label className="label">What was it?</label>
            <input
              className="field"
              placeholder="Chicken and rice"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </div>

          <div>
            <label className="label">Meal</label>
            <div className="grid grid-cols-4 gap-2">
              {['breakfast', 'lunch', 'dinner', 'snack'].map((s) => (
                <button
                  key={s}
                  type="button"
                  onClick={() => setSlot(s)}
                  className={`rounded-xl border py-2.5 text-sm capitalize transition ${
                    slot === s
                      ? 'border-flame-500 bg-flame-500/15 text-flame-400'
                      : 'border-ink-800 bg-ink-850 text-ink-400'
                  }`}
                >
                  {s}
                </button>
              ))}
            </div>
          </div>

          <div className="flex items-center justify-between rounded-xl bg-ink-850 px-4 py-3">
            <span className="text-sm text-ink-300">Break it into ingredients</span>
            <button
              type="button"
              role="switch"
              aria-checked={itemised}
              aria-label="Break it into ingredients"
              onClick={() => setItemised(!itemised)}
              className={`relative h-7 w-12 rounded-full transition-colors ${
                itemised ? 'bg-flame-500' : 'bg-ink-700'
              }`}
            >
              <span
                className={`absolute top-1 h-5 w-5 rounded-full bg-white transition-transform ${
                  itemised ? 'translate-x-6' : 'translate-x-1'
                }`}
              />
            </button>
          </div>

          {itemised ? (
            <div className="space-y-3">
              {items.map((item, i) => (
                <div key={i} className="rounded-xl border border-ink-800 bg-ink-850 p-3">
                  <div className="mb-2 flex gap-2">
                    <input
                      className="field flex-1 py-2"
                      placeholder="Ingredient"
                      value={item.name}
                      onChange={(e) =>
                        setItems(items.map((it, j) => (j === i ? { ...it, name: e.target.value } : it)))
                      }
                    />
                    {items.length > 1 && (
                      <button
                        className="px-2 text-ink-600 hover:text-red-400"
                        aria-label="Remove ingredient"
                        onClick={() => setItems(items.filter((_, j) => j !== i))}
                      >
                        ×
                      </button>
                    )}
                  </div>
                  <div className="grid grid-cols-4 gap-2">
                    {(['kcal', 'protein_g', 'carbs_g', 'fat_g'] as const).map((field) => (
                      <input
                        key={field}
                        type="number"
                        inputMode="decimal"
                        className="field py-2 text-center text-sm"
                        placeholder={field === 'kcal' ? 'kcal' : field.replace('_g', '')}
                        aria-label={field}
                        value={item[field]}
                        onChange={(e) =>
                          setItems(items.map((it, j) => (j === i ? { ...it, [field]: e.target.value } : it)))
                        }
                      />
                    ))}
                  </div>
                </div>
              ))}
              <button
                type="button"
                className="btn-ghost w-full py-2 text-sm"
                onClick={() => setItems([...items, { ...emptyItem }])}
              >
                + Add ingredient
              </button>
              <p className="text-center text-sm text-ink-500">
                Total{' '}
                <span className="font-mono text-ink-200">{Math.round(itemTotals.kcal)} kcal</span> ·{' '}
                {Math.round(itemTotals.protein)}p / {Math.round(itemTotals.carbs)}c /{' '}
                {Math.round(itemTotals.fat)}f
              </p>
            </div>
          ) : (
            <div className="grid grid-cols-4 gap-2">
              <Field label="kcal" value={kcal} onChange={setKcal} />
              <Field label="protein" value={protein} onChange={setProtein} />
              <Field label="carbs" value={carbs} onChange={setCarbs} />
              <Field label="fat" value={fat} onChange={setFat} />
            </div>
          )}

          {error && <p className="text-sm text-red-400">{error}</p>}

          <div className="flex gap-3 pt-1">
            <button className="btn-ghost flex-1" onClick={onClose}>
              Cancel
            </button>
            <button className="btn-primary flex-1" onClick={save} disabled={busy}>
              {busy ? 'Saving…' : 'Save meal'}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

function Field({
  label,
  value,
  onChange,
}: {
  label: string
  value: string
  onChange: (v: string) => void
}) {
  return (
    <div>
      <input
        type="number"
        inputMode="decimal"
        className="field py-2 text-center"
        placeholder="0"
        aria-label={label}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
      <p className="mt-1 text-center text-[11px] text-ink-600">{label}</p>
    </div>
  )
}

function num(v: string): number {
  const n = Number(v)
  return Number.isFinite(n) ? n : 0
}
