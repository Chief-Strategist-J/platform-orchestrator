/*
Package paths provides centralized, dynamic path resolution for workspace roots, configuration directories, certificates, and infrastructure compose definitions.

ALGORITHM BLUEPRINT:
1. PathResolver: Dynamically discovers workspace root, config hierarchy, and service definitions using marker-based detection.
2. Dynamic Discovery Hierarchy for ConfigDir:
   - Priority 1: Environment variable LLMOBS_CONFIG_DIR override.
   - Priority 2: Direct baseDir/config directory with marker verification.
   - Priority 3: Dynamic glob scan across packages/* /config for marker matches.
   - Priority 4: Upward directory walk from baseDir to detect enclosing config directories.
3. YAML-Driven Configuration:
   - Loads marker definitions dynamically from default.yaml under paths.markers.
   - Exposes dynamic network parameters (name, subnet, gateway) configurable via command, API, environment, or YAML.
4. Fail-Fast & Dynamic Discovery:
   - ListServices: Scans ConfigDir and returns all active service configuration names.
   - HasService: Checks for presence of a specific service's configuration directory.
   - FindFile: Recursively searches ConfigDir for a specific file by name without hardcoded paths.
   - FindExistingCert: Scans CertDir for specified candidate names with automatic .crt/.pem extension fallback.
5. Invariants:
   - Zero inline comments inside function bodies.
   - Thread-safe resolution with in-memory caching.
   - Fail-fast error propagation.
*/
package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

var defaultMarkers = []string{"env.schema", "default.yaml", "certs", "traefik", "alloydb"}

type ServiceDefinition struct {
	URL             string `yaml:"url" json:"url"`
	Host            string `yaml:"host" json:"host"`
	Port            int    `yaml:"port" json:"port"`
	HTTPPort        int    `yaml:"httpPort" json:"httpPort"`
	TCPPort         int    `yaml:"tcpPort" json:"tcpPort"`
	GRPCPort        int    `yaml:"grpcPort" json:"grpcPort"`
	User            string `yaml:"user" json:"user"`
	Database        string `yaml:"database" json:"database"`
	EnvURL          string `yaml:"envUrl" json:"envUrl"`
	EnvHost         string `yaml:"envHost" json:"envHost"`
	EnvPort         string `yaml:"envPort" json:"envPort"`
	EnvUser         string `yaml:"envUser" json:"envUser"`
	EnvPassword     string `yaml:"envPassword" json:"envPassword"`
	EnvDatabase     string `yaml:"envDatabase" json:"envDatabase"`
	DefaultPassword string `yaml:"defaultPassword" json:"defaultPassword"`
}

type SetupCredentialConfig struct {
	Prompt  string `yaml:"prompt" json:"prompt"`
	EnvKey  string `yaml:"envKey" json:"envKey"`
	Default string `yaml:"default" json:"default"`
	User    string `yaml:"user,omitempty" json:"user,omitempty"`
	DBName  string `yaml:"dbName,omitempty" json:"dbName,omitempty"`
}

type SetupYAMLConfig struct {
	Images       []string                         `yaml:"images" json:"images"`
	Certificates []string                         `yaml:"certificates" json:"certificates"`
	ComposeFile  string                           `yaml:"composeFile" json:"composeFile"`
	Domains      []string                         `yaml:"domains" json:"domains"`
	Credentials  map[string]SetupCredentialConfig `yaml:"credentials" json:"credentials"`
}

type DnsMarkersConfig struct {
	Start string `yaml:"start" json:"start"`
	End   string `yaml:"end" json:"end"`
}

type DnsYAMLConfig struct {
	DefaultIP string           `yaml:"defaultIp" json:"defaultIp"`
	HostsPath string           `yaml:"hostsPath" json:"hostsPath"`
	Markers   DnsMarkersConfig `yaml:"markers" json:"markers"`
	Domains   []string         `yaml:"domains" json:"domains"`
}

