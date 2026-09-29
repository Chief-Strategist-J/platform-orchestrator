/*
Package cmd provides Cobra CLI commands for Traefik Ingress Gateway management and routing inspection.

ALGORITHM BLUEPRINT:
1. NewTraefikCommand: Instantiates parent 'traefik' (alias: 'gw', 'gateway') command hierarchy.
2. Subcommands:
   - ping / status: Diagnostic availability test with latency report.
   - overview: Displays active HTTP/TCP router and service counts.
   - entrypoints: Lists port bindings and entrypoints.
   - routers: Lists, retrieves, registers, and deletes HTTP routing rules.
   - services: Lists backend load balancer services.
   - middlewares: Lists active security and rate-limiting middlewares.
   - tcp: Lists, registers, and deletes TCP routing rules.
3. Invariants:
   - Zero inline comments inside function bodies.
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

	traefikSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/schema"
	traefikService "github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/services"
	traefikTypes "github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/types"
)

func NewTraefikCommand(svc *traefikService.TraefikService) *cobra.Command {
	var traefikURL string
	var timeoutSec int

	getClientOpts := func() traefikTypes.ClientOptions {
		return traefikTypes.ClientOptions{
			TraefikURL: traefikURL,
			Timeout:    time.Duration(timeoutSec) * time.Second,
		}
	}

	rootCmd := &cobra.Command{
		Use:     "traefik [subcommand]",
		Aliases: []string{"gateway", "gw"},
		Short:   "Inspect, configure, and manage Traefik Ingress Gateway and dynamic routes",
	}

	rootCmd.PersistentFlags().StringVar(&traefikURL, "url", "", "Traefik management API URL (default: http://localhost:8080)")
	rootCmd.PersistentFlags().IntVar(&timeoutSec, "timeout", 10, "HTTP timeout in seconds")

	pingCmd := &cobra.Command{
		Use:     "ping",
		Aliases: []string{"status", "health"},
		Short:   "Test Traefik Ingress Gateway health and availability",
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := svc.Ping(context.Background(), getClientOpts())
			if err != nil {
				fmt.Printf("✗ Traefik Ingress Gateway is unreachable (%s, %.2fms): %v\n", res.Status, res.LatencyMs, err)
				os.Exit(1)
			}
			fmt.Printf("✓ Traefik Ingress Gateway is healthy (%s, %.2fms)\n", res.Status, res.LatencyMs)
			return nil
		},
	}

	overviewCmd := &cobra.Command{
		Use:   "overview",
		Short: "Display overview summary of active routers, services, and middlewares",
		RunE: func(cmd *cobra.Command, args []string) error {
			overview, err := svc.GetOverview(context.Background(), getClientOpts())
			if err != nil {
				return fmt.Errorf("failed fetching Traefik overview: %w", err)
			}

			fmt.Println("================================================================================")
			fmt.Println("  TRAEFIK INGRESS GATEWAY OVERVIEW")
			fmt.Println("================================================================================")
			fmt.Printf("  HTTP Routers     : %d (warnings: %d, errors: %d)\n", overview.HTTP.Routers.Total, overview.HTTP.Routers.Warnings, overview.HTTP.Routers.Errors)
			fmt.Printf("  HTTP Services    : %d (warnings: %d, errors: %d)\n", overview.HTTP.Services.Total, overview.HTTP.Services.Warnings, overview.HTTP.Services.Errors)
			fmt.Printf("  HTTP Middlewares : %d (warnings: %d, errors: %d)\n", overview.HTTP.Middlewares.Total, overview.HTTP.Middlewares.Warnings, overview.HTTP.Middlewares.Errors)
			fmt.Println("--------------------------------------------------------------------------------")
			fmt.Printf("  TCP Routers      : %d (warnings: %d, errors: %d)\n", overview.TCP.Routers.Total, overview.TCP.Routers.Warnings, overview.TCP.Routers.Errors)
			fmt.Printf("  TCP Services     : %d (warnings: %d, errors: %d)\n", overview.TCP.Services.Total, overview.TCP.Services.Warnings, overview.TCP.Services.Errors)
			fmt.Println("================================================================================")
			return nil
		},
	}

	entrypointsCmd := &cobra.Command{
		Use:     "entrypoints",
		Aliases: []string{"ep"},
		Short:   "List configured network entrypoints",
		RunE: func(cmd *cobra.Command, args []string) error {
			eps, err := svc.ListEntryPoints(context.Background(), getClientOpts())
			if err != nil {
				return fmt.Errorf("failed listing entrypoints: %w", err)
			}

			fmt.Println("------------------------------------------------------------")
			fmt.Printf("%-20s %-30s\n", "NAME", "ADDRESS")
			fmt.Println("------------------------------------------------------------")
			for _, ep := range eps {
				fmt.Printf("%-20s %-30s\n", ep.Name, ep.Address)
			}
			fmt.Println("------------------------------------------------------------")
			return nil
		},
	}

	routerCmd := &cobra.Command{
		Use:     "router [subcommand]",
		Aliases: []string{"routers", "r"},
		Short:   "Manage dynamic HTTP routers",
	}

	routerListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all active HTTP routers",
		RunE: func(cmd *cobra.Command, args []string) error {
			routers, err := svc.ListHTTPRouters(context.Background(), getClientOpts())
			if err != nil {
				return fmt.Errorf("failed listing HTTP routers: %w", err)
			}

			fmt.Println("---------------------------------------------------------------------------------------------------------")
			fmt.Printf("%-25s %-35s %-25s %-15s\n", "NAME", "RULE", "SERVICE", "ENTRYPOINTS")
			fmt.Println("---------------------------------------------------------------------------------------------------------")
			for _, r := range routers {
				eps := strings.Join(r.EntryPoints, ",")
				if eps == "" {
					eps = "websecure"
				}
				fmt.Printf("%-25s %-35s %-25s %-15s\n", r.Name, r.Rule, r.Service, eps)
			}
			fmt.Println("---------------------------------------------------------------------------------------------------------")
			return nil
		},
	}

	routerGetCmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Get HTTP router details by name",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := svc.GetHTTPRouter(context.Background(), getClientOpts(), args[0])
			if err != nil {
				return err
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(r)
		},
	}

	var addRule string
	var addService string
	var addEntryPoints []string
	var addMiddlewares []string

	routerAddCmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Register or update an HTTP router in dynamic configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			routerDef := traefikSchema.HTTPRouterDefinition{
				Name:        args[0],
				Rule:        addRule,
				Service:     addService,
				EntryPoints: addEntryPoints,
				Middlewares: addMiddlewares,
				TLS:         map[string]interface{}{},
			}
			res, err := svc.SaveHTTPRouter(context.Background(), routerDef)
			if err != nil {
				return err
			}
			fmt.Printf("✓ %s (%.2fms)\n", res.Message, res.LatencyMs)
			return nil
		},
	}
	routerAddCmd.Flags().StringVar(&addRule, "rule", "", "Routing rule expression (e.g. Host(`grafana.llmobs.local`))")
	routerAddCmd.Flags().StringVar(&addService, "service", "", "Target backend service name")
	routerAddCmd.Flags().StringSliceVar(&addEntryPoints, "entrypoints", []string{"websecure"}, "EntryPoints list")
	routerAddCmd.Flags().StringSliceVar(&addMiddlewares, "middlewares", nil, "Middleware names to attach")
	_ = routerAddCmd.MarkFlagRequired("rule")
	_ = routerAddCmd.MarkFlagRequired("service")

	routerDeleteCmd := &cobra.Command{
		Use:     "delete <name>",
		Aliases: []string{"rm"},
		Short:   "Delete an HTTP router from dynamic configuration",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := svc.DeleteHTTPRouter(context.Background(), args[0])
			if err != nil {
				return err
			}
			fmt.Printf("✓ %s (%.2fms)\n", res.Message, res.LatencyMs)
			return nil
		},
	}

	routerCmd.AddCommand(routerListCmd, routerGetCmd, routerAddCmd, routerDeleteCmd)

	servicesCmd := &cobra.Command{
		Use:     "services",
		Aliases: []string{"svc"},
		Short:   "List backend load balancer services",
		RunE: func(cmd *cobra.Command, args []string) error {
			svcs, err := svc.ListHTTPServices(context.Background(), getClientOpts())
			if err != nil {
				return fmt.Errorf("failed listing services: %w", err)
			}

			fmt.Println("--------------------------------------------------------------------------------")
			fmt.Printf("%-30s %-15s %-30s\n", "NAME", "STATUS", "SERVERS")
			fmt.Println("--------------------------------------------------------------------------------")
			for _, s := range svcs {
				var servers []string
				if s.LoadBalancer != nil {
					for _, srv := range s.LoadBalancer.Servers {
						if srv.URL != "" {
							servers = append(servers, srv.URL)
						} else if srv.Address != "" {
							servers = append(servers, srv.Address)
						}
					}
				}
				fmt.Printf("%-30s %-15s %-30s\n", s.Name, s.Status, strings.Join(servers, ", "))
			}
			fmt.Println("--------------------------------------------------------------------------------")
			return nil
		},
	}

	middlewaresCmd := &cobra.Command{
		Use:     "middlewares",
		Aliases: []string{"mw"},
		Short:   "List active middlewares",
		RunE: func(cmd *cobra.Command, args []string) error {
			mws, err := svc.ListMiddlewares(context.Background(), getClientOpts())
			if err != nil {
				return fmt.Errorf("failed listing middlewares: %w", err)
			}

			fmt.Println("--------------------------------------------------------------------------------")
			fmt.Printf("%-35s %-25s %-15s\n", "NAME", "TYPE", "STATUS")
			fmt.Println("--------------------------------------------------------------------------------")
			for _, mw := range mws {
				fmt.Printf("%-35s %-25s %-15s\n", mw.Name, mw.Type, mw.Status)
			}
			fmt.Println("--------------------------------------------------------------------------------")
			return nil
		},
	}

	tcpCmd := &cobra.Command{
		Use:   "tcp [subcommand]",
		Short: "Manage TCP/gRPC routing rules",
	}

	tcpListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all active TCP routers",
		RunE: func(cmd *cobra.Command, args []string) error {
			routers, err := svc.ListTCPRouters(context.Background(), getClientOpts())
			if err != nil {
				return fmt.Errorf("failed listing TCP routers: %w", err)
			}

			fmt.Println("--------------------------------------------------------------------------------")
			fmt.Printf("%-25s %-35s %-20s\n", "NAME", "RULE", "SERVICE")
			fmt.Println("--------------------------------------------------------------------------------")
			for _, r := range routers {
				fmt.Printf("%-25s %-35s %-20s\n", r.Name, r.Rule, r.Service)
			}
			fmt.Println("--------------------------------------------------------------------------------")
			return nil
		},
	}

	var tcpRule string
	var tcpService string
	var tcpEntryPoints []string

	tcpAddCmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Register or update a TCP router in dynamic configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tcpDef := traefikSchema.TCPRouterDefinition{
				Name:        args[0],
				Rule:        tcpRule,
				Service:     tcpService,
				EntryPoints: tcpEntryPoints,
				TLS:         map[string]interface{}{},
			}
			res, err := svc.SaveTCPRouter(context.Background(), tcpDef)
			if err != nil {
				return err
			}
			fmt.Printf("✓ %s (%.2fms)\n", res.Message, res.LatencyMs)
			return nil
		},
	}
	tcpAddCmd.Flags().StringVar(&tcpRule, "rule", "", "TCP routing rule (e.g. HostSNI(`alloydb.llmobs.local`))")
	tcpAddCmd.Flags().StringVar(&tcpService, "service", "", "Target backend TCP service name")
	tcpAddCmd.Flags().StringSliceVar(&tcpEntryPoints, "entrypoints", []string{"tcp-db"}, "EntryPoints list")
	_ = tcpAddCmd.MarkFlagRequired("rule")
	_ = tcpAddCmd.MarkFlagRequired("service")

	tcpDeleteCmd := &cobra.Command{
		Use:     "delete <name>",
		Aliases: []string{"rm"},
		Short:   "Delete a TCP router from dynamic configuration",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := svc.DeleteTCPRouter(context.Background(), args[0])
			if err != nil {
				return err
			}
			fmt.Printf("✓ %s (%.2fms)\n", res.Message, res.LatencyMs)
			return nil
		},
	}

	tcpCmd.AddCommand(tcpListCmd, tcpAddCmd, tcpDeleteCmd)

	rootCmd.AddCommand(pingCmd, overviewCmd, entrypointsCmd, routerCmd, servicesCmd, middlewaresCmd, tcpCmd)
	return rootCmd
}
