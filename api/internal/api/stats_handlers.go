package api

import (
	"net/http"
	"time"

	"github.com/anchoo2kewl/75hard/api/internal/program"
)

// Stats is the dashboard summary for the active program.
type Stats struct {
	ProgramID      int64         `json:"program_id"`
	CurrentDay     int           `json:"current_day"`
	LengthDays     int           `json:"length_days"`
	DaysComplete   int           `json:"days_complete"`
	DaysMissed     int           `json:"days_missed"`
	Streak         int           `json:"streak"`
	BestStreak     int           `json:"best_streak"`
	PercentDone    float64       `json:"percent_done"`
	TotalPhotos    int           `json:"total_photos"`
	TotalWorkouts  int           `json:"total_workouts"`
	TotalMinutes   int           `json:"total_minutes"`
	AvgKcal        float64       `json:"avg_kcal"`
	TaskCompletion []TaskStat    `json:"task_completion"`
	WeightSeries   []WeightPoint `json:"weight_series"`
}

// TaskStat is how reliably one task has been completed across the attempt.
type TaskStat struct {
	TaskID    int64   `json:"task_id"`
	Title     string  `json:"title"`
	Icon      string  `json:"icon"`
	Completed int     `json:"completed"`
	Rate      float64 `json:"rate"`
}

// WeightPoint is one weigh-in.
type WeightPoint struct {
	DayNumber int     `json:"day_number"`
	Date      string  `json:"date"`
	WeightKg  float64 `json:"weight_kg"`
}

// HandleGetStats returns the dashboard summary for the active program.
func (s *Server) HandleGetStats(w http.ResponseWriter, r *http.Request) {
	programID, err := s.activeProgramID(r)
	if err != nil {
		respondError(w, http.StatusNotFound, "no active program", "no_active_program")
		return
	}
	ctx := r.Context()

	var startDate string
	stats := Stats{ProgramID: programID}
	if err := s.db.QueryRowContext(ctx,
		`SELECT start_date, length_days FROM programs WHERE id = ?`, programID).
		Scan(&startDate, &stats.LengthDays); err != nil {
		respondError(w, http.StatusInternalServerError, "could not load program", "internal")
		return
	}

	today := program.LocalDate(time.Now(), s.userLocation(r))
	stats.CurrentDay = program.DayNumber(startDate, today)
	if stats.CurrentDay > stats.LengthDays {
		stats.CurrentDay = stats.LengthDays
	}

	statuses := map[int]string{}
	rows, err := s.db.QueryContext(ctx,
		`SELECT day_number, status FROM days WHERE program_id = ?`, programID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load days", "internal")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var n int
		var st string
		if err := rows.Scan(&n, &st); err != nil {
			respondError(w, http.StatusInternalServerError, "could not read days", "internal")
			return
		}
		statuses[n] = st
		switch st {
		case program.StatusComplete:
			stats.DaysComplete++
		case program.StatusMissed:
			stats.DaysMissed++
		}
	}

	stats.Streak = program.Streak(statuses, stats.CurrentDay)
	for d := 1; d <= stats.LengthDays; d++ {
		if run := program.Streak(statuses, d); run > stats.BestStreak {
			stats.BestStreak = run
		}
	}
	if stats.LengthDays > 0 {
		stats.PercentDone = float64(stats.DaysComplete) / float64(stats.LengthDays) * 100
	}

	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM photos WHERE program_id = ?`, programID).Scan(&stats.TotalPhotos)
	_ = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(minutes), 0) FROM workouts
		WHERE day_id IN (SELECT id FROM days WHERE program_id = ?)`, programID).
		Scan(&stats.TotalWorkouts, &stats.TotalMinutes)

	// Average over days that actually have meals logged — dividing by every
	// elapsed day would report a misleadingly low intake.
	_ = s.db.QueryRowContext(ctx, `
		SELECT COALESCE(AVG(day_kcal), 0) FROM (
			SELECT SUM(kcal) AS day_kcal FROM meals
			WHERE day_id IN (SELECT id FROM days WHERE program_id = ?)
			GROUP BY day_id
		)`, programID).Scan(&stats.AvgKcal)

	stats.TaskCompletion = []TaskStat{}
	// As in the calendar, a photo task is satisfied by an upload rather than
	// by a completed task_entry, so the days it was met on have to be counted
	// separately or it reports 0% on a day it plainly completed.
	trows, err := s.db.QueryContext(ctx, `
		SELECT pt.id, pt.title, pt.icon,
		       (SELECT COUNT(*) FROM task_entries te
		        JOIN days d ON d.id = te.day_id
		        WHERE te.program_task_id = pt.id AND te.completed_at IS NOT NULL AND d.program_id = ?)
		     + (CASE WHEN pt.kind = 'photo' THEN
		            (SELECT COUNT(*) FROM days d2
		             WHERE d2.program_id = ?
		               AND EXISTS (SELECT 1 FROM photos p
		                           WHERE p.day_id = d2.id AND p.kind = 'progress')
		               AND NOT EXISTS (SELECT 1 FROM task_entries te2
		                               WHERE te2.day_id = d2.id AND te2.program_task_id = pt.id
		                                 AND te2.completed_at IS NOT NULL))
		        ELSE 0 END)
		FROM program_tasks pt WHERE pt.program_id = ? ORDER BY pt.sort_order, pt.id`,
		programID, programID, programID)
	if err == nil {
		defer trows.Close()
		elapsed := stats.CurrentDay
		if elapsed < 1 {
			elapsed = 1
		}
		for trows.Next() {
			var ts TaskStat
			if err := trows.Scan(&ts.TaskID, &ts.Title, &ts.Icon, &ts.Completed); err != nil {
				continue
			}
			ts.Rate = float64(ts.Completed) / float64(elapsed) * 100
			if ts.Rate > 100 {
				ts.Rate = 100
			}
			stats.TaskCompletion = append(stats.TaskCompletion, ts)
		}
	}

	stats.WeightSeries = []WeightPoint{}
	wrows, err := s.db.QueryContext(ctx,
		`SELECT day_number, on_date, weight_kg FROM days
		 WHERE program_id = ? AND weight_kg IS NOT NULL ORDER BY day_number`, programID)
	if err == nil {
		defer wrows.Close()
		for wrows.Next() {
			var p WeightPoint
			if err := wrows.Scan(&p.DayNumber, &p.Date, &p.WeightKg); err == nil {
				stats.WeightSeries = append(stats.WeightSeries, p)
			}
		}
	}

	respondJSON(w, http.StatusOK, stats)
}