type TraefikAPIEndpointsConfig struct {
	Ping            string `yaml:"ping" json:"ping"`
	Overview        string `yaml:"overview" json:"overview"`
	RawData         string `yaml:"rawdata" json:"rawdata"`
	Entrypoints     string `yaml:"entrypoints" json:"entrypoints"`
	HTTPRouters     string `yaml:"httpRouters" json:"httpRouters"`
	HTTPServices    string `yaml:"httpServices" json:"httpServices"`
	HTTPMiddlewares string `yaml:"httpMiddlewares" json:"httpMiddlewares"`
	TCPRouters      string `yaml:"tcpRouters" json:"tcpRouters"`
	TCPServices     string `yaml:"tcpServices" json:"tcpServices"`
	TCPMiddlewares  string `yaml:"tcpMiddlewares" json:"tcpMiddlewares"`
	UDPRouters      string `yaml:"udpRouters" json:"udpRouters"`
	UDPServices     string `yaml:"udpServices" json:"udpServices"`
}

type TraefikYAMLConfig struct {
	API struct {
		DefaultURL string                    `yaml:"defaultUrl" json:"defaultUrl"`
		Timeout    string                    `yaml:"timeout" json:"timeout"`
		Endpoints  TraefikAPIEndpointsConfig `yaml:"endpoints" json:"endpoints"`
	} `yaml:"api" json:"api"`
}

type YAMLConfig struct {
	Server struct {
		Port         int    `yaml:"port"`
		ReadTimeout  string `yaml:"readTimeout"`
		WriteTimeout string `yaml:"writeTimeout"`
	} `yaml:"server"`
	Docker struct {
		Network        string `yaml:"network"`
		Subnet         string `yaml:"subnet"`
		Gateway        string `yaml:"gateway"`
		ComposeFile    string `yaml:"composeFile"`
		StatelessFile  string `yaml:"statelessFile"`
		ProdFile       string `yaml:"prodFile"`
		CloudflareFile string `yaml:"cloudflareFile"`
	} `yaml:"docker"`
	Orchestrator struct {
		DefaultProfiles []string `yaml:"defaultProfiles"`
		DataDir         string   `yaml:"dataDir"`
		PrimaryDataHost string   `yaml:"primaryDataHost"`
	} `yaml:"orchestrator"`
	Paths struct {
		Markers []string `yaml:"markers"`
	} `yaml:"paths"`
	DNS      DnsYAMLConfig                `yaml:"dns"`
	Traefik  TraefikYAMLConfig            `yaml:"traefik"`
	Services map[string]ServiceDefinition `yaml:"services"`
	Setup    SetupYAMLConfig              `yaml:"setup"`
}

type PathResolver struct {
	baseDir      string
	configDirMu  sync.RWMutex
	cachedCfg    string
	markersMu    sync.RWMutex
	markers      []string
	parsedConfig *YAMLConfig
}

func NewPathResolver(baseDir string) *PathResolver {
	if baseDir == "" {
		baseDir = DiscoverWorkspaceRoot()
	}
	r := &PathResolver{
		baseDir: baseDir,
		markers: append([]string(nil), defaultMarkers...),
	}
	r.loadYAMLConfig()
	return r
}

func (r *PathResolver) loadYAMLConfig() {
	cfgDir := r.ConfigDir()
	yamlPath := filepath.Join(cfgDir, "default.yaml")
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return
	}

	var parsed YAMLConfig
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return
	}

	r.parsedConfig = &parsed
	if len(parsed.Paths.Markers) > 0 {
		r.markersMu.Lock()
		r.markers = parsed.Paths.Markers
		r.markersMu.Unlock()
	}
}

func DiscoverWorkspaceRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	curr := dir
	for {
		if _, err := os.Stat(filepath.Join(curr, "go.work")); err == nil {
			return curr
		}
		if fi, err := os.Stat(filepath.Join(curr, ".git")); err == nil && fi.IsDir() {
			return curr
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}
	curr = dir
	for {
		if _, err := os.Stat(filepath.Join(curr, "docker-compose.yml")); err == nil {
			return curr
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}
	return "."
}

func (r *PathResolver) isConfigDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}

	r.markersMu.RLock()
	currentMarkers := r.markers
	r.markersMu.RUnlock()

	for _, marker := range currentMarkers {
		if _, err := os.Stat(filepath.Join(path, marker)); err == nil {
			return true
		}
	}
	return false
}

func (r *PathResolver) BaseDir() string {
	return r.baseDir
}

func (r *PathResolver) GetMarkers() []string {
	r.markersMu.RLock()
	defer r.markersMu.RUnlock()
	cp := make([]string, len(r.markers))
	copy(cp, r.markers)
	return cp
}

func (r *PathResolver) SetMarkers(markers []string) {
	r.markersMu.Lock()
	defer r.markersMu.Unlock()
	r.markers = append([]string(nil), markers...)
}

func (r *PathResolver) ConfigDir() string {
	r.configDirMu.RLock()
	if r.cachedCfg != "" {
		defer r.configDirMu.RUnlock()
		return r.cachedCfg
	}
	r.configDirMu.RUnlock()

	r.configDirMu.Lock()
	defer r.configDirMu.Unlock()

	if r.cachedCfg != "" {
		return r.cachedCfg
	}

	if env := os.Getenv("LLMOBS_CONFIG_DIR"); env != "" && r.isConfigDir(env) {
		r.cachedCfg = env
		return r.cachedCfg
	}

	direct := filepath.Join(r.baseDir, "config")
	if r.isConfigDir(direct) {
		r.cachedCfg = direct
		return r.cachedCfg
	}

	pattern := filepath.Join(r.baseDir, "packages", "*", "config")
	if matches, err := filepath.Glob(pattern); err == nil {
		for _, m := range matches {
			if r.isConfigDir(m) {
				r.cachedCfg = m
				return r.cachedCfg
			}
		}
	}

	curr := r.baseDir
	for {
		cand := filepath.Join(curr, "config")
		if r.isConfigDir(cand) {
			r.cachedCfg = cand
			return r.cachedCfg
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}

	r.cachedCfg = direct
	return r.cachedCfg
}

func (r *PathResolver) ConfigSubdir(service string) string {
	return filepath.Join(r.ConfigDir(), service)
}

func (r *PathResolver) ConfigFile(service string, filename string) string {
	return filepath.Join(r.ConfigDir(), service, filename)
}

func (r *PathResolver) ListServices() ([]string, error) {
	cfg := r.ConfigDir()
	entries, err := os.ReadDir(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to read config directory %s: %w", cfg, err)
	}

	var services []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && e.Name() != "certs" {
			services = append(services, e.Name())
		}
	}
	return services, nil
}

func (r *PathResolver) HasService(service string) bool {
	info, err := os.Stat(r.ConfigSubdir(service))
	return err == nil && info.IsDir()
}

