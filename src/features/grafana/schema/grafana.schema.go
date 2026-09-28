/*
Package schema defines data contracts for Grafana datasource provisioning, mutations, and verification.

ALGORITHM BLUEPRINT:
1. DatasourcePayload: Flexible schema accommodating all standard and custom Grafana datasource types (PostgreSQL, ClickHouse, Redis, Tempo, Prometheus, Elasticsearch, Loki, MySQL, etc.).
2. DatasourceSyncOptions: Batch synchronization parameters supporting built-in service templates and custom payloads.
3. SingleDatasourceResult: Result representation of individual datasource operations (Create, Update, Sync).
4. DatasourceSyncReport: Aggregated report container for multi-datasource sync pipelines.
5. Invariants:
   - Zero inline comments inside function bodies.
   - Credentials default to workspace platform .env values when not explicitly overridden.
   - Safe defaults and nil-safe maps applied across all payload operations.
*/
package schema

import "time"

type DatasourcePayload struct {
	ID             int64                  `json:"id,omitempty"`
	UID            string                 `json:"uid,omitempty"`
	OrgID          int64                  `json:"orgId,omitempty"`
	Name           string                 `json:"name"`
	Type           string                 `json:"type"`
	TypeName       string                 `json:"typeName,omitempty"`
	Access         string                 `json:"access"`
	URL            string                 `json:"url"`
	User           string                 `json:"user,omitempty"`
	Database       string                 `json:"database,omitempty"`
	BasicAuth      bool                   `json:"basicAuth"`
	BasicAuthUser  string                 `json:"basicAuthUser,omitempty"`
	IsDefault      bool                   `json:"isDefault"`
	ReadOnly       bool                   `json:"readOnly,omitempty"`
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
	DatasourceID   int64   `json:"datasourceId,omitempty"`
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
