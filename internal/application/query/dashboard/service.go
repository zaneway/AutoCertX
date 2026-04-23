package dashboard

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	auditdomain "github.com/zaneway/AutoCertX/internal/domain/audit"
	"github.com/zaneway/AutoCertX/internal/domain/dnscredentials"
	domaindomain "github.com/zaneway/AutoCertX/internal/domain/domains"
	"github.com/zaneway/AutoCertX/internal/domain/issuer"
	"github.com/zaneway/AutoCertX/internal/domain/job"
	"github.com/zaneway/AutoCertX/internal/domain/resource"
	"github.com/zaneway/AutoCertX/internal/platform/apperr"
)

// JobReader exposes only the scheduler facts needed by the dashboard.
type JobReader interface {
	List(context.Context, resource.Scope) ([]job.Job, error)
}

// Summary is the phase-A dashboard KPI payload.
type Summary struct {
	DomainCount        int `json:"domain_count"`
	DNSCredentialCount int `json:"dns_credential_count"`
	CAAccountCount     int `json:"ca_account_count"`
	WebhookCount       int `json:"webhook_count"`
	FailedJobCount     int `json:"failed_job_count"`
}

// JobFailureItem is the dashboard risk row for one failed or timed-out job.
type JobFailureItem struct {
	ID               string     `json:"id"`
	JobType          string     `json:"job_type"`
	AggregateType    string     `json:"aggregate_type"`
	AggregateID      string     `json:"aggregate_id"`
	Status           string     `json:"status"`
	LeaseOwner       string     `json:"lease_owner,omitempty"`
	AttemptCount     int        `json:"attempt_count"`
	MaxAttempts      int        `json:"max_attempts"`
	NextRunAt        time.Time  `json:"next_run_at"`
	LastErrorCode    string     `json:"last_error_code,omitempty"`
	LastErrorMessage string     `json:"last_error_message,omitempty"`
	UpdatedAt        time.Time  `json:"updated_at"`
	LeaseExpireAt    *time.Time `json:"lease_expire_at,omitempty"`
}

// Service builds page-oriented dashboard aggregates from existing domain facts.
type Service struct {
	domains *domaindomain.Service
	dns     *dnscredentials.Service
	issuer  *issuer.Service
	audit   *auditdomain.Service
	jobs    JobReader
}

// NewService constructs the dashboard query service.
func NewService(
	domainService *domaindomain.Service,
	dnsService *dnscredentials.Service,
	issuerService *issuer.Service,
	auditService *auditdomain.Service,
	jobReader JobReader,
) (*Service, error) {
	switch {
	case domainService == nil:
		return nil, fmt.Errorf("dashboard domain service required")
	case dnsService == nil:
		return nil, fmt.Errorf("dashboard dns service required")
	case issuerService == nil:
		return nil, fmt.Errorf("dashboard issuer service required")
	case auditService == nil:
		return nil, fmt.Errorf("dashboard audit service required")
	case jobReader == nil:
		return nil, fmt.Errorf("dashboard job reader required")
	default:
		return &Service{
			domains: domainService,
			dns:     dnsService,
			issuer:  issuerService,
			audit:   auditService,
			jobs:    jobReader,
		}, nil
	}
}

// GetSummary returns the phase-A KPI summary backed by currently implemented facts only.
func (s *Service) GetSummary(ctx context.Context, scope resource.Scope) (Summary, error) {
	domains, err := s.domains.List(scope)
	if err != nil {
		return Summary{}, translateError(err)
	}
	credentials, err := s.dns.List(scope)
	if err != nil {
		return Summary{}, translateError(err)
	}
	accounts, err := s.issuer.List(scope)
	if err != nil {
		return Summary{}, translateError(err)
	}
	webhooks, err := s.audit.ListWebhookEndpoints(scope)
	if err != nil {
		return Summary{}, translateError(err)
	}
	jobs, err := s.jobs.List(ctx, scope)
	if err != nil {
		return Summary{}, translateError(err)
	}

	failedJobCount := 0
	for _, record := range jobs {
		if isFailureStatus(record.Status) {
			failedJobCount++
		}
	}

	return Summary{
		DomainCount:        len(domains),
		DNSCredentialCount: len(credentials),
		CAAccountCount:     len(accounts),
		WebhookCount:       len(webhooks),
		FailedJobCount:     failedJobCount,
	}, nil
}

// ListJobFailures returns the most recent failed or timed-out jobs for the dashboard.
func (s *Service) ListJobFailures(ctx context.Context, scope resource.Scope, limit int) ([]JobFailureItem, error) {
	records, err := s.jobs.List(ctx, scope)
	if err != nil {
		return nil, translateError(err)
	}

	items := make([]JobFailureItem, 0, len(records))
	for _, record := range records {
		if !isFailureStatus(record.Status) {
			continue
		}
		items = append(items, mapJobFailure(record))
	}

	slices.SortFunc(items, compareFailures)
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func isFailureStatus(status job.Status) bool {
	return status == job.StatusFailed || status == job.StatusTimedOut
}

func mapJobFailure(record job.Job) JobFailureItem {
	return JobFailureItem{
		ID:               record.ID,
		JobType:          record.JobType,
		AggregateType:    record.AggregateType,
		AggregateID:      record.AggregateID,
		Status:           string(record.Status),
		LeaseOwner:       record.LeaseOwner,
		AttemptCount:     record.AttemptCount,
		MaxAttempts:      record.MaxAttempts,
		NextRunAt:        record.NextRunAt,
		LastErrorCode:    record.LastErrorCode,
		LastErrorMessage: record.LastErrorMessage,
		UpdatedAt:        record.UpdatedAt,
		LeaseExpireAt:    timePointer(record.LeaseExpireAt),
	}
}

func compareFailures(left JobFailureItem, right JobFailureItem) int {
	if !left.UpdatedAt.Equal(right.UpdatedAt) {
		if left.UpdatedAt.After(right.UpdatedAt) {
			return -1
		}
		return 1
	}
	return strings.Compare(left.ID, right.ID)
}

func timePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	cloned := value
	return &cloned
}

func translateError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, resource.ErrNotFound):
		return apperr.Wrap(err, http.StatusNotFound, "RESOURCE_NOT_FOUND", "resource not found")
	case errors.Is(err, resource.ErrScopeMismatch):
		return apperr.Wrap(err, http.StatusConflict, "TENANT_SCOPE_MISMATCH", "scope mismatch")
	case errors.Is(err, resource.ErrValidation), errors.Is(err, job.ErrInvalidJob):
		return apperr.Wrap(err, http.StatusBadRequest, "REQUEST_VALIDATION_FAILED", "request validation failed")
	default:
		return apperr.Wrap(err, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}
