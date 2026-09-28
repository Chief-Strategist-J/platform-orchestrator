/*
Package rules implements profile resolution and compose file selection logic as pure data.

ALGORITHM BLUEPRINT:
1. ResolveProfiles: Maps input profile strings against known aliases and resolves inter-profile dependencies (e.g., workflows requires db).
2. SelectComposeFiles: Returns the correct list of compose file paths based on resolved profiles:
   - If 'stateless' or 'compute': appends docker-compose.stateless.yml.
   - If 'stateful' or 'data': appends docker-compose.prod.yml if available.
   - Baseline: always includes docker-compose.yml.
3. Invariants:
   - Requesting 'workflows' automatically injects 'db' to ensure database prerequisites are active.
   - Profile list deduplication is maintained across all resolution branches with string normalization.
   - Zero inline comments inside function bodies.
*/
package rules

import (
	"path/filepath"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/stack/schema"
)

var KnownProfiles = map[string][]string{
	schema.ProfileStateful:  {schema.ProfileStateful, schema.ProfileData},
	schema.ProfileStateless: {schema.ProfileStateless, schema.ProfileCompute},
	schema.ProfileData:      {schema.ProfileData},
	schema.ProfileCompute:   {schema.ProfileCompute},
	schema.ProfileNetwork:   {schema.ProfileNetwork},
	schema.ProfileEdge:      {schema.ProfileEdge},
	schema.ProfileDB:        {schema.ProfileDB},
	schema.ProfileWorkflows: {schema.ProfileWorkflows},
	schema.ProfileStreaming: {schema.ProfileStreaming},
	schema.ProfileAnalytics: {schema.ProfileAnalytics},
	schema.ProfileTracing:   {schema.ProfileTracing},
	schema.ProfilePortal:    {schema.ProfilePortal},
	schema.ProfileFull:      {schema.ProfileFull},
}

func ResolveProfiles(requested []string) []string {
	if len(requested) == 0 {
		return []string{schema.ProfileFull}
	}

	var resolved []string
	seen := make(map[string]bool)

	for _, r := range requested {
		norm := schema.NormalizeString(r)
		if norm == "" {
			continue
		}
		if !seen[norm] {
			resolved = append(resolved, norm)
			seen[norm] = true
		}
	}

	if len(resolved) == 0 {
		return []string{schema.ProfileFull}
	}

	if seen[schema.ProfileWorkflows] && !seen[schema.ProfileDB] {
		resolved = append(resolved, schema.ProfileDB)
		seen[schema.ProfileDB] = true
	}

	return resolved
}

func SelectComposeFiles(baseDir string, profiles []string) []string {
	files := []string{filepath.Join(baseDir, schema.DefaultComposeFile)}

	isStateless := false
	isStateful := false

	for _, p := range profiles {
		if schema.IsStatelessProfile(p) {
			isStateless = true
		}
		if schema.IsStatefulProfile(p) {
			isStateful = true
		}
	}

	if isStateless {
		files = append(files, filepath.Join(baseDir, schema.DefaultStatelessComposeFile))
	} else if isStateful {
		files = append(files, filepath.Join(baseDir, schema.DefaultProdComposeFile))
	}

	return files
}
