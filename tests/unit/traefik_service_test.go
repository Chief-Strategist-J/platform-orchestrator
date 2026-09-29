/*
Package unit provides unit test coverage for the Traefik feature and dynamic routing engine.
*/
package unit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	traefikSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/schema"
	traefikService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/services"
	traefikTypes "github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/types"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/infra/observability"
)

func TestTraefikDynamicRouterLifecycle(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "traefik-test-*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configDir := filepath.Join(tempDir, "config", "traefik")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed creating config dir: %v", err)
	}

	initialYAML := `
http:
  routers:
    grafana-router:
      rule: "Host(` + "`" + `grafana.llmobs.local` + "`" + `)"
      service: "grafana-service"
      entryPoints: ["websecure"]
  services:
    grafana-service:
      loadBalancer:
        servers:
          - url: "http://llmobs-grafana:3000"
`
	if err := os.WriteFile(filepath.Join(configDir, "dynamic.yml"), []byte(initialYAML), 0644); err != nil {
		t.Fatalf("failed writing initial dynamic.yml: %v", err)
	}

	tracer := observability.NewOTelTracerAdapter("test-tracer")
	svc := traefikService.NewTraefikService(tracer, tempDir)

	routers, err := svc.ListHTTPRouters(context.Background(), traefikTypes.ClientOptions{})
	if err != nil {
		t.Fatalf("failed listing HTTP routers: %v", err)
	}
	if len(routers) != 1 {
		t.Fatalf("expected 1 router, got %d", len(routers))
	}
	if routers[0].Name != "grafana-router" {
		t.Fatalf("expected router name 'grafana-router', got %q", routers[0].Name)
	}

	newRouter := traefikSchema.HTTPRouterDefinition{
		Name:    "tempo-router",
		Rule:    "Host(`tempo.llmobs.local`)",
		Service: "tempo-service",
	}
	saveRes, err := svc.SaveHTTPRouter(context.Background(), newRouter)
	if err != nil {
		t.Fatalf("failed saving HTTP router: %v", err)
	}
	if !saveRes.Success || saveRes.Status != "created" {
		t.Fatalf("unexpected save result: %+v", saveRes)
	}

	tempoRouter, err := svc.GetHTTPRouter(context.Background(), traefikTypes.ClientOptions{}, "tempo-router")
	if err != nil {
		t.Fatalf("failed getting tempo router: %v", err)
	}
	if tempoRouter.Rule != "Host(`tempo.llmobs.local`)" {
		t.Fatalf("unexpected rule: %s", tempoRouter.Rule)
	}
	if len(tempoRouter.EntryPoints) == 0 || tempoRouter.EntryPoints[0] != "websecure" {
		t.Fatalf("expected default entrypoint 'websecure', got %v", tempoRouter.EntryPoints)
	}

	delRes, err := svc.DeleteHTTPRouter(context.Background(), "tempo-router")
	if err != nil {
		t.Fatalf("failed deleting HTTP router: %v", err)
	}
	if !delRes.Success {
		t.Fatalf("expected delete success, got: %+v", delRes)
	}

	routersAfter, err := svc.ListHTTPRouters(context.Background(), traefikTypes.ClientOptions{})
	if err != nil {
		t.Fatalf("failed listing routers after delete: %v", err)
	}
	if len(routersAfter) != 1 {
		t.Fatalf("expected 1 router after delete, got %d", len(routersAfter))
	}
}

func TestTraefikTCPRouterLifecycle(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "traefik-tcp-test-*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configDir := filepath.Join(tempDir, "config", "traefik")
	_ = os.MkdirAll(configDir, 0755)
	_ = os.WriteFile(filepath.Join(configDir, "dynamic.yml"), []byte("tcp:\n  routers: {}\n"), 0644)

	tracer := observability.NewOTelTracerAdapter("test-tracer")
	svc := traefikService.NewTraefikService(tracer, tempDir)

	tcpRouter := traefikSchema.TCPRouterDefinition{
		Name:        "alloydb-tcp-router",
		Rule:        "HostSNI(`alloydb.llmobs.local`)",
		Service:     "alloydb-service",
		EntryPoints: []string{"tcp-db"},
	}

	saveRes, err := svc.SaveTCPRouter(context.Background(), tcpRouter)
	if err != nil {
		t.Fatalf("failed saving TCP router: %v", err)
	}
	if !saveRes.Success {
		t.Fatalf("expected save success, got %+v", saveRes)
	}

	tcpRouters, err := svc.ListTCPRouters(context.Background(), traefikTypes.ClientOptions{})
	if err != nil {
		t.Fatalf("failed listing TCP routers: %v", err)
	}
	if len(tcpRouters) != 1 {
		t.Fatalf("expected 1 TCP router, got %d", len(tcpRouters))
	}

	delRes, err := svc.DeleteTCPRouter(context.Background(), "alloydb-tcp-router")
	if err != nil {
		t.Fatalf("failed deleting TCP router: %v", err)
	}
	if !delRes.Success {
		t.Fatalf("expected delete success, got %+v", delRes)
	}
}

func TestTraefikMockAPIPingAndOverview(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ping":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		case "/api/overview":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"http": {
					"routers": {"total": 5, "warnings": 0, "errors": 0},
					"services": {"total": 4, "warnings": 0, "errors": 0},
					"middlewares": {"total": 3, "warnings": 0, "errors": 0}
				},
				"tcp": {
					"routers": {"total": 1, "warnings": 0, "errors": 0},
					"services": {"total": 1, "warnings": 0, "errors": 0}
				}
			}`))
		case "/api/entrypoints":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"name": "web", "address": ":80"},
				{"name": "websecure", "address": ":443"},
				{"name": "dashboard", "address": ":8080"}
			]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockServer.Close()

	tracer := observability.NewOTelTracerAdapter("test-tracer")
	svc := traefikService.NewTraefikService(tracer, "")

	opts := traefikTypes.ClientOptions{
		TraefikURL: mockServer.URL,
	}

	pingRes, err := svc.Ping(context.Background(), opts)
	if err != nil {
		t.Fatalf("failed ping: %v", err)
	}
	if !pingRes.IsHealthy || pingRes.Status != "OK" {
		t.Fatalf("unexpected ping response: %+v", pingRes)
	}

	overview, err := svc.GetOverview(context.Background(), opts)
	if err != nil {
		t.Fatalf("failed overview: %v", err)
	}
	if overview.HTTP.Routers.Total != 5 {
		t.Fatalf("expected 5 HTTP routers, got %d", overview.HTTP.Routers.Total)
	}

	entrypoints, err := svc.ListEntryPoints(context.Background(), opts)
	if err != nil {
		t.Fatalf("failed entrypoints: %v", err)
	}
	if len(entrypoints) != 3 {
		t.Fatalf("expected 3 entrypoints, got %d", len(entrypoints))
	}
}
