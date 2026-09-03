package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/anchoo2kewl/75hard/api/internal/aifeatures"
	"github.com/anchoo2kewl/75hard/api/internal/secret"
	goai "github.com/anchoo2kewl/go-ai"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// MaxAISlots is how many providers one person can chain.
//
// Three is a primary and two backups, which is as deep as a fallback chain
// usefully goes: if three providers are down, the fourth was not going to save
// the request either.
const MaxAISlots = 3

// AISlot is one configured provider, as shown to its owner.
//
// The key itself is never included. KeyHint is the last four characters, which
// is enough to recognise which key is stored and useless for using it.
type AISlot struct {
	Slot     int    `json:"slot"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	BaseURL  string `json:"base_url,omitempty"`
	KeyHint  string `json:"key_hint"`
	Enabled  bool   `json:"enabled"`
	HasKey   bool   `json:"has_key"`
}

// AIKeysResponse is the whole picture for the settings screen.
type AIKeysResponse struct {
	Slots []AISlot `json:"slots"`
	// UsingOwnKeys is false when the request would fall back to the server's.
	UsingOwnKeys bool `json:"using_own_keys"`
	// ServerFallback reports whether this account may use the instance's own
	// keys when it has configured none.
	ServerFallback bool `json:"server_fallback"`
	// Configurable is false when the server has no encryption key, so a
	// personal key could not be stored safely and the form is pointless.
	Configurable bool `json:"configurable"`
	// Known providers, with whether each publishes a balance and whether it
	// has a free tier — the two things that decide which to pick.
	Providers []ProviderInfo `json:"providers"`
}

// ProviderInfo describes a provider a person can choose.
type ProviderInfo struct {
	Name string `json:"name"`
	// Label is how it is written for people.
	Label string `json:"label"`
	// SuggestedModel is a sensible default, and for vision often the only one.
	SuggestedModel string `json:"suggested_model"`
	// VisionModel is the model to use for photographs, when it differs.
	VisionModel string `json:"vision_model,omitempty"`
	// Free reports a usable free tier.
	Free bool `json:"free"`
	// PublishesBalance reports whether remaining credit can be shown.
	PublishesBalance bool `json:"publishes_balance"`
	// SignupURL is where to get a key.
	SignupURL string `json:"signup_url"`
}

// KnownProviders is the list offered in the UI.
//
// Deliberately short. A person configuring this is not shopping for an
// inference vendor; they want the one that works, and the two facts that
// actually decide it are whether it costs anything and whether they can see
// what is left.
var KnownProviders = []ProviderInfo{
	{
		Name: "nvidia", Label: "NVIDIA NIM",
		SuggestedModel: "meta/llama-3.2-90b-vision-instruct",
		VisionModel:    "meta/llama-3.2-90b-vision-instruct",
		Free:           true, PublishesBalance: false,
		SignupURL: "https://build.nvidia.com/",
	},
	{
		Name: "deepseek", Label: "DeepSeek",
		SuggestedModel: "deepseek-v4-flash",
		VisionModel:    "deepseek-v4-flash-vision-exp",
		Free:           false, PublishesBalance: true,
		SignupURL: "https://platform.deepseek.com/",
	},
	{
		Name: "openai", Label: "OpenAI",
		SuggestedModel: "gpt-5.2", VisionModel: "gpt-5.2",
		Free: false, PublishesBalance: false,
		SignupURL: "https://platform.openai.com/",
	},
	{
		Name: "anthropic", Label: "Anthropic",
		SuggestedModel: "claude-sonnet-5", VisionModel: "claude-sonnet-5",
		Free: false, PublishesBalance: false,
		SignupURL: "https://console.anthropic.com/",
	},
	{
		Name: "openrouter", Label: "OpenRouter",
		SuggestedModel: "meta-llama/llama-3.3-70b-instruct",
		Free:           false, PublishesBalance: false,
		SignupURL: "https://openrouter.ai/",
	},
}

// HandleListAIKeys returns the caller's configured providers.
func (s *Server) HandleListAIKeys(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := UserID(ctx)

	slots, err := s.userAISlots(ctx, userID)
	if err != nil {
		s.log.Error("list ai slots", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "could not read your providers", "internal")
		return
	}

	out := AIKeysResponse{
		Slots:          slots,
		UsingOwnKeys:   len(slots) > 0,
		ServerFallback: s.serverKeysAllowed(ctx, userID),
		Configurable:   s.cfg.EncryptionKey != "",
		Providers:      KnownProviders,
	}
	respondJSON(w, http.StatusOK, out)
}

type saveAISlotRequest struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	BaseURL  string `json:"base_url"`
	// APIKey is write-only. An empty string on an existing slot keeps the key
	// already stored, so somebody can change the model without re-pasting it.
	APIKey  string `json:"api_key"`
	Enabled *bool  `json:"enabled"`
}

// HandleSaveAIKey stores one provider slot.
func (s *Server) HandleSaveAIKey(w http.ResponseWriter, r *http.Request) {
	slot, err := strconv.Atoi(chi.URLParam(r, "slot"))
	if err != nil || slot < 1 || slot > MaxAISlots {
		respondError(w, http.StatusBadRequest,
			"slot must be between 1 and "+strconv.Itoa(MaxAISlots), "invalid_slot")
		return
	}

	var req saveAISlotRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	userID := UserID(ctx)

	cipher, err := s.cipher()
	if err != nil {
		respondError(w, http.StatusServiceUnavailable,
			"this server cannot store API keys: no encryption key is configured", "no_encryption")
		return
	}

	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if provider == "" {
		respondError(w, http.StatusBadRequest, "a provider is required", "invalid_provider")
		return
	}
	// Refuse a provider go-ai cannot reach rather than storing a key that will
	// never work — or worse, sending it to some other vendor's endpoint.
	if goai.BaseURLFor(provider) == "" && strings.TrimSpace(req.BaseURL) == "" &&
		provider != "anthropic" && provider != "claude" {
		respondError(w, http.StatusBadRequest,
			"unknown provider; supply a base URL for a custom endpoint", "invalid_provider")
		return
	}

	// An empty key on an existing slot keeps what is stored.
	var encKey, hint string
	if strings.TrimSpace(req.APIKey) != "" {
		encKey, err = cipher.Encrypt(strings.TrimSpace(req.APIKey))
		if err != nil {
			respondError(w, http.StatusInternalServerError, "could not store the key", "internal")
			return
		}
		hint = secret.Hint(req.APIKey)
	} else {
		var existing string
		err := s.db.QueryRowContext(ctx,
			`SELECT api_key_enc, key_hint FROM user_ai_providers WHERE user_id = ? AND slot = ?`,
			userID, slot).Scan(&encKey, &existing)
		if errors.Is(err, sql.ErrNoRows) || encKey == "" {
			respondError(w, http.StatusBadRequest, "an API key is required", "missing_key")
			return
		}
		hint = existing
	}

	enabled := 1
	if req.Enabled != nil && !*req.Enabled {
		enabled = 0
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO user_ai_providers (user_id, slot, provider, model, base_url, api_key_enc, key_hint, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, slot) DO UPDATE SET
			provider    = excluded.provider,
			model       = excluded.model,
			base_url    = excluded.base_url,
			api_key_enc = excluded.api_key_enc,
			key_hint    = excluded.key_hint,
			enabled     = excluded.enabled,
			updated_at  = CURRENT_TIMESTAMP`,
		userID, slot, provider, strings.TrimSpace(req.Model),
		strings.TrimSpace(req.BaseURL), encKey, hint, enabled); err != nil {
		s.log.Error("save ai slot", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "could not save the provider", "internal")
		return
	}

	// The balance shown belongs to whichever key is configured, so a change
	// invalidates it.
	invalidateBalanceCache()

	slots, _ := s.userAISlots(ctx, userID)
	respondJSON(w, http.StatusOK, map[string]any{"slots": slots})
}

