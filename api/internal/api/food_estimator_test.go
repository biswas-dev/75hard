package api

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestSlotForTime(t *testing.T) {
	tests := []struct {
		hour int
		want string
	}{
		// Before dawn is the tail of last night, not the start of today.
		{2, "snack"},
		{7, "breakfast"},
		{10, "breakfast"},
		{12, "lunch"},
		{14, "lunch"},
		{16, "snack"},
		{19, "dinner"},
		{21, "dinner"},
		{23, "snack"},
	}
	for _, tc := range tests {
		at := time.Date(2026, 9, 2, tc.hour, 30, 0, 0, time.UTC)
		if got := slotForTime(at); got != tc.want {
			t.Errorf("%02d:30 -> %q, want %q", tc.hour, got, tc.want)
		}
	}
}

func TestSlotForTimeAlwaysReturnsAValidSlot(t *testing.T) {
	// The guess is only a default, but it is written straight to a column the
	// rest of the app reads, so it must never produce something unknown.
	for h := 0; h < 24; h++ {
		got := slotForTime(time.Date(2026, 9, 2, h, 0, 0, 0, time.UTC))
		if !validSlot(got) {
			t.Errorf("hour %d produced invalid slot %q", h, got)
		}
	}
}

func TestEnqueueDoesNotBlockWhenTheQueueIsFull(t *testing.T) {
	// A full queue must never stall an upload; the meal stays pending and the
	// next restart collects it.
	e := &FoodEstimator{
		// Only the logger is reached on the full-queue path.
		srv:  &Server{log: zap.NewNop()},
		jobs: make(chan estimateJob, 1),
		done: make(chan struct{}),
	}

	if !e.Enqueue(estimateJob{mealID: 1}) {
		t.Fatal("first job should fit")
	}

	done := make(chan bool, 1)
	go func() { done <- e.Enqueue(estimateJob{mealID: 2}) }()

	select {
	case accepted := <-done:
		if accepted {
			t.Error("a full queue reported the job as accepted")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Enqueue blocked on a full queue")
	}
}
