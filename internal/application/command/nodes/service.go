package nodes

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/zaneway/AutoCertX/internal/domain/agentnode"
	"github.com/zaneway/AutoCertX/internal/domain/resource"
	"github.com/zaneway/AutoCertX/internal/platform/apperr"
)

// RegistrationToken represents an operator-created Agent bootstrap token.
type RegistrationToken = agentnode.RegistrationToken

// LabelUpdateInput replaces one node's operator-managed labels.
type LabelUpdateInput struct {
	Labels map[string]string
}

// RegistrationInput contains Agent registration facts after token validation.
type RegistrationInput = agentnode.RegistrationInput

// HeartbeatInput contains Agent heartbeat facts after transport authentication.
type HeartbeatInput = agentnode.HeartbeatInput

// Service orchestrates Agent node governance commands.
type Service struct {
	nodes *agentnode.Service
}

// NewService constructs the Agent node command service.
func NewService(nodeService *agentnode.Service) *Service {
	return &Service{
		nodes: nodeService,
	}
}

// ListNodes returns Agent nodes under the caller scope.
func (s *Service) ListNodes(_ context.Context, scope resource.Scope) ([]agentnode.Node, error) {
	items, err := s.nodes.List(scope)
	if err != nil {
		return nil, translateNodeError(err)
	}
	return items, nil
}

// GetNode returns one Agent node under the caller scope.
func (s *Service) GetNode(_ context.Context, scope resource.Scope, id string) (agentnode.Node, error) {
	node, err := s.nodes.Get(scope, id)
	if err != nil {
		return agentnode.Node{}, translateNodeError(err)
	}
	return node, nil
}

// RegisterNode creates or accepts one node after a registration token exchange.
func (s *Service) RegisterNode(_ context.Context, scope resource.Scope, input RegistrationInput) (agentnode.Node, error) {
	node, err := s.nodes.Register(scope, input)
	if err != nil {
		return agentnode.Node{}, translateNodeError(err)
	}
	return node, nil
}

// LookupNode resolves one node without the caller pre-resolving its scope.
func (s *Service) LookupNode(_ context.Context, id string) (agentnode.Node, error) {
	node, err := s.nodes.Lookup(strings.TrimSpace(id))
	if err != nil {
		return agentnode.Node{}, translateNodeError(err)
	}
	return node, nil
}

// FindNodeByName resolves one node by environment-scoped name.
func (s *Service) FindNodeByName(_ context.Context, scope resource.Scope, name string) (agentnode.Node, error) {
	node, err := s.nodes.FindByName(scope, strings.TrimSpace(name))
	if err != nil {
		return agentnode.Node{}, translateNodeError(err)
	}
	return node, nil
}

// HeartbeatNode updates runtime facts for one node.
func (s *Service) HeartbeatNode(_ context.Context, scope resource.Scope, id string, input HeartbeatInput) (agentnode.Node, error) {
	node, err := s.nodes.Heartbeat(scope, id, input)
	if err != nil {
		return agentnode.Node{}, translateNodeError(err)
	}
	return node, nil
}

// UpdateLabels replaces the operator-managed labels for one node.
func (s *Service) UpdateLabels(_ context.Context, scope resource.Scope, id string, input LabelUpdateInput) (agentnode.Node, error) {
	node, err := s.nodes.UpdateLabels(scope, strings.TrimSpace(id), agentnode.LabelUpdateInput{
		Labels: input.Labels,
	})
	if err != nil {
		return agentnode.Node{}, translateNodeError(err)
	}
	return node, nil
}

// DisableNode prevents one node from receiving new work.
func (s *Service) DisableNode(_ context.Context, scope resource.Scope, id string) (agentnode.Node, error) {
	node, err := s.nodes.Disable(scope, strings.TrimSpace(id))
	if err != nil {
		return agentnode.Node{}, translateNodeError(err)
	}
	return node, nil
}

// CreateRegistrationToken creates a short-lived bootstrap token.
func (s *Service) CreateRegistrationToken(_ context.Context, scope resource.Scope) (RegistrationToken, error) {
	token, err := s.nodes.CreateRegistrationToken(scope)
	if err != nil {
		return RegistrationToken{}, translateNodeError(err)
	}
	return token, nil
}

// ResolveRegistrationToken validates one previously issued bootstrap token.
func (s *Service) ResolveRegistrationToken(_ context.Context, token string) (RegistrationToken, error) {
	record, err := s.nodes.ResolveRegistrationToken(strings.TrimSpace(token))
	if err != nil {
		return RegistrationToken{}, translateNodeError(err)
	}
	return record, nil
}

func translateNodeError(err error) error {
	switch {
	case errors.Is(err, resource.ErrValidation):
		return apperr.Wrap(err, http.StatusBadRequest, "REQUEST_VALIDATION_FAILED", "request validation failed", validationDetail(err))
	case errors.Is(err, resource.ErrNotFound):
		return apperr.Wrap(err, http.StatusNotFound, "RESOURCE_NOT_FOUND", "resource not found", apperr.Detail{})
	case errors.Is(err, resource.ErrConflict):
		return apperr.Wrap(err, http.StatusConflict, "RESOURCE_CONFLICT", "resource conflict", apperr.Detail{})
	case errors.Is(err, resource.ErrScopeMismatch):
		return apperr.Wrap(err, http.StatusConflict, "TENANT_SCOPE_MISMATCH", "tenant scope mismatch", apperr.Detail{})
	case errors.Is(err, resource.ErrUnavailable):
		return apperr.Wrap(err, http.StatusConflict, "RESOURCE_UNAVAILABLE", "resource unavailable", apperr.Detail{})
	default:
		return apperr.Wrap(err, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", apperr.Detail{})
	}
}

func validationDetail(err error) apperr.Detail {
	message := err.Error()
	field := "request"
	switch {
	case strings.Contains(message, "node_name"):
		field = "node_name"
	case strings.Contains(message, "hostname"):
		field = "hostname"
	case strings.Contains(message, "ip_address"):
		field = "ip_address"
	case strings.Contains(message, "version"):
		field = "version"
	case strings.Contains(message, "protocol_version"):
		field = "protocol_version"
	case strings.Contains(message, "status"):
		field = "status"
	case strings.Contains(message, "capabilities"):
		field = "capabilities"
	}
	return apperr.Field(field, message)
}
