package wiring

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	controlplanehttp "github.com/zaneway/AutoCertX/internal/app/controlplane/http"
	caaccountscmd "github.com/zaneway/AutoCertX/internal/application/command/caaccounts"
	domainscmd "github.com/zaneway/AutoCertX/internal/application/command/domains"
	commandjobs "github.com/zaneway/AutoCertX/internal/application/command/jobs"
	settingscmd "github.com/zaneway/AutoCertX/internal/application/command/settings"
	auditquery "github.com/zaneway/AutoCertX/internal/application/query/audit"
	dashboardquery "github.com/zaneway/AutoCertX/internal/application/query/dashboard"
	domainsquery "github.com/zaneway/AutoCertX/internal/application/query/domains"
	jobsquery "github.com/zaneway/AutoCertX/internal/application/query/jobs"
	settingsquery "github.com/zaneway/AutoCertX/internal/application/query/settings"
	auditdomain "github.com/zaneway/AutoCertX/internal/domain/audit"
	"github.com/zaneway/AutoCertX/internal/domain/dnscredentials"
	domaindomain "github.com/zaneway/AutoCertX/internal/domain/domains"
	"github.com/zaneway/AutoCertX/internal/domain/issuer"
	"github.com/zaneway/AutoCertX/internal/domain/job"
	"github.com/zaneway/AutoCertX/internal/domain/resource"
	settingsdomain "github.com/zaneway/AutoCertX/internal/domain/settings"
	"github.com/zaneway/AutoCertX/internal/platform/buildinfo"
	"github.com/zaneway/AutoCertX/internal/platform/config"
)

func TestQueryRoutesRequireAuthentication(t *testing.T) {
	handler, _ := newQueryIntegrationRouter(t)

	resp := performJSONRequest(t, handler, http.MethodGet, "/api/v1/jobs", nil, nil)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/v1/jobs status = %d, want %d", resp.Code, http.StatusUnauthorized)
	}
}

func TestQueryRoutesAdminFlow(t *testing.T) {
	handler, seed := newQueryIntegrationRouter(t)
	login := loginForQueryTest(t, handler, "admin", "admin123!")
	headers := adminScopeHeaders(login.AccessToken)

	dashboard := performJSONRequest(t, handler, http.MethodGet, "/api/v1/dashboard/summary", nil, headers)
	if dashboard.Code != http.StatusOK {
		t.Fatalf("dashboard summary status = %d, want %d", dashboard.Code, http.StatusOK)
	}
	var dashboardPayload struct {
		Data struct {
			DomainCount        int `json:"domain_count"`
			DNSCredentialCount int `json:"dns_credential_count"`
			CAAccountCount     int `json:"ca_account_count"`
			WebhookCount       int `json:"webhook_count"`
			FailedJobCount     int `json:"failed_job_count"`
		} `json:"data"`
	}
	decodeResponse(t, dashboard.Body.Bytes(), &dashboardPayload)
	if dashboardPayload.Data.DomainCount != 1 || dashboardPayload.Data.DNSCredentialCount != 1 || dashboardPayload.Data.CAAccountCount != 1 || dashboardPayload.Data.WebhookCount != 1 || dashboardPayload.Data.FailedJobCount != 1 {
		t.Fatalf("dashboard summary = %+v, want all counts = 1", dashboardPayload.Data)
	}

	domains := performJSONRequest(t, handler, http.MethodGet, "/api/v1/domains", nil, headers)
	if domains.Code != http.StatusOK {
		t.Fatalf("domain list status = %d, want %d", domains.Code, http.StatusOK)
	}
	var domainsPayload struct {
		Items []map[string]any `json:"items"`
	}
	decodeResponse(t, domains.Body.Bytes(), &domainsPayload)
	if len(domainsPayload.Items) != 1 {
		t.Fatalf("domain list count = %d, want 1", len(domainsPayload.Items))
	}

	jobs := performJSONRequest(t, handler, http.MethodGet, "/api/v1/jobs?status=failed", nil, headers)
	if jobs.Code != http.StatusOK {
		t.Fatalf("job list status = %d, want %d", jobs.Code, http.StatusOK)
	}
	var jobsPayload struct {
		Items []map[string]any `json:"items"`
	}
	decodeResponse(t, jobs.Body.Bytes(), &jobsPayload)
	if len(jobsPayload.Items) != 1 {
		t.Fatalf("job list count = %d, want 1", len(jobsPayload.Items))
	}
	if jobsPayload.Items[0]["id"] != seed.JobID {
		t.Fatalf("job id = %v, want %q", jobsPayload.Items[0]["id"], seed.JobID)
	}

	jobDetail := performJSONRequest(t, handler, http.MethodGet, "/api/v1/jobs/"+seed.JobID, nil, headers)
	if jobDetail.Code != http.StatusOK {
		t.Fatalf("job detail status = %d, want %d", jobDetail.Code, http.StatusOK)
	}

	attempts := performJSONRequest(t, handler, http.MethodGet, "/api/v1/jobs/"+seed.JobID+"/attempts", nil, headers)
	if attempts.Code != http.StatusOK {
		t.Fatalf("job attempts status = %d, want %d", attempts.Code, http.StatusOK)
	}
	var attemptsPayload struct {
		Items []map[string]any `json:"items"`
	}
	decodeResponse(t, attempts.Body.Bytes(), &attemptsPayload)
	if len(attemptsPayload.Items) != 1 {
		t.Fatalf("attempt count = %d, want 1", len(attemptsPayload.Items))
	}
}

