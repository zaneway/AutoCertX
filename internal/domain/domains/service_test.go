package domains

import (
	"testing"

	"github.com/zaneway/AutoCertX/internal/domain/resource"
)

func TestServiceRejectsWildcardHTTP01(t *testing.T) {
	service := NewService()
	scope := resource.Scope{
		TenantID:      "11111111-1111-4111-8111-111111111111",
		ProjectID:     "22222222-2222-4222-8222-222222222222",
		EnvironmentID: "33333333-3333-4333-8333-333333333333",
	}

	_, err := service.Create(scope, UpsertInput{
		Name:          "*.example.com",
		ChallengeType: ChallengeHTTP01,
	})
	if err == nil {
		t.Fatal("Create() error = nil, want validation error")
	}
}
