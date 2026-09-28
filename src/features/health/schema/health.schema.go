/*
Package schema defines entities and reporting models for platform health verification.

ALGORITHM BLUEPRINT:
1. ServiceHealthTarget: Specification of target address, port, protocol, path, criticality, and profile tags.
2. HealthCheckReport: Aggregated report indicating overall health status, latency, and individual probe results.
3. TargetsForProfiles: Filters the full target list to only those whose Profiles intersect the requested set.
   This ensures health checks only fail on services that were actually started.
4. Invariants:
   - Target timeout defaults to 3 seconds if unspecified.
   - Overall healthy status is true if and only if all required targets pass.
   - An empty profiles list means "check everything" (backward-compatible default).
*/
package schema

import "time"

type ServiceHealthTarget struct {
	Service  string        `json:"service"`
	Host     string        `json:"host"`
	Port     int           `json:"port"`
	Protocol string        `json:"protocol"`
	Path     string        `json:"path,omitempty"`
	Timeout  time.Duration `json:"timeout"`
	Required bool          `json:"required"`
	Profiles []string      `json:"profiles"` // Docker Compose profiles that start this service
}

type SingleProbeResult struct {
	Service   string  `json:"service"`
	Target    string  `json:"target"`
	Status    string  `json:"status"`
	LatencyMs float64 `json:"latencyMs"`
	Error     string  `json:"error,omitempty"`
	IsHealthy bool    `json:"isHealthy"`
	Required  bool    `json:"required"`
}

type HealthCheckReport struct {
	Healthy      bool                `json:"healthy"`
	CheckedCount int                 `json:"checkedCount"`
	HealthyCount int                 `json:"healthyCount"`
	Results      []SingleProbeResult `json:"results"`
}

// TargetsForProfiles returns only the health targets whose profile tags
// intersect the given set of active Docker Compose profiles.
// An empty profiles slice returns all targets (backward-compatible default).
func TargetsForProfiles(primaryHost string, activeProfiles []string) []ServiceHealthTarget {
	all := DefaultHealthTargets(primaryHost)
	if len(activeProfiles) == 0 {
		return all
	}
	activeSet := make(map[string]struct{}, len(activeProfiles))
	for _, p := range activeProfiles {
		activeSet[p] = struct{}{}
	}
	var out []ServiceHealthTarget
	for _, t := range all {
		for _, p := range t.Profiles {
			if _, ok := activeSet[p]; ok {
				out = append(out, t)
				break
			}
		}
	}
	return out
}

func DefaultHealthTargets(primaryHost string) []ServiceHealthTarget {
	if primaryHost == "" {
		primaryHost = "localhost"
	}
	return []ServiceHealthTarget{
		{Service: "traefik", Host: "localhost", Port: 31410, Protocol: "http", Path: "/ping", Timeout: 3 * time.Second, Required: true, Profiles: []string{"network", "stateless", "full"}},
		{Service: "service-registry", Host: "localhost", Port: 31426, Protocol: "http", Path: "/health", Timeout: 3 * time.Second, Required: true, Profiles: []string{"network", "stateless", "full"}},
		{Service: "otel-collector", Host: "localhost", Port: 31417, Protocol: "http", Path: "/", Timeout: 3 * time.Second, Required: true, Profiles: []string{"tracing", "stateless", "full"}},
		{Service: "grafana", Host: "localhost", Port: 31415, Protocol: "http", Path: "/api/health", Timeout: 3 * time.Second, Required: false, Profiles: []string{"tracing", "stateless", "full"}},
		{Service: "redis", Host: primaryHost, Port: 31413, Protocol: "tcp", Timeout: 3 * time.Second, Required: true, Profiles: []string{"db", "stateful", "full"}},
		{Service: "kafka", Host: primaryHost, Port: 31414, Protocol: "tcp", Timeout: 3 * time.Second, Required: true, Profiles: []string{"streaming", "stateful", "full"}},
		{Service: "tempo", Host: primaryHost, Port: 31416, Protocol: "http", Path: "/ready", Timeout: 3 * time.Second, Required: false, Profiles: []string{"tracing", "stateless", "full"}},
		{Service: "alloydb", Host: primaryHost, Port: 31420, Protocol: "tcp", Timeout: 3 * time.Second, Required: true, Profiles: []string{"db", "stateful", "full"}},
		{Service: "clickhouse", Host: primaryHost, Port: 31421, Protocol: "http", Path: "/ping", Timeout: 3 * time.Second, Required: false, Profiles: []string{"analytics", "stateful", "full"}},
		{Service: "temporal", Host: primaryHost, Port: 31424, Protocol: "tcp", Timeout: 3 * time.Second, Required: false, Profiles: []string{"workflows", "stateless", "full"}},
	}
}
