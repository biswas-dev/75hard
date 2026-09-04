package api

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/anchoo2kewl/75hard/api/internal/db"
	"go.uber.org/zap"
)

// TestWorkoutQueriesMatchTheScanner guards the two queries that feed
// scanWorkout against drifting apart.
//
// They already did: a column was added to one and not the other, so creating a
// workout inserted the row and then failed to read it back. The write had
// happened, the response was a 500, and the app showed nothing at all — the
// worst shape a bug can take.
func TestWorkoutQueriesMatchTheScanner(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer database.Close()

	if _, err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ctx := context.Background()
	s := &Server{db: database, log: zap.NewNop()}

	if _, err := database.ExecContext(ctx,
		`INSERT INTO users (id, email, name, password_hash) VALUES (1, 'a@b.c', 'A', 'x')`); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := database.ExecContext(ctx,
		`INSERT INTO programs (id, user_id, name, start_date, length_days)
		 VALUES (1, 1, 'p', '2026-09-01', 75)`); err != nil {
		t.Fatalf("seed program: %v", err)
	}
	if _, err := database.ExecContext(ctx,
		`INSERT INTO days (id, program_id, day_number, on_date) VALUES (1, 1, 1, '2026-09-01')`); err != nil {
		t.Fatalf("seed day: %v", err)
	}
	res, err := database.ExecContext(ctx,
		`INSERT INTO workouts (user_id, day_id, kind, activity, minutes) VALUES (1, 1, 'outdoor', 'Stretch', 10)`)
	if err != nil {
		t.Fatalf("seed workout: %v", err)
	}
	id, _ := res.LastInsertId()

	// The read-back after a create.
	wo, err := s.workoutByID(ctx, id)
	if err != nil {
		t.Fatalf("workoutByID: %v", err)
	}
	if wo.Activity != "Stretch" || wo.Minutes != 10 {
		t.Errorf("workoutByID returned %+v", wo)
	}

	// The list the day is rendered from.
	list, err := s.workoutsForDay(ctx, 1)
	if err != nil {
		t.Fatalf("workoutsForDay: %v", err)
	}
	if len(list) != 1 || list[0].Session != 1 {
		t.Errorf("workoutsForDay returned %+v", list)
	}
}
