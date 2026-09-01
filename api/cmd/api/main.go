// Command api is the 75hard server: REST API, photo storage and the built SPA
// in a single binary.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	ai "github.com/anchoo2kewl/go-ai"
	gologin "github.com/anchoo2kewl/go-login"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"go.uber.org/zap"

	"github.com/anchoo2kewl/75hard/api/internal/aifeatures"
	"github.com/anchoo2kewl/75hard/api/internal/api"
	"github.com/anchoo2kewl/75hard/api/internal/auth"
	"github.com/anchoo2kewl/75hard/api/internal/config"
	"github.com/anchoo2kewl/75hard/api/internal/db"
	"github.com/anchoo2kewl/75hard/api/internal/photo"
	"github.com/anchoo2kewl/75hard/api/internal/version"
)

func main() {
	cfg := config.Load()
	logger := config.MustInitLogger(cfg.Env, cfg.LogLevel)
	defer logger.Sync() //nolint:errcheck

	logger.Info("starting 75hard",
		zap.String("version", version.Version),
		zap.String("commit", version.GitCommit),
		zap.String("env", cfg.Env),
		zap.String("port", cfg.Port))

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		logger.Fatal("failed to open database", zap.Error(err))
	}
	defer database.Close()

	applied, err := database.Migrate()
	if err != nil {
		logger.Fatal("failed to run migrations", zap.Error(err))
	}
	if len(applied) > 0 {
		logger.Info("applied migrations", zap.Strings("versions", applied))
	}

	if err := seedAdmin(database, cfg, logger); err != nil {
		logger.Error("failed to seed admin user", zap.Error(err))
	}

	photos, err := photo.NewStore(cfg.PhotosDir, cfg.MaxPhotoEdge, cfg.ThumbEdge)
	if err != nil {
		logger.Fatal("failed to open photo store", zap.Error(err))
	}

	// The AI chain is optional: with no AI_n_* slots configured the app runs
	// exactly as before and the AI endpoints report themselves as unavailable.
	aiSvc := aifeatures.New(nil)
	if chain, err := ai.ChainFromEnv(); err != nil {
		if !errors.Is(err, ai.ErrNoProviders) {
			logger.Warn("ai chain not configured", zap.Error(err))
		} else {
			logger.Info("ai features disabled: no AI_1_PROVIDER configured")
		}
	} else {
		chain.OnFallback(func(provider string, err error) {
			logger.Warn("ai provider failed, falling through", zap.String("provider", provider), zap.Error(err))
		})
		aiSvc = aifeatures.New(chain)
		logger.Info("ai features enabled", zap.Strings("chain", chain.Names()))
	}

	server := api.NewServer(database, cfg, logger, photos, aiSvc)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(api.ZapLogger(logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Unauthenticated: the load balancer and the deploy pipeline use these.
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := database.PingContext(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"degraded","database":"down"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	r.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// OAuth is optional; the app works with passwords alone.
	var oauthHandler *gologin.Handler
	if cfg.OAuthEnabled() {
		oauthCfg := &gologin.Config{
			SuccessURL:  cfg.OAuthSuccessURL,
			ErrorURL:    cfg.OAuthErrorURL,
			StateSecret: cfg.OAuthStateSecret,
			JWTSecret:   cfg.JWTSecret,
			JWTExpiry:   cfg.JWTExpiry,
			Logger:      logger,
		}
		if cfg.GoogleClientID != "" {
			oauthCfg.Google = &gologin.OAuthProviderConfig{
				ClientID:     cfg.GoogleClientID,
				ClientSecret: cfg.GoogleClientSecret,
				RedirectURL:  cfg.AppURL + "/api/auth/google/callback",
			}
		}
		if cfg.GitHubClientID != "" {
			oauthCfg.GitHub = &gologin.OAuthProviderConfig{
				ClientID:     cfg.GitHubClientID,
				ClientSecret: cfg.GitHubClientSecret,
				RedirectURL:  cfg.AppURL + "/api/auth/github/callback",
			}
		}
		oauthHandler, err = gologin.NewHandler(oauthCfg, db.NewOAuthStore(database))
		if err != nil {
			logger.Fatal("failed to init OAuth handler", zap.Error(err))
		}
		logger.Info("oauth enabled",
			zap.Bool("google", cfg.GoogleClientID != ""),
			zap.Bool("github", cfg.GitHubClientID != ""))
	}

	r.Route("/api", func(r chi.Router) {
		r.Get("/version", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = writeJSON(w, version.Get())
		})

		// Public auth endpoints, rate limited harder than the rest: this is
		// where credential stuffing shows up.
		r.Route("/auth", func(r chi.Router) {
			r.Use(api.RateLimitMiddleware(20))
			r.Post("/signup", server.HandleSignup)
			r.Post("/login", server.HandleLogin)
			r.Get("/config", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = writeJSON(w, map[string]bool{
					"allow_signup": cfg.AllowSignup,
					"google":       cfg.GoogleClientID != "" && cfg.OAuthEnabled(),
					"github":       cfg.GitHubClientID != "" && cfg.OAuthEnabled(),
				})
			})

			if oauthHandler != nil {
				r.Get("/google", oauthHandler.HandleGoogleInitiate)
				r.Get("/google/callback", oauthHandler.HandleGoogleCallback)
				r.Get("/github", oauthHandler.HandleGithubInitiate)
				r.Get("/github/callback", oauthHandler.HandleGithubCallback)
			}
		})

		// Everything below requires a valid token.
		r.Group(func(r chi.Router) {
			r.Use(server.JWTAuth)
			r.Use(api.RateLimitMiddleware(cfg.RateLimitPerMin))

			r.Get("/me", server.HandleMe)
			r.Patch("/me", server.HandleUpdateProfile)
			r.Post("/me/password", server.HandleChangePassword)

			r.Get("/programs", server.HandleListPrograms)
			r.Post("/programs", server.HandleCreateProgram)
			r.Get("/programs/active", server.HandleGetActiveProgram)
			r.Get("/programs/{programID}", server.HandleGetProgram)
			r.Patch("/programs/{programID}", server.HandleUpdateProgram)
			r.Post("/programs/{programID}/restart", server.HandleRestartProgram)

			r.Post("/programs/{programID}/tasks", server.HandleCreateTask)
			r.Patch("/programs/{programID}/tasks/{taskID}", server.HandleUpdateTask)
			r.Delete("/programs/{programID}/tasks/{taskID}", server.HandleDeleteTask)

			r.Get("/today", server.HandleGetToday)
			r.Get("/programs/{programID}/days", server.HandleListDays)
			r.Get("/programs/{programID}/days/{dayNumber}", server.HandleGetDay)
			r.Patch("/programs/{programID}/days/{dayNumber}", server.HandleUpdateDay)
			r.Post("/programs/{programID}/days/{dayNumber}/tasks/{taskID}", server.HandleToggleTask)

			r.Get("/stats", server.HandleGetStats)

			r.Get("/photos", server.HandleListPhotos)
			r.Post("/photos", server.HandleUploadPhoto)
			r.Get("/photos/{photoID}/file", server.HandleServePhoto)
			r.Delete("/photos/{photoID}", server.HandleDeletePhoto)

			r.Post("/meals", server.HandleCreateMeal)
			r.Patch("/meals/{mealID}", server.HandleUpdateMeal)
			r.Delete("/meals/{mealID}", server.HandleDeleteMeal)

			r.Post("/workouts", server.HandleCreateWorkout)
			r.Delete("/workouts/{workoutID}", server.HandleDeleteWorkout)

			// AI features. Rate limited far harder than the rest: each of
			// these is a paid upstream call, not a database read.
			r.Group(func(r chi.Router) {
				r.Use(api.RateLimitMiddleware(20))
				r.Get("/ai/status", server.HandleAIStatus)
				r.Post("/ai/food", server.HandleAnalyzeFood)
				r.Post("/ai/recipes", server.HandleSuggestRecipes)
				r.Post("/ai/plan", server.HandleBuildPlan)
				r.Get("/ai/coach", server.HandleCoachNote)
			})
		})
	})

	// Anything not under /api is the SPA.
	r.NotFound(api.SPAHandler(cfg.FrontendDist).ServeHTTP)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		// Generous: a phone on a slow connection uploading a photo needs it.
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		logger.Info("listening", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", zap.Error(err))
	}
	logger.Info("stopped")
}

// seedAdmin creates the configured admin account on first boot so a fresh
// deploy is usable without a shell. It never touches an existing account —
// re-running a deploy must not reset a password the user has changed.
func seedAdmin(database *db.DB, cfg *config.Config, logger *zap.Logger) error {
	if cfg.AdminEmail == "" || cfg.AdminPassword == "" {
		return nil
	}
	email := strings.ToLower(strings.TrimSpace(cfg.AdminEmail))

	var existing int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM users WHERE lower(email) = ?`, email).Scan(&existing); err != nil {
		return err
	}
	if existing > 0 {
		return nil
	}

	hash, err := auth.HashPassword(cfg.AdminPassword)
	if err != nil {
		return err
	}
	if _, err := database.Exec(
		`INSERT INTO users (email, password_hash, name, is_admin, auth_provider) VALUES (?, ?, ?, 1, 'password')`,
		email, hash, "Admin"); err != nil {
		return err
	}
	logger.Info("seeded admin user", zap.String("email", email))
	return nil
}

func writeJSON(w http.ResponseWriter, v any) error {
	return json.NewEncoder(w).Encode(v)
}
