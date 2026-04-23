package settings

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	auditdomain "github.com/zaneway/AutoCertX/internal/domain/audit"
	"github.com/zaneway/AutoCertX/internal/domain/resource"
	settingsdomain "github.com/zaneway/AutoCertX/internal/domain/settings"
	"github.com/zaneway/AutoCertX/internal/platform/apperr"
)

// WebhookUpsertInput is the application-layer webhook write model.
type WebhookUpsertInput struct {
	Name       string
	URL        string
	Secret     string
	EventTypes []string
	Enabled    bool
}

// RenewalWindowInput is the application-layer renewal settings write model.
type RenewalWindowInput struct {
	DaysBeforeExpiry   int
	ScanIntervalMinute int
}

// SecuritySettingsInput is the application-layer partial security settings write model.
type SecuritySettingsInput struct {
	EnforceStrongPassword *bool
	PasswordRotationDays  *int
	MaxSessionCount       *int
}

// AuditExportFilter narrows synchronous CSV export.
type AuditExportFilter struct {
	ActorID      string
	Action       string
	ResourceType string
	ResourceID   string
	StartTime    *time.Time
	EndTime      *time.Time
}

// ExportCSVResult carries the generated artifact and the persisted export record.
type ExportCSVResult struct {
	Filename string
	Content  []byte
	Record   auditdomain.ExportRecord
}

// Service orchestrates T06 settings, webhook, and export commands.
type Service struct {
	audit     *auditdomain.Service
	settings  *settingsdomain.Service
	exportDir string
}

// NewService constructs the settings command service.
func NewService(auditService *auditdomain.Service, settingsService *settingsdomain.Service, exportDir string) *Service {
	if strings.TrimSpace(exportDir) == "" {
		exportDir = filepath.Join(os.TempDir(), "autocertx-exports")
	}

	return &Service{
		audit:     auditService,
		settings:  settingsService,
		exportDir: exportDir,
	}
}

// ListWebhookEndpoints returns configured webhook endpoints under scope.
func (s *Service) ListWebhookEndpoints(_ context.Context, scope resource.Scope) ([]auditdomain.WebhookEndpoint, error) {
	items, err := s.audit.ListWebhookEndpoints(scope)
	if err != nil {
		return nil, translateResourceError(err)
	}
	return items, nil
}

// CreateWebhookEndpoint creates one webhook endpoint and records the audit trail.
func (s *Service) CreateWebhookEndpoint(
	ctx context.Context,
	scope resource.Scope,
	actorID string,
	input WebhookUpsertInput,
) (auditdomain.WebhookEndpoint, error) {
	endpoint, err := s.audit.CreateWebhookEndpoint(scope, auditdomain.WebhookUpsertInput{
		Name:       input.Name,
		URL:        input.URL,
		Secret:     input.Secret,
		EventTypes: input.EventTypes,
		Enabled:    input.Enabled,
	})
	if err != nil {
		return auditdomain.WebhookEndpoint{}, translateResourceError(err)
	}

	// Webhook writes emit a separate audit event so operators can trace settings
	// mutations independently from later delivery attempts.
	_ = s.recordAudit(ctx, scope, actorID, "settings.webhook.create", "webhook_endpoint", endpoint.ID, map[string]string{
		"name":        endpoint.Name,
		"url":         endpoint.URL,
		"status":      endpoint.Status,
		"event_types": strings.Join(endpoint.EventTypes, ","),
	})
	return endpoint, nil
}

// UpdateWebhookEndpoint updates one webhook endpoint and records the audit trail.
func (s *Service) UpdateWebhookEndpoint(
	ctx context.Context,
	scope resource.Scope,
	actorID string,
	id string,
	input WebhookUpsertInput,
) (auditdomain.WebhookEndpoint, error) {
	endpoint, err := s.audit.UpdateWebhookEndpoint(scope, strings.TrimSpace(id), auditdomain.WebhookUpsertInput{
		Name:       input.Name,
		URL:        input.URL,
		Secret:     input.Secret,
		EventTypes: input.EventTypes,
		Enabled:    input.Enabled,
	})
	if err != nil {
		return auditdomain.WebhookEndpoint{}, translateResourceError(err)
	}

	// Updating the endpoint preserves the same audit vocabulary as create so
	// webhook lifecycle changes remain easy to query/export.
	_ = s.recordAudit(ctx, scope, actorID, "settings.webhook.update", "webhook_endpoint", endpoint.ID, map[string]string{
		"name":        endpoint.Name,
		"url":         endpoint.URL,
		"status":      endpoint.Status,
		"event_types": strings.Join(endpoint.EventTypes, ","),
	})
	return endpoint, nil
}

