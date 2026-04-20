package httpserver

import (
	"net/http"

	caaccountscmd "github.com/zaneway/AutoCertX/internal/application/command/caaccounts"
	domainscmd "github.com/zaneway/AutoCertX/internal/application/command/domains"
	"github.com/zaneway/AutoCertX/internal/domain/issuer"
)

type governanceHandler struct {
	domains    *domainscmd.Service
	caAccounts *caaccountscmd.Service
}

type domainUpsertRequest struct {
	Name            string `json:"name"`
	ChallengeType   string `json:"challenge_type"`
	AutoRenew       bool   `json:"auto_renew"`
	DNSCredentialID string `json:"dns_credential_id"`
}

type domainDNSBindingRequest struct {
	DNSCredentialID string `json:"dns_credential_id"`
}

type dnsCredentialUpsertRequest struct {
	DisplayName  string `json:"display_name"`
	ProviderType string `json:"provider_type"`
	AccessKeyID  string `json:"access_key_id"`
	Secret       string `json:"secret"`
	ScopeMode    string `json:"scope_mode"`
}

type caAccountUpsertRequest struct {
	DisplayName  string `json:"display_name"`
	DirectoryURL string `json:"directory_url"`
	Email        string `json:"email"`
}

func registerGovernanceRoutes(mux *http.ServeMux, deps Deps) {
	if deps.DomainCommands == nil || deps.CAAccountCommands == nil {
		return
	}

	handler := governanceHandler{
		domains:    deps.DomainCommands,
		caAccounts: deps.CAAccountCommands,
	}

	mux.HandleFunc("GET /api/v1/domains", handler.listDomains)
	mux.HandleFunc("POST /api/v1/domains", handler.createDomain)
	mux.HandleFunc("GET /api/v1/domains/{id}", handler.getDomain)
	mux.HandleFunc("PUT /api/v1/domains/{id}", handler.updateDomain)
	mux.HandleFunc("POST /api/v1/domains/{id}/bind-dns-credential", handler.bindDomainDNSCredential)
	mux.HandleFunc("GET /api/v1/domains/{id}/validation-records", handler.listDomainValidationRecords)
	mux.HandleFunc("GET /api/v1/domains/{id}/txt-operations", handler.listDomainTXTOperations)
	mux.HandleFunc("GET /api/v1/domains/{id}/certificate-assets", handler.listDomainCertificateAssets)

	mux.HandleFunc("GET /api/v1/dns-credentials", handler.listDNSCredentials)
	mux.HandleFunc("POST /api/v1/dns-credentials", handler.createDNSCredential)
	mux.HandleFunc("PUT /api/v1/dns-credentials/{id}", handler.updateDNSCredential)
	mux.HandleFunc("POST /api/v1/dns-credentials/{id}/rotate", handler.rotateDNSCredential)

	mux.HandleFunc("GET /api/v1/ca-accounts", handler.listCAAccounts)
	mux.HandleFunc("POST /api/v1/ca-accounts", handler.createCAAccount)
	mux.HandleFunc("GET /api/v1/ca-accounts/{id}", handler.getCAAccount)
	mux.HandleFunc("GET /api/v1/ca-accounts/{id}/capabilities", handler.getCAAccountCapabilities)
}

func (h governanceHandler) listDomains(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveGovernanceScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	items, err := h.domains.ListDomains(r.Context(), scope)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeListEnvelope(w, r, http.StatusOK, items)
}

func (h governanceHandler) createDomain(w http.ResponseWriter, r *http.Request) {
	scope, actorID, err := resolveGovernanceScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	var req domainUpsertRequest
	if err := decodeJSON(r, &req); err != nil {
		writeValidationError(w, r, "body", "invalid_json")
		return
	}

	asset, err := h.domains.CreateDomain(r.Context(), scope, actorID, domainscmd.DomainUpsertInput{
		Name:            req.Name,
		ChallengeType:   req.ChallengeType,
		AutoRenew:       req.AutoRenew,
		DNSCredentialID: req.DNSCredentialID,
	})
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeObjectEnvelope(w, r, http.StatusCreated, asset)
}

func (h governanceHandler) getDomain(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveGovernanceScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	id := r.PathValue("id")
	if err := validateGovernanceID(id); err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	asset, err := h.domains.GetDomain(r.Context(), scope, id)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeObjectEnvelope(w, r, http.StatusOK, asset)
}

func (h governanceHandler) updateDomain(w http.ResponseWriter, r *http.Request) {
	scope, actorID, err := resolveGovernanceScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	id := r.PathValue("id")
	if err := validateGovernanceID(id); err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	var req domainUpsertRequest
	if err := decodeJSON(r, &req); err != nil {
		writeValidationError(w, r, "body", "invalid_json")
		return
	}

	asset, err := h.domains.UpdateDomain(r.Context(), scope, actorID, id, domainscmd.DomainUpsertInput{
		Name:            req.Name,
		ChallengeType:   req.ChallengeType,
		AutoRenew:       req.AutoRenew,
		DNSCredentialID: req.DNSCredentialID,
	})
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeObjectEnvelope(w, r, http.StatusOK, asset)
}

