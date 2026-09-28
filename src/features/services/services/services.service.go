/*
Package services implements business logic, persistence, and diagnostic probing for external service connections.

ALGORITHM BLUEPRINT:
1. ServicesService: Manages registration, discovery, persistence, and health probes for external and custom services.
2. Catalog Persistence: Atomically reads/writes configuration to workspace JSON store (config/services-catalog.json).
3. Health Probing: Concurrently conducts HTTP status probes, TCP socket handshakes, and database availability tests.
4. Grafana Telemetry Bridge: Bridges registered data services (PostgreSQL, ClickHouse, Redis, Tempo, etc.) directly to Grafana datasources.
5. Invariants:
   - Zero inline comments inside function bodies.
   - Atomic file writes prevent catalog corruption.
   - OpenTelemetry spans wrap all CRUD and diagnostic executions.
*/
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	grafanaSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/schema"
	grafanaService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/services"
	grafanaTypes "github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/types"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/services/rules"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/services/schema"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/services/types"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

type ServicesService struct {
	tracer    ports.TracerPort
	baseDir   string
	storeMu   sync.RWMutex
	storePath string
}

func NewServicesService(tracer ports.TracerPort, baseDir string) *ServicesService {
	storePath := filepath.Join(baseDir, "config", "services-catalog.json")
	return &ServicesService{
		tracer:    tracer,
		baseDir:   baseDir,
		storePath: storePath,
	}
}

func (s *ServicesService) RegisterService(ctx context.Context, svc schema.ServiceDefinition) (*schema.ServiceDefinition, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "services.register")
	defer endSpan()

	svc = rules.NormalizeServiceDefinition(svc)
	if err := rules.ValidateServiceDefinition(svc); err != nil {
		return nil, err
	}

	s.storeMu.Lock()
	defer s.storeMu.Unlock()

	catalog, err := s.loadCatalogUnsafe()
	if err != nil {
		return nil, err
	}

	catalog.Services[svc.ID] = svc
	catalog.UpdatedAt = time.Now().UTC()

	if err := s.saveCatalogUnsafe(catalog); err != nil {
		return nil, err
	}

	return &svc, nil
}

func (s *ServicesService) ListServices(ctx context.Context, filter schema.ServiceFilterOptions) ([]schema.ServiceDefinition, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "services.list")
	defer endSpan()

	s.storeMu.RLock()
	defer s.storeMu.RUnlock()

	catalog, err := s.loadCatalogUnsafe()
	if err != nil {
		return nil, err
	}

	var results []schema.ServiceDefinition
	for _, svc := range catalog.Services {
		if filter.Category != "" && !strings.EqualFold(svc.Category, filter.Category) {
			continue
		}
		if filter.Type != "" && !strings.EqualFold(svc.Type, filter.Type) {
			continue
		}
		if filter.Query != "" {
			q := strings.ToLower(filter.Query)
			if !strings.Contains(strings.ToLower(svc.Name), q) && !strings.Contains(strings.ToLower(svc.ID), q) && !strings.Contains(strings.ToLower(svc.URL), q) {
				continue
			}
		}
		if filter.Tag != "" {
			matched := false
			for _, t := range svc.Tags {
				if strings.EqualFold(t, filter.Tag) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		results = append(results, svc)
	}

	return results, nil
}

func (s *ServicesService) GetService(ctx context.Context, idOrName string) (*schema.ServiceDefinition, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "services.get")
	defer endSpan()

	s.storeMu.RLock()
	defer s.storeMu.RUnlock()

	catalog, err := s.loadCatalogUnsafe()
	if err != nil {
		return nil, err
	}

	if svc, exists := catalog.Services[idOrName]; exists {
		return &svc, nil
	}

	for _, svc := range catalog.Services {
		if strings.EqualFold(svc.Name, idOrName) || strings.EqualFold(svc.ID, idOrName) {
			return &svc, nil
		}
	}

	return nil, fmt.Errorf("service %q not found in registry", idOrName)
}

