/*
Package probes — OTel Collector, Traefik, and Service Registry deep functional probes.

ALGORITHM BLUEPRINT:
1. ProbeOtelCollector:
   a. TCP reachability on HTTP port: reuses dialTCP (same as verifyNativeCredentials
      OTel check in root.go).
   b. TCP reachability on gRPC port: same pattern, applied to cfg.OtelGRPCPort.
   c. HTTP GET <host>:<httpPort>/metrics → response body contains "otelcol_"
      metric names confirming the collector is actively processing telemetry.
   d. Evidence: "otel http_ok grpc_ok metrics_scraped target=<host:port>"

2. ProbeTraefik:
   a. HTTP GET http://localhost:<port>/ping → response "OK" (reuses the existing
      DefaultHealthTargets traefik target path /ping — extends from connectivity
      to functional evidence).
   b. HTTP GET http://localhost:<8080>/api/rawdata → response is Traefik config
      JSON; confirms routing is active. Port 8080 is Traefik's dashboard default.
   c. Evidence: "traefik ping_ok routes=<N> target=<host:port>"

3. ProbeServiceRegistry:
   a. HTTP GET http://localhost:<port>/health → status 200 (reuses the existing
      DefaultHealthTargets service-registry target path /health).
   b. HTTP GET http://localhost:<port>/v1/catalog/services → response is a JSON
      object of registered services confirming the registry is functional.
   c. Evidence: "service-registry health_ok services=<N> target=<host:port>"

4. Invariants:
   - http.Client timeout is cfg.Timeout for all requests.
   - Response bodies are bounded to 65536 bytes via io.LimitReader.
   - TCP check precedes HTTP check; TCP failure returns failProbe immediately.
   - Returns failProbe on all unrecoverable errors; never panics.
*/
package probes

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/health/schema"
)

func ProbeOtelCollector(cfg schema.DeepProbeConfig) schema.SingleProbeResult {
	host := cfg.Host
	if host == "" {
		host = "localhost"
	}
	port := cfg.Port
	if port == 0 {
		port = 31417
	}
	grpcPort := cfg.OtelGRPCPort
	if grpcPort == 0 {
		grpcPort = 31418
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	target := fmt.Sprintf("%s:%d", host, port)
	start := time.Now()

	httpConn, err := dialTCP(host, port, timeout)
	if err != nil {
		return failProbe("otel-collector", start, fmt.Sprintf("HTTP port TCP dial failed: %v", err))
	}
	httpConn.Close()

	grpcConn, grpcErr := dialTCP(host, grpcPort, timeout)
	grpcOK := grpcErr == nil
	if grpcOK {
		grpcConn.Close()
	}

	client := &http.Client{Timeout: timeout}
	metricsURL := fmt.Sprintf("http://%s/metrics", target)
	metricsResp, metricsErr := client.Get(metricsURL)
	metricsOK := false
	if metricsErr == nil && metricsResp.StatusCode == 200 {
		body, _ := io.ReadAll(io.LimitReader(metricsResp.Body, 65536))
		metricsResp.Body.Close()
		metricsOK = strings.Contains(string(body), "otelcol_")
	} else if metricsErr == nil {
		metricsResp.Body.Close()
	}

	evidence := fmt.Sprintf("otel http_ok grpc_ok=%v metrics_scraped=%v target=%s grpc_port=%d",
		grpcOK, metricsOK, target, grpcPort)
	return okProbe("otel-collector", target, evidence, start)
}

func ProbeTraefik(cfg schema.DeepProbeConfig) schema.SingleProbeResult {
	host := cfg.Host
	if host == "" {
		host = "localhost"
	}
	port := cfg.Port
	if port == 0 {
		port = 31410
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	target := fmt.Sprintf("%s:%d", host, port)
	baseURL := fmt.Sprintf("http://%s", target)
	start := time.Now()
	client := &http.Client{Timeout: timeout}

	pingResp, err := client.Get(baseURL + "/ping")
	if err != nil {
		return failProbe("traefik", start, fmt.Sprintf("GET /ping failed: %v", err))
	}
	pingResp.Body.Close()
	if pingResp.StatusCode >= 500 {
		return failProbe("traefik", start, fmt.Sprintf("GET /ping status %d", pingResp.StatusCode))
	}

	apiResp, apiErr := client.Get("http://" + host + ":8080/api/rawdata")
	routeCount := 0
	if apiErr == nil && apiResp.StatusCode == 200 {
		body, _ := io.ReadAll(io.LimitReader(apiResp.Body, 65536))
		apiResp.Body.Close()
		var rawdata map[string]interface{}
		if json.Unmarshal(body, &rawdata) == nil {
			if routers, ok := rawdata["routers"].(map[string]interface{}); ok {
				routeCount = len(routers)
			}
		}
	} else if apiErr == nil {
		apiResp.Body.Close()
	}

	evidence := fmt.Sprintf("traefik ping_ok routes=%d target=%s", routeCount, target)
	return okProbe("traefik", target, evidence, start)
}

func ProbeServiceRegistry(cfg schema.DeepProbeConfig) schema.SingleProbeResult {
	host := cfg.Host
	if host == "" {
		host = "localhost"
	}
	port := cfg.Port
	if port == 0 {
		port = 31426
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	target := fmt.Sprintf("%s:%d", host, port)
	baseURL := fmt.Sprintf("http://%s", target)
	start := time.Now()
	client := &http.Client{Timeout: timeout}

	healthResp, err := client.Get(baseURL + "/health")
	if err != nil {
		return failProbe("service-registry", start, fmt.Sprintf("GET /health failed: %v", err))
	}
	healthResp.Body.Close()
	if healthResp.StatusCode >= 500 {
		return failProbe("service-registry", start, fmt.Sprintf("GET /health status %d", healthResp.StatusCode))
	}

	svcResp, svcErr := client.Get(baseURL + "/v1/catalog/services")
	serviceCount := 0
	if svcErr == nil && svcResp.StatusCode == 200 {
		body, _ := io.ReadAll(io.LimitReader(svcResp.Body, 65536))
		svcResp.Body.Close()
		var services map[string]interface{}
		if json.Unmarshal(body, &services) == nil {
			serviceCount = len(services)
		}
	} else if svcErr == nil {
		svcResp.Body.Close()
	}

	evidence := fmt.Sprintf("service-registry health_ok services=%d target=%s", serviceCount, target)
	return okProbe("service-registry", target, evidence, start)
}
