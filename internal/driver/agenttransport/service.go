package agenttransport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/zaneway/AutoCertX/internal/domain/agentnode"
	"github.com/zaneway/AutoCertX/internal/domain/resource"
	"github.com/zaneway/AutoCertX/internal/platform/apperr"
	"github.com/zaneway/AutoCertX/internal/platform/uuidx"
)

const (
	SchemaVersionV1 = 1

	JobTypePresentHTTP01Challenge     = "present_http01_challenge"
	JobTypeCleanupHTTP01Challenge     = "cleanup_http01_challenge"
	JobTypeDeployNGINXCertificate     = "deploy_nginx_certificate"
	JobTypeDeployTomcatCertificate    = "deploy_tomcat_certificate"
	JobTypeVerifyNGINXDeployment      = "verify_nginx_deployment"
	JobTypeVerifyTomcatDeployment     = "verify_tomcat_deployment"
	JobTypeDiscoverNGINXCertificates  = "discover_nginx_certificates"
	JobTypeDiscoverTomcatCertificates = "discover_tomcat_certificates"

	progressStatusStarted      = "started"
	progressStatusHeartbeating = "heartbeating"
	progressStatusRunning      = "running"

	resultStatusSucceeded = "succeeded"
	resultStatusFailed    = "failed"
	resultStatusTimedOut  = "timed_out"
	resultStatusAbandoned = "abandoned"
)

// LeaseReport carries one active operation observed by the Agent.
type LeaseReport struct {
	JobID       string `json:"job_id"`
	OperationID string `json:"operation_id"`
	Status      string `json:"status"`
}

