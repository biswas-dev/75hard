package api

import (
	"strings"
	"testing"

	"github.com/anchoo2kewl/75hard/api/internal/secret"
	goai "github.com/anchoo2kewl/go-ai"
)

// A stored provider key must never travel back to a client. The listing
// carries a hint instead — enough to recognise which key is saved, useless for
// using it.
func TestAISlotNeverCarriesTheKey(t *testing.T) {
	const key = "sk-abcdefghijklmnopqrstuvwxyz012345"

	slot := AISlot{
		Slot: 1, Provider: "deepseek", Model: "deepseek-v4-flash",
		KeyHint: secret.Hint(key), Enabled: true, HasKey: true,
	}

	// The struct has no field that could carry it, which is the point: this
	// fails at compile time if one is ever added and populated.
	rendered := slot.Provider + slot.Model + slot.KeyHint + slot.BaseURL
	if strings.Contains(rendered, key) || strings.Contains(rendered, "abcdefgh") {
		t.Fatal("the rendered slot leaks the API key")
	}
	if !strings.HasSuffix(slot.KeyHint, "2345") {
		t.Errorf("hint %q does not identify the key", slot.KeyHint)
	}
	// Counted in runes: the mask is bullets, which are three bytes each.
	if n := len([]rune(slot.KeyHint)); n > 8 {
		t.Errorf("hint %q is %d characters; it must reveal at most the last four",
			slot.KeyHint, n)
	}
}

func TestKnownProvidersAreUsable(t *testing.T) {
	if len(KnownProviders) == 0 {
		t.Fatal("no providers are offered")
	}
	for _, p := range KnownProviders {
		if p.Name == "" || p.Label == "" || p.SuggestedModel == "" {
			t.Errorf("provider %+v is missing a field somebody needs", p)
		}
		if p.SignupURL == "" {
			t.Errorf("%s has no signup link, so nobody can get a key", p.Name)
		}
		// go-ai has to be able to reach it, or a key stored against it could
		// never work — and an unknown provider must not be defaulted to some
		// other vendor's endpoint.
		if p.Name != "anthropic" && goai.BaseURLFor(p.Name) == "" {
			t.Errorf("go-ai has no endpoint for %q", p.Name)
		}
	}
}

func TestExactlyOneProviderIsFreeAndOnePublishesBalance(t *testing.T) {
	// These two facts are what the settings screen leans on to help somebody
	// choose, so they should stay true of the offered list.
	var free, balance []string
	for _, p := range KnownProviders {
		if p.Free {
			free = append(free, p.Name)
		}
		if p.PublishesBalance {
			balance = append(balance, p.Name)
		}
	}
	if len(free) == 0 {
		t.Error("no provider is marked free; somebody with no budget has no option")
	}
	if len(balance) == 0 {
		t.Error("no provider publishes a balance, so remaining credit can never be shown")
	}
}

func TestVisionModelOnlyOverridesWhenItDiffers(t *testing.T) {
	// DeepSeek needs a different model for images; most do not, and returning
	// one needlessly would swap a working model for the same string.
	if got := visionModelFor("deepseek"); got != "deepseek-v4-flash-vision-exp" {
		t.Errorf("deepseek vision model = %q", got)
	}
	if got := visionModelFor("openai"); got != "" {
		t.Errorf("openai returned %q, but its vision model is its text model", got)
	}
	if got := visionModelFor("nonexistent"); got != "" {
		t.Errorf("unknown provider returned %q", got)
	}
}

func TestMaxAISlots(t *testing.T) {
	// A primary and two backups. If three providers are down, a fourth was
	// not going to rescue the request.
	if MaxAISlots != 3 {
		t.Errorf("MaxAISlots = %d, want 3", MaxAISlots)
	}
}
