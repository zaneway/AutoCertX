package scheduler

import (
	"context"
	"errors"
	"fmt"
	"time"

	jobcommand "github.com/zaneway/AutoCertX/internal/application/command/jobs"
	"github.com/zaneway/AutoCertX/internal/domain/job"
)

// JobService captures the job use cases the scheduler depends on.
type JobService interface {
	Claim(context.Context, jobcommand.ClaimParams) ([]jobcommand.ClaimedJob, error)
	MarkRunning(context.Context, jobcommand.MarkRunningParams) (job.Job, error)
	Heartbeat(context.Context, jobcommand.HeartbeatParams) (jobcommand.HeartbeatResult, error)
	Complete(context.Context, jobcommand.CompleteParams) (jobcommand.CompletionResult, error)
}

// Executor runs one claimed job and returns the desired completion result.
type Executor interface {
	Execute(context.Context, jobcommand.ClaimedJob) ExecutionResult
}

// ExecutionResult describes how the worker should complete one attempt.
type ExecutionResult struct {
	ResultStatus job.AttemptStatus
	Retryable    bool
	RetryDelay   time.Duration
	ErrorCode    string
	ErrorMessage string
	EvidenceRef  string
}

// Worker claims due jobs, renews leases while executing, and completes attempts.
type Worker struct {
	Service       JobService
	Executor      Executor
	WorkerID      string
	MaxJobs       int
	LeaseTTL      time.Duration
	RenewInterval time.Duration
	Clock         func() time.Time
}

// RunOnce executes one polling iteration.
func (w Worker) RunOnce(ctx context.Context) (int, error) {
	if w.Service == nil {
		return 0, fmt.Errorf("scheduler worker service required")
	}
	if w.Executor == nil {
		return 0, fmt.Errorf("scheduler worker executor required")
	}
	if w.WorkerID == "" {
		return 0, fmt.Errorf("scheduler worker id required")
	}
	if w.MaxJobs <= 0 {
		w.MaxJobs = 1
	}
	if w.LeaseTTL <= 0 {
		return 0, fmt.Errorf("scheduler worker lease ttl required")
	}

	now := w.now()
	claims, err := w.Service.Claim(ctx, jobcommand.ClaimParams{
		WorkerID: w.WorkerID,
		MaxJobs:  w.MaxJobs,
		LeaseTTL: w.LeaseTTL,
		Now:      now,
	})
	if err != nil {
		return 0, err
	}

	// Claimed jobs are executed independently so one failure does not prevent the
	// worker from acknowledging other work acquired in the same poll.
	processed := 0
	var errs []error
	for _, claim := range claims {
		if err := w.executeClaim(ctx, claim); err != nil {
			errs = append(errs, fmt.Errorf("job %s: %w", claim.Job.ID, err))
			continue
		}
		processed++
	}

	return processed, errors.Join(errs...)
}

func (w Worker) executeClaim(ctx context.Context, claim jobcommand.ClaimedJob) error {
	// Mark the job as running only when execution is actually about to start so
	// the persisted state reflects real worker progress.
	if _, err := w.Service.MarkRunning(ctx, jobcommand.MarkRunningParams{
		JobID:    claim.Job.ID,
		WorkerID: w.WorkerID,
		Now:      w.now(),
	}); err != nil {
		return err
	}

	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	renewErr := make(chan error, 1)
	stopped := make(chan struct{})
	if w.RenewInterval > 0 {
		// Long-running executions renew the lease in the background to avoid being
		// reaped while healthy work is still in progress.
		go w.renewLoop(execCtx, claim.Job.ID, renewErr, stopped)
	} else {
		close(stopped)
	}

	result := w.Executor.Execute(execCtx, claim)
	cancel()
	<-stopped

	// Lease-renewal failures take precedence because the worker may have lost
	// ownership of the job before trying to complete it.
	select {
	case err := <-renewErr:
		if err != nil {
			return err
		}
	default:
	}

	_, err := w.Service.Complete(ctx, jobcommand.CompleteParams{
		JobID:        claim.Job.ID,
		WorkerID:     w.WorkerID,
		ResultStatus: normalizeResultStatus(result.ResultStatus),
		Retryable:    result.Retryable,
		RetryDelay:   result.RetryDelay,
		ErrorCode:    result.ErrorCode,
		ErrorMessage: result.ErrorMessage,
		EvidenceRef:  result.EvidenceRef,
		Now:          w.now(),
	})
	return err
}

func (w Worker) renewLoop(ctx context.Context, jobID string, renewErr chan<- error, stopped chan<- struct{}) {
	defer close(stopped)

	ticker := time.NewTicker(w.RenewInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Heartbeats extend the worker lease and update attempt liveness for
			// reaper safety and operational visibility.
			if _, err := w.Service.Heartbeat(ctx, jobcommand.HeartbeatParams{
				JobID:    jobID,
				WorkerID: w.WorkerID,
				LeaseTTL: w.LeaseTTL,
				Now:      w.now(),
			}); err != nil {
				select {
				case renewErr <- err:
				default:
				}
				return
			}
		}
	}
}

func (w Worker) now() time.Time {
	if w.Clock == nil {
		return time.Now().UTC()
	}

	return w.Clock().UTC()
}

func normalizeResultStatus(status job.AttemptStatus) job.AttemptStatus {
	if status == "" {
		return job.AttemptStatusSucceeded
	}

	return status
}
