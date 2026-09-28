/*
Package unit provides regression and unit test verification for config service.

ALGORITHM BLUEPRINT:
1. TestGetPlatformConfig: Verifies resource defaults and active env file discovery.
2. TestUpdatePlatformConfig: Verifies writing custom resource allocations to .env file.
3. Invariants:
   - Zero inline comments inside function bodies.
*/
package unit

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	configSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/config/schema"
	configService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/config/services"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/infra/observability"
)

func TestGetPlatformConfig(t *testing.T) {
	tempDir := t.TempDir()
	pkgDir := filepath.Join(tempDir, "packages", "platform-orchestrator")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatalf("failed to create temp pkg dir: %v", err)
	}

	envFile := filepath.Join(pkgDir, ".env")
	envContent := "ALLOYDB_MEM_LIMIT=5120M\nTEMPORAL_MEM_LIMIT=3072M\n"
	if err := os.WriteFile(envFile, []byte(envContent), 0644); err != nil {
		t.Fatalf("failed to write test env: %v", err)
	}

	tracer := observability.NewOTelTracerAdapter("test-config")
	svc := configService.NewConfigService(tempDir, tracer)

	ctx := context.Background()
	report, err := svc.GetPlatformConfig(ctx)
	if err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}

	if report.Resources.AlloyDB.MemoryLimit != "5120M" {
		t.Errorf("expected 5120M AlloyDB mem limit, got: %s", report.Resources.AlloyDB.MemoryLimit)
	}
	if report.Resources.Temporal.MemoryLimit != "3072M" {
		t.Errorf("expected 3072M Temporal mem limit, got: %s", report.Resources.Temporal.MemoryLimit)
	}
	if report.Resources.ClickHouse.MemoryLimit != "4096M" {
		t.Errorf("expected fallback default 4096M ClickHouse mem limit, got: %s", report.Resources.ClickHouse.MemoryLimit)
	}
}

func TestUpdatePlatformConfig(t *testing.T) {
	tempDir, err := os.MkdirTemp(".", ".test_cfg_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	absTempDir, err := filepath.Abs(tempDir)
	if err != nil {
		t.Fatalf("failed to get abs path: %v", err)
	}

	pkgDir := filepath.Join(absTempDir, "packages", "platform-orchestrator")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatalf("failed to create temp pkg dir: %v", err)
	}

	envFile := filepath.Join(pkgDir, ".env")
	initialContent := "ALLOYDB_MEM_LIMIT=3072M\n"
	if err := os.WriteFile(envFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("failed to write initial env: %v", err)
	}

	composeFile := filepath.Join(pkgDir, "docker-compose.yml")
	composeContent := "services:\n  test:\n    image: alpine:latest\n"
	if err := os.WriteFile(composeFile, []byte(composeContent), 0644); err != nil {
		t.Fatalf("failed to write compose file: %v", err)
	}

	tracer := observability.NewOTelTracerAdapter("test-config")
	svc := configService.NewConfigService(absTempDir, tracer)

	ctx := context.Background()
	cmd := configSchema.UpdateConfigCommand{
		AlloyDBMemory:    "6144M",
		TemporalMemory:   "4096M",
		ClickHouseMemory: "8192M",
	}

	report, err := svc.UpdatePlatformConfig(ctx, cmd)
	if err != nil {
		t.Fatalf("failed to update config: %v", err)
	}

	if report.Resources.AlloyDB.MemoryLimit != "6144M" {
		t.Errorf("expected 6144M, got: %s", report.Resources.AlloyDB.MemoryLimit)
	}
	if report.Resources.Temporal.MemoryLimit != "4096M" {
		t.Errorf("expected 4096M, got: %s", report.Resources.Temporal.MemoryLimit)
	}
	if report.Resources.ClickHouse.MemoryLimit != "8192M" {
		t.Errorf("expected 8192M, got: %s", report.Resources.ClickHouse.MemoryLimit)
	}

	savedContent, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("failed to read updated env: %v", err)
	}
	if string(savedContent) == "" {
		t.Errorf("env file was empty")
	}
}
