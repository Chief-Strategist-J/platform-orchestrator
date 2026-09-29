package unit

import (
	"context"
	"testing"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/endpoints"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/schema"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/types"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/infra/observability"
)

func TestGrafanaSpecializedServices(t *testing.T) {
	tracer := observability.NewOTelTracerAdapter("test-grafana-tracer")
	baseDir := "/tmp"

	dsSvc := grafana.NewDatasourceService(tracer, baseDir)
	if dsSvc == nil {
		t.Fatal("expected DatasourceService to not be nil")
	}

	dashSvc := grafana.NewDashboardService(tracer, baseDir)
	if dashSvc == nil {
		t.Fatal("expected DashboardService to not be nil")
	}

	alertSvc := grafana.NewAlertService(tracer, baseDir)
	if alertSvc == nil {
		t.Fatal("expected AlertService to not be nil")
	}

	facade := grafana.NewGrafanaService(tracer, baseDir)
	if facade.Datasources() == nil || facade.Dashboards() == nil || facade.Alerts() == nil {
		t.Fatal("expected Facade to provide all specialized services")
	}

	payload, ok := dsSvc.BuildDefaultPayload("alloydb")
	if !ok || payload.Type != "postgres" {
		t.Fatalf("expected alloydb payload resolution, got ok=%v, type=%s", ok, payload.Type)
	}

	opts := schema.DatasourceSyncOptions{
		Services: []string{"alloydb", "clickhouse", "redis", "tempo"},
	}
	report, err := dsSvc.Sync(context.Background(), opts)
	if err == nil {
		t.Logf("Sync report: total=%d, success=%d", report.TotalCount, report.SuccessCount)
	}

	uidPath := endpoints.BuildDatasourceUIDPath("test-uid")
	if uidPath != "/api/datasources/uid/test-uid" {
		t.Fatalf("unexpected uid path: %s", uidPath)
	}
}

func TestGrafanaClientOptionsHandling(t *testing.T) {
	opts := types.ClientOptions{
		GrafanaURL: "http://127.0.0.1:31415",
		Username:   "admin",
		Password:   "admin",
	}

	tracer := observability.NewOTelTracerAdapter("test-grafana-tracer")
	svc := grafana.NewGrafanaService(tracer, "/tmp")

	_, _ = svc.ListDatasources(context.Background(), opts)
	_, _ = svc.SearchDashboards(context.Background(), opts, "test", "", "")
	_, _ = svc.ListAlertRules(context.Background(), opts)
	_, _ = svc.ListContactPoints(context.Background(), opts)
}
