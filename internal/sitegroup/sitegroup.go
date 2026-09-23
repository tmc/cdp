// Package sitegroup provides stable filesystem grouping keys for network hosts.
package sitegroup

import (
	"net"
	"strings"
)

// Host returns the full hostname grouping key for host: the visited subdomain
// (e.g. notebooklm.google.com), not a parent domain. The host is
// lowercased and any port is stripped. An empty host is represented as
// unknown_domain.
func Host(host string) string {
	host = normalizeHost(host)
	if host == "" {
		return "unknown_domain"
	}
	return host
}

// normalizeHost lowercases host, trims surrounding whitespace, strips any
// port, and removes IPv6 brackets. It returns "" for an empty result.
func normalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return ""
	}
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = parsed
	} else {
		host = strings.Trim(host, "[]")
	}
	return host
}
