package api

import (
	"context"
	"database/sql"
	"math"
	"net/http"
	"time"

	"github.com/anchoo2kewl/75hard/api/internal/program"
	"go.uber.org/zap"
)

// VitalPoint is one day's optional readings.
//
// Weight and resting HR share a series because they are entered together and
// read together: the question is whether both are trending the right way.
type VitalPoint struct {
	DayNumber int      `json:"day_number"`
	Date      string   `json:"date"`
	WeightKg  *float64 `json:"weight_kg,omitempty"`
	RestingHR *int     `json:"resting_hr,omitempty"`
}

// Trend summarises one measurement over the attempt.
//
// First and Latest rather than min and max: the question a person actually
// has is "where did I start and where am I now", and Change answers it
// without the reader doing arithmetic.
type Trend struct {
	First   *float64 `json:"first,omitempty"`
	Latest  *float64 `json:"latest,omitempty"`
	Change  *float64 `json:"change,omitempty"`
	Best    *float64 `json:"best,omitempty"`
	Average *float64 `json:"average,omitempty"`
	Count   int      `json:"count"`
}

// HeartRatePoint is the heart-rate summary for one day's training.
type HeartRatePoint struct {
	DayNumber int     `json:"day_number"`
	Date      string  `json:"date"`
	AverageHR float64 `json:"average_hr"`
	MaxHR     float64 `json:"max_hr"`
	Minutes   int     `json:"minutes"`
}

// Summary is the whole picture for the main page.
type Summary struct {
	ProgramID    int64   `json:"program_id"`
	CurrentDay   int     `json:"current_day"`
	LengthDays   int     `json:"length_days"`
	DaysComplete int     `json:"days_complete"`
	DaysMissed   int     `json:"days_missed"`
	Streak       int     `json:"streak"`
	BestStreak   int     `json:"best_streak"`
	PercentDone  float64 `json:"percent_done"`

	TotalPhotos       int     `json:"total_photos"`
	TotalWorkouts     int     `json:"total_workouts"`
	TotalMinutes      int     `json:"total_minutes"`
	OutdoorMinutes    int     `json:"outdoor_minutes"`
	MeditationMinutes int     `json:"meditation_minutes"`
	AvgKcal           float64 `json:"avg_kcal"`

	Vitals    []VitalPoint     `json:"vitals"`
	Weight    Trend            `json:"weight"`
	RestingHR Trend            `json:"resting_hr"`
	HeartRate []HeartRatePoint `json:"heart_rate"`
	// ActivityHR is the trend in average heart rate across training, which is
	// the fitness signal Strava can actually supply.
	ActivityHR Trend `json:"activity_hr"`
}

// HandleGetSummary returns everything the main page shows in one call.
//
// One endpoint rather than five, because the main page renders all of it at
// once and five round trips on a phone is the difference between the page
// appearing and the page assembling itself in front of you.
func (s *Server) HandleGetSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := UserID(ctx)

	programID, startDate, length, ok := s.activeProgramWindow(ctx, r)
	if !ok {
		respondError(w, http.StatusNotFound, "no active program", "no_program")
		return
	}

	out := Summary{
		ProgramID:  programID,
		LengthDays: length,
		Vitals:     []VitalPoint{},
		HeartRate:  []HeartRatePoint{},
	}

	_ = s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM days WHERE program_id = ? AND status = 'complete'),
			(SELECT COUNT(*) FROM days WHERE program_id = ? AND status = 'missed'),
			(SELECT COUNT(*) FROM photos WHERE program_id = ?)`,
		programID, programID, programID).
		Scan(&out.DaysComplete, &out.DaysMissed, &out.TotalPhotos)

	_ = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(minutes), 0),
		       COALESCE(SUM(CASE WHEN kind = 'outdoor' THEN minutes ELSE 0 END), 0)
		  FROM workouts WHERE day_id IN (SELECT id FROM days WHERE program_id = ?)`,
		programID).Scan(&out.TotalWorkouts, &out.TotalMinutes, &out.OutdoorMinutes)

	_ = s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(minutes), 0) FROM meditation_sessions
		 WHERE day_id IN (SELECT id FROM days WHERE program_id = ?)`,
		programID).Scan(&out.MeditationMinutes)

	// Averaged over days that actually logged food; dividing by every elapsed
	// day would report a misleadingly low intake.
	_ = s.db.QueryRowContext(ctx, `
		SELECT COALESCE(AVG(day_kcal), 0) FROM (
			SELECT SUM(kcal) AS day_kcal FROM meals
			 WHERE day_id IN (SELECT id FROM days WHERE program_id = ?)
			 GROUP BY day_id)`, programID).Scan(&out.AvgKcal)

	out.CurrentDay, out.Streak, out.BestStreak = s.programProgress(ctx, r, programID, startDate, length)
	if length > 0 {
		out.PercentDone = math.Round(float64(out.DaysComplete)/float64(length)*1000) / 10
	}

	out.Vitals = s.vitalSeries(ctx, programID)

	var weights, restingHRs []float64
	for _, v := range out.Vitals {
		if v.WeightKg != nil {
			weights = append(weights, *v.WeightKg)
		}
		if v.RestingHR != nil {
			restingHRs = append(restingHRs, float64(*v.RestingHR))
		}
	}
	out.Weight = trendOf(weights)
	out.RestingHR = trendOf(restingHRs)

	out.HeartRate = s.heartRateSeries(ctx, userID, programID)
	activityHRs := make([]float64, 0, len(out.HeartRate))
	for _, p := range out.HeartRate {
		activityHRs = append(activityHRs, p.AverageHR)
	}
	out.ActivityHR = trendOf(activityHRs)

	respondJSON(w, http.StatusOK, out)
}

// programProgress reads the current day and both streaks, from the same day
// statuses the calendar renders.
func (s *Server) programProgress(
	ctx context.Context, r *http.Request, programID int64, startDate string, length int,
) (current, streak, best int) {
	today := program.LocalDate(time.Now(), s.userLocation(r))
	current = program.DayNumber(startDate, today)
	if current > length {
		current = length
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT day_number, status FROM days WHERE program_id = ?`, programID)
	if err != nil {
		s.log.Error("summary day statuses", zap.Error(err))
		return current, 0, 0
	}
	defer rows.Close()

	statuses := map[int]string{}
	for rows.Next() {
		var n int
		var st string
		if err := rows.Scan(&n, &st); err != nil {
			return current, 0, 0
		}
		statuses[n] = st
	}

	streak = program.Streak(statuses, current)
	for d := 1; d <= length; d++ {
		if run := program.Streak(statuses, d); run > best {
			best = run
		}
	}
	return current, streak, best
}

