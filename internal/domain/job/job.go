package job

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const defaultMaxAttempts = 3

// Failure carries the last known execution failure information.
type Failure struct {
	Code    string
	Message string
}

// CreateParams contains the immutable facts required to create a job.
type CreateParams struct {
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
}

// Job is the hot scheduling record used by claim/lease/retry operations.
type Job struct {
	ID               string
	TenantID         string
	ProjectID        string
	EnvironmentID    string
	JobType          string
	AggregateType    string
	AggregateID      string
	Status           Status
	Priority         int
	Payload          json.RawMessage
	IdempotencyKey   string
	LeaseOwner       string
	LeaseExpireAt    time.Time
	AttemptCount     int
	MaxAttempts      int
	NextRunAt        time.Time
	LastErrorCode    string
	LastErrorMessage string
	Version          int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// New creates a new pending job.
func New(params CreateParams, now time.Time) (Job, error) {
	now = normalizeTime(now)
	nextRunAt := normalizeTime(params.NextRunAt)
	if nextRunAt.IsZero() {
		nextRunAt = now
	}

	if strings.TrimSpace(params.TenantID) == "" {
		return Job{}, fmt.Errorf("%w: tenant id required", ErrInvalidJob)
	}
	if strings.TrimSpace(params.ProjectID) == "" {
		return Job{}, fmt.Errorf("%w: project id required", ErrInvalidJob)
	}
	if strings.TrimSpace(params.EnvironmentID) == "" {
		return Job{}, fmt.Errorf("%w: environment id required", ErrInvalidJob)
	}
	if strings.TrimSpace(params.JobType) == "" {
		return Job{}, fmt.Errorf("%w: job type required", ErrInvalidJob)
	}
	if strings.TrimSpace(params.AggregateType) == "" {
		return Job{}, fmt.Errorf("%w: aggregate type required", ErrInvalidJob)
	}
	if strings.TrimSpace(params.AggregateID) == "" {
		return Job{}, fmt.Errorf("%w: aggregate id required", ErrInvalidJob)
	}
	if strings.TrimSpace(params.IdempotencyKey) == "" {
		return Job{}, fmt.Errorf("%w: idempotency key required", ErrInvalidJob)
	}

	maxAttempts := params.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxAttempts
	}

	id, err := generateID()
	if err != nil {
		return Job{}, fmt.Errorf("generate job id: %w", err)
	}

	return Job{
		ID:             id,
		TenantID:       strings.TrimSpace(params.TenantID),
		ProjectID:      strings.TrimSpace(params.ProjectID),
		EnvironmentID:  strings.TrimSpace(params.EnvironmentID),
		JobType:        strings.TrimSpace(params.JobType),
		AggregateType:  strings.TrimSpace(params.AggregateType),
		AggregateID:    strings.TrimSpace(params.AggregateID),
		Status:         StatusPending,
		Priority:       params.Priority,
		Payload:        clonePayload(params.Payload),
		IdempotencyKey: strings.TrimSpace(params.IdempotencyKey),
		MaxAttempts:    maxAttempts,
		NextRunAt:      nextRunAt,
		Version:        1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

// Due reports whether the job can be claimed at the given time.
func (j Job) Due(now time.Time) bool {
	now = normalizeTime(now)
	if now.IsZero() {
		return false
	}

	return !now.Before(j.NextRunAt)
}

// LeaseActive reports whether the logical lease is still valid.
func (j Job) LeaseActive(now time.Time) bool {
	now = normalizeTime(now)
	return j.LeaseOwner != "" && !j.LeaseExpireAt.IsZero() && j.LeaseExpireAt.After(now)
}

// LeaseExpired reports whether a held lease has already expired.
func (j Job) LeaseExpired(now time.Time) bool {
	now = normalizeTime(now)
	return j.LeaseOwner != "" && !j.LeaseExpireAt.IsZero() && !j.LeaseExpireAt.After(now)
}

// CanRetry reports whether the job still has retry budget.
func (j Job) CanRetry() bool {
	return j.AttemptCount < j.MaxAttempts
}

// Claim moves a schedulable job into claimed state and creates a new attempt.
func (j Job) Claim(owner string, leaseTTL time.Duration, now time.Time) (Job, Attempt, error) {
	now = normalizeTime(now)
	owner = strings.TrimSpace(owner)

	if owner == "" {
		return Job{}, Attempt{}, fmt.Errorf("%w: lease owner required", ErrInvalidJob)
	}
	if leaseTTL <= 0 {
		return Job{}, Attempt{}, fmt.Errorf("%w: lease ttl must be positive", ErrInvalidJob)
	}
	if !j.Status.CanClaim() || !j.Due(now) {
		return Job{}, Attempt{}, ErrJobNotSchedulable
	}
	if j.LeaseActive(now) {
		return Job{}, Attempt{}, ErrLeaseNotHeld
	}

	next := j
	next.Status = StatusClaimed
	next.LeaseOwner = owner
	next.LeaseExpireAt = now.Add(leaseTTL)
	next.AttemptCount++
	next.Version++
	next.UpdatedAt = now

	attempt, err := NewAttempt(next.ID, next.AttemptCount, owner, now)
	if err != nil {
		return Job{}, Attempt{}, err
	}

	return next, attempt, nil
}

// MarkRunning confirms that a claimed job has entered active execution.
func (j Job) MarkRunning(owner string, now time.Time) (Job, error) {
	if err := j.requireLeaseOwner(owner, normalizeTime(now)); err != nil {
		return Job{}, err
	}
	if j.Status != StatusClaimed {
		return Job{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, j.Status, StatusRunning)
	}

	next := j
	next.Status = StatusRunning
	next.Version++
	next.UpdatedAt = normalizeTime(now)
	return next, nil
}

// Renew extends the lease of a claimed or running job.
func (j Job) Renew(owner string, leaseTTL time.Duration, now time.Time) (Job, error) {
	now = normalizeTime(now)
	if leaseTTL <= 0 {
		return Job{}, fmt.Errorf("%w: lease ttl must be positive", ErrInvalidJob)
	}
	if !j.Status.IsLeased() {
		return Job{}, fmt.Errorf("%w: %s cannot be renewed", ErrInvalidTransition, j.Status)
	}
	if err := j.requireLeaseOwner(owner, now); err != nil {
		return Job{}, err
	}

	next := j
	next.LeaseExpireAt = now.Add(leaseTTL)
	next.Version++
	next.UpdatedAt = now
	return next, nil
}

// MarkSucceeded finishes a job successfully and releases the lease.
func (j Job) MarkSucceeded(owner string, now time.Time) (Job, error) {
	now = normalizeTime(now)
	if err := j.requireActiveExecution(owner, now); err != nil {
		return Job{}, err
	}

	next := j
	next.Status = StatusSucceeded
	next.LastErrorCode = ""
	next.LastErrorMessage = ""
	next.clearLease()
	next.Version++
	next.UpdatedAt = now
	return next, nil
}

// MarkFailed records a failed execution and releases the lease.
func (j Job) MarkFailed(owner string, failure Failure, now time.Time) (Job, error) {
	now = normalizeTime(now)
	if err := j.requireActiveExecution(owner, now); err != nil {
		return Job{}, err
	}

	next := j
	next.Status = StatusFailed
	next.applyFailure(failure)
	next.clearLease()
	next.Version++
	next.UpdatedAt = now
	return next, nil
}

// MarkTimedOut records a timed out execution and releases the lease.
func (j Job) MarkTimedOut(owner string, failure Failure, now time.Time) (Job, error) {
	now = normalizeTime(now)
	if err := j.requireActiveExecution(owner, now); err != nil {
		return Job{}, err
	}

	next := j
	next.Status = StatusTimedOut
	next.applyFailure(failure)
	next.clearLease()
	next.Version++
	next.UpdatedAt = now
	return next, nil
}

// ExpireLease is used by the reaper when the worker disappears before completion.
func (j Job) ExpireLease(now time.Time, failure Failure) (Job, AttemptStatus, error) {
	now = normalizeTime(now)
	if !j.Status.IsLeased() {
		return Job{}, "", fmt.Errorf("%w: %s cannot expire lease", ErrInvalidTransition, j.Status)
	}
	if !j.LeaseExpired(now) {
		return Job{}, "", ErrLeaseExpired
	}

	resultStatus := AttemptStatusTimedOut
	if j.Status == StatusClaimed {
		resultStatus = AttemptStatusAbandoned
	}

	next := j
	next.Status = StatusTimedOut
	next.applyFailure(failure)
	next.clearLease()
	next.Version++
	next.UpdatedAt = now
	return next, resultStatus, nil
}

// ScheduleRetry moves a failed or timed out job back to the retry queue.
func (j Job) ScheduleRetry(delay time.Duration, now time.Time) (Job, error) {
	now = normalizeTime(now)
	if delay < 0 {
		return Job{}, fmt.Errorf("%w: retry delay must not be negative", ErrInvalidJob)
	}
	if j.Status != StatusFailed && j.Status != StatusTimedOut {
		return Job{}, fmt.Errorf("%w: %s cannot enter retry", ErrInvalidTransition, j.Status)
	}
	if !j.CanRetry() {
		return Job{}, ErrJobNotRetryable
	}

	next := j
	next.Status = StatusRetry
	next.NextRunAt = now.Add(delay)
	next.clearLease()
	next.Version++
	next.UpdatedAt = now
	return next, nil
}

// Cancel stops a pending, claimed, running or retry job.
func (j Job) Cancel(now time.Time) (Job, error) {
	now = normalizeTime(now)
	if !j.Status.CanCancel() {
		return Job{}, fmt.Errorf("%w: %s cannot be cancelled", ErrInvalidTransition, j.Status)
	}

	next := j
	next.Status = StatusCancelled
	next.clearLease()
	next.Version++
	next.UpdatedAt = now
	return next, nil
}

func (j Job) requireActiveExecution(owner string, now time.Time) error {
	if !j.Status.IsLeased() {
		return fmt.Errorf("%w: %s is not actively leased", ErrInvalidTransition, j.Status)
	}
	return j.requireLeaseOwner(owner, now)
}

func (j Job) requireLeaseOwner(owner string, now time.Time) error {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return fmt.Errorf("%w: lease owner required", ErrInvalidJob)
	}
	if j.LeaseOwner == "" || j.LeaseExpireAt.IsZero() {
		return ErrLeaseNotHeld
	}
	if j.LeaseOwner != owner {
		return ErrLeaseOwnerMismatch
	}
	if !j.LeaseExpireAt.After(now) {
		return ErrLeaseExpired
	}

	return nil
}

func (j *Job) clearLease() {
	j.LeaseOwner = ""
	j.LeaseExpireAt = time.Time{}
}

func (j *Job) applyFailure(failure Failure) {
	j.LastErrorCode = strings.TrimSpace(failure.Code)
	j.LastErrorMessage = strings.TrimSpace(failure.Message)
}

func clonePayload(payload json.RawMessage) json.RawMessage {
	if len(payload) == 0 {
		return json.RawMessage(`{}`)
	}

	cloned := make(json.RawMessage, len(payload))
	copy(cloned, payload)
	return cloned
}

func normalizeTime(ts time.Time) time.Time {
	if ts.IsZero() {
		return time.Time{}
	}

	return ts.UTC()
}

func generateID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}

	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80

	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		raw[0:4],
		raw[4:6],
		raw[6:8],
		raw[8:10],
		raw[10:16],
	), nil
}
