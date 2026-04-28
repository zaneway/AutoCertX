package bootstrap

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	httpserver "github.com/zaneway/AutoCertX/internal/app/controlplane/http"
	nodescmd "github.com/zaneway/AutoCertX/internal/application/command/nodes"
	"github.com/zaneway/AutoCertX/internal/domain/agentnode"
	"github.com/zaneway/AutoCertX/internal/domain/resource"
	"github.com/zaneway/AutoCertX/internal/driver/agenttransport"
	"github.com/zaneway/AutoCertX/internal/platform/buildinfo"
	"github.com/zaneway/AutoCertX/internal/platform/config"
)

func TestClientRegisterPollProgressAndComplete(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	nodeService := agentnode.NewService()
	nodeCommands := nodescmd.NewService(nodeService)
	transport, err := agenttransport.NewService(nodeService, agenttransport.Options{})
	if err != nil {
		t.Fatalf("agenttransport.NewService() error = %v", err)
	}
	token, err := nodeCommands.CreateRegistrationToken(t.Context(), testScope())
	if err != nil {
		t.Fatalf("create registration token: %v", err)
	}

	handler := httpserver.NewRouter(httpserver.Deps{
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

	client := Client{
		BaseURL: "http://agenttransport.test",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, req)
				return recorder.Result(), nil
			}),
		},
	}
	registerResp, err := client.Register(t.Context(), agenttransport.RegisterRequest{
		Token:           token.Token,
		NodeName:        "edge-agent-a",
		Hostname:        "edge-a.local",
		IPAddress:       "10.0.0.12",
		OS:              "linux",
		Arch:            "amd64",
		Version:         "0.1.0",
		ProtocolVersion: 1,
		Capabilities:    []string{agentnode.CapabilityDeployNGINX},
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	if _, err := client.Heartbeat(t.Context(), agenttransport.HeartbeatRequest{
		Status:          agentnode.StatusOnline,
		Version:         "0.1.1",
		ProtocolVersion: 1,
		Capabilities:    []string{agentnode.CapabilityDeployNGINX},
	}); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	job, err := transport.Dispatch(t.Context(), agenttransport.DispatchInput{
		Scope:       testScope(),
		NodeID:      registerResp.NodeID,
		JobType:     agenttransport.JobTypeDeployNGINXCertificate,
		OperationID: "op-bootstrap-001",
		Payload: map[string]any{
			"target_id": "55555555-5555-4555-8555-555555555555",
		},
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	pollResp, err := client.Poll(t.Context(), agenttransport.JobPollRequest{
		ProtocolVersion:         1,
		SupportedSchemaVersions: []int{1},
		MaxJobs:                 1,
	})
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(pollResp.Items) != 1 || pollResp.Items[0].JobID != job.JobID {
		t.Fatalf("polled items = %+v, want job %s", pollResp.Items, job.JobID)
	}

	if err := client.Progress(t.Context(), job.JobID, agenttransport.JobProgressRequest{
		OperationID: "op-bootstrap-001",
		Status:      "running",
	}); err != nil {
		t.Fatalf("progress: %v", err)
	}
	if err := client.Complete(t.Context(), job.JobID, agenttransport.JobCompleteRequest{
		OperationID:  "op-bootstrap-001",
		ResultStatus: "succeeded",
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}
}

func testScope() resource.Scope {
	return resource.Scope{
		TenantID:      "11111111-1111-4111-8111-111111111111",
		ProjectID:     "22222222-2222-4222-8222-222222222222",
		EnvironmentID: "33333333-3333-4333-8333-333333333333",
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
