package issueworkflow

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/zaneway/AutoCertX/internal/domain/resource"
	"github.com/zaneway/AutoCertX/internal/platform/uuidx"
)

const (
	WorkflowTypeIssue = "issue"
	WorkflowTypeRenew = "renew"

	StatusCreated             = "created"
	StatusOrderPending        = "order_pending"
	StatusChallengePending    = "challenge_pending"
	StatusChallengeProcessing = "challenge_processing"
	StatusChallengeValid      = "challenge_valid"
	StatusFinalizing          = "finalizing"
	StatusIssued              = "issued"
	StatusDeployPending       = "deploy_pending"
	StatusDeploying           = "deploying"
	StatusDeployed            = "deployed"
	StatusPartiallyFailed     = "partially_failed"
	StatusFailed              = "failed"
	StatusCancelled           = "cancelled"

	ChallengeStatusPending        = "pending"
	ChallengeStatusPresenting     = "presenting"
	ChallengeStatusPresented      = "presented"
	ChallengeStatusPropagating    = "propagating"
	ChallengeStatusReady          = "ready"
	ChallengeStatusVerifying      = "verifying"
	ChallengeStatusValid          = "valid"
	ChallengeStatusInvalid        = "invalid"
	ChallengeStatusCleanupPending = "cleanup_pending"
	ChallengeStatusCleaned        = "cleaned"
	ChallengeStatusCleanupFailed  = "cleanup_failed"
)

