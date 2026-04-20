package httpserver

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	caaccountscmd "github.com/zaneway/AutoCertX/internal/application/command/caaccounts"
	domainscmd "github.com/zaneway/AutoCertX/internal/application/command/domains"
	"github.com/zaneway/AutoCertX/internal/domain/dnscredentials"
	"github.com/zaneway/AutoCertX/internal/domain/domains"
	"github.com/zaneway/AutoCertX/internal/domain/issuer"
	"github.com/zaneway/AutoCertX/internal/platform/buildinfo"
	"github.com/zaneway/AutoCertX/internal/platform/config"
)

func TestGovernanceAPIHappyPath(t *testing.T) {
	handler := newGovernanceRouter(t)

	credentialBody := `{"display_name":"alidns-prod","provider_type":"alidns","access_key_id":"ak","secret":"sec","scope_mode":"environment"}`
	credentialResp := performJSONRequest(t, handler, http.MethodPost, "/api/v1/dns-credentials", credentialBody, nil)
	if credentialResp.Code != http.StatusCreated {
		t.Fatalf("create credential status = %d, want %d", credentialResp.Code, http.StatusCreated)
	}
	var credentialEnvelope objectEnvelope
	decodeBody(t, credentialResp.Body, &credentialEnvelope)
	credentialData := credentialEnvelope.Data.(map[string]any)
	credentialID := credentialData["id"].(string)

	domainBody := `{"name":"api.example.com","challenge_type":"dns-01","auto_renew":true,"dns_credential_id":"` + credentialID + `"}`
	domainResp := performJSONRequest(t, handler, http.MethodPost, "/api/v1/domains", domainBody, nil)
	if domainResp.Code != http.StatusCreated {
		t.Fatalf("create domain status = %d, want %d", domainResp.Code, http.StatusCreated)
	}
	var domainEnvelope objectEnvelope
	decodeBody(t, domainResp.Body, &domainEnvelope)
	domainData := domainEnvelope.Data.(map[string]any)
	domainID := domainData["id"].(string)

	listResp := performJSONRequest(t, handler, http.MethodGet, "/api/v1/domains", "", nil)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list domains status = %d, want %d", listResp.Code, http.StatusOK)
	}
	var domainList listEnvelope
	decodeBody(t, listResp.Body, &domainList)
	items := domainList.Items.([]any)
	if len(items) != 1 {
		t.Fatalf("domain list size = %d, want %d", len(items), 1)
	}

	validationResp := performJSONRequest(t, handler, http.MethodGet, "/api/v1/domains/"+domainID+"/validation-records", "", nil)
	if validationResp.Code != http.StatusOK {
		t.Fatalf("validation record status = %d, want %d", validationResp.Code, http.StatusOK)
	}
	var validationList listEnvelope
	decodeBody(t, validationResp.Body, &validationList)
	if len(validationList.Items.([]any)) != 0 {
		t.Fatal("validation records should be empty for new domain")
	}

	accountBody := `{"display_name":"letsencrypt-staging","directory_url":"https://acme-staging-v02.api.letsencrypt.org/directory","email":"ops@example.com"}`
	accountResp := performJSONRequest(t, handler, http.MethodPost, "/api/v1/ca-accounts", accountBody, nil)
	if accountResp.Code != http.StatusCreated {
		t.Fatalf("create ca account status = %d, want %d", accountResp.Code, http.StatusCreated)
	}
	var accountEnvelope objectEnvelope
	decodeBody(t, accountResp.Body, &accountEnvelope)
	accountID := accountEnvelope.Data.(map[string]any)["id"].(string)

	capabilityResp := performJSONRequest(t, handler, http.MethodGet, "/api/v1/ca-accounts/"+accountID+"/capabilities", "", nil)
	if capabilityResp.Code != http.StatusOK {
		t.Fatalf("capability status = %d, want %d", capabilityResp.Code, http.StatusOK)
	}
	var capabilityEnvelope objectEnvelope
	decodeBody(t, capabilityResp.Body, &capabilityEnvelope)
	capabilities := capabilityEnvelope.Data.(map[string]any)
	if capabilities["provider_name"] != "letsencrypt" {
		t.Fatalf("provider_name = %v, want letsencrypt", capabilities["provider_name"])
	}
}

func TestGovernanceAPIBindScopeMismatch(t *testing.T) {
	handler := newGovernanceRouter(t)

	credentialResp := performJSONRequest(t, handler, http.MethodPost, "/api/v1/dns-credentials", `{"display_name":"alidns-prod","provider_type":"alidns","access_key_id":"ak","secret":"sec","scope_mode":"environment"}`, nil)
	var credentialEnvelope objectEnvelope
	decodeBody(t, credentialResp.Body, &credentialEnvelope)
	credentialID := credentialEnvelope.Data.(map[string]any)["id"].(string)

	domainResp := performJSONRequest(t, handler, http.MethodPost, "/api/v1/domains", `{"name":"api.example.com","challenge_type":"dns-01"}`, nil)
	var domainEnvelope objectEnvelope
	decodeBody(t, domainResp.Body, &domainEnvelope)
	domainID := domainEnvelope.Data.(map[string]any)["id"].(string)

	headers := map[string]string{
		headerEnvironmentID: "66666666-6666-4666-8666-666666666666",
	}
	bindResp := performJSONRequest(t, handler, http.MethodPost, "/api/v1/domains/"+domainID+"/bind-dns-credential", `{"dns_credential_id":"`+credentialID+`"}`, headers)
	if bindResp.Code != http.StatusConflict {
		t.Fatalf("bind status = %d, want %d", bindResp.Code, http.StatusConflict)
	}
	var errResp errorResponse
	decodeBody(t, bindResp.Body, &errResp)
	if errResp.Error.Code != "TENANT_SCOPE_MISMATCH" {
		t.Fatalf("error code = %q, want %q", errResp.Error.Code, "TENANT_SCOPE_MISMATCH")
	}
}

func newGovernanceRouter(t *testing.T) http.Handler {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	domainCommands := domainscmd.NewService(domains.NewService(), dnscredentials.NewService(), domainscmd.NopAuditRecorder{})
	caAccountCommands := caaccountscmd.NewService(issuer.NewService())

	return NewRouter(Deps{
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
		Logger:            logger,
		DomainCommands:    domainCommands,
		CAAccountCommands: caAccountCommands,
	})
}

func performJSONRequest(t *testing.T, handler http.Handler, method string, target string, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeBody(t *testing.T, body *bytes.Buffer, target any) {
	t.Helper()
	if err := json.Unmarshal(body.Bytes(), target); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
}
