/*
Package schema defines external endpoint data models and resolution logic for active stack services.

ALGORITHM BLUEPRINT:
1. ServiceEndpoint: Unified data model representing an exposed platform endpoint.
2. ResolveActiveEndpoints: Maps normalized active profiles against registered infrastructure services.
3. Invariants:
   - Credentials in user-facing endpoints must be masked (e.g. ***).
   - Zero inline comments inside function bodies.
*/
package schema

import (
	"fmt"
)

type ServiceEndpoint struct {
	Service     string `json:"service"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Endpoint    string `json:"endpoint"`
}

func ResolveActiveEndpoints(profiles []string, getEnv func(key, fallback string) string) []ServiceEndpoint {
	normProfiles := NormalizeProfiles(profiles)
	isFull := false
	profMap := make(map[string]bool)
	for _, p := range normProfiles {
		profMap[p] = true
		if p == ProfileFull {
			isFull = true
		}
	}

	var endpoints []ServiceEndpoint

	if isFull || profMap[ProfileDB] || profMap[ProfileStateful] {
		portAlloy := getEnv(EnvPortAlloyDB, DefaultPortAlloyDB)
		dbUser := getEnv(EnvAlloyDBUser, DefaultAlloyDBUser)
		dbName := getEnv(EnvAlloyDBName, DefaultAlloyDBName)
		endpoints = append(endpoints, ServiceEndpoint{
			Service:     "AlloyDB (PostgreSQL)",
			Category:    "Database",
			Description: "Primary relational storage with vector embeddings",
			Endpoint:    fmt.Sprintf("postgresql://%s:***@localhost:%s/%s", dbUser, portAlloy, dbName),
		})

		portRedis := getEnv(EnvPortRedis, DefaultPortRedis)
		endpoints = append(endpoints, ServiceEndpoint{
			Service:     "Redis Ledger",
			Category:    "Database",
			Description: "Token quota tracking and fast cache",
			Endpoint:    fmt.Sprintf("redis://:***@localhost:%s/0", portRedis),
		})
	}

	if isFull || profMap[ProfileAnalytics] || profMap[ProfileStateful] {
		pHttp := getEnv(EnvPortClickHouseHTTP, DefaultPortClickHouseHTTP)
		pNative := getEnv(EnvPortClickHouseNative, DefaultPortClickHouseNative)
		endpoints = append(endpoints, ServiceEndpoint{
			Service:     "ClickHouse Analytics",
			Category:    "Analytics",
			Description: "Columnar OLAP event and span store",
			Endpoint:    fmt.Sprintf("HTTP: http://localhost:%s | Native TCP: localhost:%s", pHttp, pNative),
		})
	}

	if isFull || profMap[ProfileStreaming] || profMap[ProfileStateful] {
		pKafka := getEnv(EnvPortKafka, DefaultPortKafka)
		endpoints = append(endpoints, ServiceEndpoint{
			Service:     "Kafka Broker",
			Category:    "Streaming",
			Description: "Distributed event stream broker",
			Endpoint:    fmt.Sprintf("localhost:%s", pKafka),
		})
	}

	if isFull || profMap[ProfileWorkflows] || profMap[ProfileStateless] {
		pGrpc := getEnv(EnvPortTemporalGRPC, DefaultPortTemporalGRPC)
		pUI := getEnv(EnvPortTemporalUI, DefaultPortTemporalUI)
		endpoints = append(endpoints, ServiceEndpoint{
			Service:     "Temporal Engine",
			Category:    "Workflows",
			Description: "Durable workflow orchestration engine",
			Endpoint:    fmt.Sprintf("gRPC: localhost:%s | Web UI: http://localhost:%s", pGrpc, pUI),
		})
	}

	if isFull || profMap[ProfileTracing] || profMap[ProfileStateful] || profMap[ProfileStateless] {
		pGraf := getEnv(EnvPortGrafana, DefaultPortGrafana)
		pOtelHttp := getEnv(EnvPortOtelHTTP, DefaultPortOtelHTTP)
		pOtelGrpc := getEnv(EnvPortOtelGRPC, DefaultPortOtelGRPC)
		pTempo := getEnv(EnvPortTempo, DefaultPortTempo)

		endpoints = append(endpoints, ServiceEndpoint{
			Service:     "Grafana Dashboard",
			Category:    "Observability",
			Description: "Unified metrics, traces, and logs visualizer",
			Endpoint:    fmt.Sprintf("http://localhost:%s", pGraf),
		})
		endpoints = append(endpoints, ServiceEndpoint{
			Service:     "OTel Collector",
			Category:    "Observability",
			Description: "OpenTelemetry telemetry ingestion pipeline",
			Endpoint:    fmt.Sprintf("HTTP: http://localhost:%s | gRPC: localhost:%s", pOtelHttp, pOtelGrpc),
		})
		endpoints = append(endpoints, ServiceEndpoint{
			Service:     "Grafana Tempo",
			Category:    "Observability",
			Description: "High-volume distributed tracing backend",
			Endpoint:    fmt.Sprintf("http://localhost:%s", pTempo),
		})
	}

	if isFull || profMap[ProfileNetwork] || profMap[ProfileStateless] {
		pTrHttp := getEnv(EnvPortTraefikHTTP, DefaultPortTraefikHTTP)
		pTrDash := getEnv(EnvPortTraefikDashboard, DefaultPortTraefikDashboard)
		pTrHttps := getEnv(EnvPortTraefikHTTPS, DefaultPortTraefikHTTPS)
		pReg := getEnv(EnvPortServiceRegistry, DefaultPortServiceRegistry)

		endpoints = append(endpoints, ServiceEndpoint{
			Service:     "Traefik Gateway",
			Category:    "Network",
			Description: "Edge proxy with mTLS and route discovery",
			Endpoint:    fmt.Sprintf("http://localhost:%s (→ HTTPS:%s) | Dashboard: http://localhost:%s", pTrHttp, pTrHttps, pTrDash),
		})
		endpoints = append(endpoints, ServiceEndpoint{
			Service:     "Service Registry",
			Category:    "Network",
			Description: "Internal microservice catalog and heartbeat registry",
			Endpoint:    fmt.Sprintf("http://localhost:%s", pReg),
		})
	}

	return endpoints
}
