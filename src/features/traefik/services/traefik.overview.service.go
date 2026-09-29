/*
Package services provides domain-specific services for Traefik diagnostic and overview operations.

ALGORITHM BLUEPRINT (TraefikOverviewService):
1. Ping: Calls /ping endpoint and measures round-trip latency.
2. GetOverview: Calls /api/overview and aggregates active router/service/middleware counts, with fallback to dynamic configuration.
3. ListEntryPoints: Calls /api/entrypoints and returns port and protocol bindings.
4. Invariants:
   - Zero inline comments inside function bodies.
   - All calls instrumented with OpenTelemetry spans and latency metrics.
*/
package services

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/client"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/schema"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/types"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/infra/observability"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/paths"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

type TraefikOverviewService struct {
	tracer   ports.TracerPort
	baseDir  string
	resolver *paths.PathResolver
}

func NewTraefikOverviewService(tracer ports.TracerPort, baseDir string) *TraefikOverviewService {
	return &TraefikOverviewService{
		tracer:   tracer,
		baseDir:  baseDir,
		resolver: paths.NewPathResolver(baseDir),
	}
}

func (s *TraefikOverviewService) Ping(ctx context.Context, opts types.ClientOptions) (*schema.PingResult, error) {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "traefik.ping", map[string]interface{}{
		"target": "traefik-gateway",
	})
	defer span.End()

	c := client.NewTraefikClient(opts, s.resolver)
	traefikCfg := s.resolver.GetTraefikConfig()
	start := time.Now()

	status, err := c.Do(ctx, http.MethodGet, traefikCfg.API.Endpoints.Ping, nil, nil)
	if err != nil || status != http.StatusOK {
		status, err = c.Do(ctx, http.MethodGet, traefikCfg.API.Endpoints.Overview, nil, nil)
	}
	latency := float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil || status != http.StatusOK {
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		}
		observability.RecordError(ctx, err)
		return &schema.PingResult{
			Status:    "unreachable",
			IsHealthy: false,
			LatencyMs: latency,
			Error:     errMsg,
		}, err
	}

	observability.SetAttributes(ctx, map[string]interface{}{
		"ping.status":  "OK",
		"ping.latency": latency,
	})

	return &schema.PingResult{
		Status:    "OK",
		IsHealthy: true,
		LatencyMs: latency,
	}, nil
}

func (s *TraefikOverviewService) GetOverview(ctx context.Context, opts types.ClientOptions) (*schema.OverviewReport, error) {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "traefik.overview.get", nil)
	defer span.End()

	c := client.NewTraefikClient(opts, s.resolver)
	traefikCfg := s.resolver.GetTraefikConfig()
	var overview schema.OverviewReport

	_, err := c.Do(ctx, http.MethodGet, traefikCfg.API.Endpoints.Overview, nil, &overview)
	if err == nil {
		observability.SetAttributes(ctx, map[string]interface{}{
			"http.routers.total": overview.HTTP.Routers.Total,
			"tcp.routers.total":  overview.TCP.Routers.Total,
		})
		return &overview, nil
	}

	dynConfig, fileErr := s.readDynamicConfig()
	if fileErr != nil {
		observability.RecordError(ctx, err)
		return nil, err
	}

	httpRoutersCount := 0
	httpServicesCount := 0
	httpMiddlewaresCount := 0
	tcpRoutersCount := 0
	tcpServicesCount := 0

	if dynConfig.HTTP != nil {
		httpRoutersCount = len(dynConfig.HTTP.Routers)
		httpServicesCount = len(dynConfig.HTTP.Services)
		httpMiddlewaresCount = len(dynConfig.HTTP.Middlewares)
	}
	if dynConfig.TCP != nil {
		tcpRoutersCount = len(dynConfig.TCP.Routers)
		tcpServicesCount = len(dynConfig.TCP.Services)
	}

	return &schema.OverviewReport{
		HTTP: schema.OverviewSection{
			Routers:     schema.ComponentCount{Total: httpRoutersCount},
			Services:    schema.ComponentCount{Total: httpServicesCount},
			Middlewares: schema.ComponentCount{Total: httpMiddlewaresCount},
		},
		TCP: schema.OverviewSection{
			Routers:  schema.ComponentCount{Total: tcpRoutersCount},
			Services: schema.ComponentCount{Total: tcpServicesCount},
		},
	}, nil
}

func (s *TraefikOverviewService) ListEntryPoints(ctx context.Context, opts types.ClientOptions) ([]schema.EntryPointInfo, error) {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "traefik.entrypoints.list", nil)
	defer span.End()

	c := client.NewTraefikClient(opts, s.resolver)
	traefikCfg := s.resolver.GetTraefikConfig()
	var rawEntryPoints []schema.EntryPointInfo

	_, err := c.Do(ctx, http.MethodGet, traefikCfg.API.Endpoints.Entrypoints, nil, &rawEntryPoints)
	if err != nil {
		observability.RecordError(ctx, err)
		return nil, err
	}

	observability.SetAttribute(ctx, "entrypoints.count", len(rawEntryPoints))
	return rawEntryPoints, nil
}

func (s *TraefikOverviewService) readDynamicConfig() (*schema.DynamicConfiguration, error) {
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
