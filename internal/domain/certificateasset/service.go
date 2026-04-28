package certificateasset

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
	StatusActive     = "active"
	StatusExpiring   = "expiring"
	StatusRenewing   = "renewing"
	StatusDeployFail = "deploy_failed"
	StatusExpired    = "expired"
	StatusRevoked    = "revoked"
	StatusOrphaned   = "orphaned"

	VersionStatusIssued = "issued"
)

// Asset is the long-lived certificate governance anchor.
type Asset struct {
	ID               string         `json:"id"`
	Scope            resource.Scope `json:"-"`
	Name             string         `json:"name"`
	Status           string         `json:"status"`
	CAAccountID      string         `json:"ca_account_id"`
	CertificateType  string         `json:"certificate_type"`
	ChallengeType    string         `json:"challenge_type"`
	CommonName       string         `json:"common_name"`
	DomainIDs        []string       `json:"domain_ids,omitempty"`
	CurrentVersionID string         `json:"current_version_id,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

// Version is one immutable issued material snapshot for an asset.
type Version struct {
	ID                   string    `json:"id"`
	AssetID              string    `json:"asset_id"`
	VersionNo            int       `json:"version_no"`
	Status               string    `json:"status"`
	CertificateRequestID string    `json:"certificate_request_id"`
	IssueWorkflowID      string    `json:"issue_workflow_id"`
	CertificateRef       string    `json:"certificate_ref"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// IssueInput contains the facts required to persist an issued certificate.
type IssueInput struct {
	AssetID              string
	Name                 string
	CAAccountID          string
	CertificateType      string
	ChallengeType        string
	CommonName           string
	DomainIDs            []string
	CertificateRequestID string
	IssueWorkflowID      string
	CertificateRef       string
}

// Service stores certificate assets and versions in memory for the GA baseline.
type Service struct {
	mu              sync.RWMutex
	now             func() time.Time
	newID           func() string
	byID            map[string]Asset
	byEnvName       map[string]string
	versionsByAsset map[string][]Version
}

// NewService constructs an in-memory certificate asset service.
func NewService() *Service {
	return &Service{
		now: func() time.Time {
			return time.Now().UTC()
		},
		newID:           uuidx.New,
		byID:            make(map[string]Asset),
		byEnvName:       make(map[string]string),
		versionsByAsset: make(map[string][]Version),
	}
}

// List returns all assets under the caller scope.
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
		items = append(items, cloneAsset(asset))
	}

	slices.SortFunc(items, func(a Asset, b Asset) int {
		return strings.Compare(a.Name, b.Name)
	})
	return items, nil
}

