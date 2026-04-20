package postgres

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestManifestHasInitMigration(t *testing.T) {
	migrations := Manifest()
	if len(migrations) == 0 {
		t.Fatal("Manifest() should not be empty")
	}

	first := migrations[0]
	if first.Version != 1 {
		t.Fatalf("first migration version = %d, want 1", first.Version)
	}
	if first.Path != "migrations/0001_init_schema.sql" {
		t.Fatalf("first migration path = %q, want %q", first.Path, "migrations/0001_init_schema.sql")
	}

	if _, err := os.Stat(filepath.Join(repoRoot(t), first.Path)); err != nil {
		t.Fatalf("migration file should exist: %v", err)
	}
}

func TestMigrationWrapperReferencesBaseSchema(t *testing.T) {
	content := readFile(t, "migrations/0001_init_schema.sql")
	if !strings.Contains(content, `\ir ../sql/001_init_schema.sql`) {
		t.Fatal("migration wrapper should reference sql/001_init_schema.sql with \\ir")
	}
}

func TestBaseSchemaHasStandardColumnsOnAllTables(t *testing.T) {
	content := readFile(t, BaseSchemaPath())
	matches := tableRegexp().FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		t.Fatal("base schema should contain CREATE TABLE statements")
	}

	for _, match := range matches {
		tableName := match[1]
		body := match[2]
		for _, field := range []string{"id uuid", "created_at timestamptz", "updated_at timestamptz"} {
			if !strings.Contains(body, field) {
				t.Fatalf("table %q missing standard field %q", tableName, field)
			}
		}
	}
}

func TestBaseSchemaIncludesCoreTablesAndIndexes(t *testing.T) {
	content := readFile(t, BaseSchemaPath())

	requiredTables := []string{
		"tenants",
		"projects",
		"environments",
		"users",
		"roles",
		"role_bindings",
		"dns_credentials",
		"ca_accounts",
		"domain_assets",
		"certificate_assets",
		"certificate_requests",
		"issue_workflows",
		"workflow_challenges",
		"certificate_versions",
		"deployment_targets",
		"agents",
		"jobs",
		"job_attempts",
		"discovery_records",
		"audit_events",
		"webhook_endpoints",
		"notification_events",
	}
	for _, tableName := range requiredTables {
		if !strings.Contains(content, "CREATE TABLE "+tableName+" (") {
			t.Fatalf("base schema missing required table %q", tableName)
		}
	}

	requiredIndexes := []string{
		"idx_jobs_schedulable",
		"idx_jobs_lease",
		"idx_discovery_records_env_status",
		"idx_audit_events_tenant_time",
		"uk_webhook_endpoints_tenant_env_name",
	}
	for _, indexName := range requiredIndexes {
		if !strings.Contains(content, indexName) {
			t.Fatalf("base schema missing required index or constraint %q", indexName)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}

	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repo root should contain go.mod: %v", err)
	}

	return root
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(repoRoot(t), path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return string(content)
}

func tableRegexp() *regexp.Regexp {
	return regexp.MustCompile(`(?ms)CREATE TABLE ([a-z_]+) \((.*?)\n\);`)
}
