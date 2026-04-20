package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/zaneway/AutoCertX/internal/domain/identity"
	"github.com/zaneway/AutoCertX/internal/domain/tenancy"
)

const (
	LocaleZH = "zh-CN"
	LocaleEN = "en-US"
)

// Principal is the authenticated subject injected into protected requests.
type Principal struct {
	User    identity.User
	Session identity.Session
	Context tenancy.ResolvedContext
}

// Selection contains the request-selected tenant/project/environment scope.
type Selection struct {
	TenantID      string
	ProjectID     string
	EnvironmentID string
}

// Result is the response payload returned by login and refresh flows.
type Result struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
	Principal    Principal
}

// Service orchestrates identity and tenancy concerns for auth use cases.
type Service struct {
	identity *identity.Service
	tenancy  *tenancy.Service
}

// NewService constructs a new auth command service.
func NewService(identityService *identity.Service, tenancyService *tenancy.Service) *Service {
	return &Service{
		identity: identityService,
		tenancy:  tenancyService,
	}
}

// Login authenticates the user and resolves the active scope.
func (s *Service) Login(
	ctx context.Context,
	username string,
	password string,
	selection Selection,
	metadata identity.SessionMetadata,
) (Result, error) {
	result, err := s.identity.Login(ctx, username, password, metadata)
	if err != nil {
		return Result{}, err
	}

	principal, err := s.principalFromAuth(ctx, result.AuthenticatedUser, selection)
	if err != nil {
		return Result{}, err
	}

	return Result{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresIn:    result.ExpiresIn,
		Principal:    principal,
	}, nil
}

// Refresh rotates the session tokens and resolves the requested scope again.
func (s *Service) Refresh(
	ctx context.Context,
	refreshToken string,
	selection Selection,
	metadata identity.SessionMetadata,
) (Result, error) {
	result, err := s.identity.Refresh(ctx, refreshToken, metadata)
	if err != nil {
		return Result{}, err
	}

	principal, err := s.principalFromAuth(ctx, result.AuthenticatedUser, selection)
	if err != nil {
		return Result{}, err
	}

	return Result{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresIn:    result.ExpiresIn,
		Principal:    principal,
	}, nil
}

// Authenticate verifies the access token and resolves request-selected scope.
func (s *Service) Authenticate(ctx context.Context, accessToken string, selection Selection) (Principal, error) {
	authenticated, err := s.identity.AuthenticateAccessToken(ctx, accessToken)
	if err != nil {
		return Principal{}, err
	}

	return s.principalFromAuth(ctx, authenticated, selection)
}

// Logout revokes the current session.
func (s *Service) Logout(ctx context.Context, principal Principal) error {
	return s.identity.Logout(ctx, principal.Session.ID)
}

// UpdateLocale persists the user locale and returns the refreshed principal.
func (s *Service) UpdateLocale(ctx context.Context, principal Principal, locale string) (Principal, error) {
	if !isSupportedLocale(locale) {
		return Principal{}, fmt.Errorf("unsupported locale: %s", locale)
	}

	user, err := s.identity.UpdateLocale(ctx, principal.User.ID, locale)
	if err != nil {
		return Principal{}, err
	}

	resolved, err := s.tenancy.Resolve(ctx, user.ID, Selection{
		TenantID:      principal.Context.Tenant.ID,
		ProjectID:     principal.Context.Project.ID,
		EnvironmentID: principal.Context.Environment.ID,
	}.toTenancySelection())
	if err != nil {
		return Principal{}, err
	}

	return Principal{
		User:    user,
		Session: principal.Session,
		Context: resolved,
	}, nil
}

func (s *Service) principalFromAuth(
	ctx context.Context,
	authenticated identity.AuthenticatedUser,
	selection Selection,
) (Principal, error) {
	resolved, err := s.tenancy.Resolve(ctx, authenticated.User.ID, selection.toTenancySelection())
	if err != nil {
		if errors.Is(err, tenancy.ErrScopeMismatch) {
			return Principal{}, tenancy.ErrScopeMismatch
		}
		return Principal{}, err
	}

	return Principal{
		User:    authenticated.User,
		Session: authenticated.Session,
		Context: resolved,
	}, nil
}

func (s Selection) toTenancySelection() tenancy.Selection {
	return tenancy.Selection{
		TenantID:      s.TenantID,
		ProjectID:     s.ProjectID,
		EnvironmentID: s.EnvironmentID,
	}
}

func isSupportedLocale(locale string) bool {
	return locale == LocaleZH || locale == LocaleEN
}
