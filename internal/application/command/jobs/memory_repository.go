package jobs

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zaneway/AutoCertX/internal/domain/job"
	"github.com/zaneway/AutoCertX/internal/domain/resource"
)

// MemoryRepository provides an in-process reference implementation for T05 tests.
type MemoryRepository struct {
	mu          sync.Mutex
	jobs        map[string]job.Job
	attempts    map[string][]job.Attempt
	idempotency map[string]string
}

// NewMemoryRepository creates an empty in-memory job store.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		jobs:        make(map[string]job.Job),
		attempts:    make(map[string][]job.Attempt),
		idempotency: make(map[string]string),
	}
}

// Jobs returns a point-in-time copy used by tests.
func (r *MemoryRepository) Jobs() []job.Job {
	r.mu.Lock()
	defer r.mu.Unlock()

	items := make([]job.Job, 0, len(r.jobs))
	for _, record := range r.jobs {
		items = append(items, cloneJob(record))
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items
}

func (r *MemoryRepository) Create(_ context.Context, record job.Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// The idempotency index enforces the same duplicate-submission semantics as
	// the production repository used by the scheduler.
	if existingID, exists := r.idempotency[record.IdempotencyKey]; exists {
		if existingID != "" {
			return job.ErrDuplicateJob
		}
	}

	r.jobs[record.ID] = cloneJob(record)
	r.idempotency[record.IdempotencyKey] = record.ID
	return nil
}

func (r *MemoryRepository) EnsurePlanned(_ context.Context, record job.Job) (PlannedJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Planned jobs are keyed by slot-derived idempotency key so rerunning the
	// planner in the same slot returns the existing record instead of a duplicate.
	if existingID, exists := r.idempotency[record.IdempotencyKey]; exists {
		existing := r.jobs[existingID]
		return PlannedJob{Job: cloneJob(existing), Created: false}, nil
	}

	r.jobs[record.ID] = cloneJob(record)
	r.idempotency[record.IdempotencyKey] = record.ID
	return PlannedJob{Job: cloneJob(record), Created: true}, nil
}

func (r *MemoryRepository) Claim(_ context.Context, params ClaimParams) ([]ClaimedJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if strings.TrimSpace(params.WorkerID) == "" {
		return nil, fmt.Errorf("%w: worker id required", job.ErrInvalidJob)
	}
	if params.MaxJobs <= 0 {
		params.MaxJobs = 1
	}

	ids := r.claimableIDsLocked(params.Now)
	results := make([]ClaimedJob, 0, min(params.MaxJobs, len(ids)))

	for _, id := range ids {
		if len(results) >= params.MaxJobs {
			break
		}

		record := r.jobs[id]
		next, attempt, err := record.Claim(params.WorkerID, params.LeaseTTL, params.Now)
		if err != nil {
			continue
		}

		// Claim updates the durable job state and appends the first attempt record
		// atomically inside the repository lock.
		r.jobs[id] = cloneJob(next)
		r.attempts[id] = append(r.attempts[id], cloneAttempt(attempt))
		results = append(results, ClaimedJob{
			Job:     cloneJob(next),
			Attempt: cloneAttempt(attempt),
		})
	}

	return results, nil
}

func (r *MemoryRepository) MarkRunning(_ context.Context, params MarkRunningParams) (job.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	record, err := r.requireJobLocked(params.JobID)
	if err != nil {
		return job.Job{}, err
	}

	activeAttempt, _, err := r.activeAttemptLocked(record.ID, params.WorkerID)
	if err != nil {
		return job.Job{}, err
	}
	_ = activeAttempt

	next, err := record.MarkRunning(params.WorkerID, params.Now)
	if err != nil {
		return job.Job{}, err
	}

	r.jobs[record.ID] = cloneJob(next)
	return cloneJob(next), nil
}

func (r *MemoryRepository) Heartbeat(_ context.Context, params HeartbeatParams) (HeartbeatResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	record, err := r.requireJobLocked(params.JobID)
	if err != nil {
		return HeartbeatResult{}, err
	}

	nextJob, err := record.Renew(params.WorkerID, params.LeaseTTL, params.Now)
	if err != nil {
		return HeartbeatResult{}, err
	}

	activeAttempt, attemptIndex, err := r.activeAttemptLocked(record.ID, params.WorkerID)
	if err != nil {
		return HeartbeatResult{}, err
	}

	nextAttempt, err := activeAttempt.Heartbeat(params.Now)
	if err != nil {
		return HeartbeatResult{}, err
	}

	r.jobs[record.ID] = cloneJob(nextJob)
	r.attempts[record.ID][attemptIndex] = cloneAttempt(nextAttempt)

	return HeartbeatResult{
		Job:     cloneJob(nextJob),
		Attempt: cloneAttempt(nextAttempt),
	}, nil
}

func (r *MemoryRepository) Complete(_ context.Context, params CompleteParams) (CompletionResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	record, err := r.requireJobLocked(params.JobID)
	if err != nil {
		return CompletionResult{}, err
	}

	activeAttempt, attemptIndex, err := r.activeAttemptLocked(record.ID, params.WorkerID)
	if err != nil {
		return CompletionResult{}, err
	}

	failure := job.Failure{
		Code:    params.ErrorCode,
		Message: params.ErrorMessage,
	}

	nextJob, nextAttempt, err := completeLocked(record, activeAttempt, params, failure)
	if err != nil {
		return CompletionResult{}, err
	}

	retryScheduled := false
	if params.Retryable && nextJob.CanRetry() && nextJob.Status != job.StatusSucceeded {
		// Retry scheduling mirrors production behavior: executor intent plus job
		// retry budget determines whether the job re-enters the queue.
		retried, retryErr := nextJob.ScheduleRetry(params.RetryDelay, params.Now)
		if retryErr == nil {
			nextJob = retried
			retryScheduled = true
		}
	}

	r.jobs[record.ID] = cloneJob(nextJob)
	r.attempts[record.ID][attemptIndex] = cloneAttempt(nextAttempt)

	return CompletionResult{
		Job:            cloneJob(nextJob),
		Attempt:        cloneAttempt(nextAttempt),
		RetryScheduled: retryScheduled,
	}, nil
}

func (r *MemoryRepository) Cancel(_ context.Context, params CancelParams) (job.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	record, err := r.requireJobLocked(params.JobID)
	if err != nil {
		return job.Job{}, err
	}

	if activeAttempt, attemptIndex, err := r.activeAttemptLocked(record.ID, record.LeaseOwner); err == nil {
		cancelledAttempt, finishErr := activeAttempt.MarkAbandoned(params.Now, job.Failure{
			Code:    "cancelled",
			Message: "job cancelled",
		})
		if finishErr == nil {
			r.attempts[record.ID][attemptIndex] = cloneAttempt(cancelledAttempt)
		}
	}

	next, err := record.Cancel(params.Now)
	if err != nil {
		return job.Job{}, err
	}

	r.jobs[record.ID] = cloneJob(next)
	return cloneJob(next), nil
}

func (r *MemoryRepository) ReapExpired(_ context.Context, params ReapParams) ([]ReapedLease, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ids := r.expiredLeaseIDsLocked(params.Now)
	if params.Limit > 0 && len(ids) > params.Limit {
		ids = ids[:params.Limit]
	}

	results := make([]ReapedLease, 0, len(ids))
	for _, id := range ids {
		record := r.jobs[id]
		activeAttempt, attemptIndex, err := r.activeAttemptLocked(record.ID, record.LeaseOwner)
		if err != nil {
			return nil, err
		}

		expiredJob, attemptStatus, err := record.ExpireLease(params.Now, job.Failure{
			Code:    "lease_expired",
			Message: "worker lease expired before completion",
		})
		if err != nil {
			return nil, err
		}

		var nextAttempt job.Attempt
		switch attemptStatus {
		case job.AttemptStatusAbandoned:
			// Jobs that never made it to running are marked abandoned to distinguish
			// lease loss before execution from timeouts during execution.
			nextAttempt, err = activeAttempt.MarkAbandoned(params.Now, job.Failure{
				Code:    "lease_expired",
				Message: "worker lost before execution started",
			})
		default:
			nextAttempt, err = activeAttempt.MarkTimedOut(params.Now, job.Failure{
				Code:    "lease_expired",
				Message: "worker lost during execution",
			})
		}
		if err != nil {
			return nil, err
		}

		retryScheduled := false
		if expiredJob.CanRetry() {
			// Reaped jobs use the shared retry policy so crash recovery and normal
			// executor failures back off consistently.
			retried, retryErr := expiredJob.ScheduleRetry(params.RetryPolicy.Delay(expiredJob.AttemptCount), params.Now)
			if retryErr == nil {
				expiredJob = retried
				retryScheduled = true
			}
		}

		r.jobs[id] = cloneJob(expiredJob)
		r.attempts[id][attemptIndex] = cloneAttempt(nextAttempt)
		results = append(results, ReapedLease{
			Job:            cloneJob(expiredJob),
			Attempt:        cloneAttempt(nextAttempt),
			RetryScheduled: retryScheduled,
		})
	}

	return results, nil
}

// List returns the point-in-time scheduler facts visible under one environment scope.
func (r *MemoryRepository) List(_ context.Context, scope resource.Scope) ([]job.Job, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	items := make([]job.Job, 0, len(r.jobs))
	for _, record := range r.jobs {
		if record.TenantID != scope.TenantID || record.ProjectID != scope.ProjectID || record.EnvironmentID != scope.EnvironmentID {
			continue
		}
		items = append(items, cloneJob(record))
	}

	sort.Slice(items, func(i, j int) bool {
		if !items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].CreatedAt.Before(items[j].CreatedAt)
		}
		return items[i].ID < items[j].ID
	})
	return items, nil
}

