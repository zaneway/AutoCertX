package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/zaneway/AutoCertX/internal/driver/agenttransport"
)

// Client calls the control-plane Agent transport API.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	NodeID     string
}

// Register performs the initial bootstrap exchange and stores the returned node id.
func (c *Client) Register(ctx context.Context, req agenttransport.RegisterRequest) (agenttransport.RegisterResponse, error) {
	var resp agenttransport.RegisterResponse
	if err := c.doJSON(ctx, http.MethodPost, "/agent/v1/register", "", req, &resp); err != nil {
		return agenttransport.RegisterResponse{}, err
	}
	c.NodeID = resp.NodeID
	return resp, nil
}

// Heartbeat reports the current node liveness facts.
func (c *Client) Heartbeat(ctx context.Context, req agenttransport.HeartbeatRequest) (agenttransport.HeartbeatResponse, error) {
	if strings.TrimSpace(req.NodeID) == "" {
		req.NodeID = c.NodeID
	}
	var resp agenttransport.HeartbeatResponse
	if err := c.doJSON(ctx, http.MethodPost, "/agent/v1/heartbeat", "", req, &resp); err != nil {
		return agenttransport.HeartbeatResponse{}, err
	}
	return resp, nil
}

// Poll leases pending jobs for the current node.
func (c *Client) Poll(ctx context.Context, req agenttransport.JobPollRequest) (agenttransport.JobPollResponse, error) {
	if strings.TrimSpace(req.NodeID) == "" {
		req.NodeID = c.NodeID
	}
	var resp agenttransport.JobPollResponse
	if err := c.doJSON(ctx, http.MethodPost, "/agent/v1/jobs/poll", "", req, &resp); err != nil {
		return agenttransport.JobPollResponse{}, err
	}
	return resp, nil
}

// Progress reports one incremental execution heartbeat.
func (c *Client) Progress(ctx context.Context, jobID string, req agenttransport.JobProgressRequest) error {
	return c.doJSON(ctx, http.MethodPost, "/agent/v1/jobs/"+jobID+"/progress", c.NodeID, req, nil)
}

// Complete reports one terminal execution result.
func (c *Client) Complete(ctx context.Context, jobID string, req agenttransport.JobCompleteRequest) error {
	return c.doJSON(ctx, http.MethodPost, "/agent/v1/jobs/"+jobID+"/complete", c.NodeID, req, nil)
}

func (c *Client) doJSON(ctx context.Context, method string, path string, nodeID string, reqBody any, respBody any) error {
	baseURL := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if baseURL == "" {
		return fmt.Errorf("agent bootstrap base url required")
	}

	var body bytes.Buffer
	if reqBody != nil {
		if err := json.NewEncoder(&body).Encode(reqBody); err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, &body)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(nodeID) != "" {
		req.Header.Set("X-Agent-Node-Id", strings.TrimSpace(nodeID))
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var payload transportError
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return fmt.Errorf("transport request failed with status %d", resp.StatusCode)
		}
		return fmt.Errorf("%s: %s", payload.Error.Code, payload.Error.Message)
	}
	if respBody == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(respBody); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

type transportError struct {
	RequestID string `json:"request_id"`
	Error     struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}
