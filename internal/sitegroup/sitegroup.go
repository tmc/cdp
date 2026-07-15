// Package sitegroup provides stable filesystem grouping keys for network hosts.
package sitegroup

import (
	"net"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// RegistrableDomain returns the public-suffix-plus-one grouping key for host.
// IP literals, single-label hosts, and empty hosts fall back to the normalized
// host itself. An empty host is represented as unknown_domain.
func RegistrableDomain(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return "unknown_domain"
	}
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = parsed
	} else {
		host = strings.Trim(host, "[]")
	}
	if host == "" {
		return "unknown_domain"
	}
	if net.ParseIP(host) != nil {
		return host
	}
	if domain, err := publicsuffix.EffectiveTLDPlusOne(host); err == nil {
		return domain
	}
	return host
}
