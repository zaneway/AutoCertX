package nginx

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io/fs"
	"math/big"
	"testing"
	"time"

	verifynginx "github.com/zaneway/AutoCertX/internal/agent/verify/nginx"
)

func TestDeployNginxSuccess(t *testing.T) {
	req, store, service := newDeployFixture(t)

	result, err := service.Deploy(t.Context(), req)
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	if result.Status != StatusSucceeded {
		t.Fatalf("status = %q, want %q", result.Status, StatusSucceeded)
	}
	if !bytes.Equal(store.files[req.CertificatePath], req.CertificatePEM) {
		t.Fatal("certificate file was not replaced")
	}
	if !bytes.Equal(store.files[req.PrivateKeyPath], req.PrivateKeyPEM) {
		t.Fatal("private key file was not replaced")
	}
	if store.modes[req.PrivateKeyPath] != 0o600 {
		t.Fatalf("private key mode = %v, want 0600", store.modes[req.PrivateKeyPath])
	}
	if len(result.BackupRefs) != 2 {
		t.Fatalf("backup ref count = %d, want 2", len(result.BackupRefs))
	}
	if result.ActiveFingerprint == "" {
		t.Fatal("active fingerprint should be recorded")
	}
}

func TestDeployNginxRejectsPathOutsideAllowList(t *testing.T) {
	req, _, service := newDeployFixture(t)
	req.AllowedPaths = []string{"/opt/app"}

	result, err := service.Deploy(t.Context(), req)
	if err == nil {
		t.Fatal("Deploy() error should not be nil")
	}
	if result.Status != StatusFailed {
		t.Fatalf("status = %q, want %q", result.Status, StatusFailed)
	}
	if len(result.BackupRefs) != 0 {
		t.Fatalf("backup refs = %+v, want empty", result.BackupRefs)
	}
}

func TestDeployNginxRollsBackAfterInstallFailure(t *testing.T) {
	req, store, service := newDeployFixture(t)
	oldCert := append([]byte(nil), store.files[req.CertificatePath]...)
	store.failWritePath = req.PrivateKeyPath

	result, err := service.Deploy(t.Context(), req)
	if err == nil {
		t.Fatal("Deploy() error should not be nil")
	}
	if result.Status != StatusRolledBack {
		t.Fatalf("status = %q, want %q", result.Status, StatusRolledBack)
	}
	if !bytes.Equal(store.files[req.CertificatePath], oldCert) {
		t.Fatal("certificate file should be restored after install failure")
	}
	if result.Rollback == nil || result.Rollback.Status != StatusRolledBack {
		t.Fatalf("rollback = %+v, want rolled_back", result.Rollback)
	}
}

func TestDeployNginxRollsBackAfterReloadFailure(t *testing.T) {
	req, _, service := newDeployFixture(t)
	service.Runner.(*fakeRunner).sequences["nginx-reload"] = []CommandResult{
		{ExitCode: 1, Stderr: "reload failed"},
		{ExitCode: 0, Stdout: "rollback reload ok"},
	}

	result, err := service.Deploy(t.Context(), req)
	if err == nil {
		t.Fatal("Deploy() error should not be nil")
	}
	if result.Status != StatusRolledBack {
		t.Fatalf("status = %q, want %q", result.Status, StatusRolledBack)
	}
	if result.Rollback == nil || result.Rollback.Reload.ExitCode != 0 {
		t.Fatalf("rollback reload = %+v, want successful reload", result.Rollback)
	}
}

func TestDeployNginxRollsBackAfterVerifyFailure(t *testing.T) {
	req, _, service := newDeployFixture(t)
	service.Verifier = fakeVerifier{err: fmt.Errorf("active certificate fingerprint mismatch")}

	result, err := service.Deploy(t.Context(), req)
	if err == nil {
		t.Fatal("Deploy() error should not be nil")
	}
	if result.Status != StatusRolledBack {
		t.Fatalf("status = %q, want %q", result.Status, StatusRolledBack)
	}
}

func TestDeployNginxReportsRollbackFailure(t *testing.T) {
	req, store, service := newDeployFixture(t)
	service.Runner.(*fakeRunner).results["nginx-reload"] = CommandResult{ExitCode: 1, Stderr: "reload failed"}
	store.failRestore = true

	result, err := service.Deploy(t.Context(), req)
	if err == nil {
		t.Fatal("Deploy() error should not be nil")
	}
	if result.Status != StatusRollbackFailed {
		t.Fatalf("status = %q, want %q", result.Status, StatusRollbackFailed)
	}
}

func TestDeployNginxRejectsCertificateKeyMismatch(t *testing.T) {
	req, _, service := newDeployFixture(t)
	_, otherKeyPEM, _ := newCertificateMaterial(t)
	req.PrivateKeyPEM = otherKeyPEM

	result, err := service.Deploy(t.Context(), req)
	if err == nil {
		t.Fatal("Deploy() error should not be nil")
	}
	if result.Status != StatusFailed {
		t.Fatalf("status = %q, want %q", result.Status, StatusFailed)
	}
}

