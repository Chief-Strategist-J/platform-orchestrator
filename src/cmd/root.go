/*
Package cmd provides the primary Cobra CLI interface and execution wiring for the platform orchestrator.

ALGORITHM BLUEPRINT:
1. RootCommand: Initializes top-level 'llmobs' CLI, base path resolution, and dependency injection wiring.
2. Subcommand Registration:
   - up: Interactive stack profile selector (TTY) or direct argument resolution; executes pre-flight checks, TLS certs, port checks, and stack startup.
   - down: Shuts down all infrastructure containers across all profiles.
   - restart: Restarts stack services and triggers automated post-boot health verification.
   - status: Renders tabular overview of all managed container statuses.
   - logs: Follows container log streams.
   - free-ports: Scans and terminates stale socket listeners blocking platform ports.
   - scale: Supports interactive and argument-driven scaling of services or simulated compute nodes.
   - health: Executes concurrent TCP and HTTP diagnostic probes across platform services.
   - certs: Generates native Go X.509 certificates for CA, server, and client.
   - backup-purge: Performs disaster recovery dumps for AlloyDB and ClickHouse; optionally purges volumes.
   - setup: Executes end-to-end 7-step platform bootstrapping pipeline.
   - cloudflare: Manages Cloudflare Tunnel token configuration and container lifecycle.
   - gdpr-erasure: Performs GDPR/CCPA data erasure across analytical and relational stores with audit logging.
   - verify-credentials: Runs domain service credential verification test suites.
   - server: Boots REST API daemon conforming to OpenAPI 3.1.0 specification.
3. Invariants:
   - Root execution resolves workspace path automatically.
   - Zero inline comments inside function bodies.
   - Cobra commands return non-zero exit codes upon failure.
*/
package cmd

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/api/rest"
	backupSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/backup/schema"
	backupService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/backup/services"
	certsSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/certs/schema"
	certsService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/certs/services"
	cloudflareService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/cloudflare/services"
	configSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/config/schema"
	configService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/config/services"
	gdprSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/gdpr/schema"
	gdprService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/gdpr/services"
	grafanaService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/services"
	healthSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/health/schema"
	healthService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/health/services"
	portsService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/ports/services"
	prereqsService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/prereqs/services"
	scaleSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/scale/schema"
	scaleService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/scale/services"
	servicesService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/services/services"
	setupSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/setup/schema"
	setupService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/setup/services"
	stackSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/stack/schema"
	stackService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/stack/services"
	traefikService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/services"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/infra/docker"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/infra/observability"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/paths"
)

func findWorkspaceRoot() string {
	return paths.DiscoverWorkspaceRoot()
}

func promptInteractiveProfile() []string {
	fileInfo, _ := os.Stdin.Stat()
	if (fileInfo.Mode() & os.ModeCharDevice) == 0 {
		return []string{"full"}
	}

	fmt.Println("\n=====================================================")
	fmt.Println("  LLM Observability Infrastructure Stack Selector    ")
	fmt.Println("=====================================================")
	fmt.Println("Select the infrastructure stack profile you want to launch:")
	fmt.Println()
	fmt.Println("  [1] Full Stack      - All 10 services (Default)")
	fmt.Println("  [2] Database Stack  - AlloyDB (PostgreSQL) + Redis Ledger")
	fmt.Println("  [3] Analytics Stack - ClickHouse Analytics DB")
	fmt.Println("  [4] Streaming Stack - Apache Kafka Event Broker")
	fmt.Println("  [5] Workflows Engine- Temporal Engine (+ auto-includes Database dependency)")
	fmt.Println("  [6] Tracing Stack   - Tempo + OpenTelemetry Collector + Grafana UI")
	fmt.Println("  [7] Network Gateway - Traefik Gateway + Service Registry")
	fmt.Println("  [8] Stateful Plane  - Dedicated Data Tier (AlloyDB, Kafka, Redis, ClickHouse, Tempo)")
	fmt.Println("  [9] Stateless Plane - Autoscaled Compute/Edge (Traefik, Registry, OTel, Grafana, Temporal)")
	fmt.Println("  [10] Custom Profiles- Specify custom profiles (e.g. 'stateful' or 'stateless')")
	fmt.Println("-----------------------------------------------------")
	fmt.Print("Enter choice [1-10] (default: 1): ")

	reader := bufio.NewReader(os.Stdin)
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	switch choice {
	case "2":
		return []string{"db"}
	case "3":
		return []string{"analytics"}
	case "4":
		return []string{"streaming"}
	case "5":
		return []string{"workflows"}
	case "6":
		return []string{"tracing"}
	case "7":
		return []string{"network"}
	case "8":
		return []string{"stateful"}
	case "9":
		return []string{"stateless"}
	case "10":
		fmt.Print("Enter profiles separated by space (e.g. db streaming): ")
		custom, _ := reader.ReadString('\n')
		custom = strings.TrimSpace(custom)
		if custom == "" {
			return []string{"full"}
		}
		return strings.Fields(custom)
	default:
		return []string{"full"}
	}
}

