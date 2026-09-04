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

// Meal is a logged eating occasion, with its itemised breakdown.
type Meal struct {
	ID       int64      `json:"id"`
	DayID    int64      `json:"day_id"`
	PhotoID  *int64     `json:"photo_id"`
	PhotoURL string     `json:"photo_url,omitempty"`
	Name     string     `json:"name"`
	Slot     string     `json:"slot"`
	Kcal     float64    `json:"kcal"`
	ProteinG float64    `json:"protein_g"`
	CarbsG   float64    `json:"carbs_g"`
	FatG     float64    `json:"fat_g"`
	Source   string     `json:"source"`
	Notes    string     `json:"notes"`
	EatenAt  string     `json:"eaten_at"`
	Items    []MealItem `json:"items"`
	// EstimateStatus is '' for a hand-entered meal, or pending/done/failed
	// while a photo is being estimated in the background. The client needs it
	// to tell "no calories yet" from "a zero-calorie meal".
	EstimateStatus string `json:"estimate_status"`
	EstimateError  string `json:"estimate_error,omitempty"`
}

// MealItem is one component of a meal.
type MealItem struct {
	ID         int64    `json:"id"`
	Name       string   `json:"name"`
	Qty        float64  `json:"qty"`
	Unit       string   `json:"unit"`
	Kcal       float64  `json:"kcal"`
	ProteinG   float64  `json:"protein_g"`
	CarbsG     float64  `json:"carbs_g"`
	FatG       float64  `json:"fat_g"`
	Confidence *float64 `json:"confidence"`
}

// Workout is a logged training session.
type Workout struct {
	ID        int64    `json:"id"`
	DayID     int64    `json:"day_id"`
	Kind      string   `json:"kind"`
	Activity  string   `json:"activity"`
	Minutes   int      `json:"minutes"`
	Kcal      *float64 `json:"kcal"`
	Notes     string   `json:"notes"`
	CreatedAt string   `json:"created_at"`
	// StartedAt is empty for a hand-logged entry with no time on it.
	StartedAt string `json:"started_at,omitempty"`
	// Session is the 1-based workout this record belongs to, so the UI can
	// show which effort each row is part of rather than one flat list.
	Session int `json:"session"`
}

type mealItemPayload struct {
	Name       string   `json:"name"`
	Qty        *float64 `json:"qty"`
	Unit       string   `json:"unit"`
	Kcal       float64  `json:"kcal"`
	ProteinG   float64  `json:"protein_g"`
	CarbsG     float64  `json:"carbs_g"`
	FatG       float64  `json:"fat_g"`
	Confidence *float64 `json:"confidence"`
}

type createMealRequest struct {
	DayNumber *int              `json:"day_number"`
	PhotoID   *int64            `json:"photo_id"`
	Name      string            `json:"name"`
	Slot      string            `json:"slot"`
	Kcal      *float64          `json:"kcal"`
	ProteinG  *float64          `json:"protein_g"`
	CarbsG    *float64          `json:"carbs_g"`
	FatG      *float64          `json:"fat_g"`
	Source    string            `json:"source"`
	Notes     string            `json:"notes"`
	Items     []mealItemPayload `json:"items"`
}

