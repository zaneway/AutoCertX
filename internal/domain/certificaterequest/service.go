package certificaterequest

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
	RequestTypeIssue = "issue"
	RequestTypeRenew = "renew"

	RequestSourceManual    = "manual"
	RequestSourceScheduled = "scheduled"

	AlgorithmRSA = "rsa"

	CertificateTypeSingle   = "single"
	CertificateTypeSAN      = "san"
	CertificateTypeWildcard = "wildcard"

	ChallengeTypeHTTP01 = "http-01"
	ChallengeTypeDNS01  = "dns-01"

	StatusSubmitted = "submitted"
	StatusAccepted  = "accepted"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"

	RelationPrimary  = "primary"
	RelationSAN      = "san"
	RelationWildcard = "wildcard"
)

// DomainRef keeps the scoped domain set attached to one certificate request.
type DomainRef struct {
	DomainID      string `json:"domain_id"`
	RelationType  string `json:"relation_type"`
	SortOrder     int    `json:"sort_order"`
	DomainName    string `json:"domain_name,omitempty"`
	AllowWildcard bool   `json:"allow_wildcard,omitempty"`
}

// Request is the user-facing issuance or renewal request fact.
type Request struct {
	ID              string         `json:"id"`
	Scope           resource.Scope `json:"-"`
	RequestType     string         `json:"request_type"`
	RequestSource   string         `json:"request_source"`
	AssetID         string         `json:"asset_id,omitempty"`
	CAAccountID     string         `json:"ca_account_id"`
	Algorithm       string         `json:"algorithm"`
	CertificateType string         `json:"certificate_type"`
	ChallengeType   string         `json:"challenge_type"`
	CommonName      string         `json:"common_name"`
	Status          string         `json:"status"`
	RequestedBy     string         `json:"requested_by,omitempty"`
	IdempotencyKey  string         `json:"idempotency_key"`
	Reason          string         `json:"reason,omitempty"`
	SubmittedAt     *time.Time     `json:"submitted_at,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// CreateInput is the request write model used by the orchestration layer.
type CreateInput struct {
	RequestType     string
	RequestSource   string
	AssetID         string
	CAAccountID     string
	CertificateType string
	ChallengeType   string
	CommonName      string
	RequestedBy     string
	IdempotencyKey  string
	Reason          string
	Domains         []DomainRef
}

// Service stores certificate request facts in memory for the GA baseline.
type Service struct {
	mu             sync.RWMutex
	now            func() time.Time
	newID          func() string
	byID           map[string]Request
	byIdempotency  map[string]string
	domainsByReqID map[string][]DomainRef
}

// NewService constructs an in-memory certificate request service.
func NewService() *Service {
	return &Service{
		now: func() time.Time {
			return time.Now().UTC()
		},
		newID:          uuidx.New,
		byID:           make(map[string]Request),
		byIdempotency:  make(map[string]string),
		domainsByReqID: make(map[string][]DomainRef),
	}
}

// Create stores a new request under the provided scope.
func (s *Service) Create(scope resource.Scope, input CreateInput) (Request, error) {
	if err := scope.Validate(); err != nil {
		return Request{}, err
	}

	normalized, refs, idemKey, err := normalizeCreateInput(input)
	if err != nil {
		return Request{}, err
	}

	now := s.now()
	record := Request{
		ID:              s.newID(),
		Scope:           scope,
		RequestType:     normalized.RequestType,
		RequestSource:   normalized.RequestSource,
		AssetID:         normalized.AssetID,
		CAAccountID:     normalized.CAAccountID,
		Algorithm:       AlgorithmRSA,
		CertificateType: normalized.CertificateType,
		ChallengeType:   normalized.ChallengeType,
		CommonName:      normalized.CommonName,
		Status:          StatusSubmitted,
		RequestedBy:     normalized.RequestedBy,
		IdempotencyKey:  idemKey,
		Reason:          normalized.Reason,
		SubmittedAt:     timePointer(now),
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	key := scope.EnvironmentKey() + "/" + idemKey

	s.mu.Lock()
	defer s.mu.Unlock()

	if existingID, exists := s.byIdempotency[key]; exists {
		return Request{}, fmt.Errorf("certificate request %q already exists (%s): %w", idemKey, existingID, resource.ErrConflict)
	}

	s.byID[record.ID] = record
	s.byIdempotency[key] = record.ID
	s.domainsByReqID[record.ID] = refs
	return record, nil
}

// Get returns one request under the caller scope.
func (s *Service) Get(scope resource.Scope, id string) (Request, error) {
	if err := scope.Validate(); err != nil {
		return Request{}, err
	}
	if strings.TrimSpace(id) == "" {
		return Request{}, fmt.Errorf("certificate request id required: %w", resource.ErrValidation)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	record, ok := s.byID[id]
	if !ok {
		return Request{}, fmt.Errorf("certificate request %s: %w", id, resource.ErrNotFound)
	}
	if !record.Scope.Equals(scope) {
		return Request{}, fmt.Errorf("certificate request %s: %w", id, resource.ErrScopeMismatch)
	}

	return record, nil
}

// GetByIdempotency returns one request by its scoped idempotency key.
func (s *Service) GetByIdempotency(scope resource.Scope, idempotencyKey string) (Request, error) {
	if err := scope.Validate(); err != nil {
		return Request{}, err
	}
	if strings.TrimSpace(idempotencyKey) == "" {
		return Request{}, fmt.Errorf("idempotency_key required: %w", resource.ErrValidation)
	}

	key := scope.EnvironmentKey() + "/" + strings.TrimSpace(idempotencyKey)

	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.byIdempotency[key]
	if !ok {
		return Request{}, fmt.Errorf("certificate request by idempotency key %q: %w", idempotencyKey, resource.ErrNotFound)
	}

	return s.byID[id], nil
}

// ListDomains returns the immutable domain set attached to one request.
func (s *Service) ListDomains(scope resource.Scope, requestID string) ([]DomainRef, error) {
	if _, err := s.Get(scope, requestID); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	items := append([]DomainRef(nil), s.domainsByReqID[requestID]...)
	slices.SortFunc(items, func(a DomainRef, b DomainRef) int {
		if a.SortOrder == b.SortOrder {
			return strings.Compare(a.DomainID, b.DomainID)
		}
		if a.SortOrder < b.SortOrder {
			return -1
		}
		return 1
	})
	return items, nil
}

// MarkAccepted transitions the request into accepted state once bootstrap work is queued.
func (s *Service) MarkAccepted(scope resource.Scope, id string) (Request, error) {
	return s.transition(scope, id, StatusAccepted)
}

// MarkRunning transitions the request into running state.
func (s *Service) MarkRunning(scope resource.Scope, id string) (Request, error) {
	return s.transition(scope, id, StatusRunning)
}

// MarkCompleted transitions the request into completed state.
func (s *Service) MarkCompleted(scope resource.Scope, id string) (Request, error) {
	return s.transition(scope, id, StatusCompleted)
}

// MarkFailed transitions the request into failed state.
func (s *Service) MarkFailed(scope resource.Scope, id string) (Request, error) {
	return s.transition(scope, id, StatusFailed)
}

// Cancel transitions the request into cancelled state when no irreversible work has happened yet.
func (s *Service) Cancel(scope resource.Scope, id string) (Request, error) {
	return s.transition(scope, id, StatusCancelled)
}

func (s *Service) transition(scope resource.Scope, id string, nextStatus string) (Request, error) {
	if err := scope.Validate(); err != nil {
		return Request{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.byID[id]
	if !ok {
		return Request{}, fmt.Errorf("certificate request %s: %w", id, resource.ErrNotFound)
	}
	if !record.Scope.Equals(scope) {
		return Request{}, fmt.Errorf("certificate request %s: %w", id, resource.ErrScopeMismatch)
	}
	if !canTransition(record.Status, nextStatus) {
		return Request{}, fmt.Errorf("request status %s -> %s: %w", record.Status, nextStatus, resource.ErrValidation)
	}

	record.Status = nextStatus
	record.UpdatedAt = s.now()
	s.byID[id] = record
	return record, nil
}

func canTransition(current string, next string) bool {
	switch current {
	case StatusSubmitted:
		return next == StatusAccepted || next == StatusCancelled || next == StatusFailed
	case StatusAccepted:
		return next == StatusRunning || next == StatusCancelled || next == StatusFailed
	case StatusRunning:
		return next == StatusCompleted || next == StatusFailed
	default:
		return false
	}
}

func normalizeCreateInput(input CreateInput) (CreateInput, []DomainRef, string, error) {
	requestType := strings.ToLower(strings.TrimSpace(input.RequestType))
	if requestType == "" {
		requestType = RequestTypeIssue
	}
	if requestType != RequestTypeIssue && requestType != RequestTypeRenew {
		return CreateInput{}, nil, "", fmt.Errorf("unsupported request_type %q: %w", input.RequestType, resource.ErrValidation)
	}

	requestSource := strings.ToLower(strings.TrimSpace(input.RequestSource))
	if requestSource == "" {
		requestSource = RequestSourceManual
	}
	if requestSource != RequestSourceManual && requestSource != RequestSourceScheduled {
		return CreateInput{}, nil, "", fmt.Errorf("unsupported request_source %q: %w", input.RequestSource, resource.ErrValidation)
	}

	certificateType := strings.ToLower(strings.TrimSpace(input.CertificateType))
	switch certificateType {
	case CertificateTypeSingle, CertificateTypeSAN, CertificateTypeWildcard:
	default:
		return CreateInput{}, nil, "", fmt.Errorf("unsupported certificate_type %q: %w", input.CertificateType, resource.ErrValidation)
	}

	challengeType := strings.ToLower(strings.TrimSpace(input.ChallengeType))
	switch challengeType {
	case ChallengeTypeHTTP01, ChallengeTypeDNS01:
	default:
		return CreateInput{}, nil, "", fmt.Errorf("unsupported challenge_type %q: %w", input.ChallengeType, resource.ErrValidation)
	}

	commonName := strings.ToLower(strings.TrimSpace(input.CommonName))
	if commonName == "" {
		return CreateInput{}, nil, "", fmt.Errorf("common_name required: %w", resource.ErrValidation)
	}

	caAccountID := strings.TrimSpace(input.CAAccountID)
	if caAccountID == "" {
		return CreateInput{}, nil, "", fmt.Errorf("ca_account_id required: %w", resource.ErrValidation)
	}

	idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
	if idempotencyKey == "" {
		return CreateInput{}, nil, "", fmt.Errorf("idempotency_key required: %w", resource.ErrValidation)
	}

	if requestType == RequestTypeRenew && strings.TrimSpace(input.AssetID) == "" {
		return CreateInput{}, nil, "", fmt.Errorf("asset_id required for renew request: %w", resource.ErrValidation)
	}

	refs := make([]DomainRef, 0, len(input.Domains))
	seen := make(map[string]struct{}, len(input.Domains))
	hasWildcard := false
	for idx, ref := range input.Domains {
		domainID := strings.TrimSpace(ref.DomainID)
		if domainID == "" {
			return CreateInput{}, nil, "", fmt.Errorf("domain_id required: %w", resource.ErrValidation)
		}
		if _, ok := seen[domainID]; ok {
			return CreateInput{}, nil, "", fmt.Errorf("duplicate domain_id %q: %w", domainID, resource.ErrConflict)
		}
		seen[domainID] = struct{}{}

		relationType := strings.ToLower(strings.TrimSpace(ref.RelationType))
		if relationType == "" {
			if idx == 0 {
				relationType = RelationPrimary
			} else {
				relationType = RelationSAN
			}
		}
		switch relationType {
		case RelationPrimary, RelationSAN, RelationWildcard:
		default:
			return CreateInput{}, nil, "", fmt.Errorf("unsupported relation_type %q: %w", ref.RelationType, resource.ErrValidation)
		}
		if relationType == RelationWildcard {
			hasWildcard = true
		}

		sortOrder := ref.SortOrder
		if sortOrder <= 0 {
			sortOrder = idx + 1
		}
		refs = append(refs, DomainRef{
			DomainID:      domainID,
			RelationType:  relationType,
			SortOrder:     sortOrder,
			DomainName:    strings.ToLower(strings.TrimSpace(ref.DomainName)),
			AllowWildcard: ref.AllowWildcard,
		})
	}

	if len(refs) == 0 {
		return CreateInput{}, nil, "", fmt.Errorf("at least one domain required: %w", resource.ErrValidation)
	}
	if certificateType == CertificateTypeSingle && len(refs) != 1 {
		return CreateInput{}, nil, "", fmt.Errorf("single certificate requires exactly one domain: %w", resource.ErrValidation)
	}
	if certificateType == CertificateTypeWildcard && !hasWildcard {
		return CreateInput{}, nil, "", fmt.Errorf("wildcard certificate requires wildcard domain: %w", resource.ErrValidation)
	}
	if hasWildcard && challengeType != ChallengeTypeDNS01 {
		return CreateInput{}, nil, "", fmt.Errorf("wildcard request requires dns-01 challenge: %w", resource.ErrValidation)
	}

	return CreateInput{
		RequestType:     requestType,
		RequestSource:   requestSource,
		AssetID:         strings.TrimSpace(input.AssetID),
		CAAccountID:     caAccountID,
		CertificateType: certificateType,
		ChallengeType:   challengeType,
		CommonName:      commonName,
		RequestedBy:     strings.TrimSpace(input.RequestedBy),
		IdempotencyKey:  idempotencyKey,
		Reason:          strings.TrimSpace(input.Reason),
	}, refs, idempotencyKey, nil
}

func timePointer(value time.Time) *time.Time {
	clone := value
	return &clone
}
