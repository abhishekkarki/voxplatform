package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Event is a single entry in the append-only inference event log.
// Every stage of a pipeline request emits at least two events:
// one when it starts and one when it completes (or fails).
type Event struct {
	RequestID string    `json:"request_id"`
	Timestamp time.Time `json:"timestamp"`
	// Type is one of: pipeline.start, stage.start, stage.complete,
	// stage.error, pipeline.complete, pipeline.error
	Type  string `json:"type"`
	Stage string `json:"stage,omitempty"`
	// Data holds stage-specific payload (e.g. transcript text, duration).
	Data map[string]any `json:"data,omitempty"`
}

// EventLogger is the interface every backend must implement.
type EventLogger interface {
	// Log appends one event. Implementations must be safe for concurrent use.
	Log(ctx context.Context, e Event) error
	// Close flushes pending writes and releases resources.
	Close() error
}

// NewEventLogger constructs the right logger based on cfg.
//   - "local" (default) → LocalFileLogger writing JSONL to cfg.EventLogDir
//   - "gcs"             → GCSLogger writing JSONL to cfg.EventLogBucket
//   - anything else     → NoOpLogger (useful in tests)
func NewEventLogger(cfg Config) EventLogger {
	switch cfg.EventLogBackend {
	case "gcs":
		if cfg.EventLogBucket == "" {
			slog.Warn("EVENT_LOG_BACKEND=gcs but EVENT_LOG_BUCKET is empty — falling back to local")
		} else {
			return newGCSLogger(cfg.EventLogBucket)
		}
		fallthrough
	case "local":
		if err := os.MkdirAll(cfg.EventLogDir, 0o750); err != nil {
			slog.Warn("cannot create event log dir, using no-op logger", "dir", cfg.EventLogDir, "error", err)
			return &noOpLogger{}
		}
		return &localFileLogger{dir: cfg.EventLogDir}
	default:
		return &noOpLogger{}
	}
}

// --- Local file backend ---

// localFileLogger writes one JSONL file per request-ID to a local directory.
// Each line is a single JSON-encoded Event. Files are named by request ID so
// replaying a request is as simple as: cat <dir>/<request_id>.jsonl | jq .
type localFileLogger struct {
	dir string
	mu  sync.Mutex
	fds map[string]*os.File // open file handles, keyed by request_id
}

func (l *localFileLogger) Log(_ context.Context, e Event) error {
	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshalling event: %w", err)
	}
	line = append(line, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.fds == nil {
		l.fds = make(map[string]*os.File)
	}

	f, ok := l.fds[e.RequestID]
	if !ok {
		path := filepath.Join(l.dir, e.RequestID+".jsonl")
		f, err = os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
		if err != nil {
			return fmt.Errorf("opening event log file: %w", err)
		}
		l.fds[e.RequestID] = f
	}

	_, err = f.Write(line)
	return err
}

func (l *localFileLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	var lastErr error
	for _, f := range l.fds {
		if err := f.Close(); err != nil {
			lastErr = err
		}
	}
	l.fds = nil
	return lastErr
}

// --- GCS backend ---

// gcsLogger writes events to Google Cloud Storage as JSONL objects.
// Authentication uses the GKE metadata server (Workload Identity) — no
// credentials file needed when running on GKE. For local testing use the
// local file backend instead.
type gcsLogger struct {
	bucket string
	client *http.Client
}

func newGCSLogger(bucket string) *gcsLogger {
	return &gcsLogger{bucket: bucket, client: &http.Client{Timeout: 10 * time.Second}}
}

func (g *gcsLogger) Log(ctx context.Context, e Event) error {
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	// Object path: events/YYYY-MM-DD/<request_id>.jsonl
	// We append to the object using a compose trick, but for simplicity
	// each event is written as a separate small object and can be merged
	// offline. Production deployments should batch-buffer writes.
	objectName := fmt.Sprintf("events/%s/%s/%s.jsonl",
		e.Timestamp.UTC().Format("2006-01-02"),
		e.RequestID,
		e.Type,
	)

	token, err := g.fetchMetadataToken(ctx)
	if err != nil {
		return fmt.Errorf("fetching GCS token: %w", err)
	}

	url := fmt.Sprintf(
		"https://storage.googleapis.com/upload/storage/v1/b/%s/o?uploadType=media&name=%s",
		g.bucket, objectName,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(line))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-ndjson")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("GCS upload: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("GCS upload returned %d", resp.StatusCode)
	}
	return nil
}

func (g *gcsLogger) fetchMetadataToken(ctx context.Context) (string, error) {
	const metadataURL = "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Metadata-Flavor", "Google")

	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("metadata server unavailable: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parsing token response: %w", err)
	}
	return result.AccessToken, nil
}

func (g *gcsLogger) Close() error { return nil }

// --- No-op backend ---

type noOpLogger struct{}

func (n *noOpLogger) Log(_ context.Context, _ Event) error { return nil }
func (n *noOpLogger) Close() error                         { return nil }
