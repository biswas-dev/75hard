// Package aifeatures turns a go-ai chain into the app's four AI features:
// estimating a meal from a photo, suggesting recipes, building a training and
// diet plan, and writing a short daily coaching note.
//
// The prompts live here as constants, the parsing is defensive, and every
// result is validated against what the app can actually store — a model is
// asked for JSON, not trusted to produce it.
package aifeatures

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	ai "github.com/anchoo2kewl/go-ai"
)

// Feature names, used for the run ledger and the cache key.
const (
	FeatureFoodPhoto = "food_photo"
	FeatureRecipes   = "recipes"
	FeaturePlan      = "plan"
	FeatureCoach     = "coach"
)

// Service performs the app's AI calls against a provider chain.
type Service struct {
	chain *ai.Chain
}

// New builds a Service. A nil chain is allowed and reports Enabled() == false,
// so the app runs normally with the AI features simply switched off.
func New(chain *ai.Chain) *Service { return &Service{chain: chain} }

// Enabled reports whether any provider is configured.
func (s *Service) Enabled() bool { return s != nil && s.chain != nil && s.chain.Len() > 0 }

// Providers lists the configured chain, primary first.
func (s *Service) Providers() []string {
	if !s.Enabled() {
		return nil
	}
	return s.chain.Names()
}

// ErrDisabled is returned when a feature is called with no provider set up.
var ErrDisabled = fmt.Errorf("aifeatures: no AI provider configured")

// Meta describes how a result was produced, for the ledger.
type Meta struct {
	Provider   string
	Model      string
	TokensIn   int
	TokensOut  int
	Attempts   int
	InputHash  string
	ResultJSON string
}

// ---- food photo ----

const foodSystemPrompt = `You estimate the nutritional content of a meal from a photograph.

Reply with a single JSON object and nothing else:
{"name":"short dish name","items":[{"name":"ingredient","qty":number,"unit":"g|ml|piece|serving","kcal":number,"protein_g":number,"carbs_g":number,"fat_g":number,"confidence":0.0-1.0}],"notes":"one short caveat"}

Rules:
- Estimate portion sizes from visible cues: plate size, utensils, the depth of the pile.
- One entry per distinguishable component. Do not exceed 12.
- kcal must be roughly consistent with the macros (protein 4, carbs 4, fat 9 per gram).
- confidence reflects how sure you are of BOTH the identification and the portion.
- If the photo does not show food, return {"name":"","items":[],"notes":"no food visible"}.
- Never invent a component you cannot see. An incomplete honest answer beats a confident wrong one.`

// FoodItem is one estimated component of a meal.
type FoodItem struct {
	Name       string  `json:"name"`
	Qty        float64 `json:"qty"`
	Unit       string  `json:"unit"`
	Kcal       float64 `json:"kcal"`
	ProteinG   float64 `json:"protein_g"`
	CarbsG     float64 `json:"carbs_g"`
	FatG       float64 `json:"fat_g"`
	Confidence float64 `json:"confidence"`
}

// FoodEstimate is a meal estimated from a photo.
type FoodEstimate struct {
	Name  string     `json:"name"`
	Items []FoodItem `json:"items"`
	Notes string     `json:"notes"`

	// Totals, summed here rather than asked of the model, so the breakdown and
	// the total can never disagree.
	Kcal     float64 `json:"kcal"`
	ProteinG float64 `json:"protein_g"`
	CarbsG   float64 `json:"carbs_g"`
	FatG     float64 `json:"fat_g"`
}

