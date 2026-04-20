package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/abhishekkarki/voxplatform/internal/gateway"
)

func main() {
	// Structured logging — standard in Go since 1.21
	// JSON format so it's parseable by Loki/Cloud Logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Load config from environment variables
	// This is the 12-factor app way — config comes from the environment, not files
	cfg := gateway.LoadConfig()

	// Create the gateway server
	srv := gateway.NewServer(cfg, logger)

	// Create the HTTP server
	httpServer := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      srv.Router(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second, // Long timeout — transcription can take a while
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		slog.Info("gateway starting", "port", cfg.Port, "whisper_url", cfg.WhisperURL)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown — wait for SIGINT or SIGTERM
	// Kubernetes sends SIGTERM when it wants to stop your pod
	// This gives in-flight requests time to finish before the pod dies
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down gracefully")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("forced shutdown", "error", err)
	}

	slog.Info("gateway stopped")
}
