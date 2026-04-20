package tenancy

import (
	"context"
	"errors"
	"testing"
)

func TestServiceResolveHonorsRequestedScope(t *testing.T) {
	service := NewService(newTestStore())

	resolved, err := service.Resolve(context.Background(), "user-admin", Selection{
		ProjectID:     "project-platform",
		EnvironmentID: "env-staging",
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.Project.Code != "platform" {
		t.Fatalf("project code = %q, want %q", resolved.Project.Code, "platform")
	}
	if resolved.Environment.Code != "staging" {
		t.Fatalf("environment code = %q, want %q", resolved.Environment.Code, "staging")
	}
	if !resolved.HasPermission(PermissionAuthPreferencesWrite) {
		t.Fatal("tenant_admin should have auth.preferences.write")
	}
}

func TestServiceResolveRejectsCrossTenantScope(t *testing.T) {
	service := NewService(newTestStore())

	_, err := service.Resolve(context.Background(), "user-admin", Selection{
		TenantID: "tenant-other",
	})
	if !errors.Is(err, ErrScopeMismatch) {
		t.Fatalf("Resolve() error = %v, want %v", err, ErrScopeMismatch)
	}
}

func TestServiceResolveEnvironmentScopedUserGetsReadOnlyRole(t *testing.T) {
	service := NewService(newTestStore())

	resolved, err := service.Resolve(context.Background(), "user-auditor", Selection{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if len(resolved.RoleCodes) != 1 || resolved.RoleCodes[0] != "auditor" {
		t.Fatalf("role codes = %v, want [auditor]", resolved.RoleCodes)
	}
	if !resolved.HasPermission(PermissionAuthContextRead) {
		t.Fatal("auditor should have auth.context.read")
	}
	if resolved.HasPermission(PermissionAuthPreferencesWrite) {
		t.Fatal("auditor should not have auth.preferences.write")
	}
}

func newTestStore() *MemoryStore {
	return NewMemoryStore(SeedData{
		Tenants: []Tenant{
			{ID: "tenant-acme", Name: "Acme", Code: "acme", Locale: "zh-CN", Status: StatusActive},
			{ID: "tenant-other", Name: "Other", Code: "other", Locale: "en-US", Status: StatusActive},
		},
		Projects: []Project{
			{ID: "project-core", TenantID: "tenant-acme", Name: "Core", Code: "core", Status: StatusActive},
			{ID: "project-platform", TenantID: "tenant-acme", Name: "Platform", Code: "platform", Status: StatusActive},
			{ID: "project-other", TenantID: "tenant-other", Name: "Other", Code: "other", Status: StatusActive},
		},
		Environments: []Environment{
			{ID: "env-prod", TenantID: "tenant-acme", ProjectID: "project-core", Name: "Production", Code: "prod", Status: StatusActive},
			{ID: "env-staging", TenantID: "tenant-acme", ProjectID: "project-platform", Name: "Staging", Code: "staging", Status: StatusActive},
			{ID: "env-other", TenantID: "tenant-other", ProjectID: "project-other", Name: "OtherProd", Code: "prod", Status: StatusActive},
		},
		Roles: []Role{
			{ID: "role-tenant-admin", Code: "tenant_admin", ScopeLevel: ScopeTenant, Status: StatusActive},
			{ID: "role-auditor", Code: "auditor", ScopeLevel: ScopeEnvironment, Status: StatusActive},
		},
		Bindings: []RoleBinding{
			{
				ID:        "binding-admin",
				TenantID:  "tenant-acme",
				UserID:    "user-admin",
				RoleID:    "role-tenant-admin",
				ScopeType: ScopeTenant,
				ScopeID:   "tenant-acme",
				Status:    StatusActive,
			},
			{
				ID:            "binding-auditor",
				TenantID:      "tenant-acme",
				ProjectID:     "project-core",
				EnvironmentID: "env-prod",
				UserID:        "user-auditor",
				RoleID:        "role-auditor",
				ScopeType:     ScopeEnvironment,
				ScopeID:       "env-prod",
				Status:        StatusActive,
			},
		},
	})
}
