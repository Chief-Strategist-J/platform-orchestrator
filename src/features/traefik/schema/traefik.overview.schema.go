/*
Package schema defines data contracts for Traefik diagnostic status, overview metrics, and entrypoints.

ALGORITHM BLUEPRINT:
1. PingResult: Schema for health status and roundtrip latency.
2. OverviewReport: Aggregated counts of active HTTP/TCP routers, services, and middlewares.
3. EntryPointInfo: Port binding and address definitions for network entrypoints.
4. Invariants:
   - Zero inline comments inside function bodies.
   - Deterministic JSON marshaling for all fields.
*/
package schema

type PingResult struct {
	Status    string  `json:"status"`
	IsHealthy bool    `json:"isHealthy"`
	LatencyMs float64 `json:"latencyMs"`
	Error     string  `json:"error,omitempty"`
}

type ComponentCount struct {
	Total    int `json:"total"`
	Warnings int `json:"warnings,omitempty"`
	Errors   int `json:"errors,omitempty"`
}

type OverviewSection struct {
	Routers     ComponentCount `json:"routers"`
	Services    ComponentCount `json:"services"`
	Middlewares ComponentCount `json:"middlewares"`
}

type OverviewReport struct {
	HTTP      OverviewSection        `json:"http"`
	TCP       OverviewSection        `json:"tcp"`
	UDP       OverviewSection        `json:"udp"`
	Features  map[string]interface{} `json:"features,omitempty"`
	Providers []string               `json:"providers,omitempty"`
}

type EntryPointInfo struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}
