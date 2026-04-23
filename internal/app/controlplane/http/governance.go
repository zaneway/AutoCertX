package httpserver

import (
	"net/http"

	caaccountscmd "github.com/zaneway/AutoCertX/internal/application/command/caaccounts"
	domainscmd "github.com/zaneway/AutoCertX/internal/application/command/domains"
	domainsquery "github.com/zaneway/AutoCertX/internal/application/query/domains"
	"github.com/zaneway/AutoCertX/internal/domain/issuer"
	"github.com/zaneway/AutoCertX/internal/domain/tenancy"
)

// governanceHandler serves domain, DNS credential, and CA account APIs.
type governanceHandler struct {
	domains    *domainscmd.Service
	caAccounts *caaccountscmd.Service
	query      *domainsquery.Service
}

// domainUpsertRequest is the control-plane write payload for domain assets.
type domainUpsertRequest struct {
	Name            string `json:"name"`
	ChallengeType   string `json:"challenge_type"`
	AutoRenew       bool   `json:"auto_renew"`
	DNSCredentialID string `json:"dns_credential_id"`
}

// domainDNSBindingRequest requests an asynchronous DNS credential binding.
type domainDNSBindingRequest struct {
	DNSCredentialID string `json:"dns_credential_id"`
}

// dnsCredentialUpsertRequest is the HTTP write model for DNS credentials.
type dnsCredentialUpsertRequest struct {
	DisplayName  string `json:"display_name"`
	ProviderType string `json:"provider_type"`
	AccessKeyID  string `json:"access_key_id"`
	Secret       string `json:"secret"`
	ScopeMode    string `json:"scope_mode"`
}

// caAccountUpsertRequest is the HTTP write model for CA accounts.
type caAccountUpsertRequest struct {
	DisplayName  string `json:"display_name"`
	DirectoryURL string `json:"directory_url"`
	Email        string `json:"email"`
}

func registerGovernanceRoutes(mux *http.ServeMux, deps Deps) {
	if deps.DomainCommands == nil && deps.CAAccountCommands == nil && deps.GovernanceQuery == nil {
		return
	}

	handler := governanceHandler{
		domains:    deps.DomainCommands,
		caAccounts: deps.CAAccountCommands,
		query:      deps.GovernanceQuery,
	}

	handleRead := func(pattern string, fn http.HandlerFunc) {
		var endpoint http.Handler = fn
		if deps.AuthService != nil && deps.AuthContextQuery != nil {
			authz := authHandler{
				authService:        deps.AuthService,
				authContextService: deps.AuthContextQuery,
			}
			// Query routes are protected in the full control-plane wiring but can
			// still run without auth in focused handler tests.
			endpoint = authz.withAuthentication(authz.withPermissions(endpoint, tenancy.PermissionAuthContextRead))
		}
		mux.Handle(pattern, endpoint)
	}

	if deps.GovernanceQuery != nil {
		handleRead("GET /api/v1/domains", handler.listDomains)
		handleRead("GET /api/v1/domains/{id}", handler.getDomain)
		handleRead("GET /api/v1/domains/{id}/validation-records", handler.listDomainValidationRecords)
		handleRead("GET /api/v1/domains/{id}/txt-operations", handler.listDomainTXTOperations)
		handleRead("GET /api/v1/domains/{id}/certificate-assets", handler.listDomainCertificateAssets)
		handleRead("GET /api/v1/dns-credentials", handler.listDNSCredentials)
		handleRead("GET /api/v1/ca-accounts", handler.listCAAccounts)
		handleRead("GET /api/v1/ca-accounts/{id}", handler.getCAAccount)
		handleRead("GET /api/v1/ca-accounts/{id}/capabilities", handler.getCAAccountCapabilities)
	}

	if deps.DomainCommands != nil {
		mux.HandleFunc("POST /api/v1/domains", handler.createDomain)
		mux.HandleFunc("PUT /api/v1/domains/{id}", handler.updateDomain)
		mux.HandleFunc("POST /api/v1/domains/{id}/bind-dns-credential", handler.bindDomainDNSCredential)

		mux.HandleFunc("POST /api/v1/dns-credentials", handler.createDNSCredential)
		mux.HandleFunc("PUT /api/v1/dns-credentials/{id}", handler.updateDNSCredential)
		mux.HandleFunc("POST /api/v1/dns-credentials/{id}/rotate", handler.rotateDNSCredential)
	}

	if deps.CAAccountCommands != nil {
		mux.HandleFunc("POST /api/v1/ca-accounts", handler.createCAAccount)
	}
}

func (h governanceHandler) listDomains(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveReadScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	items, err := h.query.ListDomains(r.Context(), scope)
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

	// The command layer validates both the domain payload and any referenced DNS
	// credential so the stored governance record stays internally consistent.
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
	scope, _, err := resolveReadScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	id := r.PathValue("id")
	if err := validateGovernanceID(id); err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	asset, err := h.query.GetDomain(r.Context(), scope, id)
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

	// Domain updates reuse the same binding resolution path as create to enforce
	// the same provider and scope invariants on every write.
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

	// DNS binding is modeled as an accepted async action because the downstream
	// certificate workflow may outlive the request lifecycle.
	result, err := h.domains.BindDNSCredential(r.Context(), scope, actorID, id, req.DNSCredentialID)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeAcceptedEnvelope(w, r, result.JobID)
}

func (h governanceHandler) listDomainValidationRecords(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveReadScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	id := r.PathValue("id")
	if err := validateGovernanceID(id); err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	items, err := h.query.ListValidationRecords(r.Context(), scope, id)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeListEnvelope(w, r, http.StatusOK, items)
}

func (h governanceHandler) listDomainTXTOperations(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveReadScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	id := r.PathValue("id")
	if err := validateGovernanceID(id); err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	items, err := h.query.ListTXTOperations(r.Context(), scope, id)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeListEnvelope(w, r, http.StatusOK, items)
}

func (h governanceHandler) listDomainCertificateAssets(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveReadScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	id := r.PathValue("id")
	if err := validateGovernanceID(id); err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	items, err := h.query.ListCertificateAssets(r.Context(), scope, id)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeListEnvelope(w, r, http.StatusOK, items)
}

func (h governanceHandler) listDNSCredentials(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveReadScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	items, err := h.query.ListDNSCredentials(r.Context(), scope)
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

	// Rotation also returns an accepted job handle because secret rollover is
	// exposed as an auditable long-running operation in the control plane.
	result, err := h.domains.RotateDNSCredential(r.Context(), scope, actorID, id)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeAcceptedEnvelope(w, r, result.JobID)
}

func (h governanceHandler) listCAAccounts(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveReadScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	items, err := h.query.ListCAAccounts(r.Context(), scope)
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
	scope, _, err := resolveReadScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	id := r.PathValue("id")
	if err := validateGovernanceID(id); err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	account, err := h.query.GetCAAccount(r.Context(), scope, id)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeObjectEnvelope(w, r, http.StatusOK, account)
}

func (h governanceHandler) getCAAccountCapabilities(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveReadScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	id := r.PathValue("id")
	if err := validateGovernanceID(id); err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	capabilities, err := h.query.GetCAAccountCapabilities(r.Context(), scope, id)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeObjectEnvelope(w, r, http.StatusOK, capabilities)
}
