package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/zaneway/AutoCertX/internal/domain/job"
	"github.com/zaneway/AutoCertX/internal/domain/resource"
	"github.com/zaneway/AutoCertX/internal/platform/apperr"
)

// Reader exposes the scheduler facts needed by the jobs read model.
type Reader interface {
	List(context.Context, resource.Scope) ([]job.Job, error)
	Get(context.Context, string) (job.Job, error)
	Attempts(context.Context, string) ([]job.Attempt, error)
}

// ListFilter narrows the job list returned to the console.
type ListFilter struct {
	Status        string
	JobType       string
	AggregateType string
	AggregateID   string
	WorkerID      string
	Limit         int
}

// ListItem is the list row consumed by the jobs table and risk workbench.
type ListItem struct {
	ID               string          `json:"id"`
	JobType          string          `json:"job_type"`
	AggregateType    string          `json:"aggregate_type"`
	AggregateID      string          `json:"aggregate_id"`
	Status           string          `json:"status"`
	Priority         int             `json:"priority"`
	Payload          json.RawMessage `json:"payload,omitempty"`
	LeaseOwner       string          `json:"lease_owner,omitempty"`
	AttemptCount     int             `json:"attempt_count"`
	MaxAttempts      int             `json:"max_attempts"`
	NextRunAt        time.Time       `json:"next_run_at"`
	LastErrorCode    string          `json:"last_error_code,omitempty"`
	LastErrorMessage string          `json:"last_error_message,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// Detail is the job detail payload used by the side panel and detail page.
type Detail struct {
	ID               string          `json:"id"`
	TenantID         string          `json:"tenant_id"`
	ProjectID        string          `json:"project_id"`
	EnvironmentID    string          `json:"environment_id"`
	JobType          string          `json:"job_type"`
	AggregateType    string          `json:"aggregate_type"`
	AggregateID      string          `json:"aggregate_id"`
	Status           string          `json:"status"`
	Priority         int             `json:"priority"`
	Payload          json.RawMessage `json:"payload,omitempty"`
	IdempotencyKey   string          `json:"idempotency_key"`
	LeaseOwner       string          `json:"lease_owner,omitempty"`
	LeaseExpireAt    *time.Time      `json:"lease_expire_at,omitempty"`
	AttemptCount     int             `json:"attempt_count"`
	MaxAttempts      int             `json:"max_attempts"`
	NextRunAt        time.Time       `json:"next_run_at"`
	LastErrorCode    string          `json:"last_error_code,omitempty"`
	LastErrorMessage string          `json:"last_error_message,omitempty"`
	Version          int             `json:"version"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// AttemptItem is the read-model row for one historical execution attempt.
type AttemptItem struct {
	ID              string     `json:"id"`
	JobID           string     `json:"job_id"`
	AttemptNo       int        `json:"attempt_no"`
	WorkerID        string     `json:"worker_id"`
	AgentID         string     `json:"agent_id,omitempty"`
	StartedAt       time.Time  `json:"started_at"`
	LastHeartbeatAt *time.Time `json:"last_heartbeat_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	ResultStatus    string     `json:"result_status"`
	ErrorCode       string     `json:"error_code,omitempty"`
	ErrorMessage    string     `json:"error_message,omitempty"`
	EvidenceRef     string     `json:"evidence_ref,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Service maps scheduler facts into page-oriented job query payloads.
type Service struct {
	reader Reader
}

// NewService constructs the jobs query service.
func NewService(reader Reader) (*Service, error) {
	if reader == nil {
		return nil, fmt.Errorf("jobs reader required")
	}
	return &Service{reader: reader}, nil
}

// ListJobs returns jobs visible under the resolved scope after applying list filters.
func (s *Service) ListJobs(ctx context.Context, scope resource.Scope, filter ListFilter) ([]ListItem, error) {
	records, err := s.reader.List(ctx, scope)
	if err != nil {
		return nil, translateError(err)
	}

	filter = normalizeFilter(filter)
	items := make([]ListItem, 0, len(records))
	for _, record := range records {
		if !matchesFilter(record, filter) {
			continue
		}
		items = append(items, mapListItem(record))
	}

	sortListItems(items)
	if filter.Limit > 0 && len(items) > filter.Limit {
		items = items[:filter.Limit]
	}
	return items, nil
}

// GetJob returns one job detail after enforcing the caller scope.
func (s *Service) GetJob(ctx context.Context, scope resource.Scope, id string) (Detail, error) {
	record, err := s.reader.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return Detail{}, translateError(err)
	}
	if !jobInScope(record, scope) {
		return Detail{}, translateError(resource.ErrScopeMismatch)
	}
	return mapDetail(record), nil
}

// ListJobAttempts returns the historical attempts for one visible job.
func (s *Service) ListJobAttempts(ctx context.Context, scope resource.Scope, id string) ([]AttemptItem, error) {
	record, err := s.reader.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, translateError(err)
	}
	if !jobInScope(record, scope) {
		return nil, translateError(resource.ErrScopeMismatch)
	}

	attempts, err := s.reader.Attempts(ctx, record.ID)
	if err != nil {
		return nil, translateError(err)
	}

	items := make([]AttemptItem, 0, len(attempts))
	for _, attempt := range attempts {
		items = append(items, mapAttemptItem(attempt))
	}
	slices.SortFunc(items, compareAttemptItems)
	return items, nil
}

