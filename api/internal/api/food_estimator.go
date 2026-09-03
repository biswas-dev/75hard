package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/anchoo2kewl/75hard/api/internal/aifeatures"
	"go.uber.org/zap"
)

// estimateJob is one queued food photo awaiting a nutrition estimate.
type estimateJob struct {
	userID  int64
	mealID  int64
	photoID int64
	relPath string
	hint    string
}

// FoodEstimator runs food-photo estimates off the request path.
//
// Photographing a meal should cost one tap. A vision model routinely takes
// most of a minute, and making the user hold the app open for it is the
// difference between a feature they use and one they stop using — so the meal
// row is saved immediately and the numbers arrive afterwards.
//
// The queue is deliberately small and the worker count deliberately low: these
// are paid upstream calls, and an unbounded goroutine per upload would turn a
// burst of photos into a burst of spend.
type FoodEstimator struct {
	srv     *Server
	jobs    chan estimateJob
	workers int
	done    chan struct{}
	// timeout bounds one estimate. Generous, because go-ai retries a
	// struggling provider before falling through to the next.
	timeout time.Duration
}

// NewFoodEstimator builds the background estimator. Nothing runs until Start.
func NewFoodEstimator(srv *Server, workers, queue int) *FoodEstimator {
	if workers < 1 {
		workers = 2
	}
	if queue < 1 {
		queue = 64
	}
	return &FoodEstimator{
		srv:     srv,
		jobs:    make(chan estimateJob, queue),
		workers: workers,
		done:    make(chan struct{}),
		timeout: aiCallTimeout,
	}
}

// Start launches the workers and requeues anything left pending by a restart.
func (e *FoodEstimator) Start(ctx context.Context) {
	for i := 0; i < e.workers; i++ {
		go e.run(ctx)
	}
	go e.resume(ctx)
}

// Stop closes the queue and waits briefly for in-flight work.
//
// It does not wait for the upstream call to finish: an estimate is worth a few
// seconds of shutdown at most, and anything unfinished is left 'pending' and
// picked up by resume on the next boot.
func (e *FoodEstimator) Stop() {
	select {
	case <-e.done:
		return // already stopped
	default:
		close(e.done)
	}
}

// Enqueue submits a job, reporting whether it was accepted.
//
// A full queue does not block the upload. The meal keeps its 'pending' status
// and the next restart's resume sweep collects it — a late estimate is a much
// better failure than a stalled request.
func (e *FoodEstimator) Enqueue(job estimateJob) bool {
	select {
	case e.jobs <- job:
		return true
	default:
		e.srv.log.Warn("food estimate queue is full; leaving it pending",
			zap.Int64("meal_id", job.mealID))
		return false
	}
}

func (e *FoodEstimator) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.done:
			return
		case job := <-e.jobs:
			e.process(ctx, job)
		}
	}
}

// resume re-queues meals left pending when the process last stopped, so a
// deploy mid-estimate does not strand a photo with no numbers forever.
func (e *FoodEstimator) resume(ctx context.Context) {
	rows, err := e.srv.db.QueryContext(ctx, `
		SELECT m.id, m.user_id, m.photo_id, p.rel_path, m.name
		  FROM meals m
		  JOIN photos p ON p.id = m.photo_id
		 WHERE m.estimate_status = 'pending'
		 ORDER BY m.id
		 LIMIT 100`)
	if err != nil {
		e.srv.log.Error("resume food estimates", zap.Error(err))
		return
	}
	defer rows.Close()

	var pending []estimateJob
	for rows.Next() {
		var j estimateJob
		if err := rows.Scan(&j.mealID, &j.userID, &j.photoID, &j.relPath, &j.hint); err != nil {
			e.srv.log.Error("scan pending estimate", zap.Error(err))
			return
		}
		pending = append(pending, j)
	}
	// Release the cursor before enqueueing: a worker may write to the same
	// database, and holding a read cursor across that is how the pool
	// deadlocked before.
	rows.Close()

	for _, j := range pending {
		if !e.Enqueue(j) {
			return
		}
	}
	if len(pending) > 0 {
		e.srv.log.Info("requeued food estimates left by a restart", zap.Int("count", len(pending)))
	}
}

func (e *FoodEstimator) process(ctx context.Context, job estimateJob) {
	s := e.srv

	// Detached from the caller: the upload response went out long ago, and a
	// shutdown should not cancel a call that has already been paid for.
	callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), e.timeout)
	defer cancel()

	f, err := s.photos.Open(job.relPath)
	if err != nil {
		e.fail(callCtx, job.mealID, "the photo file could not be read")
		return
	}
	image, err := readAll(f)
	f.Close()
	if err != nil {
		e.fail(callCtx, job.mealID, "the photo file could not be read")
		return
	}

	// Quota is checked here rather than at upload: the photo is saved either
	// way, and only the estimate is rationed.
	if used, err := s.aiCallsToday(callCtx, job.userID); err == nil && used >= DailyAILimit {
		e.fail(callCtx, job.mealID, "daily AI limit reached; retry the estimate tomorrow")
		return
	}

	var estimate aifeatures.FoodEstimate
	hash := hashFor(image, job.hint)
	if s.aiCacheGet(callCtx, job.userID, aifeatures.FeatureFoodPhoto, hash, &estimate) {
		e.apply(callCtx, job, &estimate)
		return
	}

	result, meta, err := s.aiForUser(callCtx, job.userID).EstimateFood(callCtx, image, "image/jpeg", job.hint)
	s.recordAIRun(callCtx, job.userID, aifeatures.FeatureFoodPhoto, meta, err)
	if err != nil {
		s.log.Warn("food estimate failed",
			zap.Int64("meal_id", job.mealID), zap.Error(err))
		e.fail(callCtx, job.mealID, summariseAIError(err))
		return
	}
	e.apply(callCtx, job, result)
}

