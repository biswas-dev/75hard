import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { StravaCard } from '../components/StravaCard'
import { api } from '../lib/api'
import type { Program } from '../lib/types'
import { useAuth } from '../state/AuthContext'

export function Settings() {
  const { user, logout, refresh } = useAuth()
  const navigate = useNavigate()

  const [name, setName] = useState(user?.name ?? '')
  const [timezone, setTimezone] = useState(user?.timezone ?? 'UTC')
  const [profileMsg, setProfileMsg] = useState('')
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [passwordMsg, setPasswordMsg] = useState('')
  const [programs, setPrograms] = useState<Program[]>([])
  const [confirmRestart, setConfirmRestart] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    api.listPrograms().then(setPrograms).catch(() => {})
  }, [])

  const active = programs.find((p) => p.status === 'active')

  async function saveProfile() {
    setError('')
    try {
      await api.updateProfile({ name, timezone })
      await refresh()
      setProfileMsg('Saved')
      window.setTimeout(() => setProfileMsg(''), 2500)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not save')
    }
  }

  async function changePassword() {
    setError('')
    try {
      await api.changePassword(currentPassword, newPassword)
      setCurrentPassword('')
      setNewPassword('')
      setPasswordMsg('Password updated')
      window.setTimeout(() => setPasswordMsg(''), 2500)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not change password')
    }
  }

  // The full IANA list is enormous; these cover the common cases and the
  // browser's own zone is always offered.
  const zones = Array.from(
    new Set([
      user?.timezone ?? 'UTC',
      Intl.DateTimeFormat().resolvedOptions().timeZone,
      'UTC',
      'America/Toronto',
      'America/New_York',
      'America/Chicago',
      'America/Denver',
      'America/Los_Angeles',
      'Europe/London',
      'Europe/Berlin',
      'Asia/Kolkata',
      'Asia/Singapore',
      'Australia/Sydney',
    ].filter(Boolean) as string[]),
  )

  return (
    <div className="space-y-5 pb-4">
      <header className="animate-slide-up">
        <h1 className="text-2xl font-semibold text-ink-100">You</h1>
        <p className="mt-1 text-sm text-ink-500">{user?.email}</p>
      </header>

      <section className="card space-y-4 p-4">
        <h2 className="text-sm font-medium uppercase tracking-wide text-ink-500">Profile</h2>
        <div>
          <label className="label" htmlFor="name">Name</label>
          <input id="name" className="field" value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        <div>
          <label className="label" htmlFor="tz">Timezone</label>
          <select id="tz" className="field" value={timezone} onChange={(e) => setTimezone(e.target.value)}>
            {zones.map((z) => (
              <option key={z} value={z}>
                {z}
              </option>
            ))}
          </select>
          <p className="mt-1.5 text-xs text-ink-600">
            Days roll over at midnight in this zone.
          </p>
        </div>
        <button className="btn-primary w-full" onClick={saveProfile}>
          {profileMsg || 'Save profile'}
        </button>
      </section>

      {user?.auth_provider === 'password' && (
        <section className="card space-y-4 p-4">
          <h2 className="text-sm font-medium uppercase tracking-wide text-ink-500">Password</h2>
          <div>
            <label className="label" htmlFor="cur">Current password</label>
            <input
              id="cur"
              type="password"
              autoComplete="current-password"
              className="field"
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
            />
          </div>
          <div>
            <label className="label" htmlFor="new">New password</label>
            <input
              id="new"
              type="password"
              autoComplete="new-password"
              className="field"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
            />
          </div>
          <button
            className="btn-ghost w-full"
            onClick={changePassword}
            disabled={!currentPassword || newPassword.length < 8}
          >
            {passwordMsg || 'Change password'}
          </button>
        </section>
      )}

      {active && (
        <section className="card space-y-3 p-4">
          <h2 className="text-sm font-medium uppercase tracking-wide text-ink-500">Current program</h2>
          <p className="text-ink-300">
            {active.name} · day {active.current_day} of {active.length_days}
          </p>
          <p className="text-sm text-ink-500">
            {active.strict_restart
              ? 'Strict: a missed day ends the attempt.'
              : 'Lenient: missed days are recorded but the attempt continues.'}
          </p>

          {confirmRestart ? (
            <div className="rounded-xl border border-red-500/25 bg-red-500/10 p-3">
              <p className="text-sm text-red-300">
                This ends the current attempt and starts again at day 1. Your history is kept.
              </p>
              <div className="mt-3 flex gap-2">
                <button className="btn-ghost flex-1 py-2 text-sm" onClick={() => setConfirmRestart(false)}>
                  Cancel
                </button>
                <button
                  className="btn-danger flex-1 py-2 text-sm"
                  onClick={async () => {
                    await api.restartProgram(active.id)
                    navigate('/app')
                  }}
                >
                  Restart at day 1
                </button>
              </div>
            </div>
          ) : (
            <button className="btn-ghost w-full" onClick={() => setConfirmRestart(true)}>
              Restart the challenge
            </button>
          )}
        </section>
      )}

      <StravaCard />

      {programs.length > 1 && (
        <section className="card p-4">
          <h2 className="mb-3 text-sm font-medium uppercase tracking-wide text-ink-500">Past attempts</h2>
          <ul className="divide-y divide-ink-800">
            {programs
              .filter((p) => p.status !== 'active')
              .map((p) => (
                <li key={p.id} className="flex items-center justify-between py-2.5 text-sm">
                  <span className="text-ink-300">
                    Attempt {p.attempt_number} · {p.start_date}
                  </span>
                  <span
                    className={`capitalize ${
                      p.status === 'completed' ? 'text-moss-400' : 'text-ink-500'
                    }`}
                  >
                    {p.status}
                  </span>
                </li>
              ))}
          </ul>
        </section>
      )}

      {error && <p className="text-center text-sm text-red-400">{error}</p>}

      <button
        className="btn-ghost w-full"
        onClick={() => {
          logout()
          navigate('/login', { replace: true })
        }}
      >
        Sign out
      </button>
    </div>
  )
}
