package unit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/paths"
)

func TestPathResolverDynamicDiscovery(t *testing.T) {
	tempDir := t.TempDir()

	resolver := paths.NewPathResolver(tempDir)
	if resolver.BaseDir() != tempDir {
		t.Fatalf("expected BaseDir %s, got %s", tempDir, resolver.BaseDir())
	}

	// 1. Initially without markers, resolves to fallback
	fallback := resolver.ConfigDir()
	expectedFallback := filepath.Join(tempDir, "config")
	if fallback != expectedFallback {
		t.Fatalf("expected fallback %s, got %s", expectedFallback, fallback)
	}

	// 2. Create nested package config with markers
	pkgConfigDir := filepath.Join(tempDir, "packages", "custom-service-orchestrator", "config")
	if err := os.MkdirAll(pkgConfigDir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	markerFile := filepath.Join(pkgConfigDir, "env.schema")
	if err := os.WriteFile(markerFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to write marker: %v", err)
	}

	// Create new resolver instance to clear cache
	resolver2 := paths.NewPathResolver(tempDir)
	discovered := resolver2.ConfigDir()
	if discovered != pkgConfigDir {
		t.Fatalf("expected dynamically discovered %s, got %s", pkgConfigDir, discovered)
	}

	// 3. Dynamic service creation and discovery
	customSvcDir := filepath.Join(pkgConfigDir, "custom-vector-db")
	if err := os.MkdirAll(customSvcDir, 0755); err != nil {
		t.Fatalf("failed to create service dir: %v", err)
	}
	customConf := filepath.Join(customSvcDir, "vector.yaml")
	if err := os.WriteFile(customConf, []byte("port: 8080"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	services, err := resolver2.ListServices()
	if err != nil {
		t.Fatalf("ListServices failed: %v", err)
	}
	foundCustom := false
	for _, s := range services {
		if s == "custom-vector-db" {
			foundCustom = true
			break
		}
	}
	if !foundCustom {
		t.Fatalf("expected to dynamically discover custom-vector-db in %v", services)
	}

	if !resolver2.HasService("custom-vector-db") {
		t.Fatalf("expected HasService(custom-vector-db) to be true")
	}

	// 4. Recursive FindFile
	foundFile, err := resolver2.FindFile("vector.yaml")
	if err != nil || foundFile != customConf {
		t.Fatalf("expected to find vector.yaml at %s, got %s (err: %v)", customConf, foundFile, err)
	}

	// 5. Dynamic cert discovery
	certDir := resolver2.CertDir()
	if err := os.MkdirAll(certDir, 0755); err != nil {
		t.Fatalf("failed to create cert dir: %v", err)
	}
	dynCert := filepath.Join(certDir, "my-custom.crt")
	if err := os.WriteFile(dynCert, []byte("cert"), 0644); err != nil {
		t.Fatalf("failed to write cert: %v", err)
	}

	foundCert, err := resolver2.FindExistingCert("nonexistent.crt")
	if err != nil || foundCert != dynCert {
		t.Fatalf("expected to dynamically discover %s via fallback, got %s (err: %v)", dynCert, foundCert, err)
	}
}

func TestDiscoverWorkspaceRoot(t *testing.T) {
	root := paths.DiscoverWorkspaceRoot()
	if root == "" || root == "." {
		t.Fatalf("expected valid non-empty workspace root, got %s", root)
	}
}

func TestPathResolverYAMLAndNetworkConfig(t *testing.T) {
	tempDir := t.TempDir()
	cfgDir := filepath.Join(tempDir, "config")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	yamlContent := `
docker:
  network: "custom-net"
  subnet: "192.168.10.0/24"
  gateway: "192.168.10.1"
paths:
  markers:
    - "custom.marker"
`
	if err := os.WriteFile(filepath.Join(cfgDir, "default.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write yaml: %v", err)
	}

	resolver := paths.NewPathResolver(tempDir)

	// Check YAML-loaded network config
	name, subnet, gateway := resolver.ResolveNetworkConfig("", "", "")
	if name != "custom-net" || subnet != "192.168.10.0/24" || gateway != "192.168.10.1" {
		t.Fatalf("expected custom-net from YAML, got name=%s subnet=%s gateway=%s", name, subnet, gateway)
	}

	// Check command/API override takes precedence
	overrideName, overrideSubnet, overrideGateway := resolver.ResolveNetworkConfig("api-net", "10.0.0.0/16", "10.0.0.1")
	if overrideName != "api-net" || overrideSubnet != "10.0.0.0/16" || overrideGateway != "10.0.0.1" {
		t.Fatalf("expected api-net override, got name=%s subnet=%s gateway=%s", overrideName, overrideSubnet, overrideGateway)
	}

	// Check YAML-loaded markers
	markers := resolver.GetMarkers()
	if len(markers) != 1 || markers[0] != "custom.marker" {
		t.Fatalf("expected custom.marker from YAML, got %v", markers)
	}
}

