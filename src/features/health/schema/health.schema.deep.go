/*
Package schema defines deep probe configuration and result models for platform health verification.

ALGORITHM BLUEPRINT:
1. SingleProbeResult: Canonical result type returned by every probe function.
   IsHealthy=true means the service passed its full functional verification.
   The Error field carries evidence text on success and error detail on failure.
2. DeepProbeConfig: Configuration for a single service deep probe. Contains only
   fields absent from a basic host:port target — credentials, container names,
   service-specific URLs, and derived ports.
3. DeepHealthReport: Aggregated report from RunDeepHealthChecks. Wraps
   []SingleProbeResult so results are structurally consistent across all callers.
4. DefaultDeepProbeConfigs: Returns one DeepProbeConfig per service keyed to the
   platform's canonical port assignments. Single source of truth for all service
   addresses and defaults.
5. Invariants:
   - Container field defaults to service name when empty.
   - All credential fields are optional; probes apply safe defaults.
   - ReportedAt uses RFC 3339 UTC with Z suffix per open.standard.md §deep-dive-7.
   - Port assignments must never be duplicated across services.
*/
package schema

import "time"

type SingleProbeResult struct {
	Service   string  `json:"service"`
	Target    string  `json:"target"`
	Status    string  `json:"status"`
	LatencyMs float64 `json:"latencyMs"`
	Error     string  `json:"error,omitempty"`
	IsHealthy bool    `json:"isHealthy"`
}

type DeepProbeConfig struct {
	Service   string
	Host      string
	Port      int
	Timeout   time.Duration
	Container string
	Profiles  []string

	Username string
	Password string
	Database string

	KafkaTopic   string
	GrafanaURL   string
	GrafanaUser  string
	GrafanaPass  string
	TemporalNS   string
	OtelGRPCPort int
	ClickHouseDB string
}

type DeepHealthReport struct {
	Healthy      bool                `json:"healthy"`
	CheckedCount int                 `json:"checkedCount"`
	HealthyCount int                 `json:"healthyCount"`
	Results      []SingleProbeResult `json:"results"`
	ReportedAt   string              `json:"reportedAt"`
}

func DefaultDeepProbeConfigs(primaryHost string) []DeepProbeConfig {
	if primaryHost == "" {
		primaryHost = "localhost"
	}
	return []DeepProbeConfig{
		{Service: "alloydb", Host: primaryHost, Port: 31420, Timeout: 5 * time.Second, Container: "alloydb", Username: "admin", Password: "llmobs_s3cret_2026", Database: "llm_observability", Profiles: []string{"alloydb", "db", "workflows", "stateful", "full"}},
		{Service: "redis", Host: primaryHost, Port: 31413, Timeout: 5 * time.Second, Container: "redis-ledger", Password: "llmobs_redis_s3cret_2024", Profiles: []string{"redis", "db", "stateful", "full"}},
		{Service: "kafka", Host: primaryHost, Port: 31414, Timeout: 5 * time.Second, KafkaTopic: "llmobs-health-probe", Profiles: []string{"kafka", "streaming", "stateful", "full"}},
		{Service: "clickhouse", Host: primaryHost, Port: 31421, Timeout: 5 * time.Second, Username: "default", Password: "llmobs_clickhouse_s3cret_2026", ClickHouseDB: "llm_observability", Profiles: []string{"clickhouse", "analytics", "stateful", "full"}},
		{Service: "grafana", Host: "localhost", Port: 31415, Timeout: 5 * time.Second, GrafanaURL: "http://localhost:31415", GrafanaUser: "admin", GrafanaPass: "llmobs_admin_password", Profiles: []string{"grafana", "tracing", "stateless", "full"}},
		{Service: "tempo", Host: primaryHost, Port: 31416, Timeout: 5 * time.Second, Profiles: []string{"tempo", "tracing", "stateless", "full"}},
		{Service: "temporal", Host: primaryHost, Port: 31424, Timeout: 5 * time.Second, TemporalNS: "default", Profiles: []string{"temporal", "workflows", "stateless", "full"}},
		{Service: "otel-collector", Host: "localhost", Port: 31417, Timeout: 5 * time.Second, OtelGRPCPort: 31418, Profiles: []string{"otel", "otel-collector", "tracing", "stateless", "full"}},
		{Service: "traefik", Host: "localhost", Port: 31410, Timeout: 5 * time.Second, Profiles: []string{"traefik", "network", "stateless", "full"}},
		{Service: "service-registry", Host: "localhost", Port: 31426, Timeout: 5 * time.Second, Profiles: []string{"service-registry", "network", "stateless", "full"}},
	}
}

// DeepProbeConfigsForProfiles returns only the probe configs whose profile tags
// intersect the given set of active Docker Compose profiles.
// An empty profiles slice returns all configs.
func DeepProbeConfigsForProfiles(primaryHost string, activeProfiles []string) []DeepProbeConfig {
	all := DefaultDeepProbeConfigs(primaryHost)
	if len(activeProfiles) == 0 {
		return all
	}
	activeSet := make(map[string]struct{}, len(activeProfiles))
	for _, p := range activeProfiles {
		activeSet[p] = struct{}{}
	}
	var out []DeepProbeConfig
	for _, c := range all {
		for _, p := range c.Profiles {
			if _, ok := activeSet[p]; ok {
				out = append(out, c)
				break
			}
		}
	}
	return out
}
