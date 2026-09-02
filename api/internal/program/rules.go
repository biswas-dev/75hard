// Package program holds the challenge rules: what "today" means for a user,
// when a day counts as complete, and what a miss does to the attempt.
//
// This is the only place in the codebase that decides those things, and it is
// deliberately free of database and HTTP concerns so it can be tested directly.
package program

import (
	"time"
)

// DateLayout is the calendar-date form used everywhere dates are stored.
// Dates are local to the user, so they are stored as text rather than as an
// instant — 2026-03-08 in Toronto is not a fixed UTC range.
const DateLayout = "2006-01-02"

// DefaultLength is the canonical challenge length.
const DefaultLength = 75

// Day statuses.
const (
	StatusPending  = "pending"
	StatusComplete = "complete"
	StatusMissed   = "missed"
)

// Program statuses.
const (
	ProgramActive    = "active"
	ProgramCompleted = "completed"
	ProgramFailed    = "failed"
	ProgramAbandoned = "abandoned"
)

// Task kinds.
const (
	KindCheck    = "check"
	KindNumber   = "number"
	KindDuration = "duration"
	KindPhoto    = "photo"
	KindText     = "text"
)

// LoadLocation resolves an IANA timezone name, falling back to UTC rather than
// failing — a bad timezone in a profile should not make the app unusable.
func LoadLocation(tz string) *time.Location {
	if tz == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.UTC
	}
	return loc
}

// LocalDate returns the calendar date at instant t in the given location.
func LocalDate(t time.Time, loc *time.Location) string {
	return t.In(loc).Format(DateLayout)
}

// ParseDate parses a stored calendar date.
func ParseDate(s string) (time.Time, error) {
	return time.ParseInLocation(DateLayout, s, time.UTC)
}

// DayNumber returns which day of the program the given local date is, counting
// the start date as day 1. It returns 0 for dates before the start.
func DayNumber(startDate, onDate string) int {
	start, err := ParseDate(startDate)
	if err != nil {
		return 0
	}
	on, err := ParseDate(onDate)
	if err != nil {
		return 0
	}
	diff := int(on.Sub(start).Hours()/24) + 1
	if diff < 1 {
		return 0
	}
	return diff
}

// DateForDay returns the calendar date of the nth day of a program.
func DateForDay(startDate string, day int) string {
	start, err := ParseDate(startDate)
	if err != nil {
		return ""
	}
	return start.AddDate(0, 0, day-1).Format(DateLayout)
}

// Task is the minimum a rule check needs to know about a template task.
type Task struct {
	ID        int64
	Kind      string
	TargetNum *float64
	Required  bool
}

// Entry is a user's progress against one task on one day.
type Entry struct {
	TaskID     int64
	Completed  bool
	ValueNum   *float64
	PhotoCount int
}

// EntrySatisfies reports whether an entry meets its task's bar.
//
// A check/photo/text task is done when it is marked done. A number or duration
// task is done when its value reaches the target — so logging 60oz of a 128oz
// water goal shows progress without falsely completing the day.
func EntrySatisfies(t Task, e Entry) bool {
	switch t.Kind {
	case KindNumber, KindDuration:
		if t.TargetNum == nil || *t.TargetNum <= 0 {
			return e.Completed
		}
		if e.ValueNum == nil {
			return false
		}
		return *e.ValueNum >= *t.TargetNum
	case KindPhoto:
		return e.Completed || e.PhotoCount > 0
	default:
		return e.Completed
	}
}

// DayComplete reports whether every required task has been satisfied, along
// with the counts used to render the progress ring.
//
// Both counts cover required tasks only, so done/required is always a truthful
// ratio: 6/6 means the day is complete and nothing else does.
//
// Optional tasks are excluded from both deliberately. Counting them in "done"
// would let a meditation session close out a day with the workout still
// undone, and would report "7/6" on a day where everything was finished.
// Whether an optional task was done is carried on the task itself.
func DayComplete(tasks []Task, entries map[int64]Entry) (complete bool, done, required int) {
	for _, t := range tasks {
		if !t.Required {
			continue
		}
		required++
		if EntrySatisfies(t, entries[t.ID]) {
			done++
		}
	}
	return required > 0 && done >= required, done, required
}

