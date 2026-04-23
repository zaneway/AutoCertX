package tenancy

import (
	"context"
	"errors"
	"sort"
)

var (
	ErrScopeMismatch         = errors.New("scope mismatch")
	ErrNoAccessibleScope     = errors.New("no accessible scope")
	ErrPermissionDenied      = errors.New("permission denied")
	errTenancyRecordNotFound = errors.New("tenancy record not found")
)

// Repository describes the tenancy and RBAC data required to resolve context.
type Repository interface {
	ListRoleBindingsByUser(ctx context.Context, userID string) ([]RoleBinding, error)
	ListRolesByIDs(ctx context.Context, roleIDs []string) ([]Role, error)
	GetTenant(ctx context.Context, tenantID string) (Tenant, error)
	ListProjectsByTenant(ctx context.Context, tenantID string) ([]Project, error)
	GetProject(ctx context.Context, projectID string) (Project, error)
	ListEnvironmentsByProject(ctx context.Context, projectID string) ([]Environment, error)
	GetEnvironment(ctx context.Context, environmentID string) (Environment, error)
}

// Service resolves effective tenant/project/environment scope and attached roles.
type Service struct {
	repo Repository
}

// NewService constructs a tenancy service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Resolve returns the active scope requested by the caller or the default scope derived from bindings.
func (s *Service) Resolve(ctx context.Context, userID string, selection Selection) (ResolvedContext, error) {
	bindings, err := s.repo.ListRoleBindingsByUser(ctx, userID)
	if err != nil || len(bindings) == 0 {
		return ResolvedContext{}, ErrNoAccessibleScope
	}

	// Build the active role set first so disabled bindings/roles never participate
	// in later scope expansion.
	roleIDs := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		if binding.Status != StatusActive {
			continue
		}
		roleIDs = append(roleIDs, binding.RoleID)
	}

	roles, err := s.repo.ListRolesByIDs(ctx, roleIDs)
	if err != nil {
		return ResolvedContext{}, ErrNoAccessibleScope
	}

	roleByID := make(map[string]Role, len(roles))
	for _, role := range roles {
		if role.Status != StatusActive {
			continue
		}
		roleByID[role.ID] = role
	}

	tenantID, err := s.selectTenant(ctx, selection.TenantID, bindings, roleByID)
	if err != nil {
		return ResolvedContext{}, err
	}
	tenant, err := s.repo.GetTenant(ctx, tenantID)
	if err != nil || tenant.Status != StatusActive {
		return ResolvedContext{}, ErrScopeMismatch
	}

	projectID, err := s.selectProject(ctx, tenantID, selection.ProjectID, bindings, roleByID)
	if err != nil {
		return ResolvedContext{}, err
	}
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil || project.Status != StatusActive || project.TenantID != tenant.ID {
		return ResolvedContext{}, ErrScopeMismatch
	}

	// Environment is resolved last because project/tenant-scoped roles may expand
	// the visible environment set underneath the chosen project.
	environmentID, err := s.selectEnvironment(ctx, project.ID, selection.EnvironmentID, bindings, roleByID)
	if err != nil {
		return ResolvedContext{}, err
	}
	environment, err := s.repo.GetEnvironment(ctx, environmentID)
	if err != nil || environment.Status != StatusActive || environment.ProjectID != project.ID {
		return ResolvedContext{}, ErrScopeMismatch
	}

	roleCodes := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		if binding.Status != StatusActive {
			continue
		}

		role, ok := roleByID[binding.RoleID]
		if !ok {
			continue
		}
		// Only bindings that actually cover the resolved tenant/project/
		// environment triple contribute effective role codes.
		if bindingAppliesToContext(binding, tenant.ID, project.ID, environment.ID) {
			roleCodes = append(roleCodes, role.Code)
		}
	}

	roleCodes = uniqueStrings(sortRoleCodes(roleCodes))

	return ResolvedContext{
		Tenant:      tenant,
		Project:     project,
		Environment: environment,
		RoleCodes:   roleCodes,
		permissions: rolePermissions(roleCodes),
	}, nil
}

