import { useCallback, useEffect, useState } from 'react'
import { AuthImage } from '../components/AuthImage'
import { ConfirmDelete, POSE_LABEL, Tile } from '../components/PhotoTile'
import { ApiError, api } from '../lib/api'
import type { Photo, Pose, Roll } from '../lib/types'

type Tab = 'roll' | 'compare' | 'food'



const POSE_ORDER: Pose[] = ['front', 'side', 'back', '']

/**
 * The progress camera roll.
 *
 * Grouped by day rather than shown as a flat grid, because a progress roll is
 * read as a timeline — the day number is what you are comparing, not the
 * timestamp. Compare puts the first and latest shot of one angle side by side,
 * which is the only view that actually answers "has anything changed".
 */
export function Photos() {
  const [tab, setTab] = useState<Tab>('roll')
  const [roll, setRoll] = useState<Roll | null>(null)
  const [food, setFood] = useState<Photo[]>([])
  const [pose, setPose] = useState<Pose | 'all'>('all')
  const [loading, setLoading] = useState(true)
  const [viewing, setViewing] = useState<Photo | null>(null)
  // The photo a delete has been asked for but not yet confirmed. Deleting a
  // progress shot can un-complete a day, so it is never one tap away.
  const [confirming, setConfirming] = useState<Photo | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [deleteError, setDeleteError] = useState('')
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const p = await api.activeProgram()
      const [r, f] = await Promise.all([api.roll(p.id), api.listPhotos('food')])
      setRoll(r)
      setFood(f)
    } catch (err) {
      if (!(err instanceof ApiError && err.code === 'no_active_program')) {
        setError(err instanceof Error ? err.message : 'Could not load photos')
      }
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  async function retag(photo: Photo, next: Pose) {
    await api.updatePhoto(photo.id, { pose: next })
    setViewing(null)
    load()
  }

  async function remove(photo: Photo) {
    setDeleting(true)
    setDeleteError('')
    try {
      await api.deletePhoto(photo.id)
      setViewing(null)
      setConfirming(null)
      await load()
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Could not delete that photo')
    } finally {
      setDeleting(false)
    }
  }

  if (loading) {
    return (
      <div className="space-y-3">
        {[0, 1, 2].map((i) => (
          <div key={i} className="h-40 animate-pulse rounded-2xl bg-ink-900" />
        ))}
      </div>
    )
  }

  const days =
    pose === 'all'
      ? roll?.days ?? []
      : (roll?.days ?? [])
          .map((d) => ({ ...d, photos: d.photos.filter((p) => p.pose === pose) }))
          .filter((d) => d.photos.length > 0)

  const shotCount = days.reduce((n, d) => n + d.photos.length, 0)

  return (
    <div className="space-y-5 pb-4">
      <header className="animate-slide-up">
        <h1 className="text-2xl font-semibold text-ink-100">Camera roll</h1>
        <p className="mt-1 text-sm text-ink-500">
          {roll?.total ?? 0} progress {roll?.total === 1 ? 'shot' : 'shots'}
          {food.length > 0 && ` · ${food.length} food`}
        </p>
      </header>

      <div className="grid grid-cols-3 gap-2">
        {(['roll', 'compare', 'food'] as Tab[]).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`rounded-xl border py-2.5 text-sm capitalize transition ${
              tab === t
                ? 'border-flame-500 bg-flame-500/15 text-flame-400'
                : 'border-ink-800 bg-ink-900 text-ink-400'
            }`}
          >
            {t === 'roll' ? 'Timeline' : t === 'compare' ? 'Compare' : 'Food'}
          </button>
        ))}
      </div>

      {tab !== 'food' && (roll?.poses.length ?? 0) > 0 && (
        <div className="flex gap-2 overflow-x-auto pb-1">
          {(['all', ...POSE_ORDER.filter((p) => roll?.poses.includes(p))] as (Pose | 'all')[]).map(
            (p) => (
              <button
                key={p}
                onClick={() => setPose(p)}
                className={`shrink-0 rounded-full border px-4 py-2 text-sm transition ${
                  pose === p
                    ? 'border-flame-500 bg-flame-500/15 text-flame-400'
                    : 'border-ink-800 bg-ink-900 text-ink-400'
                }`}
              >
                {p === 'all' ? 'All angles' : POSE_LABEL[p]}
              </button>
            ),
          )}
        </div>
      )}

      {error && <p className="text-center text-sm text-red-400">{error}</p>}

      {tab === 'roll' && (
        <>
          {days.length === 0 ? (
            <Empty
              title="No progress photos yet"
              body="Take one from the same spot each day. Front, side and back if you can — side shows the most change and is the one people skip."
            />
          ) : (
            <>
              <p className="px-1 text-xs text-ink-600">{shotCount} shown</p>
              {days.map((day) => (
                <section key={day.day_number} className="card p-4">
                  <div className="mb-3 flex items-baseline justify-between">
                    <h2 className="font-medium text-ink-100">Day {day.day_number}</h2>
                    <span className="text-xs text-ink-500">
                      {new Date(`${day.date}T12:00:00`).toLocaleDateString(undefined, {
                        weekday: 'short',
                        month: 'short',
                        day: 'numeric',
                      })}
                    </span>
                  </div>
                  <div className="grid grid-cols-3 gap-2">
                    {day.photos.map((p) => (
                      <Tile
                        key={p.id}
                        photo={p}
                        ratio="aspect-[3/4]"
                        alt={`Day ${day.day_number} ${POSE_LABEL[p.pose] ?? ''}`}
                        badge={p.pose ? POSE_LABEL[p.pose] : ''}
                        onOpen={() => setViewing(p)}
                        onAskDelete={() => setConfirming(p)}
                      />
                    ))}
                  </div>
                </section>
              ))}
            </>
          )}
        </>
      )}

      {tab === 'compare' && <Compare roll={roll} pose={pose} onOpen={setViewing} />}

      {tab === 'food' && (
        <>
          {food.length === 0 ? (
            <Empty title="No food photos yet" body="Photograph a meal and the coach can estimate its calories." />
          ) : (
            <div className="grid grid-cols-3 gap-2">
              {food.map((p) => (
                <Tile
                  key={p.id}
                  photo={p}
                  ratio="aspect-square"
                  alt={p.caption || 'Meal'}
                  badge={p.day_number != null ? `Day ${p.day_number}` : ''}
                  onOpen={() => setViewing(p)}
                  onAskDelete={() => setConfirming(p)}
                />
              ))}
            </div>
          )}
        </>
      )}

      {viewing && (
        <Lightbox photo={viewing} onClose={() => setViewing(null)} onRetag={retag} onDelete={remove} />
      )}

      {confirming && (
        <ConfirmDelete
          photo={confirming}
          busy={deleting}
          error={deleteError}
          onCancel={() => {
            setConfirming(null)
            setDeleteError('')
          }}
          onConfirm={() => remove(confirming)}
        />
      )}
    </div>
  )
}

