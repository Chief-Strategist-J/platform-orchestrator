/*
Package services provides isolated, domain-specific services for Grafana lifecycle management.

ALGORITHM BLUEPRINT (DatasourceService):
1. Declarative Resource Descriptor: Configures Datasource endpoints and validation rules as a data descriptor.
2. Declarative Template Registry: Resolves built-in and dynamic datasources from the declarative template data table.
3. Batch Sync Pipeline: Concurrently provisions platform datasources with health probing.
4. OpenTelemetry Tracing: Wraps every public operation in an attributed span.
5. Invariants:
   - Zero inline comments inside function bodies.
   - Non-200 responses return descriptive error envelopes.
*/
package services

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/client"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/endpoints"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/rules"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/schema"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/types"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/paths"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

type DatasourceService struct {
	tracer   ports.TracerPort
	baseDir  string
	resolver *paths.PathResolver
	registry *rules.DatasourceTemplateRegistry
	dsDesc   ResourceDescriptor[schema.DatasourcePayload]
}

func NewDatasourceService(tracer ports.TracerPort, baseDir string) *DatasourceService {
	return &DatasourceService{
		tracer:   tracer,
		baseDir:  baseDir,
		resolver: paths.NewPathResolver(baseDir),
		registry: rules.DefaultDatasourceRegistry,
		dsDesc: ResourceDescriptor[schema.DatasourcePayload]{
			ResourceName:       "Datasource",
			CollectionEndpoint: endpoints.EndpointDatasources,
			ItemEndpointFunc:   endpoints.BuildDatasourceUIDPath,
			SpanPrefix:         "grafana.datasources",
			RuleSet:            rules.DatasourceRules,
		},
	}
}

func (s *DatasourceService) List(ctx context.Context, opts types.ClientOptions) ([]schema.DatasourcePayload, error) {
	c := client.NewGrafanaClient(opts, s.resolver)
	return ExecuteList(ctx, c, s.tracer, s.dsDesc)
}

func (s *DatasourceService) Get(ctx context.Context, opts types.ClientOptions, idOrNameOrUID string) (*schema.DatasourcePayload, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.datasources.get")
	defer endSpan()

	c := client.NewGrafanaClient(opts, s.resolver)
	var ds schema.DatasourcePayload

	uidPath := endpoints.BuildDatasourceUIDPath(idOrNameOrUID)
	status, err := c.Do(ctx, http.MethodGet, uidPath, nil, &ds)
	if err == nil && status == http.StatusOK {
		return &ds, nil
	}

	namePath := endpoints.BuildDatasourceNamePath(idOrNameOrUID)
	status, err = c.Do(ctx, http.MethodGet, namePath, nil, &ds)
	if err == nil && status == http.StatusOK {
		return &ds, nil
	}

	return nil, fmt.Errorf("datasource %q not found in Grafana", idOrNameOrUID)
}

func (s *DatasourceService) Create(ctx context.Context, opts types.ClientOptions, payload schema.DatasourcePayload) (*schema.SingleDatasourceResult, error) {
	c := client.NewGrafanaClient(opts, s.resolver)
	res, err := ExecuteUpsert(ctx, c, s.tracer, s.dsDesc, "", payload)
	if err != nil {
		return &schema.SingleDatasourceResult{
			Service:        payload.Type,
			DatasourceName: payload.Name,
			Status:         "failed",
			Message:        err.Error(),
			LatencyMs:      res.LatencyMs,
			IsHealthy:      false,
		}, err
	}
	return &schema.SingleDatasourceResult{
		Service:        payload.Type,
		DatasourceName: payload.Name,
		DatasourceUID:  res.UID,
		Status:         "created",
		Message:        res.Message,
		LatencyMs:      res.LatencyMs,
		IsHealthy:      true,
	}, nil
}

func (s *DatasourceService) Update(ctx context.Context, opts types.ClientOptions, idOrUID string, payload schema.DatasourcePayload) (*schema.SingleDatasourceResult, error) {
	c := client.NewGrafanaClient(opts, s.resolver)
	res, err := ExecuteUpsert(ctx, c, s.tracer, s.dsDesc, idOrUID, payload)
	if err != nil {
		return &schema.SingleDatasourceResult{
			Service:        payload.Type,
			DatasourceName: payload.Name,
			DatasourceUID:  idOrUID,
			Status:         "failed",
			Message:        err.Error(),
			LatencyMs:      res.LatencyMs,
			IsHealthy:      false,
		}, err
	}
	return &schema.SingleDatasourceResult{
		Service:        payload.Type,
		DatasourceName: payload.Name,
		DatasourceUID:  idOrUID,
		Status:         "updated",
		Message:        res.Message,
		LatencyMs:      res.LatencyMs,
		IsHealthy:      true,
	}, nil
}

