package job

import "errors"

var (
	ErrInvalidJob         = errors.New("invalid job")
	ErrInvalidAttempt     = errors.New("invalid job attempt")
	ErrInvalidTransition  = errors.New("invalid job transition")
	ErrJobNotSchedulable  = errors.New("job not schedulable")
	ErrLeaseNotHeld       = errors.New("lease not held")
	ErrLeaseExpired       = errors.New("lease expired")
	ErrLeaseOwnerMismatch = errors.New("lease owner mismatch")
	ErrAttemptFinished    = errors.New("job attempt already finished")
	ErrJobNotRetryable    = errors.New("job is not retryable")
	ErrDuplicateJob       = errors.New("duplicate job")
	ErrJobNotFound        = errors.New("job not found")
	ErrAttemptNotFound    = errors.New("job attempt not found")
)
