package httpserver

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	certificateassetscmd "github.com/zaneway/AutoCertX/internal/application/command/certificateassets"
	jobscmd "github.com/zaneway/AutoCertX/internal/application/command/jobs"
	domainsquery "github.com/zaneway/AutoCertX/internal/application/query/domains"
	certificateasset "github.com/zaneway/AutoCertX/internal/domain/certificateasset"
	certificaterequest "github.com/zaneway/AutoCertX/internal/domain/certificaterequest"
	"github.com/zaneway/AutoCertX/internal/domain/dnscredentials"
	"github.com/zaneway/AutoCertX/internal/domain/domains"
	"github.com/zaneway/AutoCertX/internal/domain/issuer"
	issueworkflow "github.com/zaneway/AutoCertX/internal/domain/issueworkflow"
	"github.com/zaneway/AutoCertX/internal/domain/resource"
	acmedriver "github.com/zaneway/AutoCertX/internal/driver/acme"
	dnsdriver "github.com/zaneway/AutoCertX/internal/driver/dns"
	"github.com/zaneway/AutoCertX/internal/platform/buildinfo"
	"github.com/zaneway/AutoCertX/internal/platform/config"
	"github.com/zaneway/AutoCertX/internal/scheduler"
	workflowservice "github.com/zaneway/AutoCertX/internal/workflow"
)

func TestCertificateAssetAPIHappyPath(t *testing.T) {
	handler, env := newCertificateAssetRouter(t)

	body := `{"domain_ids":["` + env.domain.ID + `"],"ca_account_id":"` + env.account.ID + `","certificate_type":"single","challenge_type":"dns-01","idempotency_key":"issue:api.example.com:2026-04-24"}`
	resp := performJSONRequest(t, handler, http.MethodPost, "/api/v1/certificate-assets/requests", body, nil)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("create request status = %d, want %d: %s", resp.Code, http.StatusAccepted, resp.Body.String())
	}

	var accepted acceptedEnvelope
	decodeBody(t, resp.Body, &accepted)
	if accepted.JobID == "" {
		t.Fatal("accepted job_id should not be empty")
	}

	requestID := env.requestIDFromJobID(t, accepted.JobID)
	env.runWorkerUntil(t, requestID, 10)

	listResp := performJSONRequest(t, handler, http.MethodGet, "/api/v1/domains/"+env.domain.ID+"/certificate-assets", "", nil)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list linked assets status = %d, want %d", listResp.Code, http.StatusOK)
	}

	var linked listEnvelope
	decodeBody(t, listResp.Body, &linked)
	if len(linked.Items.([]any)) != 1 {
		t.Fatalf("len(linked.Items) = %d, want 1", len(linked.Items.([]any)))
	}
}

