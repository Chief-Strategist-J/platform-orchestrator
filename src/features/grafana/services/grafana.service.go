/*
Package services provides the unified Grafana feature domain facade, coordinating specialized sub-domain services.

ALGORITHM BLUEPRINT (GrafanaService Facade):
1. Single Responsibility Decomposition:
   - Datasource Operations: Delegated to DatasourceService.
   - Dashboard Operations: Delegated to DashboardService.
   - Alert & Contact Point Operations: Delegated to AlertService.
2. Endpoint Decoupling: Fully decoupled via client.GrafanaClient and endpoints package.
3. OpenTelemetry Integration: Passes tracer context seamlessly across all specialized service invocations.
4. Invariants:
   - Zero inline comments inside function bodies.
   - 100% backward compatibility preserved across CLI commands, REST handlers, and unit tests.
*/
package services

import (
	"context"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/schema"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/types"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

type GrafanaService struct {
	datasources *DatasourceService
	dashboards  *DashboardService
	alerts      *AlertService
}

func NewGrafanaService(tracer ports.TracerPort, baseDir string) *GrafanaService {
	return &GrafanaService{
		datasources: NewDatasourceService(tracer, baseDir),
		dashboards:  NewDashboardService(tracer, baseDir),
		alerts:      NewAlertService(tracer, baseDir),
	}
}

func (s *GrafanaService) Datasources() *DatasourceService {
	return s.datasources
}

func (s *GrafanaService) Dashboards() *DashboardService {
	return s.dashboards
}

func (s *GrafanaService) Alerts() *AlertService {
	return s.alerts
}

func (s *GrafanaService) ListDatasources(ctx context.Context, opts types.ClientOptions) ([]schema.DatasourcePayload, error) {
	return s.datasources.List(ctx, opts)
}

func (s *GrafanaService) GetDatasource(ctx context.Context, opts types.ClientOptions, idOrNameOrUID string) (*schema.DatasourcePayload, error) {
	return s.datasources.Get(ctx, opts, idOrNameOrUID)
}

func (s *GrafanaService) CreateDatasource(ctx context.Context, opts types.ClientOptions, payload schema.DatasourcePayload) (*schema.SingleDatasourceResult, error) {
	return s.datasources.Create(ctx, opts, payload)
}

func (s *GrafanaService) UpdateDatasource(ctx context.Context, opts types.ClientOptions, idOrUID string, payload schema.DatasourcePayload) (*schema.SingleDatasourceResult, error) {
	return s.datasources.Update(ctx, opts, idOrUID, payload)
}

func (s *GrafanaService) DeleteDatasource(ctx context.Context, opts types.ClientOptions, idOrUIDOrName string) (*types.DeleteDatasourceResult, error) {
	return s.datasources.Delete(ctx, opts, idOrUIDOrName)
}

func (s *GrafanaService) TestDatasourceHealth(ctx context.Context, opts types.ClientOptions, idOrUIDOrName string) (*types.DatasourceHealthResult, error) {
	return s.datasources.TestHealth(ctx, opts, idOrUIDOrName)
}

func (s *GrafanaService) SyncDatasources(ctx context.Context, opts schema.DatasourceSyncOptions) (*schema.DatasourceSyncReport, error) {
	return s.datasources.Sync(ctx, opts)
}

func (s *GrafanaService) SearchDashboards(ctx context.Context, opts types.ClientOptions, query, folderUID, tag string) ([]types.DashboardSearchResult, error) {
	return s.dashboards.Search(ctx, opts, query, folderUID, tag)
}

func (s *GrafanaService) GetDashboard(ctx context.Context, opts types.ClientOptions, uid string) (*schema.DashboardDetail, error) {
	return s.dashboards.Get(ctx, opts, uid)
}

func (s *GrafanaService) CreateOrUpdateDashboard(ctx context.Context, opts types.ClientOptions, payload schema.DashboardPayload) (*types.DashboardSaveResult, error) {
	return s.dashboards.CreateOrUpdate(ctx, opts, payload)
}

func (s *GrafanaService) ImportDashboard(ctx context.Context, opts types.ClientOptions, importOpts schema.DashboardImportOptions) (*types.DashboardSaveResult, error) {
	return s.dashboards.Import(ctx, opts, importOpts)
}

func (s *GrafanaService) DeleteDashboard(ctx context.Context, opts types.ClientOptions, uid string) (*types.DashboardDeleteResult, error) {
	return s.dashboards.Delete(ctx, opts, uid)
}

func (s *GrafanaService) ListAlertRules(ctx context.Context, opts types.ClientOptions) ([]schema.AlertRulePayload, error) {
	return s.alerts.ListAlertRules(ctx, opts)
}

func (s *GrafanaService) GetAlertRule(ctx context.Context, opts types.ClientOptions, uid string) (*schema.AlertRulePayload, error) {
	return s.alerts.GetAlertRule(ctx, opts, uid)
}

func (s *GrafanaService) CreateOrUpdateAlertRule(ctx context.Context, opts types.ClientOptions, rule schema.AlertRulePayload) (*types.AlertOperationResult, error) {
	return s.alerts.CreateOrUpdateAlertRule(ctx, opts, rule)
}

func (s *GrafanaService) DeleteAlertRule(ctx context.Context, opts types.ClientOptions, uid string) (*types.AlertOperationResult, error) {
	return s.alerts.DeleteAlertRule(ctx, opts, uid)
}

func (s *GrafanaService) ListContactPoints(ctx context.Context, opts types.ClientOptions) ([]schema.ContactPointPayload, error) {
	return s.alerts.ListContactPoints(ctx, opts)
}

func (s *GrafanaService) CreateOrUpdateContactPoint(ctx context.Context, opts types.ClientOptions, cp schema.ContactPointPayload) (*types.AlertOperationResult, error) {
	return s.alerts.CreateOrUpdateContactPoint(ctx, opts, cp)
}

func (s *GrafanaService) DeleteContactPoint(ctx context.Context, opts types.ClientOptions, uid string) (*types.AlertOperationResult, error) {
	return s.alerts.DeleteContactPoint(ctx, opts, uid)
}

func (s *GrafanaService) TestContactPoint(ctx context.Context, opts types.ClientOptions, cp schema.ContactPointPayload) (*types.AlertOperationResult, error) {
	return s.alerts.TestContactPoint(ctx, opts, cp)
}
