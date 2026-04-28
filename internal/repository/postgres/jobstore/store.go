package jobstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	commandjobs "github.com/zaneway/AutoCertX/internal/application/command/jobs"
	"github.com/zaneway/AutoCertX/internal/domain/job"
	"github.com/zaneway/AutoCertX/internal/domain/resource"
)

// Store persists scheduler jobs into PostgreSQL using short transactions and SKIP LOCKED claim.
type Store struct {
	db *sql.DB
}

// New creates a PostgreSQL-backed job repository.
func New(db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, fmt.Errorf("postgres job store db required")
	}

	return &Store{db: db}, nil
}

func (s *Store) Create(ctx context.Context, record job.Job) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO jobs (
    id, tenant_id, project_id, environment_id, job_type, aggregate_type, aggregate_id,
    status, priority, payload_jsonb, idempotency_key, lease_owner, lease_expire_at,
    attempt_count, max_attempts, next_run_at, last_error_code, last_error_message,
    version, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10::jsonb, $11, $12, $13,
    $14, $15, $16, $17, $18,
    $19, $20, $21
)`,
		record.ID,
		record.TenantID,
		record.ProjectID,
		record.EnvironmentID,
		record.JobType,
		record.AggregateType,
		record.AggregateID,
		string(record.Status),
		record.Priority,
		payloadJSON(record.Payload),
		record.IdempotencyKey,
		nullString(record.LeaseOwner),
		nullTime(record.LeaseExpireAt),
		record.AttemptCount,
		record.MaxAttempts,
		record.NextRunAt,
		nullString(record.LastErrorCode),
		nullString(record.LastErrorMessage),
		record.Version,
		record.CreatedAt,
		record.UpdatedAt,
	)
	if isUniqueViolation(err) {
		return job.ErrDuplicateJob
	}
	if err != nil {
		return fmt.Errorf("insert job: %w", err)
	}

	return nil
}

func (s *Store) EnsurePlanned(ctx context.Context, record job.Job) (commandjobs.PlannedJob, error) {
	row := s.db.QueryRowContext(ctx, `
INSERT INTO jobs (
    id, tenant_id, project_id, environment_id, job_type, aggregate_type, aggregate_id,
    status, priority, payload_jsonb, idempotency_key, lease_owner, lease_expire_at,
    attempt_count, max_attempts, next_run_at, last_error_code, last_error_message,
    version, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10::jsonb, $11, $12, $13,
    $14, $15, $16, $17, $18,
    $19, $20, $21
)
ON CONFLICT (idempotency_key) DO NOTHING
RETURNING id, tenant_id, project_id, environment_id, job_type, aggregate_type, aggregate_id,
          status, priority, payload_jsonb, idempotency_key, lease_owner, lease_expire_at,
          attempt_count, max_attempts, next_run_at, last_error_code, last_error_message,
          version, created_at, updated_at`,
		record.ID,
		record.TenantID,
		record.ProjectID,
		record.EnvironmentID,
		record.JobType,
		record.AggregateType,
		record.AggregateID,
		string(record.Status),
		record.Priority,
		payloadJSON(record.Payload),
		record.IdempotencyKey,
		nullString(record.LeaseOwner),
		nullTime(record.LeaseExpireAt),
		record.AttemptCount,
		record.MaxAttempts,
		record.NextRunAt,
		nullString(record.LastErrorCode),
		nullString(record.LastErrorMessage),
		record.Version,
		record.CreatedAt,
		record.UpdatedAt,
	)

	created, err := scanJobRow(row)
	if err == nil {
		return commandjobs.PlannedJob{Job: created, Created: true}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return commandjobs.PlannedJob{}, fmt.Errorf("ensure planned insert: %w", err)
	}

	// ON CONFLICT DO NOTHING means a concurrent planner run already created the
	// slot; load and return that existing job instead.
	existing, err := s.Get(ctx, record.ID)
	if errors.Is(err, job.ErrJobNotFound) {
		existing, err = s.getByIdempotencyKey(ctx, record.IdempotencyKey)
	}
	if err != nil {
		return commandjobs.PlannedJob{}, err
	}

	return commandjobs.PlannedJob{Job: existing, Created: false}, nil
}

func (s *Store) Claim(ctx context.Context, params commandjobs.ClaimParams) ([]commandjobs.ClaimedJob, error) {
	if strings.TrimSpace(params.WorkerID) == "" {
		return nil, fmt.Errorf("%w: worker id required", job.ErrInvalidJob)
	}
	if params.MaxJobs <= 0 {
		params.MaxJobs = 1
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("begin claim tx: %w", err)
	}
	defer rollback(tx)

	// SKIP LOCKED lets multiple workers poll concurrently without blocking each
	// other while still serializing claims per selected row.
	rows, err := tx.QueryContext(ctx, `
