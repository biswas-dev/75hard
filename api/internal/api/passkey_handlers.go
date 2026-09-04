package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	gologin "github.com/anchoo2kewl/go-login"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// passkeySessionExpiry bounds a half-finished ceremony.
//
// The browser prompt is in front of the user the whole time, so a minute or two
// is generous; leaving the challenge valid for longer only widens the window in
// which a stolen one is worth something.
const passkeySessionExpiry = 5 * time.Minute

// passkeyStore adapts the database to what go-login asks for.
//
// Two reads and no writes, by the library's design: finishing a ceremony hands
// back a credential and this application decides what to keep.
type passkeyStore struct{ s *Server }

func (p passkeyStore) PasskeyCredentials(ctx context.Context, userID int64) ([]gologin.PasskeyCredential, error) {
	rows, err := p.s.db.QueryContext(ctx, `
		SELECT credential_id, public_key, sign_count, backed_up, transports, attestation_type
		  FROM user_passkeys WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []gologin.PasskeyCredential
	for rows.Next() {
		var c gologin.PasskeyCredential
		var backedUp int
		var transports string
		if err := rows.Scan(&c.ID, &c.PublicKey, &c.SignCount, &backedUp, &transports,
			&c.AttestationType); err != nil {
			return nil, err
		}
		c.BackedUp = backedUp == 1
		if transports != "" {
			c.Transports = strings.Split(transports, ",")
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (p passkeyStore) PasskeyUserByID(ctx context.Context, userID int64) (gologin.PasskeyUser, error) {
	var u gologin.PasskeyUser
	err := p.s.db.QueryRowContext(ctx,
		`SELECT id, email, name FROM users WHERE id = ? AND deleted_at IS NULL`, userID).
		Scan(&u.ID, &u.Email, &u.DisplayName)
	if errors.Is(err, sql.ErrNoRows) {
		return u, gologin.ErrPasskeyUnknownUser
	}
	return u, err
}

// Passkey is one registered authenticator as the app shows it.
type Passkey struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	BackedUp   bool   `json:"backed_up"`
	Transports string `json:"transports,omitempty"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at,omitempty"`
}

// HandleListPasskeys lists the signed-in account's authenticators.
func (s *Server) HandleListPasskeys(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT id, name, backed_up, transports, created_at, COALESCE(last_used_at, '')
		  FROM user_passkeys WHERE user_id = ? ORDER BY created_at`, UserID(r.Context()))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not list your passkeys", "internal")
		return
	}
	defer rows.Close()

	out := []Passkey{}
	for rows.Next() {
		var pk Passkey
		var backedUp int
		if err := rows.Scan(&pk.ID, &pk.Name, &backedUp, &pk.Transports, &pk.CreatedAt,
			&pk.LastUsedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "could not list your passkeys", "internal")
			return
		}
		pk.BackedUp = backedUp == 1
		out = append(out, pk)
	}
	respondJSON(w, http.StatusOK, out)
}

// HandleDeletePasskey removes one authenticator.
func (s *Server) HandleDeletePasskey(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "passkeyID"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid passkey id", "invalid_id")
		return
	}
	res, err := s.db.ExecContext(r.Context(),
		`DELETE FROM user_passkeys WHERE id = ? AND user_id = ?`, id, UserID(r.Context()))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not remove that passkey", "internal")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		respondError(w, http.StatusNotFound, "no such passkey", "not_found")
		return
	}
	respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// HandlePasskeyRegisterBegin starts adding an authenticator.
func (s *Server) HandlePasskeyRegisterBegin(w http.ResponseWriter, r *http.Request) {
	if s.passkeys == nil {
		respondError(w, http.StatusServiceUnavailable, "passkeys are not available here", "no_passkeys")
		return
	}
	ctx := r.Context()
	userID := UserID(ctx)

	user, err := passkeyStore{s}.PasskeyUserByID(ctx, userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not read your account", "internal")
		return
	}

	options, session, err := s.passkeys.BeginRegistration(ctx, user)
	if err != nil {
		s.log.Error("begin passkey registration", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "could not start that", "internal")
		return
	}
	id, err := s.storePasskeySession(ctx, &userID, session)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not start that", "internal")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"session_id": id, "options": options})
}

type passkeyFinishRequest struct {
	SessionID string `json:"session_id"`
	// Name is what the owner will recognise it by in the list.
	Name string `json:"name"`
	// Credential is the browser's response, passed through untouched.
	Credential json.RawMessage `json:"credential"`
}

// HandlePasskeyRegisterFinish stores a newly created authenticator.
func (s *Server) HandlePasskeyRegisterFinish(w http.ResponseWriter, r *http.Request) {
	if s.passkeys == nil {
		respondError(w, http.StatusServiceUnavailable, "passkeys are not available here", "no_passkeys")
		return
	}
	var req passkeyFinishRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	userID := UserID(ctx)

	session, ok := s.takePasskeySession(ctx, req.SessionID, &userID)
	if !ok {
		respondError(w, http.StatusBadRequest, "that request expired — try again", "session_expired")
		return
	}
	user, err := passkeyStore{s}.PasskeyUserByID(ctx, userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not read your account", "internal")
		return
	}

	cred, err := s.passkeys.FinishRegistration(ctx, user, session, req.Credential)
	if err != nil {
		s.log.Warn("passkey registration rejected", zap.Error(err))
		respondError(w, http.StatusUnauthorized, "that passkey was not accepted", "passkey_rejected")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "Passkey"
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO user_passkeys (user_id, credential_id, public_key, sign_count, backed_up,
		                           transports, attestation_type, name)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, cred.ID, cred.PublicKey, cred.SignCount, boolInt(cred.BackedUp),
		strings.Join(cred.Transports, ","), cred.AttestationType, name); err != nil {
		s.log.Error("store passkey", zap.Error(err))
		respondError(w, http.StatusConflict, "that passkey is already registered", "already_registered")
		return
	}
	respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// HandlePasskeyLoginBegin starts a passwordless sign-in.
//
// It asks for no email and is deliberately available to anyone: a discoverable
// credential carries the user handle, so the browser offers the right passkey
// and the server learns who it is only from a verified assertion. Nothing here
// reveals whether an account exists.
func (s *Server) HandlePasskeyLoginBegin(w http.ResponseWriter, r *http.Request) {
	if s.passkeys == nil {
		respondError(w, http.StatusServiceUnavailable, "passkeys are not available here", "no_passkeys")
		return
	}
	ctx := r.Context()

	options, session, err := s.passkeys.BeginLogin(ctx)
	if err != nil {
		s.log.Error("begin passkey login", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "could not start sign-in", "internal")
		return
	}
	id, err := s.storePasskeySession(ctx, nil, session)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not start sign-in", "internal")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"session_id": id, "options": options})
}

// HandlePasskeyLoginFinish completes a passwordless sign-in.
//
// No second factor is asked for afterwards. The authenticator has already
// checked a fingerprint, a face or a PIN before it would sign anything, so the
// assertion is itself two factors: something you have and something you are.
func (s *Server) HandlePasskeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	if s.passkeys == nil {
		respondError(w, http.StatusServiceUnavailable, "passkeys are not available here", "no_passkeys")
		return
	}
	var req passkeyFinishRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()

	session, ok := s.takePasskeySession(ctx, req.SessionID, nil)
	if !ok {
		respondError(w, http.StatusBadRequest, "that sign-in expired — try again", "session_expired")
		return
	}

	who, cred, err := s.passkeys.FinishLogin(ctx, session, req.Credential)
	if err != nil {
		// A counter that went backwards is a copied credential, which is worth
		// saying out loud in the log even though the caller gets one message.
		if errors.Is(err, gologin.ErrPasskeyCloned) {
			s.log.Warn("passkey signature counter went backwards", zap.Int64("user_id", who.ID))
		} else {
			s.log.Warn("passkey sign-in rejected", zap.Error(err))
		}
		respondError(w, http.StatusUnauthorized, "that passkey was not accepted", "passkey_rejected")
		return
	}

	// Keep the counter in step, or the next sign-in is judged against a stale
	// one and a genuine key looks cloned.
	if _, err := s.db.ExecContext(ctx, `
		UPDATE user_passkeys SET sign_count = ?, backed_up = ?, last_used_at = CURRENT_TIMESTAMP
		 WHERE credential_id = ?`,
		cred.SignCount, boolInt(cred.BackedUp), cred.ID); err != nil {
		s.log.Error("update passkey counter", zap.Error(err))
	}

	user, err := s.getUser(r, who.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load account", "internal")
		return
	}
	s.issueToken(w, user)
}

// storePasskeySession keeps a ceremony's challenge server-side.
//
// The challenge is what the signature is verified against, so handing it to the
// browser to carry back would let the browser choose it.
func (s *Server) storePasskeySession(ctx context.Context, userID *int64, data []byte) (string, error) {
	raw := make([]byte, 24)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", err
	}
	id := hex.EncodeToString(raw)

	// Opportunistically clear expired rows; this table is otherwise unbounded.
	_, _ = s.db.ExecContext(ctx, `DELETE FROM passkey_sessions WHERE expires_at < CURRENT_TIMESTAMP`)

	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO passkey_sessions (id, user_id, data, expires_at) VALUES (?, ?, ?, ?)`,
		id, userID, data, time.Now().UTC().Add(passkeySessionExpiry)); err != nil {
		return "", err
	}
	return id, nil
}

// takePasskeySession reads a challenge and deletes it, so a ceremony cannot be
// replayed with the same one.
func (s *Server) takePasskeySession(ctx context.Context, id string, userID *int64) ([]byte, bool) {
	if id == "" {
		return nil, false
	}

	var data []byte
	var owner sql.NullInt64
	var expires time.Time
	if err := s.db.QueryRowContext(ctx,
		`SELECT data, user_id, expires_at FROM passkey_sessions WHERE id = ?`, id).
		Scan(&data, &owner, &expires); err != nil {
		return nil, false
	}
	_, _ = s.db.ExecContext(ctx, `DELETE FROM passkey_sessions WHERE id = ?`, id)

	if time.Now().UTC().After(expires) {
		return nil, false
	}
	// A registration challenge belongs to one account and must not be finished
	// by another.
	if userID != nil && (!owner.Valid || owner.Int64 != *userID) {
		return nil, false
	}
	if userID == nil && owner.Valid {
		return nil, false
	}
	return data, true
}

// PasskeysEnabled reports whether this server can run WebAuthn ceremonies, so
// the sign-in page only offers what will work.
func (s *Server) PasskeysEnabled() bool { return s.passkeys != nil }
