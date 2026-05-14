package gateway

import "os"

// Config holds all gateway configuration.
// Every field comes from an environment variable with a sensible default.
// This is the 12-factor app approach: no config files, just env vars
// that Kubernetes sets via the Helm values.

type Config struct {
	Port        string // PORT — what port the gateway listens on
	WhisperURL  string // WHISPER_URL — STT backend
	VADEndpoint string // VAD_ENDPOINT — VAD sidecar

	// Pipeline stage backends (optional — pipeline endpoint requires these)
	DiarizerURL   string // DIARIZER_URL — speaker diarization service
	SummarizerURL string // SUMMARIZER_URL — summarization service

	// Event log configuration
	EventLogBackend string // EVENT_LOG_BACKEND — "local" | "gcs" (default: "local")
	EventLogDir     string // EVENT_LOG_DIR — directory for local JSONL files
	EventLogBucket  string // EVENT_LOG_BUCKET — GCS bucket name (gcs backend only)
}

func LoadConfig() Config {
	return Config{
		Port:        getEnv("PORT", "8080"),
		WhisperURL:  getEnv("WHISPER_URL", "http://whisper.vox.svc.cluster.local:8000"),
		VADEndpoint: getEnv("VAD_ENDPOINT", "http://localhost:8001"),

		DiarizerURL:   getEnv("DIARIZER_URL", ""),
		SummarizerURL: getEnv("SUMMARIZER_URL", ""),

		EventLogBackend: getEnv("EVENT_LOG_BACKEND", "local"),
		EventLogDir:     getEnv("EVENT_LOG_DIR", "/tmp/vox-events"),
		EventLogBucket:  getEnv("EVENT_LOG_BUCKET", ""),
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
