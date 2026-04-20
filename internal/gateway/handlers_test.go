package gateway

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestServer creates a Server pointing at a fake Whisper backend.
// This is the test setup pattern - you create a real HTTP test server
// that pretends to be Whisper, so your gateway code runs against it
// without needing a real model server.
func newTestServer(t *testing.T, whisperHandler http.HandlerFunc) (*Server, *httptest.Server) {
	t.Helper()

	// Create a fake Whisper backend
	whisper := httptest.NewServer(whisperHandler)

	cfg := Config{
		Port:       "0", // OS picks a free port
		WhisperURL: whisper.URL,
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(cfg, logger)

	return srv, whisper
}

func TestHealthEndpoint(t *testing.T) {
	srv, whisper := newTestServer(t, nil)
	defer whisper.Close()

	// Table-driven test - each case is a row in the table.
	// This is THE standard Go testing pattern. Interviewers expect to see it.
	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "liveness returns ok",
			method:     http.MethodGet,
			path:       "/healthz",
			wantStatus: http.StatusOK,
			wantBody:   `"status":"ok"`,
		},
		{
			name:       "wrong method returns 405",
			method:     http.MethodPost,
			path:       "/healthz",
			wantStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()

			srv.Router().ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			if tt.wantBody != "" {
				body := rec.Body.String()
				if !bytes.Contains(rec.Body.Bytes(), []byte(tt.wantBody)) {
					t.Errorf("body = %s, want to contain %s", body, tt.wantBody)
				}
			}
		})
	}
}

func TestReadinessEndpoint(t *testing.T) {
	tests := []struct {
		name          string
		whisperStatus int
		wantStatus    int
		wantBody      string
	}{
		{
			name:          "ready when whisper is healthy",
			whisperStatus: http.StatusOK,
			wantStatus:    http.StatusOK,
			wantBody:      `"status":"ready"`,
		},
		{
			name:          "not ready when whisper returns 500",
			whisperStatus: http.StatusInternalServerError,
			wantStatus:    http.StatusServiceUnavailable,
			wantBody:      `"code":"service_unavailable"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, whisper := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.whisperStatus)
			})
			defer whisper.Close()

			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			rec := httptest.NewRecorder()

			srv.Router().ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			if !bytes.Contains(rec.Body.Bytes(), []byte(tt.wantBody)) {
				t.Errorf("body = %s, want to contain %s", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestReadinessWhenWhisperDown(t *testing.T) {
	cfg := Config{
		Port:       "0",
		WhisperURL: "http://localhost:1", // Nothing running here
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(cfg, logger)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestListModels(t *testing.T) {
	srv, whisper := newTestServer(t, nil)
	defer whisper.Close()

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	models, ok := resp["models"].([]any)
	if !ok || len(models) == 0 {
		t.Error("expected at least one model in response")
	}
}

func TestTranscribeSuccess(t *testing.T) {
	// Fake Whisper backend that returns a canned response
	srv, whisper := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/audio/transcriptions" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"text": "hello world",
			})
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	defer whisper.Close()

	// Build a multipart form with a fake audio file
	body, contentType := createMultipartForm(t, "test.wav", []byte("fake audio data"), "Systran/faster-whisper-small.en")

	req := httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp TranscriptionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if resp.Text != "hello world" {
		t.Errorf("text = %q, want %q", resp.Text, "hello world")
	}

	if resp.RequestID == "" {
		t.Error("expected request_id in response")
	}

	if resp.Duration <= 0 {
		t.Error("expected positive processing_time_seconds")
	}
}

func TestTranscribeMissingFile(t *testing.T) {
	srv, whisper := newTestServer(t, nil)
	defer whisper.Close()

	// Send an empty POST with no file
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", nil)
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestTranscribeWhisperError(t *testing.T) {
	// Whisper backend that returns an error
	srv, whisper := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/audio/transcriptions" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("model crashed"))
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	defer whisper.Close()

	body, contentType := createMultipartForm(t, "test.wav", []byte("fake audio"), "")

	req := httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}

	// Check structured error format
	var errResp APIErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("invalid error JSON: %v", err)
	}

	if errResp.Error.Code != ErrCodeBackendError {
		t.Errorf("error code = %q, want %q", errResp.Error.Code, ErrCodeBackendError)
	}

	if errResp.Error.RequestID == "" {
		t.Error("expected request_id in error response")
	}
}

func TestRequestIDPropagation(t *testing.T) {
	srv, whisper := newTestServer(t, nil)
	defer whisper.Close()

	// Send a request with a custom X-Request-ID
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-ID", "test-id-12345")
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)

	// Response should echo the same request ID
	gotID := rec.Header().Get("X-Request-ID")
	if gotID != "test-id-12345" {
		t.Errorf("X-Request-ID = %q, want %q", gotID, "test-id-12345")
	}
}

func TestRequestIDGenerated(t *testing.T) {
	srv, whisper := newTestServer(t, nil)
	defer whisper.Close()

	// Send a request WITHOUT X-Request-ID
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)

	gotID := rec.Header().Get("X-Request-ID")
	if gotID == "" {
		t.Error("expected generated X-Request-ID in response")
	}
	if len(gotID) != 16 { // 8 bytes = 16 hex chars
		t.Errorf("X-Request-ID length = %d, want 16", len(gotID))
	}
}

// createMultipartForm builds a multipart form body with a file field.
// This is a test helper - Go's testing convention is to put helpers
// at the bottom of the test file and mark them with t.Helper().
func createMultipartForm(t *testing.T, filename string, data []byte, model string) (*bytes.Buffer, string) {
	t.Helper()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("creating form file: %v", err)
	}
	part.Write(data)

	if model != "" {
		writer.WriteField("model", model)
	}

	writer.Close()

	return &buf, writer.FormDataContentType()
}