func (r *PathResolver) FindFile(filename string) (string, error) {
	cfg := r.ConfigDir()
	var found string
	err := filepath.Walk(cfg, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && info.Name() == filename {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil && err != filepath.SkipAll {
		return "", err
	}
	if found != "" {
		return found, nil
	}
	return "", fmt.Errorf("file %q not found in config tree %s", filename, cfg)
}

func (r *PathResolver) CertDir() string {
	return r.ConfigSubdir("certs")
}

func (r *PathResolver) CertFile(filename string) string {
	return filepath.Join(r.CertDir(), filename)
}

func (r *PathResolver) FindExistingCert(names ...string) (string, error) {
	certDir := r.CertDir()
	for _, n := range names {
		p := filepath.Join(certDir, n)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	if entries, err := os.ReadDir(certDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && (strings.HasSuffix(e.Name(), ".crt") || strings.HasSuffix(e.Name(), ".pem")) {
				return filepath.Join(certDir, e.Name()), nil
			}
		}
	}

	fallback := ""
	if len(names) > 0 {
		fallback = filepath.Join(certDir, names[0])
	}
	return fallback, fmt.Errorf("certificate not found in %s (candidates: %v)", certDir, names)
}

func (r *PathResolver) ComposeFile(filename string) string {
	cfgDir := r.ConfigDir()
	packageDir := filepath.Dir(cfgDir)
	candidates := []string{
		filepath.Join(packageDir, filename),
		filepath.Join(packageDir, "compose", filename),
		filepath.Join(cfgDir, "compose", filename),
		filepath.Join(cfgDir, filename),
		filepath.Join(r.baseDir, filename),
		filepath.Join(r.baseDir, "packages", "platform-orchestrator", filename),
	}
	for _, cand := range candidates {
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	return filepath.Join(packageDir, filename)
}

func (r *PathResolver) EnvFile() string {
	cfgDir := r.ConfigDir()
	packageDir := filepath.Dir(cfgDir)
	candidates := []string{
		filepath.Join(packageDir, ".env"),
		filepath.Join(r.baseDir, ".env"),
		filepath.Join(r.baseDir, "packages", "platform-orchestrator", ".env"),
	}
	for _, cand := range candidates {
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	return filepath.Join(packageDir, ".env")
}

func (r *PathResolver) EnvExampleFile() string {
	cfgDir := r.ConfigDir()
	packageDir := filepath.Dir(cfgDir)
	candidates := []string{
		filepath.Join(packageDir, ".env.example"),
		filepath.Join(r.baseDir, ".env.example"),
		filepath.Join(r.baseDir, "packages", "platform-orchestrator", ".env.example"),
	}
	for _, cand := range candidates {
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	return filepath.Join(packageDir, ".env.example")
}

func (r *PathResolver) DataDir() string {
	candidates := []string{
		filepath.Join(r.baseDir, "data"),
		filepath.Join(filepath.Dir(r.baseDir), "data"),
		filepath.Join(filepath.Dir(filepath.Dir(r.baseDir)), "data"),
	}
	for _, cand := range candidates {
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	if r.parsedConfig != nil && r.parsedConfig.Orchestrator.DataDir != "" {
		return filepath.Clean(filepath.Join(r.baseDir, r.parsedConfig.Orchestrator.DataDir))
	}
	return filepath.Join(r.baseDir, "data")
}

func (r *PathResolver) ResolveNetworkConfig(customName, customSubnet, customGateway string) (string, string, string) {
	name := customName
	if name == "" {
		name = os.Getenv("LLMOBS_NETWORK_NAME")
	}
	if name == "" && r.parsedConfig != nil && r.parsedConfig.Docker.Network != "" {
		name = r.parsedConfig.Docker.Network
	}
	if name == "" {
		name = "llmobs-network"
	}

	subnet := customSubnet
	if subnet == "" {
		subnet = os.Getenv("LLMOBS_NETWORK_SUBNET")
	}
	if subnet == "" && r.parsedConfig != nil && r.parsedConfig.Docker.Subnet != "" {
		subnet = r.parsedConfig.Docker.Subnet
	}
	if subnet == "" {
		subnet = "172.28.0.0/16"
	}

	gateway := customGateway
	if gateway == "" {
		gateway = os.Getenv("LLMOBS_NETWORK_GATEWAY")
	}
	if gateway == "" && r.parsedConfig != nil && r.parsedConfig.Docker.Gateway != "" {
		gateway = r.parsedConfig.Docker.Gateway
	}
	if gateway == "" {
		gateway = "172.28.0.1"
	}

	return name, subnet, gateway
}

func defaultSetupYAMLConfig() *SetupYAMLConfig {
	return &SetupYAMLConfig{
		Images: []string{
			"traefik:v2.10",
			"redis:7-alpine",
			"apache/kafka:latest",
			"grafana/tempo:latest",
			"otel/opentelemetry-collector-contrib:latest",
			"grafana/grafana:latest",
		},
		Certificates: []string{
			"traefik.crt",
			"server.pem",
		},
		ComposeFile: "docker-compose.yml",
		Domains: []string{
			"llmobs.gateway",
			"llmobs.grafana",
			"llmobs.tempo",
			"llmobs.otel",
			"llmobs.kafka",
			"llmobs.redis",
		},
		Credentials: map[string]SetupCredentialConfig{
			"database": {
				Prompt:  "Database (AlloyDB) Password",
				EnvKey:  "ALLOYDB_PASSWORD",
				Default: "llmobs_s3cret_2026",
				User:    "admin",
				DBName:  "llm_observability",
			},
			"redis": {
				Prompt:  "Redis Password",
				EnvKey:  "REDIS_PASSWORD",
				Default: "llmobs_redis_s3cret_2024",
			},
			"grafana": {
				Prompt:  "Grafana Admin Password",
				EnvKey:  "GF_SECURITY_ADMIN_PASSWORD",
				Default: "llmobs_admin_password",
				User:    "admin",
			},
			"clickhouse": {
				Prompt:  "ClickHouse Password",
				EnvKey:  "CLICKHOUSE_PASSWORD",
				Default: "llmobs_clickhouse_s3cret_2026",
				User:    "default",
			},
		},
	}
}

func (r *PathResolver) SetupConfig() *SetupYAMLConfig {
	if r.parsedConfig != nil && len(r.parsedConfig.Setup.Images) > 0 {
		return &r.parsedConfig.Setup
	}
	return defaultSetupYAMLConfig()
}

func (r *PathResolver) GetSetupImages() []string {
	return r.SetupConfig().Images
}

func (r *PathResolver) GetSetupCertificates() []string {
	return r.SetupConfig().Certificates
}

func (r *PathResolver) GetSetupDomains() []string {
	return r.SetupConfig().Domains
}

func (r *PathResolver) GetSetupComposeFile() string {
	if r.SetupConfig().ComposeFile != "" {
		return r.SetupConfig().ComposeFile
	}
	return "docker-compose.yml"
}

func (r *PathResolver) GetSetupCredentials() map[string]SetupCredentialConfig {
	return r.SetupConfig().Credentials
}

func (r *PathResolver) ReadEnvMap() map[string]string {
	result := make(map[string]string)
	data, err := os.ReadFile(r.EnvFile())
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

func (r *PathResolver) ResolveEnvOrConfig(envKey, fallback string) string {
	if envKey != "" {
		if val := os.Getenv(envKey); val != "" {
			return val
		}
		envMap := r.ReadEnvMap()
		if val, ok := envMap[envKey]; ok && val != "" {
			return val
		}
	}
	return fallback
}

func (r *PathResolver) GetServicesConfig() map[string]ServiceDefinition {
	if r.parsedConfig != nil && len(r.parsedConfig.Services) > 0 {
		return r.parsedConfig.Services
	}
	return make(map[string]ServiceDefinition)
}

func (r *PathResolver) GetServiceDefinition(service string) ServiceDefinition {
	svcLower := strings.ToLower(strings.TrimSpace(service))
	services := r.GetServicesConfig()
	if def, ok := services[svcLower]; ok {
		return def
	}
	for k, def := range services {
		if strings.EqualFold(k, svcLower) {
			return def
		}
	}
	return ServiceDefinition{}
}

func (r *PathResolver) ResolveServiceURL(service string, fallback string) string {
	def := r.GetServiceDefinition(service)
	if def.EnvURL != "" {
		if val := r.ResolveEnvOrConfig(def.EnvURL, ""); val != "" {
			return val
		}
	}
	if def.URL != "" {
		return def.URL
	}
	return fallback
}

func (r *PathResolver) ResolveServiceUser(service string, fallback string) string {
	def := r.GetServiceDefinition(service)
	if def.EnvUser != "" {
		if val := r.ResolveEnvOrConfig(def.EnvUser, ""); val != "" {
			return val
		}
	}
	if def.User != "" {
		return def.User
	}
	return fallback
}

func (r *PathResolver) ResolveServicePassword(service string, fallback string) string {
	def := r.GetServiceDefinition(service)
	if def.EnvPassword != "" {
		if val := r.ResolveEnvOrConfig(def.EnvPassword, ""); val != "" {
			return val
		}
	}
	if def.DefaultPassword != "" {
		return def.DefaultPassword
	}
	return fallback
}

func (r *PathResolver) ResolveServiceDatabase(service string, fallback string) string {
	def := r.GetServiceDefinition(service)
	if def.EnvDatabase != "" {
		if val := r.ResolveEnvOrConfig(def.EnvDatabase, ""); val != "" {
			return val
		}
	}
	if def.Database != "" {
		return def.Database
	}
	return fallback
}

func (r *PathResolver) ResolveServiceHost(service string, fallback string) string {
	def := r.GetServiceDefinition(service)
	if def.EnvHost != "" {
		if val := r.ResolveEnvOrConfig(def.EnvHost, ""); val != "" {
			return val
		}
	}
	if def.Host != "" {
		return def.Host
	}
	return fallback
}

func (r *PathResolver) ResolveServicePort(service string, fallback int) int {
	def := r.GetServiceDefinition(service)
	if def.EnvPort != "" {
		if val := r.ResolveEnvOrConfig(def.EnvPort, ""); val != "" {
			var p int
			if _, err := fmt.Sscanf(val, "%d", &p); err == nil && p > 0 {
				return p
			}
		}
	}
	if def.Port > 0 {
		return def.Port
	}
	return fallback
}

func (r *PathResolver) GetDNSConfig() DnsYAMLConfig {
	var cfg DnsYAMLConfig
	if r.parsedConfig != nil {
		cfg = r.parsedConfig.DNS
	}
	cfg.DefaultIP = r.ResolveEnvOrConfig("LLMOBS_DNS_DEFAULT_IP", cfg.DefaultIP)
	if cfg.DefaultIP == "" {
		cfg.DefaultIP = "127.0.0.1"
	}
	cfg.HostsPath = r.ResolveEnvOrConfig("LLMOBS_HOSTS_PATH", cfg.HostsPath)
	if cfg.HostsPath == "" {
		cfg.HostsPath = "/etc/hosts"
	}
	cfg.Markers.Start = r.ResolveEnvOrConfig("LLMOBS_DNS_MARKER_START", cfg.Markers.Start)
	if cfg.Markers.Start == "" {
		cfg.Markers.Start = "# --- BEGIN LLMOBS PLATFORM DOMAINS ---"
	}
	cfg.Markers.End = r.ResolveEnvOrConfig("LLMOBS_DNS_MARKER_END", cfg.Markers.End)
	if cfg.Markers.End == "" {
		cfg.Markers.End = "# --- END LLMOBS PLATFORM DOMAINS ---"
	}
	if envDomains := r.ResolveEnvOrConfig("LLMOBS_DNS_DOMAINS", ""); envDomains != "" {
		parts := strings.Split(envDomains, ",")
		var cleaned []string
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				cleaned = append(cleaned, trimmed)
			}
		}
		if len(cleaned) > 0 {
			cfg.Domains = cleaned
		}
	}
	return cfg
}

func (r *PathResolver) GetDNSDomains() []string {
	cfg := r.GetDNSConfig()
	return cfg.Domains
}

func (r *PathResolver) GetDNSMarkers() (string, string) {
	cfg := r.GetDNSConfig()
	return cfg.Markers.Start, cfg.Markers.End
}

func (r *PathResolver) GetTraefikConfig() TraefikYAMLConfig {
	var cfg TraefikYAMLConfig
	if r.parsedConfig != nil {
		cfg = r.parsedConfig.Traefik
	}
	cfg.API.DefaultURL = r.ResolveEnvOrConfig("TRAEFIK_API_URL", cfg.API.DefaultURL)
	if cfg.API.DefaultURL == "" {
		dashboardPort := r.ResolveEnvOrConfig("PORT_TRAEFIK_DASHBOARD", "31411")
		cfg.API.DefaultURL = fmt.Sprintf("http://localhost:%s", dashboardPort)
	}
	cfg.API.Timeout = r.ResolveEnvOrConfig("TRAEFIK_API_TIMEOUT", cfg.API.Timeout)
	if cfg.API.Timeout == "" {
		cfg.API.Timeout = "10s"
	}

	ep := &cfg.API.Endpoints
	ep.Ping = r.ResolveEnvOrConfig("TRAEFIK_EP_PING", ep.Ping)
	if ep.Ping == "" {
		ep.Ping = "/ping"
	}
	ep.Overview = r.ResolveEnvOrConfig("TRAEFIK_EP_OVERVIEW", ep.Overview)
	if ep.Overview == "" {
		ep.Overview = "/api/overview"
	}
	ep.RawData = r.ResolveEnvOrConfig("TRAEFIK_EP_RAWDATA", ep.RawData)
	if ep.RawData == "" {
		ep.RawData = "/api/rawdata"
	}
	ep.Entrypoints = r.ResolveEnvOrConfig("TRAEFIK_EP_ENTRYPOINTS", ep.Entrypoints)
	if ep.Entrypoints == "" {
		ep.Entrypoints = "/api/entrypoints"
	}
	ep.HTTPRouters = r.ResolveEnvOrConfig("TRAEFIK_EP_HTTP_ROUTERS", ep.HTTPRouters)
	if ep.HTTPRouters == "" {
		ep.HTTPRouters = "/api/http/routers"
	}
	ep.HTTPServices = r.ResolveEnvOrConfig("TRAEFIK_EP_HTTP_SERVICES", ep.HTTPServices)
	if ep.HTTPServices == "" {
		ep.HTTPServices = "/api/http/services"
	}
	ep.HTTPMiddlewares = r.ResolveEnvOrConfig("TRAEFIK_EP_HTTP_MIDDLEWARES", ep.HTTPMiddlewares)
	if ep.HTTPMiddlewares == "" {
		ep.HTTPMiddlewares = "/api/http/middlewares"
	}
	ep.TCPRouters = r.ResolveEnvOrConfig("TRAEFIK_EP_TCP_ROUTERS", ep.TCPRouters)
	if ep.TCPRouters == "" {
		ep.TCPRouters = "/api/tcp/routers"
	}
	ep.TCPServices = r.ResolveEnvOrConfig("TRAEFIK_EP_TCP_SERVICES", ep.TCPServices)
	if ep.TCPServices == "" {
		ep.TCPServices = "/api/tcp/services"
	}
	ep.TCPMiddlewares = r.ResolveEnvOrConfig("TRAEFIK_EP_TCP_MIDDLEWARES", ep.TCPMiddlewares)
	if ep.TCPMiddlewares == "" {
		ep.TCPMiddlewares = "/api/tcp/middlewares"
	}
	ep.UDPRouters = r.ResolveEnvOrConfig("TRAEFIK_EP_UDP_ROUTERS", ep.UDPRouters)
	if ep.UDPRouters == "" {
		ep.UDPRouters = "/api/udp/routers"
	}
	ep.UDPServices = r.ResolveEnvOrConfig("TRAEFIK_EP_UDP_SERVICES", ep.UDPServices)
	if ep.UDPServices == "" {
		ep.UDPServices = "/api/udp/services"
	}

	return cfg
}

func ResolveConfigDir(baseDir string) string {
	return NewPathResolver(baseDir).ConfigDir()
}

func ResolveCertDir(baseDir string) string {
	return NewPathResolver(baseDir).CertDir()
}

func ResolveConfigFile(baseDir string, service string, filename string) string {
	return NewPathResolver(baseDir).ConfigFile(service, filename)
}
