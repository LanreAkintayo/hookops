package server

import (
	"context"
	"net/http"
	"time"

	"github.com/rs/zerolog"

	"github.com/LanreAkintayo/hookops/internal/config"
)

// Server manages the lifecycle of the HTTP listener (start & graceful shutdown).
type Server struct {
	httpServer *http.Server
	logger     zerolog.Logger
}

// New wraps an http.Handler with production timeouts and binds it to the configured port.
// It is completely decoupled from any routing framework (Gin, stdlib, etc.).
func New(cfg *config.Config, log zerolog.Logger, handler http.Handler) *Server {
	return &Server{
		logger: log,
		httpServer: &http.Server{
			Addr:         ":" + cfg.Server.Port,
			Handler:      handler,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		},
	}
}

// Start runs the HTTP server listener. It blocks until the server closes or returns an error.
func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully terminates the HTTP server, giving in-flight requests time to complete.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
