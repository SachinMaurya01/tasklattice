package middleware

import (
	"crypto/rand"
	"encoding/hex"

	apperrors "tasklattice/internal/errors"

	"github.com/gin-gonic/gin"
)

// RequestID attaches a request id to the gin context, the X-Request-ID
// response header, and the standard error envelope. A client-supplied
// X-Request-ID is honored so callers can correlate across retries.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = newRequestID()
		}
		c.Set(apperrors.RequestIDKey, requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "req_unknown"
	}
	return "req_" + hex.EncodeToString(b[:])
}