// apply writes the estimate onto the meal and its items, in one transaction so
// a meal is never left with totals that disagree with its items.
func (e *FoodEstimator) apply(ctx context.Context, job estimateJob, est *aifeatures.FoodEstimate) {
	s := e.srv

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.log.Error("begin estimate tx", zap.Error(err))
		return
	}
	defer tx.Rollback()

	name := strings.TrimSpace(est.Name)
	if name == "" {
		name = "Meal from photo"
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE meals
		   SET name = CASE WHEN name = '' THEN ? ELSE name END,
		       kcal = ?, protein_g = ?, carbs_g = ?, fat_g = ?,
		       notes = CASE WHEN notes = '' THEN ? ELSE notes END,
		       estimate_status = 'done', estimate_error = '',
		       updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		name, est.Kcal, est.ProteinG, est.CarbsG, est.FatG,
		strings.TrimSpace(est.Notes), job.mealID); err != nil {
		s.log.Error("apply estimate", zap.Error(err))
		return
	}

	// Replace rather than append, so a retried estimate does not double the
	// item list.
	if _, err := tx.ExecContext(ctx, `DELETE FROM meal_items WHERE meal_id = ?`, job.mealID); err != nil {
		s.log.Error("clear meal items", zap.Error(err))
		return
	}
	for i, item := range est.Items {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO meal_items (meal_id, name, qty, unit, kcal, protein_g, carbs_g, fat_g, confidence, sort_order)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			job.mealID, item.Name, item.Qty, item.Unit,
			item.Kcal, item.ProteinG, item.CarbsG, item.FatG, item.Confidence, i); err != nil {
			s.log.Error("insert estimated item", zap.Error(err))
			return
		}
	}

	if err := tx.Commit(); err != nil {
		s.log.Error("commit estimate", zap.Error(err))
		return
	}
	s.log.Info("food estimate applied",
		zap.Int64("meal_id", job.mealID), zap.Float64("kcal", est.Kcal))
}

// fail records why an estimate did not happen, leaving the meal in place so
// the person can still edit it by hand.
func (e *FoodEstimator) fail(ctx context.Context, mealID int64, reason string) {
	if len(reason) > 300 {
		reason = reason[:300]
	}
	if _, err := e.srv.db.ExecContext(ctx, `
		UPDATE meals SET estimate_status = 'failed', estimate_error = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`, reason, mealID); err != nil {
		e.srv.log.Error("mark estimate failed", zap.Error(err))
	}
}

// summariseAIError keeps the stored reason short and free of provider detail
// that means nothing to the person reading it.
func summariseAIError(err error) string {
	msg := err.Error()
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return msg
}

// HandleRetryEstimate re-queues an estimate that failed.
//
// Worth an endpoint because the most common failure is the daily AI limit,
// which clears on its own — the photo is already saved, and the person should
// not have to re-take it to get the numbers.
func (s *Server) HandleRetryEstimate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "mealID"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid meal id", "invalid_id")
		return
	}
	ctx := r.Context()
	userID := UserID(ctx)

	if s.food == nil || !s.aiForUser(ctx, userID).Enabled() {
		respondError(w, http.StatusServiceUnavailable,
			"add an AI provider key in settings to use this", "ai_no_key")
		return
	}

	var job estimateJob
	err = s.db.QueryRowContext(ctx, `
		SELECT m.id, m.user_id, m.photo_id, p.rel_path, m.notes
		  FROM meals m
		  JOIN photos p ON p.id = m.photo_id
		 WHERE m.id = ? AND m.user_id = ?`, id, userID).
		Scan(&job.mealID, &job.userID, &job.photoID, &job.relPath, &job.hint)
	if errors.Is(err, sql.ErrNoRows) {
		respondError(w, http.StatusNotFound, "no photo-backed meal with that id", "not_found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load meal", "internal")
		return
	}

	if _, err := s.db.ExecContext(ctx, `
		UPDATE meals SET estimate_status = 'pending', estimate_error = '',
		                 updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`, id); err != nil {
		respondError(w, http.StatusInternalServerError, "could not queue the estimate", "internal")
		return
	}

	if !s.food.Enqueue(job) {
		// Still pending, so the next restart's sweep will collect it.
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
		return
	}
	respondJSON(w, http.StatusAccepted, map[string]string{"status": "pending"})
}
