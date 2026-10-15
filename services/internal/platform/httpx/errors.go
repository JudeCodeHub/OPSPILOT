package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/git4jude/opspilot/services/internal/platform/logging"
)

// Error is an API error with an HTTP status and a stable machine-readable code.
// Handlers return it (or wrap it) and WriteErr renders it.
type Error struct {
	Status  int    // HTTP status code, e.g. 409
	Code    string // stable code for clients, e.g. "SEAT_UNAVAILABLE"
	Message string // human-readable message, safe to show to end users
}

func (e *Error) Error() string {
	return fmt.Sprintf("%d %s: %s", e.Status, e.Code, e.Message)
}

// NewError builds an *Error.
func NewError(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// Codes shared by every service.
const (
	CodeInternal   = "INTERNAL_ERROR"
	CodeNotFound   = "NOT_FOUND"
	CodeBadRequest = "INVALID_REQUEST"
)

// ErrorBody is the JSON shape of every error response:
//
//	{ "error": { "code": "SEAT_UNAVAILABLE", "message": "…", "request_id": "…" } }
type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail is the inner object of ErrorBody.
type ErrorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// WriteJSON writes v as JSON with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError writes an error response in the standard shape.
func WriteError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	WriteJSON(w, status, ErrorBody{Error: ErrorDetail{
		Code:      code,
		Message:   message,
		RequestID: logging.RequestID(r.Context()),
	}})
}

// WriteErr renders err. An *Error (possibly wrapped) is written as-is.
// Anything else is an unexpected failure: it is logged at error level and the
// client gets a generic 500 without internal details.
func WriteErr(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		WriteError(w, r, apiErr.Status, apiErr.Code, apiErr.Message)
		return
	}
	logging.FromContext(r.Context()).Error("request failed", "error", err.Error())
	WriteError(w, r, http.StatusInternalServerError, CodeInternal, "Something went wrong on our side. Please try again.")
}

// MaxBodyBytes caps request bodies decoded by DecodeJSON.
const MaxBodyBytes = 1 << 20

// DecodeJSON decodes the request body into dst. Malformed input yields a 400 *Error.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return NewError(http.StatusBadRequest, CodeBadRequest, "Request body is required.")
		}
		return NewError(http.StatusBadRequest, CodeBadRequest, "Request body is not valid JSON.")
	}
	return nil
}