// HandleCreateMeal logs a meal against a day of the active program.
//
// When items are supplied, the meal's totals are summed from them rather than
// trusted from the client, so the breakdown and the total can never disagree.
func (s *Server) HandleCreateMeal(w http.ResponseWriter, r *http.Request) {
	var req createMealRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	userID := UserID(ctx)

	programID, dayID, ok := s.resolveDay(w, r, req.DayNumber)
	if !ok {
		return
	}
	_ = programID

	slot := strings.TrimSpace(req.Slot)
	if !validSlot(slot) {
		slot = "snack"
	}
	source := "manual"
	if req.Source == "ai" {
		source = "ai"
	}

	var kcal, protein, carbs, fat float64
	if len(req.Items) > 0 {
		for _, it := range req.Items {
			kcal += it.Kcal
			protein += it.ProteinG
			carbs += it.CarbsG
			fat += it.FatG
		}
	} else {
		kcal = derefFloat(req.Kcal)
		protein = derefFloat(req.ProteinG)
		carbs = derefFloat(req.CarbsG)
		fat = derefFloat(req.FatG)
	}

	// A photo attached to a meal must belong to the caller.
	if req.PhotoID != nil {
		var owner int64
		if err := s.db.QueryRowContext(ctx,
			`SELECT user_id FROM photos WHERE id = ?`, *req.PhotoID).Scan(&owner); err != nil || owner != userID {
			respondError(w, http.StatusNotFound, "photo not found", "not_found")
			return
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not log meal", "internal")
		return
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.ExecContext(ctx, `
		INSERT INTO meals (user_id, day_id, photo_id, name, slot, kcal, protein_g, carbs_g, fat_g, source, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, dayID, req.PhotoID, strings.TrimSpace(req.Name), slot,
		kcal, protein, carbs, fat, source, strings.TrimSpace(req.Notes))
	if err != nil {
		s.log.Error("insert meal", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "could not log meal", "internal")
		return
	}
	mealID, _ := res.LastInsertId()

	for i, it := range req.Items {
		qty := 1.0
		if it.Qty != nil {
			qty = *it.Qty
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO meal_items (meal_id, name, qty, unit, kcal, protein_g, carbs_g, fat_g, confidence, sort_order)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			mealID, strings.TrimSpace(it.Name), qty, defaultString(it.Unit, "serving"),
			it.Kcal, it.ProteinG, it.CarbsG, it.FatG, it.Confidence, i); err != nil {
			respondError(w, http.StatusInternalServerError, "could not log meal items", "internal")
			return
		}
	}

	if err := tx.Commit(); err != nil {
		respondError(w, http.StatusInternalServerError, "could not log meal", "internal")
		return
	}

	meal, err := s.mealByID(ctx, mealID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load meal", "internal")
		return
	}
	respondJSON(w, http.StatusCreated, meal)
}

type updateMealRequest struct {
	Name     *string  `json:"name"`
	Slot     *string  `json:"slot"`
	Kcal     *float64 `json:"kcal"`
	ProteinG *float64 `json:"protein_g"`
	CarbsG   *float64 `json:"carbs_g"`
	FatG     *float64 `json:"fat_g"`
	Notes    *string  `json:"notes"`
	// PhotoID attaches or replaces the photo behind the meal.
	PhotoID *int64 `json:"photo_id"`
	// Items replaces the breakdown wholesale when present.
	//
	// Editing a meal is most often re-running an estimate on it, and an
	// estimate arrives as a list of components. Without this the totals would
	// update while the breakdown underneath them stayed as it was, which is
	// worse than having no breakdown at all.
	Items []mealItemPayload `json:"items"`
	// DayNumber is accepted and ignored: the client sends one body for both
	// create and update, and a meal cannot change the day it was eaten on.
	DayNumber *int `json:"day_number"`
}

// HandleUpdateMeal edits a logged meal, e.g. correcting an AI estimate.
func (s *Server) HandleUpdateMeal(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "mealID"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid meal id", "invalid_id")
		return
	}
	var req updateMealRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	sets := []string{}
	args := []any{}
	if req.Name != nil {
		sets, args = append(sets, "name = ?"), append(args, strings.TrimSpace(*req.Name))
	}
	if req.Slot != nil && validSlot(*req.Slot) {
		sets, args = append(sets, "slot = ?"), append(args, *req.Slot)
	}
	if req.Kcal != nil {
		sets, args = append(sets, "kcal = ?"), append(args, *req.Kcal)
	}
	if req.ProteinG != nil {
		sets, args = append(sets, "protein_g = ?"), append(args, *req.ProteinG)
	}
	if req.CarbsG != nil {
		sets, args = append(sets, "carbs_g = ?"), append(args, *req.CarbsG)
	}
	if req.FatG != nil {
		sets, args = append(sets, "fat_g = ?"), append(args, *req.FatG)
	}
	if req.Notes != nil {
		sets, args = append(sets, "notes = ?"), append(args, strings.TrimSpace(*req.Notes))
	}
	if req.PhotoID != nil {
		sets, args = append(sets, "photo_id = ?"), append(args, *req.PhotoID)
	}
	// Totals derived from an itemised estimate, so the two cannot disagree.
	if req.Items != nil {
		var kcal, protein, carbs, fat float64
		for _, it := range req.Items {
			kcal += it.Kcal
			protein += it.ProteinG
			carbs += it.CarbsG
			fat += it.FatG
		}
		if req.Kcal == nil {
			sets, args = append(sets, "kcal = ?"), append(args, kcal)
			sets, args = append(sets, "protein_g = ?"), append(args, protein)
			sets, args = append(sets, "carbs_g = ?"), append(args, carbs)
			sets, args = append(sets, "fat_g = ?"), append(args, fat)
		}
	}
	if len(sets) == 0 && req.Items == nil {
		respondError(w, http.StatusBadRequest, "nothing to update", "no_changes")
		return
	}
	// A hand-edited meal is no longer an AI estimate.
	sets = append(sets, "source = 'manual'", "updated_at = CURRENT_TIMESTAMP")

	args = append(args, id, UserID(r.Context()))
	if _, err := s.db.ExecContext(r.Context(),
		`UPDATE meals SET `+strings.Join(sets, ", ")+` WHERE id = ? AND user_id = ?`, args...); err != nil {
		respondError(w, http.StatusInternalServerError, "could not update meal", "internal")
		return
	}

	// Replace the breakdown rather than appending, so a re-run estimate does
	// not double the list.
	if req.Items != nil {
		if _, err := s.db.ExecContext(r.Context(),
			`DELETE FROM meal_items WHERE meal_id = ?`, id); err != nil {
			respondError(w, http.StatusInternalServerError, "could not update meal", "internal")
			return
		}
		for i, it := range req.Items {
			if strings.TrimSpace(it.Name) == "" {
				continue
			}
			// Qty is optional; a nil pointer would store NULL rather than
			// falling back to the column default, so it is resolved here the
			// same way the create path does.
			qty := 1.0
			if it.Qty != nil {
				qty = *it.Qty
			}
			unit := it.Unit
			if strings.TrimSpace(unit) == "" {
				unit = "serving"
			}
			if _, err := s.db.ExecContext(r.Context(), `
				INSERT INTO meal_items (meal_id, name, qty, unit, kcal, protein_g, carbs_g, fat_g, sort_order)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				id, strings.TrimSpace(it.Name), qty, unit,
				it.Kcal, it.ProteinG, it.CarbsG, it.FatG, i); err != nil {
				respondError(w, http.StatusInternalServerError, "could not update meal", "internal")
				return
			}
		}
	}

	meal, err := s.mealByID(r.Context(), id)
	if err != nil {
		respondError(w, http.StatusNotFound, "meal not found", "not_found")
		return
	}
	respondJSON(w, http.StatusOK, meal)
}

