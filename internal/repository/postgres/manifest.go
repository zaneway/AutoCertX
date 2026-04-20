package postgres

// Migration describes one ordered SQL migration file.
type Migration struct {
	Version int
	Name    string
	Path    string
}

// Manifest returns the ordered PostgreSQL migration manifest.
func Manifest() []Migration {
	return []Migration{
		{
			Version: 1,
			Name:    "init_schema",
			Path:    "migrations/0001_init_schema.sql",
		},
	}
}

// BaseSchemaPath returns the canonical schema source used by the initial migration.
func BaseSchemaPath() string {
	return "sql/001_init_schema.sql"
}
