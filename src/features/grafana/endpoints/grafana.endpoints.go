/*
Package endpoints declares typed API route constants and URL builders for the Grafana REST API.

ALGORITHM BLUEPRINT:
1. Endpoint Declarations: Centralized route templates for datasources, dashboards, alerting, contact points, and health.
2. URL Builders: Dynamic path formatting functions preventing raw inline string concatenations.
3. Invariants:
   - Zero inline comments inside function bodies.
   - Endpoints are immutable and never hardcoded in service implementations.
*/
package endpoints

import "fmt"

const (
	EndpointDatasources         = "/api/datasources"
	EndpointDatasourceByUID     = "/api/datasources/uid/%s"
	EndpointDatasourceByName    = "/api/datasources/name/%s"
	EndpointDatasourceHealth    = "/api/datasources/uid/%s/health"
	EndpointSearch              = "/api/search"
	EndpointDashboardsDB        = "/api/dashboards/db"
	EndpointDashboardByUID      = "/api/dashboards/uid/%s"
	EndpointAlertRules          = "/api/v1/provisioning/alert-rules"
	EndpointAlertRuleByUID      = "/api/v1/provisioning/alert-rules/%s"
	EndpointContactPoints       = "/api/v1/provisioning/contact-points"
	EndpointContactPointByUID   = "/api/v1/provisioning/contact-points/%s"
	EndpointContactPointsTest   = "/api/v1/provisioning/contact-points/test"
	EndpointFolders             = "/api/folders"
	EndpointHealth              = "/api/health"
)

func BuildDatasourceUIDPath(uid string) string {
	return fmt.Sprintf(EndpointDatasourceByUID, uid)
}

func BuildDatasourceNamePath(name string) string {
	return fmt.Sprintf(EndpointDatasourceByName, name)
}

func BuildDatasourceHealthPath(uid string) string {
	return fmt.Sprintf(EndpointDatasourceHealth, uid)
}

func BuildDashboardUIDPath(uid string) string {
	return fmt.Sprintf(EndpointDashboardByUID, uid)
}

func BuildAlertRuleUIDPath(uid string) string {
	return fmt.Sprintf(EndpointAlertRuleByUID, uid)
}

func BuildContactPointUIDPath(uid string) string {
	return fmt.Sprintf(EndpointContactPointByUID, uid)
}