func Execute() {
	workspaceRoot := findWorkspaceRoot()

	tracer := observability.NewOTelTracerAdapter("llmobs-orchestrator")
	dockerAdapter := docker.NewDockerAdapter(workspaceRoot)

	prereqSvc := prereqsService.NewPrereqService(tracer)
	certsSvc := certsService.NewCertService(tracer)
	portSvc := portsService.NewPortService(dockerAdapter, tracer)
	stackSvc := stackService.NewStackService(dockerAdapter, dockerAdapter, tracer, workspaceRoot)
	scaleSvc := scaleService.NewScaleService(dockerAdapter, tracer, workspaceRoot)
	healthSvc := healthService.NewHealthService(tracer)
	backupSvc := backupService.NewBackupService(dockerAdapter, tracer, workspaceRoot)
	cloudflareSvc := cloudflareService.NewCloudflareService(dockerAdapter, tracer, workspaceRoot)
	gdprSvc := gdprService.NewGDPRService(tracer, workspaceRoot)
	setupSvc := setupService.NewSetupService(prereqSvc, certsSvc, tracer, workspaceRoot)
	configSvc := configService.NewConfigService(workspaceRoot, tracer)
	grafanaSvc := grafanaService.NewGrafanaService(tracer, workspaceRoot)
	servicesSvc := servicesService.NewServicesService(tracer, workspaceRoot)
	traefikSvc := traefikService.NewTraefikService(tracer, workspaceRoot)

	restHandler := rest.NewOrchestratorHandler(
		stackSvc,
		scaleSvc,
		healthSvc,
		certsSvc,
		backupSvc,
		cloudflareSvc,
		gdprSvc,
		prereqSvc,
		setupSvc,
		portSvc,
		configSvc,
		grafanaSvc,
		servicesSvc,
		traefikSvc,
		workspaceRoot,
	)

	rootCmd := &cobra.Command{
		Use:   "llmobs",
		Short: "LLMObs Infrastructure Platform Orchestrator CLI",
		Long:  "Open-standard, unified Go orchestration engine for LLM Observability & Infrastructure Platform.",
	}

	upCmd := &cobra.Command{
		Use:   "up [profiles...]",
		Short: "Start infrastructure stack with profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			profiles := args
			if len(profiles) == 0 {
				profiles = promptInteractiveProfile()
			}

			certSpec := certsSchema.DefaultCertSpec(workspaceRoot)
			if _, err := certsSvc.EnsureCertificates(ctx, certSpec); err != nil {
				return fmt.Errorf("certificate initialization failed: %w", err)
			}

			if err := portSvc.FreePorts(ctx, nil); err != nil {
				return fmt.Errorf("port preparation failed: %w", err)
			}

			netName, _ := cmd.Flags().GetString("network-name")
			netSubnet, _ := cmd.Flags().GetString("network-subnet")
			netGateway, _ := cmd.Flags().GetString("network-gateway")

			outcome, err := stackSvc.StartStack(ctx, stackSchema.StackUpCommand{
				Profiles:       profiles,
				Detach:         true,
				NetworkName:    netName,
				NetworkSubnet:  netSubnet,
				NetworkGateway: netGateway,
			})
			if err != nil {
				return err
			}
			fmt.Printf("✓ %s (Profiles: %v)\n", outcome.Message, outcome.ActiveServices)
			return nil
		},
	}
	upCmd.Flags().String("network-name", "", "Custom Docker network name (e.g. llmobs-network)")
	upCmd.Flags().String("network-subnet", "", "Custom Docker network subnet (e.g. 172.28.0.0/16)")
	upCmd.Flags().String("network-gateway", "", "Custom Docker network gateway (e.g. 172.28.0.1)")

	downCmd := &cobra.Command{
		Use:   "down",
		Short: "Stop infrastructure stack across all profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			outcome, err := stackSvc.StopStack(context.Background())
			if err != nil {
				return err
			}
			fmt.Println("✓", outcome.Message)
			return nil
		},
	}

	restartCmd := &cobra.Command{
		Use:   "restart [profiles...]",
		Short: "Restart infrastructure stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			outcome, err := stackSvc.RestartStack(ctx, args)
			if err != nil {
				return err
			}
			fmt.Printf("✓ %s (Profiles: %v)\n", outcome.Message, outcome.ActiveServices)

			fmt.Println("\nRunning post-restart health check...")
			configs := healthSchema.DefaultDeepProbeConfigs("localhost")
			report := healthSvc.RunDeepHealthChecks(ctx, configs)
			if !report.Healthy {
				return fmt.Errorf("one or more required services failed post-restart health checks")
			}

			return nil
		},
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Display active container statuses",
		RunE: func(cmd *cobra.Command, args []string) error {
			statuses, err := stackSvc.GetStatus(context.Background())
			if err != nil {
				return err
			}
			fmt.Printf("%-35s %-20s %-25s %s\n", "NAME", "SERVICE", "STATUS", "PORTS")
			fmt.Println("---------------------------------------------------------------------------------------------------------")
			for _, s := range statuses {
				fmt.Printf("%-35s %-20s %-25s %s\n", s.Name, s.Service, s.Status, s.Ports)
			}
			return nil
		},
	}

	logsCmd := &cobra.Command{
		Use:   "logs [services...]",
		Short: "Stream container logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			tail, _ := cmd.Flags().GetInt("tail")
			if tail == 0 {
				tail = 100
			}
			return stackSvc.StreamLogs(context.Background(), tail)
		},
	}
	logsCmd.Flags().IntP("tail", "n", 100, "Number of lines to show from end of logs")

	freePortsCmd := &cobra.Command{
		Use:   "free-ports",
		Short: "Clean up conflicting host ports",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			if err := portSvc.FreePorts(ctx, nil); err != nil {
				return err
			}
			fmt.Println("✓ Platform ports verified and ready.")
			return nil
		},
	}

	scaleCmd := &cobra.Command{
		Use:   "scale [service <name> <replicas> | node <id> [host] | list | down-node <id>]",
		Short: "Scale stateless services or simulated compute nodes",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			if len(args) == 0 {
				fmt.Println("Select scaling mode:")
				fmt.Println("  1) Scale a specific stateless service (e.g. llmobs-temporal)")
				fmt.Println("  2) Launch an additional simulated compute node (Node 2, 3...)")
				fmt.Println("  3) List active scaled containers")
				fmt.Println("  4) Stop a simulated compute node")
				fmt.Print("Enter choice [1-4]: ")
				var choice int
				fmt.Scanln(&choice)

				switch choice {
				case 1:
					fmt.Print("Enter service name [default: llmobs-temporal]: ")
					var svc string
					fmt.Scanln(&svc)
					if svc == "" {
						svc = "llmobs-temporal"
					}
					fmt.Print("Enter replica count [e.g. 2, 3]: ")
					var count int
					fmt.Scanln(&count)
					if count < 1 {
						count = 2
					}
					err := scaleSvc.ScaleService(ctx, scaleSchema.ScaleServiceCommand{Service: svc, Replicas: count})
					if err != nil {
						return err
					}
					fmt.Printf("✓ Scaled '%s' to %d replicas.\n", svc, count)
				case 2:
					fmt.Print("Enter simulated compute node ID [default: 2]: ")
					var nodeID int
					fmt.Scanln(&nodeID)
					if nodeID < 2 {
						nodeID = 2
					}
					fmt.Print("Enter Primary Data Host IP [default: host.docker.internal]: ")
					var host string
					fmt.Scanln(&host)
					if host == "" {
						host = "host.docker.internal"
					}
					meta, err := scaleSvc.LaunchComputeNode(ctx, scaleSchema.LaunchNodeCommand{
						NodeID:          nodeID,
						PrimaryDataHost: host,
					})
					if err != nil {
						return err
					}
					fmt.Printf("✓ Launched Compute Node %d under project '%s' (Data Host: %s)\n", meta.NodeID, meta.ProjectName, meta.PrimaryDataHost)
				case 3:
					containers, err := scaleSvc.ListActiveContainers(ctx)
					if err != nil {
						return err
					}
					for _, c := range containers {
						fmt.Printf("%-35s %-20s %s\n", c.Name, c.Status, c.Ports)
					}
				case 4:
					fmt.Print("Enter compute node ID to terminate [e.g. 2]: ")
					var nodeID int
					fmt.Scanln(&nodeID)
					if nodeID < 2 {
						nodeID = 2
					}
					if err := scaleSvc.TerminateComputeNode(ctx, nodeID); err != nil {
						return err
					}
					fmt.Printf("✓ Terminated compute node %d.\n", nodeID)
				}
				return nil
			}

			subcmd := args[0]
			switch subcmd {
			case "service":
				if len(args) < 3 {
					return fmt.Errorf("usage: llmobs scale service <service_name> <replicas>")
				}
				rep, err := strconv.Atoi(args[2])
				if err != nil {
					return err
				}
				if err := scaleSvc.ScaleService(ctx, scaleSchema.ScaleServiceCommand{Service: args[1], Replicas: rep}); err != nil {
					return err
				}
				fmt.Printf("✓ Scaled service '%s' to %d replicas.\n", args[1], rep)
			case "node":
				nodeID := 2
				if len(args) >= 2 {
					var err error
					nodeID, err = strconv.Atoi(args[1])
					if err != nil {
						return err
					}
				}
				host := "host.docker.internal"
				if len(args) >= 3 {
					host = args[2]
				}
				meta, err := scaleSvc.LaunchComputeNode(ctx, scaleSchema.LaunchNodeCommand{
					NodeID:          nodeID,
					PrimaryDataHost: host,
				})
				if err != nil {
					return err
				}
				fmt.Printf("✓ Launched simulated compute node %d under project '%s'\n", meta.NodeID, meta.ProjectName)
			case "down-node":
				if len(args) < 2 {
					return fmt.Errorf("usage: llmobs scale down-node <node_id>")
				}
				nodeID, err := strconv.Atoi(args[1])
				if err != nil {
					return err
				}
				if err := scaleSvc.TerminateComputeNode(ctx, nodeID); err != nil {
					return err
				}
				fmt.Printf("✓ Terminated compute node %d.\n", nodeID)
			case "list":
				containers, err := scaleSvc.ListActiveContainers(ctx)
				if err != nil {
					return err
				}
				for _, c := range containers {
					fmt.Printf("%-35s %-20s %s\n", c.Name, c.Status, c.Ports)
				}
			default:
				if len(args) >= 2 {
					rep, err := strconv.Atoi(args[1])
					if err == nil {
						return scaleSvc.ScaleService(ctx, scaleSchema.ScaleServiceCommand{Service: args[0], Replicas: rep})
					}
				}
				return fmt.Errorf("unknown scale command: %s", subcmd)
			}
			return nil
		},
	}



	deepHealthCmd := &cobra.Command{
		Use:     "health-deep [primaryHost]",
		Aliases: []string{"health"},
		Short:   "Run deep functional probes — CRUD ops, wire-protocol handshakes, API verification",
		Long: `Run service-specific deep functional health probes that go beyond TCP/HTTP
connectivity checks. Each probe performs a real operation on the service:

  alloydb      PG wire-protocol StartupMessage + auth + server_version extraction
  redis        PING + SET + GET + DEL + INFO server (RESP protocol, no redis-cli needed)
  kafka        ApiVersions + CreateTopic + Produce + Fetch + DeleteTopic (binary protocol)
  clickhouse   GET /ping + SELECT version() via HTTP interface
  grafana      GET /api/health + GET /api/datasources (confirms telemetry datasource exists)
  tempo        GET /ready + GET /api/status/buildinfo
  temporal     TCP frontend port + Temporal Web UI namespace API
  otel-collector  TCP HTTP port + TCP gRPC port + GET /metrics (confirms otelcol_ metrics exist)
  traefik      GET /ping + GET /api/rawdata (route count)
  service-registry  GET /health + GET /v1/catalog/services

All parameters are configurable via flags. Use --services to limit scope.

Examples:
  llmobs health-deep                                      # all services, localhost
  llmobs health-deep --services kafka,redis               # specific services
  llmobs health-deep --services alloydb --username admin --password secret
  llmobs health-deep --services redis --container redis-ledger
  llmobs health-deep --timeout-ms 10000 --services clickhouse`,
		RunE: func(cmd *cobra.Command, args []string) error {
			host := "localhost"
			if len(args) > 0 {
				host = args[0]
			}

			parseCsv := func(flag string) []string {
				val, _ := cmd.Flags().GetString(flag)
				if val == "" {
					return nil
				}
				var out []string
				for _, item := range strings.Split(val, ",") {
					if t := strings.TrimSpace(item); t != "" {
						out = append(out, t)
					}
				}
				return out
			}

			serviceFilter := parseCsv("services")
			profilesFilter := parseCsv("profiles")
			timeoutMs, _ := cmd.Flags().GetInt("timeout-ms")
			username, _ := cmd.Flags().GetString("username")
			password, _ := cmd.Flags().GetString("password")
			database, _ := cmd.Flags().GetString("database")
			container, _ := cmd.Flags().GetString("container")
			grafanaUser, _ := cmd.Flags().GetString("grafana-user")
			grafanaPass, _ := cmd.Flags().GetString("grafana-pass")
			temporalNS, _ := cmd.Flags().GetString("temporal-ns")

			var defaults []healthSchema.DeepProbeConfig
			if len(profilesFilter) > 0 {
				defaults = healthSchema.DeepProbeConfigsForProfiles(host, profilesFilter)
			} else {
				defaults = healthSchema.DefaultDeepProbeConfigs(host)
			}

			filterSet := make(map[string]struct{}, len(serviceFilter))
			for _, s := range serviceFilter {
				filterSet[s] = struct{}{}
			}

			var configs []healthSchema.DeepProbeConfig
			for _, d := range defaults {
				if len(filterSet) > 0 {
					if _, ok := filterSet[d.Service]; !ok {
						continue
					}
				}
				if timeoutMs > 0 {
					d.Timeout = time.Duration(timeoutMs) * time.Millisecond
				}
				if username != "" {
					d.Username = username
				}
				if password != "" {
					d.Password = password
				}
				if database != "" {
					d.Database = database
				}
				if container != "" {
					d.Container = container
				}
				if d.Service == "grafana" {
					d.GrafanaUser = grafanaUser
					d.GrafanaPass = grafanaPass
				}
				if d.Service == "temporal" {
					d.TemporalNS = temporalNS
				}
				configs = append(configs, d)
			}

			report := healthSvc.RunDeepHealthChecks(context.Background(), configs)

			fmt.Println("========================================================================================================================================")
			fmt.Printf(" Platform DEEP Health Verification (Checked: %d, Healthy: %d, ReportedAt: %s)\n",
				report.CheckedCount, report.HealthyCount, report.ReportedAt)
			if len(profilesFilter) > 0 {
				fmt.Printf(" Profiles : %s\n", strings.Join(profilesFilter, ", "))
			}
			if len(serviceFilter) > 0 {
				fmt.Printf(" Services : %s\n", strings.Join(serviceFilter, ", "))
			}
			fmt.Println("========================================================================================================================================")
			fmt.Printf("%-20s %-12s %8s   %s\n", "SERVICE", "STATUS", "LATENCY", "EVIDENCE / ERROR")
			fmt.Println("----------------------------------------------------------------------------------------------------------------------------------------")
			for _, r := range report.Results {
				detail := r.Error
				statusLabel := r.Status
				if !r.IsHealthy {
					statusLabel = "DOWN"
				}
				fmt.Printf("%-20s %-12s %6.1fms   %s\n", r.Service, statusLabel, r.LatencyMs, detail)
			}
			fmt.Println("========================================================================================================================================")
			if !report.Healthy {
				return fmt.Errorf("one or more services failed deep health verification")
			}
			fmt.Println("✓ All services passed deep functional verification.")
			return nil
		},
	}
	deepHealthCmd.Flags().String("profiles", "", "Comma-separated Docker Compose profiles to scope checks to (e.g. db,streaming,tracing)")
	deepHealthCmd.Flags().String("services", "", "Comma-separated service names to probe (e.g. kafka,redis,alloydb)")
	deepHealthCmd.Flags().Int("timeout-ms", 5000, "Probe timeout in milliseconds")
	deepHealthCmd.Flags().String("username", "", "Username override for database probes")
	deepHealthCmd.Flags().String("password", "", "Password override (redis, alloydb)")
	deepHealthCmd.Flags().String("database", "", "Database name override (alloydb, clickhouse)")
	deepHealthCmd.Flags().String("container", "", "Container name for docker exec-based probes")
	deepHealthCmd.Flags().String("grafana-user", "admin", "Grafana admin username")
	deepHealthCmd.Flags().String("grafana-pass", "admin", "Grafana admin password")
	deepHealthCmd.Flags().String("temporal-ns", "default", "Temporal namespace to verify")

	datasourceCmd := NewDatasourceCommand(grafanaSvc)
	dashboardCmd := NewDashboardCommand(grafanaSvc)
	alertCmd := NewAlertCommand(grafanaSvc)
	serviceCmd := NewServiceCommand(servicesSvc, grafanaSvc)
	traefikCmd := NewTraefikCommand(traefikSvc)


	certsCmd := &cobra.Command{
		Use:   "certs",
		Short: "Generate self-signed TLS certificates natively in Go",
		RunE: func(cmd *cobra.Command, args []string) error {
			spec := certsSchema.DefaultCertSpec(workspaceRoot)
			res, err := certsSvc.EnsureCertificates(context.Background(), spec)
			if err != nil {
				return err
			}
			if res.Generated {
				fmt.Printf("✓ Generated new TLS certificates at %s\n", res.CertPath)
			} else {
				fmt.Printf("✓ Existing valid TLS certificates verified at %s\n", res.CertPath)
			}
			return nil
		},
	}

	backupPurgeCmd := &cobra.Command{
		Use:   "backup-purge",
		Short: "Perform database disaster recovery backup and optional volume purge",
		RunE: func(cmd *cobra.Command, args []string) error {
			backupOnly, _ := cmd.Flags().GetBool("backup-only")
			opts := backupSchema.BackupOptions{
				BackupOnly: backupOnly,
				Purge:      !backupOnly,
			}
			report, err := backupSvc.ExecuteBackupAndPurge(context.Background(), opts)
			if err != nil {
				return err
			}
			if report.AlloyDBDumpFile != "" {
				fmt.Printf("✓ AlloyDB backup saved to: %s\n", report.AlloyDBDumpFile)
			}
			if report.ClickHouseDumpFile != "" {
				fmt.Printf("✓ ClickHouse schema & partition freeze saved to: %s\n", report.ClickHouseDumpFile)
			}
			fmt.Printf("✓ %s\n", report.Message)
			return nil
		},
	}
	backupPurgeCmd.Flags().Bool("backup-only", false, "Only dump databases without purging Docker volumes")

	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: "Run full 7-step platform bootstrapping pipeline",
		RunE: func(cmd *cobra.Command, args []string) error {
			pull, _ := cmd.Flags().GetBool("pull")
			interactive, _ := cmd.Flags().GetBool("interactive")
			restart, _ := cmd.Flags().GetBool("restart")

			creds := make(map[string]string)
			if dbPass, _ := cmd.Flags().GetString("db-password"); dbPass != "" {
				creds["ALLOYDB_PASSWORD"] = dbPass
			}
			if redisPass, _ := cmd.Flags().GetString("redis-password"); redisPass != "" {
				creds["REDIS_PASSWORD"] = redisPass
			}
			if grafanaPass, _ := cmd.Flags().GetString("grafana-password"); grafanaPass != "" {
				creds["GF_SECURITY_ADMIN_PASSWORD"] = grafanaPass
			}
			if chPass, _ := cmd.Flags().GetString("clickhouse-password"); chPass != "" {
				creds["CLICKHOUSE_PASSWORD"] = chPass
			}

			if interactive && len(creds) == 0 {
				reader := bufio.NewReader(os.Stdin)
				creds = setupSvc.PromptCredentialsInteractively(reader)
			}

			cmdPayload := setupSchema.SetupCommand{
				PullImages:      pull,
				Interactive:     interactive,
				Credentials:     creds,
				RestartServices: restart,
			}

			report, err := setupSvc.RunSetupPipeline(context.Background(), cmdPayload)
			for _, st := range report.Steps {
				if st.Passed {
					fmt.Printf("  ✓ [%d/7] %s (%v)\n", st.Index, st.Name, st.Duration.Round(time.Millisecond))
				} else {
					fmt.Printf("  ✖ [%d/7] %s - ERROR: %s\n", st.Index, st.Name, st.Error)
				}
			}
			if err != nil {
				return err
			}
			fmt.Printf("\n✓ %s (%d/%d steps passed)\n", report.Message, report.PassedSteps, report.TotalSteps)
			return nil
		},
	}
	setupCmd.Flags().Bool("pull", false, "Pull Docker images during setup")
	setupCmd.Flags().BoolP("interactive", "i", false, "Interactively prompt for service credentials (keeps defaults on Enter)")
	setupCmd.Flags().Bool("restart", true, "Restart running database/cache services on credential update")
	setupCmd.Flags().String("db-password", "", "Override database (AlloyDB) password directly")
	setupCmd.Flags().String("redis-password", "", "Override Redis password directly")
	setupCmd.Flags().String("grafana-password", "", "Override Grafana admin password directly")
	setupCmd.Flags().String("clickhouse-password", "", "Override ClickHouse password directly")

	cloudflareCmd := &cobra.Command{
		Use:   "cloudflare [setup|start|stop|status|logs]",
		Short: "Manage Cloudflare Tunnel ingress",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			action := "setup"
			if len(args) > 0 {
				action = args[0]
			}

			switch action {
			case "setup":
				fmt.Print("Enter your Cloudflare Tunnel Token: ")
				reader := bufio.NewReader(os.Stdin)
				token, _ := reader.ReadString('\n')
				token = strings.TrimSpace(token)
				if token == "" {
					return fmt.Errorf("tunnel token cannot be empty")
				}
				if err := cloudflareSvc.SaveTunnelToken(token); err != nil {
					return err
				}
				fmt.Println("✓ Saved Cloudflare Tunnel Token to .env")
				rep, err := cloudflareSvc.StartTunnel(ctx)
				if err != nil {
					return err
				}
				fmt.Println("✓", rep.Message)
			case "start":
				rep, err := cloudflareSvc.StartTunnel(ctx)
				if err != nil {
					return err
				}
				fmt.Println("✓", rep.Message)
			case "stop":
				rep, err := cloudflareSvc.StopTunnel(ctx)
				if err != nil {
					return err
				}
				fmt.Println("✓", rep.Message)
			case "status":
				st, err := cloudflareSvc.GetStatus(ctx)
				if err != nil {
					return err
				}
				fmt.Println(st)
			case "logs":
				return cloudflareSvc.StreamLogs(ctx)
			default:
				return fmt.Errorf("unknown cloudflare action: %s", action)
			}
			return nil
		},
	}

	gdprCmd := &cobra.Command{
		Use:   "gdpr-erasure",
		Short: "Execute GDPR/CCPA right-to-erasure across databases",
		RunE: func(cmd *cobra.Command, args []string) error {
			userID, _ := cmd.Flags().GetString("user-id")
			customerID, _ := cmd.Flags().GetString("customer-id")
			if userID == "" && customerID == "" {
				return fmt.Errorf("must provide --user-id or --customer-id")
			}
			report, err := gdprSvc.ExecuteErasure(context.Background(), gdprSchema.ErasureRequest{
				UserID:     userID,
				CustomerID: customerID,
			})
			if err != nil {
				return err
			}
			fmt.Printf("✓ %s (ClickHouse: %v, AlloyDB: %v, Audit: %v)\n", report.Message, report.ClickHousePurged, report.AlloyDBPurged, report.AuditRecorded)
			return nil
		},
	}
	gdprCmd.Flags().String("user-id", "", "Target User ID for erasure")
	gdprCmd.Flags().String("customer-id", "", "Target Customer ID for erasure")

	verifyCmd := &cobra.Command{
		Use:   "verify-credentials <label>",
		Short: "Verify connectivity and credentials for any named target",
		Long: `Verify connectivity and credentials for any named target against the platform infrastructure.

The <label> is an arbitrary identifier (profile name, service name, env name, etc.).
No target-to-component mapping is hardcoded. You explicitly declare what to check via --check.

Config resolution priority (highest wins):
  1. CLI flags        --db-host, --db-port, --redis-pass, etc.
  2. Target .env      local-services/<label>/.env  (if present)
  3. Platform .env    packages/platform-orchestrator/.env
  4. Fallback default (built-in, last resort only)

Available components for --check:
  db          AlloyDB / PostgreSQL
  redis       Redis Ledger
  kafka       Apache Kafka Broker
  otel        OpenTelemetry Collector (HTTP + gRPC)
  analytics   ClickHouse Analytics DB

Examples:
  llmobs verify-credentials myprofile --check db,redis
  llmobs verify-credentials staging   --check kafka,otel --kafka-host kafka.staging.internal
  llmobs verify-credentials prod-auth --check db --db-host 10.0.1.5 --db-port 5432
  llmobs verify-credentials analytics-svc --check analytics --clickhouse-port 8123
  llmobs verify-credentials dev         # no --check = runs ALL components`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			label := args[0]

			platformEnv := loadEnvFile(filepath.Join(workspaceRoot, "packages", "platform-orchestrator", ".env"))
			targetEnv := loadEnvFile(filepath.Join(workspaceRoot, "local-services", label, ".env"))
			if len(targetEnv) == 0 {
				targetEnv = loadEnvFile(filepath.Join(workspaceRoot, "local-services", label, ".env.example"))
			}

			resolveCfg := func(flagName, targetKey, platformKey, fallback string) string {
				if cmd.Flags().Changed(flagName) {
					if v, _ := cmd.Flags().GetString(flagName); v != "" {
						return v
					}
				}
				if targetKey != "" {
					if v := targetEnv[targetKey]; v != "" {
						return v
					}
				}
				if platformKey != "" {
					if v := platformEnv[platformKey]; v != "" {
						return v
					}
				}
				return fallback
			}

			env := strings.ToUpper(strings.ReplaceAll(label, "-", "_"))

			checkList, _ := cmd.Flags().GetString("check")

			cfg := verifyConfig{
				Label:          label,
				Components:     checkList,
				DBHost:         resolveCfg("db-host",         env+"_DB_HOST",         "ALLOYDB_HOST",         "localhost"),
				DBPort:         resolveCfg("db-port",         env+"_DB_PORT",         "PORT_ALLOYDB",         "31420"),
				DBUser:         resolveCfg("db-user",         env+"_DB_USER",         "ALLOYDB_USER",         "admin"),
				DBPass:         resolveCfg("db-pass",         env+"_DB_PASSWORD",     "ALLOYDB_PASSWORD",     ""),
				DBName:         resolveCfg("db-name",         env+"_DB_NAME",         "ALLOYDB_DB",           "llm_observability"),
				DBContainer:    resolveCfg("db-container",    "",                     "",                     "llmobs-alloydb-db"),
				RedisHost:      resolveCfg("redis-host",      env+"_REDIS_HOST",      "REDIS_HOST",           "localhost"),
				RedisPort:      resolveCfg("redis-port",      env+"_REDIS_PORT",      "PORT_REDIS",           "31413"),
				RedisPass:      resolveCfg("redis-pass",      env+"_REDIS_PASSWORD",  "REDIS_PASSWORD",       ""),
				RedisContainer: resolveCfg("redis-container", "",                     "",                     "llmobs-redis-ledger"),
				KafkaHost:      resolveCfg("kafka-host",      env+"_KAFKA_HOST",      "KAFKA_HOST",           "localhost"),
				KafkaPort:      resolveCfg("kafka-port",      env+"_KAFKA_PORT",      "PORT_KAFKA",           "31414"),
				OtelHTTPPort:   resolveCfg("otel-http-port",  env+"_OTEL_HTTP_PORT",  "PORT_OTEL_HTTP",       "31417"),
				OtelGRPCPort:   resolveCfg("otel-grpc-port",  env+"_OTEL_GRPC_PORT",  "PORT_OTEL_GRPC",       "31418"),
				ClickHousePort: resolveCfg("clickhouse-port", env+"_CLICKHOUSE_PORT", "PORT_CLICKHOUSE_HTTP", "31421"),
			}

			return verifyNativeCredentials(cfg)
		},
	}
	verifyCmd.Flags().String("check", "", "Comma-separated components to verify (default: all). e.g. db,redis or kafka,otel")
	verifyCmd.Flags().String("db-host", "", "Database host")
	verifyCmd.Flags().String("db-port", "", "Database port")
	verifyCmd.Flags().String("db-user", "", "Database username")
	verifyCmd.Flags().String("db-pass", "", "Database password")
	verifyCmd.Flags().String("db-name", "", "Database name")
	verifyCmd.Flags().String("db-container", "", "AlloyDB Docker container name (for exec-based auth fallback)")
	verifyCmd.Flags().String("redis-host", "", "Redis host")
	verifyCmd.Flags().String("redis-port", "", "Redis port")
	verifyCmd.Flags().String("redis-pass", "", "Redis password")
	verifyCmd.Flags().String("redis-container", "", "Redis Docker container name")
	verifyCmd.Flags().String("kafka-host", "", "Kafka broker host")
	verifyCmd.Flags().String("kafka-port", "", "Kafka broker port")
	verifyCmd.Flags().String("otel-http-port", "", "OTel Collector HTTP port")
	verifyCmd.Flags().String("otel-grpc-port", "", "OTel Collector gRPC port")
	verifyCmd.Flags().String("clickhouse-port", "", "ClickHouse HTTP port")

	serverCmd := &cobra.Command{
		Use:   "server",
		Short: "Start REST API daemon conforming to OpenAPI specification",
		RunE: func(cmd *cobra.Command, args []string) error {
			router := rest.NewRouter(restHandler)
			port := 31499
			srv := &http.Server{
				Addr:         fmt.Sprintf(":%d", port),
				Handler:      router,
				ReadTimeout:  15 * time.Second,
				WriteTimeout: 60 * time.Second,
			}
			fmt.Printf("⚡ Platform Orchestrator REST API listening on http://localhost:%d/api/v1\n", port)
			return srv.ListenAndServe()
		},
	}

	var configInteractive bool
	var configRestart bool
	var configAlloyDBMem string
	var configAlloyDBCpus string
	var configTemporalMem string
	var configClickHouseMem string
	var configRedisMem string
	var configKafkaMem string
	var configNetworkName string

	configCmd := &cobra.Command{
		Use:   "config",
		Short: "View and customize platform resource limits and configurations",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			report, err := configSvc.GetPlatformConfig(ctx)
			if err != nil {
				return fmt.Errorf("failed to retrieve configuration: %w", err)
			}

			if configInteractive {
				reader := bufio.NewReader(os.Stdin)
				updateCmd := configSvc.PromptInteractive(reader, report)
				updated, err := configSvc.UpdatePlatformConfig(ctx, updateCmd)
				if err != nil {
					return fmt.Errorf("failed to apply configuration: %w", err)
				}
				fmt.Println("\nConfiguration successfully updated!")
				printConfigReport(updated)
				return nil
			}

			if configAlloyDBMem != "" || configAlloyDBCpus != "" || configTemporalMem != "" ||
				configClickHouseMem != "" || configRedisMem != "" || configKafkaMem != "" || configNetworkName != "" {
				updateCmd := configSchema.UpdateConfigCommand{
					RestartServices:  configRestart,
					AlloyDBMemory:    configAlloyDBMem,
					AlloyDBCpus:      configAlloyDBCpus,
					TemporalMemory:   configTemporalMem,
					ClickHouseMemory: configClickHouseMem,
					RedisMemory:      configRedisMem,
					KafkaMemory:      configKafkaMem,
					NetworkName:      configNetworkName,
				}
				updated, err := configSvc.UpdatePlatformConfig(ctx, updateCmd)
				if err != nil {
					return fmt.Errorf("failed to apply configuration: %w", err)
				}
				fmt.Println("\nConfiguration successfully updated!")
				printConfigReport(updated)
				return nil
			}

			printConfigReport(report)
			return nil
		},
	}

	configCmd.Flags().BoolVarP(&configInteractive, "interactive", "i", false, "Interactive prompt to modify resource limits")
	configCmd.Flags().BoolVarP(&configRestart, "restart", "r", false, "Restart containers after applying changes")
	configCmd.Flags().StringVar(&configAlloyDBMem, "alloydb-memory", "", "Set AlloyDB memory limit (e.g. 4096M)")
	configCmd.Flags().StringVar(&configAlloyDBCpus, "alloydb-cpus", "", "Set AlloyDB CPU limit (e.g. 2.0)")
	configCmd.Flags().StringVar(&configTemporalMem, "temporal-memory", "", "Set Temporal memory limit (e.g. 2048M)")
	configCmd.Flags().StringVar(&configClickHouseMem, "clickhouse-memory", "", "Set ClickHouse memory limit (e.g. 4096M)")
	configCmd.Flags().StringVar(&configRedisMem, "redis-memory", "", "Set Redis memory limit (e.g. 512M)")
	configCmd.Flags().StringVar(&configKafkaMem, "kafka-memory", "", "Set Kafka memory limit (e.g. 2048M)")
	configCmd.Flags().StringVar(&configNetworkName, "network-name", "", "Set custom docker network name")

	rootCmd.AddCommand(
		upCmd,
		downCmd,
		restartCmd,
		statusCmd,
		logsCmd,
		freePortsCmd,
		scaleCmd,
		deepHealthCmd,
		certsCmd,
		backupPurgeCmd,
		setupCmd,
		cloudflareCmd,
		gdprCmd,
		verifyCmd,
		serverCmd,
		configCmd,
		datasourceCmd,
		dashboardCmd,
		alertCmd,
		serviceCmd,
		traefikCmd,
	)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func printConfigReport(report *configSchema.PlatformConfigReport) {
	fmt.Println("\n=======================================================")
	fmt.Println("       LLMOBS INFRASTRUCTURE CONFIGURATION REPORT      ")
	fmt.Println("=======================================================")
	fmt.Printf("Active Env File   : %s\n", report.ActiveEnvFile)
	fmt.Printf("Compose Spec      : %s\n", report.ComposeFile)
	fmt.Printf("Platform Network  : %s (subnet: %s, gw: %s)\n", report.NetworkName, report.NetworkSubnet, report.NetworkGateway)
	fmt.Println("-------------------------------------------------------")
	fmt.Println("Service Resource Constraints:")
	fmt.Printf("  AlloyDB (Postgres) : Mem: %s (res: %s) | CPUs: %s\n", report.Resources.AlloyDB.MemoryLimit, report.Resources.AlloyDB.MemoryReservation, report.Resources.AlloyDB.CpusLimit)
	fmt.Printf("  Temporal Engine    : Mem: %s (res: %s)\n", report.Resources.Temporal.MemoryLimit, report.Resources.Temporal.MemoryReservation)
	fmt.Printf("  ClickHouse OLAP    : Mem: %s (res: %s)\n", report.Resources.ClickHouse.MemoryLimit, report.Resources.ClickHouse.MemoryReservation)
	fmt.Printf("  Apache Kafka       : Mem: %s (res: %s)\n", report.Resources.Kafka.MemoryLimit, report.Resources.Kafka.MemoryReservation)
	fmt.Printf("  Redis Cache        : Mem: %s (res: %s)\n", report.Resources.Redis.MemoryLimit, report.Resources.Redis.MemoryReservation)
	fmt.Printf("  Traefik Gateway    : Mem: %s (res: %s)\n", report.Resources.Traefik.MemoryLimit, report.Resources.Traefik.MemoryReservation)
	fmt.Printf("  Grafana UI         : Mem: %s (res: %s)\n", report.Resources.Grafana.MemoryLimit, report.Resources.Grafana.MemoryReservation)
	fmt.Printf("  OTel Collector     : Mem: %s (res: %s)\n", report.Resources.OTelCollector.MemoryLimit, report.Resources.OTelCollector.MemoryReservation)
	fmt.Printf("  Tempo Tracing      : Mem: %s (res: %s)\n", report.Resources.Tempo.MemoryLimit, report.Resources.Tempo.MemoryReservation)
	fmt.Printf("  Service Registry   : Mem: %s (res: %s)\n", report.Resources.ServiceRegistry.MemoryLimit, report.Resources.ServiceRegistry.MemoryReservation)
	fmt.Println("=======================================================")
	fmt.Println("Hint: Run 'llmobs config -i' to interactively edit or 'llmobs config --alloydb-memory=4096M --restart'")
}

