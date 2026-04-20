package gateway

import (
	"log/slog"
	"net/http"
)

// Server holds all dependencies the handlers need.
type Server struct {
	cfg    Config
	logger *slog.Logger
	proxy  *WhisperProxy
}

func NewServer(cfg Config, logger *slog.Logger) *Server {
	return &Server{
		cfg:    cfg,
		logger: logger,
		proxy:  NewWhisperProxy(cfg.WhisperURL),
	}
}

// Router sets up all routes and wraps them with middleware.
// We use the standard library's http.ServeMux (improved in Go 1.22).
func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()

	// Health endpoints - Kubernetes probes hit these
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /readyz", s.handleReady)

	// Metrics endpoint - Prometheus scrapes this
	mux.HandleFunc("GET /metrics", s.handleMetrics)

	// API routes
	mux.HandleFunc("POST /v1/audio/transcriptions", s.handleTranscribe)
	mux.HandleFunc("GET /v1/models", s.handleListModels)

	// Middleware runs outside-in:
	// request → requestID → logging → metrics → recovery → handler
	// response ← requestID ← logging ← metrics ← recovery ← handler
	var handler http.Handler = mux
	handler = s.recoveryMiddleware(handler)
	handler = s.metricsMiddleware(handler)
	handler = s.loggingMiddleware(handler)
	handler = s.requestIDMiddleware(handler)

	return handler
}
