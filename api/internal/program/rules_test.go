package program

import (
	"slices"
	"testing"
)

func TestDayNumber(t *testing.T) {
	tests := []struct {
		start, on string
		want      int
	}{
		{"2026-01-01", "2026-01-01", 1},
		{"2026-01-01", "2026-01-02", 2},
		{"2026-01-01", "2026-03-16", 75},
		{"2026-01-01", "2025-12-31", 0}, // before the start
		// Spans a DST transition in a northern-hemisphere spring; dates are
		// stored UTC-parsed so the arithmetic must not drift by a day.
		{"2026-03-01", "2026-03-31", 31},
		{"2026-02-01", "2026-03-01", 29}, // 2026 is not a leap year
	}
	for _, tc := range tests {
		if got := DayNumber(tc.start, tc.on); got != tc.want {
			t.Errorf("DayNumber(%q, %q) = %d, want %d", tc.start, tc.on, got, tc.want)
		}
	}
}

func TestDateForDay(t *testing.T) {
	if got := DateForDay("2026-01-01", 75); got != "2026-03-16" {
		t.Errorf("DateForDay day 75 = %q, want 2026-03-16", got)
	}
	if got := DateForDay("2026-01-01", 1); got != "2026-01-01" {
		t.Errorf("DateForDay day 1 = %q, want 2026-01-01", got)
	}
}

