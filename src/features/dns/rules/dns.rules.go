/*
Package rules provides validation and normalization for domain names and IP addresses.

ALGORITHM BLUEPRINT:
1. NormalizeDomain: Lowercases and strips trailing periods and whitespace.
2. ValidateDomain: Ensures valid RFC 1123 hostname syntax.
3. ValidateIP: Verifies IPv4 or IPv6 address formatting.
4. Invariants:
   - Zero inline comments inside function bodies.
*/
package rules

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

var domainRegex = regexp.MustCompile(`^([a-zA-Z0-9_]([a-zA-Z0-9_-]{0,61}[a-zA-Z0-9_])?\.)+[a-zA-Z0-9][a-zA-Z0-9_-]{0,61}[a-zA-Z0-9]$`)

func NormalizeDomain(domain string) string {
	d := strings.TrimSpace(domain)
	d = strings.ToLower(d)
	d = strings.TrimSuffix(d, ".")
	return d
}

func ValidateDomain(domain string) error {
	normalized := NormalizeDomain(domain)
	if normalized == "" {
		return fmt.Errorf("domain name cannot be empty")
	}
	if normalized == "localhost" {
		return nil
	}
	if !domainRegex.MatchString(normalized) {
		return fmt.Errorf("domain %q is not a valid RFC-1123 hostname", domain)
	}
	return nil
}

func ValidateIP(ipStr string) error {
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return fmt.Errorf("invalid IP address format: %q", ipStr)
	}
	return nil
}
