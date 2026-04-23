package agentnode

import (
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/zaneway/AutoCertX/internal/domain/resource"
	"github.com/zaneway/AutoCertX/internal/platform/uuidx"
)

const (
	StatusRegistering  = "registering"
	StatusOnline       = "online"
	StatusDegraded     = "degraded"
	StatusOffline      = "offline"
	StatusDisabled     = "disabled"
	StatusIncompatible = "incompatible"

	CapabilityKeygenRSA             = "keygen:rsa"
	CapabilityChallengeHTTP01       = "challenge:http-01"
	CapabilityDeployNGINX           = "deploy:nginx"
	CapabilityDeployTomcatPKCS12    = "deploy:tomcat-jsse-pkcs12"
	CapabilityVerifyNGINX           = "verify:nginx"
	CapabilityVerifyTomcat          = "verify:tomcat"
	CapabilityDiscoverNGINX         = "discover:nginx"
	CapabilityDiscoverTomcat        = "discover:tomcat"
	MinimumSupportedProtocolVersion = 1
)

// Node represents one customer-side execution node.
type Node struct {
	ID              string            `json:"id"`
	Scope           resource.Scope    `json:"-"`
	Name            string            `json:"name"`
	Hostname        string            `json:"hostname"`
	IPAddress       string            `json:"ip_address"`
	Version         string            `json:"version"`
	ProtocolVersion int               `json:"protocol_version"`
	OS              string            `json:"os"`
	Arch            string            `json:"arch"`
	Labels          map[string]string `json:"labels"`
	Capabilities    []string          `json:"capabilities"`
	Status          string            `json:"status"`
	LastSeenAt      *time.Time        `json:"last_seen_at,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// RegistrationInput contains the first registration facts reported by an Agent.
type RegistrationInput struct {
	Name            string
	Hostname        string
	IPAddress       string
	Version         string
	ProtocolVersion int
	OS              string
	Arch            string
	Labels          map[string]string
	Capabilities    []string
}

// HeartbeatInput contains runtime facts reported by a registered Agent.
type HeartbeatInput struct {
	Version         string
	ProtocolVersion int
	Status          string
	Capabilities    []string
}

// LabelUpdateInput replaces the operator-managed label set for a node.
type LabelUpdateInput struct {
	Labels map[string]string
}

// Service manages Agent node governance state.
type Service struct {
	mu        sync.RWMutex
	now       func() time.Time
	newID     func() string
	byID      map[string]Node
	byEnvName map[string]string
}

// NewService constructs an in-memory Agent node service.
func NewService() *Service {
	return &Service{
		now: func() time.Time {
			return time.Now().UTC()
		},
		newID:     uuidx.New,
		byID:      make(map[string]Node),
		byEnvName: make(map[string]string),
	}
}

// List returns all nodes under the given scope.
func (s *Service) List(scope resource.Scope) ([]Node, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]Node, 0)
	for _, node := range s.byID {
		if !node.Scope.Equals(scope) {
			continue
		}
		items = append(items, cloneNode(node))
	}

	slices.SortFunc(items, func(a Node, b Node) int {
		return strings.Compare(a.Name, b.Name)
	})

	return items, nil
}

// Get returns one node scoped to the caller boundary.
func (s *Service) Get(scope resource.Scope, id string) (Node, error) {
	if err := scope.Validate(); err != nil {
		return Node{}, err
	}
	if strings.TrimSpace(id) == "" {
		return Node{}, fmt.Errorf("node id required: %w", resource.ErrValidation)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	node, ok := s.byID[id]
	if !ok {
		return Node{}, fmt.Errorf("node %s: %w", id, resource.ErrNotFound)
	}
	if !node.Scope.Equals(scope) {
		return Node{}, fmt.Errorf("node %s: %w", id, resource.ErrScopeMismatch)
	}

	return cloneNode(node), nil
}

// Register creates a node from a registration token exchange.
func (s *Service) Register(scope resource.Scope, input RegistrationInput) (Node, error) {
	if err := scope.Validate(); err != nil {
		return Node{}, err
	}
	if err := validateRegistration(input); err != nil {
		return Node{}, err
	}

	now := s.now()
	node := Node{
		ID:              s.newID(),
		Scope:           scope,
		Name:            strings.TrimSpace(input.Name),
		Hostname:        strings.TrimSpace(input.Hostname),
		IPAddress:       strings.TrimSpace(input.IPAddress),
		Version:         strings.TrimSpace(input.Version),
		ProtocolVersion: input.ProtocolVersion,
		OS:              normalizeToken(input.OS),
		Arch:            normalizeToken(input.Arch),
		Labels:          cloneStringMap(input.Labels),
		Capabilities:    normalizeCapabilities(input.Capabilities),
		Status:          StatusRegistering,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	envKey := scope.EnvironmentKey() + "/" + node.Name

	s.mu.Lock()
	defer s.mu.Unlock()

	if existingID, exists := s.byEnvName[envKey]; exists {
		return Node{}, fmt.Errorf("node %q already exists under environment (%s): %w", node.Name, existingID, resource.ErrConflict)
	}

	s.byID[node.ID] = node
	s.byEnvName[envKey] = node.ID
	return cloneNode(node), nil
}

// Heartbeat updates runtime liveness and capability facts for one node.
func (s *Service) Heartbeat(scope resource.Scope, id string, input HeartbeatInput) (Node, error) {
	if err := scope.Validate(); err != nil {
		return Node{}, err
	}
	if err := validateHeartbeat(input); err != nil {
		return Node{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	node, ok := s.byID[id]
	if !ok {
		return Node{}, fmt.Errorf("node %s: %w", id, resource.ErrNotFound)
	}
	if !node.Scope.Equals(scope) {
		return Node{}, fmt.Errorf("node %s: %w", id, resource.ErrScopeMismatch)
	}
	if node.Status == StatusDisabled {
		return Node{}, fmt.Errorf("node %s disabled: %w", id, resource.ErrUnavailable)
	}

	now := s.now()
	node.Version = strings.TrimSpace(input.Version)
	node.ProtocolVersion = input.ProtocolVersion
	node.Status = normalizeNodeStatus(input.Status)
	node.Capabilities = normalizeCapabilities(input.Capabilities)
	node.LastSeenAt = &now
	node.UpdatedAt = now

	s.byID[node.ID] = node
	return cloneNode(node), nil
}

// UpdateLabels replaces one node's operator-managed labels.
func (s *Service) UpdateLabels(scope resource.Scope, id string, input LabelUpdateInput) (Node, error) {
	if err := scope.Validate(); err != nil {
		return Node{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	node, ok := s.byID[id]
	if !ok {
		return Node{}, fmt.Errorf("node %s: %w", id, resource.ErrNotFound)
	}
	if !node.Scope.Equals(scope) {
		return Node{}, fmt.Errorf("node %s: %w", id, resource.ErrScopeMismatch)
	}

	node.Labels = cloneStringMap(input.Labels)
	node.UpdatedAt = s.now()

	s.byID[node.ID] = node
	return cloneNode(node), nil
}

// Disable prevents a node from receiving new work.
func (s *Service) Disable(scope resource.Scope, id string) (Node, error) {
	if err := scope.Validate(); err != nil {
		return Node{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	node, ok := s.byID[id]
	if !ok {
		return Node{}, fmt.Errorf("node %s: %w", id, resource.ErrNotFound)
	}
	if !node.Scope.Equals(scope) {
		return Node{}, fmt.Errorf("node %s: %w", id, resource.ErrScopeMismatch)
	}

	node.Status = StatusDisabled
	node.UpdatedAt = s.now()

	s.byID[node.ID] = node
	return cloneNode(node), nil
}

// MatchCapable returns schedulable nodes matching all required capabilities and labels.
func (s *Service) MatchCapable(scope resource.Scope, requiredCapabilities []string, requiredLabels map[string]string) ([]Node, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	normalizedCapabilities := normalizeCapabilities(requiredCapabilities)
	if len(requiredCapabilities) != len(normalizedCapabilities) {
		return nil, fmt.Errorf("required capabilities invalid: %w", resource.ErrValidation)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]Node, 0)
	for _, node := range s.byID {
		if !node.Scope.Equals(scope) || !isSchedulable(node.Status) {
			continue
		}
		if !containsAll(node.Capabilities, normalizedCapabilities) {
			continue
		}
		if !labelsMatch(node.Labels, requiredLabels) {
			continue
		}
		items = append(items, cloneNode(node))
	}

	slices.SortFunc(items, func(a Node, b Node) int {
		return strings.Compare(a.Name, b.Name)
	})

	return items, nil
}

func validateRegistration(input RegistrationInput) error {
	if strings.TrimSpace(input.Name) == "" {
		return fmt.Errorf("node_name required: %w", resource.ErrValidation)
	}
	if strings.TrimSpace(input.Hostname) == "" {
		return fmt.Errorf("hostname required: %w", resource.ErrValidation)
	}
	if _, err := netip.ParseAddr(strings.TrimSpace(input.IPAddress)); err != nil {
		return fmt.Errorf("ip_address invalid: %w", resource.ErrValidation)
	}
	if strings.TrimSpace(input.Version) == "" {
		return fmt.Errorf("version required: %w", resource.ErrValidation)
	}
	if input.ProtocolVersion < MinimumSupportedProtocolVersion {
		return fmt.Errorf("protocol_version unsupported: %w", resource.ErrValidation)
	}
	if strings.TrimSpace(input.OS) == "" {
		return fmt.Errorf("os required: %w", resource.ErrValidation)
	}
	if strings.TrimSpace(input.Arch) == "" {
		return fmt.Errorf("arch required: %w", resource.ErrValidation)
	}
	capabilities := normalizeCapabilities(input.Capabilities)
	if len(capabilities) == 0 || len(capabilities) != len(input.Capabilities) {
		return fmt.Errorf("capabilities invalid: %w", resource.ErrValidation)
	}
	return nil
}

func validateHeartbeat(input HeartbeatInput) error {
	if input.ProtocolVersion < MinimumSupportedProtocolVersion {
		return fmt.Errorf("protocol_version unsupported: %w", resource.ErrValidation)
	}
	if !validHeartbeatStatus(input.Status) {
		return fmt.Errorf("status invalid: %w", resource.ErrValidation)
	}
	capabilities := normalizeCapabilities(input.Capabilities)
	if len(capabilities) == 0 || len(capabilities) != len(input.Capabilities) {
		return fmt.Errorf("capabilities invalid: %w", resource.ErrValidation)
	}
	return nil
}

func normalizeNodeStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}

func validHeartbeatStatus(status string) bool {
	switch normalizeNodeStatus(status) {
	case StatusOnline, StatusDegraded, StatusIncompatible:
		return true
	default:
		return false
	}
}

func normalizeCapabilities(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		code := strings.ToLower(strings.TrimSpace(item))
		if !validCapability(code) {
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		result = append(result, code)
	}
	slices.Sort(result)
	return result
}

func validCapability(code string) bool {
	switch code {
	case CapabilityKeygenRSA,
		CapabilityChallengeHTTP01,
		CapabilityDeployNGINX,
		CapabilityDeployTomcatPKCS12,
		CapabilityVerifyNGINX,
		CapabilityVerifyTomcat,
		CapabilityDiscoverNGINX,
		CapabilityDiscoverTomcat:
		return true
	default:
		return false
	}
}

func isSchedulable(status string) bool {
	switch status {
	case StatusOnline, StatusDegraded:
		return true
	default:
		return false
	}
}

func containsAll(actual []string, required []string) bool {
	if len(required) == 0 {
		return true
	}
	set := make(map[string]struct{}, len(actual))
	for _, item := range actual {
		set[item] = struct{}{}
	}
	for _, item := range required {
		if _, ok := set[item]; !ok {
			return false
		}
	}
	return true
}

func labelsMatch(actual map[string]string, required map[string]string) bool {
	for key, value := range required {
		if actual[key] != value {
			return false
		}
	}
	return true
}

func normalizeToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func cloneNode(node Node) Node {
	node.Labels = cloneStringMap(node.Labels)
	node.Capabilities = slices.Clone(node.Capabilities)
	return node
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		normalizedKey := strings.TrimSpace(key)
		if normalizedKey == "" {
			continue
		}
		clone[normalizedKey] = strings.TrimSpace(value)
	}
	return clone
}
