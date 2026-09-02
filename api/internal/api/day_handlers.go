package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/anchoo2kewl/75hard/api/internal/program"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// Day is one calendar day of an attempt, with everything the day screen needs
// in a single response — the SPA should never have to fan out to render it.
type Day struct {
	ID          int64    `json:"id"`
	ProgramID   int64    `json:"program_id"`
	DayNumber   int      `json:"day_number"`
	Date        string   `json:"date"`
	Status      string   `json:"status"`
	Note        string   `json:"note"`
	WeightKg    *float64 `json:"weight_kg"`
	CompletedAt *string  `json:"completed_at"`

	IsToday    bool      `json:"is_today"`
	TasksDone  int       `json:"tasks_done"`
	TasksTotal int       `json:"tasks_total"`
	Entries    []Entry   `json:"entries"`
	Photos     []Photo   `json:"photos"`
	Meals      []Meal    `json:"meals"`
	Workouts   []Workout `json:"workouts"`
	Totals     Totals    `json:"totals"`
}

// Entry is a task's state on a day, joined with its template definition so the
// client can render the row without a second lookup.
type Entry struct {
	TaskID      int64    `json:"task_id"`
	Key         string   `json:"key"`
	Title       string   `json:"title"`
	Detail      string   `json:"detail"`
	Icon        string   `json:"icon"`
	Kind        string   `json:"kind"`
	Unit        string   `json:"unit"`
	TargetNum   *float64 `json:"target_num"`
	Required    bool     `json:"required"`
	SortOrder   int      `json:"sort_order"`
	Value       *float64 `json:"value"`
	Note        string   `json:"note"`
	Done        bool     `json:"done"`
	CompletedAt *string  `json:"completed_at"`
	Tracker     string   `json:"tracker"`
	Color       string   `json:"color"`
}

// Totals are the day's rolled-up nutrition and training numbers.
type Totals struct {
	Kcal           float64 `json:"kcal"`
	ProteinG       float64 `json:"protein_g"`
	CarbsG         float64 `json:"carbs_g"`
	FatG           float64 `json:"fat_g"`
	KcalTarget     *int    `json:"kcal_target"`
	WorkoutMinutes int     `json:"workout_minutes"`
	OutdoorMinutes int     `json:"outdoor_minutes"`
}

// HandleGetToday returns the current day of the active program, creating the
// day row on first access.
//
// A program can be scheduled to start tomorrow, and it can run past its final
// day before the user closes it out. Neither is an error, so the date is
// clamped into the program's window rather than refused — the response's
// is_today tells the client which case it is looking at.
func (s *Server) HandleGetToday(w http.ResponseWriter, r *http.Request) {
	programID, err := s.activeProgramID(r)
	if err != nil {
		respondError(w, http.StatusNotFound, "no active program", "no_active_program")
		return
	}

	var startDate string
	var length int
	if err := s.db.QueryRowContext(r.Context(),
		`SELECT start_date, length_days FROM programs WHERE id = ?`, programID).
		Scan(&startDate, &length); err != nil {
		respondError(w, http.StatusInternalServerError, "could not load program", "internal")
		return
	}

	today := program.LocalDate(time.Now(), s.userLocation(r))

	day, err := s.dayByDate(r, programID, clampToProgram(startDate, length, today))
	if err != nil {
		s.log.Error("load today", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "could not load today", "internal")
		return
	}
	respondJSON(w, http.StatusOK, day)
}

// clampToProgram maps a real-world local date onto a date the program actually
// covers: before it starts, day 1; after it ends, the final day.
func clampToProgram(startDate string, length int, today string) string {
	if today < startDate {
		return startDate
	}
	if n := program.DayNumber(startDate, today); length > 0 && n > length {
		return program.DateForDay(startDate, length)
	}
	return today
}

// HandleGetDay returns a specific day by number.
func (s *Server) HandleGetDay(w http.ResponseWriter, r *http.Request) {
	programID, ok := s.programParam(w, r)
	if !ok {
		return
	}
	n, err := strconv.Atoi(chi.URLParam(r, "dayNumber"))
	if err != nil || n < 1 {
		respondError(w, http.StatusBadRequest, "invalid day number", "invalid_id")
		return
	}

	var startDate string
	var length int
	if err := s.db.QueryRowContext(r.Context(),
		`SELECT start_date, length_days FROM programs WHERE id = ?`, programID).Scan(&startDate, &length); err != nil {
		respondError(w, http.StatusNotFound, "program not found", "not_found")
		return
	}
	if n > length {
		respondError(w, http.StatusNotFound, "day is beyond the end of the program", "not_found")
		return
	}

	day, err := s.dayByDate(r, programID, program.DateForDay(startDate, n))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load day", "internal")
		return
	}
	respondJSON(w, http.StatusOK, day)
}

