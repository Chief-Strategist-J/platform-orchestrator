/*
Package services provides domain-specific services for Traefik middleware inspection.

ALGORITHM BLUEPRINT (TraefikMiddlewareService):
1. List: Queries /api/http/middlewares or /api/tcp/middlewares.
2. Get: Retrieves specific middleware configuration details.
3. Invariants:
   - Zero inline comments inside function bodies.
*/
package services

import (
	"context"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/client"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/endpoints"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/schema"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/types"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/paths"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

type TraefikMiddlewareService struct {
	tracer         ports.TracerPort
	baseDir        string
	resolver       *paths.PathResolver
	middlewareDesc ResourceDescriptor[schema.MiddlewareDefinition]
}

func NewTraefikMiddlewareService(tracer ports.TracerPort, baseDir string) *TraefikMiddlewareService {
	return &TraefikMiddlewareService{
		tracer:   tracer,
		baseDir:  baseDir,
		resolver: paths.NewPathResolver(baseDir),
		middlewareDesc: ResourceDescriptor[schema.MiddlewareDefinition]{
			ResourceName:       "Middleware",
			CollectionEndpoint: endpoints.EndpointHTTPMiddlewares,
			ItemEndpointFunc:   endpoints.BuildHTTPMiddlewarePath,
			SpanPrefix:         "traefik.middlewares",
		},
	}
}

func (s *TraefikMiddlewareService) List(ctx context.Context, opts types.ClientOptions) ([]schema.MiddlewareDefinition, error) {
	c := client.NewTraefikClient(opts, s.resolver)
	rawMap, err := ExecuteListMap(ctx, c, s.tracer, s.middlewareDesc)
	if err != nil {
		return nil, err
	}

	var list []schema.MiddlewareDefinition
	for name, m := range rawMap {
		if m.Name == "" {
			m.Name = name
		}
		list = append(list, m)
	}
	return list, nil
}

func (s *TraefikMiddlewareService) Get(ctx context.Context, opts types.ClientOptions, name string) (*schema.MiddlewareDefinition, error) {
	c := client.NewTraefikClient(opts, s.resolver)
	return ExecuteGet(ctx, c, s.tracer, s.middlewareDesc, name)
}
