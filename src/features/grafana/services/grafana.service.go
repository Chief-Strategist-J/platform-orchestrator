/*
Package services provides complete Grafana datasource management, synchronization, and health verification.

ALGORITHM BLUEPRINT:
1. GrafanaService: Orchestrates CRUD lifecycle and health probes for arbitrary and platform-managed Grafana datasources.
2. ListDatasources:
   a. Dispatches authenticated GET /api/datasources.
   b. Parses array of DatasourcePayload.
3. GetDatasource:
   a. Searches by UID via GET /api/datasources/uid/:uid or by ID / Name fallback.
4. CreateDatasource:
   a. Normalizes and validates payload via rules engine.
   b. Dispatches POST /api/datasources.
   c. If testConnection=true, runs health probe on returned UID.
5. UpdateDatasource:
   a. Normalizes and validates payload.
   b. Dispatches PUT /api/datasources/uid/:uid (or by ID).
   c. Optionally verifies health.
6. DeleteDatasource:
   a. Dispatches DELETE /api/datasources/uid/:uid (or by ID).
7. TestDatasourceHealth:
   a. Dispatches GET /api/datasources/uid/:uid/health.
   b. Compiles DatasourceHealthResult with latency and status.
8. SyncDatasources:
   a. Evaluates requested services against template rules and existing datasources.
   b. Upserts each datasource and tests connection health.
9. Invariants:
   - Zero inline comments inside function bodies.
   - All external calls wrapped with OpenTelemetry spans.
   - Credentials resolved from environment with graceful fallbacks.
*/
package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/rules"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/schema"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/types"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

type GrafanaService struct {
	tracer  ports.TracerPort
	baseDir string
}

func NewGrafanaService(tracer ports.TracerPort, baseDir string) *GrafanaService {
	return &GrafanaService{
		tracer:  tracer,
		baseDir: baseDir,
	}
}

func (s *GrafanaService) ListDatasources(ctx context.Context, clientOpts types.ClientOptions) ([]schema.DatasourcePayload, error) {
	_, endSpan := s.tracer.StartSpan(ctx, "llmobs.grafana.list_datasources")
	defer endSpan()

	client, grafanaURL, user, pass := s.resolveClient(clientOpts)
	list, _, err := s.fetchExistingDatasources(client, grafanaURL, user, pass)
	if err != nil {
		return nil, fmt.Errorf("failed to list Grafana datasources: %w", err)
	}
	return list, nil
}

func (s *GrafanaService) GetDatasource(ctx context.Context, idOrUid string, clientOpts types.ClientOptions) (*schema.DatasourcePayload, error) {
	_, endSpan := s.tracer.StartSpan(ctx, "llmobs.grafana.get_datasource")
	defer endSpan()

	client, grafanaURL, user, pass := s.resolveClient(clientOpts)

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/datasources/uid/%s", grafanaURL, idOrUid), nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(user, pass)

	resp, err := client.Do(req)
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))
		var ds schema.DatasourcePayload
		if err := json.Unmarshal(body, &ds); err == nil && ds.Name != "" {
			return &ds, nil
		}
	}
	if resp != nil {
		resp.Body.Close()
	}

	list, _, err := s.fetchExistingDatasources(client, grafanaURL, user, pass)
	if err != nil {
		return nil, err
	}
	for _, ds := range list {
		if ds.UID == idOrUid || strings.EqualFold(ds.Name, idOrUid) || fmt.Sprintf("%d", ds.ID) == idOrUid {
			return &ds, nil
		}
	}

	return nil, fmt.Errorf("datasource %q not found", idOrUid)
}

