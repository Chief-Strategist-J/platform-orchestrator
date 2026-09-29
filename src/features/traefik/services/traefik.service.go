/*
Package services provides the unified Traefik service facade coordinating routing, middlewares, and diagnostic overview.

ALGORITHM BLUEPRINT (TraefikService):
1. Unified Facade: Delegates specialized operations to TraefikRouterService, TraefikMiddlewareService, and TraefikOverviewService.
2. Direct Access: Exposes sub-services directly for fine-grained dependency injection and testing.
3. Invariants:
   - Zero inline comments inside function bodies.
   - Preserves distributed tracing contexts across all delegates.
*/
package services

import (
	"context"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/schema"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/types"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

type TraefikService struct {
	tracer      ports.TracerPort
	baseDir     string
	Routers     *TraefikRouterService
	Middlewares *TraefikMiddlewareService
	Overview    *TraefikOverviewService
}

func NewTraefikService(tracer ports.TracerPort, baseDir string) *TraefikService {
	return &TraefikService{
		tracer:      tracer,
		baseDir:     baseDir,
		Routers:     NewTraefikRouterService(tracer, baseDir),
		Middlewares: NewTraefikMiddlewareService(tracer, baseDir),
		Overview:    NewTraefikOverviewService(tracer, baseDir),
	}
}

func (s *TraefikService) Ping(ctx context.Context, opts types.ClientOptions) (*schema.PingResult, error) {
	return s.Overview.Ping(ctx, opts)
}

func (s *TraefikService) GetOverview(ctx context.Context, opts types.ClientOptions) (*schema.OverviewReport, error) {
	return s.Overview.GetOverview(ctx, opts)
}

func (s *TraefikService) ListEntryPoints(ctx context.Context, opts types.ClientOptions) ([]schema.EntryPointInfo, error) {
	return s.Overview.ListEntryPoints(ctx, opts)
}

func (s *TraefikService) ListHTTPRouters(ctx context.Context, opts types.ClientOptions) ([]schema.HTTPRouterDefinition, error) {
	return s.Routers.ListHTTPRouters(ctx, opts)
}

func (s *TraefikService) GetHTTPRouter(ctx context.Context, opts types.ClientOptions, name string) (*schema.HTTPRouterDefinition, error) {
	return s.Routers.GetHTTPRouter(ctx, opts, name)
}

func (s *TraefikService) SaveHTTPRouter(ctx context.Context, router schema.HTTPRouterDefinition) (*types.RouterOperationResult, error) {
	return s.Routers.SaveHTTPRouter(ctx, router)
}

func (s *TraefikService) DeleteHTTPRouter(ctx context.Context, name string) (*types.RouterOperationResult, error) {
	return s.Routers.DeleteHTTPRouter(ctx, name)
}

func (s *TraefikService) ListHTTPServices(ctx context.Context, opts types.ClientOptions) ([]schema.ServiceDefinition, error) {
	return s.Routers.ListHTTPServices(ctx, opts)
}

func (s *TraefikService) GetHTTPService(ctx context.Context, opts types.ClientOptions, name string) (*schema.ServiceDefinition, error) {
	return s.Routers.GetHTTPService(ctx, opts, name)
}

func (s *TraefikService) ListMiddlewares(ctx context.Context, opts types.ClientOptions) ([]schema.MiddlewareDefinition, error) {
	return s.Middlewares.List(ctx, opts)
}

func (s *TraefikService) ListTCPRouters(ctx context.Context, opts types.ClientOptions) ([]schema.TCPRouterDefinition, error) {
	return s.Routers.ListTCPRouters(ctx, opts)
}

func (s *TraefikService) GetTCPRouter(ctx context.Context, opts types.ClientOptions, name string) (*schema.TCPRouterDefinition, error) {
	return s.Routers.GetTCPRouter(ctx, opts, name)
}

func (s *TraefikService) SaveTCPRouter(ctx context.Context, router schema.TCPRouterDefinition) (*types.RouterOperationResult, error) {
	return s.Routers.SaveTCPRouter(ctx, router)
}

func (s *TraefikService) DeleteTCPRouter(ctx context.Context, name string) (*types.RouterOperationResult, error) {
	return s.Routers.DeleteTCPRouter(ctx, name)
}

func (s *TraefikService) ListTCPServices(ctx context.Context, opts types.ClientOptions) ([]schema.ServiceDefinition, error) {
	return s.Routers.ListTCPServices(ctx, opts)
}

func (s *TraefikService) GetTCPService(ctx context.Context, opts types.ClientOptions, name string) (*schema.ServiceDefinition, error) {
	return s.Routers.GetTCPService(ctx, opts, name)
}
