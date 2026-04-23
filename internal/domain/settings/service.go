package settings

import (
	"fmt"
	"sync"
	"time"

	"github.com/zaneway/AutoCertX/internal/domain/resource"
)

const (
	defaultDaysBeforeExpiry   = 30
	defaultScanIntervalMinute = 60
	defaultPasswordRotation   = 90
	defaultMaxSessionCount    = 5
)

// RenewalWindowSettings controls proactive renewal scanning behavior.
type RenewalWindowSettings struct {
	DaysBeforeExpiry   int       `json:"days_before_expiry"`
	ScanIntervalMinute int       `json:"scan_interval_minutes"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// RenewalWindowInput is the write model for renewal settings.
type RenewalWindowInput struct {
	DaysBeforeExpiry   int
	ScanIntervalMinute int
}

// SecuritySettings controls tenant-enforced security baselines.
type SecuritySettings struct {
	EnforceStrongPassword bool      `json:"enforce_strong_password"`
	PasswordRotationDays  int       `json:"password_rotation_days"`
	MaxSessionCount       int       `json:"max_session_count"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// SecuritySettingsInput is a partial update model.
type SecuritySettingsInput struct {
	EnforceStrongPassword *bool
	PasswordRotationDays  *int
	MaxSessionCount       *int
}

// Service owns in-memory settings state.
type Service struct {
	mu              sync.RWMutex
	now             func() time.Time
	renewalByScope  map[string]RenewalWindowSettings
	securityByScope map[string]SecuritySettings
}

// NewService constructs the settings domain service.
func NewService() *Service {
	return &Service{
		now: func() time.Time {
			return time.Now().UTC()
		},
		renewalByScope:  make(map[string]RenewalWindowSettings),
		securityByScope: make(map[string]SecuritySettings),
	}
}

// GetRenewalWindowSettings returns the scoped settings or a default snapshot.
func (s *Service) GetRenewalWindowSettings(scope resource.Scope) (RenewalWindowSettings, error) {
	if err := scope.Validate(); err != nil {
		return RenewalWindowSettings{}, err
	}

	s.mu.RLock()
	settings, ok := s.renewalByScope[scope.EnvironmentKey()]
	s.mu.RUnlock()
	if ok {
		return settings, nil
	}

	// Missing settings fall back to the GA baseline instead of forcing every
	// environment to materialize a record before use.
	defaults := RenewalWindowSettings{
		DaysBeforeExpiry:   defaultDaysBeforeExpiry,
		ScanIntervalMinute: defaultScanIntervalMinute,
	}
	return defaults, nil
}

// UpdateRenewalWindowSettings applies a full replacement for renewal settings.
func (s *Service) UpdateRenewalWindowSettings(scope resource.Scope, input RenewalWindowInput) (RenewalWindowSettings, error) {
	if err := scope.Validate(); err != nil {
		return RenewalWindowSettings{}, err
	}
	if err := validateRenewalInput(input); err != nil {
		return RenewalWindowSettings{}, err
	}

	updated := RenewalWindowSettings{
		DaysBeforeExpiry:   input.DaysBeforeExpiry,
		ScanIntervalMinute: input.ScanIntervalMinute,
		UpdatedAt:          s.now(),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// Renewal settings are stored as a full replacement because the HTTP contract
	// always submits the complete renewal window snapshot.
	s.renewalByScope[scope.EnvironmentKey()] = updated
	return updated, nil
}

// GetSecuritySettings returns the scoped security baseline or defaults.
func (s *Service) GetSecuritySettings(scope resource.Scope) (SecuritySettings, error) {
	if err := scope.Validate(); err != nil {
		return SecuritySettings{}, err
	}

	s.mu.RLock()
	settings, ok := s.securityByScope[scope.EnvironmentKey()]
	s.mu.RUnlock()
	if ok {
		return settings, nil
	}

	// Security settings default to a safe baseline even before any environment-
	// specific customization is persisted.
	return SecuritySettings{
		EnforceStrongPassword: true,
		PasswordRotationDays:  defaultPasswordRotation,
		MaxSessionCount:       defaultMaxSessionCount,
	}, nil
}

// UpdateSecuritySettings applies a partial update over the current baseline.
func (s *Service) UpdateSecuritySettings(scope resource.Scope, input SecuritySettingsInput) (SecuritySettings, error) {
	if err := scope.Validate(); err != nil {
		return SecuritySettings{}, err
	}
	if err := validateSecurityInput(input); err != nil {
		return SecuritySettings{}, err
	}

	current, err := s.GetSecuritySettings(scope)
	if err != nil {
		return SecuritySettings{}, err
	}

	if input.EnforceStrongPassword != nil {
		current.EnforceStrongPassword = *input.EnforceStrongPassword
	}
	if input.PasswordRotationDays != nil {
		current.PasswordRotationDays = *input.PasswordRotationDays
	}
	if input.MaxSessionCount != nil {
		current.MaxSessionCount = *input.MaxSessionCount
	}
	current.UpdatedAt = s.now()

	s.mu.Lock()
	defer s.mu.Unlock()
	// Security settings are patched over the current baseline so callers can send
	// sparse updates without accidentally resetting other controls.
	s.securityByScope[scope.EnvironmentKey()] = current
	return current, nil
}

func validateRenewalInput(input RenewalWindowInput) error {
	if input.DaysBeforeExpiry < 1 {
		return fmt.Errorf("days_before_expiry must be >= 1: %w", resource.ErrValidation)
	}
	if input.ScanIntervalMinute < 1 {
		return fmt.Errorf("scan_interval_minutes must be >= 1: %w", resource.ErrValidation)
	}
	if input.DaysBeforeExpiry > 365 {
		return fmt.Errorf("days_before_expiry must be <= 365: %w", resource.ErrValidation)
	}
	return nil
}

func validateSecurityInput(input SecuritySettingsInput) error {
	if input.PasswordRotationDays != nil && *input.PasswordRotationDays < 1 {
		return fmt.Errorf("password_rotation_days must be >= 1: %w", resource.ErrValidation)
	}
	if input.MaxSessionCount != nil && *input.MaxSessionCount < 1 {
		return fmt.Errorf("max_session_count must be >= 1: %w", resource.ErrValidation)
	}
	if input.EnforceStrongPassword == nil && input.PasswordRotationDays == nil && input.MaxSessionCount == nil {
		return fmt.Errorf("at least one security setting must be provided: %w", resource.ErrValidation)
	}
	return nil
}
