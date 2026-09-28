/*
Package types defines domain and transport models for Grafana datasource operations.

ALGORITHM BLUEPRINT:
1. ClientOptions: Transport authentication, endpoint resolution, and HTTP timeout controls.
2. DatasourceHealthResult: Standardized health probe result from Grafana /api/datasources/uid/:uid/health.
3. Invariants:
   - Zero inline comments inside function bodies.
   - PII and credentials must never leak into log strings or public error targets.
   - All models support deterministic JSON serialization.
*/
package types

import "time"

type ClientOptions struct {
	GrafanaURL string        `json:"grafanaUrl"`
	Username   string        `json:"username"`
	Password   string        `json:"password"`
	Timeout    time.Duration `json:"timeout"`
}

type DatasourceHealthResult struct {
	UID       string  `json:"uid"`
	Name      string  `json:"name,omitempty"`
	Status    string  `json:"status"`
	Message   string  `json:"message"`
	LatencyMs float64 `json:"latencyMs"`
	IsHealthy bool    `json:"isHealthy"`
}

type DeleteDatasourceResult struct {
	UID     string `json:"uid"`
	Message string `json:"message"`
	Success bool   `json:"success"`
}
