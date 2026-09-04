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

// WorkoutCredit decides which session satisfies each of the two workout tasks,
// and how many minutes to credit them.
//
// The outdoor task takes the first session outside that reaches the target,
// because "the outdoor workout" is the one you went out and did, not whichever
// happened to run longest. The second task takes the longest of what is left.
// Assigning them in that order is what makes the app's answer match the day as
// it was lived: a morning walk is workout one, and the evening swim is the
// second — not the other way round because the swim ran two minutes longer.
//
// Until a session reaches the target the longest stands in, so a part-finished
// day shows thirty of forty-five rather than nothing.
//
// The returned indices are into sessions, or -1 when nothing is credited.
func WorkoutCredit(sessions []Session, target int) (outdoor, second int, outdoorAt, secondAt int) {
	outdoorAt, secondAt = -1, -1

	for i, s := range sessions {
		if s.Outdoor && s.Minutes >= target {
			outdoorAt = i
			break
		}
	}
	if outdoorAt < 0 {
		for i, s := range sessions {
			if s.Outdoor && (outdoorAt < 0 || s.Minutes > sessions[outdoorAt].Minutes) {
				outdoorAt = i
			}
		}
	}
	if outdoorAt >= 0 {
		outdoor = sessions[outdoorAt].Minutes
	}

	for i, s := range sessions {
		if i == outdoorAt {
			continue
		}
		if secondAt < 0 || s.Minutes > sessions[secondAt].Minutes {
			secondAt = i
		}
	}
	if secondAt >= 0 {
		second = sessions[secondAt].Minutes
	}
	return outdoor, second, outdoorAt, secondAt
}
