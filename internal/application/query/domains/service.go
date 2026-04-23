package domains

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/zaneway/AutoCertX/internal/domain/dnscredentials"
	domaindomain "github.com/zaneway/AutoCertX/internal/domain/domains"
	"github.com/zaneway/AutoCertX/internal/domain/issuer"
	"github.com/zaneway/AutoCertX/internal/domain/resource"
	"github.com/zaneway/AutoCertX/internal/platform/apperr"
)

// DomainItem is the governance list/detail view for one domain asset.
type DomainItem struct {
	ID                   string     `json:"id"`
	Name                 string     `json:"name"`
	DomainType           string     `json:"domain_type"`
	ChallengeType        string     `json:"challenge_type"`
	DNSProvider          string     `json:"dns_provider,omitempty"`
	DNSCredentialID      string     `json:"dns_credential_id,omitempty"`
	AllowWildcard        bool       `json:"allow_wildcard"`
	AutoRenew            bool       `json:"auto_renew"`
	Status               string     `json:"status"`
	LastValidationStatus string     `json:"last_validation_status,omitempty"`
	LastValidatedAt      *time.Time `json:"last_validated_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// ValidationRecordItem is the query payload for one domain validation history entry.
type ValidationRecordItem struct {
	ID                  string     `json:"id"`
	DomainID            string     `json:"domain_id"`
	ValidationType      string     `json:"validation_type"`
	ProviderType        string     `json:"provider_type"`
	WorkflowChallengeID string     `json:"workflow_challenge_id,omitempty"`
	JobID               string     `json:"job_id,omitempty"`
	Status              string     `json:"status"`
	LatencyMS           *int       `json:"latency_ms,omitempty"`
	ErrorCode           string     `json:"error_code,omitempty"`
	ErrorMessage        string     `json:"error_message,omitempty"`
	ValidatedAt         *time.Time `json:"validated_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// TXTOperationItem is the query payload for one TXT mutation history entry.
type TXTOperationItem struct {
	ID                 string     `json:"id"`
	DomainID           string     `json:"domain_id"`
	ValidationRecordID string     `json:"validation_record_id,omitempty"`
	ProviderType       string     `json:"provider_type"`
	OperationType      string     `json:"operation_type"`
	RecordName         string     `json:"record_name"`
	RecordType         string     `json:"record_type"`
	RecordValueDigest  string     `json:"record_value_digest"`
	TTL                int        `json:"ttl,omitempty"`
	Status             string     `json:"status"`
	ErrorCode          string     `json:"error_code,omitempty"`
	ErrorMessage       string     `json:"error_message,omitempty"`
	ExecutedAt         *time.Time `json:"executed_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// CertificateAssetRefItem is the scoped certificate reference attached to one domain.
type CertificateAssetRefItem struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Status    string     `json:"status"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// DNSCredentialItem is the read-model row for one DNS credential.
type DNSCredentialItem struct {
	ID            string     `json:"id"`
	DisplayName   string     `json:"display_name"`
	ProviderType  string     `json:"provider_type"`
	AccessKeyID   string     `json:"access_key_id"`
	ScopeMode     string     `json:"scope_mode"`
	Status        string     `json:"status"`
	LastRotatedAt *time.Time `json:"last_rotated_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// CAAccountItem is the governance list/detail view for one CA account.
type CAAccountItem struct {
	ID                  string               `json:"id"`
	ProviderType        string               `json:"provider_type"`
	ProviderName        string               `json:"provider_name"`
	DisplayName         string               `json:"display_name"`
	DirectoryURL        string               `json:"directory_url"`
	Email               string               `json:"email"`
	AccountKID          string               `json:"account_kid,omitempty"`
	AccountKeySecretRef string               `json:"account_key_secret_ref"`
	Status              string               `json:"status"`
	Capabilities        issuer.CapabilitySet `json:"capabilities"`
	LastCheckedAt       *time.Time           `json:"last_checked_at,omitempty"`
	CreatedAt           time.Time            `json:"created_at"`
	UpdatedAt           time.Time            `json:"updated_at"`
}

// Service maps existing governance facts into stable read DTOs.
type Service struct {
	domains *domaindomain.Service
	dns     *dnscredentials.Service
	issuer  *issuer.Service
}

// NewService constructs the governance query service.
func NewService(
	domainService *domaindomain.Service,
	dnsService *dnscredentials.Service,
	issuerService *issuer.Service,
) (*Service, error) {
	switch {
	case domainService == nil:
		return nil, fmt.Errorf("domains query domain service required")
	case dnsService == nil:
		return nil, fmt.Errorf("domains query dns service required")
	case issuerService == nil:
		return nil, fmt.Errorf("domains query issuer service required")
	default:
		return &Service{
			domains: domainService,
			dns:     dnsService,
			issuer:  issuerService,
		}, nil
	}
}

// ListDomains returns the scoped domain inventory.
func (s *Service) ListDomains(_ context.Context, scope resource.Scope) ([]DomainItem, error) {
	items, err := s.domains.List(scope)
	if err != nil {
		return nil, translateError(err)
	}
	result := make([]DomainItem, 0, len(items))
	for _, item := range items {
		result = append(result, mapDomainItem(item))
	}
	return result, nil
}

// GetDomain returns the scoped domain detail.
func (s *Service) GetDomain(_ context.Context, scope resource.Scope, id string) (DomainItem, error) {
	item, err := s.domains.Get(scope, strings.TrimSpace(id))
	if err != nil {
		return DomainItem{}, translateError(err)
	}
	return mapDomainItem(item), nil
}

// ListValidationRecords returns validation history for one visible domain.
func (s *Service) ListValidationRecords(_ context.Context, scope resource.Scope, domainID string) ([]ValidationRecordItem, error) {
	items, err := s.domains.ListValidationRecords(scope, strings.TrimSpace(domainID))
	if err != nil {
		return nil, translateError(err)
	}
	result := make([]ValidationRecordItem, 0, len(items))
	for _, item := range items {
		result = append(result, mapValidationRecord(item))
	}
	return result, nil
}

// ListTXTOperations returns TXT mutation history for one visible domain.
func (s *Service) ListTXTOperations(_ context.Context, scope resource.Scope, domainID string) ([]TXTOperationItem, error) {
	items, err := s.domains.ListTXTOperations(scope, strings.TrimSpace(domainID))
	if err != nil {
		return nil, translateError(err)
	}
	result := make([]TXTOperationItem, 0, len(items))
	for _, item := range items {
		result = append(result, mapTXTOperation(item))
	}
	return result, nil
}

// ListCertificateAssets returns scoped certificate references attached to one domain.
func (s *Service) ListCertificateAssets(_ context.Context, scope resource.Scope, domainID string) ([]CertificateAssetRefItem, error) {
	items, err := s.domains.ListCertificateAssets(scope, strings.TrimSpace(domainID))
	if err != nil {
		return nil, translateError(err)
	}
	result := make([]CertificateAssetRefItem, 0, len(items))
	for _, item := range items {
		result = append(result, mapCertificateAssetRef(item))
	}
	return result, nil
}

// ListDNSCredentials returns scoped DNS credentials.
func (s *Service) ListDNSCredentials(_ context.Context, scope resource.Scope) ([]DNSCredentialItem, error) {
	items, err := s.dns.List(scope)
	if err != nil {
		return nil, translateError(err)
	}
	result := make([]DNSCredentialItem, 0, len(items))
	for _, item := range items {
		result = append(result, mapDNSCredential(item))
	}
	return result, nil
}

// ListCAAccounts returns scoped CA accounts.
func (s *Service) ListCAAccounts(_ context.Context, scope resource.Scope) ([]CAAccountItem, error) {
	items, err := s.issuer.List(scope)
	if err != nil {
		return nil, translateError(err)
	}
	result := make([]CAAccountItem, 0, len(items))
	for _, item := range items {
		result = append(result, mapCAAccount(item))
	}
	return result, nil
}

// GetCAAccount returns one scoped CA account detail.
func (s *Service) GetCAAccount(_ context.Context, scope resource.Scope, id string) (CAAccountItem, error) {
	item, err := s.issuer.Get(scope, strings.TrimSpace(id))
	if err != nil {
		return CAAccountItem{}, translateError(err)
	}
	return mapCAAccount(item), nil
}

// GetCAAccountCapabilities returns the effective capability set for one CA account.
func (s *Service) GetCAAccountCapabilities(_ context.Context, scope resource.Scope, id string) (issuer.CapabilitySet, error) {
	item, err := s.issuer.GetCapabilities(scope, strings.TrimSpace(id))
	if err != nil {
		return issuer.CapabilitySet{}, translateError(err)
	}
	return item, nil
}

func mapDomainItem(item domaindomain.Asset) DomainItem {
	return DomainItem{
		ID:                   item.ID,
		Name:                 item.DomainName,
		DomainType:           item.DomainType,
		ChallengeType:        item.DefaultChallengeType,
		DNSProvider:          item.DefaultDNSProvider,
		DNSCredentialID:      item.DNSCredentialID,
		AllowWildcard:        item.AllowWildcard,
		AutoRenew:            item.AutoRenew,
		Status:               item.Status,
		LastValidationStatus: item.LastValidationStatus,
		LastValidatedAt:      timePointer(item.LastValidatedAt),
		CreatedAt:            item.CreatedAt,
		UpdatedAt:            item.UpdatedAt,
	}
}

func mapValidationRecord(item domaindomain.ValidationRecord) ValidationRecordItem {
	return ValidationRecordItem{
		ID:                  item.ID,
		DomainID:            item.DomainAssetID,
		ValidationType:      item.ValidationType,
		ProviderType:        item.ProviderType,
		WorkflowChallengeID: item.WorkflowChallengeID,
		JobID:               item.JobID,
		Status:              item.Status,
		LatencyMS:           intPointer(item.LatencyMS),
		ErrorCode:           item.ErrorCode,
		ErrorMessage:        item.ErrorMessage,
		ValidatedAt:         timePointer(item.ValidatedAt),
		CreatedAt:           item.CreatedAt,
		UpdatedAt:           item.UpdatedAt,
	}
}

func mapTXTOperation(item domaindomain.TXTOperation) TXTOperationItem {
	return TXTOperationItem{
		ID:                 item.ID,
		DomainID:           item.DomainAssetID,
		ValidationRecordID: item.ValidationRecordID,
		ProviderType:       item.ProviderType,
		OperationType:      item.OperationType,
		RecordName:         item.RecordName,
		RecordType:         item.RecordType,
		RecordValueDigest:  item.RecordValueDigest,
		TTL:                item.TTL,
		Status:             item.Status,
		ErrorCode:          item.ErrorCode,
		ErrorMessage:       item.ErrorMessage,
		ExecutedAt:         timePointer(item.ExecutedAt),
		CreatedAt:          item.CreatedAt,
		UpdatedAt:          item.UpdatedAt,
	}
}

func mapCertificateAssetRef(item domaindomain.CertificateAssetRef) CertificateAssetRefItem {
	return CertificateAssetRefItem{
		ID:        item.ID,
		Name:      item.Name,
		Status:    item.Status,
		ExpiresAt: timePointer(item.ExpiresAt),
	}
}

func mapDNSCredential(item dnscredentials.Credential) DNSCredentialItem {
	return DNSCredentialItem{
		ID:            item.ID,
		DisplayName:   item.DisplayName,
		ProviderType:  item.ProviderType,
		AccessKeyID:   item.AccessKeyID,
		ScopeMode:     item.ScopeMode,
		Status:        item.Status,
		LastRotatedAt: timePointer(item.LastRotatedAt),
		CreatedAt:     item.CreatedAt,
		UpdatedAt:     item.UpdatedAt,
	}
}

func mapCAAccount(item issuer.Account) CAAccountItem {
	return CAAccountItem{
		ID:                  item.ID,
		ProviderType:        item.ProviderType,
		ProviderName:        item.ProviderName,
		DisplayName:         item.DisplayName,
		DirectoryURL:        item.DirectoryURL,
		Email:               item.Email,
		AccountKID:          item.AccountKID,
		AccountKeySecretRef: item.AccountKeySecretRef,
		Status:              item.Status,
		Capabilities:        item.Capabilities,
		LastCheckedAt:       timePointer(item.LastCheckedAt),
		CreatedAt:           item.CreatedAt,
		UpdatedAt:           item.UpdatedAt,
	}
}

func timePointer(value *time.Time) *time.Time {
	if value == nil || value.IsZero() {
		return nil
	}
	cloned := *value
	return &cloned
}

func intPointer(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func translateError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, resource.ErrNotFound):
		return apperr.Wrap(err, http.StatusNotFound, "RESOURCE_NOT_FOUND", "resource not found")
	case errors.Is(err, resource.ErrConflict), errors.Is(err, resource.ErrScopeMismatch):
		return apperr.Wrap(err, http.StatusConflict, "TENANT_SCOPE_MISMATCH", "scope mismatch")
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
	case strings.Contains(message, "domain"):
		field = "domain_id"
	case strings.Contains(message, "display_name"):
		field = "display_name"
	case strings.Contains(message, "directory_url"):
		field = "directory_url"
	case strings.Contains(message, "email"):
		field = "email"
	case strings.Contains(message, "id"):
		field = "id"
	}
	return apperr.Field(field, message)
}
