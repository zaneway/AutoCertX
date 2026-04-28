package httpserver

import (
	"io"
	"log/slog"
	"net/http"
	"testing"

	nodescmd "github.com/zaneway/AutoCertX/internal/application/command/nodes"
	"github.com/zaneway/AutoCertX/internal/domain/agentnode"
	"github.com/zaneway/AutoCertX/internal/driver/agenttransport"
	"github.com/zaneway/AutoCertX/internal/platform/buildinfo"
	"github.com/zaneway/AutoCertX/internal/platform/config"
)

func TestAgentTransportAPIHappyPath(t *testing.T) {
	handler, transport, token := newAgentTransportRouter(t)

	registerBody := `{
		"token":"` + token + `",
		"node_name":"edge-agent-a",
		"hostname":"edge-a.local",
		"ip_address":"10.0.0.12",
		"os":"linux",
		"arch":"amd64",
		"version":"0.1.0",
		"protocol_version":1,
		"capabilities":["deploy:nginx","challenge:http-01","keygen:rsa"]
	}`
	registerResp := performJSONRequest(t, handler, http.MethodPost, "/agent/v1/register", registerBody, nil)
	if registerResp.Code != http.StatusOK {
		t.Fatalf("register status = %d, want %d: %s", registerResp.Code, http.StatusOK, registerResp.Body.String())
	}

	var registerPayload agenttransport.RegisterResponse
	decodeBody(t, registerResp.Body, &registerPayload)
	if registerPayload.NodeID == "" {
		t.Fatal("node id should be returned")
	}
	if registerPayload.Status != agentnode.StatusRegistering {
		t.Fatalf("status = %q, want %q", registerPayload.Status, agentnode.StatusRegistering)
	}

	heartbeatBody := `{
		"node_id":"` + registerPayload.NodeID + `",
		"protocol_version":1,
		"status":"online",
		"version":"0.1.1",
		"capabilities":["deploy:nginx","challenge:http-01","keygen:rsa"]
	}`
	heartbeatResp := performJSONRequest(t, handler, http.MethodPost, "/agent/v1/heartbeat", heartbeatBody, nil)
	if heartbeatResp.Code != http.StatusOK {
		t.Fatalf("heartbeat status = %d, want %d: %s", heartbeatResp.Code, http.StatusOK, heartbeatResp.Body.String())
	}

	job, err := transport.Dispatch(t.Context(), agenttransport.DispatchInput{
		Scope:       defaultGovernanceScope,
		NodeID:      registerPayload.NodeID,
		JobType:     agenttransport.JobTypeDeployNGINXCertificate,
		OperationID: "op-deploy-nginx-001",
		Payload: map[string]any{
			"target_id":       "55555555-5555-4555-8555-555555555555",
			"asset_id":        "66666666-6666-4666-8666-666666666666",
			"allowed_paths":   []string{"/etc/nginx/certs/site.pem"},
			"certificate_ref": "asset-version-1",
		},
	})
	if err != nil {
		t.Fatalf("dispatch job: %v", err)
	}

	pollBody := `{
		"node_id":"` + registerPayload.NodeID + `",
		"protocol_version":1,
		"supported_schema_versions":[1],
		"max_jobs":1
	}`
	pollResp := performJSONRequest(t, handler, http.MethodPost, "/agent/v1/jobs/poll", pollBody, nil)
	if pollResp.Code != http.StatusOK {
		t.Fatalf("poll status = %d, want %d: %s", pollResp.Code, http.StatusOK, pollResp.Body.String())
	}

	var pollPayload agenttransport.JobPollResponse
	decodeBody(t, pollResp.Body, &pollPayload)
	if len(pollPayload.Items) != 1 {
		t.Fatalf("poll item count = %d, want 1", len(pollPayload.Items))
	}
	if pollPayload.Items[0].JobID != job.JobID {
		t.Fatalf("polled job id = %q, want %q", pollPayload.Items[0].JobID, job.JobID)
	}

	progressHeaders := map[string]string{"X-Agent-Node-Id": registerPayload.NodeID}
	progressResp := performJSONRequest(t, handler, http.MethodPost, "/agent/v1/jobs/"+job.JobID+"/progress", `{
		"operation_id":"op-deploy-nginx-001",
		"status":"running",
		"progress_percent":40
	}`, progressHeaders)
	if progressResp.Code != http.StatusOK {
		t.Fatalf("progress status = %d, want %d: %s", progressResp.Code, http.StatusOK, progressResp.Body.String())
	}

	completeResp := performJSONRequest(t, handler, http.MethodPost, "/agent/v1/jobs/"+job.JobID+"/complete", `{
		"operation_id":"op-deploy-nginx-001",
		"result_status":"succeeded"
	}`, progressHeaders)
	if completeResp.Code != http.StatusOK {
		t.Fatalf("complete status = %d, want %d: %s", completeResp.Code, http.StatusOK, completeResp.Body.String())
	}

	duplicateCompleteResp := performJSONRequest(t, handler, http.MethodPost, "/agent/v1/jobs/"+job.JobID+"/complete", `{
		"operation_id":"op-deploy-nginx-001",
		"result_status":"succeeded"
	}`, progressHeaders)
	if duplicateCompleteResp.Code != http.StatusOK {
		t.Fatalf("duplicate complete status = %d, want %d: %s", duplicateCompleteResp.Code, http.StatusOK, duplicateCompleteResp.Body.String())
	}
}

func TestAgentTransportAPIRejectsInvalidRegistrationToken(t *testing.T) {
	handler, _, _ := newAgentTransportRouter(t)

	resp := performJSONRequest(t, handler, http.MethodPost, "/agent/v1/register", `{
		"token":"bad-token",
		"node_name":"edge-agent-a",
		"version":"0.1.0",
		"protocol_version":1
	}`, nil)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("register invalid token status = %d, want %d", resp.Code, http.StatusUnauthorized)
	}
}

func newAgentTransportRouter(t *testing.T) (http.Handler, *agenttransport.Service, string) {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	nodeService := agentnode.NewService()
	nodeCommands := nodescmd.NewService(nodeService)
	transport, err := agenttransport.NewService(nodeService, agenttransport.Options{})
	if err != nil {
		t.Fatalf("agenttransport.NewService() error = %v", err)
	}
	token, err := nodeCommands.CreateRegistrationToken(t.Context(), defaultGovernanceScope)
	if err != nil {
		t.Fatalf("create registration token: %v", err)
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
		AgentTransport: transport,
	})

	return handler, transport, token.Token
}