// EstimateFood identifies a meal from a photo and estimates its nutrition.
func (s *Service) EstimateFood(ctx context.Context, image []byte, mediaType, hint string) (*FoodEstimate, Meta, error) {
	if !s.Enabled() {
		return nil, Meta{}, ErrDisabled
	}

	prompt := "Estimate the food in this photo."
	if h := strings.TrimSpace(hint); h != "" {
		prompt += " The person says it is: " + h
	}

	resp, err := s.chain.Complete(ctx, ai.Request{
		System:    foodSystemPrompt,
		Messages:  []ai.Message{ai.UserImage(prompt, mediaType, image)},
		MaxTokens: 1600,
		JSON:      true,
	})
	if err != nil {
		return nil, Meta{}, err
	}

	var est FoodEstimate
	if err := ai.ExtractJSON(resp.Text, &est); err != nil {
		return nil, meta(resp, hashBytes(image, hint), ""), fmt.Errorf("aifeatures: could not read the estimate: %w", err)
	}

	// Drop anything unusable and recompute the totals from what survives.
	clean := est.Items[:0]
	for _, item := range est.Items {
		if strings.TrimSpace(item.Name) == "" {
			continue
		}
		if item.Kcal < 0 || item.Kcal > 5000 {
			continue
		}
		item.Name = strings.TrimSpace(item.Name)
		if item.Unit == "" {
			item.Unit = "serving"
		}
		if item.Qty <= 0 {
			item.Qty = 1
		}
		clean = append(clean, item)
		if len(clean) == 12 {
			break
		}
	}
	est.Items = clean

	est.Kcal, est.ProteinG, est.CarbsG, est.FatG = 0, 0, 0, 0
	for _, item := range est.Items {
		est.Kcal += item.Kcal
		est.ProteinG += item.ProteinG
		est.CarbsG += item.CarbsG
		est.FatG += item.FatG
	}
	est.Name = strings.TrimSpace(est.Name)

	return &est, meta(resp, hashBytes(image, hint), jsonString(est)), nil
}

// ---- recipes ----

const recipeSystemPrompt = `You suggest recipes that fit a person's remaining calorie and macro budget for the day.

Reply with a single JSON object and nothing else:
{"recipes":[{"name":"...","summary":"one sentence","minutes":number,"servings":number,"kcal_per_serving":number,"protein_g":number,"carbs_g":number,"fat_g":number,"ingredients":["200g chicken breast","..."],"steps":["...","..."]}]}

Rules:
- Return at most 3 recipes.
- Use the ingredients the person has where they are given; say plainly if something extra is needed.
- Respect the stated remaining budget. If it is very small, suggest something small rather than exceeding it.
- Keep steps to 6 or fewer, each one sentence.
- Give real quantities, not "some" or "to taste", for anything that affects the calorie count.`

// Recipe is one suggestion.
type Recipe struct {
	Name           string   `json:"name"`
	Summary        string   `json:"summary"`
	Minutes        int      `json:"minutes"`
	Servings       int      `json:"servings"`
	KcalPerServing float64  `json:"kcal_per_serving"`
	ProteinG       float64  `json:"protein_g"`
	CarbsG         float64  `json:"carbs_g"`
	FatG           float64  `json:"fat_g"`
	Ingredients    []string `json:"ingredients"`
	Steps          []string `json:"steps"`
}

// RecipeRequest is the context a suggestion is built from.
type RecipeRequest struct {
	RemainingKcal    float64
	RemainingProtein float64
	Ingredients      []string
	Preferences      string
	MealSlot         string
	// Image is an optional photo of what's in the fridge.
	Image     []byte
	MediaType string
}

