package gateway

import "os"

// Config holds all gateway configuration.
// Every field comes from an environment variable with a sensible default.
// This is the 12-factor app approach: no config files, no flag,
// just environment variables that Kubernetes sets via the Helm values.

type Config struct {
	Port        string // PORT — what port the gateway listens on
	WhisperURL  string // WHISPER_URL — where the whisper service lives
	VADEndpoint string // VAD_ENDPOINT — where the VAD sidecar lives
}

func LoadConfig() Config {
	return Config{
		Port:        getEnv("PORT", "8080"),
		WhisperURL:  getEnv("WHISPER_URL", "http://whisper.vox.svc.cluster.local:8000"),
		VADEndpoint: getEnv("VAD_ENDPOINT", "http://localhost:8001"),
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
