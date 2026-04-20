package job

import (
	"fmt"
	"strings"
	"time"
)

// Attempt stores one execution history item for a job.
type Attempt struct {
	ID              string
	JobID           string
	AttemptNo       int
	WorkerID        string
	AgentID         string
	StartedAt       time.Time
	LastHeartbeatAt time.Time
	FinishedAt      time.Time
	ResultStatus    AttemptStatus
	ErrorCode       string
	ErrorMessage    string
	EvidenceRef     string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// NewAttempt creates a new started attempt for a claimed job.
func NewAttempt(jobID string, attemptNo int, workerID string, now time.Time) (Attempt, error) {
	now = normalizeTime(now)
	if strings.TrimSpace(jobID) == "" {
		return Attempt{}, fmt.Errorf("%w: job id required", ErrInvalidAttempt)
	}
	if attemptNo <= 0 {
		return Attempt{}, fmt.Errorf("%w: attempt number must be positive", ErrInvalidAttempt)
	}
	if strings.TrimSpace(workerID) == "" {
		return Attempt{}, fmt.Errorf("%w: worker id required", ErrInvalidAttempt)
	}

	id, err := generateID()
	if err != nil {
		return Attempt{}, fmt.Errorf("generate attempt id: %w", err)
	}

	return Attempt{
		ID:           id,
		JobID:        strings.TrimSpace(jobID),
		AttemptNo:    attemptNo,
		WorkerID:     strings.TrimSpace(workerID),
		StartedAt:    now,
		ResultStatus: AttemptStatusStarted,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

// Active reports whether the attempt is still in progress.
func (a Attempt) Active() bool {
	return !a.ResultStatus.IsTerminal()
}

// Heartbeat records worker progress without closing the attempt.
func (a Attempt) Heartbeat(now time.Time) (Attempt, error) {
	now = normalizeTime(now)
	if !a.Active() {
		return Attempt{}, ErrAttemptFinished
	}

	next := a
	next.ResultStatus = AttemptStatusHeartbeating
	next.LastHeartbeatAt = now
	next.UpdatedAt = now
	return next, nil
}

// MarkSucceeded closes an attempt successfully.
func (a Attempt) MarkSucceeded(now time.Time, evidenceRef string) (Attempt, error) {
	now = normalizeTime(now)
	if !a.Active() {
		return Attempt{}, ErrAttemptFinished
	}

	next := a
	next.ResultStatus = AttemptStatusSucceeded
	next.EvidenceRef = strings.TrimSpace(evidenceRef)
	next.FinishedAt = now
	next.UpdatedAt = now
	return next, nil
}

// MarkFailed closes an attempt as failed.
func (a Attempt) MarkFailed(now time.Time, failure Failure) (Attempt, error) {
	return a.finish(AttemptStatusFailed, now, failure, "")
}

// MarkTimedOut closes an attempt as timed out.
func (a Attempt) MarkTimedOut(now time.Time, failure Failure) (Attempt, error) {
	return a.finish(AttemptStatusTimedOut, now, failure, "")
}

// MarkAbandoned closes an attempt as abandoned.
func (a Attempt) MarkAbandoned(now time.Time, failure Failure) (Attempt, error) {
	return a.finish(AttemptStatusAbandoned, now, failure, "")
}

func (a Attempt) finish(status AttemptStatus, now time.Time, failure Failure, evidenceRef string) (Attempt, error) {
	now = normalizeTime(now)
	if !a.Active() {
		return Attempt{}, ErrAttemptFinished
	}

	next := a
	next.ResultStatus = status
	next.ErrorCode = strings.TrimSpace(failure.Code)
	next.ErrorMessage = strings.TrimSpace(failure.Message)
	next.EvidenceRef = strings.TrimSpace(evidenceRef)
	next.FinishedAt = now
	next.UpdatedAt = now
	return next, nil
}
