/*
Package types defines domain and transport models for external service and custom data service registrations.

ALGORITHM BLUEPRINT:
1. HealthProbeResult: Standardized health check report capturing latency, protocol status, and error details.
2. DeleteServiceResult: Domain result representation for service un-registration.
3. ServiceSyncResult: Result representation for syncing external data services into telemetry backends.
4. Invariants:
   - Zero inline comments inside function bodies.
   - PII and credentials must never leak into log strings or public error targets.
   - All models support deterministic JSON serialization.
*/
package types

import "time"

type HealthProbeResult struct {
	ServiceID string    `json:"serviceId"`
	Name      string    `json:"name"`
	Target    string    `json:"target"`
	ProbeType string    `json:"probeType"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	LatencyMs float64   `json:"latencyMs"`
	IsHealthy bool      `json:"isHealthy"`
	CheckedAt time.Time `json:"checkedAt"`
}

type DeleteServiceResult struct {
	ServiceID string `json:"serviceId"`
	Name      string `json:"name"`
	Message   string `json:"message"`
	Success   bool   `json:"success"`
}

type ServiceSyncResult struct {
	ServiceID     string  `json:"serviceId"`
	DatasourceUID string  `json:"datasourceUid,omitempty"`
	Message       string  `json:"message"`
	LatencyMs     float64 `json:"latencyMs"`
	Success       bool    `json:"success"`
}
