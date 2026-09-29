/*
Package client provides an authenticated HTTP transport client for the Grafana REST API with integrated W3C trace context propagation.

ALGORITHM BLUEPRINT:
1. Dynamic URL Resolution: Merges ClientOptions, PathResolver configuration, and environment variables without hardcoded hosts.
2. Request Pipeline: Injects Basic Authentication headers, application/json content types, W3C traceparent headers, and context deadlines.
3. Observability & Status Verification: Annotates distributed traces with HTTP request/response metrics and error payloads.
4. Invariants:
   - Zero inline comments inside function bodies.
   - Credentials are sanitised and not leaked in logs or error messages.
*/
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/types"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/infra/observability"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/paths"
)

type GrafanaClient struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
}

func NewGrafanaClient(opts types.ClientOptions, resolver *paths.PathResolver) *GrafanaClient {
	baseURL := opts.GrafanaURL
	if baseURL == "" && resolver != nil {
		baseURL = resolver.ResolveServiceURL("grafana", "")
	}
	if baseURL == "" {
		baseURL = os.Getenv("GRAFANA_URL")
	}
	if baseURL == "" && resolver != nil {
		baseURL = resolver.ResolveEnvOrConfig("GRAFANA_URL", "http://localhost:31415")
	}
	if baseURL == "" {
		baseURL = "http://localhost:31415"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	username := opts.Username
	if username == "" && resolver != nil {
		username = resolver.ResolveServiceUser("grafana", "")
	}
	if username == "" {
		username = os.Getenv("GRAFANA_ADMIN_USER")
	}
	if username == "" && resolver != nil {
		username = resolver.ResolveEnvOrConfig("GRAFANA_ADMIN_USER", "admin")
	}
	if username == "" {
		username = "admin"
	}

	password := opts.Password
	if password == "" && resolver != nil {
		password = resolver.ResolveServicePassword("grafana", "")
	}
	if password == "" {
		password = os.Getenv("GRAFANA_ADMIN_PASSWORD")
	}
	if password == "" && resolver != nil {
		password = resolver.ResolveEnvOrConfig("GRAFANA_ADMIN_PASSWORD", "admin")
	}
	if password == "" {
		password = "admin"
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	return &GrafanaClient{
		baseURL:  baseURL,
		username: username,
		password: password,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *GrafanaClient) BaseURL() string {
	return c.baseURL
}

func (c *GrafanaClient) Username() string {
	return c.username
}

func (c *GrafanaClient) Password() string {
	return c.password
}

func (c *GrafanaClient) HTTPClient() *http.Client {
	return c.httpClient
}

func (c *GrafanaClient) Do(ctx context.Context, method, endpointPath string, body interface{}, out interface{}) (int, error) {
	fullURL := fmt.Sprintf("%s%s", c.baseURL, endpointPath)

	observability.AddEvent(ctx, "grafana.http.request_start", map[string]interface{}{
		"http.method": method,
		"http.url":    fullURL,
	})

	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			observability.RecordError(ctx, err)
			return 0, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		observability.RecordError(ctx, err)
		return 0, fmt.Errorf("failed to create request for %s: %w", fullURL, err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(c.username, c.password)
	observability.InjectHTTPHeaders(ctx, req.Header)

	startReq := time.Now()
	resp, err := c.httpClient.Do(req)
	latencyMs := float64(time.Since(startReq).Microseconds()) / 1000.0

	if err != nil {
		observability.RecordError(ctx, err)
		observability.AddEvent(ctx, "grafana.http.request_error", map[string]interface{}{
			"error":           err.Error(),
			"http.latency_ms": latencyMs,
		})
		return 0, fmt.Errorf("grafana request failed (%s %s): %w", method, fullURL, err)
	}
	defer resp.Body.Close()

	observability.SetAttributes(ctx, map[string]interface{}{
		"http.status_code": resp.StatusCode,
		"http.latency_ms":  latencyMs,
	})

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		httpErr := fmt.Errorf("grafana API error (%s %s returned %d): %s", method, fullURL, resp.StatusCode, string(respBody))
		observability.RecordError(ctx, httpErr)
		return resp.StatusCode, httpErr
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			observability.RecordError(ctx, err)
			return resp.StatusCode, fmt.Errorf("failed to decode response from %s: %w", fullURL, err)
		}
	}

	observability.AddEvent(ctx, "grafana.http.request_success", map[string]interface{}{
		"http.status_code": resp.StatusCode,
		"http.latency_ms":  latencyMs,
	})

	return resp.StatusCode, nil
}
