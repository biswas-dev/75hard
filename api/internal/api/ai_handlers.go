package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/anchoo2kewl/75hard/api/internal/aifeatures"
	"github.com/anchoo2kewl/75hard/api/internal/program"
	"go.uber.org/zap"
)

// aiCallTimeout bounds the upstream model call. Detached from the request
// context so a proxy giving up early cannot cancel a call already paid for,
// but still bounded so a hung provider does not leak a goroutine forever.
const aiCallTimeout = 3 * time.Minute

// AISlotTimeout bounds one provider. Deliberately well under aiCallTimeout:
// go-ai holds a reserve back for untried providers, but a per-slot timeout
// close to the whole budget leaves that reserve nothing to work with.
const AISlotTimeout = 45 * time.Second

// DailyAILimit caps model calls per user per rolling 24 hours. Generous for
// real use, low enough that a runaway client cannot spend the key.
const DailyAILimit = 40

// HandleAIStatus reports whether the AI features are available, so the SPA can
// hide the buttons rather than offering something that will fail.
func (s *Server) HandleAIStatus(w http.ResponseWriter, r *http.Request) {
	used, _ := s.aiCallsToday(r.Context(), UserID(r.Context()))
	respondJSON(w, http.StatusOK, map[string]any{
		"enabled":   s.ai.Enabled(),
		"providers": s.ai.Providers(),
		// Reported separately because photo analysis can run on a different
		// chain than the text features.
		"vision_providers": s.ai.VisionProviders(),
		"used_today":       used,
		"daily_limit":      DailyAILimit,
	})
}

type analyzeFoodRequest struct {
	PhotoID int64  `json:"photo_id"`
	Hint    string `json:"hint"`
}

// HandleAnalyzeFood estimates a meal from an already-uploaded food photo.
//
// It returns the estimate rather than saving it: an AI figure should be
// reviewed before it becomes a logged meal, so the client shows it in the meal
// sheet for editing and the user decides.
func (s *Server) HandleAnalyzeFood(w http.ResponseWriter, r *http.Request) {
	var req analyzeFoodRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	userID := UserID(ctx)

	if !s.aiReady(w) {
		return
	}

	var relPath string
	if err := s.db.QueryRowContext(ctx,
		`SELECT rel_path FROM photos WHERE id = ? AND user_id = ?`,
		req.PhotoID, userID).Scan(&relPath); err != nil {
		respondError(w, http.StatusNotFound, "photo not found", "not_found")
		return
	}

	f, err := s.photos.Open(relPath)
	if err != nil {
		respondError(w, http.StatusNotFound, "photo file missing", "not_found")
		return
	}
	defer f.Close()

	// The stored image is already downscaled to 1600px, which is both plenty
	// for identification and far cheaper than a full-resolution original.
	image, err := readAll(f)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not read photo", "internal")
		return
	}

	var cached aifeatures.FoodEstimate
	hash := hashFor(image, req.Hint)
	if s.aiCacheGet(ctx, userID, aifeatures.FeatureFoodPhoto, hash, &cached) {
		respondJSON(w, http.StatusOK, map[string]any{"estimate": cached, "cached": true})
		return
	}

	if !s.aiWithinQuota(w, ctx, userID) {
		return
	}

	// Detached from the request context on purpose: an edge proxy that gives
	// up at 30s would otherwise cancel a call the user has already paid for.
	callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), aiCallTimeout)
	defer cancel()

	estimate, meta, err := s.ai.EstimateFood(callCtx, image, "image/jpeg", req.Hint)
	if err != nil {
		s.recordAIRun(ctx, userID, aifeatures.FeatureFoodPhoto, meta, err)
		s.respondAIError(w, err)
		return
	}
	s.recordAIRun(ctx, userID, aifeatures.FeatureFoodPhoto, meta, nil)

	respondJSON(w, http.StatusOK, map[string]any{
		"estimate": estimate,
		"cached":   false,
		"provider": meta.Provider,
		"model":    meta.Model,
	})
}

type recipesRequest struct {
	Ingredients []string `json:"ingredients"`
	Preferences string   `json:"preferences"`
	MealSlot    string   `json:"meal_slot"`
	PhotoID     *int64   `json:"photo_id"`
}

