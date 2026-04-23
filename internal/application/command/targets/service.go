package targets

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/zaneway/AutoCertX/internal/domain/deploymenttarget"
	"github.com/zaneway/AutoCertX/internal/domain/resource"
	"github.com/zaneway/AutoCertX/internal/platform/apperr"
)

// UpsertInput is the application-layer deployment target write model.
type UpsertInput = deploymenttarget.UpsertInput

// Service orchestrates deployment target governance commands.
type Service struct {
	targets *deploymenttarget.Service
}

// NewService constructs the deployment target command service.
func NewService(targetService *deploymenttarget.Service) *Service {
	return &Service{
		targets: targetService,
	}
}

// ListTargets returns deployment targets under the caller scope.
func (s *Service) ListTargets(_ context.Context, scope resource.Scope) ([]deploymenttarget.Target, error) {
	items, err := s.targets.List(scope)
	if err != nil {
		return nil, translateTargetError(err)
	}
	return items, nil
}

// GetTarget returns one deployment target under the caller scope.
func (s *Service) GetTarget(_ context.Context, scope resource.Scope, id string) (deploymenttarget.Target, error) {
	target, err := s.targets.Get(scope, strings.TrimSpace(id))
	if err != nil {
		return deploymenttarget.Target{}, translateTargetError(err)
	}
	return target, nil
}

// CreateTarget creates a deployment target under the caller scope.
func (s *Service) CreateTarget(_ context.Context, scope resource.Scope, input UpsertInput) (deploymenttarget.Target, error) {
	target, err := s.targets.Create(scope, input)
	if err != nil {
		return deploymenttarget.Target{}, translateTargetError(err)
	}
	return target, nil
}

// UpdateTarget updates a deployment target under the caller scope.
func (s *Service) UpdateTarget(_ context.Context, scope resource.Scope, id string, input UpsertInput) (deploymenttarget.Target, error) {
	target, err := s.targets.Update(scope, strings.TrimSpace(id), input)
	if err != nil {
		return deploymenttarget.Target{}, translateTargetError(err)
	}
	return target, nil
}

func translateTargetError(err error) error {
	switch {
	case errors.Is(err, resource.ErrValidation):
		return apperr.Wrap(err, http.StatusBadRequest, "REQUEST_VALIDATION_FAILED", "request validation failed", validationDetail(err))
	case errors.Is(err, resource.ErrNotFound):
		return apperr.Wrap(err, http.StatusNotFound, "RESOURCE_NOT_FOUND", "resource not found", apperr.Detail{})
	case errors.Is(err, resource.ErrConflict):
		return apperr.Wrap(err, http.StatusConflict, "RESOURCE_CONFLICT", "resource conflict", apperr.Detail{})
	case errors.Is(err, resource.ErrScopeMismatch):
		return apperr.Wrap(err, http.StatusConflict, "TENANT_SCOPE_MISMATCH", "tenant scope mismatch", apperr.Detail{})
	default:
		return apperr.Wrap(err, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", apperr.Detail{})
	}
}

func validationDetail(err error) apperr.Detail {
	message := err.Error()
	field := "request"
	switch {
	case strings.Contains(message, "name"):
		field = "name"
	case strings.Contains(message, "target_type"):
		field = "target_type"
	case strings.Contains(message, "agent"):
		field = "agent"
	case strings.Contains(message, "config_path"):
		field = "config_path"
	case strings.Contains(message, "certificate_path"):
		field = "certificate_path"
	case strings.Contains(message, "private_key_path"):
		field = "private_key_path"
	case strings.Contains(message, "keystore_path"):
		field = "keystore_path"
	}
	return apperr.Field(field, message)
}
