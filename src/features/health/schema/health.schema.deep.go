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
	Service  string
	Host     string
	Port     int
	Timeout  time.Duration
	Container string

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
		{Service: "alloydb", Host: primaryHost, Port: 31420, Timeout: 5 * time.Second, Container: "alloydb", Username: "admin", Password: "", Database: "llm_observability"},
		{Service: "redis", Host: primaryHost, Port: 31413, Timeout: 5 * time.Second, Container: "redis-ledger", Password: ""},
		{Service: "kafka", Host: primaryHost, Port: 31414, Timeout: 5 * time.Second, KafkaTopic: "llmobs-health-probe"},
		{Service: "clickhouse", Host: primaryHost, Port: 31421, Timeout: 5 * time.Second, ClickHouseDB: "llm_observability"},
		{Service: "grafana", Host: "localhost", Port: 31415, Timeout: 5 * time.Second, GrafanaURL: "http://localhost:31415", GrafanaUser: "admin", GrafanaPass: "admin"},
		{Service: "tempo", Host: primaryHost, Port: 31416, Timeout: 5 * time.Second},
		{Service: "temporal", Host: primaryHost, Port: 31424, Timeout: 5 * time.Second, TemporalNS: "default"},
		{Service: "otel-collector", Host: "localhost", Port: 31417, Timeout: 5 * time.Second, OtelGRPCPort: 31418},
		{Service: "traefik", Host: "localhost", Port: 31410, Timeout: 5 * time.Second},
		{Service: "service-registry", Host: "localhost", Port: 31426, Timeout: 5 * time.Second},
	}
}
