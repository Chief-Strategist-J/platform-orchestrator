/*
Package types provides operational parameter types for DNS host synchronization and resolution checking.

ALGORITHM BLUEPRINT:
1. SyncOptions: Target IP, custom domain overrides, and dry-run flag.
2. Invariants:
   - Zero inline comments inside function bodies.
*/
package types

type SyncOptions struct {
	TargetIP      string   `json:"targetIp"`
	CustomDomains []string `json:"customDomains,omitempty"`
	DryRun        bool     `json:"dryRun"`
	HostsFilePath string   `json:"hostsFilePath,omitempty"`
}