func (r *MemoryRepository) Get(_ context.Context, jobID string) (job.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	record, err := r.requireJobLocked(jobID)
	if err != nil {
		return job.Job{}, err
	}

	return cloneJob(record), nil
}

func (r *MemoryRepository) Attempts(_ context.Context, jobID string) ([]job.Attempt, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, err := r.requireJobLocked(jobID); err != nil {
		return nil, err
	}

	attempts := r.attempts[jobID]
	cloned := make([]job.Attempt, 0, len(attempts))
	for _, attempt := range attempts {
		cloned = append(cloned, cloneAttempt(attempt))
	}

	return cloned, nil
}

func completeLocked(
	record job.Job,
	activeAttempt job.Attempt,
	params CompleteParams,
	failure job.Failure,
) (job.Job, job.Attempt, error) {
	switch params.ResultStatus {
	case job.AttemptStatusSucceeded:
		nextJob, err := record.MarkSucceeded(params.WorkerID, params.Now)
		if err != nil {
			return job.Job{}, job.Attempt{}, err
		}
		nextAttempt, err := activeAttempt.MarkSucceeded(params.Now, params.EvidenceRef)
		if err != nil {
			return job.Job{}, job.Attempt{}, err
		}
		return nextJob, nextAttempt, nil
	case job.AttemptStatusTimedOut:
		nextJob, err := record.MarkTimedOut(params.WorkerID, failure, params.Now)
		if err != nil {
			return job.Job{}, job.Attempt{}, err
		}
		nextAttempt, err := activeAttempt.MarkTimedOut(params.Now, failure)
		if err != nil {
			return job.Job{}, job.Attempt{}, err
		}
		return nextJob, nextAttempt, nil
	case job.AttemptStatusAbandoned:
		nextJob, err := record.MarkTimedOut(params.WorkerID, failure, params.Now)
		if err != nil {
			return job.Job{}, job.Attempt{}, err
		}
		nextAttempt, err := activeAttempt.MarkAbandoned(params.Now, failure)
		if err != nil {
			return job.Job{}, job.Attempt{}, err
		}
		return nextJob, nextAttempt, nil
	case job.AttemptStatusFailed:
		fallthrough
	default:
		nextJob, err := record.MarkFailed(params.WorkerID, failure, params.Now)
		if err != nil {
			return job.Job{}, job.Attempt{}, err
		}
		nextAttempt, err := activeAttempt.MarkFailed(params.Now, failure)
		if err != nil {
			return job.Job{}, job.Attempt{}, err
		}
		return nextJob, nextAttempt, nil
	}
}

