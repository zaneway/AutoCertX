package httpserver

import (
	"errors"
	"net/http"

	"github.com/zaneway/AutoCertX/internal/driver/agenttransport"
	"github.com/zaneway/AutoCertX/internal/platform/apperr"
	"github.com/zaneway/AutoCertX/internal/platform/httpx"
)

type agentTransportHandler struct {
	transport *agenttransport.Service
}

func registerAgentTransportRoutes(mux *http.ServeMux, deps Deps) {
	if deps.AgentTransport == nil {
		return
	}

	handler := agentTransportHandler{transport: deps.AgentTransport}
	mux.HandleFunc("POST /agent/v1/register", handler.register)
	mux.HandleFunc("POST /agent/v1/heartbeat", handler.heartbeat)
	mux.HandleFunc("POST /agent/v1/jobs/poll", handler.pollJobs)
	mux.HandleFunc("POST /agent/v1/jobs/{jobId}/progress", handler.reportProgress)
	mux.HandleFunc("POST /agent/v1/jobs/{jobId}/complete", handler.completeJob)
}

func (h agentTransportHandler) register(w http.ResponseWriter, r *http.Request) {
	var req agenttransport.RegisterRequest
	if err := decodeJSON(r, &req); err != nil {
		writeValidationError(w, r, "body", "invalid_json")
		return
	}

	resp, err := h.transport.Register(r.Context(), req)
	if err != nil {
		writeAgentTransportError(w, r, err)
		return
	}
	resp.RequestID = requestID(r)
	_ = httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h agentTransportHandler) heartbeat(w http.ResponseWriter, r *http.Request) {
	var req agenttransport.HeartbeatRequest
	if err := decodeJSON(r, &req); err != nil {
		writeValidationError(w, r, "body", "invalid_json")
		return
	}

	resp, err := h.transport.Heartbeat(r.Context(), req)
	if err != nil {
		writeAgentTransportError(w, r, err)
		return
	}
	resp.RequestID = requestID(r)
	_ = httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h agentTransportHandler) pollJobs(w http.ResponseWriter, r *http.Request) {
	var req agenttransport.JobPollRequest
	if err := decodeJSON(r, &req); err != nil {
		writeValidationError(w, r, "body", "invalid_json")
		return
	}

	resp, err := h.transport.Poll(r.Context(), req)
	if err != nil {
		writeAgentTransportError(w, r, err)
		return
	}
	resp.RequestID = requestID(r)
	_ = httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h agentTransportHandler) reportProgress(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobId")
	if err := validateGovernanceID(jobID); err != nil {
		writeAgentTransportError(w, r, err)
		return
	}

	var req agenttransport.JobProgressRequest
	if err := decodeJSON(r, &req); err != nil {
		writeValidationError(w, r, "body", "invalid_json")
		return
	}

	nodeID := r.Header.Get("X-Agent-Node-Id")
	if err := h.transport.ReportProgress(r.Context(), nodeID, jobID, req); err != nil {
		writeAgentTransportError(w, r, err)
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, emptyEnvelope{
		RequestID: requestID(r),
		Status:    "ok",
	})
}

func (h agentTransportHandler) completeJob(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobId")
	if err := validateGovernanceID(jobID); err != nil {
		writeAgentTransportError(w, r, err)
		return
	}

	var req agenttransport.JobCompleteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeValidationError(w, r, "body", "invalid_json")
		return
	}

	nodeID := r.Header.Get("X-Agent-Node-Id")
	if err := h.transport.CompleteJob(r.Context(), nodeID, jobID, req); err != nil {
		writeAgentTransportError(w, r, err)
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, emptyEnvelope{
		RequestID: requestID(r),
		Status:    "ok",
	})
}

func writeAgentTransportError(w http.ResponseWriter, r *http.Request, err error) {
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
