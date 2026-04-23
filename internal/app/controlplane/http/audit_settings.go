package httpserver

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	settingscmd "github.com/zaneway/AutoCertX/internal/application/command/settings"
	auditquery "github.com/zaneway/AutoCertX/internal/application/query/audit"
	settingsquery "github.com/zaneway/AutoCertX/internal/application/query/settings"
	"github.com/zaneway/AutoCertX/internal/domain/identity"
	"github.com/zaneway/AutoCertX/internal/domain/resource"
	"github.com/zaneway/AutoCertX/internal/domain/tenancy"
	"github.com/zaneway/AutoCertX/internal/platform/apperr"
)

// auditSettingsHandler serves audit query and settings management APIs.
type auditSettingsHandler struct {
	audit         *auditquery.Service
	settings      *settingscmd.Service
	settingsQuery *settingsquery.Service
}

// webhookUpsertRequest is the HTTP write model for outbound audit webhooks.
type webhookUpsertRequest struct {
	Name       string   `json:"name"`
	URL        string   `json:"url"`
	Secret     string   `json:"secret"`
	EventTypes []string `json:"event_types"`
	Enabled    bool     `json:"enabled"`
}

// renewalWindowSettingsRequest configures renewal scan lead time and cadence.
type renewalWindowSettingsRequest struct {
	DaysBeforeExpiry   int `json:"days_before_expiry"`
	ScanIntervalMinute int `json:"scan_interval_minutes"`
}

// securitySettingsRequest patches tenant security baseline settings.
type securitySettingsRequest struct {
	EnforceStrongPassword *bool `json:"enforce_strong_password"`
	PasswordRotationDays  *int  `json:"password_rotation_days"`
	MaxSessionCount       *int  `json:"max_session_count"`
}

// auditExportRequest carries synchronous CSV export filter criteria.
type auditExportRequest struct {
	ActorID      string `json:"actor_id"`
	Action       string `json:"action"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	StartTime    string `json:"start_time"`
	EndTime      string `json:"end_time"`
}

func registerAuditSettingsRoutes(mux *http.ServeMux, deps Deps) {
	if deps.AuthService == nil || deps.AuthContextQuery == nil {
		return
	}

	handler := auditSettingsHandler{
		audit:         deps.AuditQuery,
		settings:      deps.SettingsCommands,
		settingsQuery: deps.SettingsQuery,
	}
	authz := authHandler{
		authService:        deps.AuthService,
		authContextService: deps.AuthContextQuery,
	}

	if deps.AuditQuery != nil {
		mux.Handle("GET /api/v1/audit-events", authz.withAuthentication(authz.withPermissions(
			http.HandlerFunc(handler.listAuditEvents),
			tenancy.PermissionAuditRead,
		)))
		mux.Handle("GET /api/v1/audit-events/{id}", authz.withAuthentication(authz.withPermissions(
			http.HandlerFunc(handler.getAuditEvent),
			tenancy.PermissionAuditRead,
		)))
	}
	if deps.SettingsCommands != nil {
		mux.Handle("POST /api/v1/audit-events/export-csv", authz.withAuthentication(authz.withPermissions(
			http.HandlerFunc(handler.exportAuditCSV),
			tenancy.PermissionAuditExport,
		)))
		mux.Handle("POST /api/v1/settings/webhooks", authz.withAuthentication(authz.withPermissions(
			http.HandlerFunc(handler.createWebhookEndpoint),
			tenancy.PermissionSettingsWrite,
		)))
		mux.Handle("PUT /api/v1/settings/webhooks/{id}", authz.withAuthentication(authz.withPermissions(
			http.HandlerFunc(handler.updateWebhookEndpoint),
			tenancy.PermissionSettingsWrite,
		)))
		mux.Handle("PUT /api/v1/settings/renewal-window", authz.withAuthentication(authz.withPermissions(
			http.HandlerFunc(handler.updateRenewalWindowSettings),
			tenancy.PermissionSettingsWrite,
		)))
		mux.Handle("PUT /api/v1/settings/security", authz.withAuthentication(authz.withPermissions(
			http.HandlerFunc(handler.updateSecuritySettings),
			tenancy.PermissionSettingsWrite,
		)))
	}
	if deps.SettingsQuery != nil {
		mux.Handle("GET /api/v1/settings/webhooks", authz.withAuthentication(authz.withPermissions(
			http.HandlerFunc(handler.listWebhookEndpoints),
			tenancy.PermissionSettingsRead,
		)))
		mux.Handle("GET /api/v1/settings/renewal-window", authz.withAuthentication(authz.withPermissions(
			http.HandlerFunc(handler.getRenewalWindowSettings),
			tenancy.PermissionSettingsRead,
		)))
		mux.Handle("GET /api/v1/settings/security", authz.withAuthentication(authz.withPermissions(
			http.HandlerFunc(handler.getSecuritySettings),
			tenancy.PermissionSettingsRead,
		)))
	}
}

func (h auditSettingsHandler) listAuditEvents(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolvePrincipalScope(r)
	if err != nil {
		writeMappedError(w, r, err)
		return
	}

	filter, err := auditQueryFilterFromRequest(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	items, err := h.audit.ListAuditEvents(r.Context(), scope, filter)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeListEnvelope(w, r, http.StatusOK, items)
}

func (h auditSettingsHandler) getAuditEvent(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolvePrincipalScope(r)
	if err != nil {
		writeMappedError(w, r, err)
		return
	}

	id := r.PathValue("id")
	if err := validateGovernanceID(id); err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	item, err := h.audit.GetAuditEvent(r.Context(), scope, id)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeObjectEnvelope(w, r, http.StatusOK, item)
}

func (h auditSettingsHandler) exportAuditCSV(w http.ResponseWriter, r *http.Request) {
	scope, actorID, err := resolvePrincipalScope(r)
	if err != nil {
		writeMappedError(w, r, err)
		return
	}

	var req auditExportRequest
	if err := decodeJSONOrEmpty(r, &req); err != nil {
		writeValidationError(w, r, "body", "invalid_json")
		return
	}

	// Export reuses the same filter semantics as the audit list API so operators
	// can move from screen filtering to artifact generation without semantic drift.
	filter, err := auditExportFilterFromRequest(req)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	result, err := h.settings.ExportAuditCSV(r.Context(), scope, actorID, filter)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	// The payload is streamed directly while tracking metadata is surfaced in
	// headers for download correlation and later troubleshooting.
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+result.Filename+`"`)
	w.Header().Set("X-Export-Record-Id", result.Record.ID)
	w.Header().Set("X-Export-No", result.Record.ExportNo)
	w.Header().Set("X-Export-Storage-Ref", result.Record.StorageRef)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(result.Content)
}

