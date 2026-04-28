package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zaneway/AutoCertX/internal/domain/job"
	"github.com/zaneway/AutoCertX/internal/domain/resource"
)

// BackoffPolicy defines how long a retry should be delayed.
type BackoffPolicy struct {
	Initial    time.Duration
	Max        time.Duration
	Multiplier float64
}

// Delay returns the retry delay for a job after the given attempt count.
func (p BackoffPolicy) Delay(attemptCount int) time.Duration {
	initial := p.Initial
	if initial <= 0 {
		initial = 5 * time.Second
	}

	maxDelay := p.Max
	if maxDelay <= 0 {
		maxDelay = 5 * time.Minute
	}

	multiplier := p.Multiplier
	if multiplier < 1 {
		multiplier = 2
	}

	delay := float64(initial)
	for i := 1; i < attemptCount; i++ {
		delay *= multiplier
		if time.Duration(delay) >= maxDelay {
			return maxDelay
		}
	}

	computed := time.Duration(delay)
	if computed > maxDelay {
		return maxDelay
	}

	return computed
}

// DefaultBackoffPolicy returns the scheduler backoff baseline used by T05.
func DefaultBackoffPolicy() BackoffPolicy {
	return BackoffPolicy{
		Initial:    5 * time.Second,
		Max:        5 * time.Minute,
		Multiplier: 2,
	}
}

// Repository defines the durable job command contract used by the scheduler.
type Repository interface {
	Create(context.Context, job.Job) error
	EnsurePlanned(context.Context, job.Job) (PlannedJob, error)
	Claim(context.Context, ClaimParams) ([]ClaimedJob, error)
	MarkRunning(context.Context, MarkRunningParams) (job.Job, error)
	Heartbeat(context.Context, HeartbeatParams) (HeartbeatResult, error)
	Complete(context.Context, CompleteParams) (CompletionResult, error)
	Cancel(context.Context, CancelParams) (job.Job, error)
	ReapExpired(context.Context, ReapParams) ([]ReapedLease, error)
	List(context.Context, resource.Scope) ([]job.Job, error)
	Get(context.Context, string) (job.Job, error)
	GetByIdempotency(context.Context, string) (job.Job, error)
	Attempts(context.Context, string) ([]job.Attempt, error)
}

// Options tunes the command service behavior.
type Options struct {
	Clock       func() time.Time
	RetryPolicy BackoffPolicy
}

// Service exposes the T05 job/lease command use cases.
type Service struct {
	repo        Repository
	clock       func() time.Time
	retryPolicy BackoffPolicy
}

// ScheduleInput describes a newly created async job.
type ScheduleInput struct {
	TenantID       string
	ProjectID      string
	EnvironmentID  string
	JobType        string
	AggregateType  string
	AggregateID    string
	Priority       int
	Payload        json.RawMessage
	IdempotencyKey string
	MaxAttempts    int
	NextRunAt      time.Time
	Now            time.Time
}

// PlanDefinition describes one periodic baseline plan.
type PlanDefinition struct {
	Name          string
	TenantID      string
	ProjectID     string
	EnvironmentID string
	JobType       string
	AggregateType string
	AggregateID   string
	Priority      int
	Payload       json.RawMessage
	MaxAttempts   int
	Interval      time.Duration
}

// PlannedJob describes the result of ensuring a planned job exists.
type PlannedJob struct {
	Job     job.Job
	Created bool
}

// ClaimParams defines how a worker claims schedulable jobs.
type ClaimParams struct {
	WorkerID string
	MaxJobs  int
	LeaseTTL time.Duration
	Now      time.Time
}

// ClaimedJob is the atomic result of claiming one job and creating one attempt.
type ClaimedJob struct {
	Job     job.Job
	Attempt job.Attempt
}

// MarkRunningParams transitions a claimed job into active execution.
type MarkRunningParams struct {
	JobID    string
	WorkerID string
	Now      time.Time
}

