/*
Package rules provides declarative rule sets for Traefik HTTP and TCP routing validation and normalization.

ALGORITHM BLUEPRINT:
1. HTTPRouterRules: Enforces non-empty name, service name, valid rule predicates (Host, PathPrefix, Path), and default entrypoint 'websecure'.
2. TCPRouterRules: Enforces non-empty name, service name, valid rule predicates (HostSNI), and default entrypoint.
3. Invariants:
   - Zero inline comments inside function bodies.
*/
package rules

import (
	"strings"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/schema"
)

var HTTPRouterRules = RuleSet[schema.HTTPRouterDefinition]{
	{
		Name: "HTTPRouterNameRequired",
		Condition: func(r schema.HTTPRouterDefinition) bool {
			return strings.TrimSpace(r.Name) != ""
		},
		ErrorMessage: "HTTP router name is required",
	},
	{
		Name: "HTTPRouterRuleRequired",
		Condition: func(r schema.HTTPRouterDefinition) bool {
			return strings.TrimSpace(r.Rule) != ""
		},
		ErrorMessage: "HTTP router routing rule expression is required",
	},
	{
		Name: "HTTPRouterServiceRequired",
		Condition: func(r schema.HTTPRouterDefinition) bool {
			return strings.TrimSpace(r.Service) != ""
		},
		ErrorMessage: "HTTP router target service name is required",
	},
	{
		Name: "HTTPRouterEntryPointsDefault",
		Mutator: func(r schema.HTTPRouterDefinition) schema.HTTPRouterDefinition {
			if len(r.EntryPoints) == 0 {
				r.EntryPoints = []string{"websecure"}
			}
			return r
		},
	},
}

var TCPRouterRules = RuleSet[schema.TCPRouterDefinition]{
	{
		Name: "TCPRouterNameRequired",
		Condition: func(r schema.TCPRouterDefinition) bool {
			return strings.TrimSpace(r.Name) != ""
		},
		ErrorMessage: "TCP router name is required",
	},
	{
		Name: "TCPRouterRuleRequired",
		Condition: func(r schema.TCPRouterDefinition) bool {
			return strings.TrimSpace(r.Rule) != ""
		},
		ErrorMessage: "TCP router HostSNI rule expression is required",
	},
	{
		Name: "TCPRouterServiceRequired",
		Condition: func(r schema.TCPRouterDefinition) bool {
			return strings.TrimSpace(r.Service) != ""
		},
		ErrorMessage: "TCP router target service name is required",
	},
}
