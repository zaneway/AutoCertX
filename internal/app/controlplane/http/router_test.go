package httpserver

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zaneway/AutoCertX/internal/platform/buildinfo"
	"github.com/zaneway/AutoCertX/internal/platform/config"
)

func TestNewRouterHealthz(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewRouter(Deps{
		Config: config.Config{
			ServiceName: "controlplane",
			Environment: "test",
		},
		BuildInfo: buildinfo.Info{
			Service:   "controlplane",
			Version:   "dev",
			Commit:    "abc123",
			BuildTime: "2026-04-20T00:00:00Z",
		},
		Logger: logger,
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("X-Request-Id"); got == "" {
		t.Fatal("X-Request-Id header should not be empty")
	}

	var payload healthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Service != "controlplane" {
		t.Fatalf("Service = %q, want %q", payload.Service, "controlplane")
	}
	if payload.Status != "ok" {
		t.Fatalf("Status = %q, want %q", payload.Status, "ok")
	}
}
