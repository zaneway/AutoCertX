package agentnode

import (
	"errors"
	"testing"
	"time"

	"github.com/zaneway/AutoCertX/internal/domain/resource"
)

func TestRegisterHeartbeatAndMatchCapableNode(t *testing.T) {
	service := NewService()
	service.now = fixedClock(time.Date(2026, 4, 23, 10, 0, 0, 0, time.UTC))
	scope := testScope()

	node, err := service.Register(scope, RegistrationInput{
		Name:            "edge-agent-a",
		Hostname:        "edge-a.local",
		IPAddress:       "10.0.0.12",
		Version:         "0.1.0",
		ProtocolVersion: 1,
		OS:              "linux",
		Arch:            "amd64",
		Labels:          map[string]string{"zone": "dmz", "runtime": "nginx"},
		Capabilities:    []string{CapabilityDeployNGINX, CapabilityChallengeHTTP01, CapabilityKeygenRSA},
	})
	if err != nil {
		t.Fatalf("register node: %v", err)
	}
	if node.Status != StatusRegistering {
		t.Fatalf("status = %q, want %q", node.Status, StatusRegistering)
	}

	service.now = fixedClock(time.Date(2026, 4, 23, 10, 1, 0, 0, time.UTC))
	node, err = service.Heartbeat(scope, node.ID, HeartbeatInput{
		Version:         "0.1.1",
		ProtocolVersion: 1,
		Status:          StatusOnline,
		Capabilities:    []string{CapabilityDeployNGINX, CapabilityChallengeHTTP01, CapabilityKeygenRSA},
	})
	if err != nil {
		t.Fatalf("heartbeat node: %v", err)
	}
	if node.Status != StatusOnline {
		t.Fatalf("status = %q, want %q", node.Status, StatusOnline)
	}
	if node.LastSeenAt == nil {
		t.Fatal("last_seen_at should be updated")
	}

	matches, err := service.MatchCapable(scope, []string{CapabilityDeployNGINX}, map[string]string{"zone": "dmz"})
	if err != nil {
		t.Fatalf("match capable: %v", err)
	}
	if len(matches) != 1 || matches[0].ID != node.ID {
		t.Fatalf("matches = %+v, want node %s", matches, node.ID)
	}
}

func TestRegisterRejectsDuplicateNamesInScope(t *testing.T) {
	service := NewService()
	scope := testScope()
	input := validRegistration()

	if _, err := service.Register(scope, input); err != nil {
		t.Fatalf("register first node: %v", err)
	}
	if _, err := service.Register(scope, input); !errors.Is(err, resource.ErrConflict) {
		t.Fatalf("register duplicate error = %v, want conflict", err)
	}
}

func TestNodeScopeIsolation(t *testing.T) {
	service := NewService()
	node, err := service.Register(testScope(), validRegistration())
	if err != nil {
		t.Fatalf("register node: %v", err)
	}

	otherScope := resource.Scope{
		TenantID:      "tenant-2",
		ProjectID:     "project-1",
		EnvironmentID: "env-1",
	}
	if _, err := service.Get(otherScope, node.ID); !errors.Is(err, resource.ErrScopeMismatch) {
		t.Fatalf("get cross-scope error = %v, want scope mismatch", err)
	}
}

func TestDisabledNodeCannotHeartbeatOrMatch(t *testing.T) {
	service := NewService()
	scope := testScope()
	node, err := service.Register(scope, validRegistration())
	if err != nil {
		t.Fatalf("register node: %v", err)
	}
	if _, err := service.Disable(scope, node.ID); err != nil {
		t.Fatalf("disable node: %v", err)
	}

	if _, err := service.Heartbeat(scope, node.ID, validHeartbeat()); !errors.Is(err, resource.ErrUnavailable) {
		t.Fatalf("heartbeat disabled error = %v, want unavailable", err)
	}

	matches, err := service.MatchCapable(scope, []string{CapabilityDeployNGINX}, nil)
	if err != nil {
		t.Fatalf("match capable: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("disabled node should not match, got %+v", matches)
	}
}

func TestRegisterRejectsUnknownCapabilities(t *testing.T) {
	service := NewService()
	input := validRegistration()
	input.Capabilities = []string{"deploy:apache"}

	if _, err := service.Register(testScope(), input); !errors.Is(err, resource.ErrValidation) {
		t.Fatalf("register invalid capability error = %v, want validation", err)
	}
}

func TestRegistrationTokenResolveAndLookup(t *testing.T) {
	service := NewService()
	scope := testScope()

	token, err := service.CreateRegistrationToken(scope)
	if err != nil {
		t.Fatalf("create registration token: %v", err)
	}

	resolved, err := service.ResolveRegistrationToken(token.Token)
	if err != nil {
		t.Fatalf("resolve registration token: %v", err)
	}
	if !resolved.Scope.Equals(scope) {
		t.Fatalf("resolved scope = %+v, want %+v", resolved.Scope, scope)
	}

	node, err := service.Register(scope, validRegistration())
	if err != nil {
		t.Fatalf("register node: %v", err)
	}

	lookup, err := service.Lookup(node.ID)
	if err != nil {
		t.Fatalf("lookup node: %v", err)
	}
	if lookup.ID != node.ID {
		t.Fatalf("lookup id = %q, want %q", lookup.ID, node.ID)
	}

	found, err := service.FindByName(scope, node.Name)
	if err != nil {
		t.Fatalf("find node by name: %v", err)
	}
	if found.ID != node.ID {
		t.Fatalf("find by name id = %q, want %q", found.ID, node.ID)
	}
}

func TestRegisterAllowsSparseTransportPayload(t *testing.T) {
	service := NewService()

	node, err := service.Register(testScope(), RegistrationInput{
		Name:            "edge-agent-b",
		Version:         "0.2.0",
		ProtocolVersion: 1,
	})
	if err != nil {
		t.Fatalf("register sparse payload: %v", err)
	}
	if node.Name != "edge-agent-b" {
		t.Fatalf("node name = %q, want edge-agent-b", node.Name)
	}
}

func validRegistration() RegistrationInput {
	return RegistrationInput{
		Name:            "edge-agent-a",
		Hostname:        "edge-a.local",
		IPAddress:       "10.0.0.12",
		Version:         "0.1.0",
		ProtocolVersion: 1,
		OS:              "linux",
		Arch:            "amd64",
		Labels:          map[string]string{"zone": "dmz"},
		Capabilities:    []string{CapabilityDeployNGINX, CapabilityKeygenRSA},
	}
}

func validHeartbeat() HeartbeatInput {
	return HeartbeatInput{
		Version:         "0.1.1",
		ProtocolVersion: 1,
		Status:          StatusOnline,
		Capabilities:    []string{CapabilityDeployNGINX, CapabilityKeygenRSA},
	}
}

func testScope() resource.Scope {
	return resource.Scope{
		TenantID:      "tenant-1",
		ProjectID:     "project-1",
		EnvironmentID: "env-1",
	}
}

func fixedClock(now time.Time) func() time.Time {
	return func() time.Time {
		return now
	}
}
