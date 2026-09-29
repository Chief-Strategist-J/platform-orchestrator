/*
Package services provides a generic REST resource executor for Traefik API operations with distributed tracing.

ALGORITHM BLUEPRINT (ResourceExecutor):
1. Config-Driven Abstraction: ResourceDescriptor defines endpoints and OTel span prefixes.
2. Unified Operation Pipelines:
   - ExecuteListMap: Dispatches GET and unmarshals map[string]T into a slice of T.
   - ExecuteGet: Dispatches GET to an item endpoint and unmarshals *T.
3. Invariants:
   - Zero inline comments inside function bodies.
   - Distributed tracing is preserved across every call.
*/
package services

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/client"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/infra/observability"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

type ResourceDescriptor[T any] struct {
	ResourceName       string
	CollectionEndpoint string
	ItemEndpointFunc   func(id string) string
	SpanPrefix         string
}

func ExecuteListMap[T any](ctx context.Context, c *client.TraefikClient, tracer ports.TracerPort, desc ResourceDescriptor[T]) (map[string]T, error) {
	ctx, span := tracer.StartSpanWithAttributes(ctx, fmt.Sprintf("%s.list", desc.SpanPrefix), map[string]interface{}{
		"traefik.resource_name": desc.ResourceName,
		"traefik.endpoint":      desc.CollectionEndpoint,
	})
	defer span.End()

	var rawMap map[string]T
	_, err := c.Do(ctx, http.MethodGet, desc.CollectionEndpoint, nil, &rawMap)
	if err != nil {
		observability.RecordError(ctx, err)
		return nil, err
	}

	observability.SetAttribute(ctx, "traefik.items_count", len(rawMap))
	return rawMap, nil
}

func ExecuteGet[T any](ctx context.Context, c *client.TraefikClient, tracer ports.TracerPort, desc ResourceDescriptor[T], id string) (*T, error) {
	targetPath := desc.ItemEndpointFunc(id)
	ctx, span := tracer.StartSpanWithAttributes(ctx, fmt.Sprintf("%s.get", desc.SpanPrefix), map[string]interface{}{
		"traefik.resource_name": desc.ResourceName,
		"traefik.resource_id":   id,
		"traefik.endpoint":      targetPath,
	})
	defer span.End()

	var item T
	status, err := c.Do(ctx, http.MethodGet, targetPath, nil, &item)
	if err != nil || status != http.StatusOK {
		notFoundErr := fmt.Errorf("%s %q not found in Traefik: %w", desc.ResourceName, id, err)
		observability.RecordError(ctx, notFoundErr)
		return nil, notFoundErr
	}

	return &item, nil
}
