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
		"/api/v1/certificate-assets/{assetId}/deploy",
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

func TestContractsLifecycleEnumsAreFrozen(t *testing.T) {
	spec := loadJSONFile(t, "openapi.json")
	schemas := nestedMap(t, nestedMap(t, spec, "components"), "schemas")

	assertEnum(t, schemas, "ChallengeType", []string{"http-01", "dns-01"})
	assertEnum(t, schemas, "CertificateType", []string{"single", "san", "wildcard"})
	assertEnum(t, schemas, "CertificateRequestType", []string{"issue", "renew"})
	assertEnum(t, schemas, "CertificateRequestStatus", []string{"draft", "submitted", "accepted", "running", "completed", "failed", "cancelled"})
	assertEnum(t, schemas, "IssueWorkflowStatus", []string{
		"created",
		"order_pending",
		"challenge_pending",
		"challenge_processing",
		"challenge_valid",
		"finalizing",
		"issued",
		"deploy_pending",
		"deploying",
		"deployed",
		"partially_failed",
		"failed",
		"cancelled",
	})
	assertEnum(t, schemas, "WorkflowChallengeStatus", []string{
		"pending",
		"presenting",
		"presented",
		"propagating",
		"ready",
		"verifying",
		"valid",
		"invalid",
		"cleanup_pending",
		"cleaned",
		"cleanup_failed",
	})
	assertEnum(t, schemas, "CertificateAssetStatus", []string{"active", "expiring", "renewing", "deploy_failed", "expired", "revoked", "orphaned"})
}

func TestContractsAgentCapabilitiesAndJobTypesFreezeGAScope(t *testing.T) {
	spec := loadJSONFile(t, "openapi.json")
	schemas := nestedMap(t, nestedMap(t, spec, "components"), "schemas")

	assertEnum(t, schemas, "DeploymentTargetType", []string{"nginx", "tomcat-jsse-pkcs12"})
	assertEnum(t, schemas, "AgentCapabilityCode", []string{
		"keygen:rsa",
		"challenge:http-01",
		"deploy:nginx",
		"deploy:tomcat-jsse-pkcs12",
		"verify:nginx",
		"verify:tomcat",
		"discover:nginx",
		"discover:tomcat",
	})
	assertEnum(t, schemas, "AgentJobType", []string{
		"present_http01_challenge",
		"cleanup_http01_challenge",
		"deploy_nginx_certificate",
		"deploy_tomcat_certificate",
		"verify_nginx_deployment",
		"verify_tomcat_deployment",
		"discover_nginx_certificates",
		"discover_tomcat_certificates",
	})
	assertEnumContains(t, schemas, "PlatformJobType", []string{
		"start_issue_workflow",
		"continue_issue_workflow",
		"renewal_scan",
		"discovery_scan",
		"present_http01_challenge",
		"deploy_nginx_certificate",
	})
}

func TestContractsAgentJobEnvelopeCarriesIdempotentExecutionFields(t *testing.T) {
	spec := loadJSONFile(t, "openapi.json")
	schemas := nestedMap(t, nestedMap(t, spec, "components"), "schemas")

	agentJob := nestedMap(t, schemas, "AgentJob")
	assertRequiredFields(t, agentJob, []string{"job_id", "job_type", "schema_version", "operation_id", "lease_expire_at", "payload"})
	agentJobProperties := nestedMap(t, agentJob, "properties")
	assertPropertyRef(t, agentJobProperties, "job_type", "#/components/schemas/AgentJobType")
	assertPropertyRef(t, agentJobProperties, "payload", "#/components/schemas/AgentJobPayload")

	payload := nestedMap(t, schemas, "AgentJobPayload")
	assertRequiredFields(t, payload, []string{"schema_version", "operation_id"})
	payloadProperties := nestedMap(t, payload, "properties")
	assertPropertyRef(t, payloadProperties, "target_type", "#/components/schemas/DeploymentTargetType")
	assertPropertyRef(t, payloadProperties, "challenge_type", "#/components/schemas/ChallengeType")

	progress := nestedMap(t, schemas, "AgentJobProgressRequest")
	assertRequiredFields(t, progress, []string{"operation_id", "status"})

	complete := nestedMap(t, schemas, "AgentJobCompleteRequest")
	assertRequiredFields(t, complete, []string{"operation_id", "result_status"})
	completeProperties := nestedMap(t, complete, "properties")
	for _, field := range []string{"failed_stage", "retryable", "compensation_required", "evidence"} {
		if _, ok := completeProperties[field]; !ok {
			t.Fatalf("AgentJobCompleteRequest missing field %q", field)
		}
	}
}

