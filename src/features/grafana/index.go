/*
Package grafana exports the public facade and contracts for the Grafana feature domain.

ALGORITHM BLUEPRINT:
1. Public Facade: Exposes GrafanaService and canonical Schema/Types models.
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
	ClientOptions          = types.ClientOptions
	DatasourceHealthResult = types.DatasourceHealthResult
	DeleteDatasourceResult = types.DeleteDatasourceResult
	GrafanaService         = services.GrafanaService
)

func NewGrafanaService(tracer ports.TracerPort, baseDir string) *services.GrafanaService {
	return services.NewGrafanaService(tracer, baseDir)
}
