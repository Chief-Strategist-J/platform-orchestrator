/*
Package services provides domain-specific services for Traefik middleware inspection and discovery.

ALGORITHM BLUEPRINT (TraefikMiddlewareService):
1. Architecture & Operational Role:
   - Provides inspection and discovery capabilities for Traefik security, authentication, and traffic transformation middlewares.
   - Implements dual-mode resolution: live Traefik Admin API querying (/api/http/middlewares) with fallback to static dynamic configuration files (dynamic.yml).
2. Live Querying & Resilience Fallback Pipeline:
   - Queries Traefik Admin REST endpoints using generic ResourceDescriptor executors.
   - On network failure, daemon unavailability, or missing resource errors, falls back to parsing dynamic configuration from disk.
   - Annotates OpenTelemetry spans with source provenance ("live_api" vs "dynamic_config") and item counts.
3. Input Validation & Strict Null Handling:
   - Validates all input parameters (e.g. non-empty middleware names) prior to execution.
   - Guarantees non-nil slice returns (`make([]schema.MiddlewareDefinition, 0)`) for deterministic caller behavior.
   - Guards all pointer and map accesses against nil dereference (`dynConfig != nil && dynConfig.HTTP != nil && dynConfig.HTTP.Middlewares != nil`).
4. Thread-Safety & Concurrency Synchronization:
   - Protects filesystem reads using shared read locks (`s.fileMu.RLock`) to prevent data races against concurrent dynamic configuration writers.
5. Observability & Semantic Tracing:
   - Wraps every public operation in an OpenTelemetry span with standardized attribute schemas ("middleware.name", "traefik.source", "traefik.items_count").
   - Captures and records all errors on active spans via observability.RecordError before returning.
6. Invariants:
   - Zero inline comments inside function bodies.
   - All public methods validate inputs and return deterministic non-nil slices.
   - All disk reads are protected under shared read locks.
   - Distributed tracing spans and error metrics are recorded across all operations.
*/
package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/client"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/endpoints"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/schema"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/types"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/infra/observability"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/paths"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

type TraefikMiddlewareService struct {
	tracer         ports.TracerPort
	baseDir        string
	resolver       *paths.PathResolver
	fileMu         sync.RWMutex
	middlewareDesc ResourceDescriptor[schema.MiddlewareDefinition]
}

func NewTraefikMiddlewareService(tracer ports.TracerPort, baseDir string) *TraefikMiddlewareService {
	resolver := paths.NewPathResolver(baseDir)
	traefikCfg := resolver.GetTraefikConfig()

	collectionEndpoint := traefikCfg.API.Endpoints.HTTPMiddlewares
	if collectionEndpoint == "" {
		collectionEndpoint = endpoints.EndpointHTTPMiddlewares
	}

	return &TraefikMiddlewareService{
		tracer:   tracer,
		baseDir:  baseDir,
		resolver: resolver,
		middlewareDesc: ResourceDescriptor[schema.MiddlewareDefinition]{
			ResourceName:       "Middleware",
			CollectionEndpoint: collectionEndpoint,
			ItemEndpointFunc:   endpoints.BuildHTTPMiddlewarePath,
			SpanPrefix:         "traefik.middlewares",
		},
	}
}

func (s *TraefikMiddlewareService) List(ctx context.Context, opts types.ClientOptions) ([]schema.MiddlewareDefinition, error) {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "traefik.middlewares.list", map[string]interface{}{
		"traefik.resource": "middleware",
	})
	defer span.End()

	c := client.NewTraefikClient(opts, s.resolver)
	rawMap, err := ExecuteListMap(ctx, c, s.tracer, s.middlewareDesc)
	if err == nil && len(rawMap) > 0 {
		list := make([]schema.MiddlewareDefinition, 0, len(rawMap))
		for name, m := range rawMap {
			if m.Name == "" {
				m.Name = name
			}
			list = append(list, m)
		}
		observability.SetAttributes(ctx, map[string]interface{}{
			"traefik.source":      "live_api",
			"traefik.items_count": len(list),
		})
		return list, nil
	}

	dynConfig, fileErr := s.readDynamicConfig()
	if fileErr != nil {
		if err != nil {
			observability.RecordError(ctx, err)
			return make([]schema.MiddlewareDefinition, 0), err
		}
		observability.RecordError(ctx, fileErr)
		return make([]schema.MiddlewareDefinition, 0), fileErr
	}

	list := make([]schema.MiddlewareDefinition, 0)
	if dynConfig != nil && dynConfig.HTTP != nil && dynConfig.HTTP.Middlewares != nil {
		for name, m := range dynConfig.HTTP.Middlewares {
			m.Name = name
			list = append(list, m)
		}
	}

	observability.SetAttributes(ctx, map[string]interface{}{
		"traefik.source":      "dynamic_config",
		"traefik.items_count": len(list),
	})
	return list, nil
}

func (s *TraefikMiddlewareService) Get(ctx context.Context, opts types.ClientOptions, name string) (*schema.MiddlewareDefinition, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		err := errors.New("middleware name cannot be empty")
		observability.RecordError(ctx, err)
		return nil, err
	}

	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "traefik.middlewares.get", map[string]interface{}{
		"middleware.name": trimmedName,
	})
	defer span.End()

	c := client.NewTraefikClient(opts, s.resolver)
	m, err := ExecuteGet(ctx, c, s.tracer, s.middlewareDesc, trimmedName)
	if err == nil && m != nil {
		if m.Name == "" {
			m.Name = trimmedName
		}
		observability.SetAttribute(ctx, "traefik.source", "live_api")
		return m, nil
	}

	dynConfig, fileErr := s.readDynamicConfig()
	if fileErr != nil {
		observability.RecordError(ctx, err)
		return nil, err
	}

	if dynConfig != nil && dynConfig.HTTP != nil && dynConfig.HTTP.Middlewares != nil {
		if mw, ok := dynConfig.HTTP.Middlewares[trimmedName]; ok {
			mw.Name = trimmedName
			observability.SetAttribute(ctx, "traefik.source", "dynamic_config")
			return &mw, nil
		}
	}

	notFoundErr := fmt.Errorf("middleware %q not found", trimmedName)
	observability.RecordError(ctx, notFoundErr)
	return nil, notFoundErr
}

func (s *TraefikMiddlewareService) dynamicConfigPath() string {
	if cfgDir := s.resolver.ConfigDir(); cfgDir != "" {
		candidate := filepath.Join(cfgDir, "traefik", "dynamic.yml")
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate
		}
	}
	candidates := []string{
		filepath.Join(s.baseDir, "config", "traefik", "dynamic.yml"),
		filepath.Join(s.baseDir, "packages", "platform-orchestrator", "config", "traefik", "dynamic.yml"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	if cfgDir := s.resolver.ConfigDir(); cfgDir != "" {
		return filepath.Join(cfgDir, "traefik", "dynamic.yml")
	}
	return filepath.Join(s.baseDir, "config", "traefik", "dynamic.yml")
}

func (s *TraefikMiddlewareService) readDynamicConfig() (*schema.DynamicConfiguration, error) {
	s.fileMu.RLock()
	defer s.fileMu.RUnlock()

	path := s.dynamicConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg schema.DynamicConfiguration
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML from %s: %w", path, err)
	}
	return &cfg, nil
}
