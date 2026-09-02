package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/anchoo2kewl/75hard/api/internal/program"
	"github.com/anchoo2kewl/75hard/api/internal/strava"
	"go.uber.org/zap"
)

// StravaStatus is what the settings page renders.
type StravaStatus struct {
	// Configured is false when the server has no Strava application at all.
	Configured bool       `json:"configured"`
	Connected  bool       `json:"connected"`
	Athlete    string     `json:"athlete,omitempty"`
	AthleteID  int64      `json:"athlete_id,omitempty"`
	LastSyncAt *time.Time `json:"last_sync_at,omitempty"`
	LastError  string     `json:"last_error,omitempty"`
	Activities int        `json:"activities"`
}

// HandleStravaStatus reports whether Strava is configured and connected.
func (s *Server) HandleStravaStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := UserID(ctx)

	out := StravaStatus{Configured: s.cfg.StravaEnabled()}

	var lastSync sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT athlete_id, username, last_sync_at, last_error
		  FROM strava_accounts WHERE user_id = ?`, userID).
		Scan(&out.AthleteID, &out.Athlete, &lastSync, &out.LastError)
	if err == nil {
		out.Connected = true
		if lastSync.Valid {
			out.LastSyncAt = &lastSync.Time
		}
		_ = s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM strava_activities WHERE user_id = ?`, userID).Scan(&out.Activities)
	} else if !errors.Is(err, sql.ErrNoRows) {
		s.log.Error("strava status", zap.Error(err))
	}

	respondJSON(w, http.StatusOK, out)
}

// HandleStravaConnect starts the OAuth flow.
//
// The state parameter is an HMAC over the user id rather than a stored nonce:
// the callback has to identify the user without a session cookie (Strava sends
// the browser back on a plain redirect, and the app authenticates with a bearer
// token that will not be present), and signing it means the callback can trust
// the id without a lookup table.
func (s *Server) HandleStravaConnect(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.StravaEnabled() {
		respondError(w, http.StatusServiceUnavailable,
			"Strava is not configured on this server", "strava_disabled")
		return
	}
	userID := UserID(r.Context())

	state, err := s.signStravaState(userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not start the connection", "internal")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"url": s.stravaClient().AuthorizeURL(s.cfg.StravaRedirectURL, state),
	})
}

