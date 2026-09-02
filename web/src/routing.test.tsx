import { render, screen } from '@testing-library/react'
import { MemoryRouter, Navigate, Route, Routes, useNavigate, useParams } from 'react-router-dom'
import { describe, expect, it } from 'vitest'

/**
 * A smoke test for the router itself, added when react-router moved from v6 to
 * v7 to close an open-redirect advisory.
 *
 * A clean typecheck and build prove the API shape is compatible; they prove
 * nothing about runtime behaviour, and routing is exactly the kind of thing
 * that compiles fine and then renders the wrong screen. These exercise the
 * pieces the app actually relies on.
 */

function Screen({ label }: { label: string }) {
  return <p>{label}</p>
}

function DayScreen() {
  const { dayNumber } = useParams()
  return <p>day {dayNumber}</p>
}

function Navigator() {
  const navigate = useNavigate()
  return (
    <button onClick={() => navigate('/app/settings')}>go to settings</button>
  )
}

describe('routing on react-router v7', () => {
  it('renders the route matching the current path', () => {
    render(
      <MemoryRouter initialEntries={['/app']}>
        <Routes>
          <Route path="/app" element={<Screen label="today" />} />
          <Route path="/login" element={<Screen label="login" />} />
        </Routes>
      </MemoryRouter>,
    )
    expect(screen.getByText('today')).toBeInTheDocument()
    expect(screen.queryByText('login')).not.toBeInTheDocument()
  })

  it('redirects through Navigate, as the auth guards do', () => {
    // Every protected screen depends on this: no user means Navigate to
    // /login, and a redirect that silently stopped working would leave the
    // app rendering a blank protected page.
    render(
      <MemoryRouter initialEntries={['/']}>
        <Routes>
          <Route path="/" element={<Navigate to="/login" replace />} />
          <Route path="/login" element={<Screen label="login" />} />
        </Routes>
      </MemoryRouter>,
    )
    expect(screen.getByText('login')).toBeInTheDocument()
  })

  it('reads path parameters', () => {
    render(
      <MemoryRouter initialEntries={['/app/day/42']}>
        <Routes>
          <Route path="/app/day/:dayNumber" element={<DayScreen />} />
        </Routes>
      </MemoryRouter>,
    )
    expect(screen.getByText('day 42')).toBeInTheDocument()
  })

  it('navigates imperatively via useNavigate', async () => {
    render(
      <MemoryRouter initialEntries={['/app']}>
        <Routes>
          <Route path="/app" element={<Navigator />} />
          <Route path="/app/settings" element={<Screen label="settings" />} />
        </Routes>
      </MemoryRouter>,
    )
    screen.getByRole('button', { name: 'go to settings' }).click()
    expect(await screen.findByText('settings')).toBeInTheDocument()
  })

  it('renders nested routes through an index route', () => {
    render(
      <MemoryRouter initialEntries={['/app']}>
        <Routes>
          <Route path="/app">
            <Route index element={<Screen label="today" />} />
            <Route path="settings" element={<Screen label="settings" />} />
          </Route>
        </Routes>
      </MemoryRouter>,
    )
    expect(screen.getByText('today')).toBeInTheDocument()
  })

  it('falls through to a catch-all for an unknown path', () => {
    render(
      <MemoryRouter initialEntries={['/nope']}>
        <Routes>
          <Route path="/app" element={<Screen label="today" />} />
          <Route path="*" element={<Navigate to="/app" replace />} />
        </Routes>
      </MemoryRouter>,
    )
    expect(screen.getByText('today')).toBeInTheDocument()
  })
})
