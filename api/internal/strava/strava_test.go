package strava

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		act  Activity
		want string
	}{
		// The motivating case: a 50-minute walk is outdoors.
		{"a plain walk", Activity{SportType: "Walk", Type: "Walk"}, "outdoor"},
		{"a run", Activity{SportType: "Run", Type: "Run"}, "outdoor"},
		{"a ride", Activity{SportType: "Ride", Type: "Ride"}, "outdoor"},

		// The trainer flag is the athlete saying "this was stationary".
		{"treadmill run", Activity{SportType: "Run", Type: "Run", Trainer: true}, "indoor"},
		{"turbo ride", Activity{SportType: "Ride", Type: "Ride", Trainer: true}, "indoor"},

		// Gym sports are never outdoors.
		{"weights", Activity{SportType: "WeightTraining", Type: "WeightTraining"}, "indoor"},
		{"yoga", Activity{SportType: "Yoga", Type: "Yoga"}, "indoor"},
		{"zwift", Activity{SportType: "VirtualRide", Type: "VirtualRide"}, "indoor"},

		// An unambiguously outdoor sport outranks a mis-set trainer flag,
		// because you cannot hike on a treadmill.
		{"hike with a stray trainer flag", Activity{SportType: "Hike", Type: "Hike", Trainer: true}, "outdoor"},
		{"trail run", Activity{SportType: "TrailRun", Type: "Run"}, "outdoor"},

		// An unrecognised sport falls to outdoor: it is the common case, and
		// the classification is editable afterwards.
		{"a sport we have never seen", Activity{SportType: "Pickleball", Type: "Pickleball"}, "outdoor"},
		// But Strava's generic "Workout" type is its gym marker, so it stays
		// indoor even when the sport name is unfamiliar.
		{"generic gym workout", Activity{SportType: "Pickleball", Type: "Workout"}, "indoor"},
	}

	for _, tc := range tests {
		if got := Classify(tc.act); got != tc.want {
			t.Errorf("%s: Classify = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestLocalDateUsesTheAthletesOwnClock(t *testing.T) {
	// Strava puts a Z on start_date_local that is a lie: the time is already
	// shifted into the athlete's timezone. Reading it as UTC would push an
	// evening session onto the next day and log the walk against the wrong
	// day of the challenge.
	a := Activity{
		StartDateLocal: "2026-09-02T19:30:00Z", // 19:30 local
		StartDate:      "2026-09-02T23:30:00Z", // 23:30 UTC, same instant
	}
	if got := a.LocalDate(); got != "2026-09-02" {
		t.Errorf("LocalDate = %q, want 2026-09-02", got)
	}

	// And the late case that actually crosses midnight in UTC.
	b := Activity{
		StartDateLocal: "2026-09-02T21:00:00Z",
		StartDate:      "2026-09-03T01:00:00Z",
	}
	if got := b.LocalDate(); got != "2026-09-02" {
		t.Errorf("LocalDate = %q, want the local day 2026-09-02, not the UTC day", got)
	}
}

func TestTokenExpired(t *testing.T) {
	if !(Token{ExpiresAt: 0}).Expired() {
		t.Error("a token with no expiry must be treated as expired")
	}
	if !(Token{ExpiresAt: time.Now().Add(-time.Hour).Unix()}).Expired() {
		t.Error("a past expiry should be expired")
	}
	// Slack: a token expiring in 20 seconds must refresh before it is used.
	if !(Token{ExpiresAt: time.Now().Add(20 * time.Second).Unix()}).Expired() {
		t.Error("a token expiring within the slack window should be refreshed")
	}
	if (Token{ExpiresAt: time.Now().Add(time.Hour).Unix()}).Expired() {
		t.Error("an hour of life left is not expired")
	}
}

func TestExchangeAndActivities(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"at","refresh_token":"rt","expires_at":4102444800,
				"scope":"read,activity:read_all","athlete":{"id":42,"username":"anshuman"}}`))
		case "/athlete/activities":
			if got := r.Header.Get("Authorization"); got != "Bearer at" {
				t.Errorf("Authorization = %q", got)
			}
			if r.URL.Query().Get("after") == "" {
				t.Error("after was not sent; a full re-sync every time is wasteful")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id":9,"name":"Evening walk","type":"Walk","sport_type":"Walk",
				"moving_time":3000,"elapsed_time":3100,"distance":4200.5,
				"average_heartrate":102.4,"max_heartrate":128,
				"start_date":"2026-09-02T23:30:00Z","start_date_local":"2026-09-02T19:30:00Z"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New("id", "secret")
	c.BaseURL = srv.URL
	c.TokenEndpoint = srv.URL + "/oauth/token"

	tok, err := c.Exchange(context.Background(), "code")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if tok.AccessToken != "at" || tok.Athlete.ID != 42 {
		t.Errorf("token = %+v", tok)
	}

	acts, err := c.Activities(context.Background(), tok.AccessToken, time.Unix(1_700_000_000, 0), 30)
	if err != nil {
		t.Fatalf("Activities: %v", err)
	}
	if len(acts) != 1 {
		t.Fatalf("got %d activities", len(acts))
	}
	a := acts[0]
	if a.MovingTime != 3000 || a.Name != "Evening walk" {
		t.Errorf("activity = %+v", a)
	}
	if a.AverageHR == nil || *a.AverageHR != 102.4 {
		t.Errorf("average HR not decoded: %+v", a.AverageHR)
	}
	if Classify(a) != "outdoor" {
		t.Error("an evening walk should classify as outdoor")
	}
	if a.LocalDate() != "2026-09-02" {
		t.Errorf("LocalDate = %q", a.LocalDate())
	}
}

func TestActivitiesSurfacesRevokedAuth(t *testing.T) {
	// A revoked authorisation must be distinguishable, so the UI can ask the
	// athlete to reconnect rather than reporting a generic failure forever.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := New("id", "secret")
	c.BaseURL = srv.URL
	if _, err := c.Activities(context.Background(), "stale", time.Time{}, 10); err != ErrUnauthorized {
		t.Errorf("got %v, want ErrUnauthorized", err)
	}
}

func TestNotConfigured(t *testing.T) {
	// No credentials is a valid state: the feature is simply off.
	if (&Client{}).Configured() {
		t.Error("an empty client reported itself configured")
	}
	if !New("id", "secret").Configured() {
		t.Error("a client with credentials should be configured")
	}
}

func TestSessionMinutesUsesElapsedForSwims(t *testing.T) {
	// The real activity: forty-five minutes in the pool, of which Strava
	// counted thirty-two as moving because the rest at the wall is not
	// swimming. It is still a forty-five minute workout.
	if got := SessionMinutes("Swim", "Swim", 32*60, 45*60); got != 45 {
		t.Errorf("swim = %d minutes, want 45", got)
	}
}

func TestSessionMinutesKeepsMovingTimeForGPSSports(t *testing.T) {
	// A walk with a long stop for coffee is not a longer walk.
	for _, sport := range []string{"Walk", "Run", "Ride", "Hike"} {
		if got := SessionMinutes(sport, sport, 30*60, 90*60); got != 30 {
			t.Errorf("%s = %d minutes, want 30", sport, got)
		}
	}
}

func TestSessionMinutesIgnoresAForgottenStop(t *testing.T) {
	// Six hours elapsed against thirty minutes moving is a watch left
	// running, not six hours of rest between sets.
	if got := SessionMinutes("WeightTraining", "", 30*60, 360*60); got != 30 {
		t.Errorf("got %d minutes, want the moving time of 30", got)
	}
}

func TestSessionMinutesNeverShortensASession(t *testing.T) {
	// Elapsed below moving is nonsense; moving stands.
	if got := SessionMinutes("Swim", "Swim", 40*60, 10*60); got != 40 {
		t.Errorf("got %d minutes, want 40", got)
	}
}
