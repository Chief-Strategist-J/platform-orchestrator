/*
Package rules provides a declarative template registry for Grafana platform datasource generation.

ALGORITHM BLUEPRINT (DatasourceTemplateRegistry):
1. Registry Pattern: Maps service names/aliases to declarative template builder functions.
2. Dynamic Environment Resolution: Builders retrieve dynamic host, port, database, and credentials via PathResolver.
3. Extensibility: New datasources can be registered at runtime without modifying service switch/case statements.
4. Invariants:
   - Zero inline comments inside function bodies.
   - Lookup is case-insensitive and supports multiple aliases per service type.
*/
package rules

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/schema"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/paths"
)

type DatasourceTemplateBuilder func(resolver *paths.PathResolver) schema.DatasourcePayload

type DatasourceTemplateRegistry struct {
	mu        sync.RWMutex
	templates map[string]DatasourceTemplateBuilder
}

var DefaultDatasourceRegistry = NewDatasourceTemplateRegistry()

func NewDatasourceTemplateRegistry() *DatasourceTemplateRegistry {
	r := &DatasourceTemplateRegistry{
		templates: make(map[string]DatasourceTemplateBuilder),
	}
	r.registerBuiltins()
	return r
}

func (r *DatasourceTemplateRegistry) Register(builder DatasourceTemplateBuilder, aliases ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, alias := range aliases {
		r.templates[strings.ToLower(strings.TrimSpace(alias))] = builder
	}
}

func (r *DatasourceTemplateRegistry) Resolve(serviceName string, resolver *paths.PathResolver) (schema.DatasourcePayload, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	builder, exists := r.templates[strings.ToLower(strings.TrimSpace(serviceName))]
	if !exists {
		return schema.DatasourcePayload{}, false
	}
	return builder(resolver), true
}

func (r *DatasourceTemplateRegistry) registerBuiltins() {
	r.Register(func(resolver *paths.PathResolver) schema.DatasourcePayload {
		host := resolveEnvOrResolver(resolver, "ALLOYDB_HOST", "localhost")
		port := resolveEnvOrResolver(resolver, "ALLOYDB_PORT", "31420")
		user := resolveEnvOrResolver(resolver, "ALLOYDB_USER", "postgres")
		pass := resolveEnvOrResolver(resolver, "ALLOYDB_PASSWORD", "postgres")
		db := resolveEnvOrResolver(resolver, "ALLOYDB_DB", "llmobs")

		return schema.DatasourcePayload{
			UID:       "ds-alloydb-platform",
			Name:      "AlloyDB-Ledger",
			Type:      "postgres",
			Access:    "proxy",
			URL:       fmt.Sprintf("%s:%s", host, port),
			User:      user,
			Database:  db,
			BasicAuth: false,
			IsDefault: false,
			JSONData: map[string]interface{}{
				"sslmode":         "disable",
				"postgresVersion": 1500,
				"maxOpenConns":    20,
				"maxIdleConns":    5,
				"connMaxLifetime": 14400,
			},
			SecureJSONData: map[string]string{
				"password": pass,
			},
		}
	}, "alloydb", "postgres", "postgresql")

	r.Register(func(resolver *paths.PathResolver) schema.DatasourcePayload {
		host := resolveEnvOrResolver(resolver, "CLICKHOUSE_HOST", "localhost")
		port := resolveEnvOrResolver(resolver, "CLICKHOUSE_PORT", "31421")
		user := resolveEnvOrResolver(resolver, "CLICKHOUSE_USER", "default")
		pass := resolveEnvOrResolver(resolver, "CLICKHOUSE_PASSWORD", "")
		db := resolveEnvOrResolver(resolver, "CLICKHOUSE_DB", "llmobs")

		return schema.DatasourcePayload{
			UID:       "ds-clickhouse-analytics",
			Name:      "ClickHouse-Analytics",
			Type:      "grafana-clickhouse-datasource",
			Access:    "proxy",
			URL:       fmt.Sprintf("http://%s:%s", host, port),
			User:      user,
			Database:  db,
			BasicAuth: false,
			IsDefault: false,
			JSONData: map[string]interface{}{
				"port":            31421,
				"server":          host,
				"defaultDatabase": db,
				"protocol":        "http",
			},
			SecureJSONData: map[string]string{
				"password": pass,
			},
		}
	}, "clickhouse", "clickhouse-analytics")

	r.Register(func(resolver *paths.PathResolver) schema.DatasourcePayload {
		host := resolveEnvOrResolver(resolver, "REDIS_HOST", "localhost")
		port := resolveEnvOrResolver(resolver, "REDIS_PORT", "31413")
		pass := resolveEnvOrResolver(resolver, "REDIS_PASSWORD", "")

		return schema.DatasourcePayload{
			UID:       "ds-redis-ledger",
			Name:      "Redis-Ledger",
			Type:      "redis-datasource",
			Access:    "proxy",
			URL:       fmt.Sprintf("redis://%s:%s", host, port),
			BasicAuth: false,
			IsDefault: false,
			JSONData: map[string]interface{}{
				"poolSize": 5,
				"timeout":  10,
			},
			SecureJSONData: map[string]string{
				"password": pass,
			},
		}
	}, "redis", "redis-ledger")

	r.Register(func(resolver *paths.PathResolver) schema.DatasourcePayload {
		host := resolveEnvOrResolver(resolver, "TEMPO_HOST", "localhost")
		port := resolveEnvOrResolver(resolver, "TEMPO_PORT", "31416")

		return schema.DatasourcePayload{
			UID:       "ds-tempo-traces",
			Name:      "Tempo-Traces",
			Type:      "tempo",
			Access:    "proxy",
			URL:       fmt.Sprintf("http://%s:%s", host, port),
			BasicAuth: false,
			IsDefault: true,
			JSONData: map[string]interface{}{
				"tracesToLogs": map[string]interface{}{
					"datasourceUid": "ds-clickhouse-analytics",
				},
			},
		}
	}, "tempo", "tempo-traces")

	r.Register(func(resolver *paths.PathResolver) schema.DatasourcePayload {
		host := resolveEnvOrResolver(resolver, "PROMETHEUS_HOST", "localhost")
		port := resolveEnvOrResolver(resolver, "PROMETHEUS_PORT", "9090")

		return schema.DatasourcePayload{
			UID:       "ds-prometheus-metrics",
			Name:      "Prometheus",
			Type:      "prometheus",
			Access:    "proxy",
			URL:       fmt.Sprintf("http://%s:%s", host, port),
			BasicAuth: false,
			IsDefault: false,
			JSONData: map[string]interface{}{
				"httpMethod": "POST",
			},
		}
	}, "prometheus", "prometheus-metrics")

	r.Register(func(resolver *paths.PathResolver) schema.DatasourcePayload {
		host := resolveEnvOrResolver(resolver, "LOKI_HOST", "localhost")
		port := resolveEnvOrResolver(resolver, "LOKI_PORT", "3100")

		return schema.DatasourcePayload{
			UID:       "ds-loki-logs",
			Name:      "Loki-Logs",
			Type:      "loki",
			Access:    "proxy",
			URL:       fmt.Sprintf("http://%s:%s", host, port),
			BasicAuth: false,
			IsDefault: false,
		}
	}, "loki", "loki-logs")
}

func resolveEnvOrResolver(r *paths.PathResolver, key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	if r != nil {
		return r.ResolveEnvOrConfig(key, fallback)
	}
	return fallback
}