SELECT id, tenant_id, project_id, environment_id, job_type, aggregate_type, aggregate_id,
       status, priority, payload_jsonb, idempotency_key, lease_owner, lease_expire_at,
       attempt_count, max_attempts, next_run_at, last_error_code, last_error_message,
       version, created_at, updated_at
FROM jobs
WHERE status IN ('pending', 'retry')
  AND next_run_at <= $1
ORDER BY priority ASC, next_run_at ASC, created_at ASC, id ASC
FOR UPDATE SKIP LOCKED
LIMIT $2`,
		params.Now,
		params.MaxJobs,
	)
	if err != nil {
		return nil, fmt.Errorf("query schedulable jobs: %w", err)
	}

	selected := make([]job.Job, 0, params.MaxJobs)
	for rows.Next() {
		current, scanErr := scanJob(rows)
		if scanErr != nil {
			_ = rows.Close()
			return nil, scanErr
		}
		selected = append(selected, current)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate schedulable jobs: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close schedulable rows: %w", err)
	}

	results := make([]commandjobs.ClaimedJob, 0, len(selected))
	for _, current := range selected {
		next, attempt, claimErr := current.Claim(params.WorkerID, params.LeaseTTL, params.Now)
		if claimErr != nil {
			return nil, claimErr
		}

		// Persist the claimed job row and its new attempt in the same transaction so
		// workers never observe one without the other.
		if err := updateJobTx(ctx, tx, next); err != nil {
			return nil, err
		}
		if err := insertAttemptTx(ctx, tx, attempt); err != nil {
			return nil, err
		}

		results = append(results, commandjobs.ClaimedJob{
			Job:     next,
			Attempt: attempt,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit claim tx: %w", err)
	}

	return results, nil
}

func (s *Store) MarkRunning(ctx context.Context, params commandjobs.MarkRunningParams) (job.Job, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return job.Job{}, fmt.Errorf("begin mark running tx: %w", err)
	}
	defer rollback(tx)

	current, err := getJobForUpdateTx(ctx, tx, params.JobID)
	if err != nil {
		return job.Job{}, err
	}
	if _, _, err := getActiveAttemptForUpdateTx(ctx, tx, params.JobID, params.WorkerID); err != nil {
		return job.Job{}, err
	}

	next, err := current.MarkRunning(params.WorkerID, params.Now)
	if err != nil {
		return job.Job{}, err
	}
	if err := updateJobTx(ctx, tx, next); err != nil {
		return job.Job{}, err
	}

	if err := tx.Commit(); err != nil {
		return job.Job{}, fmt.Errorf("commit mark running tx: %w", err)
	}

	return next, nil
}

func (s *Store) Heartbeat(ctx context.Context, params commandjobs.HeartbeatParams) (commandjobs.HeartbeatResult, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return commandjobs.HeartbeatResult{}, fmt.Errorf("begin heartbeat tx: %w", err)
	}
	defer rollback(tx)

	current, err := getJobForUpdateTx(ctx, tx, params.JobID)
	if err != nil {
		return commandjobs.HeartbeatResult{}, err
	}
	nextJob, err := current.Renew(params.WorkerID, params.LeaseTTL, params.Now)
	if err != nil {
		return commandjobs.HeartbeatResult{}, err
	}

	attempt, attemptID, err := getActiveAttemptForUpdateTx(ctx, tx, params.JobID, params.WorkerID)
	if err != nil {
		return commandjobs.HeartbeatResult{}, err
	}
	nextAttempt, err := attempt.Heartbeat(params.Now)
	if err != nil {
		return commandjobs.HeartbeatResult{}, err
	}

	// Job lease and attempt heartbeat are updated together so reaper decisions and
	// operator diagnostics are based on the same heartbeat timestamp.
	if err := updateJobTx(ctx, tx, nextJob); err != nil {
		return commandjobs.HeartbeatResult{}, err
	}
	if err := updateAttemptTx(ctx, tx, attemptID, nextAttempt); err != nil {
		return commandjobs.HeartbeatResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return commandjobs.HeartbeatResult{}, fmt.Errorf("commit heartbeat tx: %w", err)
	}

	return commandjobs.HeartbeatResult{
		Job:     nextJob,
		Attempt: nextAttempt,
	}, nil
}

func (s *Store) Complete(ctx context.Context, params commandjobs.CompleteParams) (commandjobs.CompletionResult, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return commandjobs.CompletionResult{}, fmt.Errorf("begin completion tx: %w", err)
	}
	defer rollback(tx)

	current, err := getJobForUpdateTx(ctx, tx, params.JobID)
	if err != nil {
		return commandjobs.CompletionResult{}, err
	}
	attempt, attemptID, err := getActiveAttemptForUpdateTx(ctx, tx, params.JobID, params.WorkerID)
	if err != nil {
		return commandjobs.CompletionResult{}, err
	}

	failure := job.Failure{
		Code:    params.ErrorCode,
		Message: params.ErrorMessage,
	}

	nextJob, nextAttempt, err := completeJob(current, attempt, params, failure)
	if err != nil {
		return commandjobs.CompletionResult{}, err
	}

	retryScheduled := false
	if params.Retryable && nextJob.CanRetry() && nextJob.Status != job.StatusSucceeded {
		// Completion applies retry scheduling before commit so the persisted job row
		// already reflects whether it re-entered the queue.
		retried, retryErr := nextJob.ScheduleRetry(params.RetryDelay, params.Now)
		if retryErr == nil {
			nextJob = retried
			retryScheduled = true
		}
	}

	if err := updateJobTx(ctx, tx, nextJob); err != nil {
		return commandjobs.CompletionResult{}, err
	}
	if err := updateAttemptTx(ctx, tx, attemptID, nextAttempt); err != nil {
		return commandjobs.CompletionResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return commandjobs.CompletionResult{}, fmt.Errorf("commit completion tx: %w", err)
	}

	return commandjobs.CompletionResult{
		Job:            nextJob,
		Attempt:        nextAttempt,
		RetryScheduled: retryScheduled,
	}, nil
}

func (s *Store) Cancel(ctx context.Context, params commandjobs.CancelParams) (job.Job, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return job.Job{}, fmt.Errorf("begin cancel tx: %w", err)
	}
	defer rollback(tx)

	current, err := getJobForUpdateTx(ctx, tx, params.JobID)
	if err != nil {
		return job.Job{}, err
	}

	if current.LeaseOwner != "" {
		if attempt, attemptID, attemptErr := getActiveAttemptForUpdateTx(ctx, tx, params.JobID, current.LeaseOwner); attemptErr == nil {
			// Cancellation closes any in-flight attempt best-effort before flipping
			// the job into its terminal cancelled state.
			abandoned, finishErr := attempt.MarkAbandoned(params.Now, job.Failure{
				Code:    "cancelled",
				Message: "job cancelled",
			})
			if finishErr == nil {
				if err := updateAttemptTx(ctx, tx, attemptID, abandoned); err != nil {
					return job.Job{}, err
				}
			}
		}
	}

	next, err := current.Cancel(params.Now)
	if err != nil {
		return job.Job{}, err
	}
	if err := updateJobTx(ctx, tx, next); err != nil {
		return job.Job{}, err
	}

	if err := tx.Commit(); err != nil {
		return job.Job{}, fmt.Errorf("commit cancel tx: %w", err)
	}

	return next, nil
}

func (s *Store) ReapExpired(ctx context.Context, params commandjobs.ReapParams) ([]commandjobs.ReapedLease, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("begin reap tx: %w", err)
	}
	defer rollback(tx)

	limit := params.Limit
	if limit <= 0 {
		limit = 100
	}

	rows, err := tx.QueryContext(ctx, `
