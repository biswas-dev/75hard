package api

import "testing"

func TestTrendOf(t *testing.T) {
	// A weight series that fell, rose, then settled below where it started.
	tr := trendOf([]float64{82.5, 81.9, 82.1, 80.4})

	if tr.Count != 4 {
		t.Errorf("Count = %d, want 4", tr.Count)
	}
	if tr.First == nil || *tr.First != 82.5 {
		t.Errorf("First = %v, want 82.5", tr.First)
	}
	if tr.Latest == nil || *tr.Latest != 80.4 {
		t.Errorf("Latest = %v, want 80.4", tr.Latest)
	}
	// Down 2.1kg. Negative is the improvement, and the sign has to survive.
	if tr.Change == nil || *tr.Change != -2.1 {
		t.Errorf("Change = %v, want -2.1", tr.Change)
	}
	// Best is the lowest, not the most recent: these measurements improve down.
	if tr.Best == nil || *tr.Best != 80.4 {
		t.Errorf("Best = %v, want 80.4", tr.Best)
	}
	if tr.Average == nil || *tr.Average != 81.7 {
		t.Errorf("Average = %v, want 81.7", tr.Average)
	}
}

func TestTrendOfEmptyAndSingle(t *testing.T) {
	// Nothing logged yet: every field stays nil so the UI can show "no data"
	// rather than a confident zero.
	empty := trendOf(nil)
	if empty.Count != 0 || empty.First != nil || empty.Latest != nil ||
		empty.Change != nil || empty.Average != nil || empty.Best != nil {
		t.Errorf("an empty series produced %+v", empty)
	}

	// One reading is a start, not a trend: change is zero, not unknown.
	one := trendOf([]float64{75})
	if one.Count != 1 || one.First == nil || *one.First != 75 {
		t.Errorf("single = %+v", one)
	}
	if one.Change == nil || *one.Change != 0 {
		t.Errorf("Change = %v, want 0 for a single reading", one.Change)
	}
}

func TestTrendOfRisingSeries(t *testing.T) {
	// A resting pulse that went the wrong way must report a positive change,
	// so the UI can colour it as a regression rather than hiding it.
	tr := trendOf([]float64{52, 54, 57})
	if tr.Change == nil || *tr.Change != 5 {
		t.Errorf("Change = %v, want +5", tr.Change)
	}
	if tr.Best == nil || *tr.Best != 52 {
		t.Errorf("Best = %v, want the lowest reading 52", tr.Best)
	}
}
