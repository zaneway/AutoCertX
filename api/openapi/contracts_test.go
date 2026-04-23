package openapi_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
)

func TestOpenAPIDocumentIsValidJSON(t *testing.T) {
	spec := loadJSONFile(t, "openapi.json")

	if spec["openapi"] != "3.0.3" {
		t.Fatalf("openapi version = %v, want %q", spec["openapi"], "3.0.3")
	}

	info := nestedMap(t, spec, "info")
	if info["title"] == "" {
		t.Fatal("info.title should not be empty")
	}
	if info["version"] == "" {
		t.Fatal("info.version should not be empty")
	}

	paths := nestedMap(t, spec, "paths")
	if len(paths) < 30 {
		t.Fatalf("paths count = %d, want at least 30", len(paths))
	}
}

func TestOpenAPIHasRequiredPaths(t *testing.T) {
	spec := loadJSONFile(t, "openapi.json")
	paths := nestedMap(t, spec, "paths")

	requiredPaths := []string{
		"/api/v1/auth/login",
		"/api/v1/auth/refresh",
		"/api/v1/auth/me",
		"/api/v1/domains",
		"/api/v1/certificate-assets/requests",
		"/api/v1/ca-accounts/{id}/capabilities",
		"/api/v1/dns-credentials/{id}/rotate",
		"/api/v1/deployment-targets",
		"/api/v1/nodes/registration-tokens",
		"/api/v1/jobs/{id}/retry",
		"/api/v1/discoveries/{id}/claim",
		"/api/v1/dashboard/summary",
		"/api/v1/audit-events/export-csv",
		"/api/v1/settings/webhooks",
		"/api/v1/settings/renewal-window",
		"/api/v1/settings/security",
		"/agent/v1/register",
		"/agent/v1/heartbeat",
		"/agent/v1/jobs/poll",
		"/agent/v1/jobs/{jobId}/progress",
		"/agent/v1/jobs/{jobId}/complete",
	}
	for _, path := range requiredPaths {
		if _, ok := paths[path]; !ok {
			t.Fatalf("required path %q not found", path)
		}
	}
}

func TestOpenAPIContainsErrorResponseSchema(t *testing.T) {
	spec := loadJSONFile(t, "openapi.json")
	components := nestedMap(t, spec, "components")
	schemas := nestedMap(t, components, "schemas")
	responses := nestedMap(t, components, "responses")

	if _, ok := schemas["ErrorResponse"]; !ok {
		t.Fatal("components.schemas.ErrorResponse should exist")
	}
	if _, ok := responses["ErrorResponse"]; !ok {
		t.Fatal("components.responses.ErrorResponse should exist")
	}
}

func TestContractsErrorCatalogIsStable(t *testing.T) {
	catalog := loadErrorCatalog(t)
	seen := make(map[string]struct{}, len(catalog.Errors))
	codePattern := regexp.MustCompile(`^[A-Z][A-Z0-9_]+$`)

	if catalog.Version == "" {
		t.Fatal("error catalog version should not be empty")
	}
	if len(catalog.Errors) < 10 {
		t.Fatalf("error catalog size = %d, want at least 10", len(catalog.Errors))
	}

	for _, item := range catalog.Errors {
		if !codePattern.MatchString(item.Code) {
			t.Fatalf("error code %q must be upper snake case", item.Code)
		}
		if item.Message == "" {
			t.Fatalf("error code %q should have message", item.Code)
		}
		if item.HTTPStatus < 400 || item.HTTPStatus > 599 {
			t.Fatalf("error code %q has invalid http status %d", item.Code, item.HTTPStatus)
		}
		if _, ok := seen[item.Code]; ok {
			t.Fatalf("duplicate error code %q", item.Code)
		}
		seen[item.Code] = struct{}{}
	}
}

func TestContractsAuthMeResponseCarriesLocaleContext(t *testing.T) {
	spec := loadJSONFile(t, "openapi.json")
	components := nestedMap(t, spec, "components")
	schemas := nestedMap(t, components, "schemas")
	authMe := nestedMap(t, schemas, "AuthMeResponse")
	authContext := nestedMap(t, schemas, "AuthContext")

	data := nestedMap(t, authMe, "properties")
	if _, ok := data["request_id"]; !ok {
		t.Fatal("AuthMeResponse should include request_id")
	}
	if _, ok := data["data"]; !ok {
		t.Fatal("AuthMeResponse should include data")
	}

	contextProperties := nestedMap(t, authContext, "properties")
	for _, field := range []string{"locale", "available_locales", "tenant", "project", "environment", "roles"} {
		if _, ok := contextProperties[field]; !ok {
			t.Fatalf("AuthContext missing field %q", field)
		}
	}
}

func TestContractsAgentProtocolUsesPostOnly(t *testing.T) {
	spec := loadJSONFile(t, "openapi.json")
	paths := nestedMap(t, spec, "paths")

	agentPaths := []string{
		"/agent/v1/register",
		"/agent/v1/heartbeat",
		"/agent/v1/jobs/poll",
		"/agent/v1/jobs/{jobId}/progress",
		"/agent/v1/jobs/{jobId}/complete",
	}
	for _, path := range agentPaths {
		methods := nestedMap(t, paths, path)
		if len(methods) != 1 {
			t.Fatalf("agent path %q should only contain one method", path)
		}
		if _, ok := methods["post"]; !ok {
			t.Fatalf("agent path %q should use POST", path)
		}
	}
}

// errorCatalog mirrors the published OpenAPI error catalogue file.
type errorCatalog struct {
	Version string           `json:"version"`
	Errors  []catalogueError `json:"errors"`
}

// catalogueError represents one catalogued platform error contract.
type catalogueError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"http_status"`
	Retryable  bool   `json:"retryable"`
}

func loadErrorCatalog(t *testing.T) errorCatalog {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(repoRoot(t), "api", "openapi", "errors.json"))
	if err != nil {
		t.Fatalf("read errors.json: %v", err)
	}

	var catalog errorCatalog
	if err := json.Unmarshal(content, &catalog); err != nil {
		t.Fatalf("unmarshal errors.json: %v", err)
	}

	return catalog
}

func loadJSONFile(t *testing.T, filename string) map[string]any {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(repoRoot(t), "api", "openapi", filename))
	if err != nil {
		t.Fatalf("read %s: %v", filename, err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("unmarshal %s: %v", filename, err)
	}

	return parsed
}

func nestedMap(t *testing.T, parent map[string]any, key string) map[string]any {
	t.Helper()

	value, ok := parent[key]
	if !ok {
		t.Fatalf("missing key %q", key)
	}

	typed, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("key %q is not an object", key)
	}

	return typed
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}

	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repo root should contain go.mod: %v", err)
	}

	return root
}
