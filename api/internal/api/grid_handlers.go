package api

import (
	"net/http"
	"time"

	"github.com/anchoo2kewl/75hard/api/internal/program"
)

// Cell states in the activity grid. Kept as short strings because there is one
// per task per day and they all go over the wire together.
const (
	CellEmpty   = "" // nothing logged
	CellDone    = "d"
	CellPartial = "p" // progress toward a target, short of it
	CellMissed  = "m" // the day passed without this being completed
	CellFuture  = "f"
)

// GridTask is one activity's row of the grid.
type GridTask struct {
	TaskID    int64    `json:"task_id"`
	Key       string   `json:"key"`
	Title     string   `json:"title"`
	Icon      string   `json:"icon"`
	Kind      string   `json:"kind"`
	Unit      string   `json:"unit"`
	TargetNum *float64 `json:"target_num"`
	Required  bool     `json:"required"`
	Color     string   `json:"color"`

	// Cells is one entry per day of the program, index 0 == day 1.
	Cells []string `json:"cells"`
	// Values carries the logged amount for metered tasks, so a cell can show
	// how far along it got. Sparse: only days with a value appear.
	Values map[string]float64 `json:"values"`

	Completed  int `json:"completed"`
	Streak     int `json:"streak"`
	BestStreak int `json:"best_streak"`
}

// GridResponse is the whole activity grid for a program.
type GridResponse struct {
	ProgramID  int64      `json:"program_id"`
	StartDate  string     `json:"start_date"`
	LengthDays int        `json:"length_days"`
	CurrentDay int        `json:"current_day"`
	Today      string     `json:"today"`
	Tasks      []GridTask `json:"tasks"`
	// DayStatus is the overall per-day outcome, for the summary row.
	DayStatus []string `json:"day_status"`
}

// HandleGetGrid returns a task-by-day matrix for the whole program.
//
// Rendering 6 activities across 75 days is 450 cells; fetching them per cell
// would be 450 round trips, so the entries are read in one pass and pivoted in
// memory.
func (s *Server) HandleGetGrid(w http.ResponseWriter, r *http.Request) {
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

	today := program.LocalDate(time.Now(), s.userLocation(r))
	currentDay := program.DayNumber(startDate, today)

	out := GridResponse{
		ProgramID:  programID,
		StartDate:  startDate,
		LengthDays: length,
		CurrentDay: currentDay,
		Today:      today,
		Tasks:      []GridTask{},
		DayStatus:  make([]string, length),
	}

	// Day numbers keyed by id, so entries can be placed without a join back.
	dayNumberByID := map[int64]int{}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, day_number, status FROM days WHERE program_id = ?`, programID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load days", "internal")
		return
	}
	for rows.Next() {
		var id int64
		var n int
		var status string
		if err := rows.Scan(&id, &n, &status); err != nil {
			rows.Close()
			respondError(w, http.StatusInternalServerError, "could not read days", "internal")
			return
		}
		dayNumberByID[id] = n
		if n >= 1 && n <= length {
			out.DayStatus[n-1] = status
		}
	}
	rows.Close()

	// Days with a progress photo, which is what satisfies a photo task.
	photoDays := map[int]bool{}
	prows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT d.day_number
		FROM photos p JOIN days d ON d.id = p.day_id
		WHERE d.program_id = ? AND p.kind = 'progress'`, programID)
	if err == nil {
		for prows.Next() {
			var n int
			if err := prows.Scan(&n); err == nil {
				photoDays[n] = true
			}
		}
		prows.Close()
	}

	// Every entry in the program, in one pass.
	type entryKey struct {
		taskID int64
		day    int
	}
	type entryVal struct {
		done  bool
		value *float64
	}
	entries := map[entryKey]entryVal{}

	erows, err := s.db.QueryContext(ctx, `
		SELECT te.program_task_id, te.day_id, te.completed_at IS NOT NULL, te.value_num
		FROM task_entries te
		JOIN days d ON d.id = te.day_id
		WHERE d.program_id = ?`, programID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load entries", "internal")
		return
	}
	for erows.Next() {
		var taskID, dayID int64
		var done bool
		var value *float64
		if err := erows.Scan(&taskID, &dayID, &done, &value); err != nil {
			erows.Close()
			respondError(w, http.StatusInternalServerError, "could not read entries", "internal")
			return
		}
		if n, ok := dayNumberByID[dayID]; ok {
			entries[entryKey{taskID, n}] = entryVal{done: done, value: value}
		}
	}
	erows.Close()

	trows, err := s.db.QueryContext(ctx, `
		SELECT id, task_key, title, icon, kind, unit, target_num, required, color
		FROM program_tasks WHERE program_id = ? ORDER BY sort_order, id`, programID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load tasks", "internal")
		return
	}
	defer trows.Close()

	for trows.Next() {
		var t GridTask
		var required int
		if err := trows.Scan(&t.TaskID, &t.Key, &t.Title, &t.Icon, &t.Kind,
			&t.Unit, &t.TargetNum, &required, &t.Color); err != nil {
			respondError(w, http.StatusInternalServerError, "could not read tasks", "internal")
			return
		}
		t.Required = required == 1
		if t.Color == "" {
			t.Color = "#ff6b35"
		}
		t.Cells = make([]string, length)
		t.Values = map[string]float64{}

		run := 0
		for day := 1; day <= length; day++ {
			e := entries[entryKey{t.TaskID, day}]

			pe := program.Entry{TaskID: t.TaskID, Completed: e.done, ValueNum: e.value}
			if t.Kind == program.KindPhoto && photoDays[day] {
				pe.PhotoCount = 1
			}
			satisfied := program.EntrySatisfies(
				program.Task{ID: t.TaskID, Kind: t.Kind, TargetNum: t.TargetNum, Required: t.Required}, pe)

			if e.value != nil {
				t.Values[itoa(day)] = *e.value
			}

			switch {
			case satisfied:
				t.Cells[day-1] = CellDone
				t.Completed++
				run++
				if run > t.BestStreak {
					t.BestStreak = run
				}
			case currentDay >= 1 && day > currentDay:
				t.Cells[day-1] = CellFuture
			case currentDay >= 1 && day < currentDay:
				// The day has passed without this being done.
				if e.value != nil && *e.value > 0 {
					t.Cells[day-1] = CellPartial
				} else {
					t.Cells[day-1] = CellMissed
				}
				run = 0
			case currentDay < 1:
				// The program hasn't started; nothing is missed yet.
				t.Cells[day-1] = CellFuture
			default:
				// Today, still in progress.
				if e.value != nil && *e.value > 0 {
					t.Cells[day-1] = CellPartial
				} else {
					t.Cells[day-1] = CellEmpty
				}
			}
		}

		// The streak is the run ending at the most recent elapsed day.
		t.Streak = 0
		for day := minInt(currentDay, length); day >= 1; day-- {
			if t.Cells[day-1] == CellDone {
				t.Streak++
				continue
			}
			// Today not yet done doesn't break a streak that is still alive.
			if day == currentDay {
				continue
			}
			break
		}

		out.Tasks = append(out.Tasks, t)
	}

	respondJSON(w, http.StatusOK, out)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