SELECT id, tenant_id, project_id, environment_id, job_type, aggregate_type, aggregate_id,
       status, priority, payload_jsonb, idempotency_key, lease_owner, lease_expire_at,
       attempt_count, max_attempts, next_run_at, last_error_code, last_error_message,
       version, created_at, updated_at
FROM jobs
WHERE status IN ('claimed', 'running')
  AND lease_expire_at IS NOT NULL
  AND lease_expire_at <= $1
ORDER BY lease_expire_at ASC, id ASC
FOR UPDATE SKIP LOCKED
LIMIT $2`,
		params.Now,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query expired leases: %w", err)
	}

	selected := make([]job.Job, 0)
	for rows.Next() {
		current, scanErr := scanJob(rows)
		if scanErr != nil {
			_ = rows.Close()
			return nil, scanErr
		}
		selected = append(selected, current)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate expired leases: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close expired lease rows: %w", err)
	}

	results := make([]commandjobs.ReapedLease, 0, len(selected))
	for _, current := range selected {
		attempt, attemptID, err := getActiveAttemptForUpdateTx(ctx, tx, current.ID, current.LeaseOwner)
		if err != nil {
			return nil, err
		}

		nextJob, attemptStatus, err := current.ExpireLease(params.Now, job.Failure{
			Code:    "lease_expired",
			Message: "worker lease expired before completion",
		})
		if err != nil {
			return nil, err
		}

		var nextAttempt job.Attempt
		switch attemptStatus {
		case job.AttemptStatusAbandoned:
			// Claimed-but-not-running work is classified as abandoned so operators can
			// tell queue loss from runtime timeout.
			nextAttempt, err = attempt.MarkAbandoned(params.Now, job.Failure{
				Code:    "lease_expired",
				Message: "worker lost before execution started",
			})
		default:
			nextAttempt, err = attempt.MarkTimedOut(params.Now, job.Failure{
				Code:    "lease_expired",
				Message: "worker lost during execution",
			})
		}
		if err != nil {
			return nil, err
		}

		retryScheduled := false
		if nextJob.CanRetry() {
			// Reaped jobs share the same retry policy as explicit failures to keep
			// recovery behavior predictable across failure modes.
			retried, retryErr := nextJob.ScheduleRetry(params.RetryPolicy.Delay(nextJob.AttemptCount), params.Now)
			if retryErr == nil {
				nextJob = retried
				retryScheduled = true
			}
		}

		if err := updateJobTx(ctx, tx, nextJob); err != nil {
			return nil, err
		}
		if err := updateAttemptTx(ctx, tx, attemptID, nextAttempt); err != nil {
			return nil, err
		}

		results = append(results, commandjobs.ReapedLease{
			Job:            nextJob,
			Attempt:        nextAttempt,
			RetryScheduled: retryScheduled,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit reap tx: %w", err)
	}

	return results, nil
}

// List returns scheduler facts scoped to one environment boundary.
func (s *Store) List(ctx context.Context, scope resource.Scope) ([]job.Job, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT id, tenant_id, project_id, environment_id, job_type, aggregate_type, aggregate_id,
       status, priority, payload_jsonb, idempotency_key, lease_owner, lease_expire_at,
       attempt_count, max_attempts, next_run_at, last_error_code, last_error_message,
       version, created_at, updated_at
FROM jobs
WHERE tenant_id = $1
  AND project_id = $2
  AND environment_id = $3
ORDER BY created_at ASC, id ASC`,
		scope.TenantID,
		scope.ProjectID,
		scope.EnvironmentID,
	)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	items := make([]job.Job, 0)
	for rows.Next() {
		record, scanErr := scanJob(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate jobs: %w", err)
	}
	return items, nil
}

