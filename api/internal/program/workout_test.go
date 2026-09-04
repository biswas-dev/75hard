package program

import (
	"testing"
	"time"
)

func at(hhmm string) *time.Time {
	t, err := time.Parse(time.RFC3339, "2026-09-03T"+hhmm+":00Z")
	if err != nil {
		panic(err)
	}
	return &t
}

func TestGroupSessionsMergesWithinTheGap(t *testing.T) {
	// The real day 1: thirty-one minutes from Strava plus a fifteen-minute
	// treadmill leg logged by hand with no time on it.
	got := GroupSessions([]WorkoutRecord{
		{ID: 2, Kind: "indoor", Minutes: 31, StartedAt: at("21:49")},
		{ID: 4, Kind: "indoor", Minutes: 15},
	}, SessionGap)

	if len(got) != 1 {
		t.Fatalf("want one session, got %d: %+v", len(got), got)
	}
	if got[0].Minutes != 46 {
		t.Errorf("want 46 minutes, got %d", got[0].Minutes)
	}
}

func TestGroupSessionsSplitsBeyondTheGap(t *testing.T) {
	got := GroupSessions([]WorkoutRecord{
		{ID: 1, Kind: "outdoor", Minutes: 51, StartedAt: at("13:08")},
		{ID: 2, Kind: "indoor", Minutes: 31, StartedAt: at("21:49")},
	}, SessionGap)

	if len(got) != 2 {
		t.Fatalf("want two sessions, got %d", len(got))
	}
	if got[0].Minutes != 51 || got[1].Minutes != 31 {
		t.Errorf("sessions not kept apart: %+v", got)
	}
}

func TestGroupSessionsShortWalksDoNotAddUp(t *testing.T) {
	// Three twenty-minute walks spread across the day are three walks, not a
	// forty-five minute workout. This is the case the old summing rule got
	// wrong.
	got := GroupSessions([]WorkoutRecord{
		{ID: 1, Kind: "outdoor", Minutes: 20, StartedAt: at("08:00")},
		{ID: 2, Kind: "outdoor", Minutes: 20, StartedAt: at("12:00")},
		{ID: 3, Kind: "outdoor", Minutes: 20, StartedAt: at("17:00")},
	}, SessionGap)

	if len(got) != 3 {
		t.Fatalf("want three sessions, got %d", len(got))
	}
	outdoor, second, _, _ := WorkoutCredit(got, 45)
	if outdoor >= 45 || second >= 45 {
		t.Errorf("three short walks satisfied a 45-minute task: outdoor=%d second=%d", outdoor, second)
	}
}

func TestWorkoutCreditTwoOutdoorSessionsBothCount(t *testing.T) {
	// The bug that started this: a morning walk and an afternoon outdoor swim
	// are two workouts, and both being outside must not leave the day short.
	got := GroupSessions([]WorkoutRecord{
		{ID: 3, Kind: "outdoor", Minutes: 58, StartedAt: at("11:34")},
		{ID: 5, Kind: "outdoor", Minutes: 45, StartedAt: at("19:00")},
	}, SessionGap)

	outdoor, second, _, _ := WorkoutCredit(got, 45)
	if outdoor != 58 {
		t.Errorf("want the longest outdoor session credited, got %d", outdoor)
	}
	if second != 45 {
		t.Errorf("want a second session credited, got %d", second)
	}
}

func TestWorkoutCreditNeedsAnOutdoorSession(t *testing.T) {
	got := GroupSessions([]WorkoutRecord{
		{ID: 1, Kind: "indoor", Minutes: 60, StartedAt: at("08:00")},
		{ID: 2, Kind: "indoor", Minutes: 50, StartedAt: at("18:00")},
	}, SessionGap)

	outdoor, second, _, _ := WorkoutCredit(got, 45)
	if outdoor != 0 {
		t.Errorf("indoor sessions credited the outdoor task: %d", outdoor)
	}
	// With no outdoor session to claim one, the second task takes the best of
	// them. The day is short on the outdoor requirement, not on this one.
	if second != 60 {
		t.Errorf("want the longest session credited, got %d", second)
	}
}

func TestWorkoutCreditSingleSessionLeavesSecondEmpty(t *testing.T) {
	got := GroupSessions([]WorkoutRecord{
		{ID: 3, Kind: "outdoor", Minutes: 58, StartedAt: at("11:34")},
	}, SessionGap)

	outdoor, second, _, _ := WorkoutCredit(got, 45)
	if outdoor != 58 {
		t.Errorf("want 58, got %d", outdoor)
	}
	if second != 0 {
		t.Errorf("one session must not credit a second workout, got %d", second)
	}
}

func TestGroupSessionsUntimedOnlyFormsOneSession(t *testing.T) {
	got := GroupSessions([]WorkoutRecord{
		{ID: 1, Kind: "outdoor", Minutes: 45},
	}, SessionGap)

	if len(got) != 1 || got[0].Minutes != 45 || !got[0].Outdoor {
		t.Fatalf("a lone hand-logged workout should stand on its own: %+v", got)
	}
}


func TestWorkoutCreditAssignsSessionsInOrder(t *testing.T) {
	// The morning walk is workout one and the evening swim the second, even
	// though the swim ran two minutes longer. Crediting by length instead put
	// the walk under "Second 45-minute workout", which read as nonsense.
	got := GroupSessions([]WorkoutRecord{
		{ID: 3, Kind: "outdoor", Minutes: 58, StartedAt: at("11:34")},
		{ID: 5, Kind: "outdoor", Minutes: 60, StartedAt: at("20:26")},
	}, SessionGap)

	outdoor, second, outdoorAt, secondAt := WorkoutCredit(got, 45)
	if outdoorAt != 0 {
		t.Errorf("outdoor task should count the morning session, got index %d", outdoorAt)
	}
	if secondAt != 1 {
		t.Errorf("second task should count the evening session, got index %d", secondAt)
	}
	if outdoor != 58 || second != 60 {
		t.Errorf("outdoor=%d second=%d, want 58 and 60", outdoor, second)
	}
}

func TestWorkoutCreditFallsBackToTheLongestOutdoor(t *testing.T) {
	// Nothing has reached the target yet; the longest outdoor session stands
	// in so the progress bar is not stuck at zero.
	got := GroupSessions([]WorkoutRecord{
		{ID: 1, Kind: "outdoor", Minutes: 20, StartedAt: at("08:00")},
		{ID: 2, Kind: "outdoor", Minutes: 30, StartedAt: at("12:00")},
	}, SessionGap)

	outdoor, _, outdoorAt, _ := WorkoutCredit(got, 45)
	if outdoor != 30 || outdoorAt != 1 {
		t.Errorf("outdoor=%d at=%d, want 30 at index 1", outdoor, outdoorAt)
	}
}
