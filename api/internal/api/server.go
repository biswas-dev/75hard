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
}

// NewServer builds a Server.
func NewServer(database *db.DB, cfg *config.Config, log *zap.Logger, photos *photo.Store, aiSvc *aifeatures.Service) *Server {
	SetLogger(log)
	return &Server{db: database, cfg: cfg, log: log, photos: photos, ai: aiSvc}
}
