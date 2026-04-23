package settings

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	auditdomain "github.com/zaneway/AutoCertX/internal/domain/audit"
	"github.com/zaneway/AutoCertX/internal/domain/resource"
	settingsdomain "github.com/zaneway/AutoCertX/internal/domain/settings"
	"github.com/zaneway/AutoCertX/internal/platform/apperr"
)

// WebhookItem is the read-model payload for one outbound audit webhook.
type WebhookItem struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	URL          string     `json:"url"`
	EventTypes   []string   `json:"event_types"`
	Status       string     `json:"status"`
	LastTestedAt *time.Time `json:"last_tested_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// RenewalWindowItem is the query payload for renewal settings.
type RenewalWindowItem struct {
	DaysBeforeExpiry   int       `json:"days_before_expiry"`
	ScanIntervalMinute int       `json:"scan_interval_minutes"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// SecurityItem is the query payload for security baseline settings.
type SecurityItem struct {
	EnforceStrongPassword bool      `json:"enforce_strong_password"`
	PasswordRotationDays  int       `json:"password_rotation_days"`
	MaxSessionCount       int       `json:"max_session_count"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// Service maps settings and webhook facts into stable console DTOs.
type Service struct {
	audit    *auditdomain.Service
	settings *settingsdomain.Service
}

// NewService constructs the settings query service.
func NewService(auditService *auditdomain.Service, settingsService *settingsdomain.Service) (*Service, error) {
	switch {
	case auditService == nil:
		return nil, fmt.Errorf("settings query audit service required")
	case settingsService == nil:
		return nil, fmt.Errorf("settings query settings service required")
	default:
		return &Service{
			audit:    auditService,
			settings: settingsService,
		}, nil
	}
}

// ListWebhookEndpoints returns the outbound audit webhooks configured under scope.
func (s *Service) ListWebhookEndpoints(_ context.Context, scope resource.Scope) ([]WebhookItem, error) {
	items, err := s.audit.ListWebhookEndpoints(scope)
	if err != nil {
		return nil, translateError(err)
	}
	result := make([]WebhookItem, 0, len(items))
	for _, item := range items {
		result = append(result, mapWebhook(item))
	}
	return result, nil
}

// GetRenewalWindowSettings returns the effective renewal settings snapshot.
func (s *Service) GetRenewalWindowSettings(_ context.Context, scope resource.Scope) (RenewalWindowItem, error) {
	item, err := s.settings.GetRenewalWindowSettings(scope)
	if err != nil {
		return RenewalWindowItem{}, translateError(err)
	}
	return RenewalWindowItem{
		DaysBeforeExpiry:   item.DaysBeforeExpiry,
		ScanIntervalMinute: item.ScanIntervalMinute,
		UpdatedAt:          item.UpdatedAt,
	}, nil
}

// GetSecuritySettings returns the effective security baseline snapshot.
func (s *Service) GetSecuritySettings(_ context.Context, scope resource.Scope) (SecurityItem, error) {
	item, err := s.settings.GetSecuritySettings(scope)
	if err != nil {
		return SecurityItem{}, translateError(err)
	}
	return SecurityItem{
		EnforceStrongPassword: item.EnforceStrongPassword,
		PasswordRotationDays:  item.PasswordRotationDays,
		MaxSessionCount:       item.MaxSessionCount,
		UpdatedAt:             item.UpdatedAt,
	}, nil
}

func mapWebhook(item auditdomain.WebhookEndpoint) WebhookItem {
	eventTypes := append([]string(nil), item.EventTypes...)
	return WebhookItem{
		ID:           item.ID,
		Name:         item.Name,
		URL:          item.URL,
		EventTypes:   eventTypes,
		Status:       item.Status,
		LastTestedAt: timePointer(item.LastTestedAt),
		CreatedAt:    item.CreatedAt,
		UpdatedAt:    item.UpdatedAt,
	}
}

func timePointer(value *time.Time) *time.Time {
	if value == nil || value.IsZero() {
		return nil
	}
	cloned := *value
	return &cloned
}

func translateError(err error) error {
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
	case strings.Contains(message, "days_before_expiry"):
		field = "days_before_expiry"
	case strings.Contains(message, "scan_interval_minutes"):
		field = "scan_interval_minutes"
	case strings.Contains(message, "password_rotation_days"):
		field = "password_rotation_days"
	case strings.Contains(message, "max_session_count"):
		field = "max_session_count"
	}
	return apperr.Field(field, message)
}
