package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"time"

	apperrors "tasklattice/internal/errors"
	"tasklattice/internal/repository"

	"github.com/gin-gonic/gin"
)

// Idempotency-Key header carrying the client key.
const IdempotencyKeyHeader = "Idempotency-Key"

// maxStoredBody caps replayed response bodies at 1 MiB.
const maxStoredBody = 1 << 20

// Idempotency replays completed POSTs and rejects key reuse with a
// different payload. Keys scope to the authenticated user with a TTL.
type Idempotency struct {
	keys repository.IdempotencyRepository
	ttl  time.Duration
}

// NewIdempotency wires the middleware to its store and key TTL.
func NewIdempotency(keys repository.IdempotencyRepository, ttl time.Duration) *Idempotency {
	return &Idempotency{keys: keys, ttl: ttl}
}

func hashBody(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// readCloser restores a consumed request body for downstream binding.
type readCloser struct {
	*bytes.Reader
}

func (r *readCloser) Close() error { return nil }

var _ io.ReadCloser = (*readCloser)(nil)

// captureWriter buffers the response body for idempotent replay.
type captureWriter struct {
	gin.ResponseWriter
	buf       bytes.Buffer
	truncated bool
}

func (w *captureWriter) Write(b []byte) (int, error) {
	if !w.truncated {
		if w.buf.Len()+len(b) > maxStoredBody {
			w.truncated = true
		} else {
			w.buf.Write(b)
		}
	}
	return w.ResponseWriter.Write(b)
}

// Middleware applies only to POSTs carrying an Idempotency-Key. It must run
// after authentication so the key scopes to the user.
func (m *Idempotency) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost {
			c.Next()
			return
		}
		key := strings.TrimSpace(c.Request.Header.Get(IdempotencyKeyHeader))
		if key == "" {
			c.Next()
			return
		}
		if len(key) > 255 {
			apperrors.AbortWith(c, apperrors.BadRequest("Invalid idempotency key"))
			return
		}
		rawID, exists := c.Get("user_id")
		if !exists {
			apperrors.AbortWith(c, apperrors.Unauthorized("Authentication required"))
			return
		}
		userID, ok := rawID.(string)
		if !ok || userID == "" {
			apperrors.AbortWith(c, apperrors.Unauthorized("Authentication required"))
			return
		}

		body, err := c.GetRawData()
		if err != nil {
			apperrors.AbortWith(c, apperrors.BadRequest("Invalid request body"))
			return
		}
		c.Request.Body = &readCloser{bytes.NewReader(body)}
		hash := hashBody(body)

		ctx := c.Request.Context()
		created, err := m.keys.CreatePending(ctx, key, userID, hash, time.Now().Add(m.ttl))
		if err != nil {
			apperrors.AbortWith(c, apperrors.Internal())
			return
		}
		if !created {
			stored, err := m.keys.Get(ctx, key, userID)
			if err != nil {
				// Expired between calls: claim it fresh, else ask to retry.
				if recreated, rerr := m.keys.CreatePending(ctx, key, userID, hash, time.Now().Add(m.ttl)); rerr != nil || !recreated {
					apperrors.AbortWith(c, apperrors.Conflict("Request already in progress"))
					return
				}
			} else if stored.Status == repository.IdempotencyCompleted {
				if stored.RequestHash != hash {
					apperrors.AbortWith(c, apperrors.IdempotencyReused())
					return
				}
				replay := []byte{}
				if stored.ResponseBody != nil {
					replay = []byte(*stored.ResponseBody)
				}
				code := http.StatusOK
				if stored.StatusCode != nil {
					code = *stored.StatusCode
				}
				c.Data(code, "application/json; charset=utf-8", replay)
				c.Abort()
				return
			} else {
				apperrors.AbortWith(c, apperrors.Conflict("Request already in progress"))
				return
			}
		}

		cw := &captureWriter{ResponseWriter: c.Writer}
		c.Writer = cw
		c.Next()

		status := c.Writer.Status()
		if status >= 500 || cw.truncated {
			_ = m.keys.Delete(ctx, key, userID)
			return
		}
		_ = m.keys.Complete(ctx, key, userID, status, cw.buf.String())
	}
}