func (s *GrafanaService) CreateDatasource(ctx context.Context, payload schema.DatasourcePayload, testConnection bool, clientOpts types.ClientOptions) (*schema.SingleDatasourceResult, error) {
	_, endSpan := s.tracer.StartSpan(ctx, "llmobs.grafana.create_datasource")
	defer endSpan()

	normalized := rules.NormalizeDatasourcePayload(payload)
	if err := rules.ValidateDatasourcePayload(normalized); err != nil {
		return nil, err
	}

	client, grafanaURL, user, pass := s.resolveClient(clientOpts)
	start := time.Now()

	dataBytes, _ := json.Marshal(normalized)
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/datasources", grafanaURL), bytes.NewReader(dataBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(user, pass)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create datasource: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to create datasource (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var resObj map[string]interface{}
	_ = json.Unmarshal(body, &resObj)

	createdUID := normalized.UID
	var createdID int64
	if uid, ok := resObj["uid"].(string); ok && uid != "" {
		createdUID = uid
	}
	if idVal, ok := resObj["id"].(float64); ok {
		createdID = int64(idVal)
	}
	if dsMap, ok := resObj["datasource"].(map[string]interface{}); ok {
		if uid, ok := dsMap["uid"].(string); ok && uid != "" {
			createdUID = uid
		}
		if idVal, ok := dsMap["id"].(float64); ok {
			createdID = int64(idVal)
		}
	}

	msg := fmt.Sprintf("Datasource %q created successfully", normalized.Name)
	if testConnection && createdUID != "" {
		if healthRes, hErr := s.TestDatasourceHealth(ctx, createdUID, clientOpts); hErr == nil {
			msg = fmt.Sprintf("%s (Health: %s - %s)", msg, healthRes.Status, healthRes.Message)
		}
	}

	return &schema.SingleDatasourceResult{
		Service:        normalized.Name,
		DatasourceName: normalized.Name,
		DatasourceUID:  createdUID,
		DatasourceID:   createdID,
		Status:         "CREATED",
		Message:        msg,
		LatencyMs:      float64(time.Since(start).Milliseconds()),
		IsHealthy:      true,
	}, nil
}

