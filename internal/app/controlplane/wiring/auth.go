package wiring

import (
	"time"

	authcommand "github.com/zaneway/AutoCertX/internal/application/command/auth"
	authcontextquery "github.com/zaneway/AutoCertX/internal/application/query/authcontext"
	"github.com/zaneway/AutoCertX/internal/domain/identity"
	"github.com/zaneway/AutoCertX/internal/domain/tenancy"
	"github.com/zaneway/AutoCertX/internal/platform/config"
)

const (
	passwordAdmin   = "admin123!"
	passwordAuditor = "auditor123!"
)

func newAuthServices(cfg config.Config) (*authcommand.Service, *authcontextquery.Service, error) {
	hasher := identity.DefaultPasswordHasher()
	now := time.Now().UTC()

	adminHash, adminVersion, err := hasher.Hash(passwordAdmin)
	if err != nil {
		return nil, nil, err
	}
	auditorHash, auditorVersion, err := hasher.Hash(passwordAuditor)
	if err != nil {
		return nil, nil, err
	}

	identityStore := identity.NewMemoryStore(identity.SeedData{
		Users: []identity.User{
			{
				ID:          "44444444-4444-4444-8444-444444444441",
				TenantID:    "11111111-1111-4111-8111-111111111111",
				Username:    "admin",
				DisplayName: "Tenant Admin",
				Email:       "admin@acme.local",
				Locale:      authcommand.LocaleEN,
				Status:      identity.UserStatusActive,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			{
				ID:          "44444444-4444-4444-8444-444444444442",
				TenantID:    "11111111-1111-4111-8111-111111111111",
				Username:    "auditor",
				DisplayName: "Audit Reader",
				Email:       "auditor@acme.local",
				Status:      identity.UserStatusActive,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
		},
		Credentials: []identity.Credential{
			{
				ID:                  "77777777-7777-4777-8777-777777777771",
				UserID:              "44444444-4444-4444-8444-444444444441",
				CredentialType:      "password",
				PasswordHash:        adminHash,
				PasswordAlgoVersion: adminVersion,
				PasswordUpdatedAt:   now,
				CreatedAt:           now,
				UpdatedAt:           now,
			},
			{
				ID:                  "77777777-7777-4777-8777-777777777772",
				UserID:              "44444444-4444-4444-8444-444444444442",
				CredentialType:      "password",
				PasswordHash:        auditorHash,
				PasswordAlgoVersion: auditorVersion,
				PasswordUpdatedAt:   now,
				CreatedAt:           now,
				UpdatedAt:           now,
			},
		},
	})

	tenancyStore := tenancy.NewMemoryStore(tenancy.SeedData{
		Tenants: []tenancy.Tenant{
			{
				ID:     "11111111-1111-4111-8111-111111111111",
				Name:   "Acme",
				Code:   "acme",
				Locale: authcommand.LocaleZH,
				Status: tenancy.StatusActive,
			},
			{
				ID:     "11111111-1111-4111-8111-111111111112",
				Name:   "Other",
				Code:   "other",
				Locale: authcommand.LocaleEN,
				Status: tenancy.StatusActive,
			},
		},
		Projects: []tenancy.Project{
			{
				ID:       "22222222-2222-4222-8222-222222222221",
				TenantID: "11111111-1111-4111-8111-111111111111",
				Name:     "Core",
				Code:     "core",
				Status:   tenancy.StatusActive,
			},
			{
				ID:       "22222222-2222-4222-8222-222222222222",
				TenantID: "11111111-1111-4111-8111-111111111111",
				Name:     "Platform",
				Code:     "platform",
				Status:   tenancy.StatusActive,
			},
			{
				ID:       "22222222-2222-4222-8222-222222222223",
				TenantID: "11111111-1111-4111-8111-111111111112",
				Name:     "OtherProject",
				Code:     "other-project",
				Status:   tenancy.StatusActive,
			},
		},
		Environments: []tenancy.Environment{
			{
				ID:        "33333333-3333-4333-8333-333333333331",
				TenantID:  "11111111-1111-4111-8111-111111111111",
				ProjectID: "22222222-2222-4222-8222-222222222221",
				Name:      "Production",
				Code:      "prod",
				Status:    tenancy.StatusActive,
				EnvType:   "prod",
			},
			{
				ID:        "33333333-3333-4333-8333-333333333332",
				TenantID:  "11111111-1111-4111-8111-111111111111",
				ProjectID: "22222222-2222-4222-8222-222222222222",
				Name:      "Staging",
				Code:      "staging",
				Status:    tenancy.StatusActive,
				EnvType:   "staging",
			},
			{
				ID:        "33333333-3333-4333-8333-333333333333",
				TenantID:  "11111111-1111-4111-8111-111111111112",
				ProjectID: "22222222-2222-4222-8222-222222222223",
				Name:      "OtherProd",
				Code:      "prod",
				Status:    tenancy.StatusActive,
				EnvType:   "prod",
			},
		},
		Roles: []tenancy.Role{
			{
				ID:         "55555555-5555-4555-8555-555555555551",
				Code:       "tenant_admin",
				Name:       "Tenant Admin",
				ScopeLevel: tenancy.ScopeTenant,
				IsSystem:   true,
				Status:     tenancy.StatusActive,
			},
			{
				ID:         "55555555-5555-4555-8555-555555555552",
				Code:       "auditor",
				Name:       "Auditor",
				ScopeLevel: tenancy.ScopeEnvironment,
				IsSystem:   true,
				Status:     tenancy.StatusActive,
			},
		},
		Bindings: []tenancy.RoleBinding{
			{
				ID:        "66666666-6666-4666-8666-666666666661",
				TenantID:  "11111111-1111-4111-8111-111111111111",
				UserID:    "44444444-4444-4444-8444-444444444441",
				RoleID:    "55555555-5555-4555-8555-555555555551",
				ScopeType: tenancy.ScopeTenant,
				ScopeID:   "11111111-1111-4111-8111-111111111111",
				Status:    tenancy.StatusActive,
			},
			{
				ID:            "66666666-6666-4666-8666-666666666662",
				TenantID:      "11111111-1111-4111-8111-111111111111",
				ProjectID:     "22222222-2222-4222-8222-222222222221",
				EnvironmentID: "33333333-3333-4333-8333-333333333331",
				UserID:        "44444444-4444-4444-8444-444444444442",
				RoleID:        "55555555-5555-4555-8555-555555555552",
				ScopeType:     tenancy.ScopeEnvironment,
				ScopeID:       "33333333-3333-4333-8333-333333333331",
				Status:        tenancy.StatusActive,
			},
		},
	})

	signer := identity.NewTokenSigner(cfg.Auth.SigningKey, cfg.ServiceName, func() time.Time {
		return time.Now().UTC()
	})
	identityService := identity.NewService(
		identityStore,
		identityStore,
		identityStore,
		hasher,
		signer,
		identity.Options{
			AccessTTL:  cfg.Auth.AccessTokenTTL,
			RefreshTTL: cfg.Auth.RefreshTokenTTL,
		},
	)

	return authcommand.NewService(identityService, tenancy.NewService(tenancyStore)), authcontextquery.NewService(authcommand.LocaleZH), nil
}
