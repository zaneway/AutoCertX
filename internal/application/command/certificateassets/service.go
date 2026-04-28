package certificateassets

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/zaneway/AutoCertX/internal/domain/resource"
	"github.com/zaneway/AutoCertX/internal/platform/apperr"
	issueworkflow "github.com/zaneway/AutoCertX/internal/workflow"
)

// CreateRequestInput is the application-layer request payload for asset workspace issuance.
type CreateRequestInput struct {
	RequestType         string
	DomainIDs           []string
	CAAccountID         string
	AssetID             string
	CertificateType     string
	ChallengeType       string
	IdempotencyKey      string
	DeploymentTargetIDs []string
}

// AcceptedResult represents an accepted asynchronous issuance action.
type AcceptedResult = issueworkflow.AcceptedResult

// Service exposes the T08 asset workspace write use cases.
type Service struct {
	workflows *issueworkflow.Service
}

// NewService constructs the certificate asset command service.
func NewService(workflowService *issueworkflow.Service) *Service {
	return &Service{workflows: workflowService}
}

// CreateRequest creates an issuance request and enqueues workflow bootstrap.
func (s *Service) CreateRequest(ctx context.Context, scope resource.Scope, actorID string, input CreateRequestInput) (AcceptedResult, error) {
	result, err := s.workflows.SubmitRequest(ctx, scope, actorID, issueworkflow.SubmitInput(input))
	if err != nil {
		return AcceptedResult{}, translateError(err)
	}
	return result, nil
}

// RenewAsset triggers a manual renewal request for an existing asset.
func (s *Service) RenewAsset(ctx context.Context, scope resource.Scope, actorID string, assetID string) (AcceptedResult, error) {
	result, err := s.workflows.RenewAsset(ctx, scope, actorID, strings.TrimSpace(assetID))
	if err != nil {
		return AcceptedResult{}, translateError(err)
	}
	return result, nil
}

func translateError(err error) error {
	switch {
	case errors.Is(err, resource.ErrValidation):
		return apperr.Wrap(err, http.StatusBadRequest, "REQUEST_VALIDATION_FAILED", "request validation failed", validationDetail(err))
	case errors.Is(err, resource.ErrNotFound):
		return apperr.Wrap(err, http.StatusNotFound, "RESOURCE_NOT_FOUND", "resource not found", apperr.Detail{})
	case errors.Is(err, resource.ErrConflict):
		return apperr.Wrap(err, http.StatusConflict, "RESOURCE_CONFLICT", "resource conflict", apperr.Detail{})
	case errors.Is(err, resource.ErrScopeMismatch):
		return apperr.Wrap(err, http.StatusConflict, "TENANT_SCOPE_MISMATCH", "tenant scope mismatch", apperr.Detail{})
	case errors.Is(err, resource.ErrUnavailable):
		return apperr.Wrap(err, http.StatusConflict, "RESOURCE_UNAVAILABLE", "resource unavailable", apperr.Detail{})
	default:
		return apperr.Wrap(err, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", apperr.Detail{})
	}
}

func validationDetail(err error) apperr.Detail {
	message := err.Error()
	field := "request"
	switch {
	case strings.Contains(message, "domain"):
		field = "domain_ids"
	case strings.Contains(message, "ca_account"):
		field = "ca_account_id"
	case strings.Contains(message, "asset_id"):
		field = "asset_id"
	case strings.Contains(message, "certificate_type"):
		field = "certificate_type"
	case strings.Contains(message, "challenge_type"):
		field = "challenge_type"
	case strings.Contains(message, "idempotency_key"):
		field = "idempotency_key"
	}
	return apperr.Field(field, message)
}
