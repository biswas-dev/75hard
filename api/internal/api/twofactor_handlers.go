package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/anchoo2kewl/75hard/api/internal/auth"
	"github.com/anchoo2kewl/75hard/api/internal/secret"
	gologin "github.com/anchoo2kewl/go-login"
	"go.uber.org/zap"
)

// twoFactorChallengeExpiry is how long the window between a correct password
// and a correct code stays open.
//
// Long enough to find your phone, unlock it and read six digits; short enough
// that a challenge left on a shared machine is worthless by the time anyone
// finds it.
const twoFactorChallengeExpiry = 5 * time.Minute

// challengeSecret derives the key that signs half-finished sign-ins.
//
// Deliberately not the session key. A challenge token proves one thing — that
// a password was right — and must never be accepted as proof of a completed
// sign-in, which is exactly what would happen if the same secret validated
// both.
func (s *Server) challengeSecret() string {
	return deriveSecret(s.cfg.JWTSecret, "two-factor-challenge")
}

// deriveSecret produces a key for one purpose from the master signing secret.
//
// Reusing one secret for two kinds of token is how a token minted for one
// purpose gets accepted for another; an HMAC over a label keeps them apart
// without another secret to configure and rotate.
func deriveSecret(master, label string) string {
	mac := hmac.New(sha256.New, []byte(master))
	mac.Write([]byte(label))
	return hex.EncodeToString(mac.Sum(nil))
}

// hostOf is the bare host of a URL, used as the issuer an authenticator app
// shows beside the code.
func hostOf(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	return raw
}

// twoFactorStatus reports what an account has enrolled.
type twoFactorStatus struct {
	Enabled bool `json:"enabled"`
	// Pending is true when a secret has been generated but no code has proved
	// it yet, so the UI can offer to finish or abandon the enrolment.
	Pending bool `json:"pending"`
	// ConfigurableO is false when the server has no encryption key, so no
	// secret could be stored even if one were generated.
	Configurable   bool   `json:"configurable"`
	ConfirmedAt    string `json:"confirmed_at,omitempty"`
	RecoveryLeft   int    `json:"recovery_codes_left"`
	PasskeyCount   int    `json:"passkey_count"`
	PasskeysUsable bool   `json:"passkeys_usable"`
}

// HandleTwoFactorStatus describes the signed-in account's second factors.
func (s *Server) HandleTwoFactorStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := UserID(ctx)

	var enc string
	var enabled int
	var confirmed sql.NullString
	if err := s.db.QueryRowContext(ctx,
		`SELECT totp_secret_enc, totp_enabled, totp_confirmed_at FROM users WHERE id = ?`,
		userID).Scan(&enc, &enabled, &confirmed); err != nil {
		respondError(w, http.StatusInternalServerError, "could not read your settings", "internal")
		return
	}

	out := twoFactorStatus{
		Enabled:        enabled == 1,
		Pending:        enabled == 0 && enc != "",
		Configurable:   strings.TrimSpace(s.cfg.EncryptionKey) != "",
		PasskeysUsable: gologin.PasskeysUsable(s.cfg.AppURL),
	}
	if confirmed.Valid {
		out.ConfirmedAt = confirmed.String
	}
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_recovery_codes WHERE user_id = ? AND used_at IS NULL`,
		userID).Scan(&out.RecoveryLeft)
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_passkeys WHERE user_id = ?`, userID).Scan(&out.PasskeyCount)

	respondJSON(w, http.StatusOK, out)
}

type twoFactorSetupResponse struct {
	// Secret in its grouped form, for typing into an app by hand when the QR
	// code cannot be scanned.
	Secret string `json:"secret"`
	// URI is the otpauth:// string the QR code encodes.
	URI string `json:"uri"`
}