// HandleDeleteMeal removes a logged meal.
func (s *Server) HandleDeleteMeal(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "mealID"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid meal id", "invalid_id")
		return
	}
	if _, err := s.db.ExecContext(r.Context(),
		`DELETE FROM meals WHERE id = ? AND user_id = ?`, id, UserID(r.Context())); err != nil {
		respondError(w, http.StatusInternalServerError, "could not delete meal", "internal")
		return
	}
	respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type createWorkoutRequest struct {
	DayNumber *int     `json:"day_number"`
	Kind      string   `json:"kind"`
	Activity  string   `json:"activity"`
	Minutes   int      `json:"minutes"`
	Kcal      *float64 `json:"kcal"`
	Notes     string   `json:"notes"`
	// When set, ticking this workout also credits the named task, so logging
	// a 45-minute outdoor session completes the outdoor task in one action.
	TaskID *int64 `json:"task_id"`
	// StartedAt decides which session a workout belongs to. Optional: the
	// server stamps now for an entry against today, and leaves it unset for
	// an earlier day rather than inventing an hour.
	StartedAt *time.Time `json:"started_at"`
}

// HandleCreateWorkout logs a training session.
func (s *Server) HandleCreateWorkout(w http.ResponseWriter, r *http.Request) {
	var req createWorkoutRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	userID := UserID(ctx)

	programID, dayID, ok := s.resolveDay(w, r, req.DayNumber)
	if !ok {
		return
	}

	kind := "indoor"
	if req.Kind == "outdoor" {
		kind = "outdoor"
	}
	if req.Minutes < 0 {
		respondError(w, http.StatusBadRequest, "minutes cannot be negative", "invalid_minutes")
		return
	}

	// Stamp the time only when the entry is for today.
	//
	// A workout logged against an earlier day happened at an hour nobody has
	// recorded, and "now" would be a lie that puts it in the wrong session.
	// A record with no time joins the day's most recent session instead,
	// which is where minutes added by hand almost always belong.
	var startedAt any
	if req.StartedAt != nil {
		startedAt = req.StartedAt.UTC()
	} else if s.dayIsToday(ctx, dayID) {
		startedAt = time.Now().UTC()
	}

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO workouts (user_id, day_id, kind, activity, minutes, kcal, notes, started_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, dayID, kind, strings.TrimSpace(req.Activity), req.Minutes, req.Kcal,
		strings.TrimSpace(req.Notes), startedAt)
	if err != nil {
		s.log.Error("insert workout", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "could not log workout", "internal")
		return
	}
	id, _ := res.LastInsertId()

	// Credit the day's workout tasks from its sessions.
	//
	// This runs whether or not a task was named: the two tasks are decided by
	// the shape of the whole day — the longest outdoor session, and the
	// longest session after the longest — so crediting only the task the
	// caller pointed at would leave the other one stale.
	s.tickWorkoutTasks(ctx, r, programID, dayID)
	{
		if err := s.refreshDayStatus(r, programID, dayID); err != nil {
			s.log.Error("refresh day after workout", zap.Error(err))
		}
	}

	wo, err := s.workoutByID(ctx, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load workout", "internal")
		return
	}
	respondJSON(w, http.StatusCreated, wo)
}

