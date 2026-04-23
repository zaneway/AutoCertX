package deploymenttarget

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
	TypeNGINX                = "nginx"
	TypeTomcatJSSEPKCS12     = "tomcat-jsse-pkcs12"
	StatusActive             = "active"
	StatusDisabled           = "disabled"
	StatusConfigurationError = "configuration_error"
)

// Target represents one deployable runtime endpoint.
type Target struct {
	ID              string            `json:"id"`
	Scope           resource.Scope    `json:"-"`
	Name            string            `json:"name"`
	TargetType      string            `json:"target_type"`
	AgentID         string            `json:"agent_id,omitempty"`
	AgentSelector   map[string]string `json:"agent_selector"`
	ConfigPath      string            `json:"config_path,omitempty"`
	CertificatePath string            `json:"certificate_path,omitempty"`
	PrivateKeyPath  string            `json:"private_key_path,omitempty"`
	KeystorePath    string            `json:"keystore_path,omitempty"`
	Status          string            `json:"status"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// UpsertInput is the request model for create/update.
type UpsertInput struct {
	Name            string
	TargetType      string
	AgentID         string
	AgentSelector   map[string]string
	ConfigPath      string
	CertificatePath string
	PrivateKeyPath  string
	KeystorePath    string
}

// Service manages deployment target governance state.
type Service struct {
	mu        sync.RWMutex
	now       func() time.Time
	newID     func() string
	byID      map[string]Target
	byEnvName map[string]string
}

// NewService constructs an in-memory deployment target service.
func NewService() *Service {
	return &Service{
		now: func() time.Time {
			return time.Now().UTC()
		},
		newID:     uuidx.New,
		byID:      make(map[string]Target),
		byEnvName: make(map[string]string),
	}
}

// List returns all deployment targets under the given scope.
func (s *Service) List(scope resource.Scope) ([]Target, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]Target, 0)
	for _, target := range s.byID {
		if !target.Scope.Equals(scope) {
			continue
		}
		items = append(items, cloneTarget(target))
	}

	slices.SortFunc(items, func(a Target, b Target) int {
		return strings.Compare(a.Name, b.Name)
	})

	return items, nil
}

// Get returns one deployment target under the caller scope.
func (s *Service) Get(scope resource.Scope, id string) (Target, error) {
	if err := scope.Validate(); err != nil {
		return Target{}, err
	}
	if strings.TrimSpace(id) == "" {
		return Target{}, fmt.Errorf("deployment target id required: %w", resource.ErrValidation)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	target, ok := s.byID[id]
	if !ok {
		return Target{}, fmt.Errorf("deployment target %s: %w", id, resource.ErrNotFound)
	}
	if !target.Scope.Equals(scope) {
		return Target{}, fmt.Errorf("deployment target %s: %w", id, resource.ErrScopeMismatch)
	}

	return cloneTarget(target), nil
}

// Create creates a deployment target under the caller scope.
func (s *Service) Create(scope resource.Scope, input UpsertInput) (Target, error) {
	if err := scope.Validate(); err != nil {
		return Target{}, err
	}
	if err := validateUpsert(input); err != nil {
		return Target{}, err
	}

	now := s.now()
	target := Target{
		ID:              s.newID(),
		Scope:           scope,
		Name:            strings.TrimSpace(input.Name),
		TargetType:      normalizeTargetType(input.TargetType),
		AgentID:         strings.TrimSpace(input.AgentID),
		AgentSelector:   cloneStringMap(input.AgentSelector),
		ConfigPath:      strings.TrimSpace(input.ConfigPath),
		CertificatePath: strings.TrimSpace(input.CertificatePath),
		PrivateKeyPath:  strings.TrimSpace(input.PrivateKeyPath),
		KeystorePath:    strings.TrimSpace(input.KeystorePath),
		Status:          StatusActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	envKey := scope.EnvironmentKey() + "/" + target.Name

	s.mu.Lock()
	defer s.mu.Unlock()

	if existingID, exists := s.byEnvName[envKey]; exists {
		return Target{}, fmt.Errorf("deployment target %q already exists under environment (%s): %w", target.Name, existingID, resource.ErrConflict)
	}

	s.byID[target.ID] = target
	s.byEnvName[envKey] = target.ID
	return cloneTarget(target), nil
}

// Update updates an existing deployment target.
func (s *Service) Update(scope resource.Scope, id string, input UpsertInput) (Target, error) {
	if err := scope.Validate(); err != nil {
		return Target{}, err
	}
	if err := validateUpsert(input); err != nil {
		return Target{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	target, ok := s.byID[id]
	if !ok {
		return Target{}, fmt.Errorf("deployment target %s: %w", id, resource.ErrNotFound)
	}
	if !target.Scope.Equals(scope) {
		return Target{}, fmt.Errorf("deployment target %s: %w", id, resource.ErrScopeMismatch)
	}

	newName := strings.TrimSpace(input.Name)
	oldKey := target.Scope.EnvironmentKey() + "/" + target.Name
	newKey := target.Scope.EnvironmentKey() + "/" + newName
	if newKey != oldKey {
		if existingID, exists := s.byEnvName[newKey]; exists && existingID != target.ID {
			return Target{}, fmt.Errorf("deployment target %q already exists under environment (%s): %w", newName, existingID, resource.ErrConflict)
		}
		delete(s.byEnvName, oldKey)
		s.byEnvName[newKey] = target.ID
	}

	target.Name = newName
	target.TargetType = normalizeTargetType(input.TargetType)
	target.AgentID = strings.TrimSpace(input.AgentID)
	target.AgentSelector = cloneStringMap(input.AgentSelector)
	target.ConfigPath = strings.TrimSpace(input.ConfigPath)
	target.CertificatePath = strings.TrimSpace(input.CertificatePath)
	target.PrivateKeyPath = strings.TrimSpace(input.PrivateKeyPath)
	target.KeystorePath = strings.TrimSpace(input.KeystorePath)
	target.Status = StatusActive
	target.UpdatedAt = s.now()

	s.byID[target.ID] = target
	return cloneTarget(target), nil
}

// Disable marks a deployment target unavailable for new deployment work.
func (s *Service) Disable(scope resource.Scope, id string) (Target, error) {
	if err := scope.Validate(); err != nil {
		return Target{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	target, ok := s.byID[id]
	if !ok {
		return Target{}, fmt.Errorf("deployment target %s: %w", id, resource.ErrNotFound)
	}
	if !target.Scope.Equals(scope) {
		return Target{}, fmt.Errorf("deployment target %s: %w", id, resource.ErrScopeMismatch)
	}

	target.Status = StatusDisabled
	target.UpdatedAt = s.now()

	s.byID[target.ID] = target
	return cloneTarget(target), nil
}

func validateUpsert(input UpsertInput) error {
	if strings.TrimSpace(input.Name) == "" {
		return fmt.Errorf("name required: %w", resource.ErrValidation)
	}
	targetType := normalizeTargetType(input.TargetType)
	if targetType != TypeNGINX && targetType != TypeTomcatJSSEPKCS12 {
		return fmt.Errorf("target_type invalid: %w", resource.ErrValidation)
	}
	if strings.TrimSpace(input.AgentID) == "" && len(input.AgentSelector) == 0 {
		return fmt.Errorf("agent binding required: %w", resource.ErrValidation)
	}
	switch targetType {
	case TypeNGINX:
		if strings.TrimSpace(input.ConfigPath) == "" {
			return fmt.Errorf("config_path required for nginx: %w", resource.ErrValidation)
		}
		if strings.TrimSpace(input.CertificatePath) == "" {
			return fmt.Errorf("certificate_path required for nginx: %w", resource.ErrValidation)
		}
		if strings.TrimSpace(input.PrivateKeyPath) == "" {
			return fmt.Errorf("private_key_path required for nginx: %w", resource.ErrValidation)
		}
	case TypeTomcatJSSEPKCS12:
		if strings.TrimSpace(input.ConfigPath) == "" {
			return fmt.Errorf("config_path required for tomcat: %w", resource.ErrValidation)
		}
		if strings.TrimSpace(input.KeystorePath) == "" {
			return fmt.Errorf("keystore_path required for tomcat: %w", resource.ErrValidation)
		}
	}
	return nil
}

func normalizeTargetType(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func cloneTarget(target Target) Target {
	target.AgentSelector = cloneStringMap(target.AgentSelector)
	return target
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		normalizedKey := strings.TrimSpace(key)
		if normalizedKey == "" {
			continue
		}
		clone[normalizedKey] = strings.TrimSpace(value)
	}
	return clone
}
