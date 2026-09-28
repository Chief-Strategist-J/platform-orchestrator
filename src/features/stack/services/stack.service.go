/*
Package services implements stack lifecycle management (up, down, restart, status, logs) and endpoint reporting.

ALGORITHM BLUEPRINT:
1. StartStack:
   - Ensures workspace .env exists; copies from .env.example if missing.
   - Resolves profiles and selects compose files via rules with string normalization.
   - Ensures docker bridge network exists with parameters from command/API, environment, or YAML config.
   - Prepares persistent data storage directories from schema unless running in pure stateless profile.
   - Executes ComposeUp through ContainerPort and renders active endpoint URLs.
2. StopStack: Executes ComposeDown across all profiles with schema constants and fail-fast errors.
3. RestartStack: Executes ComposeRestart with normalized profiles and fail-fast errors.
4. GetStatus: Returns active container statuses using resolved compose paths.
5. PrintEndpoints: Inspects active profiles using schema.ResolveActiveEndpoints.
6. Invariants:
   - Stateless profiles skip local persistent storage directory creation.
   - Zero inline comments inside function bodies.
*/
package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/stack/rules"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/stack/schema"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/paths"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

type StackService struct {
	containerPort ports.ContainerPort
	networkPort   ports.NetworkPort
	tracer        ports.TracerPort
	baseDir       string
}

func NewStackService(
	containerPort ports.ContainerPort,
	networkPort ports.NetworkPort,
	tracer ports.TracerPort,
	baseDir string,
) *StackService {
	return &StackService{
		containerPort: containerPort,
		networkPort:   networkPort,
		tracer:        tracer,
		baseDir:       baseDir,
	}
}

func (s *StackService) ensureEnvFile() error {
	resolver := paths.NewPathResolver(s.baseDir)
	envPath := resolver.EnvFile()
	if _, err := os.Stat(envPath); err == nil {
		return nil
	}

	examplePath := resolver.EnvExampleFile()
	data, err := os.ReadFile(examplePath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", examplePath, err)
	}
	return os.WriteFile(envPath, data, 0644)
}

func (s *StackService) ensureStorageDirectories(dataDir string) error {
	for _, sub := range schema.DefaultStorageSubdirs {
		p := filepath.Join(dataDir, sub)
		if err := os.MkdirAll(p, 0777); err != nil {
			return fmt.Errorf("failed to create data dir %s: %w", p, err)
		}
		_ = os.Chmod(p, 0777)
	}
	return nil
}

func (s *StackService) getEnv(key string, fallback string) string {
	normKey := schema.NormalizeString(key)
	resolver := paths.NewPathResolver(s.baseDir)
	envPath := resolver.EnvFile()
	data, err := os.ReadFile(envPath)
	if err != nil {
		return fallback
	}
	lines := strings.Split(string(data), "\n")
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 2 && schema.NormalizeString(parts[0]) == normKey {
			return strings.Trim(parts[1], `"' `)
		}
	}
	return fallback
}

