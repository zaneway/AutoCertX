package httpserver_test

import (
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestAuditSettingsAPIAdminFlow(t *testing.T) {
	handler := newRouterForAuthTest(t)
	login := loginForTest(t, handler, "admin", "admin123!")

	webhook := performJSONRequest(t, handler, http.MethodPost, "/api/v1/settings/webhooks", map[string]any{
		"name":        "ops-primary",
		"url":         "https://ops.example.com/webhooks/audit",
		"secret":      "top-secret",
		"event_types": []string{"settings.security.update"},
		"enabled":     true,
	}, adminScopeHeaders(login.AccessToken))
	if webhook.Code != http.StatusCreated {
		t.Fatalf("create webhook status = %d, want %d", webhook.Code, http.StatusCreated)
	}

	list := performJSONRequest(t, handler, http.MethodGet, "/api/v1/settings/webhooks", nil, adminScopeHeaders(login.AccessToken))
	if list.Code != http.StatusOK {
		t.Fatalf("list webhooks status = %d, want %d", list.Code, http.StatusOK)
	}
	var listPayload struct {
		Items []map[string]any `json:"items"`
	}
	decodeResponse(t, list.Body.Bytes(), &listPayload)
	if len(listPayload.Items) != 1 {
		t.Fatalf("webhook list size = %d, want %d", len(listPayload.Items), 1)
	}
	webhookID := listPayload.Items[0]["id"].(string)

	updateWebhook := performJSONRequest(t, handler, http.MethodPut, "/api/v1/settings/webhooks/"+webhookID, map[string]any{
		"name":        "ops-primary",
		"url":         "https://ops.example.com/webhooks/audit",
		"event_types": []string{"settings.security.update"},
		"enabled":     true,
	}, adminScopeHeaders(login.AccessToken))
	if updateWebhook.Code != http.StatusOK {
		t.Fatalf("update webhook status = %d, want %d", updateWebhook.Code, http.StatusOK)
	}

	security := performJSONRequest(t, handler, http.MethodPut, "/api/v1/settings/security", map[string]any{
		"max_session_count": 8,
	}, adminScopeHeaders(login.AccessToken))
	if security.Code != http.StatusOK {
		t.Fatalf("update security status = %d, want %d", security.Code, http.StatusOK)
	}

	auditList := performJSONRequest(t, handler, http.MethodGet, "/api/v1/audit-events?action=settings.security.update", nil, adminScopeHeaders(login.AccessToken))
	if auditList.Code != http.StatusOK {
		t.Fatalf("list audit status = %d, want %d", auditList.Code, http.StatusOK)
	}
	var auditPayload struct {
		Items []map[string]any `json:"items"`
	}
	decodeResponse(t, auditList.Body.Bytes(), &auditPayload)
	if len(auditPayload.Items) != 1 {
		t.Fatalf("filtered audit item count = %d, want %d", len(auditPayload.Items), 1)
	}

	export := performJSONRequest(t, handler, http.MethodPost, "/api/v1/audit-events/export-csv", map[string]any{
		"action": "settings.security.update",
	}, adminScopeHeaders(login.AccessToken))
	if export.Code != http.StatusOK {
		t.Fatalf("export csv status = %d, want %d", export.Code, http.StatusOK)
	}
	if contentType := export.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/csv") {
		t.Fatalf("content-type = %q, want text/csv", contentType)
	}
	if !strings.Contains(export.Body.String(), "settings.security.update") {
		t.Fatalf("csv body should contain audit action, got %q", export.Body.String())
	}
	storageRef := export.Header().Get("X-Export-Storage-Ref")
	if storageRef == "" {
		t.Fatal("X-Export-Storage-Ref should not be empty")
	}
	stored, err := os.ReadFile(storageRef)
	if err != nil {
		t.Fatalf("os.ReadFile(export storage) error = %v", err)
	}
	if string(stored) != export.Body.String() {
		t.Fatal("stored csv should match HTTP response body")
	}
}

func TestAuditSettingsPermissionsForAuditor(t *testing.T) {
	handler := newRouterForAuthTest(t)
	login := loginForTest(t, handler, "auditor", "auditor123!")

	auditList := performJSONRequest(t, handler, http.MethodGet, "/api/v1/audit-events", nil, auditorScopeHeaders(login.AccessToken))
	if auditList.Code != http.StatusOK {
		t.Fatalf("auditor audit list status = %d, want %d", auditList.Code, http.StatusOK)
	}

	export := performJSONRequest(t, handler, http.MethodPost, "/api/v1/audit-events/export-csv", map[string]any{}, auditorScopeHeaders(login.AccessToken))
	if export.Code != http.StatusOK {
		t.Fatalf("auditor export status = %d, want %d", export.Code, http.StatusOK)
	}

	security := performJSONRequest(t, handler, http.MethodGet, "/api/v1/settings/security", nil, auditorScopeHeaders(login.AccessToken))
	if security.Code != http.StatusForbidden {
		t.Fatalf("auditor security status = %d, want %d", security.Code, http.StatusForbidden)
	}
}

// loginResult mirrors the subset of the login response used by auth tests.
type loginResult struct {
	AccessToken string `json:"access_token"`
}

func loginForTest(t *testing.T, handler http.Handler, username string, password string) loginResult {
	t.Helper()

	login := performJSONRequest(t, handler, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": username,
		"password": password,
	}, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d", login.Code, http.StatusOK)
	}

	var payload loginResult
	decodeResponse(t, login.Body.Bytes(), &payload)
	if payload.AccessToken == "" {
		t.Fatal("access_token should not be empty")
	}
	return payload
}

func adminScopeHeaders(token string) map[string]string {
	return map[string]string{
		"Authorization":    "Bearer " + token,
		"X-Tenant-Id":      "11111111-1111-4111-8111-111111111111",
		"X-Project-Id":     "22222222-2222-4222-8222-222222222222",
		"X-Environment-Id": "33333333-3333-4333-8333-333333333332",
	}
}

func auditorScopeHeaders(token string) map[string]string {
	return map[string]string{
		"Authorization":    "Bearer " + token,
		"X-Tenant-Id":      "11111111-1111-4111-8111-111111111111",
		"X-Project-Id":     "22222222-2222-4222-8222-222222222221",
		"X-Environment-Id": "33333333-3333-4333-8333-333333333331",
	}
}