// DaySummary is the compact form used by the calendar grid.
type DaySummary struct {
	DayNumber  int    `json:"day_number"`
	Date       string `json:"date"`
	Status     string `json:"status"`
	TasksDone  int    `json:"tasks_done"`
	TasksTotal int    `json:"tasks_total"`
	PhotoCount int    `json:"photo_count"`
}

// HandleListDays returns one summary per day of the program, including days
// that have no row yet, so the calendar can render the full 75 up front.
func (s *Server) HandleListDays(w http.ResponseWriter, r *http.Request) {
	programID, ok := s.programParam(w, r)
	if !ok {
		return
	}
	ctx := r.Context()

	var startDate string
	var length int
	if err := s.db.QueryRowContext(ctx,
		`SELECT start_date, length_days FROM programs WHERE id = ?`, programID).
		Scan(&startDate, &length); err != nil {
		respondError(w, http.StatusNotFound, "program not found", "not_found")
		return
	}

	// One query for the stored days, then fill the gaps in memory rather than
	// materialising 75 rows for a program the user just started.
	type stored struct {
		status     string
		done       int
		photoCount int
	}
	byNumber := map[int]stored{}

	var total int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM program_tasks WHERE program_id = ? AND required = 1`, programID).Scan(&total)

	// A photo task is satisfied by an upload rather than by a task_entry row,
	// so counting entries alone under-reports the day and the calendar ring
	// disagrees with the day screen. The second term adds photo-kind tasks
	// that a progress photo has satisfied but that have no completed entry,
	// which is what keeps the two views in step without double counting.
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.day_number, d.status,
		       (SELECT COUNT(*) FROM task_entries te
		        WHERE te.day_id = d.id AND te.completed_at IS NOT NULL)
		     + (SELECT COUNT(*) FROM program_tasks pt
		        WHERE pt.program_id = d.program_id AND pt.kind = 'photo'
		          AND EXISTS (SELECT 1 FROM photos p WHERE p.day_id = d.id AND p.kind = 'progress')
		          AND NOT EXISTS (SELECT 1 FROM task_entries te2
		                          WHERE te2.day_id = d.id AND te2.program_task_id = pt.id
		                            AND te2.completed_at IS NOT NULL)),
		       (SELECT COUNT(*) FROM photos p WHERE p.day_id = d.id)
		FROM days d WHERE d.program_id = ?`, programID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not list days", "internal")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var n int
		var st stored
		if err := rows.Scan(&n, &st.status, &st.done, &st.photoCount); err != nil {
			respondError(w, http.StatusInternalServerError, "could not read days", "internal")
			return
		}
		byNumber[n] = st
	}

	out := make([]DaySummary, 0, length)
	for n := 1; n <= length; n++ {
		sum := DaySummary{
			DayNumber:  n,
			Date:       program.DateForDay(startDate, n),
			Status:     program.StatusPending,
			TasksTotal: total,
		}
		if st, ok := byNumber[n]; ok {
			sum.Status = st.status
			sum.TasksDone = st.done
			sum.PhotoCount = st.photoCount
		}
		out = append(out, sum)
	}
	respondJSON(w, http.StatusOK, out)
}

type toggleTaskRequest struct {
	// Done drives check/photo/text tasks. Ignored when Value is supplied for a
	// number or duration task, where the target decides completion.
	Done  *bool    `json:"done"`
	Value *float64 `json:"value"`
	Note  *string  `json:"note"`
}

