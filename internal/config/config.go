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
