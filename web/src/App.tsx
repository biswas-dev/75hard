import { Suspense, lazy } from 'react'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { Layout } from './components/Layout'
import { AuthProvider, useAuth } from './state/AuthContext'

import { Login } from './routes/Login'
import { Signup } from './routes/Signup'
import { ForgotPassword, ResetPassword } from './routes/ForgotPassword'
import { OAuthCallback } from './routes/OAuthCallback'
import { Today } from './routes/Today'

// The screens behind the tab bar load on demand — the first paint after login
// only needs Today.
const Calendar = lazy(() => import('./routes/Calendar').then((m) => ({ default: m.Calendar })))
const DayDetail = lazy(() => import('./routes/DayDetail').then((m) => ({ default: m.DayDetail })))
const Photos = lazy(() => import('./routes/Photos').then((m) => ({ default: m.Photos })))
const StatsPage = lazy(() => import('./routes/Stats').then((m) => ({ default: m.StatsPage })))
const Settings = lazy(() => import('./routes/Settings').then((m) => ({ default: m.Settings })))
const Coach = lazy(() => import('./routes/Coach').then((m) => ({ default: m.Coach })))
const StartProgram = lazy(() => import('./routes/StartProgram').then((m) => ({ default: m.StartProgram })))

function ProtectedRoute({ children }: { children: JSX.Element }) {
  const { user, loading } = useAuth()
  if (loading) return <FullPageSpinner />
  if (!user) return <Navigate to="/login" replace />
  return children
}

function PublicOnly({ children }: { children: JSX.Element }) {
  const { user, loading } = useAuth()
  if (loading) return <FullPageSpinner />
  if (user) return <Navigate to="/app" replace />
  return children
}

function FullPageSpinner() {
  return (
    <div className="flex min-h-dvh items-center justify-center">
      <div className="h-8 w-8 animate-spin rounded-full border-2 border-ink-700 border-t-flame-500" />
    </div>
  )
}

export default function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Suspense fallback={<FullPageSpinner />}>
          <Routes>
            <Route path="/" element={<Navigate to="/app" replace />} />
            <Route path="/login" element={<PublicOnly><Login /></PublicOnly>} />
            <Route path="/signup" element={<PublicOnly><Signup /></PublicOnly>} />
            <Route path="/forgot-password" element={<PublicOnly><ForgotPassword /></PublicOnly>} />
            {/* Not PublicOnly: a signed-in user following a reset link should
                still be able to set the new password. */}
            <Route path="/reset-password" element={<ResetPassword />} />
            {/* go-login redirects here with ?token= after a provider sign-in. */}
            <Route path="/oauth/callback" element={<OAuthCallback />} />

            <Route path="/start" element={<ProtectedRoute><StartProgram /></ProtectedRoute>} />

            <Route path="/app" element={<ProtectedRoute><Layout /></ProtectedRoute>}>
              <Route index element={<Today />} />
              <Route path="calendar" element={<Calendar />} />
              <Route path="day/:dayNumber" element={<DayDetail />} />
              <Route path="photos" element={<Photos />} />
              <Route path="coach" element={<Coach />} />
              <Route path="stats" element={<StatsPage />} />
              <Route path="settings" element={<Settings />} />
            </Route>

            <Route path="*" element={<Navigate to="/app" replace />} />
          </Routes>
        </Suspense>
      </BrowserRouter>
    </AuthProvider>
  )
}
