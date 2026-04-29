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
func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()

	// Health endpoints — Kubernetes probes
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /readyz", s.handleReady)

	// Metrics endpoint — Prometheus scrapes this
	mux.HandleFunc("GET /metrics", s.handleMetrics)

	// Batch API routes
	mux.HandleFunc("POST /v1/audio/transcriptions", s.handleTranscribe)
	mux.HandleFunc("GET /v1/models", s.handleListModels)

	// Streaming API — WebSocket endpoint
	mux.HandleFunc("/v1/audio/stream", s.handleStream)

	// Middleware chain: requestID → logging → metrics → recovery → handler
	var handler http.Handler = mux
	handler = s.recoveryMiddleware(handler)
	handler = s.metricsMiddleware(handler)
	handler = s.loggingMiddleware(handler)
	handler = s.requestIDMiddleware(handler)

	return handler
}
