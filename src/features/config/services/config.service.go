/*
Package services implements configuration management for platform resources, environment parameters, and container limits.

ALGORITHM BLUEPRINT:
1. GetPlatformConfig: Reads active environment configuration and merges with default.yaml contracts.
2. UpdatePlatformConfig: Validates and writes updated resource limits and env keys to the active .env file.
3. ServiceRestart: Optionally applies new resource limits by triggering container restarts.
4. Validation: Tests compose specification parsing before confirming updates.
5. Invariants:
   - Zero inline comments inside function bodies.
   - Unspecified resource limits preserve established production baselines.
*/
package services

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/config/schema"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/paths"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

type ConfigService struct {
	workspaceRoot string
	tracer        ports.TracerPort
}

func NewConfigService(workspaceRoot string, tracer ports.TracerPort) *ConfigService {
	return &ConfigService{
		workspaceRoot: workspaceRoot,
		tracer:        tracer,
	}
}

func (s *ConfigService) GetPlatformConfig(ctx context.Context) (*schema.PlatformConfigReport, error) {
	_, end := s.tracer.StartSpan(ctx, "ConfigService.GetPlatformConfig")
	defer end()

	resolver := paths.NewPathResolver(s.workspaceRoot)
	envMap := s.readEnvMap(resolver.EnvFile())

	netName, netSubnet, netGateway := resolver.ResolveNetworkConfig("", "", "")

	report := &schema.PlatformConfigReport{
		NetworkName:    netName,
		NetworkSubnet:  netSubnet,
		NetworkGateway: netGateway,
		ActiveEnvFile:  resolver.EnvFile(),
		ComposeFile:    resolver.ComposeFile("docker-compose.yml"),
		Resources: schema.PlatformResources{
			AlloyDB: schema.ServiceResources{
				MemoryLimit:       s.getWithFallback(envMap, "ALLOYDB_MEM_LIMIT", "3072M"),
				MemoryReservation: s.getWithFallback(envMap, "ALLOYDB_MEM_RESERVATION", "1024M"),
				CpusLimit:         s.getWithFallback(envMap, "ALLOYDB_CPUS_LIMIT", "2.0"),
				CpusReservation:   s.getWithFallback(envMap, "ALLOYDB_CPUS_RESERVATION", "0.5"),
			},
			Temporal: schema.ServiceResources{
				MemoryLimit:       s.getWithFallback(envMap, "TEMPORAL_MEM_LIMIT", "2048M"),
				MemoryReservation: s.getWithFallback(envMap, "TEMPORAL_MEM_RESERVATION", "512M"),
			},
			ClickHouse: schema.ServiceResources{
				MemoryLimit:       s.getWithFallback(envMap, "CLICKHOUSE_MEM_LIMIT", "4096M"),
				MemoryReservation: s.getWithFallback(envMap, "CLICKHOUSE_MEM_RESERVATION", "1024M"),
			},
			Redis: schema.ServiceResources{
				MemoryLimit:       s.getWithFallback(envMap, "REDIS_MEM_LIMIT", "256M"),
				MemoryReservation: s.getWithFallback(envMap, "REDIS_MEM_RESERVATION", "64M"),
			},
			Kafka: schema.ServiceResources{
				MemoryLimit:       s.getWithFallback(envMap, "KAFKA_MEM_LIMIT", "2048M"),
				MemoryReservation: s.getWithFallback(envMap, "KAFKA_MEM_RESERVATION", "512M"),
			},
			Traefik: schema.ServiceResources{
				MemoryLimit:       s.getWithFallback(envMap, "TRAEFIK_MEM_LIMIT", "256M"),
				MemoryReservation: s.getWithFallback(envMap, "TRAEFIK_MEM_RESERVATION", "64M"),
			},
			Tempo: schema.ServiceResources{
				MemoryLimit:       s.getWithFallback(envMap, "TEMPO_MEM_LIMIT", "1024M"),
				MemoryReservation: s.getWithFallback(envMap, "TEMPO_MEM_RESERVATION", "128M"),
			},
			OTelCollector: schema.ServiceResources{
				MemoryLimit:       s.getWithFallback(envMap, "OTEL_MEM_LIMIT", "1024M"),
				MemoryReservation: s.getWithFallback(envMap, "OTEL_MEM_RESERVATION", "256M"),
			},
			Grafana: schema.ServiceResources{
				MemoryLimit:       s.getWithFallback(envMap, "GRAFANA_MEM_LIMIT", "512M"),
				MemoryReservation: s.getWithFallback(envMap, "GRAFANA_MEM_RESERVATION", "128M"),
			},
			ServiceRegistry: schema.ServiceResources{
				MemoryLimit:       s.getWithFallback(envMap, "REGISTRY_MEM_LIMIT", "128M"),
				MemoryReservation: s.getWithFallback(envMap, "REGISTRY_MEM_RESERVATION", "32M"),
			},
		},
	}

	return report, nil
}

