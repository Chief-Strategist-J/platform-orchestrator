/*
Package services provides domain-specific services for Traefik middleware inspection.

ALGORITHM BLUEPRINT (TraefikMiddlewareService):
1. Live Querying: Attempts live Traefik API query to /api/http/middlewares.
2. Dynamic Configuration Fallback: Parses config/traefik/dynamic.yml when the live server is unreachable.
3. Invariants:
   - Zero inline comments inside function bodies.
*/
package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

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
	if err == nil && len(rawMap) > 0 {
		var list []schema.MiddlewareDefinition
		for name, m := range rawMap {
			if m.Name == "" {
				m.Name = name
			}
			list = append(list, m)
		}
		return list, nil
	}

	dynConfig, fileErr := s.readDynamicConfig()
	if fileErr != nil {
		if err != nil {
			return nil, err
		}
		return nil, fileErr
	}

	var list []schema.MiddlewareDefinition
	if dynConfig.HTTP != nil {
		for name, m := range dynConfig.HTTP.Middlewares {
			m.Name = name
			list = append(list, m)
		}
	}
	return list, nil
}

func (s *TraefikMiddlewareService) Get(ctx context.Context, opts types.ClientOptions, name string) (*schema.MiddlewareDefinition, error) {
	c := client.NewTraefikClient(opts, s.resolver)
	m, err := ExecuteGet(ctx, c, s.tracer, s.middlewareDesc, name)
	if err == nil {
		if m.Name == "" {
			m.Name = name
		}
		return m, nil
	}

	dynConfig, fileErr := s.readDynamicConfig()
	if fileErr != nil {
		return nil, err
	}

	if dynConfig.HTTP != nil && dynConfig.HTTP.Middlewares != nil {
		if mw, ok := dynConfig.HTTP.Middlewares[name]; ok {
			mw.Name = name
			return &mw, nil
		}
	}

	return nil, fmt.Errorf("middleware %q not found", name)
}

func (s *TraefikMiddlewareService) readDynamicConfig() (*schema.DynamicConfiguration, error) {
	candidates := []string{
		filepath.Join(s.baseDir, "config", "traefik", "dynamic.yml"),
		filepath.Join(s.baseDir, "packages", "platform-orchestrator", "config", "traefik", "dynamic.yml"),
	}
	if cfgDir := s.resolver.ConfigDir(); cfgDir != "" {
		candidates = append([]string{filepath.Join(cfgDir, "traefik", "dynamic.yml")}, candidates...)
	}

	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err == nil {
			var cfg schema.DynamicConfiguration
			if err := yaml.Unmarshal(data, &cfg); err == nil {
				return &cfg, nil
			}
		}
	}
	return nil, fmt.Errorf("could not find or parse dynamic.yml")
}
