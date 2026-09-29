/*
Package services provides domain-specific services for Traefik HTTP and TCP routing lifecycle management.

ALGORITHM BLUEPRINT (TraefikRouterService):
1. Live Traefik Querying: Dispatches GET requests to /api/http/routers, /api/http/services, and /api/tcp/routers.
2. Dynamic Configuration Persistence: Safely reads, validates via RuleSet, updates, and atomically writes changes to config/traefik/dynamic.yml.
3. Invariants:
   - Zero inline comments inside function bodies.
   - Non-empty validation errors abort before mutating disk files.
*/
package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/client"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/endpoints"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/rules"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/schema"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/types"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/infra/observability"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/paths"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

type TraefikRouterService struct {
	tracer        ports.TracerPort
	baseDir       string
	resolver      *paths.PathResolver
	fileMu        sync.RWMutex
	httpRouterDesc ResourceDescriptor[schema.HTTPRouterDefinition]
	tcpRouterDesc  ResourceDescriptor[schema.TCPRouterDefinition]
	serviceDesc    ResourceDescriptor[schema.ServiceDefinition]
}

func NewTraefikRouterService(tracer ports.TracerPort, baseDir string) *TraefikRouterService {
	return &TraefikRouterService{
		tracer:   tracer,
		baseDir:  baseDir,
		resolver: paths.NewPathResolver(baseDir),
		httpRouterDesc: ResourceDescriptor[schema.HTTPRouterDefinition]{
			ResourceName:       "HTTP Router",
			CollectionEndpoint: endpoints.EndpointHTTPRouters,
			ItemEndpointFunc:   endpoints.BuildHTTPRouterPath,
			SpanPrefix:         "traefik.http_routers",
		},
		tcpRouterDesc: ResourceDescriptor[schema.TCPRouterDefinition]{
			ResourceName:       "TCP Router",
			CollectionEndpoint: endpoints.EndpointTCPRouters,
			ItemEndpointFunc:   endpoints.BuildTCPRouterPath,
			SpanPrefix:         "traefik.tcp_routers",
		},
		serviceDesc: ResourceDescriptor[schema.ServiceDefinition]{
			ResourceName:       "HTTP Service",
			CollectionEndpoint: endpoints.EndpointHTTPServices,
			ItemEndpointFunc:   endpoints.BuildHTTPServicePath,
			SpanPrefix:         "traefik.http_services",
		},
	}
}

