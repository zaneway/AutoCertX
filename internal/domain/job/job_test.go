package job

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestNewJobDefaults(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)

	record, err := New(CreateParams{
		TenantID:       "tenant-1",
		ProjectID:      "project-1",
		EnvironmentID:  "env-1",
		JobType:        "renewal_scan",
		AggregateType:  "environment",
		AggregateID:    "env-1",
		IdempotencyKey: "plan:renewal:2026-04-20T12",
	}, now)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if record.Status != StatusPending {
		t.Fatalf("Status = %q, want %q", record.Status, StatusPending)
	}
	if record.MaxAttempts != defaultMaxAttempts {
		t.Fatalf("MaxAttempts = %d, want %d", record.MaxAttempts, defaultMaxAttempts)
	}
	if string(record.Payload) != "{}" {
		t.Fatalf("Payload = %s, want {}", record.Payload)
	}
	if record.NextRunAt != now {
		t.Fatalf("NextRunAt = %s, want %s", record.NextRunAt, now)
	}
}

func TestJobClaimRenewAndSuccess(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	record := mustJob(t, CreateParams{
		TenantID:       "tenant-1",
		ProjectID:      "project-1",
		EnvironmentID:  "env-1",
		JobType:        "deploy_nginx",
		AggregateType:  "certificate_version",
		AggregateID:    "cert-version-1",
		IdempotencyKey: "deploy:cert-version-1",
		Payload:        json.RawMessage(`{"target":"nginx-1"}`),
		MaxAttempts:    4,
	}, now)

	claimed, attempt, err := record.Claim("worker-a", 30*time.Second, now)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if claimed.Status != StatusClaimed {
		t.Fatalf("Status = %q, want %q", claimed.Status, StatusClaimed)
	}
	if attempt.AttemptNo != 1 {
		t.Fatalf("AttemptNo = %d, want 1", attempt.AttemptNo)
	}
	if attempt.ResultStatus != AttemptStatusStarted {
		t.Fatalf("ResultStatus = %q, want %q", attempt.ResultStatus, AttemptStatusStarted)
	}

	running, err := claimed.MarkRunning("worker-a", now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("MarkRunning() error = %v", err)
	}
	renewed, err := running.Renew("worker-a", 45*time.Second, now.Add(10*time.Second))
	if err != nil {
		t.Fatalf("Renew() error = %v", err)
	}
	if renewed.LeaseExpireAt != now.Add(55*time.Second) {
		t.Fatalf("LeaseExpireAt = %s, want %s", renewed.LeaseExpireAt, now.Add(55*time.Second))
	}

	succeeded, err := renewed.MarkSucceeded("worker-a", now.Add(20*time.Second))
	if err != nil {
		t.Fatalf("MarkSucceeded() error = %v", err)
	}
	if succeeded.Status != StatusSucceeded {
		t.Fatalf("Status = %q, want %q", succeeded.Status, StatusSucceeded)
	}
	if succeeded.LeaseOwner != "" {
		t.Fatalf("LeaseOwner = %q, want empty", succeeded.LeaseOwner)
	}
}

func TestJobRetryFlow(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	record := mustJob(t, CreateParams{
		TenantID:       "tenant-1",
		ProjectID:      "project-1",
		EnvironmentID:  "env-1",
		JobType:        "agent_poll",
		AggregateType:  "agent",
		AggregateID:    "agent-1",
		IdempotencyKey: "agent-poll-1",
		MaxAttempts:    3,
	}, now)

	claimed, _, err := record.Claim("worker-a", 30*time.Second, now)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	running, err := claimed.MarkRunning("worker-a", now.Add(time.Second))
	if err != nil {
		t.Fatalf("MarkRunning() error = %v", err)
	}
	failed, err := running.MarkFailed("worker-a", Failure{
		Code:    "alidns_timeout",
		Message: "temporary upstream timeout",
	}, now.Add(5*time.Second))
	if err != nil {
		t.Fatalf("MarkFailed() error = %v", err)
	}
	if failed.Status != StatusFailed {
		t.Fatalf("Status = %q, want %q", failed.Status, StatusFailed)
	}

	retried, err := failed.ScheduleRetry(2*time.Minute, now.Add(5*time.Second))
	if err != nil {
		t.Fatalf("ScheduleRetry() error = %v", err)
	}
	if retried.Status != StatusRetry {
		t.Fatalf("Status = %q, want %q", retried.Status, StatusRetry)
	}
	if retried.NextRunAt != now.Add(125*time.Second) {
		t.Fatalf("NextRunAt = %s, want %s", retried.NextRunAt, now.Add(125*time.Second))
	}
	if retried.LastErrorCode != "alidns_timeout" {
		t.Fatalf("LastErrorCode = %q, want %q", retried.LastErrorCode, "alidns_timeout")
	}
}

