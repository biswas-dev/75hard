import { useState } from 'react'
import { AuthImage } from './AuthImage'
import { api } from '../lib/api'
import type { Photo } from '../lib/types'

export const POSE_LABEL: Record<string, string> = {
  '': 'Untagged',
  front: 'Front',
  side: 'Side',
  back: 'Back',
}

/**
 * One photo in a grid, with a delete control.
 *
 * The trash sits on the tile rather than only inside the viewer: deleting a
 * duplicate meant opening it full screen and scrolling past the angle picker,
 * which is enough friction that photos simply accumulated. It appears on hover
 * on a pointer device and stays visible on a touch screen, where there is no
 * hover to reveal it.
 */
export function Tile({
  photo,
  ratio,
  alt,
  badge,
  onOpen,
  onAskDelete,
}: {
  photo: Photo
  ratio: string
  alt: string
  badge: string
  onOpen: () => void
  onAskDelete: () => void
}) {
  return (
    <div className={`group relative ${ratio} overflow-hidden rounded-xl`}>
      <button onClick={onOpen} className="block h-full w-full transition active:scale-95">
        <AuthImage src={photo.thumb_url} alt={alt} className="h-full w-full object-cover" />
      </button>

      {badge && (
        <span className="pointer-events-none absolute bottom-1 left-1 rounded bg-black/70 px-1.5 py-0.5 text-[10px] text-white">
          {badge}
        </span>
      )}

      <button
        onClick={onAskDelete}
        aria-label="Delete photo"
        title="Delete photo"
        className="absolute right-1 top-1 rounded-lg bg-black/70 p-1.5 text-ink-300 opacity-100 transition hover:bg-red-500/80 hover:text-white focus:opacity-100 md:opacity-0 md:group-hover:opacity-100"
      >
        <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" strokeWidth="2">
          <path d="M3 6h18M8 6V4h8v2M6 6l1 14h10l1-14" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </button>
    </div>
  )
}

/** Asks before a photo goes, and says what losing it costs. */
export function ConfirmDelete({
  photo,
  busy,
  error,
  onCancel,
  onConfirm,
}: {
  photo: Photo
  busy: boolean
  error: string
  onCancel: () => void
  onConfirm: () => void
}) {
  return (
    <div className="fixed inset-0 z-[60] flex items-center justify-center p-6">
      <button className="absolute inset-0 bg-black/80 backdrop-blur-sm" onClick={onCancel} aria-label="Cancel" />

      <div className="card relative w-full max-w-sm space-y-4 p-5">
        <div className="flex gap-3">
          <div className="h-20 w-16 shrink-0 overflow-hidden rounded-lg">
            <AuthImage src={photo.thumb_url} alt="" className="h-full w-full object-cover" />
          </div>
          <div className="min-w-0">
            <h2 className="font-medium text-ink-100">Delete this photo?</h2>
            <p className="mt-1 text-sm text-ink-500">
              {photo.day_number != null ? `Day ${photo.day_number}` : 'Not on a day'}
              {photo.pose ? ` · ${POSE_LABEL[photo.pose]}` : ''}
            </p>
            <p className="mt-2 text-sm text-ink-400">
              {photo.kind === 'progress'
                ? 'This cannot be undone, and if it is the only progress photo on that day, the day stops counting as complete.'
                : 'This cannot be undone.'}
            </p>
          </div>
        </div>

        {error && <p className="text-sm text-red-400">{error}</p>}

        <div className="flex gap-2">
          <button className="btn-ghost flex-1" onClick={onCancel} disabled={busy}>
            Keep
          </button>
          <button className="btn-danger flex-1" onClick={onConfirm} disabled={busy}>
            {busy ? 'Deleting…' : 'Delete'}
          </button>
        </div>
      </div>
    </div>
  )
}


/**
 * A grid of photos that can be deleted, with the confirmation flow attached.
 *
 * Kept in one place because the day view and the gallery both show photos and
 * both need to be able to remove one; two copies of a destructive flow is two
 * chances to guard it differently.
 */
export function DeletablePhotos({
  photos,
  ratio,
  className,
  altFor,
  badgeFor,
  onOpen,
  onDeleted,
}: {
  photos: Photo[]
  ratio: string
  className: string
  altFor: (p: Photo) => string
  badgeFor: (p: Photo) => string
  onOpen?: (p: Photo) => void
  onDeleted: () => void | Promise<void>
}) {
  const [confirming, setConfirming] = useState<Photo | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function remove(photo: Photo) {
    setBusy(true)
    setError('')
    try {
      await api.deletePhoto(photo.id)
      setConfirming(null)
      await onDeleted()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not delete that photo')
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <div className={className}>
        {photos.map((p) => (
          <Tile
            key={p.id}
            photo={p}
            ratio={ratio}
            alt={altFor(p)}
            badge={badgeFor(p)}
            onOpen={() => onOpen?.(p)}
            onAskDelete={() => setConfirming(p)}
          />
        ))}
      </div>

      {confirming && (
        <ConfirmDelete
          photo={confirming}
          busy={busy}
          error={error}
          onCancel={() => {
            setConfirming(null)
            setError('')
          }}
          onConfirm={() => remove(confirming)}
        />
      )}
    </>
  )
}
