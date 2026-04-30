package nginx

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"path/filepath"
	"strings"

	verifynginx "github.com/zaneway/AutoCertX/internal/agent/verify/nginx"
)

// Service performs an Agent-local NGINX certificate deployment.
type Service struct {
	Files      FileStore
	Runner     CommandRunner
	Verifier   Verifier
	PathPolicy PathPolicy
}

// Deploy installs certificate material, reloads NGINX, verifies runtime state,
// and rolls back local changes when any post-install stage fails.
func (s Service) Deploy(ctx context.Context, req DeployRequest) (DeployResult, error) {
	result := newResult(req)

	result.Status = StatusPrechecking
	expectedFingerprint, err := s.precheck(ctx, req)
	if err != nil {
		result.Status = StatusFailed
		result.addStep(StatusPrechecking, StepStatusFailed, err.Error(), CommandResult{})
		return result, err
	}
	result.Evidence.ExpectedFingerprint = expectedFingerprint
	result.addStep(StatusPrechecking, StepStatusSucceeded, "request and certificate material validated", CommandResult{})

	result.Status = StatusBackingUp
	if err := s.backup(ctx, req, &result); err != nil {
		result.Status = StatusFailed
		result.addStep(StatusBackingUp, StepStatusFailed, err.Error(), CommandResult{})
		return result, err
	}

	result.Status = StatusInstalling
	if err := s.install(ctx, req, &result); err != nil {
		result.addStep(StatusInstalling, StepStatusFailed, err.Error(), CommandResult{})
		return s.rollback(ctx, req, result, err)
	}

	result.Status = StatusReloading
	if err := s.reload(ctx, req, &result, false); err != nil {
		result.addStep(StatusReloading, StepStatusFailed, err.Error(), CommandResult{})
		return s.rollback(ctx, req, result, err)
	}

	result.Status = StatusVerifying
	verifyResult, err := s.Verifier.Verify(ctx, verifynginx.VerifyRequest{
		Host:                req.VerifyHost,
		Port:                req.VerifyPort,
		ServerName:          req.ServerName,
		ExpectedFingerprint: expectedFingerprint,
	})
	result.Evidence.Verify = verifyResult
	result.ActiveFingerprint = verifyResult.ActiveFingerprint
	result.Evidence.ActiveFingerprint = verifyResult.ActiveFingerprint
	if err != nil {
		result.addStep(StatusVerifying, StepStatusFailed, err.Error(), CommandResult{})
		return s.rollback(ctx, req, result, err)
	}

	result.Status = StatusSucceeded
	result.addStep(StatusVerifying, StepStatusSucceeded, "active certificate fingerprint matched", CommandResult{})
	return result, nil
}

func (s Service) precheck(ctx context.Context, req DeployRequest) (string, error) {
	if s.Files == nil {
		return "", fmt.Errorf("file store required")
	}
	if s.Runner == nil {
		return "", fmt.Errorf("command runner required")
	}
	if s.Verifier == nil {
		return "", fmt.Errorf("verifier required")
	}
	if strings.TrimSpace(req.CertificatePath) == "" {
		return "", fmt.Errorf("certificate_path required")
	}
	if strings.TrimSpace(req.PrivateKeyPath) == "" {
		return "", fmt.Errorf("private_key_path required")
	}
	if strings.TrimSpace(req.ConfigPath) == "" {
		return "", fmt.Errorf("config_path required")
	}
	if strings.TrimSpace(req.NGINXTestCommand.Name) == "" {
		return "", fmt.Errorf("nginx test command required")
	}
	if strings.TrimSpace(req.ReloadCommand.Name) == "" {
		return "", fmt.Errorf("reload command required")
	}
	if strings.TrimSpace(req.VerifyHost) == "" {
		return "", fmt.Errorf("verify_host required")
	}
	if req.VerifyPort <= 0 || req.VerifyPort > 65535 {
		return "", fmt.Errorf("verify_port invalid")
	}
	if len(req.AllowedPaths) == 0 {
		return "", fmt.Errorf("allowed_paths required")
	}

	policy := s.PathPolicy
	if policy == nil {
		policy = DefaultPathPolicy{}
	}
	for _, path := range []string{req.CertificatePath, req.PrivateKeyPath, req.ConfigPath} {
		if err := policy.Validate(path, req.AllowedPaths); err != nil {
			return "", err
		}
	}

	cert, err := parseCertificate(req.CertificatePEM)
	if err != nil {
		return "", err
	}
	keyPublic, err := parsePrivateKeyPublic(req.PrivateKeyPEM)
	if err != nil {
		return "", err
	}
	if !publicKeysEqual(cert.PublicKey, keyPublic) {
		return "", fmt.Errorf("certificate and private key mismatch")
	}
	if _, err := s.Files.Read(ctx, req.ConfigPath); err != nil {
		return "", fmt.Errorf("read config path: %w", err)
	}

	expected := verifynginx.NormalizeFingerprint(req.ExpectedFingerprint)
	if expected == "" {
		expected, err = verifynginx.FingerprintPEM(req.CertificatePEM)
		if err != nil {
			return "", err
		}
	}
	return expected, nil
}

