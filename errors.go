package gofastapi

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

// Error represents a structured API error
type Error struct {
	Status  int               `json:"-"`
	Code    string            `json:"code,omitempty"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

func (e *Error) Error() string {
	return e.Message
}

// NewError creates a new API error
func NewError(status int, message string) *Error {
	return &Error{
		Status:  status,
		Message: message,
	}
}

// NewErrorWithCode creates a new API error with a code
func NewErrorWithCode(status int, code, message string) *Error {
	return &Error{
		Status:  status,
		Code:    code,
		Message: message,
	}
}

// ValidationError represents validation errors
type ValidationError struct {
	Status  int                 `json:"-"`
	Code    string              `json:"code"`
	Message string              `json:"message"`
	Fields  map[string][]string `json:"validation_errors"`
}

// Error implements the error interface for ValidationError
func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %v", e.Message, e.Fields)
}

// NewValidationError creates a new validation error
func NewValidationError(fields map[string][]string) *ValidationError {
	return &ValidationError{
		Status:  http.StatusBadRequest,
		Code:    "VALIDATION_ERROR",
		Message: "Validation failed",
		Fields:  fields,
	}
}

// ErrorResponse is the standard error response structure
type ErrorResponse struct {
	Code    string              `json:"code,omitempty"`
	Message string              `json:"message"`
	Details map[string]string   `json:"details,omitempty"`
	Fields  map[string][]string `json:"validation_errors,omitempty"`
}

// ErrorHandler is the function signature for custom error handlers
type ErrorHandler func(w http.ResponseWriter, r *http.Request, err error)

// defaultErrorHandler is the default error handler
func defaultErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var response ErrorResponse
	status := http.StatusInternalServerError

	switch e := err.(type) {
	case *Error:
		status = e.Status
		response = ErrorResponse{
			Code:    e.Code,
			Message: e.Message,
			Details: e.Details,
		}
	case *ValidationError:
		status = e.Status
		response = ErrorResponse{
			Code:    e.Code,
			Message: e.Message,
			Fields:  e.Fields,
		}
	default:
		slog.Error("internal server error", "error", err)
		response = ErrorResponse{
			Code:    "INTERNAL_ERROR",
			Message: "An internal error occurred",
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(response)
}

// WithDetails adds details to an error
func (e *Error) WithDetails(details map[string]string) *Error {
	e.Details = details
	return e
}

// WithDetail adds a single detail to an error
func (e *Error) WithDetail(key, value string) *Error {
	if e.Details == nil {
		e.Details = make(map[string]string)
	}
	e.Details[key] = value
	return e
}
