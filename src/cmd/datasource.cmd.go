/*
Package cmd provides Cobra CLI commands for arbitrary Grafana datasource provisioning and lifecycle operations.

ALGORITHM BLUEPRINT:
1. NewDatasourceCommand: Instantiates parent 'datasource' (alias: 'ds', 'configure-datasources') command hierarchy.
2. Subcommands:
   - add: Dynamically registers any arbitrary Grafana datasource (PostgreSQL, ClickHouse, Redis, Tempo, Prometheus, Loki, MySQL, etc.) with custom JSON and SecureJSON configs.
   - list: Renders tabular list of all provisioned datasources from Grafana REST API.
   - get: Retrieves full JSON schema definition of a specific datasource by name or UID.
   - update: Mutates properties or credentials of an existing datasource.
   - delete: De-provisions a datasource by name or UID.
   - test: Triggers live backend connectivity and query health checks.
   - sync: Auto-synchronizes platform service datasources (AlloyDB, ClickHouse, Redis, Tempo) from environment templates.
3. Invariants:
   - Zero inline comments inside function bodies.
   - All flags provide safe defaults and fallbacks to workspace .env.
   - Non-zero exit code on failure.
*/
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	grafanaSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/schema"
	grafanaService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/services"
	grafanaTypes "github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/types"
)