func (h auditSettingsHandler) listWebhookEndpoints(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolvePrincipalScope(r)
	if err != nil {
		writeMappedError(w, r, err)
		return
	}

	items, err := h.settingsQuery.ListWebhookEndpoints(r.Context(), scope)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeListEnvelope(w, r, http.StatusOK, items)
}

func (h auditSettingsHandler) createWebhookEndpoint(w http.ResponseWriter, r *http.Request) {
	scope, actorID, err := resolvePrincipalScope(r)
	if err != nil {
		writeMappedError(w, r, err)
		return
	}

	var req webhookUpsertRequest
	if err := decodeJSON(r, &req); err != nil {
		writeValidationError(w, r, "body", "invalid_json")
		return
	}

	item, err := h.settings.CreateWebhookEndpoint(r.Context(), scope, actorID, settingscmd.WebhookUpsertInput{
		Name:       req.Name,
		URL:        req.URL,
		Secret:     req.Secret,
		EventTypes: req.EventTypes,
		Enabled:    req.Enabled,
	})
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeObjectEnvelope(w, r, http.StatusCreated, item)
}

func (h auditSettingsHandler) updateWebhookEndpoint(w http.ResponseWriter, r *http.Request) {
	scope, actorID, err := resolvePrincipalScope(r)
	if err != nil {
		writeMappedError(w, r, err)
		return
	}

	id := r.PathValue("id")
	if err := validateGovernanceID(id); err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	var req webhookUpsertRequest
	if err := decodeJSON(r, &req); err != nil {
		writeValidationError(w, r, "body", "invalid_json")
		return
	}

	item, err := h.settings.UpdateWebhookEndpoint(r.Context(), scope, actorID, id, settingscmd.WebhookUpsertInput{
		Name:       req.Name,
		URL:        req.URL,
		Secret:     req.Secret,
		EventTypes: req.EventTypes,
		Enabled:    req.Enabled,
	})
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeObjectEnvelope(w, r, http.StatusOK, item)
}

func (h auditSettingsHandler) getRenewalWindowSettings(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolvePrincipalScope(r)
	if err != nil {
		writeMappedError(w, r, err)
		return
	}

	item, err := h.settingsQuery.GetRenewalWindowSettings(r.Context(), scope)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeObjectEnvelope(w, r, http.StatusOK, item)
}

