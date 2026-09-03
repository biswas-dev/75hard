import { useEffect, useState } from 'react'
import { PhotoUpload } from '../components/PhotoUpload'
import { AuthImage } from '../components/AuthImage'
import { api } from '../lib/api'
import type { FoodEstimate, Meal, Photo } from '../lib/types'

interface Props {
  dayNumber: number
  onClose: () => void
  onSaved: () => void
  /**
   * Refresh the day without closing the sheet. Used when an estimate is
   * written straight to an existing meal, so the day's totals keep up while
   * the sheet stays open for review.
   */
  onChanged?: () => void
  /**
   * An existing meal to edit. Absent means a new one.
   *
   * Without this a meal was write-once: a wrong estimate, or one that arrived
   * empty because the model timed out, could only be deleted and retyped.
   */
  meal?: Meal | null
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
export function MealSheet({ dayNumber, onClose, onSaved, onChanged, meal }: Props) {
  const editing = meal ?? null
  const [name, setName] = useState(editing?.name ?? '')
  // Sent to the model with the photo, and kept with the meal.
  //
  // It was discarded after the estimate, which meant re-opening a meal to
  // re-run one started from a blank field — and the description of what the
  // photograph cannot show is the most valuable part of the request, the part
  // that took thought to write. It is stored as the meal's note.
  const [detail, setDetail] = useState(editing?.notes ?? '')
  const [slot, setSlot] = useState<Meal['slot']>(editing?.slot ?? 'lunch')
  // When editing, the meal's existing photo has to be present or the
  // "estimate from the photo" button never appears — which is exactly the
  // case somebody is in when a meal was saved with no calories on it.
  const [photo, setPhoto] = useState<Photo | null>(
    editing && editing.photo_id
      ? ({
          id: editing.photo_id,
          kind: 'food',
          pose: '',
          day_id: editing.day_id,
          day_number: null,
          caption: '',
          width: 0,
          height: 0,
          bytes: 0,
          taken_at: editing.eaten_at,
          url: editing.photo_url ?? `/api/photos/${editing.photo_id}/file`,
          thumb_url: editing.photo_url ?? `/api/photos/${editing.photo_id}/file?size=thumb`,
        } as Photo)
      : null,
  )
  const [itemised, setItemised] = useState(false)
  const [kcal, setKcal] = useState(editing ? String(editing.kcal || '') : '')
  const [protein, setProtein] = useState(editing ? String(editing.protein_g || '') : '')
  const [carbs, setCarbs] = useState(editing ? String(editing.carbs_g || '') : '')
  const [fat, setFat] = useState(editing ? String(editing.fat_g || '') : '')
  const [items, setItems] = useState<DraftItem[]>([{ ...emptyItem }])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [aiAvailable, setAiAvailable] = useState(false)
  const [analyzing, setAnalyzing] = useState(false)
  const [aiNote, setAiNote] = useState('')

  useEffect(() => {
    // Only offer the button if the server actually has a provider configured.
    api
      .aiStatus()
      .then((s) => setAiAvailable(s.enabled))
      .catch(() => setAiAvailable(false))
  }, [])

  /**
   * Ask the model to read the photo, then drop the result straight into the
   * form. It is filled in as an editable draft rather than saved: an estimate
   * should be reviewed before it becomes a logged meal.
   */
  async function analyze() {
    if (!photo) return
    setAnalyzing(true)
    setError('')
    setAiNote('')
    try {
      // Name and detail together: a photograph cannot show what is under the
      // sauce, how much oil went in, or that the milk was whole — and those
      // are exactly what the calorie figure turns on.
      const hint = [name.trim(), detail.trim()].filter(Boolean).join('. ')
      const { estimate, cached } = await api.analyzeFood(photo.id, hint)
      applyEstimate(estimate)
      const prefix = cached ? 'From a previous estimate. ' : ''

      // For a meal that already exists, write the numbers immediately.
      //
      // They used to live only in this form until Save was pressed, so closing
      // the sheet — or letting the phone reclaim the tab — silently threw away
      // an estimate that had just cost a model call and a wait. Nothing is
      // lost by storing it: the fields below stay editable and saving again
      // overwrites it. A new meal still waits for Save, because there is no
      // row yet and an unreviewed estimate should not create one.
      if (editing) {
        try {
          await api.updateMeal(editing.id, estimateChanges(estimate))
          onChanged?.()
          setAiNote(prefix + 'Saved. ' + (estimate.notes || 'Adjust anything that looks wrong.'))
          return
        } catch {
          setAiNote(prefix + 'Could not save this automatically — press Save to keep it.')
          return
        }
      }
      setAiNote(prefix + (estimate.notes || 'Check the numbers before saving.'))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not analyse that photo')
    } finally {
      setAnalyzing(false)
    }
  }

  /**
   * The update body for an estimate, derived from the estimate itself.
   *
   * It deliberately does not read the form state that applyEstimate has just
   * set: those setters are asynchronous, so reading them here would save the
   * values from before the estimate arrived.
   */
  function estimateChanges(est: FoodEstimate) {
    const base = {
      photo_id: photo?.id ?? null,
      name: name.trim() || est.name || '',
      slot,
      notes: detail.trim(),
    }
    if (est.items.length > 0) {
      return {
        ...base,
        items: est.items.map((it) => ({
          name: it.name,
          qty: it.qty || 1,
          unit: it.unit || 'serving',
          kcal: Math.round(it.kcal),
          protein_g: Math.round(it.protein_g),
          carbs_g: Math.round(it.carbs_g),
          fat_g: Math.round(it.fat_g),
        })),
      }
    }
    return {
      ...base,
      kcal: Math.round(est.kcal),
      protein_g: Math.round(est.protein_g),
      carbs_g: Math.round(est.carbs_g),
      fat_g: Math.round(est.fat_g),
    }
  }

  function applyEstimate(est: FoodEstimate) {
    if (est.name && !name.trim()) setName(est.name)
    if (est.items.length > 0) {
      setItemised(true)
      setItems(
        est.items.map((it) => ({
          name: it.name,
          qty: String(it.qty),
          unit: it.unit,
          kcal: String(Math.round(it.kcal)),
          protein_g: String(Math.round(it.protein_g)),
          carbs_g: String(Math.round(it.carbs_g)),
          fat_g: String(Math.round(it.fat_g)),
        })),
      )
    } else {
      setKcal(String(Math.round(est.kcal)))
      setProtein(String(Math.round(est.protein_g)))
      setCarbs(String(Math.round(est.carbs_g)))
      setFat(String(Math.round(est.fat_g)))
    }
  }

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
      const body = {
        day_number: dayNumber,
        photo_id: photo?.id ?? null,
        name: name.trim(),
        slot,
        // Kept so a later re-run starts from what was already described.
        notes: detail.trim(),
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
      }

      if (editing) {
        const { day_number: _unused, ...changes } = body
        await api.updateMeal(editing.id, changes)
      } else {
        await api.createMeal(body)
      }
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

          {photo && aiAvailable && (
            <div>
              <label className="label" htmlFor="meal-detail">
                Anything the photo cannot show
              </label>
              <input
                id="meal-detail"
                className="field"
                placeholder="e.g. cooked in butter, whole milk, large portion"
                value={detail}
                onChange={(e) => setDetail(e.target.value)}
              />
              <p className="mb-2 mt-1 text-xs text-ink-600">
                Saved with the meal, so re-running the estimate later starts from what you
                already wrote. Naming the items is what stops the model guessing at portions
                it cannot see.
              </p>
              <button
                type="button"
                className="btn-ghost w-full border-flame-500/30 text-flame-400"
                onClick={analyze}
                disabled={analyzing}
              >
                {analyzing ? <Spinner /> : <SparkIcon />}
                {analyzing ? 'Reading the photo…' : 'Estimate calories from the photo'}
              </button>
              {aiNote && <p className="mt-2 text-xs text-ink-500">{aiNote}</p>}
            </div>
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
                  onClick={() => setSlot(s as Meal['slot'])}
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

function SparkIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8">
      <path d="M12 3v4M12 17v4M3 12h4M17 12h4M5.6 5.6l2.8 2.8M15.6 15.6l2.8 2.8M18.4 5.6l-2.8 2.8M8.4 15.6l-2.8 2.8" strokeLinecap="round" />
    </svg>
  )
}

function Spinner() {
  return (
    <svg className="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none">
      <circle cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="3" className="opacity-25" />
      <path d="M12 2a10 10 0 0 1 10 10" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
    </svg>
  )
}

function num(v: string): number {
  const n = Number(v)
  return Number.isFinite(n) ? n : 0
}