// HandleStravaCallback completes the OAuth flow.
//
// This is a browser redirect, not an API call, so it always ends in a redirect
// rather than a JSON error — there is nothing on the other end to render one.
func (s *Server) HandleStravaCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	fail := func(reason string, err error) {
		if err != nil {
			s.log.Warn("strava callback failed", zap.String("reason", reason), zap.Error(err))
		}
		http.Redirect(w, r, s.cfg.StravaErrorURL, http.StatusFound)
	}

	if e := q.Get("error"); e != "" {
		// The athlete pressed cancel. Not an error worth logging loudly.
		fail("denied", nil)
		return
	}

	userID, ok := s.verifyStravaState(q.Get("state"))
	if !ok {
		fail("bad state", nil)
		return
	}

	code := q.Get("code")
	if code == "" {
		fail("no code", nil)
		return
	}

	// Strava grants scopes individually; without activity:read the connection
	// is useless, so refuse it now rather than importing nothing forever.
	if scope := q.Get("scope"); scope != "" && !strings.Contains(scope, "activity:read") {
		fail("insufficient scope", fmt.Errorf("granted scope %q", scope))
		return
	}

	tok, err := s.stravaClient().Exchange(ctx, code)
	if err != nil {
		fail("exchange", err)
		return
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO strava_accounts (user_id, athlete_id, username, access_token, refresh_token, expires_at, scope)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			athlete_id    = excluded.athlete_id,
			username      = excluded.username,
			access_token  = excluded.access_token,
			refresh_token = excluded.refresh_token,
			expires_at    = excluded.expires_at,
			scope         = excluded.scope,
			last_error    = '',
			updated_at    = CURRENT_TIMESTAMP`,
		userID, tok.Athlete.ID, tok.Athlete.Username,
		tok.AccessToken, tok.RefreshToken, tok.ExpiresAt, tok.Scope); err != nil {
		fail("store", err)
		return
	}

	// The sync path reads the user from the request context, the way every
	// authenticated handler does. This callback arrives without a token — the
	// user came from the signed state — so the context has to be populated
	// here or the program lookup silently finds nothing and imports land
	// against no day at all.
	r = r.WithContext(context.WithValue(ctx, UserIDKey, userID))
	ctx = r.Context()

	// Pull straight away so the athlete sees their activities rather than an
	// empty connection they have to work out how to populate.
	if n, err := s.syncStrava(ctx, userID, r); err != nil {
		s.log.Warn("first strava sync failed", zap.Error(err))
	} else {
		s.log.Info("strava connected", zap.Int64("user", userID), zap.Int("imported", n))
	}

	http.Redirect(w, r, s.cfg.StravaSuccessURL, http.StatusFound)
}

// HandleStravaDisconnect forgets the account and its imported activities.
//
// Workouts already created are left alone: they are part of the person's
// record, and deleting them could un-complete finished days.
func (s *Server) HandleStravaDisconnect(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := UserID(ctx)

	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM strava_activities WHERE user_id = ?`, userID); err != nil {
		respondError(w, http.StatusInternalServerError, "could not disconnect", "internal")
		return
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM strava_accounts WHERE user_id = ?`, userID); err != nil {
		respondError(w, http.StatusInternalServerError, "could not disconnect", "internal")
		return
	}
	respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// HandleStravaSync pulls recent activities on demand.
func (s *Server) HandleStravaSync(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := UserID(ctx)

	if !s.cfg.StravaEnabled() {
		respondError(w, http.StatusServiceUnavailable,
			"Strava is not configured on this server", "strava_disabled")
		return
	}

	n, err := s.syncStrava(ctx, userID, r)
	if errors.Is(err, sql.ErrNoRows) {
		respondError(w, http.StatusBadRequest, "no Strava account is connected", "not_connected")
		return
	}
	if errors.Is(err, strava.ErrUnauthorized) {
		respondError(w, http.StatusBadRequest,
			"Strava access was revoked; reconnect the account", "strava_reauth")
		return
	}
	if err != nil {
		s.log.Error("strava sync", zap.Error(err))
		respondError(w, http.StatusBadGateway, "could not reach Strava", "strava_error")
		return
	}
	respondJSON(w, http.StatusOK, map[string]int{"imported": n})
}

// syncStrava pulls activities and reconciles them against the program.
//
// Returns how many activities were imported or updated.
func (s *Server) syncStrava(ctx context.Context, userID int64, r *http.Request) (int, error) {
	var (
		accessToken, refreshToken string
		expiresAt                 int64
		lastSync                  sql.NullTime
	)
	if err := s.db.QueryRowContext(ctx, `
		SELECT access_token, refresh_token, expires_at, last_sync_at
		  FROM strava_accounts WHERE user_id = ?`, userID).
		Scan(&accessToken, &refreshToken, &expiresAt, &lastSync); err != nil {
		return 0, err
	}

	client := s.stravaClient()

	// Refresh first if the token is spent. Strava may rotate the refresh
	// token, so whatever comes back is what gets stored.
	if (strava.Token{ExpiresAt: expiresAt}).Expired() {
		tok, err := client.Refresh(ctx, refreshToken)
		if err != nil {
			s.noteStravaError(ctx, userID, err)
			return 0, err
		}
		accessToken = tok.AccessToken
		if _, err := s.db.ExecContext(ctx, `
			UPDATE strava_accounts
			   SET access_token = ?, refresh_token = ?, expires_at = ?, updated_at = CURRENT_TIMESTAMP
			 WHERE user_id = ?`,
			tok.AccessToken, tok.RefreshToken, tok.ExpiresAt, userID); err != nil {
			return 0, err
		}
	}

	// Overlap the window by a day: activities are often edited after upload,
	// and re-reading one is cheap because the upsert updates in place.
	after := time.Now().AddDate(0, 0, -30)
	if lastSync.Valid && lastSync.Time.After(after) {
		after = lastSync.Time.AddDate(0, 0, -1)
	}

	activities, err := client.Activities(ctx, accessToken, after, 100)
	if err != nil {
		s.noteStravaError(ctx, userID, err)
		return 0, err
	}

	programID, startDate, length, ok := s.activeProgramWindow(ctx, r)

	imported := 0
	for _, a := range activities {
		if err := s.storeStravaActivity(ctx, r, userID, a, programID, startDate, length, ok); err != nil {
			s.log.Error("store strava activity",
				zap.Int64("strava_id", a.ID), zap.Error(err))
			continue
		}
		imported++
	}

	if _, err := s.db.ExecContext(ctx, `
		UPDATE strava_accounts SET last_sync_at = CURRENT_TIMESTAMP, last_error = '',
		                           updated_at = CURRENT_TIMESTAMP
		 WHERE user_id = ?`, userID); err != nil {
		s.log.Error("mark strava sync", zap.Error(err))
	}
	return imported, nil
}

// storeStravaActivity upserts one activity and credits the matching task.
func (s *Server) storeStravaActivity(
	ctx context.Context, r *http.Request, userID int64, a strava.Activity,
	programID int64, startDate string, length int, haveProgram bool,
) error {
	kind := strava.Classify(a)
	localDate := a.LocalDate()

	// Map onto a day of the program, when the activity falls inside one.
	var dayID *int64
	if haveProgram && localDate != "" {
		if n := program.DayNumber(startDate, localDate); n >= 1 && n <= length {
			if id, err := s.ensureDay(ctx, programID, startDate, localDate); err == nil {
				dayID = &id
			}
		}
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO strava_activities (
			user_id, strava_id, day_id, name, sport_type, kind, trainer, commute,
			moving_seconds, elapsed_seconds, distance_m, elevation_m, kcal,
			average_hr, max_hr, average_speed, start_at, local_date)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, strava_id) DO UPDATE SET
			day_id          = excluded.day_id,
			name            = excluded.name,
			sport_type      = excluded.sport_type,
			-- kind is not overwritten: a correction made in the app must
			-- survive a re-sync, or fixing a misclassified session is futile.
			trainer         = excluded.trainer,
			commute         = excluded.commute,
			moving_seconds  = excluded.moving_seconds,
			elapsed_seconds = excluded.elapsed_seconds,
			distance_m      = excluded.distance_m,
			elevation_m     = excluded.elevation_m,
			kcal            = excluded.kcal,
			average_hr      = excluded.average_hr,
			max_hr          = excluded.max_hr,
			average_speed   = excluded.average_speed,
			start_at        = excluded.start_at,
			local_date      = excluded.local_date,
			updated_at      = CURRENT_TIMESTAMP`,
		userID, a.ID, dayID, a.Name, a.SportType, kind, boolInt(a.Trainer), boolInt(a.Commute),
		a.MovingTime, a.ElapsedTime, a.Distance, a.TotalElevation, a.Calories,
		a.AverageHR, a.MaxHR, a.AverageSpeed, a.StartTime(), localDate); err != nil {
		return err
	}

	if dayID != nil && haveProgram {
		return s.creditStravaActivity(ctx, r, userID, a.ID, programID, *dayID)
	}
	return nil
}