/** First shot against the most recent, per angle. */
function Compare({
  roll,
  pose,
  onOpen,
}: {
  roll: Roll | null
  pose: Pose | 'all'
  onOpen: (p: Photo) => void
}) {
  if (!roll) return null

  const poses = (pose === 'all' ? roll.poses : ([pose] as Pose[])).filter(
    (p) => roll.first_by_pose[p] && roll.latest_by_pose[p],
  )

  if (poses.length === 0) {
    return (
      <Empty
        title="Nothing to compare yet"
        body="Two shots of the same angle on different days is all it takes."
      />
    )
  }

  return (
    <>
      {poses.map((p) => {
        const first = roll.first_by_pose[p]!
        const latest = roll.latest_by_pose[p]!
        const sameShot = first.id === latest.id
        return (
          <section key={p || 'untagged'} className="card p-4">
            <h2 className="mb-3 font-medium text-ink-100">{POSE_LABEL[p] ?? 'Untagged'}</h2>
            {sameShot ? (
              <p className="text-sm text-ink-500">
                Only one {POSE_LABEL[p]?.toLowerCase()} shot so far — take another in a week.
              </p>
            ) : (
              <>
                <div className="grid grid-cols-2 gap-2">
                  {[first, latest].map((shot, i) => (
                    <button
                      key={shot.id}
                      onClick={() => onOpen(shot)}
                      className="overflow-hidden rounded-xl transition active:scale-95"
                    >
                      <AuthImage
                        src={shot.thumb_url}
                        alt={`${POSE_LABEL[p]} day ${shot.day_number}`}
                        className="aspect-[3/4] w-full object-cover"
                      />
                      <span className="mt-1.5 block text-center text-xs text-ink-500">
                        {i === 0 ? 'Day ' + shot.day_number : 'Day ' + shot.day_number + ' · latest'}
                      </span>
                    </button>
                  ))}
                </div>
                <p className="mt-3 text-center text-xs text-ink-600">
                  {(latest.day_number ?? 0) - (first.day_number ?? 0)} days apart
                </p>
              </>
            )}
          </section>
        )
      })}
    </>
  )
}

