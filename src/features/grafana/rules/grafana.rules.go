/*
Package rules provides declarative validation and normalization rules for Grafana datasources.

ALGORITHM BLUEPRINT:
1. NormalizeDatasourcePayload: Ensures essential fields (Name, Type, Access, URL) are populated with valid defaults.
2. ValidateDatasourcePayload: Evaluates mandatory payload invariants, returning structured validation error messages if constraints are violated.
3. Invariants:
   - Zero inline comments inside function bodies.
   - Access mode defaults to 'proxy' when omitted.
   - Name and Type must not be blank strings.
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
