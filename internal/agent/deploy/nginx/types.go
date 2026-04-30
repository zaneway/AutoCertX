package nginx

import (
	"context"
	"io/fs"

	verifynginx "github.com/zaneway/AutoCertX/internal/agent/verify/nginx"
)

const (
	StatusPending        = "pending"
	StatusPrechecking    = "prechecking"
	StatusBackingUp      = "backing_up"
	StatusInstalling     = "installing"
	StatusReloading      = "reloading"
	StatusVerifying      = "verifying"
	StatusSucceeded      = "succeeded"
	StatusFailed         = "failed"
	StatusRollingBack    = "rolling_back"
	StatusRolledBack     = "rolled_back"
	StatusRollbackFailed = "rollback_failed"

	StepStatusSucceeded = "succeeded"
	StepStatusFailed    = "failed"
)

// DeployRequest is the Agent-local input for installing one NGINX certificate.
type DeployRequest struct {
	TargetID            string
	CertificatePEM      []byte
	PrivateKeyPEM       []byte
	CertificatePath     string
	PrivateKeyPath      string
	ConfigPath          string
	NGINXTestCommand    Command
	ReloadCommand       Command
	AllowedPaths        []string
	VerifyHost          string
	VerifyPort          int
	ServerName          string
	ExpectedFingerprint string
}

// Command describes a local command execution.
type Command struct {
	Name string
	Args []string
}

// CommandResult captures stdout/stderr for evidence and diagnostics.
type CommandResult struct {
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode int    `json:"exit_code"`
}

// StepResult is one state-machine step outcome.
type StepResult struct {
	Stage    string `json:"stage"`
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
}

// RollbackResult records compensation details.
type RollbackResult struct {
	Status        string        `json:"status"`
	Message       string        `json:"message,omitempty"`
	RestoredPaths []string      `json:"restored_paths,omitempty"`
	ConfigTest    CommandResult `json:"config_test"`
	Reload        CommandResult `json:"reload"`
}

// Evidence summarizes facts later persisted by DeploymentRecord/Audit.
type Evidence struct {
	TargetID            string                   `json:"target_id,omitempty"`
	AffectedPaths       []string                 `json:"affected_paths,omitempty"`
	BackupRefs          map[string]string        `json:"backup_refs,omitempty"`
	ConfigTest          CommandResult            `json:"config_test"`
	Reload              CommandResult            `json:"reload"`
	Verify              verifynginx.VerifyResult `json:"verify"`
	Rollback            *RollbackResult          `json:"rollback,omitempty"`
	ExpectedFingerprint string                   `json:"expected_fingerprint,omitempty"`
	ActiveFingerprint   string                   `json:"active_fingerprint,omitempty"`
}

// DeployResult is the complete local execution result.
type DeployResult struct {
	Status            string            `json:"status"`
	ActiveFingerprint string            `json:"active_fingerprint,omitempty"`
	BackupRefs        map[string]string `json:"backup_refs,omitempty"`
	Steps             []StepResult      `json:"steps,omitempty"`
	Rollback          *RollbackResult   `json:"rollback,omitempty"`
	Evidence          Evidence          `json:"evidence"`
}

// FileStore abstracts local file mutations.
type FileStore interface {
	Read(context.Context, string) ([]byte, error)
	WriteAtomic(context.Context, string, []byte, fs.FileMode) error
	Backup(context.Context, string) (string, error)
	Restore(context.Context, string, string) error
	Remove(context.Context, string) error
}

// CommandRunner executes local commands such as nginx -t and reload.
type CommandRunner interface {
	Run(context.Context, string, ...string) (CommandResult, error)
}

// Verifier checks the post-reload active certificate.
type Verifier interface {
	Verify(context.Context, verifynginx.VerifyRequest) (verifynginx.VerifyResult, error)
}

// PathPolicy validates whether one path is inside the job's write boundary.
type PathPolicy interface {
	Validate(path string, allowedPaths []string) error
}
