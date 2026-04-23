package settings

import (
	"context"
	"os"
	"strings"
	"testing"

	auditdomain "github.com/zaneway/AutoCertX/internal/domain/audit"
	"github.com/zaneway/AutoCertX/internal/domain/resource"
	settingsdomain "github.com/zaneway/AutoCertX/internal/domain/settings"
)

func TestServiceTracksWebhookNotificationsAndExportsCSV(t *testing.T) {
	scope := resource.Scope{
		TenantID:      "11111111-1111-4111-8111-111111111111",
		ProjectID:     "22222222-2222-4222-8222-222222222222",
		EnvironmentID: "33333333-3333-4333-8333-333333333333",
	}

	auditService := auditdomain.NewService()
	service := NewService(auditService, settingsdomain.NewService(), t.TempDir())

	ctx := context.Background()
	actorID := "44444444-4444-4444-8444-444444444441"

	if _, err := service.CreateWebhookEndpoint(ctx, scope, actorID, WebhookUpsertInput{
		Name:       "ops-primary",
		URL:        "https://ops.example.com/webhooks/audit",
		Secret:     "top-secret",
		EventTypes: []string{"settings.security.update"},
		Enabled:    true,
	}); err != nil {
		t.Fatalf("CreateWebhookEndpoint(success) error = %v", err)
	}
	if _, err := service.CreateWebhookEndpoint(ctx, scope, actorID, WebhookUpsertInput{
		Name:       "ops-failover",
		URL:        "https://fail.example.com/webhooks/audit",
		Secret:     "top-secret",
		EventTypes: []string{"settings.security.update"},
		Enabled:    true,
	}); err != nil {
		t.Fatalf("CreateWebhookEndpoint(retry) error = %v", err)
	}

	if _, err := service.UpdateSecuritySettings(ctx, scope, actorID, SecuritySettingsInput{
		MaxSessionCount: intPtr(8),
	}); err != nil {
		t.Fatalf("UpdateSecuritySettings() error = %v", err)
	}

	notifications, err := auditService.ListNotificationEvents(scope)
	if err != nil {
		t.Fatalf("ListNotificationEvents() error = %v", err)
	}
	if len(notifications) != 2 {
		t.Fatalf("notification count = %d, want %d", len(notifications), 2)
	}

	statusCount := map[string]int{}
	for _, notification := range notifications {
		statusCount[notification.Status]++
	}
	if statusCount[auditdomain.NotificationStatusSucceeded] != 1 {
		t.Fatalf("succeeded notifications = %d, want 1", statusCount[auditdomain.NotificationStatusSucceeded])
	}
	if statusCount[auditdomain.NotificationStatusRetry] != 1 {
		t.Fatalf("retry notifications = %d, want 1", statusCount[auditdomain.NotificationStatusRetry])
	}

	result, err := service.ExportAuditCSV(ctx, scope, actorID, AuditExportFilter{
		Action: "settings.security.update",
	})
	if err != nil {
		t.Fatalf("ExportAuditCSV() error = %v", err)
	}
	if result.Record.Status != auditdomain.ExportStatusSucceeded {
		t.Fatalf("export status = %q, want %q", result.Record.Status, auditdomain.ExportStatusSucceeded)
	}
	if result.Record.StorageRef == "" {
		t.Fatal("export storage_ref should not be empty")
	}

	exported, err := os.ReadFile(result.Record.StorageRef)
	if err != nil {
		t.Fatalf("os.ReadFile(export) error = %v", err)
	}
	if string(exported) != string(result.Content) {
		t.Fatal("stored export content should match response content")
	}
	if !strings.Contains(string(result.Content), "settings.security.update") {
		t.Fatalf("csv content should contain audit action, got %q", string(result.Content))
	}

	records, err := auditService.ListEvents(scope, auditdomain.EventFilter{Action: "audit.export_csv"})
	if err != nil {
		t.Fatalf("ListEvents(audit.export_csv) error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("audit.export_csv events = %d, want %d", len(records), 1)
	}
}

func intPtr(value int) *int {
	return &value
}
