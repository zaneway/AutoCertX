package caaccounts

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/zaneway/AutoCertX/internal/domain/issuer"
	"github.com/zaneway/AutoCertX/internal/domain/resource"
	"github.com/zaneway/AutoCertX/internal/platform/apperr"
)

// Service orchestrates T04 CA account commands.
type Service struct {
	issuer *issuer.Service
}

// NewService constructs the CA account command service.
func NewService(issuerService *issuer.Service) *Service {
	return &Service{
		issuer: issuerService,
	}
}

// ListCAAccounts returns CA accounts under the given scope.
func (s *Service) ListCAAccounts(_ context.Context, scope resource.Scope) ([]issuer.Account, error) {
	items, err := s.issuer.List(scope)
	if err != nil {
		return nil, translateIssuerError(err)
	}
	return items, nil
}

// CreateCAAccount creates a CA account under the given scope.
func (s *Service) CreateCAAccount(_ context.Context, scope resource.Scope, input issuer.UpsertInput) (issuer.Account, error) {
	account, err := s.issuer.Create(scope, input)
	if err != nil {
		return issuer.Account{}, translateIssuerError(err)
	}
	return account, nil
}

// GetCAAccount returns one CA account.
func (s *Service) GetCAAccount(_ context.Context, scope resource.Scope, id string) (issuer.Account, error) {
	account, err := s.issuer.Get(scope, strings.TrimSpace(id))
	if err != nil {
		return issuer.Account{}, translateIssuerError(err)
	}
	return account, nil
}

// GetCAAccountCapabilities returns the backend capability metadata for one account.
func (s *Service) GetCAAccountCapabilities(_ context.Context, scope resource.Scope, id string) (issuer.CapabilitySet, error) {
	capabilities, err := s.issuer.GetCapabilities(scope, strings.TrimSpace(id))
	if err != nil {
		return issuer.CapabilitySet{}, translateIssuerError(err)
	}
	return capabilities, nil
}

func translateIssuerError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, resource.ErrNotFound):
		return apperr.Wrap(err, http.StatusNotFound, "RESOURCE_NOT_FOUND", "resource not found")
	case errors.Is(err, resource.ErrConflict):
		return apperr.Wrap(err, http.StatusConflict, "RESOURCE_CONFLICT", "resource conflict")
	case errors.Is(err, resource.ErrScopeMismatch):
		return apperr.Wrap(err, http.StatusConflict, "TENANT_SCOPE_MISMATCH", "scope mismatch")
	case errors.Is(err, resource.ErrValidation):
		return apperr.Wrap(err, http.StatusBadRequest, "REQUEST_VALIDATION_FAILED", "request validation failed", validationDetail(err))
	default:
		// Domain errors are flattened here so HTTP handlers can remain transport-
		// focused and free of issuer-specific branching.
		return apperr.Wrap(err, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}

func validationDetail(err error) apperr.Detail {
	message := err.Error()
	field := ""
	switch {
	case strings.Contains(message, "display_name"):
		field = "display_name"
	case strings.Contains(message, "directory_url"):
		field = "directory_url"
	case strings.Contains(message, "email"):
		field = "email"
	}
	return apperr.Field(field, message)
}