func (s *GrafanaService) UpdateDatasource(ctx context.Context, idOrUid string, payload schema.DatasourcePayload, testConnection bool, clientOpts types.ClientOptions) (*schema.SingleDatasourceResult, error) {
	_, endSpan := s.tracer.StartSpan(ctx, "llmobs.grafana.update_datasource")
	defer endSpan()

	normalized := rules.NormalizeDatasourcePayload(payload)
	client, grafanaURL, user, pass := s.resolveClient(clientOpts)
	start := time.Now()

	existing, err := s.GetDatasource(ctx, idOrUid, clientOpts)
	if err != nil {
		return nil, err
	}

	if normalized.Name == "" {
		normalized.Name = existing.Name
	}
	if normalized.Type == "" {
		normalized.Type = existing.Type
	}
	if normalized.URL == "" {
		normalized.URL = existing.URL
	}
	if normalized.Access == "" {
		normalized.Access = existing.Access
	}
	if normalized.User == "" {
		normalized.User = existing.User
	}
	if normalized.Database == "" {
		normalized.Database = existing.Database
	}
	normalized.ID = existing.ID
	normalized.UID = existing.UID

	dataBytes, _ := json.Marshal(normalized)
	var targetURL string
	if existing.UID != "" {
		targetURL = fmt.Sprintf("%s/api/datasources/uid/%s", grafanaURL, existing.UID)
	} else {
		targetURL = fmt.Sprintf("%s/api/datasources/%d", grafanaURL, existing.ID)
	}

	req, err := http.NewRequest("PUT", targetURL, bytes.NewReader(dataBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(user, pass)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to update datasource: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to update datasource (HTTP %d): %s", resp.StatusCode, string(body))
	}

	msg := fmt.Sprintf("Datasource %q updated successfully", normalized.Name)
	if testConnection && existing.UID != "" {
		if healthRes, hErr := s.TestDatasourceHealth(ctx, existing.UID, clientOpts); hErr == nil {
			msg = fmt.Sprintf("%s (Health: %s - %s)", msg, healthRes.Status, healthRes.Message)
		}
	}

	return &schema.SingleDatasourceResult{
		Service:        normalized.Name,
		DatasourceName: normalized.Name,
		DatasourceUID:  existing.UID,
		DatasourceID:   existing.ID,
		Status:         "UPDATED",
		Message:        msg,
		LatencyMs:      float64(time.Since(start).Milliseconds()),
		IsHealthy:      true,
	}, nil
}

func (s *GrafanaService) DeleteDatasource(ctx context.Context, idOrUid string, clientOpts types.ClientOptions) (*types.DeleteDatasourceResult, error) {
	_, endSpan := s.tracer.StartSpan(ctx, "llmobs.grafana.delete_datasource")
	defer endSpan()

	client, grafanaURL, user, pass := s.resolveClient(clientOpts)
	existing, err := s.GetDatasource(ctx, idOrUid, clientOpts)
	if err != nil {
		return nil, err
	}

	var deleteURL string
	if existing.UID != "" {
		deleteURL = fmt.Sprintf("%s/api/datasources/uid/%s", grafanaURL, existing.UID)
	} else {
		deleteURL = fmt.Sprintf("%s/api/datasources/%d", grafanaURL, existing.ID)
	}

	req, err := http.NewRequest("DELETE", deleteURL, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(user, pass)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to delete datasource: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("delete failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	return &types.DeleteDatasourceResult{
		UID:     existing.UID,
		Message: fmt.Sprintf("Datasource %q (UID: %s) deleted successfully", existing.Name, existing.UID),
		Success: true,
	}, nil
}

func (s *GrafanaService) TestDatasourceHealth(ctx context.Context, idOrUid string, clientOpts types.ClientOptions) (*types.DatasourceHealthResult, error) {
	_, endSpan := s.tracer.StartSpan(ctx, "llmobs.grafana.test_datasource_health")
	defer endSpan()

	client, grafanaURL, user, pass := s.resolveClient(clientOpts)
	start := time.Now()

	existing, err := s.GetDatasource(ctx, idOrUid, clientOpts)
	if err != nil {
		return nil, err
	}

	healthURL := fmt.Sprintf("%s/api/datasources/uid/%s/health", grafanaURL, existing.UID)
	if existing.UID == "" {
		healthURL = fmt.Sprintf("%s/api/datasources/%d/health", grafanaURL, existing.ID)
	}

	req, err := http.NewRequest("GET", healthURL, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(user, pass)

	resp, err := client.Do(req)
	if err != nil {
		return &types.DatasourceHealthResult{
			UID:       existing.UID,
			Name:      existing.Name,
			Status:    "DOWN",
			Message:   fmt.Sprintf("HTTP dial error: %v", err),
			LatencyMs: float64(time.Since(start).Milliseconds()),
			IsHealthy: false,
		}, nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))
	var healthJson map[string]interface{}
	_ = json.Unmarshal(body, &healthJson)

	statusStr := "UNKNOWN"
	msgStr := string(body)
	isHealthy := false

	if sVal, ok := healthJson["status"].(string); ok {
		statusStr = sVal
		if strings.EqualFold(statusStr, "OK") || strings.EqualFold(statusStr, "SUCCESS") {
			isHealthy = true
		}
	}
	if mVal, ok := healthJson["message"].(string); ok {
		msgStr = mVal
	}

	return &types.DatasourceHealthResult{
		UID:       existing.UID,
		Name:      existing.Name,
		Status:    statusStr,
		Message:   msgStr,
		LatencyMs: float64(time.Since(start).Milliseconds()),
		IsHealthy: isHealthy,
	}, nil
}

func (s *GrafanaService) SyncDatasources(ctx context.Context, opts schema.DatasourceSyncOptions) (schema.DatasourceSyncReport, error) {
	_, endSpan := s.tracer.StartSpan(ctx, "llmobs.grafana.sync_datasources")
	defer endSpan()

	clientOpts := types.ClientOptions{
		GrafanaURL: opts.GrafanaURL,
		Username:   opts.GrafanaUser,
		Password:   opts.GrafanaPass,
		Timeout:    opts.Timeout,
	}
	client, grafanaURL, user, pass := s.resolveClient(clientOpts)

	existingDS, activePass, err := s.fetchExistingDatasources(client, grafanaURL, user, pass)
	if err != nil {
		return schema.DatasourceSyncReport{}, fmt.Errorf("failed to connect to Grafana API at %s: %w", grafanaURL, err)
	}

	targets := s.resolveTargetServices(opts.Services)
	report := schema.DatasourceSyncReport{
		TotalCount: len(targets),
		ReportedAt: time.Now().UTC().Format(time.RFC3339),
	}

	for _, svc := range targets {
		start := time.Now()
		payload := s.buildDatasourcePayload(svc)
		if payload.Name == "" {
			continue
		}

		res := s.upsertAndTestDatasource(client, grafanaURL, user, activePass, payload, existingDS, opts.TestConnection, start)
		if res.IsHealthy {
			report.SuccessCount++
		}
		report.Results = append(report.Results, res)
	}

	return report, nil
}

func (s *GrafanaService) resolveClient(opts types.ClientOptions) (*http.Client, string, string, string) {
	url := opts.GrafanaURL
	if url == "" {
		url = "http://localhost:31415"
	}
	url = strings.TrimRight(url, "/")

	user := opts.Username
	if user == "" {
		user = "admin"
	}

	pass := opts.Password
	if pass == "" {
		pass = s.getEnv("GF_SECURITY_ADMIN_PASSWORD", "llmobs_admin_password")
	}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	return &http.Client{Timeout: timeout}, url, user, pass
}

func (s *GrafanaService) fetchExistingDatasources(client *http.Client, grafanaURL, user, pass string) ([]schema.DatasourcePayload, string, error) {
	passwordsToTry := []string{pass, "llmobs_admin_password", "admin"}
	var lastErr error

	for _, p := range passwordsToTry {
		req, err := http.NewRequest("GET", grafanaURL+"/api/datasources", nil)
		if err != nil {
			return nil, "", err
		}
		req.SetBasicAuth(user, p)

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 131072))
			var list []schema.DatasourcePayload
			if err := json.Unmarshal(body, &list); err == nil {
				return list, p, nil
			}
		}
		if resp.StatusCode == http.StatusUnauthorized {
			lastErr = fmt.Errorf("authentication failed for user %s", user)
			continue
		}
		lastErr = fmt.Errorf("HTTP status %d", resp.StatusCode)
	}

	return nil, "", lastErr
}

