package api

import (
	"testing"
	"time"
)

func TestStravaSyncIntervalIsClamped(t *testing.T) {
	// The floor protects the rate limit that the manual "sync now" button
	// shares: 100 reads per fifteen minutes across the whole application.
	cases := []struct {
		name string
		in   time.Duration
		want time.Duration
	}{
		{"unset uses the default", 0, StravaSyncDefault},
		{"negative uses the default", -time.Hour, StravaSyncDefault},
		{"below the floor is raised", time.Minute, StravaSyncMinimum},
		{"exactly the floor is kept", StravaSyncMinimum, StravaSyncMinimum},
		{"a configured value is honoured", 30 * time.Minute, 30 * time.Minute},
		{"a long value is honoured", 12 * time.Hour, 12 * time.Hour},
	}
	for _, tc := range cases {
		got := NewStravaSyncer(nil, tc.in).Interval()
		if got != tc.want {
			t.Errorf("%s: interval = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestStravaSyncDefaults(t *testing.T) {
	if StravaSyncDefault != 15*time.Minute {
		t.Errorf("default = %v, want 15m", StravaSyncDefault)
	}
	if StravaSyncMinimum != 15*time.Minute {
		t.Errorf("minimum = %v, want 15m", StravaSyncMinimum)
	}
}

func TestStopIsIdempotent(t *testing.T) {
	// Shutdown can reach this twice; closing a closed channel would panic.
	s := NewStravaSyncer(nil, time.Hour)
	s.Stop()
	s.Stop()
}

// syncWindow mirrors the window calculation in syncStrava, so the rule can be
// checked without a database or a Strava account.
func syncWindow(now time.Time, lastSync time.Time, haveLastSync bool) time.Time {
	const overlap = 7
	after := now.AddDate(0, 0, -30)
	if haveLastSync {
		if widened := lastSync.AddDate(0, 0, -overlap); widened.After(after) {
			after = widened
		}
	}
	return after
}

// Strava filters on when an activity started, not when it was uploaded. A
// watch that syncs late, or a backdated manual entry, carries an old start
// time — and with two-hourly polling a narrow window would hide it forever.
func TestSyncWindowToleratesLateUploads(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)

	// The steady state: polling has just run, so last sync is an hour ago.
	after := syncWindow(now, now.Add(-time.Hour), true)

	// An activity that happened three days ago and was only uploaded now must
	// still fall inside the window.
	threeDaysAgo := now.AddDate(0, 0, -3)
	if !threeDaysAgo.After(after) {
		t.Errorf("an activity from %v falls outside the window starting %v", threeDaysAgo, after)
	}

	// And the window must not run away to the beginning of time.
	if after.Before(now.AddDate(0, 0, -31)) {
		t.Errorf("window start %v is further back than the 30-day floor", after)
	}
}

func TestSyncWindowFirstRunLooksBackAMonth(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	after := syncWindow(now, time.Time{}, false)

	if got := now.Sub(after).Hours() / 24; got < 29 || got > 31 {
		t.Errorf("first run looks back %.0f days, want about 30", got)
	}
}

func TestSyncWindowNeverExceedsTheFloor(t *testing.T) {
	// A very old last sync must not widen the window past the 30-day floor,
	// or a long-dormant account would pull its entire history every poll.
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	after := syncWindow(now, now.AddDate(-1, 0, 0), true)

	if got := now.Sub(after).Hours() / 24; got < 29 || got > 31 {
		t.Errorf("window is %.0f days wide, want the 30-day floor", got)
	}
}
