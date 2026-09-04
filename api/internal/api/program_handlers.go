package api

import (
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

// Program is an attempt at the challenge as returned to the SPA.
type Program struct {
	ID              int64   `json:"id"`
	Name            string  `json:"name"`
	StartDate       string  `json:"start_date"`
	LengthDays      int     `json:"length_days"`
	Status          string  `json:"status"`
	StrictRestart   bool    `json:"strict_restart"`
	AttemptNumber   int     `json:"attempt_number"`
	DailyKcalTarget *int    `json:"daily_kcal_target"`
	Notes           string  `json:"notes"`
	CreatedAt       string  `json:"created_at"`
	EndedAt         *string `json:"ended_at"`

	// Derived, not stored.
	CurrentDay   int    `json:"current_day"`
	DaysComplete int    `json:"days_complete"`
	Streak       int    `json:"streak"`
	Today        string `json:"today"`

	Tasks []ProgramTask `json:"tasks,omitempty"`
}

// ProgramTask is one item in a program's editable daily template.
type ProgramTask struct {
	ID        int64    `json:"id"`
	Key       string   `json:"key"`
	Title     string   `json:"title"`
	Detail    string   `json:"detail"`
	Icon      string   `json:"icon"`
	Kind      string   `json:"kind"`
	TargetNum *float64 `json:"target_num"`
	Unit      string   `json:"unit"`
	SortOrder int      `json:"sort_order"`
	Required  bool     `json:"required"`
	Color     string   `json:"color"`
	// Tracker names an optional richer panel behind the task. Never affects
	// completion — a tap is always enough.
	Tracker string `json:"tracker"`
	// Hidden removes an optional task from the day screen, the grid and every
	// count, without deleting the history it has already accumulated. Only
	// optional tasks can be hidden; a required one is the challenge.
	Hidden bool `json:"hidden"`
}

type createProgramRequest struct {
	Name            string        `json:"name"`
	StartDate       string        `json:"start_date"`
	LengthDays      int           `json:"length_days"`
	StrictRestart   *bool         `json:"strict_restart"`
	DailyKcalTarget *int          `json:"daily_kcal_target"`
	Notes           string        `json:"notes"`
	Tasks           []taskPayload `json:"tasks"`
}

type taskPayload struct {
	Key       string   `json:"key"`
	Title     string   `json:"title"`
	Detail    string   `json:"detail"`
	Icon      string   `json:"icon"`
	Kind      string   `json:"kind"`
	TargetNum *float64 `json:"target_num"`
	Unit      string   `json:"unit"`
	Required  *bool    `json:"required"`
	Color     string   `json:"color"`
	Tracker   string   `json:"tracker"`
}

// HandleCreateProgram starts a new attempt, seeding the canonical six tasks
// when the caller doesn't supply their own template.
func (s *Server) HandleCreateProgram(w http.ResponseWriter, r *http.Request) {
	var req createProgramRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	userID := UserID(ctx)
	loc := s.userLocation(r)

	// One active attempt at a time — two would make "today" ambiguous.
	var existing int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM programs WHERE user_id = ? AND status = 'active'`, userID).Scan(&existing)
	if err == nil {
		respondError(w, http.StatusConflict, "you already have an active program", "program_active")
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		s.log.Error("check active program", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "could not start program", "internal")
		return
	}

	startDate := strings.TrimSpace(req.StartDate)
	if startDate == "" {
		startDate = program.LocalDate(time.Now(), loc)
	}
	if _, err := program.ParseDate(startDate); err != nil {
		respondError(w, http.StatusBadRequest, "start_date must be YYYY-MM-DD", "invalid_date")
		return
	}

	length := req.LengthDays
	if length <= 0 {
		length = program.DefaultLength
	}
	if length > 365 {
		respondError(w, http.StatusBadRequest, "length_days must be 365 or fewer", "invalid_length")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "75 Hard"
	}
	strict := true
	if req.StrictRestart != nil {
		strict = *req.StrictRestart
	}

	// Continue the attempt numbering from any previous try.
	var attempt int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(attempt_number), 0) FROM programs WHERE user_id = ?`, userID).Scan(&attempt)
	attempt++

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not start program", "internal")
		return
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.ExecContext(ctx, `
		INSERT INTO programs (user_id, name, start_date, length_days, strict_restart,
		                      attempt_number, daily_kcal_target, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, name, startDate, length, boolToInt(strict), attempt, req.DailyKcalTarget, strings.TrimSpace(req.Notes))
	if err != nil {
		s.log.Error("insert program", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "could not start program", "internal")
		return
	}
	programID, _ := res.LastInsertId()

	tasks := req.Tasks
	if len(tasks) == 0 {
		for _, d := range program.DefaultTasks() {
			required := d.Required
			tasks = append(tasks, taskPayload{
				Key: d.Key, Title: d.Title, Detail: d.Detail, Icon: d.Icon,
				Kind: d.Kind, TargetNum: d.TargetNum, Unit: d.Unit, Required: &required,
				Tracker: d.Tracker,
			})
		}
	}

	for i, t := range tasks {
		title := strings.TrimSpace(t.Title)
		if title == "" {
			respondError(w, http.StatusBadRequest, "every task needs a title", "invalid_task")
			return
		}
		kind := t.Kind
		if !validTaskKind(kind) {
			respondError(w, http.StatusBadRequest, "unknown task kind: "+kind, "invalid_task")
			return
		}
		key := strings.TrimSpace(t.Key)
		if key == "" {
			key = slugify(title) + "_" + strconv.Itoa(i)
		}
		required := true
		if t.Required != nil {
			required = *t.Required
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO program_tasks (program_id, task_key, title, detail, icon, kind,
			                           target_num, unit, sort_order, required, color, tracker)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			programID, key, title, strings.TrimSpace(t.Detail), defaultString(t.Icon, "check"),
			kind, t.TargetNum, strings.TrimSpace(t.Unit), i, boolToInt(required),
			defaultString(t.Color, paletteColor(i)), trackerOrNone(t.Tracker)); err != nil {
			s.log.Error("insert program task", zap.Error(err))
			respondError(w, http.StatusInternalServerError, "could not create tasks", "internal")
			return
		}
	}

	if err := tx.Commit(); err != nil {
		respondError(w, http.StatusInternalServerError, "could not start program", "internal")
		return
	}

	p, err := s.loadProgram(r, programID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load program", "internal")
		return
	}
	respondJSON(w, http.StatusCreated, p)
}

// HandleListPrograms returns every attempt, newest first.
func (s *Server) HandleListPrograms(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT id, name, start_date, length_days, status, strict_restart, attempt_number,
		       daily_kcal_target, notes, created_at, ended_at
		FROM programs WHERE user_id = ? ORDER BY created_at DESC`, UserID(r.Context()))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not list programs", "internal")
		return
	}
	defer rows.Close()

	out := []Program{}
	for rows.Next() {
		p, err := scanProgram(rows)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "could not read programs", "internal")
			return
		}
		out = append(out, p)
	}
	// Done with the cursor before the queries below.
	rows.Close()

	// The scanned row carries none of the derived fields, so every program in
	// this list used to report day 0 and no completed days — which is what
	// settings showed for a run on its third day.
	loc := s.userLocation(r)
	today := program.LocalDate(time.Now(), loc)

	complete := s.completedDaysByProgram(r)
	for i := range out {
		out[i].Today = today
		out[i].CurrentDay = currentDayOf(out[i], today)
		out[i].DaysComplete = complete[out[i].ID]
	}

	respondJSON(w, http.StatusOK, out)
}