func NewDatasourceCommand(grafanaSvc *grafanaService.GrafanaService) *cobra.Command {
	var grafanaURL string
	var grafanaUser string
	var grafanaPass string
	var timeoutSec int

	getClientOpts := func() grafanaTypes.ClientOptions {
		return grafanaTypes.ClientOptions{
			GrafanaURL: grafanaURL,
			Username:   grafanaUser,
			Password:   grafanaPass,
			Timeout:    time.Duration(timeoutSec) * time.Second,
		}
	}

	rootDsCmd := &cobra.Command{
		Use:     "datasource [subcommand | services...]",
		Aliases: []string{"datasources", "configure-datasources", "ds"},
		Short:   "Manage, provision, and verify Grafana data sources dynamically",
		Long: `Dynamically provision, list, test, update, and delete ANY Grafana datasource.

Supported Datasource Types:
  - PostgreSQL / AlloyDB       : postgres, grafana-postgresql-datasource
  - ClickHouse OLAP Analytics  : grafana-clickhouse-datasource
  - Redis Cache & Spend Ledger : redis-datasource
  - Grafana Tempo Tracing      : tempo
  - Prometheus / Cortex / Mimir: prometheus
  - Loki Log Aggregation       : loki
  - Elasticsearch / OpenSearch : elasticsearch
  - MySQL / MariaDB            : mysql
  - Any custom Grafana plugin  : custom-type

Subcommands:
  add       Register any new datasource with custom connection parameters
  list      Display tabular overview of all configured datasources
  get       View details of a specific datasource by Name or UID
  update    Update connection URL, credentials, or JSON configurations
  delete    Remove a datasource by Name or UID
  test      Run live health verification probe against a datasource
  sync      Batch synchronize platform datasources from environment (Default)

Examples:
  llmobs datasource list
  llmobs datasource add Prometheus --type prometheus --url http://prometheus:9090
  llmobs datasource add CustomPG --type postgres --url db.internal:5432 --user admin --password secret --database analytics
  llmobs datasource test AlloyDB
  llmobs datasource sync alloydb clickhouse redis tempo
  llmobs ds sync --no-test`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			noTest, _ := cmd.Flags().GetBool("no-test")
			servicesFlag, _ := cmd.Flags().GetString("services")

			var services []string
			if len(args) > 0 {
				services = args
			} else if servicesFlag != "" {
				for _, s := range strings.Split(servicesFlag, ",") {
					if t := strings.TrimSpace(s); t != "" {
						services = append(services, t)
					}
				}
			}

			opts := grafanaSchema.DatasourceSyncOptions{
				GrafanaURL:     grafanaURL,
				GrafanaUser:    grafanaUser,
				GrafanaPass:    grafanaPass,
				Services:       services,
				TestConnection: !noTest,
				Timeout:        time.Duration(timeoutSec) * time.Second,
			}

			fmt.Println("========================================================================================================================================")
			fmt.Println(" Grafana Datasource Synchronization Pipeline")
			fmt.Println("========================================================================================================================================")

			report, err := grafanaSvc.SyncDatasources(ctx, opts)
			if err != nil {
				return err
			}

			fmt.Printf("%-20s %-15s %8s   %s\n", "DATASOURCE", "STATUS", "LATENCY", "DETAILS / HEALTH")
			fmt.Println("----------------------------------------------------------------------------------------------------------------------------------------")
			for _, r := range report.Results {
				statusLabel := r.Status
				if !r.IsHealthy {
					statusLabel = "FAILED"
				}
				fmt.Printf("%-20s %-15s %6.1fms   %s\n", r.DatasourceName, statusLabel, r.LatencyMs, r.Message)
			}
			fmt.Println("========================================================================================================================================")
			if report.SuccessCount < report.TotalCount {
				return fmt.Errorf("failed to configure %d of %d datasources", report.TotalCount-report.SuccessCount, report.TotalCount)
			}
			fmt.Printf("✓ Successfully synchronized and verified %d/%d Grafana datasources.\n", report.SuccessCount, report.TotalCount)
			return nil
		},
	}

	rootDsCmd.PersistentFlags().StringVar(&grafanaURL, "grafana-url", "http://localhost:31415", "Grafana server URL")
	rootDsCmd.PersistentFlags().StringVar(&grafanaUser, "grafana-user", "admin", "Grafana admin username")
	rootDsCmd.PersistentFlags().StringVar(&grafanaPass, "grafana-pass", "", "Grafana admin password (default: from .env)")
	rootDsCmd.PersistentFlags().IntVar(&timeoutSec, "timeout", 10, "HTTP timeout in seconds")
	rootDsCmd.Flags().String("services", "", "Comma-separated list of services to synchronize (e.g. alloydb,clickhouse)")
	rootDsCmd.Flags().Bool("no-test", false, "Skip connection health test against Grafana API")

	addCmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Dynamically add any arbitrary Grafana datasource",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			name := args[0]

			dsType, _ := cmd.Flags().GetString("type")
			url, _ := cmd.Flags().GetString("url")
			user, _ := cmd.Flags().GetString("user")
			password, _ := cmd.Flags().GetString("password")
			database, _ := cmd.Flags().GetString("database")
			access, _ := cmd.Flags().GetString("access")
			isDefault, _ := cmd.Flags().GetBool("default")
			basicAuth, _ := cmd.Flags().GetBool("basic-auth")
			jsonDataStr, _ := cmd.Flags().GetString("json-data")
			secureJsonDataStr, _ := cmd.Flags().GetString("secure-json-data")
			noTest, _ := cmd.Flags().GetBool("no-test")

			if dsType == "" {
				return fmt.Errorf("--type is required (e.g. postgres, grafana-clickhouse-datasource, redis-datasource, tempo, prometheus)")
			}
			if url == "" {
				return fmt.Errorf("--url is required (e.g. http://tempo:3200, db:5432)")
			}

			jsonData := make(map[string]interface{})
			if jsonDataStr != "" {
				if err := json.Unmarshal([]byte(jsonDataStr), &jsonData); err != nil {
					return fmt.Errorf("invalid --json-data JSON: %w", err)
				}
			}

			secureJsonData := make(map[string]string)
			if secureJsonDataStr != "" {
				if err := json.Unmarshal([]byte(secureJsonDataStr), &secureJsonData); err != nil {
					return fmt.Errorf("invalid --secure-json-data JSON: %w", err)
				}
			}
			if password != "" {
				secureJsonData["password"] = password
			}

			payload := grafanaSchema.DatasourcePayload{
				Name:           name,
				Type:           dsType,
				URL:            url,
				User:           user,
				Database:       database,
				Access:         access,
				IsDefault:      isDefault,
				BasicAuth:      basicAuth,
				JSONData:       jsonData,
				SecureJSONData: secureJsonData,
			}

			res, err := grafanaSvc.CreateDatasource(ctx, getClientOpts(), payload)
			if err != nil {
				return fmt.Errorf("failed to create datasource: %w", err)
			}

			fmt.Printf("✓ %s (UID: %s, Latency: %.1fms)\n", res.Message, res.DatasourceUID, res.LatencyMs)
			if !noTest && res.DatasourceUID != "" {
				hRes, hErr := grafanaSvc.TestDatasourceHealth(ctx, getClientOpts(), res.DatasourceUID)
				if hErr == nil && hRes != nil && hRes.IsHealthy {
					fmt.Printf("  Connection Health: OK (%s - %.1fms)\n", hRes.Message, hRes.LatencyMs)
				}
			}
			return nil
		},
	}
	addCmd.Flags().String("type", "", "Datasource plugin type (e.g. postgres, grafana-clickhouse-datasource, redis-datasource, tempo, prometheus, loki)")
	addCmd.Flags().String("url", "", "Backend server URL or address")
	addCmd.Flags().String("user", "", "Database or API username")
	addCmd.Flags().String("password", "", "Database or API password")
	addCmd.Flags().String("database", "", "Database name")
	addCmd.Flags().String("access", "proxy", "Access mode (proxy or direct)")
	addCmd.Flags().Bool("default", false, "Set as default datasource")
	addCmd.Flags().Bool("basic-auth", false, "Enable HTTP basic authentication")
	addCmd.Flags().String("json-data", "", "Custom JSONData properties as JSON string (e.g. '{\"sslmode\":\"disable\"}')")
	addCmd.Flags().String("secure-json-data", "", "Custom SecureJSONData secrets as JSON string")
	addCmd.Flags().Bool("no-test", false, "Skip health check after creation")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all configured Grafana data sources",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			list, err := grafanaSvc.ListDatasources(ctx, getClientOpts())
			if err != nil {
				return err
			}

			fmt.Println("========================================================================================================================================")
			fmt.Printf(" Grafana Configured Datasources (%d found)\n", len(list))
			fmt.Println("========================================================================================================================================")
			fmt.Printf("%-20s %-30s %-20s %-30s %-10s %s\n", "NAME", "TYPE", "UID", "URL", "DEFAULT", "ACCESS")
			fmt.Println("----------------------------------------------------------------------------------------------------------------------------------------")
			for _, ds := range list {
				def := "No"
				if ds.IsDefault {
					def = "YES"
				}
				fmt.Printf("%-20s %-30s %-20s %-30s %-10s %s\n", ds.Name, ds.Type, ds.UID, ds.URL, def, ds.Access)
			}
			fmt.Println("========================================================================================================================================")
			return nil
		},
	}

	getCmd := &cobra.Command{
		Use:   "get <nameOrUid>",
		Short: "Display details of a specific datasource by Name or UID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			ds, err := grafanaSvc.GetDatasource(ctx, getClientOpts(), args[0])
			if err != nil {
				return err
			}
			data, _ := json.MarshalIndent(ds, "", "  ")
			fmt.Println(string(data))
			return nil
		},
	}

	updateCmd := &cobra.Command{
		Use:   "update <nameOrUid>",
		Short: "Update connection parameters or credentials for a datasource",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			idOrUid := args[0]

			url, _ := cmd.Flags().GetString("url")
			user, _ := cmd.Flags().GetString("user")
			password, _ := cmd.Flags().GetString("password")
			database, _ := cmd.Flags().GetString("database")
			access, _ := cmd.Flags().GetString("access")
			isDefault, _ := cmd.Flags().GetBool("default")
			jsonDataStr, _ := cmd.Flags().GetString("json-data")
			secureJsonDataStr, _ := cmd.Flags().GetString("secure-json-data")
			noTest, _ := cmd.Flags().GetBool("no-test")

			jsonData := make(map[string]interface{})
			if jsonDataStr != "" {
				if err := json.Unmarshal([]byte(jsonDataStr), &jsonData); err != nil {
					return fmt.Errorf("invalid --json-data JSON: %w", err)
				}
			}

			secureJsonData := make(map[string]string)
			if secureJsonDataStr != "" {
				if err := json.Unmarshal([]byte(secureJsonDataStr), &secureJsonData); err != nil {
					return fmt.Errorf("invalid --secure-json-data JSON: %w", err)
				}
			}
			if password != "" {
				secureJsonData["password"] = password
			}

			payload := grafanaSchema.DatasourcePayload{
				URL:            url,
				User:           user,
				Database:       database,
				Access:         access,
				IsDefault:      isDefault,
				JSONData:       jsonData,
				SecureJSONData: secureJsonData,
			}

			res, err := grafanaSvc.UpdateDatasource(ctx, getClientOpts(), idOrUid, payload)
			if err != nil {
				return fmt.Errorf("failed to update datasource: %w", err)
			}

			fmt.Printf("✓ %s (UID: %s, Latency: %.1fms)\n", res.Message, res.DatasourceUID, res.LatencyMs)
			if !noTest && res.DatasourceUID != "" {
				hRes, hErr := grafanaSvc.TestDatasourceHealth(ctx, getClientOpts(), res.DatasourceUID)
				if hErr == nil && hRes != nil && hRes.IsHealthy {
					fmt.Printf("  Connection Health: OK (%s - %.1fms)\n", hRes.Message, hRes.LatencyMs)
				}
			}
			return nil
		},
	}
	updateCmd.Flags().String("url", "", "New server URL or address")
	updateCmd.Flags().String("user", "", "New username")
	updateCmd.Flags().String("password", "", "New password")
	updateCmd.Flags().String("database", "", "New database name")
	updateCmd.Flags().String("access", "", "New access mode (proxy or direct)")
	updateCmd.Flags().Bool("default", false, "Set as default datasource")
	updateCmd.Flags().String("json-data", "", "Custom JSONData properties as JSON string")
	updateCmd.Flags().String("secure-json-data", "", "Custom SecureJSONData secrets as JSON string")
	updateCmd.Flags().Bool("no-test", false, "Skip health check after update")

	deleteCmd := &cobra.Command{
		Use:   "delete <nameOrUid>",
		Short: "Remove a Grafana datasource by Name or UID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			res, err := grafanaSvc.DeleteDatasource(ctx, getClientOpts(), args[0])
			if err != nil {
				return err
			}
			fmt.Printf("✓ %s\n", res.Message)
			return nil
		},
	}

	testCmd := &cobra.Command{
		Use:   "test <nameOrUid>",
		Short: "Test connection and health of a Grafana datasource",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			res, err := grafanaSvc.TestDatasourceHealth(ctx, getClientOpts(), args[0])
			if err != nil {
				return err
			}

			statusLabel := res.Status
			if !res.IsHealthy {
				statusLabel = "FAILED"
			}
			fmt.Printf("Datasource: %s (UID: %s)\n", res.Name, res.UID)
			fmt.Printf("Status    : %s\n", statusLabel)
			fmt.Printf("Latency   : %.1fms\n", res.LatencyMs)
			fmt.Printf("Evidence  : %s\n", res.Message)

			if !res.IsHealthy {
				os.Exit(1)
			}
			return nil
		},
	}

	syncCmd := &cobra.Command{
		Use:   "sync [services...]",
		Short: "Batch synchronize platform datasources from environment",
		RunE:  rootDsCmd.RunE,
	}
	syncCmd.Flags().String("services", "", "Comma-separated list of services to synchronize")
	syncCmd.Flags().Bool("no-test", false, "Skip connection health test against Grafana API")

	rootDsCmd.AddCommand(
		addCmd,
		listCmd,
		getCmd,
		updateCmd,
		deleteCmd,
		testCmd,
		syncCmd,
	)

	return rootDsCmd
}
