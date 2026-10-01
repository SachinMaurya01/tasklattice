package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds environment-based application configuration.
// Load fails fast on missing or invalid required values.
type Config struct {
	Env                 string
	Port                string
	DatabaseURL         string
	JWTSecret           string
	AccessTTL           time.Duration
	RefreshTTL          time.Duration
	ShutdownTimeout     time.Duration
	AuthRateLimitPerMin int
	// RedisURL empty means Redis features degrade to
	// in-memory behavior; SQS URLs are required only by the worker.
	RedisURL              string
	AuthedRateLimitPerMin int
	IdempotencyTTL        time.Duration
	AWSRegion             string
	AWSEndpointURL        string
	SQSQueueURL           string
	SQSDLQURL             string
	WorkerOutboxBatch     int
	WorkerPollInterval    time.Duration
	SQSVisibilityTimeout  int32
	SQSMaxReceiveCount    int
}

// Load reads configuration from the environment and applies defaults.
func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env load failed")
	}

	cfg := &Config{
		Env:  getEnv("APP_ENV", "development"),
		Port: getEnv("PORT", getEnv("APP_PORT", "5000")),
	}

	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = os.Getenv("JWTSecret") // legacy alias, see .env.example
		if secret != "" {
			log.Println("Warning: JWTSecret is deprecated, use JWT_SECRET")
		}
	}
	if secret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}
	cfg.JWTSecret = secret

	var err error
	if cfg.AccessTTL, err = parseDurationEnv("JWT_ACCESS_TTL", 15*time.Minute); err != nil {
		return nil, err
	}
	if cfg.RefreshTTL, err = parseDurationEnv("JWT_REFRESH_TTL", 720*time.Hour); err != nil {
		return nil, err
	}
	if cfg.ShutdownTimeout, err = parseDurationEnv("SHUTDOWN_TIMEOUT", 10*time.Second); err != nil {
		return nil, err
	}
	if cfg.AuthRateLimitPerMin, err = parseIntEnv("RATE_LIMIT_AUTH_PER_MIN", 5); err != nil {
		return nil, err
	}
	if cfg.AuthRateLimitPerMin <= 0 {
		return nil, fmt.Errorf("RATE_LIMIT_AUTH_PER_MIN must be positive")
	}

	cfg.RedisURL = getEnv("REDIS_URL", "")
	if cfg.AuthedRateLimitPerMin, err = parseIntEnv("RATE_LIMIT_API_PER_MIN", 100); err != nil {
		return nil, err
	}
	if cfg.AuthedRateLimitPerMin <= 0 {
		return nil, fmt.Errorf("RATE_LIMIT_API_PER_MIN must be positive")
	}
	if cfg.IdempotencyTTL, err = parseDurationEnv("IDEMPOTENCY_TTL", 24*time.Hour); err != nil {
		return nil, err
	}

	cfg.AWSRegion = getEnv("AWS_REGION", "us-east-1")
	cfg.AWSEndpointURL = getEnv("AWS_ENDPOINT_URL", "")
	cfg.SQSQueueURL = os.Getenv("SQS_QUEUE_URL")
	cfg.SQSDLQURL = os.Getenv("SQS_DLQ_URL")
	if cfg.WorkerOutboxBatch, err = parseIntEnv("WORKER_OUTBOX_BATCH", 50); err != nil {
		return nil, err
	}
	if cfg.WorkerOutboxBatch <= 0 {
		return nil, fmt.Errorf("WORKER_OUTBOX_BATCH must be positive")
	}
	if cfg.WorkerPollInterval, err = parseDurationEnv("WORKER_POLL_INTERVAL", 2*time.Second); err != nil {
		return nil, err
	}
	if cfg.SQSVisibilityTimeout, err = parseInt32Env("SQS_VISIBILITY_TIMEOUT", 30); err != nil {
		return nil, err
	}
	if cfg.SQSMaxReceiveCount, err = parseIntEnv("SQS_MAX_RECEIVE_COUNT", 5); err != nil {
		return nil, err
	}
	if cfg.SQSMaxReceiveCount <= 0 {
		return nil, fmt.Errorf("SQS_MAX_RECEIVE_COUNT must be positive")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func parseDurationEnv(key string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return d, nil
}

func parseIntEnv(key string, fallback int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return n, nil
}

func parseInt32Env(key string, fallback int32) (int32, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("invalid %s: must be positive", key)
	}
	return int32(n), nil
}