// Workflow is the durable issuance execution fact.
type Workflow struct {
	ID                   string         `json:"id"`
	Scope                resource.Scope `json:"-"`
	CertificateRequestID string         `json:"certificate_request_id"`
	CAAccountID          string         `json:"ca_account_id"`
	WorkflowType         string         `json:"workflow_type"`
	Status               string         `json:"status"`
	OrderURL             string         `json:"order_url,omitempty"`
	FinalizeURL          string         `json:"finalize_url,omitempty"`
	CSRRef               string         `json:"csr_ref,omitempty"`
	CertificateRef       string         `json:"certificate_ref,omitempty"`
	LastErrorCode        string         `json:"last_error_code,omitempty"`
	LastErrorMessage     string         `json:"last_error_message,omitempty"`
	StartedAt            *time.Time     `json:"started_at,omitempty"`
	FinishedAt           *time.Time     `json:"finished_at,omitempty"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
}

// Challenge captures per-identifier challenge execution facts.
type Challenge struct {
	ID               string         `json:"id"`
	Scope            resource.Scope `json:"-"`
	IssueWorkflowID  string         `json:"issue_workflow_id"`
	DomainAssetID    string         `json:"domain_asset_id,omitempty"`
	ChallengeType    string         `json:"challenge_type"`
	Identifier       string         `json:"identifier"`
	Token            string         `json:"token,omitempty"`
	KeyAuthorization string         `json:"key_authorization,omitempty"`
	HTTPPath         string         `json:"http_path,omitempty"`
	DNSRecordName    string         `json:"dns_record_name,omitempty"`
	DNSRecordValue   string         `json:"dns_record_value,omitempty"`
	Status           string         `json:"status"`
	PresentedAt      *time.Time     `json:"presented_at,omitempty"`
	ValidatedAt      *time.Time     `json:"validated_at,omitempty"`
	CleanedAt        *time.Time     `json:"cleaned_at,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

// CreateInput defines the immutable workflow facts required at creation.
type CreateInput struct {
	CertificateRequestID string
	CAAccountID          string
	WorkflowType         string
}

// ChallengeInput defines the challenge facts returned by the ACME adapter.
type ChallengeInput struct {
	DomainAssetID    string
	ChallengeType    string
	Identifier       string
	Token            string
	KeyAuthorization string
	HTTPPath         string
	DNSRecordName    string
	DNSRecordValue   string
}

// Service stores workflows and challenges in memory for the GA baseline.
type Service struct {
	mu                 sync.RWMutex
	now                func() time.Time
	newID              func() string
	byID               map[string]Workflow
	byRequest          map[string]string
	challengesByFlowID map[string][]Challenge
}

// NewService constructs an in-memory workflow service.
func NewService() *Service {
	return &Service{
		now: func() time.Time {
			return time.Now().UTC()
		},
		newID:              uuidx.New,
		byID:               make(map[string]Workflow),
		byRequest:          make(map[string]string),
		challengesByFlowID: make(map[string][]Challenge),
	}
}

// Create stores the unique workflow attached to one request.
func (s *Service) Create(scope resource.Scope, input CreateInput) (Workflow, error) {
	if err := scope.Validate(); err != nil {
		return Workflow{}, err
	}
	requestID := strings.TrimSpace(input.CertificateRequestID)
	caAccountID := strings.TrimSpace(input.CAAccountID)
	workflowType := strings.ToLower(strings.TrimSpace(input.WorkflowType))
	if requestID == "" {
		return Workflow{}, fmt.Errorf("certificate_request_id required: %w", resource.ErrValidation)
	}
	if caAccountID == "" {
		return Workflow{}, fmt.Errorf("ca_account_id required: %w", resource.ErrValidation)
	}
	if workflowType != WorkflowTypeIssue && workflowType != WorkflowTypeRenew {
		return Workflow{}, fmt.Errorf("unsupported workflow_type %q: %w", input.WorkflowType, resource.ErrValidation)
	}

	now := s.now()
	record := Workflow{
		ID:                   s.newID(),
		Scope:                scope,
		CertificateRequestID: requestID,
		CAAccountID:          caAccountID,
		WorkflowType:         workflowType,
		Status:               StatusCreated,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	key := scope.EnvironmentKey() + "/" + requestID

	s.mu.Lock()
	defer s.mu.Unlock()

	if existingID, exists := s.byRequest[key]; exists {
		return Workflow{}, fmt.Errorf("workflow for request %s already exists (%s): %w", requestID, existingID, resource.ErrConflict)
	}

	s.byID[record.ID] = record
	s.byRequest[key] = record.ID
	return record, nil
}

// Get returns one workflow under scope.
func (s *Service) Get(scope resource.Scope, id string) (Workflow, error) {
	if err := scope.Validate(); err != nil {
		return Workflow{}, err
	}
	if strings.TrimSpace(id) == "" {
		return Workflow{}, fmt.Errorf("workflow id required: %w", resource.ErrValidation)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	record, ok := s.byID[id]
	if !ok {
		return Workflow{}, fmt.Errorf("workflow %s: %w", id, resource.ErrNotFound)
	}
	if !record.Scope.Equals(scope) {
		return Workflow{}, fmt.Errorf("workflow %s: %w", id, resource.ErrScopeMismatch)
	}
	return record, nil
}

// GetByRequest returns the unique workflow attached to one request.
func (s *Service) GetByRequest(scope resource.Scope, requestID string) (Workflow, error) {
	if err := scope.Validate(); err != nil {
		return Workflow{}, err
	}
	key := scope.EnvironmentKey() + "/" + strings.TrimSpace(requestID)

	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.byRequest[key]
	if !ok {
		return Workflow{}, fmt.Errorf("workflow for request %s: %w", requestID, resource.ErrNotFound)
	}
	return s.byID[id], nil
}

// ListChallenges returns all challenges for one workflow.
func (s *Service) ListChallenges(scope resource.Scope, workflowID string) ([]Challenge, error) {
	if _, err := s.Get(scope, workflowID); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	items := append([]Challenge(nil), s.challengesByFlowID[workflowID]...)
	slices.SortFunc(items, func(a Challenge, b Challenge) int {
		if a.CreatedAt.Equal(b.CreatedAt) {
			return strings.Compare(a.ID, b.ID)
		}
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		return 1
	})
	return items, nil
}

// RecordOrder stores the upstream order facts and moves the workflow into order_pending.
func (s *Service) RecordOrder(scope resource.Scope, workflowID string, orderURL string, finalizeURL string) (Workflow, error) {
	return s.updateWorkflow(scope, workflowID, func(record *Workflow, now time.Time) error {
		if !canWorkflowTransition(record.Status, StatusOrderPending) {
			return fmt.Errorf("workflow status %s -> %s: %w", record.Status, StatusOrderPending, resource.ErrValidation)
		}
		record.Status = StatusOrderPending
		record.OrderURL = strings.TrimSpace(orderURL)
		record.FinalizeURL = strings.TrimSpace(finalizeURL)
		record.StartedAt = cloneTime(now)
		record.LastErrorCode = ""
		record.LastErrorMessage = ""
		return nil
	})
}

// EnsureChallenges creates the workflow challenges once and transitions to challenge_pending.
func (s *Service) EnsureChallenges(scope resource.Scope, workflowID string, inputs []ChallengeInput) ([]Challenge, Workflow, error) {
	if len(inputs) == 0 {
		return nil, Workflow{}, fmt.Errorf("at least one challenge required: %w", resource.ErrValidation)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.byID[workflowID]
	if !ok {
		return nil, Workflow{}, fmt.Errorf("workflow %s: %w", workflowID, resource.ErrNotFound)
	}
	if !record.Scope.Equals(scope) {
		return nil, Workflow{}, fmt.Errorf("workflow %s: %w", workflowID, resource.ErrScopeMismatch)
	}
	if existing := s.challengesByFlowID[workflowID]; len(existing) > 0 {
		return append([]Challenge(nil), existing...), record, nil
	}
	if !canWorkflowTransition(record.Status, StatusChallengePending) {
		return nil, Workflow{}, fmt.Errorf("workflow status %s -> %s: %w", record.Status, StatusChallengePending, resource.ErrValidation)
	}

	now := s.now()
	items := make([]Challenge, 0, len(inputs))
	for _, input := range inputs {
		challengeType := strings.ToLower(strings.TrimSpace(input.ChallengeType))
		switch challengeType {
		case "http-01", "dns-01":
		default:
			return nil, Workflow{}, fmt.Errorf("unsupported challenge_type %q: %w", input.ChallengeType, resource.ErrValidation)
		}

		identifier := strings.ToLower(strings.TrimSpace(input.Identifier))
		if identifier == "" {
			return nil, Workflow{}, fmt.Errorf("identifier required: %w", resource.ErrValidation)
		}

		items = append(items, Challenge{
			ID:               s.newID(),
			Scope:            scope,
			IssueWorkflowID:  workflowID,
			DomainAssetID:    strings.TrimSpace(input.DomainAssetID),
			ChallengeType:    challengeType,
			Identifier:       identifier,
			Token:            strings.TrimSpace(input.Token),
			KeyAuthorization: strings.TrimSpace(input.KeyAuthorization),
			HTTPPath:         strings.TrimSpace(input.HTTPPath),
			DNSRecordName:    strings.TrimSpace(input.DNSRecordName),
			DNSRecordValue:   strings.TrimSpace(input.DNSRecordValue),
			Status:           ChallengeStatusPending,
			CreatedAt:        now,
			UpdatedAt:        now,
		})
	}

	record.Status = StatusChallengePending
	record.UpdatedAt = now
	record.LastErrorCode = ""
	record.LastErrorMessage = ""
	s.byID[workflowID] = record
	s.challengesByFlowID[workflowID] = items
	return append([]Challenge(nil), items...), record, nil
}

// Transition moves the workflow into the provided next state.
func (s *Service) Transition(scope resource.Scope, workflowID string, nextStatus string) (Workflow, error) {
	return s.updateWorkflow(scope, workflowID, func(record *Workflow, now time.Time) error {
		if !canWorkflowTransition(record.Status, nextStatus) {
			return fmt.Errorf("workflow status %s -> %s: %w", record.Status, nextStatus, resource.ErrValidation)
		}
		record.Status = nextStatus
		record.UpdatedAt = now
		record.LastErrorCode = ""
		record.LastErrorMessage = ""
		return nil
	})
}

// RecordError updates the last error fields without changing terminal state.
func (s *Service) RecordError(scope resource.Scope, workflowID string, code string, message string) (Workflow, error) {
	return s.updateWorkflow(scope, workflowID, func(record *Workflow, now time.Time) error {
		record.LastErrorCode = strings.TrimSpace(code)
		record.LastErrorMessage = strings.TrimSpace(message)
		record.UpdatedAt = now
		return nil
	})
}

// MarkFailed terminates the workflow as failed.
func (s *Service) MarkFailed(scope resource.Scope, workflowID string, code string, message string) (Workflow, error) {
	return s.updateWorkflow(scope, workflowID, func(record *Workflow, now time.Time) error {
		if !canWorkflowTransition(record.Status, StatusFailed) {
			return fmt.Errorf("workflow status %s -> %s: %w", record.Status, StatusFailed, resource.ErrValidation)
		}
		record.Status = StatusFailed
		record.LastErrorCode = strings.TrimSpace(code)
		record.LastErrorMessage = strings.TrimSpace(message)
		record.FinishedAt = cloneTime(now)
		return nil
	})
}

// MarkIssued stores the certificate ref and transitions into issued.
func (s *Service) MarkIssued(scope resource.Scope, workflowID string, certificateRef string) (Workflow, error) {
	return s.updateWorkflow(scope, workflowID, func(record *Workflow, now time.Time) error {
		if !canWorkflowTransition(record.Status, StatusIssued) {
			return fmt.Errorf("workflow status %s -> %s: %w", record.Status, StatusIssued, resource.ErrValidation)
		}
		record.Status = StatusIssued
		record.CertificateRef = strings.TrimSpace(certificateRef)
		record.FinishedAt = cloneTime(now)
		record.LastErrorCode = ""
		record.LastErrorMessage = ""
		return nil
	})
}

// UpdateChallengeStatus transitions one challenge to the requested next state.
func (s *Service) UpdateChallengeStatus(scope resource.Scope, workflowID string, challengeID string, nextStatus string) (Challenge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.byID[workflowID]
	if !ok {
		return Challenge{}, fmt.Errorf("workflow %s: %w", workflowID, resource.ErrNotFound)
	}
	if !record.Scope.Equals(scope) {
		return Challenge{}, fmt.Errorf("workflow %s: %w", workflowID, resource.ErrScopeMismatch)
	}

	items := s.challengesByFlowID[workflowID]
	for idx := range items {
		if items[idx].ID != challengeID {
			continue
		}
		if !canChallengeTransition(items[idx].Status, nextStatus) {
			return Challenge{}, fmt.Errorf("challenge status %s -> %s: %w", items[idx].Status, nextStatus, resource.ErrValidation)
		}
		now := s.now()
		items[idx].Status = nextStatus
		items[idx].UpdatedAt = now
		switch nextStatus {
		case ChallengeStatusPresented:
			items[idx].PresentedAt = cloneTime(now)
		case ChallengeStatusValid:
			items[idx].ValidatedAt = cloneTime(now)
		case ChallengeStatusCleaned, ChallengeStatusCleanupFailed:
			items[idx].CleanedAt = cloneTime(now)
		}
		s.challengesByFlowID[workflowID] = items
		return items[idx], nil
	}

	return Challenge{}, fmt.Errorf("challenge %s: %w", challengeID, resource.ErrNotFound)
}

func (s *Service) updateWorkflow(scope resource.Scope, workflowID string, update func(*Workflow, time.Time) error) (Workflow, error) {
	if err := scope.Validate(); err != nil {
		return Workflow{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.byID[workflowID]
	if !ok {
		return Workflow{}, fmt.Errorf("workflow %s: %w", workflowID, resource.ErrNotFound)
	}
	if !record.Scope.Equals(scope) {
		return Workflow{}, fmt.Errorf("workflow %s: %w", workflowID, resource.ErrScopeMismatch)
	}

	now := s.now()
	if err := update(&record, now); err != nil {
		return Workflow{}, err
	}
	record.UpdatedAt = now
	s.byID[workflowID] = record
	return record, nil
}

func canWorkflowTransition(current string, next string) bool {
	switch current {
	case StatusCreated:
		return next == StatusOrderPending || next == StatusCancelled || next == StatusFailed
	case StatusOrderPending:
		return next == StatusChallengePending || next == StatusFailed
	case StatusChallengePending:
		return next == StatusChallengeProcessing || next == StatusCancelled || next == StatusFailed
	case StatusChallengeProcessing:
		return next == StatusChallengeValid || next == StatusFailed
	case StatusChallengeValid:
		return next == StatusFinalizing
	case StatusFinalizing:
		return next == StatusIssued || next == StatusFailed
	case StatusIssued:
		return false
	default:
		return false
	}
}

func canChallengeTransition(current string, next string) bool {
	switch current {
	case ChallengeStatusPending:
		return next == ChallengeStatusPresenting
	case ChallengeStatusPresenting:
		return next == ChallengeStatusPresented
	case ChallengeStatusPresented:
		return next == ChallengeStatusPropagating || next == ChallengeStatusReady
	case ChallengeStatusPropagating:
		return next == ChallengeStatusReady
	case ChallengeStatusReady:
		return next == ChallengeStatusVerifying
	case ChallengeStatusVerifying:
		return next == ChallengeStatusValid || next == ChallengeStatusInvalid
	case ChallengeStatusInvalid:
		return next == ChallengeStatusCleanupPending
	case ChallengeStatusCleanupPending:
		return next == ChallengeStatusCleaned || next == ChallengeStatusCleanupFailed
	default:
		return false
	}
}

func cloneTime(value time.Time) *time.Time {
	cloned := value
	return &cloned
}
