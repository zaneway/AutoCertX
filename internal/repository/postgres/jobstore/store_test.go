package jobstore

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	commandjobs "github.com/zaneway/AutoCertX/internal/application/command/jobs"
	"github.com/zaneway/AutoCertX/internal/domain/job"
)

const (
	testTenantID      = "11111111-1111-1111-1111-111111111111"
	testProjectID     = "22222222-2222-2222-2222-222222222222"
	testEnvironmentID = "33333333-3333-3333-3333-333333333333"
)

func TestPostgresEnsurePlannedIdempotent(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Date(2026, 4, 20, 12, 15, 0, 0, time.UTC)
	service := mustService(t, store, now)

	plan := commandjobs.PlanDefinition{
		Name:          "renewal-scan",
		TenantID:      testTenantID,
		ProjectID:     testProjectID,
		EnvironmentID: testEnvironmentID,
		JobType:       "renewal_scan",
		AggregateType: "environment",
		AggregateID:   testEnvironmentID,
		Interval:      time.Hour,
	}

	first, err := service.EnsurePlanned(context.Background(), plan, now)
	if err != nil {
		t.Fatalf("EnsurePlanned() error = %v", err)
	}
	second, err := service.EnsurePlanned(context.Background(), plan, now.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("EnsurePlanned() second error = %v", err)
	}

	if !first.Created {
		t.Fatal("first.Created = false, want true")
	}
	if second.Created {
		t.Fatal("second.Created = true, want false")
	}
	if first.Job.ID != second.Job.ID {
		t.Fatalf("job id mismatch: %s != %s", first.Job.ID, second.Job.ID)
	}
}

func TestPostgresConcurrentClaimDoesNotDuplicateJobs(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	service := mustService(t, store, now)

	for idx := 0; idx < 10; idx++ {
		_, err := service.Schedule(context.Background(), commandjobs.ScheduleInput{
			TenantID:       testTenantID,
			ProjectID:      testProjectID,
			EnvironmentID:  testEnvironmentID,
			JobType:        "discovery_scan",
			AggregateType:  "environment",
			AggregateID:    fmt.Sprintf("00000000-0000-0000-0000-%012d", idx+1),
			IdempotencyKey: fmt.Sprintf("discover:%02d", idx),
			Now:            now,
		})
		if err != nil {
			t.Fatalf("Schedule(%d) error = %v", idx, err)
		}
	}

	type result struct {
		items []commandjobs.ClaimedJob
		err   error
	}

	ch := make(chan result, 4)
	var wg sync.WaitGroup
	for idx := 0; idx < 4; idx++ {
		wg.Add(1)
		go func(workerID string) {
			defer wg.Done()
			items, err := service.Claim(context.Background(), commandjobs.ClaimParams{
				WorkerID: workerID,
				MaxJobs:  10,
				LeaseTTL: time.Minute,
				Now:      now,
			})
			ch <- result{items: items, err: err}
		}(fmt.Sprintf("worker-%d", idx))
	}

	wg.Wait()
	close(ch)

	seen := make(map[string]string)
	total := 0
	for item := range ch {
		if item.err != nil {
			t.Fatalf("Claim() error = %v", item.err)
		}
		for _, claimed := range item.items {
			total++
			if owner, exists := seen[claimed.Job.ID]; exists {
				t.Fatalf("job %s claimed twice: %s and %s", claimed.Job.ID, owner, claimed.Job.LeaseOwner)
			}
			seen[claimed.Job.ID] = claimed.Job.LeaseOwner
		}
	}

	if total != 10 {
		t.Fatalf("total claimed = %d, want 10", total)
	}
}

