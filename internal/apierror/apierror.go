// Package apierror provides structured error types for consistent JSON error responses
// across all HTTP handlers.
package apierror

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// Code is a machine-readable error code that clients can switch on.
type Code string

const (
	// Client-side errors (4xx)
	InvalidRequest      Code = "INVALID_REQUEST"
	InvalidRequestField Code = "INVALID_FIELD"
	InvalidRequestBody  Code = "INVALID_BODY"
	NotFound            Code = "NOT_FOUND"
	AlreadyExists       Code = "ALREADY_EXISTS"
	Conflict            Code = "CONFLICT"

	// Server-side errors (5xx)
	InternalError   Code = "INTERNAL"
	StoreError      Code = "STORE_ERROR"
	QueueError      Code = "QUEUE_ERROR"
	ProxyError      Code = "PROXY_ERROR"
	ModelNotFound   Code = "MODEL_UNAVAILABLE"
	BackendUnhealthy Code = "BACKEND_UNHEALTHY"

	// Special
	ConfigurationError Code = "MISCONFIGURED"
)

type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"-"`
	// Cause is logged but never exposed to the client (prevents leaking internals).
	Cause error `json:"-"`
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return string(e.Code) + ": " + e.Message + " (" + e.Cause.Error() + ")"
	}
	return string(e.Code) + ": " + e.Message
}

func (e *Error) Unwrap() error { return e.Cause }

// Respond writes a structured error response to the Echo context.
// The JSON shape is always:  {"code":"X","message":"Y"}
// Any existing response body fields (from success responses) should not be mixed with errors.
func Respond(c echo.Context, err *Error) error {
	return c.JSON(err.Status, map[string]any{
		"code":    string(err.Code),
		"message": err.Message,
	})
}

// Factory helpers for common error patterns

func BadRequest(code Code, message string) *Error {
	if code == "" {
		code = InvalidRequest
	}
	return &Error{Code: code, Message: message, Status: http.StatusBadRequest}
}

func NotFoundCode(code Code, message string) *Error {
	if code == "" {
		code = NotFound
	}
	return &Error{Code: code, Message: message, Status: http.StatusNotFound}
}

func ConflictErr(code Code, message string) *Error {
	if code == "" {
		code = Conflict
	}
	return &Error{Code: code, Message: message, Status: http.StatusConflict}
}

func Internal(code Code, message string, cause error) *Error {
	if code == "" {
		code = InternalError
	}
	return &Error{Code: code, Message: message, Status: http.StatusInternalServerError, Cause: cause}
}

func ServiceUnavailable(code Code, message string) *Error {
	if code == "" {
		code = ModelNotFound
	}
	return &Error{Code: code, Message: message, Status: http.StatusServiceUnavailable}
}

func Accepted() *Error {
	// Convenience — handlers should Respond directly for 202;
	// this exists so the type system is consistent if needed inline.
	return nil
}
