/*
Package services provides complete lifecycle management for Grafana datasources, dashboards, and unified alerting rules.

ALGORITHM BLUEPRINT:
1. GrafanaService: Orchestrates authenticated REST requests against Grafana HTTP APIs (/api/datasources, /api/search, /api/dashboards, /api/v1/provisioning/alert-rules, /api/v1/provisioning/contact-points).
2. Datasource Lifecycle: List, Get, Create, Update, Delete, Health Probe, Batch Sync.
3. Dashboard Lifecycle: Search, Get by UID, Save/Update, Import from file/URL, Export to JSON, Delete by UID.
4. Unified Alerting Lifecycle: List/Get/Create/Update/Delete Alert Rules, List/Create/Delete/Test Contact Points (Slack, Webhooks, Email, PagerDuty, etc.).
5. Trace Context: Wraps operations in OpenTelemetry spans with latency attribution and error tagging.
6. Invariants:
   - Zero inline comments inside function bodies.
   - Failures return descriptive Go errors and non-200 responses are captured.
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
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/paths"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

type GrafanaService struct {
	tracer   ports.TracerPort
	baseDir  string
	resolver *paths.PathResolver
}

func NewGrafanaService(tracer ports.TracerPort, baseDir string) *GrafanaService {
	return &GrafanaService{
		tracer:   tracer,
		baseDir:  baseDir,
		resolver: paths.NewPathResolver(baseDir),
	}
}

func (s *GrafanaService) ListDatasources(ctx context.Context, opts types.ClientOptions) ([]schema.DatasourcePayload, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.datasources.list")
	defer endSpan()

	url, user, pass, client := s.resolveClient(opts)
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/api/datasources", url), nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(user, pass)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Grafana at %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("grafana list datasources returned status %d: %s", resp.StatusCode, string(body))
	}

	var datasources []schema.DatasourcePayload
	if err := json.NewDecoder(resp.Body).Decode(&datasources); err != nil {
		return nil, fmt.Errorf("failed to parse datasources JSON response: %w", err)
	}

	return datasources, nil
}

func (s *GrafanaService) GetDatasource(ctx context.Context, opts types.ClientOptions, idOrNameOrUID string) (*schema.DatasourcePayload, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.datasources.get")
	defer endSpan()

	url, user, pass, client := s.resolveClient(opts)
	endpoint := fmt.Sprintf("%s/api/datasources/uid/%s", url, idOrNameOrUID)

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(user, pass)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var ds schema.DatasourcePayload
		if err := json.NewDecoder(resp.Body).Decode(&ds); err == nil {
			return &ds, nil
		}
	}

	reqName, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/api/datasources/name/%s", url, idOrNameOrUID), nil)
	reqName.SetBasicAuth(user, pass)
	respName, errName := client.Do(reqName)
	if errName == nil {
		defer respName.Body.Close()
		if respName.StatusCode == http.StatusOK {
			var ds schema.DatasourcePayload
			if err := json.NewDecoder(respName.Body).Decode(&ds); err == nil {
				return &ds, nil
			}
		}
	}

	return nil, fmt.Errorf("datasource %q not found in Grafana", idOrNameOrUID)
}

func (s *GrafanaService) CreateDatasource(ctx context.Context, opts types.ClientOptions, payload schema.DatasourcePayload) (*schema.SingleDatasourceResult, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.datasources.create")
	defer endSpan()

	payload = rules.NormalizeDatasourcePayload(payload)
	if err := rules.ValidateDatasourcePayload(payload); err != nil {
		return nil, err
	}

	url, user, pass, client := s.resolveClient(opts)
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/api/datasources", url), bytes.NewReader(bodyBytes))
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

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))
	latency := float64(time.Since(start).Milliseconds())

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("grafana create datasource failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var createdObj struct {
		ID         int64  `json:"id"`
		UID        string `json:"uid"`
		Message    string `json:"message"`
		Datasource struct {
			ID  int64  `json:"id"`
			UID string `json:"uid"`
		} `json:"datasource"`
	}
	_ = json.Unmarshal(respBody, &createdObj)

	uid := createdObj.UID
	if uid == "" {
		uid = createdObj.Datasource.UID
	}
	id := createdObj.ID
	if id == 0 {
		id = createdObj.Datasource.ID
	}

	return &schema.SingleDatasourceResult{
		Service:        payload.Name,
		DatasourceName: payload.Name,
		DatasourceUID:  uid,
		DatasourceID:   id,
		Status:         "CREATED",
		Message:        "Datasource created successfully",
		LatencyMs:      latency,
		IsHealthy:      true,
	}, nil
}

func (s *GrafanaService) UpdateDatasource(ctx context.Context, opts types.ClientOptions, idOrUID string, payload schema.DatasourcePayload) (*schema.SingleDatasourceResult, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.datasources.update")
	defer endSpan()

	payload = rules.NormalizeDatasourcePayload(payload)
	url, user, pass, client := s.resolveClient(opts)

	existing, err := s.GetDatasource(ctx, opts, idOrUID)
	if err == nil && existing != nil {
		if payload.ID == 0 {
			payload.ID = existing.ID
		}
		if payload.UID == "" {
			payload.UID = existing.UID
		}
		if payload.Name == "" {
			payload.Name = existing.Name
		}
		if payload.Type == "" {
			payload.Type = existing.Type
		}
	}

	targetUID := payload.UID
	if targetUID == "" {
		targetUID = idOrUID
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	endpoint := fmt.Sprintf("%s/api/datasources/uid/%s", url, targetUID)
	req, err := http.NewRequestWithContext(ctx, "PUT", endpoint, bytes.NewReader(bodyBytes))
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

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))
	latency := float64(time.Since(start).Milliseconds())

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("grafana update datasource failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return &schema.SingleDatasourceResult{
		Service:        payload.Name,
		DatasourceName: payload.Name,
		DatasourceUID:  targetUID,
		Status:         "UPDATED",
		Message:        "Datasource updated successfully",
		LatencyMs:      latency,
		IsHealthy:      true,
	}, nil
}

func (s *GrafanaService) DeleteDatasource(ctx context.Context, opts types.ClientOptions, idOrUIDOrName string) (*types.DeleteDatasourceResult, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.datasources.delete")
	defer endSpan()

	url, user, pass, client := s.resolveClient(opts)
	ds, err := s.GetDatasource(ctx, opts, idOrUIDOrName)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/api/datasources/uid/%s", url, ds.UID)
	req, err := http.NewRequestWithContext(ctx, "DELETE", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(user, pass)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to delete datasource: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("failed to delete datasource (status %d): %s", resp.StatusCode, string(body))
	}

	return &types.DeleteDatasourceResult{
		UID:     ds.UID,
		Message: fmt.Sprintf("Datasource %q (UID: %s) deleted successfully", ds.Name, ds.UID),
		Success: true,
	}, nil
}

func (s *GrafanaService) TestDatasourceHealth(ctx context.Context, opts types.ClientOptions, idOrUIDOrName string) (*types.DatasourceHealthResult, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.datasources.test_health")
	defer endSpan()

	url, user, pass, client := s.resolveClient(opts)
	ds, err := s.GetDatasource(ctx, opts, idOrUIDOrName)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	endpoint := fmt.Sprintf("%s/api/datasources/uid/%s/health", url, ds.UID)
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(user, pass)

	resp, err := client.Do(req)
	if err != nil {
		return &types.DatasourceHealthResult{
			UID:       ds.UID,
			Name:      ds.Name,
			Status:    "ERROR",
			Message:   fmt.Sprintf("health check probe failed: %v", err),
			LatencyMs: float64(time.Since(start).Milliseconds()),
			IsHealthy: false,
		}, nil
	}
	defer resp.Body.Close()

	latency := float64(time.Since(start).Milliseconds())
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))

	var healthResp struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &healthResp)

	status := strings.ToUpper(healthResp.Status)
	isHealthy := status == "OK" || status == "SUCCESS"
	if status == "" {
		status = fmt.Sprintf("HTTP_%d", resp.StatusCode)
		isHealthy = resp.StatusCode == http.StatusOK
	}

	msg := healthResp.Message
	if msg == "" {
		msg = string(body)
	}

	return &types.DatasourceHealthResult{
		UID:       ds.UID,
		Name:      ds.Name,
		Status:    status,
		Message:   msg,
		LatencyMs: latency,
		IsHealthy: isHealthy,
	}, nil
}

func (s *GrafanaService) SearchDashboards(ctx context.Context, opts types.ClientOptions, query, folderUID, tag string) ([]types.DashboardSearchResult, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.dashboards.search")
	defer endSpan()

	url, user, pass, client := s.resolveClient(opts)
	searchURL := fmt.Sprintf("%s/api/search?type=dash-db", url)
	if query != "" {
		searchURL += fmt.Sprintf("&query=%s", query)
	}
	if folderUID != "" {
		searchURL += fmt.Sprintf("&folderUIDs=%s", folderUID)
	}
	if tag != "" {
		searchURL += fmt.Sprintf("&tag=%s", tag)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(user, pass)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query dashboards from %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("dashboard search failed (status %d): %s", resp.StatusCode, string(body))
	}

	var results []types.DashboardSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("failed to parse dashboard search JSON: %w", err)
	}

	return results, nil
}

func (s *GrafanaService) GetDashboard(ctx context.Context, opts types.ClientOptions, uid string) (*schema.DashboardDetail, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.dashboards.get")
	defer endSpan()

	url, user, pass, client := s.resolveClient(opts)
	endpoint := fmt.Sprintf("%s/api/dashboards/uid/%s", url, uid)

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(user, pass)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch dashboard %s: %w", uid, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("dashboard %s fetch failed (status %d): %s", uid, resp.StatusCode, string(body))
	}

	var detail schema.DashboardDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return nil, fmt.Errorf("failed to parse dashboard JSON: %w", err)
	}

	return &detail, nil
}

func (s *GrafanaService) CreateOrUpdateDashboard(ctx context.Context, opts types.ClientOptions, payload schema.DashboardPayload) (*types.DashboardSaveResult, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.dashboards.save")
	defer endSpan()

	if err := rules.ValidateDashboardPayload(payload); err != nil {
		return nil, err
	}

	url, user, pass, client := s.resolveClient(opts)
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/api/dashboards/db", url), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(user, pass)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to save dashboard: %w", err)
	}
	defer resp.Body.Close()

	latency := float64(time.Since(start).Milliseconds())
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("save dashboard failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var saveRes types.DashboardSaveResult
	if err := json.Unmarshal(respBody, &saveRes); err != nil {
		return nil, fmt.Errorf("failed to parse save dashboard response: %w", err)
	}
	saveRes.Latency = latency

	return &saveRes, nil
}

func (s *GrafanaService) ImportDashboard(ctx context.Context, opts types.ClientOptions, importOpts schema.DashboardImportOptions) (*types.DashboardSaveResult, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.dashboards.import")
	defer endSpan()

	var data []byte
	var err error

	if strings.HasPrefix(importOpts.SourcePathOrURL, "http://") || strings.HasPrefix(importOpts.SourcePathOrURL, "https://") {
		req, _ := http.NewRequestWithContext(ctx, "GET", importOpts.SourcePathOrURL, nil)
		res, fetchErr := http.DefaultClient.Do(req)
		if fetchErr != nil {
			return nil, fmt.Errorf("failed to download dashboard from %s: %w", importOpts.SourcePathOrURL, fetchErr)
		}
		defer res.Body.Close()
		data, err = io.ReadAll(res.Body)
	} else {
		resolvedPath := importOpts.SourcePathOrURL
		if !filepath.IsAbs(resolvedPath) {
			resolvedPath = filepath.Join(s.baseDir, resolvedPath)
		}
		data, err = os.ReadFile(resolvedPath)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read dashboard content: %w", err)
	}

	var rootObj map[string]interface{}
	if err := json.Unmarshal(data, &rootObj); err != nil {
		return nil, fmt.Errorf("invalid JSON dashboard payload: %w", err)
	}

	var dashMap map[string]interface{}
	if nestedDash, ok := rootObj["dashboard"].(map[string]interface{}); ok {
		dashMap = nestedDash
	} else {
		dashMap = rootObj
	}

	if importOpts.TitleOverride != "" {
		dashMap["title"] = importOpts.TitleOverride
	}

	payload := schema.DashboardPayload{
		Dashboard: dashMap,
		FolderUID: importOpts.FolderUID,
		Overwrite: importOpts.Overwrite,
		Message:   "Imported via LLMObs Orchestrator",
	}

	return s.CreateOrUpdateDashboard(ctx, opts, payload)
}

func (s *GrafanaService) DeleteDashboard(ctx context.Context, opts types.ClientOptions, uid string) (*types.DashboardDeleteResult, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.dashboards.delete")
	defer endSpan()

	url, user, pass, client := s.resolveClient(opts)
	endpoint := fmt.Sprintf("%s/api/dashboards/uid/%s", url, uid)

	req, err := http.NewRequestWithContext(ctx, "DELETE", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(user, pass)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to delete dashboard %s: %w", uid, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("delete dashboard %s failed (status %d): %s", uid, resp.StatusCode, string(body))
	}

	var delRes types.DashboardDeleteResult
	_ = json.NewDecoder(resp.Body).Decode(&delRes)
	delRes.Success = true

	return &delRes, nil
}

func (s *GrafanaService) ListAlertRules(ctx context.Context, opts types.ClientOptions) ([]schema.AlertRulePayload, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.alerts.list_rules")
	defer endSpan()

	url, user, pass, client := s.resolveClient(opts)
	endpoint := fmt.Sprintf("%s/api/v1/provisioning/alert-rules", url)

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(user, pass)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch alert rules: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("fetch alert rules failed (status %d): %s", resp.StatusCode, string(body))
	}

	var rulesList []schema.AlertRulePayload
	if err := json.NewDecoder(resp.Body).Decode(&rulesList); err != nil {
		return nil, fmt.Errorf("failed to decode alert rules: %w", err)
	}

	return rulesList, nil
}

func (s *GrafanaService) GetAlertRule(ctx context.Context, opts types.ClientOptions, uid string) (*schema.AlertRulePayload, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.alerts.get_rule")
	defer endSpan()

	url, user, pass, client := s.resolveClient(opts)
	endpoint := fmt.Sprintf("%s/api/v1/provisioning/alert-rules/%s", url, uid)

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(user, pass)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch alert rule %s: %w", uid, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("alert rule %s not found (status %d): %s", uid, resp.StatusCode, string(body))
	}

	var rule schema.AlertRulePayload
	if err := json.NewDecoder(resp.Body).Decode(&rule); err != nil {
		return nil, fmt.Errorf("failed to decode alert rule JSON: %w", err)
	}

	return &rule, nil
}

func (s *GrafanaService) CreateOrUpdateAlertRule(ctx context.Context, opts types.ClientOptions, rule schema.AlertRulePayload) (*types.AlertOperationResult, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.alerts.save_rule")
	defer endSpan()

	if err := rules.ValidateAlertRulePayload(rule); err != nil {
		return nil, err
	}

	url, user, pass, client := s.resolveClient(opts)
	bodyBytes, err := json.Marshal(rule)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	method := "POST"
	endpoint := fmt.Sprintf("%s/api/v1/provisioning/alert-rules", url)
	if rule.UID != "" {
		method = "PUT"
		endpoint = fmt.Sprintf("%s/api/v1/provisioning/alert-rules/%s", url, rule.UID)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(user, pass)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to save alert rule: %w", err)
	}
	defer resp.Body.Close()

	latency := float64(time.Since(start).Milliseconds())
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("alert rule save failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var savedRule schema.AlertRulePayload
	_ = json.Unmarshal(respBody, &savedRule)
	resUID := savedRule.UID
	if resUID == "" {
		resUID = rule.UID
	}

	return &types.AlertOperationResult{
		UID:       resUID,
		Title:     rule.Title,
		Message:   "Alert rule configured successfully",
		Status:    "SUCCESS",
		Success:   true,
		LatencyMs: latency,
	}, nil
}

func (s *GrafanaService) DeleteAlertRule(ctx context.Context, opts types.ClientOptions, uid string) (*types.AlertOperationResult, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.alerts.delete_rule")
	defer endSpan()

	url, user, pass, client := s.resolveClient(opts)
	endpoint := fmt.Sprintf("%s/api/v1/provisioning/alert-rules/%s", url, uid)

	req, err := http.NewRequestWithContext(ctx, "DELETE", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(user, pass)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to delete alert rule: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("failed to delete alert rule %s (status %d): %s", uid, resp.StatusCode, string(body))
	}

	return &types.AlertOperationResult{
		UID:     uid,
		Message: fmt.Sprintf("Alert rule %s deleted successfully", uid),
		Status:  "DELETED",
		Success: true,
	}, nil
}

func (s *GrafanaService) ListContactPoints(ctx context.Context, opts types.ClientOptions) ([]schema.ContactPointPayload, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.alerts.list_contact_points")
	defer endSpan()

	url, user, pass, client := s.resolveClient(opts)
	endpoint := fmt.Sprintf("%s/api/v1/provisioning/contact-points", url)

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(user, pass)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch contact points: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("fetch contact points failed (status %d): %s", resp.StatusCode, string(body))
	}

	var cpList []schema.ContactPointPayload
	if err := json.NewDecoder(resp.Body).Decode(&cpList); err != nil {
		return nil, fmt.Errorf("failed to decode contact points: %w", err)
	}

	return cpList, nil
}

func (s *GrafanaService) CreateOrUpdateContactPoint(ctx context.Context, opts types.ClientOptions, cp schema.ContactPointPayload) (*types.AlertOperationResult, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.alerts.save_contact_point")
	defer endSpan()

	if err := rules.ValidateContactPointPayload(cp); err != nil {
		return nil, err
	}

	url, user, pass, client := s.resolveClient(opts)
	bodyBytes, err := json.Marshal(cp)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	method := "POST"
	endpoint := fmt.Sprintf("%s/api/v1/provisioning/contact-points", url)
	if cp.UID != "" {
		method = "PUT"
		endpoint = fmt.Sprintf("%s/api/v1/provisioning/contact-points/%s", url, cp.UID)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(user, pass)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to save contact point: %w", err)
	}
	defer resp.Body.Close()

	latency := float64(time.Since(start).Milliseconds())
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("contact point save failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	return &types.AlertOperationResult{
		UID:       cp.UID,
		Title:     cp.Name,
		Message:   "Contact point created/updated successfully",
		Status:    "SUCCESS",
		Success:   true,
		LatencyMs: latency,
	}, nil
}

func (s *GrafanaService) DeleteContactPoint(ctx context.Context, opts types.ClientOptions, uid string) (*types.AlertOperationResult, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.alerts.delete_contact_point")
	defer endSpan()

	url, user, pass, client := s.resolveClient(opts)
	endpoint := fmt.Sprintf("%s/api/v1/provisioning/contact-points/%s", url, uid)

	req, err := http.NewRequestWithContext(ctx, "DELETE", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(user, pass)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to delete contact point: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("failed to delete contact point %s (status %d): %s", uid, resp.StatusCode, string(body))
	}

	return &types.AlertOperationResult{
		UID:     uid,
		Message: fmt.Sprintf("Contact point %s deleted successfully", uid),
		Status:  "DELETED",
		Success: true,
	}, nil
}

func (s *GrafanaService) TestContactPoint(ctx context.Context, opts types.ClientOptions, cp schema.ContactPointPayload) (*types.AlertOperationResult, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.alerts.test_contact_point")
	defer endSpan()

	if err := rules.ValidateContactPointPayload(cp); err != nil {
		return nil, err
	}

	url, user, pass, client := s.resolveClient(opts)
	bodyBytes, err := json.Marshal(cp)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	endpoint := fmt.Sprintf("%s/api/v1/provisioning/contact-points/test", url)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(user, pass)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to test contact point: %w", err)
	}
	defer resp.Body.Close()

	latency := float64(time.Since(start).Milliseconds())
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return &types.AlertOperationResult{
			Title:     cp.Name,
			Message:   fmt.Sprintf("Contact point test failed (status %d): %s", resp.StatusCode, string(respBody)),
			Status:    "FAILED",
			Success:   false,
			LatencyMs: latency,
		}, nil
	}

	return &types.AlertOperationResult{
		Title:     cp.Name,
		Message:   "Contact point test notification sent successfully",
		Status:    "SUCCESS",
		Success:   true,
		LatencyMs: latency,
	}, nil
}

func (s *GrafanaService) SyncDatasources(ctx context.Context, opts schema.DatasourceSyncOptions) (*schema.DatasourceSyncReport, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.datasources.sync")
	defer endSpan()

	clientOpts := types.ClientOptions{
		GrafanaURL: opts.GrafanaURL,
		Username:   opts.GrafanaUser,
		Password:   opts.GrafanaPass,
		Timeout:    opts.Timeout,
	}

	url, user, pass, client := s.resolveClient(clientOpts)

	servicesToSync := opts.Services
	if len(servicesToSync) == 0 {
		servicesToSync = []string{"alloydb", "clickhouse", "redis", "tempo"}
	}

	var results []schema.SingleDatasourceResult

	for _, svc := range servicesToSync {
		payload, ok := s.buildServiceDatasourcePayload(svc)
		if !ok {
			results = append(results, schema.SingleDatasourceResult{
				Service:        svc,
				DatasourceName: svc,
				Status:         "SKIPPED",
				Message:        fmt.Sprintf("unknown service identifier %q", svc),
				IsHealthy:      false,
			})
			continue
		}

		res := s.provisionSingleDatasource(ctx, client, url, user, pass, payload, opts.TestConnection)
		results = append(results, res)
	}

	successCount := 0
	for _, r := range results {
		if r.IsHealthy {
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

func (s *GrafanaService) resolveClient(opts types.ClientOptions) (string, string, string, *http.Client) {
	url := opts.GrafanaURL
	if url == "" {
		url = s.resolver.ResolveServiceURL("grafana", "http://localhost:31415")
	}
	user := opts.Username
	if user == "" {
		user = s.resolver.ResolveServiceUser("grafana", "admin")
	}
	pass := opts.Password
	if pass == "" {
		pass = s.resolver.ResolveServicePassword("grafana", "")
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	client := &http.Client{Timeout: timeout}
	return strings.TrimRight(url, "/"), user, pass, client
}

func (s *GrafanaService) buildServiceDatasourcePayload(svc string) (schema.DatasourcePayload, bool) {
	svcLower := strings.ToLower(strings.TrimSpace(svc))

	switch svcLower {
	case "alloydb", "postgres", "postgresql", "db":
		user := s.resolver.ResolveServiceUser("alloydb", "admin")
		pass := s.resolver.ResolveServicePassword("alloydb", "")
		db := s.resolver.ResolveServiceDatabase("alloydb", "llm_observability")
		host := s.resolver.ResolveServiceHost("alloydb", "llmobs-alloydb")
		port := s.resolver.ResolveServicePort("alloydb", 5432)
		return schema.DatasourcePayload{
			Name:      "AlloyDB",
			Type:      "grafana-postgresql-datasource",
			TypeName:  "PostgreSQL",
			Access:    "proxy",
			URL:       fmt.Sprintf("%s:%d", host, port),
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

	case "clickhouse", "analytics", "ch":
		user := s.resolver.ResolveServiceUser("clickhouse", "default")
		pass := s.resolver.ResolveServicePassword("clickhouse", "")
		db := s.resolver.ResolveServiceDatabase("clickhouse", "llm_telemetry_analytics")
		host := s.resolver.ResolveServiceHost("clickhouse", "llmobs-clickhouse")
		port := s.resolver.GetServiceDefinition("clickhouse").TCPPort
		if port <= 0 {
			port = 9000
		}
		return schema.DatasourcePayload{
			Name:      "ClickHouse",
			Type:      "grafana-clickhouse-datasource",
			TypeName:  "ClickHouse",
			Access:    "proxy",
			URL:       fmt.Sprintf("%s:%d", host, port),
			User:      user,
			Database:  db,
			BasicAuth: false,
			IsDefault: false,
			JSONData: map[string]interface{}{
				"port":          port,
				"server":        host,
				"protocol":      "native",
				"secure":        false,
				"defaultDb":     db,
				"tlsSkipVerify": true,
			},
			SecureJSONData: map[string]string{
				"password": pass,
			},
		}, true

	case "redis", "cache", "spend":
		pass := s.resolver.ResolveServicePassword("redis", "")
		url := s.resolver.ResolveServiceURL("redis", "")
		if url == "" {
			host := s.resolver.ResolveServiceHost("redis", "llmobs-redis")
			port := s.resolver.ResolveServicePort("redis", 6379)
			url = fmt.Sprintf("redis://%s:%d", host, port)
		}
		return schema.DatasourcePayload{
			Name:      "Redis",
			Type:      "redis-datasource",
			TypeName:  "Redis",
			Access:    "proxy",
			URL:       url,
			BasicAuth: false,
			IsDefault: false,
			JSONData: map[string]interface{}{
				"poolSize":     5,
				"timeout":      10,
				"pingInterval": 0,
				"pipeline":     false,
			},
			SecureJSONData: map[string]string{
				"password": pass,
			},
		}, true

	case "tempo", "tracing", "traces":
		url := s.resolver.ResolveServiceURL("tempo", "http://llmobs-tempo:3200")
		return schema.DatasourcePayload{
			Name:      "Tempo",
			Type:      "tempo",
			TypeName:  "Tempo",
			Access:    "proxy",
			URL:       url,
			BasicAuth: false,
			IsDefault: true,
			JSONData: map[string]interface{}{
				"httpMethod": "GET",
				"tracesToLogs": map[string]interface{}{
					"datasourceUid": "",
					"filterByTrace": true,
				},
				"serviceMap": map[string]interface{}{
					"datasourceUid": "",
				},
				"search": map[string]interface{}{
					"hide": false,
				},
				"nodeGraph": map[string]interface{}{
					"enabled": true,
				},
			},
		}, true

	case "prometheus", "prom", "metrics":
		url := s.resolver.ResolveServiceURL("prometheus", "http://llmobs-prometheus:9090")
		return schema.DatasourcePayload{
			Name:      "Prometheus",
			Type:      "prometheus",
			TypeName:  "Prometheus",
			Access:    "proxy",
			URL:       url,
			BasicAuth: false,
			IsDefault: false,
			JSONData: map[string]interface{}{
				"httpMethod": "POST",
			},
		}, true

	case "loki", "logs":
		url := s.resolver.ResolveServiceURL("loki", "http://llmobs-loki:3100")
		return schema.DatasourcePayload{
			Name:      "Loki",
			Type:      "loki",
			TypeName:  "Loki",
			Access:    "proxy",
			URL:       url,
			BasicAuth: false,
			IsDefault: false,
		}, true

	default:
		return schema.DatasourcePayload{}, false
	}
}

func (s *GrafanaService) provisionSingleDatasource(ctx context.Context, client *http.Client, grafanaURL, user, pass string, payload schema.DatasourcePayload, testConnection bool) schema.SingleDatasourceResult {
	start := time.Now()

	var existingUID string
	var existingID int64

	checkURL := fmt.Sprintf("%s/api/datasources/name/%s", grafanaURL, payload.Name)
	reqCheck, err := http.NewRequestWithContext(ctx, "GET", checkURL, nil)
	if err == nil {
		reqCheck.SetBasicAuth(user, pass)
		if respCheck, errCheck := client.Do(reqCheck); errCheck == nil {
			defer respCheck.Body.Close()
			if respCheck.StatusCode == http.StatusOK {
				var existingObj struct {
					ID  int64  `json:"id"`
					UID string `json:"uid"`
				}
				if json.NewDecoder(respCheck.Body).Decode(&existingObj) == nil {
					existingUID = existingObj.UID
					existingID = existingObj.ID
				}
			}
		}
	}

	method := "POST"
	targetURL := fmt.Sprintf("%s/api/datasources", grafanaURL)
	if existingUID != "" {
		method = "PUT"
		targetURL = fmt.Sprintf("%s/api/datasources/uid/%s", grafanaURL, existingUID)
		payload.ID = existingID
		payload.UID = existingUID
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return schema.SingleDatasourceResult{
			Service:        payload.Name,
			DatasourceName: payload.Name,
			Status:         "ERROR",
			Message:        fmt.Sprintf("serialization failed: %v", err),
			LatencyMs:      float64(time.Since(start).Milliseconds()),
			IsHealthy:      false,
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, targetURL, bytes.NewReader(bodyBytes))
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
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
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
	if existingUID == "" {
		if uid, ok := savedObj["uid"].(string); ok && uid != "" {
			existingUID = uid
		} else if ds, ok := savedObj["datasource"].(map[string]interface{}); ok {
			if uid, ok := ds["uid"].(string); ok {
				existingUID = uid
			}
		}
	}

	msg := "Datasource configured and active"
	if existingUID != "" {
		msg = fmt.Sprintf("Datasource configured (UID: %s)", existingUID)
	}

	if testConnection && existingUID != "" {
		healthURL := fmt.Sprintf("%s/api/datasources/uid/%s/health", grafanaURL, existingUID)
		hReq, _ := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
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
	return s.resolver.ResolveEnvOrConfig(key, fallback)
}