func TestCertificateAssetRenewAPIAppendsVersion(t *testing.T) {
	handler, env := newCertificateAssetRouter(t)

	createBody := `{"domain_ids":["` + env.domain.ID + `"],"ca_account_id":"` + env.account.ID + `","certificate_type":"single","challenge_type":"dns-01","idempotency_key":"issue:api.example.com:2026-04-24"}`
	createResp := performJSONRequest(t, handler, http.MethodPost, "/api/v1/certificate-assets/requests", createBody, nil)
	if createResp.Code != http.StatusAccepted {
		t.Fatalf("create request status = %d, want %d", createResp.Code, http.StatusAccepted)
	}
	var created acceptedEnvelope
	decodeBody(t, createResp.Body, &created)
	env.runWorkerUntil(t, env.requestIDFromJobID(t, created.JobID), 10)

	assets, err := env.assetService.List(env.scope)
	if err != nil {
		t.Fatalf("List(assets) error = %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("len(assets) = %d, want 1", len(assets))
	}

	renewResp := performJSONRequest(t, handler, http.MethodPost, "/api/v1/certificate-assets/"+assets[0].ID+"/renew", "", nil)
	if renewResp.Code != http.StatusAccepted {
		t.Fatalf("renew status = %d, want %d: %s", renewResp.Code, http.StatusAccepted, renewResp.Body.String())
	}
	var renewed acceptedEnvelope
	decodeBody(t, renewResp.Body, &renewed)

	env.runWorkerUntil(t, env.requestIDFromJobID(t, renewed.JobID), 10)

	versions, err := env.assetService.ListVersions(env.scope, assets[0].ID)
	if err != nil {
		t.Fatalf("ListVersions() error = %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("len(versions) = %d, want 2", len(versions))
	}
}

type certificateAssetTestEnv struct {
	scope           resource.Scope
	domain          domains.Asset
	account         issuer.Account
	requestService  *certificaterequest.Service
	workflowService *workflowservice.Service
	assetService    *certificateasset.Service
	clock           *assetStepClock
	jobsService     *jobscmd.Service
}

func newCertificateAssetRouter(t *testing.T) (http.Handler, *certificateAssetTestEnv) {
	t.Helper()

	scope := defaultGovernanceScope
	clock := &assetStepClock{
		current: time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC),
		step:    10 * time.Second,
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	domainService := domains.NewService()
	domain, err := domainService.Create(scope, domains.UpsertInput{
		Name:          "api.example.com",
		ChallengeType: domains.ChallengeDNS01,
	})
	if err != nil {
		t.Fatalf("seed domain: %v", err)
	}

	issuerService := issuer.NewService()
	account, err := issuerService.Create(scope, issuer.UpsertInput{
		DisplayName:  "LetsEncrypt Prod",
		DirectoryURL: "https://acme-v02.api.letsencrypt.org/directory",
		Email:        "ops@example.com",
	})
	if err != nil {
		t.Fatalf("seed ca account: %v", err)
	}

	requestService := certificaterequest.NewService()
	workflowDomainService := issueworkflow.NewService()
	assetService := certificateasset.NewService()
	jobRepo := jobscmd.NewMemoryRepository()
	jobsService, err := jobscmd.NewService(jobRepo, jobscmd.Options{
		Clock: clock.Now,
	})
	if err != nil {
		t.Fatalf("new jobs service: %v", err)
	}

	workflowSvc, err := workflowservice.NewService(
		requestService,
		workflowDomainService,
		assetService,
		domainService,
		issuerService,
		jobsService,
		acmedriver.NewFakeClient(),
		dnsdriver.NewFakeExecutor(),
		workflowservice.FakeHTTP01Presenter{},
	)
	if err != nil {
		t.Fatalf("new workflow service: %v", err)
	}
	governanceQuery, err := domainsquery.NewService(domainService, dnscredentials.NewService(), issuerService)
	if err != nil {
		t.Fatalf("domainsquery.NewService() error = %v", err)
	}

	handler := NewRouter(Deps{
		Config: config.Config{
			ServiceName: "controlplane",
			Environment: "test",
		},
		BuildInfo: buildinfo.Info{
			Service:   "controlplane",
			Version:   "dev",
			Commit:    "abc123",
			BuildTime: "2026-04-24T00:00:00Z",
		},
		Logger:            logger,
		CertificateAssets: certificateassetscmd.NewService(workflowSvc),
		GovernanceQuery:   governanceQuery,
	})

	return handler, &certificateAssetTestEnv{
		scope:           scope,
		domain:          domain,
		account:         account,
		requestService:  requestService,
		workflowService: workflowSvc,
		assetService:    assetService,
		clock:           clock,
		jobsService:     jobsService,
	}
}

func (e *certificateAssetTestEnv) requestIDFromJobID(t *testing.T, jobID string) string {
	t.Helper()

	record, err := e.jobsService.Get(t.Context(), jobID)
	if err != nil {
		t.Fatalf("Get(job) error = %v", err)
	}

	var payload struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	return payload.RequestID
}

func (e *certificateAssetTestEnv) runWorkerUntil(t *testing.T, requestID string, maxRuns int) {
	t.Helper()

	worker := scheduler.Worker{
		Service:  e.jobsService,
		Executor: workflowservice.Executor{Service: e.workflowService},
		WorkerID: "worker-a",
		MaxJobs:  1,
		LeaseTTL: time.Hour,
		Clock:    e.clock.Now,
	}

	for i := 0; i < maxRuns; i++ {
		if _, err := worker.RunOnce(t.Context()); err != nil {
			t.Fatalf("worker run %d: %v", i+1, err)
		}
		request, err := e.requestService.Get(e.scope, requestID)
		if err != nil {
			t.Fatalf("Get(request) error = %v", err)
		}
		if request.Status == certificaterequest.StatusCompleted {
			return
		}
	}

	request, err := e.requestService.Get(e.scope, requestID)
	if err != nil {
		t.Fatalf("Get(request) error = %v", err)
	}
	t.Fatalf("request status = %q after %d runs, want completed", request.Status, maxRuns)
}

type assetStepClock struct {
	current time.Time
	step    time.Duration
}

func (c *assetStepClock) Now() time.Time {
	now := c.current
	c.current = c.current.Add(c.step)
	return now
}
