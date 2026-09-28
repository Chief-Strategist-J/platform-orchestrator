/*
Package schema defines infrastructure profile definitions, normalization, and categorization rules.

ALGORITHM BLUEPRINT:
1. Profile Enumeration: Canonical constants for stateful, stateless, and service-specific tiers.
2. Normalization Helpers: Enforces trimmed lowercase matching across CLI flags, API bodies, and configuration.
3. Invariants:
   - Profile comparison must always use NormalizeString to prevent case/whitespace discrepancies.
   - Zero inline comments inside function bodies.
*/
package schema

import (
	"strings"
)

const (
	ProfileFull      = "full"
	ProfileStateful  = "stateful"
	ProfileStateless = "stateless"
	ProfileData      = "data"
	ProfileCompute   = "compute"
	ProfileNetwork   = "network"
	ProfileEdge      = "edge"
	ProfileDB        = "db"
	ProfileWorkflows = "workflows"
	ProfileStreaming = "streaming"
	ProfileAnalytics = "analytics"
	ProfileTracing   = "tracing"
	ProfilePortal    = "portal"
	ProfileAll       = "*"
)

func NormalizeString(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func NormalizeProfiles(profiles []string) []string {
	res := make([]string, 0, len(profiles))
	for _, p := range profiles {
		if norm := NormalizeString(p); norm != "" {
			res = append(res, norm)
		}
	}
	return res
}

func IsStatelessProfile(profile string) bool {
	norm := NormalizeString(profile)
	return norm == ProfileStateless || norm == ProfileCompute
}

func IsStatefulProfile(profile string) bool {
	norm := NormalizeString(profile)
	return norm == ProfileStateful || norm == ProfileData
}

func ContainsProfile(profiles []string, target string) bool {
	targetNorm := NormalizeString(target)
	for _, p := range profiles {
		if NormalizeString(p) == targetNorm {
			return true
		}
	}
	return false
}
