package httpserver_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zaneway/AutoCertX/internal/app/controlplane/wiring"
)

func TestAuthLoginRefreshLogoutFlow(t *testing.T) {
	handler := newRouterForAuthTest(t)

	login := performJSONRequest(t, handler, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": "admin",
		"password": "admin123!",
	}, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d", login.Code, http.StatusOK)
	}

	var loginPayload struct {
		RequestID    string `json:"request_id"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Context      struct {
			Locale string   `json:"locale"`
			Roles  []string `json:"roles"`
		} `json:"context"`
	}
	decodeResponse(t, login.Body.Bytes(), &loginPayload)
	if loginPayload.RequestID == "" {
		t.Fatal("login request_id should not be empty")
	}
	if loginPayload.Context.Locale != "en-US" {
		t.Fatalf("login locale = %q, want %q", loginPayload.Context.Locale, "en-US")
	}
	if len(loginPayload.Context.Roles) != 1 || loginPayload.Context.Roles[0] != "tenant_admin" {
		t.Fatalf("login roles = %v, want [tenant_admin]", loginPayload.Context.Roles)
	}

	me := performJSONRequest(t, handler, http.MethodGet, "/api/v1/auth/me", nil, map[string]string{
		"Authorization":    "Bearer " + loginPayload.AccessToken,
		"X-Project-Id":     "22222222-2222-4222-8222-222222222222",
		"X-Environment-Id": "33333333-3333-4333-8333-333333333332",
	})
	if me.Code != http.StatusOK {
		t.Fatalf("me status = %d, want %d", me.Code, http.StatusOK)
	}

	var mePayload struct {
		Data struct {
			Project struct {
				Code string `json:"code"`
			} `json:"project"`
			Environment struct {
				Code string `json:"code"`
			} `json:"environment"`
		} `json:"data"`
	}
	decodeResponse(t, me.Body.Bytes(), &mePayload)
	if mePayload.Data.Project.Code != "platform" {
		t.Fatalf("project code = %q, want %q", mePayload.Data.Project.Code, "platform")
	}
	if mePayload.Data.Environment.Code != "staging" {
		t.Fatalf("environment code = %q, want %q", mePayload.Data.Environment.Code, "staging")
	}

	refresh := performJSONRequest(t, handler, http.MethodPost, "/api/v1/auth/refresh", map[string]any{
		"refresh_token": loginPayload.RefreshToken,
	}, nil)
	if refresh.Code != http.StatusOK {
		t.Fatalf("refresh status = %d, want %d", refresh.Code, http.StatusOK)
	}

	var refreshPayload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	decodeResponse(t, refresh.Body.Bytes(), &refreshPayload)
	if refreshPayload.RefreshToken == loginPayload.RefreshToken {
		t.Fatal("refresh should rotate refresh token")
	}

	logout := performJSONRequest(t, handler, http.MethodPost, "/api/v1/auth/logout", nil, map[string]string{
		"Authorization": "Bearer " + refreshPayload.AccessToken,
	})
	if logout.Code != http.StatusOK {
		t.Fatalf("logout status = %d, want %d", logout.Code, http.StatusOK)
	}

	meAfterLogout := performJSONRequest(t, handler, http.MethodGet, "/api/v1/auth/me", nil, map[string]string{
		"Authorization": "Bearer " + refreshPayload.AccessToken,
	})
	if meAfterLogout.Code != http.StatusUnauthorized {
		t.Fatalf("me after logout status = %d, want %d", meAfterLogout.Code, http.StatusUnauthorized)
	}

	var errorPayload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeResponse(t, meAfterLogout.Body.Bytes(), &errorPayload)
	if errorPayload.Error.Code != "AUTH_SESSION_EXPIRED" {
		t.Fatalf("error code = %q, want %q", errorPayload.Error.Code, "AUTH_SESSION_EXPIRED")
	}
}

func TestAuthPreferencesPermissionDeniedForAuditor(t *testing.T) {
	handler := newRouterForAuthTest(t)

	login := performJSONRequest(t, handler, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": "auditor",
		"password": "auditor123!",
	}, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d", login.Code, http.StatusOK)
	}

	var loginPayload struct {
		AccessToken string `json:"access_token"`
		Context     struct {
			Locale string   `json:"locale"`
			Roles  []string `json:"roles"`
		} `json:"context"`
	}
	decodeResponse(t, login.Body.Bytes(), &loginPayload)
	if loginPayload.Context.Locale != "zh-CN" {
		t.Fatalf("auditor locale = %q, want tenant fallback %q", loginPayload.Context.Locale, "zh-CN")
	}
	if len(loginPayload.Context.Roles) != 1 || loginPayload.Context.Roles[0] != "auditor" {
		t.Fatalf("auditor roles = %v, want [auditor]", loginPayload.Context.Roles)
	}

	preferences := performJSONRequest(t, handler, http.MethodPatch, "/api/v1/auth/me/preferences", map[string]any{
		"locale": "en-US",
	}, map[string]string{
		"Authorization": "Bearer " + loginPayload.AccessToken,
	})
	if preferences.Code != http.StatusForbidden {
		t.Fatalf("preferences status = %d, want %d", preferences.Code, http.StatusForbidden)
	}

	var errorPayload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeResponse(t, preferences.Body.Bytes(), &errorPayload)
	if errorPayload.Error.Code != "PERMISSION_DENIED" {
		t.Fatalf("error code = %q, want %q", errorPayload.Error.Code, "PERMISSION_DENIED")
	}
}

func TestAuthRejectsCrossTenantContextSwitch(t *testing.T) {
	handler := newRouterForAuthTest(t)

	login := performJSONRequest(t, handler, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": "admin",
		"password": "admin123!",
	}, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d", login.Code, http.StatusOK)
	}

	var loginPayload struct {
		AccessToken string `json:"access_token"`
	}
	decodeResponse(t, login.Body.Bytes(), &loginPayload)

	me := performJSONRequest(t, handler, http.MethodGet, "/api/v1/auth/me", nil, map[string]string{
		"Authorization": "Bearer " + loginPayload.AccessToken,
		"X-Tenant-Id":   "11111111-1111-4111-8111-111111111112",
	})
	if me.Code != http.StatusConflict {
		t.Fatalf("cross-tenant me status = %d, want %d", me.Code, http.StatusConflict)
	}

	var errorPayload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeResponse(t, me.Body.Bytes(), &errorPayload)
	if errorPayload.Error.Code != "TENANT_SCOPE_MISMATCH" {
		t.Fatalf("error code = %q, want %q", errorPayload.Error.Code, "TENANT_SCOPE_MISMATCH")
	}
}

func newRouterForAuthTest(t *testing.T) http.Handler {
	t.Helper()

	result, err := wiring.Build(wiring.Options{
		ServiceName:     "controlplane",
		EnvPrefix:       "CONTROLPLANE",
		DefaultHTTPAddr: ":0",
	})
	if err != nil {
		t.Fatalf("wiring.Build() error = %v", err)
	}

	return result.Server.Handler
}

func performJSONRequest(t *testing.T, handler http.Handler, method string, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	var payload []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		payload = encoded
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeResponse(t *testing.T, body []byte, dst any) {
	t.Helper()
	if err := json.Unmarshal(body, dst); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; body = %s", err, string(body))
	}
}
