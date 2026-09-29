/*
Package services provides network resolution probes for domain verification.

ALGORITHM BLUEPRINT (DNSProbeService):
1. Domain Probing: Queries system DNS/hosts resolution for a domain list.
2. Latency Measurement: Measures query round-trip time in milliseconds.
3. Resilience & Tracing: Encapsulates resolution failures with descriptive errors and emits OpenTelemetry spans.
4. Invariants:
   - Zero inline comments inside function bodies.
   - Non-blocking parallel probe execution with bounded concurrency or sequential resolution.
*/
package services

import (
	"context"
	"net"
	"time"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/dns/rules"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/dns/schema"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/infra/observability"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

type DNSProbeService struct {
	tracer ports.TracerPort
}

func NewDNSProbeService(tracer ports.TracerPort) *DNSProbeService {
	return &DNSProbeService{
		tracer: tracer,
	}
}

func (s *DNSProbeService) CheckDomain(ctx context.Context, domain string) schema.DnsCheckResult {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "dns.probe.check", map[string]interface{}{
		"domain": domain,
	})
	defer span.End()

	normalized := rules.NormalizeDomain(domain)
	if err := rules.ValidateDomain(normalized); err != nil {
		return schema.DnsCheckResult{
			Domain:       domain,
			IsResolvable: false,
			Error:        err.Error(),
		}
	}

	start := time.Now()
	resolver := net.DefaultResolver
	addrs, err := resolver.LookupHost(ctx, normalized)
	duration := time.Since(start).Seconds() * 1000.0

	if err != nil || len(addrs) == 0 {
		errMsg := "lookup failed"
		if err != nil {
			errMsg = err.Error()
		}
		observability.SetAttributes(ctx, map[string]interface{}{
			"probe.resolvable": false,
			"probe.error":      errMsg,
		})
		return schema.DnsCheckResult{
			Domain:       normalized,
			IsResolvable: false,
			LatencyMs:    duration,
			Error:        errMsg,
		}
	}

	resolvedIP := addrs[0]
	observability.SetAttributes(ctx, map[string]interface{}{
		"probe.resolvable":  true,
		"probe.resolved_ip": resolvedIP,
		"probe.latency_ms":  duration,
	})

	return schema.DnsCheckResult{
		Domain:       normalized,
		ResolvedIP:   resolvedIP,
		IsResolvable: true,
		LatencyMs:    duration,
	}
}

func (s *DNSProbeService) CheckDomains(ctx context.Context, domains []string) []schema.DnsCheckResult {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "dns.probe.check_all", map[string]interface{}{
		"domains.count": len(domains),
	})
	defer span.End()

	results := make([]schema.DnsCheckResult, 0, len(domains))
	for _, domain := range domains {
		results = append(results, s.CheckDomain(ctx, domain))
	}
	return results
}
