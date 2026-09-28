/*
Package cmd provides Cobra CLI commands for dynamic Grafana dashboard lifecycle operations.

ALGORITHM BLUEPRINT:
1. NewDashboardCommand: Instantiates parent 'dashboard' (alias: 'dashboards', 'dash') command hierarchy.
2. Subcommands:
   - list: Renders tabular list of all provisioned dashboards from Grafana REST API.
   - get: Retrieves full JSON schema definition of a specific dashboard by UID.
   - import: Dynamically imports dashboard JSON from a local file path, URL, or raw JSON.
   - export: Exports dashboard schema to a specified file or stdout.
   - delete: Removes a dashboard by UID.
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
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	grafanaSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/schema"
	grafanaService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/services"
	grafanaTypes "github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/types"
)

func NewDashboardCommand(grafanaSvc *grafanaService.GrafanaService) *cobra.Command {
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

	rootDashCmd := &cobra.Command{
		Use:     "dashboard [subcommand]",
		Aliases: []string{"dashboards", "dash"},
		Short:   "Manage, import, export, and search Grafana dashboards dynamically",
		Long: `Dynamically import, export, list, search, and delete Grafana dashboards.

Subcommands:
  list      Display tabular overview of all configured dashboards
  get       View JSON definition of a dashboard by UID
  import    Import dashboard from a local JSON file or URL
  export    Export dashboard JSON to a file or stdout
  delete    Remove a dashboard by UID

Examples:
  llmobs dashboard list
  llmobs dashboard import ./dashboards/llm-telemetry.json --overwrite
  llmobs dashboard import https://grafana.com/api/dashboards/1860/revisions/latest/download
  llmobs dashboard export P214B5B846CF3925F --output ./backup-dash.json
  llmobs dashboard delete P214B5B846CF3925F`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runListDashboards(cmd.Context(), grafanaSvc, getClientOpts(), "", "", "")
		},
	}

	rootDashCmd.PersistentFlags().StringVar(&grafanaURL, "grafana-url", "http://localhost:31415", "Grafana server URL")
	rootDashCmd.PersistentFlags().StringVar(&grafanaUser, "grafana-user", "admin", "Grafana admin username")
	rootDashCmd.PersistentFlags().StringVar(&grafanaPass, "grafana-pass", "", "Grafana admin password (default: from .env)")
	rootDashCmd.PersistentFlags().IntVar(&timeoutSec, "timeout", 10, "HTTP timeout in seconds")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all dashboards configured in Grafana",
		RunE: func(cmd *cobra.Command, args []string) error {
			query, _ := cmd.Flags().GetString("query")
			folderUID, _ := cmd.Flags().GetString("folder")
			tag, _ := cmd.Flags().GetString("tag")
			return runListDashboards(cmd.Context(), grafanaSvc, getClientOpts(), query, folderUID, tag)
		},
	}
	listCmd.Flags().String("query", "", "Search query to filter dashboards by title")
	listCmd.Flags().String("folder", "", "Folder UID filter")
	listCmd.Flags().String("tag", "", "Tag filter")

	getCmd := &cobra.Command{
		Use:   "get <uid>",
		Short: "Fetch full JSON definition of a dashboard",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			uid := args[0]
			detail, err := grafanaSvc.GetDashboard(ctx, getClientOpts(), uid)
			if err != nil {
				return err
			}
			out, _ := json.MarshalIndent(detail, "", "  ")
			fmt.Println(string(out))
			return nil
		},
	}

	importCmd := &cobra.Command{
		Use:   "import <file-or-url>",
		Short: "Import a dashboard JSON file or URL into Grafana",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			source := args[0]
			folderUID, _ := cmd.Flags().GetString("folder-uid")
			overwrite, _ := cmd.Flags().GetBool("overwrite")
			titleOverride, _ := cmd.Flags().GetString("title")

			importOpts := grafanaSchema.DashboardImportOptions{
				SourcePathOrURL: source,
				FolderUID:       folderUID,
				Overwrite:       overwrite,
				TitleOverride:   titleOverride,
			}

			fmt.Printf("Importing dashboard from %s...\n", source)
			res, err := grafanaSvc.ImportDashboard(ctx, getClientOpts(), importOpts)
			if err != nil {
				return err
			}

			fmt.Println("========================================================================================================================================")
			fmt.Printf("✓ Dashboard imported successfully (UID: %s, Status: %s, Version: %d, Latency: %.1fms)\n", res.UID, res.Status, res.Version, res.Latency)
			if res.URL != "" {
				fmt.Printf("  Access URL: %s%s\n", grafanaURL, res.URL)
			}
			fmt.Println("========================================================================================================================================")
			return nil
		},
	}
	importCmd.Flags().String("folder-uid", "", "Target Grafana folder UID")
	importCmd.Flags().Bool("overwrite", true, "Overwrite existing dashboard if UID matches")
	importCmd.Flags().String("title", "", "Override dashboard title on import")

	exportCmd := &cobra.Command{
		Use:   "export <uid>",
		Short: "Export a dashboard to a JSON file or stdout",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			uid := args[0]
			outputPath, _ := cmd.Flags().GetString("output")

			detail, err := grafanaSvc.GetDashboard(ctx, getClientOpts(), uid)
			if err != nil {
				return err
			}

			dashJSON, err := json.MarshalIndent(detail.Dashboard, "", "  ")
			if err != nil {
				return err
			}

			if outputPath == "" {
				fmt.Println(string(dashJSON))
				return nil
			}

			dir := filepath.Dir(outputPath)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return err
			}
			if err := os.WriteFile(outputPath, dashJSON, 0644); err != nil {
				return fmt.Errorf("failed to write export file %s: %w", outputPath, err)
			}

			fmt.Printf("✓ Dashboard %s exported successfully to %s\n", uid, outputPath)
			return nil
		},
	}
	exportCmd.Flags().StringP("output", "o", "", "Destination JSON file path (default: stdout)")

	deleteCmd := &cobra.Command{
		Use:   "delete <uid>",
		Short: "Delete a dashboard by UID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			uid := args[0]
			res, err := grafanaSvc.DeleteDashboard(ctx, getClientOpts(), uid)
			if err != nil {
				return err
			}
			fmt.Printf("✓ Dashboard %q (UID: %s) deleted successfully\n", res.Title, uid)
			return nil
		},
	}

	rootDashCmd.AddCommand(listCmd, getCmd, importCmd, exportCmd, deleteCmd)
	return rootDashCmd
}

func runListDashboards(ctx context.Context, grafanaSvc *grafanaService.GrafanaService, opts grafanaTypes.ClientOptions, query, folderUID, tag string) error {
	results, err := grafanaSvc.SearchDashboards(ctx, opts, query, folderUID, tag)
	if err != nil {
		return err
	}

	fmt.Println("========================================================================================================================================")
	fmt.Printf(" Grafana Dashboards (%d found)\n", len(results))
	fmt.Println("========================================================================================================================================")
	fmt.Printf("%-30s %-20s %-25s %s\n", "TITLE", "UID", "FOLDER", "TAGS")
	fmt.Println("----------------------------------------------------------------------------------------------------------------------------------------")
	for _, d := range results {
		folderTitle := d.FolderTitle
		if folderTitle == "" {
			folderTitle = "General"
		}
		tagsStr := ""
		if len(d.Tags) > 0 {
			tagsStr = fmt.Sprintf("[%s]", d.Tags[0])
		}
		title := d.Title
		if len(title) > 28 {
			title = title[:25] + "..."
		}
		fmt.Printf("%-30s %-20s %-25s %s\n", title, d.UID, folderTitle, tagsStr)
	}
	fmt.Println("========================================================================================================================================")
	return nil
}
