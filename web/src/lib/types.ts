// Shapes returned by the Go API. Kept hand-written and close to the handler
// structs — there is no generated client here.

export interface User {
  id: number
  email: string
  name: string
  avatar_url: string
  timezone: string
  is_admin: boolean
  auth_provider: string
  /** Display preference only — the API always uses kilograms. */
  weight_unit: 'kg' | 'lb'
  created_at: string
}

export interface AuthResponse {
  token: string
  user: User
}

export interface AuthConfig {
  allow_signup: boolean
  google: boolean
  github: boolean
  /** Whether this server can run WebAuthn ceremonies at all. */
  passkeys: boolean
}

export type TaskKind = 'check' | 'number' | 'duration' | 'photo' | 'text'

export interface ProgramTask {
  id: number
  key: string
  title: string
  detail: string
  icon: string
  kind: TaskKind
  target_num: number | null
  unit: string
  sort_order: number
  required: boolean
  color: string
  tracker: '' | 'nutrition' | 'workout' | 'meditation' | 'journal'
  /** Hidden optional tasks keep their history but leave the day and the grid. */
  hidden?: boolean
}

export interface Program {
  id: number
  name: string
  start_date: string
  length_days: number
  status: 'active' | 'completed' | 'failed' | 'abandoned'
  strict_restart: boolean
  attempt_number: number
  daily_kcal_target: number | null
  notes: string
  created_at: string
  ended_at: string | null
  current_day: number
  days_complete: number
  streak: number
  today: string
  tasks: ProgramTask[]
}

export interface Entry {
  task_id: number
  key: string
  title: string
  detail: string
  icon: string
  kind: TaskKind
  unit: string
  target_num: number | null
  required: boolean
  sort_order: number
  value: number | null
  note: string
  done: boolean
  completed_at: string | null
  /** Optional richer panel behind the task; never gates completion. */
  tracker: '' | 'nutrition' | 'workout' | 'meditation' | 'journal'
  color: string
}

export type Pose = '' | 'front' | 'side' | 'back'

export interface Photo {
  id: number
  kind: 'progress' | 'food' | 'ingredients'
  pose: Pose
  day_id: number | null
  day_number: number | null
  caption: string
  width: number
  height: number
  bytes: number
  taken_at: string
  url: string
  thumb_url: string
}

export interface MealItem {
  id: number
  name: string
  qty: number
  unit: string
  kcal: number
  protein_g: number
  carbs_g: number
  fat_g: number
  confidence: number | null
}

export interface Meal {
  id: number
  day_id: number
  photo_id: number | null
  photo_url?: string
  name: string
  slot: 'breakfast' | 'lunch' | 'dinner' | 'snack'
  kcal: number
  protein_g: number
  carbs_g: number
  fat_g: number
  source: 'manual' | 'ai'
  notes: string
  eaten_at: string
  items: MealItem[]
  /**
   * '' for a hand-entered meal, or the state of the background estimate for a
   * photographed one. Needed to tell "no numbers yet" from "zero calories".
   */
  estimate_status: '' | 'pending' | 'done' | 'failed'
  estimate_error?: string
}

export interface Meditation {
  id: number
  day_id: number
  minutes: number
  /** Free text: Calm, Headspace, Waking Up, Muse, or anything typed in. */
  source: string
  style: 'guided' | 'unguided' | 'breathwork' | 'body_scan' | 'walking' | 'other'
  notes: string
  /** A line about the sitting itself, kept separate from the journal. */
  reflection: string
  started_at?: string
  created_at: string
}

export interface Workout {
  id: number
  day_id: number
  kind: 'indoor' | 'outdoor'
  activity: string
  minutes: number
  kcal: number | null
  notes: string
  created_at: string
  /** Absent for a hand-logged entry with no time on it. */
  started_at?: string
  /** 1-based workout this record belongs to, grouped by the server. */
  session: number
}

export interface Totals {
  kcal: number
  protein_g: number
  carbs_g: number
  fat_g: number
  kcal_target: number | null
  workout_minutes: number
  outdoor_minutes: number
  /** Optional; zero on days with no sitting logged. */
  meditation_minutes: number
}

