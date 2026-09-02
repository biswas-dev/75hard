package api

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/anchoo2kewl/75hard/api/internal/auth"
	"go.uber.org/zap"
)

// ResetTokenTTL is how long a reset link stays usable.
const ResetTokenTTL = time.Hour

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

// HandleForgotPassword issues a reset token.
//
// It always reports success, whether or not the address exists: a different
// answer for a known address turns this endpoint into a way to enumerate who
// has an account.
//
// There is no mail sender wired into this app, so the token is returned in the
// response when RESET_TOKEN_IN_RESPONSE is on — a deliberate, off-by-default
// setting for a single-operator instance. With it off the token is only ever
// written to the log, where the operator can retrieve it.
func (s *Server) HandleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req forgotPasswordRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	email := normalizeEmail(req.Email)

	// The same body regardless of outcome.
	ok := map[string]any{
		"ok":      true,
		"message": "If that address has an account, a reset link has been created.",
	}

	var userID int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM users WHERE lower(email) = ? AND deleted_at IS NULL`, email).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		respondJSON(w, http.StatusOK, ok)
		return
	}
	if err != nil {
		s.log.Error("forgot password lookup", zap.Error(err))
		respondJSON(w, http.StatusOK, ok)
		return
	}

	token, hash, err := newResetToken()
	if err != nil {
		s.log.Error("generate reset token", zap.Error(err))
		respondJSON(w, http.StatusOK, ok)
		return
	}

	// Any earlier outstanding token is retired, so a reset request always
	// leaves exactly one live link.
	if _, err := s.db.ExecContext(ctx,
		`UPDATE password_resets SET used_at = CURRENT_TIMESTAMP
		 WHERE user_id = ? AND used_at IS NULL`, userID); err != nil {
		s.log.Warn("could not retire earlier reset tokens", zap.Error(err))
	}

	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO password_resets (user_id, token_hash, expires_at) VALUES (?, ?, ?)`,
		userID, hash, time.Now().UTC().Add(ResetTokenTTL)); err != nil {
		s.log.Error("store reset token", zap.Error(err))
		respondJSON(w, http.StatusOK, ok)
		return
	}

	link := strings.TrimRight(s.cfg.AppURL, "/") + "/reset-password?token=" + token
	s.log.Info("password reset requested",
		zap.String("email", email),
		zap.String("reset_link", link),
		zap.Duration("valid_for", ResetTokenTTL))

	if s.cfg.ResetTokenInResponse {
		ok["token"] = token
		ok["reset_url"] = link
		ok["expires_in_seconds"] = int(ResetTokenTTL.Seconds())
	}
	respondJSON(w, http.StatusOK, ok)
}

type resetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

// HandleResetPassword exchanges a valid token for a new password.
func (s *Server) HandleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetPasswordRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.NewPassword) < 8 {
		respondError(w, http.StatusBadRequest, "password must be at least 8 characters", "weak_password")
		return
	}
	ctx := r.Context()

	hash := hashResetToken(strings.TrimSpace(req.Token))

	var (
		id        int64
		userID    int64
		expiresAt time.Time
		usedAt    *time.Time
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, expires_at, used_at FROM password_resets WHERE token_hash = ?`, hash).
		Scan(&id, &userID, &expiresAt, &usedAt)
	if err != nil {
		// One message for every failure mode: an unknown token, a spent one
		// and an expired one are all equally unusable, and distinguishing them
		// only helps someone probing.
		respondError(w, http.StatusBadRequest, "that reset link is not valid or has expired", "invalid_token")
		return
	}
	if usedAt != nil || time.Now().UTC().After(expiresAt) {
		respondError(w, http.StatusBadRequest, "that reset link is not valid or has expired", "invalid_token")
		return
	}

	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not set the password", "internal")
		return
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not set the password", "internal")
		return
	}
	defer tx.Rollback() //nolint:errcheck

	// Burn the token in the same transaction as the change, so a retry cannot
	// set the password twice.
	res, err := tx.ExecContext(ctx,
		`UPDATE password_resets SET used_at = CURRENT_TIMESTAMP WHERE id = ? AND used_at IS NULL`, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not set the password", "internal")
		return
	}
	if n, _ := res.RowsAffected(); n != 1 {
		respondError(w, http.StatusBadRequest, "that reset link has already been used", "invalid_token")
		return
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, auth_provider = 'password', updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`, newHash, userID); err != nil {
		respondError(w, http.StatusInternalServerError, "could not set the password", "internal")
		return
	}
	if err := tx.Commit(); err != nil {
		respondError(w, http.StatusInternalServerError, "could not set the password", "internal")
		return
	}

	s.log.Info("password reset completed", zap.Int64("user_id", userID))

	user, err := s.getUser(r, userID)
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	// Sign them straight in: they have just proved control of the address, and
	// making them retype the password they set ten seconds ago is friction for
	// nothing.
	s.issueToken(w, user)
}

// newResetToken returns the token to hand out and the hash to store.
func newResetToken() (token, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(buf)
	return token, hashResetToken(token), nil
}

func hashResetToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// constantTimeEquals is used where a token is compared directly rather than
// looked up by hash.
func constantTimeEquals(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
