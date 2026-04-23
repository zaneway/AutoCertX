package authcontext

import (
	"github.com/zaneway/AutoCertX/internal/domain/identity"
	"github.com/zaneway/AutoCertX/internal/domain/tenancy"
)

// NamedResource is the stable response shape used by the auth context API.
type NamedResource struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Code        string `json:"code,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

// View is the response payload for GET /api/v1/auth/me.
type View struct {
	Locale           string        `json:"locale"`
	AvailableLocales []string      `json:"available_locales"`
	User             NamedResource `json:"user"`
	Tenant           NamedResource `json:"tenant"`
	Project          NamedResource `json:"project"`
	Environment      NamedResource `json:"environment"`
	Roles            []string      `json:"roles"`
}

// Service builds the auth context payload consumed by the frontend shell.
type Service struct {
	systemDefaultLocale string
}

// NewService constructs a new auth context query service.
func NewService(systemDefaultLocale string) *Service {
	return &Service{systemDefaultLocale: systemDefaultLocale}
}

// Build maps authenticated user and scope data into the API response contract.
func (s *Service) Build(user identity.User, context tenancy.ResolvedContext) View {
	locale := user.Locale
	if locale == "" {
		// Locale falls back from user preference to tenant preference and finally to
		// the system default so the shell always has a deterministic language.
		locale = context.Tenant.Locale
	}
	if locale == "" {
		locale = s.systemDefaultLocale
	}

	return View{
		Locale:           locale,
		AvailableLocales: []string{"zh-CN", "en-US"},
		User: NamedResource{
			ID:          user.ID,
			Name:        user.Username,
			DisplayName: user.DisplayName,
		},
		Tenant: NamedResource{
			ID:   context.Tenant.ID,
			Name: context.Tenant.Name,
			Code: context.Tenant.Code,
		},
		Project: NamedResource{
			ID:   context.Project.ID,
			Name: context.Project.Name,
			Code: context.Project.Code,
		},
		Environment: NamedResource{
			ID:   context.Environment.ID,
			Name: context.Environment.Name,
			Code: context.Environment.Code,
		},
		Roles: append([]string(nil), context.RoleCodes...),
	}
}