export interface Day {
  id: number
  program_id: number
  day_number: number
  date: string
  status: 'pending' | 'complete' | 'missed'
  note: string
  weight_kg: number | null
  /**
   * Entered by hand: Strava's API exposes average and max HR per activity,
   * but true resting HR lives in the device app, not in Strava.
   */
  resting_hr: number | null
  completed_at: string | null
  is_today: boolean
  tasks_done: number
  tasks_total: number
  entries: Entry[]
  photos: Photo[]
  meals: Meal[]
  workouts: Workout[]
  meditations: Meditation[]
  totals: Totals
}

export interface DaySummary {
  day_number: number
  date: string
  status: 'pending' | 'complete' | 'missed'
  tasks_done: number
  tasks_total: number
  photo_count: number
}

export interface TaskStat {
  task_id: number
  title: string
  icon: string
  completed: number
  rate: number
}

export interface WeightPoint {
  day_number: number
  date: string
  weight_kg: number
}

export interface Stats {
  program_id: number
  current_day: number
  length_days: number
  days_complete: number
  days_missed: number
  streak: number
  best_streak: number
  percent_done: number
  total_photos: number
  total_workouts: number
  total_minutes: number
  avg_kcal: number
  task_completion: TaskStat[]
  weight_series: WeightPoint[]
}

// ---- AI ----

export interface AIStatus {
  enabled: boolean
  providers: string[] | null
  used_today: number
  daily_limit: number
}

export interface FoodItemEstimate {
  name: string
  qty: number
  unit: string
  kcal: number
  protein_g: number
  carbs_g: number
  fat_g: number
  confidence: number
}

export interface FoodEstimate {
  name: string
  items: FoodItemEstimate[]
  notes: string
  kcal: number
  protein_g: number
  carbs_g: number
  fat_g: number
}

export interface Recipe {
  name: string
  summary: string
  minutes: number
  servings: number
  kcal_per_serving: number
  protein_g: number
  carbs_g: number
  fat_g: number
  ingredients: string[]
  steps: string[]
}

export interface PlanDay {
  day: number
  indoor: string
  outdoor: string
  nutrition: string
  note: string
}

export interface Plan {
  summary: string
  focus: string
  days: PlanDay[]
  tips: string[]
}

export interface CoachNote {
  note: string
  tone: string
}

// ---- activity grid ----

/** One activity's row: a cell per day of the program, index 0 == day 1. */
export interface GridTask {
  task_id: number
  key: string
  title: string
  icon: string
  kind: TaskKind
  unit: string
  target_num: number | null
  required: boolean
  color: string
  /** '' not done | 'd' done | 'p' partial | 'm' missed | 'f' future */
  cells: string[]
  values: Record<string, number>
  completed: number
  streak: number
  best_streak: number
}

export interface Grid {
  program_id: number
  start_date: string
  length_days: number
  current_day: number
  today: string
  tasks: GridTask[]
  day_status: string[]
}

// ---- camera roll ----

export interface RollDay {
  day_number: number
  date: string
  status: 'pending' | 'complete' | 'missed'
  photos: Photo[]
}

export interface Roll {
  program_id: number
  start_date: string
  length_days: number
  current_day: number
  days: RollDay[]
  total: number
  poses: Pose[]
  first_by_pose: Record<string, Photo | null>
  latest_by_pose: Record<string, Photo | null>
}

// ---- daily summary ----

export interface VitalPoint {
  day_number: number
  date: string
  weight_kg?: number
  resting_hr?: number
}

/** A measurement's arc over the attempt. All fields absent until logged. */
export interface Trend {
  first?: number
  latest?: number
  /** latest - first. Negative is an improvement for everything here. */
  change?: number
  /** The lowest reading: weight, resting HR and training HR all improve down. */
  best?: number
  average?: number
  count: number
}

export interface HeartRatePoint {
  day_number: number
  date: string
  average_hr: number
  max_hr: number
  minutes: number
}