func (s *ServicesService) UpdateService(ctx context.Context, idOrName string, patch schema.ServiceDefinition) (*schema.ServiceDefinition, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "services.update")
	defer endSpan()

	s.storeMu.Lock()
	defer s.storeMu.Unlock()

	catalog, err := s.loadCatalogUnsafe()
	if err != nil {
		return nil, err
	}

	var targetID string
	if _, exists := catalog.Services[idOrName]; exists {
		targetID = idOrName
	} else {
		for id, svc := range catalog.Services {
			if strings.EqualFold(svc.Name, idOrName) {
				targetID = id
				break
			}
		}
	}

	if targetID == "" {
		return nil, fmt.Errorf("service %q not found for update", idOrName)
	}

	existing := catalog.Services[targetID]

	if patch.Name != "" {
		existing.Name = patch.Name
	}
	if patch.Type != "" {
		existing.Type = patch.Type
	}
	if patch.Category != "" {
		existing.Category = patch.Category
	}
	if patch.URL != "" {
		existing.URL = patch.URL
	}
	if patch.Host != "" {
		existing.Host = patch.Host
	}
	if patch.Port > 0 {
		existing.Port = patch.Port
	}
	if patch.Database != "" {
		existing.Database = patch.Database
	}
	if patch.HealthCheck.Type != "" {
		existing.HealthCheck = patch.HealthCheck
	}
	if patch.Auth.Type != "" || patch.Auth.Token != "" || patch.Auth.Password != "" {
		existing.Auth = patch.Auth
	}
	if len(patch.Tags) > 0 {
		existing.Tags = patch.Tags
	}
	if patch.Metadata != nil {
		for k, v := range patch.Metadata {
			existing.Metadata[k] = v
		}
	}

	existing = rules.NormalizeServiceDefinition(existing)
	if err := rules.ValidateServiceDefinition(existing); err != nil {
		return nil, err
	}

	catalog.Services[targetID] = existing
	catalog.UpdatedAt = time.Now().UTC()

	if err := s.saveCatalogUnsafe(catalog); err != nil {
		return nil, err
	}

	return &existing, nil
}

func (s *ServicesService) DeleteService(ctx context.Context, idOrName string) (*types.DeleteServiceResult, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "services.delete")
	defer endSpan()

	s.storeMu.Lock()
	defer s.storeMu.Unlock()

	catalog, err := s.loadCatalogUnsafe()
	if err != nil {
		return nil, err
	}

	var targetID string
	var serviceName string
	if svc, exists := catalog.Services[idOrName]; exists {
		targetID = idOrName
		serviceName = svc.Name
	} else {
		for id, svc := range catalog.Services {
			if strings.EqualFold(svc.Name, idOrName) {
				targetID = id
				serviceName = svc.Name
				break
			}
		}
	}

	if targetID == "" {
		return nil, fmt.Errorf("service %q not found", idOrName)
	}

	delete(catalog.Services, targetID)
	catalog.UpdatedAt = time.Now().UTC()

	if err := s.saveCatalogUnsafe(catalog); err != nil {
		return nil, err
	}

	return &types.DeleteServiceResult{
		ServiceID: targetID,
		Name:      serviceName,
		Message:   fmt.Sprintf("Service %q (%s) removed from catalog", serviceName, targetID),
		Success:   true,
	}, nil
}

