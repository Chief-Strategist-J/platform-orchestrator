/*
Package services provides isolated, domain-specific services for Grafana lifecycle management with deep OpenTelemetry tracing.

ALGORITHM BLUEPRINT (DashboardService):
1. Declarative Resource Descriptor: Configures Dashboard endpoints, OTel span prefixes, and validation rules as a data descriptor.
2. Declarative Source Loaders: Delegates source loading to DashboardLoaderRegistry with traced loader spans.
3. Deep Observability: Wraps search, get, import, and delete operations in attributed spans with context propagation.
4. Invariants:
   - Zero inline comments inside function bodies.
   - Non-200 responses return descriptive error envelopes.
*/
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/client"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/endpoints"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/rules"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/schema"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/types"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/infra/observability"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/paths"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

type DashboardService struct {
	tracer    ports.TracerPort
	baseDir   string
	resolver  *paths.PathResolver
	loaders   *DashboardLoaderRegistry
	dashDesc  ResourceDescriptor[schema.DashboardPayload]
}

func NewDashboardService(tracer ports.TracerPort, baseDir string) *DashboardService {
	return &DashboardService{
		tracer:   tracer,
		baseDir:  baseDir,
		resolver: paths.NewPathResolver(baseDir),
		loaders:  NewDashboardLoaderRegistry(tracer),
		dashDesc: ResourceDescriptor[schema.DashboardPayload]{
			ResourceName:       "Dashboard",
			CollectionEndpoint: endpoints.EndpointDashboardsDB,
			ItemEndpointFunc:   endpoints.BuildDashboardUIDPath,
			SpanPrefix:         "grafana.dashboards",
			RuleSet:            rules.DashboardRules,
		},
	}
}

func (s *DashboardService) Search(ctx context.Context, opts types.ClientOptions, query, folderUID, tag string) ([]types.DashboardSearchResult, error) {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "grafana.dashboards.search", map[string]interface{}{
		"search.query":      query,
		"search.folder_uid": folderUID,
		"search.tag":        tag,
	})
	defer span.End()

	c := client.NewGrafanaClient(opts, s.resolver)

	params := url.Values{}
	params.Set("type", "dash-db")
	if query != "" {
		params.Set("query", query)
	}
	if folderUID != "" {
		params.Set("folderUids", folderUID)
	}
	if tag != "" {
		params.Set("tag", tag)
	}

	searchEndpoint := fmt.Sprintf("%s?%s", endpoints.EndpointSearch, params.Encode())
	var results []types.DashboardSearchResult

	_, err := c.Do(ctx, http.MethodGet, searchEndpoint, nil, &results)
	if err != nil {
		observability.RecordError(ctx, err)
		return nil, err
	}

	observability.SetAttribute(ctx, "search.results_count", len(results))
	observability.AddEvent(ctx, "grafana.dashboards_found", map[string]interface{}{
		"count": len(results),
	})

	return results, nil
}

func (s *DashboardService) Get(ctx context.Context, opts types.ClientOptions, uid string) (*schema.DashboardDetail, error) {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "grafana.dashboards.get", map[string]interface{}{
		"dashboard.uid": uid,
	})
	defer span.End()

	c := client.NewGrafanaClient(opts, s.resolver)
	dashboardPath := endpoints.BuildDashboardUIDPath(uid)

	var detail schema.DashboardDetail
	_, err := c.Do(ctx, http.MethodGet, dashboardPath, nil, &detail)
	if err != nil {
		observability.RecordError(ctx, err)
		return nil, err
	}

	if title, ok := detail.Dashboard["title"].(string); ok {
		observability.SetAttribute(ctx, "dashboard.title", title)
	}
	if slug, ok := detail.Meta["slug"].(string); ok {
		observability.SetAttribute(ctx, "dashboard.slug", slug)
	}

	return &detail, nil
}

func (s *DashboardService) CreateOrUpdate(ctx context.Context, opts types.ClientOptions, payload schema.DashboardPayload) (*types.DashboardSaveResult, error) {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "grafana.dashboards.save", map[string]interface{}{
		"dashboard.folder_uid": payload.FolderUID,
		"dashboard.overwrite":  payload.Overwrite,
	})
	defer span.End()

	if err := rules.ValidateDashboardPayload(payload); err != nil {
		observability.RecordError(ctx, err)
		return nil, err
	}

	c := client.NewGrafanaClient(opts, s.resolver)
	start := time.Now()

	var saveResult types.DashboardSaveResult
	_, err := c.Do(ctx, http.MethodPost, endpoints.EndpointDashboardsDB, payload, &saveResult)
	saveResult.Latency = float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil {
		observability.RecordError(ctx, err)
		return nil, err
	}

	observability.SetAttributes(ctx, map[string]interface{}{
		"dashboard.id":      saveResult.ID,
		"dashboard.uid":     saveResult.UID,
		"dashboard.url":     saveResult.URL,
		"dashboard.status":  saveResult.Status,
		"dashboard.latency": saveResult.Latency,
	})
	observability.AddEvent(ctx, "grafana.dashboard_saved", map[string]interface{}{
		"uid":    saveResult.UID,
		"status": saveResult.Status,
	})

	return &saveResult, nil
}

func (s *DashboardService) Import(ctx context.Context, opts types.ClientOptions, importOpts schema.DashboardImportOptions) (*types.DashboardSaveResult, error) {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "grafana.dashboards.import", map[string]interface{}{
		"import.source":     importOpts.SourcePathOrURL,
		"import.folder_uid": importOpts.FolderUID,
		"import.overwrite":  importOpts.Overwrite,
	})
	defer span.End()

	rawJSON, err := s.loaders.Load(ctx, importOpts.SourcePathOrURL, s.baseDir)
	if err != nil {
		observability.RecordError(ctx, err)
		return nil, err
	}

	var dashMap map[string]interface{}
	if err := json.Unmarshal(rawJSON, &dashMap); err != nil {
		unmarshalErr := fmt.Errorf("invalid dashboard JSON format: %w", err)
		observability.RecordError(ctx, unmarshalErr)
		return nil, unmarshalErr
	}

	if nested, ok := dashMap["dashboard"].(map[string]interface{}); ok {
		dashMap = nested
	}

	if importOpts.TitleOverride != "" {
		dashMap["title"] = importOpts.TitleOverride
	}

	payload := schema.DashboardPayload{
		Dashboard: dashMap,
		FolderUID: importOpts.FolderUID,
		Overwrite: importOpts.Overwrite,
		Message:   "Imported via Platform Orchestrator",
	}

	return s.CreateOrUpdate(ctx, opts, payload)
}

func (s *DashboardService) Delete(ctx context.Context, opts types.ClientOptions, uid string) (*types.DashboardDeleteResult, error) {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "grafana.dashboards.delete", map[string]interface{}{
		"dashboard.uid": uid,
	})
	defer span.End()

	c := client.NewGrafanaClient(opts, s.resolver)
	res, err := ExecuteDelete(ctx, c, s.tracer, s.dashDesc, uid)
	if err != nil {
		observability.RecordError(ctx, err)
		return &types.DashboardDeleteResult{
			Title:   uid,
			Message: res.Message,
			Success: false,
		}, err
	}
	return &types.DashboardDeleteResult{
		Title:   uid,
		Message: res.Message,
		Success: true,
	}, nil
}