func TestIntegrationNginxDeployConnectorWithFakes(t *testing.T) {
	req, _, service := newDeployFixture(t)

	result, err := service.Deploy(t.Context(), req)
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	if result.Evidence.ConfigTest.ExitCode != 0 {
		t.Fatalf("config test = %+v, want success", result.Evidence.ConfigTest)
	}
	if result.Evidence.Reload.ExitCode != 0 {
		t.Fatalf("reload = %+v, want success", result.Evidence.Reload)
	}
	if !result.Evidence.Verify.Matched {
		t.Fatalf("verify = %+v, want matched", result.Evidence.Verify)
	}
}

func newDeployFixture(t *testing.T) (DeployRequest, *memoryFileStore, Service) {
	t.Helper()

	certPEM, keyPEM, fingerprint := newCertificateMaterial(t)
	store := &memoryFileStore{
		files: map[string][]byte{
			"/etc/nginx/certs/site.pem": []byte("old-cert"),
			"/etc/nginx/certs/site.key": []byte("old-key"),
			"/etc/nginx/nginx.conf":     []byte("events {}\nhttp {}\n"),
		},
		modes:   map[string]fs.FileMode{},
		backups: map[string][]byte{},
	}
	req := DeployRequest{
		TargetID:            "target-nginx-1",
		CertificatePEM:      certPEM,
		PrivateKeyPEM:       keyPEM,
		CertificatePath:     "/etc/nginx/certs/site.pem",
		PrivateKeyPath:      "/etc/nginx/certs/site.key",
		ConfigPath:          "/etc/nginx/nginx.conf",
		NGINXTestCommand:    Command{Name: "nginx-test", Args: []string{"-t", "-c", "/etc/nginx/nginx.conf"}},
		ReloadCommand:       Command{Name: "nginx-reload", Args: []string{"reload"}},
		AllowedPaths:        []string{"/etc/nginx"},
		VerifyHost:          "127.0.0.1",
		VerifyPort:          443,
		ServerName:          "api.example.com",
		ExpectedFingerprint: fingerprint,
	}
	service := Service{
		Files: store,
		Runner: &fakeRunner{
			results: map[string]CommandResult{
				"nginx-test":   {ExitCode: 0, Stdout: "syntax is ok"},
				"nginx-reload": {ExitCode: 0, Stdout: "reload ok"},
			},
			sequences: map[string][]CommandResult{},
		},
		Verifier: fakeVerifier{
			result: verifynginx.VerifyResult{
				Status:              verifynginx.StatusVerified,
				Matched:             true,
				ExpectedFingerprint: fingerprint,
				ActiveFingerprint:   fingerprint,
			},
		},
	}
	return req, store, service
}

type memoryFileStore struct {
	files         map[string][]byte
	modes         map[string]fs.FileMode
	backups       map[string][]byte
	nextBackup    int
	failWritePath string
	failRestore   bool
}

func (s *memoryFileStore) Read(_ context.Context, path string) ([]byte, error) {
	content, ok := s.files[path]
	if !ok {
		return nil, fmt.Errorf("file not found")
	}
	return append([]byte(nil), content...), nil
}

func (s *memoryFileStore) WriteAtomic(_ context.Context, path string, content []byte, mode fs.FileMode) error {
	if path == s.failWritePath {
		return fmt.Errorf("write failed")
	}
	s.files[path] = append([]byte(nil), content...)
	s.modes[path] = mode
	return nil
}

func (s *memoryFileStore) Backup(_ context.Context, path string) (string, error) {
	content, ok := s.files[path]
	if !ok {
		return "", fmt.Errorf("file not found")
	}
	s.nextBackup++
	ref := fmt.Sprintf("backup-%d", s.nextBackup)
	s.backups[ref] = append([]byte(nil), content...)
	return ref, nil
}

func (s *memoryFileStore) Restore(_ context.Context, path string, backupRef string) error {
	if s.failRestore {
		return fmt.Errorf("restore failed")
	}
	content, ok := s.backups[backupRef]
	if !ok {
		return fmt.Errorf("backup not found")
	}
	s.files[path] = append([]byte(nil), content...)
	return nil
}

func (s *memoryFileStore) Remove(_ context.Context, path string) error {
	delete(s.files, path)
	delete(s.modes, path)
	return nil
}

type fakeRunner struct {
	results   map[string]CommandResult
	sequences map[string][]CommandResult
	errs      map[string]error
	calls     []string
}

func (r *fakeRunner) Run(_ context.Context, name string, args ...string) (CommandResult, error) {
	r.calls = append(r.calls, name)
	if r.errs != nil && r.errs[name] != nil {
		return CommandResult{}, r.errs[name]
	}
	if len(r.sequences[name]) > 0 {
		result := r.sequences[name][0]
		r.sequences[name] = r.sequences[name][1:]
		return result, nil
	}
	if result, ok := r.results[name]; ok {
		return result, nil
	}
	return CommandResult{ExitCode: 0}, nil
}

type fakeVerifier struct {
	result verifynginx.VerifyResult
	err    error
}

func (v fakeVerifier) Verify(context.Context, verifynginx.VerifyRequest) (verifynginx.VerifyResult, error) {
	return v.result, v.err
}

func newCertificateMaterial(t *testing.T) ([]byte, []byte, string) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName: "api.example.com",
		},
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter:  time.Now().Add(time.Hour),
		DNSNames:  []string{"api.example.com"},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate() error = %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	fingerprint, err := verifynginx.FingerprintPEM(certPEM)
	if err != nil {
		t.Fatalf("FingerprintPEM() error = %v", err)
	}
	return certPEM, keyPEM, fingerprint
}