func mustGetString(cmd *cobra.Command, name string) string {
	v, _ := cmd.Flags().GetString(name)
	return v
}

func loadEnvFile(path string) map[string]string {
	result := make(map[string]string)
	data, err := os.ReadFile(path)
	if err != nil {
		return result
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 2 {
			result[strings.TrimSpace(parts[0])] = strings.Trim(strings.TrimSpace(parts[1]), `"' `)
		}
	}
	return result
}

type verifyConfig struct {
	Label          string
	Components     string
	DBHost         string
	DBPort         string
	DBUser         string
	DBPass         string
	DBName         string
	DBContainer    string
	RedisHost      string
	RedisPort      string
	RedisPass      string
	RedisContainer string
	KafkaHost      string
	KafkaPort      string
	OtelHTTPPort   string
	OtelGRPCPort   string
	ClickHousePort string
}

func verifyNativeCredentials(cfg verifyConfig) error {
	want := func(c string) bool {
		if cfg.Components == "" {
			return true
		}
		for _, item := range strings.Split(cfg.Components, ",") {
			if strings.TrimSpace(item) == c {
				return true
			}
		}
		return false
	}

	fmt.Printf("\n\033[94m====================================================\033[0m\n")
	fmt.Printf("\033[1m CREDENTIAL VERIFICATION: %s\033[0m\n", strings.ToUpper(cfg.Label))
	if cfg.Components != "" {
		fmt.Printf("\033[93m Checking: %s\033[0m\n", cfg.Components)
	} else {
		fmt.Printf("\033[93m Checking: all components\033[0m\n")
	}
	fmt.Printf("\033[94m====================================================\033[0m\n\n")

	passed, total := 0, 0

	if want("db") {
		total++
		fmt.Printf("\033[1m1. Database (PostgreSQL / AlloyDB):\033[0m\n")
		fmt.Printf("   Target: %s@%s:%s/%s  (container: %s)\n",
			cfg.DBUser, cfg.DBHost, cfg.DBPort, cfg.DBName, cfg.DBContainer)

		conn, tcpErr := net.DialTimeout("tcp", cfg.DBHost+":"+cfg.DBPort, 3*time.Second)
		if tcpErr != nil {
			fmt.Printf("  \033[91m[FAIL]\033[0m AlloyDB (PostgreSQL) -> %s:%s unreachable: %v\n", cfg.DBHost, cfg.DBPort, tcpErr)
		} else {
			conn.Close()
			out, execErr := exec.Command(
				"docker", "exec", "-e", "PGPASSWORD="+cfg.DBPass,
				cfg.DBContainer,
				"psql", "-U", cfg.DBUser, "-d", cfg.DBName, "-c", "SELECT 'AUTH_OK' AS status;",
			).CombinedOutput()
			if execErr == nil && strings.Contains(string(out), "AUTH_OK") {
				fmt.Printf("  \033[92m[PASS]\033[0m AlloyDB (PostgreSQL) -> Authenticated & query verified (User: '%s', DB: '%s')\n", cfg.DBUser, cfg.DBName)
				passed++
			} else {
				fmt.Printf("  \033[91m[FAIL]\033[0m AlloyDB (PostgreSQL) -> Query failed: %s\n", strings.TrimSpace(string(out)))
			}
		}
		fmt.Println()
	}

	if want("redis") {
		total++
		fmt.Printf("\033[1m2. Redis Ledger:\033[0m\n")
		fmt.Printf("   Target: %s:%s  (container: %s, auth: ***)\n", cfg.RedisHost, cfg.RedisPort, cfg.RedisContainer)

		redisCmd := exec.Command("docker", "exec", cfg.RedisContainer, "redis-cli", "-a", cfg.RedisPass, "ping")
		redisOut, rErr := redisCmd.CombinedOutput()
		if rErr == nil && strings.Contains(string(redisOut), "PONG") {
			fmt.Printf("  \033[92m[PASS]\033[0m Redis Ledger -> Authentication successful (PONG received)\n")
			passed++
		} else {
			rCmd2 := exec.Command("docker", "exec", cfg.RedisContainer, "redis-cli", "ping")
			rOut2, rErr2 := rCmd2.CombinedOutput()
			if rErr2 == nil && strings.Contains(string(rOut2), "PONG") {
				fmt.Printf("  \033[92m[PASS]\033[0m Redis Ledger -> Connected (no password required)\n")
				passed++
			} else {
				fmt.Printf("  \033[91m[FAIL]\033[0m Redis Ledger -> Authentication failed: %s\n", strings.TrimSpace(string(redisOut)))
			}
		}
		fmt.Println()
	}

	if want("kafka") {
		total++
		fmt.Printf("\033[1m3. Apache Kafka Event Broker:\033[0m\n")
		fmt.Printf("   Target: %s:%s\n", cfg.KafkaHost, cfg.KafkaPort)

		conn, kErr := net.DialTimeout("tcp", cfg.KafkaHost+":"+cfg.KafkaPort, 3*time.Second)
		if kErr == nil {
			conn.Close()
			fmt.Printf("  \033[92m[PASS]\033[0m Kafka Broker -> TCP connection verified (%s:%s)\n", cfg.KafkaHost, cfg.KafkaPort)
			passed++
		} else {
			fmt.Printf("  \033[91m[FAIL]\033[0m Kafka Broker -> Connection failed: %v\n", kErr)
		}
		fmt.Println()
	}

	if want("analytics") {
		total++
		fmt.Printf("\033[1m4. ClickHouse Analytics:\033[0m\n")
		fmt.Printf("   Target: localhost:%s\n", cfg.ClickHousePort)

		conn, chErr := net.DialTimeout("tcp", "localhost:"+cfg.ClickHousePort, 3*time.Second)
		if chErr == nil {
			conn.Close()
			fmt.Printf("  \033[92m[PASS]\033[0m ClickHouse -> HTTP port %s reachable\n", cfg.ClickHousePort)
			passed++
		} else {
			fmt.Printf("  \033[91m[FAIL]\033[0m ClickHouse -> Port %s unreachable: %v\n", cfg.ClickHousePort, chErr)
		}
		fmt.Println()
	}

	if want("otel") {
		total += 2
		fmt.Printf("\033[1m5. OpenTelemetry Collector:\033[0m\n")
		fmt.Printf("   HTTP: http://localhost:%s/v1/traces\n", cfg.OtelHTTPPort)
		fmt.Printf("   gRPC: localhost:%s\n", cfg.OtelGRPCPort)

		otelConn, oErr := net.DialTimeout("tcp", "localhost:"+cfg.OtelHTTPPort, 3*time.Second)
		if oErr == nil {
			otelConn.Close()
			fmt.Printf("  \033[92m[PASS]\033[0m OTel Collector HTTP -> Port %s reachable\n", cfg.OtelHTTPPort)
			passed++
		} else {
			fmt.Printf("  \033[91m[FAIL]\033[0m OTel Collector HTTP -> Port %s unreachable: %v\n", cfg.OtelHTTPPort, oErr)
		}

		grpcConn, gErr := net.DialTimeout("tcp", "localhost:"+cfg.OtelGRPCPort, 3*time.Second)
		if gErr == nil {
			grpcConn.Close()
			fmt.Printf("  \033[92m[PASS]\033[0m OTel Collector gRPC -> Port %s reachable\n", cfg.OtelGRPCPort)
			passed++
		} else {
			fmt.Printf("  \033[91m[FAIL]\033[0m OTel Collector gRPC -> Port %s unreachable: %v\n", cfg.OtelGRPCPort, gErr)
		}
		fmt.Println()
	}

	fmt.Printf("\033[94m====================================================\033[0m\n")
	if passed == total {
		fmt.Printf("\033[92m\033[1m✓ ALL %d/%d VERIFICATION CHECKS PASSED!\033[0m\n", passed, total)
	} else {
		fmt.Printf("\033[91m\033[1m✗ %d OF %d CHECKS FAILED!\033[0m\n", total-passed, total)
	}
	fmt.Printf("\033[94m====================================================\033[0m\n\n")

	return nil
}


