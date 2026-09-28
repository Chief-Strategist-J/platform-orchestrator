/*
Package services implements scaling orchestration for services and simulated compute nodes.

ALGORITHM BLUEPRINT:
1. ScaleService: Validates replica count, configures compose options, and executes ComposeScale via ContainerPort.
2. LaunchComputeNode: Derives node metadata, constructs port environment overrides, includes docker-compose.stateless.yml, and executes ComposeUp.
3. TerminateComputeNode: Identifies project 'llmobs-compute-{nodeId}' and executes ComposeDown.
4. ListActiveNodes: Fetches container statuses, parses project prefixes, and correlates running compute node replicas.
5. Invariants:
   - Stateless compute nodes must never mount local persistent database volumes.
   - Operations must wrap execution in OpenTelemetry spans.
*/
package services

import (
	"context"
	"fmt"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/scale/rules"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/scale/schema"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/paths"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

type ScaleService struct {
	containerPort ports.ContainerPort
	tracer        ports.TracerPort
	baseDir       string
}

func NewScaleService(containerPort ports.ContainerPort, tracer ports.TracerPort, baseDir string) *ScaleService {
	return &ScaleService{
		containerPort: containerPort,
		tracer:        tracer,
		baseDir:       baseDir,
	}
}

func (s *ScaleService) ScaleService(ctx context.Context, cmd schema.ScaleServiceCommand) error {
	_, endSpan := s.tracer.StartSpan(ctx, "llmobs.scale.service")
	defer endSpan()

	if cmd.Replicas < 1 {
		return fmt.Errorf("replicas must be greater than or equal to 1")
	}

	resolver := paths.NewPathResolver(s.baseDir)
	opts := ports.ComposeOptions{
		ComposeFiles: []string{resolver.ComposeFile("docker-compose.yml")},
		EnvVars: map[string]string{
			"CONTAINER_PREFIX": "",
		},
		Detach: true,
	}

	return s.containerPort.ComposeScale(ctx, opts, cmd.Service, cmd.Replicas)
}

func (s *ScaleService) LaunchComputeNode(ctx context.Context, cmd schema.LaunchNodeCommand) (schema.NodeMetadata, error) {
	_, endSpan := s.tracer.StartSpan(ctx, "llmobs.scale.launch_node")
	defer endSpan()

	if cmd.NodeID < 2 {
		return schema.NodeMetadata{}, fmt.Errorf("nodeId must be >= 2 for simulated compute nodes")
	}

	meta := rules.DeriveNodeMetadata(cmd)
	envVars := rules.ConvertPortsToEnv(meta)

	resolver := paths.NewPathResolver(s.baseDir)
	composeFiles := []string{
		resolver.ComposeFile("docker-compose.yml"),
		resolver.ComposeFile("docker-compose.stateless.yml"),
	}

	profiles := []string{"stateless"}
	if cmd.EnableCloudflare {
		cfFile := resolver.ComposeFile("docker-compose.cloudflare.yml")
		composeFiles = append(composeFiles, cfFile)
		profiles = append(profiles, "cloudflare")
	}

	opts := ports.ComposeOptions{
		ComposeFiles: composeFiles,
		Profiles:     profiles,
		ProjectName:  meta.ProjectName,
		EnvVars:      envVars,
		Detach:       true,
	}

	if err := s.containerPort.ComposeUp(ctx, opts); err != nil {
		return schema.NodeMetadata{}, fmt.Errorf("failed to launch compute node %d: %w", cmd.NodeID, err)
	}

	return meta, nil
}

func (s *ScaleService) TerminateComputeNode(ctx context.Context, nodeID int) error {
	_, endSpan := s.tracer.StartSpan(ctx, "llmobs.scale.terminate_node")
	defer endSpan()

	projectName := fmt.Sprintf("llmobs-compute-%d", nodeID)
	resolver := paths.NewPathResolver(s.baseDir)
	opts := ports.ComposeOptions{
		ComposeFiles: []string{resolver.ComposeFile("docker-compose.yml")},
		Profiles:     []string{"*"},
		ProjectName:  projectName,
	}

	return s.containerPort.ComposeDown(ctx, opts)
}

func (s *ScaleService) ListActiveContainers(ctx context.Context) ([]ports.ContainerStatus, error) {
	_, endSpan := s.tracer.StartSpan(ctx, "llmobs.scale.list_containers")
	defer endSpan()

	resolver := paths.NewPathResolver(s.baseDir)
	opts := ports.ComposeOptions{
		ComposeFiles: []string{resolver.ComposeFile("docker-compose.yml")},
		Profiles:     []string{"*"},
	}

	return s.containerPort.ComposeStatus(ctx, opts)
}
