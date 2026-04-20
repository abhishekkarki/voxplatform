package gateway

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// handleHealth is the liveness probe - Kubernetes calls this to check
// "is the process alive?" If this fails, K8s restarts the pod.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

// handleReady is the readiness probe - Kubernetes calls this to check
// "can this pod handle traffic?"
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.proxy.HealthCheck(ctx); err != nil {
		slog.Warn("readiness check failed",
			"error", err,
			"request_id", RequestIDFromContext(ctx),
		)
		writeError(w, r, http.StatusServiceUnavailable,
			ErrCodeServiceUnavail, "whisper backend unreachable")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ready",
	})
}

// TranscriptionRequest represents what the client sends.
type TranscriptionRequest struct {
	Model          string
	Language       string
	ResponseFormat string
}

// TranscriptionResponse is what we return to the client.
type TranscriptionResponse struct {
	Text      string  `json:"text"`
	Model     string  `json:"model"`
	Duration  float64 `json:"processing_time_seconds"`
	RequestID string  `json:"request_id"`
	CreatedAt string  `json:"created_at"`
}

// handleTranscribe is the core endpoint - receives audio, forwards to Whisper,
// enriches the response with metadata, and returns it.
func (s *Server) handleTranscribe(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx := r.Context()
	requestID := RequestIDFromContext(ctx)

	// Parse the multipart form - 32MB max file size
	maxBytes := int64(32 << 20)
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

	if err := r.ParseMultipartForm(maxBytes); err != nil {
		slog.Warn("invalid request",
			"error", err,
			"request_id", requestID,
		)
		writeError(w, r, http.StatusBadRequest,
			ErrCodeFileTooLarge, "invalid multipart form or file too large (max 32MB)")
		return
	}
	defer r.MultipartForm.RemoveAll()

	// Get the audio file from the form
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, r, http.StatusBadRequest,
			ErrCodeFileRequired, "missing 'file' field in form data")
		return
	}
	defer file.Close()

	// Extract optional parameters
	req := TranscriptionRequest{
		Model:          r.FormValue("model"),
		Language:       r.FormValue("language"),
		ResponseFormat: r.FormValue("response_format"),
	}

	if req.Model == "" {
		req.Model = "Systran/faster-whisper-small.en"
	}

	slog.Info("transcription request",
		"request_id", requestID,
		"filename", header.Filename,
		"size_bytes", header.Size,
		"model", req.Model,
	)

	// Forward to Whisper backend
	result, err := s.proxy.Transcribe(ctx, file, header, req)
	if err != nil {
		slog.Error("transcription failed",
			"error", err,
			"request_id", requestID,
		)
		writeError(w, r, http.StatusBadGateway,
			ErrCodeBackendError, "whisper backend error: "+err.Error())
		return
	}

	response := TranscriptionResponse{
		Text:      result.Text,
		Model:     req.Model,
		Duration:  time.Since(start).Seconds(),
		RequestID: requestID,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	slog.Info("transcription complete",
		"request_id", requestID,
		"duration_seconds", response.Duration,
		"text_length", len(response.Text),
	)

	writeJSON(w, http.StatusOK, response)
}

// handleListModels returns available models.
func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	models := []map[string]string{
		{
			"id":    "Systran/faster-whisper-small.en",
			"type":  "stt",
			"state": "loaded",
		},
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"models": models,
	})
}

// handleMetrics serves Prometheus metrics.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	promHandler.ServeHTTP(w, r)
}

// writeJSON helper.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