// vitalSeries returns every day that has a weight or a resting pulse.
func (s *Server) vitalSeries(ctx context.Context, programID int64) []VitalPoint {
	rows, err := s.db.QueryContext(ctx, `
		SELECT day_number, on_date, weight_kg, resting_hr
		  FROM days
		 WHERE program_id = ? AND (weight_kg IS NOT NULL OR resting_hr IS NOT NULL)
		 ORDER BY day_number`, programID)
	if err != nil {
		s.log.Error("vital series", zap.Error(err))
		return []VitalPoint{}
	}
	defer rows.Close()

	out := []VitalPoint{}
	for rows.Next() {
		var p VitalPoint
		var weight sql.NullFloat64
		var hr sql.NullInt64
		if err := rows.Scan(&p.DayNumber, &p.Date, &weight, &hr); err != nil {
			return out
		}
		if weight.Valid {
			v := weight.Float64
			p.WeightKg = &v
		}
		if hr.Valid {
			v := int(hr.Int64)
			p.RestingHR = &v
		}
		out = append(out, p)
	}
	return out
}

// heartRateSeries summarises training heart rate per day.
//
// Weighted by moving time rather than a plain average of activities, so a
// ten-minute warm-up does not count as much as an hour-long walk.
func (s *Server) heartRateSeries(ctx context.Context, userID, programID int64) []HeartRatePoint {
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.day_number, d.on_date,
		       SUM(a.average_hr * a.moving_seconds) / NULLIF(SUM(a.moving_seconds), 0),
		       MAX(a.max_hr),
		       SUM(a.moving_seconds) / 60
		  FROM strava_activities a
		  JOIN days d ON d.id = a.day_id
		 WHERE a.user_id = ? AND d.program_id = ? AND a.average_hr IS NOT NULL
		 GROUP BY d.day_number, d.on_date
		 ORDER BY d.day_number`, userID, programID)
	if err != nil {
		s.log.Error("heart rate series", zap.Error(err))
		return []HeartRatePoint{}
	}
	defer rows.Close()

	out := []HeartRatePoint{}
	for rows.Next() {
		var p HeartRatePoint
		var avg, max sql.NullFloat64
		if err := rows.Scan(&p.DayNumber, &p.Date, &avg, &max, &p.Minutes); err != nil {
			return out
		}
		if !avg.Valid {
			continue
		}
		p.AverageHR = math.Round(avg.Float64*10) / 10
		p.MaxHR = max.Float64
		out = append(out, p)
	}
	return out
}

// trendOf summarises a series of readings in chronological order.
//
// Best is the lowest value, because every measurement fed through here
// improves downward: a falling weight, a falling resting pulse, and a falling
// average training heart rate are all progress.
func trendOf(values []float64) Trend {
	var t Trend
	var sum float64

	for _, value := range values {
		t.Count++
		sum += value

		if t.First == nil {
			f := value
			t.First = &f
		}
		l := value
		t.Latest = &l
		if t.Best == nil || value < *t.Best {
			b := value
			t.Best = &b
		}
	}

	if t.Count == 0 {
		return t
	}
	avg := math.Round(sum/float64(t.Count)*10) / 10
	t.Average = &avg
	if t.First != nil && t.Latest != nil {
		c := math.Round((*t.Latest-*t.First)*10) / 10
		t.Change = &c
	}
	return t
}
