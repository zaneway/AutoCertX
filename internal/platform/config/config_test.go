package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("AUTOCERTX_ENV", "")

	cfg, err := Load(LoadOptions{
		ServiceName:     "controlplane",
		EnvPrefix:       "CONTROLPLANE",
		DefaultHTTPAddr: ":8080",
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ServiceName != "controlplane" {
		t.Fatalf("ServiceName = %q, want %q", cfg.ServiceName, "controlplane")
	}
	if cfg.Environment != "local" {
		t.Fatalf("Environment = %q, want %q", cfg.Environment, "local")
	}
	if cfg.HTTP.Addr != ":8080" {
		t.Fatalf("HTTP.Addr = %q, want %q", cfg.HTTP.Addr, ":8080")
	}
	if cfg.Storage.PostgresURL == "" {
		t.Fatal("PostgresURL should not be empty")
	}
	if cfg.Auth.SigningKey == "" {
		t.Fatal("Auth.SigningKey should not be empty")
	}
}

func TestLoadUsesServiceSpecificOverrides(t *testing.T) {
	t.Setenv("AUTOCERTX_LOG_LEVEL", "warn")
	t.Setenv("AUTOCERTX_REDIS_URL", "redis://localhost:6379/9")
	t.Setenv("AGENT_HTTP_ADDR", ":19081")
	t.Setenv("AGENT_HTTP_SHUTDOWN_TIMEOUT", "15s")
	t.Setenv("AGENT_AUTH_ACCESS_TOKEN_TTL", "30m")

	cfg, err := Load(LoadOptions{
		ServiceName:     "agent",
		EnvPrefix:       "AGENT",
		DefaultHTTPAddr: ":8081",
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.LogLevel != "warn" {
		t.Fatalf("LogLevel = %q, want %q", cfg.LogLevel, "warn")
	}
	if cfg.HTTP.Addr != ":19081" {
		t.Fatalf("HTTP.Addr = %q, want %q", cfg.HTTP.Addr, ":19081")
	}
	if cfg.HTTP.ShutdownTimeout != 15*time.Second {
		t.Fatalf("ShutdownTimeout = %v, want %v", cfg.HTTP.ShutdownTimeout, 15*time.Second)
	}
	if cfg.Storage.RedisURL != "redis://localhost:6379/9" {
		t.Fatalf("RedisURL = %q, want override", cfg.Storage.RedisURL)
	}
	if cfg.Auth.AccessTokenTTL != 30*time.Minute {
		t.Fatalf("AccessTokenTTL = %v, want %v", cfg.Auth.AccessTokenTTL, 30*time.Minute)
	}
}