func TestPostgresReapExpiredLeasePreservesAttemptHistory(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	service := mustService(t, store, now)

	record, err := service.Schedule(context.Background(), commandjobs.ScheduleInput{
		TenantID:       testTenantID,
		ProjectID:      testProjectID,
		EnvironmentID:  testEnvironmentID,
		JobType:        "deploy_nginx",
		AggregateType:  "environment",
		AggregateID:    testEnvironmentID,
		IdempotencyKey: "deploy:env-1",
		MaxAttempts:    2,
		Now:            now,
	})
	if err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}

	claims, err := service.Claim(context.Background(), commandjobs.ClaimParams{
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
	if _, err := service.MarkRunning(context.Background(), commandjobs.MarkRunningParams{
		JobID:    record.ID,
		WorkerID: "worker-a",
		Now:      now.Add(time.Second),
	}); err != nil {
		t.Fatalf("MarkRunning() error = %v", err)
	}

	reaped, err := service.ReapExpired(context.Background(), 10, now.Add(12*time.Second))
	if err != nil {
		t.Fatalf("ReapExpired() error = %v", err)
	}
	if len(reaped) != 1 {
		t.Fatalf("len(reaped) = %d, want 1", len(reaped))
	}

	if !reaped[0].RetryScheduled {
		t.Fatal("RetryScheduled = false, want true")
	}
	if reaped[0].Job.Status != job.StatusRetry {
		t.Fatalf("Job.Status = %q, want %q", reaped[0].Job.Status, job.StatusRetry)
	}
	if reaped[0].Attempt.ResultStatus != job.AttemptStatusTimedOut {
		t.Fatalf("Attempt.ResultStatus = %q, want %q", reaped[0].Attempt.ResultStatus, job.AttemptStatusTimedOut)
	}

	attempts, err := service.Attempts(context.Background(), record.ID)
	if err != nil {
		t.Fatalf("Attempts() error = %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("len(attempts) = %d, want 1", len(attempts))
	}
	if attempts[0].ErrorCode != "lease_expired" {
		t.Fatalf("attempt error code = %q, want %q", attempts[0].ErrorCode, "lease_expired")
	}
}

func newTestStore(t *testing.T) (*Store, func()) {
	t.Helper()

	dsn := os.Getenv("AUTOCERTX_POSTGRES_URL")
	if dsn == "" {
		t.Skip("AUTOCERTX_POSTGRES_URL not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(16)
	db.SetConnMaxLifetime(2 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("PingContext() error = %v", err)
	}

	resetSchedulerTables(t, db)
	insertScopeFixtures(t, db)

	store, err := New(db)
	if err != nil {
		_ = db.Close()
		t.Fatalf("New() error = %v", err)
	}

	return store, func() {
		_ = db.Close()
	}
}

func resetSchedulerTables(t *testing.T, db *sql.DB) {
	t.Helper()

	statements := []string{
		"DELETE FROM job_attempts",
		"DELETE FROM jobs",
		"DELETE FROM environments",
		"DELETE FROM projects",
		"DELETE FROM tenants",
	}

	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("reset statement %q error = %v", statement, err)
		}
	}
}

func insertScopeFixtures(t *testing.T, db *sql.DB) {
	t.Helper()

	if _, err := db.Exec(`
INSERT INTO tenants (id, name, code, status)
VALUES ($1, 'Tenant 1', 'tenant-1', 'active')`,
		testTenantID,
	); err != nil {
		t.Fatalf("insert tenant error = %v", err)
	}

	if _, err := db.Exec(`
INSERT INTO projects (id, tenant_id, name, code, status)
VALUES ($1, $2, 'Project 1', 'project-1', 'active')`,
		testProjectID,
		testTenantID,
	); err != nil {
		t.Fatalf("insert project error = %v", err)
	}

	if _, err := db.Exec(`
INSERT INTO environments (id, tenant_id, project_id, name, code, environment_type, status)
VALUES ($1, $2, $3, 'Prod', 'prod', 'prod', 'active')`,
		testEnvironmentID,
		testTenantID,
		testProjectID,
	); err != nil {
		t.Fatalf("insert environment error = %v", err)
	}
}

func mustService(t *testing.T, repo commandjobs.Repository, now time.Time) *commandjobs.Service {
	t.Helper()

	service, err := commandjobs.NewService(repo, commandjobs.Options{
		Clock: func() time.Time { return now },
		RetryPolicy: commandjobs.BackoffPolicy{
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