function Lightbox({
  photo,
  onClose,
  onRetag,
  onDelete,
}: {
  photo: Photo
  onClose: () => void
  onRetag: (p: Photo, pose: Pose) => void
  onDelete: (p: Photo) => void
}) {
  const [confirming, setConfirming] = useState(false)

  return (
    <div className="fixed inset-0 z-50 flex flex-col bg-black/95">
      <div className="flex items-center justify-between p-4">
        <span className="text-sm text-ink-400">
          {photo.day_number != null ? `Day ${photo.day_number} · ` : ''}
          {new Date(photo.taken_at).toLocaleDateString()}
        </span>
        <button className="p-2 text-2xl leading-none text-ink-300" onClick={onClose} aria-label="Close">
          ×
        </button>
      </div>

      <div className="flex flex-1 items-center justify-center overflow-hidden p-4">
        <AuthImage
          src={photo.url}
          alt={photo.caption || 'Photo'}
          className="max-h-full max-w-full rounded-xl object-contain"
        />
      </div>

      <div className="space-y-3 p-4">
        {photo.kind === 'progress' && (
          <div>
            <p className="mb-2 text-xs uppercase tracking-wide text-ink-500">Angle</p>
            <div className="grid grid-cols-4 gap-2">
              {(['front', 'side', 'back', ''] as Pose[]).map((p) => (
                <button
                  key={p || 'none'}
                  onClick={() => onRetag(photo, p)}
                  className={`rounded-xl border py-2 text-sm transition ${
                    photo.pose === p
                      ? 'border-flame-500 bg-flame-500/15 text-flame-400'
                      : 'border-ink-800 bg-ink-900 text-ink-400'
                  }`}
                >
                  {p ? POSE_LABEL[p] : 'None'}
                </button>
              ))}
            </div>
          </div>
        )}

        {confirming ? (
          <div className="flex gap-2">
            <button className="btn-ghost flex-1" onClick={() => setConfirming(false)}>
              Keep
            </button>
            <button className="btn-danger flex-1" onClick={() => onDelete(photo)}>
              Delete for good
            </button>
          </div>
        ) : (
          <button className="btn-danger w-full" onClick={() => setConfirming(true)}>
            Delete photo
          </button>
        )}
      </div>
    </div>
  )
}

function Empty({ title, body }: { title: string; body: string }) {
  return (
    <div className="card p-10 text-center">
      <p className="text-ink-300">{title}</p>
      <p className="mx-auto mt-2 max-w-xs text-sm text-ink-500">{body}</p>
    </div>
  )
}
