/*
Package cmd provides Cobra CLI commands for DNS discovery, /etc/hosts synchronization, and resolution diagnostics.

ALGORITHM BLUEPRINT:
1. NewDNSCommand: Instantiates parent 'dns' (aliases: 'hosts', 'domains') command hierarchy.
2. Subcommands:
   - list: Displays all discovered platform domains, target IPs, and sync status.
   - sync: Updates /etc/hosts within demarcated LLMObs blocks.
   - check: Measures latency and verifies live resolution of platform domains.
   - discover: Prints raw discovered domains from Traefik and platform definitions.
3. Invariants:
   - Zero inline comments inside function bodies.
   - Non-zero exit code on critical failures.
*/
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	dnsService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/dns/services"
	dnsTypes "github.com/Chief-Strategist-J/platform-orchestrator/src/features/dns/types"
)

func NewDNSCommand(svc *dnsService.DNSService) *cobra.Command {
	var hostsPath string

	rootCmd := &cobra.Command{
		Use:     "dns [subcommand]",
		Aliases: []string{"hosts", "domains"},
		Short:   "Manage and verify local DNS domain resolution and /etc/hosts records",
	}

	rootCmd.PersistentFlags().StringVar(&hostsPath, "hosts-file", "", "Target hosts file path (default: /etc/hosts)")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all platform domain mappings and their synchronization status",
		RunE: func(cmd *cobra.Command, args []string) error {
			records, err := svc.ListRecords(context.Background(), hostsPath)
			if err != nil {
				return err
			}

			if outputJSON, _ := cmd.Flags().GetBool("json"); outputJSON {
				data, _ := json.MarshalIndent(records, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			fmt.Fprintf(os.Stdout, "\n%-35s %-16s %-12s %s\n", "DOMAIN", "IP ADDRESS", "SYNCED", "SOURCE")
			fmt.Fprintf(os.Stdout, "%s\n", "-----------------------------------------------------------------------------")
			for _, r := range records {
				syncStatus := "❌ No"
				if r.IsSynced {
					syncStatus = "✅ Yes"
				}
				fmt.Fprintf(os.Stdout, "%-35s %-16s %-12s %s\n", r.Domain, r.IP, syncStatus, r.Source)
			}
			fmt.Println()
			return nil
		},
	}
	listCmd.Flags().Bool("json", false, "Output results as JSON")

	var targetIP string
	var dryRun bool
	var customDomains []string

	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Synchronize platform domains to /etc/hosts",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := dnsTypes.SyncOptions{
				TargetIP:      targetIP,
				DryRun:        dryRun,
				CustomDomains: customDomains,
				HostsFilePath: hostsPath,
			}

			report, err := svc.SyncHosts(context.Background(), opts)
			if err != nil {
				return err
			}

			if outputJSON, _ := cmd.Flags().GetBool("json"); outputJSON {
				data, _ := json.MarshalIndent(report, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			if report.Success {
				fmt.Fprintf(os.Stdout, "\n✅ %s\n", report.Message)
				fmt.Fprintf(os.Stdout, "Synced %d domains into %s:\n", report.SyncedCount, report.HostsPath)
				for _, d := range report.Domains {
					fmt.Fprintf(os.Stdout, "  - %s -> %s\n", d, targetIP)
				}
				fmt.Println()
			} else {
				fmt.Fprintf(os.Stderr, "\n❌ %s\n", report.Message)
			}
			return nil
		},
	}
	syncCmd.Flags().StringVar(&targetIP, "ip", "127.0.0.1", "Target IP address for domain mapping")
	syncCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Simulate changes without writing to hosts file")
	syncCmd.Flags().StringSliceVar(&customDomains, "domains", nil, "Additional custom domains to sync")
	syncCmd.Flags().Bool("json", false, "Output results as JSON")

	checkCmd := &cobra.Command{
		Use:     "check [domains...]",
		Aliases: []string{"test", "probe"},
		Short:   "Check live domain name resolution and query latency",
		RunE: func(cmd *cobra.Command, args []string) error {
			results := svc.CheckResolution(context.Background(), args, hostsPath)

			if outputJSON, _ := cmd.Flags().GetBool("json"); outputJSON {
				data, _ := json.MarshalIndent(results, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			fmt.Fprintf(os.Stdout, "\n%-35s %-16s %-10s %-12s %s\n", "DOMAIN", "RESOLVED IP", "STATUS", "LATENCY", "DETAILS")
			fmt.Fprintf(os.Stdout, "%s\n", "---------------------------------------------------------------------------------------------")
			for _, r := range results {
				statusStr := "❌ Failed"
				if r.IsResolvable {
					statusStr = "✅ OK"
				}
				ipStr := r.ResolvedIP
				if ipStr == "" {
					ipStr = "-"
				}
				fmt.Fprintf(os.Stdout, "%-35s %-16s %-10s %-12.2fms %s\n", r.Domain, ipStr, statusStr, r.LatencyMs, r.Error)
			}
			fmt.Println()
			return nil
		},
	}
	checkCmd.Flags().Bool("json", false, "Output results as JSON")

	discoverCmd := &cobra.Command{
		Use:   "discover",
		Short: "Discover platform domains from Traefik configuration and defaults",
		RunE: func(cmd *cobra.Command, args []string) error {
			domains := svc.DiscoverDomains(context.Background())

			if outputJSON, _ := cmd.Flags().GetBool("json"); outputJSON {
				data, _ := json.MarshalIndent(domains, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			fmt.Fprintf(os.Stdout, "\nDiscovered %d platform domains:\n", len(domains))
			for _, d := range domains {
				fmt.Fprintf(os.Stdout, "  - %s\n", d)
			}
			fmt.Println()
			return nil
		},
	}
	discoverCmd.Flags().Bool("json", false, "Output results as JSON")

	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(discoverCmd)

	return rootCmd
}
