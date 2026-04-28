package httpserver

import (
	"net/http"

	certificateassetscmd "github.com/zaneway/AutoCertX/internal/application/command/certificateassets"
)

// certificateAssetHandler serves the Phase A issuance write entry points.
type certificateAssetHandler struct {
	commands *certificateassetscmd.Service
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

func registerCertificateAssetRoutes(mux *http.ServeMux, deps Deps) {
	if deps.CertificateAssets == nil {
		return
	}

	handler := certificateAssetHandler{
		commands: deps.CertificateAssets,
	}

	mux.HandleFunc("POST /api/v1/certificate-assets/requests", handler.createCertificateRequest)
	mux.HandleFunc("POST /api/v1/certificate-assets/{assetId}/renew", handler.renewCertificateAsset)
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