func (s *Store) Get(ctx context.Context, jobID string) (job.Job, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, tenant_id, project_id, environment_id, job_type, aggregate_type, aggregate_id,
       status, priority, payload_jsonb, idempotency_key, lease_owner, lease_expire_at,
       attempt_count, max_attempts, next_run_at, last_error_code, last_error_message,
       version, created_at, updated_at
FROM jobs
WHERE id = $1`,
		jobID,
	)

	record, err := scanJobRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return job.Job{}, job.ErrJobNotFound
	}
	if err != nil {
		return job.Job{}, fmt.Errorf("get job: %w", err)
	}

	return record, nil
}

// GetByIdempotency returns one job by idempotency key.
func (s *Store) GetByIdempotency(ctx context.Context, idempotencyKey string) (job.Job, error) {
	return s.getByIdempotencyKey(ctx, strings.TrimSpace(idempotencyKey))
}

func (s *Store) Attempts(ctx context.Context, jobID string) ([]job.Attempt, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, job_id, attempt_no, worker_id, agent_id, started_at, last_heartbeat_at,
       finished_at, result_status, error_code, error_message, evidence_ref, created_at, updated_at
FROM job_attempts
WHERE job_id = $1
ORDER BY attempt_no ASC`,
		jobID,
	)
	if err != nil {
		return nil, fmt.Errorf("query attempts: %w", err)
	}
	defer rows.Close()

	var items []job.Attempt
	for rows.Next() {
		item, scanErr := scanAttempt(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate attempts: %w", err)
	}

	return items, nil
}

