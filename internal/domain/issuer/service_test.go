package issuer

import (
	"testing"

	"github.com/zaneway/AutoCertX/internal/domain/resource"
)

func TestServiceCreateProvidesCapabilityMetadata(t *testing.T) {
	service := NewService()
	scope := resource.Scope{
		TenantID:      "11111111-1111-4111-8111-111111111111",
		ProjectID:     "22222222-2222-4222-8222-222222222222",
		EnvironmentID: "33333333-3333-4333-8333-333333333333",
	}

	account, err := service.Create(scope, UpsertInput{
		DisplayName:  "le-staging",
		DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
		Email:        "ops@example.com",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if account.Capabilities.Environment != "staging" {
		t.Fatalf("Capabilities.Environment = %q, want %q", account.Capabilities.Environment, "staging")
	}
	if got := account.Capabilities.WildcardChallengeTypes; len(got) != 1 || got[0] != "dns-01" {
		t.Fatalf("WildcardChallengeTypes = %v, want [dns-01]", got)
	}
}
