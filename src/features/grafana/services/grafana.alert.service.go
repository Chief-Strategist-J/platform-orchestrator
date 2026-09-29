/*
Package services provides isolated, domain-specific services for Grafana lifecycle management.

ALGORITHM BLUEPRINT (AlertService):
1. Alert Rules Lifecycle: List, Get, Upsert, Delete Grafana Unified Alerting rules.
2. Contact Points Lifecycle: List, Create/Update, Delete, Test notification channels (Slack, Webhook, Email, PagerDuty).
3. Folder Verification: Automatically ensures prerequisite folder existence before provisioning rules.
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
	"time"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/client"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/endpoints"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/rules"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/schema"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/types"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/paths"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

type AlertService struct {
	tracer   ports.TracerPort
	baseDir  string
	resolver *paths.PathResolver
}

func NewAlertService(tracer ports.TracerPort, baseDir string) *AlertService {
	return &AlertService{
		tracer:   tracer,
		baseDir:  baseDir,
		resolver: paths.NewPathResolver(baseDir),
	}
}

func (s *AlertService) ListAlertRules(ctx context.Context, opts types.ClientOptions) ([]schema.AlertRulePayload, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.alerts.list")
	defer endSpan()

	c := client.NewGrafanaClient(opts, s.resolver)
	var rulesList []schema.AlertRulePayload

	_, err := c.Do(ctx, http.MethodGet, endpoints.EndpointAlertRules, nil, &rulesList)
	if err != nil {
		return nil, err
	}

	return rulesList, nil
}

func (s *AlertService) GetAlertRule(ctx context.Context, opts types.ClientOptions, uid string) (*schema.AlertRulePayload, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.alerts.get")
	defer endSpan()

	c := client.NewGrafanaClient(opts, s.resolver)
	rulePath := endpoints.BuildAlertRuleUIDPath(uid)

	var rule schema.AlertRulePayload
	_, err := c.Do(ctx, http.MethodGet, rulePath, nil, &rule)
	if err != nil {
		return nil, err
	}

	return &rule, nil
}

func (s *AlertService) CreateOrUpdateAlertRule(ctx context.Context, opts types.ClientOptions, rule schema.AlertRulePayload) (*types.AlertOperationResult, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.alerts.save")
	defer endSpan()

	rule = rules.NormalizeAlertRulePayload(rule)
	if err := rules.ValidateAlertRulePayload(rule); err != nil {
		return nil, err
	}

	c := client.NewGrafanaClient(opts, s.resolver)
	start := time.Now()

	if rule.FolderUID != "" {
		_ = s.EnsureFolderExists(ctx, opts, rule.FolderUID)
	}

	targetPath := endpoints.BuildAlertRuleUIDPath(rule.UID)
	var existingRule schema.AlertRulePayload
	status, err := c.Do(ctx, http.MethodGet, targetPath, nil, &existingRule)

	var result types.AlertOperationResult

	if err == nil && status == http.StatusOK {
		var updateResp struct {
			UID     string `json:"uid"`
			Title   string `json:"title"`
			Message string `json:"message"`
		}
		_, err = c.Do(ctx, http.MethodPut, targetPath, rule, &updateResp)
		latency := float64(time.Since(start).Microseconds()) / 1000.0

		if err != nil {
			result = types.AlertOperationResult{
				UID:       rule.UID,
				Title:     rule.Title,
				Message:   fmt.Sprintf("Failed to update alert rule: %v", err),
				Status:    "failed",
				Success:   false,
				LatencyMs: latency,
			}
			return &result, err
		}

		result = types.AlertOperationResult{
			UID:       rule.UID,
			Title:     rule.Title,
			Message:   "Alert rule updated successfully",
			Status:    "updated",
			Success:   true,
			LatencyMs: latency,
		}
	} else {
		var createResp struct {
			UID     string `json:"uid"`
			Title   string `json:"title"`
			Message string `json:"message"`
		}
		_, err = c.Do(ctx, http.MethodPost, endpoints.EndpointAlertRules, rule, &createResp)
		latency := float64(time.Since(start).Microseconds()) / 1000.0

		if err != nil {
			result = types.AlertOperationResult{
				UID:       rule.UID,
				Title:     rule.Title,
				Message:   fmt.Sprintf("Failed to create alert rule: %v", err),
				Status:    "failed",
				Success:   false,
				LatencyMs: latency,
			}
			return &result, err
		}

		resUID := createResp.UID
		if resUID == "" {
			resUID = rule.UID
		}

		result = types.AlertOperationResult{
			UID:       resUID,
			Title:     rule.Title,
			Message:   "Alert rule created successfully",
			Status:    "created",
			Success:   true,
			LatencyMs: latency,
		}
	}

	return &result, nil
}

func (s *AlertService) DeleteAlertRule(ctx context.Context, opts types.ClientOptions, uid string) (*types.AlertOperationResult, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.alerts.delete")
	defer endSpan()

	c := client.NewGrafanaClient(opts, s.resolver)
	start := time.Now()
	rulePath := endpoints.BuildAlertRuleUIDPath(uid)

	var deleteResponse struct {
		Message string `json:"message"`
	}

	_, err := c.Do(ctx, http.MethodDelete, rulePath, nil, &deleteResponse)
	latency := float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil {
		return &types.AlertOperationResult{
			UID:       uid,
			Message:   fmt.Sprintf("Failed to delete alert rule: %v", err),
			Status:    "failed",
			Success:   false,
			LatencyMs: latency,
		}, err
	}

	return &types.AlertOperationResult{
		UID:       uid,
		Message:   deleteResponse.Message,
		Status:    "deleted",
		Success:   true,
		LatencyMs: latency,
	}, nil
}

func (s *AlertService) ListContactPoints(ctx context.Context, opts types.ClientOptions) ([]schema.ContactPointPayload, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.contact_points.list")
	defer endSpan()

	c := client.NewGrafanaClient(opts, s.resolver)
	var contactPoints []schema.ContactPointPayload

	_, err := c.Do(ctx, http.MethodGet, endpoints.EndpointContactPoints, nil, &contactPoints)
	if err != nil {
		return nil, err
	}

	return contactPoints, nil
}

func (s *AlertService) CreateOrUpdateContactPoint(ctx context.Context, opts types.ClientOptions, cp schema.ContactPointPayload) (*types.AlertOperationResult, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.contact_points.save")
	defer endSpan()

	cp = rules.NormalizeContactPointPayload(cp)
	if err := rules.ValidateContactPointPayload(cp); err != nil {
		return nil, err
	}

	c := client.NewGrafanaClient(opts, s.resolver)
	start := time.Now()

	var existingCPs []schema.ContactPointPayload
	_, _ = c.Do(ctx, http.MethodGet, endpoints.EndpointContactPoints, nil, &existingCPs)

	found := false
	for _, existing := range existingCPs {
		if existing.UID == cp.UID || existing.Name == cp.Name {
			found = true
			if cp.UID == "" {
				cp.UID = existing.UID
			}
			break
		}
	}

	var result types.AlertOperationResult

	if found && cp.UID != "" {
		targetPath := endpoints.BuildContactPointUIDPath(cp.UID)
		var updateResp struct {
			UID     string `json:"uid"`
			Name    string `json:"name"`
			Message string `json:"message"`
		}
		_, err := c.Do(ctx, http.MethodPut, targetPath, cp, &updateResp)
		latency := float64(time.Since(start).Microseconds()) / 1000.0

		if err != nil {
			return &types.AlertOperationResult{
				UID:       cp.UID,
				Title:     cp.Name,
				Message:   fmt.Sprintf("Failed to update contact point: %v", err),
				Status:    "failed",
				Success:   false,
				LatencyMs: latency,
			}, err
		}

		result = types.AlertOperationResult{
			UID:       cp.UID,
			Title:     cp.Name,
			Message:   "Contact point updated successfully",
			Status:    "updated",
			Success:   true,
			LatencyMs: latency,
		}
	} else {
		var createResp struct {
			UID     string `json:"uid"`
			Name    string `json:"name"`
			Message string `json:"message"`
		}
		_, err := c.Do(ctx, http.MethodPost, endpoints.EndpointContactPoints, cp, &createResp)
		latency := float64(time.Since(start).Microseconds()) / 1000.0

		if err != nil {
			return &types.AlertOperationResult{
				UID:       cp.UID,
				Title:     cp.Name,
				Message:   fmt.Sprintf("Failed to create contact point: %v", err),
				Status:    "failed",
				Success:   false,
				LatencyMs: latency,
			}, err
		}

		resUID := createResp.UID
		if resUID == "" {
			resUID = cp.UID
		}

		result = types.AlertOperationResult{
			UID:       resUID,
			Title:     cp.Name,
			Message:   "Contact point created successfully",
			Status:    "created",
			Success:   true,
			LatencyMs: latency,
		}
	}

	return &result, nil
}

func (s *AlertService) DeleteContactPoint(ctx context.Context, opts types.ClientOptions, uid string) (*types.AlertOperationResult, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.contact_points.delete")
	defer endSpan()

	c := client.NewGrafanaClient(opts, s.resolver)
	start := time.Now()
	targetPath := endpoints.BuildContactPointUIDPath(uid)

	var deleteResponse struct {
		Message string `json:"message"`
	}

	_, err := c.Do(ctx, http.MethodDelete, targetPath, nil, &deleteResponse)
	latency := float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil {
		return &types.AlertOperationResult{
			UID:       uid,
			Message:   fmt.Sprintf("Failed to delete contact point: %v", err),
			Status:    "failed",
			Success:   false,
			LatencyMs: latency,
		}, err
	}

	return &types.AlertOperationResult{
		UID:       uid,
		Message:   deleteResponse.Message,
		Status:    "deleted",
		Success:   true,
		LatencyMs: latency,
	}, nil
}

func (s *AlertService) TestContactPoint(ctx context.Context, opts types.ClientOptions, cp schema.ContactPointPayload) (*types.AlertOperationResult, error) {
	ctx, endSpan := s.tracer.StartSpan(ctx, "grafana.contact_points.test")
	defer endSpan()

	cp = rules.NormalizeContactPointPayload(cp)
	c := client.NewGrafanaClient(opts, s.resolver)
	start := time.Now()

	testPayload := map[string]interface{}{
		"annotation": map[string]string{
			"description": "Synthetic test alert notification from LLMObs Platform Orchestrator",
			"summary":     "Platform Alert Verification",
		},
		"labels": map[string]string{
			"alertname": "TestAlert",
			"severity":  "info",
		},
		"receivers": []map[string]interface{}{
			{
				"name":     cp.Name,
				"type":     cp.Type,
				"settings": cp.Settings,
			},
		},
	}

	var testResponse struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}

	_, err := c.Do(ctx, http.MethodPost, endpoints.EndpointContactPointsTest, testPayload, &testResponse)
	latency := float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil {
		return &types.AlertOperationResult{
			Title:     cp.Name,
			Status:    "failed",
			Message:   fmt.Sprintf("Contact point test failed: %v", err),
			Success:   false,
			LatencyMs: latency,
		}, err
	}

	return &types.AlertOperationResult{
		Title:     cp.Name,
		Status:    "success",
		Message:   "Test notification successfully dispatched and accepted",
		Success:   true,
		LatencyMs: latency,
	}, nil
}

func (s *AlertService) EnsureFolderExists(ctx context.Context, opts types.ClientOptions, folderUID string) error {
	c := client.NewGrafanaClient(opts, s.resolver)
	targetPath := fmt.Sprintf("%s/%s", endpoints.EndpointFolders, folderUID)

	var existingFolder struct {
		ID    int64  `json:"id"`
		UID   string `json:"uid"`
		Title string `json:"title"`
	}

	status, err := c.Do(ctx, http.MethodGet, targetPath, nil, &existingFolder)
	if err == nil && status == http.StatusOK {
		return nil
	}

	folderPayload := map[string]string{
		"uid":   folderUID,
		"title": fmt.Sprintf("Folder-%s", folderUID),
	}

	_, err = c.Do(ctx, http.MethodPost, endpoints.EndpointFolders, folderPayload, nil)
	return err
}