func (s Service) backup(ctx context.Context, req DeployRequest, result *DeployResult) error {
	certRef, err := s.Files.Backup(ctx, req.CertificatePath)
	if err != nil {
		return fmt.Errorf("backup certificate: %w", err)
	}
	keyRef, err := s.Files.Backup(ctx, req.PrivateKeyPath)
	if err != nil {
		return fmt.Errorf("backup private key: %w", err)
	}

	result.BackupRefs[req.CertificatePath] = certRef
	result.BackupRefs[req.PrivateKeyPath] = keyRef
	result.Evidence.BackupRefs[req.CertificatePath] = certRef
	result.Evidence.BackupRefs[req.PrivateKeyPath] = keyRef
	result.addStep(StatusBackingUp, StepStatusSucceeded, "certificate and key backed up", CommandResult{})
	return nil
}

func (s Service) install(ctx context.Context, req DeployRequest, result *DeployResult) error {
	if err := s.Files.WriteAtomic(ctx, req.CertificatePath, req.CertificatePEM, 0o644); err != nil {
		return fmt.Errorf("write certificate: %w", err)
	}
	if err := s.Files.WriteAtomic(ctx, req.PrivateKeyPath, req.PrivateKeyPEM, 0o600); err != nil {
		return fmt.Errorf("write private key: %w", err)
	}

	result.addStep(StatusInstalling, StepStatusSucceeded, "certificate and key installed", CommandResult{})
	return nil
}

func (s Service) reload(ctx context.Context, req DeployRequest, result *DeployResult, rollback bool) error {
	testResult, err := runCommand(ctx, s.Runner, req.NGINXTestCommand)
	if rollback {
		if result.Rollback != nil {
			result.Rollback.ConfigTest = testResult
		}
	} else {
		result.Evidence.ConfigTest = testResult
	}
	if err != nil {
		return fmt.Errorf("nginx config test: %w", err)
	}

	reloadResult, err := runCommand(ctx, s.Runner, req.ReloadCommand)
	if rollback {
		if result.Rollback != nil {
			result.Rollback.Reload = reloadResult
		}
	} else {
		result.Evidence.Reload = reloadResult
	}
	if err != nil {
		return fmt.Errorf("nginx reload: %w", err)
	}

	result.addStep(StatusReloading, StepStatusSucceeded, "nginx config test and reload succeeded", reloadResult)
	return nil
}