// HeartbeatParams renews the lease and marks the latest attempt as heartbeating.
type HeartbeatParams struct {
	JobID    string
	WorkerID string
	LeaseTTL time.Duration
	Now      time.Time
}

// HeartbeatResult carries the updated job and current attempt after renewal.
type HeartbeatResult struct {
	Job     job.Job
	Attempt job.Attempt
}

// CompleteParams finalizes one claimed/running job attempt.
type CompleteParams struct {
	JobID        string
	WorkerID     string
	ResultStatus job.AttemptStatus
	Retryable    bool
	RetryDelay   time.Duration
	ErrorCode    string
	ErrorMessage string
	EvidenceRef  string
	Now          time.Time
}

// CompletionResult is the stored outcome of finishing one job attempt.
type CompletionResult struct {
	Job            job.Job
	Attempt        job.Attempt
	RetryScheduled bool
}

// CancelParams stops a job.
type CancelParams struct {
	JobID string
	Now   time.Time
}

// ReapParams recovers expired leases from crashed workers.
type ReapParams struct {
	Now         time.Time
	Limit       int
	RetryPolicy BackoffPolicy
}

// ReapedLease is the result of one recovered expired lease.
type ReapedLease struct {
	Job            job.Job
	Attempt        job.Attempt
	RetryScheduled bool
}

// NewService builds the T05 job command service.
func NewService(repo Repository, opts Options) (*Service, error) {
	if repo == nil {
		return nil, fmt.Errorf("jobs repository required")
	}

	clock := opts.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}

	policy := opts.RetryPolicy
	if policy == (BackoffPolicy{}) {
		policy = DefaultBackoffPolicy()
	}

	return &Service{
		repo:        repo,
		clock:       clock,
		retryPolicy: policy,
	}, nil
}

// Schedule creates a new pending job.
func (s *Service) Schedule(ctx context.Context, input ScheduleInput) (job.Job, error) {
	record, err := job.New(job.CreateParams{
		TenantID:       input.TenantID,
		ProjectID:      input.ProjectID,
		EnvironmentID:  input.EnvironmentID,
		JobType:        input.JobType,
		AggregateType:  input.AggregateType,
		AggregateID:    input.AggregateID,
		Priority:       input.Priority,
		Payload:        input.Payload,
		IdempotencyKey: input.IdempotencyKey,
		MaxAttempts:    input.MaxAttempts,
		NextRunAt:      coalesceTime(input.NextRunAt, s.clock()),
	}, coalesceTime(input.Now, s.clock()))
	if err != nil {
		return job.Job{}, err
	}

	if err := s.repo.Create(ctx, record); err != nil {
		return job.Job{}, err
	}

	return record, nil
}

// EnsurePlanned ensures one plan slot exists exactly once by idempotency key.
func (s *Service) EnsurePlanned(ctx context.Context, plan PlanDefinition, now time.Time) (PlannedJob, error) {
	slot, key, err := plan.slot(coalesceTime(now, s.clock()))
	if err != nil {
		return PlannedJob{}, err
	}

	record, err := job.New(job.CreateParams{
		TenantID:       plan.TenantID,
		ProjectID:      plan.ProjectID,
		EnvironmentID:  plan.EnvironmentID,
		JobType:        plan.JobType,
		AggregateType:  plan.AggregateType,
		AggregateID:    plan.AggregateID,
		Priority:       plan.Priority,
		Payload:        plan.Payload,
		IdempotencyKey: key,
		MaxAttempts:    plan.MaxAttempts,
		NextRunAt:      slot,
	}, coalesceTime(now, s.clock()))
	if err != nil {
		return PlannedJob{}, err
	}

	return s.repo.EnsurePlanned(ctx, record)
}

// EnsurePlans ensures a set of recurring baseline jobs.
func (s *Service) EnsurePlans(ctx context.Context, plans []PlanDefinition, now time.Time) ([]PlannedJob, error) {
	results := make([]PlannedJob, 0, len(plans))
	tickAt := coalesceTime(now, s.clock())

	for _, plan := range plans {
		// Reuse one tick timestamp for the whole planner pass so every recurring
		// definition maps into the same deterministic slot boundary.
		result, err := s.EnsurePlanned(ctx, plan, tickAt)
		if err != nil {
			return nil, err
		}

		results = append(results, result)
	}

	return results, nil
}