// SuggestRecipes proposes meals that fit the day's remaining budget.
func (s *Service) SuggestRecipes(ctx context.Context, req RecipeRequest) ([]Recipe, Meta, error) {
	if !s.Enabled() {
		return nil, Meta{}, ErrDisabled
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Remaining today: %.0f kcal", req.RemainingKcal)
	if req.RemainingProtein > 0 {
		fmt.Fprintf(&b, ", %.0fg protein still to hit", req.RemainingProtein)
	}
	b.WriteString(".\n")
	if req.MealSlot != "" {
		fmt.Fprintf(&b, "This is for %s.\n", req.MealSlot)
	}
	if len(req.Ingredients) > 0 {
		fmt.Fprintf(&b, "Ingredients on hand: %s.\n", strings.Join(req.Ingredients, ", "))
	}
	if req.Image != nil {
		b.WriteString("A photo of the available ingredients is attached; work from what you can see in it.\n")
	}
	if p := strings.TrimSpace(req.Preferences); p != "" {
		fmt.Fprintf(&b, "Preferences and restrictions: %s\n", p)
	}
	b.WriteString("\nThis person is doing the 75 Hard challenge, so the food should be simple, high protein and strictly on-plan.")

	msg := ai.UserText(b.String())
	if req.Image != nil {
		msg = ai.UserImage(b.String(), mediaTypeOr(req.MediaType), req.Image)
	}

	resp, err := s.chain.Complete(ctx, ai.Request{
		System:    recipeSystemPrompt,
		Messages:  []ai.Message{msg},
		MaxTokens: 3000,
		JSON:      true,
	})
	if err != nil {
		return nil, Meta{}, err
	}

	var parsed struct {
		Recipes []Recipe `json:"recipes"`
	}
	hash := hashBytes(req.Image, b.String())
	if err := ai.ExtractJSON(resp.Text, &parsed); err != nil {
		return nil, meta(resp, hash, ""), fmt.Errorf("aifeatures: could not read the recipes: %w", err)
	}

	out := parsed.Recipes[:0]
	for _, r := range parsed.Recipes {
		if strings.TrimSpace(r.Name) == "" || len(r.Ingredients) == 0 {
			continue
		}
		if r.Servings <= 0 {
			r.Servings = 1
		}
		out = append(out, r)
		if len(out) == 3 {
			break
		}
	}

	return out, meta(resp, hash, jsonString(out)), nil
}

// ---- plan ----

const planSystemPrompt = `You write a one-week training and nutrition plan for someone partway through the 75 Hard challenge.

Reply with a single JSON object and nothing else:
{"summary":"two sentences on where they are and what this week focuses on","focus":"a short phrase","days":[{"day":number,"indoor":"the indoor session","outdoor":"the outdoor session","nutrition":"one line","note":"one short line of encouragement or caution"}],"tips":["...","..."]}

Rules:
- Exactly 7 day entries, numbered from the day given as the start.
- Both a 45-minute indoor and a 45-minute outdoor session every day, since the challenge requires them.
- Vary the stimulus across the week and include at least one deliberately easy outdoor day for recovery.
- Base the advice on the history given. If consistency has slipped on a particular task, address that directly.
- At most 4 tips.
- General fitness guidance only. Do not diagnose anything or give medical advice; if the history suggests a problem, say to see a professional.`

// PlanDay is one day of a generated plan.
type PlanDay struct {
	Day       int    `json:"day"`
	Indoor    string `json:"indoor"`
	Outdoor   string `json:"outdoor"`
	Nutrition string `json:"nutrition"`
	Note      string `json:"note"`
}

// Plan is a week of personalised guidance.
type Plan struct {
	Summary string    `json:"summary"`
	Focus   string    `json:"focus"`
	Days    []PlanDay `json:"days"`
	Tips    []string  `json:"tips"`
}

// History is what the plan is built from — the user's actual logged record.
type History struct {
	CurrentDay     int
	LengthDays     int
	DaysComplete   int
	DaysMissed     int
	Streak         int
	AvgKcal        float64
	KcalTarget     int
	TotalMinutes   int
	TaskRates      map[string]float64
	WeightChangeKg float64
	Goals          string
}

// BuildPlan writes a week of training and nutrition guidance from the user's
// logged history.
func (s *Service) BuildPlan(ctx context.Context, h History) (*Plan, Meta, error) {
	if !s.Enabled() {
		return nil, Meta{}, ErrDisabled
	}

	brief := renderHistory(h)
	resp, err := s.chain.Complete(ctx, ai.Request{
		System:    planSystemPrompt,
		Messages:  []ai.Message{ai.UserText(brief)},
		MaxTokens: 3000,
		JSON:      true,
	})
	if err != nil {
		return nil, Meta{}, err
	}

	var plan Plan
	hash := hashBytes(nil, brief)
	if err := ai.ExtractJSON(resp.Text, &plan); err != nil {
		return nil, meta(resp, hash, ""), fmt.Errorf("aifeatures: could not read the plan: %w", err)
	}
	if len(plan.Days) > 7 {
		plan.Days = plan.Days[:7]
	}
	if len(plan.Tips) > 4 {
		plan.Tips = plan.Tips[:4]
	}

	return &plan, meta(resp, hash, jsonString(plan)), nil
}

// ---- coaching note ----

const coachSystemPrompt = `You write one short daily note for someone doing the 75 Hard challenge.

Reply with a single JSON object and nothing else:
{"note":"two or three sentences","tone":"encouraging|direct|celebratory"}

Rules:
- Speak to their actual record: name what is going well and the one thing worth attention.
- Never invent a fact that is not in the summary you were given.
- No medical advice.
- Say it plainly. No exclamation marks, no motivational slogans.`

// CoachNote is the short daily message.
type CoachNote struct {
	Note string `json:"note"`
	Tone string `json:"tone"`
}

// DailyNote writes a short note about where the user stands today.
func (s *Service) DailyNote(ctx context.Context, h History) (*CoachNote, Meta, error) {
	if !s.Enabled() {
		return nil, Meta{}, ErrDisabled
	}

	brief := renderHistory(h)
	resp, err := s.chain.Complete(ctx, ai.Request{
		System:    coachSystemPrompt,
		Messages:  []ai.Message{ai.UserText(brief)},
		MaxTokens: 500,
		JSON:      true,
	})
	if err != nil {
		return nil, Meta{}, err
	}

	var note CoachNote
	hash := hashBytes(nil, brief)
	if err := ai.ExtractJSON(resp.Text, &note); err != nil {
		return nil, meta(resp, hash, ""), fmt.Errorf("aifeatures: could not read the note: %w", err)
	}
	note.Note = strings.TrimSpace(note.Note)
	if note.Note == "" {
		return nil, meta(resp, hash, ""), fmt.Errorf("aifeatures: empty note")
	}

	return &note, meta(resp, hash, jsonString(note)), nil
}

// ---- helpers ----

// renderHistory turns the record into a compact brief. Deliberately prose
// rather than raw JSON: models reason better over a short readable summary,
// and it keeps the token count down.
func renderHistory(h History) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Day %d of %d.\n", h.CurrentDay, h.LengthDays)
	fmt.Fprintf(&b, "Days completed: %d. Days missed: %d. Current streak: %d.\n",
		h.DaysComplete, h.DaysMissed, h.Streak)

	if h.AvgKcal > 0 {
		fmt.Fprintf(&b, "Average intake on logged days: %.0f kcal", h.AvgKcal)
		if h.KcalTarget > 0 {
			fmt.Fprintf(&b, " against a target of %d", h.KcalTarget)
		}
		b.WriteString(".\n")
	}
	if h.TotalMinutes > 0 {
		fmt.Fprintf(&b, "Training logged so far: %d minutes.\n", h.TotalMinutes)
	}
	if h.WeightChangeKg != 0 {
		fmt.Fprintf(&b, "Weight change since day 1: %+.1f kg.\n", h.WeightChangeKg)
	}

	if len(h.TaskRates) > 0 {
		b.WriteString("Task completion rates:\n")
		for name, rate := range h.TaskRates {
			fmt.Fprintf(&b, "  - %s: %.0f%%\n", name, rate)
		}
	}
	if g := strings.TrimSpace(h.Goals); g != "" {
		fmt.Fprintf(&b, "Their stated goals: %s\n", g)
	}

	return b.String()
}

func meta(resp ai.Response, hash, result string) Meta {
	return Meta{
		Provider:   resp.Provider,
		Model:      resp.Model,
		TokensIn:   resp.Usage.InputTokens,
		TokensOut:  resp.Usage.OutputTokens,
		Attempts:   resp.Attempts,
		InputHash:  hash,
		ResultJSON: result,
	}
}

// hashBytes fingerprints the inputs so an identical request reuses the cached
// answer rather than paying for it again.
func hashBytes(data []byte, text string) string {
	h := sha256.New()
	h.Write(data)
	h.Write([]byte{0})
	h.Write([]byte(text))
	return hex.EncodeToString(h.Sum(nil))
}

func jsonString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func mediaTypeOr(mt string) string {
	if mt == "" {
		return "image/jpeg"
	}
	return mt
}
