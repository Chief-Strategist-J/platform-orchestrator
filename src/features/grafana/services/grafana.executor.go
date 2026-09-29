/*
Package services provides a generic, data-driven REST resource executor for Grafana API operations.

ALGORITHM BLUEPRINT (ResourceExecutor):
1. Config-Driven Abstraction: ResourceDescriptor defines endpoints, OTel span prefixes, and validation rules as pure data.
2. Unified Operation Pipelines:
   - ExecuteList: Dispatches GET to collection endpoint and unmarshals []T.
   - ExecuteGet: Dispatches GET to item endpoint and unmarshals *T.
   - ExecuteDelete: Dispatches DELETE, measures round-trip latency, and returns standardized OperationResult.
   - ExecuteUpsert: Normalizes payload, checks existing resource, routes to PUT or POST, and formats OperationResult.
3. Invariants:
   - Zero inline comments inside function bodies.
   - Eliminates redundant error-checking, latency calculation, and HTTP request boilerplate across services.
*/
package services

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/client"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/rules"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/types"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

type ResourceDescriptor[T any] struct {
	ResourceName       string
	CollectionEndpoint string
	ItemEndpointFunc   func(id string) string
	SpanPrefix         string
	RuleSet            rules.RuleSet[T]
}

type GenericOperationResult struct {
	ID        string  `json:"id,omitempty"`
	UID       string  `json:"uid,omitempty"`
	Name      string  `json:"name,omitempty"`
	Status    string  `json:"status"`
	Message   string  `json:"message"`
	Success   bool    `json:"success"`
	LatencyMs float64 `json:"latencyMs"`
}

func ExecuteList[T any](ctx context.Context, c *client.GrafanaClient, tracer ports.TracerPort, desc ResourceDescriptor[T]) ([]T, error) {
	ctx, endSpan := tracer.StartSpan(ctx, fmt.Sprintf("%s.list", desc.SpanPrefix))
	defer endSpan()

	var items []T
	_, err := c.Do(ctx, http.MethodGet, desc.CollectionEndpoint, nil, &items)
	if err != nil {
		return nil, err
	}
	return items, nil
}

func ExecuteGet[T any](ctx context.Context, c *client.GrafanaClient, tracer ports.TracerPort, desc ResourceDescriptor[T], id string) (*T, error) {
	ctx, endSpan := tracer.StartSpan(ctx, fmt.Sprintf("%s.get", desc.SpanPrefix))
	defer endSpan()

	targetPath := desc.ItemEndpointFunc(id)
	var item T
	status, err := c.Do(ctx, http.MethodGet, targetPath, nil, &item)
	if err != nil || status != http.StatusOK {
		return nil, fmt.Errorf("%s %q not found: %w", desc.ResourceName, id, err)
	}
	return &item, nil
}

func ExecuteDelete[T any](ctx context.Context, c *client.GrafanaClient, tracer ports.TracerPort, desc ResourceDescriptor[T], id string) (*types.AlertOperationResult, error) {
	ctx, endSpan := tracer.StartSpan(ctx, fmt.Sprintf("%s.delete", desc.SpanPrefix))
	defer endSpan()

	start := time.Now()
	targetPath := desc.ItemEndpointFunc(id)

	var deleteResponse struct {
		Message string `json:"message"`
		Title   string `json:"title"`
	}

	_, err := c.Do(ctx, http.MethodDelete, targetPath, nil, &deleteResponse)
	latency := float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil {
		return &types.AlertOperationResult{
			UID:       id,
			Message:   fmt.Sprintf("Failed to delete %s: %v", desc.ResourceName, err),
			Status:    "failed",
			Success:   false,
			LatencyMs: latency,
		}, err
	}

	msg := deleteResponse.Message
	if msg == "" {
		msg = fmt.Sprintf("%s deleted successfully", desc.ResourceName)
	}

	return &types.AlertOperationResult{
		UID:       id,
		Message:   msg,
		Status:    "deleted",
		Success:   true,
		LatencyMs: latency,
	}, nil
}

func ExecuteUpsert[T any](ctx context.Context, c *client.GrafanaClient, tracer ports.TracerPort, desc ResourceDescriptor[T], id string, payload T) (*types.AlertOperationResult, error) {
	ctx, endSpan := tracer.StartSpan(ctx, fmt.Sprintf("%s.save", desc.SpanPrefix))
	defer endSpan()

	normalized, err := desc.RuleSet.Execute(payload)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	targetPath := desc.ItemEndpointFunc(id)

	var (
		action string
		method string
		path   string
	)

	if id != "" {
		var existing T
		status, getErr := c.Do(ctx, http.MethodGet, targetPath, nil, &existing)
		if getErr == nil && status == http.StatusOK {
			action = "updated"
			method = http.MethodPut
			path = targetPath
		} else {
			action = "created"
			method = http.MethodPost
			path = desc.CollectionEndpoint
		}
	} else {
		action = "created"
		method = http.MethodPost
		path = desc.CollectionEndpoint
	}

	var opResp struct {
		ID      int64  `json:"id"`
		UID     string `json:"uid"`
		Title   string `json:"title"`
		Name    string `json:"name"`
		Message string `json:"message"`
	}

	_, opErr := c.Do(ctx, method, path, normalized, &opResp)
	latency := float64(time.Since(start).Microseconds()) / 1000.0

	if opErr != nil {
		return &types.AlertOperationResult{
			UID:       id,
			Message:   fmt.Sprintf("Failed to %s %s: %v", action, desc.ResourceName, opErr),
			Status:    "failed",
			Success:   false,
			LatencyMs: latency,
		}, opErr
	}

	resUID := opResp.UID
	if resUID == "" {
		resUID = id
	}

	title := opResp.Title
	if title == "" {
		title = opResp.Name
	}

	msg := opResp.Message
	if msg == "" {
		msg = fmt.Sprintf("%s %s successfully", desc.ResourceName, action)
	}

	return &types.AlertOperationResult{
		UID:       resUID,
		Title:     title,
		Message:   msg,
		Status:    action,
		Success:   true,
		LatencyMs: latency,
	}, nil
}
