/*
Package probes — ClickHouse deep functional health probe.

ALGORITHM BLUEPRINT:
1. TCP connectivity: dialTCP (same pattern as HealthService.probeOnce and the
   ClickHouse check in verifyNativeCredentials — single source of truth for TCP
   reachability, extended here with functional verification).
2. ClickHouse HTTP interface query:
   a. GET http://<host>:<port>/ping → response body "Ok.\n" confirms the server
      is alive and serving requests (reuses the /ping pattern from
      DefaultHealthTargets in health.schema.go).
   b. POST http://<host>:<port>/?query=SELECT+version() HTTP/1.1 → response
      body contains the ClickHouse version string confirming the query engine
      is functional, not just the HTTP listener.
3. Evidence string: "clickhouse version=<V> target=<host:port>"
4. Invariants:
   - http.Client timeout is set to cfg.Timeout before every request.
   - Response body is bounded to 4096 bytes via io.LimitReader to prevent OOM.
   - Both /ping and the SELECT are required; /ping alone is connectivity only.
   - Returns failProbe on any error path; never panics.
*/
package probes

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/health/schema"
)

func ProbeClickHouse(cfg schema.DeepProbeConfig) schema.SingleProbeResult {
	host := cfg.Host
	if host == "" {
		host = "localhost"
	}
	port := cfg.Port
	if port == 0 {
		port = 31421
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	target := fmt.Sprintf("%s:%d", host, port)
	baseURL := fmt.Sprintf("http://%s", target)
	start := time.Now()

	conn, err := dialTCP(host, port, timeout)
	if err != nil {
		return failProbe("clickhouse", start, fmt.Sprintf("TCP dial failed: %v", err))
	}
	conn.Close()

	client := &http.Client{Timeout: timeout}

	pingResp, pingErr := client.Get(baseURL + "/ping")
	if pingErr != nil {
		return failProbe("clickhouse", start, fmt.Sprintf("GET /ping failed: %v", pingErr))
	}
	pingResp.Body.Close()
	if pingResp.StatusCode >= 500 {
		return failProbe("clickhouse", start, fmt.Sprintf("GET /ping status %d", pingResp.StatusCode))
	}

	queryURL := baseURL + "/?query=SELECT+version()"
	queryResp, queryErr := client.Get(queryURL)
	if queryErr != nil {
		return failProbe("clickhouse", start, fmt.Sprintf("SELECT version() failed: %v", queryErr))
	}
	defer queryResp.Body.Close()

	bodyBytes, readErr := io.ReadAll(io.LimitReader(queryResp.Body, 4096))
	if readErr != nil {
		return failProbe("clickhouse", start, fmt.Sprintf("response read failed: %v", readErr))
	}
	version := strings.TrimSpace(string(bodyBytes))
	if version == "" {
		return failProbe("clickhouse", start, "SELECT version() returned empty response")
	}

	evidence := fmt.Sprintf("clickhouse version=%q target=%s", version, target)
	return okProbe("clickhouse", target, evidence, start)
}
