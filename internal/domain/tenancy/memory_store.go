package tenancy

import (
	"context"
	"sync"
)

// SeedData contains the bootstrap records for the in-memory tenancy repository.
type SeedData struct {
	Tenants      []Tenant
	Projects     []Project
	Environments []Environment
	Roles        []Role
	Bindings     []RoleBinding
}

// MemoryStore is a thread-safe in-memory tenancy repository.
type MemoryStore struct {
	mu             sync.RWMutex
	tenants        map[string]Tenant
	projects       map[string]Project
	environments   map[string]Environment
	roles          map[string]Role
	bindingsByUser map[string][]RoleBinding
}

// NewMemoryStore constructs a new in-memory tenancy repository.
func NewMemoryStore(seed SeedData) *MemoryStore {
	store := &MemoryStore{
		tenants:        make(map[string]Tenant, len(seed.Tenants)),
		projects:       make(map[string]Project, len(seed.Projects)),
		environments:   make(map[string]Environment, len(seed.Environments)),
		roles:          make(map[string]Role, len(seed.Roles)),
		bindingsByUser: make(map[string][]RoleBinding),
	}

	for _, tenant := range seed.Tenants {
		store.tenants[tenant.ID] = tenant
	}
	for _, project := range seed.Projects {
		store.projects[project.ID] = project
	}
	for _, environment := range seed.Environments {
		store.environments[environment.ID] = environment
	}
	for _, role := range seed.Roles {
		store.roles[role.ID] = role
	}
	for _, binding := range seed.Bindings {
		store.bindingsByUser[binding.UserID] = append(store.bindingsByUser[binding.UserID], binding)
	}

	return store
}

func (s *MemoryStore) ListRoleBindingsByUser(_ context.Context, userID string) ([]RoleBinding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	bindings := append([]RoleBinding(nil), s.bindingsByUser[userID]...)
	return bindings, nil
}

func (s *MemoryStore) ListRolesByIDs(_ context.Context, roleIDs []string) ([]Role, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Role, 0, len(roleIDs))
	seen := make(map[string]struct{}, len(roleIDs))
	for _, roleID := range roleIDs {
		if _, ok := seen[roleID]; ok {
			continue
		}
		seen[roleID] = struct{}{}
		role, ok := s.roles[roleID]
		if !ok {
			continue
		}
		result = append(result, role)
	}

	return result, nil
}

func (s *MemoryStore) GetTenant(_ context.Context, tenantID string) (Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tenant, ok := s.tenants[tenantID]
	if !ok {
		return Tenant{}, errTenancyRecordNotFound
	}
	return tenant, nil
}

func (s *MemoryStore) ListProjectsByTenant(_ context.Context, tenantID string) ([]Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Project, 0)
	for _, project := range s.projects {
		if project.TenantID == tenantID {
			result = append(result, project)
		}
	}

	return result, nil
}

func (s *MemoryStore) GetProject(_ context.Context, projectID string) (Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	project, ok := s.projects[projectID]
	if !ok {
		return Project{}, errTenancyRecordNotFound
	}
	return project, nil
}

func (s *MemoryStore) ListEnvironmentsByProject(_ context.Context, projectID string) ([]Environment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Environment, 0)
	for _, environment := range s.environments {
		if environment.ProjectID == projectID {
			result = append(result, environment)
		}
	}

	return result, nil
}

func (s *MemoryStore) GetEnvironment(_ context.Context, environmentID string) (Environment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	environment, ok := s.environments[environmentID]
	if !ok {
		return Environment{}, errTenancyRecordNotFound
	}
	return environment, nil
}
