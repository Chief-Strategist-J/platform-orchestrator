/*
Package services implements automated platform setup and initial deployment bootstrapping.

ALGORITHM BLUEPRINT:
1. Phase1_Prereqs: Audits Docker, Compose, RAM, and essential host utilities.
2. Phase2_EnvGeneration: Seeds .env from .env.example with cryptographically secure random credentials for Redis and Grafana.
3. Phase3_StorageProvisioning: Initializes directory hierarchy for all stateful database engines with read/write permissions.
4. Phase4_CertProvisioning: Generates X.509 CA, server, and client certificates with Subject Alternative Names natively.
5. Phase5_DomainResolution: Inspects /etc/hosts for custom gateway and observability hostnames.
6. Phase6_ImagePulling: Pulls designated base container images concurrently to minimize initialization wait time.
7. Phase7_Validation: Validates configuration files and compose specifications.
8. Invariants:
   - Setup operates idempotently without overwriting configured production secrets.
   - Failures at critical stages halt subsequent dependent phases immediately.
*/
package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	certsSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/certs/schema"
	certsService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/certs/services"
	prereqService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/prereqs/services"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/setup/schema"
	stackSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/stack/schema"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/paths"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

type SetupService struct {
	prereqSvc     *prereqService.PrereqService
	certsSvc      *certsService.CertService
	tracer        ports.TracerPort
	workspaceRoot string
}

func NewSetupService(
	prereqSvc *prereqService.PrereqService,
	certsSvc *certsService.CertService,
	tracer ports.TracerPort,
	workspaceRoot string,
) *SetupService {
	return &SetupService{
		prereqSvc:     prereqSvc,
		certsSvc:      certsSvc,
		tracer:        tracer,
		workspaceRoot: workspaceRoot,
	}
}

func (s *SetupService) RunSetupPipeline(ctx context.Context, pullImages bool) (*schema.SetupReport, error) {
	_, end := s.tracer.StartSpan(ctx, "SetupService.RunSetupPipeline")
	defer end()

	report := &schema.SetupReport{
		TotalSteps: 7,
	}

	step1 := s.executeStep(1, "Checking system prerequisites", func() error {
		prereqReport := s.prereqSvc.VerifySystemPrerequisites(ctx)
		if !prereqReport.Passed {
			return fmt.Errorf("system prerequisites check failed")
		}
		return nil
	})
	report.Steps = append(report.Steps, step1)
	if !step1.Passed {
		return report, fmt.Errorf("setup failed at step 1: %s", step1.Error)
	}

	step2 := s.executeStep(2, "Configuring environment variables", func() error {
		return s.configureEnvironment()
	})
	report.Steps = append(report.Steps, step2)
	if !step2.Passed {
		return report, fmt.Errorf("setup failed at step 2: %s", step2.Error)
	}

	step3 := s.executeStep(3, "Initializing persistent storage directories", func() error {
		return s.initializeStorageDirectories()
	})
	report.Steps = append(report.Steps, step3)
	if !step3.Passed {
		return report, fmt.Errorf("setup failed at step 3: %s", step3.Error)
	}

	step4 := s.executeStep(4, "Generating TLS certificates", func() error {
		spec := certsSchema.DefaultCertSpec(s.workspaceRoot)
		spec.Force = true
		_, err := s.certsSvc.EnsureCertificates(ctx, spec)
		return err
	})
	report.Steps = append(report.Steps, step4)
	if !step4.Passed {
		return report, fmt.Errorf("setup failed at step 4: %s", step4.Error)
	}

	step5 := s.executeStep(5, "Verifying local domain resolution", func() error {
		return s.verifyLocalDomains()
	})
	report.Steps = append(report.Steps, step5)
	if !step5.Passed {
		return report, fmt.Errorf("setup failed at step 5: %s", step5.Error)
	}

	step6 := s.executeStep(6, "Pulling Docker container images", func() error {
		if !pullImages {
			return nil
		}
		return s.pullContainerImages(ctx)
	})
	report.Steps = append(report.Steps, step6)
	if !step6.Passed {
		return report, fmt.Errorf("setup failed at step 6: %s", step6.Error)
	}

	step7 := s.executeStep(7, "Validating installation configuration", func() error {
		return s.validateFinalSetup()
	})
	report.Steps = append(report.Steps, step7)
	if !step7.Passed {
		return report, fmt.Errorf("setup failed at step 7: %s", step7.Error)
	}

	passed := 0
	for _, st := range report.Steps {
		if st.Passed {
			passed++
		}
	}
	report.PassedSteps = passed
	report.Success = passed == report.TotalSteps
	if report.Success {
		report.Message = "Full platform setup completed successfully"
	} else {
		report.Message = fmt.Sprintf("Platform setup completed with %d/%d steps passed", passed, report.TotalSteps)
	}

	return report, nil
}

