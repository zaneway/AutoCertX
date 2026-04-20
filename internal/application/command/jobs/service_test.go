package jobs

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/zaneway/AutoCertX/internal/domain/job"
)

func TestEnsurePlansAvoidsDuplicateJobs(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 3, 47, 0, time.UTC)
	repo := NewMemoryRepository()
	service := mustService(t, repo, now)

	plans := []PlanDefinition{
		{
			Name:          "renewal-scan",
			TenantID:      "tenant-1",
			ProjectID:     "project-1",
			EnvironmentID: "env-1",
			JobType:       "renewal_scan",
			AggregateType: "environment",
			AggregateID:   "env-1",
			Interval:      time.Hour,
		},
		{
			Name:          "discovery-plan",
			TenantID:      "tenant-1",
			ProjectID:     "project-1",
			EnvironmentID: "env-1",
			JobType:       "discovery_scan",
			AggregateType: "environment",
			AggregateID:   "env-1",
			Interval:      time.Hour,
		},
	}

	first, err := service.EnsurePlans(context.Background(), plans, now)
	if err != nil {
		t.Fatalf("EnsurePlans() error = %v", err)
	}
	second, err := service.EnsurePlans(context.Background(), plans, now.Add(10*time.Minute))
	if err != nil {
		t.Fatalf("EnsurePlans() second error = %v", err)
	}

	for idx, result := range first {
		if !result.Created {
			t.Fatalf("first[%d].Created = false, want true", idx)
		}
	}
	for idx, result := range second {
		if result.Created {
			t.Fatalf("second[%d].Created = true, want false", idx)
		}
	}

	if got := len(repo.Jobs()); got != 2 {
		t.Fatalf("len(repo.Jobs()) = %d, want 2", got)
	}
}

func TestConcurrentClaimDoesNotDuplicateJobs(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	repo := NewMemoryRepository()
	service := mustService(t, repo, now)

	for idx := 0; idx < 12; idx++ {
		_, err := service.Schedule(context.Background(), ScheduleInput{
			TenantID:       "tenant-1",
			ProjectID:      "project-1",
			EnvironmentID:  "env-1",
			JobType:        "renewal_scan",
			AggregateType:  "environment",
			AggregateID:    fmt.Sprintf("env-%d", idx),
			IdempotencyKey: fmt.Sprintf("renewal:%d", idx),
			Now:            now,
		})
		if err != nil {
			t.Fatalf("Schedule(%d) error = %v", idx, err)
		}
	}

	type claimBatch struct {
		items []ClaimedJob
		err   error
	}

	results := make(chan claimBatch, 4)
	var wg sync.WaitGroup
	for idx := 0; idx < 4; idx++ {
		wg.Add(1)
		go func(workerID string) {
			defer wg.Done()
			items, err := service.Claim(context.Background(), ClaimParams{
				WorkerID: workerID,
				MaxJobs:  12,
				LeaseTTL: time.Minute,
				Now:      now,
			})
			results <- claimBatch{items: items, err: err}
		}(fmt.Sprintf("worker-%d", idx))
	}

	wg.Wait()
	close(results)

	seen := make(map[string]string)
	total := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("Claim() error = %v", result.err)
		}
		for _, item := range result.items {
			total++
			if owner, exists := seen[item.Job.ID]; exists {
				t.Fatalf("job %s claimed twice: %s and %s", item.Job.ID, owner, item.Job.LeaseOwner)
			}
			seen[item.Job.ID] = item.Job.LeaseOwner
		}
	}

	if total != 12 {
		t.Fatalf("total claimed = %d, want 12", total)
	}
	if len(seen) != 12 {
		t.Fatalf("unique claimed jobs = %d, want 12", len(seen))
	}
}

