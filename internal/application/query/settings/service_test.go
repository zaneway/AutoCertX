package settings

import (
	"context"
	"testing"

	auditdomain "github.com/zaneway/AutoCertX/internal/domain/audit"
	"github.com/zaneway/AutoCertX/internal/domain/resource"
	settingsdomain "github.com/zaneway/AutoCertX/internal/domain/settings"
)

func TestSettingsQueryReturnsWebhookAndProfiles(t *testing.T) {
	scope := resource.Scope{
		TenantID:      "11111111-1111-4111-8111-111111111111",
		ProjectID:     "22222222-2222-4222-8222-222222222222",
		EnvironmentID: "33333333-3333-4333-8333-333333333332",
	}

	auditService := auditdomain.NewService()
	settingsService := settingsdomain.NewService()
	if _, err := auditService.CreateWebhookEndpoint(scope, auditdomain.WebhookUpsertInput{
		Name:       "ops-primary",
		URL:        "https://ops.example.com/webhooks/audit",
		Secret:     "secret",
		EventTypes: []string{"settings.security.update"},
		Enabled:    true,
	}); err != nil {
		t.Fatalf("auditService.CreateWebhookEndpoint() error = %v", err)
	}
	if _, err := settingsService.UpdateRenewalWindowSettings(scope, settingsdomain.RenewalWindowInput{
		DaysBeforeExpiry:   21,
		ScanIntervalMinute: 30,
	}); err != nil {
		t.Fatalf("settingsService.UpdateRenewalWindowSettings() error = %v", err)
	}
	maxSessionCount := 8
	if _, err := settingsService.UpdateSecuritySettings(scope, settingsdomain.SecuritySettingsInput{
		MaxSessionCount: &maxSessionCount,
	}); err != nil {
		t.Fatalf("settingsService.UpdateSecuritySettings() error = %v", err)
	}

	service, err := NewService(auditService, settingsService)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	webhooks, err := service.ListWebhookEndpoints(context.Background(), scope)
	if err != nil {
		t.Fatalf("ListWebhookEndpoints() error = %v", err)
	}
	if len(webhooks) != 1 || webhooks[0].Name != "ops-primary" {
		t.Fatalf("ListWebhookEndpoints() = %+v, want ops-primary", webhooks)
	}

	renewal, err := service.GetRenewalWindowSettings(context.Background(), scope)
	if err != nil {
		t.Fatalf("GetRenewalWindowSettings() error = %v", err)
	}
	if renewal.DaysBeforeExpiry != 21 || renewal.ScanIntervalMinute != 30 {
		t.Fatalf("renewal settings = %+v, want days=21 interval=30", renewal)
	}

	security, err := service.GetSecuritySettings(context.Background(), scope)
	if err != nil {
		t.Fatalf("GetSecuritySettings() error = %v", err)
	}
	if security.MaxSessionCount != 8 {
		t.Fatalf("security max_session_count = %d, want 8", security.MaxSessionCount)
	}
}
