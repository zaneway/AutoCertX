package dns

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/zaneway/AutoCertX/internal/domain/resource"
)

// PresentInput contains the TXT record material to publish.
type PresentInput struct {
	RecordName  string
	RecordValue string
}

// CleanupInput contains the TXT record material to remove.
type CleanupInput struct {
	RecordName  string
	RecordValue string
}

// PropagationInput contains the TXT record material to check.
type PropagationInput struct {
	RecordName  string
	RecordValue string
}

// Executor is the DNS-01 adapter boundary used by T08 orchestration.
type Executor interface {
	PresentTXT(context.Context, PresentInput) error
	CleanupTXT(context.Context, CleanupInput) error
	CheckPropagation(context.Context, PropagationInput) (bool, error)
}

// TemporaryError marks a retryable DNS failure.
type TemporaryError struct {
	Err error
}

func (e TemporaryError) Error() string {
	if e.Err == nil {
		return "temporary dns failure"
	}
	return e.Err.Error()
}

func (e TemporaryError) Unwrap() error {
	return e.Err
}

// IsTemporary reports whether a DNS error should be retried.
func IsTemporary(err error) bool {
	var target TemporaryError
	return errors.As(err, &target)
}

// FakeExecutor provides a deterministic DNS executor for Phase A tests.
type FakeExecutor struct {
	mu             sync.Mutex
	PendingChecks  int
	PresentErr     error
	CleanupErr     error
	PropagationErr error
	checkCounts    map[string]int
}

// NewFakeExecutor constructs an empty fake DNS executor.
func NewFakeExecutor() *FakeExecutor {
	return &FakeExecutor{
		checkCounts: make(map[string]int),
	}
}

// PresentTXT records a successful presentation unless configured otherwise.
func (e *FakeExecutor) PresentTXT(_ context.Context, input PresentInput) error {
	if strings.TrimSpace(input.RecordName) == "" || strings.TrimSpace(input.RecordValue) == "" {
		return fmt.Errorf("dns record material required: %w", resource.ErrValidation)
	}
	if e.PresentErr != nil {
		return e.PresentErr
	}
	return nil
}

// CleanupTXT records a successful cleanup unless configured otherwise.
func (e *FakeExecutor) CleanupTXT(_ context.Context, input CleanupInput) error {
	if strings.TrimSpace(input.RecordName) == "" || strings.TrimSpace(input.RecordValue) == "" {
		return fmt.Errorf("dns record material required: %w", resource.ErrValidation)
	}
	if e.CleanupErr != nil {
		return e.CleanupErr
	}
	return nil
}

// CheckPropagation returns true once the configured pending count has been exhausted.
func (e *FakeExecutor) CheckPropagation(_ context.Context, input PropagationInput) (bool, error) {
	if strings.TrimSpace(input.RecordName) == "" || strings.TrimSpace(input.RecordValue) == "" {
		return false, fmt.Errorf("dns record material required: %w", resource.ErrValidation)
	}
	if e.PropagationErr != nil {
		return false, e.PropagationErr
	}

	key := input.RecordName + "|" + input.RecordValue
	e.mu.Lock()
	defer e.mu.Unlock()

	e.checkCounts[key]++
	return e.checkCounts[key] > e.PendingChecks, nil
}
