// Package api holds the HTTP server: middleware, handlers and the types they
// exchange with the SPA.
package api

import (
	gologin "github.com/anchoo2kewl/go-login"

	"github.com/anchoo2kewl/75hard/api/internal/aifeatures"
	"github.com/anchoo2kewl/75hard/api/internal/config"
	"github.com/anchoo2kewl/75hard/api/internal/db"
	"github.com/anchoo2kewl/75hard/api/internal/photo"
	"go.uber.org/zap"
)

// Server carries the dependencies every handler needs.
type Server struct {
	db     *db.DB
	cfg    *config.Config
	log    *zap.Logger
	photos *photo.Store
	ai     *aifeatures.Service
	// food, when set, estimates food photos off the request path. Nil is a
	// valid state: without it a food photo is simply saved without numbers.
	food *FoodEstimator
	// journalParser, when set, transcribes handwritten pages. Nil means an
	// uploaded page is stored and simply not searchable.
	journalParser *JournalParser
	// passkeys runs the WebAuthn ceremonies. Nil when the app URL is one a
	// browser will not accept a credential for, such as a bare IP or plain
	// http on anything but localhost.
	passkeys *gologin.Passkeys
}

// NewServer builds a Server.
func NewServer(database *db.DB, cfg *config.Config, log *zap.Logger, photos *photo.Store, aiSvc *aifeatures.Service) *Server {
	SetLogger(log)
	s := &Server{db: database, cfg: cfg, log: log, photos: photos, ai: aiSvc}

	// Passkeys are bound to an origin by design, so a server reachable at an
	// address the browser will not accept simply does not offer them rather
	// than failing at the point of use.
	if gologin.PasskeysUsable(cfg.AppURL) {
		pk, err := gologin.NewPasskeys(gologin.PasskeyConfig{
			DisplayName: "75 Hard",
			AppURL:      cfg.AppURL,
		}, passkeyStore{s})
		if err != nil {
			log.Warn("passkeys unavailable", zap.Error(err))
		} else {
			s.passkeys = pk
		}
	} else {
		log.Info("passkeys off: app url is not a valid relying party",
			zap.String("app_url", cfg.AppURL))
	}
	return s
}

// SetFoodEstimator attaches the background estimator. Separate from NewServer
// because the estimator needs the server it runs against.
func (s *Server) SetFoodEstimator(e *FoodEstimator) { s.food = e }

// SetJournalParser attaches the background transcriber.
func (s *Server) SetJournalParser(p *JournalParser) { s.journalParser = p }