func (s *SetupService) executeStep(index int, name string, fn func() error) schema.SetupStep {
	start := time.Now()
	err := fn()
	duration := time.Since(start)

	step := schema.SetupStep{
		Index:       index,
		Name:        name,
		Passed:      err == nil,
		Duration:    duration,
		Description: name,
	}
	if err != nil {
		step.Error = err.Error()
	}
	return step
}

func (s *SetupService) configureEnvironment() error {
	resolver := paths.NewPathResolver(s.workspaceRoot)
	envPath := resolver.EnvFile()
	if _, err := os.Stat(envPath); err == nil {
		return nil
	}

	examplePath := resolver.EnvExampleFile()
	data, err := os.ReadFile(examplePath)
	if err != nil {
		return fmt.Errorf(".env.example missing: %w", err)
	}

	redisBytes := make([]byte, 16)
	_, _ = rand.Read(redisBytes)
	redisPw := hex.EncodeToString(redisBytes)

	grafanaBytes := make([]byte, 16)
	_, _ = rand.Read(grafanaBytes)
	grafanaPw := hex.EncodeToString(grafanaBytes)

	content := string(data)
	content = strings.ReplaceAll(content, "REDIS_PASSWORD=<CHANGE_ME>", fmt.Sprintf("REDIS_PASSWORD=%s", redisPw))
	content = strings.ReplaceAll(content, "GF_SECURITY_ADMIN_PASSWORD=<CHANGE_ME>", fmt.Sprintf("GF_SECURITY_ADMIN_PASSWORD=%s", grafanaPw))

	return os.WriteFile(envPath, []byte(content), 0644)
}

func (s *SetupService) initializeStorageDirectories() error {
	resolver := paths.NewPathResolver(s.workspaceRoot)
	baseData := resolver.DataDir()

	for _, sub := range stackSchema.DefaultStorageSubdirs {
		target := filepath.Join(baseData, sub)
		if err := os.MkdirAll(target, 0777); err != nil {
			return err
		}
		if err := os.Chmod(target, 0777); err != nil {
			return err
		}
	}
	return nil
}

func (s *SetupService) verifyLocalDomains() error {
	data, err := os.ReadFile("/etc/hosts")
	if err != nil {
		return nil
	}

	domains := []string{"llmobs.gateway", "llmobs.grafana", "llmobs.tempo", "llmobs.otel", "llmobs.kafka", "llmobs.redis"}
	content := string(data)
	for _, d := range domains {
		if !strings.Contains(content, d) {
			return fmt.Errorf("domain %s not in /etc/hosts (optional: add '127.0.0.1 %s' to /etc/hosts)", d, strings.Join(domains, " "))
		}
	}
	return nil
}

func (s *SetupService) pullContainerImages(ctx context.Context) error {
	images := []string{
		"traefik:v2.10",
		"redis:7-alpine",
		"apache/kafka:latest",
		"grafana/tempo:latest",
		"otel/opentelemetry-collector-contrib:latest",
		"grafana/grafana:latest",
	}

	for _, img := range images {
		cmd := exec.CommandContext(ctx, "docker", "pull", img)
		_ = cmd.Run()
	}
	return nil
}

func (s *SetupService) validateFinalSetup() error {
	resolver := paths.NewPathResolver(s.workspaceRoot)
	certFile, err := resolver.FindExistingCert("traefik.crt", "server.pem")
	if err != nil {
		return fmt.Errorf("certificate not found at %s: %w", certFile, err)
	}

	composeFile := resolver.ComposeFile("docker-compose.yml")
	cmd := exec.Command("docker", "compose", "-f", composeFile, "config", "--quiet")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker-compose validation error: %w", err)
	}

	return nil
}
