package acme

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/zaneway/AutoCertX/internal/domain/resource"
)

const (
	AuthorizationPending = "pending"
	AuthorizationValid   = "valid"
	AuthorizationInvalid = "invalid"
)

// DomainIdentifier describes one requested identifier in ACME order creation.
type DomainIdentifier struct {
	DomainAssetID string
	Name          string
}

// OrderRequest is the adapter input for creating one order.
type OrderRequest struct {
	RequestID         string
	CommonName        string
	CertificateType   string
	ChallengeType     string
	DomainIdentifiers []DomainIdentifier
}

// OrderChallenge is the adapter view of one challenge material set.
type OrderChallenge struct {
	DomainAssetID    string
	Identifier       string
	ChallengeType    string
	Token            string
	KeyAuthorization string
	HTTPPath         string
	DNSRecordName    string
	DNSRecordValue   string
}

// Order contains the ACME order facts needed by the workflow layer.
type Order struct {
	OrderURL    string
	FinalizeURL string
	Challenges  []OrderChallenge
}

// ChallengeReadyInput notifies ACME that one challenge can be verified.
type ChallengeReadyInput struct {
	OrderURL      string
	Identifier    string
	ChallengeType string
}

// AuthorizationInput polls one authorization state.
type AuthorizationInput struct {
	OrderURL      string
	Identifier    string
	ChallengeType string
}

// AuthorizationResult returns the authorization status for one challenge.
type AuthorizationResult struct {
	Status string
}

// FinalizeInput finalizes one order.
type FinalizeInput struct {
	OrderURL string
}

// CertificateBundle is the issued certificate material reference.
type CertificateBundle struct {
	CertificateRef string
}

// Client is the ACME adapter boundary used by T08 orchestration.
type Client interface {
	CreateOrder(context.Context, OrderRequest) (Order, error)
	NotifyChallengeReady(context.Context, ChallengeReadyInput) error
	PollAuthorization(context.Context, AuthorizationInput) (AuthorizationResult, error)
	FinalizeOrder(context.Context, FinalizeInput) (CertificateBundle, error)
	DownloadCertificate(context.Context, FinalizeInput) (CertificateBundle, error)
}

// TemporaryError marks a retryable upstream failure.
type TemporaryError struct {
	Err error
}

func (e TemporaryError) Error() string {
	if e.Err == nil {
		return "temporary acme failure"
	}
	return e.Err.Error()
}

func (e TemporaryError) Unwrap() error {
	return e.Err
}

// IsTemporary reports whether an error should be retried by the scheduler.
func IsTemporary(err error) bool {
	var target TemporaryError
	return err != nil && (strings.Contains(strings.ToLower(err.Error()), "temporary") || errors.As(err, &target))
}

// FakeClient provides a deterministic ACME adapter for Phase A tests.
type FakeClient struct {
	mu                 sync.Mutex
	nextOrderNo        int
	PendingPolls       int
	CreateOrderErr     error
	NotifyReadyErr     error
	FinalizeOrderErr   error
	DownloadErr        error
	InvalidIdentifiers map[string]bool
	pollCounts         map[string]int
	issuedByOrder      map[string]string
}

// NewFakeClient constructs an empty fake ACME client.
func NewFakeClient() *FakeClient {
	return &FakeClient{
		InvalidIdentifiers: make(map[string]bool),
		pollCounts:         make(map[string]int),
		issuedByOrder:      make(map[string]string),
	}
}

// CreateOrder creates a fake order and returns one challenge per domain.
func (c *FakeClient) CreateOrder(_ context.Context, input OrderRequest) (Order, error) {
	if input.RequestID == "" {
		return Order{}, fmt.Errorf("request_id required: %w", resource.ErrValidation)
	}
	if len(input.DomainIdentifiers) == 0 {
		return Order{}, fmt.Errorf("at least one identifier required: %w", resource.ErrValidation)
	}
	if c.CreateOrderErr != nil {
		return Order{}, c.CreateOrderErr
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.nextOrderNo++
	orderURL := fmt.Sprintf("acme://orders/%d", c.nextOrderNo)
	finalizeURL := orderURL + "/finalize"
	challenges := make([]OrderChallenge, 0, len(input.DomainIdentifiers))
	for idx, identifier := range input.DomainIdentifiers {
		name := strings.ToLower(strings.TrimSpace(identifier.Name))
		if name == "" {
			return Order{}, fmt.Errorf("identifier required: %w", resource.ErrValidation)
		}
		token := fmt.Sprintf("token-%d-%d", c.nextOrderNo, idx+1)
		challenges = append(challenges, OrderChallenge{
			DomainAssetID:    strings.TrimSpace(identifier.DomainAssetID),
			Identifier:       name,
			ChallengeType:    strings.ToLower(strings.TrimSpace(input.ChallengeType)),
			Token:            token,
			KeyAuthorization: token + ".keyauth",
			HTTPPath:         "/.well-known/acme-challenge/" + token,
			DNSRecordName:    "_acme-challenge." + strings.TrimPrefix(name, "*."),
			DNSRecordValue:   "txt-" + token,
		})
	}

	return Order{
		OrderURL:    orderURL,
		FinalizeURL: finalizeURL,
		Challenges:  challenges,
	}, nil
}

// NotifyChallengeReady records the fact that verification may begin.
func (c *FakeClient) NotifyChallengeReady(_ context.Context, _ ChallengeReadyInput) error {
	if c.NotifyReadyErr != nil {
		return c.NotifyReadyErr
	}
	return nil
}

// PollAuthorization returns pending, valid, or invalid according to the fake configuration.
func (c *FakeClient) PollAuthorization(_ context.Context, input AuthorizationInput) (AuthorizationResult, error) {
	key := input.OrderURL + "|" + input.Identifier + "|" + input.ChallengeType

	c.mu.Lock()
	defer c.mu.Unlock()

	c.pollCounts[key]++
	if c.InvalidIdentifiers[strings.ToLower(strings.TrimSpace(input.Identifier))] {
		return AuthorizationResult{Status: AuthorizationInvalid}, nil
	}
	if c.pollCounts[key] <= c.PendingPolls {
		return AuthorizationResult{Status: AuthorizationPending}, nil
	}
	return AuthorizationResult{Status: AuthorizationValid}, nil
}

// FinalizeOrder marks the order as issued and returns a material reference.
func (c *FakeClient) FinalizeOrder(_ context.Context, input FinalizeInput) (CertificateBundle, error) {
	if c.FinalizeOrderErr != nil {
		return CertificateBundle{}, c.FinalizeOrderErr
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	ref := input.OrderURL + "/certificate"
	c.issuedByOrder[input.OrderURL] = ref
	return CertificateBundle{CertificateRef: ref}, nil
}

// DownloadCertificate returns the issued material reference for one order.
func (c *FakeClient) DownloadCertificate(_ context.Context, input FinalizeInput) (CertificateBundle, error) {
	if c.DownloadErr != nil {
		return CertificateBundle{}, c.DownloadErr
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	ref, ok := c.issuedByOrder[input.OrderURL]
	if !ok {
		return CertificateBundle{}, fmt.Errorf("order %s not finalized: %w", input.OrderURL, resource.ErrUnavailable)
	}
	return CertificateBundle{CertificateRef: ref}, nil
}
