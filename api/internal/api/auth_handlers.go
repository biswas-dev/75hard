package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/anchoo2kewl/75hard/api/internal/auth"
	"github.com/anchoo2kewl/75hard/api/internal/program"
	"go.uber.org/zap"
)

// User is the account shape returned to the SPA. It never carries the hash.
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	AvatarURL    string `json:"avatar_url"`
	Timezone     string `json:"timezone"`
	IsAdmin      bool   `json:"is_admin"`
	AuthProvider string `json:"auth_provider"`
	CreatedAt    string `json:"created_at"`
}

type authResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

type signupRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
	Timezone string `json:"timezone"`
}

// HandleSignup creates an account and returns a token, so the SPA can go
// straight into the app without a second round trip through login.
func (s *Server) HandleSignup(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.AllowSignup {
		respondError(w, http.StatusForbidden, "registration is closed", "signup_disabled")
		return
	}

	var req signupRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	email := normalizeEmail(req.Email)
	if !validEmail(email) {
		respondError(w, http.StatusBadRequest, "a valid email is required", "invalid_email")
		return
	}
	if len(req.Password) < 8 {
		respondError(w, http.StatusBadRequest, "password must be at least 8 characters", "weak_password")
		return
	}

	tz := strings.TrimSpace(req.Timezone)
	if tz == "" {
		tz = "UTC"
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.log.Error("hash password", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "could not create account", "internal")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name, _, _ = strings.Cut(email, "@")
	}

	res, err := s.db.ExecContext(r.Context(),
		`INSERT INTO users (email, password_hash, name, timezone, auth_provider) VALUES (?, ?, ?, ?, 'password')`,
		email, hash, name, tz)
	if err != nil {
		if isUniqueViolation(err) {
			respondError(w, http.StatusConflict, "an account with that email already exists", "email_taken")
			return
		}
		s.log.Error("create user", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "could not create account", "internal")
		return
	}

	id, _ := res.LastInsertId()
	user, err := s.getUser(r, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load account", "internal")
		return
	}
	s.issueToken(w, user)
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// HandleLogin exchanges credentials for a token.
func (s *Server) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	email := normalizeEmail(req.Email)

	var (
		id   int64
		hash string
	)
	err := s.db.QueryRowContext(r.Context(),
		`SELECT id, password_hash FROM users WHERE lower(email) = ? AND deleted_at IS NULL`,
		email).Scan(&id, &hash)
	if err != nil {
		// Deliberately the same message and rough timing as a wrong password,
		// so the endpoint doesn't confirm which emails have accounts.
		if errors.Is(err, sql.ErrNoRows) {
			auth.HashPassword(req.Password) //nolint:errcheck // constant-time-ish padding
			respondError(w, http.StatusUnauthorized, "invalid email or password", "invalid_credentials")
			return
		}
		s.log.Error("login lookup", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "could not sign in", "internal")
		return
	}

	if err := auth.VerifyPassword(hash, req.Password); err != nil {
		respondError(w, http.StatusUnauthorized, "invalid email or password", "invalid_credentials")
		return
	}

	user, err := s.getUser(r, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load account", "internal")
		return
	}
	s.issueToken(w, user)
}

// HandleMe returns the signed-in account.
func (s *Server) HandleMe(w http.ResponseWriter, r *http.Request) {
	user, err := s.getUser(r, UserID(r.Context()))
	if err != nil {
		respondError(w, http.StatusNotFound, "account not found", "not_found")
		return
	}
	respondJSON(w, http.StatusOK, user)
}

type updateProfileRequest struct {
	Name     *string `json:"name"`
	Timezone *string `json:"timezone"`
}

// HandleUpdateProfile updates the display name and timezone.
func (s *Server) HandleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	var req updateProfileRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	userID := UserID(r.Context())

	if req.Name != nil {
		if _, err := s.db.ExecContext(r.Context(),
			`UPDATE users SET name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			strings.TrimSpace(*req.Name), userID); err != nil {
			respondError(w, http.StatusInternalServerError, "could not update profile", "internal")
			return
		}
	}
	if req.Timezone != nil {
		tz := strings.TrimSpace(*req.Timezone)
		// Reject an unknown zone rather than silently storing one that will
		// fall back to UTC and quietly shift every day boundary.
		if _, err := time.LoadLocation(tz); err != nil {
			respondError(w, http.StatusBadRequest, "unknown timezone", "invalid_timezone")
			return
		}
		if _, err := s.db.ExecContext(r.Context(),
			`UPDATE users SET timezone = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			tz, userID); err != nil {
			respondError(w, http.StatusInternalServerError, "could not update profile", "internal")
			return
		}
	}

	user, err := s.getUser(r, userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not load account", "internal")
		return
	}
	respondJSON(w, http.StatusOK, user)
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// HandleChangePassword rotates the account password.
func (s *Server) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	var req changePasswordRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.NewPassword) < 8 {
		respondError(w, http.StatusBadRequest, "password must be at least 8 characters", "weak_password")
		return
	}

	userID := UserID(r.Context())
	var hash string
	if err := s.db.QueryRowContext(r.Context(),
		`SELECT password_hash FROM users WHERE id = ?`, userID).Scan(&hash); err != nil {
		respondError(w, http.StatusNotFound, "account not found", "not_found")
		return
	}
	if err := auth.VerifyPassword(hash, req.CurrentPassword); err != nil {
		respondError(w, http.StatusUnauthorized, "current password is incorrect", "invalid_credentials")
		return
	}

	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "could not update password", "internal")
		return
	}
	if _, err := s.db.ExecContext(r.Context(),
		`UPDATE users SET password_hash = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		newHash, userID); err != nil {
		respondError(w, http.StatusInternalServerError, "could not update password", "internal")
		return
	}
	respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) issueToken(w http.ResponseWriter, user User) {
	token, err := auth.GenerateToken(user.ID, user.Email, s.cfg.JWTSecret, s.cfg.JWTExpiry)
	if err != nil {
		s.log.Error("generate token", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "could not issue token", "internal")
		return
	}
	respondJSON(w, http.StatusOK, authResponse{Token: token, User: user})
}

func (s *Server) getUser(r *http.Request, id int64) (User, error) {
	var u User
	err := s.db.QueryRowContext(r.Context(),
		`SELECT id, email, name, avatar_url, timezone, is_admin, auth_provider, created_at
		 FROM users WHERE id = ? AND deleted_at IS NULL`, id).
		Scan(&u.ID, &u.Email, &u.Name, &u.AvatarURL, &u.Timezone, &u.IsAdmin, &u.AuthProvider, &u.CreatedAt)
	return u, err
}

// userLocation resolves the caller's timezone for date arithmetic.
func (s *Server) userLocation(r *http.Request) *time.Location {
	var tz string
	if err := s.db.QueryRowContext(r.Context(),
		`SELECT timezone FROM users WHERE id = ?`, UserID(r.Context())).Scan(&tz); err != nil {
		return time.UTC
	}
	return program.LoadLocation(tz)
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func validEmail(email string) bool {
	at := strings.Index(email, "@")
	if at < 1 || at == len(email)-1 {
		return false
	}
	domain := email[at+1:]
	return strings.Contains(domain, ".") && !strings.Contains(email, " ")
}

func isUniqueViolation(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "constraint failed")
}
