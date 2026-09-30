// Package errors centralizes API error codes and the JSON error envelope
// Services return *AppError; handlers render it via Respond.
// Internal details are never leaked: unknown errors become INTERNAL_ERROR.
package errors

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequestIDKey is the gin context key holding the request id.
const RequestIDKey = "request_id"

// Stable API error codes.
const (
	CodeValidation   = "VALIDATION_ERROR"
	CodeUnauthorized = "UNAUTHORIZED"
	CodeForbidden    = "FORBIDDEN"
	CodeNotFound     = "NOT_FOUND"
	CodeConflict     = "CONFLICT"
	CodeRateLimited  = "RATE_LIMITED"
	CodeInternal     = "INTERNAL_ERROR"

	CodeEmailTaken   = "EMAIL_ALREADY_REGISTERED"
	CodeInvalidCreds = "INVALID_CREDENTIALS"
	CodeTokenExpired = "TOKEN_EXPIRED"
	CodeTokenInvalid = "INVALID_TOKEN"
	CodeTokenReused  = "REFRESH_TOKEN_REUSED"

	CodeVersionConflict = "RESOURCE_VERSION_CONFLICT"
)

// AppError is a classified application error.
type AppError struct {
	Code    string
	Message string
	Status  int
}

func (e *AppError) Error() string { return e.Code + ": " + e.Message }

// New builds an AppError.
func New(code, message string, status int) *AppError {
	return &AppError{Code: code, Message: message, Status: status}
}

// BadRequest is a 400 validation failure.
func BadRequest(message string) *AppError {
	return New(CodeValidation, message, http.StatusBadRequest)
}

// Unauthorized is a 401 authentication failure.
func Unauthorized(message string) *AppError {
	return New(CodeUnauthorized, message, http.StatusUnauthorized)
}

// Forbidden is a 403 authorization failure.
func Forbidden(message string) *AppError {
	return New(CodeForbidden, message, http.StatusForbidden)
}

// NotFound is a 404 missing resource.
func NotFound(message string) *AppError {
	return New(CodeNotFound, message, http.StatusNotFound)
}

// Conflict is a 409 state conflict.
func Conflict(message string) *AppError {
	return New(CodeConflict, message, http.StatusConflict)
}

// RateLimited is a 429 rate-limit rejection.
func RateLimited() *AppError {
	return New(CodeRateLimited, "Rate limit exceeded. Try again later.", http.StatusTooManyRequests)
}

// Internal is a 500 with no leaked details.
func Internal() *AppError {
	return New(CodeInternal, "Internal server error", http.StatusInternalServerError)
}

// EmailTaken is a 409 duplicate registration.
func EmailTaken() *AppError {
	return New(CodeEmailTaken, "Email already registered", http.StatusConflict)
}

// InvalidCredentials is a 401 that never distinguishes "unknown user" from
// "wrong password" (prevents user enumeration).
func InvalidCredentials() *AppError {
	return New(CodeInvalidCreds, "Invalid credentials", http.StatusUnauthorized)
}

// TokenExpired is a 401 for expired tokens.
func TokenExpired() *AppError {
	return New(CodeTokenExpired, "Token has expired", http.StatusUnauthorized)
}

// TokenInvalid is a 401 for malformed or unknown tokens.
func TokenInvalid() *AppError {
	return New(CodeTokenInvalid, "Invalid token", http.StatusUnauthorized)
}

// TokenReused is a 401 for refresh-token reuse (possible theft).
func TokenReused() *AppError {
	return New(CodeTokenReused, "Refresh token reuse detected", http.StatusUnauthorized)
}

// VersionConflict is a 409 for stale optimistic-concurrency versions.
func VersionConflict() *AppError {
	return New(CodeVersionConflict, "The task was modified by another request", http.StatusConflict)
}

// Respond renders err using the standard envelope:
// {"error": {"code": ..., "message": ..., "request_id": ...}}.
func Respond(c *gin.Context, err error) {
	if appErr, ok := err.(*AppError); ok && appErr != nil {
		write(c, appErr)
		return
	}
	write(c, Internal())
}

// AbortWith renders appErr and aborts the chain (for middleware).
func AbortWith(c *gin.Context, appErr *AppError) {
	write(c, appErr)
	c.Abort()
}

func write(c *gin.Context, appErr *AppError) {
	requestID, _ := c.Get(RequestIDKey)
	c.JSON(appErr.Status, gin.H{"error": gin.H{
		"code":       appErr.Code,
		"message":    appErr.Message,
		"request_id": requestID,
	}})
}