// currentDayOf is how far through an attempt is, counted to today while it
// runs and to the day it ended once it has.
//
// Counting a finished attempt to today would have a run abandoned on day 3
// climbing towards 75 for the rest of the year.
func currentDayOf(p Program, today string) int {
	on := today
	if p.Status != program.ProgramActive && p.EndedAt != nil && *p.EndedAt != "" {
		if len(*p.EndedAt) >= len(program.DateLayout) {
			on = (*p.EndedAt)[:len(program.DateLayout)]
		}
	}
	day := program.DayNumber(p.StartDate, on)
	if day > p.LengthDays {
		return p.LengthDays
	}
	if day < 0 {
		return 0
	}
	return day
}

// completedDaysByProgram counts complete days for every attempt at once,
// rather than a query per row.
func (s *Server) completedDaysByProgram(r *http.Request) map[int64]int {
	out := map[int64]int{}
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT d.program_id, COUNT(*)
		  FROM days d JOIN programs p ON p.id = d.program_id
		 WHERE p.user_id = ? AND d.status = ?
		 GROUP BY d.program_id`, UserID(r.Context()), program.StatusComplete)
	if err != nil {
		return out
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err == nil {
			out[id] = n
		}
	}
	return out
}

// HandleGetActiveProgram returns the running attempt with its task template,
// or 404 when the user hasn't started one. The SPA uses the 404 to decide
// whether to show the "begin the program" screen.
func (s *Server) HandleGetActiveProgram(w http.ResponseWriter, r *http.Request) {
	id, err := s.activeProgramID(r)
	if err != nil {
		respondError(w, http.StatusNotFound, "no active program", "no_active_program")
		return
	}
	p, err := s.loadProgram(r, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load program", "internal")
		return
	}
	respondJSON(w, http.StatusOK, p)
}

// HandleGetProgram returns one attempt by id.
func (s *Server) HandleGetProgram(w http.ResponseWriter, r *http.Request) {
	id, ok := s.programParam(w, r)
	if !ok {
		return
	}
	p, err := s.loadProgram(r, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load program", "internal")
		return
	}
	respondJSON(w, http.StatusOK, p)
}

type updateProgramRequest struct {
	Name            *string `json:"name"`
	Notes           *string `json:"notes"`
	DailyKcalTarget *int    `json:"daily_kcal_target"`
	StrictRestart   *bool   `json:"strict_restart"`
	Status          *string `json:"status"`
}

// HandleUpdateProgram edits an attempt's settings, including abandoning it.
func (s *Server) HandleUpdateProgram(w http.ResponseWriter, r *http.Request) {
	id, ok := s.programParam(w, r)
	if !ok {
		return
	}
	var req updateProgramRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	sets := []string{}
	args := []any{}
	if req.Name != nil {
		sets, args = append(sets, "name = ?"), append(args, strings.TrimSpace(*req.Name))
	}
	if req.Notes != nil {
		sets, args = append(sets, "notes = ?"), append(args, strings.TrimSpace(*req.Notes))
	}
	if req.DailyKcalTarget != nil {
		sets, args = append(sets, "daily_kcal_target = ?"), append(args, *req.DailyKcalTarget)
	}
	if req.StrictRestart != nil {
		sets, args = append(sets, "strict_restart = ?"), append(args, boolToInt(*req.StrictRestart))
	}
	if req.Status != nil {
		st := *req.Status
		if st != program.ProgramActive && st != program.ProgramAbandoned && st != program.ProgramCompleted {
			respondError(w, http.StatusBadRequest, "invalid status", "invalid_status")
			return
		}
		sets, args = append(sets, "status = ?"), append(args, st)
		if st != program.ProgramActive {
			sets = append(sets, "ended_at = CURRENT_TIMESTAMP")
		}
	}
	if len(sets) == 0 {
		respondError(w, http.StatusBadRequest, "nothing to update", "no_changes")
		return
	}

	args = append(args, id)
	if _, err := s.db.ExecContext(r.Context(),
		`UPDATE programs SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
		respondError(w, http.StatusInternalServerError, "could not update program", "internal")
		return
	}

	p, err := s.loadProgram(r, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load program", "internal")
		return
	}
	respondJSON(w, http.StatusOK, p)
}

