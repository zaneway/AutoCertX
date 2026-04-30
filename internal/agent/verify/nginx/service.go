package nginx

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

const StatusVerified = "verified"

// VerifyRequest describes the externally observable certificate state expected
// after an NGINX reload.
type VerifyRequest struct {
	Host                string
	Port                int
	ServerName          string
	ExpectedFingerprint string
}

// VerifyResult captures the observed certificate fingerprint.
type VerifyResult struct {
	Status              string `json:"status"`
	Matched             bool   `json:"matched"`
	HostPort            string `json:"host_port"`
	ServerName          string `json:"server_name"`
	ExpectedFingerprint string `json:"expected_fingerprint"`
	ActiveFingerprint   string `json:"active_fingerprint"`
}

// CertificateProbe fetches the active leaf certificate fingerprint.
type CertificateProbe interface {
	FetchLeafFingerprint(context.Context, ProbeRequest) (string, error)
}

// ProbeRequest is the concrete probe target.
type ProbeRequest struct {
	Host       string
	Port       int
	ServerName string
}

// Service verifies the NGINX runtime certificate after reload.
type Service struct {
	Probe CertificateProbe
}

// Verify confirms the active certificate fingerprint matches the expected one.
func (s Service) Verify(ctx context.Context, req VerifyRequest) (VerifyResult, error) {
	if err := validateRequest(req); err != nil {
		return VerifyResult{}, err
	}
	probe := s.Probe
	if probe == nil {
		probe = TLSSocketProbe{Timeout: 5 * time.Second}
	}

	active, err := probe.FetchLeafFingerprint(ctx, ProbeRequest{
		Host:       strings.TrimSpace(req.Host),
		Port:       req.Port,
		ServerName: strings.TrimSpace(req.ServerName),
	})
	if err != nil {
		return VerifyResult{}, fmt.Errorf("fetch leaf fingerprint: %w", err)
	}

	expected := NormalizeFingerprint(req.ExpectedFingerprint)
	active = NormalizeFingerprint(active)
	result := VerifyResult{
		Status:              StatusVerified,
		Matched:             active == expected,
		HostPort:            net.JoinHostPort(strings.TrimSpace(req.Host), strconv.Itoa(req.Port)),
		ServerName:          strings.TrimSpace(req.ServerName),
		ExpectedFingerprint: expected,
		ActiveFingerprint:   active,
	}
	if !result.Matched {
		return result, fmt.Errorf("active certificate fingerprint mismatch")
	}
	return result, nil
}

// TLSSocketProbe fetches the active certificate through a TLS handshake.
type TLSSocketProbe struct {
	Timeout time.Duration
}

// FetchLeafFingerprint connects to the target and hashes the peer leaf certificate.
func (p TLSSocketProbe) FetchLeafFingerprint(ctx context.Context, req ProbeRequest) (string, error) {
	if err := validateProbeRequest(req); err != nil {
		return "", err
	}
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	dialer := tls.Dialer{
		NetDialer: &net.Dialer{Timeout: timeout},
		Config: &tls.Config{
			// Runtime verification compares the observed leaf fingerprint; CA trust
			// is intentionally not part of this local deployment check.
			InsecureSkipVerify: true,
			ServerName:         strings.TrimSpace(req.ServerName),
		},
	}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(strings.TrimSpace(req.Host), strconv.Itoa(req.Port)))
	if err != nil {
		return "", err
	}
	defer conn.Close()

	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return "", fmt.Errorf("tls connection type invalid")
	}
	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return "", fmt.Errorf("peer certificate missing")
	}
	return FingerprintCertificate(state.PeerCertificates[0]), nil
}

// FingerprintPEM returns the SHA-256 fingerprint for the first certificate PEM block.
func FingerprintPEM(certPEM []byte) (string, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", fmt.Errorf("certificate pem invalid")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("parse certificate: %w", err)
	}
	return FingerprintCertificate(cert), nil
}

// FingerprintCertificate returns a normalized SHA-256 fingerprint.
func FingerprintCertificate(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:])
}

// NormalizeFingerprint makes API comparisons tolerant of colon-delimited input.
func NormalizeFingerprint(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, ":", "")
	normalized = strings.ReplaceAll(normalized, " ", "")
	return normalized
}

func validateRequest(req VerifyRequest) error {
	if strings.TrimSpace(req.Host) == "" {
		return fmt.Errorf("verify host required")
	}
	if req.Port <= 0 || req.Port > 65535 {
		return fmt.Errorf("verify port invalid")
	}
	if strings.TrimSpace(req.ExpectedFingerprint) == "" {
		return fmt.Errorf("expected fingerprint required")
	}
	return nil
}

func validateProbeRequest(req ProbeRequest) error {
	if strings.TrimSpace(req.Host) == "" {
		return fmt.Errorf("probe host required")
	}
	if req.Port <= 0 || req.Port > 65535 {
		return fmt.Errorf("probe port invalid")
	}
	return nil
}
