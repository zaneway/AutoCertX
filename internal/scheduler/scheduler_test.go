package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	jobcommand "github.com/zaneway/AutoCertX/internal/application/command/jobs"
	"github.com/zaneway/AutoCertX/internal/domain/job"
)

func TestWorkerRunOnceCompletesJob(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	repo := jobcommand.NewMemoryRepository()
	service := mustService(t, repo)

	record, err := service.Schedule(context.Background(), jobcommand.ScheduleInput{
		TenantID:       "tenant-1",
		ProjectID:      "project-1",
		EnvironmentID:  "env-1",
		JobType:        "renewal_scan",
		AggregateType:  "environment",
		AggregateID:    "env-1",
		IdempotencyKey: "renewal:env-1",
		Now:            now,
	})
	if err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}

	worker := Worker{
		Service:       service,
		Executor:      staticExecutor{result: ExecutionResult{ResultStatus: job.AttemptStatusSucceeded}},
		WorkerID:      "worker-a",
		MaxJobs:       1,
		LeaseTTL:      time.Minute,
		RenewInterval: 0,
		Clock:         clockSequence(now, now.Add(time.Second), now.Add(2*time.Second)),
	}

	processed, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}

	stored, err := service.Get(context.Background(), record.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if stored.Status != job.StatusSucceeded {
		t.Fatalf("Status = %q, want %q", stored.Status, job.StatusSucceeded)
	}

	attempts, err := service.Attempts(context.Background(), record.ID)
	if err != nil {
		t.Fatalf("Attempts() error = %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("len(attempts) = %d, want 1", len(attempts))
	}
	if attempts[0].ResultStatus != job.AttemptStatusSucceeded {
		t.Fatalf("Attempt.ResultStatus = %q, want %q", attempts[0].ResultStatus, job.AttemptStatusSucceeded)
	}
}

func TestWorkerRenewsLeaseDuringLongExecution(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	repo := jobcommand.NewMemoryRepository()
	service := mustService(t, repo)

	record, err := service.Schedule(context.Background(), jobcommand.ScheduleInput{
		TenantID:       "tenant-1",
		ProjectID:      "project-1",
		EnvironmentID:  "env-1",
		JobType:        "discovery_scan",
		AggregateType:  "environment",
		AggregateID:    "env-1",
		IdempotencyKey: "discovery:env-1",
		Now:            now,
	})
	if err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}

	worker := Worker{
		Service:       service,
		Executor:      sleepingExecutor{sleep: 80 * time.Millisecond},
		WorkerID:      "worker-a",
		MaxJobs:       1,
		LeaseTTL:      50 * time.Millisecond,
		RenewInterval: 15 * time.Millisecond,
		Clock:         time.Now,
	}

	processed, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}

	attempts, err := service.Attempts(context.Background(), record.ID)
	if err != nil {
		t.Fatalf("Attempts() error = %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("len(attempts) = %d, want 1", len(attempts))
	}
	if attempts[0].LastHeartbeatAt.IsZero() {
		t.Fatal("LastHeartbeatAt should be set after lease renewals")
	}
}

func TestPlannerAndReaperRunOnce(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	repo := jobcommand.NewMemoryRepository()
	service := mustService(t, repo)

	planner := Planner{
		Service: service,
		Plans: []jobcommand.PlanDefinition{
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
		},
		Clock: func() time.Time { return now },
	}

	created, err := planner.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("Planner.RunOnce() error = %v", err)
	}
	if created != 1 {
		t.Fatalf("created = %d, want 1", created)
	}
	created, err = planner.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("Planner.RunOnce() second error = %v", err)
	}
	if created != 0 {
		t.Fatalf("created = %d, want 0", created)
	}

	plannedJobs := repo.Jobs()
	if len(plannedJobs) != 1 {
		t.Fatalf("len(plannedJobs) = %d, want 1", len(plannedJobs))
	}

	claims, err := service.Claim(context.Background(), jobcommand.ClaimParams{
		WorkerID: "worker-a",
		MaxJobs:  1,
		LeaseTTL: 10 * time.Second,
		Now:      now,
	})
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if len(claims) != 1 {
		t.Fatalf("len(claims) = %d, want 1", len(claims))
	}
	if _, err := service.MarkRunning(context.Background(), jobcommand.MarkRunningParams{
		JobID:    claims[0].Job.ID,
		WorkerID: "worker-a",
		Now:      now.Add(time.Second),
	}); err != nil {
		t.Fatalf("MarkRunning() error = %v", err)
	}

	reaper := Reaper{
		Service: service,
		Limit:   10,
		Clock:   func() time.Time { return now.Add(12 * time.Second) },
	}
	reaped, err := reaper.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("Reaper.RunOnce() error = %v", err)
	}
	if reaped != 1 {
		t.Fatalf("reaped = %d, want 1", reaped)
	}

	stored, err := service.Get(context.Background(), plannedJobs[0].ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if stored.Status != job.StatusRetry {
		t.Fatalf("Status = %q, want %q", stored.Status, job.StatusRetry)
	}
}

type staticExecutor struct {
	result ExecutionResult
}

func (e staticExecutor) Execute(context.Context, jobcommand.ClaimedJob) ExecutionResult {
	return e.result
}

type sleepingExecutor struct {
	sleep time.Duration
}

func (e sleepingExecutor) Execute(ctx context.Context, _ jobcommand.ClaimedJob) ExecutionResult {
	timer := time.NewTimer(e.sleep)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ExecutionResult{
			ResultStatus: job.AttemptStatusAbandoned,
			ErrorCode:    "cancelled",
			ErrorMessage: ctx.Err().Error(),
		}
	case <-timer.C:
		return ExecutionResult{ResultStatus: job.AttemptStatusSucceeded}
	}
}

func mustService(t *testing.T, repo jobcommand.Repository) *jobcommand.Service {
	t.Helper()

	service, err := jobcommand.NewService(repo, jobcommand.Options{
		RetryPolicy: jobcommand.BackoffPolicy{
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

func clockSequence(times ...time.Time) func() time.Time {
	var index atomic.Int64
	return func() time.Time {
		current := int(index.Add(1)) - 1
		if current < len(times) {
			return times[current]
		}
		return times[len(times)-1]
	}
}
