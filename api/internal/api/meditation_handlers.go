package api

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/anchoo2kewl/75hard/api/internal/program"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// Meditation is one logged sitting.
type Meditation struct {
	ID      int64  `json:"id"`
	DayID   int64  `json:"day_id"`
	Minutes int    `json:"minutes"`
	Source  string `json:"source"`
	Style   string `json:"style"`
	Notes   string `json:"notes"`
	// Reflection is a note about the sitting itself, written while closing it
	// off. Kept apart from the journal because a line about how the ten
	// minutes went and a journal entry are different kinds of writing.
	Reflection string     `json:"reflection"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// MeditationStyles are the shapes a sitting can take. Unknown values fall back
// to "guided" rather than being rejected — the detail is never worth a failed
// request on a task that cannot fail the challenge anyway.
var MeditationStyles = []string{"guided", "unguided", "breathwork", "body_scan", "walking", "other"}

type createMeditationRequest struct {
	DayNumber  *int   `json:"day_number"`
	Minutes    int    `json:"minutes"`
	Source     string `json:"source"`
	Style      string `json:"style"`
	Notes      string `json:"notes"`
	Reflection string `json:"reflection"`
	// When set, logging a sitting also ticks the meditation task, so one
	// action does both.
	TaskID *int64 `json:"task_id"`
}

// HandleCreateMeditation logs a meditation sitting.
//
// This is the optional task: it is tracked like the rest, but a day with no
// sitting is still a complete day, and refreshDayStatus below is what keeps
// that true rather than an assumption made here.
func (s *Server) HandleCreateMeditation(w http.ResponseWriter, r *http.Request) {
	var req createMeditationRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	userID := UserID(ctx)

	programID, dayID, ok := s.resolveDay(w, r, req.DayNumber)
	if !ok {
		return
	}

	if req.Minutes < 0 {
		respondError(w, http.StatusBadRequest, "minutes cannot be negative", "invalid_minutes")
		return
	}
	// A sitting nobody timed is still a sitting; only the absurd is refused.
	if req.Minutes > 24*60 {
		respondError(w, http.StatusBadRequest, "that is more than a day", "invalid_minutes")
		return
	}

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO meditation_sessions (user_id, day_id, minutes, source, style, notes, reflection)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, dayID, req.Minutes,
		strings.TrimSpace(req.Source), meditationStyle(req.Style),
		strings.TrimSpace(req.Notes), strings.TrimSpace(req.Reflection))
	if err != nil {
		s.log.Error("insert meditation", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "could not log meditation", "internal")
		return
	}
	id, _ := res.LastInsertId()

	if req.TaskID != nil {
		s.creditMeditationTask(r, programID, dayID, *req.TaskID)
	}

	m, err := s.meditationByID(ctx, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load meditation", "internal")
		return
	}
	respondJSON(w, http.StatusCreated, m)
}

// HandleDeleteMeditation removes a logged sitting.
func (s *Server) HandleDeleteMeditation(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "meditationID"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid meditation id", "invalid_id")
		return
	}
	if _, err := s.db.ExecContext(r.Context(),
		`DELETE FROM meditation_sessions WHERE id = ? AND user_id = ?`,
		id, UserID(r.Context())); err != nil {
		respondError(w, http.StatusInternalServerError, "could not delete meditation", "internal")
		return
	}
	respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// creditMeditationTask ticks the linked task, matching how a workout credits
// its own. Failures here are logged, not surfaced: the sitting is already
// saved, and an optional task failing to tick is not worth losing it over.
func (s *Server) creditMeditationTask(r *http.Request, programID, dayID, taskID int64) {
	ctx := r.Context()

	var owns int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM program_tasks WHERE id = ? AND program_id = ?`,
		taskID, programID).Scan(&owns); err != nil || owns != 1 {
		return
	}

	var total int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(minutes), 0) FROM meditation_sessions WHERE day_id = ?`,
		dayID).Scan(&total)

	var taskKind string
	var target *float64
	if err := s.db.QueryRowContext(ctx,
		`SELECT kind, target_num FROM program_tasks WHERE id = ?`, taskID).
		Scan(&taskKind, &target); err != nil {
		return
	}

	value := float64(total)
	// Required: true here asks only "does this entry meet the task's own bar",
	// which is independent of whether the task holds the day back.
	done := program.EntrySatisfies(
		program.Task{ID: taskID, Kind: taskKind, TargetNum: target, Required: true},
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
		dayID, taskID, completedAt, value); err != nil {
		s.log.Error("credit meditation task", zap.Error(err))
		return
	}
	if err := s.refreshDayStatus(r, programID, dayID); err != nil {
		s.log.Error("refresh day after meditation", zap.Error(err))
	}
}

func (s *Server) meditationByID(ctx context.Context, id int64) (Meditation, error) {
	return scanMeditation(s.db.QueryRowContext(ctx, `
		SELECT id, day_id, minutes, source, style, notes, reflection, started_at, created_at
		  FROM meditation_sessions WHERE id = ?`, id))
}

func (s *Server) meditationsForDay(ctx context.Context, dayID int64) ([]Meditation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, day_id, minutes, source, style, notes, reflection, started_at, created_at
		  FROM meditation_sessions WHERE day_id = ? ORDER BY id`, dayID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Meditation{}
	for rows.Next() {
		m, err := scanMeditation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func scanMeditation(row scanner) (Meditation, error) {
	var m Meditation
	var started sql.NullTime
	err := row.Scan(&m.ID, &m.DayID, &m.Minutes, &m.Source, &m.Style, &m.Notes,
		&m.Reflection, &started, &m.CreatedAt)
	if started.Valid {
		m.StartedAt = &started.Time
	}
	return m, err
}

func meditationStyle(style string) string {
	style = strings.ToLower(strings.TrimSpace(style))
	for _, known := range MeditationStyles {
		if style == known {
			return style
		}
	}
	return "guided"
}