// HandleDeleteWorkout removes a logged session.
func (s *Server) HandleDeleteWorkout(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "workoutID"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid workout id", "invalid_id")
		return
	}
	if _, err := s.db.ExecContext(r.Context(),
		`DELETE FROM workouts WHERE id = ? AND user_id = ?`, id, UserID(r.Context())); err != nil {
		respondError(w, http.StatusInternalServerError, "could not delete workout", "internal")
		return
	}
	respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- helpers ----

// resolveDay maps an optional day number on the active program to a day row,
// defaulting to today.
func (s *Server) resolveDay(w http.ResponseWriter, r *http.Request, dayNumber *int) (programID, dayID int64, ok bool) {
	programID, err := s.activeProgramID(r)
	if err != nil {
		respondError(w, http.StatusNotFound, "no active program", "no_active_program")
		return 0, 0, false
	}

	var startDate string
	var length int
	if err := s.db.QueryRowContext(r.Context(),
		`SELECT start_date, length_days FROM programs WHERE id = ?`, programID).
		Scan(&startDate, &length); err != nil {
		respondError(w, http.StatusInternalServerError, "could not load program", "internal")
		return 0, 0, false
	}

	// Clamped for the same reason as the Today screen: logging a meal the
	// evening before a program starts should land on day 1, not fail.
	onDate := clampToProgram(startDate, length, program.LocalDate(time.Now(), s.userLocation(r)))
	if dayNumber != nil && *dayNumber >= 1 {
		onDate = program.DateForDay(startDate, *dayNumber)
	}

	dayID, err = s.ensureDay(r.Context(), programID, startDate, onDate)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not resolve day", "internal")
		return 0, 0, false
	}
	return programID, dayID, true
}

func (s *Server) mealByID(ctx context.Context, id int64) (Meal, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, day_id, photo_id, name, slot, kcal, protein_g, carbs_g, fat_g, source, notes, eaten_at,
		       estimate_status, estimate_error
		FROM meals WHERE id = ?`, id)
	m, err := scanMeal(row)
	if err != nil {
		return m, err
	}
	m.Items, err = s.mealItems(ctx, id)
	return m, err
}

func (s *Server) mealsForDay(ctx context.Context, dayID int64) ([]Meal, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, day_id, photo_id, name, slot, kcal, protein_g, carbs_g, fat_g, source, notes, eaten_at,
		       estimate_status, estimate_error
		FROM meals WHERE day_id = ? ORDER BY eaten_at, id`, dayID)
	if err != nil {
		return nil, err
	}

	out := []Meal{}
	for rows.Next() {
		m, err := scanMeal(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, m)
	}
	err = rows.Err()
	// Close before loading items: the per-meal queries below must not run
	// while this cursor still holds a connection.
	rows.Close()
	if err != nil {
		return nil, err
	}

	for i := range out {
		items, err := s.mealItems(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Items = items
	}
	return out, nil
}