func (s *Service) selectTenant(
	ctx context.Context,
	requestedTenantID string,
	bindings []RoleBinding,
	roleByID map[string]Role,
) (string, error) {
	tenantIDs := make(map[string]struct{})
	for _, binding := range bindings {
		if binding.Status != StatusActive {
			continue
		}
		if _, ok := roleByID[binding.RoleID]; !ok {
			continue
		}
		tenantIDs[binding.TenantID] = struct{}{}
	}

	if requestedTenantID != "" {
		// Explicit selections must already be reachable from the user's bindings;
		// the resolver does not silently substitute another tenant.
		if _, ok := tenantIDs[requestedTenantID]; !ok {
			return "", ErrScopeMismatch
		}
		return requestedTenantID, nil
	}

	ordered := sortedKeys(tenantIDs)
	if len(ordered) == 0 {
		return "", ErrNoAccessibleScope
	}

	return ordered[0], nil
}

func (s *Service) selectProject(
	ctx context.Context,
	tenantID string,
	requestedProjectID string,
	bindings []RoleBinding,
	roleByID map[string]Role,
) (string, error) {
	projects, err := s.repo.ListProjectsByTenant(ctx, tenantID)
	if err != nil {
		return "", ErrNoAccessibleScope
	}

	projectByID := make(map[string]Project, len(projects))
	for _, project := range projects {
		if project.Status != StatusActive {
			continue
		}
		projectByID[project.ID] = project
	}

	accessible := make(map[string]struct{})
	for _, binding := range bindings {
		if binding.Status != StatusActive || binding.TenantID != tenantID {
			continue
		}
		role, ok := roleByID[binding.RoleID]
		if !ok {
			continue
		}

		switch role.ScopeLevel {
		case ScopeTenant:
			// Tenant-scoped roles implicitly cover every active project beneath the
			// selected tenant.
			for projectID := range projectByID {
				accessible[projectID] = struct{}{}
			}
		case ScopeProject:
			if _, ok := projectByID[binding.ProjectID]; ok {
				accessible[binding.ProjectID] = struct{}{}
			}
		case ScopeEnvironment:
			// Environment-scoped roles still grant access to the parent project so
			// the caller can materialize a coherent project context.
			environment, err := s.repo.GetEnvironment(ctx, binding.EnvironmentID)
			if err != nil || environment.Status != StatusActive {
				continue
			}
			if _, ok := projectByID[environment.ProjectID]; ok {
				accessible[environment.ProjectID] = struct{}{}
			}
		}
	}

	if requestedProjectID != "" {
		if _, ok := accessible[requestedProjectID]; !ok {
			return "", ErrScopeMismatch
		}
		return requestedProjectID, nil
	}

	ordered := sortedKeys(accessible)
	if len(ordered) == 0 {
		return "", ErrNoAccessibleScope
	}

	return ordered[0], nil
}

func (s *Service) selectEnvironment(
	ctx context.Context,
	projectID string,
	requestedEnvironmentID string,
	bindings []RoleBinding,
	roleByID map[string]Role,
) (string, error) {
	environments, err := s.repo.ListEnvironmentsByProject(ctx, projectID)
	if err != nil {
		return "", ErrNoAccessibleScope
	}

	environmentByID := make(map[string]Environment, len(environments))
	for _, environment := range environments {
		if environment.Status != StatusActive {
			continue
		}
		environmentByID[environment.ID] = environment
	}

	accessible := make(map[string]struct{})
	for _, binding := range bindings {
		role, ok := roleByID[binding.RoleID]
		if !ok || binding.Status != StatusActive {
			continue
		}

		switch role.ScopeLevel {
		case ScopeTenant, ScopeProject:
			// Tenant/project roles inherit visibility to every active environment in
			// the selected project.
			for environmentID := range environmentByID {
				accessible[environmentID] = struct{}{}
			}
		case ScopeEnvironment:
			if _, ok := environmentByID[binding.EnvironmentID]; ok {
				accessible[binding.EnvironmentID] = struct{}{}
			}
		}
	}

	if requestedEnvironmentID != "" {
		if _, ok := accessible[requestedEnvironmentID]; !ok {
			return "", ErrScopeMismatch
		}
		return requestedEnvironmentID, nil
	}

	ordered := sortedKeys(accessible)
	if len(ordered) == 0 {
		return "", ErrNoAccessibleScope
	}

	return ordered[0], nil
}

func bindingAppliesToContext(binding RoleBinding, tenantID string, projectID string, environmentID string) bool {
	switch binding.ScopeType {
	case ScopeTenant:
		return binding.TenantID == tenantID
	case ScopeProject:
		return binding.ProjectID == projectID
	case ScopeEnvironment:
		return binding.EnvironmentID == environmentID
	default:
		return false
	}
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for value := range values {
		keys = append(keys, value)
	}
	sort.Strings(keys)
	return keys
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}

	return result
}