// HandleToggleTask records progress against one task on one day. It is the
// endpoint behind the check-off tap, so it returns the whole recomputed day:
// the client applies its optimistic update, then reconciles against this.
func (s *Server) HandleToggleTask(w http.ResponseWriter, r *http.Request) {
	programID, ok := s.programParam(w, r)
	if !ok {
		return
	}
	dayNumber, err := strconv.Atoi(chi.URLParam(r, "dayNumber"))
	if err != nil || dayNumber < 1 {
		respondError(w, http.StatusBadRequest, "invalid day number", "invalid_id")
		return
	}
	taskID, err := strconv.ParseInt(chi.URLParam(r, "taskID"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid task id", "invalid_id")
		return
	}

	var req toggleTaskRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()

	var startDate string
	if err := s.db.QueryRowContext(ctx,
		`SELECT start_date FROM programs WHERE id = ?`, programID).Scan(&startDate); err != nil {
		respondError(w, http.StatusNotFound, "program not found", "not_found")
		return
	}
	onDate := program.DateForDay(startDate, dayNumber)

	// Confirm the task belongs to this program before writing anything.
	var kind string
	var target *float64
	if err := s.db.QueryRowContext(ctx,
		`SELECT kind, target_num FROM program_tasks WHERE id = ? AND program_id = ?`,
		taskID, programID).Scan(&kind, &target); err != nil {
		respondError(w, http.StatusNotFound, "task not found", "not_found")
		return
	}

	dayID, err := s.ensureDay(ctx, programID, startDate, onDate)
	if err != nil {
		s.log.Error("ensure day", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "could not record task", "internal")
		return
	}

	// Decide completion from the rules, not from the client: a number task is
	// done when it reaches its target, whatever the request claims.
	entry := program.Entry{TaskID: taskID}
	if req.Value != nil {
		entry.ValueNum = req.Value
	}
	if req.Done != nil {
		entry.Completed = *req.Done
	}
	if kind == program.KindPhoto && req.Done == nil {
		// Leave photo tasks to the upload path.
		entry.Completed = false
	}
	done := program.EntrySatisfies(
		program.Task{ID: taskID, Kind: kind, TargetNum: target, Required: true}, entry)

	var completedAt any
	if done {
		completedAt = time.Now().UTC()
	}

	note := ""
	if req.Note != nil {
		note = strings.TrimSpace(*req.Note)
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO task_entries (day_id, program_task_id, completed_at, value_num, note)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(day_id, program_task_id) DO UPDATE SET
			completed_at = excluded.completed_at,
			value_num    = COALESCE(excluded.value_num, task_entries.value_num),
			note         = CASE WHEN excluded.note != '' THEN excluded.note ELSE task_entries.note END,
			updated_at   = CURRENT_TIMESTAMP`,
		dayID, taskID, completedAt, req.Value, note); err != nil {
		s.log.Error("upsert task entry", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "could not record task", "internal")
		return
	}

	if err := s.refreshDayStatus(r, programID, dayID); err != nil {
		s.log.Error("refresh day status", zap.Error(err))
	}

	day, err := s.dayByDate(r, programID, onDate)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load day", "internal")
		return
	}
	respondJSON(w, http.StatusOK, day)
}

type updateDayRequest struct {
	Note     *string  `json:"note"`
	WeightKg *float64 `json:"weight_kg"`
}

// HandleUpdateDay saves the day's journal note and weigh-in.
func (s *Server) HandleUpdateDay(w http.ResponseWriter, r *http.Request) {
	programID, ok := s.programParam(w, r)
	if !ok {
		return
	}
	dayNumber, err := strconv.Atoi(chi.URLParam(r, "dayNumber"))
	if err != nil || dayNumber < 1 {
		respondError(w, http.StatusBadRequest, "invalid day number", "invalid_id")
		return
	}
	var req updateDayRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	var startDate string
	if err := s.db.QueryRowContext(r.Context(),
		`SELECT start_date FROM programs WHERE id = ?`, programID).Scan(&startDate); err != nil {
		respondError(w, http.StatusNotFound, "program not found", "not_found")
		return
	}
	onDate := program.DateForDay(startDate, dayNumber)

	dayID, err := s.ensureDay(r.Context(), programID, startDate, onDate)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not update day", "internal")
		return
	}

	if req.Note != nil {
		if _, err := s.db.ExecContext(r.Context(),
			`UPDATE days SET note = ? WHERE id = ?`, strings.TrimSpace(*req.Note), dayID); err != nil {
			respondError(w, http.StatusInternalServerError, "could not update day", "internal")
			return
		}
	}
	if req.WeightKg != nil {
		if _, err := s.db.ExecContext(r.Context(),
			`UPDATE days SET weight_kg = ? WHERE id = ?`, *req.WeightKg, dayID); err != nil {
			respondError(w, http.StatusInternalServerError, "could not update day", "internal")
			return
		}
	}

	day, err := s.dayByDate(r, programID, onDate)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load day", "internal")
		return
	}
	respondJSON(w, http.StatusOK, day)
}

// ---- helpers ----

// ensureDay returns the id of the day row for onDate, creating it if needed.
func (s *Server) ensureDay(ctx context.Context, programID int64, startDate, onDate string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM days WHERE program_id = ? AND on_date = ?`, programID, onDate).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}

	dayNumber := program.DayNumber(startDate, onDate)
	if dayNumber < 1 {
		return 0, errors.New("date is before the program start")
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO days (program_id, day_number, on_date) VALUES (?, ?, ?)`,
		programID, dayNumber, onDate)
	if err != nil {
		// Lost a race with a concurrent request; the row exists now.
		if err2 := s.db.QueryRowContext(ctx,
			`SELECT id FROM days WHERE program_id = ? AND on_date = ?`, programID, onDate).Scan(&id); err2 == nil {
			return id, nil
		}
		return 0, err
	}
	return res.LastInsertId()
}

// refreshDayStatus re-evaluates the day against the rules and, when the rules
// say the attempt is over, updates the program too.
func (s *Server) refreshDayStatus(r *http.Request, programID, dayID int64) error {
	ctx := r.Context()

	var (
		startDate string
		length    int
		strict    int
		status    string
	)
	if err := s.db.QueryRowContext(ctx,
		`SELECT start_date, length_days, strict_restart, status FROM programs WHERE id = ?`, programID).
		Scan(&startDate, &length, &strict, &status); err != nil {
		return err
	}

	var dayNumber int
	var onDate string
	if err := s.db.QueryRowContext(ctx,
		`SELECT day_number, on_date FROM days WHERE id = ?`, dayID).Scan(&dayNumber, &onDate); err != nil {
		return err
	}

	tasks, entries, err := s.dayProgress(ctx, programID, dayID)
	if err != nil {
		return err
	}
	complete, _, _ := program.DayComplete(tasks, entries)

	today := program.LocalDate(time.Now(), s.userLocation(r))
	outcome := program.EvaluateDay(onDate, today, dayNumber, length, complete, strict == 1)

	var completedAt any
	if outcome.DayStatus == program.StatusComplete {
		completedAt = time.Now().UTC()
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE days SET status = ?, completed_at = ? WHERE id = ?`,
		outcome.DayStatus, completedAt, dayID); err != nil {
		return err
	}

	// Only ever move an active program; never resurrect one the user ended.
	if outcome.ProgramStatus != "" && status == program.ProgramActive {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE programs SET status = ?, ended_at = CURRENT_TIMESTAMP WHERE id = ?`,
			outcome.ProgramStatus, programID); err != nil {
			return err
		}
	}
	return nil
}

// dayProgress loads the template and the day's entries in the shape the rules
// package expects.
func (s *Server) dayProgress(ctx context.Context, programID, dayID int64) ([]program.Task, map[int64]program.Entry, error) {
	// A photo task is satisfied by an upload, so the photo count has to reach
	// the rules alongside the entries. Read it first, while no cursor is open.
	photoCount := 0
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM photos WHERE day_id = ? AND kind = 'progress'`, dayID).Scan(&photoCount)

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, kind, target_num, required FROM program_tasks WHERE program_id = ?`, programID)
	if err != nil {
		return nil, nil, err
	}
	var tasks []program.Task
	for rows.Next() {
		var t program.Task
		var required int
		if err := rows.Scan(&t.ID, &t.Kind, &t.TargetNum, &required); err != nil {
			rows.Close()
			return nil, nil, err
		}
		t.Required = required == 1
		tasks = append(tasks, t)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}

	entries := map[int64]program.Entry{}
	erows, err := s.db.QueryContext(ctx,
		`SELECT program_task_id, completed_at IS NOT NULL, value_num FROM task_entries WHERE day_id = ?`, dayID)
	if err != nil {
		return nil, nil, err
	}
	for erows.Next() {
		var e program.Entry
		var done bool
		if err := erows.Scan(&e.TaskID, &done, &e.ValueNum); err != nil {
			erows.Close()
			return nil, nil, err
		}
		e.Completed = done
		entries[e.TaskID] = e
	}
	err = erows.Err()
	erows.Close()
	if err != nil {
		return nil, nil, err
	}

	if photoCount > 0 {
		for _, t := range tasks {
			if t.Kind == program.KindPhoto {
				e := entries[t.ID]
				e.TaskID = t.ID
				e.PhotoCount = photoCount
				entries[t.ID] = e
			}
		}
	}

	return tasks, entries, nil
}

// dayByDate assembles the full day payload, creating the row if this is the
// first time the day has been looked at.
func (s *Server) dayByDate(r *http.Request, programID int64, onDate string) (*Day, error) {
	ctx := r.Context()

	var startDate string
	var kcalTarget *int
	if err := s.db.QueryRowContext(ctx,
		`SELECT start_date, daily_kcal_target FROM programs WHERE id = ?`, programID).
		Scan(&startDate, &kcalTarget); err != nil {
		return nil, err
	}

	dayID, err := s.ensureDay(ctx, programID, startDate, onDate)
	if err != nil {
		return nil, err
	}

	d := &Day{ProgramID: programID}
	if err := s.db.QueryRowContext(ctx,
		`SELECT id, day_number, on_date, status, note, weight_kg, completed_at FROM days WHERE id = ?`, dayID).
		Scan(&d.ID, &d.DayNumber, &d.Date, &d.Status, &d.Note, &d.WeightKg, &d.CompletedAt); err != nil {
		return nil, err
	}
	d.IsToday = d.Date == program.LocalDate(time.Now(), s.userLocation(r))
	d.Totals.KcalTarget = kcalTarget

	// Read this before opening the entries cursor: running a query while
	// another result set is still open borrows a second pool connection for
	// no reason.
	photoCount := 0
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM photos WHERE day_id = ? AND kind = 'progress'`, dayID).Scan(&photoCount)

	// Left join so tasks with no entry yet still appear, in template order.
	rows, err := s.db.QueryContext(ctx, `
		SELECT pt.id, pt.task_key, pt.title, pt.detail, pt.icon, pt.kind, pt.unit,
		       pt.target_num, pt.required, pt.sort_order, pt.tracker, pt.color,
		       te.value_num, COALESCE(te.note, ''), te.completed_at
		FROM program_tasks pt
		LEFT JOIN task_entries te ON te.program_task_id = pt.id AND te.day_id = ?
		WHERE pt.program_id = ?
		ORDER BY pt.sort_order, pt.id`, dayID, programID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	d.Entries = []Entry{}
	for rows.Next() {
		var e Entry
		var required int
		if err := rows.Scan(&e.TaskID, &e.Key, &e.Title, &e.Detail, &e.Icon, &e.Kind, &e.Unit,
			&e.TargetNum, &required, &e.SortOrder, &e.Tracker, &e.Color,
			&e.Value, &e.Note, &e.CompletedAt); err != nil {
			return nil, err
		}
		e.Required = required == 1

		pe := program.Entry{
			TaskID:    e.TaskID,
			Completed: e.CompletedAt != nil,
			ValueNum:  e.Value,
		}
		if e.Kind == program.KindPhoto {
			pe.PhotoCount = photoCount
		}
		e.Done = program.EntrySatisfies(
			program.Task{ID: e.TaskID, Kind: e.Kind, TargetNum: e.TargetNum, Required: e.Required}, pe)

		if e.Required {
			d.TasksTotal++
		}
		if e.Done {
			d.TasksDone++
		}
		d.Entries = append(d.Entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Done with the cursor; release it before the follow-up queries.
	rows.Close()

	if d.Photos, err = s.photosForDay(ctx, dayID); err != nil {
		return nil, err
	}
	if d.Meals, err = s.mealsForDay(ctx, dayID); err != nil {
		return nil, err
	}
	if d.Workouts, err = s.workoutsForDay(ctx, dayID); err != nil {
		return nil, err
	}

	for _, m := range d.Meals {
		d.Totals.Kcal += m.Kcal
		d.Totals.ProteinG += m.ProteinG
		d.Totals.CarbsG += m.CarbsG
		d.Totals.FatG += m.FatG
	}
	for _, wo := range d.Workouts {
		d.Totals.WorkoutMinutes += wo.Minutes
		if wo.Kind == "outdoor" {
			d.Totals.OutdoorMinutes += wo.Minutes
		}
	}

	return d, nil
}