// HandleTwoFactorSetup generates a secret and returns it for enrolment.
//
// Nothing is switched on here. The secret is stored unconfirmed, and only a
// correct code from the authenticator turns it into a requirement — a
// mis-scanned QR code that locked the account would be far worse than one that
// simply has to be scanned again.
func (s *Server) HandleTwoFactorSetup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := UserID(ctx)

	cipher, ok := s.cipherOrFail(w)
	if !ok {
		return
	}

	var enabled int
	var email string
	if err := s.db.QueryRowContext(ctx,
		`SELECT totp_enabled, email FROM users WHERE id = ?`, userID).Scan(&enabled, &email); err != nil {
		respondError(w, http.StatusInternalServerError, "could not read your account", "internal")
		return
	}
	if enabled == 1 {
		respondError(w, http.StatusConflict,
			"two-factor is already on; turn it off before enrolling again", "already_enabled")
		return
	}

	secretValue, err := gologin.NewTOTPSecret()
	if err != nil {
		s.log.Error("generate totp secret", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "could not start enrolment", "internal")
		return
	}
	enc, err := cipher.Encrypt(secretValue)
	if err != nil {
		s.log.Error("encrypt totp secret", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "could not store the secret", "internal")
		return
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE users SET totp_secret_enc = ?, totp_enabled = 0, totp_confirmed_at = NULL,
		                  updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`, enc, userID); err != nil {
		respondError(w, http.StatusInternalServerError, "could not store the secret", "internal")
		return
	}

	respondJSON(w, http.StatusOK, twoFactorSetupResponse{
		Secret: gologin.FormatTOTPSecret(secretValue),
		URI:    gologin.TOTPURI(hostOf(s.cfg.AppURL), email, secretValue),
	})
}

type twoFactorConfirmRequest struct {
	Code string `json:"code"`
}

type twoFactorConfirmResponse struct {
	Enabled bool `json:"enabled"`
	// RecoveryCodes are returned exactly once. They are stored hashed, so
	// there is no second chance to read them.
	RecoveryCodes []string `json:"recovery_codes"`
}

// HandleTwoFactorConfirm proves the secret was scanned and turns it on.
func (s *Server) HandleTwoFactorConfirm(w http.ResponseWriter, r *http.Request) {
	var req twoFactorConfirmRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	userID := UserID(ctx)

	cipher, ok := s.cipherOrFail(w)
	if !ok {
		return
	}

	var enc string
	if err := s.db.QueryRowContext(ctx,
		`SELECT totp_secret_enc FROM users WHERE id = ?`, userID).Scan(&enc); err != nil || enc == "" {
		respondError(w, http.StatusBadRequest, "start the enrolment first", "no_pending_secret")
		return
	}
	secretValue, err := cipher.Decrypt(enc)
	if err != nil {
		s.log.Error("decrypt totp secret", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "could not read the secret", "internal")
		return
	}
	if !gologin.VerifyTOTP(secretValue, req.Code, time.Now()) {
		respondError(w, http.StatusUnauthorized, "that code did not match", "invalid_code")
		return
	}

	codes, hashes, err := gologin.NewRecoveryCodes()
	if err != nil {
		s.log.Error("generate recovery codes", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "could not finish enrolment", "internal")
		return
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not finish enrolment", "internal")
		return
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET totp_enabled = 1, totp_confirmed_at = CURRENT_TIMESTAMP,
		                  updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`, userID); err != nil {
		respondError(w, http.StatusInternalServerError, "could not finish enrolment", "internal")
		return
	}
	// Replace any codes from an earlier enrolment: old ones must not open a
	// new secret.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM user_recovery_codes WHERE user_id = ?`, userID); err != nil {
		respondError(w, http.StatusInternalServerError, "could not finish enrolment", "internal")
		return
	}
	for _, h := range hashes {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO user_recovery_codes (user_id, code_hash) VALUES (?, ?)`, userID, h); err != nil {
			respondError(w, http.StatusInternalServerError, "could not finish enrolment", "internal")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		respondError(w, http.StatusInternalServerError, "could not finish enrolment", "internal")
		return
	}

	respondJSON(w, http.StatusOK, twoFactorConfirmResponse{Enabled: true, RecoveryCodes: codes})
}

type twoFactorDisableRequest struct {
	// Password re-checks the person at the keyboard. Turning a factor off is
	// exactly what someone on a borrowed session would try first.
	Password string `json:"password"`
	Code     string `json:"code"`
}

// HandleTwoFactorDisable turns two-factor off.
func (s *Server) HandleTwoFactorDisable(w http.ResponseWriter, r *http.Request) {
	var req twoFactorDisableRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	userID := UserID(ctx)

	var hash, enc, provider string
	if err := s.db.QueryRowContext(ctx,
		`SELECT password_hash, totp_secret_enc, auth_provider FROM users WHERE id = ?`,
		userID).Scan(&hash, &enc, &provider); err != nil {
		respondError(w, http.StatusInternalServerError, "could not read your account", "internal")
		return
	}

	// An account that signs in with Google has no password to re-check, so a
	// current code stands in for one.
	switch {
	case provider == "password" && hash != "":
		if err := auth.VerifyPassword(hash, req.Password); err != nil {
			respondError(w, http.StatusUnauthorized, "that password is not right", "invalid_credentials")
			return
		}
	default:
		cipher, ok := s.cipherOrFail(w)
		if !ok {
			return
		}
		secretValue, err := cipher.Decrypt(enc)
		if err != nil || !gologin.VerifyTOTP(secretValue, req.Code, time.Now()) {
			respondError(w, http.StatusUnauthorized, "that code did not match", "invalid_code")
			return
		}
	}

	if err := s.clearTwoFactor(r, userID); err != nil {
		respondError(w, http.StatusInternalServerError, "could not turn it off", "internal")
		return
	}
	respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// clearTwoFactor removes the secret and every recovery code.
func (s *Server) clearTwoFactor(r *http.Request, userID int64) error {
	ctx := r.Context()
	if _, err := s.db.ExecContext(ctx,
		`UPDATE users SET totp_secret_enc = '', totp_enabled = 0, totp_confirmed_at = NULL,
		                  updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`, userID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM user_recovery_codes WHERE user_id = ?`, userID)
	return err
}

