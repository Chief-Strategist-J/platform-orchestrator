/*
Package services provides domain discovery, host file orchestration, and connectivity checks.

ALGORITHM BLUEPRINT (DNSService):
1. Domain Discovery: Parses Traefik dynamic configuration and system defaults to extract all platform domains.
2. Status Aggregation: Combines discovered domains with active /etc/hosts entries to evaluate synchronization status.
3. Synchronized State: Traces and executes atomic hosts file syncing and DNS resolution probes.
4. Invariants:
   - Zero inline comments inside function bodies.
   - Idempotent domain parsing and extraction.
*/
package services

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/dns/rules"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/dns/schema"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/dns/types"
	traefikSchema "github.com/Chief-Strategist-J/platform-orchestrator/src/features/traefik/schema"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/infra/observability"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/paths"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

var (
	hostRuleRegex    = regexp.MustCompile(`(?:Host|HostSNI)\s*\(\s*` + "`" + `([^` + "`" + `]+)` + "`" + `\s*\)`)
	hostRuleAltRegex = regexp.MustCompile(`(?:Host|HostSNI)\s*\(\s*"([^"]+)"\s*\)`)
)

type DNSService struct {
	tracer       ports.TracerPort
	baseDir      string
	resolver     *paths.PathResolver
	hostsService *DNSHostsService
	probeService *DNSProbeService
}

func NewDNSService(tracer ports.TracerPort, baseDir string) *DNSService {
	resolver := paths.NewPathResolver(baseDir)
	return &DNSService{
		tracer:       tracer,
		baseDir:      baseDir,
		resolver:     resolver,
		hostsService: NewDNSHostsService(tracer, resolver),
		probeService: NewDNSProbeService(tracer),
	}
}

func (s *DNSService) DiscoverDomains(ctx context.Context) []string {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "dns.domains.discover", map[string]interface{}{
		"base_dir": s.baseDir,
	})
	defer span.End()

	seen := make(map[string]bool)
	var discovered []string

	for _, d := range s.resolver.GetDNSDomains() {
		norm := rules.NormalizeDomain(d)
		if norm != "" && !seen[norm] {
			seen[norm] = true
			discovered = append(discovered, norm)
		}
	}

	dynCandidates := []string{
		filepath.Join(s.baseDir, "config", "traefik", "dynamic.yml"),
		filepath.Join(s.baseDir, "packages", "platform-orchestrator", "config", "traefik", "dynamic.yml"),
	}
	if cfgDir := s.resolver.ConfigDir(); cfgDir != "" {
		dynCandidates = append([]string{filepath.Join(cfgDir, "traefik", "dynamic.yml")}, dynCandidates...)
	}

	for _, dynPath := range dynCandidates {
		if data, err := os.ReadFile(dynPath); err == nil {
			var dyn traefikSchema.DynamicConfiguration
			if err := yaml.Unmarshal(data, &dyn); err == nil {
				if dyn.HTTP != nil {
					for _, r := range dyn.HTTP.Routers {
						for _, d := range s.extractDomainsFromRule(r.Rule) {
							norm := rules.NormalizeDomain(d)
							if norm != "" && !seen[norm] {
								seen[norm] = true
								discovered = append(discovered, norm)
							}
						}
					}
				}
				if dyn.TCP != nil {
					for _, r := range dyn.TCP.Routers {
						for _, d := range s.extractDomainsFromRule(r.Rule) {
							norm := rules.NormalizeDomain(d)
							if norm != "" && !seen[norm] {
								seen[norm] = true
								discovered = append(discovered, norm)
							}
						}
					}
				}
			}
			break
		}
	}

	sort.Strings(discovered)
	observability.SetAttributes(ctx, map[string]interface{}{
		"discovered.count": len(discovered),
	})

	return discovered
}

func (s *DNSService) ListRecords(ctx context.Context, hostsPath string) ([]schema.DnsRecord, error) {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "dns.records.list", map[string]interface{}{
		"hosts_path": hostsPath,
	})
	defer span.End()

	discovered := s.DiscoverDomains(ctx)
	syncedMap, err := s.hostsService.ReadSyncedDomains(hostsPath)
	if err != nil {
		syncedMap = make(map[string]string)
	}

	defaultIP := s.resolver.GetDNSConfig().DefaultIP
	if defaultIP == "" {
		defaultIP = "127.0.0.1"
	}

	records := make([]schema.DnsRecord, 0, len(discovered))
	for _, domain := range discovered {
		ip, synced := syncedMap[domain]
		targetIP := ip
		if !synced {
			targetIP = defaultIP
		}
		records = append(records, schema.DnsRecord{
			Domain:   domain,
			IP:       targetIP,
			Source:   "traefik/platform",
			IsSynced: synced,
		})
	}

	for domain, ip := range syncedMap {
		found := false
		for _, r := range records {
			if r.Domain == domain {
				found = true
				break
			}
		}
		if !found {
			records = append(records, schema.DnsRecord{
				Domain:   domain,
				IP:       ip,
				Source:   "hosts",
				IsSynced: true,
			})
		}
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].Domain < records[j].Domain
	})

	return records, nil
}

func (s *DNSService) SyncHosts(ctx context.Context, opts types.SyncOptions) (*schema.DnsSyncReport, error) {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "dns.service.sync_hosts", map[string]interface{}{
		"dry_run": opts.DryRun,
	})
	defer span.End()

	domains := s.DiscoverDomains(ctx)
	return s.hostsService.SyncHosts(ctx, domains, opts)
}

func (s *DNSService) CheckResolution(ctx context.Context, domains []string, hostsPath string) []schema.DnsCheckResult {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "dns.service.check_resolution", map[string]interface{}{
		"requested_count": len(domains),
	})
	defer span.End()

	targetDomains := domains
	if len(targetDomains) == 0 {
		targetDomains = s.DiscoverDomains(ctx)
	}

	return s.probeService.CheckDomains(ctx, targetDomains)
}

func (s *DNSService) extractDomainsFromRule(rule string) []string {
	if rule == "" {
		return nil
	}

	var results []string
	matches := hostRuleRegex.FindAllStringSubmatch(rule, -1)
	for _, m := range matches {
		if len(m) > 1 {
			for _, part := range strings.Split(m[1], ",") {
				results = append(results, strings.TrimSpace(part))
			}
		}
	}

	altMatches := hostRuleAltRegex.FindAllStringSubmatch(rule, -1)
	for _, m := range altMatches {
		if len(m) > 1 {
			for _, part := range strings.Split(m[1], ",") {
				results = append(results, strings.TrimSpace(part))
			}
		}
	}

	return results
}