type querySeed struct {
	JobID string
}

type queryLoginResult struct {
	AccessToken string `json:"access_token"`
}

func newQueryIntegrationRouter(t *testing.T) (http.Handler, querySeed) {
	t.Helper()

	cfg := config.Config{
		ServiceName: "controlplane",
		Environment: "test",
		Auth: config.AuthConfig{
			SigningKey:      "test-signing-key",
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 24 * time.Hour,
		},
	}
	authService, authContextQuery, err := newAuthServices(cfg)
	if err != nil {
		t.Fatalf("newAuthServices() error = %v", err)
	}

	scope := resource.Scope{
		TenantID:      "11111111-1111-4111-8111-111111111111",
		ProjectID:     "22222222-2222-4222-8222-222222222222",
		EnvironmentID: "33333333-3333-4333-8333-333333333332",
	}
	now := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)

	domainService := domaindomain.NewService()
	dnsService := dnscredentials.NewService()
	issuerService := issuer.NewService()
	auditService := auditdomain.NewService()
	settingsService := settingsdomain.NewService()
	jobRepo := commandjobs.NewMemoryRepository()
	jobCommandService, err := commandjobs.NewService(jobRepo, commandjobs.Options{
		Clock: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("commandjobs.NewService() error = %v", err)
	}

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
	if _, err := domainService.Create(scope, domaindomain.UpsertInput{
		Name:            "api.example.com",
		ChallengeType:   domaindomain.ChallengeDNS01,
		AutoRenew:       true,
		DNSCredentialID: credential.ID,
		DNSProvider:     credential.ProviderType,
	}); err != nil {
		t.Fatalf("domainService.Create() error = %v", err)
	}
	if _, err := issuerService.Create(scope, issuer.UpsertInput{
		DisplayName:  "letsencrypt-staging",
		DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
		Email:        "ops@example.com",
	}); err != nil {
		t.Fatalf("issuerService.Create() error = %v", err)
	}
	if _, err := auditService.CreateWebhookEndpoint(scope, auditdomain.WebhookUpsertInput{
		Name:       "ops-primary",
		URL:        "https://ops.example.com/webhooks/audit",
		Secret:     "secret",
		EventTypes: []string{"settings.security.update"},
		Enabled:    true,
	}); err != nil {
		t.Fatalf("auditService.CreateWebhookEndpoint() error = %v", err)
	}

	jobRecord, err := jobCommandService.Schedule(context.Background(), commandjobs.ScheduleInput{
		TenantID:       scope.TenantID,
		ProjectID:      scope.ProjectID,
		EnvironmentID:  scope.EnvironmentID,
		JobType:        "domain.bind_dns_credential",
		AggregateType:  "domain_asset",
		AggregateID:    "asset-1",
		IdempotencyKey: "query-job-1",
		NextRunAt:      now,
		Now:            now,
	})
	if err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	claimed, err := jobCommandService.Claim(context.Background(), commandjobs.ClaimParams{
		WorkerID: "worker-1",
		MaxJobs:  1,
		LeaseTTL: time.Minute,
		Now:      now.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("Claim() count = %d, want 1", len(claimed))
	}
	if _, err := jobCommandService.MarkRunning(context.Background(), commandjobs.MarkRunningParams{
		JobID:    jobRecord.ID,
		WorkerID: "worker-1",
		Now:      now.Add(2 * time.Second),
	}); err != nil {
		t.Fatalf("MarkRunning() error = %v", err)
	}
	if _, err := jobCommandService.Complete(context.Background(), commandjobs.CompleteParams{
		JobID:        jobRecord.ID,
		WorkerID:     "worker-1",
		ResultStatus: job.AttemptStatusFailed,
		Retryable:    false,
		ErrorCode:    "executor_failed",
		ErrorMessage: "execution failed",
		Now:          now.Add(3 * time.Second),
	}); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	governanceQuery, err := domainsquery.NewService(domainService, dnsService, issuerService)
	if err != nil {
		t.Fatalf("domainsquery.NewService() error = %v", err)
	}
	settingsQuery, err := settingsquery.NewService(auditService, settingsService)
	if err != nil {
		t.Fatalf("settingsquery.NewService() error = %v", err)
	}
	jobsQuery, err := jobsquery.NewService(jobRepo)
	if err != nil {
		t.Fatalf("jobsquery.NewService() error = %v", err)
	}
	dashboardQuery, err := dashboardquery.NewService(domainService, dnsService, issuerService, auditService, jobRepo)
	if err != nil {
		t.Fatalf("dashboardquery.NewService() error = %v", err)
	}

	handler := controlplanehttp.NewRouter(controlplanehttp.Deps{
		Config:            cfg,
		BuildInfo:         buildinfo.Info{Service: "controlplane", Version: "dev", Commit: "abc123", BuildTime: "2026-04-22T00:00:00Z"},
		Logger:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		AuthService:       authService,
		AuthContextQuery:  authContextQuery,
		DomainCommands:    domainscmd.NewService(domainService, dnsService, domainscmd.NopAuditRecorder{}),
		CAAccountCommands: caaccountscmd.NewService(issuerService),
		SettingsCommands:  settingscmd.NewService(auditService, settingsService, ""),
		GovernanceQuery:   governanceQuery,
		SettingsQuery:     settingsQuery,
		JobsQuery:         jobsQuery,
		DashboardQuery:    dashboardQuery,
		AuditQuery:        auditquery.NewService(auditService),
	})

	return handler, querySeed{JobID: jobRecord.ID}
}

func loginForQueryTest(t *testing.T, handler http.Handler, username string, password string) queryLoginResult {
	t.Helper()

	resp := performJSONRequest(t, handler, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": username,
		"password": password,
	}, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d", resp.Code, http.StatusOK)
	}

	var payload queryLoginResult
	decodeResponse(t, resp.Body.Bytes(), &payload)
	if payload.AccessToken == "" {
		t.Fatal("access_token should not be empty")
	}
	return payload
}

func adminScopeHeaders(token string) map[string]string {
	return map[string]string{
		"Authorization":    "Bearer " + token,
		"X-Tenant-Id":      "11111111-1111-4111-8111-111111111111",
		"X-Project-Id":     "22222222-2222-4222-8222-222222222222",
		"X-Environment-Id": "33333333-3333-4333-8333-333333333332",
	}
}

func performJSONRequest(t *testing.T, handler http.Handler, method string, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	var payload []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		payload = encoded
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeResponse(t *testing.T, body []byte, dst any) {
	t.Helper()
	if err := json.Unmarshal(body, dst); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; body = %s", err, string(body))
	}
}