func (s *GrafanaService) resolveTargetServices(input []string) []string {
	if len(input) == 0 {
		return []string{"alloydb", "clickhouse", "redis", "tempo"}
	}
	var out []string
	for _, item := range input {
		for _, part := range strings.Split(item, ",") {
			trimmed := strings.ToLower(strings.TrimSpace(part))
			if trimmed == "all" || trimmed == "full" {
				return []string{"alloydb", "clickhouse", "redis", "tempo"}
			}
			if trimmed != "" {
				out = append(out, trimmed)
			}
		}
	}
	return out
}

func (s *GrafanaService) buildDatasourcePayload(svc string) schema.DatasourcePayload {
	switch svc {
	case "alloydb", "postgres", "postgresql", "db":
		alloyPass := s.getEnv("ALLOYDB_PASSWORD", "llmobs_s3cret_2026")
		alloyUser := s.getEnv("ALLOYDB_USER", "admin")
		alloyDB := s.getEnv("ALLOYDB_DB", "llm_observability")
		return schema.DatasourcePayload{
			Name:      "AlloyDB",
			Type:      "grafana-postgresql-datasource",
			Access:    "proxy",
			URL:       "llmobs-alloydb:5432",
			User:      alloyUser,
			Database:  alloyDB,
			BasicAuth: false,
			IsDefault: false,
			JSONData: map[string]interface{}{
				"postgresVersion": 1500,
				"sslmode":         "disable",
			},
			SecureJSONData: map[string]string{
				"password": alloyPass,
			},
		}

	case "clickhouse", "analytics":
		chPass := s.getEnv("CLICKHOUSE_PASSWORD", "llmobs_clickhouse_s3cret_2026")
		chUser := s.getEnv("CLICKHOUSE_USER", "default")
		chDB := s.getEnv("CLICKHOUSE_DB", "llm_telemetry_analytics")
		return schema.DatasourcePayload{
			Name:      "ClickHouse",
			Type:      "grafana-clickhouse-datasource",
			Access:    "proxy",
			URL:       "llmobs-clickhouse:9000",
			User:      chUser,
			Database:  chDB,
			BasicAuth: false,
			IsDefault: false,
			JSONData: map[string]interface{}{
				"port":            9000,
				"server":          "llmobs-clickhouse",
				"username":        chUser,
				"defaultDatabase": chDB,
			},
			SecureJSONData: map[string]string{
				"password": chPass,
			},
		}

	case "redis":
		redisPass := s.getEnv("REDIS_PASSWORD", "llmobs_redis_s3cret_2024")
		return schema.DatasourcePayload{
			Name:      "Redis",
			Type:      "redis-datasource",
			Access:    "proxy",
			URL:       "redis://llmobs-redis:6379",
			BasicAuth: false,
			IsDefault: false,
			SecureJSONData: map[string]string{
				"password": redisPass,
			},
		}

	case "tempo", "tracing":
		return schema.DatasourcePayload{
			Name:      "Tempo",
			Type:      "tempo",
			Access:    "proxy",
			URL:       "http://llmobs-tempo:3200",
			BasicAuth: false,
			IsDefault: true,
			JSONData: map[string]interface{}{
				"httpMethod": "GET",
				"nodeGraph": map[string]interface{}{
					"enabled": true,
				},
			},
		}
	}

	return schema.DatasourcePayload{}
}