func (s *TraefikRouterService) ListHTTPRouters(ctx context.Context, opts types.ClientOptions) ([]schema.HTTPRouterDefinition, error) {
	c := client.NewTraefikClient(opts, s.resolver)
	rawMap, err := ExecuteListMap(ctx, c, s.tracer, s.httpRouterDesc)
	if err == nil && len(rawMap) > 0 {
		var list []schema.HTTPRouterDefinition
		for name, r := range rawMap {
			if r.Name == "" {
				r.Name = name
			}
			list = append(list, r)
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

	var list []schema.HTTPRouterDefinition
	if dynConfig.HTTP != nil {
		for name, r := range dynConfig.HTTP.Routers {
			r.Name = name
			list = append(list, r)
		}
	}
	return list, nil
}

func (s *TraefikRouterService) GetHTTPRouter(ctx context.Context, opts types.ClientOptions, name string) (*schema.HTTPRouterDefinition, error) {
	c := client.NewTraefikClient(opts, s.resolver)
	router, err := ExecuteGet(ctx, c, s.tracer, s.httpRouterDesc, name)
	if err == nil {
		if router.Name == "" {
			router.Name = name
		}
		return router, nil
	}

	dynConfig, fileErr := s.readDynamicConfig()
	if fileErr != nil {
		return nil, err
	}

	if dynConfig.HTTP != nil && dynConfig.HTTP.Routers != nil {
		if r, ok := dynConfig.HTTP.Routers[name]; ok {
			r.Name = name
			return &r, nil
		}
	}

	return nil, fmt.Errorf("HTTP router %q not found", name)
}

func (s *TraefikRouterService) SaveHTTPRouter(ctx context.Context, router schema.HTTPRouterDefinition) (*types.RouterOperationResult, error) {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "traefik.http_router.save", map[string]interface{}{
		"router.name": router.Name,
	})
	defer span.End()

	start := time.Now()
	normalized, err := rules.HTTPRouterRules.Execute(router)
	if err != nil {
		observability.RecordError(ctx, err)
		return nil, err
	}

	s.fileMu.Lock()
	defer s.fileMu.Unlock()

	dynConfig, err := s.readDynamicConfig()
	if err != nil {
		dynConfig = &schema.DynamicConfiguration{}
	}
	if dynConfig.HTTP == nil {
		dynConfig.HTTP = &schema.HTTPConfiguration{}
	}
	if dynConfig.HTTP.Routers == nil {
		dynConfig.HTTP.Routers = make(map[string]schema.HTTPRouterDefinition)
	}

	action := "created"
	if _, exists := dynConfig.HTTP.Routers[normalized.Name]; exists {
		action = "updated"
	}

	dynConfig.HTTP.Routers[normalized.Name] = normalized

	if err := s.writeDynamicConfig(dynConfig); err != nil {
		observability.RecordError(ctx, err)
		return nil, fmt.Errorf("failed writing dynamic config: %w", err)
	}

	latency := float64(time.Since(start).Microseconds()) / 1000.0
	observability.SetAttributes(ctx, map[string]interface{}{
		"router.action": action,
		"latency_ms":    latency,
	})

	return &types.RouterOperationResult{
		Name:      normalized.Name,
		Status:    action,
		Message:   fmt.Sprintf("HTTP router %q %s successfully in dynamic configuration", normalized.Name, action),
		Success:   true,
		LatencyMs: latency,
	}, nil
}

func (s *TraefikRouterService) DeleteHTTPRouter(ctx context.Context, name string) (*types.RouterOperationResult, error) {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "traefik.http_router.delete", map[string]interface{}{
		"router.name": name,
	})
	defer span.End()

	start := time.Now()
	s.fileMu.Lock()
	defer s.fileMu.Unlock()

	dynConfig, err := s.readDynamicConfig()
	if err != nil {
		return nil, err
	}

	if dynConfig.HTTP == nil || dynConfig.HTTP.Routers == nil {
		return nil, fmt.Errorf("HTTP router %q not found", name)
	}

	if _, ok := dynConfig.HTTP.Routers[name]; !ok {
		return nil, fmt.Errorf("HTTP router %q not found", name)
	}

	delete(dynConfig.HTTP.Routers, name)

	if err := s.writeDynamicConfig(dynConfig); err != nil {
		observability.RecordError(ctx, err)
		return nil, fmt.Errorf("failed persisting dynamic config: %w", err)
	}

	latency := float64(time.Since(start).Microseconds()) / 1000.0
	return &types.RouterOperationResult{
		Name:      name,
		Status:    "deleted",
		Message:   fmt.Sprintf("HTTP router %q removed from dynamic configuration", name),
		Success:   true,
		LatencyMs: latency,
	}, nil
}

