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
	if StravaSyncDefault != 2*time.Hour {
		t.Errorf("default = %v, want 2h", StravaSyncDefault)
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
