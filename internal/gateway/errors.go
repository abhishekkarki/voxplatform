package gateway

import (
	"encoding/json"
	"net/http"
)

// APIError is the standard error response format.
// Every error from the gateway looks the same - clients can parse it reliably.
// This matches the pattern used by Stripe, GitHub, and most production APIs.
//
// Example response:
//
//	{
//	    "error": {
//	        "code": "file_too_large",
//	        "message": "File exceeds maximum size of 32MB",
//	        "request_id": "a1b2c3d4e5f6g7h8"
//	    }
//	}
type APIError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

// APIErrorResponse wraps the error in an "error" key.
// This is the convention - successful responses have the data at the top level,
// error responses wrap in {"error": {...}}.
type APIErrorResponse struct {
	Error APIError `json:"error"`
}

// Common error codes - define them as constants so they're consistent
// and searchable across the codebase.
const (
	ErrCodeBadRequest     = "bad_request"
	ErrCodeFileRequired   = "file_required"
	ErrCodeFileTooLarge   = "file_too_large"
	ErrCodeBackendError   = "backend_error"
	ErrCodeBackendTimeout = "backend_timeout"
	ErrCodeInternalError  = "internal_error"
	ErrCodeServiceUnavail = "service_unavailable"
)

// writeError sends a structured error response.
// Every error handler in the gateway calls this instead of writing raw JSON.
func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	requestID := RequestIDFromContext(r.Context())

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(APIErrorResponse{
		Error: APIError{
			Code:      code,
			Message:   message,
			RequestID: requestID,
		},
	})
}