func normalizeFilter(filter ListFilter) ListFilter {
	return ListFilter{
		Status:        strings.ToLower(strings.TrimSpace(filter.Status)),
		JobType:       strings.TrimSpace(filter.JobType),
		AggregateType: strings.TrimSpace(filter.AggregateType),
		AggregateID:   strings.TrimSpace(filter.AggregateID),
		WorkerID:      strings.TrimSpace(filter.WorkerID),
		Limit:         filter.Limit,
	}
}

func matchesFilter(record job.Job, filter ListFilter) bool {
	if filter.Status != "" && string(record.Status) != filter.Status {
		return false
	}
	if filter.JobType != "" && record.JobType != filter.JobType {
		return false
	}
	if filter.AggregateType != "" && record.AggregateType != filter.AggregateType {
		return false
	}
	if filter.AggregateID != "" && record.AggregateID != filter.AggregateID {
		return false
	}
	if filter.WorkerID != "" && record.LeaseOwner != filter.WorkerID {
		return false
	}
	return true
}

func sortListItems(items []ListItem) {
	slices.SortFunc(items, compareListItems)
}

func compareListItems(left ListItem, right ListItem) int {
	if !left.UpdatedAt.Equal(right.UpdatedAt) {
		if left.UpdatedAt.After(right.UpdatedAt) {
			return -1
		}
		return 1
	}
	if !left.CreatedAt.Equal(right.CreatedAt) {
		if left.CreatedAt.After(right.CreatedAt) {
			return -1
		}
		return 1
	}
	return strings.Compare(left.ID, right.ID)
}

func compareAttemptItems(left AttemptItem, right AttemptItem) int {
	if left.AttemptNo != right.AttemptNo {
		if left.AttemptNo > right.AttemptNo {
			return -1
		}
		return 1
	}
	return strings.Compare(left.ID, right.ID)
}

func mapListItem(record job.Job) ListItem {
	return ListItem{
		ID:               record.ID,
		JobType:          record.JobType,
		AggregateType:    record.AggregateType,
		AggregateID:      record.AggregateID,
		Status:           string(record.Status),
		Priority:         record.Priority,
		Payload:          clonePayload(record.Payload),
		LeaseOwner:       record.LeaseOwner,
		AttemptCount:     record.AttemptCount,
		MaxAttempts:      record.MaxAttempts,
		NextRunAt:        record.NextRunAt,
		LastErrorCode:    record.LastErrorCode,
		LastErrorMessage: record.LastErrorMessage,
		CreatedAt:        record.CreatedAt,
		UpdatedAt:        record.UpdatedAt,
	}
}

func mapDetail(record job.Job) Detail {
	var leaseExpireAt *time.Time
	if !record.LeaseExpireAt.IsZero() {
		value := record.LeaseExpireAt
		leaseExpireAt = &value
	}

	return Detail{
		ID:               record.ID,
		TenantID:         record.TenantID,
		ProjectID:        record.ProjectID,
		EnvironmentID:    record.EnvironmentID,
		JobType:          record.JobType,
		AggregateType:    record.AggregateType,
		AggregateID:      record.AggregateID,
		Status:           string(record.Status),
		Priority:         record.Priority,
		Payload:          clonePayload(record.Payload),
		IdempotencyKey:   record.IdempotencyKey,
		LeaseOwner:       record.LeaseOwner,
		LeaseExpireAt:    leaseExpireAt,
		AttemptCount:     record.AttemptCount,
		MaxAttempts:      record.MaxAttempts,
		NextRunAt:        record.NextRunAt,
		LastErrorCode:    record.LastErrorCode,
		LastErrorMessage: record.LastErrorMessage,
		Version:          record.Version,
		CreatedAt:        record.CreatedAt,
		UpdatedAt:        record.UpdatedAt,
	}
}

func mapAttemptItem(record job.Attempt) AttemptItem {
	return AttemptItem{
		ID:              record.ID,
		JobID:           record.JobID,
		AttemptNo:       record.AttemptNo,
		WorkerID:        record.WorkerID,
		AgentID:         record.AgentID,
		StartedAt:       record.StartedAt,
		LastHeartbeatAt: timePointer(record.LastHeartbeatAt),
		FinishedAt:      timePointer(record.FinishedAt),
		ResultStatus:    string(record.ResultStatus),
		ErrorCode:       record.ErrorCode,
		ErrorMessage:    record.ErrorMessage,
		EvidenceRef:     record.EvidenceRef,
		CreatedAt:       record.CreatedAt,
		UpdatedAt:       record.UpdatedAt,
	}
}

func timePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	cloned := value
	return &cloned
}

func clonePayload(payload json.RawMessage) json.RawMessage {
	if len(payload) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), payload...)
}

func jobInScope(record job.Job, scope resource.Scope) bool {
	return record.TenantID == scope.TenantID &&
		record.ProjectID == scope.ProjectID &&
		record.EnvironmentID == scope.EnvironmentID
}

func translateError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, job.ErrJobNotFound), errors.Is(err, job.ErrAttemptNotFound):
		return apperr.Wrap(err, http.StatusNotFound, "RESOURCE_NOT_FOUND", "resource not found")
	case errors.Is(err, resource.ErrScopeMismatch):
		return apperr.Wrap(err, http.StatusConflict, "TENANT_SCOPE_MISMATCH", "scope mismatch")
	case errors.Is(err, resource.ErrValidation), errors.Is(err, job.ErrInvalidJob), errors.Is(err, job.ErrInvalidAttempt):
		return apperr.Wrap(err, http.StatusBadRequest, "REQUEST_VALIDATION_FAILED", "request validation failed")
	default:
		return apperr.Wrap(err, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}