func (s *DatasourceService) Delete(ctx context.Context, opts types.ClientOptions, idOrUIDOrName string) (*types.DeleteDatasourceResult, error) {
	c := client.NewGrafanaClient(opts, s.resolver)
	res, err := ExecuteDelete(ctx, c, s.tracer, s.dsDesc, idOrUIDOrName)
	if err != nil {
		return &types.DeleteDatasourceResult{
			UID:     idOrUIDOrName,
			Message: res.Message,
			Success: false,
		}, err
	}
	return &types.DeleteDatasourceResult{
		UID:     idOrUIDOrName,
		Message: res.Message,
		Success: true,
	}, nil
}

func (s *DatasourceService) TestHealth(ctx context.Context, opts types.ClientOptions, idOrUIDOrName string) (*types.DatasourceHealthResult, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.datasources.health_test")
	defer endSpan()

	c := client.NewGrafanaClient(opts, s.resolver)
	start := time.Now()

	ds, err := s.Get(ctx, opts, idOrUIDOrName)
	if err != nil {
		return &types.DatasourceHealthResult{
			UID:       idOrUIDOrName,
			Status:    "error",
			Message:   fmt.Sprintf("Datasource not found: %v", err),
			LatencyMs: float64(time.Since(start).Microseconds()) / 1000.0,
			IsHealthy: false,
		}, err
	}

	healthPath := endpoints.BuildDatasourceHealthPath(ds.UID)
	var healthResp struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}

	_, err = c.Do(ctx, http.MethodGet, healthPath, nil, &healthResp)
	latency := float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil {
		return &types.DatasourceHealthResult{
			UID:       ds.UID,
			Name:      ds.Name,
			Status:    "error",
			Message:   err.Error(),
			LatencyMs: latency,
			IsHealthy: false,
		}, err
	}

	isHealthy := strings.EqualFold(healthResp.Status, "success") || strings.EqualFold(healthResp.Status, "ok")
	return &types.DatasourceHealthResult{
		UID:       ds.UID,
		Name:      ds.Name,
		Status:    healthResp.Status,
		Message:   healthResp.Message,
		LatencyMs: latency,
		IsHealthy: isHealthy,
	}, nil
}

func (s *DatasourceService) Sync(ctx context.Context, opts schema.DatasourceSyncOptions) (*schema.DatasourceSyncReport, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.datasources.sync")
	defer endSpan()

	servicesToSync := opts.Services
	if len(servicesToSync) == 0 {
		servicesToSync = []string{"alloydb", "clickhouse", "redis", "tempo"}
	}

	clientOpts := types.ClientOptions{
		GrafanaURL: opts.GrafanaURL,
		Username:   opts.GrafanaUser,
		Password:   opts.GrafanaPass,
		Timeout:    opts.Timeout,
	}
	c := client.NewGrafanaClient(clientOpts, s.resolver)

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []schema.SingleDatasourceResult
	)

	for _, svc := range servicesToSync {
		payload, ok := s.BuildDefaultPayload(svc)
		if !ok {
			continue
		}

		wg.Add(1)
		go func(svcName string, dsPayload schema.DatasourcePayload) {
			defer wg.Done()
			res := s.provisionSingle(ctx, c, dsPayload, opts.TestConnection)
			res.Service = svcName
			mu.Lock()
			results = append(results, res)
			mu.Unlock()
		}(svc, payload)
	}

	wg.Wait()

	successCount := 0
	for _, r := range results {
		if r.Status == "created" || r.Status == "updated" || r.Status == "ok" {
			successCount++
		}
	}

	return &schema.DatasourceSyncReport{
		TotalCount:   len(results),
		SuccessCount: successCount,
		Results:      results,
		ReportedAt:   time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (s *DatasourceService) BuildDefaultPayload(svc string) (schema.DatasourcePayload, bool) {
	return s.registry.Resolve(svc, s.resolver)
}

func (s *DatasourceService) TemplateRegistry() *rules.DatasourceTemplateRegistry {
	return s.registry
}

func (s *DatasourceService) provisionSingle(ctx context.Context, c *client.GrafanaClient, payload schema.DatasourcePayload, testConnection bool) schema.SingleDatasourceResult {
	res, err := ExecuteUpsert(ctx, c, s.tracer, s.dsDesc, payload.UID, payload)

	status := "created"
	if res.Status == "updated" {
		status = "updated"
	}
	if err != nil {
		status = "failed"
	}

	result := schema.SingleDatasourceResult{
		DatasourceName: payload.Name,
		DatasourceUID:  payload.UID,
		Status:         status,
		Message:        res.Message,
		LatencyMs:      res.LatencyMs,
		IsHealthy:      err == nil,
	}

	if testConnection && result.IsHealthy {
		healthPath := endpoints.BuildDatasourceHealthPath(payload.UID)
		var healthResp struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		}
		_, err := c.Do(ctx, http.MethodGet, healthPath, nil, &healthResp)
		if err != nil || (!strings.EqualFold(healthResp.Status, "success") && !strings.EqualFold(healthResp.Status, "ok")) {
			result.IsHealthy = false
			result.Status = "unreachable"
			if healthResp.Message != "" {
				result.Message = fmt.Sprintf("%s (Health check: %s)", result.Message, healthResp.Message)
			}
		}
	}

	return result
}
