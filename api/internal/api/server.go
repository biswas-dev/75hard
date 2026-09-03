// Package api holds the HTTP server: middleware, handlers and the types they
// exchange with the SPA.
package api

import (
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
}

// NewServer builds a Server.
func NewServer(database *db.DB, cfg *config.Config, log *zap.Logger, photos *photo.Store, aiSvc *aifeatures.Service) *Server {
	SetLogger(log)
	return &Server{db: database, cfg: cfg, log: log, photos: photos, ai: aiSvc}
}

// SetFoodEstimator attaches the background estimator. Separate from NewServer
// because the estimator needs the server it runs against.
func (s *Server) SetFoodEstimator(e *FoodEstimator) { s.food = e }

// SetJournalParser attaches the background transcriber.
func (s *Server) SetJournalParser(p *JournalParser) { s.journalParser = p }
