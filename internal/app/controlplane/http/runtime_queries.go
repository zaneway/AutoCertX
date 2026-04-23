package httpserver

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	dashboardquery "github.com/zaneway/AutoCertX/internal/application/query/dashboard"
	jobsquery "github.com/zaneway/AutoCertX/internal/application/query/jobs"
	"github.com/zaneway/AutoCertX/internal/domain/tenancy"
)

// runtimeQueryHandler serves the dashboard and jobs read endpoints introduced in T07.
type runtimeQueryHandler struct {
	dashboard *dashboardquery.Service
	jobs      *jobsquery.Service
}

func registerRuntimeQueryRoutes(mux *http.ServeMux, deps Deps) {
	if deps.DashboardQuery == nil && deps.JobsQuery == nil {
		return
	}

	handler := runtimeQueryHandler{
		dashboard: deps.DashboardQuery,
		jobs:      deps.JobsQuery,
	}

	handleRead := func(pattern string, fn http.HandlerFunc) {
		var endpoint http.Handler = fn
		if deps.AuthService != nil && deps.AuthContextQuery != nil {
			authz := authHandler{
				authService:        deps.AuthService,
				authContextService: deps.AuthContextQuery,
			}
			endpoint = authz.withAuthentication(authz.withPermissions(endpoint, tenancy.PermissionAuthContextRead))
		}
		mux.Handle(pattern, endpoint)
	}

	if deps.DashboardQuery != nil {
		handleRead("GET /api/v1/dashboard/summary", handler.getDashboardSummary)
		handleRead("GET /api/v1/dashboard/job-failures", handler.listDashboardJobFailures)
	}
	if deps.JobsQuery != nil {
		handleRead("GET /api/v1/jobs", handler.listJobs)
		handleRead("GET /api/v1/jobs/{id}", handler.getJob)
		handleRead("GET /api/v1/jobs/{id}/attempts", handler.listJobAttempts)
	}
}

func (h runtimeQueryHandler) getDashboardSummary(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveReadScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	item, err := h.dashboard.GetSummary(r.Context(), scope)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeObjectEnvelope(w, r, http.StatusOK, item)
}

func (h runtimeQueryHandler) listDashboardJobFailures(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveReadScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	limit, err := parseOptionalLimit(r, 10)
	if err != nil {
		writeValidationError(w, r, "limit", "invalid")
		return
	}

	items, err := h.dashboard.ListJobFailures(r.Context(), scope, limit)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeListEnvelope(w, r, http.StatusOK, items)
}

func (h runtimeQueryHandler) listJobs(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveReadScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	limit, err := parseOptionalLimit(r, 0)
	if err != nil {
		writeValidationError(w, r, "limit", "invalid")
		return
	}

	items, err := h.jobs.ListJobs(r.Context(), scope, jobsquery.ListFilter{
		Status:        r.URL.Query().Get("status"),
		JobType:       r.URL.Query().Get("job_type"),
		AggregateType: r.URL.Query().Get("aggregate_type"),
		AggregateID:   r.URL.Query().Get("aggregate_id"),
		WorkerID:      r.URL.Query().Get("worker_id"),
		Limit:         limit,
	})
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeListEnvelope(w, r, http.StatusOK, items)
}

func (h runtimeQueryHandler) getJob(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveReadScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	id := r.PathValue("id")
	if err := validateGovernanceID(id); err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	item, err := h.jobs.GetJob(r.Context(), scope, id)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeObjectEnvelope(w, r, http.StatusOK, item)
}

func (h runtimeQueryHandler) listJobAttempts(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveReadScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	id := r.PathValue("id")
	if err := validateGovernanceID(id); err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	items, err := h.jobs.ListJobAttempts(r.Context(), scope, id)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeListEnvelope(w, r, http.StatusOK, items)
}

func parseOptionalLimit(r *http.Request, fallback int) (int, error) {
	value := strings.TrimSpace(r.URL.Query().Get("limit"))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return 0, fmt.Errorf("limit must be >= 1")
	}
	return parsed, nil
}
