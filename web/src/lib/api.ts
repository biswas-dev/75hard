import type {
  AIKeys,
  AIStatus,
  APIToken,
  AuthConfig,
  CoachNote,
  CreatedToken,
  FoodEstimate,
  AuthResponse,
  Day,
  DaySummary,
  Grid,
  JournalEntry,
  Meal,
  Meditation,
  Photo,
  Plan,
  Passkey,
  PasskeyCeremony,
  Pose,
  ProviderBalance,
  Program,
  ProgramTask,
  Recipe,
  Roll,
  Stats,
  StravaStatus,
  Summary,
  TwoFactorChallenge,
  TwoFactorSetup,
  TwoFactorStatus,
  User,
  Workout,
} from './types'

const BASE_URL = import.meta.env.VITE_API_URL || ''
const TOKEN_KEY = 'sh_token'

export class ApiError extends Error {
  status: number
  code: string

  constructor(message: string, status: number, code = '') {
    super(message)
    this.status = status
    this.code = code
  }
}

class ApiClient {
  private token: string | null

  constructor() {
    this.token = localStorage.getItem(TOKEN_KEY)
  }

  setToken(token: string | null) {
    this.token = token
    if (token) localStorage.setItem(TOKEN_KEY, token)
    else localStorage.removeItem(TOKEN_KEY)
  }

  getToken() {
    return this.token
  }

  /** Absolute URL for an image, with the bearer token unavailable to <img>. */
  private authHeaders(): Record<string, string> {
    return this.token ? { Authorization: `Bearer ${this.token}` } : {}
  }

  private async request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const headers: Record<string, string> = {
      ...this.authHeaders(),
      ...(options.headers as Record<string, string>),
    }
    if (options.body && !(options.body instanceof FormData)) {
      headers['Content-Type'] = 'application/json'
    }

    const res = await fetch(`${BASE_URL}${path}`, { ...options, headers })

    if (res.status === 204) return {} as T

    // An HTML error page from nginx would blow up JSON.parse with a useless
    // message, so check the content type before trusting the body.
    const contentType = res.headers.get('content-type') || ''
    if (!contentType.includes('application/json')) {
      if (!res.ok) throw new ApiError(`Request failed (${res.status})`, res.status)
      return {} as T
    }

