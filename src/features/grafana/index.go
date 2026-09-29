/*
Package grafana exports the public facade, domain services, and contracts for the Grafana feature domain.

ALGORITHM BLUEPRINT:
1. Public Facade: Exposes GrafanaService, DatasourceService, DashboardService, AlertService and canonical Schema/Types models.
2. Invariants:
   - Zero inline comments inside function bodies.
   - Internal implementation details remain encapsulated.
*/
package grafana

import (
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/schema"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/services"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/types"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

type (
	DatasourcePayload      = schema.DatasourcePayload
	DatasourceSyncOptions  = schema.DatasourceSyncOptions
	SingleDatasourceResult = schema.SingleDatasourceResult
	DatasourceSyncReport   = schema.DatasourceSyncReport
	DashboardPayload       = schema.DashboardPayload
	DashboardDetail        = schema.DashboardDetail
	DashboardImportOptions = schema.DashboardImportOptions
	AlertRulePayload       = schema.AlertRulePayload
	ContactPointPayload    = schema.ContactPointPayload
	ClientOptions          = types.ClientOptions
	DatasourceHealthResult = types.DatasourceHealthResult
	DeleteDatasourceResult = types.DeleteDatasourceResult
	DashboardSearchResult  = types.DashboardSearchResult
	DashboardSaveResult    = types.DashboardSaveResult
	DashboardDeleteResult  = types.DashboardDeleteResult
	AlertOperationResult   = types.AlertOperationResult

	GrafanaService    = services.GrafanaService
	DatasourceService = services.DatasourceService
	DashboardService  = services.DashboardService
	AlertService      = services.AlertService
)

func NewGrafanaService(tracer ports.TracerPort, baseDir string) *services.GrafanaService {
	return services.NewGrafanaService(tracer, baseDir)
}

func NewDatasourceService(tracer ports.TracerPort, baseDir string) *services.DatasourceService {
	return services.NewDatasourceService(tracer, baseDir)
}

func NewDashboardService(tracer ports.TracerPort, baseDir string) *services.DashboardService {
	return services.NewDashboardService(tracer, baseDir)
}

func NewAlertService(tracer ports.TracerPort, baseDir string) *services.AlertService {
	return services.NewAlertService(tracer, baseDir)
}