func TestJobExpireLeaseMapsAttemptStatus(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)

	claimedJob := mustJob(t, CreateParams{
		TenantID:       "tenant-1",
		ProjectID:      "project-1",
		EnvironmentID:  "env-1",
		JobType:        "discovery_scan",
		AggregateType:  "environment",
		AggregateID:    "env-1",
		IdempotencyKey: "discovery-plan-1",
	}, now)
	claimedJob, _, _ = claimedJob.Claim("worker-a", 10*time.Second, now)

	expiredClaimed, attemptStatus, err := claimedJob.ExpireLease(now.Add(11*time.Second), Failure{
		Code:    "worker_lost",
		Message: "worker disappeared before start",
	})
	if err != nil {
		t.Fatalf("ExpireLease() error = %v", err)
	}
	if attemptStatus != AttemptStatusAbandoned {
		t.Fatalf("attemptStatus = %q, want %q", attemptStatus, AttemptStatusAbandoned)
	}
	if expiredClaimed.Status != StatusTimedOut {
		t.Fatalf("Status = %q, want %q", expiredClaimed.Status, StatusTimedOut)
	}

	runningJob := mustJob(t, CreateParams{
		TenantID:       "tenant-1",
		ProjectID:      "project-1",
		EnvironmentID:  "env-1",
		JobType:        "discovery_scan",
		AggregateType:  "environment",
		AggregateID:    "env-1",
		IdempotencyKey: "discovery-plan-2",
	}, now)
	runningJob, _, _ = runningJob.Claim("worker-a", 10*time.Second, now)
	runningJob, _ = runningJob.MarkRunning("worker-a", now.Add(time.Second))

	_, attemptStatus, err = runningJob.ExpireLease(now.Add(11*time.Second), Failure{
		Code:    "worker_lost",
		Message: "worker disappeared during execution",
	})
	if err != nil {
		t.Fatalf("ExpireLease() error = %v", err)
	}
	if attemptStatus != AttemptStatusTimedOut {
		t.Fatalf("attemptStatus = %q, want %q", attemptStatus, AttemptStatusTimedOut)
	}
}

func TestAttemptHeartbeatAndFinish(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)

	attempt, err := NewAttempt("job-1", 1, "worker-a", now)
	if err != nil {
		t.Fatalf("NewAttempt() error = %v", err)
	}

	attempt, err = attempt.Heartbeat(now.Add(5 * time.Second))
	if err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}
	if attempt.ResultStatus != AttemptStatusHeartbeating {
		t.Fatalf("ResultStatus = %q, want %q", attempt.ResultStatus, AttemptStatusHeartbeating)
	}

	attempt, err = attempt.MarkSucceeded(now.Add(10*time.Second), "evidence://job-1")
	if err != nil {
		t.Fatalf("MarkSucceeded() error = %v", err)
	}
	if attempt.ResultStatus != AttemptStatusSucceeded {
		t.Fatalf("ResultStatus = %q, want %q", attempt.ResultStatus, AttemptStatusSucceeded)
	}

	_, err = attempt.Heartbeat(now.Add(15 * time.Second))
	if !errors.Is(err, ErrAttemptFinished) {
		t.Fatalf("Heartbeat() error = %v, want %v", err, ErrAttemptFinished)
	}
}

func TestScheduleRetryStopsAtMaxAttempts(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	record := mustJob(t, CreateParams{
		TenantID:       "tenant-1",
		ProjectID:      "project-1",
		EnvironmentID:  "env-1",
		JobType:        "renewal_scan",
		AggregateType:  "environment",
		AggregateID:    "env-1",
		IdempotencyKey: "renewal:env-1",
		MaxAttempts:    1,
	}, now)

	claimed, _, err := record.Claim("worker-a", 10*time.Second, now)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	running, err := claimed.MarkRunning("worker-a", now.Add(time.Second))
	if err != nil {
		t.Fatalf("MarkRunning() error = %v", err)
	}
	timedOut, err := running.MarkTimedOut("worker-a", Failure{
		Code:    "timeout",
		Message: "execution timeout",
	}, now.Add(3*time.Second))
	if err != nil {
		t.Fatalf("MarkTimedOut() error = %v", err)
	}

	_, err = timedOut.ScheduleRetry(time.Minute, now.Add(4*time.Second))
	if !errors.Is(err, ErrJobNotRetryable) {
		t.Fatalf("ScheduleRetry() error = %v, want %v", err, ErrJobNotRetryable)
	}
}

func mustJob(t *testing.T, params CreateParams, now time.Time) Job {
	t.Helper()

	record, err := New(params, now)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	return record
}
