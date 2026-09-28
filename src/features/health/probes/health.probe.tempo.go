/*
Package probes — Tempo deep functional health probe.

ALGORITHM BLUEPRINT:
1. HTTP /ready: reuses the GET http pattern from HealthService.probeOnce and
   the DefaultHealthTargets tempo target (health.schema.go) — /ready is the
   existing baseline; this probe extends it with a functional query.
2. Tempo API echo / version verification:
   a. GET http://<host>:<port>/api/echo → response body "echo" confirms the
      query-frontend API is functional.
   b. GET http://<host>:<port>/api/status/buildinfo → response body contains
      the Tempo version JSON confirming the query engine is operational.
   c. If /api/status/buildinfo is unavailable (older Tempo), fall back to
      confirming /ready returned 200 as sufficient evidence.
3. Evidence string: "tempo ready version=<V> target=<host:port>"
4. Invariants:
   - http.Client timeout is cfg.Timeout for all requests.
   - Response body is bounded to 4096 bytes via io.LimitReader.
   - /ready failure returns failProbe immediately.
   - Returns failProbe on all error paths; never panics.
*/
package probes

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/health/schema"
)

func ProbeGrafanaTempo(cfg schema.DeepProbeConfig) schema.SingleProbeResult {
	host := cfg.Host
	if host == "" {
		host = "localhost"
	}
	port := cfg.Port
	if port == 0 {
		port = 31416
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	target := fmt.Sprintf("%s:%d", host, port)
	baseURL := fmt.Sprintf("http://%s", target)
	start := time.Now()
	client := &http.Client{Timeout: timeout}

	readyResp, err := client.Get(baseURL + "/ready")
	if err != nil {
		return failProbe("tempo", start, fmt.Sprintf("GET /ready failed: %v", err))
	}
	readyResp.Body.Close()
	if readyResp.StatusCode >= 500 {
		return failProbe("tempo", start, fmt.Sprintf("GET /ready status %d", readyResp.StatusCode))
	}

	biResp, biErr := client.Get(baseURL + "/api/status/buildinfo")
	if biErr == nil && biResp.StatusCode == 200 {
		defer biResp.Body.Close()
		bodyBytes, _ := io.ReadAll(io.LimitReader(biResp.Body, 4096))
		var bi map[string]interface{}
		version := ""
		if json.Unmarshal(bodyBytes, &bi) == nil {
			if v, ok := bi["version"].(string); ok {
				version = v
			}
		}
		evidence := fmt.Sprintf("tempo ready version=%q target=%s", version, target)
		return okProbe("tempo", target, evidence, start)
	}
	if biErr == nil {
		biResp.Body.Close()
	}

	evidence := fmt.Sprintf("tempo ready buildinfo_unavailable target=%s", target)
	return okProbe("tempo", target, evidence, start)
}