func (s *ConfigService) UpdatePlatformConfig(ctx context.Context, cmd schema.UpdateConfigCommand) (*schema.PlatformConfigReport, error) {
	_, end := s.tracer.StartSpan(ctx, "ConfigService.UpdatePlatformConfig")
	defer end()

	resolver := paths.NewPathResolver(s.workspaceRoot)
	envPath := resolver.EnvFile()
	examplePath := resolver.EnvExampleFile()

	var content string
	if data, err := os.ReadFile(envPath); err == nil {
		content = string(data)
	} else if data, err := os.ReadFile(examplePath); err == nil {
		content = string(data)
	}

	updates := make(map[string]string)
	if cmd.AlloyDBMemory != "" {
		updates["ALLOYDB_MEM_LIMIT"] = cmd.AlloyDBMemory
	}
	if cmd.AlloyDBCpus != "" {
		updates["ALLOYDB_CPUS_LIMIT"] = cmd.AlloyDBCpus
	}
	if cmd.TemporalMemory != "" {
		updates["TEMPORAL_MEM_LIMIT"] = cmd.TemporalMemory
	}
	if cmd.ClickHouseMemory != "" {
		updates["CLICKHOUSE_MEM_LIMIT"] = cmd.ClickHouseMemory
	}
	if cmd.RedisMemory != "" {
		updates["REDIS_MEM_LIMIT"] = cmd.RedisMemory
	}
	if cmd.KafkaMemory != "" {
		updates["KAFKA_MEM_LIMIT"] = cmd.KafkaMemory
	}
	if cmd.NetworkName != "" {
		updates["LLMOBS_NETWORK_NAME"] = cmd.NetworkName
	}
	for k, v := range cmd.CustomEnvSettings {
		if k != "" && v != "" {
			updates[k] = v
		}
	}

	for k, v := range updates {
		content = s.updateOrAppendEnv(content, k, v)
		_ = os.Setenv(k, v)
	}

	if err := os.WriteFile(envPath, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("failed to save configuration: %w", err)
	}

	composeFile := resolver.ComposeFile("docker-compose.yml")
	if abs, err := filepath.Abs(composeFile); err == nil {
		composeFile = abs
	}
	if _, statErr := os.Stat(composeFile); statErr == nil {
		testCmd := exec.CommandContext(ctx, "docker", "compose", "-f", composeFile, "config", "--quiet")
		testCmd.Dir = filepath.Dir(composeFile)
		if out, err := testCmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("compose validation failed with new configuration (%s): %w", strings.TrimSpace(string(out)), err)
		}
	}

	if cmd.RestartServices {
		recreateCmd := exec.CommandContext(ctx, "docker", "compose", "-f", composeFile, "up", "-d")
		recreateCmd.Dir = filepath.Dir(composeFile)
		_ = recreateCmd.Run()
	}

	return s.GetPlatformConfig(ctx)
}

func (s *ConfigService) PromptInteractive(reader *bufio.Reader, current *schema.PlatformConfigReport) schema.UpdateConfigCommand {
	cmd := schema.UpdateConfigCommand{}

	fmt.Println("\nConfigure Platform Resource Constraints (press Enter to keep current value):")

	fmt.Printf("  AlloyDB Memory Limit [current: %s]: ", current.Resources.AlloyDB.MemoryLimit)
	if val := s.readLine(reader); val != "" {
		cmd.AlloyDBMemory = val
	}

	fmt.Printf("  AlloyDB CPU Limit [current: %s]: ", current.Resources.AlloyDB.CpusLimit)
	if val := s.readLine(reader); val != "" {
		cmd.AlloyDBCpus = val
	}

	fmt.Printf("  Temporal Memory Limit [current: %s]: ", current.Resources.Temporal.MemoryLimit)
	if val := s.readLine(reader); val != "" {
		cmd.TemporalMemory = val
	}

	fmt.Printf("  ClickHouse Memory Limit [current: %s]: ", current.Resources.ClickHouse.MemoryLimit)
	if val := s.readLine(reader); val != "" {
		cmd.ClickHouseMemory = val
	}

	fmt.Printf("  Redis Memory Limit [current: %s]: ", current.Resources.Redis.MemoryLimit)
	if val := s.readLine(reader); val != "" {
		cmd.RedisMemory = val
	}

	fmt.Printf("  Kafka Memory Limit [current: %s]: ", current.Resources.Kafka.MemoryLimit)
	if val := s.readLine(reader); val != "" {
		cmd.KafkaMemory = val
	}

	fmt.Printf("  Apply and restart running containers immediately? (Y/n) [default: Y]: ")
	ans := strings.ToLower(s.readLine(reader))
	cmd.RestartServices = ans == "" || ans == "y" || ans == "yes"

	return cmd
}

func (s *ConfigService) readLine(reader *bufio.Reader) string {
	val, err := reader.ReadString('\n')
	if err != nil {
		return ""
	}
	return strings.TrimSpace(val)
}

func (s *ConfigService) readEnvMap(filepath string) map[string]string {
	result := make(map[string]string)
	data, err := os.ReadFile(filepath)
	if err != nil {
		return result
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if idx := strings.Index(trimmed, "="); idx > 0 {
			k := strings.TrimSpace(trimmed[:idx])
			v := strings.Trim(strings.TrimSpace(trimmed[idx+1:]), "\"'")
			result[k] = v
		}
	}
	return result
}

func (s *ConfigService) getWithFallback(envMap map[string]string, key, fallback string) string {
	if val, ok := envMap[key]; ok && val != "" {
		return val
	}
	if env := os.Getenv(key); env != "" {
		return env
	}
	return fallback
}

func (s *ConfigService) updateOrAppendEnv(content, key, val string) string {
	lines := strings.Split(content, "\n")
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, key+"=") || strings.HasPrefix(trimmed, "# "+key+"=") {
			lines[i] = fmt.Sprintf("%s=%s", key, val)
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, fmt.Sprintf("%s=%s", key, val))
	}
	return strings.Join(lines, "\n")
}