func TestCompleteRetryableSchedulesRetry(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	repo := NewMemoryRepository()
	service := mustService(t, repo, now)

	record, err := service.Schedule(context.Background(), ScheduleInput{
		TenantID:       "tenant-1",
		ProjectID:      "project-1",
		EnvironmentID:  "env-1",
		JobType:        "deploy_nginx",
		AggregateType:  "certificate_version",
		AggregateID:    "cert-v1",
		IdempotencyKey: "deploy:cert-v1",
		Now:            now,
	})
	if err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}

	claims, err := service.Claim(context.Background(), ClaimParams{
		WorkerID: "worker-a",
		MaxJobs:  1,
		LeaseTTL: 30 * time.Second,
		Now:      now,
	})
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if len(claims) != 1 {
		t.Fatalf("len(claims) = %d, want 1", len(claims))
	}

	if _, err := service.MarkRunning(context.Background(), MarkRunningParams{
		JobID:    claims[0].Job.ID,
		WorkerID: "worker-a",
		Now:      now.Add(time.Second),
	}); err != nil {
		t.Fatalf("MarkRunning() error = %v", err)
	}

	completion, err := service.Complete(context.Background(), CompleteParams{
		JobID:        record.ID,
		WorkerID:     "worker-a",
		ResultStatus: job.AttemptStatusFailed,
		Retryable:    true,
		ErrorCode:    "temporary_dns_error",
		ErrorMessage: "alidns timeout",
		Now:          now.Add(5 * time.Second),
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if !completion.RetryScheduled {
		t.Fatal("RetryScheduled = false, want true")
	}
	if completion.Job.Status != job.StatusRetry {
		t.Fatalf("Status = %q, want %q", completion.Job.Status, job.StatusRetry)
	}
	if completion.Job.NextRunAt != now.Add(10*time.Second) {
		t.Fatalf("NextRunAt = %s, want %s", completion.Job.NextRunAt, now.Add(10*time.Second))
	}
	if completion.Attempt.ResultStatus != job.AttemptStatusFailed {
		t.Fatalf("Attempt.ResultStatus = %q, want %q", completion.Attempt.ResultStatus, job.AttemptStatusFailed)
	}

	attempts, err := service.Attempts(context.Background(), record.ID)
	if err != nil {
		t.Fatalf("Attempts() error = %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("len(attempts) = %d, want 1", len(attempts))
	}
}

func TestReapExpiredLeaseSchedulesRetryAndKeepsHistory(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	repo := NewMemoryRepository()
	service := mustService(t, repo, now)

	record, err := service.Schedule(context.Background(), ScheduleInput{
		TenantID:       "tenant-1",
		ProjectID:      "project-1",
		EnvironmentID:  "env-1",
		JobType:        "discovery_scan",
		AggregateType:  "environment",
		AggregateID:    "env-1",
		IdempotencyKey: "discover:env-1",
		MaxAttempts:    2,
		Now:            now,
	})
	if err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}

	claims, err := service.Claim(context.Background(), ClaimParams{
		WorkerID: "worker-a",
		MaxJobs:  1,
		LeaseTTL: 15 * time.Second,
		Now:      now,
	})
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if len(claims) != 1 {
		t.Fatalf("len(claims) = %d, want 1", len(claims))
	}

	if _, err := service.MarkRunning(context.Background(), MarkRunningParams{
		JobID:    record.ID,
		WorkerID: "worker-a",
		Now:      now.Add(time.Second),
	}); err != nil {
		t.Fatalf("MarkRunning() error = %v", err)
	}

	reaped, err := service.ReapExpired(context.Background(), 10, now.Add(20*time.Second))
	if err != nil {
		t.Fatalf("ReapExpired() error = %v", err)
	}
	if len(reaped) != 1 {
		t.Fatalf("len(reaped) = %d, want 1", len(reaped))
	}

	item := reaped[0]
	if !item.RetryScheduled {
		t.Fatal("RetryScheduled = false, want true")
	}
	if item.Job.Status != job.StatusRetry {
		t.Fatalf("Status = %q, want %q", item.Job.Status, job.StatusRetry)
	}
	if item.Attempt.ResultStatus != job.AttemptStatusTimedOut {
		t.Fatalf("Attempt.ResultStatus = %q, want %q", item.Attempt.ResultStatus, job.AttemptStatusTimedOut)
	}

	attempts, err := service.Attempts(context.Background(), record.ID)
	if err != nil {
		t.Fatalf("Attempts() error = %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("len(attempts) = %d, want 1", len(attempts))
	}
}

func mustService(t *testing.T, repo Repository, now time.Time) *Service {
	t.Helper()

	service, err := NewService(repo, Options{
		Clock: func() time.Time { return now },
		RetryPolicy: BackoffPolicy{
			Initial:    5 * time.Second,
			Max:        time.Minute,
			Multiplier: 2,
		},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	return service
}