func (r *MemoryRepository) requireJobLocked(jobID string) (job.Job, error) {
	jobID = strings.TrimSpace(jobID)
	record, exists := r.jobs[jobID]
	if !exists {
		return job.Job{}, job.ErrJobNotFound
	}

	return cloneJob(record), nil
}

func (r *MemoryRepository) activeAttemptLocked(jobID string, workerID string) (job.Attempt, int, error) {
	attempts := r.attempts[jobID]
	for idx := len(attempts) - 1; idx >= 0; idx-- {
		candidate := attempts[idx]
		if !candidate.Active() {
			continue
		}
		if strings.TrimSpace(workerID) != "" && candidate.WorkerID != strings.TrimSpace(workerID) {
			return job.Attempt{}, -1, job.ErrLeaseOwnerMismatch
		}

		return cloneAttempt(candidate), idx, nil
	}

	return job.Attempt{}, -1, job.ErrAttemptNotFound
}

func (r *MemoryRepository) claimableIDsLocked(now time.Time) []string {
	ids := make([]string, 0, len(r.jobs))
	for id, record := range r.jobs {
		if !record.Status.CanClaim() || !record.Due(now) {
			continue
		}
		ids = append(ids, id)
	}

	sort.Slice(ids, func(i, j int) bool {
		left := r.jobs[ids[i]]
		right := r.jobs[ids[j]]
		if left.Priority != right.Priority {
			return left.Priority < right.Priority
		}
		if !left.NextRunAt.Equal(right.NextRunAt) {
			return left.NextRunAt.Before(right.NextRunAt)
		}
		if !left.CreatedAt.Equal(right.CreatedAt) {
			return left.CreatedAt.Before(right.CreatedAt)
		}
		return left.ID < right.ID
	})

	return ids
}

func (r *MemoryRepository) expiredLeaseIDsLocked(now time.Time) []string {
	ids := make([]string, 0)
	for id, record := range r.jobs {
		if !record.Status.IsLeased() || !record.LeaseExpired(now) {
			continue
		}
		ids = append(ids, id)
	}

	sort.Slice(ids, func(i, j int) bool {
		left := r.jobs[ids[i]]
		right := r.jobs[ids[j]]
		if !left.LeaseExpireAt.Equal(right.LeaseExpireAt) {
			return left.LeaseExpireAt.Before(right.LeaseExpireAt)
		}
		return left.ID < right.ID
	})

	return ids
}

func cloneJob(record job.Job) job.Job {
	cloned := record
	if len(record.Payload) > 0 {
		cloned.Payload = append([]byte(nil), record.Payload...)
	}
	return cloned
}

func cloneAttempt(record job.Attempt) job.Attempt {
	return record
}

func min(left int, right int) int {
	if left < right {
		return left
	}
	return right
}
