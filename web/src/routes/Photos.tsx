import { useEffect, useState } from 'react'
import { AuthImage } from '../components/AuthImage'
import { api } from '../lib/api'
import type { Photo } from '../lib/types'

const FILTERS = [
  { key: '', label: 'All' },
  { key: 'progress', label: 'Progress' },
  { key: 'food', label: 'Food' },
  { key: 'ingredients', label: 'Ingredients' },
] as const

export function Photos() {
  const [photos, setPhotos] = useState<Photo[]>([])
  const [filter, setFilter] = useState('')
  const [loading, setLoading] = useState(true)
  const [viewing, setViewing] = useState<Photo | null>(null)

  useEffect(() => {
    setLoading(true)
    api
      .listPhotos(filter || undefined)
      .then(setPhotos)
      .finally(() => setLoading(false))
  }, [filter])

  async function remove(photo: Photo) {
    await api.deletePhoto(photo.id)
    setPhotos((prev) => prev.filter((p) => p.id !== photo.id))
    setViewing(null)
  }

  return (
    <div className="space-y-5">
      <header className="animate-slide-up">
        <h1 className="text-2xl font-semibold text-ink-100">Photos</h1>
        <p className="mt-1 text-sm text-ink-500">
          {photos.length} image{photos.length === 1 ? '' : 's'}
        </p>
      </header>

      <div className="flex gap-2 overflow-x-auto pb-1">
        {FILTERS.map((f) => (
          <button
            key={f.key}
            onClick={() => setFilter(f.key)}
            className={`shrink-0 rounded-full border px-4 py-2 text-sm transition ${
              filter === f.key
                ? 'border-flame-500 bg-flame-500/15 text-flame-400'
                : 'border-ink-800 bg-ink-900 text-ink-400'
            }`}
          >
            {f.label}
          </button>
        ))}
      </div>

      {loading ? (
        <div className="grid grid-cols-3 gap-2">
          {Array.from({ length: 9 }).map((_, i) => (
            <div key={i} className="aspect-square animate-pulse rounded-xl bg-ink-900" />
          ))}
        </div>
      ) : photos.length === 0 ? (
        <div className="card p-10 text-center text-ink-500">
          <p>No photos yet.</p>
          <p className="mt-1 text-sm">Your daily progress shots will show up here.</p>
        </div>
      ) : (
        <div className="grid grid-cols-3 gap-2">
          {photos.map((p) => (
            <button
              key={p.id}
              onClick={() => setViewing(p)}
              className="relative aspect-square overflow-hidden rounded-xl transition active:scale-95"
            >
              <AuthImage
                src={p.thumb_url}
                alt={p.caption || `${p.kind} photo`}
                className="h-full w-full object-cover"
              />
              {p.day_number != null && (
                <span className="absolute bottom-1 left-1 rounded bg-black/65 px-1.5 py-0.5 text-[10px] text-white">
                  Day {p.day_number}
                </span>
              )}
            </button>
          ))}
        </div>
      )}

      {viewing && (
        <div className="fixed inset-0 z-50 flex flex-col bg-black/95">
          <div className="flex items-center justify-between p-4">
            <span className="text-sm text-ink-400">
              {viewing.day_number != null ? `Day ${viewing.day_number} · ` : ''}
              {new Date(viewing.taken_at).toLocaleDateString()}
            </span>
            <button className="p-2 text-2xl leading-none text-ink-300" onClick={() => setViewing(null)} aria-label="Close">
              ×
            </button>
          </div>
          <div className="flex flex-1 items-center justify-center p-4">
            <AuthImage
              src={viewing.url}
              alt={viewing.caption || 'Photo'}
              className="max-h-full max-w-full rounded-xl object-contain"
            />
          </div>
          <div className="p-4">
            <button className="btn-danger w-full" onClick={() => remove(viewing)}>
              Delete photo
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
