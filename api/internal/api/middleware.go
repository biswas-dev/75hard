package api

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/anchoo2kewl/75hard/api/internal/auth"
	"go.uber.org/zap"
)

type contextKey string

const (
	// UserIDKey carries the authenticated user's id.
	UserIDKey contextKey = "user_id"
	// UserEmailKey carries the authenticated user's email.
	UserEmailKey contextKey = "user_email"
)

// UserID returns the authenticated user's id from the request context.
func UserID(ctx context.Context) int64 {
	id, _ := ctx.Value(UserIDKey).(int64)
	return id
}

// JWTAuth rejects requests without a valid bearer token and puts the user's
// identity on the request context.
func (s *Server) JWTAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if header == "" {
			respondError(w, http.StatusUnauthorized, "authorization required", "unauthorized")
			return
		}
		scheme, credential, ok := strings.Cut(header, " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") {
			respondError(w, http.StatusUnauthorized, "invalid authorization header", "unauthorized")
			return
		}

		claims, err := auth.ValidateToken(credential, s.cfg.JWTSecret)
		if err != nil {
			respondError(w, http.StatusUnauthorized, "invalid or expired token", "unauthorized")
			return
		}

		// A token stays valid until it expires, so a deleted account must be
		// checked on every request rather than trusted from the signature.
		var deleted bool
		err = s.db.QueryRowContext(r.Context(),
			`SELECT deleted_at IS NOT NULL FROM users WHERE id = ?`, claims.UserID).Scan(&deleted)
		if err != nil || deleted {
			respondError(w, http.StatusUnauthorized, "account unavailable", "unauthorized")
			return
		}

		ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
		ctx = context.WithValue(ctx, UserEmailKey, claims.Email)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ZapLogger logs each request with its status and duration.
func ZapLogger(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r)

			fields := []zap.Field{
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", rw.status),
				zap.Duration("took", time.Since(start)),
				zap.String("ip", ClientIP(r)),
			}
			switch {
			case rw.status >= 500:
				log.Error("request", fields...)
			case rw.status >= 400:
				log.Warn("request", fields...)
			default:
				log.Info("request", fields...)
			}
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// Unwrap lets http.ResponseController reach the underlying writer, so
// streaming and deadline control still work through the wrapper.
func (w *responseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// RateLimitMiddleware is a per-IP token bucket refilled once a minute.
// In-process and best-effort: it exists to blunt credential stuffing, while
// nginx does the real rate limiting at the edge.
func RateLimitMiddleware(perMinute int) func(http.Handler) http.Handler {
	type bucket struct {
		tokens int
		reset  time.Time
	}
	var (
		mu      sync.Mutex
		buckets = map[string]*bucket{}
	)

	// Buckets accumulate one entry per client IP; sweep them so a long-running
	// process doesn't grow without bound.
	go func() {
		for range time.Tick(10 * time.Minute) {
			mu.Lock()
			now := time.Now()
			for ip, b := range buckets {
				if now.After(b.reset.Add(10 * time.Minute)) {
					delete(buckets, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := ClientIP(r)
			now := time.Now()

			mu.Lock()
			b, ok := buckets[ip]
			if !ok || now.After(b.reset) {
				b = &bucket{tokens: perMinute, reset: now.Add(time.Minute)}
				buckets[ip] = b
			}
			allowed := b.tokens > 0
			if allowed {
				b.tokens--
			}
			mu.Unlock()

			if !allowed {
				w.Header().Set("Retry-After", "60")
				respondError(w, http.StatusTooManyRequests, "too many requests", "rate_limited")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ClientIP resolves the caller's address, preferring the headers Cloudflare
// and nginx set in front of us.
func ClientIP(r *http.Request) string {
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first, _, ok := strings.Cut(xff, ","); ok {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(xff)
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
