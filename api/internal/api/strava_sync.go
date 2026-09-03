package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"go.uber.org/zap"
)

// StravaSyncDefault is how often connected accounts are polled when nothing
// else is configured.
//
// Two hours is chosen against Strava's rate limit rather than against
// impatience: 100 read requests per fifteen minutes is the ceiling, and a walk
// finished at nine is still logged against the ninth long before the day ends.
const StravaSyncDefault = 2 * time.Hour

// StravaSyncMinimum is the floor on the configured interval.
//
// Below this the polling stops being a background convenience and starts
// consuming the rate limit that the manual "sync now" button depends on.
const StravaSyncMinimum = 15 * time.Minute

// StravaSyncer polls connected Strava accounts on a timer.
//
// Strava supports webhooks, which would be strictly better than polling — but
// they need a publicly reachable callback registered with the application and
// verified, and a timer needs nothing. This is the version that works today;
// a webhook can replace it without the rest of the app noticing.
type StravaSyncer struct {
	srv      *Server
	interval time.Duration
	done     chan struct{}
}

// NewStravaSyncer builds the poller. Nothing runs until Start.
func NewStravaSyncer(srv *Server, interval time.Duration) *StravaSyncer {
	if interval <= 0 {
		interval = StravaSyncDefault
	}
	if interval < StravaSyncMinimum {
		interval = StravaSyncMinimum
	}
	return &StravaSyncer{srv: srv, interval: interval, done: make(chan struct{})}
}

// Interval reports the effective polling interval.
func (s *StravaSyncer) Interval() time.Duration { return s.interval }

// Start begins polling.
func (s *StravaSyncer) Start(ctx context.Context) {
	go s.run(ctx)
}

// Stop halts polling.
func (s *StravaSyncer) Stop() {
	select {
	case <-s.done:
	default:
		close(s.done)
	}
}

func (s *StravaSyncer) run(ctx context.Context) {
	// A short delay before the first pass: at boot the database may still be
	// applying migrations, and an activity that arrived overnight can wait
	// another half minute.
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case <-timer.C:
			s.syncAll(ctx)
			timer.Reset(s.interval)
		}
	}
}

// syncAll polls every connected account.
func (s *StravaSyncer) syncAll(ctx context.Context) {
	if !s.srv.cfg.StravaEnabled() {
		return
	}

	rows, err := s.srv.db.QueryContext(ctx,
		`SELECT user_id FROM strava_accounts`)
	if err != nil {
		s.srv.log.Error("strava auto-sync: listing accounts", zap.Error(err))
		return
	}

	var users []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return
		}
		users = append(users, id)
	}
	// Release the cursor before syncing: each pass writes to the same
	// database, and holding a read cursor across that has deadlocked this
	// pool before.
	rows.Close()

	if len(users) == 0 {
		return
	}

	for _, userID := range users {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		default:
		}

		// syncStrava reads the user and their timezone from the request
		// context, the way every handler does. There is no request here, so
		// one is constructed carrying the same values — cheaper than
		// threading an alternative path through the whole sync.
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req = req.WithContext(context.WithValue(ctx, UserIDKey, userID))

		n, err := s.srv.syncStrava(req.Context(), userID, req)
		if err != nil {
			// Expected and frequent: a revoked authorisation, or a rate
			// limit. Recorded on the account by syncStrava, and logged at
			// warning rather than error because neither needs attention.
			s.srv.log.Warn("strava auto-sync failed",
				zap.Int64("user", userID), zap.Error(err))
			continue
		}
		if n > 0 {
			s.srv.log.Info("strava auto-sync imported activities",
				zap.Int64("user", userID), zap.Int("count", n))
		}
	}
}
