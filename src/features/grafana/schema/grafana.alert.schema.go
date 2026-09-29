/*
Package schema defines data structures and contracts for Grafana Unified Alerting rules and contact points.

ALGORITHM BLUEPRINT:
1. AlertQueryData: Query and expression definition for an alert rule condition.
2. AlertRulePayload: Unified Alerting rule definition for metric evaluations.
3. ContactPointPayload: Notification receiver definition (Slack, Webhook, Email, PagerDuty, etc.).
4. Invariants:
   - Zero inline comments inside function bodies.
   - All fields support deterministic JSON marshaling.
*/
package schema

type AlertQueryData struct {
	RefID             string                 `json:"refId"`
	QueryType         string                 `json:"queryType,omitempty"`
	RelativeTimeRange map[string]interface{} `json:"relativeTimeRange,omitempty"`
	DatasourceUID     string                 `json:"datasourceUid"`
	Model             map[string]interface{} `json:"model"`
}

type AlertRulePayload struct {
	ID           int64             `json:"id,omitempty"`
	UID          string            `json:"uid,omitempty"`
	OrgID        int64             `json:"orgId,omitempty"`
	Title        string            `json:"title"`
	RuleGroup    string            `json:"ruleGroup"`
	FolderUID    string            `json:"folderUID"`
	Condition    string            `json:"condition"`
	Data         []AlertQueryData  `json:"data"`
	NoDataState  string            `json:"noDataState,omitempty"`
	ExecErrState string            `json:"execErrState,omitempty"`
	For          string            `json:"for,omitempty"`
	Annotations  map[string]string `json:"annotations,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	IsPaused     bool              `json:"isPaused,omitempty"`
}

type ContactPointPayload struct {
	UID                   string                 `json:"uid,omitempty"`
	Name                  string                 `json:"name"`
	Type                  string                 `json:"type"`
	Settings              map[string]interface{} `json:"settings"`
	DisableResolveMessage bool                   `json:"disableResolveMessage,omitempty"`
	Provenance            string                 `json:"provenance,omitempty"`
}