// creditStravaActivity turns an imported activity into a workout and ticks the
// task it satisfies.
//
// The workout row is the bridge: the app already sums workout minutes per kind
// to decide whether a duration task is met, so writing one here means a Strava
// import completes a day through exactly the same path a manual entry does.
func (s *Server) creditStravaActivity(
	ctx context.Context, r *http.Request, userID, stravaID, programID, dayID int64,
) error {
	var (
		rowID   int64
		kind    string
		name    string
		minutes int
		kcal    *float64
		workout sql.NullInt64
	)
	if err := s.db.QueryRowContext(ctx, `
		SELECT id, kind, name, moving_seconds / 60, kcal, workout_id
		  FROM strava_activities WHERE user_id = ? AND strava_id = ?`,
		userID, stravaID).Scan(&rowID, &kind, &name, &minutes, &kcal, &workout); err != nil {
		return err
	}

	if workout.Valid {
		// Already credited; keep the workout in step with any edit made on
		// Strava rather than creating a second one.
		_, err := s.db.ExecContext(ctx, `
			UPDATE workouts SET kind = ?, activity = ?, minutes = ?, kcal = ?,
			                    updated_at = CURRENT_TIMESTAMP
			 WHERE id = ? AND user_id = ?`,
			kind, name, minutes, kcal, workout.Int64, userID)
		if err == nil {
			s.tickWorkoutTasks(ctx, r, programID, dayID)
		}
		return err
	}

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO workouts (user_id, day_id, kind, activity, minutes, kcal, notes)
		VALUES (?, ?, ?, ?, ?, ?, 'Imported from Strava')`,
		userID, dayID, kind, name, minutes, kcal)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	if _, err := s.db.ExecContext(ctx,
		`UPDATE strava_activities SET workout_id = ? WHERE id = ?`, id, rowID); err != nil {
		return err
	}

	s.tickWorkoutTasks(ctx, r, programID, dayID)
	return nil
}

// tickWorkoutTasks re-evaluates every duration task with a workout tracker
// against the minutes now logged for its kind.
//
// Doing it for both kinds at once, from the totals, is what makes two short
// sessions add up and what keeps an import idempotent: running it twice
// produces the same answer.
func (s *Server) tickWorkoutTasks(ctx context.Context, r *http.Request, programID, dayID int64) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, task_key, kind, target_num FROM program_tasks
		 WHERE program_id = ? AND tracker = 'workout'`, programID)
	if err != nil {
		s.log.Error("load workout tasks", zap.Error(err))
		return
	}

	type task struct {
		id     int64
		key    string
		kind   string
		target *float64
	}
	var tasks []task
	for rows.Next() {
		var t task
		if err := rows.Scan(&t.id, &t.key, &t.kind, &t.target); err != nil {
			rows.Close()
			return
		}
		tasks = append(tasks, t)
	}
	// Release the cursor before the writes below; holding it across them is
	// how this pool has deadlocked before.
	rows.Close()

	for _, t := range tasks {
		// An outdoor task is fed by outdoor minutes; everything else indoor.
		wantKind := "indoor"
		if strings.Contains(t.key, "outdoor") {
			wantKind = "outdoor"
		}

		var total int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COALESCE(SUM(minutes), 0) FROM workouts WHERE day_id = ? AND kind = ?`,
			dayID, wantKind).Scan(&total); err != nil {
			continue
		}
		if total == 0 {
			continue
		}

		value := float64(total)
		done := program.EntrySatisfies(
			program.Task{ID: t.id, Kind: t.kind, TargetNum: t.target, Required: true},
			program.Entry{ValueNum: &value, Completed: true})

		var completedAt any
		if done {
			completedAt = time.Now().UTC()
		}
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO task_entries (day_id, program_task_id, completed_at, value_num)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(day_id, program_task_id) DO UPDATE SET
				completed_at = excluded.completed_at,
				value_num    = excluded.value_num,
				updated_at   = CURRENT_TIMESTAMP`,
			dayID, t.id, completedAt, value); err != nil {
			s.log.Error("credit strava task", zap.Error(err))
		}
	}

	if err := s.refreshDayStatus(r, programID, dayID); err != nil {
		s.log.Error("refresh day after strava import", zap.Error(err))
	}
}

