package deploymenttarget

import (
	"errors"
	"testing"
	"time"

	"github.com/zaneway/AutoCertX/internal/domain/resource"
)

func TestCreateNGINXDeploymentTarget(t *testing.T) {
	service := NewService()
	service.now = fixedClock(time.Date(2026, 4, 23, 10, 0, 0, 0, time.UTC))

	target, err := service.Create(testScope(), UpsertInput{
		Name:            "edge-nginx",
		TargetType:      TypeNGINX,
		AgentID:         "agent-1",
		ConfigPath:      "/etc/nginx/nginx.conf",
		CertificatePath: "/etc/nginx/certs/site.pem",
		PrivateKeyPath:  "/etc/nginx/certs/site.key",
	})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	if target.Status != StatusActive {
		t.Fatalf("status = %q, want %q", target.Status, StatusActive)
	}
	if target.TargetType != TypeNGINX {
		t.Fatalf("target_type = %q, want %q", target.TargetType, TypeNGINX)
	}
}

func TestCreateTomcatDeploymentTargetWithSelector(t *testing.T) {
	service := NewService()

	target, err := service.Create(testScope(), UpsertInput{
		Name:          "billing-tomcat",
		TargetType:    TypeTomcatJSSEPKCS12,
		AgentSelector: map[string]string{"runtime": "tomcat", "zone": "dmz"},
		ConfigPath:    "/opt/tomcat/conf/server.xml",
		KeystorePath:  "/opt/tomcat/conf/site.p12",
	})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	if target.AgentSelector["runtime"] != "tomcat" {
		t.Fatalf("agent selector = %+v, want runtime=tomcat", target.AgentSelector)
	}
}

func TestCreateRejectsTargetTypeSpecificMissingFields(t *testing.T) {
	service := NewService()

	_, err := service.Create(testScope(), UpsertInput{
		Name:       "bad-nginx",
		TargetType: TypeNGINX,
		AgentID:    "agent-1",
		ConfigPath: "/etc/nginx/nginx.conf",
	})
	if !errors.Is(err, resource.ErrValidation) {
		t.Fatalf("create invalid nginx error = %v, want validation", err)
	}

	_, err = service.Create(testScope(), UpsertInput{
		Name:       "bad-tomcat",
		TargetType: TypeTomcatJSSEPKCS12,
		AgentID:    "agent-1",
		ConfigPath: "/opt/tomcat/conf/server.xml",
	})
	if !errors.Is(err, resource.ErrValidation) {
		t.Fatalf("create invalid tomcat error = %v, want validation", err)
	}
}

func TestDeploymentTargetScopeAndNameIsolation(t *testing.T) {
	service := NewService()
	scope := testScope()
	target, err := service.Create(scope, validTarget())
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	if _, err := service.Create(scope, validTarget()); !errors.Is(err, resource.ErrConflict) {
		t.Fatalf("create duplicate error = %v, want conflict", err)
	}

	otherScope := resource.Scope{
		TenantID:      "tenant-2",
		ProjectID:     "project-1",
		EnvironmentID: "env-1",
	}
	if _, err := service.Get(otherScope, target.ID); !errors.Is(err, resource.ErrScopeMismatch) {
		t.Fatalf("get cross-scope error = %v, want scope mismatch", err)
	}
}

func TestUpdateAndDisableDeploymentTarget(t *testing.T) {
	service := NewService()
	scope := testScope()
	target, err := service.Create(scope, validTarget())
	if err != nil {
		t.Fatalf("create target: %v", err)
	}

	updated, err := service.Update(scope, target.ID, UpsertInput{
		Name:            "edge-nginx-renamed",
		TargetType:      TypeNGINX,
		AgentID:         "agent-2",
		ConfigPath:      "/etc/nginx/nginx.conf",
		CertificatePath: "/etc/nginx/certs/renamed.pem",
		PrivateKeyPath:  "/etc/nginx/certs/renamed.key",
	})
	if err != nil {
		t.Fatalf("update target: %v", err)
	}
	if updated.Name != "edge-nginx-renamed" || updated.AgentID != "agent-2" {
		t.Fatalf("updated target = %+v", updated)
	}

	disabled, err := service.Disable(scope, target.ID)
	if err != nil {
		t.Fatalf("disable target: %v", err)
	}
	if disabled.Status != StatusDisabled {
		t.Fatalf("status = %q, want %q", disabled.Status, StatusDisabled)
	}
}

func validTarget() UpsertInput {
	return UpsertInput{
		Name:            "edge-nginx",
		TargetType:      TypeNGINX,
		AgentID:         "agent-1",
		ConfigPath:      "/etc/nginx/nginx.conf",
		CertificatePath: "/etc/nginx/certs/site.pem",
		PrivateKeyPath:  "/etc/nginx/certs/site.key",
	}
}

func testScope() resource.Scope {
	return resource.Scope{
		TenantID:      "tenant-1",
		ProjectID:     "project-1",
		EnvironmentID: "env-1",
	}
}

func fixedClock(now time.Time) func() time.Time {
	return func() time.Time {
		return now
	}
}