func (s *Store) getByIdempotencyKey(ctx context.Context, idempotencyKey string) (job.Job, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, tenant_id, project_id, environment_id, job_type, aggregate_type, aggregate_id,
       status, priority, payload_jsonb, idempotency_key, lease_owner, lease_expire_at,
       attempt_count, max_attempts, next_run_at, last_error_code, last_error_message,
       version, created_at, updated_at
FROM jobs
WHERE idempotency_key = $1`,
		idempotencyKey,
	)

	record, err := scanJobRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return job.Job{}, job.ErrJobNotFound
	}
	if err != nil {
		return job.Job{}, fmt.Errorf("get job by idempotency key: %w", err)
	}

	return record, nil
}

func getJobForUpdateTx(ctx context.Context, tx *sql.Tx, jobID string) (job.Job, error) {
	row := tx.QueryRowContext(ctx, `
SELECT id, tenant_id, project_id, environment_id, job_type, aggregate_type, aggregate_id,
       status, priority, payload_jsonb, idempotency_key, lease_owner, lease_expire_at,
       attempt_count, max_attempts, next_run_at, last_error_code, last_error_message,
       version, created_at, updated_at
FROM jobs
WHERE id = $1
FOR UPDATE`,
		jobID,
	)

	record, err := scanJobRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return job.Job{}, job.ErrJobNotFound
	}
	if err != nil {
		return job.Job{}, fmt.Errorf("select job for update: %w", err)
	}

	return record, nil
}

func getActiveAttemptForUpdateTx(ctx context.Context, tx *sql.Tx, jobID string, workerID string) (job.Attempt, string, error) {
	row := tx.QueryRowContext(ctx, `
SELECT id, job_id, attempt_no, worker_id, agent_id, started_at, last_heartbeat_at,
       finished_at, result_status, error_code, error_message, evidence_ref, created_at, updated_at
FROM job_attempts
WHERE job_id = $1
  AND worker_id = $2
  AND result_status IN ('started', 'heartbeating')
ORDER BY attempt_no DESC
LIMIT 1
FOR UPDATE`,
		jobID,
		workerID,
	)

	var id string
	attempt, err := scanAttemptWithID(row, &id)
	if errors.Is(err, sql.ErrNoRows) {
		return job.Attempt{}, "", job.ErrAttemptNotFound
	}
	if err != nil {
		return job.Attempt{}, "", fmt.Errorf("select attempt for update: %w", err)
	}

	return attempt, id, nil
}

func updateJobTx(ctx context.Context, tx *sql.Tx, record job.Job) error {
	result, err := tx.ExecContext(ctx, `
UPDATE jobs
SET status = $2,
    priority = $3,
    payload_jsonb = $4::jsonb,
    idempotency_key = $5,
    lease_owner = $6,
    lease_expire_at = $7,
    attempt_count = $8,
    max_attempts = $9,
    next_run_at = $10,
    last_error_code = $11,
    last_error_message = $12,
    version = $13,
    updated_at = $14
WHERE id = $1`,
		record.ID,
		string(record.Status),
		record.Priority,
		payloadJSON(record.Payload),
		record.IdempotencyKey,
		nullString(record.LeaseOwner),
		nullTime(record.LeaseExpireAt),
		record.AttemptCount,
		record.MaxAttempts,
		record.NextRunAt,
		nullString(record.LastErrorCode),
		nullString(record.LastErrorMessage),
		record.Version,
		record.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("update job: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("job rows affected: %w", err)
	}
	if rowsAffected != 1 {
		return fmt.Errorf("update job: expected 1 row, got %d", rowsAffected)
	}

	return nil
}

func insertAttemptTx(ctx context.Context, tx *sql.Tx, attempt job.Attempt) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO job_attempts (
    id, job_id, attempt_no, worker_id, agent_id, started_at, last_heartbeat_at,
    finished_at, result_status, error_code, error_message, evidence_ref, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10, $11, $12, $13, $14
)`,
		attempt.ID,
		attempt.JobID,
		attempt.AttemptNo,
		attempt.WorkerID,
		nullString(attempt.AgentID),
		attempt.StartedAt,
		nullTime(attempt.LastHeartbeatAt),
		nullTime(attempt.FinishedAt),
		string(attempt.ResultStatus),
		nullString(attempt.ErrorCode),
		nullString(attempt.ErrorMessage),
		nullString(attempt.EvidenceRef),
		attempt.CreatedAt,
		attempt.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert attempt: %w", err)
	}

	return nil
}