func (s *ServicesService) TestServiceHealth(ctx context.Context, idOrName string) (*types.HealthProbeResult, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "services.test_health")
	defer endSpan()

	svc, err := s.GetService(ctx, idOrName)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	timeout := time.Duration(svc.HealthCheck.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	if svc.HealthCheck.Type == "http" || strings.HasPrefix(svc.URL, "http://") || strings.HasPrefix(svc.URL, "https://") {
		targetURL := svc.URL
		if svc.HealthCheck.Path != "" {
			if strings.HasPrefix(svc.HealthCheck.Path, "/") {
				targetURL = strings.TrimRight(targetURL, "/") + svc.HealthCheck.Path
			} else {
				targetURL = strings.TrimRight(targetURL, "/") + "/" + svc.HealthCheck.Path
			}
		}
		if targetURL == "" && svc.Host != "" {
			portStr := ""
			if svc.Port > 0 {
				portStr = fmt.Sprintf(":%d", svc.Port)
			}
			pathStr := svc.HealthCheck.Path
			if pathStr == "" {
				pathStr = "/healthz"
			}
			targetURL = fmt.Sprintf("http://%s%s%s", svc.Host, portStr, pathStr)
		}

		method := svc.HealthCheck.Method
		if method == "" {
			method = "GET"
		}

		req, err := http.NewRequestWithContext(ctx, method, targetURL, nil)
		if err != nil {
			return &types.HealthProbeResult{
				ServiceID: svc.ID,
				Name:      svc.Name,
				Target:    targetURL,
				ProbeType: "http",
				Status:    "INVALID_REQUEST",
				Message:   err.Error(),
				LatencyMs: float64(time.Since(start).Milliseconds()),
				IsHealthy: false,
				CheckedAt: time.Now().UTC(),
			}, nil
		}

		for k, v := range svc.HealthCheck.Headers {
			req.Header.Set(k, v)
		}

		if svc.Auth.Type == "bearer" && svc.Auth.Token != "" {
			req.Header.Set("Authorization", "Bearer "+svc.Auth.Token)
		} else if svc.Auth.Type == "basic" {
			req.SetBasicAuth(svc.Auth.Username, svc.Auth.Password)
		} else if svc.Auth.Type == "api-key" && svc.Auth.HeaderName != "" {
			req.Header.Set(svc.Auth.HeaderName, svc.Auth.Key)
		}

		client := &http.Client{Timeout: timeout}
		resp, err := client.Do(req)
		latency := float64(time.Since(start).Milliseconds())

		if err != nil {
			return &types.HealthProbeResult{
				ServiceID: svc.ID,
				Name:      svc.Name,
				Target:    targetURL,
				ProbeType: "http",
				Status:    "CONNECTION_FAILED",
				Message:   fmt.Sprintf("HTTP probe failed: %v", err),
				LatencyMs: latency,
				IsHealthy: false,
				CheckedAt: time.Now().UTC(),
			}, nil
		}
		defer resp.Body.Close()

		expected := svc.HealthCheck.ExpectedStatus
		if expected <= 0 {
			expected = 200
		}

		isHealthy := resp.StatusCode == expected || (expected == 200 && resp.StatusCode >= 200 && resp.StatusCode < 300)
		status := fmt.Sprintf("HTTP_%d", resp.StatusCode)
		msg := fmt.Sprintf("HTTP endpoint responded with status %d (Expected: %d)", resp.StatusCode, expected)

		return &types.HealthProbeResult{
			ServiceID: svc.ID,
			Name:      svc.Name,
			Target:    targetURL,
			ProbeType: "http",
			Status:    status,
			Message:   msg,
			LatencyMs: latency,
			IsHealthy: isHealthy,
			CheckedAt: time.Now().UTC(),
		}, nil
	}

	targetHost := svc.Host
	targetPort := svc.Port
	if targetHost == "" && svc.URL != "" {
		if parsed, err := url.Parse(svc.URL); err == nil {
			targetHost = parsed.Hostname()
			if parsed.Port() != "" {
				fmt.Sscanf(parsed.Port(), "%d", &targetPort)
			}
		}
	}

	if targetPort == 0 {
		switch svc.Type {
		case "postgres", "alloydb":
			targetPort = 5432
		case "clickhouse":
			targetPort = 9000
		case "redis":
			targetPort = 6379
		case "tempo":
			targetPort = 3200
		case "prometheus":
			targetPort = 9090
		default:
			targetPort = 80
		}
	}

	address := fmt.Sprintf("%s:%d", targetHost, targetPort)
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	latency := float64(time.Since(start).Milliseconds())

	if err != nil {
		return &types.HealthProbeResult{
			ServiceID: svc.ID,
			Name:      svc.Name,
			Target:    address,
			ProbeType: "tcp",
			Status:    "CONNECTION_REFUSED",
			Message:   fmt.Sprintf("TCP dial to %s failed: %v", address, err),
			LatencyMs: latency,
			IsHealthy: false,
			CheckedAt: time.Now().UTC(),
		}, nil
	}
	defer conn.Close()

	return &types.HealthProbeResult{
		ServiceID: svc.ID,
		Name:      svc.Name,
		Target:    address,
		ProbeType: "tcp",
		Status:    "OPEN",
		Message:   fmt.Sprintf("TCP handshake succeeded on %s", address),
		LatencyMs: latency,
		IsHealthy: true,
		CheckedAt: time.Now().UTC(),
	}, nil
}

