package aifeatures

import (
	"errors"
	"strings"
	"testing"
)

// The real answer that started this: a reasoning model spent its whole budget
// thinking and then filled in the shape it was given rather than the plate it
// was shown. It parsed perfectly, totalled zero, and was written to the meal
// as fact.
func TestUsableEstimateRejectsTheSchemaEcho(t *testing.T) {
	est := FoodEstimate{
		Name: "short dish name",
		Items: []FoodItem{
			{Name: "ingredient", Qty: 1, Unit: "serving"},
		},
	}
	if err := usableEstimate(est); !errors.Is(err, ErrNoEstimate) {
		t.Errorf("the prompt's own example was accepted as an estimate: %v", err)
	}
}

func TestUsableEstimateRejectsZeroCalories(t *testing.T) {
	// Zero is not a measurement, it is the absence of one. Saving it makes the
	// meal read as counted when it was not.
	est := FoodEstimate{
		Name:  "Salad",
		Items: []FoodItem{{Name: "lettuce", Qty: 1, Unit: "serving"}},
	}
	if err := usableEstimate(est); !errors.Is(err, ErrNoEstimate) {
		t.Errorf("a zero-calorie estimate was accepted: %v", err)
	}
}

func TestUsableEstimateCarriesTheModelsReasonWhenThereIsNoFood(t *testing.T) {
	est := FoodEstimate{Notes: "no food visible"}
	err := usableEstimate(est)
	if !errors.Is(err, ErrNoEstimate) {
		t.Fatalf("want ErrNoEstimate, got %v", err)
	}
	// The model's own words are more use than a generic failure.
	if got := err.Error(); !strings.Contains(got, "no food visible") {
		t.Errorf("the reason was dropped: %q", got)
	}
}

func TestUsableEstimateAcceptsARealAnswer(t *testing.T) {
	est := FoodEstimate{
		Name: "Bagel with cream cheese",
		Items: []FoodItem{
			{Name: "Bagel", Qty: 100, Unit: "g", Kcal: 250},
			{Name: "Cream cheese", Qty: 30, Unit: "g", Kcal: 100},
		},
		Kcal: 350,
	}
	if err := usableEstimate(est); err != nil {
		t.Errorf("a good estimate was refused: %v", err)
	}
}

// One placeholder among real items is a naming quirk, not a non-answer.
func TestUsableEstimateKeepsAnAnswerThatIsMostlyReal(t *testing.T) {
	est := FoodEstimate{
		Name: "Curry",
		Items: []FoodItem{
			{Name: "rice", Kcal: 200},
			{Name: "ingredient", Kcal: 50},
		},
		Kcal: 250,
	}
	if err := usableEstimate(est); err != nil {
		t.Errorf("a usable estimate was refused: %v", err)
	}
}
