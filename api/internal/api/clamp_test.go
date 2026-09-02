package api

import "testing"

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
