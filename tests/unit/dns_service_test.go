/*
Package unit provides unit test coverage for DNS domain discovery, host file synchronization, and resolution probes.
*/
package unit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dnsSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/dns/schema"
	dnsService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/dns/services"
	dnsTypes "github.com/Chief-Strategist-J/platform-orchestrator/src/features/dns/types"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/infra/observability"
)

func TestDNSHostsSyncAndRead(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "dns-hosts-test-*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	hostsPath := filepath.Join(tempDir, "hosts")
	initialHosts := "127.0.0.1 localhost\n::1 localhost ip6-localhost ip6-loopback\n"
	if err := os.WriteFile(hostsPath, []byte(initialHosts), 0644); err != nil {
		t.Fatalf("failed writing initial hosts: %v", err)
	}

	tracer := observability.NewOTelTracerAdapter("test-tracer")
	hostsSvc := dnsService.NewDNSHostsService(tracer)

	report, err := hostsSvc.SyncHosts(context.Background(), []string{"grafana.internal.local", "traefik.internal.local"}, dnsTypes.SyncOptions{
		TargetIP:      "127.0.0.1",
		HostsFilePath: hostsPath,
	})
	if err != nil {
		t.Fatalf("failed syncing hosts: %v", err)
	}
	if !report.Success {
		t.Fatalf("expected success sync report, got false")
	}
	if report.SyncedCount != 2 {
		t.Fatalf("expected 2 synced domains, got %d", report.SyncedCount)
	}

	syncedMap, err := hostsSvc.ReadSyncedDomains(hostsPath)
	if err != nil {
		t.Fatalf("failed reading synced domains: %v", err)
	}
	if syncedMap["grafana.internal.local"] != "127.0.0.1" {
		t.Fatalf("expected grafana.internal.local mapped to 127.0.0.1, got %q", syncedMap["grafana.internal.local"])
	}
	if syncedMap["traefik.internal.local"] != "127.0.0.1" {
		t.Fatalf("expected traefik.internal.local mapped to 127.0.0.1, got %q", syncedMap["traefik.internal.local"])
	}

	content, err := os.ReadFile(hostsPath)
	if err != nil {
		t.Fatalf("failed reading hosts file: %v", err)
	}
	if !strings.Contains(string(content), "::1 localhost ip6-localhost ip6-loopback") {
		t.Fatalf("expected original hosts content to be preserved")
	}
}

func TestDNSDiscoveryFromDynamicTraefik(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "dns-discovery-test-*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configDir := filepath.Join(tempDir, "config", "traefik")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed creating config dir: %v", err)
	}

	dynYAML := `
http:
  routers:
    custom-router:
      rule: "Host(` + "`" + `custom.obs.internal` + "`" + `)"
      service: "custom-svc"
`
	if err := os.WriteFile(filepath.Join(configDir, "dynamic.yml"), []byte(dynYAML), 0644); err != nil {
		t.Fatalf("failed writing dynamic.yml: %v", err)
	}

	tracer := observability.NewOTelTracerAdapter("test-tracer")
	dnsSvc := dnsService.NewDNSService(tracer, tempDir)

	discovered := dnsSvc.DiscoverDomains(context.Background())
	found := false
	for _, d := range discovered {
		if d == "custom.obs.internal" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected custom.obs.internal to be discovered from dynamic config")
	}
}

func TestDNSProbeLocalhost(t *testing.T) {
	tracer := observability.NewOTelTracerAdapter("test-tracer")
	probeSvc := dnsService.NewDNSProbeService(tracer)

	res := probeSvc.CheckDomain(context.Background(), "localhost")
	if !res.IsResolvable {
		t.Fatalf("expected localhost to resolve, got error: %s", res.Error)
	}
}

func TestDNSServiceRecordsList(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "dns-records-test-*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	hostsPath := filepath.Join(tempDir, "hosts")
	initialHosts := "127.0.0.1 localhost\n# --- BEGIN LLMOBS PLATFORM DOMAINS ---\n127.0.0.1 grafana.internal.local\n# --- END LLMOBS PLATFORM DOMAINS ---\n"
	if err := os.WriteFile(hostsPath, []byte(initialHosts), 0644); err != nil {
		t.Fatalf("failed writing initial hosts: %v", err)
	}

	tracer := observability.NewOTelTracerAdapter("test-tracer")
	dnsSvc := dnsService.NewDNSService(tracer, tempDir)

	records, err := dnsSvc.ListRecords(context.Background(), hostsPath)
	if err != nil {
		t.Fatalf("failed listing dns records: %v", err)
	}
	if len(records) == 0 {
		t.Fatalf("expected records, got 0")
	}

	var grafanaRecord *dnsSchema.DnsRecord
	for i := range records {
		if records[i].Domain == "grafana.internal.local" {
			grafanaRecord = &records[i]
			break
		}
	}
	if grafanaRecord == nil {
		t.Fatalf("expected grafana.internal.local record")
	}
	if !grafanaRecord.IsSynced {
		t.Fatalf("expected grafana.internal.local to be marked IsSynced=true")
	}
}
