/*
Package schema defines data contracts for platform resources, environment parameters, and service configuration.

ALGORITHM BLUEPRINT:
1. ResourceConfig: Specifies CPU and memory limits and reservations per service.
2. PlatformConfigReport: Aggregates resource limits, network configuration, and active endpoints.
3. UpdateConfigCommand: Input payload for modifying platform resource constraints and credentials.
4. Invariants:
   - All resource constraints maintain deterministic fallback defaults aligned with default.yaml.
   - Zero inline comments inside function bodies.
*/
package schema

type ServiceResources struct {
	MemoryLimit       string `json:"memoryLimit"`
	MemoryReservation string `json:"memoryReservation"`
	CpusLimit         string `json:"cpusLimit,omitempty"`
	CpusReservation   string `json:"cpusReservation,omitempty"`
}

type PlatformResources struct {
	AlloyDB         ServiceResources `json:"alloydb"`
	Temporal        ServiceResources `json:"temporal"`
	ClickHouse      ServiceResources `json:"clickhouse"`
	Redis           ServiceResources `json:"redis"`
	Kafka           ServiceResources `json:"kafka"`
	Traefik         ServiceResources `json:"traefik"`
	Tempo           ServiceResources `json:"tempo"`
	OTelCollector   ServiceResources `json:"otelCollector"`
	Grafana         ServiceResources `json:"grafana"`
	ServiceRegistry ServiceResources `json:"serviceRegistry"`
}

type PlatformConfigReport struct {
	Resources      PlatformResources `json:"resources"`
	NetworkName    string            `json:"networkName"`
	NetworkSubnet  string            `json:"networkSubnet"`
	NetworkGateway string            `json:"networkGateway"`
	ActiveEnvFile  string            `json:"activeEnvFile"`
	ComposeFile    string            `json:"composeFile"`
}

type UpdateConfigCommand struct {
	Interactive       bool              `json:"interactive,omitempty"`
	RestartServices   bool              `json:"restartServices,omitempty"`
	AlloyDBMemory     string            `json:"alloydbMemory,omitempty"`
	AlloyDBCpus       string            `json:"alloydbCpus,omitempty"`
	TemporalMemory    string            `json:"temporalMemory,omitempty"`
	ClickHouseMemory  string            `json:"clickhouseMemory,omitempty"`
	RedisMemory       string            `json:"redisMemory,omitempty"`
	KafkaMemory       string            `json:"kafkaMemory,omitempty"`
	NetworkName       string            `json:"networkName,omitempty"`
	CustomEnvSettings map[string]string `json:"customEnvSettings,omitempty"`
}
