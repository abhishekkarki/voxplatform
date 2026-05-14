package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// bytesFile wraps bytes.Reader so it satisfies multipart.File
// (which requires Read + ReadAt + Seek + Close).
type bytesFile struct{ *bytes.Reader }

func (b *bytesFile) Close() error { return nil }

// PipelineOrchestrator runs the multi-stage inference pipeline:
// STT → diarize → summarize. Each stage is optional and gracefully
// skipped if its backend URL is empty or the stage is not requested.
type PipelineOrchestrator struct {
	proxy       *WhisperProxy
	diarizerURL string
	summarizerURL string
	events      EventLogger
}

func NewPipelineOrchestrator(cfg Config, proxy *WhisperProxy, events EventLogger) *PipelineOrchestrator {
	return &PipelineOrchestrator{
		proxy:         proxy,
		diarizerURL:   cfg.DiarizerURL,
		summarizerURL: cfg.SummarizerURL,
		events:        events,
	}
}

// PipelineResponse is the JSON envelope returned by POST /v1/pipeline/run.
type PipelineResponse struct {
	RequestID        string                   `json:"request_id"`
	CreatedAt        string                   `json:"created_at"`
	ProcessingSeconds float64                 `json:"processing_time_seconds"`
	Stages           map[string]stageResult   `json:"stages"`
	Transcript       string                   `json:"transcript"`
	Segments         []speakerSegment         `json:"segments,omitempty"`
	Summary          string                   `json:"summary,omitempty"`
}

type stageResult struct {
	DurationSeconds float64 `json:"duration_seconds"`
	Success         bool    `json:"success"`
	Error           string  `json:"error,omitempty"`
}

type speakerSegment struct {
	Start   float64 `json:"start"`
	End     float64 `json:"end"`
	Speaker string  `json:"speaker"`
	Text    string  `json:"text,omitempty"`
}

// handlePipeline is the HTTP handler for POST /v1/pipeline/run.
// It reads the audio file from the multipart form, runs each enabled stage
// in sequence, logs events for each stage, and returns the aggregated result.
func (s *Server) handlePipeline(w http.ResponseWriter, r *http.Request) {
	pipelineStart := time.Now()
	ctx := r.Context()
	requestID := RequestIDFromContext(ctx)

	maxBytes := int64(32 << 20)
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

	if err := r.ParseMultipartForm(maxBytes); err != nil {
		writeError(w, r, http.StatusBadRequest, ErrCodeFileTooLarge,
			"invalid multipart form or file too large (max 32MB)")
		return
	}
	defer r.MultipartForm.RemoveAll()

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, r, http.StatusBadRequest, ErrCodeFileRequired,
			"missing 'file' field in form data")
		return
	}
	defer file.Close()

	// Read audio into memory once so each stage can access it
	audioBytes, err := io.ReadAll(file)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, ErrCodeInternalError,
			"reading audio file")
		return
	}

	// Stages to run (default: all)
	stagesParam := r.FormValue("stages")
	enabledStages := parseStages(stagesParam)

	slog.Info("pipeline request",
		"request_id", requestID,
		"filename", header.Filename,
		"size_bytes", len(audioBytes),
		"stages", strings.Join(enabledStages, ","),
	)

	_ = s.pipeline.events.Log(ctx, Event{
		RequestID: requestID,
		Timestamp: time.Now().UTC(),
		Type:      "pipeline.start",
		Data:      map[string]any{"stages": enabledStages, "filename": header.Filename},
	})

	resp := PipelineResponse{
		RequestID: requestID,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Stages:    make(map[string]stageResult),
	}

	// ── Stage 1: STT ────────────────────────────────────────────────────────
	if contains(enabledStages, "stt") {
		stageStart := time.Now()
		_ = s.pipeline.events.Log(ctx, Event{
			RequestID: requestID, Timestamp: time.Now().UTC(), Type: "stage.start", Stage: "stt",
		})

		transcript, sttErr := s.pipeline.runSTT(ctx, audioBytes, header, r.FormValue("model"), r.FormValue("language"))
		dur := time.Since(stageStart).Seconds()

		if sttErr != nil {
			slog.Error("STT stage failed", "request_id", requestID, "error", sttErr)
			resp.Stages["stt"] = stageResult{DurationSeconds: dur, Success: false, Error: sttErr.Error()}
			_ = s.pipeline.events.Log(ctx, Event{
				RequestID: requestID, Timestamp: time.Now().UTC(), Type: "stage.error", Stage: "stt",
				Data: map[string]any{"error": sttErr.Error()},
			})
			// STT is required — abort the pipeline if it fails
			writeError(w, r, http.StatusBadGateway, ErrCodeBackendError,
				"STT stage failed: "+sttErr.Error())
			return
		}

		resp.Transcript = transcript
		resp.Stages["stt"] = stageResult{DurationSeconds: dur, Success: true}
		_ = s.pipeline.events.Log(ctx, Event{
			RequestID: requestID, Timestamp: time.Now().UTC(), Type: "stage.complete", Stage: "stt",
			Data: map[string]any{"duration_seconds": dur, "text_length": len(transcript)},
		})
	}

	// ── Stage 2: Diarize ────────────────────────────────────────────────────
	if contains(enabledStages, "diarize") && s.pipeline.diarizerURL != "" {
		stageStart := time.Now()
		_ = s.pipeline.events.Log(ctx, Event{
			RequestID: requestID, Timestamp: time.Now().UTC(), Type: "stage.start", Stage: "diarize",
		})

		segments, diarizeErr := s.pipeline.runDiarize(ctx, audioBytes, header.Filename)
		dur := time.Since(stageStart).Seconds()

		if diarizeErr != nil {
			slog.Warn("diarize stage failed (non-fatal)", "request_id", requestID, "error", diarizeErr)
			resp.Stages["diarize"] = stageResult{DurationSeconds: dur, Success: false, Error: diarizeErr.Error()}
			_ = s.pipeline.events.Log(ctx, Event{
				RequestID: requestID, Timestamp: time.Now().UTC(), Type: "stage.error", Stage: "diarize",
				Data: map[string]any{"error": diarizeErr.Error()},
			})
		} else {
			resp.Segments = segments
			resp.Stages["diarize"] = stageResult{DurationSeconds: dur, Success: true}
			_ = s.pipeline.events.Log(ctx, Event{
				RequestID: requestID, Timestamp: time.Now().UTC(), Type: "stage.complete", Stage: "diarize",
				Data: map[string]any{"duration_seconds": dur, "num_segments": len(segments)},
			})
		}
	}

	// ── Stage 3: Summarize ──────────────────────────────────────────────────
	if contains(enabledStages, "summarize") && s.pipeline.summarizerURL != "" && resp.Transcript != "" {
		stageStart := time.Now()
		_ = s.pipeline.events.Log(ctx, Event{
			RequestID: requestID, Timestamp: time.Now().UTC(), Type: "stage.start", Stage: "summarize",
		})

		summary, sumErr := s.pipeline.runSummarize(ctx, resp.Transcript, resp.Segments)
		dur := time.Since(stageStart).Seconds()

		if sumErr != nil {
			slog.Warn("summarize stage failed (non-fatal)", "request_id", requestID, "error", sumErr)
			resp.Stages["summarize"] = stageResult{DurationSeconds: dur, Success: false, Error: sumErr.Error()}
			_ = s.pipeline.events.Log(ctx, Event{
				RequestID: requestID, Timestamp: time.Now().UTC(), Type: "stage.error", Stage: "summarize",
				Data: map[string]any{"error": sumErr.Error()},
			})
		} else {
			resp.Summary = summary
			resp.Stages["summarize"] = stageResult{DurationSeconds: dur, Success: true}
			_ = s.pipeline.events.Log(ctx, Event{
				RequestID: requestID, Timestamp: time.Now().UTC(), Type: "stage.complete", Stage: "summarize",
				Data: map[string]any{"duration_seconds": dur},
			})
		}
	}

	resp.ProcessingSeconds = time.Since(pipelineStart).Seconds()

	_ = s.pipeline.events.Log(ctx, Event{
		RequestID: requestID,
		Timestamp: time.Now().UTC(),
		Type:      "pipeline.complete",
		Data:      map[string]any{"duration_seconds": resp.ProcessingSeconds},
	})

	slog.Info("pipeline complete",
		"request_id", requestID,
		"duration_seconds", resp.ProcessingSeconds,
		"stages_run", len(resp.Stages),
	)

	writeJSON(w, http.StatusOK, resp)
}

