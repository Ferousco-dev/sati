package apierror

import (
	"encoding/json"
	"net/http"
)

type Error struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func New(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

var (
	ErrNotFound     = New(http.StatusNotFound, "NOT_FOUND", "Resource not found")
	ErrMethod       = New(http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
	ErrInternal     = New(http.StatusInternalServerError, "INTERNAL_ERROR", "Something went wrong")
	ErrUnavailable  = New(http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Service unavailable")
	ErrRateLimited  = New(http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests")
	ErrUnauthorized = New(http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
	ErrForbidden    = New(http.StatusForbidden, "FORBIDDEN", "You do not have access to this resource")
)

func JSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func Write(w http.ResponseWriter, err *Error) {
	JSON(w, err.Status, map[string]*Error{"error": err})
}