// RegisterRequest is the transport payload for first-time bootstrap.
type RegisterRequest struct {
	Token           string            `json:"token"`
	NodeName        string            `json:"node_name"`
	Hostname        string            `json:"hostname,omitempty"`
	IPAddress       string            `json:"ip_address,omitempty"`
	OS              string            `json:"os,omitempty"`
	Arch            string            `json:"arch,omitempty"`
	Version         string            `json:"version"`
	ProtocolVersion int               `json:"protocol_version"`
	Capabilities    []string          `json:"capabilities,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
}

// RegisterResponse is returned to one freshly registered Agent.
type RegisterResponse struct {
	RequestID           string `json:"request_id,omitempty"`
	NodeID              string `json:"node_id"`
	Status              string `json:"status"`
	PollIntervalSeconds int    `json:"poll_interval_seconds"`
}

// HeartbeatRequest carries liveness and capability facts from one Agent.
type HeartbeatRequest struct {
	NodeID          string        `json:"node_id"`
	ProtocolVersion int           `json:"protocol_version"`
	Status          string        `json:"status"`
	Version         string        `json:"version,omitempty"`
	Capabilities    []string      `json:"capabilities,omitempty"`
	Leases          []LeaseReport `json:"leases,omitempty"`
}

// HeartbeatResponse acknowledges one Agent heartbeat.
type HeartbeatResponse struct {
	RequestID  string    `json:"request_id,omitempty"`
	Status     string    `json:"status"`
	ServerTime time.Time `json:"server_time"`
}

// JobPollRequest asks the control plane for pending work.
type JobPollRequest struct {
	NodeID                  string `json:"node_id"`
	ProtocolVersion         int    `json:"protocol_version"`
	SupportedSchemaVersions []int  `json:"supported_schema_versions"`
	MaxJobs                 int    `json:"max_jobs,omitempty"`
}

// JobPollResponse returns all jobs currently leased to one Agent poll.
type JobPollResponse struct {
	RequestID string `json:"request_id,omitempty"`
	Items     []Job  `json:"items"`
}

// Job is the Agent-facing execution envelope.
type Job struct {
	JobID         string         `json:"job_id"`
	JobType       string         `json:"job_type"`
	SchemaVersion int            `json:"schema_version"`
	OperationID   string         `json:"operation_id"`
	LeaseExpireAt time.Time      `json:"lease_expire_at"`
	Payload       map[string]any `json:"payload"`
}

// JobProgressRequest reports incremental execution progress.
type JobProgressRequest struct {
	OperationID     string         `json:"operation_id"`
	Status          string         `json:"status"`
	Message         string         `json:"message,omitempty"`
	ErrorCode       string         `json:"error_code,omitempty"`
	ProgressPercent *int           `json:"progress_percent,omitempty"`
	Evidence        map[string]any `json:"evidence,omitempty"`
}

// JobCompleteRequest reports one terminal execution result.
type JobCompleteRequest struct {
	OperationID          string         `json:"operation_id"`
	ResultStatus         string         `json:"result_status"`
	ErrorCode            string         `json:"error_code,omitempty"`
	ErrorMessage         string         `json:"error_message,omitempty"`
	FailedStage          string         `json:"failed_stage,omitempty"`
	Retryable            bool           `json:"retryable,omitempty"`
	CompensationRequired bool           `json:"compensation_required,omitempty"`
	Evidence             map[string]any `json:"evidence,omitempty"`
}

// DispatchInput creates one Agent-visible job assignment for a chosen node.
type DispatchInput struct {
	Scope         resource.Scope
	NodeID        string
	JobType       string
	SchemaVersion int
	OperationID   string
	Payload       map[string]any
}

// Options tune the fake transport runtime used in Phase A.
type Options struct {
	Clock              func() time.Time
	NewID              func() string
	PollInterval       time.Duration
	LeaseTTL           time.Duration
	ProgressCallback   func(context.Context, JobProgressRequest) error
	CompletionCallback func(context.Context, JobCompleteRequest) error
}

// Service owns the Phase A pull-style Agent transport contract.
type Service struct {
	mu                 sync.Mutex
	nodes              *agentnode.Service
	now                func() time.Time
	newID              func() string
	pollInterval       time.Duration
	leaseTTL           time.Duration
	progressCallback   func(context.Context, JobProgressRequest) error
	completionCallback func(context.Context, JobCompleteRequest) error
	jobsByID           map[string]*jobRecord
	queueByNode        map[string][]string
}

type jobRecord struct {
	scope       resource.Scope
	nodeID      string
	job         Job
	progress    JobProgressRequest
	completion  JobCompleteRequest
	completed   bool
	completedAt time.Time
}

// NewService constructs the minimal Agent transport baseline.
func NewService(nodeService *agentnode.Service, opts Options) (*Service, error) {
	if nodeService == nil {
		return nil, fmt.Errorf("agent transport node service required")
	}

	now := opts.Clock
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	newID := opts.NewID
	if newID == nil {
		newID = uuidx.New
	}
	pollInterval := opts.PollInterval
	if pollInterval <= 0 {
		pollInterval = 15 * time.Second
	}
	leaseTTL := opts.LeaseTTL
	if leaseTTL <= 0 {
		leaseTTL = 30 * time.Second
	}

	return &Service{
		nodes:              nodeService,
		now:                now,
		newID:              newID,
		pollInterval:       pollInterval,
		leaseTTL:           leaseTTL,
		progressCallback:   opts.ProgressCallback,
		completionCallback: opts.CompletionCallback,
		jobsByID:           make(map[string]*jobRecord),
		queueByNode:        make(map[string][]string),
	}, nil
}

// Register validates one bootstrap token and creates or reuses one node record.
func (s *Service) Register(_ context.Context, req RegisterRequest) (RegisterResponse, error) {
	token, err := s.nodes.ResolveRegistrationToken(req.Token)
	if err != nil {
		return RegisterResponse{}, translateRegistrationTokenError(err)
	}

	node, err := s.nodes.Register(token.Scope, agentnode.RegistrationInput{
		Name:            req.NodeName,
		Hostname:        req.Hostname,
		IPAddress:       req.IPAddress,
		Version:         req.Version,
		ProtocolVersion: req.ProtocolVersion,
		OS:              req.OS,
		Arch:            req.Arch,
		Labels:          req.Labels,
		Capabilities:    req.Capabilities,
	})
	if err != nil {
		if errors.Is(err, resource.ErrConflict) {
			existing, lookupErr := s.nodes.FindByName(token.Scope, req.NodeName)
			if lookupErr == nil {
				return RegisterResponse{
					NodeID:              existing.ID,
					Status:              existing.Status,
					PollIntervalSeconds: int(s.pollInterval / time.Second),
				}, nil
			}
		}
		return RegisterResponse{}, translateNodeError(err)
	}

	return RegisterResponse{
		NodeID:              node.ID,
		Status:              node.Status,
		PollIntervalSeconds: int(s.pollInterval / time.Second),
	}, nil
}

// Heartbeat updates one node's liveness facts.
func (s *Service) Heartbeat(_ context.Context, req HeartbeatRequest) (HeartbeatResponse, error) {
	node, err := s.nodes.Lookup(req.NodeID)
	if err != nil {
		return HeartbeatResponse{}, translateNodeError(err)
	}

	if _, err := s.nodes.Heartbeat(node.Scope, node.ID, agentnode.HeartbeatInput{
		Version:         req.Version,
		ProtocolVersion: req.ProtocolVersion,
		Status:          req.Status,
		Capabilities:    req.Capabilities,
	}); err != nil {
		return HeartbeatResponse{}, translateNodeError(err)
	}

	return HeartbeatResponse{
		Status:     "ok",
		ServerTime: s.now(),
	}, nil
}

// Dispatch enqueues one job for one already selected Agent.
func (s *Service) Dispatch(_ context.Context, input DispatchInput) (Job, error) {
	if err := input.Scope.Validate(); err != nil {
		return Job{}, translateNodeError(err)
	}
	if strings.TrimSpace(input.NodeID) == "" {
		return Job{}, validationError("node_id", "required")
	}
	if strings.TrimSpace(input.OperationID) == "" {
		return Job{}, validationError("operation_id", "required")
	}

	node, err := s.nodes.Lookup(input.NodeID)
	if err != nil {
		return Job{}, translateNodeError(err)
	}
	if !node.Scope.Equals(input.Scope) {
		return Job{}, apperr.New(http.StatusConflict, "TENANT_SCOPE_MISMATCH", "tenant scope mismatch")
	}

	requiredCapability, err := capabilityForJobType(input.JobType)
	if err != nil {
		return Job{}, err
	}
	if requiredCapability != "" && !containsCapability(node.Capabilities, requiredCapability) {
		return Job{}, apperr.New(http.StatusConflict, "AGENT_CAPABILITY_MISMATCH", "agent capability mismatch", apperr.Field("job_type", input.JobType))
	}

	schemaVersion := input.SchemaVersion
	if schemaVersion == 0 {
		schemaVersion = SchemaVersionV1
	}
	if schemaVersion != SchemaVersionV1 {
		return Job{}, validationError("schema_version", "unsupported")
	}

	payload, err := clonePayload(input.Payload)
	if err != nil {
		return Job{}, apperr.Wrap(err, http.StatusBadRequest, "REQUEST_VALIDATION_FAILED", "request validation failed", apperr.Field("payload", "invalid"))
	}
	if payload == nil {
		payload = map[string]any{}
	}
	payload["schema_version"] = schemaVersion
	payload["operation_id"] = strings.TrimSpace(input.OperationID)

	now := s.now()
	record := &jobRecord{
		scope:  input.Scope,
		nodeID: node.ID,
		job: Job{
			JobID:         s.newID(),
			JobType:       strings.TrimSpace(input.JobType),
			SchemaVersion: schemaVersion,
			OperationID:   strings.TrimSpace(input.OperationID),
			LeaseExpireAt: time.Time{},
			Payload:       payload,
		},
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobsByID[record.job.JobID] = record
	s.queueByNode[node.ID] = append(s.queueByNode[node.ID], record.job.JobID)
	record.job.LeaseExpireAt = now

	return cloneJob(record.job), nil
}

// Poll leases a bounded set of compatible jobs to one Agent.
func (s *Service) Poll(_ context.Context, req JobPollRequest) (JobPollResponse, error) {
	if strings.TrimSpace(req.NodeID) == "" {
		return JobPollResponse{}, validationError("node_id", "required")
	}
	if req.ProtocolVersion < agentnode.MinimumSupportedProtocolVersion {
		return JobPollResponse{}, validationError("protocol_version", "unsupported")
	}
	if !supportsSchema(req.SupportedSchemaVersions, SchemaVersionV1) {
		return JobPollResponse{}, validationError("supported_schema_versions", "missing_supported_schema")
	}

	node, err := s.nodes.Lookup(req.NodeID)
	if err != nil {
		return JobPollResponse{}, translateNodeError(err)
	}

	maxJobs := req.MaxJobs
	if maxJobs <= 0 {
		maxJobs = 1
	}
	if maxJobs > 20 {
		maxJobs = 20
	}
	if node.Status != agentnode.StatusOnline && node.Status != agentnode.StatusDegraded {
		return JobPollResponse{Items: []Job{}}, nil
	}

	now := s.now()
	items := make([]Job, 0, maxJobs)

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, jobID := range s.queueByNode[node.ID] {
		record := s.jobsByID[jobID]
		if record == nil || record.completed {
			continue
		}
		if record.job.SchemaVersion != SchemaVersionV1 {
			continue
		}
		if !record.job.LeaseExpireAt.IsZero() && record.job.LeaseExpireAt.After(now) {
			continue
		}

		record.job.LeaseExpireAt = now.Add(s.leaseTTL)
		items = append(items, cloneJob(record.job))
		if len(items) >= maxJobs {
			break
		}
	}

	return JobPollResponse{Items: items}, nil
}

// ReportProgress updates the last known Agent-side execution heartbeat.
func (s *Service) ReportProgress(ctx context.Context, nodeID string, jobID string, req JobProgressRequest) error {
	if strings.TrimSpace(nodeID) == "" {
		return validationError("node_id", "required")
	}
	if strings.TrimSpace(req.OperationID) == "" {
		return validationError("operation_id", "required")
	}
	if !validProgressStatus(req.Status) {
		return validationError("status", "invalid")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.requireOwnedJobLocked(nodeID, jobID, req.OperationID)
	if err != nil {
		return err
	}
	if record.completed {
		return nil
	}

	progress := JobProgressRequest{
		OperationID:     strings.TrimSpace(req.OperationID),
		Status:          strings.TrimSpace(req.Status),
		Message:         strings.TrimSpace(req.Message),
		ErrorCode:       strings.TrimSpace(req.ErrorCode),
		ProgressPercent: cloneIntPointer(req.ProgressPercent),
		Evidence:        cloneEvidence(req.Evidence),
	}
	if s.progressCallback != nil {
		if err := s.progressCallback(ctx, progress); err != nil {
			return err
		}
	}

	record.progress = progress
	record.job.LeaseExpireAt = s.now().Add(s.leaseTTL)
	return nil
}

// CompleteJob records one terminal result for one Agent-side job.
func (s *Service) CompleteJob(ctx context.Context, nodeID string, jobID string, req JobCompleteRequest) error {
	if strings.TrimSpace(nodeID) == "" {
		return validationError("node_id", "required")
	}
	if strings.TrimSpace(req.OperationID) == "" {
		return validationError("operation_id", "required")
	}
	if !validResultStatus(req.ResultStatus) {
		return validationError("result_status", "invalid")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.requireOwnedJobLocked(nodeID, jobID, req.OperationID)
	if err != nil {
		return err
	}
	if record.completed {
		if record.completion.ResultStatus == strings.TrimSpace(req.ResultStatus) &&
			record.completion.OperationID == strings.TrimSpace(req.OperationID) {
			return nil
		}
		return apperr.New(http.StatusConflict, "RESOURCE_CONFLICT", "resource conflict", apperr.Field("operation_id", "job already completed"))
	}

	completion := JobCompleteRequest{
		OperationID:          strings.TrimSpace(req.OperationID),
		ResultStatus:         strings.TrimSpace(req.ResultStatus),
		ErrorCode:            strings.TrimSpace(req.ErrorCode),
		ErrorMessage:         strings.TrimSpace(req.ErrorMessage),
		FailedStage:          strings.TrimSpace(req.FailedStage),
		Retryable:            req.Retryable,
		CompensationRequired: req.CompensationRequired,
		Evidence:             cloneEvidence(req.Evidence),
	}
	if s.completionCallback != nil {
		if err := s.completionCallback(ctx, completion); err != nil {
			return err
		}
	}

	record.completed = true
	record.completedAt = s.now()
	record.completion = completion
	record.job.LeaseExpireAt = time.Time{}
	return nil
}

func (s *Service) requireOwnedJobLocked(nodeID string, jobID string, operationID string) (*jobRecord, error) {
	record, ok := s.jobsByID[strings.TrimSpace(jobID)]
	if !ok {
		return nil, apperr.New(http.StatusNotFound, "RESOURCE_NOT_FOUND", "resource not found")
	}
	if record.nodeID != strings.TrimSpace(nodeID) {
		return nil, apperr.New(http.StatusConflict, "TENANT_SCOPE_MISMATCH", "tenant scope mismatch")
	}
	if record.job.OperationID != strings.TrimSpace(operationID) {
		return nil, apperr.New(http.StatusConflict, "RESOURCE_CONFLICT", "resource conflict", apperr.Field("operation_id", "mismatch"))
	}
	return record, nil
}

func capabilityForJobType(jobType string) (string, error) {
	switch strings.TrimSpace(jobType) {
	case JobTypePresentHTTP01Challenge, JobTypeCleanupHTTP01Challenge:
		return agentnode.CapabilityChallengeHTTP01, nil
	case JobTypeDeployNGINXCertificate:
		return agentnode.CapabilityDeployNGINX, nil
	case JobTypeDeployTomcatCertificate:
		return agentnode.CapabilityDeployTomcatPKCS12, nil
	case JobTypeVerifyNGINXDeployment:
		return agentnode.CapabilityVerifyNGINX, nil
	case JobTypeVerifyTomcatDeployment:
		return agentnode.CapabilityVerifyTomcat, nil
	case JobTypeDiscoverNGINXCertificates:
		return agentnode.CapabilityDiscoverNGINX, nil
	case JobTypeDiscoverTomcatCertificates:
		return agentnode.CapabilityDiscoverTomcat, nil
	default:
		return "", validationError("job_type", "invalid")
	}
}

func validProgressStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case progressStatusStarted, progressStatusHeartbeating, progressStatusRunning:
		return true
	default:
		return false
	}
}

func validResultStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case resultStatusSucceeded, resultStatusFailed, resultStatusTimedOut, resultStatusAbandoned:
		return true
	default:
		return false
	}
}

func supportsSchema(items []int, expected int) bool {
	for _, item := range items {
		if item == expected {
			return true
		}
	}
	return false
}

func containsCapability(items []string, expected string) bool {
	for _, item := range items {
		if item == expected {
			return true
		}
	}
	return false
}

func cloneJob(job Job) Job {
	job.Payload = cloneEvidence(job.Payload)
	return job
}

func cloneEvidence(values map[string]any) map[string]any {
	if len(values) == 0 {
		return map[string]any{}
	}
	clone, err := clonePayload(values)
	if err != nil {
		return map[string]any{}
	}
	return clone
}

func clonePayload(values map[string]any) (map[string]any, error) {
	if values == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	var clone map[string]any
	if err := json.Unmarshal(encoded, &clone); err != nil {
		return nil, err
	}
	return clone, nil
}

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func validationError(field string, reason string) error {
	return apperr.New(http.StatusBadRequest, "REQUEST_VALIDATION_FAILED", "request validation failed", apperr.Field(field, reason))
}

func translateRegistrationTokenError(err error) error {
	switch {
	case errors.Is(err, resource.ErrValidation):
		return validationError("token", "required")
	case errors.Is(err, resource.ErrNotFound), errors.Is(err, resource.ErrUnavailable):
		return apperr.New(http.StatusUnauthorized, "AGENT_REGISTRATION_DENIED", "agent registration denied")
	default:
		return apperr.Wrap(err, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}

func translateNodeError(err error) error {
	switch {
	case errors.Is(err, resource.ErrValidation):
		return apperr.Wrap(err, http.StatusBadRequest, "REQUEST_VALIDATION_FAILED", "request validation failed", apperr.Field("request", err.Error()))
	case errors.Is(err, resource.ErrNotFound):
		return apperr.Wrap(err, http.StatusNotFound, "RESOURCE_NOT_FOUND", "resource not found")
	case errors.Is(err, resource.ErrConflict):
		return apperr.Wrap(err, http.StatusConflict, "RESOURCE_CONFLICT", "resource conflict")
	case errors.Is(err, resource.ErrScopeMismatch):
		return apperr.Wrap(err, http.StatusConflict, "TENANT_SCOPE_MISMATCH", "tenant scope mismatch")
	case errors.Is(err, resource.ErrUnavailable):
		return apperr.Wrap(err, http.StatusConflict, "RESOURCE_UNAVAILABLE", "resource unavailable")
	default:
		return apperr.Wrap(err, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}