// HandleDeleteAIKey removes one slot.
func (s *Server) HandleDeleteAIKey(w http.ResponseWriter, r *http.Request) {
	slot, err := strconv.Atoi(chi.URLParam(r, "slot"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid slot", "invalid_slot")
		return
	}
	ctx := r.Context()

	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM user_ai_providers WHERE user_id = ? AND slot = ?`,
		UserID(ctx), slot); err != nil {
		respondError(w, http.StatusInternalServerError, "could not remove the provider", "internal")
		return
	}
	invalidateBalanceCache()

	slots, _ := s.userAISlots(ctx, UserID(ctx))
	respondJSON(w, http.StatusOK, map[string]any{"slots": slots})
}

// userAISlots reads a person's configured providers, without their keys.
func (s *Server) userAISlots(ctx context.Context, userID int64) ([]AISlot, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT slot, provider, model, base_url, key_hint, enabled, api_key_enc != ''
		  FROM user_ai_providers WHERE user_id = ? ORDER BY slot`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []AISlot{}
	for rows.Next() {
		var a AISlot
		var enabled, hasKey int
		if err := rows.Scan(&a.Slot, &a.Provider, &a.Model, &a.BaseURL,
			&a.KeyHint, &enabled, &hasKey); err != nil {
			return nil, err
		}
		a.Enabled = enabled == 1
		a.HasKey = hasKey == 1
		out = append(out, a)
	}
	return out, rows.Err()
}

// cipher builds the encryptor, or reports that this server cannot store keys.
func (s *Server) cipher() (*secret.Cipher, error) {
	return secret.New(s.cfg.EncryptionKey)
}

// serverKeysAllowed reports whether this account may fall back to the keys the
// instance itself is configured with.
//
// Only the administrator may. The server's keys are somebody's personal
// credit, and a stranger signing up should not be able to spend it — they
// bring their own, or the AI features stay off for them.
func (s *Server) serverKeysAllowed(ctx context.Context, userID int64) bool {
	var isAdmin bool
	if err := s.db.QueryRowContext(ctx,
		`SELECT is_admin FROM users WHERE id = ?`, userID).Scan(&isAdmin); err != nil {
		return false
	}
	return isAdmin
}

// aiForUser builds the AI service this request should use.
//
// A person's own providers win. Failing that, the administrator falls back to
// the instance's own configuration, and everybody else gets a disabled service
// that reports plainly that a key is needed.
func (s *Server) aiForUser(ctx context.Context, userID int64) *aifeatures.Service {
	slots, err := s.userAISlots(ctx, userID)
	if err != nil || len(slots) == 0 {
		if s.serverKeysAllowed(ctx, userID) {
			return s.ai
		}
		return aifeatures.New(nil)
	}

	cipher, err := s.cipher()
	if err != nil {
		return aifeatures.New(nil)
	}

	text := make([]goai.Slot, 0, len(slots))
	vision := make([]goai.Slot, 0, len(slots))
	for _, sl := range slots {
		if !sl.Enabled {
			continue
		}
		var enc string
		if err := s.db.QueryRowContext(ctx,
			`SELECT api_key_enc FROM user_ai_providers WHERE user_id = ? AND slot = ?`,
			userID, sl.Slot).Scan(&enc); err != nil {
			continue
		}
		key, err := cipher.Decrypt(enc)
		if err != nil {
			// A key encrypted under a previous secret cannot be recovered.
			// Skipping it is right: the alternative is sending rubbish to a
			// provider and reporting an authentication error.
			s.log.Warn("could not decrypt a stored provider key",
				zap.Int64("user", userID), zap.Int("slot", sl.Slot))
			continue
		}

		base := goai.Slot{
			Provider: sl.Provider,
			Model:    sl.Model,
			APIKey:   key,
			BaseURL:  sl.BaseURL,
			Timeout:  AISlotTimeout,
		}
		text = append(text, base)

		// Several providers use a different model for images, so the vision
		// chain is built from the same credentials with the model swapped.
		visionSlot := base
		if vm := visionModelFor(sl.Provider); vm != "" {
			visionSlot.Model = vm
		}
		vision = append(vision, visionSlot)
	}

	if len(text) == 0 {
		return aifeatures.New(nil)
	}

	policy := goai.DefaultRetryPolicy()
	policy.MaxAttempts = AIMaxAttempts

	textChain, err := goai.NewChainFromSlots(text...)
	if err != nil {
		return aifeatures.New(nil)
	}
	textChain = textChain.WithRetry(policy)

	visionChain, err := goai.NewChainFromSlots(vision...)
	if err != nil {
		visionChain = nil
	} else {
		visionChain = visionChain.WithRetry(policy)
	}
	return aifeatures.NewWithVision(textChain, visionChain)
}

// visionModelFor returns the image model for a provider, when it differs from
// its text model.
func visionModelFor(provider string) string {
	for _, p := range KnownProviders {
		if p.Name == provider && p.VisionModel != "" && p.VisionModel != p.SuggestedModel {
			return p.VisionModel
		}
	}
	return ""
}
