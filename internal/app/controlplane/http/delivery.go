package httpserver

import (
	"net/http"

	nodescmd "github.com/zaneway/AutoCertX/internal/application/command/nodes"
	targetscmd "github.com/zaneway/AutoCertX/internal/application/command/targets"
	"github.com/zaneway/AutoCertX/internal/domain/tenancy"
)

// deliveryHandler serves deployment target and Agent node governance APIs.
type deliveryHandler struct {
	nodes   *nodescmd.Service
	targets *targetscmd.Service
}

type deploymentTargetUpsertRequest struct {
	Name            string            `json:"name"`
	TargetType      string            `json:"target_type"`
	NodeID          string            `json:"node_id"`
	AgentSelector   map[string]string `json:"agent_selector"`
	ConfigPath      string            `json:"config_path"`
	CertificatePath string            `json:"certificate_path"`
	PrivateKeyPath  string            `json:"private_key_path"`
	KeystorePath    string            `json:"keystore_path"`
}

type nodeLabelUpdateRequest struct {
	Labels map[string]string `json:"labels"`
}

func registerDeliveryRoutes(mux *http.ServeMux, deps Deps) {
	if deps.NodeCommands == nil && deps.TargetCommands == nil {
		return
	}

	handler := deliveryHandler{
		nodes:   deps.NodeCommands,
		targets: deps.TargetCommands,
	}

	handleRead := func(pattern string, fn http.HandlerFunc) {
		var endpoint http.Handler = fn
		if deps.AuthService != nil && deps.AuthContextQuery != nil {
			authz := authHandler{
				authService:        deps.AuthService,
				authContextService: deps.AuthContextQuery,
			}
			endpoint = authz.withAuthentication(authz.withPermissions(endpoint, tenancy.PermissionAuthContextRead))
		}
		mux.Handle(pattern, endpoint)
	}

	if deps.TargetCommands != nil {
		handleRead("GET /api/v1/deployment-targets", handler.listDeploymentTargets)
		handleRead("GET /api/v1/deployment-targets/{id}", handler.getDeploymentTarget)
		mux.HandleFunc("POST /api/v1/deployment-targets", handler.createDeploymentTarget)
		mux.HandleFunc("PUT /api/v1/deployment-targets/{id}", handler.updateDeploymentTarget)
	}

	if deps.NodeCommands != nil {
		handleRead("GET /api/v1/nodes", handler.listNodes)
		handleRead("GET /api/v1/nodes/{id}", handler.getNode)
		mux.HandleFunc("POST /api/v1/nodes/registration-tokens", handler.createNodeRegistrationToken)
		mux.HandleFunc("POST /api/v1/nodes/{id}/disable", handler.disableNode)
		mux.HandleFunc("PUT /api/v1/nodes/{id}/labels", handler.updateNodeLabels)
	}
}

func (h deliveryHandler) listDeploymentTargets(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveReadScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	items, err := h.targets.ListTargets(r.Context(), scope)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeListEnvelope(w, r, http.StatusOK, items)
}

func (h deliveryHandler) createDeploymentTarget(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveGovernanceScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	var req deploymentTargetUpsertRequest
	if err := decodeJSON(r, &req); err != nil {
		writeValidationError(w, r, "body", "invalid_json")
		return
	}

	target, err := h.targets.CreateTarget(r.Context(), scope, targetInput(req))
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeObjectEnvelope(w, r, http.StatusCreated, target)
}

func (h deliveryHandler) getDeploymentTarget(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveReadScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	id := r.PathValue("id")
	if err := validateGovernanceID(id); err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	target, err := h.targets.GetTarget(r.Context(), scope, id)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeObjectEnvelope(w, r, http.StatusOK, target)
}

func (h deliveryHandler) updateDeploymentTarget(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveGovernanceScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	id := r.PathValue("id")
	if err := validateGovernanceID(id); err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	var req deploymentTargetUpsertRequest
	if err := decodeJSON(r, &req); err != nil {
		writeValidationError(w, r, "body", "invalid_json")
		return
	}

	target, err := h.targets.UpdateTarget(r.Context(), scope, id, targetInput(req))
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeObjectEnvelope(w, r, http.StatusOK, target)
}

func (h deliveryHandler) listNodes(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveReadScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	items, err := h.nodes.ListNodes(r.Context(), scope)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeListEnvelope(w, r, http.StatusOK, items)
}

func (h deliveryHandler) getNode(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveReadScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	id := r.PathValue("id")
	if err := validateGovernanceID(id); err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	node, err := h.nodes.GetNode(r.Context(), scope, id)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeObjectEnvelope(w, r, http.StatusOK, node)
}

func (h deliveryHandler) createNodeRegistrationToken(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveGovernanceScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	token, err := h.nodes.CreateRegistrationToken(r.Context(), scope)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeObjectEnvelope(w, r, http.StatusCreated, token)
}

func (h deliveryHandler) disableNode(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveGovernanceScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	id := r.PathValue("id")
	if err := validateGovernanceID(id); err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	node, err := h.nodes.DisableNode(r.Context(), scope, id)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeObjectEnvelope(w, r, http.StatusAccepted, node)
}

func (h deliveryHandler) updateNodeLabels(w http.ResponseWriter, r *http.Request) {
	scope, _, err := resolveGovernanceScope(r)
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	id := r.PathValue("id")
	if err := validateGovernanceID(id); err != nil {
		writeGovernanceError(w, r, err)
		return
	}

	var req nodeLabelUpdateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeValidationError(w, r, "body", "invalid_json")
		return
	}

	node, err := h.nodes.UpdateLabels(r.Context(), scope, id, nodescmd.LabelUpdateInput{
		Labels: req.Labels,
	})
	if err != nil {
		writeGovernanceError(w, r, err)
		return
	}
	writeObjectEnvelope(w, r, http.StatusOK, node)
}

func targetInput(req deploymentTargetUpsertRequest) targetscmd.UpsertInput {
	return targetscmd.UpsertInput{
		Name:            req.Name,
		TargetType:      req.TargetType,
		AgentID:         req.NodeID,
		AgentSelector:   req.AgentSelector,
		ConfigPath:      req.ConfigPath,
		CertificatePath: req.CertificatePath,
		PrivateKeyPath:  req.PrivateKeyPath,
		KeystorePath:    req.KeystorePath,
	}
}