func (s Service) rollback(ctx context.Context, req DeployRequest, result DeployResult, cause error) (DeployResult, error) {
	result.Status = StatusRollingBack
	rollback := &RollbackResult{Status: StatusRollingBack}
	result.Rollback = rollback
	result.Evidence.Rollback = rollback

	for _, path := range []string{req.CertificatePath, req.PrivateKeyPath} {
		ref := result.BackupRefs[path]
		if strings.TrimSpace(ref) == "" {
			continue
		}
		if err := s.Files.Restore(ctx, path, ref); err != nil {
			rollback.Status = StatusRollbackFailed
			rollback.Message = fmt.Sprintf("restore %s: %v", path, err)
			result.Status = StatusRollbackFailed
			result.addStep(StatusRollingBack, StepStatusFailed, rollback.Message, CommandResult{})
			return result, cause
		}
		rollback.RestoredPaths = append(rollback.RestoredPaths, path)
	}

	if err := s.reload(ctx, req, &result, true); err != nil {
		rollback.Status = StatusRollbackFailed
		rollback.Message = err.Error()
		result.Status = StatusRollbackFailed
		result.addStep(StatusRollingBack, StepStatusFailed, err.Error(), CommandResult{})
		return result, cause
	}

	rollback.Status = StatusRolledBack
	rollback.Message = "rollback succeeded"
	result.Status = StatusRolledBack
	result.addStep(StatusRollingBack, StepStatusSucceeded, rollback.Message, CommandResult{})
	return result, cause
}

func runCommand(ctx context.Context, runner CommandRunner, cmd Command) (CommandResult, error) {
	result, err := runner.Run(ctx, strings.TrimSpace(cmd.Name), cmd.Args...)
	if err != nil {
		return result, err
	}
	if result.ExitCode != 0 {
		return result, fmt.Errorf("exit code %d", result.ExitCode)
	}
	return result, nil
}

func newResult(req DeployRequest) DeployResult {
	affectedPaths := []string{req.CertificatePath, req.PrivateKeyPath, req.ConfigPath}
	return DeployResult{
		Status:     StatusPending,
		BackupRefs: map[string]string{},
		Evidence: Evidence{
			TargetID:      req.TargetID,
			AffectedPaths: affectedPaths,
			BackupRefs:    map[string]string{},
		},
	}
}

func (r *DeployResult) addStep(stage string, status string, message string, command CommandResult) {
	r.Steps = append(r.Steps, StepResult{
		Stage:    stage,
		Status:   status,
		Message:  message,
		Stdout:   command.Stdout,
		Stderr:   command.Stderr,
		ExitCode: command.ExitCode,
	})
}

// DefaultPathPolicy restricts deployment paths to the Agent job allow-list.
type DefaultPathPolicy struct{}

func (DefaultPathPolicy) Validate(path string, allowedPaths []string) error {
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	if cleanPath == "." || !filepath.IsAbs(cleanPath) {
		return fmt.Errorf("path %q must be absolute", path)
	}
	for _, allowed := range allowedPaths {
		cleanAllowed := filepath.Clean(strings.TrimSpace(allowed))
		if cleanAllowed == "." || !filepath.IsAbs(cleanAllowed) {
			continue
		}
		if cleanPath == cleanAllowed || strings.HasPrefix(cleanPath, cleanAllowed+string(filepath.Separator)) {
			return nil
		}
	}
	return fmt.Errorf("path %q is outside allowed_paths", path)
}

func parseCertificate(certPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("certificate pem invalid")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}
	return cert, nil
}

func parsePrivateKeyPublic(keyPEM []byte) (any, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, fmt.Errorf("private key pem invalid")
	}

	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return &key.PublicKey, nil
	}
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return &key.PublicKey, nil
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	switch typed := key.(type) {
	case *rsa.PrivateKey:
		return &typed.PublicKey, nil
	case *ecdsa.PrivateKey:
		return &typed.PublicKey, nil
	case ed25519.PrivateKey:
		return typed.Public(), nil
	default:
		return nil, fmt.Errorf("private key type unsupported")
	}
}

func publicKeysEqual(a any, b any) bool {
	left, err := x509.MarshalPKIXPublicKey(a)
	if err != nil {
		return false
	}
	right, err := x509.MarshalPKIXPublicKey(b)
	if err != nil {
		return false
	}
	return bytes.Equal(left, right)
}
