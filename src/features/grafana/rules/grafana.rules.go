/*
Package rules provides declarative validation and normalization rules for Grafana datasources, dashboards, and alerts.

ALGORITHM BLUEPRINT (Declarative Rule Sets):
1. DatasourceRules: Pipeline normalizing strings, access mode, JSON maps, and evaluating mandatory fields.
2. DashboardRules: Pipeline evaluating dashboard JSON presence and non-empty title.
3. AlertRuleRules: Pipeline normalizing folder UIDs, rule groups, and evaluating query data items.
4. ContactPointRules: Pipeline validating contact point names, types, and settings.
5. Invariants:
   - Zero inline comments inside function bodies.
   - All rules expressed as data inside declarative RuleSet definitions.
*/
package rules

import (
	"strings"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/schema"
)

var DatasourceRules = RuleSet[schema.DatasourcePayload]{
	{
		Name: "NormalizeAttributes",
		Mutator: func(p schema.DatasourcePayload) schema.DatasourcePayload {
			p.Name = strings.TrimSpace(p.Name)
			p.Type = strings.TrimSpace(p.Type)
			p.URL = strings.TrimSpace(p.URL)
			if p.Access == "" {
				p.Access = "proxy"
			}
			if p.JSONData == nil {
				p.JSONData = make(map[string]interface{})
			}
			if p.SecureJSONData == nil {
				p.SecureJSONData = make(map[string]string)
			}
			return p
		},
	},
	{
		Name:         "MandatoryName",
		Condition:    func(p schema.DatasourcePayload) bool { return p.Name != "" },
		ErrorMessage: "datasource name is required and cannot be empty",
	},
	{
		Name:         "MandatoryType",
		Condition:    func(p schema.DatasourcePayload) bool { return p.Type != "" },
		ErrorMessage: "datasource type is required (e.g. postgres, grafana-clickhouse-datasource, redis-datasource, tempo, prometheus, loki)",
	},
	{
		Name:         "MandatoryURL",
		Condition:    func(p schema.DatasourcePayload) bool { return p.URL != "" },
		ErrorMessage: "datasource URL/address is required",
	},
}

var DashboardRules = RuleSet[schema.DashboardPayload]{
	{
		Name:         "MandatoryDashboardMap",
		Condition:    func(p schema.DashboardPayload) bool { return p.Dashboard != nil },
		ErrorMessage: "dashboard payload must contain a valid 'dashboard' JSON object",
	},
	{
		Name: "MandatoryTitle",
		Condition: func(p schema.DashboardPayload) bool {
			if p.Dashboard == nil {
				return false
			}
			title, _ := p.Dashboard["title"].(string)
			return strings.TrimSpace(title) != ""
		},
		ErrorMessage: "dashboard JSON must specify a non-empty 'title' field",
	},
}

var AlertRuleRules = RuleSet[schema.AlertRulePayload]{
	{
		Name: "NormalizeAlertAttributes",
		Mutator: func(r schema.AlertRulePayload) schema.AlertRulePayload {
			r.Title = strings.TrimSpace(r.Title)
			r.RuleGroup = strings.TrimSpace(r.RuleGroup)
			r.FolderUID = strings.TrimSpace(r.FolderUID)
			r.Condition = strings.TrimSpace(r.Condition)
			if r.Annotations == nil {
				r.Annotations = make(map[string]string)
			}
			if r.Labels == nil {
				r.Labels = make(map[string]string)
			}
			return r
		},
	},
	{
		Name:         "MandatoryTitle",
		Condition:    func(r schema.AlertRulePayload) bool { return r.Title != "" },
		ErrorMessage: "alert rule title is required",
	},
	{
		Name:         "MandatoryRuleGroup",
		Condition:    func(r schema.AlertRulePayload) bool { return r.RuleGroup != "" },
		ErrorMessage: "alert ruleGroup is required",
	},
	{
		Name:         "MandatoryFolderUID",
		Condition:    func(r schema.AlertRulePayload) bool { return r.FolderUID != "" },
		ErrorMessage: "alert folderUID is required",
	},
	{
		Name:         "MandatoryQueryData",
		Condition:    func(r schema.AlertRulePayload) bool { return len(r.Data) > 0 },
		ErrorMessage: "alert rule must contain at least one query/expression data item",
	},
}

var ContactPointRules = RuleSet[schema.ContactPointPayload]{
	{
		Name: "NormalizeContactPointAttributes",
		Mutator: func(cp schema.ContactPointPayload) schema.ContactPointPayload {
			cp.Name = strings.TrimSpace(cp.Name)
			cp.Type = strings.TrimSpace(cp.Type)
			if cp.Settings == nil {
				cp.Settings = make(map[string]interface{})
			}
			return cp
		},
	},
	{
		Name:         "MandatoryName",
		Condition:    func(cp schema.ContactPointPayload) bool { return cp.Name != "" },
		ErrorMessage: "contact point name is required",
	},
	{
		Name:         "MandatoryType",
		Condition:    func(cp schema.ContactPointPayload) bool { return cp.Type != "" },
		ErrorMessage: "contact point type is required (e.g. slack, webhook, email, pagerduty, opsgenie, discord)",
	},
	{
		Name:         "MandatorySettings",
		Condition:    func(cp schema.ContactPointPayload) bool { return cp.Settings != nil },
		ErrorMessage: "contact point settings map is required",
	},
}

func NormalizeDatasourcePayload(payload schema.DatasourcePayload) schema.DatasourcePayload {
	return DatasourceRules.Normalize(payload)
}

func ValidateDatasourcePayload(payload schema.DatasourcePayload) error {
	return DatasourceRules.Validate(payload)
}

func ValidateDashboardPayload(payload schema.DashboardPayload) error {
	return DashboardRules.Validate(payload)
}

func NormalizeAlertRulePayload(rule schema.AlertRulePayload) schema.AlertRulePayload {
	return AlertRuleRules.Normalize(rule)
}

func ValidateAlertRulePayload(rule schema.AlertRulePayload) error {
	return AlertRuleRules.Validate(rule)
}

func NormalizeContactPointPayload(cp schema.ContactPointPayload) schema.ContactPointPayload {
	return ContactPointRules.Normalize(cp)
}

func ValidateContactPointPayload(cp schema.ContactPointPayload) error {
	return ContactPointRules.Validate(cp)
}