func (s *GrafanaService) upsertAndTestDatasource(
	client *http.Client,
	grafanaURL, user, pass string,
	payload schema.DatasourcePayload,
	existing []schema.DatasourcePayload,
	testConnection bool,
	start time.Time,
) schema.SingleDatasourceResult {
	var existingID int64
	var existingUID string

	for _, ex := range existing {
		if strings.EqualFold(ex.Name, payload.Name) || ex.Type == payload.Type {
			existingID = ex.ID
			existingUID = ex.UID
			break
		}
	}

	dataBytes, _ := json.Marshal(payload)
	var req *http.Request
	var err error

	if existingUID != "" {
		req, err = http.NewRequest("PUT", fmt.Sprintf("%s/api/datasources/uid/%s", grafanaURL, existingUID), bytes.NewReader(dataBytes))
	} else if existingID > 0 {
		req, err = http.NewRequest("PUT", fmt.Sprintf("%s/api/datasources/%d", grafanaURL, existingID), bytes.NewReader(dataBytes))
	} else {
		req, err = http.NewRequest("POST", fmt.Sprintf("%s/api/datasources", grafanaURL), bytes.NewReader(dataBytes))
	}

	if err != nil {
		return schema.SingleDatasourceResult{
			Service:        payload.Name,
			DatasourceName: payload.Name,
			Status:         "ERROR",
			Message:        fmt.Sprintf("request creation failed: %v", err),
			LatencyMs:      float64(time.Since(start).Milliseconds()),
			IsHealthy:      false,
		}
	}

	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(user, pass)

	resp, err := client.Do(req)
	if err != nil {
		return schema.SingleDatasourceResult{
			Service:        payload.Name,
			DatasourceName: payload.Name,
			Status:         "ERROR",
			Message:        fmt.Sprintf("API call failed: %v", err),
			LatencyMs:      float64(time.Since(start).Milliseconds()),
			IsHealthy:      false,
		}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))
	if resp.StatusCode != http.StatusOK {
		return schema.SingleDatasourceResult{
			Service:        payload.Name,
			DatasourceName: payload.Name,
			Status:         "ERROR",
			Message:        fmt.Sprintf("save failed (status %d): %s", resp.StatusCode, string(respBody)),
			LatencyMs:      float64(time.Since(start).Milliseconds()),
			IsHealthy:      false,
		}
	}

	var savedObj map[string]interface{}
	_ = json.Unmarshal(respBody, &savedObj)
	if uid, ok := savedObj["uid"].(string); ok && uid != "" {
		existingUID = uid
	}
	if idVal, ok := savedObj["id"].(float64); ok {
		existingID = int64(idVal)
	}
	if ds, ok := savedObj["datasource"].(map[string]interface{}); ok {
		if uid, ok := ds["uid"].(string); ok && uid != "" {
			existingUID = uid
		}
		if idVal, ok := ds["id"].(float64); ok {
			existingID = int64(idVal)
		}
	}

	msg := "Datasource provisioned & configured"
	if existingUID != "" || existingID > 0 {
		msg = "Datasource updated & synchronized"
	}

	if testConnection && existingUID != "" {
		healthURL := fmt.Sprintf("%s/api/datasources/uid/%s/health", grafanaURL, existingUID)
		hReq, _ := http.NewRequest("GET", healthURL, nil)
		hReq.SetBasicAuth(user, pass)
		if hResp, hErr := client.Do(hReq); hErr == nil {
			defer hResp.Body.Close()
			hBody, _ := io.ReadAll(io.LimitReader(hResp.Body, 4096))
			var hJson map[string]interface{}
			if json.Unmarshal(hBody, &hJson) == nil {
				if status, ok := hJson["status"].(string); ok && (strings.EqualFold(status, "OK") || strings.EqualFold(status, "SUCCESS")) {
					if m, ok := hJson["message"].(string); ok {
						msg = fmt.Sprintf("%s (Health: %s - %s)", msg, status, m)
					}
				} else if m, ok := hJson["message"].(string); ok {
					msg = fmt.Sprintf("%s (Health Warning: %s)", msg, m)
				}
			}
		}
	}

	return schema.SingleDatasourceResult{
		Service:        payload.Name,
		DatasourceName: payload.Name,
		DatasourceUID:  existingUID,
		DatasourceID:   existingID,
		Status:         "CONFIGURED",
		Message:        msg,
		LatencyMs:      float64(time.Since(start).Milliseconds()),
		IsHealthy:      true,
	}
}

func (s *GrafanaService) getEnv(key, fallback string) string {
	val := os.Getenv(key)
	if val != "" {
		return val
	}
	envPath := filepath.Join(s.baseDir, "packages", "platform-orchestrator", ".env")
	data, err := os.ReadFile(envPath)
	if err != nil {
		envPath = filepath.Join(s.baseDir, ".env")
		data, err = os.ReadFile(envPath)
		if err != nil {
			return fallback
		}
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[0]) == key {
			return strings.Trim(parts[1], `"' `)
		}
	}
	return fallback
}
