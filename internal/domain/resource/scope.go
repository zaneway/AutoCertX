package resource

import "fmt"

// Scope identifies the tenant/project/environment boundary of a resource.
type Scope struct {
	TenantID      string `json:"tenant_id"`
	ProjectID     string `json:"project_id"`
	EnvironmentID string `json:"environment_id"`
}

// Validate ensures the scope is complete.
func (s Scope) Validate() error {
	if s.TenantID == "" {
		return fmt.Errorf("tenant_id required: %w", ErrValidation)
	}
	if s.ProjectID == "" {
		return fmt.Errorf("project_id required: %w", ErrValidation)
	}
	if s.EnvironmentID == "" {
		return fmt.Errorf("environment_id required: %w", ErrValidation)
	}
	return nil
}

// Equals reports whether two scopes point at the same boundary.
func (s Scope) Equals(other Scope) bool {
	return s.TenantID == other.TenantID &&
		s.ProjectID == other.ProjectID &&
		s.EnvironmentID == other.EnvironmentID
}

// EnvironmentKey provides a stable key for environment-level uniqueness checks.
func (s Scope) EnvironmentKey() string {
	return s.TenantID + "/" + s.ProjectID + "/" + s.EnvironmentID
}
