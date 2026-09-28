/*
Package cmd provides Cobra CLI commands for managing external service connections and dynamic health probes.

ALGORITHM BLUEPRINT:
1. NewServiceCommand: Instantiates parent 'service' (alias: 'services', 'connection', 'connections') command hierarchy.
2. Subcommands:
   - add: Dynamically registers any arbitrary external service or custom data service (PostgreSQL, ClickHouse, Redis, LLMs, Vector DBs, APIs).
   - list: Renders tabular overview of all registered external services.
   - get: Retrieves full JSON schema definition of a service by ID or Name.
   - test: Executes live health probes (HTTP, TCP socket) and verifies availability.
   - update: Mutates service properties, auth tokens, or health check configs.
   - delete: Unregisters a service from the catalog.
   - sync-to-grafana: Automatically provisions the service as a Grafana datasource.
3. Invariants:
   - Zero inline comments inside function bodies.
   - Atomic persistence ensures data safety.
   - Non-zero exit code on probe failure.
*/
package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	grafanaService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/services"
	servicesSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/services/schema"
	servicesService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/services/services"
)

func NewServiceCommand(svcManager *servicesService.ServicesService, grafanaSvc *grafanaService.GrafanaService) *cobra.Command {
	rootServiceCmd := &cobra.Command{
		Use:     "service [subcommand]",
		Aliases: []string{"services", "connection", "connections"},
		Short:   "Manage external services, data service connections, and health probes",
		Long: `Dynamically register, probe, manage, and bridge external services and data service connections.

Supported Service Categories:
  - Database  : postgres, mysql, clickhouse, mongodb, sqlite
  - Cache     : redis, memcached, valkey
  - LLM API   : openai, anthropic, groq, ollama, cohere, bedrock
  - Vector DB : qdrant, pinecone, milvus, weaviate, chroma
  - Queue     : kafka, redpanda, rabbitmq, nats
  - Monitoring: tempo, prometheus, loki, jaeger
  - Custom    : any HTTP, gRPC, or TCP service

Subcommands:
  add             Register any external service or data connection
  list            Display tabular overview of all registered services
  get             View full JSON definition of a service by ID or Name
  test            Run live diagnostic probe (HTTP status or TCP ping)
  update          Update connection parameters or credentials
  delete          Unregister a service
  sync-to-grafana Register a data service directly as a Grafana datasource

Examples:
  llmobs service add OpenAI-API --type openai --url https://api.openai.com/v1 --auth-token $OPENAI_API_KEY
  llmobs service add Ext-Postgres --type postgres --host db.prod.internal --port 5432 --category database
  llmobs service test Ext-Postgres
  llmobs service sync-to-grafana Ext-Postgres
  llmobs service list --category database`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runListServices(cmd.Context(), svcManager, servicesSchema.ServiceFilterOptions{})
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all registered external services",
		RunE: func(cmd *cobra.Command, args []string) error {
			category, _ := cmd.Flags().GetString("category")
			svcType, _ := cmd.Flags().GetString("type")
			query, _ := cmd.Flags().GetString("query")
			tag, _ := cmd.Flags().GetString("tag")

			filter := servicesSchema.ServiceFilterOptions{
				Category: category,
				Type:     svcType,
				Query:    query,
				Tag:      tag,
			}
			return runListServices(cmd.Context(), svcManager, filter)
		},
	}
	listCmd.Flags().StringP("category", "c", "", "Filter by service category (database, cache, llm, vector-db, queue, api, monitoring, custom)")
	listCmd.Flags().StringP("type", "t", "", "Filter by service type (postgres, clickhouse, redis, http, etc.)")
	listCmd.Flags().StringP("query", "q", "", "Search query to filter by name or URL")
	listCmd.Flags().String("tag", "", "Filter by tag")

	getCmd := &cobra.Command{
		Use:   "get <idOrName>",
		Short: "Get details of a registered service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			svc, err := svcManager.GetService(ctx, args[0])
			if err != nil {
				return err
			}
			out, _ := json.MarshalIndent(svc, "", "  ")
			fmt.Println(string(out))
			return nil
		},
	}

	addCmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Register a new external service or data connection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			name := args[0]
			svcType, _ := cmd.Flags().GetString("type")
			category, _ := cmd.Flags().GetString("category")
			rawURL, _ := cmd.Flags().GetString("url")
			host, _ := cmd.Flags().GetString("host")
			port, _ := cmd.Flags().GetInt("port")
			database, _ := cmd.Flags().GetString("database")
			authToken, _ := cmd.Flags().GetString("auth-token")
			authPass, _ := cmd.Flags().GetString("auth-pass")
			authUser, _ := cmd.Flags().GetString("auth-user")
			healthType, _ := cmd.Flags().GetString("health-type")
			healthPath, _ := cmd.Flags().GetString("health-path")
			expectedStatus, _ := cmd.Flags().GetInt("expected-status")
			timeoutSec, _ := cmd.Flags().GetInt("timeout")

			svc := servicesSchema.ServiceDefinition{
				Name:     name,
				Type:     svcType,
				Category: category,
				URL:      rawURL,
				Host:     host,
				Port:     port,
				Database: database,
				HealthCheck: servicesSchema.HealthCheckConfig{
					Type:           healthType,
					Path:           healthPath,
					ExpectedStatus: expectedStatus,
					TimeoutSec:     timeoutSec,
				},
				Auth: servicesSchema.AuthConfig{
					Token:    authToken,
					Password: authPass,
					Username: authUser,
				},
			}

			if authToken != "" {
				svc.Auth.Type = "bearer"
			} else if authUser != "" || authPass != "" {
				svc.Auth.Type = "basic"
			}

			created, err := svcManager.RegisterService(ctx, svc)
			if err != nil {
				return err
			}

			fmt.Println("========================================================================================================================================")
			fmt.Printf("✓ Service %q registered successfully (ID: %s, Category: %s, Type: %s)\n", created.Name, created.ID, created.Category, created.Type)
			fmt.Println("========================================================================================================================================")
			return nil
		},
	}
	addCmd.Flags().StringP("type", "t", "custom", "Service type (postgres, clickhouse, redis, mysql, http, grpc, openai, qdrant, custom)")
	addCmd.Flags().StringP("category", "c", "", "Category (database, cache, llm, vector-db, queue, api, monitoring, custom)")
	addCmd.Flags().String("url", "", "Full service URL (e.g. https://api.openai.com/v1, http://ext-db:5432)")
	addCmd.Flags().String("host", "", "Service hostname or IP address")
	addCmd.Flags().IntP("port", "p", 0, "Service port number")
	addCmd.Flags().StringP("database", "d", "", "Target database name (for SQL/NoSQL stores)")
	addCmd.Flags().String("auth-token", "", "Bearer token or API Key")
	addCmd.Flags().String("auth-user", "", "Basic Auth username")
	addCmd.Flags().String("auth-pass", "", "Basic Auth password")
	addCmd.Flags().String("health-type", "", "Health check probe type (http, tcp)")
	addCmd.Flags().String("health-path", "", "HTTP health check endpoint path (e.g. /healthz, /ping)")
	addCmd.Flags().Int("expected-status", 200, "Expected HTTP response status code")
	addCmd.Flags().Int("timeout", 5, "Probe timeout in seconds")

	testCmd := &cobra.Command{
		Use:   "test <idOrName>",
		Short: "Run a live health probe against a registered service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			target := args[0]
			res, err := svcManager.TestServiceHealth(ctx, target)
			if err != nil {
				return err
			}

			statusLabel := res.Status
			if !res.IsHealthy {
				statusLabel = "FAILED (" + res.Status + ")"
			}

			fmt.Println("========================================================================================================================================")
			fmt.Printf(" Service Health Probe: %s (%s)\n", res.Name, res.ServiceID)
			fmt.Println("========================================================================================================================================")
			fmt.Printf("Target     : %s\n", res.Target)
			fmt.Printf("Probe Type : %s\n", res.ProbeType)
			fmt.Printf("Status     : %s\n", statusLabel)
			fmt.Printf("Latency    : %.1fms\n", res.LatencyMs)
			fmt.Printf("Details    : %s\n", res.Message)
			fmt.Printf("Checked At : %s\n", res.CheckedAt.Format("2006-01-02 15:04:05 UTC"))
			fmt.Println("========================================================================================================================================")

			if !res.IsHealthy {
				return fmt.Errorf("service health check failed for %s", res.Name)
			}
			return nil
		},
	}

	deleteCmd := &cobra.Command{
		Use:   "delete <idOrName>",
		Short: "Unregister an external service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			res, err := svcManager.DeleteService(ctx, args[0])
			if err != nil {
				return err
			}
			fmt.Printf("✓ %s\n", res.Message)
			return nil
		},
	}

	syncGrafanaCmd := &cobra.Command{
		Use:   "sync-to-grafana <idOrName>",
		Short: "Register a data service connection as a Grafana datasource",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			target := args[0]
			res, err := svcManager.SyncToGrafana(ctx, target, grafanaSvc)
			if err != nil {
				return err
			}
			fmt.Println("========================================================================================================================================")
			fmt.Printf("✓ %s (Datasource UID: %s, Latency: %.1fms)\n", res.Message, res.DatasourceUID, res.LatencyMs)
			fmt.Println("========================================================================================================================================")
			return nil
		},
	}

	rootServiceCmd.AddCommand(listCmd, getCmd, addCmd, testCmd, deleteCmd, syncGrafanaCmd)
	return rootServiceCmd
}

