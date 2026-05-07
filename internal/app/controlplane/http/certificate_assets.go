package httpserver

import (
	"errors"
	"net/http"
	"strings"

	certificateassetscmd "github.com/zaneway/AutoCertX/internal/application/command/certificateassets"
	deploymentservice "github.com/zaneway/AutoCertX/internal/deployment"
	"github.com/zaneway/AutoCertX/internal/domain/resource"
	"github.com/zaneway/AutoCertX/internal/platform/apperr"
	"github.com/zaneway/AutoCertX/internal/platform/httpx"
)

// certificateAssetHandler serves the Phase A issuance write entry points.
type certificateAssetHandler struct {
	commands    *certificateassetscmd.Service
	deployments *deploymentservice.Service
}

// certificateRequestCreateRequest is the HTTP write model for creating one issuance request.
type certificateRequestCreateRequest struct {
	RequestType         string   `json:"request_type"`
	DomainIDs           []string `json:"domain_ids"`
	CAAccountID         string   `json:"ca_account_id"`
	AssetID             string   `json:"asset_id"`
	CertificateType     string   `json:"certificate_type"`
	ChallengeType       string   `json:"challenge_type"`
	IdempotencyKey      string   `json:"idempotency_key"`
	DeploymentTargetIDs []string `json:"deployment_target_ids"`
}

type certificateAssetDeployRequest struct {
	VersionID      string `json:"version_id"`
	TargetID       string `json:"target_id"`
	IdempotencyKey string `json:"idempotency_key"`
}

type deploymentAcceptedEnvelope struct {
	RequestID    string `json:"request_id"`
	Status       string `json:"status"`
	DeploymentID string `json:"deployment_id"`
	JobID        string `json:"job_id"`
}

func registerCertificateAssetRoutes(mux *http.ServeMux, deps Deps) {
	if deps.CertificateAssets == nil && deps.DeploymentService == nil {
		return
	}

	handler := certificateAssetHandler{
		commands:    deps.CertificateAssets,
		deployments: deps.DeploymentService,
	}

	if deps.CertificateAssets != nil {
		mux.HandleFunc("POST /api/v1/certificate-assets/requests", handler.createCertificateRequest)
		mux.HandleFunc("POST /api/v1/certificate-assets/{assetId}/renew", handler.renewCertificateAsset)
	}
	if deps.DeploymentService != nil {
		mux.HandleFunc("POST /api/v1/certificate-assets/{assetId}/deploy", handler.deployCertificateAsset)
	}
}

func (h certificateAssetHandler) createCertificateRequest(w http.ResponseWriter, r *http.Request) {
	scope, actorID, err := resolveGovernanceScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	var req certificateRequestCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeValidationError(w, r, "body", "invalid_json")
		return
	}

	result, err := h.commands.CreateRequest(r.Context(), scope, actorID, certificateassetscmd.CreateRequestInput{
		RequestType:         req.RequestType,
		DomainIDs:           req.DomainIDs,
		CAAccountID:         req.CAAccountID,
		AssetID:             req.AssetID,
		CertificateType:     req.CertificateType,
		ChallengeType:       req.ChallengeType,
		IdempotencyKey:      req.IdempotencyKey,
		DeploymentTargetIDs: req.DeploymentTargetIDs,
	})
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeAcceptedEnvelope(w, r, result.JobID)
}

func (h certificateAssetHandler) deployCertificateAsset(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveGovernanceScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	assetID := r.PathValue("assetId")
	if err := validateGovernanceID(assetID); err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	var req certificateAssetDeployRequest
	if err := decodeJSON(r, &req); err != nil {
		writeValidationError(w, r, "body", "invalid_json")
		return
	}
	if err := validateGovernanceID(req.VersionID); err != nil {
		writeGovernanceError(w, r, apperr.Wrap(err, http.StatusBadRequest, "REQUEST_VALIDATION_FAILED", "request validation failed", apperr.Field("version_id", "invalid resource id")))
		return
	}
	if err := validateGovernanceID(req.TargetID); err != nil {
		writeGovernanceError(w, r, apperr.Wrap(err, http.StatusBadRequest, "REQUEST_VALIDATION_FAILED", "request validation failed", apperr.Field("target_id", "invalid resource id")))
		return
	}

	result, err := h.deployments.StartNGINXDeployment(r.Context(), scope, deploymentservice.StartInput{
		AssetID:        assetID,
		VersionID:      req.VersionID,
		TargetID:       req.TargetID,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		writeGovernanceError(w, r, translateDeploymentError(err))
		return
	}
	_ = httpx.WriteJSON(w, http.StatusAccepted, deploymentAcceptedEnvelope{
		RequestID:    requestID(r),
		Status:       "accepted",
		DeploymentID: result.DeploymentID,
		JobID:        result.JobID,
	})
}

func translateDeploymentError(err error) error {
	if _, ok := apperr.As(err); ok {
		return err
	}
	switch {
	case errors.Is(err, resource.ErrValidation):
		return apperr.Wrap(err, http.StatusBadRequest, "REQUEST_VALIDATION_FAILED", "request validation failed", deploymentValidationDetail(err))
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

func deploymentValidationDetail(err error) apperr.Detail {
	message := err.Error()
	field := "request"
	switch {
	case strings.Contains(message, "asset_id"):
		field = "asset_id"
	case strings.Contains(message, "version_id"):
		field = "version_id"
	case strings.Contains(message, "target_id"):
		field = "target_id"
	case strings.Contains(message, "idempotency_key"):
		field = "idempotency_key"
	case strings.Contains(message, "target"):
		field = "target_id"
	}
	return apperr.Field(field, message)
}

func (h certificateAssetHandler) renewCertificateAsset(w http.ResponseWriter, r *http.Request) {
	scope, actorID, err := resolveGovernanceScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	assetID := r.PathValue("assetId")
	if err := validateGovernanceID(assetID); err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	result, err := h.commands.RenewAsset(r.Context(), scope, actorID, assetID)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeAcceptedEnvelope(w, r, result.JobID)
}
