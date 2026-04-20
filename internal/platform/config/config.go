package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const globalPrefix = "AUTOCERTX"

type HTTPConfig struct {
	Addr              string
	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

type StorageConfig struct {
	PostgresURL string
	RedisURL    string
}

type Config struct {
	ServiceName string
	Environment string
	LogLevel    string
	HTTP        HTTPConfig
	Storage     StorageConfig
}

type LoadOptions struct {
	ServiceName     string
	EnvPrefix       string
	DefaultHTTPAddr string
}

func Load(opts LoadOptions) (Config, error) {
	prefix := normalizePrefix(opts.EnvPrefix)
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
	}

	if cfg.ServiceName == "" {
		return Config{}, fmt.Errorf("service name required")
	}
	if cfg.HTTP.Addr == "" {
		return Config{}, fmt.Errorf("http addr required")
	}
	if cfg.HTTP.ShutdownTimeout <= 0 {
		return Config{}, fmt.Errorf("http shutdown timeout must be positive")
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