// activeProgramWindow returns the active program's id, start date and length.
func (s *Server) activeProgramWindow(ctx context.Context, r *http.Request) (int64, string, int, bool) {
	pid, err := s.activeProgramID(r)
	if err != nil {
		return 0, "", 0, false
	}
	var startDate string
	var length int
	if err := s.db.QueryRowContext(ctx,
		`SELECT start_date, length_days FROM programs WHERE id = ?`, pid).
		Scan(&startDate, &length); err != nil {
		return 0, "", 0, false
	}
	return pid, startDate, length, true
}

func (s *Server) noteStravaError(ctx context.Context, userID int64, err error) {
	msg := err.Error()
	if len(msg) > 300 {
		msg = msg[:300]
	}
	if _, e := s.db.ExecContext(ctx,
		`UPDATE strava_accounts SET last_error = ?, updated_at = CURRENT_TIMESTAMP WHERE user_id = ?`,
		msg, userID); e != nil {
		s.log.Error("note strava error", zap.Error(e))
	}
}

// stravaClient builds a client for the configured application.
func (s *Server) stravaClient() *strava.Client {
	c := strava.New(s.cfg.StravaClientID, s.cfg.StravaClientSecret)
	c.BaseURL = s.cfg.StravaAPIBase
	c.TokenEndpoint = s.cfg.StravaTokenURL
	return c
}

// ---- signed state ----

// signStravaState returns "<userID>.<hmac>", which the callback can verify
// without a stored nonce.
func (s *Server) signStravaState(userID int64) (string, error) {
	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	payload := fmt.Sprintf("%d:%s", userID, hex.EncodeToString(nonce))
	mac := hmac.New(sha256.New, []byte(s.stravaStateKey()))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// verifyStravaState checks the signature and returns the user id.
func (s *Server) verifyStravaState(state string) (int64, bool) {
	parts := strings.SplitN(state, ".", 2)
	if len(parts) != 2 {
		return 0, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return 0, false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, false
	}

	mac := hmac.New(sha256.New, []byte(s.stravaStateKey()))
	mac.Write(payload)
	// Constant time: a leaky comparison here would let someone forge a state
	// naming another user's id and attach their Strava account to it.
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return 0, false
	}

	var userID int64
	if _, err := fmt.Sscanf(string(payload), "%d:", &userID); err != nil || userID <= 0 {
		return 0, false
	}
	return userID, true
}

// stravaStateKey prefers the dedicated OAuth secret, falling back to the JWT
// secret so the flow works on an instance that never set one.
func (s *Server) stravaStateKey() string {
	if s.cfg.OAuthStateSecret != "" {
		return s.cfg.OAuthStateSecret
	}
	return s.cfg.JWTSecret
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