func (s *StackService) StartStack(ctx context.Context, cmd schema.StackUpCommand) (schema.StackActionOutcome, error) {
	_, endSpan := s.tracer.StartSpan(ctx, "llmobs.stack.up")
	defer endSpan()

	if err := s.ensureEnvFile(); err != nil {
		return schema.StackActionOutcome{
			Status:  schema.StatusError,
			Message: fmt.Sprintf("environment initialization failed: %v", err),
		}, fmt.Errorf("environment initialization failed: %w", err)
	}

	profiles := rules.ResolveProfiles(cmd.Profiles)
	composeFiles := rules.SelectComposeFiles(s.baseDir, profiles)

	resolver := paths.NewPathResolver(s.baseDir)
	netName, netSubnet, netGateway := resolver.ResolveNetworkConfig(
		cmd.NetworkName,
		cmd.NetworkSubnet,
		cmd.NetworkGateway,
	)

	if err := s.networkPort.EnsureNetwork(ctx, netName, netSubnet, netGateway); err != nil {
		return schema.StackActionOutcome{
			Status:  schema.StatusError,
			Message: fmt.Sprintf("network initialization failed (%s): %v", netName, err),
		}, fmt.Errorf("network initialization failed (%s): %w", netName, err)
	}

	isStateless := false
	for _, p := range profiles {
		if schema.IsStatelessProfile(p) {
			isStateless = true
			break
		}
	}

	if !isStateless {
		dataDir := resolver.DataDir()
		if err := s.ensureStorageDirectories(dataDir); err != nil {
			return schema.StackActionOutcome{
				Status:  schema.StatusError,
				Message: fmt.Sprintf("storage directory initialization failed: %v", err),
			}, fmt.Errorf("storage directory initialization failed: %w", err)
		}
	}

	opts := ports.ComposeOptions{
		ComposeFiles: composeFiles,
		Profiles:     profiles,
		EnvVars:      cmd.EnvVars,
		Detach:       cmd.Detach,
	}

	if err := s.containerPort.ComposeUp(ctx, opts); err != nil {
		return schema.StackActionOutcome{
			Status:  schema.StatusError,
			Message: fmt.Sprintf("compose up failed: %v", err),
		}, fmt.Errorf("compose up failed: %w", err)
	}

	s.healTemporalIfCorrupted(ctx, profiles)

	s.PrintEndpoints(profiles)

	return schema.StackActionOutcome{
		Status:         schema.StatusRunning,
		Message:        schema.MsgStackStarted,
		ActiveServices: profiles,
	}, nil
}

func (s *StackService) StopStack(ctx context.Context) (schema.StackActionOutcome, error) {
	_, endSpan := s.tracer.StartSpan(ctx, "llmobs.stack.down")
	defer endSpan()

	resolver := paths.NewPathResolver(s.baseDir)
	composeFiles := []string{resolver.ComposeFile(schema.DefaultComposeFile)}
	opts := ports.ComposeOptions{
		ComposeFiles: composeFiles,
		Profiles:     []string{schema.ProfileAll},
	}

	if err := s.containerPort.ComposeDown(ctx, opts); err != nil {
		return schema.StackActionOutcome{
			Status:  schema.StatusError,
			Message: fmt.Sprintf("compose down failed: %v", err),
		}, fmt.Errorf("compose down failed: %w", err)
	}

	return schema.StackActionOutcome{
		Status:         schema.StatusStopped,
		Message:        schema.MsgStackStopped,
		ActiveServices: []string{},
	}, nil
}

func (s *StackService) RestartStack(ctx context.Context, profiles []string) (schema.StackActionOutcome, error) {
	_, endSpan := s.tracer.StartSpan(ctx, "llmobs.stack.restart")
	defer endSpan()

	resolved := rules.ResolveProfiles(profiles)
	composeFiles := rules.SelectComposeFiles(s.baseDir, resolved)

	opts := ports.ComposeOptions{
		ComposeFiles: composeFiles,
		Profiles:     resolved,
	}

	if err := s.containerPort.ComposeRestart(ctx, opts); err != nil {
		return schema.StackActionOutcome{
			Status:  schema.StatusError,
			Message: fmt.Sprintf("compose restart failed: %v", err),
		}, fmt.Errorf("compose restart failed: %w", err)
	}

	s.healTemporalIfCorrupted(ctx, resolved)

	return schema.StackActionOutcome{
		Status:         schema.StatusRestarted,
		Message:        schema.MsgStackRestarted,
		ActiveServices: resolved,
	}, nil
}

func (s *StackService) GetStatus(ctx context.Context) ([]ports.ContainerStatus, error) {
	_, endSpan := s.tracer.StartSpan(ctx, "llmobs.stack.status")
	defer endSpan()

	resolver := paths.NewPathResolver(s.baseDir)
	composeFiles := []string{resolver.ComposeFile(schema.DefaultComposeFile)}
	opts := ports.ComposeOptions{
		ComposeFiles: composeFiles,
		Profiles:     []string{schema.ProfileAll},
	}

	return s.containerPort.ComposeStatus(ctx, opts)
}

