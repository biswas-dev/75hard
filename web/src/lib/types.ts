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
}

export interface Photo {
  id: number
  kind: 'progress' | 'food' | 'ingredients'
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
}

export interface Totals {
  kcal: number
  protein_g: number
  carbs_g: number
  fat_g: number
  kcal_target: number | null
  workout_minutes: number
  outdoor_minutes: number
}

export interface Day {
  id: number
  program_id: number
  day_number: number
  date: string
  status: 'pending' | 'complete' | 'missed'
  note: string
  weight_kg: number | null
  completed_at: string | null
  is_today: boolean
  tasks_done: number
  tasks_total: number
  entries: Entry[]
  photos: Photo[]
  meals: Meal[]
  workouts: Workout[]
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
