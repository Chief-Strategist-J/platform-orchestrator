/*
Package services provides domain-specific services for Traefik HTTP and TCP routing lifecycle management.

ALGORITHM BLUEPRINT (TraefikRouterService):
1. Architecture & Operational Role:
   - Manages complete lifecycle (query, inspection, registration, mutation, deletion) of Traefik HTTP routers, HTTP services, TCP routers, and TCP services.
   - Implements dual-mode resolution: live Traefik Admin API inspection with fallback to static dynamic configuration files (dynamic.yml).
2. Live Querying & Resilience Fallback Pipeline:
   - Queries Traefik Admin REST endpoints using generic ResourceDescriptor executors.
   - On network timeout, unreachable daemon, or 404 response, seamlessly falls back to reading dynamic YAML configuration from disk.
   - Annotates OpenTelemetry spans with source provenance ("live_api" vs "dynamic_config") and result counts.
3. Declarative Rule Engine Execution:
   - All mutations (SaveHTTPRouter, SaveTCPRouter) are validated and normalized through declarative RuleSets (HTTPRouterRules, TCPRouterRules) before any filesystem mutation.
   - Rejects invalid payloads immediately with structured validation errors, preventing corrupted configuration files.
4. Safe Atomic Configuration Persistence:
   - Reads existing dynamic configuration under thread-safe synchronization.
   - Ensures target parent directories exist with standard permissions (0755).
   - Serializes sanitized configuration to YAML, writes to an isolated temporary file (.tmp), and commits via atomic file rename (os.Rename).
5. Thread-Safety & Concurrency Synchronization:
   - Synchronizes concurrent filesystem reads using shared read locks (s.fileMu.RLock).
   - Protects filesystem modifications using exclusive write locks (s.fileMu.Lock).
   - Separates unlocked public facades from internal locked helpers to prevent self-deadlocks.
6. Observability & Semantic Tracing:
   - Wraps every public operation in an OpenTelemetry span with standardized attribute schemas ("router.name", "router.action", "latency_ms", "traefik.source").
   - Captures and records all errors on active spans via observability.RecordError before returning.
7. Invariants:
   - Zero inline comments inside function bodies.
   - Non-empty validation errors abort execution before mutating disk files.
   - All disk mutations are executed under exclusive write locks with atomic rename.
   - All disk reads are protected under shared read locks.
   - Distributed tracing spans and error metrics are recorded across all operations.
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
	tracer         ports.TracerPort
	baseDir        string
	resolver       *paths.PathResolver
	fileMu         sync.RWMutex
	httpRouterDesc ResourceDescriptor[schema.HTTPRouterDefinition]
	tcpRouterDesc  ResourceDescriptor[schema.TCPRouterDefinition]
	serviceDesc    ResourceDescriptor[schema.ServiceDefinition]
	tcpServiceDesc ResourceDescriptor[schema.ServiceDefinition]
}

func NewTraefikRouterService(tracer ports.TracerPort, baseDir string) *TraefikRouterService {
	resolver := paths.NewPathResolver(baseDir)
	traefikCfg := resolver.GetTraefikConfig()

	httpRoutersEndpoint := traefikCfg.API.Endpoints.HTTPRouters
	if httpRoutersEndpoint == "" {
		httpRoutersEndpoint = endpoints.EndpointHTTPRouters
	}

	tcpRoutersEndpoint := traefikCfg.API.Endpoints.TCPRouters
	if tcpRoutersEndpoint == "" {
		tcpRoutersEndpoint = endpoints.EndpointTCPRouters
	}

	httpServicesEndpoint := traefikCfg.API.Endpoints.HTTPServices
	if httpServicesEndpoint == "" {
		httpServicesEndpoint = endpoints.EndpointHTTPServices
	}

	tcpServicesEndpoint := traefikCfg.API.Endpoints.TCPServices
	if tcpServicesEndpoint == "" {
		tcpServicesEndpoint = endpoints.EndpointTCPServices
	}

	return &TraefikRouterService{
		tracer:   tracer,
		baseDir:  baseDir,
		resolver: resolver,
		httpRouterDesc: ResourceDescriptor[schema.HTTPRouterDefinition]{
			ResourceName:       "HTTP Router",
			CollectionEndpoint: httpRoutersEndpoint,
			ItemEndpointFunc:   endpoints.BuildHTTPRouterPath,
			SpanPrefix:         "traefik.http_routers",
		},
		tcpRouterDesc: ResourceDescriptor[schema.TCPRouterDefinition]{
			ResourceName:       "TCP Router",
			CollectionEndpoint: tcpRoutersEndpoint,
			ItemEndpointFunc:   endpoints.BuildTCPRouterPath,
			SpanPrefix:         "traefik.tcp_routers",
		},
		serviceDesc: ResourceDescriptor[schema.ServiceDefinition]{
			ResourceName:       "HTTP Service",
			CollectionEndpoint: httpServicesEndpoint,
			ItemEndpointFunc:   endpoints.BuildHTTPServicePath,
			SpanPrefix:         "traefik.http_services",
		},
		tcpServiceDesc: ResourceDescriptor[schema.ServiceDefinition]{
			ResourceName:       "TCP Service",
			CollectionEndpoint: tcpServicesEndpoint,
			ItemEndpointFunc:   endpoints.BuildTCPServicePath,
			SpanPrefix:         "traefik.tcp_services",
		},
	}
}

func (s *TraefikRouterService) ListHTTPRouters(ctx context.Context, opts types.ClientOptions) ([]schema.HTTPRouterDefinition, error) {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "traefik.http_routers.list", map[string]interface{}{
		"traefik.resource": "http_router",
	})
	defer span.End()

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
			return nil, err
		}
		observability.RecordError(ctx, fileErr)
		return nil, fileErr
	}

	var list []schema.HTTPRouterDefinition
	if dynConfig.HTTP != nil {
		for name, r := range dynConfig.HTTP.Routers {
			r.Name = name
			list = append(list, r)
		}
	}

	observability.SetAttributes(ctx, map[string]interface{}{
		"traefik.source":      "dynamic_config",
		"traefik.items_count": len(list),
	})
	return list, nil
}

func (s *TraefikRouterService) GetHTTPRouter(ctx context.Context, opts types.ClientOptions, name string) (*schema.HTTPRouterDefinition, error) {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "traefik.http_routers.get", map[string]interface{}{
		"router.name": name,
	})
	defer span.End()

	c := client.NewTraefikClient(opts, s.resolver)
	router, err := ExecuteGet(ctx, c, s.tracer, s.httpRouterDesc, name)
	if err == nil {
		if router.Name == "" {
			router.Name = name
		}
		observability.SetAttribute(ctx, "traefik.source", "live_api")
		return router, nil
	}

	dynConfig, fileErr := s.readDynamicConfig()
	if fileErr != nil {
		observability.RecordError(ctx, err)
		return nil, err
	}

	if dynConfig.HTTP != nil && dynConfig.HTTP.Routers != nil {
		if r, ok := dynConfig.HTTP.Routers[name]; ok {
			r.Name = name
			observability.SetAttribute(ctx, "traefik.source", "dynamic_config")
			return &r, nil
		}
	}

	notFoundErr := fmt.Errorf("HTTP router %q not found", name)
	observability.RecordError(ctx, notFoundErr)
	return nil, notFoundErr
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

	dynConfig, err := s.readDynamicConfigFileLocked()
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

	if err := s.writeDynamicConfigFileLocked(dynConfig); err != nil {
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

	dynConfig, err := s.readDynamicConfigFileLocked()
	if err != nil {
		observability.RecordError(ctx, err)
		return nil, err
	}

	if dynConfig.HTTP == nil || dynConfig.HTTP.Routers == nil {
		notFoundErr := fmt.Errorf("HTTP router %q not found", name)
		observability.RecordError(ctx, notFoundErr)
		return nil, notFoundErr
	}

	if _, ok := dynConfig.HTTP.Routers[name]; !ok {
		notFoundErr := fmt.Errorf("HTTP router %q not found", name)
		observability.RecordError(ctx, notFoundErr)
		return nil, notFoundErr
	}

	delete(dynConfig.HTTP.Routers, name)

	if err := s.writeDynamicConfigFileLocked(dynConfig); err != nil {
		observability.RecordError(ctx, err)
		return nil, fmt.Errorf("failed persisting dynamic config: %w", err)
	}

	latency := float64(time.Since(start).Microseconds()) / 1000.0
	observability.SetAttributes(ctx, map[string]interface{}{
		"router.action": "deleted",
		"latency_ms":    latency,
	})

	return &types.RouterOperationResult{
		Name:      name,
		Status:    "deleted",
		Message:   fmt.Sprintf("HTTP router %q removed from dynamic configuration", name),
		Success:   true,
		LatencyMs: latency,
	}, nil
}

func (s *TraefikRouterService) ListHTTPServices(ctx context.Context, opts types.ClientOptions) ([]schema.ServiceDefinition, error) {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "traefik.http_services.list", map[string]interface{}{
		"traefik.resource": "http_service",
	})
	defer span.End()

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
			return nil, err
		}
		observability.RecordError(ctx, fileErr)
		return nil, fileErr
	}

	var list []schema.ServiceDefinition
	if dynConfig.HTTP != nil {
		for name, svc := range dynConfig.HTTP.Services {
			svc.Name = name
			list = append(list, svc)
		}
	}

	observability.SetAttributes(ctx, map[string]interface{}{
		"traefik.source":      "dynamic_config",
		"traefik.items_count": len(list),
	})
	return list, nil
}

func (s *TraefikRouterService) GetHTTPService(ctx context.Context, opts types.ClientOptions, name string) (*schema.ServiceDefinition, error) {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "traefik.http_services.get", map[string]interface{}{
		"service.name": name,
	})
	defer span.End()

	c := client.NewTraefikClient(opts, s.resolver)
	svc, err := ExecuteGet(ctx, c, s.tracer, s.serviceDesc, name)
	if err == nil {
		if svc.Name == "" {
			svc.Name = name
		}
		observability.SetAttribute(ctx, "traefik.source", "live_api")
		return svc, nil
	}

	dynConfig, fileErr := s.readDynamicConfig()
	if fileErr != nil {
		observability.RecordError(ctx, err)
		return nil, err
	}

	if dynConfig.HTTP != nil && dynConfig.HTTP.Services != nil {
		if targetSvc, ok := dynConfig.HTTP.Services[name]; ok {
			targetSvc.Name = name
			observability.SetAttribute(ctx, "traefik.source", "dynamic_config")
			return &targetSvc, nil
		}
	}

	notFoundErr := fmt.Errorf("HTTP service %q not found", name)
	observability.RecordError(ctx, notFoundErr)
	return nil, notFoundErr
}

func (s *TraefikRouterService) ListTCPRouters(ctx context.Context, opts types.ClientOptions) ([]schema.TCPRouterDefinition, error) {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "traefik.tcp_routers.list", map[string]interface{}{
		"traefik.resource": "tcp_router",
	})
	defer span.End()

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
			return nil, err
		}
		observability.RecordError(ctx, fileErr)
		return nil, fileErr
	}

	var list []schema.TCPRouterDefinition
	if dynConfig.TCP != nil {
		for name, r := range dynConfig.TCP.Routers {
			r.Name = name
			list = append(list, r)
		}
	}

	observability.SetAttributes(ctx, map[string]interface{}{
		"traefik.source":      "dynamic_config",
		"traefik.items_count": len(list),
	})
	return list, nil
}

func (s *TraefikRouterService) GetTCPRouter(ctx context.Context, opts types.ClientOptions, name string) (*schema.TCPRouterDefinition, error) {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "traefik.tcp_routers.get", map[string]interface{}{
		"router.name": name,
	})
	defer span.End()

	c := client.NewTraefikClient(opts, s.resolver)
	router, err := ExecuteGet(ctx, c, s.tracer, s.tcpRouterDesc, name)
	if err == nil {
		if router.Name == "" {
			router.Name = name
		}
		observability.SetAttribute(ctx, "traefik.source", "live_api")
		return router, nil
	}

	dynConfig, fileErr := s.readDynamicConfig()
	if fileErr != nil {
		observability.RecordError(ctx, err)
		return nil, err
	}

	if dynConfig.TCP != nil && dynConfig.TCP.Routers != nil {
		if r, ok := dynConfig.TCP.Routers[name]; ok {
			r.Name = name
			observability.SetAttribute(ctx, "traefik.source", "dynamic_config")
			return &r, nil
		}
	}

	notFoundErr := fmt.Errorf("TCP router %q not found", name)
	observability.RecordError(ctx, notFoundErr)
	return nil, notFoundErr
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

	dynConfig, err := s.readDynamicConfigFileLocked()
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

	if err := s.writeDynamicConfigFileLocked(dynConfig); err != nil {
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

	dynConfig, err := s.readDynamicConfigFileLocked()
	if err != nil {
		observability.RecordError(ctx, err)
		return nil, err
	}

	if dynConfig.TCP == nil || dynConfig.TCP.Routers == nil {
		notFoundErr := fmt.Errorf("TCP router %q not found", name)
		observability.RecordError(ctx, notFoundErr)
		return nil, notFoundErr
	}

	if _, ok := dynConfig.TCP.Routers[name]; !ok {
		notFoundErr := fmt.Errorf("TCP router %q not found", name)
		observability.RecordError(ctx, notFoundErr)
		return nil, notFoundErr
	}

	delete(dynConfig.TCP.Routers, name)

	if err := s.writeDynamicConfigFileLocked(dynConfig); err != nil {
		observability.RecordError(ctx, err)
		return nil, fmt.Errorf("failed persisting dynamic config: %w", err)
	}

	latency := float64(time.Since(start).Microseconds()) / 1000.0
	observability.SetAttributes(ctx, map[string]interface{}{
		"router.action": "deleted",
		"latency_ms":    latency,
	})

	return &types.RouterOperationResult{
		Name:      name,
		Status:    "deleted",
		Message:   fmt.Sprintf("TCP router %q removed from dynamic configuration", name),
		Success:   true,
		LatencyMs: latency,
	}, nil
}

func (s *TraefikRouterService) ListTCPServices(ctx context.Context, opts types.ClientOptions) ([]schema.ServiceDefinition, error) {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "traefik.tcp_services.list", map[string]interface{}{
		"traefik.resource": "tcp_service",
	})
	defer span.End()

	c := client.NewTraefikClient(opts, s.resolver)
	rawMap, err := ExecuteListMap(ctx, c, s.tracer, s.tcpServiceDesc)
	if err == nil && len(rawMap) > 0 {
		var list []schema.ServiceDefinition
		for name, svc := range rawMap {
			if svc.Name == "" {
				svc.Name = name
			}
			list = append(list, svc)
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
			return nil, err
		}
		observability.RecordError(ctx, fileErr)
		return nil, fileErr
	}

	var list []schema.ServiceDefinition
	if dynConfig.TCP != nil {
		for name, svc := range dynConfig.TCP.Services {
			svc.Name = name
			list = append(list, svc)
		}
	}

	observability.SetAttributes(ctx, map[string]interface{}{
		"traefik.source":      "dynamic_config",
		"traefik.items_count": len(list),
	})
	return list, nil
}

func (s *TraefikRouterService) GetTCPService(ctx context.Context, opts types.ClientOptions, name string) (*schema.ServiceDefinition, error) {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "traefik.tcp_services.get", map[string]interface{}{
		"service.name": name,
	})
	defer span.End()

	c := client.NewTraefikClient(opts, s.resolver)
	svc, err := ExecuteGet(ctx, c, s.tracer, s.tcpServiceDesc, name)
	if err == nil {
		if svc.Name == "" {
			svc.Name = name
		}
		observability.SetAttribute(ctx, "traefik.source", "live_api")
		return svc, nil
	}

	dynConfig, fileErr := s.readDynamicConfig()
	if fileErr != nil {
		observability.RecordError(ctx, err)
		return nil, err
	}

	if dynConfig.TCP != nil && dynConfig.TCP.Services != nil {
		if targetSvc, ok := dynConfig.TCP.Services[name]; ok {
			targetSvc.Name = name
			observability.SetAttribute(ctx, "traefik.source", "dynamic_config")
			return &targetSvc, nil
		}
	}

	notFoundErr := fmt.Errorf("TCP service %q not found", name)
	observability.RecordError(ctx, notFoundErr)
	return nil, notFoundErr
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
	if cfgDir := s.resolver.ConfigDir(); cfgDir != "" {
		return filepath.Join(cfgDir, "traefik", "dynamic.yml")
	}
	return filepath.Join(s.baseDir, "config", "traefik", "dynamic.yml")
}

func (s *TraefikRouterService) readDynamicConfig() (*schema.DynamicConfiguration, error) {
	s.fileMu.RLock()
	defer s.fileMu.RUnlock()
	return s.readDynamicConfigFileLocked()
}

func (s *TraefikRouterService) readDynamicConfigFileLocked() (*schema.DynamicConfiguration, error) {
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

func (s *TraefikRouterService) writeDynamicConfigFileLocked(cfg *schema.DynamicConfiguration) error {
	path := s.dynamicConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed creating dynamic config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}

	tmpPath := fmt.Sprintf("%s.tmp.%d", path, time.Now().UnixNano())
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