func (h auditSettingsHandler) updateRenewalWindowSettings(w http.ResponseWriter, r *http.Request) {
	scope, actorID, err := resolvePrincipalScope(r)
	if err != nil {
		writeMappedError(w, r, err)
		return
	}

	var req renewalWindowSettingsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeValidationError(w, r, "body", "invalid_json")
		return
	}

	item, err := h.settings.UpdateRenewalWindowSettings(r.Context(), scope, actorID, settingscmd.RenewalWindowInput{
		DaysBeforeExpiry:   req.DaysBeforeExpiry,
		ScanIntervalMinute: req.ScanIntervalMinute,
	})
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeObjectEnvelope(w, r, http.StatusOK, item)
}

func (h auditSettingsHandler) getSecuritySettings(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolvePrincipalScope(r)
	if err != nil {
		writeMappedError(w, r, err)
		return
	}

	item, err := h.settingsQuery.GetSecuritySettings(r.Context(), scope)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeObjectEnvelope(w, r, http.StatusOK, item)
}

func (h auditSettingsHandler) updateSecuritySettings(w http.ResponseWriter, r *http.Request) {
	scope, actorID, err := resolvePrincipalScope(r)
	if err != nil {
		writeMappedError(w, r, err)
		return
	}

	var req securitySettingsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeValidationError(w, r, "body", "invalid_json")
		return
	}

	item, err := h.settings.UpdateSecuritySettings(r.Context(), scope, actorID, settingscmd.SecuritySettingsInput{
		EnforceStrongPassword: req.EnforceStrongPassword,
		PasswordRotationDays:  req.PasswordRotationDays,
		MaxSessionCount:       req.MaxSessionCount,
	})
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeObjectEnvelope(w, r, http.StatusOK, item)
}

func resolvePrincipalScope(r *http.Request) (resource.Scope, string, error) {
	principal, ok := principalFromRequest(r)
	if !ok {
		return resource.Scope{}, "", identity.ErrUnauthorized
	}

	// Settings and audit APIs always operate on the authenticated principal's
	// resolved scope instead of trusting caller-supplied scope headers again.
	return resource.Scope{
		TenantID:      principal.Context.Tenant.ID,
		ProjectID:     principal.Context.Project.ID,
		EnvironmentID: principal.Context.Environment.ID,
	}, principal.User.ID, nil
}

func auditQueryFilterFromRequest(r *http.Request) (auditquery.Filter, error) {
	query := r.URL.Query()
	startTime, err := parseOptionalRFC3339(query.Get("start_time"), "start_time")
	if err != nil {
		return auditquery.Filter{}, err
	}
	endTime, err := parseOptionalRFC3339(query.Get("end_time"), "end_time")
	if err != nil {
		return auditquery.Filter{}, err
	}

	return auditquery.Filter{
		ActorID:      strings.TrimSpace(query.Get("actor_id")),
		Action:       strings.TrimSpace(query.Get("action")),
		ResourceType: strings.TrimSpace(query.Get("resource_type")),
		ResourceID:   strings.TrimSpace(query.Get("resource_id")),
		StartTime:    startTime,
		EndTime:      endTime,
	}, nil
}

func auditExportFilterFromRequest(req auditExportRequest) (settingscmd.AuditExportFilter, error) {
	startTime, err := parseOptionalRFC3339(req.StartTime, "start_time")
	if err != nil {
		return settingscmd.AuditExportFilter{}, err
	}
	endTime, err := parseOptionalRFC3339(req.EndTime, "end_time")
	if err != nil {
		return settingscmd.AuditExportFilter{}, err
	}

	return settingscmd.AuditExportFilter{
		ActorID:      strings.TrimSpace(req.ActorID),
		Action:       strings.TrimSpace(req.Action),
		ResourceType: strings.TrimSpace(req.ResourceType),
		ResourceID:   strings.TrimSpace(req.ResourceID),
		StartTime:    startTime,
		EndTime:      endTime,
	}, nil
}

func parseOptionalRFC3339(value string, field string) (*time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}

	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return nil, apperr.New(http.StatusBadRequest, "REQUEST_VALIDATION_FAILED", "request validation failed", apperr.Field(field, "invalid_rfc3339"))
	}
	return &parsed, nil
}

func decodeJSONOrEmpty(r *http.Request, dst any) error {
	err := decodeJSON(r, dst)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}
