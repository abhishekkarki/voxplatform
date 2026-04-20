package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

// WhisperProxy handles communication with the Whisper model server.
// This is the "reverse proxy" pattern — the gateway receives the client's request,
// rebuilds it as a new request to the backend, and returns the backend's response.
//
// Why not just use httputil.ReverseProxy?
// Because we want control over error handling, request transformation,
// and response enrichment. A raw reverse proxy just forwards bytes blindly.
type WhisperProxy struct {
	baseURL string
	client  *http.Client
}

// WhisperResult is the response from the Whisper model server.
type WhisperResult struct {
	Text string `json:"text"`
}

func NewWhisperProxy(baseURL string) *WhisperProxy {
	return &WhisperProxy{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 120 * time.Second, // Transcription can be slow on CPU
		},
	}
}

// HealthCheck pings Whisper's /health endpoint.
// Used by the readiness probe to check if the backend is alive.
func (p *WhisperProxy) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("creating health request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("whisper unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("whisper returned status %d", resp.StatusCode)
	}

	return nil
}

// Transcribe sends an audio file to Whisper and returns the transcription.
// This rebuilds the multipart form because the original request's form data
// has already been consumed by the gateway's handler.
func (p *WhisperProxy) Transcribe(
	ctx context.Context,
	file multipart.File,
	header *multipart.FileHeader,
	req TranscriptionRequest,
) (*WhisperResult, error) {

	// Build a new multipart form for the backend request.
	// We can't just forward the original request body because
	// the handler already parsed it to validate and log the request.
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Copy the audio file into the new form
	part, err := writer.CreateFormFile("file", header.Filename)
	if err != nil {
		return nil, fmt.Errorf("creating form file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("copying file data: %w", err)
	}

	// Add the model parameter
	if err := writer.WriteField("model", req.Model); err != nil {
		return nil, fmt.Errorf("writing model field: %w", err)
	}

	// Add optional language parameter
	if req.Language != "" {
		if err := writer.WriteField("language", req.Language); err != nil {
			return nil, fmt.Errorf("writing language field: %w", err)
		}
	}

	// Add optional response format
	if req.ResponseFormat != "" {
		if err := writer.WriteField("response_format", req.ResponseFormat); err != nil {
			return nil, fmt.Errorf("writing response_format field: %w", err)
		}
	}

	writer.Close()

	// Send to Whisper
	whisperURL := p.baseURL + "/v1/audio/transcriptions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, whisperURL, &buf)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending to whisper: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("whisper returned %d: %s", resp.StatusCode, string(body))
	}

	// Parse the JSON response
	var result WhisperResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	return &result, nil
}