func updateAttemptTx(ctx context.Context, tx *sql.Tx, attemptID string, attempt job.Attempt) error {
	result, err := tx.ExecContext(ctx, `
UPDATE job_attempts
SET worker_id = $2,
    agent_id = $3,
    started_at = $4,
    last_heartbeat_at = $5,
    finished_at = $6,
    result_status = $7,
    error_code = $8,
    error_message = $9,
    evidence_ref = $10,
    updated_at = $11
WHERE id = $1`,
		attemptID,
		attempt.WorkerID,
		nullString(attempt.AgentID),
		attempt.StartedAt,
		nullTime(attempt.LastHeartbeatAt),
		nullTime(attempt.FinishedAt),
		string(attempt.ResultStatus),
		nullString(attempt.ErrorCode),
		nullString(attempt.ErrorMessage),
		nullString(attempt.EvidenceRef),
		attempt.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("update attempt: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("attempt rows affected: %w", err)
	}
	if rowsAffected != 1 {
		return fmt.Errorf("update attempt: expected 1 row, got %d", rowsAffected)
	}

	return nil
}

func completeJob(
	current job.Job,
	attempt job.Attempt,
	params commandjobs.CompleteParams,
	failure job.Failure,
) (job.Job, job.Attempt, error) {
	switch params.ResultStatus {
	case job.AttemptStatusSucceeded:
		// Success updates both aggregates into their terminal succeeded state.
		nextJob, err := current.MarkSucceeded(params.WorkerID, params.Now)
		if err != nil {
			return job.Job{}, job.Attempt{}, err
		}
		nextAttempt, err := attempt.MarkSucceeded(params.Now, params.EvidenceRef)
		if err != nil {
			return job.Job{}, job.Attempt{}, err
		}
		return nextJob, nextAttempt, nil
	case job.AttemptStatusTimedOut:
		// Timeouts are treated as job-level timeout failures, which may still be
		// retried later by the command service depending on policy.
		nextJob, err := current.MarkTimedOut(params.WorkerID, failure, params.Now)
		if err != nil {
			return job.Job{}, job.Attempt{}, err
		}
		nextAttempt, err := attempt.MarkTimedOut(params.Now, failure)
		if err != nil {
			return job.Job{}, job.Attempt{}, err
		}
		return nextJob, nextAttempt, nil
	case job.AttemptStatusAbandoned:
		nextJob, err := current.MarkTimedOut(params.WorkerID, failure, params.Now)
		if err != nil {
			return job.Job{}, job.Attempt{}, err
		}
		nextAttempt, err := attempt.MarkAbandoned(params.Now, failure)
		if err != nil {
			return job.Job{}, job.Attempt{}, err
		}
		return nextJob, nextAttempt, nil
	default:
		nextJob, err := current.MarkFailed(params.WorkerID, failure, params.Now)
		if err != nil {
			return job.Job{}, job.Attempt{}, err
		}
		nextAttempt, err := attempt.MarkFailed(params.Now, failure)
		if err != nil {
			return job.Job{}, job.Attempt{}, err
		}
		return nextJob, nextAttempt, nil
	}
}

func scanJobRow(row interface {
	Scan(...any) error
}) (job.Job, error) {
	return scanJob(row)
}

func scanJob(scanner interface {
	Scan(...any) error
}) (job.Job, error) {
	var (
		record           job.Job
		payload          []byte
		status           string
		leaseOwner       sql.NullString
		leaseExpireAt    sql.NullTime
		lastErrorCode    sql.NullString
		lastErrorMessage sql.NullString
	)

	if err := scanner.Scan(
		&record.ID,
		&record.TenantID,
		&record.ProjectID,
		&record.EnvironmentID,
		&record.JobType,
		&record.AggregateType,
		&record.AggregateID,
		&status,
		&record.Priority,
		&payload,
		&record.IdempotencyKey,
		&leaseOwner,
		&leaseExpireAt,
		&record.AttemptCount,
		&record.MaxAttempts,
		&record.NextRunAt,
		&lastErrorCode,
		&lastErrorMessage,
		&record.Version,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		return job.Job{}, err
	}

	record.Status = job.Status(status)
	record.Payload = append([]byte(nil), payload...)
	if leaseOwner.Valid {
		record.LeaseOwner = leaseOwner.String
	}
	if leaseExpireAt.Valid {
		record.LeaseExpireAt = leaseExpireAt.Time.UTC()
	}
	if lastErrorCode.Valid {
		record.LastErrorCode = lastErrorCode.String
	}
	if lastErrorMessage.Valid {
		record.LastErrorMessage = lastErrorMessage.String
	}
	record.NextRunAt = record.NextRunAt.UTC()
	record.CreatedAt = record.CreatedAt.UTC()
	record.UpdatedAt = record.UpdatedAt.UTC()

	return record, nil
}

func scanAttempt(scanner interface {
	Scan(...any) error
}) (job.Attempt, error) {
	return scanAttemptWithID(scanner, nil)
}

func scanAttemptWithID(scanner interface {
	Scan(...any) error
}, rawID *string) (job.Attempt, error) {
	var (
		attempt         job.Attempt
		agentID         sql.NullString
		lastHeartbeatAt sql.NullTime
		finishedAt      sql.NullTime
		resultStatus    string
		errorCode       sql.NullString
		errorMessage    sql.NullString
		evidenceRef     sql.NullString
		id              string
	)

	if err := scanner.Scan(
		&id,
		&attempt.JobID,
		&attempt.AttemptNo,
		&attempt.WorkerID,
		&agentID,
		&attempt.StartedAt,
		&lastHeartbeatAt,
		&finishedAt,
		&resultStatus,
		&errorCode,
		&errorMessage,
		&evidenceRef,
		&attempt.CreatedAt,
		&attempt.UpdatedAt,
	); err != nil {
		return job.Attempt{}, err
	}

	attempt.ID = id
	attempt.ResultStatus = job.AttemptStatus(resultStatus)
	attempt.StartedAt = attempt.StartedAt.UTC()
	attempt.CreatedAt = attempt.CreatedAt.UTC()
	attempt.UpdatedAt = attempt.UpdatedAt.UTC()
	if rawID != nil {
		*rawID = id
	}
	if agentID.Valid {
		attempt.AgentID = agentID.String
	}
	if lastHeartbeatAt.Valid {
		attempt.LastHeartbeatAt = lastHeartbeatAt.Time.UTC()
	}
	if finishedAt.Valid {
		attempt.FinishedAt = finishedAt.Time.UTC()
	}
	if errorCode.Valid {
		attempt.ErrorCode = errorCode.String
	}
	if errorMessage.Valid {
		attempt.ErrorMessage = errorMessage.String
	}
	if evidenceRef.Valid {
		attempt.EvidenceRef = evidenceRef.String
	}

	return attempt, nil
}

func payloadJSON(payload json.RawMessage) string {
	if len(payload) == 0 {
		return "{}"
	}

	return string(payload)
}

func nullString(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	return trimmed
}

func nullTime(ts time.Time) any {
	if ts.IsZero() {
		return nil
	}

	return ts.UTC()
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	return pgErr.Code == "23505"
}

func rollback(tx *sql.Tx) {
	_ = tx.Rollback()
}
