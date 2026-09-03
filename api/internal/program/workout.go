package program

import (
	"sort"
	"time"
)

// SessionGap is how far apart two workouts must start before they count as
// two sessions rather than one.
//
// Below it, a day's records are describing the same trip to the gym from two
// sources — thirty-one minutes picked up from Strava and a fifteen-minute
// treadmill leg added by hand — and adding them is right. Above it, they are
// genuinely separate efforts, and adding them would let three short walks
// stand in for a forty-five minute workout, which is not the same thing.
const SessionGap = 2 * time.Hour

// WorkoutRecord is one logged effort: an imported activity or a hand-entered
// session.
type WorkoutRecord struct {
	ID      int64
	Kind    string // "indoor" or "outdoor"
	Minutes int
	// StartedAt is nil for a hand-logged entry that carries no time. Strava
	// always supplies one; the app only can when the entry is for today.
	StartedAt *time.Time
}

// Session is one workout, possibly assembled from several records.
type Session struct {
	Start   time.Time
	Minutes int
	// Outdoor is true when any part of the session was outside. A swim in a
	// lake followed by a stretch in the car park is still an outdoor session.
	Outdoor bool
	Records []int64
}

// GroupSessions folds a day's records into sessions.
//
// Timed records are walked in order and merged while they start within gap of
// the session already open. Untimed records join the latest session, because a
// figure added by hand is almost always topping up the effort just finished;
// with no session to join they form one of their own.
func GroupSessions(recs []WorkoutRecord, gap time.Duration) []Session {
	timed := make([]WorkoutRecord, 0, len(recs))
	var untimed []WorkoutRecord
	for _, r := range recs {
		if r.StartedAt == nil {
			untimed = append(untimed, r)
			continue
		}
		timed = append(timed, r)
	}
	sort.SliceStable(timed, func(i, j int) bool {
		return timed[i].StartedAt.Before(*timed[j].StartedAt)
	})

	var sessions []Session
	for _, r := range timed {
		if n := len(sessions); n > 0 && r.StartedAt.Sub(sessions[n-1].Start) < gap {
			sessions[n-1].Minutes += r.Minutes
			sessions[n-1].Outdoor = sessions[n-1].Outdoor || r.Kind == "outdoor"
			sessions[n-1].Records = append(sessions[n-1].Records, r.ID)
			continue
		}
		sessions = append(sessions, Session{
			Start:   *r.StartedAt,
			Minutes: r.Minutes,
			Outdoor: r.Kind == "outdoor",
			Records: []int64{r.ID},
		})
	}

	for _, r := range untimed {
		if n := len(sessions); n > 0 {
			sessions[n-1].Minutes += r.Minutes
			sessions[n-1].Outdoor = sessions[n-1].Outdoor || r.Kind == "outdoor"
			sessions[n-1].Records = append(sessions[n-1].Records, r.ID)
			continue
		}
		sessions = append(sessions, Session{
			Minutes: r.Minutes,
			Outdoor: r.Kind == "outdoor",
			Records: []int64{r.ID},
		})
	}
	return sessions
}

// WorkoutCredit reports the minutes to credit each of the two workout tasks.
//
// outdoor is the longest session that happened outside, and second is the
// longest session other than the longest — so the pair answers "was one of
// them outdoors" and "was there a second one at all". Reporting minutes rather
// than a yes/no keeps the progress bars honest on a part-finished day: a
// thirty-minute walk shows as thirty of forty-five, not as nothing.
func WorkoutCredit(sessions []Session) (outdoor, second int) {
	mins := make([]int, 0, len(sessions))
	for _, s := range sessions {
		mins = append(mins, s.Minutes)
		if s.Outdoor && s.Minutes > outdoor {
			outdoor = s.Minutes
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(mins)))
	if len(mins) >= 2 {
		second = mins[1]
	}
	return outdoor, second
}
