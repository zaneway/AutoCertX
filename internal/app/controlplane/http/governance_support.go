package httpserver

import (
	"errors"
	"net/http"
	"reflect"
	"strings"

	"github.com/zaneway/AutoCertX/internal/domain/resource"
	"github.com/zaneway/AutoCertX/internal/platform/apperr"
	"github.com/zaneway/AutoCertX/internal/platform/httpx"
	"github.com/zaneway/AutoCertX/internal/platform/uuidx"
)

const headerUserID = "X-User-Id"

var defaultGovernanceScope = resource.Scope{
	TenantID:      "11111111-1111-4111-8111-111111111111",
	ProjectID:     "22222222-2222-4222-8222-222222222222",
	EnvironmentID: "33333333-3333-4333-8333-333333333333",
}

const defaultGovernanceActorID = "44444444-4444-4444-8444-444444444444"

type objectEnvelope struct {
	RequestID string `json:"request_id"`
	Data      any    `json:"data"`
}

type listEnvelope struct {
	RequestID string `json:"request_id"`
	Items     any    `json:"items"`
}

type acceptedEnvelope struct {
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
	JobID     string `json:"job_id,omitempty"`
}

func resolveGovernanceScope(r *http.Request) (resource.Scope, string, error) {
	scope := resource.Scope{
		TenantID:      headerOrDefault(r, headerTenantID, defaultGovernanceScope.TenantID),
		ProjectID:     headerOrDefault(r, headerProjectID, defaultGovernanceScope.ProjectID),
		EnvironmentID: headerOrDefault(r, headerEnvironmentID, defaultGovernanceScope.EnvironmentID),
	}
	actorID := headerOrDefault(r, headerUserID, defaultGovernanceActorID)

	if !uuidx.IsValid(scope.TenantID) {
		return resource.Scope{}, "", apperr.New(http.StatusBadRequest, "REQUEST_VALIDATION_FAILED", "request validation failed", apperr.Field("tenant_id", "invalid tenant scope header"))
	}
	if !uuidx.IsValid(scope.ProjectID) {
		return resource.Scope{}, "", apperr.New(http.StatusBadRequest, "REQUEST_VALIDATION_FAILED", "request validation failed", apperr.Field("project_id", "invalid project scope header"))
	}
	if !uuidx.IsValid(scope.EnvironmentID) {
		return resource.Scope{}, "", apperr.New(http.StatusBadRequest, "REQUEST_VALIDATION_FAILED", "request validation failed", apperr.Field("environment_id", "invalid environment scope header"))
	}
	if !uuidx.IsValid(actorID) {
		return resource.Scope{}, "", apperr.New(http.StatusBadRequest, "REQUEST_VALIDATION_FAILED", "request validation failed", apperr.Field("user_id", "invalid user scope header"))
	}

	return scope, actorID, nil
}

func validateGovernanceID(value string) error {
	if !uuidx.IsValid(strings.TrimSpace(value)) {
		return apperr.New(http.StatusBadRequest, "REQUEST_VALIDATION_FAILED", "request validation failed", apperr.Field("id", "invalid resource id"))
	}
	return nil
}

func writeObjectEnvelope(w http.ResponseWriter, r *http.Request, status int, data any) {
	_ = httpx.WriteJSON(w, status, objectEnvelope{
		RequestID: requestID(r),
		Data:      data,
	})
}

func writeListEnvelope(w http.ResponseWriter, r *http.Request, status int, items any) {
	items = normalizeListItems(items)
	_ = httpx.WriteJSON(w, status, listEnvelope{
		RequestID: requestID(r),
		Items:     items,
	})
}

func writeAcceptedEnvelope(w http.ResponseWriter, r *http.Request, jobID string) {
	_ = httpx.WriteJSON(w, http.StatusAccepted, acceptedEnvelope{
		RequestID: requestID(r),
		Status:    "accepted",
		JobID:     jobID,
	})
}

func writeGovernanceError(w http.ResponseWriter, r *http.Request, err error) {
	var appErr *apperr.Error
	if !errors.As(err, &appErr) {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", nil)
		return
	}

	details := make([]errorDetail, 0, len(appErr.Details))
	for _, detail := range appErr.Details {
		details = append(details, errorDetail{
			Field:  detail.Field,
			Reason: detail.Reason,
		})
	}
	writeError(w, r, appErr.Status, appErr.Code, appErr.Message, details)
}

func headerOrDefault(r *http.Request, key string, fallback string) string {
	value := strings.TrimSpace(r.Header.Get(key))
	if value == "" {
		return fallback
	}
	return value
}

func normalizeListItems(items any) any {
	if items == nil {
		return []any{}
	}

	value := reflect.ValueOf(items)
	if value.Kind() == reflect.Slice && value.IsNil() {
		return reflect.MakeSlice(value.Type(), 0, 0).Interface()
	}

	return items
}
