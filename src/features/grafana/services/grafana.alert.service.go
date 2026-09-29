/*
Package services provides isolated, domain-specific services for Grafana lifecycle management with deep OpenTelemetry tracing.

ALGORITHM BLUEPRINT (AlertService):
1. Declarative Resource Descriptors: Configures AlertRule and ContactPoint endpoints, OTel span prefixes, and rules as data descriptors.
2. Generic Execution: Delegates List, Get, Delete, and Upsert operations to ResourceExecutor with distributed trace context.
3. Notification Testing: Dispatches synthetic alert notifications to verify contact point connectivity, tracking latency and delivery spans.
4. Invariants:
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
	"github.com/Chief-Strategist-J/platform-orchestrator/src/infra/observability"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/paths"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

type AlertService struct {
	tracer           ports.TracerPort
	baseDir          string
	resolver         *paths.PathResolver
	alertRuleDesc    ResourceDescriptor[schema.AlertRulePayload]
	contactPointDesc ResourceDescriptor[schema.ContactPointPayload]
}

func NewAlertService(tracer ports.TracerPort, baseDir string) *AlertService {
	return &AlertService{
		tracer:   tracer,
		baseDir:  baseDir,
		resolver: paths.NewPathResolver(baseDir),
		alertRuleDesc: ResourceDescriptor[schema.AlertRulePayload]{
			ResourceName:       "Alert rule",
			CollectionEndpoint: endpoints.EndpointAlertRules,
			ItemEndpointFunc:   endpoints.BuildAlertRuleUIDPath,
			SpanPrefix:         "grafana.alerts",
			RuleSet:            rules.AlertRuleRules,
		},
		contactPointDesc: ResourceDescriptor[schema.ContactPointPayload]{
			ResourceName:       "Contact point",
			CollectionEndpoint: endpoints.EndpointContactPoints,
			ItemEndpointFunc:   endpoints.BuildContactPointUIDPath,
			SpanPrefix:         "grafana.contact_points",
			RuleSet:            rules.ContactPointRules,
		},
	}
}

func (s *AlertService) ListAlertRules(ctx context.Context, opts types.ClientOptions) ([]schema.AlertRulePayload, error) {
	c := client.NewGrafanaClient(opts, s.resolver)
	return ExecuteList(ctx, c, s.tracer, s.alertRuleDesc)
}

func (s *AlertService) GetAlertRule(ctx context.Context, opts types.ClientOptions, uid string) (*schema.AlertRulePayload, error) {
	c := client.NewGrafanaClient(opts, s.resolver)
	return ExecuteGet(ctx, c, s.tracer, s.alertRuleDesc, uid)
}

func (s *AlertService) CreateOrUpdateAlertRule(ctx context.Context, opts types.ClientOptions, rule schema.AlertRulePayload) (*types.AlertOperationResult, error) {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "grafana.alerts.save", map[string]interface{}{
		"alert.title":      rule.Title,
		"alert.rule_group": rule.RuleGroup,
		"alert.folder_uid": rule.FolderUID,
	})
	defer span.End()

	if rule.FolderUID != "" {
		_ = s.EnsureFolderExists(ctx, opts, rule.FolderUID)
	}
	c := client.NewGrafanaClient(opts, s.resolver)
	return ExecuteUpsert(ctx, c, s.tracer, s.alertRuleDesc, rule.UID, rule)
}

func (s *AlertService) DeleteAlertRule(ctx context.Context, opts types.ClientOptions, uid string) (*types.AlertOperationResult, error) {
	c := client.NewGrafanaClient(opts, s.resolver)
	return ExecuteDelete(ctx, c, s.tracer, s.alertRuleDesc, uid)
}

func (s *AlertService) ListContactPoints(ctx context.Context, opts types.ClientOptions) ([]schema.ContactPointPayload, error) {
	c := client.NewGrafanaClient(opts, s.resolver)
	return ExecuteList(ctx, c, s.tracer, s.contactPointDesc)
}

func (s *AlertService) CreateOrUpdateContactPoint(ctx context.Context, opts types.ClientOptions, cp schema.ContactPointPayload) (*types.AlertOperationResult, error) {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "grafana.contact_points.save", map[string]interface{}{
		"contact_point.name": cp.Name,
		"contact_point.type": cp.Type,
	})
	defer span.End()

	c := client.NewGrafanaClient(opts, s.resolver)

	if cp.UID == "" {
		var existingCPs []schema.ContactPointPayload
		_, _ = c.Do(ctx, http.MethodGet, endpoints.EndpointContactPoints, nil, &existingCPs)
		for _, existing := range existingCPs {
			if existing.Name == cp.Name {
				cp.UID = existing.UID
				break
			}
		}
	}

	return ExecuteUpsert(ctx, c, s.tracer, s.contactPointDesc, cp.UID, cp)
}

func (s *AlertService) DeleteContactPoint(ctx context.Context, opts types.ClientOptions, uid string) (*types.AlertOperationResult, error) {
	c := client.NewGrafanaClient(opts, s.resolver)
	return ExecuteDelete(ctx, c, s.tracer, s.contactPointDesc, uid)
}

func (s *AlertService) TestContactPoint(ctx context.Context, opts types.ClientOptions, cp schema.ContactPointPayload) (*types.AlertOperationResult, error) {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "grafana.contact_points.test", map[string]interface{}{
		"contact_point.name": cp.Name,
		"contact_point.type": cp.Type,
	})
	defer span.End()

	normalized := rules.ContactPointRules.Normalize(cp)
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
				"name":     normalized.Name,
				"type":     normalized.Type,
				"settings": normalized.Settings,
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
		observability.RecordError(ctx, err)
		return &types.AlertOperationResult{
			Title:     normalized.Name,
			Status:    "failed",
			Message:   fmt.Sprintf("Contact point test failed: %v", err),
			Success:   false,
			LatencyMs: latency,
		}, err
	}

	observability.SetAttributes(ctx, map[string]interface{}{
		"test.status":     "success",
		"test.latency_ms": latency,
	})
	observability.AddEvent(ctx, "grafana.contact_point_tested", map[string]interface{}{
		"name": normalized.Name,
	})

	return &types.AlertOperationResult{
		Title:     normalized.Name,
		Status:    "success",
		Message:   "Test notification successfully dispatched and accepted",
		Success:   true,
		LatencyMs: latency,
	}, nil
}

func (s *AlertService) EnsureFolderExists(ctx context.Context, opts types.ClientOptions, folderUID string) error {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "grafana.folders.ensure", map[string]interface{}{
		"folder.uid": folderUID,
	})
	defer span.End()

	c := client.NewGrafanaClient(opts, s.resolver)
	targetPath := fmt.Sprintf("%s/%s", endpoints.EndpointFolders, folderUID)

	var existingFolder struct {
		ID int64 `json:"id"`
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
	if err != nil {
		observability.RecordError(ctx, err)
	}
	return err
}
