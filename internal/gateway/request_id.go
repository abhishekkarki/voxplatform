package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

// contextKey is an unexported type for context keys to avoid collisions.
// This is the standard Go pattern - you never use string keys in context
// because two packages could accidentally use the same string.
type contextKey string

const requestIDKey contextKey = "request_id"

// RequestIDFromContext extracts the request ID from context.
// Handlers and the proxy use this to include the request ID in logs and responses.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return "unknown"
}

// requestIDMiddleware injects a unique request ID into every request.
// If the client sends X-Request-ID, we use theirs (useful for distributed tracing
// where an upstream service already generated one). Otherwise we generate one.
func (s *Server) requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if client sent a request ID
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}

		// Set it in the response header so the client can reference it
		w.Header().Set("X-Request-ID", requestID)

		// Inject into context so handlers can access it
		ctx := context.WithValue(r.Context(), requestIDKey, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// generateRequestID creates a short random hex string.
// 8 bytes = 16 hex characters - short enough to paste in Slack,
// long enough that collisions are essentially impossible.
func generateRequestID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
