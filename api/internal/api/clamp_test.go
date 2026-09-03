package api

import (
	"testing"
	"time"
)

// A program scheduled to start tomorrow used to 500 the Today screen: the
// local date was before the start, DayNumber returned 0, and ensureDay refused
// to create the row. Clamping is what keeps that a normal state.
func TestClampToProgram(t *testing.T) {
	const start = "2026-09-02"
	const length = 75
	// Day 75 of a program starting 2026-09-02.
	const lastDay = "2026-11-15"

	tests := []struct {
		name  string
		today string
		want  string
	}{
		{"the day before it starts shows day 1", "2026-09-01", start},
		{"a week before it starts still shows day 1", "2026-08-26", start},
		{"the start date itself is unchanged", start, start},
		{"a day inside the program is unchanged", "2026-10-01", "2026-10-01"},
		{"the final day is unchanged", lastDay, lastDay},
		{"past the end pins to the final day", "2026-12-25", lastDay},
	}

	for _, tc := range tests {
		if got := clampToProgram(start, length, tc.today); got != tc.want {
			t.Errorf("%s: clampToProgram(%q) = %q, want %q", tc.name, tc.today, got, tc.want)
		}
	}
}

func TestClampToProgramWithNoLength(t *testing.T) {
	// A zero length must not pin every date to a nonsense day.
	if got := clampToProgram("2026-09-02", 0, "2026-10-01"); got != "2026-10-01" {
		t.Errorf("got %q, want the date unchanged when length is 0", got)
	}
}

// A day that has not happened has nothing to record, and creating a row for one
// leaves an orphan in the calendar and the counts. A stray day 37 dated three
// weeks ahead is what prompted this.
func TestFutureDayBound(t *testing.T) {
	// The bound the plain ensureDay uses: UTC tomorrow. Timezones span
	// UTC-12 to UTC+14, so a local date is never more than one day ahead of
	// UTC's — which means this never refuses somebody's genuine today.
	bound := time.Now().AddDate(0, 0, 1).UTC().Format("2006-01-02")

	// Everywhere on earth, "today" is at or before the bound.
	for _, offset := range []int{-12, -5, 0, 1, 8, 14} {
		local := time.Now().UTC().Add(time.Duration(offset) * time.Hour).Format("2006-01-02")
		if local > bound {
			t.Errorf("UTC%+d local date %s exceeds the bound %s; a real today would be refused",
				offset, local, bound)
		}
	}

	// A day genuinely in the future is beyond it.
	future := time.Now().AddDate(0, 0, 30).UTC().Format("2006-01-02")
	if !(future > bound) {
		t.Errorf("a date 30 days out (%s) is not beyond the bound %s", future, bound)
	}
}
