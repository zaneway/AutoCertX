package dashboard

import (
	"context"
	"testing"
	"time"

	commandjobs "github.com/zaneway/AutoCertX/internal/application/command/jobs"
	auditdomain "github.com/zaneway/AutoCertX/internal/domain/audit"
	"github.com/zaneway/AutoCertX/internal/domain/dnscredentials"
	domaindomain "github.com/zaneway/AutoCertX/internal/domain/domains"
	"github.com/zaneway/AutoCertX/internal/domain/issuer"
	"github.com/zaneway/AutoCertX/internal/domain/job"
	"github.com/zaneway/AutoCertX/internal/domain/resource"
)

func TestDashboardSummaryAndJobFailures(t *testing.T) {
	scope := resource.Scope{
		TenantID:      "11111111-1111-4111-8111-111111111111",
		ProjectID:     "22222222-2222-4222-8222-222222222222",
		EnvironmentID: "33333333-3333-4333-8333-333333333332",
	}
	now := time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC)

	domainService := domaindomain.NewService()
	dnsService := dnscredentials.NewService()
	issuerService := issuer.NewService()
	auditService := auditdomain.NewService()
	jobRepo := commandjobs.NewMemoryRepository()
	jobCommandService, err := commandjobs.NewService(jobRepo, commandjobs.Options{
		Clock: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("commandjobs.NewService() error = %v", err)
	}

	if _, err := domainService.Create(scope, domaindomain.UpsertInput{
		Name:          "api.example.com",
		ChallengeType: domaindomain.ChallengeDNS01,
		AutoRenew:     true,
	}); err != nil {
		t.Fatalf("domainService.Create() error = %v", err)
	}
	if _, err := dnsService.Create(scope, dnscredentials.UpsertInput{
		DisplayName:  "alidns-prod",
		ProviderType: dnscredentials.ProviderAliDNS,
		AccessKeyID:  "ak",
		Secret:       "sk",
		ScopeMode:    dnscredentials.ScopeEnvironment,
	}); err != nil {
		t.Fatalf("dnsService.Create() error = %v", err)
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

	failedJob := scheduleFailedJob(t, jobCommandService, scope, now)

	service, err := NewService(domainService, dnsService, issuerService, auditService, jobRepo)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	summary, err := service.GetSummary(context.Background(), scope)
	if err != nil {
		t.Fatalf("GetSummary() error = %v", err)
	}
	if summary.DomainCount != 1 || summary.DNSCredentialCount != 1 || summary.CAAccountCount != 1 || summary.WebhookCount != 1 || summary.FailedJobCount != 1 {
		t.Fatalf("summary = %+v, want one populated item for each metric", summary)
	}

	failures, err := service.ListJobFailures(context.Background(), scope, 10)
	if err != nil {
		t.Fatalf("ListJobFailures() error = %v", err)
	}
	if len(failures) != 1 {
		t.Fatalf("failure count = %d, want 1", len(failures))
	}
	if failures[0].ID != failedJob.ID {
		t.Fatalf("failure id = %q, want %q", failures[0].ID, failedJob.ID)
	}
	if failures[0].Status != string(job.StatusFailed) {
		t.Fatalf("failure status = %q, want %q", failures[0].Status, job.StatusFailed)
	}
}

func scheduleFailedJob(t *testing.T, service *commandjobs.Service, scope resource.Scope, now time.Time) job.Job {
	t.Helper()
	record, err := service.Schedule(context.Background(), commandjobs.ScheduleInput{
		TenantID:       scope.TenantID,
		ProjectID:      scope.ProjectID,
		EnvironmentID:  scope.EnvironmentID,
		JobType:        "domain.bind_dns_credential",
		AggregateType:  "domain_asset",
		AggregateID:    "domain-1",
		IdempotencyKey: "dashboard-job-1",
		NextRunAt:      now,
		Now:            now,
	})
	if err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	claimed, err := service.Claim(context.Background(), commandjobs.ClaimParams{
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
	if _, err := service.MarkRunning(context.Background(), commandjobs.MarkRunningParams{
		JobID:    record.ID,
		WorkerID: "worker-1",
		Now:      now.Add(2 * time.Second),
	}); err != nil {
		t.Fatalf("MarkRunning() error = %v", err)
	}
	result, err := service.Complete(context.Background(), commandjobs.CompleteParams{
		JobID:        record.ID,
		WorkerID:     "worker-1",
		ResultStatus: job.AttemptStatusFailed,
		Retryable:    false,
		ErrorCode:    "executor_failed",
		ErrorMessage: "execution failed",
		Now:          now.Add(3 * time.Second),
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	return result.Job
}
