package api

import (
	"testing"

	"github.com/anchoo2kewl/75hard/api/internal/program"
)

func ended(v string) *string { return &v }

// The settings page reads current_day straight off this list. It reported day
// 0 of 75 on the third day of a run, because the scanned row carries none of
// the derived fields and nothing filled them in.
func TestCurrentDayOfCountsToToday(t *testing.T) {
	p := Program{StartDate: "2026-09-02", LengthDays: 75, Status: program.ProgramActive}
	if got := currentDayOf(p, "2026-09-04"); got != 3 {
		t.Errorf("day %d, want 3", got)
	}
}

func TestCurrentDayOfStopsAtTheEndOfTheProgram(t *testing.T) {
	p := Program{StartDate: "2026-01-01", LengthDays: 75, Status: program.ProgramActive}
	if got := currentDayOf(p, "2026-09-04"); got != 75 {
		t.Errorf("day %d, want it capped at 75", got)
	}
}

// A run abandoned on day 3 is a run that reached day 3, not one still climbing
// towards 75 for the rest of the year.
func TestCurrentDayOfCountsAFinishedAttemptToItsEnd(t *testing.T) {
	p := Program{
		StartDate:  "2026-09-02",
		LengthDays: 75,
		Status:     program.ProgramFailed,
		EndedAt:    ended("2026-09-04 11:22:33"),
	}
	if got := currentDayOf(p, "2026-12-25"); got != 3 {
		t.Errorf("day %d, want 3", got)
	}
}

func TestCurrentDayOfIsZeroBeforeTheStart(t *testing.T) {
	p := Program{StartDate: "2026-09-10", LengthDays: 75, Status: program.ProgramActive}
	if got := currentDayOf(p, "2026-09-04"); got != 0 {
		t.Errorf("day %d, want 0", got)
	}
}