func TestContractsCertificateRequestsRequireIdempotency(t *testing.T) {
	spec := loadJSONFile(t, "openapi.json")
	schemas := nestedMap(t, nestedMap(t, spec, "components"), "schemas")

	request := nestedMap(t, schemas, "CertificateRequestCreateRequest")
	assertRequiredFields(t, request, []string{"domain_ids", "ca_account_id", "certificate_type", "challenge_type", "idempotency_key"})
	properties := nestedMap(t, request, "properties")
	assertPropertyRef(t, properties, "certificate_type", "#/components/schemas/CertificateType")
	assertPropertyRef(t, properties, "challenge_type", "#/components/schemas/ChallengeType")
	assertPropertyRef(t, properties, "request_type", "#/components/schemas/CertificateRequestType")
}

func TestContractsCertificateDeploymentRequiresIdempotency(t *testing.T) {
	spec := loadJSONFile(t, "openapi.json")
	schemas := nestedMap(t, nestedMap(t, spec, "components"), "schemas")

	request := nestedMap(t, schemas, "CertificateAssetDeployRequest")
	assertRequiredFields(t, request, []string{"version_id", "target_id", "idempotency_key"})

	response := nestedMap(t, schemas, "DeploymentAcceptedEnvelope")
	assertRequiredFields(t, response, []string{"request_id", "status", "deployment_id", "job_id"})
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

func assertEnum(t *testing.T, schemas map[string]any, schemaName string, expected []string) {
	t.Helper()

	schema := nestedMap(t, schemas, schemaName)
	actual := stringArray(t, schema, "enum")
	if len(actual) != len(expected) {
		t.Fatalf("%s enum length = %d, want %d: %v", schemaName, len(actual), len(expected), actual)
	}
	assertStringSetContains(t, schemaName, actual, expected)
}

func assertEnumContains(t *testing.T, schemas map[string]any, schemaName string, expected []string) {
	t.Helper()

	schema := nestedMap(t, schemas, schemaName)
	actual := stringArray(t, schema, "enum")
	assertStringSetContains(t, schemaName, actual, expected)
}

func assertRequiredFields(t *testing.T, schema map[string]any, expected []string) {
	t.Helper()

	actual := stringArray(t, schema, "required")
	assertStringSetContains(t, "required", actual, expected)
}

func assertPropertyRef(t *testing.T, properties map[string]any, field string, expectedRef string) {
	t.Helper()

	property := nestedMap(t, properties, field)
	if property["$ref"] != expectedRef {
		t.Fatalf("property %q $ref = %v, want %q", field, property["$ref"], expectedRef)
	}
}

func assertStringSetContains(t *testing.T, label string, actual []string, expected []string) {
	t.Helper()

	seen := make(map[string]struct{}, len(actual))
	for _, value := range actual {
		seen[value] = struct{}{}
	}
	for _, value := range expected {
		if _, ok := seen[value]; !ok {
			t.Fatalf("%s missing value %q in %v", label, value, actual)
		}
	}
}

func stringArray(t *testing.T, parent map[string]any, key string) []string {
	t.Helper()

	value, ok := parent[key]
	if !ok {
		t.Fatalf("missing key %q", key)
	}
	values, ok := value.([]any)
	if !ok {
		t.Fatalf("key %q is not an array", key)
	}
	result := make([]string, 0, len(values))
	for _, item := range values {
		typed, ok := item.(string)
		if !ok {
			t.Fatalf("key %q contains non-string value %v", key, item)
		}
		result = append(result, typed)
	}
	return result
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