func (s *ServicesService) SyncToGrafana(ctx context.Context, idOrName string, grafanaSvc *grafanaService.GrafanaService) (*types.ServiceSyncResult, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "services.sync_to_grafana")
	defer endSpan()

	svc, err := s.GetService(ctx, idOrName)
	if err != nil {
		return nil, err
	}

	dsType := svc.Type
	switch svc.Type {
	case "postgres", "alloydb", "postgresql":
		dsType = "grafana-postgresql-datasource"
	case "clickhouse":
		dsType = "grafana-clickhouse-datasource"
	case "redis":
		dsType = "redis-datasource"
	case "tempo":
		dsType = "tempo"
	case "prometheus":
		dsType = "prometheus"
	case "loki":
		dsType = "loki"
	case "elasticsearch":
		dsType = "elasticsearch"
	}

	targetURL := svc.URL
	if targetURL == "" && svc.Host != "" {
		if svc.Port > 0 {
			targetURL = fmt.Sprintf("%s:%d", svc.Host, svc.Port)
		} else {
			targetURL = svc.Host
		}
	}

	dsPayload := grafanaSchema.DatasourcePayload{
		Name:      svc.Name,
		Type:      dsType,
		URL:       targetURL,
		Database:  svc.Database,
		User:      svc.Auth.Username,
		Access:    "proxy",
		IsDefault: false,
		JSONData:  svc.Metadata,
	}

	if svc.Auth.Password != "" {
		dsPayload.SecureJSONData = map[string]string{
			"password": svc.Auth.Password,
		}
	}

	clientOpts := grafanaTypes.ClientOptions{
		Timeout: 10 * time.Second,
	}

	res, err := grafanaSvc.CreateDatasource(ctx, clientOpts, dsPayload)
	if err != nil {
		resUp, errUp := grafanaSvc.UpdateDatasource(ctx, clientOpts, svc.Name, dsPayload)
		if errUp != nil {
			return nil, fmt.Errorf("failed to sync service to Grafana: %w (update fallback error: %v)", err, errUp)
		}
		return &types.ServiceSyncResult{
			ServiceID:     svc.ID,
			DatasourceUID: resUp.DatasourceUID,
			Message:       fmt.Sprintf("Service %q synchronized as Grafana datasource (UID: %s)", svc.Name, resUp.DatasourceUID),
			LatencyMs:     resUp.LatencyMs,
			Success:       true,
		}, nil
	}

	return &types.ServiceSyncResult{
		ServiceID:     svc.ID,
		DatasourceUID: res.DatasourceUID,
		Message:       fmt.Sprintf("Service %q registered as new Grafana datasource (UID: %s)", svc.Name, res.DatasourceUID),
		LatencyMs:     res.LatencyMs,
		Success:       true,
	}, nil
}

func (s *ServicesService) loadCatalogUnsafe() (*schema.ServiceCatalog, error) {
	if _, err := os.Stat(s.storePath); os.IsNotExist(err) {
		return &schema.ServiceCatalog{
			Version:   "1.0.0",
			Services:  make(map[string]schema.ServiceDefinition),
			UpdatedAt: time.Now().UTC(),
		}, nil
	}

	data, err := os.ReadFile(s.storePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read services catalog from %s: %w", s.storePath, err)
	}

	var catalog schema.ServiceCatalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("failed to parse services catalog JSON: %w", err)
	}

	if catalog.Services == nil {
		catalog.Services = make(map[string]schema.ServiceDefinition)
	}

	return &catalog, nil
}

func (s *ServicesService) saveCatalogUnsafe(catalog *schema.ServiceCatalog) error {
	dir := filepath.Dir(s.storePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize services catalog: %w", err)
	}

	tempPath := s.storePath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary catalog file: %w", err)
	}

	if err := os.Rename(tempPath, s.storePath); err != nil {
		return fmt.Errorf("failed to atomically update catalog: %w", err)
	}

	return nil
}
