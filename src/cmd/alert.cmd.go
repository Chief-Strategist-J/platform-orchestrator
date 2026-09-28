/*
Package cmd provides Cobra CLI commands for Grafana Unified Alerting rules and Contact Point notifications.

ALGORITHM BLUEPRINT:
1. NewAlertCommand: Instantiates parent 'alert' (alias: 'alerts', 'alerting') command hierarchy.
2. Subcommands:
   - list: Renders tabular list of all provisioned alert rules from Grafana.
   - get: Retrieves full JSON schema definition of an alert rule by UID.
   - add: Creates or updates an alert rule from a local JSON file or inline payload.
   - delete: Removes an alert rule by UID.
   - contact-point: Manages notification receivers (Slack, Webhooks, Email, PagerDuty, Discord, OpsGenie).
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
	"time"

	"github.com/spf13/cobra"

	grafanaSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/schema"
	grafanaService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/services"
	grafanaTypes "github.com/Chief-Strategist-J/platform-orchestrator/src/features/grafana/types"
)

func NewAlertCommand(grafanaSvc *grafanaService.GrafanaService) *cobra.Command {
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

	rootAlertCmd := &cobra.Command{
		Use:     "alert [subcommand]",
		Aliases: []string{"alerts", "alerting"},
		Short:   "Manage, provision, and verify Grafana alert rules and contact points",
		Long: `Dynamically provision, test, list, and delete Grafana Unified Alerting rules and contact point notification receivers.

Subcommands:
  list            Display tabular overview of all configured alert rules
  get             View details of a specific alert rule by UID
  add             Create or update an alert rule from JSON payload/file
  delete          Remove an alert rule by UID
  contact-point   Manage notification receivers (Slack, Webhook, Email, PagerDuty)

Examples:
  llmobs alert list
  llmobs alert add ./alerts/high-latency-rule.json
  llmobs alert contact-point list
  llmobs alert contact-point add Slack-Alerts --type slack --webhook-url https://hooks.slack.com/services/...
  llmobs alert delete f72a19e8-48b2`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runListAlertRules(cmd.Context(), grafanaSvc, getClientOpts())
		},
	}

	rootAlertCmd.PersistentFlags().StringVar(&grafanaURL, "grafana-url", "http://localhost:31415", "Grafana server URL")
	rootAlertCmd.PersistentFlags().StringVar(&grafanaUser, "grafana-user", "admin", "Grafana admin username")
	rootAlertCmd.PersistentFlags().StringVar(&grafanaPass, "grafana-pass", "", "Grafana admin password (default: from .env)")
	rootAlertCmd.PersistentFlags().IntVar(&timeoutSec, "timeout", 10, "HTTP timeout in seconds")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all alert rules configured in Grafana",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runListAlertRules(cmd.Context(), grafanaSvc, getClientOpts())
		},
	}

	getCmd := &cobra.Command{
		Use:   "get <uid>",
		Short: "Get details of an alert rule by UID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			uid := args[0]
			rule, err := grafanaSvc.GetAlertRule(ctx, getClientOpts(), uid)
			if err != nil {
				return err
			}
			out, _ := json.MarshalIndent(rule, "", "  ")
			fmt.Println(string(out))
			return nil
		},
	}

	addCmd := &cobra.Command{
		Use:   "add <file-or-json>",
		Short: "Create or update an alert rule from a JSON file or inline JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			input := args[0]

			var data []byte
			if _, err := os.Stat(input); err == nil {
				data, err = os.ReadFile(input)
				if err != nil {
					return err
				}
			} else {
				data = []byte(input)
			}

			var rule grafanaSchema.AlertRulePayload
			if err := json.Unmarshal(data, &rule); err != nil {
				return fmt.Errorf("failed to parse alert rule JSON: %w", err)
			}

			res, err := grafanaSvc.CreateOrUpdateAlertRule(ctx, getClientOpts(), rule)
			if err != nil {
				return err
			}

			fmt.Printf("✓ Alert rule %q (UID: %s) configured successfully (Latency: %.1fms)\n", res.Title, res.UID, res.LatencyMs)
			return nil
		},
	}

	deleteCmd := &cobra.Command{
		Use:   "delete <uid>",
		Short: "Delete an alert rule by UID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			uid := args[0]
			res, err := grafanaSvc.DeleteAlertRule(ctx, getClientOpts(), uid)
			if err != nil {
				return err
			}
			fmt.Printf("✓ %s\n", res.Message)
			return nil
		},
	}

	cpCmd := &cobra.Command{
		Use:     "contact-point [subcommand]",
		Aliases: []string{"contact-points", "cp"},
		Short:   "Manage notification contact points (Slack, Webhook, Email, PagerDuty)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runListContactPoints(cmd.Context(), grafanaSvc, getClientOpts())
		},
	}

	cpListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all configured contact points",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runListContactPoints(cmd.Context(), grafanaSvc, getClientOpts())
		},
	}

	cpAddCmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Create a new contact point receiver",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			name := args[0]
			cpType, _ := cmd.Flags().GetString("type")
			uid, _ := cmd.Flags().GetString("uid")
			webhookURL, _ := cmd.Flags().GetString("webhook-url")
			slackURL, _ := cmd.Flags().GetString("slack-url")
			email, _ := cmd.Flags().GetString("email")
			settingsJSON, _ := cmd.Flags().GetString("settings-json")

			settings := make(map[string]interface{})
			if settingsJSON != "" {
				_ = json.Unmarshal([]byte(settingsJSON), &settings)
			}

			if webhookURL != "" {
				settings["url"] = webhookURL
			}
			if slackURL != "" {
				settings["url"] = slackURL
			}
			if email != "" {
				settings["addresses"] = email
			}

			cpPayload := grafanaSchema.ContactPointPayload{
				UID:      uid,
				Name:     name,
				Type:     cpType,
				Settings: settings,
			}

			res, err := grafanaSvc.CreateOrUpdateContactPoint(ctx, getClientOpts(), cpPayload)
			if err != nil {
				return err
			}

			fmt.Printf("✓ Contact point %q configured successfully (Latency: %.1fms)\n", name, res.LatencyMs)
			return nil
		},
	}
	cpAddCmd.Flags().String("type", "webhook", "Contact point type (slack, webhook, email, pagerduty, opsgenie, discord)")
	cpAddCmd.Flags().String("uid", "", "Explicit UID for contact point")
	cpAddCmd.Flags().String("webhook-url", "", "HTTP Webhook endpoint URL")
	cpAddCmd.Flags().String("slack-url", "", "Slack Incoming Webhook URL")
	cpAddCmd.Flags().String("email", "", "Recipient email address(es)")
	cpAddCmd.Flags().String("settings-json", "", "Raw JSON settings object")

	cpDeleteCmd := &cobra.Command{
		Use:   "delete <uid>",
		Short: "Delete a contact point by UID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			uid := args[0]
			res, err := grafanaSvc.DeleteContactPoint(ctx, getClientOpts(), uid)
			if err != nil {
				return err
			}
			fmt.Printf("✓ %s\n", res.Message)
			return nil
		},
	}

	cpTestCmd := &cobra.Command{
		Use:   "test <file-or-json>",
		Short: "Send a test alert notification to a contact point payload",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			input := args[0]
			var data []byte
			if _, err := os.Stat(input); err == nil {
				data, _ = os.ReadFile(input)
			} else {
				data = []byte(input)
			}

			var cp grafanaSchema.ContactPointPayload
			if err := json.Unmarshal(data, &cp); err != nil {
				return fmt.Errorf("failed to parse contact point JSON: %w", err)
			}

			res, err := grafanaSvc.TestContactPoint(ctx, getClientOpts(), cp)
			if err != nil {
				return err
			}

			if res.Success {
				fmt.Printf("✓ %s (Latency: %.1fms)\n", res.Message, res.LatencyMs)
			} else {
				fmt.Printf("✗ %s\n", res.Message)
			}
			return nil
		},
	}

	cpCmd.AddCommand(cpListCmd, cpAddCmd, cpDeleteCmd, cpTestCmd)
	rootAlertCmd.AddCommand(listCmd, getCmd, addCmd, deleteCmd, cpCmd)

	return rootAlertCmd
}

func runListAlertRules(ctx context.Context, grafanaSvc *grafanaService.GrafanaService, opts grafanaTypes.ClientOptions) error {
	rules, err := grafanaSvc.ListAlertRules(ctx, opts)
	if err != nil {
		return err
	}

	fmt.Println("========================================================================================================================================")
	fmt.Printf(" Grafana Alert Rules (%d found)\n", len(rules))
	fmt.Println("========================================================================================================================================")
	fmt.Printf("%-30s %-20s %-20s %-15s %s\n", "TITLE", "UID", "RULE GROUP", "FOLDER UID", "FOR")
	fmt.Println("----------------------------------------------------------------------------------------------------------------------------------------")
	for _, r := range rules {
		title := r.Title
		if len(title) > 28 {
			title = title[:25] + "..."
		}
		fmt.Printf("%-30s %-20s %-20s %-15s %s\n", title, r.UID, r.RuleGroup, r.FolderUID, r.For)
	}
	fmt.Println("========================================================================================================================================")
	return nil
}

func runListContactPoints(ctx context.Context, grafanaSvc *grafanaService.GrafanaService, opts grafanaTypes.ClientOptions) error {
	cps, err := grafanaSvc.ListContactPoints(ctx, opts)
	if err != nil {
		return err
	}

	fmt.Println("========================================================================================================================================")
	fmt.Printf(" Grafana Notification Contact Points (%d found)\n", len(cps))
	fmt.Println("========================================================================================================================================")
	fmt.Printf("%-30s %-20s %-20s %s\n", "NAME", "UID", "TYPE", "SETTINGS PREVIEW")
	fmt.Println("----------------------------------------------------------------------------------------------------------------------------------------")
	for _, cp := range cps {
		settingsBytes, _ := json.Marshal(cp.Settings)
		settingsStr := string(settingsBytes)
		if len(settingsStr) > 40 {
			settingsStr = settingsStr[:37] + "..."
		}
		fmt.Printf("%-30s %-20s %-20s %s\n", cp.Name, cp.UID, cp.Type, settingsStr)
	}
	fmt.Println("========================================================================================================================================")
	return nil
}
