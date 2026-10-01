package middleware

import (
	"context"
	"strconv"
	"time"

	apperrors "tasklattice/internal/errors"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// fixedWindow counts hits per key per window atomically.
const fixedWindowLua = `
local current = redis.call('INCR', KEYS[1])
if current == 1 then
	redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return current
`

// RedisRateLimiter is a distributed fixed-window limiter. When client is
// nil it degrades to the in-memory limiter (single-instance correctness),
// so local development works without Redis.
type RedisRateLimiter struct {
	client   *redis.Client
	limit    int
	window   time.Duration
	prefix   string
	fallback *RateLimiter
}

// NewRedisRateLimiter allows limit requests per window per key. A nil
// client selects the in-memory fallback.
func NewRedisRateLimiter(client *redis.Client, prefix string, limit int, window time.Duration) *RedisRateLimiter {
	return &RedisRateLimiter{
		client:   client,
		limit:    limit,
		window:   window,
		prefix:   prefix,
		fallback: NewRateLimiter(limit, window),
	}
}

// UserIDKey buckets callers by authenticated user.
func UserIDKey(c *gin.Context) string {
	if id, ok := c.Get("user_id"); ok {
		if s, ok := id.(string); ok && s != "" {
			return "user:" + s
		}
	}
	return ClientIPKey(c)
}

// Middleware rejects callers over the limit with 429 + Retry-After.
// Redis failures fail open (logged by the caller at startup choice);
// correctness without Redis comes from the in-memory fallback only when
// Redis was never configured.
func (l *RedisRateLimiter) Middleware(keyFunc func(*gin.Context) string) gin.HandlerFunc {
	if l.client == nil {
		return l.fallback.Middleware(keyFunc)
	}
	windowSec := int(l.window.Seconds())
	if windowSec < 1 {
		windowSec = 1
	}
	return func(c *gin.Context) {
		bucket := time.Now().Unix() / int64(windowSec)
		key := l.prefix + ":" + keyFunc(c) + ":" + strconv.FormatInt(bucket, 10)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 500*time.Millisecond)
		defer cancel()
		n, err := l.client.Eval(ctx, fixedWindowLua, []string{key}, windowSec).Int64()
		if err != nil {
			// Fail open: Redis is auxiliary, never the source of truth.
			c.Next()
			return
		}
		if n > int64(l.limit) {
			c.Header("Retry-After", strconv.Itoa(windowSec))
			apperrors.AbortWith(c, apperrors.RateLimited())
			return
		}
		c.Next()
	}
}

// PingRedis reports whether the Redis client answers.
func PingRedis(ctx context.Context, client *redis.Client) error {
	if client == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return client.Ping(ctx).Err()
}
