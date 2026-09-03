package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// ProviderBalance is what a provider says is left on the account.
type ProviderBalance struct {
	Provider string `json:"provider"`
	// Currency and Amount are what the provider reported.
	Currency string  `json:"currency"`
	Amount   float64 `json:"amount"`
	// Available is the provider's own view of whether the account can still
	// be used, which is not always "amount is above zero".
	Available bool `json:"available"`
	// Error explains why a balance could not be read, when it could not be.
	Error string `json:"error,omitempty"`
	// CheckedAt is when this was last fetched.
	CheckedAt time.Time `json:"checked_at"`
}

// balanceCache holds the last reading.
//
// Balance is not a per-request concern: it changes slowly, the endpoint is
// rate limited like any other, and polling it on every page load would spend
// request budget to tell somebody a number that has not moved.
type balanceCache struct {
	mu sync.Mutex
	// Keyed by user, because the key being asked about is now theirs. One
	// shared entry would show somebody another person's remaining credit.
	entries   map[int64]map[string]ProviderBalance
	refreshed map[int64]time.Time
}

var balances = balanceCache{
	entries:   map[int64]map[string]ProviderBalance{},
	refreshed: map[int64]time.Time{},
}

// BalanceCacheTTL is how long a reading is reused.
const BalanceCacheTTL = 10 * time.Minute

// HandleAIBalance reports the credit left with each provider that publishes it.
//
// Only some do. DeepSeek exposes a balance endpoint; NVIDIA and Anthropic do
// not, so they are simply absent rather than reported as zero — an unknown
// balance and an empty one are very different things to show somebody.
func (s *Server) HandleAIBalance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID := UserID(ctx)

	balances.mu.Lock()
	if mine := balances.entries[userID]; len(mine) > 0 &&
		time.Since(balances.refreshed[userID]) < BalanceCacheTTL {
		out := make([]ProviderBalance, 0, len(mine))
		for _, b := range mine {
			out = append(out, b)
		}
		balances.mu.Unlock()
		respondJSON(w, http.StatusOK, map[string]any{"balances": out, "cached": true})
		return
	}
	balances.mu.Unlock()

	out := []ProviderBalance{}
	// Detached: a slow provider must not hold the page, and the reading is
	// worth caching even if the caller has gone.
	fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()

	// The caller's own key first: the balance has to belong to whoever is
	// looking, or somebody sees another person's remaining credit.
	if key := s.userProviderKey(ctx, UserID(ctx), "deepseek"); key != "" {
		out = append(out, fetchDeepSeekBalance(fetchCtx, key))
	} else if s.serverKeysAllowed(ctx, UserID(ctx)) {
		if key := deepseekKey(); key != "" {
			out = append(out, fetchDeepSeekBalance(fetchCtx, key))
		}
	}

	balances.mu.Lock()
	if balances.entries[userID] == nil {
		balances.entries[userID] = map[string]ProviderBalance{}
	}
	for _, b := range out {
		balances.entries[userID][b.Provider] = b
	}
	if len(out) > 0 {
		balances.refreshed[userID] = time.Now()
	}
	balances.mu.Unlock()

	if len(out) == 0 {
		s.log.Debug("no provider publishes a balance")
	}
	respondJSON(w, http.StatusOK, map[string]any{"balances": out, "cached": false})
}

// deepseekKey finds the configured DeepSeek key in whichever slot holds it.
//
// The slot order changes when a provider goes down, so the key cannot be read
// from a fixed position.
func deepseekKey() string {
	for _, prefix := range []string{"AI", "AIV"} {
		for i := 1; i <= 4; i++ {
			p := fmt.Sprintf("%s_%d_", prefix, i)
			if strings.EqualFold(strings.TrimSpace(os.Getenv(p+"PROVIDER")), "deepseek") {
				if key := strings.TrimSpace(os.Getenv(p + "API_KEY")); key != "" {
					return key
				}
			}
		}
	}
	return ""
}

// fetchDeepSeekBalance reads the account balance.
func fetchDeepSeekBalance(ctx context.Context, key string) ProviderBalance {
	b := ProviderBalance{Provider: "deepseek", CheckedAt: time.Now()}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.deepseek.com/user/balance", nil)
	if err != nil {
		b.Error = "could not build the request"
		return b
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := (&http.Client{Timeout: 12 * time.Second}).Do(req)
	if err != nil {
		b.Error = "could not reach DeepSeek"
		return b
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		b.Error = "could not read the response"
		return b
	}
	if resp.StatusCode != http.StatusOK {
		b.Error = fmt.Sprintf("DeepSeek returned %d", resp.StatusCode)
		return b
	}

	var parsed struct {
		IsAvailable  bool `json:"is_available"`
		BalanceInfos []struct {
			Currency     string `json:"currency"`
			TotalBalance string `json:"total_balance"`
		} `json:"balance_infos"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		b.Error = "could not decode the response"
		return b
	}

	b.Available = parsed.IsAvailable
	if len(parsed.BalanceInfos) > 0 {
		info := parsed.BalanceInfos[0]
		b.Currency = info.Currency
		// Sent as a string, deliberately, to avoid float rounding on money.
		// Parsing to a float here is fine for display; nothing is charged on it.
		if _, err := fmt.Sscanf(info.TotalBalance, "%f", &b.Amount); err != nil {
			b.Error = "could not read the amount"
		}
	}
	return b
}

// invalidateBalanceCache forces the next read to refetch. Called after a run
// so the figure someone sees reflects what they have just spent.
func invalidateBalanceCache() {
	balances.mu.Lock()
	balances.refreshed = map[int64]time.Time{}
	balances.mu.Unlock()
}

// userProviderKey returns a person's stored key for one provider, decrypted.
//
// Empty when they have none, when the server cannot decrypt, or when the key
// was encrypted under a secret that has since changed — all of which mean the
// same thing to the caller: there is no usable key here.
func (s *Server) userProviderKey(ctx context.Context, userID int64, provider string) string {
	cipher, err := s.cipher()
	if err != nil {
		return ""
	}
	var enc string
	if err := s.db.QueryRowContext(ctx,
		`SELECT api_key_enc FROM user_ai_providers
		  WHERE user_id = ? AND provider = ? AND enabled = 1 AND api_key_enc != ''
		  ORDER BY slot LIMIT 1`, userID, provider).Scan(&enc); err != nil {
		return ""
	}
	key, err := cipher.Decrypt(enc)
	if err != nil {
		return ""
	}
	return key
}