// HandleSuggestRecipes proposes meals that fit what is left of today's budget.
func (s *Server) HandleSuggestRecipes(w http.ResponseWriter, r *http.Request) {
	var req recipesRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	userID := UserID(ctx)

	if !s.aiReady(w) {
		return
	}

	// Work out what is actually left today, so a suggestion fits the day
	// rather than being generic.
	air := aifeatures.RecipeRequest{
		Ingredients: req.Ingredients,
		Preferences: req.Preferences,
		MealSlot:    req.MealSlot,
	}
	if programID, err := s.activeProgramID(r); err == nil {
		var startDate string
		var target *int
		if err := s.db.QueryRowContext(ctx,
			`SELECT start_date, daily_kcal_target FROM programs WHERE id = ?`, programID).
			Scan(&startDate, &target); err == nil {
			today := program.LocalDate(time.Now(), s.userLocation(r))
			if dayID, err := s.ensureDay(ctx, programID, startDate, today); err == nil {
				var consumed, protein float64
				_ = s.db.QueryRowContext(ctx,
					`SELECT COALESCE(SUM(kcal), 0), COALESCE(SUM(protein_g), 0) FROM meals WHERE day_id = ?`,
					dayID).Scan(&consumed, &protein)
				if target != nil {
					air.RemainingKcal = float64(*target) - consumed
					if air.RemainingKcal < 0 {
						air.RemainingKcal = 0
					}
					// A rough protein floor of 1.6g per kg is not knowable
					// without a bodyweight, so key off the calorie target
					// instead: 30% of calories from protein.
					air.RemainingProtein = (float64(*target) * 0.3 / 4) - protein
					if air.RemainingProtein < 0 {
						air.RemainingProtein = 0
					}
				}
			}
		}
	}

	if req.PhotoID != nil {
		var relPath string
		if err := s.db.QueryRowContext(ctx,
			`SELECT rel_path FROM photos WHERE id = ? AND user_id = ?`,
			*req.PhotoID, userID).Scan(&relPath); err == nil {
			if f, err := s.photos.Open(relPath); err == nil {
				defer f.Close()
				if image, err := readAll(f); err == nil {
					air.Image = image
					air.MediaType = "image/jpeg"
				}
			}
		}
	}

	if !s.aiWithinQuota(w, ctx, userID) {
		return
	}

	callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), aiCallTimeout)
	defer cancel()

	recipes, meta, err := s.ai.SuggestRecipes(callCtx, air)
	if err != nil {
		s.recordAIRun(ctx, userID, aifeatures.FeatureRecipes, meta, err)
		s.respondAIError(w, err)
		return
	}
	s.recordAIRun(ctx, userID, aifeatures.FeatureRecipes, meta, nil)

	respondJSON(w, http.StatusOK, map[string]any{
		"recipes":  recipes,
		"provider": meta.Provider,
		"model":    meta.Model,
	})
}

type planRequest struct {
	Goals string `json:"goals"`
	// Force skips the cache, for a user who wants a different week.
	Force bool `json:"force"`
}

// HandleBuildPlan writes a week of training and nutrition guidance from the
// user's own logged record.
func (s *Server) HandleBuildPlan(w http.ResponseWriter, r *http.Request) {
	var req planRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	userID := UserID(ctx)

	if !s.aiReady(w) {
		return
	}

	history, ok := s.buildHistory(w, r, req.Goals)
	if !ok {
		return
	}

	hash := hashFor(nil, historyKey(history))
	if !req.Force {
		var cached aifeatures.Plan
		if s.aiCacheGet(ctx, userID, aifeatures.FeaturePlan, hash, &cached) {
			respondJSON(w, http.StatusOK, map[string]any{"plan": cached, "cached": true})
			return
		}
	}

	if !s.aiWithinQuota(w, ctx, userID) {
		return
	}

	callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), aiCallTimeout)
	defer cancel()

	plan, meta, err := s.ai.BuildPlan(callCtx, history)
	if err != nil {
		s.recordAIRun(ctx, userID, aifeatures.FeaturePlan, meta, err)
		s.respondAIError(w, err)
		return
	}
	s.recordAIRun(ctx, userID, aifeatures.FeaturePlan, meta, nil)

	respondJSON(w, http.StatusOK, map[string]any{
		"plan":     plan,
		"cached":   false,
		"provider": meta.Provider,
		"model":    meta.Model,
	})
}

