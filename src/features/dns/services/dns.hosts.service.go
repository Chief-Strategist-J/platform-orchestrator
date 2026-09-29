/*
Package services implements atomic /etc/hosts domain synchronization and management.

ALGORITHM BLUEPRINT (DNSHostsService):
1. Demarcated Block Management: Updates hosts mappings inside a deterministic '# --- BEGIN LLMOBS PLATFORM DOMAINS ---' block.
2. Safe Atomic Persistence: Reads original file, replaces or appends the block, writes to temporary file, and renames atomically.
3. Backup & Rollback: Saves /etc/hosts.llmobs.bak before modifying target file.
4. Invariants:
   - Zero inline comments inside function bodies.
   - Non-LLMObs lines in the hosts file are strictly preserved unchanged.
*/
package services

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/dns/rules"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/dns/schema"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/dns/types"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/infra/observability"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

const (
	BlockStartMarker = "# --- BEGIN LLMOBS PLATFORM DOMAINS ---"
	BlockEndMarker   = "# --- END LLMOBS PLATFORM DOMAINS ---"
)

type DNSHostsService struct {
	tracer ports.TracerPort
}

func NewDNSHostsService(tracer ports.TracerPort) *DNSHostsService {
	return &DNSHostsService{
		tracer: tracer,
	}
}

func (s *DNSHostsService) GetHostsPath(customPath string) string {
	if customPath != "" {
		return customPath
	}
	return "/etc/hosts"
}

func (s *DNSHostsService) ReadSyncedDomains(hostsPath string) (map[string]string, error) {
	path := s.GetHostsPath(hostsPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	inBlock := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == BlockStartMarker {
			inBlock = true
			continue
		}
		if line == BlockEndMarker {
			inBlock = false
			continue
		}
		if inBlock && line != "" && !strings.HasPrefix(line, "#") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				ip := parts[0]
				for _, domain := range parts[1:] {
					result[rules.NormalizeDomain(domain)] = ip
				}
			}
		}
	}

	return result, nil
}

func (s *DNSHostsService) SyncHosts(ctx context.Context, domains []string, opts types.SyncOptions) (*schema.DnsSyncReport, error) {
	ctx, span := s.tracer.StartSpanWithAttributes(ctx, "dns.hosts.sync", map[string]interface{}{
		"domains.count": len(domains),
		"dry_run":       opts.DryRun,
	})
	defer span.End()

	targetIP := opts.TargetIP
	if targetIP == "" {
		targetIP = "127.0.0.1"
	}
	if err := rules.ValidateIP(targetIP); err != nil {
		observability.RecordError(ctx, err)
		return nil, err
	}

	targetPath := s.GetHostsPath(opts.HostsFilePath)

	var validDomains []string
	seen := make(map[string]bool)
	for _, d := range domains {
		norm := rules.NormalizeDomain(d)
		if norm != "" && !seen[norm] {
			if err := rules.ValidateDomain(norm); err == nil {
				seen[norm] = true
				validDomains = append(validDomains, norm)
			}
		}
	}

	for _, d := range opts.CustomDomains {
		norm := rules.NormalizeDomain(d)
		if norm != "" && !seen[norm] {
			if err := rules.ValidateDomain(norm); err == nil {
				seen[norm] = true
				validDomains = append(validDomains, norm)
			}
		}
	}

	originalData, err := os.ReadFile(targetPath)
	if err != nil {
		if !os.IsNotExist(err) {
			observability.RecordError(ctx, err)
			return nil, fmt.Errorf("failed reading hosts file at %s: %w", targetPath, err)
		}
		originalData = []byte("")
	}

	var cleanedLines []string
	scanner := bufio.NewScanner(bytes.NewReader(originalData))
	inBlock := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == BlockStartMarker {
			inBlock = true
			continue
		}
		if trimmed == BlockEndMarker {
			inBlock = false
			continue
		}
		if !inBlock {
			cleanedLines = append(cleanedLines, line)
		}
	}

	var blockLines []string
	blockLines = append(blockLines, BlockStartMarker)
	for _, d := range validDomains {
		blockLines = append(blockLines, fmt.Sprintf("%-16s %s", targetIP, d))
	}
	blockLines = append(blockLines, BlockEndMarker)

	var finalBuffer bytes.Buffer
	for _, l := range cleanedLines {
		finalBuffer.WriteString(l)
		finalBuffer.WriteString("\n")
	}
	for _, l := range blockLines {
		finalBuffer.WriteString(l)
		finalBuffer.WriteString("\n")
	}

	if opts.DryRun {
		return &schema.DnsSyncReport{
			SyncedCount: len(validDomains),
			HostsPath:   targetPath,
			Domains:     validDomains,
			Message:     fmt.Sprintf("Dry run: %d domains ready to sync into %s", len(validDomains), targetPath),
			Success:     true,
		}, nil
	}

	backupPath := fmt.Sprintf("%s.llmobs.bak", targetPath)
	_ = os.WriteFile(backupPath, originalData, 0644)

	tmpPath := fmt.Sprintf("%s.llmobs.tmp", targetPath)
	if err := os.WriteFile(tmpPath, finalBuffer.Bytes(), 0644); err != nil {
		observability.RecordError(ctx, err)
		return nil, fmt.Errorf("failed writing temporary hosts file: %w (ensure sudo/write permissions)", err)
	}

	if err := os.Rename(tmpPath, targetPath); err != nil {
		observability.RecordError(ctx, err)
		return nil, fmt.Errorf("failed replacing hosts file: %w", err)
	}

	observability.SetAttributes(ctx, map[string]interface{}{
		"synced.count": len(validDomains),
		"hosts.path":   targetPath,
	})

	return &schema.DnsSyncReport{
		SyncedCount: len(validDomains),
		HostsPath:   targetPath,
		Domains:     validDomains,
		Message:     fmt.Sprintf("Successfully synced %d platform domain records to %s", len(validDomains), targetPath),
		Success:     true,
	}, nil
}
