package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	commandjobs "github.com/zaneway/AutoCertX/internal/application/command/jobs"
	"github.com/zaneway/AutoCertX/internal/domain/job"
	"github.com/zaneway/AutoCertX/internal/domain/resource"
	"github.com/zaneway/AutoCertX/internal/platform/apperr"
)

func TestListJobsFiltersByScopeAndStatus(t *testing.T) {
	now := time.Date(2026, 4, 22, 8, 0, 0, 0, time.UTC)
	repo := commandjobs.NewMemoryRepository()
	commandService, err := commandjobs.NewService(repo, commandjobs.Options{
		Clock: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("commandjobs.NewService() error = %v", err)
	}
	queryService, err := NewService(repo)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

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

	failedJob := seedFailedJob(t, commandService, scopeA, "job-a", now)
	seedSucceededJob(t, commandService, scopeA, "job-b", now.Add(time.Minute))
	seedFailedJob(t, commandService, scopeB, "job-c", now.Add(2*time.Minute))

	items, err := queryService.ListJobs(context.Background(), scopeA, ListFilter{Status: "failed"})
	if err != nil {
		t.Fatalf("ListJobs() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("filtered jobs count = %d, want 1", len(items))
	}
	if items[0].ID != failedJob.ID {
		t.Fatalf("filtered job id = %q, want %q", items[0].ID, failedJob.ID)
	}
	if items[0].LastErrorCode != "executor_failed" {
		t.Fatalf("last_error_code = %q, want %q", items[0].LastErrorCode, "executor_failed")
	}
}

func TestGetJobAndAttemptsRespectScope(t *testing.T) {
	now := time.Date(2026, 4, 22, 8, 30, 0, 0, time.UTC)
	repo := commandjobs.NewMemoryRepository()
	commandService, err := commandjobs.NewService(repo, commandjobs.Options{
		Clock: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("commandjobs.NewService() error = %v", err)
	}
	queryService, err := NewService(repo)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

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

	failedJob := seedFailedJob(t, commandService, scopeA, "job-z", now)

	detail, err := queryService.GetJob(context.Background(), scopeA, failedJob.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if detail.AggregateID != "job-z" {
		t.Fatalf("aggregate_id = %q, want %q", detail.AggregateID, "job-z")
	}

	attempts, err := queryService.ListJobAttempts(context.Background(), scopeA, failedJob.ID)
	if err != nil {
		t.Fatalf("ListJobAttempts() error = %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("attempt count = %d, want 1", len(attempts))
	}
	if attempts[0].ResultStatus != string(job.AttemptStatusFailed) {
		t.Fatalf("attempt result_status = %q, want %q", attempts[0].ResultStatus, job.AttemptStatusFailed)
	}

	_, err = queryService.GetJob(context.Background(), scopeB, failedJob.ID)
	if err == nil {
		t.Fatal("GetJob() should reject cross-scope access")
	}
	var appErr *apperr.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("cross-scope error type = %T, want *apperr.Error", err)
	}
	if appErr.Code != "TENANT_SCOPE_MISMATCH" {
		t.Fatalf("cross-scope error code = %q, want %q", appErr.Code, "TENANT_SCOPE_MISMATCH")
	}
}

func seedFailedJob(t *testing.T, service *commandjobs.Service, scope resource.Scope, aggregateID string, now time.Time) job.Job {
	t.Helper()
	scheduled := scheduleJob(t, service, scope, "job:"+aggregateID, aggregateID, now)
	claimAndRun(t, service, scheduled.ID, "worker-"+aggregateID, now.Add(time.Second))
	result, err := service.Complete(context.Background(), commandjobs.CompleteParams{
		JobID:        scheduled.ID,
		WorkerID:     "worker-" + aggregateID,
		ResultStatus: job.AttemptStatusFailed,
		Retryable:    false,
		ErrorCode:    "executor_failed",
		ErrorMessage: "execution failed",
		Now:          now.Add(2 * time.Second),
	})
	if err != nil {
		t.Fatalf("Complete(failed) error = %v", err)
	}
	return result.Job
}

func seedSucceededJob(t *testing.T, service *commandjobs.Service, scope resource.Scope, aggregateID string, now time.Time) job.Job {
	t.Helper()
	scheduled := scheduleJob(t, service, scope, "job:"+aggregateID, aggregateID, now)
	claimAndRun(t, service, scheduled.ID, "worker-"+aggregateID, now.Add(time.Second))
	result, err := service.Complete(context.Background(), commandjobs.CompleteParams{
		JobID:        scheduled.ID,
		WorkerID:     "worker-" + aggregateID,
		ResultStatus: job.AttemptStatusSucceeded,
		Retryable:    false,
		Now:          now.Add(2 * time.Second),
	})
	if err != nil {
		t.Fatalf("Complete(succeeded) error = %v", err)
	}
	return result.Job
}

func scheduleJob(t *testing.T, service *commandjobs.Service, scope resource.Scope, idempotencyKey string, aggregateID string, now time.Time) job.Job {
	t.Helper()
	record, err := service.Schedule(context.Background(), commandjobs.ScheduleInput{
		TenantID:       scope.TenantID,
		ProjectID:      scope.ProjectID,
		EnvironmentID:  scope.EnvironmentID,
		JobType:        "domain.bind_dns_credential",
		AggregateType:  "domain_asset",
		AggregateID:    aggregateID,
		IdempotencyKey: idempotencyKey,
		NextRunAt:      now,
		Now:            now,
	})
	if err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	return record
}

func claimAndRun(t *testing.T, service *commandjobs.Service, jobID string, workerID string, now time.Time) {
	t.Helper()
	claimed, err := service.Claim(context.Background(), commandjobs.ClaimParams{
		WorkerID: workerID,
		MaxJobs:  1,
		LeaseTTL: time.Minute,
		Now:      now,
	})
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if len(claimed) != 1 || claimed[0].Job.ID != jobID {
		t.Fatalf("Claim() returned unexpected jobs: %+v", claimed)
	}
	if _, err := service.MarkRunning(context.Background(), commandjobs.MarkRunningParams{
		JobID:    jobID,
		WorkerID: workerID,
		Now:      now.Add(time.Second),
	}); err != nil {
		t.Fatalf("MarkRunning() error = %v", err)
	}
}
