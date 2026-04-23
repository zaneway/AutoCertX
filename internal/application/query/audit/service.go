package audit

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	auditdomain "github.com/zaneway/AutoCertX/internal/domain/audit"
	"github.com/zaneway/AutoCertX/internal/domain/resource"
	"github.com/zaneway/AutoCertX/internal/platform/apperr"
)

// Filter is the application-level audit query filter.
type Filter struct {
	ActorID      string
	Action       string
	ResourceType string
	ResourceID   string
	StartTime    *time.Time
	EndTime      *time.Time
}

// Service exposes T06 audit read models.
type Service struct {
	audit *auditdomain.Service
}

// NewService constructs the audit query service.
func NewService(auditService *auditdomain.Service) *Service {
	return &Service{
		audit: auditService,
	}
}

// ListAuditEvents returns audit events under the given scope.
func (s *Service) ListAuditEvents(_ context.Context, scope resource.Scope, filter Filter) ([]auditdomain.Event, error) {
	// Normalize the filter at the application boundary so downstream domain logic
	// only sees trimmed, typed criteria.
	items, err := s.audit.ListEvents(scope, auditdomain.EventFilter{
		ActorID:      strings.TrimSpace(filter.ActorID),
		Action:       strings.TrimSpace(filter.Action),
		ResourceType: strings.TrimSpace(filter.ResourceType),
		ResourceID:   strings.TrimSpace(filter.ResourceID),
		StartTime:    filter.StartTime,
		EndTime:      filter.EndTime,
	})
	if err != nil {
		return nil, translateAuditError(err)
	}
	return items, nil
}

// GetAuditEvent returns one audit event detail.
func (s *Service) GetAuditEvent(_ context.Context, scope resource.Scope, id string) (auditdomain.Event, error) {
	item, err := s.audit.GetEvent(scope, strings.TrimSpace(id))
	if err != nil {
		return auditdomain.Event{}, translateAuditError(err)
	}
	return item, nil
}

func translateAuditError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, resource.ErrNotFound):
		return apperr.Wrap(err, http.StatusNotFound, "RESOURCE_NOT_FOUND", "resource not found")
	case errors.Is(err, resource.ErrScopeMismatch):
		return apperr.Wrap(err, http.StatusConflict, "TENANT_SCOPE_MISMATCH", "scope mismatch")
	case errors.Is(err, resource.ErrValidation):
		return apperr.Wrap(err, http.StatusBadRequest, "REQUEST_VALIDATION_FAILED", "request validation failed", validationDetail(err))
	default:
		return apperr.Wrap(err, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}

func validationDetail(err error) apperr.Detail {
	message := err.Error()
	field := ""
	switch {
	case strings.Contains(message, "actor_id"):
		field = "actor_id"
	case strings.Contains(message, "action"):
		field = "action"
	case strings.Contains(message, "resource_type"):
		field = "resource_type"
	case strings.Contains(message, "resource_id"):
		field = "resource_id"
	case strings.Contains(message, "start_time"):
		field = "start_time"
	case strings.Contains(message, "end_time"):
		field = "end_time"
	case strings.Contains(message, "id"):
		field = "id"
	}
	return apperr.Field(field, message)
}
