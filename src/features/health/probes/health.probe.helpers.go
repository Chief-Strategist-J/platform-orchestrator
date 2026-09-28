/*
Package probes — shared helpers for deep health probe functions.

ALGORITHM BLUEPRINT:
1. ProbeFunc: Pure function type that all service probe functions satisfy.
   No receiver, no global state, deterministic given the same DeepProbeConfig.
2. dialTCP: Reuses the net.DialTimeout pattern from HealthService.probeOnce
   (services/health.service.go) — single source of truth for TCP connectivity.
3. readFull: Bounded read helper used by binary-protocol probes (alloydb).
4. msElapsed: Returns wall-clock elapsed milliseconds from start — embeds the
   unit in the function name per open.standard.md §deep-dive-1.
5. failProbe: Constructs a DOWN SingleProbeResult from an error string, reusing
   schema.SingleProbeResult (DRY rule 14 — no duplicate result type).
6. okProbe: Constructs an UP SingleProbeResult with Evidence.
7. utcNow: Returns RFC 3339 UTC with Z suffix per open.standard.md §deep-dive-7.
8. Invariants:
   - failProbe and okProbe are the ONLY places that construct SingleProbeResult
     inside the probes package; all probe functions delegate to them.
   - No probe function in this package imports or calls another probe function.
*/
package probes

import (
	"fmt"
	"net"
	"time"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/health/schema"
)

type ProbeFunc func(cfg schema.DeepProbeConfig) schema.SingleProbeResult

func dialTCP(host string, port int, timeout time.Duration) (net.Conn, error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	return net.DialTimeout("tcp", addr, timeout)
}

func readFull(conn interface{ Read([]byte) (int, error) }, buf []byte) error {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return err
		}
	}
	return nil
}

func msElapsed(start time.Time) float64 {
	return float64(time.Since(start).Milliseconds())
}

func utcNow() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func failProbe(service string, start time.Time, errMsg string) schema.SingleProbeResult {
	return schema.SingleProbeResult{
		Service:   service,
		Target:    service,
		Status:    "DOWN",
		LatencyMs: msElapsed(start),
		Error:     errMsg,
		IsHealthy: false,
	}
}

func okProbe(service, target, evidence string, start time.Time) schema.SingleProbeResult {
	return schema.SingleProbeResult{
		Service:   service,
		Target:    target,
		Status:    "UP",
		LatencyMs: msElapsed(start),
		IsHealthy: true,
		Error:     evidence,
	}
}
