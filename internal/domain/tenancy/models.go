package tenancy

import "sort"

type Status string

const (
	StatusActive   Status = "active"
	StatusArchived Status = "archived"
	StatusDisabled Status = "disabled"
)

type ScopeType string

const (
	ScopeTenant      ScopeType = "tenant"
	ScopeProject     ScopeType = "project"
	ScopeEnvironment ScopeType = "environment"
)

type Permission string

const (
	PermissionWildcard             Permission = "*"
	PermissionAuthContextRead      Permission = "auth.context.read"
	PermissionAuthPreferencesWrite Permission = "auth.preferences.write"
	PermissionAuditRead            Permission = "audit.read"
	PermissionAuditExport          Permission = "audit.export"
	PermissionSettingsRead         Permission = "settings.read"
	PermissionSettingsWrite        Permission = "settings.write"
)

// Tenant models the first-level isolation boundary.
type Tenant struct {
	ID     string
	Name   string
	Code   string
	Locale string
	Status Status
}

// Project groups environments inside a tenant.
type Project struct {
	ID       string
	TenantID string
	Name     string
	Code     string
	Status   Status
}

// Environment is the resource scope used by business capabilities.
type Environment struct {
	ID        string
	TenantID  string
	ProjectID string
	Name      string
	Code      string
	Status    Status
	EnvType   string
}

// Role is a permission bundle with a scope level.
type Role struct {
	ID         string
	TenantID   string
	Code       string
	Name       string
	ScopeLevel ScopeType
	IsSystem   bool
	Status     Status
}

// RoleBinding attaches a role to a user and a concrete scope object.
type RoleBinding struct {
	ID            string
	TenantID      string
	ProjectID     string
	EnvironmentID string
	UserID        string
	RoleID        string
	ScopeType     ScopeType
	ScopeID       string
	Status        Status
}

// Selection contains the caller-requested scope.
type Selection struct {
	TenantID      string
	ProjectID     string
	EnvironmentID string
}

// ResolvedContext is the active tenant/project/environment scope plus effective roles.
type ResolvedContext struct {
	Tenant      Tenant
	Project     Project
	Environment Environment
	RoleCodes   []string
	permissions map[Permission]struct{}
}

// HasPermission reports whether the resolved context grants the supplied permission.
func (c ResolvedContext) HasPermission(permission Permission) bool {
	if _, ok := c.permissions[PermissionWildcard]; ok {
		return true
	}
	_, ok := c.permissions[permission]
	return ok
}

func rolePermissions(roleCodes []string) map[Permission]struct{} {
	result := make(map[Permission]struct{})
	for _, roleCode := range roleCodes {
		for _, permission := range systemRolePermissions[roleCode] {
			result[permission] = struct{}{}
		}
	}

	return result
}

var systemRolePermissions = map[string][]Permission{
	"tenant_admin": {
		PermissionWildcard,
	},
	"security_admin": {
		PermissionAuthContextRead,
		PermissionAuthPreferencesWrite,
		PermissionAuditRead,
		PermissionSettingsRead,
		PermissionSettingsWrite,
	},
	"platform_engineer": {
		PermissionAuthContextRead,
		PermissionAuthPreferencesWrite,
		PermissionAuditRead,
		PermissionSettingsRead,
		PermissionSettingsWrite,
	},
	"auditor": {
		PermissionAuthContextRead,
		PermissionAuditRead,
		PermissionAuditExport,
	},
}

func sortRoleCodes(roleCodes []string) []string {
	cloned := append([]string(nil), roleCodes...)
	sort.Strings(cloned)
	return cloned
}