func runListServices(ctx context.Context, svcManager *servicesService.ServicesService, filter servicesSchema.ServiceFilterOptions) error {
	servicesList, err := svcManager.ListServices(ctx, filter)
	if err != nil {
		return err
	}

	fmt.Println("========================================================================================================================================")
	fmt.Printf(" External Services & Data Connections (%d registered)\n", len(servicesList))
	fmt.Println("========================================================================================================================================")
	fmt.Printf("%-20s %-15s %-15s %-35s %s\n", "NAME", "CATEGORY", "TYPE", "TARGET (URL / HOST)", "PROBE")
	fmt.Println("----------------------------------------------------------------------------------------------------------------------------------------")
	for _, s := range servicesList {
		target := s.URL
		if target == "" {
			if s.Port > 0 {
				target = fmt.Sprintf("%s:%d", s.Host, s.Port)
			} else {
				target = s.Host
			}
		}
		if len(target) > 33 {
			target = target[:30] + "..."
		}
		probeStr := fmt.Sprintf("%s", s.HealthCheck.Type)
		if s.HealthCheck.Path != "" {
			probeStr = fmt.Sprintf("%s (%s)", s.HealthCheck.Type, s.HealthCheck.Path)
		}
		fmt.Printf("%-20s %-15s %-15s %-35s %s\n", s.Name, s.Category, s.Type, target, probeStr)
	}
	fmt.Println("========================================================================================================================================")
	return nil
}
