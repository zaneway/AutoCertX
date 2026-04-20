package issuer

import (
	"fmt"
	"net/mail"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/zaneway/AutoCertX/internal/domain/resource"
	"github.com/zaneway/AutoCertX/internal/platform/uuidx"
)

const (
	ProviderTypeACME = "acme"
	ProviderLE       = "letsencrypt"

	StatusActive    = "active"
	StatusDisabled  = "disabled"
	StatusError     = "error"
	StatusVerifying = "verifying"
)

// CapabilitySet describes the CA capability metadata returned to clients.
type CapabilitySet struct {
	ProviderType               string   `json:"provider_type"`
	ProviderName               string   `json:"provider_name"`
	DirectoryURL               string   `json:"directory_url"`
	Environment                string   `json:"environment"`
	SupportedCertificateTypes  []string `json:"supported_certificate_types"`
	SupportedChallengeTypes    []string `json:"supported_challenge_types"`
	WildcardChallengeTypes     []string `json:"wildcard_challenge_types"`
	SupportsExternalAccountKey bool     `json:"supports_external_account_binding"`
}

// Account represents one managed CA account.
type Account struct {
	ID                  string         `json:"id"`
	Scope               resource.Scope `json:"-"`
	ProviderType        string         `json:"provider_type"`
	ProviderName        string         `json:"provider_name"`
	DisplayName         string         `json:"display_name"`
	DirectoryURL        string         `json:"directory_url"`
	Email               string         `json:"email"`
	AccountKID          string         `json:"account_kid,omitempty"`
	AccountKeySecretRef string         `json:"account_key_secret_ref"`
	Status              string         `json:"status"`
	Capabilities        CapabilitySet  `json:"capabilities"`
	LastCheckedAt       *time.Time     `json:"last_checked_at,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

// UpsertInput is the request model for account creation.
type UpsertInput struct {
	DisplayName  string
	DirectoryURL string
	Email        string
}

// Service manages CA account governance.
type Service struct {
	mu        sync.RWMutex
	now       func() time.Time
	newID     func() string
	byID      map[string]Account
	byEnvName map[string]string
}

// NewService constructs an in-memory CA account service.
func NewService() *Service {
	return &Service{
		now: func() time.Time {
			return time.Now().UTC()
		},
		newID:     uuidx.New,
		byID:      make(map[string]Account),
		byEnvName: make(map[string]string),
	}
}

// List returns all accounts under the given scope.
func (s *Service) List(scope resource.Scope) ([]Account, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]Account, 0)
	for _, account := range s.byID {
		if !account.Scope.Equals(scope) {
			continue
		}
		items = append(items, account)
	}

	slices.SortFunc(items, func(a Account, b Account) int {
		return strings.Compare(a.DisplayName, b.DisplayName)
	})
	return items, nil
}

// Get returns a CA account scoped to the caller boundary.
func (s *Service) Get(scope resource.Scope, id string) (Account, error) {
	if err := scope.Validate(); err != nil {
		return Account{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	account, ok := s.byID[id]
	if !ok {
		return Account{}, fmt.Errorf("ca account %s: %w", id, resource.ErrNotFound)
	}
	if !account.Scope.Equals(scope) {
		return Account{}, fmt.Errorf("ca account %s: %w", id, resource.ErrScopeMismatch)
	}

	return account, nil
}

// GetCapabilities returns the capability metadata for one account.
func (s *Service) GetCapabilities(scope resource.Scope, id string) (CapabilitySet, error) {
	account, err := s.Get(scope, id)
	if err != nil {
		return CapabilitySet{}, err
	}
	return account.Capabilities, nil
}

// Create creates a new CA account.
func (s *Service) Create(scope resource.Scope, input UpsertInput) (Account, error) {
	if err := scope.Validate(); err != nil {
		return Account{}, err
	}
	if err := validateInput(input); err != nil {
		return Account{}, err
	}

	now := s.now()
	capabilities := buildCapabilities(input.DirectoryURL)
	account := Account{
		ID:                  s.newID(),
		Scope:               scope,
		ProviderType:        ProviderTypeACME,
		ProviderName:        ProviderLE,
		DisplayName:         strings.TrimSpace(input.DisplayName),
		DirectoryURL:        strings.TrimSpace(input.DirectoryURL),
		Email:               strings.TrimSpace(input.Email),
		AccountKeySecretRef: "secret://ca-accounts/" + uuidx.New(),
		Status:              StatusActive,
		Capabilities:        capabilities,
		LastCheckedAt:       &now,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	envKey := scope.EnvironmentKey() + "/" + account.DisplayName

	s.mu.Lock()
	defer s.mu.Unlock()

	if existingID, exists := s.byEnvName[envKey]; exists {
		return Account{}, fmt.Errorf("ca account display_name already exists under environment (%s): %w", existingID, resource.ErrConflict)
	}

	s.byID[account.ID] = account
	s.byEnvName[envKey] = account.ID
	return account, nil
}

func validateInput(input UpsertInput) error {
	if strings.TrimSpace(input.DisplayName) == "" {
		return fmt.Errorf("display_name required: %w", resource.ErrValidation)
	}
	parsedURL, err := url.ParseRequestURI(strings.TrimSpace(input.DirectoryURL))
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return fmt.Errorf("directory_url invalid: %w", resource.ErrValidation)
	}
	if _, err := mail.ParseAddress(strings.TrimSpace(input.Email)); err != nil {
		return fmt.Errorf("email invalid: %w", resource.ErrValidation)
	}
	return nil
}

func buildCapabilities(directoryURL string) CapabilitySet {
	environment := "production"
	if strings.Contains(strings.ToLower(directoryURL), "staging") {
		environment = "staging"
	}

	return CapabilitySet{
		ProviderType:               ProviderTypeACME,
		ProviderName:               ProviderLE,
		DirectoryURL:               strings.TrimSpace(directoryURL),
		Environment:                environment,
		SupportedCertificateTypes:  []string{"single", "san", "wildcard"},
		SupportedChallengeTypes:    []string{"http-01", "dns-01"},
		WildcardChallengeTypes:     []string{"dns-01"},
		SupportsExternalAccountKey: false,
	}
}
