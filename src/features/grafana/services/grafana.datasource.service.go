/*
Package services provides isolated, domain-specific services for Grafana lifecycle management.

ALGORITHM BLUEPRINT (DatasourceService):
1. Datasource Operations: List, Get, Create, Update, Delete, Health Probe, Batch Sync.
2. Endpoint Decoupling: Uses centralized typed endpoint constants from the endpoints package.
3. Client Dynamic Resolution: Uses GrafanaClient for auth headers and network transport.
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
	"os"
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
}

func NewDatasourceService(tracer ports.TracerPort, baseDir string) *DatasourceService {
	return &DatasourceService{
		tracer:   tracer,
		baseDir:  baseDir,
		resolver: paths.NewPathResolver(baseDir),
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

	payload = rules.NormalizeDatasourcePayload(payload)
	if err := rules.ValidateDatasourcePayload(payload); err != nil {
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

	_, err := c.Do(ctx, http.MethodPost, endpoints.EndpointDatasources, payload, &createdDS)
	latency := float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil {
		return &schema.SingleDatasourceResult{
			Service:        payload.Type,
			DatasourceName: payload.Name,
			Status:         "failed",
			Message:        err.Error(),
			LatencyMs:      latency,
			IsHealthy:      false,
		}, err
	}

	uid := createdDS.UID
	if uid == "" {
		uid = payload.UID
	}

	return &schema.SingleDatasourceResult{
		Service:        payload.Type,
		DatasourceName: payload.Name,
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

	payload = rules.NormalizeDatasourcePayload(payload)
	c := client.NewGrafanaClient(opts, s.resolver)
	start := time.Now()

	targetPath := endpoints.BuildDatasourceUIDPath(idOrUID)
	var updatedDS struct {
		ID      int64  `json:"id"`
		UID     string `json:"uid"`
		Name    string `json:"name"`
		Message string `json:"message"`
	}

	_, err := c.Do(ctx, http.MethodPut, targetPath, payload, &updatedDS)
	latency := float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil {
		return &schema.SingleDatasourceResult{
			Service:        payload.Type,
			DatasourceName: payload.Name,
			Status:         "failed",
			Message:        err.Error(),
			LatencyMs:      latency,
			IsHealthy:      false,
		}, err
	}

	return &schema.SingleDatasourceResult{
		Service:        payload.Type,
		DatasourceName: payload.Name,
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
	switch strings.ToLower(svc) {
	case "alloydb", "postgres", "postgresql":
		host := s.getEnv("ALLOYDB_HOST", "localhost")
		port := s.getEnv("ALLOYDB_PORT", "31420")
		user := s.getEnv("ALLOYDB_USER", "postgres")
		pass := s.getEnv("ALLOYDB_PASSWORD", "postgres")
		db := s.getEnv("ALLOYDB_DB", "llmobs")

		return schema.DatasourcePayload{
			UID:       "ds-alloydb-platform",
			Name:      "AlloyDB-Ledger",
			Type:      "postgres",
			Access:    "proxy",
			URL:       fmt.Sprintf("%s:%s", host, port),
			User:      user,
			Database:  db,
			BasicAuth: false,
			IsDefault: false,
			JSONData: map[string]interface{}{
				"sslmode":         "disable",
				"postgresVersion": 1500,
				"maxOpenConns":    20,
				"maxIdleConns":    5,
				"connMaxLifetime": 14400,
			},
			SecureJSONData: map[string]string{
				"password": pass,
			},
		}, true

	case "clickhouse":
		host := s.getEnv("CLICKHOUSE_HOST", "localhost")
		port := s.getEnv("CLICKHOUSE_PORT", "31421")
		user := s.getEnv("CLICKHOUSE_USER", "default")
		pass := s.getEnv("CLICKHOUSE_PASSWORD", "")
		db := s.getEnv("CLICKHOUSE_DB", "llmobs")

		return schema.DatasourcePayload{
			UID:       "ds-clickhouse-analytics",
			Name:      "ClickHouse-Analytics",
			Type:      "grafana-clickhouse-datasource",
			Access:    "proxy",
			URL:       fmt.Sprintf("http://%s:%s", host, port),
			User:      user,
			Database:  db,
			BasicAuth: false,
			IsDefault: false,
			JSONData: map[string]interface{}{
				"port":            31421,
				"server":          host,
				"defaultDatabase": db,
				"protocol":        "http",
			},
			SecureJSONData: map[string]string{
				"password": pass,
			},
		}, true

	case "redis":
		host := s.getEnv("REDIS_HOST", "localhost")
		port := s.getEnv("REDIS_PORT", "31413")
		pass := s.getEnv("REDIS_PASSWORD", "")

		return schema.DatasourcePayload{
			UID:       "ds-redis-ledger",
			Name:      "Redis-Ledger",
			Type:      "redis-datasource",
			Access:    "proxy",
			URL:       fmt.Sprintf("redis://%s:%s", host, port),
			BasicAuth: false,
			IsDefault: false,
			JSONData: map[string]interface{}{
				"poolSize": 5,
				"timeout":  10,
			},
			SecureJSONData: map[string]string{
				"password": pass,
			},
		}, true

	case "tempo":
		host := s.getEnv("TEMPO_HOST", "localhost")
		port := s.getEnv("TEMPO_PORT", "31416")

		return schema.DatasourcePayload{
			UID:       "ds-tempo-traces",
			Name:      "Tempo-Traces",
			Type:      "tempo",
			Access:    "proxy",
			URL:       fmt.Sprintf("http://%s:%s", host, port),
			BasicAuth: false,
			IsDefault: true,
			JSONData: map[string]interface{}{
				"tracesToLogs": map[string]interface{}{
					"datasourceUid": "ds-clickhouse-analytics",
				},
			},
		}, true

	case "prometheus":
		host := s.getEnv("PROMETHEUS_HOST", "localhost")
		port := s.getEnv("PROMETHEUS_PORT", "9090")

		return schema.DatasourcePayload{
			UID:       "ds-prometheus-metrics",
			Name:      "Prometheus",
			Type:      "prometheus",
			Access:    "proxy",
			URL:       fmt.Sprintf("http://%s:%s", host, port),
			BasicAuth: false,
			IsDefault: false,
			JSONData: map[string]interface{}{
				"httpMethod": "POST",
			},
		}, true

	case "loki":
		host := s.getEnv("LOKI_HOST", "localhost")
		port := s.getEnv("LOKI_PORT", "3100")

		return schema.DatasourcePayload{
			UID:       "ds-loki-logs",
			Name:      "Loki-Logs",
			Type:      "loki",
			Access:    "proxy",
			URL:       fmt.Sprintf("http://%s:%s", host, port),
			BasicAuth: false,
			IsDefault: false,
		}, true
	}

	return schema.DatasourcePayload{}, false
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

func (s *DatasourceService) getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	if s.resolver != nil {
		return s.resolver.ResolveEnvOrConfig(key, fallback)
	}
	return fallback
}
