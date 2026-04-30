package nginx

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"testing"
	"time"
)

func TestVerifyMatchesFingerprint(t *testing.T) {
	service := Service{Probe: fakeProbe{fingerprint: "aa:bb:cc"}}

	result, err := service.Verify(t.Context(), VerifyRequest{
		Host:                "127.0.0.1",
		Port:                443,
		ServerName:          "api.example.com",
		ExpectedFingerprint: "aabbcc",
	})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if !result.Matched {
		t.Fatal("expected fingerprint match")
	}
	if result.ActiveFingerprint != "aabbcc" {
		t.Fatalf("active fingerprint = %q, want aabbcc", result.ActiveFingerprint)
	}
}

func TestVerifyRejectsFingerprintMismatch(t *testing.T) {
	service := Service{Probe: fakeProbe{fingerprint: "ddeeff"}}

	result, err := service.Verify(t.Context(), VerifyRequest{
		Host:                "127.0.0.1",
		Port:                443,
		ExpectedFingerprint: "aabbcc",
	})
	if err == nil {
		t.Fatal("Verify() error should not be nil")
	}
	if result.Matched {
		t.Fatal("fingerprint should not match")
	}
}

func TestVerifyReturnsProbeFailure(t *testing.T) {
	service := Service{Probe: fakeProbe{err: fmt.Errorf("dial timeout")}}

	if _, err := service.Verify(t.Context(), VerifyRequest{
		Host:                "127.0.0.1",
		Port:                443,
		ExpectedFingerprint: "aabbcc",
	}); err == nil {
		t.Fatal("Verify() error should not be nil")
	}
}

func TestFingerprintPEM(t *testing.T) {
	certPEM := newSelfSignedCertPEM(t)

	fingerprint, err := FingerprintPEM(certPEM)
	if err != nil {
		t.Fatalf("FingerprintPEM() error = %v", err)
	}
	if fingerprint == "" {
		t.Fatal("fingerprint should not be empty")
	}
	if NormalizeFingerprint(fingerprint) != fingerprint {
		t.Fatalf("fingerprint should already be normalized: %q", fingerprint)
	}
}

func TestVerifyRejectsInvalidInput(t *testing.T) {
	service := Service{Probe: fakeProbe{fingerprint: "aabbcc"}}

	if _, err := service.Verify(t.Context(), VerifyRequest{
		Host:                "",
		Port:                443,
		ExpectedFingerprint: "aabbcc",
	}); err == nil {
		t.Fatal("Verify() should reject missing host")
	}
	if _, err := service.Verify(t.Context(), VerifyRequest{
		Host:                "127.0.0.1",
		Port:                0,
		ExpectedFingerprint: "aabbcc",
	}); err == nil {
		t.Fatal("Verify() should reject invalid port")
	}
	if _, err := service.Verify(t.Context(), VerifyRequest{
		Host: "127.0.0.1",
		Port: 443,
	}); err == nil {
		t.Fatal("Verify() should reject missing fingerprint")
	}
}

type fakeProbe struct {
	fingerprint string
	err         error
}

func (p fakeProbe) FetchLeafFingerprint(context.Context, ProbeRequest) (string, error) {
	if p.err != nil {
		return "", p.err
	}
	return p.fingerprint, nil
}

func newSelfSignedCertPEM(t *testing.T) []byte {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "api.example.com",
		},
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter:  time.Now().Add(time.Hour),
		DNSNames:  []string{"api.example.com"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate() error = %v", err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
