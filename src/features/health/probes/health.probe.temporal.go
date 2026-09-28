/*
Package probes — Temporal deep functional health probe.

ALGORITHM BLUEPRINT:
1. TCP connectivity: dialTCP (same pattern as HealthService.probeOnce and the
   Temporal TCP target in DefaultHealthTargets — extends the existing baseline).
2. Temporal Web UI / frontend HTTP probe:
   a. Temporal's frontend gRPC runs on cfg.Port (31424); the Temporal Web UI HTTP
      API runs on port 8080 (or cfg.Port + offset). Since we have no gRPC library,
      we probe the Temporal Web UI health endpoint if available.
   b. GET http://<host>:<webUIPort>/api/v1/namespaces → expects JSON with
      namespace list; presence of "default" namespace confirms a live cluster.
   c. Temporal's default health check path: GET /api/v1/system-info.
3. Fallback: If HTTP health check is unavailable, confirm TCP reachability of
   the gRPC frontend port as minimum evidence (same as existing TCP check in
   DefaultHealthTargets, promoted from connectivity to evidence).
4. Evidence string: "temporal namespace=<ns> target=<host:port>"
5. Invariants:
   - TCP check comes first; HTTP check is additive.
   - http.Client timeout is cfg.Timeout.
   - JSON response body is bounded to 65536 bytes via io.LimitReader.
   - Returns failProbe on TCP failure; falls back to TCP-only evidence on HTTP failure.
   - Never panics.
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

func ProbeTemporalWorkflow(cfg schema.DeepProbeConfig) schema.SingleProbeResult {
	host := cfg.Host
	if host == "" {
		host = "localhost"
	}
	port := cfg.Port
	if port == 0 {
		port = 31424
	}
	namespace := cfg.TemporalNS
	if namespace == "" {
		namespace = "default"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	target := fmt.Sprintf("%s:%d", host, port)
	start := time.Now()

	conn, err := dialTCP(host, port, timeout)
	if err != nil {
		return failProbe("temporal", start, fmt.Sprintf("TCP dial failed: %v", err))
	}
	conn.Close()

	webUIPort := 31425
	webUIURL := fmt.Sprintf("http://%s:%d", host, webUIPort)
	client := &http.Client{Timeout: timeout}

	nsURL := fmt.Sprintf("%s/api/v1/namespaces/%s", webUIURL, namespace)
	nsResp, nsErr := client.Get(nsURL)
	if nsErr == nil && nsResp.StatusCode == 200 {
		defer nsResp.Body.Close()
		bodyBytes, _ := io.ReadAll(io.LimitReader(nsResp.Body, 65536))
		var ns map[string]interface{}
		nsName := namespace
		if json.Unmarshal(bodyBytes, &ns) == nil {
			if info, ok := ns["namespaceInfo"].(map[string]interface{}); ok {
				if n, ok := info["name"].(string); ok {
					nsName = n
				}
			}
		}
		evidence := fmt.Sprintf("temporal namespace=%q target=%s webUI=%s", nsName, target, webUIURL)
		return okProbe("temporal", target, evidence, start)
	}
	if nsErr == nil {
		nsResp.Body.Close()
	}

	evidence := fmt.Sprintf("temporal tcp_ok target=%s webUI_unavailable", target)
	return okProbe("temporal", target, evidence, start)
}
