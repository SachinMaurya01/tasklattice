package middleware

import (
	"strconv"
	"sync"
	"time"

	apperrors "tasklattice/internal/errors"

	"github.com/gin-gonic/gin"
)

// RateLimiter is an in-memory sliding-window limiter guarding the auth
// endpoints (5 requests/minute/IP). The Redis limiter supersedes it when
// Redis is configured.
type RateLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string][]time.Time
}

// NewRateLimiter allows limit requests per window per key.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{limit: limit, window: window, hits: make(map[string][]time.Time)}
}

// ClientIPKey buckets callers by IP.
func ClientIPKey(c *gin.Context) string { return "ip:" + c.ClientIP() }

// Middleware rejects callers over the limit with 429 + Retry-After.
func (l *RateLimiter) Middleware(keyFunc func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := keyFunc(c)
		now := time.Now()
		cutoff := now.Add(-l.window)

		l.mu.Lock()
		fresh := make([]time.Time, 0, len(l.hits[key])+1)
		for _, t := range l.hits[key] {
			if t.After(cutoff) {
				fresh = append(fresh, t)
			}
		}
		if len(fresh) >= l.limit {
			l.hits[key] = fresh
			l.mu.Unlock()
			retryAfter := int(fresh[0].Add(l.window).Sub(now).Seconds()) + 1
			if retryAfter < 1 {
				retryAfter = 1
			}
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			apperrors.AbortWith(c, apperrors.RateLimited())
			return
		}
		l.hits[key] = append(fresh, now)
		l.mu.Unlock()
		c.Next()
	}
}
