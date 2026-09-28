/*
Package unit implements unit verification tests for setup service credentials and configurations.

ALGORITHM BLUEPRINT:
1. TestPromptCredentialsInteractively: Validates prompt reader keeps defaults when empty and accepts custom overrides.
2. Invariants:
   - Zero inline comments inside function bodies.
   - Non-interactive and interactive modes behave deterministically.
*/
package unit

import (
	"bufio"
	"strings"
	"testing"

	prereqsService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/prereqs/services"
	setupService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/setup/services"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/infra/observability"
)

func TestPromptCredentialsInteractively(t *testing.T) {
	tempDir := t.TempDir()
	tracer := observability.NewOTelTracerAdapter("test-tracer")
	prereqSvc := prereqsService.NewPrereqService(tracer)
	svc := setupService.NewSetupService(prereqSvc, nil, tracer, tempDir)

	input := "my_custom_db_password\n\n\n\n"
	reader := bufio.NewReader(strings.NewReader(input))

	creds := svc.PromptCredentialsInteractively(reader)

	if creds["ALLOYDB_PASSWORD"] != "my_custom_db_password" {
		t.Fatalf("expected custom ALLOYDB_PASSWORD 'my_custom_db_password', got '%s'", creds["ALLOYDB_PASSWORD"])
	}

	if creds["REDIS_PASSWORD"] != "llmobs_redis_s3cret_2024" {
		t.Fatalf("expected default REDIS_PASSWORD, got '%s'", creds["REDIS_PASSWORD"])
	}

	if creds["GF_SECURITY_ADMIN_PASSWORD"] != "llmobs_admin_password" {
		t.Fatalf("expected default GF_SECURITY_ADMIN_PASSWORD, got '%s'", creds["GF_SECURITY_ADMIN_PASSWORD"])
	}

	if creds["CLICKHOUSE_PASSWORD"] != "llmobs_clickhouse_s3cret_2026" {
		t.Fatalf("expected default CLICKHOUSE_PASSWORD, got '%s'", creds["CLICKHOUSE_PASSWORD"])
	}
}