type twoFactorVerifyRequest struct {
	Challenge string `json:"challenge"`
	Code      string `json:"code"`
}

// HandleTwoFactorVerify finishes a sign-in that stopped for a code.
func (s *Server) HandleTwoFactorVerify(w http.ResponseWriter, r *http.Request) {
	var req twoFactorVerifyRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	claims, err := auth.ValidateToken(req.Challenge, s.challengeSecret())
	if err != nil {
		respondError(w, http.StatusUnauthorized,
			"that sign-in has expired — start again", "challenge_expired")
		return
	}

	ok, err := s.consumeSecondFactor(r, claims.UserID, req.Code)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not check that code", "internal")
		return
	}
	if !ok {
		respondError(w, http.StatusUnauthorized, "that code did not match", "invalid_code")
		return
	}

	user, err := s.getUser(r, claims.UserID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load account", "internal")
		return
	}
	s.issueToken(w, user)
}

// consumeSecondFactor accepts either a current authenticator code or an unused
// recovery code, spending the recovery code if that is what was given.
func (s *Server) consumeSecondFactor(r *http.Request, userID int64, code string) (bool, error) {
	ctx := r.Context()
	code = strings.TrimSpace(code)
	if code == "" {
		return false, nil
	}

	var enc string
	if err := s.db.QueryRowContext(ctx,
		`SELECT totp_secret_enc FROM users WHERE id = ? AND totp_enabled = 1`,
		userID).Scan(&enc); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	if cipher, err := secret.New(s.cfg.EncryptionKey); err == nil {
		if secretValue, err := cipher.Decrypt(enc); err == nil {
			if gologin.VerifyTOTP(secretValue, code, time.Now()) {
				return true, nil
			}
		}
	}

	// Not a live code. It may be one of the recovery codes, which are stored
	// hashed and spent on use.
	res, err := s.db.ExecContext(ctx,
		`UPDATE user_recovery_codes SET used_at = CURRENT_TIMESTAMP
		  WHERE user_id = ? AND code_hash = ? AND used_at IS NULL`,
		userID, gologin.HashRecoveryCode(code))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n == 1 {
		s.log.Info("recovery code used", zap.Int64("user_id", userID))
		return true, nil
	}
	return false, nil
}

// twoFactorRequired reports whether an account finishes sign-in with a code.
func (s *Server) twoFactorRequired(r *http.Request, userID int64) bool {
	var enabled int
	if err := s.db.QueryRowContext(r.Context(),
		`SELECT totp_enabled FROM users WHERE id = ?`, userID).Scan(&enabled); err != nil {
		return false
	}
	return enabled == 1
}

// issueChallenge answers a correct password with a half-finished sign-in
// rather than a session.
func (s *Server) issueChallenge(w http.ResponseWriter, userID int64, email string) {
	token, err := auth.GenerateToken(userID, email, s.challengeSecret(), twoFactorChallengeExpiry)
	if err != nil {
		s.log.Error("generate 2fa challenge", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "could not sign in", "internal")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"two_factor": true,
		"challenge":  token,
	})
}

// cipherOrFail returns the secret cipher, answering the caller when there is
// none configured.
func (s *Server) cipherOrFail(w http.ResponseWriter) (*secret.Cipher, bool) {
	c, err := secret.New(s.cfg.EncryptionKey)
	if err != nil {
		respondError(w, http.StatusServiceUnavailable,
			"this server has no encryption key, so two-factor cannot be stored", "no_encryption_key")
		return nil, false
	}
	return c, true
}
