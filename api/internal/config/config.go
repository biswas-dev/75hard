// Package config loads runtime configuration from the environment.
//
// Deliberately a plain struct plus a handful of getEnv helpers — the same
// shape taskai uses. No config library, no file formats, no magic.
package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds every tunable the server reads at startup.
type Config struct {
	Env      string
	Port     string
	AppURL   string
	LogLevel string

	DBPath string

	JWTSecret string
	JWTExpiry time.Duration

	PhotosDir       string
	MaxUploadBytes  int64
	MaxPhotoEdge    int
	ThumbEdge       int
	RateLimitPerMin int

	FrontendDist string

	CORSAllowedOrigins []string

	// OAuth (go-login). Empty client IDs disable the provider.
	GoogleClientID     string
	GoogleClientSecret string
	GitHubClientID     string
	GitHubClientSecret string
	OAuthStateSecret   string
	OAuthSuccessURL    string
	OAuthErrorURL      string

	// Seeded on first boot so a fresh deploy is usable immediately.
	AdminEmail    string
	AdminPassword string

	// Registration is open by default; set to false to lock the instance down
	// to accounts that already exist.
	AllowSignup bool
}

// Load reads configuration from the environment, applying defaults suited to
// local development. It fatals on configuration that is unsafe in production
// rather than starting a server that looks fine and isn't.
func Load() *Config {
	c := &Config{
		Env:      getEnv("ENV", "development"),
		Port:     getEnv("PORT", "8087"),
		AppURL:   getEnv("APP_URL", "http://localhost:8087"),
		LogLevel: getEnv("LOG_LEVEL", "info"),

		DBPath: getEnv("DB_PATH", "./data/75hard.db"),

		JWTSecret: getEnv("JWT_SECRET", "dev-secret-change-me"),
		JWTExpiry: time.Duration(getEnvInt("JWT_EXPIRY_HOURS", 24*30)) * time.Hour,

		PhotosDir:       getEnv("PHOTOS_DIR", "./data/photos"),
		MaxUploadBytes:  int64(getEnvInt("MAX_UPLOAD_MB", 15)) << 20,
		MaxPhotoEdge:    getEnvInt("MAX_PHOTO_EDGE", 1600),
		ThumbEdge:       getEnvInt("THUMB_EDGE", 320),
		RateLimitPerMin: getEnvInt("RATE_LIMIT_PER_MIN", 300),

		FrontendDist: getEnv("FRONTEND_DIST", ""),

		CORSAllowedOrigins: getEnvList("CORS_ALLOWED_ORIGINS", "http://localhost:5175,http://localhost:8087"),

		GoogleClientID:     getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),
		GitHubClientID:     getEnv("LOGIN_GITHUB_CLIENT_ID", ""),
		GitHubClientSecret: getEnv("LOGIN_GITHUB_CLIENT_SECRET", ""),
		OAuthStateSecret:   getEnv("OAUTH_STATE_SECRET", ""),
		OAuthSuccessURL:    getEnv("OAUTH_SUCCESS_URL", "http://localhost:5175/oauth/callback"),
		OAuthErrorURL:      getEnv("OAUTH_ERROR_URL", "http://localhost:5175/login"),

		AdminEmail:    getEnv("ADMIN_EMAIL", ""),
		AdminPassword: getEnv("ADMIN_PASSWORD", ""),

		AllowSignup: getEnvBool("ALLOW_SIGNUP", true),
	}

	if c.IsProduction() {
		if c.JWTSecret == "dev-secret-change-me" {
			log.Fatal("config: JWT_SECRET must be set in production")
		}
		// go-login refuses to start if these match, but catching it here gives
		// a clearer message than a handler-construction failure later.
		if c.OAuthStateSecret != "" && c.OAuthStateSecret == c.JWTSecret {
			log.Fatal("config: OAUTH_STATE_SECRET must differ from JWT_SECRET")
		}
	}

	return c
}

// IsProduction reports whether the server is running in production.
func (c *Config) IsProduction() bool { return c.Env == "production" }

// OAuthEnabled reports whether at least one OAuth provider is configured.
func (c *Config) OAuthEnabled() bool {
	return c.OAuthStateSecret != "" && (c.GoogleClientID != "" || c.GitHubClientID != "")
}

func getEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func getEnvList(key, fallback string) []string {
	raw := getEnv(key, fallback)
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