// GetRenewalWindowSettings returns renewal settings under scope.
func (s *Service) GetRenewalWindowSettings(_ context.Context, scope resource.Scope) (settingsdomain.RenewalWindowSettings, error) {
	item, err := s.settings.GetRenewalWindowSettings(scope)
	if err != nil {
		return settingsdomain.RenewalWindowSettings{}, translateResourceError(err)
	}
	return item, nil
}

// UpdateRenewalWindowSettings writes renewal settings and records the audit trail.
func (s *Service) UpdateRenewalWindowSettings(
	ctx context.Context,
	scope resource.Scope,
	actorID string,
	input RenewalWindowInput,
) (settingsdomain.RenewalWindowSettings, error) {
	item, err := s.settings.UpdateRenewalWindowSettings(scope, settingsdomain.RenewalWindowInput{
		DaysBeforeExpiry:   input.DaysBeforeExpiry,
		ScanIntervalMinute: input.ScanIntervalMinute,
	})
	if err != nil {
		return settingsdomain.RenewalWindowSettings{}, translateResourceError(err)
	}

	_ = s.recordAudit(ctx, scope, actorID, "settings.renewal_window.update", "settings_profile", scope.EnvironmentID, map[string]string{
		"days_before_expiry":    strconv.Itoa(item.DaysBeforeExpiry),
		"scan_interval_minutes": strconv.Itoa(item.ScanIntervalMinute),
	})
	return item, nil
}

// GetSecuritySettings returns security settings under scope.
func (s *Service) GetSecuritySettings(_ context.Context, scope resource.Scope) (settingsdomain.SecuritySettings, error) {
	item, err := s.settings.GetSecuritySettings(scope)
	if err != nil {
		return settingsdomain.SecuritySettings{}, translateResourceError(err)
	}
	return item, nil
}

// UpdateSecuritySettings writes security settings and records the audit trail.
func (s *Service) UpdateSecuritySettings(
	ctx context.Context,
	scope resource.Scope,
	actorID string,
	input SecuritySettingsInput,
) (settingsdomain.SecuritySettings, error) {
	item, err := s.settings.UpdateSecuritySettings(scope, settingsdomain.SecuritySettingsInput{
		EnforceStrongPassword: input.EnforceStrongPassword,
		PasswordRotationDays:  input.PasswordRotationDays,
		MaxSessionCount:       input.MaxSessionCount,
	})
	if err != nil {
		return settingsdomain.SecuritySettings{}, translateResourceError(err)
	}

	detail := map[string]string{
		"enforce_strong_password": strconv.FormatBool(item.EnforceStrongPassword),
		"password_rotation_days":  strconv.Itoa(item.PasswordRotationDays),
		"max_session_count":       strconv.Itoa(item.MaxSessionCount),
	}
	_ = s.recordAudit(ctx, scope, actorID, "settings.security.update", "settings_profile", scope.EnvironmentID, detail)
	return item, nil
}