func (h governanceHandler) bindDomainDNSCredential(w http.ResponseWriter, r *http.Request) {
	scope, actorID, err := resolveGovernanceScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	id := r.PathValue("id")
	if err := validateGovernanceID(id); err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	var req domainDNSBindingRequest
	if err := decodeJSON(r, &req); err != nil {
		writeValidationError(w, r, "body", "invalid_json")
		return
	}

	result, err := h.domains.BindDNSCredential(r.Context(), scope, actorID, id, req.DNSCredentialID)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeAcceptedEnvelope(w, r, result.JobID)
}

func (h governanceHandler) listDomainValidationRecords(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveGovernanceScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	id := r.PathValue("id")
	if err := validateGovernanceID(id); err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	items, err := h.domains.ListValidationRecords(r.Context(), scope, id)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeListEnvelope(w, r, http.StatusOK, items)
}

func (h governanceHandler) listDomainTXTOperations(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveGovernanceScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	id := r.PathValue("id")
	if err := validateGovernanceID(id); err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	items, err := h.domains.ListTXTOperations(r.Context(), scope, id)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeListEnvelope(w, r, http.StatusOK, items)
}

func (h governanceHandler) listDomainCertificateAssets(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveGovernanceScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	id := r.PathValue("id")
	if err := validateGovernanceID(id); err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	items, err := h.domains.ListCertificateAssets(r.Context(), scope, id)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeListEnvelope(w, r, http.StatusOK, items)
}

func (h governanceHandler) listDNSCredentials(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveGovernanceScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	items, err := h.domains.ListDNSCredentials(r.Context(), scope)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeListEnvelope(w, r, http.StatusOK, items)
}

func (h governanceHandler) createDNSCredential(w http.ResponseWriter, r *http.Request) {
	scope, actorID, err := resolveGovernanceScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	var req dnsCredentialUpsertRequest
	if err := decodeJSON(r, &req); err != nil {
		writeValidationError(w, r, "body", "invalid_json")
		return
	}

	credential, err := h.domains.CreateDNSCredential(r.Context(), scope, actorID, domainscmd.DNSCredentialUpsertInput{
		DisplayName:  req.DisplayName,
		ProviderType: req.ProviderType,
		AccessKeyID:  req.AccessKeyID,
		Secret:       req.Secret,
		ScopeMode:    req.ScopeMode,
	})
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeObjectEnvelope(w, r, http.StatusCreated, credential)
}

func (h governanceHandler) updateDNSCredential(w http.ResponseWriter, r *http.Request) {
	scope, actorID, err := resolveGovernanceScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	id := r.PathValue("id")
	if err := validateGovernanceID(id); err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	var req dnsCredentialUpsertRequest
	if err := decodeJSON(r, &req); err != nil {
		writeValidationError(w, r, "body", "invalid_json")
		return
	}

	credential, err := h.domains.UpdateDNSCredential(r.Context(), scope, actorID, id, domainscmd.DNSCredentialUpsertInput{
		DisplayName:  req.DisplayName,
		ProviderType: req.ProviderType,
		AccessKeyID:  req.AccessKeyID,
		Secret:       req.Secret,
		ScopeMode:    req.ScopeMode,
	})
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeObjectEnvelope(w, r, http.StatusOK, credential)
}

func (h governanceHandler) rotateDNSCredential(w http.ResponseWriter, r *http.Request) {
	scope, actorID, err := resolveGovernanceScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	id := r.PathValue("id")
	if err := validateGovernanceID(id); err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	result, err := h.domains.RotateDNSCredential(r.Context(), scope, actorID, id)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeAcceptedEnvelope(w, r, result.JobID)
}

func (h governanceHandler) listCAAccounts(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveGovernanceScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	items, err := h.caAccounts.ListCAAccounts(r.Context(), scope)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeListEnvelope(w, r, http.StatusOK, items)
}

func (h governanceHandler) createCAAccount(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveGovernanceScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	var req caAccountUpsertRequest
	if err := decodeJSON(r, &req); err != nil {
		writeValidationError(w, r, "body", "invalid_json")
		return
	}

	account, err := h.caAccounts.CreateCAAccount(r.Context(), scope, issuer.UpsertInput{
		DisplayName:  req.DisplayName,
		DirectoryURL: req.DirectoryURL,
		Email:        req.Email,
	})
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeObjectEnvelope(w, r, http.StatusCreated, account)
}

func (h governanceHandler) getCAAccount(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveGovernanceScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	id := r.PathValue("id")
	if err := validateGovernanceID(id); err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	account, err := h.caAccounts.GetCAAccount(r.Context(), scope, id)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeObjectEnvelope(w, r, http.StatusOK, account)
}

func (h governanceHandler) getCAAccountCapabilities(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveGovernanceScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	id := r.PathValue("id")
	if err := validateGovernanceID(id); err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	capabilities, err := h.caAccounts.GetCAAccountCapabilities(r.Context(), scope, id)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeObjectEnvelope(w, r, http.StatusOK, capabilities)
}
