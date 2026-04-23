package domains

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
	TypeSingle       = "single"
	TypeWildcardRoot = "wildcard_root"
	TypeSANMember    = "san_member"

	ChallengeHTTP01 = "http-01"
	ChallengeDNS01  = "dns-01"

	StatusActive   = "active"
	StatusDisabled = "disabled"
	StatusError    = "error"
)

// Asset is the long-lived governance record for one domain.
type Asset struct {
	ID                   string         `json:"id"`
	Scope                resource.Scope `json:"-"`
	DomainName           string         `json:"name"`
	DomainType           string         `json:"domain_type"`
	DefaultChallengeType string         `json:"challenge_type"`
	DefaultDNSProvider   string         `json:"dns_provider,omitempty"`
	DNSCredentialID      string         `json:"dns_credential_id,omitempty"`
	AllowWildcard        bool           `json:"allow_wildcard"`
	AutoRenew            bool           `json:"auto_renew"`
	Status               string         `json:"status"`
	LastValidationStatus string         `json:"last_validation_status,omitempty"`
	LastValidatedAt      *time.Time     `json:"last_validated_at,omitempty"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
}

// ValidationRecord preserves one domain validation attempt history entry.
type ValidationRecord struct {
	ID                  string         `json:"id"`
	Scope               resource.Scope `json:"-"`
	DomainAssetID       string         `json:"domain_id"`
	ValidationType      string         `json:"validation_type"`
	ProviderType        string         `json:"provider_type"`
	WorkflowChallengeID string         `json:"workflow_challenge_id,omitempty"`
	JobID               string         `json:"job_id,omitempty"`
	Status              string         `json:"status"`
	LatencyMS           *int           `json:"latency_ms,omitempty"`
	ErrorCode           string         `json:"error_code,omitempty"`
	ErrorMessage        string         `json:"error_message,omitempty"`
	ValidatedAt         *time.Time     `json:"validated_at,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

// TXTOperation preserves one TXT record mutation entry.
type TXTOperation struct {
	ID                 string         `json:"id"`
	Scope              resource.Scope `json:"-"`
	DomainAssetID      string         `json:"domain_id"`
	ValidationRecordID string         `json:"validation_record_id,omitempty"`
	ProviderType       string         `json:"provider_type"`
	OperationType      string         `json:"operation_type"`
	RecordName         string         `json:"record_name"`
	RecordType         string         `json:"record_type"`
	RecordValueDigest  string         `json:"record_value_digest"`
	TTL                int            `json:"ttl,omitempty"`
	Status             string         `json:"status"`
	ErrorCode          string         `json:"error_code,omitempty"`
	ErrorMessage       string         `json:"error_message,omitempty"`
	ExecutedAt         *time.Time     `json:"executed_at,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

// CertificateAssetRef is a read model placeholder for domain-related assets.
type CertificateAssetRef struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Status    string     `json:"status"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// UpsertInput is the request model for create/update.
type UpsertInput struct {
	Name            string
	ChallengeType   string
	AutoRenew       bool
	DNSCredentialID string
	DNSProvider     string
}

// Service manages domain governance state.
type Service struct {
	mu                 sync.RWMutex
	now                func() time.Time
	newID              func() string
	byID               map[string]Asset
	byEnvDomain        map[string]string
	validationByDomain map[string][]ValidationRecord
	txtOpsByDomain     map[string][]TXTOperation
	certsByDomain      map[string][]CertificateAssetRef
}

// NewService constructs an in-memory domain governance service.
func NewService() *Service {
	return &Service{
		now: func() time.Time {
			return time.Now().UTC()
		},
		newID:              uuidx.New,
		byID:               make(map[string]Asset),
		byEnvDomain:        make(map[string]string),
		validationByDomain: make(map[string][]ValidationRecord),
		txtOpsByDomain:     make(map[string][]TXTOperation),
		certsByDomain:      make(map[string][]CertificateAssetRef),
	}
}

// List returns all domain assets under the given scope.
func (s *Service) List(scope resource.Scope) ([]Asset, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]Asset, 0)
	for _, asset := range s.byID {
		if !asset.Scope.Equals(scope) {
			continue
		}
		items = append(items, asset)
	}

	slices.SortFunc(items, func(a Asset, b Asset) int {
		return strings.Compare(a.DomainName, b.DomainName)
	})

	return items, nil
}