func (s *Server) mealItems(ctx context.Context, mealID int64) ([]MealItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, qty, unit, kcal, protein_g, carbs_g, fat_g, confidence
		FROM meal_items WHERE meal_id = ? ORDER BY sort_order, id`, mealID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []MealItem{}
	for rows.Next() {
		var it MealItem
		if err := rows.Scan(&it.ID, &it.Name, &it.Qty, &it.Unit, &it.Kcal,
			&it.ProteinG, &it.CarbsG, &it.FatG, &it.Confidence); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Server) workoutByID(ctx context.Context, id int64) (Workout, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, day_id, kind, activity, minutes, kcal, notes, created_at FROM workouts WHERE id = ?`, id)
	return scanWorkout(row)
}

func (s *Server) workoutsForDay(ctx context.Context, dayID int64) ([]Workout, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, day_id, kind, activity, minutes, kcal, notes, created_at, started_at
		 FROM workouts WHERE day_id = ? ORDER BY created_at, id`, dayID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Workout{}
	for rows.Next() {
		wo, err := scanWorkout(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, wo)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	numberSessions(out)
	return out, nil
}

// parseStoredTime reads the timestamp forms SQLite hands back here: Go's own
// rendering of a time.Time, which carries a " +0000 UTC" suffix, and the plain
// form a SQL default writes.
func parseStoredTime(v string) (time.Time, bool) {
	v = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), "+0000 UTC"))
	for _, layout := range []string{"2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05", time.RFC3339} {
		if t, err := time.Parse(layout, v); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// numberSessions stamps each record with the workout it belongs to, using the
// same grouping the server credits the tasks from — so what the app shows and
// what the day is scored on cannot drift apart.
func numberSessions(out []Workout) {
	recs := make([]program.WorkoutRecord, 0, len(out))
	for _, wo := range out {
		rec := program.WorkoutRecord{ID: wo.ID, Kind: wo.Kind, Minutes: wo.Minutes}
		if t, ok := parseStoredTime(wo.StartedAt); ok {
			rec.StartedAt = &t
		}
		recs = append(recs, rec)
	}
	for i, sess := range program.GroupSessions(recs, program.SessionGap) {
		for _, id := range sess.Records {
			for j := range out {
				if out[j].ID == id {
					out[j].Session = i + 1
				}
			}
		}
	}
}

func scanMeal(row scanner) (Meal, error) {
	var m Meal
	err := row.Scan(&m.ID, &m.DayID, &m.PhotoID, &m.Name, &m.Slot, &m.Kcal,
		&m.ProteinG, &m.CarbsG, &m.FatG, &m.Source, &m.Notes, &m.EatenAt,
		&m.EstimateStatus, &m.EstimateError)
	if m.PhotoID != nil {
		m.PhotoURL = "/api/photos/" + strconv.FormatInt(*m.PhotoID, 10) + "/file"
	}
	m.Items = []MealItem{}
	return m, err
}

func scanWorkout(row scanner) (Workout, error) {
	var wo Workout
	var started sql.NullString
	err := row.Scan(&wo.ID, &wo.DayID, &wo.Kind, &wo.Activity, &wo.Minutes,
		&wo.Kcal, &wo.Notes, &wo.CreatedAt, &started)
	if started.Valid {
		wo.StartedAt = started.String
	}
	return wo, err
}

func validSlot(slot string) bool {
	switch slot {
	case "breakfast", "lunch", "dinner", "snack":
		return true
	}
	return false
}

func derefFloat(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

// dayIsToday reports whether a day row is the user's current local date.
//
// Used to decide whether "now" is a truthful start time for a hand-logged
// workout: it is, for an entry against today, and it is not for one filed
// against an earlier day.
func (s *Server) dayIsToday(ctx context.Context, dayID int64) bool {
	var date, tz string
	if err := s.db.QueryRowContext(ctx, `
		SELECT d.date, COALESCE(u.timezone, 'UTC')
		  FROM days d
		  JOIN programs p ON p.id = d.program_id
		  JOIN users u ON u.id = p.user_id
		 WHERE d.id = ?`, dayID).Scan(&date, &tz); err != nil {
		return false
	}
	return date == program.LocalDate(time.Now(), program.LoadLocation(tz))
}
