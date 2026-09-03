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
	"github.com/anchoo2kewl/75hard/api/internal/api/spec"
	"github.com/anchoo2kewl/75hard/api/internal/auth"
	"github.com/anchoo2kewl/75hard/api/internal/config"
	"github.com/anchoo2kewl/75hard/api/internal/db"
	"github.com/anchoo2kewl/75hard/api/internal/photo"
	"github.com/anchoo2kewl/75hard/api/internal/version"
)

// aiRequestTimeout bounds a model-backed request. Generous because the work is
// upstream and the cost is already incurred once the call is in flight.
const aiRequestTimeout = 4 * time.Minute

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

	// The AI chains are optional: with no AI_n_* slots configured the app runs
	// exactly as before and the AI endpoints report themselves as unavailable.
	//
	// AI_*  is the text chain (recipes, plans, coaching notes).
	// AIV_* is the vision chain (food photos). It is separate because a
	// provider's cheap text model and its vision model are different models.
	aiSvc := aifeatures.New(nil)
	textChain, err := boundedChain("AI")
	if err != nil {
		if errors.Is(err, ai.ErrNoProviders) {
			logger.Info("ai features disabled: no AI_1_PROVIDER configured")
		} else {
			logger.Warn("ai chain not configured", zap.Error(err))
		}
	} else {
		textChain.OnFallback(func(provider string, err error) {
			logger.Warn("ai provider failed, falling through",
				zap.String("chain", "text"), zap.String("provider", provider), zap.Error(err))
		})
		textChain.OnRetry(func(provider string, attempt int, delay time.Duration, err error) {
			logger.Warn("ai provider retrying",
				zap.String("chain", "text"), zap.String("provider", provider),
				zap.Int("attempt", attempt), zap.Duration("in", delay), zap.Error(err))
		})

		visionChain, verr := boundedChain("AIV")
		if verr != nil {
			// No dedicated vision chain: photo requests reuse the text chain,
			// which works when the primary model is multimodal.
			visionChain = nil
			logger.Info("no dedicated vision chain; photo analysis will use the text chain")
		} else {
			visionChain.OnRetry(func(provider string, attempt int, delay time.Duration, err error) {
				logger.Warn("ai provider retrying",
					zap.String("chain", "vision"), zap.String("provider", provider),
					zap.Int("attempt", attempt), zap.Duration("in", delay), zap.Error(err))
			})
			visionChain.OnFallback(func(provider string, err error) {
				logger.Warn("ai provider failed, falling through",
					zap.String("chain", "vision"), zap.String("provider", provider), zap.Error(err))
			})
		}

		aiSvc = aifeatures.NewWithVision(textChain, visionChain)
		logger.Info("ai features enabled",
			zap.Strings("text", aiSvc.Providers()),
			zap.Strings("vision", aiSvc.VisionProviders()))
	}

	server := api.NewServer(database, cfg, logger, photos, aiSvc)

	// Food photos are estimated off the request path: a vision call takes most
	// of a minute, and the upload must not wait for it. Two workers, because
	// each job is a paid upstream call rather than a database read.
	var estimator *api.FoodEstimator
	if aiSvc.Enabled() {
		estimator = api.NewFoodEstimator(server, 2, 64)
		server.SetFoodEstimator(estimator)
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(api.ZapLogger(logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))
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
		// The OpenAPI document, served under the go-api convention. Public
		// on purpose: it describes the shape of the API, not anyone's data,
		// and requiring a credential to discover how to present a credential
		// is a loop.
		r.Method(http.MethodGet, "/openapi.yaml", spec.Document.Handler())
		r.Method(http.MethodHead, "/openapi.yaml", spec.Document.Handler())

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
			r.Post("/forgot-password", server.HandleForgotPassword)
			r.Post("/reset-password", server.HandleResetPassword)
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

		// Strava sends the browser back on a plain redirect with no bearer
		// token, so the callback cannot sit behind JWTAuth. It identifies the
		// user from the signed state it issued at connect time instead.
		r.Group(func(r chi.Router) {
			r.Use(api.RateLimitMiddleware(20))
			r.Get("/strava/callback", server.HandleStravaCallback)
		})

		// Everything below requires a valid token.
		r.Group(func(r chi.Router) {
			r.Use(server.JWTAuth)
			r.Use(api.RateLimitMiddleware(cfg.RateLimitPerMin))
			// Ordinary handlers are database work and should never take this
			// long; the AI group overrides it below.
			r.Use(middleware.Timeout(60 * time.Second))

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
			r.Get("/programs/{programID}/grid", server.HandleGetGrid)
			r.Get("/programs/{programID}/days/{dayNumber}", server.HandleGetDay)
			r.Patch("/programs/{programID}/days/{dayNumber}", server.HandleUpdateDay)
			r.Post("/programs/{programID}/days/{dayNumber}/tasks/{taskID}", server.HandleToggleTask)

			r.Get("/stats", server.HandleGetStats)
			// One call for the whole main page: five round trips on a phone is
			// the difference between the page appearing and assembling itself.
			r.Get("/summary", server.HandleGetSummary)

			// Personal API tokens. Creating one deliberately requires a real
			// login, so a read token cannot mint itself a write token.
			r.Get("/tokens", server.HandleListTokens)
			r.Post("/tokens", server.HandleCreateToken)
			r.Delete("/tokens/{tokenID}", server.HandleRevokeToken)

			// Strava. The callback is registered outside this group because
			// it is a browser redirect carrying a signed state rather than a
			// bearer token.
			r.Get("/strava/status", server.HandleStravaStatus)
			r.Post("/strava/connect", server.HandleStravaConnect)
			r.Post("/strava/sync", server.HandleStravaSync)
			r.Delete("/strava", server.HandleStravaDisconnect)

			r.Get("/photos", server.HandleListPhotos)
			r.Post("/photos", server.HandleUploadPhoto)
			r.Get("/photos/{photoID}/file", server.HandleServePhoto)
			r.Patch("/photos/{photoID}", server.HandleUpdatePhoto)
			r.Get("/programs/{programID}/roll", server.HandleGetRoll)
			r.Delete("/photos/{photoID}", server.HandleDeletePhoto)

			r.Post("/meals", server.HandleCreateMeal)
			// Re-runs a background estimate that failed — usually because the
			// daily AI limit was reached, which clears on its own.
			r.Post("/meals/{mealID}/estimate", server.HandleRetryEstimate)
			r.Patch("/meals/{mealID}", server.HandleUpdateMeal)
			r.Delete("/meals/{mealID}", server.HandleDeleteMeal)

			r.Post("/workouts", server.HandleCreateWorkout)
			r.Delete("/workouts/{workoutID}", server.HandleDeleteWorkout)

			// Optional tracking; never affects whether a day counts.
			r.Post("/meditations", server.HandleCreateMeditation)
			r.Delete("/meditations/{meditationID}", server.HandleDeleteMeditation)

			// AI features. Rate limited far harder than the rest: each of
			// these is a paid upstream call, not a database read.
			r.Group(func(r chi.Router) {
				r.Use(api.RateLimitMiddleware(20))
				// A recipe or plan request against a large model routinely
				// takes most of a minute, so the 60s budget that suits a
				// database read would cut it off mid-answer — and the call is
				// already paid for by then.
				r.Use(middleware.Timeout(aiRequestTimeout))
				r.Get("/ai/status", server.HandleAIStatus)
				// What credit is left, for providers that publish it.
				r.Get("/ai/balance", server.HandleAIBalance)
			})

			// Provider keys are configuration, not model calls, so they sit
			// outside the AI rate limit and its long timeout.
			r.Get("/ai/keys", server.HandleListAIKeys)
			r.Put("/ai/keys/{slot}", server.HandleSaveAIKey)
			r.Delete("/ai/keys/{slot}", server.HandleDeleteAIKey)

			r.Group(func(r chi.Router) {
				r.Use(api.RateLimitMiddleware(20))
				r.Use(middleware.Timeout(aiRequestTimeout))
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

	// Started before the listener so a restart's pending estimates are already
	// being collected as the first requests arrive.
	if estimator != nil {
		estimatorCtx, stopEstimator := context.WithCancel(context.Background())
		defer stopEstimator()
		estimator.Start(estimatorCtx)
	}

	// Strava activities arrive without telling us, so connected accounts are
	// polled. Without this, a walk only appears when somebody opens settings
	// and presses sync, which is not something anyone remembers to do.
	var stravaSyncer *api.StravaSyncer
	if cfg.StravaEnabled() {
		stravaSyncer = api.NewStravaSyncer(server, cfg.StravaSyncInterval)
		syncCtx, stopSync := context.WithCancel(context.Background())
		defer stopSync()
		stravaSyncer.Start(syncCtx)
		logger.Info("strava auto-sync enabled",
			zap.Duration("every", stravaSyncer.Interval()))
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
	// Anything in flight stays 'pending' and is requeued on the next boot,
	// which is a far better outcome than holding shutdown open for a model.
	if estimator != nil {
		estimator.Stop()
	}
	if stravaSyncer != nil {
		stravaSyncer.Stop()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", zap.Error(err))
	}
	logger.Info("stopped")
}

// boundedChain builds a provider chain with a per-slot timeout that leaves
// room for the fallback.
//
// go-ai's default per-request timeout is 150s. Inside the app's 3-minute call
// budget that means one unresponsive provider can consume almost everything,
// and the chain never reaches a backup — which is exactly how food estimation
// broke when NVIDIA stopped responding. A slot bounded well below the budget
// keeps the fallback reachable. An explicit AI_n_TIMEOUT_SECONDS still wins.
func boundedChain(prefix string) (*ai.Chain, error) {
	slots := ai.SlotsFromEnv(prefix)
	for i := range slots {
		if slots[i].Timeout == 0 {
			slots[i].Timeout = api.AISlotTimeout
		}
	}
	chain, err := ai.NewChainFromSlots(slots...)
	if err != nil {
		return nil, err
	}

	policy := ai.RetryFromEnv(prefix)
	if os.Getenv(prefix+"_MAX_ATTEMPTS") == "" {
		policy.MaxAttempts = api.AIMaxAttempts
	}
	return chain.WithRetry(policy), nil
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