func TestEntrySatisfies(t *testing.T) {
	target := 128.0
	partial := 60.0
	met := 128.0

	tests := []struct {
		name string
		task Task
		e    Entry
		want bool
	}{
		{"check ticked", Task{Kind: KindCheck}, Entry{Completed: true}, true},
		{"check untouched", Task{Kind: KindCheck}, Entry{}, false},
		{"number short of target", Task{Kind: KindNumber, TargetNum: &target}, Entry{ValueNum: &partial}, false},
		{"number at target", Task{Kind: KindNumber, TargetNum: &target}, Entry{ValueNum: &met}, true},
		{"number with no value", Task{Kind: KindNumber, TargetNum: &target}, Entry{}, false},
		// A number task with no target falls back to the manual tick, rather
		// than being impossible to complete.
		{"number without target falls back to tick", Task{Kind: KindNumber}, Entry{Completed: true}, true},
		{"photo satisfied by an upload", Task{Kind: KindPhoto}, Entry{PhotoCount: 1}, true},
		{"photo with nothing uploaded", Task{Kind: KindPhoto}, Entry{}, false},
	}
	for _, tc := range tests {
		if got := EntrySatisfies(tc.task, tc.e); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestDayCompleteIgnoresOptionalTasks(t *testing.T) {
	tasks := []Task{
		{ID: 1, Kind: KindCheck, Required: true},
		{ID: 2, Kind: KindCheck, Required: true},
		{ID: 3, Kind: KindCheck, Required: false},
	}
	entries := map[int64]Entry{
		1: {Completed: true},
		2: {Completed: true},
	}

	complete, done, required := DayComplete(tasks, entries)
	if !complete {
		t.Error("day with both required tasks done should be complete")
	}
	if required != 2 {
		t.Errorf("required = %d, want 2 (optional task must not count)", required)
	}
	if done != 2 {
		t.Errorf("done = %d, want 2", done)
	}
}

func TestDayCompleteWithNoTasks(t *testing.T) {
	// An empty template must not report a vacuously complete day.
	if complete, _, _ := DayComplete(nil, nil); complete {
		t.Error("a program with no tasks should never report a complete day")
	}
}

func TestEvaluateDay(t *testing.T) {
	tests := []struct {
		name                 string
		onDate, today        string
		dayNumber, length    int
		complete, strict     bool
		wantDay, wantProgram string
		wantRestart          bool
	}{
		{
			name:   "unfinished but still today is pending, not missed",
			onDate: "2026-01-10", today: "2026-01-10", dayNumber: 10, length: 75,
			wantDay: StatusPending,
		},
		{
			name:   "unfinished and in the past fails a strict program",
			onDate: "2026-01-09", today: "2026-01-10", dayNumber: 9, length: 75,
			strict:  true,
			wantDay: StatusMissed, wantProgram: ProgramFailed, wantRestart: true,
		},
		{
			name:   "unfinished and in the past only marks the day when lenient",
			onDate: "2026-01-09", today: "2026-01-10", dayNumber: 9, length: 75,
			strict:  false,
			wantDay: StatusMissed,
		},
		{
			name:   "finishing the final day completes the program",
			onDate: "2026-03-16", today: "2026-03-16", dayNumber: 75, length: 75,
			complete: true,
			wantDay:  StatusComplete, wantProgram: ProgramCompleted,
		},
		{
			name:   "finishing a mid-program day leaves the program alone",
			onDate: "2026-01-10", today: "2026-01-10", dayNumber: 10, length: 75,
			complete: true,
			wantDay:  StatusComplete,
		},
		{
			name:   "a future day is pending",
			onDate: "2026-01-11", today: "2026-01-10", dayNumber: 11, length: 75,
			strict:  true,
			wantDay: StatusPending,
		},
	}

	for _, tc := range tests {
		got := EvaluateDay(tc.onDate, tc.today, tc.dayNumber, tc.length, tc.complete, tc.strict)
		if got.DayStatus != tc.wantDay {
			t.Errorf("%s: DayStatus = %q, want %q", tc.name, got.DayStatus, tc.wantDay)
		}
		if got.ProgramStatus != tc.wantProgram {
			t.Errorf("%s: ProgramStatus = %q, want %q", tc.name, got.ProgramStatus, tc.wantProgram)
		}
		if got.ShouldRestart != tc.wantRestart {
			t.Errorf("%s: ShouldRestart = %v, want %v", tc.name, got.ShouldRestart, tc.wantRestart)
		}
	}
}

func TestStreak(t *testing.T) {
	statuses := map[int]string{
		1: StatusComplete, 2: StatusComplete, 3: StatusMissed,
		4: StatusComplete, 5: StatusComplete, 6: StatusComplete,
	}
	if got := Streak(statuses, 6); got != 3 {
		t.Errorf("Streak = %d, want 3 (a miss at day 3 breaks it)", got)
	}
	if got := Streak(statuses, 2); got != 2 {
		t.Errorf("Streak through day 2 = %d, want 2", got)
	}
	if got := Streak(map[int]string{1: StatusPending}, 1); got != 0 {
		t.Errorf("Streak with a pending day = %d, want 0", got)
	}
}

func TestLocalDateUsesUserTimezone(t *testing.T) {
	// 03:30 UTC on the 11th is still the 10th in Toronto. Checking a task off
	// late at night must land on the day the user thinks it is.
	ts, err := ParseDate("2026-01-11")
	if err != nil {
		t.Fatal(err)
	}
	ts = ts.Add(3*60*60*1000*1000*1000 + 30*60*1000*1000*1000)

	toronto := LoadLocation("America/Toronto")
	if got := LocalDate(ts, toronto); got != "2026-01-10" {
		t.Errorf("LocalDate in Toronto = %q, want 2026-01-10", got)
	}
	if got := LocalDate(ts, LoadLocation("UTC")); got != "2026-01-11" {
		t.Errorf("LocalDate in UTC = %q, want 2026-01-11", got)
	}
}

func TestLoadLocationFallsBackToUTC(t *testing.T) {
	if loc := LoadLocation("Not/AZone"); loc != nil && loc.String() != "UTC" {
		t.Errorf("bad timezone should fall back to UTC, got %v", loc)
	}
}

func TestDefaultTasksAreTheCanonicalSix(t *testing.T) {
	// The template may grow optional extras, but the canonical six must all be
	// present and all be required — that is what makes the challenge itself.
	canonical := []string{
		"workout_indoor", "workout_outdoor", "diet",
		"water", "reading", "progress_photo",
	}

	tasks := DefaultTasks()
	seen := map[string]DefaultTask{}
	for _, task := range tasks {
		if _, dup := seen[task.Key]; dup {
			t.Errorf("duplicate task key %q", task.Key)
		}
		seen[task.Key] = task
	}

	for _, key := range canonical {
		task, ok := seen[key]
		if !ok {
			t.Errorf("canonical task %q is missing from the template", key)
			continue
		}
		if !task.Required {
			t.Errorf("canonical task %q should be required", key)
		}
	}

	// Anything else is an addition, and additions must never fail a run.
	for key, task := range seen {
		if !slices.Contains(canonical, key) && task.Required {
			t.Errorf("non-canonical task %q is required; it must be optional", key)
		}
	}
}

func TestOptionalTaskCannotCompleteTheDay(t *testing.T) {
	// The trap: "done" counts optional tasks so finishing one is visible, but
	// comparing that total against the required count would let a meditation
	// session stand in for the workout that was never done.
	tasks := []Task{
		{ID: 1, Kind: KindCheck, Required: true},
		{ID: 2, Kind: KindCheck, Required: true},
		{ID: 3, Kind: KindCheck, Required: false}, // meditation
	}
	entries := map[int64]Entry{
		1: {TaskID: 1, Completed: true},
		3: {TaskID: 3, Completed: true}, // optional done, required task 2 is not
	}

	complete, done, required := DayComplete(tasks, entries)
	if complete {
		t.Error("day reported complete with a required task outstanding")
	}
	if required != 2 {
		t.Errorf("required = %d, want 2 — the optional task must not be counted", required)
	}
	if done != 1 {
		t.Errorf("done = %d, want 1 — only the finished required task counts", done)
	}
}

func TestOptionalTaskIsNotNeededForCompletion(t *testing.T) {
	// The other half: skipping meditation entirely must still complete the day.
	tasks := []Task{
		{ID: 1, Kind: KindCheck, Required: true},
		{ID: 2, Kind: KindCheck, Required: false},
	}
	complete, done, required := DayComplete(tasks, map[int64]Entry{1: {TaskID: 1, Completed: true}})
	if !complete {
		t.Error("an untouched optional task held the day back")
	}
	if done != 1 || required != 1 {
		t.Errorf("done/required = %d/%d, want 1/1", done, required)
	}

	// And a finished optional task must not push the ring past its own total.
	both := map[int64]Entry{1: {TaskID: 1, Completed: true}, 2: {TaskID: 2, Completed: true}}
	if _, done, required = DayComplete(tasks, both); done != 1 || required != 1 {
		t.Errorf("done/required = %d/%d, want 1/1 — the ring must never exceed its total", done, required)
	}
}

func TestMeditationIsOptionalByDefault(t *testing.T) {
	var found bool
	for _, task := range DefaultTasks() {
		if task.Key != "meditation" {
			continue
		}
		found = true
		if task.Required {
			t.Error("meditation is required; missing it must never fail a run")
		}
		if task.Tracker != TrackerMeditation {
			t.Errorf("tracker = %q, want %q", task.Tracker, TrackerMeditation)
		}
	}
	if !found {
		t.Fatal("no meditation task in the default template")
	}

}

func TestJournalIsOptionalByDefault(t *testing.T) {
	var found bool
	for _, task := range DefaultTasks() {
		if task.Key != "journal" {
			continue
		}
		found = true
		if task.Required {
			t.Error("journal is required; missing it must never fail a run")
		}
		if task.Tracker != TrackerJournal {
			t.Errorf("tracker = %q, want %q", task.Tracker, TrackerJournal)
		}
	}
	if !found {
		t.Fatal("no journal task in the default template")
	}
}