// runSTT calls the Whisper backend and returns the transcript text.
func (o *PipelineOrchestrator) runSTT(
	ctx context.Context,
	audioBytes []byte,
	originalHeader *multipart.FileHeader,
	model, language string,
) (string, error) {
	req := TranscriptionRequest{Model: model, Language: language}
	if req.Model == "" {
		req.Model = "Systran/faster-whisper-small.en"
	}

	fh := &multipart.FileHeader{
		Filename: originalHeader.Filename,
		Size:     int64(len(audioBytes)),
		Header:   originalHeader.Header,
	}

	result, err := o.proxy.Transcribe(ctx, &bytesFile{bytes.NewReader(audioBytes)}, fh, req)
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

// runDiarize calls the diarizer service and returns speaker segments.
func (o *PipelineOrchestrator) runDiarize(
	ctx context.Context,
	audioBytes []byte,
	filename string,
) ([]speakerSegment, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("creating form file: %w", err)
	}
	if _, err := fw.Write(audioBytes); err != nil {
		return nil, fmt.Errorf("writing audio to form: %w", err)
	}
	mw.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.diarizerURL+"/diarize", &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("diarizer request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("diarizer returned %d", resp.StatusCode)
	}

	var result struct {
		Segments []speakerSegment `json:"segments"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding diarizer response: %w", err)
	}
	return result.Segments, nil
}

// runSummarize calls the summarizer service and returns a summary string.
func (o *PipelineOrchestrator) runSummarize(
	ctx context.Context,
	transcript string,
	segments []speakerSegment,
) (string, error) {
	body, err := json.Marshal(map[string]any{
		"transcript": transcript,
		"segments":   segments,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.summarizerURL+"/summarize", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("summarizer request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("summarizer returned %d", resp.StatusCode)
	}

	var result struct {
		Summary string `json:"summary"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding summarizer response: %w", err)
	}
	return result.Summary, nil
}

// parseStages returns the list of stages to run.
// If the "stages" form field is empty, all three stages are enabled.
func parseStages(param string) []string {
	if param == "" {
		return []string{"stt", "diarize", "summarize"}
	}
	parts := strings.Split(param, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
