/*
Package schema defines data contracts for DNS records, synchronization reports, and domain health probes.

ALGORITHM BLUEPRINT:
1. DnsRecord: Local domain mapping with IP, source discovery origin, and synchronization state.
2. DnsSyncReport: Result of atomic /etc/hosts modification.
3. DnsCheckResult: Live resolution check result for a given domain.
4. Invariants:
   - Zero inline comments inside function bodies.
   - Deterministic JSON marshaling.
*/
package schema

type DnsRecord struct {
	Domain   string `json:"domain"`
	IP       string `json:"ip"`
	Source   string `json:"source"`
	IsSynced bool   `json:"isSynced"`
}

type DnsSyncReport struct {
	SyncedCount int      `json:"syncedCount"`
	HostsPath   string   `json:"hostsPath"`
	Domains     []string `json:"domains"`
	Message     string   `json:"message"`
	Success     bool     `json:"success"`
}

type DnsCheckResult struct {
	Domain       string  `json:"domain"`
	ResolvedIP   string  `json:"resolvedIp,omitempty"`
	IsResolvable bool    `json:"isResolvable"`
	LatencyMs    float64 `json:"latencyMs"`
	Error        string  `json:"error,omitempty"`
}
