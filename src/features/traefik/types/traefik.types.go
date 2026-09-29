/*
Package types provides options and operation results for Traefik ingress management.

ALGORITHM BLUEPRINT:
1. ClientOptions: Runtime parameters for targeting Traefik API (dashboard port, timeout).
2. OperationResult: Unified response model across all mutating actions.
3. Invariants:
   - Zero inline comments inside function bodies.
*/
package types

import "time"

type ClientOptions struct {
	TraefikURL string        `json:"traefikUrl"`
	Timeout    time.Duration `json:"timeout"`
}

type RouterOperationResult struct {
	Name      string  `json:"name"`
	Status    string  `json:"status"`
	Message   string  `json:"message"`
	Success   bool    `json:"success"`
	LatencyMs float64 `json:"latencyMs"`
}
