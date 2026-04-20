package dnscredentials

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/zaneway/AutoCertX/internal/domain/resource"
	"github.com/zaneway/AutoCertX/internal/platform/uuidx"
)

const (
	ProviderAliDNS = "alidns"

	ScopeEnvironment = "environment"
	ScopeDomain      = "domain"

	StatusActive   = "active"
	StatusDisabled = "disabled"
	StatusError    = "error"
	StatusRotating = "rotating"
)

// Credential represents one managed DNS credential.
type Credential struct {
	ID             string         `json:"id"`
	Scope          resource.Scope `json:"-"`
	ProviderType   string         `json:"provider_type"`
	DisplayName    string         `json:"display_name"`
	AccessKeyID    string         `json:"access_key_id"`
	SecretDigest   string         `json:"-"`
	ScopeMode      string         `json:"scope_mode"`
	Status         string         `json:"status"`
	LastVerifiedAt *time.Time     `json:"last_verified_at,omitempty"`
	LastRotatedAt  *time.Time     `json:"last_rotated_at,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// UpsertInput is the request model for create/update.
type UpsertInput struct {
	DisplayName  string
	ProviderType string
	AccessKeyID  string
	Secret       string
	ScopeMode    string
}

// Ref is the binding-safe credential projection used by other domains.
type Ref struct {
	ID           string
	Scope        resource.Scope
	ProviderType string
	ScopeMode    string
	Status       string
}

// Service manages DNS credential governance state.
type Service struct {
	mu        sync.RWMutex
	now       func() time.Time
	newID     func() string
	byID      map[string]Credential
	byEnvName map[string]string
}

// NewService constructs an in-memory DNS credential service.
func NewService() *Service {
	return &Service{
		now: func() time.Time {
			return time.Now().UTC()
		},
		newID:     uuidx.New,
		byID:      make(map[string]Credential),
		byEnvName: make(map[string]string),
	}
}

// List returns all credentials under the given scope.
func (s *Service) List(scope resource.Scope) ([]Credential, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]Credential, 0)
	for _, credential := range s.byID {
		if !credential.Scope.Equals(scope) {
			continue
		}
		items = append(items, credential)
	}

	slices.SortFunc(items, func(a Credential, b Credential) int {
		return strings.Compare(a.DisplayName, b.DisplayName)
	})

	return items, nil
}

// Get returns one credential scoped to the caller boundary.
func (s *Service) Get(scope resource.Scope, id string) (Credential, error) {
	if err := scope.Validate(); err != nil {
		return Credential{}, err
	}
	if strings.TrimSpace(id) == "" {
		return Credential{}, fmt.Errorf("credential id required: %w", resource.ErrValidation)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	credential, ok := s.byID[id]
	if !ok {
		return Credential{}, fmt.Errorf("credential %s: %w", id, resource.ErrNotFound)
	}
	if !credential.Scope.Equals(scope) {
		return Credential{}, fmt.Errorf("credential %s: %w", id, resource.ErrScopeMismatch)
	}

	return credential, nil
}

// LookupAvailable resolves a credential for cross-domain binding.
func (s *Service) LookupAvailable(scope resource.Scope, id string) (Ref, error) {
	credential, err := s.Get(scope, id)
	if err != nil {
		return Ref{}, err
	}
	if credential.Status != StatusActive {
		return Ref{}, fmt.Errorf("credential %s status %s: %w", id, credential.Status, resource.ErrUnavailable)
	}

	return Ref{
		ID:           credential.ID,
		Scope:        credential.Scope,
		ProviderType: credential.ProviderType,
		ScopeMode:    credential.ScopeMode,
		Status:       credential.Status,
	}, nil
}

// Create creates a credential under the caller scope.
func (s *Service) Create(scope resource.Scope, input UpsertInput) (Credential, error) {
	if err := scope.Validate(); err != nil {
		return Credential{}, err
	}
	if err := validateUpsert(input); err != nil {
		return Credential{}, err
	}

	now := s.now()
	credential := Credential{
		ID:           s.newID(),
		Scope:        scope,
		ProviderType: normalizeProvider(input.ProviderType),
		DisplayName:  strings.TrimSpace(input.DisplayName),
		AccessKeyID:  strings.TrimSpace(input.AccessKeyID),
		SecretDigest: digestSecret(input.Secret),
		ScopeMode:    normalizeScopeMode(input.ScopeMode),
		Status:       StatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	envKey := scope.EnvironmentKey() + "/" + credential.DisplayName

	s.mu.Lock()
	defer s.mu.Unlock()

	if existingID, ok := s.byEnvName[envKey]; ok {
		return Credential{}, fmt.Errorf("credential display_name already exists under environment (%s): %w", existingID, resource.ErrConflict)
	}

	s.byID[credential.ID] = credential
	s.byEnvName[envKey] = credential.ID
	return credential, nil
}

// Update updates the credential configuration and secret digest.
func (s *Service) Update(scope resource.Scope, id string, input UpsertInput) (Credential, error) {
	if err := scope.Validate(); err != nil {
		return Credential{}, err
	}
	if err := validateUpsert(input); err != nil {
		return Credential{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	credential, ok := s.byID[id]
	if !ok {
		return Credential{}, fmt.Errorf("credential %s: %w", id, resource.ErrNotFound)
	}
	if !credential.Scope.Equals(scope) {
		return Credential{}, fmt.Errorf("credential %s: %w", id, resource.ErrScopeMismatch)
	}

	newDisplayName := strings.TrimSpace(input.DisplayName)
	oldKey := credential.Scope.EnvironmentKey() + "/" + credential.DisplayName
	newKey := credential.Scope.EnvironmentKey() + "/" + newDisplayName
	if newKey != oldKey {
		if existingID, exists := s.byEnvName[newKey]; exists && existingID != credential.ID {
			return Credential{}, fmt.Errorf("credential display_name already exists under environment (%s): %w", existingID, resource.ErrConflict)
		}
		delete(s.byEnvName, oldKey)
		s.byEnvName[newKey] = credential.ID
	}

	credential.DisplayName = newDisplayName
	credential.ProviderType = normalizeProvider(input.ProviderType)
	credential.AccessKeyID = strings.TrimSpace(input.AccessKeyID)
	credential.SecretDigest = digestSecret(input.Secret)
	credential.ScopeMode = normalizeScopeMode(input.ScopeMode)
	credential.Status = StatusActive
	credential.UpdatedAt = s.now()

	s.byID[credential.ID] = credential
	return credential, nil
}

// Rotate marks the credential as rotated and returns the updated state.
func (s *Service) Rotate(scope resource.Scope, id string) (Credential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	credential, ok := s.byID[id]
	if !ok {
		return Credential{}, fmt.Errorf("credential %s: %w", id, resource.ErrNotFound)
	}
	if !credential.Scope.Equals(scope) {
		return Credential{}, fmt.Errorf("credential %s: %w", id, resource.ErrScopeMismatch)
	}

	now := s.now()
	credential.LastRotatedAt = &now
	credential.Status = StatusActive
	credential.UpdatedAt = now

	s.byID[credential.ID] = credential
	return credential, nil
}

func validateUpsert(input UpsertInput) error {
	if strings.TrimSpace(input.DisplayName) == "" {
		return fmt.Errorf("display_name required: %w", resource.ErrValidation)
	}
	if normalizeProvider(input.ProviderType) != ProviderAliDNS {
		return fmt.Errorf("unsupported provider_type %q: %w", input.ProviderType, resource.ErrValidation)
	}
	if strings.TrimSpace(input.AccessKeyID) == "" {
		return fmt.Errorf("access_key_id required: %w", resource.ErrValidation)
	}
	if strings.TrimSpace(input.Secret) == "" {
		return fmt.Errorf("secret required: %w", resource.ErrValidation)
	}
	scopeMode := normalizeScopeMode(input.ScopeMode)
	if scopeMode != ScopeEnvironment && scopeMode != ScopeDomain {
		return fmt.Errorf("unsupported scope_mode %q: %w", input.ScopeMode, resource.ErrValidation)
	}
	return nil
}

func normalizeProvider(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeScopeMode(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return ScopeEnvironment
	}
	return normalized
}

func digestSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
