package domains

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/zaneway/AutoCertX/internal/domain/dnscredentials"
	domaindomain "github.com/zaneway/AutoCertX/internal/domain/domains"
	"github.com/zaneway/AutoCertX/internal/domain/resource"
	"github.com/zaneway/AutoCertX/internal/platform/apperr"
	"github.com/zaneway/AutoCertX/internal/platform/uuidx"
)

// AuditEvent captures one governance action that should be auditable.
type AuditEvent struct {
	Action       string            `json:"action"`
	ResourceType string            `json:"resource_type"`
	ResourceID   string            `json:"resource_id"`
	ActorID      string            `json:"actor_id,omitempty"`
	Scope        resource.Scope    `json:"scope"`
	OccurredAt   time.Time         `json:"occurred_at"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// AuditRecorder is the audit hook owned by T04 until T06 lands.
type AuditRecorder interface {
	Record(context.Context, AuditEvent)
}

// NopAuditRecorder discards audit events.
type NopAuditRecorder struct{}

// Record implements AuditRecorder.
func (NopAuditRecorder) Record(context.Context, AuditEvent) {}

// MemoryAuditRecorder stores audit events in memory for tests.
type MemoryAuditRecorder struct {
	mu     sync.Mutex
	events []AuditEvent
}

// Record implements AuditRecorder.
func (r *MemoryAuditRecorder) Record(_ context.Context, event AuditEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

// Events returns a snapshot of recorded events.
func (r *MemoryAuditRecorder) Events() []AuditEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]AuditEvent(nil), r.events...)
}

// DomainUpsertInput is the application-layer domain write model.
type DomainUpsertInput struct {
	Name            string
	ChallengeType   string
	AutoRenew       bool
	DNSCredentialID string
}

// DNSCredentialUpsertInput is the application-layer DNS credential write model.
type DNSCredentialUpsertInput struct {
	DisplayName  string
	ProviderType string
	AccessKeyID  string
	Secret       string
	ScopeMode    string
}

// AcceptedResult represents an accepted asynchronous governance action.
type AcceptedResult struct {
	Status string `json:"status"`
	JobID  string `json:"job_id"`
}

// Service orchestrates T04 domain and DNS governance commands.
type Service struct {
	domains *domaindomain.Service
	dns     *dnscredentials.Service
	audit   AuditRecorder
	now     func() time.Time
	newJob  func() string
}

// NewService constructs the governance command service.
func NewService(domainService *domaindomain.Service, dnsService *dnscredentials.Service, audit AuditRecorder) *Service {
	if audit == nil {
		audit = NopAuditRecorder{}
	}

	return &Service{
		domains: domainService,
		dns:     dnsService,
		audit:   audit,
		now: func() time.Time {
			return time.Now().UTC()
		},
		newJob: uuidx.New,
	}
}

// ListDomains returns domain assets within the caller scope.
func (s *Service) ListDomains(_ context.Context, scope resource.Scope) ([]domaindomain.Asset, error) {
	items, err := s.domains.List(scope)
	if err != nil {
		return nil, translateDomainError(err)
	}
	return items, nil
}

// CreateDomain creates a domain asset and optionally validates the DNS binding.
func (s *Service) CreateDomain(ctx context.Context, scope resource.Scope, actorID string, input DomainUpsertInput) (domaindomain.Asset, error) {
	domainInput, err := s.resolveDomainInput(scope, input)
	if err != nil {
		return domaindomain.Asset{}, err
	}

	asset, err := s.domains.Create(scope, domainInput)
	if err != nil {
		return domaindomain.Asset{}, translateDomainError(err)
	}

	s.recordAudit(ctx, AuditEvent{
		Action:       "domain.create",
		ResourceType: "domain_asset",
		ResourceID:   asset.ID,
		ActorID:      actorID,
		Scope:        scope,
		OccurredAt:   s.now(),
		Metadata: map[string]string{
			"name":           asset.DomainName,
			"challenge_type": asset.DefaultChallengeType,
		},
	})

	return asset, nil
}

// GetDomain returns one domain asset.
func (s *Service) GetDomain(_ context.Context, scope resource.Scope, id string) (domaindomain.Asset, error) {
	asset, err := s.domains.Get(scope, id)
	if err != nil {
		return domaindomain.Asset{}, translateDomainError(err)
	}
	return asset, nil
}

// UpdateDomain updates one domain asset.
func (s *Service) UpdateDomain(ctx context.Context, scope resource.Scope, actorID string, id string, input DomainUpsertInput) (domaindomain.Asset, error) {
	domainInput, err := s.resolveDomainInput(scope, input)
	if err != nil {
		return domaindomain.Asset{}, err
	}

	asset, err := s.domains.Update(scope, id, domainInput)
	if err != nil {
		return domaindomain.Asset{}, translateDomainError(err)
	}

	s.recordAudit(ctx, AuditEvent{
		Action:       "domain.update",
		ResourceType: "domain_asset",
		ResourceID:   asset.ID,
		ActorID:      actorID,
		Scope:        scope,
		OccurredAt:   s.now(),
		Metadata: map[string]string{
			"name":           asset.DomainName,
			"challenge_type": asset.DefaultChallengeType,
		},
	})

	return asset, nil
}

// BindDNSCredential binds a DNS credential to a domain asset.
func (s *Service) BindDNSCredential(ctx context.Context, scope resource.Scope, actorID string, domainID string, dnsCredentialID string) (AcceptedResult, error) {
	ref, err := s.dns.LookupAvailable(scope, strings.TrimSpace(dnsCredentialID))
	if err != nil {
		return AcceptedResult{}, translateDNSCredentialError(err)
	}

	asset, err := s.domains.BindDNSCredential(scope, domainID, ref.ID, ref.ProviderType)
	if err != nil {
		return AcceptedResult{}, translateDomainError(err)
	}

	jobID := s.newJob()
	s.recordAudit(ctx, AuditEvent{
		Action:       "domain.bind_dns_credential",
		ResourceType: "domain_asset",
		ResourceID:   asset.ID,
		ActorID:      actorID,
		Scope:        scope,
		OccurredAt:   s.now(),
		Metadata: map[string]string{
			"dns_credential_id": ref.ID,
			"provider_type":     ref.ProviderType,
			"job_id":            jobID,
		},
	})

	return AcceptedResult{
		Status: "accepted",
		JobID:  jobID,
	}, nil
}

// ListValidationRecords returns domain validation history.
func (s *Service) ListValidationRecords(_ context.Context, scope resource.Scope, domainID string) ([]domaindomain.ValidationRecord, error) {
	items, err := s.domains.ListValidationRecords(scope, domainID)
	if err != nil {
		return nil, translateDomainError(err)
	}
	return items, nil
}

// ListTXTOperations returns TXT operation history.
func (s *Service) ListTXTOperations(_ context.Context, scope resource.Scope, domainID string) ([]domaindomain.TXTOperation, error) {
	items, err := s.domains.ListTXTOperations(scope, domainID)
	if err != nil {
		return nil, translateDomainError(err)
	}
	return items, nil
}

// ListCertificateAssets returns assets associated with a domain.
func (s *Service) ListCertificateAssets(_ context.Context, scope resource.Scope, domainID string) ([]domaindomain.CertificateAssetRef, error) {
	items, err := s.domains.ListCertificateAssets(scope, domainID)
	if err != nil {
		return nil, translateDomainError(err)
	}
	return items, nil
}

// ListDNSCredentials returns DNS credentials within the caller scope.
func (s *Service) ListDNSCredentials(_ context.Context, scope resource.Scope) ([]dnscredentials.Credential, error) {
	items, err := s.dns.List(scope)
	if err != nil {
		return nil, translateDNSCredentialError(err)
	}
	return items, nil
}

// CreateDNSCredential creates a DNS credential.
func (s *Service) CreateDNSCredential(ctx context.Context, scope resource.Scope, actorID string, input DNSCredentialUpsertInput) (dnscredentials.Credential, error) {
	credential, err := s.dns.Create(scope, dnscredentials.UpsertInput(input))
	if err != nil {
		return dnscredentials.Credential{}, translateDNSCredentialError(err)
	}

	s.recordAudit(ctx, AuditEvent{
		Action:       "dns_credential.create",
		ResourceType: "dns_credential",
		ResourceID:   credential.ID,
		ActorID:      actorID,
		Scope:        scope,
		OccurredAt:   s.now(),
		Metadata: map[string]string{
			"display_name":  credential.DisplayName,
			"provider_type": credential.ProviderType,
			"scope_mode":    credential.ScopeMode,
		},
	})

	return credential, nil
}

// UpdateDNSCredential updates a DNS credential.
func (s *Service) UpdateDNSCredential(ctx context.Context, scope resource.Scope, actorID string, id string, input DNSCredentialUpsertInput) (dnscredentials.Credential, error) {
	credential, err := s.dns.Update(scope, id, dnscredentials.UpsertInput(input))
	if err != nil {
		return dnscredentials.Credential{}, translateDNSCredentialError(err)
	}

	s.recordAudit(ctx, AuditEvent{
		Action:       "dns_credential.update",
		ResourceType: "dns_credential",
		ResourceID:   credential.ID,
		ActorID:      actorID,
		Scope:        scope,
		OccurredAt:   s.now(),
		Metadata: map[string]string{
			"display_name":  credential.DisplayName,
			"provider_type": credential.ProviderType,
			"scope_mode":    credential.ScopeMode,
		},
	})

	return credential, nil
}

// RotateDNSCredential triggers credential rotation bookkeeping.
func (s *Service) RotateDNSCredential(ctx context.Context, scope resource.Scope, actorID string, id string) (AcceptedResult, error) {
	credential, err := s.dns.Rotate(scope, id)
	if err != nil {
		return AcceptedResult{}, translateDNSCredentialError(err)
	}

	jobID := s.newJob()
	s.recordAudit(ctx, AuditEvent{
		Action:       "dns_credential.rotate",
		ResourceType: "dns_credential",
		ResourceID:   credential.ID,
		ActorID:      actorID,
		Scope:        scope,
		OccurredAt:   s.now(),
		Metadata: map[string]string{
			"display_name": credential.DisplayName,
			"job_id":       jobID,
		},
	})

	return AcceptedResult{
		Status: "accepted",
		JobID:  jobID,
	}, nil
}

func (s *Service) resolveDomainInput(scope resource.Scope, input DomainUpsertInput) (domaindomain.UpsertInput, error) {
	domainInput := domaindomain.UpsertInput{
		Name:          input.Name,
		ChallengeType: input.ChallengeType,
		AutoRenew:     input.AutoRenew,
	}

	if strings.TrimSpace(input.DNSCredentialID) == "" {
		return domainInput, nil
	}

	ref, err := s.dns.LookupAvailable(scope, strings.TrimSpace(input.DNSCredentialID))
	if err != nil {
		return domaindomain.UpsertInput{}, translateDNSCredentialError(err)
	}

	domainInput.DNSCredentialID = ref.ID
	domainInput.DNSProvider = ref.ProviderType
	return domainInput, nil
}

func (s *Service) recordAudit(ctx context.Context, event AuditEvent) {
	s.audit.Record(ctx, event)
}

func translateDomainError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, resource.ErrNotFound):
		return apperr.Wrap(err, http.StatusNotFound, "RESOURCE_NOT_FOUND", "resource not found")
	case errors.Is(err, resource.ErrConflict):
		return apperr.Wrap(err, http.StatusConflict, "RESOURCE_CONFLICT", "resource conflict")
	case errors.Is(err, resource.ErrScopeMismatch):
		return apperr.Wrap(err, http.StatusConflict, "TENANT_SCOPE_MISMATCH", "scope mismatch")
	case errors.Is(err, resource.ErrValidation):
		return apperr.Wrap(err, http.StatusBadRequest, "REQUEST_VALIDATION_FAILED", "request validation failed", validationDetail(err))
	default:
		return apperr.Wrap(err, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}

func translateDNSCredentialError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, resource.ErrNotFound):
		return apperr.Wrap(err, http.StatusNotFound, "RESOURCE_NOT_FOUND", "resource not found")
	case errors.Is(err, resource.ErrConflict):
		return apperr.Wrap(err, http.StatusConflict, "RESOURCE_CONFLICT", "resource conflict")
	case errors.Is(err, resource.ErrScopeMismatch):
		return apperr.Wrap(err, http.StatusConflict, "TENANT_SCOPE_MISMATCH", "scope mismatch")
	case errors.Is(err, resource.ErrUnavailable):
		return apperr.Wrap(err, http.StatusConflict, "DNS_CREDENTIAL_UNAVAILABLE", "dns credential unavailable")
	case errors.Is(err, resource.ErrValidation):
		return apperr.Wrap(err, http.StatusBadRequest, "REQUEST_VALIDATION_FAILED", "request validation failed", validationDetail(err))
	default:
		return apperr.Wrap(err, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}

func validationDetail(err error) apperr.Detail {
	message := err.Error()
	field := ""
	switch {
	case strings.Contains(message, "display_name"):
		field = "display_name"
	case strings.Contains(message, "provider_type"):
		field = "provider_type"
	case strings.Contains(message, "access_key_id"):
		field = "access_key_id"
	case strings.Contains(message, "secret"):
		field = "secret"
	case strings.Contains(message, "scope_mode"):
		field = "scope_mode"
	case strings.Contains(message, "directory_url"):
		field = "directory_url"
	case strings.Contains(message, "email"):
		field = "email"
	case strings.Contains(message, "challenge_type"):
		field = "challenge_type"
	case strings.Contains(message, "name"):
		field = "name"
	case strings.Contains(message, "dns_credential_id"):
		field = "dns_credential_id"
	}
	return apperr.Field(field, message)
}