// HandleRestartProgram abandons the current attempt and opens a fresh one at
// day 1, carrying the task template and settings across and linking the two.
func (s *Server) HandleRestartProgram(w http.ResponseWriter, r *http.Request) {
	id, ok := s.programParam(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	userID := UserID(ctx)
	loc := s.userLocation(r)

	var (
		name, notes string
		length      int
		strict      int
		kcal        *int
		attempt     int
	)
	if err := s.db.QueryRowContext(ctx, `
		SELECT name, notes, length_days, strict_restart, daily_kcal_target, attempt_number
		FROM programs WHERE id = ? AND user_id = ?`, id, userID).
		Scan(&name, &notes, &length, &strict, &kcal, &attempt); err != nil {
		respondError(w, http.StatusNotFound, "program not found", "not_found")
		return
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not restart", "internal")
		return
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx,
		`UPDATE programs SET status = CASE WHEN status = 'active' THEN 'failed' ELSE status END,
		                     ended_at = COALESCE(ended_at, CURRENT_TIMESTAMP)
		 WHERE id = ?`, id); err != nil {
		respondError(w, http.StatusInternalServerError, "could not restart", "internal")
		return
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO programs (user_id, name, start_date, length_days, strict_restart,
		                      previous_attempt_id, attempt_number, daily_kcal_target, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, name, program.LocalDate(time.Now(), loc), length, strict, id, attempt+1, kcal, notes)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not restart", "internal")
		return
	}
	newID, _ := res.LastInsertId()

	// Copy the template rather than pointing at the old rows, so editing the
	// new attempt's tasks can't rewrite the history of the old one.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO program_tasks (program_id, task_key, title, detail, icon, kind, target_num, unit, sort_order, required, color, tracker)
		SELECT ?, task_key, title, detail, icon, kind, target_num, unit, sort_order, required, color, tracker
		FROM program_tasks WHERE program_id = ?`, newID, id); err != nil {
		respondError(w, http.StatusInternalServerError, "could not copy tasks", "internal")
		return
	}

	if err := tx.Commit(); err != nil {
		respondError(w, http.StatusInternalServerError, "could not restart", "internal")
		return
	}

	p, err := s.loadProgram(r, newID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load program", "internal")
		return
	}
	respondJSON(w, http.StatusCreated, p)
}

// HandleCreateTask adds a task to a program's template.
func (s *Server) HandleCreateTask(w http.ResponseWriter, r *http.Request) {
	id, ok := s.programParam(w, r)
	if !ok {
		return
	}
	var t taskPayload
	if !decodeJSON(w, r, &t) {
		return
	}
	title := strings.TrimSpace(t.Title)
	if title == "" {
		respondError(w, http.StatusBadRequest, "title is required", "invalid_task")
		return
	}
	if !validTaskKind(t.Kind) {
		respondError(w, http.StatusBadRequest, "unknown task kind", "invalid_task")
		return
	}

	var nextOrder int
	_ = s.db.QueryRowContext(r.Context(),
		`SELECT COALESCE(MAX(sort_order), -1) + 1 FROM program_tasks WHERE program_id = ?`, id).Scan(&nextOrder)

	key := strings.TrimSpace(t.Key)
	if key == "" {
		key = slugify(title) + "_" + strconv.Itoa(nextOrder)
	}
	required := true
	if t.Required != nil {
		required = *t.Required
	}

	if _, err := s.db.ExecContext(r.Context(), `
		INSERT INTO program_tasks (program_id, task_key, title, detail, icon, kind, target_num, unit, sort_order, required, color, tracker)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, key, title, strings.TrimSpace(t.Detail), defaultString(t.Icon, "check"),
		t.Kind, t.TargetNum, strings.TrimSpace(t.Unit), nextOrder, boolToInt(required),
		defaultString(t.Color, paletteColor(nextOrder)), trackerOrNone(t.Tracker)); err != nil {
		if isUniqueViolation(err) {
			respondError(w, http.StatusConflict, "a task with that key already exists", "task_exists")
			return
		}
		respondError(w, http.StatusInternalServerError, "could not add task", "internal")
		return
	}

	tasks, err := s.loadTasks(r, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load tasks", "internal")
		return
	}
	respondJSON(w, http.StatusCreated, tasks)
}

