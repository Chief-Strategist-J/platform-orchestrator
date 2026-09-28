package unit

import (
	"context"
	"testing"

	grafanaSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/schema"
	grafanaService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/services"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/infra/observability"
)

func TestGrafanaDatasourceResolution(t *testing.T) {
	tracer := observability.NewOTelTracerAdapter("test-tracer")
	svc := grafanaService.NewGrafanaService(tracer, "/tmp")

	opts := grafanaSchema.DatasourceSyncOptions{
		Services: []string{"alloydb", "clickhouse", "redis", "tempo"},
	}

	report, err := svc.SyncDatasources(context.Background(), opts)
	if err == nil {
		t.Logf("Sync completed: total=%d, success=%d", report.TotalCount, report.SuccessCount)
	}
}
