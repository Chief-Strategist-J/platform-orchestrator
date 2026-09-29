/*
Package rules provides declarative validation and normalization rules for Grafana datasources, dashboards, and alerts.

ALGORITHM BLUEPRINT:
1. NormalizeDatasourcePayload: Ensures essential fields (Name, Type, Access, URL) are populated with valid defaults.
2. NormalizeAlertRulePayload: Trims whitespace and ensures maps are initialized.
3. NormalizeContactPointPayload: Trims whitespace and ensures settings map is initialized.
4. ValidateDatasourcePayload: Evaluates mandatory payload invariants.
5. ValidateDashboardPayload: Asserts dashboard object structure and required title metadata.
6. ValidateAlertRulePayload: Asserts rule title, folder UID, rule group, and condition queries.
7. ValidateContactPointPayload: Asserts contact point name and supported notification type.
8. Invariants:
   - Zero inline comments inside function bodies.
   - Access mode defaults to 'proxy' when omitted.
   - All string attributes trimmed of trailing/leading whitespace.
*/
package rules

import (
	"fmt"
	"strings"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/schema"
)

func NormalizeDatasourcePayload(payload schema.DatasourcePayload) schema.DatasourcePayload {
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Type = strings.TrimSpace(payload.Type)
	payload.URL = strings.TrimSpace(payload.URL)

	if payload.Access == "" {
		payload.Access = "proxy"
	}
	if payload.JSONData == nil {
		payload.JSONData = make(map[string]interface{})
	}
	if payload.SecureJSONData == nil {
		payload.SecureJSONData = make(map[string]string)
	}

	return payload
}

func NormalizeAlertRulePayload(rule schema.AlertRulePayload) schema.AlertRulePayload {
	rule.Title = strings.TrimSpace(rule.Title)
	rule.RuleGroup = strings.TrimSpace(rule.RuleGroup)
	rule.FolderUID = strings.TrimSpace(rule.FolderUID)
	rule.Condition = strings.TrimSpace(rule.Condition)

	if rule.Annotations == nil {
		rule.Annotations = make(map[string]string)
	}
	if rule.Labels == nil {
		rule.Labels = make(map[string]string)
	}

	return rule
}

func NormalizeContactPointPayload(cp schema.ContactPointPayload) schema.ContactPointPayload {
	cp.Name = strings.TrimSpace(cp.Name)
	cp.Type = strings.TrimSpace(cp.Type)

	if cp.Settings == nil {
		cp.Settings = make(map[string]interface{})
	}

	return cp
}

func ValidateDatasourcePayload(payload schema.DatasourcePayload) error {
	if payload.Name == "" {
		return fmt.Errorf("datasource name is required and cannot be empty")
	}
	if payload.Type == "" {
		return fmt.Errorf("datasource type is required (e.g. postgres, grafana-postgresql-datasource, grafana-clickhouse-datasource, redis-datasource, tempo, prometheus)")
	}
	if payload.URL == "" {
		return fmt.Errorf("datasource URL/address is required")
	}
	return nil
}

func ValidateDashboardPayload(payload schema.DashboardPayload) error {
	if payload.Dashboard == nil {
		return fmt.Errorf("dashboard payload must contain a valid 'dashboard' JSON object")
	}
	title, _ := payload.Dashboard["title"].(string)
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("dashboard JSON must specify a non-empty 'title' field")
	}
	return nil
}

func ValidateAlertRulePayload(rule schema.AlertRulePayload) error {
	if strings.TrimSpace(rule.Title) == "" {
		return fmt.Errorf("alert rule title is required")
	}
	if strings.TrimSpace(rule.RuleGroup) == "" {
		return fmt.Errorf("alert ruleGroup is required")
	}
	if strings.TrimSpace(rule.FolderUID) == "" {
		return fmt.Errorf("alert folderUID is required")
	}
	if len(rule.Data) == 0 {
		return fmt.Errorf("alert rule must contain at least one query/expression data item")
	}
	return nil
}

func ValidateContactPointPayload(cp schema.ContactPointPayload) error {
	if strings.TrimSpace(cp.Name) == "" {
		return fmt.Errorf("contact point name is required")
	}
	if strings.TrimSpace(cp.Type) == "" {
		return fmt.Errorf("contact point type is required (e.g. slack, webhook, email, pagerduty, opsgenie, discord)")
	}
	if cp.Settings == nil {
		return fmt.Errorf("contact point settings map is required")
	}
	return nil
}
