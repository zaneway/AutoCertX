package workflow

import (
	"context"
	"testing"
	"time"

	jobscmd "github.com/zaneway/AutoCertX/internal/application/command/jobs"
	"github.com/zaneway/AutoCertX/internal/domain/certificateasset"
	"github.com/zaneway/AutoCertX/internal/domain/certificaterequest"
	"github.com/zaneway/AutoCertX/internal/domain/domains"
	"github.com/zaneway/AutoCertX/internal/domain/issuer"
	"github.com/zaneway/AutoCertX/internal/domain/issueworkflow"
	"github.com/zaneway/AutoCertX/internal/domain/resource"
	acmedriver "github.com/zaneway/AutoCertX/internal/driver/acme"
	dnsdriver "github.com/zaneway/AutoCertX/internal/driver/dns"
	"github.com/zaneway/AutoCertX/internal/scheduler"
)

func TestServiceProcessJobRetriesAndCompletesDNSWorkflow(t *testing.T) {
	scope := resource.Scope{
		TenantID:      "11111111-1111-4111-8111-111111111111",
		ProjectID:     "22222222-2222-4222-8222-222222222222",
		EnvironmentID: "33333333-3333-4333-8333-333333333333",
	}
	clock := &stepClock{
		current: time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC),
		step:    10 * time.Second,
	}

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

	acmeClient := acmedriver.NewFakeClient()
	acmeClient.PendingPolls = 1
	dnsExecutor := dnsdriver.NewFakeExecutor()
	dnsExecutor.PendingChecks = 1

	service, err := NewService(
		requestService,
		workflowDomainService,
		assetService,
		domainService,
		issuerService,
		jobsService,
		acmeClient,
		dnsExecutor,
		FakeHTTP01Presenter{},
	)
	if err != nil {
		t.Fatalf("new workflow service: %v", err)
	}
	service.now = clock.Now

	result, err := service.SubmitRequest(context.Background(), scope, "44444444-4444-4444-8444-444444444444", SubmitInput{
		DomainIDs:       []string{domain.ID},
		CAAccountID:     account.ID,
		CertificateType: certificaterequest.CertificateTypeSingle,
		ChallengeType:   certificaterequest.ChallengeTypeDNS01,
		IdempotencyKey:  "issue:api.example.com:2026-04-24",
	})
	if err != nil {
		t.Fatalf("SubmitRequest() error = %v", err)
	}
	if result.JobID == "" {
		t.Fatal("SubmitRequest() should return job id")
	}

	worker := scheduler.Worker{
		Service:  jobsService,
		Executor: Executor{Service: service},
		WorkerID: "worker-a",
		MaxJobs:  1,
		LeaseTTL: time.Hour,
		Clock:    clock.Now,
	}

	for i := 0; i < 10; i++ {
		if _, err := worker.RunOnce(context.Background()); err != nil {
			t.Fatalf("worker run %d: %v", i+1, err)
		}
		request, err := requestService.Get(scope, result.RequestID)
		if err != nil {
			t.Fatalf("Get(request) error = %v", err)
		}
		workflowRecord, err := workflowDomainService.GetByRequest(scope, result.RequestID)
		if err != nil {
			t.Fatalf("GetByRequest() error = %v", err)
		}
		_ = workflowRecord
		if request.Status == certificaterequest.StatusCompleted {
			break
		}
		if i == 9 {
			t.Fatalf("request status = %q, want completed", request.Status)
		}
	}

	request, err := requestService.Get(scope, result.RequestID)
	if err != nil {
		t.Fatalf("Get(request) error = %v", err)
	}
	if request.Status != certificaterequest.StatusCompleted {
		t.Fatalf("request status = %q, want %q", request.Status, certificaterequest.StatusCompleted)
	}

	workflowRecord, err := workflowDomainService.GetByRequest(scope, request.ID)
	if err != nil {
		t.Fatalf("GetByRequest() error = %v", err)
	}
	if workflowRecord.Status != issueworkflow.StatusIssued {
		t.Fatalf("workflow status = %q, want %q", workflowRecord.Status, issueworkflow.StatusIssued)
	}

	assets, err := assetService.List(scope)
	if err != nil {
		t.Fatalf("List(assets) error = %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("len(assets) = %d, want 1", len(assets))
	}

	versions, err := assetService.ListVersions(scope, assets[0].ID)
	if err != nil {
		t.Fatalf("ListVersions() error = %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("len(versions) = %d, want 1", len(versions))
	}
	if versions[0].CertificateRequestID != request.ID {
		t.Fatalf("version.CertificateRequestID = %q, want %q", versions[0].CertificateRequestID, request.ID)
	}

	linked, err := domainService.ListCertificateAssets(scope, domain.ID)
	if err != nil {
		t.Fatalf("ListCertificateAssets() error = %v", err)
	}
	if len(linked) != 1 {
		t.Fatalf("len(linked) = %d, want 1", len(linked))
	}
	if linked[0].ID != assets[0].ID {
		t.Fatalf("linked asset id = %q, want %q", linked[0].ID, assets[0].ID)
	}
}

type stepClock struct {
	current time.Time
	step    time.Duration
}

func (c *stepClock) Now() time.Time {
	now := c.current
	c.current = c.current.Add(c.step)
	return now
}
