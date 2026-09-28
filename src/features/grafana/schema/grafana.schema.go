/*
Package schema defines data contracts for Grafana datasource provisioning and configuration.

ALGORITHM BLUEPRINT:
1. DatasourceDefinition: Model for Grafana REST API datasource payload (type, URL, credentials, jsonData, secureJsonData).
2. DatasourceSyncOptions: Parameters for configuring datasources (Grafana URL, credentials, target services, test connection flag).
3. DatasourceSyncResult: Result of configuring an individual datasource (Service, DatasourceName, Status, Message, LatencyMs).
4. DatasourceSyncReport: Aggregated report of all processed datasources.
5. Invariants:
   - Zero inline comments inside function bodies.
   - Credentials default to workspace platform .env values.
   - Safe defaults applied when fields are omitted.
*/
package schema

import "time"

type DatasourcePayload struct {
	ID             int64                  `json:"id,omitempty"`
	UID            string                 `json:"uid,omitempty"`
	Name           string                 `json:"name"`
	Type           string                 `json:"type"`
	Access         string                 `json:"access"`
	URL            string                 `json:"url"`
	User           string                 `json:"user,omitempty"`
	Database       string                 `json:"database,omitempty"`
	BasicAuth      bool                   `json:"basicAuth"`
	IsDefault      bool                   `json:"isDefault"`
	JSONData       map[string]interface{} `json:"jsonData,omitempty"`
	SecureJSONData map[string]string      `json:"secureJsonData,omitempty"`
}

type DatasourceSyncOptions struct {
	GrafanaURL     string
	GrafanaUser    string
	GrafanaPass    string
	Services       []string
	TestConnection bool
	Timeout        time.Duration
}

type SingleDatasourceResult struct {
	Service        string  `json:"service"`
	DatasourceName string  `json:"datasourceName"`
	DatasourceUID  string  `json:"datasourceUid"`
	Status         string  `json:"status"`
	Message        string  `json:"message"`
	LatencyMs      float64 `json:"latencyMs"`
	IsHealthy      bool    `json:"isHealthy"`
}

type DatasourceSyncReport struct {
	TotalCount   int                      `json:"totalCount"`
	SuccessCount int                      `json:"successCount"`
	Results      []SingleDatasourceResult `json:"results"`
	ReportedAt   string                   `json:"reportedAt"`
}