type updateTaskRequest struct {
	Title     *string  `json:"title"`
	Detail    *string  `json:"detail"`
	Icon      *string  `json:"icon"`
	Kind      *string  `json:"kind"`
	TargetNum *float64 `json:"target_num"`
	Unit      *string  `json:"unit"`
	SortOrder *int     `json:"sort_order"`
	Required  *bool    `json:"required"`
	Color     *string  `json:"color"`
	Tracker   *string  `json:"tracker"`
	Hidden    *bool    `json:"hidden"`
}

// HandleUpdateTask edits one task in the template.
func (s *Server) HandleUpdateTask(w http.ResponseWriter, r *http.Request) {
	programID, ok := s.programParam(w, r)
	if !ok {
		return
	}
	taskID, err := strconv.ParseInt(chi.URLParam(r, "taskID"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid task id", "invalid_id")
		return
	}

	var req updateTaskRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	sets := []string{}
	args := []any{}
	if req.Title != nil {
		sets, args = append(sets, "title = ?"), append(args, strings.TrimSpace(*req.Title))
	}
	if req.Detail != nil {
		sets, args = append(sets, "detail = ?"), append(args, strings.TrimSpace(*req.Detail))
	}
	if req.Icon != nil {
		sets, args = append(sets, "icon = ?"), append(args, *req.Icon)
	}
	if req.Kind != nil {
		if !validTaskKind(*req.Kind) {
			respondError(w, http.StatusBadRequest, "unknown task kind", "invalid_task")
			return
		}
		sets, args = append(sets, "kind = ?"), append(args, *req.Kind)
	}
	if req.TargetNum != nil {
		sets, args = append(sets, "target_num = ?"), append(args, *req.TargetNum)
	}
	if req.Unit != nil {
		sets, args = append(sets, "unit = ?"), append(args, strings.TrimSpace(*req.Unit))
	}
	if req.SortOrder != nil {
		sets, args = append(sets, "sort_order = ?"), append(args, *req.SortOrder)
	}
	if req.Required != nil {
		sets, args = append(sets, "required = ?"), append(args, boolToInt(*req.Required))
	}
	if req.Hidden != nil {
		// Only an optional task can be hidden. Letting a required one vanish
		// would silently change what completing a day means, which is the one
		// thing the rules are for.
		if *req.Hidden {
			var required int
			if err := s.db.QueryRowContext(r.Context(),
				`SELECT required FROM program_tasks WHERE id = ? AND program_id = ?`,
				taskID, programID).Scan(&required); err == nil && required == 1 {
				respondError(w, http.StatusBadRequest,
					"a required task cannot be hidden; make it optional first", "task_required")
				return
			}
		}
		sets, args = append(sets, "hidden = ?"), append(args, boolToInt(*req.Hidden))
	}
	if req.Tracker != nil {
		if !program.ValidTracker(*req.Tracker) {
			respondError(w, http.StatusBadRequest, "unknown tracker", "invalid_tracker")
			return
		}
		sets, args = append(sets, "tracker = ?"), append(args, *req.Tracker)
	}
	if req.Color != nil {
		if !validHexColor(*req.Color) {
			respondError(w, http.StatusBadRequest, "color must be a hex value like #ff6b35", "invalid_color")
			return
		}
		sets, args = append(sets, "color = ?"), append(args, strings.ToLower(*req.Color))
	}
	if len(sets) == 0 {
		respondError(w, http.StatusBadRequest, "nothing to update", "no_changes")
		return
	}

	args = append(args, taskID, programID)
	if _, err := s.db.ExecContext(r.Context(),
		`UPDATE program_tasks SET `+strings.Join(sets, ", ")+` WHERE id = ? AND program_id = ?`, args...); err != nil {
		respondError(w, http.StatusInternalServerError, "could not update task", "internal")
		return
	}

	tasks, err := s.loadTasks(r, programID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load tasks", "internal")
		return
	}
	respondJSON(w, http.StatusOK, tasks)
}

// HandleDeleteTask removes a task from the template. Its entries go with it,
// by cascade — a task that no longer exists shouldn't hold days back.
func (s *Server) HandleDeleteTask(w http.ResponseWriter, r *http.Request) {
	programID, ok := s.programParam(w, r)
	if !ok {
		return
	}
	taskID, err := strconv.ParseInt(chi.URLParam(r, "taskID"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid task id", "invalid_id")
		return
	}
	if _, err := s.db.ExecContext(r.Context(),
		`DELETE FROM program_tasks WHERE id = ? AND program_id = ?`, taskID, programID); err != nil {
		respondError(w, http.StatusInternalServerError, "could not delete task", "internal")
		return
	}
	respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- helpers ----

// programParam reads the :programID route parameter and confirms the caller
// owns it. Every program-scoped route goes through here, so ownership is
// checked in exactly one place.
func (s *Server) programParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := chi.URLParam(r, "programID")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid program id", "invalid_id")
		return 0, false
	}
	var owner int64
	if err := s.db.QueryRowContext(r.Context(),
		`SELECT user_id FROM programs WHERE id = ?`, id).Scan(&owner); err != nil {
		respondError(w, http.StatusNotFound, "program not found", "not_found")
		return 0, false
	}
	if owner != UserID(r.Context()) {
		// 404 rather than 403: a stranger's program id should not be
		// confirmable by probing.
		respondError(w, http.StatusNotFound, "program not found", "not_found")
		return 0, false
	}
	return id, true
}

func (s *Server) activeProgramID(r *http.Request) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(r.Context(),
		`SELECT id FROM programs WHERE user_id = ? AND status = 'active'
		 ORDER BY created_at DESC LIMIT 1`, UserID(r.Context())).Scan(&id)
	return id, err
}