export interface Summary {
  program_id: number
  current_day: number
  length_days: number
  days_complete: number
  days_missed: number
  streak: number
  best_streak: number
  percent_done: number
  total_photos: number
  total_workouts: number
  total_minutes: number
  outdoor_minutes: number
  meditation_minutes: number
  avg_kcal: number
  vitals: VitalPoint[]
  weight: Trend
  resting_hr: Trend
  heart_rate: HeartRatePoint[]
  /** Average training HR over time — the fitness signal Strava can supply. */
  activity_hr: Trend
}

// ---- strava ----

export interface StravaStatus {
  /** False when the server has no Strava application configured at all. */
  configured: boolean
  connected: boolean
  athlete?: string
  athlete_id?: number
  last_sync_at?: string
  last_error?: string
  activities: number
}

// ---- personal API tokens ----

export interface APIToken {
  id: number
  name: string
  /** The non-secret leading portion, for identifying a token in a list. */
  prefix: string
  scopes: ('read' | 'write')[]
  last_used_at?: string
  expires_at?: string
  created_at: string
}

/** What a caller needs to start using the API, returned with a new token. */
export interface TokenDiscovery {
  base_url: string
  spec_url: string
  auth_scheme: string
  example: string
  scopes: string[]
}

export interface CreatedToken {
  token: APIToken
  /** Shown once and unrecoverable. */
  secret: string
  discovery: TokenDiscovery
}

export interface ProviderBalance {
  provider: string
  currency: string
  amount: number
  available: boolean
  error?: string
  checked_at: string
}

// ---- per-account AI providers ----

export interface AISlot {
  slot: number
  provider: string
  model: string
  base_url?: string
  /** Last four characters of the stored key. The key itself never leaves the server. */
  key_hint: string
  enabled: boolean
  has_key: boolean
}

export interface ProviderInfo {
  name: string
  label: string
  suggested_model: string
  vision_model?: string
  free: boolean
  publishes_balance: boolean
  signup_url: string
}

export interface AIKeys {
  slots: AISlot[]
  using_own_keys: boolean
  /** Whether this account may fall back to the instance's own keys. */
  server_fallback: boolean
  /** False when the server has no encryption key, so nothing can be stored. */
  configurable: boolean
  providers: ProviderInfo[]
}

// ---- journal ----

export interface JournalEntry {
  id: number
  day_id?: number
  day_number?: number
  title: string
  kind: 'typed' | 'pdf'
  body: string
  file_name?: string
  file_bytes?: number
  page_count?: number
  /**
   * Text read out of an uploaded page. Kept apart from `body` so a machine
   * transcription is never mistaken for what was actually written.
   */
  parsed_text?: string
  parse_status: '' | 'pending' | 'done' | 'failed'
  parse_error?: string
  has_file: boolean
  created_at: string
  /** Set on search results: the matching text with the term marked. */
  snippet?: string
}

// ---- second factors ----

/**
 * What a password sign-in answers with when the account needs a code.
 *
 * The challenge is signed with a key that is not the session key, so it cannot
 * be presented to the rest of the API as though sign-in had finished.
 */
export interface TwoFactorChallenge {
  two_factor: true
  challenge: string
}

export interface TwoFactorStatus {
  enabled: boolean
  /** A secret exists but no code has proved it yet. */
  pending: boolean
  /** False when the server has no encryption key, so no secret can be stored. */
  configurable: boolean
  confirmed_at?: string
  recovery_codes_left: number
  passkey_count: number
  passkeys_usable: boolean
}

export interface TwoFactorSetup {
  /** Grouped for typing in by hand when the QR code cannot be scanned. */
  secret: string
  /** The otpauth:// string the QR code encodes. */
  uri: string
}

export interface Passkey {
  id: number
  name: string
  /** Synced to a password manager rather than bound to one device. */
  backed_up: boolean
  transports?: string
  created_at: string
  last_used_at?: string
}

/** One half of a WebAuthn ceremony: the server's options and its session id. */
export interface PasskeyCeremony {
  session_id: string
  options: { publicKey: Record<string, unknown> } | Record<string, unknown>
}
