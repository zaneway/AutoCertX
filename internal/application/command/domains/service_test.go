package domains

import (
	"context"
	"testing"

	"github.com/zaneway/AutoCertX/internal/domain/dnscredentials"
	domaindomain "github.com/zaneway/AutoCertX/internal/domain/domains"
	"github.com/zaneway/AutoCertX/internal/domain/resource"
	"github.com/zaneway/AutoCertX/internal/platform/apperr"
)

func TestServiceAuditsGovernanceWrites(t *testing.T) {
	scope := resource.Scope{
		TenantID:      "11111111-1111-4111-8111-111111111111",
		ProjectID:     "22222222-2222-4222-8222-222222222222",
		EnvironmentID: "33333333-3333-4333-8333-333333333333",
	}
	audit := &MemoryAuditRecorder{}
	service := NewService(domaindomain.NewService(), dnscredentials.NewService(), audit)

	credential, err := service.CreateDNSCredential(context.Background(), scope, "55555555-5555-4555-8555-555555555555", DNSCredentialUpsertInput{
		DisplayName:  "alidns-prod",
		ProviderType: dnscredentials.ProviderAliDNS,
		AccessKeyID:  "ak",
		Secret:       "secret",
		ScopeMode:    dnscredentials.ScopeEnvironment,
	})
	if err != nil {
		t.Fatalf("CreateDNSCredential() error = %v", err)
	}

	asset, err := service.CreateDomain(context.Background(), scope, "55555555-5555-4555-8555-555555555555", DomainUpsertInput{
		Name:            "api.example.com",
		ChallengeType:   domaindomain.ChallengeDNS01,
		DNSCredentialID: credential.ID,
	})
	if err != nil {
		t.Fatalf("CreateDomain() error = %v", err)
	}

	if _, err := service.BindDNSCredential(context.Background(), scope, "55555555-5555-4555-8555-555555555555", asset.ID, credential.ID); err != nil {
		t.Fatalf("BindDNSCredential() error = %v", err)
	}
	if _, err := service.RotateDNSCredential(context.Background(), scope, "55555555-5555-4555-8555-555555555555", credential.ID); err != nil {
		t.Fatalf("RotateDNSCredential() error = %v", err)
	}

	events := audit.Events()
	if len(events) != 4 {
		t.Fatalf("audit events = %d, want %d", len(events), 4)
	}
	if events[0].Action != "dns_credential.create" {
		t.Fatalf("first audit action = %q", events[0].Action)
	}
	if events[1].Action != "domain.create" {
		t.Fatalf("second audit action = %q", events[1].Action)
	}
	if events[2].Action != "domain.bind_dns_credential" {
		t.Fatalf("third audit action = %q", events[2].Action)
	}
	if events[3].Action != "dns_credential.rotate" {
		t.Fatalf("fourth audit action = %q", events[3].Action)
	}
}

func TestServiceRejectsWildcardHTTP01(t *testing.T) {
	scope := resource.Scope{
		TenantID:      "11111111-1111-4111-8111-111111111111",
		ProjectID:     "22222222-2222-4222-8222-222222222222",
		EnvironmentID: "33333333-3333-4333-8333-333333333333",
	}
	service := NewService(domaindomain.NewService(), dnscredentials.NewService(), NopAuditRecorder{})

	_, err := service.CreateDomain(context.Background(), scope, "55555555-5555-4555-8555-555555555555", DomainUpsertInput{
		Name:          "*.example.com",
		ChallengeType: domaindomain.ChallengeHTTP01,
	})
	if err == nil {
		t.Fatal("CreateDomain() error = nil, want validation error")
	}
	appErr, ok := apperr.As(err)
	if !ok {
		t.Fatalf("CreateDomain() error = %T, want *apperr.Error", err)
	}
	if appErr.Code != "REQUEST_VALIDATION_FAILED" {
		t.Fatalf("appErr.Code = %q, want %q", appErr.Code, "REQUEST_VALIDATION_FAILED")
	}
}
