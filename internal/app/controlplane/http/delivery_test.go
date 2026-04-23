package httpserver

import (
	"io"
	"log/slog"
	"net/http"
	"testing"

	nodescmd "github.com/zaneway/AutoCertX/internal/application/command/nodes"
	targetscmd "github.com/zaneway/AutoCertX/internal/application/command/targets"
	"github.com/zaneway/AutoCertX/internal/domain/agentnode"
	"github.com/zaneway/AutoCertX/internal/domain/deploymenttarget"
	"github.com/zaneway/AutoCertX/internal/platform/buildinfo"
	"github.com/zaneway/AutoCertX/internal/platform/config"
)

func TestDeliveryAPIHappyPath(t *testing.T) {
	handler, nodeID := newDeliveryRouter(t)

	tokenResp := performJSONRequest(t, handler, http.MethodPost, "/api/v1/nodes/registration-tokens", "", nil)
	if tokenResp.Code != http.StatusCreated {
		t.Fatalf("create registration token status = %d, want %d", tokenResp.Code, http.StatusCreated)
	}
	var tokenEnvelope objectEnvelope
	decodeBody(t, tokenResp.Body, &tokenEnvelope)
	tokenData := tokenEnvelope.Data.(map[string]any)
	if tokenData["token"] == "" {
		t.Fatal("registration token should be returned")
	}

	listNodesResp := performJSONRequest(t, handler, http.MethodGet, "/api/v1/nodes", "", nil)
	if listNodesResp.Code != http.StatusOK {
		t.Fatalf("list nodes status = %d, want %d", listNodesResp.Code, http.StatusOK)
	}
	var nodeList listEnvelope
	decodeBody(t, listNodesResp.Body, &nodeList)
	if len(nodeList.Items.([]any)) != 1 {
		t.Fatalf("node list size = %d, want %d", len(nodeList.Items.([]any)), 1)
	}

	labelResp := performJSONRequest(t, handler, http.MethodPut, "/api/v1/nodes/"+nodeID+"/labels", `{"labels":{"zone":"dmz","runtime":"nginx"}}`, nil)
	if labelResp.Code != http.StatusOK {
		t.Fatalf("update labels status = %d, want %d", labelResp.Code, http.StatusOK)
	}

	targetBody := `{"name":"edge-nginx","target_type":"nginx","node_id":"` + nodeID + `","config_path":"/etc/nginx/nginx.conf","certificate_path":"/etc/nginx/certs/site.pem","private_key_path":"/etc/nginx/certs/site.key"}`
	targetResp := performJSONRequest(t, handler, http.MethodPost, "/api/v1/deployment-targets", targetBody, nil)
	if targetResp.Code != http.StatusCreated {
		t.Fatalf("create target status = %d, want %d: %s", targetResp.Code, http.StatusCreated, targetResp.Body.String())
	}

	listTargetsResp := performJSONRequest(t, handler, http.MethodGet, "/api/v1/deployment-targets", "", nil)
	if listTargetsResp.Code != http.StatusOK {
		t.Fatalf("list targets status = %d, want %d", listTargetsResp.Code, http.StatusOK)
	}
	var targetList listEnvelope
	decodeBody(t, listTargetsResp.Body, &targetList)
	if len(targetList.Items.([]any)) != 1 {
		t.Fatalf("target list size = %d, want %d", len(targetList.Items.([]any)), 1)
	}

	disableResp := performJSONRequest(t, handler, http.MethodPost, "/api/v1/nodes/"+nodeID+"/disable", "", nil)
	if disableResp.Code != http.StatusAccepted {
		t.Fatalf("disable node status = %d, want %d", disableResp.Code, http.StatusAccepted)
	}
}

func TestDeliveryAPIRejectsInvalidTarget(t *testing.T) {
	handler, nodeID := newDeliveryRouter(t)

	body := `{"name":"bad-tomcat","target_type":"tomcat-jsse-pkcs12","node_id":"` + nodeID + `","config_path":"/opt/tomcat/conf/server.xml"}`
	resp := performJSONRequest(t, handler, http.MethodPost, "/api/v1/deployment-targets", body, nil)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("create invalid target status = %d, want %d", resp.Code, http.StatusBadRequest)
	}
	var errResp errorResponse
	decodeBody(t, resp.Body, &errResp)
	if errResp.Error.Code != "REQUEST_VALIDATION_FAILED" {
		t.Fatalf("error code = %q, want REQUEST_VALIDATION_FAILED", errResp.Error.Code)
	}
}

func newDeliveryRouter(t *testing.T) (http.Handler, string) {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	nodeService := agentnode.NewService()
	targetService := deploymenttarget.NewService()
	nodeCommands := nodescmd.NewService(nodeService)
	targetCommands := targetscmd.NewService(targetService)
	node, err := nodeCommands.RegisterNode(t.Context(), defaultGovernanceScope, nodescmd.RegistrationInput{
		Name:            "edge-agent-a",
		Hostname:        "edge-a.local",
		IPAddress:       "10.0.0.12",
		Version:         "0.1.0",
		ProtocolVersion: 1,
		OS:              "linux",
		Arch:            "amd64",
		Labels:          map[string]string{"zone": "dmz"},
		Capabilities:    []string{agentnode.CapabilityDeployNGINX, agentnode.CapabilityKeygenRSA},
	})
	if err != nil {
		t.Fatalf("seed node: %v", err)
	}

	handler := NewRouter(Deps{
		Config: config.Config{
			ServiceName: "controlplane",
			Environment: "test",
		},
		BuildInfo: buildinfo.Info{
			Service:   "controlplane",
			Version:   "dev",
			Commit:    "abc123",
			BuildTime: "2026-04-20T00:00:00Z",
		},
		Logger:         logger,
		NodeCommands:   nodeCommands,
		TargetCommands: targetCommands,
	})

	return handler, node.ID
}