func (s *TraefikRouterService) ListHTTPServices(ctx context.Context, opts types.ClientOptions) ([]schema.ServiceDefinition, error) {
	c := client.NewTraefikClient(opts, s.resolver)
	rawMap, err := ExecuteListMap(ctx, c, s.tracer, s.serviceDesc)
	if err == nil && len(rawMap) > 0 {
		var list []schema.ServiceDefinition
		for name, svc := range rawMap {
			if svc.Name == "" {
				svc.Name = name
			}
			list = append(list, svc)
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

	var list []schema.ServiceDefinition
	if dynConfig.HTTP != nil {
		for name, svc := range dynConfig.HTTP.Services {
			svc.Name = name
			list = append(list, svc)
		}
	}
	return list, nil
}

func (s *TraefikRouterService) ListTCPRouters(ctx context.Context, opts types.ClientOptions) ([]schema.TCPRouterDefinition, error) {
	c := client.NewTraefikClient(opts, s.resolver)
	rawMap, err := ExecuteListMap(ctx, c, s.tracer, s.tcpRouterDesc)
	if err == nil && len(rawMap) > 0 {
		var list []schema.TCPRouterDefinition
		for name, r := range rawMap {
			if r.Name == "" {
				r.Name = name
			}
			list = append(list, r)
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

	var list []schema.TCPRouterDefinition
	if dynConfig.TCP != nil {
		for name, r := range dynConfig.TCP.Routers {
			r.Name = name
			list = append(list, r)
		}
	}
	return list, nil
}

func (s *TraefikRouterService) SaveTCPRouter(ctx context.Context, router schema.TCPRouterDefinition) (*types.RouterOperationResult, error) {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "traefik.tcp_router.save", map[string]interface{}{
		"router.name": router.Name,
	})
	defer span.End()

	start := time.Now()
	normalized, err := rules.TCPRouterRules.Execute(router)
	if err != nil {
		observability.RecordError(ctx, err)
		return nil, err
	}

	s.fileMu.Lock()
	defer s.fileMu.Unlock()

	dynConfig, err := s.readDynamicConfig()
	if err != nil {
		dynConfig = &schema.DynamicConfiguration{}
	}
	if dynConfig.TCP == nil {
		dynConfig.TCP = &schema.TCPConfiguration{}
	}
	if dynConfig.TCP.Routers == nil {
		dynConfig.TCP.Routers = make(map[string]schema.TCPRouterDefinition)
	}

	action := "created"
	if _, exists := dynConfig.TCP.Routers[normalized.Name]; exists {
		action = "updated"
	}

	dynConfig.TCP.Routers[normalized.Name] = normalized

	if err := s.writeDynamicConfig(dynConfig); err != nil {
		observability.RecordError(ctx, err)
		return nil, fmt.Errorf("failed writing dynamic config: %w", err)
	}

	latency := float64(time.Since(start).Microseconds()) / 1000.0
	return &types.RouterOperationResult{
		Name:      normalized.Name,
		Status:    action,
		Message:   fmt.Sprintf("TCP router %q %s successfully in dynamic configuration", normalized.Name, action),
		Success:   true,
		LatencyMs: latency,
	}, nil
}

func (s *TraefikRouterService) DeleteTCPRouter(ctx context.Context, name string) (*types.RouterOperationResult, error) {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "traefik.tcp_router.delete", map[string]interface{}{
		"router.name": name,
	})
	defer span.End()

	start := time.Now()
	s.fileMu.Lock()
	defer s.fileMu.Unlock()

	dynConfig, err := s.readDynamicConfig()
	if err != nil {
		return nil, err
	}

	if dynConfig.TCP == nil || dynConfig.TCP.Routers == nil {
		return nil, fmt.Errorf("TCP router %q not found", name)
	}

	if _, ok := dynConfig.TCP.Routers[name]; !ok {
		return nil, fmt.Errorf("TCP router %q not found", name)
	}

	delete(dynConfig.TCP.Routers, name)

	if err := s.writeDynamicConfig(dynConfig); err != nil {
		observability.RecordError(ctx, err)
		return nil, fmt.Errorf("failed persisting dynamic config: %w", err)
	}

	latency := float64(time.Since(start).Microseconds()) / 1000.0
	return &types.RouterOperationResult{
		Name:      name,
		Status:    "deleted",
		Message:   fmt.Sprintf("TCP router %q removed from dynamic configuration", name),
		Success:   true,
		LatencyMs: latency,
	}, nil
}

func (s *TraefikRouterService) dynamicConfigPath() string {
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
	return filepath.Join(s.baseDir, "config", "traefik", "dynamic.yml")
}

func (s *TraefikRouterService) readDynamicConfig() (*schema.DynamicConfiguration, error) {
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

func (s *TraefikRouterService) writeDynamicConfig(cfg *schema.DynamicConfiguration) error {
	path := s.dynamicConfigPath()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}

	tmpPath := fmt.Sprintf("%s.tmp", path)
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