func (s *StackService) StreamLogs(ctx context.Context, tail int) error {
	resolver := paths.NewPathResolver(s.baseDir)
	composeFiles := []string{resolver.ComposeFile(schema.DefaultComposeFile)}
	opts := ports.ComposeOptions{
		ComposeFiles: composeFiles,
		Profiles:     []string{schema.ProfileAll},
	}
	return s.containerPort.ComposeLogs(ctx, opts, tail)
}

func (s *StackService) GetActiveEndpoints(profiles []string) []schema.ServiceEndpoint {
	return schema.ResolveActiveEndpoints(profiles, s.getEnv)
}

func (s *StackService) PrintEndpoints(profiles []string) {
	endpoints := s.GetActiveEndpoints(profiles)
	fmt.Println("\n=====================================================")
	fmt.Println("  Active Services & Configurations                   ")
	fmt.Println("=====================================================")
	for _, ep := range endpoints {
		fmt.Printf("  • %-22s: %s\n", ep.Service, ep.Endpoint)
	}
	fmt.Println("=====================================================")
}

func (s *StackService) healTemporalIfCorrupted(ctx context.Context, profiles []string) {
	hasTemporal := false
	for _, p := range profiles {
		if p == schema.ProfileFull || p == schema.ProfileWorkflows || p == schema.ProfileStateless || p == schema.ProfileAll || p == schema.ProfileTemporal {
			hasTemporal = true
			break
		}
	}
	if !hasTemporal {
		return
	}

	time.Sleep(2 * time.Second)
	_ = exec.CommandContext(ctx, "docker", "exec", "llmobs-alloydb-db", "psql", "-U", "admin", "-d", "temporal_visibility", "-c", "CREATE TABLE IF NOT EXISTS executions_visibility (namespace_id CHAR(64) NOT NULL, run_id CHAR(64) NOT NULL, start_time TIMESTAMP NOT NULL, execution_time TIMESTAMP NOT NULL, workflow_id VARCHAR(255) NOT NULL, workflow_type_name VARCHAR(255) NOT NULL, status INTEGER NOT NULL, close_time TIMESTAMP NULL, history_length BIGINT, memo BYTEA, encoding VARCHAR(64) NOT NULL, task_queue VARCHAR(255) DEFAULT '' NOT NULL, PRIMARY KEY (namespace_id, run_id)); CREATE INDEX IF NOT EXISTS by_type_start_time ON executions_visibility (namespace_id, workflow_type_name, status, start_time DESC, run_id); CREATE INDEX IF NOT EXISTS by_workflow_id_start_time ON executions_visibility (namespace_id, workflow_id, status, start_time DESC, run_id); CREATE INDEX IF NOT EXISTS by_status_by_start_time ON executions_visibility (namespace_id, status, start_time DESC, run_id); CREATE INDEX IF NOT EXISTS by_type_close_time ON executions_visibility (namespace_id, workflow_type_name, status, close_time DESC, run_id); CREATE INDEX IF NOT EXISTS by_workflow_id_close_time ON executions_visibility (namespace_id, workflow_id, status, close_time DESC, run_id); CREATE INDEX IF NOT EXISTS by_status_by_close_time ON executions_visibility (namespace_id, status, close_time DESC, run_id); ALTER TABLE executions_visibility ADD COLUMN IF NOT EXISTS history_size_bytes BIGINT DEFAULT 0; ALTER TABLE executions_visibility ADD COLUMN IF NOT EXISTS execution_duration BIGINT DEFAULT 0; ALTER TABLE executions_visibility ADD COLUMN IF NOT EXISTS state_transition_count BIGINT DEFAULT 0; ALTER TABLE executions_visibility ADD COLUMN IF NOT EXISTS parent_workflow_id VARCHAR(255); ALTER TABLE executions_visibility ADD COLUMN IF NOT EXISTS parent_run_id VARCHAR(255); ALTER TABLE executions_visibility ADD COLUMN IF NOT EXISTS root_workflow_id VARCHAR(255); ALTER TABLE executions_visibility ADD COLUMN IF NOT EXISTS root_run_id VARCHAR(255);").Run()
}
