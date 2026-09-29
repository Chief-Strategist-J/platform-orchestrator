/*
Package services provides isolated, domain-specific services for Grafana lifecycle management.

ALGORITHM BLUEPRINT (DashboardService):
1. Dashboard Operations: Search, Get by UID, Save/Update, Import from file/URL, Export to JSON, Delete by UID.
2. Endpoint Decoupling: Uses centralized typed endpoint constants from the endpoints package.
3. Declarative Source Loaders: Delegates source loading to DashboardLoaderRegistry (Rule 1: Checking WHAT something is -> Registry).
4. OpenTelemetry Tracing: Wraps every public operation in an attributed span.
5. Invariants:
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
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/schema"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/types"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/paths"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

type DashboardService struct {
	tracer   ports.TracerPort
	baseDir  string
	resolver *paths.PathResolver
	loaders  *DashboardLoaderRegistry
}

func NewDashboardService(tracer ports.TracerPort, baseDir string) *DashboardService {
	return &DashboardService{
		tracer:   tracer,
		baseDir:  baseDir,
		resolver: paths.NewPathResolver(baseDir),
		loaders:  NewDashboardLoaderRegistry(),
	}
}

func (s *DashboardService) Search(ctx context.Context, opts types.ClientOptions, query, folderUID, tag string) ([]types.DashboardSearchResult, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.dashboards.search")
	defer endSpan()

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
		return nil, err
	}

	return results, nil
}

func (s *DashboardService) Get(ctx context.Context, opts types.ClientOptions, uid string) (*schema.DashboardDetail, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.dashboards.get")
	defer endSpan()

	c := client.NewGrafanaClient(opts, s.resolver)
	dashboardPath := endpoints.BuildDashboardUIDPath(uid)

	var detail schema.DashboardDetail
	_, err := c.Do(ctx, http.MethodGet, dashboardPath, nil, &detail)
	if err != nil {
		return nil, err
	}

	return &detail, nil
}

func (s *DashboardService) CreateOrUpdate(ctx context.Context, opts types.ClientOptions, payload schema.DashboardPayload) (*types.DashboardSaveResult, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.dashboards.save")
	defer endSpan()

	c := client.NewGrafanaClient(opts, s.resolver)
	start := time.Now()

	var saveResult types.DashboardSaveResult
	_, err := c.Do(ctx, http.MethodPost, endpoints.EndpointDashboardsDB, payload, &saveResult)
	saveResult.Latency = float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil {
		return nil, err
	}

	return &saveResult, nil
}

func (s *DashboardService) Import(ctx context.Context, opts types.ClientOptions, importOpts schema.DashboardImportOptions) (*types.DashboardSaveResult, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.dashboards.import")
	defer endSpan()

	rawJSON, err := s.loaders.Load(ctx, importOpts.SourcePathOrURL, s.baseDir)
	if err != nil {
		return nil, err
	}

	var dashMap map[string]interface{}
	if err := json.Unmarshal(rawJSON, &dashMap); err != nil {
		return nil, fmt.Errorf("invalid dashboard JSON format: %w", err)
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
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.dashboards.delete")
	defer endSpan()

	c := client.NewGrafanaClient(opts, s.resolver)
	dashboardPath := endpoints.BuildDashboardUIDPath(uid)

	var deleteResponse struct {
		Title   string `json:"title"`
		Message string `json:"message"`
	}

	_, err := c.Do(ctx, http.MethodDelete, dashboardPath, nil, &deleteResponse)
	if err != nil {
		return &types.DashboardDeleteResult{
			Title:   uid,
			Message: fmt.Sprintf("Failed to delete dashboard: %v", err),
			Success: false,
		}, err
	}

	return &types.DashboardDeleteResult{
		Title:   deleteResponse.Title,
		Message: deleteResponse.Message,
		Success: true,
	}, nil
}