// Outcome describes what a day's state means for the attempt as a whole.
type Outcome struct {
	// DayStatus is the status the day should be stored with.
	DayStatus string
	// ProgramStatus is the status the program should move to, or "" to leave
	// it as it is.
	ProgramStatus string
	// ShouldRestart is true when the rules require starting over at day 1.
	ShouldRestart bool
}

// EvaluateDay decides the standing of a single day.
//
// today is the user's current local date; onDate is the day being judged. A day
// is only ever missed once it is in the past — an unfinished day that is still
// today is simply pending, which is the difference between "you have work left"
// and "you failed".
func EvaluateDay(onDate, today string, dayNumber, length int, complete, strictRestart bool) Outcome {
	if complete {
		if dayNumber >= length {
			return Outcome{DayStatus: StatusComplete, ProgramStatus: ProgramCompleted}
		}
		return Outcome{DayStatus: StatusComplete}
	}

	if onDate >= today {
		// Today, or a day that hasn't happened yet. Nothing has been lost.
		return Outcome{DayStatus: StatusPending}
	}

	if strictRestart {
		return Outcome{DayStatus: StatusMissed, ProgramStatus: ProgramFailed, ShouldRestart: true}
	}
	return Outcome{DayStatus: StatusMissed}
}

// Streak counts consecutive complete days ending at the most recent one, given
// day statuses keyed by day number.
func Streak(statuses map[int]string, throughDay int) int {
	n := 0
	for d := throughDay; d >= 1; d-- {
		if statuses[d] == StatusComplete {
			n++
			continue
		}
		break
	}
	return n
}

// Trackers a task can carry. A tracker is an optional richer panel; it never
// affects whether the task counts as done.
const (
	TrackerNone       = ""
	TrackerNutrition  = "nutrition"
	TrackerWorkout    = "workout"
	TrackerMeditation = "meditation"
)

// ValidTracker reports whether t is a tracker we know how to render.
func ValidTracker(t string) bool {
	switch t {
	case TrackerNone, TrackerNutrition, TrackerWorkout, TrackerMeditation:
		return true
	}
	return false
}

// DefaultTask is one entry in the starting template.
type DefaultTask struct {
	Key       string
	Title     string
	Detail    string
	Icon      string
	Kind      string
	TargetNum *float64
	Unit      string
	Required  bool
	Tracker   string
}

func f(v float64) *float64 { return &v }

// DefaultTasks returns the canonical six plus one optional extra, used as the
// starting template for a new program. Every field is editable afterwards, and
// the optional task can simply be deleted.
func DefaultTasks() []DefaultTask {
	return []DefaultTask{
		{
			Key: "workout_indoor", Title: "45-minute workout",
			Detail: "Any training you like, at least 45 minutes.",
			Icon:   "dumbbell", Kind: KindDuration, TargetNum: f(45), Unit: "min", Required: true,
			Tracker: TrackerWorkout,
		},
		{
			Key: "workout_outdoor", Title: "45-minute outdoor workout",
			Detail: "A second session, outside, whatever the weather.",
			Icon:   "tree", Kind: KindDuration, TargetNum: f(45), Unit: "min", Required: true,
			Tracker: TrackerWorkout,
		},
		{
			Key: "diet", Title: "Follow the diet",
			Detail: "No cheat meals, no alcohol. Tick it, or log what you ate.",
			Icon:   "salad", Kind: KindCheck, Required: true,
			Tracker: TrackerNutrition,
		},
		{
			Key: "water", Title: "Drink 1 gallon of water",
			Detail: "128 oz over the day.",
			Icon:   "droplet", Kind: KindNumber, TargetNum: f(128), Unit: "oz", Required: true,
		},
		{
			Key: "reading", Title: "Read 10 pages",
			Detail: "Non-fiction, personal development.",
			Icon:   "book", Kind: KindNumber, TargetNum: f(10), Unit: "pages", Required: true,
		},
		{
			Key: "progress_photo", Title: "Take a progress photo",
			Detail: "One photo, every day.",
			Icon:   "camera", Kind: KindPhoto, Required: true,
		},
		// Not part of the canonical rules, and deliberately not required:
		// missing it must never fail a run. It is here because the habit fits
		// the same daily rhythm, and tracking it alongside the rest is more
		// useful than tracking it somewhere else.
		{
			Key: "meditation", Title: "Meditate",
			Detail: "Optional. Log how long and where — this never fails the challenge.",
			Icon:   "lotus", Kind: KindCheck, Required: false,
			Tracker: TrackerMeditation,
		},
	}
}
