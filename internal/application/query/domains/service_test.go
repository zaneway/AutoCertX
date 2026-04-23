package domains

import (
	"context"
	"errors"
	"testing"

	"github.com/zaneway/AutoCertX/internal/domain/dnscredentials"
	domaindomain "github.com/zaneway/AutoCertX/internal/domain/domains"
	"github.com/zaneway/AutoCertX/internal/domain/issuer"
	"github.com/zaneway/AutoCertX/internal/domain/resource"
	"github.com/zaneway/AutoCertX/internal/platform/apperr"
)

func TestGovernanceQueryReadsScopedResources(t *testing.T) {
	scope := resource.Scope{
		TenantID:      "11111111-1111-4111-8111-111111111111",
		ProjectID:     "22222222-2222-4222-8222-222222222222",
		EnvironmentID: "33333333-3333-4333-8333-333333333332",
	}

	domainService := domaindomain.NewService()
	dnsService := dnscredentials.NewService()
	issuerService := issuer.NewService()

	credential, err := dnsService.Create(scope, dnscredentials.UpsertInput{
		DisplayName:  "alidns-prod",
		ProviderType: dnscredentials.ProviderAliDNS,
		AccessKeyID:  "ak",
		Secret:       "sk",
		ScopeMode:    dnscredentials.ScopeEnvironment,
	})
	if err != nil {
		t.Fatalf("dnsService.Create() error = %v", err)
	}
	asset, err := domainService.Create(scope, domaindomain.UpsertInput{
		Name:            "api.example.com",
		ChallengeType:   domaindomain.ChallengeDNS01,
		AutoRenew:       true,
		DNSCredentialID: credential.ID,
		DNSProvider:     credential.ProviderType,
	})
	if err != nil {
		t.Fatalf("domainService.Create() error = %v", err)
	}
	account, err := issuerService.Create(scope, issuer.UpsertInput{
		DisplayName:  "letsencrypt-staging",
		DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
		Email:        "ops@example.com",
	})
	if err != nil {
		t.Fatalf("issuerService.Create() error = %v", err)
	}

	service, err := NewService(domainService, dnsService, issuerService)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	domains, err := service.ListDomains(context.Background(), scope)
	if err != nil {
		t.Fatalf("ListDomains() error = %v", err)
	}
	if len(domains) != 1 || domains[0].ID != asset.ID {
		t.Fatalf("ListDomains() = %+v, want one item %q", domains, asset.ID)
	}

	detail, err := service.GetDomain(context.Background(), scope, asset.ID)
	if err != nil {
		t.Fatalf("GetDomain() error = %v", err)
	}
	if detail.DNSCredentialID != credential.ID {
		t.Fatalf("domain dns_credential_id = %q, want %q", detail.DNSCredentialID, credential.ID)
	}

	credentials, err := service.ListDNSCredentials(context.Background(), scope)
	if err != nil {
		t.Fatalf("ListDNSCredentials() error = %v", err)
	}
	if len(credentials) != 1 || credentials[0].ID != credential.ID {
		t.Fatalf("ListDNSCredentials() = %+v, want one item %q", credentials, credential.ID)
	}

	caAccounts, err := service.ListCAAccounts(context.Background(), scope)
	if err != nil {
		t.Fatalf("ListCAAccounts() error = %v", err)
	}
	if len(caAccounts) != 1 || caAccounts[0].ID != account.ID {
		t.Fatalf("ListCAAccounts() = %+v, want one item %q", caAccounts, account.ID)
	}

	capabilities, err := service.GetCAAccountCapabilities(context.Background(), scope, account.ID)
	if err != nil {
		t.Fatalf("GetCAAccountCapabilities() error = %v", err)
	}
	if capabilities.ProviderName != issuer.ProviderLE {
		t.Fatalf("provider_name = %q, want %q", capabilities.ProviderName, issuer.ProviderLE)
	}
}

func TestGovernanceQueryMapsScopeMismatch(t *testing.T) {
	scopeA := resource.Scope{
		TenantID:      "11111111-1111-4111-8111-111111111111",
		ProjectID:     "22222222-2222-4222-8222-222222222222",
		EnvironmentID: "33333333-3333-4333-8333-333333333332",
	}
	scopeB := resource.Scope{
		TenantID:      "11111111-1111-4111-8111-111111111111",
		ProjectID:     "22222222-2222-4222-8222-222222222221",
		EnvironmentID: "33333333-3333-4333-8333-333333333331",
	}

	domainService := domaindomain.NewService()
	dnsService := dnscredentials.NewService()
	issuerService := issuer.NewService()
	asset, err := domainService.Create(scopeA, domaindomain.UpsertInput{
		Name:          "api.example.com",
		ChallengeType: domaindomain.ChallengeDNS01,
		AutoRenew:     true,
	})
	if err != nil {
		t.Fatalf("domainService.Create() error = %v", err)
	}

	service, err := NewService(domainService, dnsService, issuerService)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.GetDomain(context.Background(), scopeB, asset.ID)
	if err == nil {
		t.Fatal("GetDomain() should reject cross-scope access")
	}
	var appErr *apperr.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("cross-scope error type = %T, want *apperr.Error", err)
	}
	if appErr.Code != "TENANT_SCOPE_MISMATCH" {
		t.Fatalf("cross-scope error code = %q, want %q", appErr.Code, "TENANT_SCOPE_MISMATCH")
	}
}