// Claim atomically claims due jobs for one worker.
func (s *Service) Claim(ctx context.Context, params ClaimParams) ([]ClaimedJob, error) {
	params.Now = coalesceTime(params.Now, s.clock())
	return s.repo.Claim(ctx, params)
}

// MarkRunning transitions a claimed job into running state.
func (s *Service) MarkRunning(ctx context.Context, params MarkRunningParams) (job.Job, error) {
	params.Now = coalesceTime(params.Now, s.clock())
	return s.repo.MarkRunning(ctx, params)
}

// Heartbeat renews the lease and persists attempt heartbeats.
func (s *Service) Heartbeat(ctx context.Context, params HeartbeatParams) (HeartbeatResult, error) {
	params.Now = coalesceTime(params.Now, s.clock())
	return s.repo.Heartbeat(ctx, params)
}

// Complete finalizes a job and optionally reschedules it according to retry policy.
func (s *Service) Complete(ctx context.Context, params CompleteParams) (CompletionResult, error) {
	params.Now = coalesceTime(params.Now, s.clock())
	if params.Retryable && params.RetryDelay == 0 {
		// Executors can mark a failure retryable without knowing the platform
		// policy; the command layer computes the default exponential backoff.
		record, err := s.repo.Get(ctx, params.JobID)
		if err != nil {
			return CompletionResult{}, err
		}
		params.RetryDelay = s.retryPolicy.Delay(record.AttemptCount)
	}

	return s.repo.Complete(ctx, params)
}

// Cancel stops a job.
func (s *Service) Cancel(ctx context.Context, jobID string) (job.Job, error) {
	return s.repo.Cancel(ctx, CancelParams{
		JobID: jobID,
		Now:   s.clock(),
	})
}

// ReapExpired recovers jobs whose worker lease has already expired.
func (s *Service) ReapExpired(ctx context.Context, limit int, now time.Time) ([]ReapedLease, error) {
	return s.repo.ReapExpired(ctx, ReapParams{
		Now:         coalesceTime(now, s.clock()),
		Limit:       limit,
		RetryPolicy: s.retryPolicy,
	})
}

// Get returns one job by id.
func (s *Service) Get(ctx context.Context, jobID string) (job.Job, error) {
	return s.repo.Get(ctx, jobID)
}

// GetByIdempotency returns one job by idempotency key.
func (s *Service) GetByIdempotency(ctx context.Context, idempotencyKey string) (job.Job, error) {
	return s.repo.GetByIdempotency(ctx, idempotencyKey)
}

// Attempts returns the execution history of one job.
func (s *Service) Attempts(ctx context.Context, jobID string) ([]job.Attempt, error) {
	return s.repo.Attempts(ctx, jobID)
}

func (p PlanDefinition) slot(now time.Time) (time.Time, string, error) {
	if strings.TrimSpace(p.Name) == "" {
		return time.Time{}, "", fmt.Errorf("%w: plan name required", job.ErrInvalidJob)
	}
	if p.Interval <= 0 {
		return time.Time{}, "", fmt.Errorf("%w: plan interval must be positive", job.ErrInvalidJob)
	}

	now = coalesceTime(now, time.Now().UTC())
	// The slot timestamp and idempotency key are both derived from the truncated
	// interval so reruns within the same window cannot create duplicates.
	slot := now.Truncate(p.Interval)
	key := fmt.Sprintf("plan:%s:%s", strings.TrimSpace(p.Name), slot.Format(time.RFC3339))
	return slot, key, nil
}

func coalesceTime(ts time.Time, fallback time.Time) time.Time {
	if ts.IsZero() {
		return fallback.UTC()
	}

	return ts.UTC()
}