func (s *Server) loadProgram(r *http.Request, id int64) (Program, error) {
	row := s.db.QueryRowContext(r.Context(), `
		SELECT id, name, start_date, length_days, status, strict_restart, attempt_number,
		       daily_kcal_target, notes, created_at, ended_at
		FROM programs WHERE id = ?`, id)

	p, err := scanProgram(row)
	if err != nil {
		return p, err
	}

	tasks, err := s.loadTasks(r, id)
	if err != nil {
		return p, err
	}
	p.Tasks = tasks

	loc := s.userLocation(r)
	today := program.LocalDate(time.Now(), loc)
	p.Today = today
	p.CurrentDay = program.DayNumber(p.StartDate, today)
	if p.CurrentDay > p.LengthDays {
		p.CurrentDay = p.LengthDays
	}

	statuses := map[int]string{}
	rows, err := s.db.QueryContext(r.Context(),
		`SELECT day_number, status FROM days WHERE program_id = ?`, id)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var n int
			var st string
			if err := rows.Scan(&n, &st); err == nil {
				statuses[n] = st
				if st == program.StatusComplete {
					p.DaysComplete++
				}
			}
		}
	}
	p.Streak = program.Streak(statuses, p.CurrentDay)

	return p, nil
}

func (s *Server) loadTasks(r *http.Request, programID int64) ([]ProgramTask, error) {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT id, task_key, title, detail, icon, kind, target_num, unit, sort_order, required, color, tracker, hidden
		FROM program_tasks WHERE program_id = ? ORDER BY sort_order, id`, programID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ProgramTask{}
	for rows.Next() {
		var t ProgramTask
		var required, hidden int
		if err := rows.Scan(&t.ID, &t.Key, &t.Title, &t.Detail, &t.Icon, &t.Kind,
			&t.TargetNum, &t.Unit, &t.SortOrder, &required, &t.Color, &t.Tracker,
			&hidden); err != nil {
			return nil, err
		}
		t.Required = required == 1
		t.Hidden = hidden == 1
		out = append(out, t)
	}
	return out, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanProgram(row scanner) (Program, error) {
	var p Program
	var strict int
	err := row.Scan(&p.ID, &p.Name, &p.StartDate, &p.LengthDays, &p.Status, &strict,
		&p.AttemptNumber, &p.DailyKcalTarget, &p.Notes, &p.CreatedAt, &p.EndedAt)
	p.StrictRestart = strict == 1
	return p, err
}

func validTaskKind(kind string) bool {
	switch kind {
	case program.KindCheck, program.KindNumber, program.KindDuration,
		program.KindPhoto, program.KindText:
		return true
	}
	return false
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func defaultString(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func slugify(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "task"
	}
	return out
}

// defaultPalette is the colour assigned to each task by position, so a fresh
// program's grids are distinguishable without the user picking anything.
var defaultPalette = []string{"#ff6b35", "#37d67a", "#4a9eff", "#ffd166", "#b47aea", "#ff5d8f"}

func paletteColor(i int) string {
	if i < 0 {
		i = 0
	}
	return defaultPalette[i%len(defaultPalette)]
}

// validHexColor accepts #rgb and #rrggbb, which is all the client sends.
func validHexColor(c string) bool {
	if len(c) != 4 && len(c) != 7 {
		return false
	}
	if c[0] != '#' {
		return false
	}
	for _, r := range c[1:] {
		isHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
		if !isHex {
			return false
		}
	}
	return true
}

// trackerOrNone validates a tracker name, falling back to none rather than
// rejecting a task over an optional extra.
func trackerOrNone(t string) string {
	if program.ValidTracker(t) {
		return t
	}
	return program.TrackerNone
}