// ExportAuditCSV generates the CSV response body and also persists an export artifact on disk.
func (s *Service) ExportAuditCSV(
	ctx context.Context,
	scope resource.Scope,
	actorID string,
	filter AuditExportFilter,
) (ExportCSVResult, error) {
	events, err := s.audit.ListEvents(scope, auditdomain.EventFilter{
		ActorID:      strings.TrimSpace(filter.ActorID),
		Action:       strings.TrimSpace(filter.Action),
		ResourceType: strings.TrimSpace(filter.ResourceType),
		ResourceID:   strings.TrimSpace(filter.ResourceID),
		StartTime:    filter.StartTime,
		EndTime:      filter.EndTime,
	})
	if err != nil {
		return ExportCSVResult{}, translateResourceError(err)
	}

	record, err := s.audit.CreateExportRecord(scope, auditdomain.ExportInput{
		ExportType:   auditdomain.ExportTypeAudit,
		ResourceType: "audit_event",
		RequestedBy:  strings.TrimSpace(actorID),
		Format:       auditdomain.ExportFormatCSV,
		Filters:      exportFilterSnapshot(filter),
		Status:       auditdomain.ExportStatusRunning,
	})
	if err != nil {
		return ExportCSVResult{}, translateResourceError(err)
	}

	// Rendering happens after the export record is created so failures can mark
	// the tracked export instead of disappearing as an uncorrelated 500.
	content, err := renderAuditCSV(events)
	if err != nil {
		_, _ = s.audit.MarkExportFailed(scope, record.ID, "CSV_RENDER_FAILED", err.Error())
		return ExportCSVResult{}, translateResourceError(err)
	}

	if err := os.MkdirAll(s.exportDir, 0o755); err != nil {
		_, _ = s.audit.MarkExportFailed(scope, record.ID, "EXPORT_STORAGE_FAILED", err.Error())
		return ExportCSVResult{}, apperr.Wrap(err, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}

	filename := "audit-" + strings.ToLower(record.ExportNo) + ".csv"
	storageRef := filepath.Join(s.exportDir, filename)
	// The CSV bytes returned to the caller and the artifact persisted on disk are
	// intentionally identical so later audits can re-open the same export.
	if err := os.WriteFile(storageRef, content, 0o600); err != nil {
		_, _ = s.audit.MarkExportFailed(scope, record.ID, "EXPORT_STORAGE_FAILED", err.Error())
		return ExportCSVResult{}, apperr.Wrap(err, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}

	record, err = s.audit.MarkExportSucceeded(scope, record.ID, storageRef)
	if err != nil {
		return ExportCSVResult{}, translateResourceError(err)
	}

	_ = s.recordAudit(ctx, scope, actorID, "audit.export_csv", "export_record", record.ID, map[string]string{
		"export_no":   record.ExportNo,
		"storage_ref": record.StorageRef,
		"rows":        strconv.Itoa(len(events)),
	})

	return ExportCSVResult{
		Filename: filename,
		Content:  content,
		Record:   record,
	}, nil
}

func (s *Service) recordAudit(
	ctx context.Context,
	scope resource.Scope,
	actorID string,
	action string,
	resourceType string,
	resourceID string,
	detail map[string]string,
) error {
	_, err := s.audit.RecordEvent(ctx, scope, auditdomain.EventInput{
		ActorType:    auditdomain.ActorTypeUser,
		ActorID:      strings.TrimSpace(actorID),
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   strings.TrimSpace(resourceID),
		Detail:       detail,
	})
	return err
}

func renderAuditCSV(events []auditdomain.Event) ([]byte, error) {
	buffer := &bytes.Buffer{}
	writer := csv.NewWriter(buffer)

	header := []string{
		"id",
		"tenant_id",
		"project_id",
		"environment_id",
		"occurred_at",
		"actor_type",
		"actor_id",
		"action",
		"resource_type",
		"resource_id",
		"request_id",
		"trace_id",
		"detail_json",
	}
	if err := writer.Write(header); err != nil {
		return nil, err
	}

	for _, event := range events {
		detailJSON := "{}"
		if len(event.Detail) > 0 {
			// Detail is serialized as one JSON cell so the CSV remains column-stable
			// while preserving structured audit metadata.
			encoded, err := json.Marshal(event.Detail)
			if err != nil {
				return nil, err
			}
			detailJSON = string(encoded)
		}

		record := []string{
			event.ID,
			event.Scope.TenantID,
			event.Scope.ProjectID,
			event.Scope.EnvironmentID,
			event.OccurredAt.UTC().Format(time.RFC3339),
			event.ActorType,
			event.ActorID,
			event.Action,
			event.ResourceType,
			event.ResourceID,
			event.RequestID,
			event.TraceID,
			detailJSON,
		}
		if err := writer.Write(record); err != nil {
			return nil, err
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func exportFilterSnapshot(filter AuditExportFilter) map[string]string {
	result := map[string]string{}
	if actorID := strings.TrimSpace(filter.ActorID); actorID != "" {
		result["actor_id"] = actorID
	}
	if action := strings.TrimSpace(filter.Action); action != "" {
		result["action"] = action
	}
	if resourceType := strings.TrimSpace(filter.ResourceType); resourceType != "" {
		result["resource_type"] = resourceType
	}
	if resourceID := strings.TrimSpace(filter.ResourceID); resourceID != "" {
		result["resource_id"] = resourceID
	}
	if filter.StartTime != nil {
		result["start_time"] = filter.StartTime.UTC().Format(time.RFC3339)
	}
	if filter.EndTime != nil {
		result["end_time"] = filter.EndTime.UTC().Format(time.RFC3339)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func translateResourceError(err error) error {
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
		return apperr.Wrap(err, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}

func validationDetail(err error) apperr.Detail {
	message := err.Error()
	field := ""
	switch {
	case strings.Contains(message, "name"):
		field = "name"
	case strings.Contains(message, "url"):
		field = "url"
	case strings.Contains(message, "secret"):
		field = "secret"
	case strings.Contains(message, "event_types"):
		field = "event_types"
	case strings.Contains(message, "days_before_expiry"):
		field = "days_before_expiry"
	case strings.Contains(message, "scan_interval_minutes"):
		field = "scan_interval_minutes"
	case strings.Contains(message, "password_rotation_days"):
		field = "password_rotation_days"
	case strings.Contains(message, "max_session_count"):
		field = "max_session_count"
	case strings.Contains(message, "security setting"):
		field = "body"
	case strings.Contains(message, "actor_id"):
		field = "actor_id"
	}
	return apperr.Field(field, message)
}
