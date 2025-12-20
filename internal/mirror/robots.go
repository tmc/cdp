package mirror

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
)

// RobotsParser handles parsing and interpretation of robots.txt files.
type RobotsParser struct {
	baseURL   *url.URL
	userAgent string
	rules     *RobotsRules
}

// NewRobotsParser creates a new robots.txt parser.
func NewRobotsParser(baseURL *url.URL, userAgent string) *RobotsParser {
	if userAgent == "" {
		userAgent = "Mozilla/5.0 (compatible; Churl/1.0)"
	}
	return &RobotsParser{
		baseURL:   baseURL,
		userAgent: userAgent,
	}
}

// RobotsRules represents the parsed robots.txt rules.
type RobotsRules struct {
	UserAgentRules map[string]*UserAgentRules // Keyed by user agent pattern
	Sitemaps       []string                   // Sitemap URLs found in robots.txt
}

// UserAgentRules represents rules for a specific user agent.
type UserAgentRules struct {
	Disallow   []string // Disallow paths
	Allow      []string // Allow paths (override Disallow)
	CrawlDelay int      // Crawl delay in seconds
}

// ParseRobotsTxt parses a robots.txt file from a reader.
func (rp *RobotsParser) ParseRobotsTxt(r io.Reader) (*RobotsRules, error) {
	rules := &RobotsRules{
		UserAgentRules: make(map[string]*UserAgentRules),
		Sitemaps:       make([]string, 0),
	}

	scanner := bufio.NewScanner(r)
	var currentUserAgents []string
	var currentRules *UserAgentRules

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Remove inline comments
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}

		// Parse directive
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		directive := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])

		switch directive {
		case "user-agent":
			// New user-agent block
			if currentRules != nil && len(currentUserAgents) > 0 {
				// Save previous rules
				for _, ua := range currentUserAgents {
					rules.UserAgentRules[strings.ToLower(ua)] = currentRules
				}
			}
			currentUserAgents = []string{value}
			currentRules = &UserAgentRules{
				Disallow: make([]string, 0),
				Allow:    make([]string, 0),
			}

		case "disallow":
			if currentRules != nil {
				currentRules.Disallow = append(currentRules.Disallow, value)
			}

		case "allow":
			if currentRules != nil {
				currentRules.Allow = append(currentRules.Allow, value)
			}

		case "crawl-delay":
			if currentRules != nil {
				var delay int
				fmt.Sscanf(value, "%d", &delay)
				currentRules.CrawlDelay = delay
			}

		case "sitemap":
			rules.Sitemaps = append(rules.Sitemaps, value)
		}
	}

	// Save last user-agent rules
	if currentRules != nil && len(currentUserAgents) > 0 {
		for _, ua := range currentUserAgents {
			rules.UserAgentRules[strings.ToLower(ua)] = currentRules
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read robots.txt: %w", err)
	}

	return rules, nil
}

// IsAllowed checks if a URL is allowed to be crawled according to robots.txt rules.
func (rp *RobotsParser) IsAllowed(urlStr string, rules *RobotsRules) bool {
	if rules == nil {
		return true
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return true
	}

	urlPath := parsedURL.Path
	if urlPath == "" {
		urlPath = "/"
	}

	// Get rules for our user agent
	var agentRules *UserAgentRules

	// Try exact match first
	userAgent := strings.ToLower(rp.userAgent)
	if r, ok := rules.UserAgentRules[userAgent]; ok {
		agentRules = r
	} else {
		// Try wildcard match
		if r, ok := rules.UserAgentRules["*"]; ok {
			agentRules = r
		} else {
			// No rules for this user agent, allowed by default
			return true
		}
	}

	if agentRules == nil {
		return true
	}

	// Check Allow rules first (they take precedence)
	for _, allowPath := range agentRules.Allow {
		if rp.matchesPath(urlPath, allowPath) {
			return true
		}
	}

	// Check Disallow rules
	for _, disallowPath := range agentRules.Disallow {
		if disallowPath == "" {
			// Empty disallow means allow all
			continue
		}
		if rp.matchesPath(urlPath, disallowPath) {
			return false
		}
	}

	// No matching rules, allowed by default
	return true
}

// GetCrawlDelay returns the crawl delay for the user agent.
func (rp *RobotsParser) GetCrawlDelay(rules *RobotsRules) int {
	if rules == nil {
		return 0
	}

	// Try exact match first
	userAgent := strings.ToLower(rp.userAgent)
	if r, ok := rules.UserAgentRules[userAgent]; ok && r.CrawlDelay > 0 {
		return r.CrawlDelay
	}

	// Try wildcard match
	if r, ok := rules.UserAgentRules["*"]; ok && r.CrawlDelay > 0 {
		return r.CrawlDelay
	}

	return 0
}

// GetSitemaps returns sitemap URLs from robots.txt.
func (rp *RobotsParser) GetSitemaps(rules *RobotsRules) []ExtractedLink {
	if rules == nil {
		return nil
	}

	var links []ExtractedLink
	for _, sitemap := range rules.Sitemaps {
		links = append(links, ExtractedLink{
			URL:      sitemap,
			Type:     LinkTypeSitemap,
			Priority: PriorityHigh,
			Source:   "robots.txt",
		})
	}
	return links
}

// matchesPath checks if a URL path matches a robots.txt path pattern.
func (rp *RobotsParser) matchesPath(urlPath, pattern string) bool {
	// Handle wildcard patterns
	if strings.Contains(pattern, "*") {
		return rp.wildcardMatch(urlPath, pattern)
	}

	// Simple prefix match for patterns ending with /
	if strings.HasSuffix(pattern, "/") {
		return strings.HasPrefix(urlPath, pattern)
	}

	// Exact match for patterns ending with $
	if strings.HasSuffix(pattern, "$") {
		pattern = strings.TrimSuffix(pattern, "$")
		return urlPath == pattern
	}

	// Default: prefix match
	return strings.HasPrefix(urlPath, pattern)
}

// wildcardMatch matches a path against a pattern with wildcards.
func (rp *RobotsParser) wildcardMatch(s, pattern string) bool {
	// Simple wildcard matching (* matches any sequence)
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return s == pattern
	}

	// Check if string starts with first part
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]

	// Check middle parts
	for i := 1; i < len(parts)-1; i++ {
		idx := strings.Index(s, parts[i])
		if idx < 0 {
			return false
		}
		s = s[idx+len(parts[i]):]
	}

	// Check if string ends with last part
	return strings.HasSuffix(s, parts[len(parts)-1])
}

// GetRobotsTxtURL returns the robots.txt URL for a base URL.
func GetRobotsTxtURL(baseURL *url.URL) string {
	robotsURL := *baseURL
	robotsURL.Path = "/robots.txt"
	robotsURL.RawQuery = ""
	robotsURL.Fragment = ""
	return robotsURL.String()
}

// ShouldFollowRobotsTxt determines if robots.txt should be respected.
func ShouldFollowRobotsTxt(opts *MirrorOptions) bool {
	if opts == nil {
		return true
	}
	// For now, always respect robots.txt
	// Could add an option to ignore it in the future
	return true
}

// NormalizePath normalizes a URL path for comparison.
func NormalizePath(p string) string {
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return path.Clean(p)
}