    const data = await res.json()
    if (!res.ok) {
      throw new ApiError(data?.error || `Request failed (${res.status})`, res.status, data?.code || '')
    }
    return data as T
  }

  // ---- auth ----

  authConfig() {
    return this.request<AuthConfig>('/api/auth/config')
  }

  signup(body: { email: string; password: string; name?: string; timezone?: string }) {
    return this.request<AuthResponse>('/api/auth/signup', {
      method: 'POST',
      body: JSON.stringify(body),
    })
  }

  /**
   * Sign in with a password.
   *
   * Answers with either a session or a two-factor challenge, so the caller has
   * to look before assuming it is signed in.
   */
  login(email: string, password: string) {
    return this.request<AuthResponse | TwoFactorChallenge>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ email, password }),
    })
  }

  /** Finish a sign-in that stopped for a code. */
  verifyTwoFactor(challenge: string, code: string) {
    return this.request<AuthResponse>('/api/auth/2fa/verify', {
      method: 'POST',
      body: JSON.stringify({ challenge, code }),
    })
  }

  twoFactorStatus() {
    return this.request<TwoFactorStatus>('/api/2fa')
  }

  twoFactorSetup() {
    return this.request<TwoFactorSetup>('/api/2fa/setup', { method: 'POST' })
  }

  twoFactorConfirm(code: string) {
    return this.request<{ enabled: boolean; recovery_codes: string[] }>('/api/2fa/confirm', {
      method: 'POST',
      body: JSON.stringify({ code }),
    })
  }

  twoFactorDisable(body: { password?: string; code?: string }) {
    return this.request<{ ok: boolean }>('/api/2fa/disable', {
      method: 'POST',
      body: JSON.stringify({ password: body.password ?? '', code: body.code ?? '' }),
    })
  }

  // ---- passkeys ----

  listPasskeys() {
    return this.request<Passkey[]>('/api/passkeys')
  }

  deletePasskey(id: number) {
    return this.request<{ ok: boolean }>(`/api/passkeys/${id}`, { method: 'DELETE' })
  }

  passkeyRegisterBegin() {
    return this.request<PasskeyCeremony>('/api/passkeys/register/begin', { method: 'POST' })
  }

  passkeyRegisterFinish(body: { session_id: string; name: string; credential: unknown }) {
    return this.request<{ ok: boolean }>('/api/passkeys/register/finish', {
      method: 'POST',
      body: JSON.stringify(body),
    })
  }

  passkeyLoginBegin() {
    return this.request<PasskeyCeremony>('/api/auth/passkeys/login/begin', { method: 'POST' })
  }

  passkeyLoginFinish(body: { session_id: string; credential: unknown }) {
    return this.request<AuthResponse>('/api/auth/passkeys/login/finish', {
      method: 'POST',
      body: JSON.stringify({ ...body, name: '' }),
    })
  }

  me() {
    return this.request<User>('/api/me')
  }

  updateProfile(body: { name?: string; timezone?: string; weight_unit?: 'kg' | 'lb' }) {
    return this.request<User>('/api/me', { method: 'PATCH', body: JSON.stringify(body) })
  }

  changePassword(current_password: string, new_password: string) {
    return this.request<{ ok: boolean }>('/api/me/password', {
      method: 'POST',
      body: JSON.stringify({ current_password, new_password }),
    })
  }

  // ---- programs ----

  activeProgram() {
    return this.request<Program>('/api/programs/active')
  }

  listPrograms() {
    return this.request<Program[]>('/api/programs')
  }

  createProgram(body: {
    name?: string
    start_date?: string
    length_days?: number
    strict_restart?: boolean
    daily_kcal_target?: number | null
    tasks?: Array<Partial<ProgramTask>>
  }) {
    return this.request<Program>('/api/programs', { method: 'POST', body: JSON.stringify(body) })
  }

  updateProgram(id: number, body: Record<string, unknown>) {
    return this.request<Program>(`/api/programs/${id}`, { method: 'PATCH', body: JSON.stringify(body) })
  }

  restartProgram(id: number) {
    return this.request<Program>(`/api/programs/${id}/restart`, { method: 'POST' })
  }

  addTask(programId: number, body: Partial<ProgramTask>) {
    return this.request<ProgramTask[]>(`/api/programs/${programId}/tasks`, {
      method: 'POST',
      body: JSON.stringify(body),
    })
  }

  updateTask(programId: number, taskId: number, body: Partial<ProgramTask>) {
    return this.request<ProgramTask[]>(`/api/programs/${programId}/tasks/${taskId}`, {
      method: 'PATCH',
      body: JSON.stringify(body),
    })
  }

  deleteTask(programId: number, taskId: number) {
    return this.request<{ ok: boolean }>(`/api/programs/${programId}/tasks/${taskId}`, {
      method: 'DELETE',
    })
  }

  // ---- days ----

  today() {
    return this.request<Day>('/api/today')
  }

  listDays(programId: number) {
    return this.request<DaySummary[]>(`/api/programs/${programId}/days`)
  }

  grid(programId: number) {
    return this.request<Grid>(`/api/programs/${programId}/grid`)
  }

  getDay(programId: number, dayNumber: number) {
    return this.request<Day>(`/api/programs/${programId}/days/${dayNumber}`)
  }

  updateDay(
    programId: number,
    dayNumber: number,
    body: { note?: string; weight_kg?: number; resting_hr?: number },
  ) {
    return this.request<Day>(`/api/programs/${programId}/days/${dayNumber}`, {
      method: 'PATCH',
      body: JSON.stringify(body),
    })
  }

  toggleTask(
    programId: number,
    dayNumber: number,
    taskId: number,
    body: { done?: boolean; value?: number; note?: string },
  ) {
    return this.request<Day>(`/api/programs/${programId}/days/${dayNumber}/tasks/${taskId}`, {
      method: 'POST',
      body: JSON.stringify(body),
    })
  }

  stats() {
    return this.request<Stats>('/api/stats')
  }

  // ---- photos ----

  listPhotos(kind?: string) {
    const q = kind ? `?kind=${encodeURIComponent(kind)}` : ''
    return this.request<Photo[]>(`/api/photos${q}`)
  }

  uploadPhoto(
    file: Blob,
    opts: {
      kind?: string
      dayNumber?: number
      caption?: string
      pose?: Pose
      slot?: string
      /** Food photos only: create the meal and estimate it in the background. */
      autolog?: boolean
    } = {},
  ) {
    const form = new FormData()
    form.append('file', file, 'photo.webp')
    if (opts.kind) form.append('kind', opts.kind)
    if (opts.dayNumber) form.append('day_number', String(opts.dayNumber))
    if (opts.caption) form.append('caption', opts.caption)
    // Only for food shots; omitted so the server infers it from the clock.
    if (opts.slot) form.append('slot', opts.slot)
    if (opts.autolog) form.append('autolog', '1')
    if (opts.pose) form.append('pose', opts.pose)
    return this.request<Photo>('/api/photos', { method: 'POST', body: form })
  }

  updatePhoto(id: number, body: { pose?: Pose; caption?: string }) {
    return this.request<Photo>(`/api/photos/${id}`, { method: 'PATCH', body: JSON.stringify(body) })
  }

  roll(programId: number, pose?: Pose) {
    const q = pose ? `?pose=${encodeURIComponent(pose)}` : ''
    return this.request<Roll>(`/api/programs/${programId}/roll${q}`)
  }

  forgotPassword(email: string) {
    return this.request<{ ok: boolean; message: string; token?: string; reset_url?: string }>(
      '/api/auth/forgot-password',
      { method: 'POST', body: JSON.stringify({ email }) },
    )
  }

  resetPassword(token: string, newPassword: string) {
    return this.request<AuthResponse>('/api/auth/reset-password', {
      method: 'POST',
      body: JSON.stringify({ token, new_password: newPassword }),
    })
  }

  deletePhoto(id: number) {
    return this.request<{ ok: boolean }>(`/api/photos/${id}`, { method: 'DELETE' })
  }

  /**
   * Photos sit behind bearer auth, so an <img src> cannot fetch them directly.
   * Fetch the bytes and hand back an object URL the caller must revoke.
   */
  async photoObjectURL(url: string): Promise<string> {
    const res = await fetch(`${BASE_URL}${url}`, { headers: this.authHeaders() })
    if (!res.ok) throw new ApiError('could not load image', res.status)
    return URL.createObjectURL(await res.blob())
  }

  // ---- nutrition and training ----

  createMeal(body: Record<string, unknown>) {
    return this.request<Meal>('/api/meals', { method: 'POST', body: JSON.stringify(body) })
  }

  updateMeal(id: number, body: Record<string, unknown>) {
    return this.request<Meal>(`/api/meals/${id}`, { method: 'PATCH', body: JSON.stringify(body) })
  }

  deleteMeal(id: number) {
    return this.request<{ ok: boolean }>(`/api/meals/${id}`, { method: 'DELETE' })
  }

  // ---- AI ----

  aiStatus() {
    return this.request<AIStatus>('/api/ai/status')
  }

  analyzeFood(photoId: number, hint?: string) {
    return this.request<{ estimate: FoodEstimate; cached: boolean; provider?: string; model?: string }>(
      '/api/ai/food',
      { method: 'POST', body: JSON.stringify({ photo_id: photoId, hint: hint || '' }) },
    )
  }

  suggestRecipes(body: {
    ingredients?: string[]
    preferences?: string
    meal_slot?: string
    photo_id?: number | null
  }) {
    return this.request<{ recipes: Recipe[]; provider?: string }>('/api/ai/recipes', {
      method: 'POST',
      body: JSON.stringify(body),
    })
  }

  buildPlan(goals: string, force = false) {
    return this.request<{ plan: Plan; cached: boolean }>('/api/ai/plan', {
      method: 'POST',
      body: JSON.stringify({ goals, force }),
    })
  }

  coachNote() {
    return this.request<{ note: CoachNote; cached: boolean }>('/api/ai/coach')
  }

  createWorkout(body: Record<string, unknown>) {
    return this.request<Workout>('/api/workouts', { method: 'POST', body: JSON.stringify(body) })
  }

  deleteWorkout(id: number) {
    return this.request<{ ok: boolean }>(`/api/workouts/${id}`, { method: 'DELETE' })
  }

  createMeditation(body: Record<string, unknown>) {
    return this.request<Meditation>('/api/meditations', {
      method: 'POST',
      body: JSON.stringify(body),
    })
  }

  deleteMeditation(id: number) {
    return this.request<{ ok: boolean }>(`/api/meditations/${id}`, { method: 'DELETE' })
  }

  /** Everything the main page renders, in one call. */
  getSummary() {
    return this.request<Summary>('/api/summary')
  }

  listTokens() {
    return this.request<APIToken[]>('/api/tokens')
  }

  createToken(body: { name: string; scopes: string[]; expires_in_days?: number }) {
    return this.request<CreatedToken>('/api/tokens', {
      method: 'POST',
      body: JSON.stringify(body),
    })
  }

  revokeToken(id: number) {
    return this.request<{ ok: boolean }>(`/api/tokens/${id}`, { method: 'DELETE' })
  }

  /** Credit left with providers that publish it. Cached server-side. */
  aiBalance() {
    return this.request<{ balances: ProviderBalance[]; cached: boolean }>('/api/ai/balance')
  }

  journal(query?: string) {
    const q = query ? `?q=${encodeURIComponent(query)}` : ''
    return this.request<JournalEntry[]>(`/api/journal${q}`)
  }

  createJournal(body: { day_number?: number; title?: string; body: string }) {
    return this.request<JournalEntry>('/api/journal', {
      method: 'POST',
      body: JSON.stringify(body),
    })
  }

  updateJournal(id: number, body: { title?: string; body?: string }) {
    return this.request<JournalEntry>(`/api/journal/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(body),
    })
  }

  deleteJournal(id: number) {
    return this.request<{ ok: boolean }>(`/api/journal/${id}`, { method: 'DELETE' })
  }

  uploadJournal(file: File, opts: { dayNumber?: number; title?: string } = {}) {
    const form = new FormData()
    form.append('file', file, file.name)
    if (opts.dayNumber) form.append('day_number', String(opts.dayNumber))
    if (opts.title) form.append('title', opts.title)
    return this.request<JournalEntry>('/api/journal/upload', { method: 'POST', body: form })
  }

  aiKeys() {
    return this.request<AIKeys>('/api/ai/keys')
  }

  saveAIKey(
    slot: number,
    body: { provider: string; model: string; base_url?: string; api_key?: string; enabled?: boolean },
  ) {
    return this.request<{ slots: AIKeys['slots'] }>(`/api/ai/keys/${slot}`, {
      method: 'PUT',
      body: JSON.stringify(body),
    })
  }

  deleteAIKey(slot: number) {
    return this.request<{ slots: AIKeys['slots'] }>(`/api/ai/keys/${slot}`, { method: 'DELETE' })
  }

  stravaStatus() {
    return this.request<StravaStatus>('/api/strava/status')
  }

  /** Returns the Strava consent URL to send the browser to. */
  stravaConnect() {
    return this.request<{ url: string }>('/api/strava/connect', { method: 'POST' })
  }

  stravaSync() {
    return this.request<{ imported: number }>('/api/strava/sync', { method: 'POST' })
  }

  stravaDisconnect() {
    return this.request<{ ok: boolean }>('/api/strava', { method: 'DELETE' })
  }

  /** Re-runs a background food estimate that failed, usually on quota. */
  retryEstimate(mealId: number) {
    return this.request<{ status: string }>(`/api/meals/${mealId}/estimate`, { method: 'POST' })
  }
}

export const api = new ApiClient()
