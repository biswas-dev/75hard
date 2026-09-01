package api

import (
	"context"
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
	ID        int64      `json:"id"`
	DayID     int64      `json:"day_id"`
	PhotoID   *int64     `json:"photo_id"`
	PhotoURL  string     `json:"photo_url,omitempty"`
	Name      string     `json:"name"`
	Slot      string     `json:"slot"`
	Kcal      float64    `json:"kcal"`
	ProteinG  float64    `json:"protein_g"`
	CarbsG    float64    `json:"carbs_g"`
	FatG      float64    `json:"fat_g"`
	Source    string     `json:"source"`
	Notes     string     `json:"notes"`
	EatenAt   string     `json:"eaten_at"`
	Items     []MealItem `json:"items"`
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
	if len(sets) == 0 {
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

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO workouts (user_id, day_id, kind, activity, minutes, kcal, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, dayID, kind, strings.TrimSpace(req.Activity), req.Minutes, req.Kcal,
		strings.TrimSpace(req.Notes))
	if err != nil {
		s.log.Error("insert workout", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "could not log workout", "internal")
		return
	}
	id, _ := res.LastInsertId()

	// Credit the linked duration task with the total minutes logged for it
	// today, so two short sessions can add up to the target.
	if req.TaskID != nil {
		var owns int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM program_tasks WHERE id = ? AND program_id = ?`,
			*req.TaskID, programID).Scan(&owns); err == nil && owns == 1 {
			var total int
			_ = s.db.QueryRowContext(ctx,
				`SELECT COALESCE(SUM(minutes), 0) FROM workouts WHERE day_id = ? AND kind = ?`,
				dayID, kind).Scan(&total)

			var target *float64
			var taskKind string
			if err := s.db.QueryRowContext(ctx,
				`SELECT kind, target_num FROM program_tasks WHERE id = ?`, *req.TaskID).
				Scan(&taskKind, &target); err == nil {
				value := float64(total)
				done := program.EntrySatisfies(
					program.Task{ID: *req.TaskID, Kind: taskKind, TargetNum: target, Required: true},
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
					dayID, *req.TaskID, completedAt, value); err != nil {
					s.log.Error("credit workout task", zap.Error(err))
				}
			}
		}
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
	if err := s.db.QueryRowContext(r.Context(),
		`SELECT start_date FROM programs WHERE id = ?`, programID).Scan(&startDate); err != nil {
		respondError(w, http.StatusInternalServerError, "could not load program", "internal")
		return 0, 0, false
	}

	onDate := program.LocalDate(time.Now(), s.userLocation(r))
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
		SELECT id, day_id, photo_id, name, slot, kcal, protein_g, carbs_g, fat_g, source, notes, eaten_at
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
		SELECT id, day_id, photo_id, name, slot, kcal, protein_g, carbs_g, fat_g, source, notes, eaten_at
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
		`SELECT id, day_id, kind, activity, minutes, kcal, notes, created_at
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
	return out, rows.Err()
}

func scanMeal(row scanner) (Meal, error) {
	var m Meal
	err := row.Scan(&m.ID, &m.DayID, &m.PhotoID, &m.Name, &m.Slot, &m.Kcal,
		&m.ProteinG, &m.CarbsG, &m.FatG, &m.Source, &m.Notes, &m.EatenAt)
	if m.PhotoID != nil {
		m.PhotoURL = "/api/photos/" + strconv.FormatInt(*m.PhotoID, 10) + "/file"
	}
	m.Items = []MealItem{}
	return m, err
}

func scanWorkout(row scanner) (Workout, error) {
	var wo Workout
	err := row.Scan(&wo.ID, &wo.DayID, &wo.Kind, &wo.Activity, &wo.Minutes,
		&wo.Kcal, &wo.Notes, &wo.CreatedAt)
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
