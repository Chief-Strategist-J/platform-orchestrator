/*
Package endpoints centralizes all Traefik REST API path constants and dynamic URL builders.

ALGORITHM BLUEPRINT:
1. Static Constants: Centralizes Traefik rawdata, overview, entrypoints, ping, and resource collection endpoints.
2. Dynamic Builders: Safe URL path formatting for specific HTTP/TCP routers, services, and middlewares.
3. Invariants:
   - Zero inline comments inside function bodies.
   - All paths are prefixed with leading slash without trailing slashes.
*/
package endpoints

import "fmt"

const (
	EndpointPing            = "/ping"
	EndpointOverview        = "/api/overview"
	EndpointRawData         = "/api/rawdata"
	EndpointEntryPoints     = "/api/entrypoints"
	EndpointHTTPRouters     = "/api/http/routers"
	EndpointHTTPServices    = "/api/http/services"
	EndpointHTTPMiddlewares = "/api/http/middlewares"
	EndpointTCPRouters      = "/api/tcp/routers"
	EndpointTCPServices     = "/api/tcp/services"
	EndpointTCPMiddlewares  = "/api/tcp/middlewares"
)

func BuildHTTPRouterPath(name string) string {
	return fmt.Sprintf("%s/%s", EndpointHTTPRouters, name)
}

func BuildHTTPServicePath(name string) string {
	return fmt.Sprintf("%s/%s", EndpointHTTPServices, name)
}

func BuildHTTPMiddlewarePath(name string) string {
	return fmt.Sprintf("%s/%s", EndpointHTTPMiddlewares, name)
}

func BuildTCPRouterPath(name string) string {
	return fmt.Sprintf("%s/%s", EndpointTCPRouters, name)
}

func BuildTCPServicePath(name string) string {
	return fmt.Sprintf("%s/%s", EndpointTCPServices, name)
}