// Get returns one asset under the caller scope.
func (s *Service) Get(scope resource.Scope, id string) (Asset, error) {
	if err := scope.Validate(); err != nil {
		return Asset{}, err
	}
	if strings.TrimSpace(id) == "" {
		return Asset{}, fmt.Errorf("asset id required: %w", resource.ErrValidation)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	asset, ok := s.byID[id]
	if !ok {
		return Asset{}, fmt.Errorf("certificate asset %s: %w", id, resource.ErrNotFound)
	}
	if !asset.Scope.Equals(scope) {
		return Asset{}, fmt.Errorf("certificate asset %s: %w", id, resource.ErrScopeMismatch)
	}
	return cloneAsset(asset), nil
}

// ListVersions returns the version history for one asset.
func (s *Service) ListVersions(scope resource.Scope, assetID string) ([]Version, error) {
	if _, err := s.Get(scope, assetID); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return append([]Version(nil), s.versionsByAsset[assetID]...), nil
}

// UpsertIssued persists one issued certificate version and creates the asset on first issue.
func (s *Service) UpsertIssued(scope resource.Scope, input IssueInput) (Asset, Version, bool, error) {
	if err := scope.Validate(); err != nil {
		return Asset{}, Version{}, false, err
	}

	normalized, err := normalizeIssueInput(input)
	if err != nil {
		return Asset{}, Version{}, false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	created := false
	var asset Asset
	if normalized.AssetID != "" {
		existing, ok := s.byID[normalized.AssetID]
		if !ok {
			return Asset{}, Version{}, false, fmt.Errorf("certificate asset %s: %w", normalized.AssetID, resource.ErrNotFound)
		}
		if !existing.Scope.Equals(scope) {
			return Asset{}, Version{}, false, fmt.Errorf("certificate asset %s: %w", normalized.AssetID, resource.ErrScopeMismatch)
		}
		asset = existing
	} else {
		envKey := scope.EnvironmentKey() + "/" + normalized.Name
		if existingID, ok := s.byEnvName[envKey]; ok {
			asset = s.byID[existingID]
		} else {
			created = true
			asset = Asset{
				ID:              s.newID(),
				Scope:           scope,
				Name:            normalized.Name,
				Status:          StatusActive,
				CAAccountID:     normalized.CAAccountID,
				CertificateType: normalized.CertificateType,
				ChallengeType:   normalized.ChallengeType,
				CommonName:      normalized.CommonName,
				DomainIDs:       append([]string(nil), normalized.DomainIDs...),
				CreatedAt:       now,
				UpdatedAt:       now,
			}
			s.byEnvName[envKey] = asset.ID
		}
	}

	versions := s.versionsByAsset[asset.ID]
	for _, version := range versions {
		if version.IssueWorkflowID == normalized.IssueWorkflowID && normalized.IssueWorkflowID != "" {
			asset.CurrentVersionID = version.ID
			asset.Status = StatusActive
			asset.UpdatedAt = now
			s.byID[asset.ID] = cloneAsset(asset)
			return cloneAsset(asset), version, created, nil
		}
	}

	version := Version{
		ID:                   s.newID(),
		AssetID:              asset.ID,
		VersionNo:            len(versions) + 1,
		Status:               VersionStatusIssued,
		CertificateRequestID: normalized.CertificateRequestID,
		IssueWorkflowID:      normalized.IssueWorkflowID,
		CertificateRef:       normalized.CertificateRef,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	asset.Status = StatusActive
	asset.CAAccountID = normalized.CAAccountID
	asset.CertificateType = normalized.CertificateType
	asset.ChallengeType = normalized.ChallengeType
	asset.CommonName = normalized.CommonName
	asset.DomainIDs = append([]string(nil), normalized.DomainIDs...)
	asset.CurrentVersionID = version.ID
	if asset.CreatedAt.IsZero() {
		asset.CreatedAt = now
	}
	asset.UpdatedAt = now

	s.byID[asset.ID] = cloneAsset(asset)
	s.versionsByAsset[asset.ID] = append(versions, version)
	return cloneAsset(asset), version, created, nil
}

func normalizeIssueInput(input IssueInput) (IssueInput, error) {
	name := strings.ToLower(strings.TrimSpace(input.Name))
	if name == "" {
		return IssueInput{}, fmt.Errorf("name required: %w", resource.ErrValidation)
	}
	if strings.TrimSpace(input.CAAccountID) == "" {
		return IssueInput{}, fmt.Errorf("ca_account_id required: %w", resource.ErrValidation)
	}
	if strings.TrimSpace(input.CommonName) == "" {
		return IssueInput{}, fmt.Errorf("common_name required: %w", resource.ErrValidation)
	}
	if strings.TrimSpace(input.CertificateRef) == "" {
		return IssueInput{}, fmt.Errorf("certificate_ref required: %w", resource.ErrValidation)
	}
	if strings.TrimSpace(input.CertificateRequestID) == "" {
		return IssueInput{}, fmt.Errorf("certificate_request_id required: %w", resource.ErrValidation)
	}
	if strings.TrimSpace(input.IssueWorkflowID) == "" {
		return IssueInput{}, fmt.Errorf("issue_workflow_id required: %w", resource.ErrValidation)
	}

	domainIDs := make([]string, 0, len(input.DomainIDs))
	seen := make(map[string]struct{}, len(input.DomainIDs))
	for _, domainID := range input.DomainIDs {
		domainID = strings.TrimSpace(domainID)
		if domainID == "" {
			return IssueInput{}, fmt.Errorf("domain_id required: %w", resource.ErrValidation)
		}
		if _, ok := seen[domainID]; ok {
			continue
		}
		seen[domainID] = struct{}{}
		domainIDs = append(domainIDs, domainID)
	}
	if len(domainIDs) == 0 {
		return IssueInput{}, fmt.Errorf("at least one domain required: %w", resource.ErrValidation)
	}

	return IssueInput{
		AssetID:              strings.TrimSpace(input.AssetID),
		Name:                 name,
		CAAccountID:          strings.TrimSpace(input.CAAccountID),
		CertificateType:      strings.ToLower(strings.TrimSpace(input.CertificateType)),
		ChallengeType:        strings.ToLower(strings.TrimSpace(input.ChallengeType)),
		CommonName:           strings.ToLower(strings.TrimSpace(input.CommonName)),
		DomainIDs:            domainIDs,
		CertificateRequestID: strings.TrimSpace(input.CertificateRequestID),
		IssueWorkflowID:      strings.TrimSpace(input.IssueWorkflowID),
		CertificateRef:       strings.TrimSpace(input.CertificateRef),
	}, nil
}

func cloneAsset(asset Asset) Asset {
	asset.DomainIDs = append([]string(nil), asset.DomainIDs...)
	return asset
}
