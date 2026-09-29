/*
Package rules provides a declarative, config-driven template registry for Grafana platform datasources.

ALGORITHM BLUEPRINT (DatasourceDefinitionTable):
1. Declarative Data Table: All built-in platform datasources declared as static configuration entries (Rule 3: Rules as Data).
2. Dynamic Environment Resolution: Resolves environment overrides via PathResolver and os.Getenv without procedural branching.
3. Registry Pattern: Maps service aliases to declarative definitions for O(1) resolution.
4. Invariants:
   - Zero inline comments inside function bodies.
   - Adding a new datasource requires only adding an entry to BuiltinDatasourceDefinitions.
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

type DatasourceDefinition struct {
	Aliases      []string
	UID          string
	Name         string
	Type         string
	Access       string
	URLPattern   string
	HostEnvKey   string
	HostDefault  string
	PortEnvKey   string
	PortDefault  string
	UserEnvKey   string
	UserDefault  string
	PassEnvKey   string
	PassDefault  string
	DBEnvKey     string
	DBDefault    string
	IsDefault    bool
	JSONDataFunc func(host, port, db string) map[string]interface{}
}

var BuiltinDatasourceDefinitions = []DatasourceDefinition{
	{
		Aliases:     []string{"alloydb", "postgres", "postgresql"},
		UID:         "ds-alloydb-platform",
		Name:        "AlloyDB-Ledger",
		Type:        "postgres",
		Access:      "proxy",
		URLPattern:  "%s:%s",
		HostEnvKey:  "ALLOYDB_HOST",
		HostDefault: "localhost",
		PortEnvKey:  "ALLOYDB_PORT",
		PortDefault: "31420",
		UserEnvKey:  "ALLOYDB_USER",
		UserDefault: "postgres",
		PassEnvKey:  "ALLOYDB_PASSWORD",
		PassDefault: "postgres",
		DBEnvKey:    "ALLOYDB_DB",
		DBDefault:   "llmobs",
		IsDefault:   false,
		JSONDataFunc: func(_, _, _ string) map[string]interface{} {
			return map[string]interface{}{
				"sslmode":         "disable",
				"postgresVersion": 1500,
				"maxOpenConns":    20,
				"maxIdleConns":    5,
				"connMaxLifetime": 14400,
			}
		},
	},
	{
		Aliases:     []string{"clickhouse", "clickhouse-analytics"},
		UID:         "ds-clickhouse-analytics",
		Name:        "ClickHouse-Analytics",
		Type:        "grafana-clickhouse-datasource",
		Access:      "proxy",
		URLPattern:  "http://%s:%s",
		HostEnvKey:  "CLICKHOUSE_HOST",
		HostDefault: "localhost",
		PortEnvKey:  "CLICKHOUSE_PORT",
		PortDefault: "31421",
		UserEnvKey:  "CLICKHOUSE_USER",
		UserDefault: "default",
		PassEnvKey:  "CLICKHOUSE_PASSWORD",
		PassDefault: "",
		DBEnvKey:    "CLICKHOUSE_DB",
		DBDefault:   "llmobs",
		IsDefault:   false,
		JSONDataFunc: func(host, _, db string) map[string]interface{} {
			return map[string]interface{}{
				"port":            31421,
				"server":          host,
				"defaultDatabase": db,
				"protocol":        "http",
			}
		},
	},
	{
		Aliases:     []string{"redis", "redis-ledger"},
		UID:         "ds-redis-ledger",
		Name:        "Redis-Ledger",
		Type:        "redis-datasource",
		Access:      "proxy",
		URLPattern:  "redis://%s:%s",
		HostEnvKey:  "REDIS_HOST",
		HostDefault: "localhost",
		PortEnvKey:  "REDIS_PORT",
		PortDefault: "31413",
		PassEnvKey:  "REDIS_PASSWORD",
		PassDefault: "",
		IsDefault:   false,
		JSONDataFunc: func(_, _, _ string) map[string]interface{} {
			return map[string]interface{}{
				"poolSize": 5,
				"timeout":  10,
			}
		},
	},
	{
		Aliases:     []string{"tempo", "tempo-traces"},
		UID:         "ds-tempo-traces",
		Name:        "Tempo-Traces",
		Type:        "tempo",
		Access:      "proxy",
		URLPattern:  "http://%s:%s",
		HostEnvKey:  "TEMPO_HOST",
		HostDefault: "localhost",
		PortEnvKey:  "TEMPO_PORT",
		PortDefault: "31416",
		IsDefault:   true,
		JSONDataFunc: func(_, _, _ string) map[string]interface{} {
			return map[string]interface{}{
				"tracesToLogs": map[string]interface{}{
					"datasourceUid": "ds-clickhouse-analytics",
				},
			}
		},
	},
	{
		Aliases:     []string{"prometheus", "prometheus-metrics"},
		UID:         "ds-prometheus-metrics",
		Name:        "Prometheus",
		Type:        "prometheus",
		Access:      "proxy",
		URLPattern:  "http://%s:%s",
		HostEnvKey:  "PROMETHEUS_HOST",
		HostDefault: "localhost",
		PortEnvKey:  "PROMETHEUS_PORT",
		PortDefault: "9090",
		IsDefault:   false,
		JSONDataFunc: func(_, _, _ string) map[string]interface{} {
			return map[string]interface{}{
				"httpMethod": "POST",
			}
		},
	},
	{
		Aliases:     []string{"loki", "loki-logs"},
		UID:         "ds-loki-logs",
		Name:        "Loki-Logs",
		Type:        "loki",
		Access:      "proxy",
		URLPattern:  "http://%s:%s",
		HostEnvKey:  "LOKI_HOST",
		HostDefault: "localhost",
		PortEnvKey:  "LOKI_PORT",
		PortDefault: "3100",
		IsDefault:   false,
	},
}

type DatasourceTemplateRegistry struct {
	mu          sync.RWMutex
	definitions map[string]DatasourceDefinition
}

var DefaultDatasourceRegistry = NewDatasourceTemplateRegistry()

func NewDatasourceTemplateRegistry() *DatasourceTemplateRegistry {
	r := &DatasourceTemplateRegistry{
		definitions: make(map[string]DatasourceDefinition),
	}
	for _, def := range BuiltinDatasourceDefinitions {
		r.RegisterDefinition(def)
	}
	return r
}

func (r *DatasourceTemplateRegistry) RegisterDefinition(def DatasourceDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, alias := range def.Aliases {
		r.definitions[strings.ToLower(strings.TrimSpace(alias))] = def
	}
}

func (r *DatasourceTemplateRegistry) Resolve(serviceName string, resolver *paths.PathResolver) (schema.DatasourcePayload, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	def, exists := r.definitions[strings.ToLower(strings.TrimSpace(serviceName))]
	if !exists {
		return schema.DatasourcePayload{}, false
	}

	host := resolveEnv(resolver, def.HostEnvKey, def.HostDefault)
	port := resolveEnv(resolver, def.PortEnvKey, def.PortDefault)
	user := resolveEnv(resolver, def.UserEnvKey, def.UserDefault)
	pass := resolveEnv(resolver, def.PassEnvKey, def.PassDefault)
	db := resolveEnv(resolver, def.DBEnvKey, def.DBDefault)

	urlStr := ""
	if def.URLPattern != "" {
		urlStr = fmt.Sprintf(def.URLPattern, host, port)
	}

	var jsonData map[string]interface{}
	if def.JSONDataFunc != nil {
		jsonData = def.JSONDataFunc(host, port, db)
	}

	var secureJSONData map[string]string
	if pass != "" {
		secureJSONData = map[string]string{"password": pass}
	}

	return schema.DatasourcePayload{
		UID:            def.UID,
		Name:           def.Name,
		Type:           def.Type,
		Access:         def.Access,
		URL:            urlStr,
		User:           user,
		Database:       db,
		BasicAuth:      false,
		IsDefault:      def.IsDefault,
		JSONData:       jsonData,
		SecureJSONData: secureJSONData,
	}, true
}

func resolveEnv(r *paths.PathResolver, key, fallback string) string {
	if key == "" {
		return fallback
	}
	if val := os.Getenv(key); val != "" {
		return val
	}
	if r != nil {
		return r.ResolveEnvOrConfig(key, fallback)
	}
	return fallback
}
