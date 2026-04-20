package dnscredentials

import (
	"testing"

	"github.com/zaneway/AutoCertX/internal/domain/resource"
)

func TestServiceCreateAndRotate(t *testing.T) {
	service := NewService()
	scope := resource.Scope{
		TenantID:      "11111111-1111-4111-8111-111111111111",
		ProjectID:     "22222222-2222-4222-8222-222222222222",
		EnvironmentID: "33333333-3333-4333-8333-333333333333",
	}

	credential, err := service.Create(scope, UpsertInput{
		DisplayName:  "alidns-prod",
		ProviderType: ProviderAliDNS,
		AccessKeyID:  "ak",
		Secret:       "secret",
		ScopeMode:    ScopeEnvironment,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if credential.SecretDigest == "" {
		t.Fatal("Create() should store secret digest")
	}
	if credential.SecretDigest == "secret" {
		t.Fatal("Create() should not keep plaintext secret")
	}

	rotated, err := service.Rotate(scope, credential.ID)
	if err != nil {
		t.Fatalf("Rotate() error = %v", err)
	}
	if rotated.LastRotatedAt == nil {
		t.Fatal("Rotate() should set LastRotatedAt")
	}
	if rotated.Status != StatusActive {
		t.Fatalf("Rotate() status = %q, want %q", rotated.Status, StatusActive)
	}
}
