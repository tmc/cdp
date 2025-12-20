package mirror

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"time"
)

// SitemapParser handles parsing of sitemap.xml files.
type SitemapParser struct {
	baseURL string
}

// NewSitemapParser creates a new sitemap parser.
func NewSitemapParser(baseURL string) *SitemapParser {
	return &SitemapParser{
		baseURL: baseURL,
	}
}

// Sitemap represents a sitemap.xml file.
type Sitemap struct {
	XMLName xml.Name `xml:"urlset"`
	URLs    []SitemapURL `xml:"url"`
}

// SitemapURL represents a URL entry in a sitemap.
type SitemapURL struct {
	Loc        string  `xml:"loc"`
	LastMod    string  `xml:"lastmod,omitempty"`
	ChangeFreq string  `xml:"changefreq,omitempty"`
	Priority   float64 `xml:"priority,omitempty"`
}

// SitemapIndex represents a sitemap index file.
type SitemapIndex struct {
	XMLName  xml.Name         `xml:"sitemapindex"`
	Sitemaps []SitemapIndexEntry `xml:"sitemap"`
}

// SitemapIndexEntry represents an entry in a sitemap index.
type SitemapIndexEntry struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

// ParseSitemap parses a sitemap.xml file from a reader.
func (sp *SitemapParser) ParseSitemap(r io.Reader) ([]ExtractedLink, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read sitemap: %w", err)
	}

	// Try parsing as sitemap index first
	var sitemapIndex SitemapIndex
	if err := xml.Unmarshal(data, &sitemapIndex); err == nil && len(sitemapIndex.Sitemaps) > 0 {
		return sp.extractFromSitemapIndex(&sitemapIndex), nil
	}

	// Try parsing as regular sitemap
	var sitemap Sitemap
	if err := xml.Unmarshal(data, &sitemap); err != nil {
		return nil, fmt.Errorf("failed to parse sitemap: %w", err)
	}

	return sp.extractFromSitemap(&sitemap), nil
}

// extractFromSitemap extracts links from a sitemap.
func (sp *SitemapParser) extractFromSitemap(sitemap *Sitemap) []ExtractedLink {
	var links []ExtractedLink

	for _, u := range sitemap.URLs {
		if u.Loc == "" {
			continue
		}

		// Determine priority based on sitemap priority and change frequency
		priority := PriorityNormal
		if u.Priority >= 0.8 {
			priority = PriorityHigh
		} else if u.Priority <= 0.3 {
			priority = PriorityLow
		}

		// Boost priority for frequently changing content
		switch strings.ToLower(u.ChangeFreq) {
		case "always", "hourly", "daily":
			priority = PriorityHigh
		case "weekly":
			priority = PriorityNormal
		case "monthly", "yearly":
			priority = PriorityLow
		}

		links = append(links, ExtractedLink{
			URL:      u.Loc,
			Type:     LinkTypeHTML,
			Priority: priority,
			Source:   "sitemap.xml",
		})
	}

	return links
}

// extractFromSitemapIndex extracts sitemap URLs from a sitemap index.
func (sp *SitemapParser) extractFromSitemapIndex(index *SitemapIndex) []ExtractedLink {
	var links []ExtractedLink

	for _, entry := range index.Sitemaps {
		if entry.Loc == "" {
			continue
		}

		links = append(links, ExtractedLink{
			URL:      entry.Loc,
			Type:     LinkTypeSitemap,
			Priority: PriorityHigh, // Sitemaps should be processed early
			Source:   "sitemap-index",
		})
	}

	return links
}

// DiscoverSitemaps returns common sitemap URLs to try.
func (sp *SitemapParser) DiscoverSitemaps() []string {
	base := strings.TrimSuffix(sp.baseURL, "/")
	return []string{
		base + "/sitemap.xml",
		base + "/sitemap_index.xml",
		base + "/sitemap-index.xml",
		base + "/sitemap1.xml",
		base + "/sitemaps/sitemap.xml",
		base + "/sitemap/sitemap.xml",
	}
}

// ParseLastModified parses the lastmod field from a sitemap entry.
func ParseLastModified(lastmod string) (time.Time, error) {
	// Try various date formats
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, lastmod); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse date: %s", lastmod)
}
