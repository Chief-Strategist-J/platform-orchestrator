/*
Package services provides isolated, domain-specific services for Grafana lifecycle management.

ALGORITHM BLUEPRINT (DatasourceService):
1. Datasource Operations: List, Get, Create, Update, Delete, Health Probe, Batch Sync.
2. Endpoint Decoupling: Uses centralized typed endpoint constants from the endpoints package.
3. Declarative Template Registry: Resolves built-in and custom datasource definitions from rules.DefaultDatasourceRegistry (Rule 1: Registry pattern).
4. Declarative Validation: Enforces data normalization and invariants via rules.DatasourceRules (Rule 3: Rules as Data).
5. OpenTelemetry Tracing: Wraps every public operation in an attributed span.
6. Invariants:
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
}

func NewDatasourceService(tracer ports.TracerPort, baseDir string) *DatasourceService {
	return &DatasourceService{
		tracer:   tracer,
		baseDir:  baseDir,
		resolver: paths.NewPathResolver(baseDir),
		registry: rules.DefaultDatasourceRegistry,
	}
}

func (s *DatasourceService) List(ctx context.Context, opts types.ClientOptions) ([]schema.DatasourcePayload, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.datasources.list")
	defer endSpan()

	c := client.NewGrafanaClient(opts, s.resolver)
	var datasources []schema.DatasourcePayload
	_, err := c.Do(ctx, http.MethodGet, endpoints.EndpointDatasources, nil, &datasources)
	if err != nil {
		return nil, err
	}

	return datasources, nil
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
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.datasources.create")
	defer endSpan()

	normalized, err := rules.DatasourceRules.Execute(payload)
	if err != nil {
		return nil, err
	}

	c := client.NewGrafanaClient(opts, s.resolver)
	start := time.Now()

	var createdDS struct {
		ID      int64  `json:"id"`
		UID     string `json:"uid"`
		Name    string `json:"name"`
		Message string `json:"message"`
	}

	_, err = c.Do(ctx, http.MethodPost, endpoints.EndpointDatasources, normalized, &createdDS)
	latency := float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil {
		return &schema.SingleDatasourceResult{
			Service:        normalized.Type,
			DatasourceName: normalized.Name,
			Status:         "failed",
			Message:        err.Error(),
			LatencyMs:      latency,
			IsHealthy:      false,
		}, err
	}

	uid := createdDS.UID
	if uid == "" {
		uid = normalized.UID
	}

	return &schema.SingleDatasourceResult{
		Service:        normalized.Type,
		DatasourceName: normalized.Name,
		DatasourceUID:  uid,
		DatasourceID:   createdDS.ID,
		Status:         "created",
		Message:        "Datasource created successfully",
		LatencyMs:      latency,
		IsHealthy:      true,
	}, nil
}

func (s *DatasourceService) Update(ctx context.Context, opts types.ClientOptions, idOrUID string, payload schema.DatasourcePayload) (*schema.SingleDatasourceResult, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.datasources.update")
	defer endSpan()

	normalized := rules.DatasourceRules.Normalize(payload)
	c := client.NewGrafanaClient(opts, s.resolver)
	start := time.Now()

	targetPath := endpoints.BuildDatasourceUIDPath(idOrUID)
	var updatedDS struct {
		ID      int64  `json:"id"`
		UID     string `json:"uid"`
		Name    string `json:"name"`
		Message string `json:"message"`
	}

	_, err := c.Do(ctx, http.MethodPut, targetPath, normalized, &updatedDS)
	latency := float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil {
		return &schema.SingleDatasourceResult{
			Service:        normalized.Type,
			DatasourceName: normalized.Name,
			Status:         "failed",
			Message:        err.Error(),
			LatencyMs:      latency,
			IsHealthy:      false,
		}, err
	}

	return &schema.SingleDatasourceResult{
		Service:        normalized.Type,
		DatasourceName: normalized.Name,
		DatasourceUID:  idOrUID,
		DatasourceID:   updatedDS.ID,
		Status:         "updated",
		Message:        "Datasource updated successfully",
		LatencyMs:      latency,
		IsHealthy:      true,
	}, nil
}

func (s *DatasourceService) Delete(ctx context.Context, opts types.ClientOptions, idOrUIDOrName string) (*types.DeleteDatasourceResult, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.datasources.delete")
	defer endSpan()

	c := client.NewGrafanaClient(opts, s.resolver)
	uidPath := endpoints.BuildDatasourceUIDPath(idOrUIDOrName)

	var deleteResponse struct {
		Message string `json:"message"`
	}

	status, err := c.Do(ctx, http.MethodDelete, uidPath, nil, &deleteResponse)
	if err == nil && status == http.StatusOK {
		return &types.DeleteDatasourceResult{
			UID:     idOrUIDOrName,
			Message: deleteResponse.Message,
			Success: true,
		}, nil
	}

	namePath := endpoints.BuildDatasourceNamePath(idOrUIDOrName)
	status, err = c.Do(ctx, http.MethodDelete, namePath, nil, &deleteResponse)
	if err == nil && status == http.StatusOK {
		return &types.DeleteDatasourceResult{
			UID:     idOrUIDOrName,
			Message: deleteResponse.Message,
			Success: true,
		}, nil
	}

	return &types.DeleteDatasourceResult{
		UID:     idOrUIDOrName,
		Message: fmt.Sprintf("Failed to delete datasource: %v", err),
		Success: false,
	}, err
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

	report := &schema.DatasourceSyncReport{
		TotalCount:   len(results),
		SuccessCount: successCount,
		Results:      results,
		ReportedAt:   time.Now().UTC().Format(time.RFC3339),
	}

	return report, nil
}

func (s *DatasourceService) BuildDefaultPayload(svc string) (schema.DatasourcePayload, bool) {
	return s.registry.Resolve(svc, s.resolver)
}

func (s *DatasourceService) TemplateRegistry() *rules.DatasourceTemplateRegistry {
	return s.registry
}

func (s *DatasourceService) provisionSingle(ctx context.Context, c *client.GrafanaClient, payload schema.DatasourcePayload, testConnection bool) schema.SingleDatasourceResult {
	start := time.Now()

	targetPath := endpoints.BuildDatasourceUIDPath(payload.UID)
	var existingDS schema.DatasourcePayload
	status, err := c.Do(ctx, http.MethodGet, targetPath, nil, &existingDS)

	var result schema.SingleDatasourceResult

	if err == nil && status == http.StatusOK {
		var updateResp struct {
			ID      int64  `json:"id"`
			Message string `json:"message"`
		}
		_, err = c.Do(ctx, http.MethodPut, targetPath, payload, &updateResp)
		latency := float64(time.Since(start).Microseconds()) / 1000.0

		if err != nil {
			result = schema.SingleDatasourceResult{
				DatasourceName: payload.Name,
				DatasourceUID:  payload.UID,
				Status:         "failed",
				Message:        fmt.Sprintf("Update failed: %v", err),
				LatencyMs:      latency,
				IsHealthy:      false,
			}
		} else {
			result = schema.SingleDatasourceResult{
				DatasourceName: payload.Name,
				DatasourceUID:  payload.UID,
				DatasourceID:   existingDS.ID,
				Status:         "updated",
				Message:        "Datasource updated successfully",
				LatencyMs:      latency,
				IsHealthy:      true,
			}
		}
	} else {
		var createResp struct {
			ID      int64  `json:"id"`
			UID     string `json:"uid"`
			Message string `json:"message"`
		}
		_, err = c.Do(ctx, http.MethodPost, endpoints.EndpointDatasources, payload, &createResp)
		latency := float64(time.Since(start).Microseconds()) / 1000.0

		if err != nil {
			result = schema.SingleDatasourceResult{
				DatasourceName: payload.Name,
				DatasourceUID:  payload.UID,
				Status:         "failed",
				Message:        fmt.Sprintf("Creation failed: %v", err),
				LatencyMs:      latency,
				IsHealthy:      false,
			}
		} else {
			result = schema.SingleDatasourceResult{
				DatasourceName: payload.Name,
				DatasourceUID:  payload.UID,
				DatasourceID:   createResp.ID,
				Status:         "created",
				Message:        "Datasource created successfully",
				LatencyMs:      latency,
				IsHealthy:      true,
			}
		}
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