// HandleCoachNote returns a short note about where the user stands today.
func (s *Server) HandleCoachNote(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := UserID(ctx)

	if !s.aiReady(w) {
		return
	}

	history, ok := s.buildHistory(w, r, "")
	if !ok {
		return
	}

	// Cached on the history fingerprint, so opening the app repeatedly on an
	// unchanged day costs nothing.
	hash := hashFor(nil, historyKey(history))
	var cached aifeatures.CoachNote
	if s.aiCacheGet(ctx, userID, aifeatures.FeatureCoach, hash, &cached) {
		respondJSON(w, http.StatusOK, map[string]any{"note": cached, "cached": true})
		return
	}

	if !s.aiWithinQuota(w, ctx, userID) {
		return
	}

	callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), aiCallTimeout)
	defer cancel()

	note, meta, err := s.ai.DailyNote(callCtx, history)
	if err != nil {
		s.recordAIRun(ctx, userID, aifeatures.FeatureCoach, meta, err)
		s.respondAIError(w, err)
		return
	}
	s.recordAIRun(ctx, userID, aifeatures.FeatureCoach, meta, nil)

	respondJSON(w, http.StatusOK, map[string]any{"note": note, "cached": false})
}

// ---- helpers ----

func (s *Server) aiReady(w http.ResponseWriter) bool {
	if !s.ai.Enabled() {
		respondError(w, http.StatusServiceUnavailable,
			"AI features are not configured on this server", "ai_disabled")
		return false
	}
	return true
}

func (s *Server) respondAIError(w http.ResponseWriter, err error) {
	if errors.Is(err, aifeatures.ErrDisabled) {
		respondError(w, http.StatusServiceUnavailable, "AI features are not configured", "ai_disabled")
		return
	}
	s.log.Error("ai call failed", zap.Error(err))
	// 502: the failure is upstream, not in the caller's request.
	respondError(w, http.StatusBadGateway,
		"the AI provider could not complete that request", "ai_failed")
}

