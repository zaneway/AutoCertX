package wiring

import "testing"

func TestBuild(t *testing.T) {
	t.Setenv("CONTROLPLANE_HTTP_ADDR", ":18080")

	result, err := Build(Options{
		ServiceName:     "controlplane",
		EnvPrefix:       "CONTROLPLANE",
		DefaultHTTPAddr: ":8080",
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if result.Config.ServiceName != "controlplane" {
		t.Fatalf("ServiceName = %q, want %q", result.Config.ServiceName, "controlplane")
	}
	if result.Logger == nil {
		t.Fatal("Logger should not be nil")
	}
	if result.Server == nil {
		t.Fatal("Server should not be nil")
	}
	if result.Server.Addr != ":18080" {
		t.Fatalf("Server.Addr = %q, want %q", result.Server.Addr, ":18080")
	}
}
