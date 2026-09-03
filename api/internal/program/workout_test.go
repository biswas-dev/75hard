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
	outdoor, second := WorkoutCredit(got)
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

	outdoor, second := WorkoutCredit(got)
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

	outdoor, second := WorkoutCredit(got)
	if outdoor != 0 {
		t.Errorf("indoor sessions credited the outdoor task: %d", outdoor)
	}
	if second != 50 {
		t.Errorf("want the second session credited, got %d", second)
	}
}

func TestWorkoutCreditSingleSessionLeavesSecondEmpty(t *testing.T) {
	got := GroupSessions([]WorkoutRecord{
		{ID: 3, Kind: "outdoor", Minutes: 58, StartedAt: at("11:34")},
	}, SessionGap)

	outdoor, second := WorkoutCredit(got)
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