// Get returns one domain asset under the given scope.
func (s *Service) Get(scope resource.Scope, id string) (Asset, error) {
	if err := scope.Validate(); err != nil {
		return Asset{}, err
	}
	if strings.TrimSpace(id) == "" {
		return Asset{}, fmt.Errorf("domain id required: %w", resource.ErrValidation)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	asset, ok := s.byID[id]
	if !ok {
		return Asset{}, fmt.Errorf("domain %s: %w", id, resource.ErrNotFound)
	}
	if !asset.Scope.Equals(scope) {
		return Asset{}, fmt.Errorf("domain %s: %w", id, resource.ErrScopeMismatch)
	}

	return asset, nil
}

// Create creates a new domain governance asset.
func (s *Service) Create(scope resource.Scope, input UpsertInput) (Asset, error) {
	if err := scope.Validate(); err != nil {
		return Asset{}, err
	}

	name, domainType, allowWildcard, err := validateInput(input)
	if err != nil {
		return Asset{}, err
	}

	now := s.now()
	asset := Asset{
		ID:                   s.newID(),
		Scope:                scope,
		DomainName:           name,
		DomainType:           domainType,
		DefaultChallengeType: strings.ToLower(strings.TrimSpace(input.ChallengeType)),
		DNSCredentialID:      strings.TrimSpace(input.DNSCredentialID),
		DefaultDNSProvider:   strings.ToLower(strings.TrimSpace(input.DNSProvider)),
		AllowWildcard:        allowWildcard,
		AutoRenew:            input.AutoRenew,
		Status:               StatusActive,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	envKey := scope.EnvironmentKey() + "/" + asset.DomainName

	s.mu.Lock()
	defer s.mu.Unlock()

	// Domain names are unique within one environment boundary to avoid ambiguous
	// governance targets for later certificate workflows.
	if existingID, exists := s.byEnvDomain[envKey]; exists {
		return Asset{}, fmt.Errorf("domain %q already exists under environment (%s): %w", asset.DomainName, existingID, resource.ErrConflict)
	}

	s.byID[asset.ID] = asset
	s.byEnvDomain[envKey] = asset.ID
	return asset, nil
}

// Update updates a domain governance asset.
func (s *Service) Update(scope resource.Scope, id string, input UpsertInput) (Asset, error) {
	if err := scope.Validate(); err != nil {
		return Asset{}, err
	}

	name, domainType, allowWildcard, err := validateInput(input)
	if err != nil {
		return Asset{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	asset, ok := s.byID[id]
	if !ok {
		return Asset{}, fmt.Errorf("domain %s: %w", id, resource.ErrNotFound)
	}
	if !asset.Scope.Equals(scope) {
		return Asset{}, fmt.Errorf("domain %s: %w", id, resource.ErrScopeMismatch)
	}

	oldKey := asset.Scope.EnvironmentKey() + "/" + asset.DomainName
	newKey := asset.Scope.EnvironmentKey() + "/" + name
	if newKey != oldKey {
		// Renames enforce the same uniqueness rule as create before mutating the
		// lookup index.
		if existingID, exists := s.byEnvDomain[newKey]; exists && existingID != asset.ID {
			return Asset{}, fmt.Errorf("domain %q already exists under environment (%s): %w", name, existingID, resource.ErrConflict)
		}
		delete(s.byEnvDomain, oldKey)
		s.byEnvDomain[newKey] = asset.ID
	}

	asset.DomainName = name
	asset.DomainType = domainType
	asset.DefaultChallengeType = strings.ToLower(strings.TrimSpace(input.ChallengeType))
	asset.DNSCredentialID = strings.TrimSpace(input.DNSCredentialID)
	asset.DefaultDNSProvider = strings.ToLower(strings.TrimSpace(input.DNSProvider))
	asset.AllowWildcard = allowWildcard
	asset.AutoRenew = input.AutoRenew
	asset.UpdatedAt = s.now()

	s.byID[asset.ID] = asset
	return asset, nil
}

// BindDNSCredential updates the default DNS credential for the domain.
func (s *Service) BindDNSCredential(scope resource.Scope, domainID string, credentialID string, provider string) (Asset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	asset, ok := s.byID[domainID]
	if !ok {
		return Asset{}, fmt.Errorf("domain %s: %w", domainID, resource.ErrNotFound)
	}
	if !asset.Scope.Equals(scope) {
		return Asset{}, fmt.Errorf("domain %s: %w", domainID, resource.ErrScopeMismatch)
	}
	if strings.TrimSpace(credentialID) == "" {
		return Asset{}, fmt.Errorf("dns_credential_id required: %w", resource.ErrValidation)
	}

	asset.DNSCredentialID = strings.TrimSpace(credentialID)
	asset.DefaultDNSProvider = strings.ToLower(strings.TrimSpace(provider))
	asset.UpdatedAt = s.now()
	s.byID[asset.ID] = asset
	return asset, nil
}

// ListValidationRecords returns validation history for one domain.
func (s *Service) ListValidationRecords(scope resource.Scope, domainID string) ([]ValidationRecord, error) {
	if _, err := s.Get(scope, domainID); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	items := append([]ValidationRecord(nil), s.validationByDomain[domainID]...)
	slices.SortFunc(items, func(a ValidationRecord, b ValidationRecord) int {
		if a.CreatedAt.Equal(b.CreatedAt) {
			return strings.Compare(a.ID, b.ID)
		}
		if a.CreatedAt.After(b.CreatedAt) {
			return -1
		}
		return 1
	})
	return items, nil
}

// ListTXTOperations returns TXT operation history for one domain.
func (s *Service) ListTXTOperations(scope resource.Scope, domainID string) ([]TXTOperation, error) {
	if _, err := s.Get(scope, domainID); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	items := append([]TXTOperation(nil), s.txtOpsByDomain[domainID]...)
	slices.SortFunc(items, func(a TXTOperation, b TXTOperation) int {
		if a.CreatedAt.Equal(b.CreatedAt) {
			return strings.Compare(a.ID, b.ID)
		}
		if a.CreatedAt.After(b.CreatedAt) {
			return -1
		}
		return 1
	})
	return items, nil
}

// ListCertificateAssets returns assets associated with the domain.
func (s *Service) ListCertificateAssets(scope resource.Scope, domainID string) ([]CertificateAssetRef, error) {
	if _, err := s.Get(scope, domainID); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return append([]CertificateAssetRef(nil), s.certsByDomain[domainID]...), nil
}

func validateInput(input UpsertInput) (string, string, bool, error) {
	name := normalizeDomain(input.Name)
	if name == "" {
		return "", "", false, fmt.Errorf("name required: %w", resource.ErrValidation)
	}

	challengeType := strings.ToLower(strings.TrimSpace(input.ChallengeType))
	if challengeType != ChallengeHTTP01 && challengeType != ChallengeDNS01 {
		return "", "", false, fmt.Errorf("unsupported challenge_type %q: %w", input.ChallengeType, resource.ErrValidation)
	}

	domainType := TypeSingle
	allowWildcard := false
	if strings.HasPrefix(name, "*.") {
		// Wildcard roots are a first-class domain type and must use DNS-01 because
		// HTTP-01 cannot validate wildcard ownership.
		domainType = TypeWildcardRoot
		allowWildcard = true
		if challengeType != ChallengeDNS01 {
			return "", "", false, fmt.Errorf("wildcard domain requires dns-01 challenge: %w", resource.ErrValidation)
		}
	}

	return name, domainType, allowWildcard, nil
}

func normalizeDomain(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	normalized = strings.TrimSuffix(normalized, ".")
	return normalized
}
