package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const globalPrefix = "AUTOCERTX"

// HTTPConfig controls listener address and timeout budgets.
type HTTPConfig struct {
	Addr              string
	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

// StorageConfig carries durable dependency connection strings.
type StorageConfig struct {
	PostgresURL string
	RedisURL    string
}

// AuthConfig controls token issuance secrets and TTLs.
type AuthConfig struct {
	SigningKey      string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
}

// Config is the aggregated runtime configuration snapshot.
type Config struct {
	ServiceName string
	Environment string
	LogLevel    string
	HTTP        HTTPConfig
	Storage     StorageConfig
	Auth        AuthConfig
}

// LoadOptions define how environment variables are resolved for one process.
type LoadOptions struct {
	ServiceName     string
	EnvPrefix       string
	DefaultHTTPAddr string
}

func Load(opts LoadOptions) (Config, error) {
	prefix := normalizePrefix(opts.EnvPrefix)
	// Process-local prefixes override the shared AUTOCERTX_* defaults so one
	// repository can host multiple binaries without config collisions.
	cfg := Config{
		ServiceName: opts.ServiceName,
		Environment: lookupString(prefix, "ENV", "local"),
		LogLevel:    lookupString(prefix, "LOG_LEVEL", "info"),
		HTTP: HTTPConfig{
			Addr:              lookupString(prefix, "HTTP_ADDR", opts.DefaultHTTPAddr),
			ReadTimeout:       lookupDuration(prefix, "HTTP_READ_TIMEOUT", 5*time.Second),
			ReadHeaderTimeout: lookupDuration(prefix, "HTTP_READ_HEADER_TIMEOUT", 2*time.Second),
			WriteTimeout:      lookupDuration(prefix, "HTTP_WRITE_TIMEOUT", 10*time.Second),
			IdleTimeout:       lookupDuration(prefix, "HTTP_IDLE_TIMEOUT", 60*time.Second),
			ShutdownTimeout:   lookupDuration(prefix, "HTTP_SHUTDOWN_TIMEOUT", 10*time.Second),
		},
		Storage: StorageConfig{
			PostgresURL: lookupString(prefix, "POSTGRES_URL", "postgres://autocertx:autocertx@localhost:5432/autocertx?sslmode=disable"),
			RedisURL:    lookupString(prefix, "REDIS_URL", "redis://localhost:6379/0"),
		},
		Auth: AuthConfig{
			SigningKey:      lookupString(prefix, "AUTH_SIGNING_KEY", "autocertx-dev-signing-key"),
			AccessTokenTTL:  lookupDuration(prefix, "AUTH_ACCESS_TOKEN_TTL", 15*time.Minute),
			RefreshTokenTTL: lookupDuration(prefix, "AUTH_REFRESH_TOKEN_TTL", 24*time.Hour),
		},
	}

	// Validate the computed snapshot after all defaults and env overrides have
	// been applied so startup fails fast with actionable configuration errors.
	if cfg.ServiceName == "" {
		return Config{}, fmt.Errorf("service name required")
	}
	if cfg.HTTP.Addr == "" {
		return Config{}, fmt.Errorf("http addr required")
	}
	if cfg.HTTP.ShutdownTimeout <= 0 {
		return Config{}, fmt.Errorf("http shutdown timeout must be positive")
	}
	if strings.TrimSpace(cfg.Auth.SigningKey) == "" {
		return Config{}, fmt.Errorf("auth signing key required")
	}
	if cfg.Auth.AccessTokenTTL <= 0 {
		return Config{}, fmt.Errorf("auth access token ttl must be positive")
	}
	if cfg.Auth.RefreshTokenTTL <= 0 {
		return Config{}, fmt.Errorf("auth refresh token ttl must be positive")
	}

	return cfg, nil
}

func lookupDuration(prefix string, key string, fallback time.Duration) time.Duration {
	value, ok := lookupEnv(prefix, key)
	if !ok {
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func lookupString(prefix string, key string, fallback string) string {
	value, ok := lookupEnv(prefix, key)
	if !ok {
		return fallback
	}

	return value
}

func lookupEnv(prefix string, key string) (string, bool) {
	candidates := make([]string, 0, 2)
	if prefix != "" {
		candidates = append(candidates, prefix+"_"+key)
	}
	candidates = append(candidates, globalPrefix+"_"+key)

	for _, candidate := range candidates {
		value, ok := os.LookupEnv(candidate)
		if !ok {
			continue
		}

		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}

		return trimmed, true
	}

	return "", false
}

func normalizePrefix(prefix string) string {
	normalized := strings.TrimSpace(prefix)
	normalized = strings.ReplaceAll(normalized, "-", "_")
	return strings.ToUpper(normalized)
}
