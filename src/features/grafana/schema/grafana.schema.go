/*
Package schema defines data contracts for Grafana datasource, dashboard, and alert provisioning and lifecycle management.

ALGORITHM BLUEPRINT:
1. DatasourcePayload: Flexible schema accommodating all standard and custom Grafana datasource types.
2. DatasourceSyncOptions: Batch synchronization parameters supporting built-in service templates and custom payloads.
3. DashboardPayload & DashboardImportOptions: Schema for creating, updating, importing, and exporting dashboards.
4. AlertRulePayload & ContactPointPayload: Schema for Grafana Unified Alerting rules and notification receivers.
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

type DashboardPayload struct {
	Dashboard map[string]interface{} `json:"dashboard"`
	FolderID  int64                  `json:"folderId,omitempty"`
	FolderUID string                 `json:"folderUid,omitempty"`
	Message   string                 `json:"message,omitempty"`
	Overwrite bool                   `json:"overwrite"`
}

type DashboardDetail struct {
	Meta      map[string]interface{} `json:"meta"`
	Dashboard map[string]interface{} `json:"dashboard"`
}

type DashboardImportOptions struct {
	SourcePathOrURL string `json:"sourcePathOrUrl"`
	FolderUID       string `json:"folderUid,omitempty"`
	Overwrite       bool   `json:"overwrite"`
	TitleOverride   string `json:"titleOverride,omitempty"`
}

type AlertQueryData struct {
	RefID             string                 `json:"refId"`
	QueryType         string                 `json:"queryType,omitempty"`
	RelativeTimeRange map[string]interface{} `json:"relativeTimeRange,omitempty"`
	DatasourceUID     string                 `json:"datasourceUid"`
	Model             map[string]interface{} `json:"model"`
}

type AlertRulePayload struct {
	ID           int64                  `json:"id,omitempty"`
	UID          string                 `json:"uid,omitempty"`
	OrgID        int64                  `json:"orgId,omitempty"`
	Title        string                 `json:"title"`
	RuleGroup    string                 `json:"ruleGroup"`
	FolderUID    string                 `json:"folderUID"`
	Condition    string                 `json:"condition"`
	Data         []AlertQueryData       `json:"data"`
	NoDataState  string                 `json:"noDataState,omitempty"`
	ExecErrState string                 `json:"execErrState,omitempty"`
	For          string                 `json:"for,omitempty"`
	Annotations  map[string]string      `json:"annotations,omitempty"`
	Labels       map[string]string      `json:"labels,omitempty"`
	IsPaused     bool                   `json:"isPaused,omitempty"`
}

type ContactPointPayload struct {
	UID                   string                 `json:"uid,omitempty"`
	Name                  string                 `json:"name"`
	Type                  string                 `json:"type"`
	Settings              map[string]interface{} `json:"settings"`
	DisableResolveMessage bool                   `json:"disableResolveMessage,omitempty"`
	Provenance            string                 `json:"provenance,omitempty"`
}
