/*
Package schema defines platform service ports, credentials, and socket listener contracts.

ALGORITHM BLUEPRINT:
1. Environment Variable Mapping: Binds service ports and credentials to canonical environment keys.
2. Port Fallback Defaults: Standard platform port assignments isolating LLMObs infrastructure.
3. Invariants:
   - Port constants must align with config/env.schema and config/default.yaml.
   - Zero inline comments inside function bodies.
*/
package schema

const (
	EnvPortAlloyDB              = "PORT_ALLOYDB"
	DefaultPortAlloyDB          = "31420"
	EnvAlloyDBUser              = "ALLOYDB_USER"
	DefaultAlloyDBUser          = "admin"
	EnvAlloyDBName              = "ALLOYDB_DB"
	DefaultAlloyDBName          = "llm_observability"

	EnvPortRedis                = "PORT_REDIS"
	DefaultPortRedis            = "31413"

	EnvPortClickHouseHTTP       = "PORT_CLICKHOUSE_HTTP"
	DefaultPortClickHouseHTTP   = "31421"
	EnvPortClickHouseNative     = "PORT_CLICKHOUSE_NATIVE"
	DefaultPortClickHouseNative = "31422"

	EnvPortKafka                = "PORT_KAFKA"
	DefaultPortKafka            = "31414"

	EnvPortTemporalGRPC         = "PORT_TEMPORAL_GRPC"
	DefaultPortTemporalGRPC     = "31424"
	EnvPortTemporalUI           = "PORT_TEMPORAL_UI"
	DefaultPortTemporalUI       = "31425"

	EnvPortGrafana              = "PORT_GRAFANA"
	DefaultPortGrafana          = "31415"

	EnvPortOtelHTTP             = "PORT_OTEL_HTTP"
	DefaultPortOtelHTTP         = "31417"
	EnvPortOtelGRPC             = "PORT_OTEL_GRPC"
	DefaultPortOtelGRPC         = "31418"

	EnvPortTempo                = "PORT_TEMPO"
	DefaultPortTempo            = "31416"

	EnvPortTraefikHTTP          = "PORT_TRAEFIK_HTTP"
	DefaultPortTraefikHTTP      = "31410"
	EnvPortTraefikDashboard     = "PORT_TRAEFIK_DASHBOARD"
	DefaultPortTraefikDashboard = "31411"
	EnvPortTraefikHTTPS         = "PORT_TRAEFIK_HTTPS"
	DefaultPortTraefikHTTPS     = "31419"

	EnvPortServiceRegistry      = "PORT_SERVICE_REGISTRY"
	DefaultPortServiceRegistry  = "31426"
)
