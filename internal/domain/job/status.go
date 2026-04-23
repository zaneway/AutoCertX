package job

// Status represents the current scheduling state of a job.
type Status string

const (
	StatusPending   Status = "pending"
	StatusClaimed   Status = "claimed"
	StatusRunning   Status = "running"
	StatusRetry     Status = "retry"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
	StatusTimedOut  Status = "timed_out"
)

func (s Status) IsTerminal() bool {
	switch s {
	case StatusSucceeded, StatusFailed, StatusCancelled, StatusTimedOut:
		// Terminal jobs will never be claimed again by the scheduler.
		return true
	default:
		return false
	}
}

func (s Status) CanClaim() bool {
	// Only pending or retried jobs are eligible for worker claims.
	return s == StatusPending || s == StatusRetry
}

func (s Status) CanCancel() bool {
	switch s {
	case StatusPending, StatusClaimed, StatusRunning, StatusRetry:
		return true
	default:
		return false
	}
}

func (s Status) IsLeased() bool {
	// Claimed and running are the only states that should carry a live worker lease.
	return s == StatusClaimed || s == StatusRunning
}

// AttemptStatus represents the current execution state of one job attempt.
type AttemptStatus string

const (
	AttemptStatusStarted      AttemptStatus = "started"
	AttemptStatusHeartbeating AttemptStatus = "heartbeating"
	AttemptStatusSucceeded    AttemptStatus = "succeeded"
	AttemptStatusFailed       AttemptStatus = "failed"
	AttemptStatusTimedOut     AttemptStatus = "timed_out"
	AttemptStatusAbandoned    AttemptStatus = "abandoned"
)

func (s AttemptStatus) IsTerminal() bool {
	switch s {
	case AttemptStatusSucceeded, AttemptStatusFailed, AttemptStatusTimedOut, AttemptStatusAbandoned:
		// Terminal attempts are immutable historical records.
		return true
	default:
		return false
	}
}
