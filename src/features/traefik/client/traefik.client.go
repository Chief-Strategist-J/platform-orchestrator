/*
Package client provides an HTTP transport client for the Traefik management API with integrated W3C trace context propagation.

ALGORITHM BLUEPRINT:
1. Dynamic URL Resolution: Resolves Traefik API base URL from ClientOptions, PathResolver, or default (http://localhost:8080).
2. Request Pipeline: Injects application/json headers, W3C traceparent context, and handles context deadlines.
3. Observability & Error Handling: Wraps outbound calls in span events and decodes JSON response payloads.
4. Invariants:
   - Zero inline comments inside function bodies.
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

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/types"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/infra/observability"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/paths"
)

type TraefikClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewTraefikClient(opts types.ClientOptions, resolver *paths.PathResolver) *TraefikClient {
	baseURL := opts.TraefikURL
	if baseURL == "" && resolver != nil {
		baseURL = resolver.ResolveServiceURL("traefik", "")
	}
	if baseURL == "" {
		baseURL = os.Getenv("TRAEFIK_API_URL")
	}
	if baseURL == "" && resolver != nil {
		dashboardPort := resolver.ResolveEnvOrConfig("PORT_TRAEFIK_DASHBOARD", "31411")
		baseURL = fmt.Sprintf("http://localhost:%s", dashboardPort)
	}
	if baseURL == "" {
		baseURL = "http://localhost:31411"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	return &TraefikClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *TraefikClient) BaseURL() string {
	return c.baseURL
}

func (c *TraefikClient) HTTPClient() *http.Client {
	return c.httpClient
}

func (c *TraefikClient) Do(ctx context.Context, method, endpointPath string, body interface{}, out interface{}) (int, error) {
	fullURL := fmt.Sprintf("%s%s", c.baseURL, endpointPath)

	observability.AddEvent(ctx, "traefik.http.request_start", map[string]interface{}{
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
	observability.InjectHTTPHeaders(ctx, req.Header)

	startReq := time.Now()
	resp, err := c.httpClient.Do(req)
	latencyMs := float64(time.Since(startReq).Microseconds()) / 1000.0

	if err != nil {
		observability.RecordError(ctx, err)
		observability.AddEvent(ctx, "traefik.http.request_error", map[string]interface{}{
			"error":           err.Error(),
			"http.latency_ms": latencyMs,
		})
		return 0, fmt.Errorf("traefik request failed (%s %s): %w", method, fullURL, err)
	}
	defer resp.Body.Close()

	observability.SetAttributes(ctx, map[string]interface{}{
		"http.status_code": resp.StatusCode,
		"http.latency_ms":  latencyMs,
	})

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		httpErr := fmt.Errorf("traefik API error (%s %s returned %d): %s", method, fullURL, resp.StatusCode, string(respBody))
		observability.RecordError(ctx, httpErr)
		return resp.StatusCode, httpErr
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			observability.RecordError(ctx, err)
			return resp.StatusCode, fmt.Errorf("failed to decode response from %s: %w", fullURL, err)
		}
	}

	observability.AddEvent(ctx, "traefik.http.request_success", map[string]interface{}{
		"http.status_code": resp.StatusCode,
		"http.latency_ms":  latencyMs,
	})

	return resp.StatusCode, nil
}