// aiCallsToday counts a user's model calls in the trailing 24 hours.
func (s *Server) aiCallsToday(ctx context.Context, userID int64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM ai_runs WHERE user_id = ? AND created_at > datetime('now', '-1 day')`,
		userID).Scan(&n)
	return n, err
}

func (s *Server) aiWithinQuota(w http.ResponseWriter, ctx context.Context, userID int64) bool {
	used, err := s.aiCallsToday(ctx, userID)
	if err != nil {
		s.log.Error("ai quota check", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "could not check the AI quota", "internal")
		return false
	}
	if used >= DailyAILimit {
		respondError(w, http.StatusTooManyRequests,
			"you have reached today's AI limit; it resets on a rolling 24-hour window", "ai_quota")
		return false
	}
	return true
}

// aiCacheGet returns a previous successful result for identical inputs.
func (s *Server) aiCacheGet(ctx context.Context, userID int64, feature, hash string, out any) bool {
	var body string
	err := s.db.QueryRowContext(ctx, `
		SELECT result_json FROM ai_runs
		WHERE user_id = ? AND feature = ? AND input_hash = ? AND error = '' AND result_json != ''
		ORDER BY created_at DESC LIMIT 1`, userID, feature, hash).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) || err != nil || body == "" {
		return false
	}
	return json.Unmarshal([]byte(body), out) == nil
}

// recordAIRun writes the ledger entry. It is both the audit trail and the
// quota counter, so a failed call is recorded too — a provider erroring out
// still consumed an attempt.
func (s *Server) recordAIRun(ctx context.Context, userID int64, feature string, meta aifeatures.Meta, callErr error) {
	var msg string
	if callErr != nil {
		msg = callErr.Error()
		if len(msg) > 500 {
			msg = msg[:500]
		}
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO ai_runs (user_id, feature, provider, model, input_hash, result_json,
		                     tokens_in, tokens_out, attempts, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, feature, meta.Provider, meta.Model, meta.InputHash, meta.ResultJSON,
		meta.TokensIn, meta.TokensOut, maxInt(meta.Attempts, 1), msg); err != nil {
		s.log.Error("record ai run", zap.Error(err))
	}
}

// buildHistory gathers the user's record for the plan and coaching prompts.
func (s *Server) buildHistory(w http.ResponseWriter, r *http.Request, goals string) (aifeatures.History, bool) {
	ctx := r.Context()

	programID, err := s.activeProgramID(r)
	if err != nil {
		respondError(w, http.StatusNotFound, "no active program", "no_active_program")
		return aifeatures.History{}, false
	}

	h := aifeatures.History{TaskRates: map[string]float64{}, Goals: goals}

	var startDate string
	var target *int
	if err := s.db.QueryRowContext(ctx,
		`SELECT start_date, length_days, daily_kcal_target FROM programs WHERE id = ?`, programID).
		Scan(&startDate, &h.LengthDays, &target); err != nil {
		respondError(w, http.StatusInternalServerError, "could not load program", "internal")
		return aifeatures.History{}, false
	}
	if target != nil {
		h.KcalTarget = *target
	}

	h.CurrentDay = program.DayNumber(startDate, program.LocalDate(time.Now(), s.userLocation(r)))
	if h.CurrentDay > h.LengthDays {
		h.CurrentDay = h.LengthDays
	}

	_ = s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(status = 'complete'), 0),
			COALESCE(SUM(status = 'missed'), 0)
		FROM days WHERE program_id = ?`, programID).Scan(&h.DaysComplete, &h.DaysMissed)

	statuses := map[int]string{}
	if rows, err := s.db.QueryContext(ctx,
		`SELECT day_number, status FROM days WHERE program_id = ?`, programID); err == nil {
		for rows.Next() {
			var n int
			var st string
			if err := rows.Scan(&n, &st); err == nil {
				statuses[n] = st
			}
		}
		rows.Close()
	}
	h.Streak = program.Streak(statuses, h.CurrentDay)

	_ = s.db.QueryRowContext(ctx, `
		SELECT COALESCE(AVG(day_kcal), 0) FROM (
			SELECT SUM(kcal) AS day_kcal FROM meals
			WHERE day_id IN (SELECT id FROM days WHERE program_id = ?) GROUP BY day_id
		)`, programID).Scan(&h.AvgKcal)

	_ = s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(minutes), 0) FROM workouts
		WHERE day_id IN (SELECT id FROM days WHERE program_id = ?)`, programID).Scan(&h.TotalMinutes)

	elapsed := h.CurrentDay
	if elapsed < 1 {
		elapsed = 1
	}
	if rows, err := s.db.QueryContext(ctx, `
		SELECT pt.title,
		       (SELECT COUNT(*) FROM task_entries te
		        JOIN days d ON d.id = te.day_id
		        WHERE te.program_task_id = pt.id AND te.completed_at IS NOT NULL AND d.program_id = ?)
		     + (CASE WHEN pt.kind = 'photo' THEN
		            (SELECT COUNT(*) FROM days d2
		             WHERE d2.program_id = ?
		               AND EXISTS (SELECT 1 FROM photos p WHERE p.day_id = d2.id AND p.kind = 'progress')
		               AND NOT EXISTS (SELECT 1 FROM task_entries te2
		                               WHERE te2.day_id = d2.id AND te2.program_task_id = pt.id
		                                 AND te2.completed_at IS NOT NULL))
		        ELSE 0 END)
		FROM program_tasks pt WHERE pt.program_id = ?`, programID, programID, programID); err == nil {
		for rows.Next() {
			var title string
			var done int
			if err := rows.Scan(&title, &done); err == nil {
				rate := float64(done) / float64(elapsed) * 100
				if rate > 100 {
					rate = 100
				}
				h.TaskRates[title] = rate
			}
		}
		rows.Close()
	}

	var first, last *float64
	_ = s.db.QueryRowContext(ctx,
		`SELECT (SELECT weight_kg FROM days WHERE program_id = ? AND weight_kg IS NOT NULL ORDER BY day_number LIMIT 1),
		        (SELECT weight_kg FROM days WHERE program_id = ? AND weight_kg IS NOT NULL ORDER BY day_number DESC LIMIT 1)`,
		programID, programID).Scan(&first, &last)
	if first != nil && last != nil {
		h.WeightChangeKg = *last - *first
	}

	return h, true
}

// historyKey is the cache key for a history: the same record on the same day
// should not be re-analysed.
func historyKey(h aifeatures.History) string {
	var b strings.Builder
	b.WriteString(strconv.Itoa(h.CurrentDay))
	b.WriteByte('|')
	b.WriteString(strconv.Itoa(h.DaysComplete))
	b.WriteByte('|')
	b.WriteString(strconv.Itoa(h.DaysMissed))
	b.WriteByte('|')
	b.WriteString(strconv.Itoa(h.Streak))
	b.WriteByte('|')
	b.WriteString(strconv.Itoa(h.TotalMinutes))
	b.WriteByte('|')
	b.WriteString(strconv.FormatFloat(h.AvgKcal, 'f', 0, 64))
	b.WriteByte('|')
	b.WriteString(h.Goals)
	return b.String()
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
