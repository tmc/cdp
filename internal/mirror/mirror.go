// Package mirror implements website mirroring functionality with SPA support.
package mirror

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Mirror coordinates the website mirroring operation.
type Mirror struct {
	ctx           context.Context
	state         *CrawlState
	pathGenerator *PathGenerator
	linkExtractor *LinkExtractor
	sitemapParser *SitemapParser
	robotsParser  *RobotsParser
	robotsRules   *RobotsRules
	verbose       bool
}

// New creates a new Mirror instance for the given URL and options.
func New(ctx context.Context, baseURL string, opts *MirrorOptions) (*Mirror, error) {
	if opts == nil {
		opts = DefaultMirrorOptions()
	}

	state, err := NewCrawlState(baseURL, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create crawl state: %w", err)
	}

	pathGenerator := NewPathGenerator(opts.DirectoryPrefix, opts)
	linkExtractor := NewLinkExtractor(state.BaseURL, opts)
	sitemapParser := NewSitemapParser(baseURL)
	robotsParser := NewRobotsParser(state.BaseURL, opts.UserAgent)

	m := &Mirror{
		ctx:           ctx,
		state:         state,
		pathGenerator: pathGenerator,
		linkExtractor: linkExtractor,
		sitemapParser: sitemapParser,
		robotsParser:  robotsParser,
		verbose:       opts.Verbose,
	}

	return m, nil
}

// Start begins the mirroring operation.
func (m *Mirror) Start() error {
	if m.verbose {
		log.Printf("Starting mirror of %s", m.state.BaseURL.String())
		log.Printf("Output directory: %s", m.state.OutputDir)
		log.Printf("Max depth: %d", m.state.MaxDepth)
	}

	// Create base directory structure
	if err := CreateMirrorStructure(m.state.OutputDir); err != nil {
		return fmt.Errorf("failed to create mirror structure: %w", err)
	}

	// Fetch and parse robots.txt if enabled
	if ShouldFollowRobotsTxt(m.state.Options) {
		if err := m.fetchRobotsTxt(); err != nil {
			if m.verbose {
				log.Printf("Warning: Failed to fetch robots.txt: %v", err)
			}
			// Continue even if robots.txt fails
		}
	}

	// Discover and queue sitemaps
	if err := m.discoverSitemaps(); err != nil {
		if m.verbose {
			log.Printf("Warning: Failed to discover sitemaps: %v", err)
		}
		// Continue even if sitemap discovery fails
	}

	// Start crawling
	if err := m.crawl(); err != nil {
		return fmt.Errorf("crawl failed: %w", err)
	}

	// Finalize the mirror
	m.state.Finalize()

	// Print summary
	if m.verbose {
		m.printSummary()
	}

	return nil
}

// crawl performs the main crawling loop.
func (m *Mirror) crawl() error {
	for !m.state.Queue.IsEmpty() {
		// Check context cancellation
		select {
		case <-m.ctx.Done():
			return m.ctx.Err()
		default:
		}

		// Get next URL from queue
		item := m.state.Queue.Pop()
		if item == nil {
			break
		}

		// Check if we should visit this URL
		if !m.state.ShouldVisit(item.URL, item.Depth) {
			m.state.Stats.IncrementSkipped()
			if m.verbose {
				log.Printf("Skipping: %s (depth: %d)", item.URL, item.Depth)
			}
			continue
		}

		// Mark as visited
		m.state.MarkVisited(item.URL)

		// Process the URL
		if err := m.processURL(item); err != nil {
			m.state.Stats.IncrementErrors()
			if m.verbose {
				log.Printf("Error processing %s: %v", item.URL, err)
			}
			continue
		}

		// Wait between requests if configured
		if m.state.Options.Wait > 0 {
			time.Sleep(m.state.Options.Wait)
		}

		// Check quota
		if m.state.Options.Quota > 0 && m.state.Stats.TotalBytes >= m.state.Options.Quota {
			if m.verbose {
				log.Printf("Quota reached: %d bytes", m.state.Stats.TotalBytes)
			}
			break
		}
	}

	return nil
}

// processURL processes a single URL (downloads and extracts links).
func (m *Mirror) processURL(item *QueueItem) error {
	if m.verbose {
		log.Printf("Processing: %s (depth: %d, priority: %d)", item.URL, item.Depth, item.Priority)
	}

	// Generate local path
	localPath, err := m.pathGenerator.GenerateLocalPath(item.URL)
	if err != nil {
		return fmt.Errorf("failed to generate local path: %w", err)
	}

	// Check if file exists and should not be clobbered
	if m.state.Options.NoClobber {
		if _, err := os.Stat(localPath); err == nil {
			if m.verbose {
				log.Printf("File exists, skipping: %s", localPath)
			}
			return nil
		}
	}

	// Create directory for the file
	if err := m.pathGenerator.CreateDirectory(localPath); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// TODO: Download the content using browser
	// For now, just create a placeholder file
	if err := m.downloadContent(item.URL, localPath); err != nil {
		return fmt.Errorf("failed to download content: %w", err)
	}

	// Read the downloaded content
	content, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("failed to read downloaded content: %w", err)
	}

	// Extract links based on content type
	var links []ExtractedLink
	contentType := detectContentType(localPath, content)

	switch {
	case strings.Contains(contentType, "text/html"):
		links, err = m.linkExtractor.ExtractFromHTML(content, item.URL)
		if err != nil {
			if m.verbose {
				log.Printf("Warning: Failed to extract HTML links from %s: %v", item.URL, err)
			}
		}
	case strings.Contains(contentType, "text/css"):
		links = m.linkExtractor.ExtractFromCSS(content, item.URL)
	case strings.Contains(contentType, "javascript") || strings.Contains(contentType, "application/javascript"):
		links = m.linkExtractor.ExtractFromJS(content, item.URL)
	case strings.Contains(contentType, "xml") && strings.Contains(item.URL, "sitemap"):
		// Parse sitemap
		sitemapLinks, err := m.sitemapParser.ParseSitemap(strings.NewReader(string(content)))
		if err == nil {
			links = sitemapLinks
		}
	}

	// Add discovered links to queue
	for _, link := range links {
		// Check if URL is allowed by robots.txt
		if m.robotsRules != nil && !m.robotsParser.IsAllowed(link.URL, m.robotsRules) {
			if m.verbose {
				log.Printf("Skipping %s (blocked by robots.txt)", link.URL)
			}
			m.state.Stats.IncrementSkipped()
			continue
		}

		// Add to queue with appropriate depth
		nextDepth := item.Depth + 1
		m.AddURL(link.URL, link.Priority, nextDepth, item.URL)
	}

	// Update stats
	m.state.Stats.IncrementRoutesCrawled()

	return nil
}

// downloadContent downloads content from a URL to a local file.
// This is a placeholder that will be replaced with browser-based downloading.
func (m *Mirror) downloadContent(urlStr, localPath string) error {
	// Placeholder implementation
	// In the full implementation, this will use the browser package to:
	// 1. Navigate to the URL
	// 2. Wait for content to load
	// 3. Extract HTML, assets, links
	// 4. Save to local file

	if m.verbose {
		log.Printf("Would download: %s -> %s", urlStr, localPath)
	}

	// Create a minimal placeholder file for now
	content := []byte(fmt.Sprintf("<!-- Placeholder for %s -->\n", urlStr))
	return os.WriteFile(localPath, content, 0644)
}

// AddURL adds a URL to the crawl queue.
func (m *Mirror) AddURL(urlStr string, priority URLPriority, depth int, parent string) bool {
	// Normalize URL
	normalized, err := NormalizeURL(urlStr)
	if err != nil {
		if m.verbose {
			log.Printf("Failed to normalize URL %s: %v", urlStr, err)
		}
		return false
	}

	// Check if we should visit
	if !m.state.ShouldVisit(normalized, depth) {
		return false
	}

	// Add to queue
	return m.state.Queue.Push(normalized, priority, depth, parent)
}

// GetState returns the current crawl state (for inspection/debugging).
func (m *Mirror) GetState() *CrawlState {
	return m.state
}

// printSummary prints a summary of the mirroring operation.
func (m *Mirror) printSummary() {
	discovered, crawled, downloaded, totalBytes, errors, skipped, duplicates := m.state.Stats.GetStats()
	duration := m.state.Stats.Duration()

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("Mirror Summary")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Base URL:           %s\n", m.state.BaseURL.String())
	fmt.Printf("Output Directory:   %s\n", m.state.OutputDir)
	fmt.Printf("Duration:           %s\n", duration)
	fmt.Println()
	fmt.Printf("Routes Discovered:  %d\n", discovered)
	fmt.Printf("Routes Crawled:     %d\n", crawled)
	fmt.Printf("Assets Downloaded:  %d\n", downloaded)
	fmt.Printf("Total Bytes:        %d (%.2f MB)\n", totalBytes, float64(totalBytes)/(1024*1024))
	fmt.Println()
	fmt.Printf("Errors:             %d\n", errors)
	fmt.Printf("Skipped:            %d\n", skipped)
	fmt.Printf("Duplicates:         %d\n", duplicates)
	fmt.Println(strings.Repeat("=", 60))

	// Calculate and display rate
	if duration.Seconds() > 0 {
		rate := float64(totalBytes) / duration.Seconds() / 1024 // KB/s
		fmt.Printf("Average Rate:       %.2f KB/s\n", rate)
		fmt.Println(strings.Repeat("=", 60))
	}
}

// SaveMetadata saves mirror metadata to a JSON file.
func (m *Mirror) SaveMetadata(filename string) error {
	// TODO: Implement metadata saving
	// This will include:
	// - Mirror configuration
	// - Statistics
	// - Route map
	// - Asset registry
	return nil
}

// WriteTo writes mirror progress/status to a writer.
func (m *Mirror) WriteTo(w io.Writer) error {
	discovered, crawled, downloaded, totalBytes, errors, skipped, _ := m.state.Stats.GetStats()
	duration := m.state.Stats.Duration()

	fmt.Fprintf(w, "Progress: %d/%d routes crawled, %d assets downloaded\n",
		crawled, discovered, downloaded)
	fmt.Fprintf(w, "Total: %.2f MB, Duration: %s, Errors: %d, Skipped: %d\n",
		float64(totalBytes)/(1024*1024), duration, errors, skipped)
	fmt.Fprintf(w, "Queue: %d URLs remaining\n", m.state.Queue.Len())

	return nil
}

// fetchRobotsTxt fetches and parses the robots.txt file.
func (m *Mirror) fetchRobotsTxt() error {
	robotsURL := GetRobotsTxtURL(m.state.BaseURL)
	if m.verbose {
		log.Printf("Fetching robots.txt from %s", robotsURL)
	}

	// TODO: Use browser to fetch robots.txt
	// For now, we'll skip this and just note that it should be implemented
	// when browser integration is complete

	// Placeholder: In production, we would:
	// 1. Use HTTP client or browser to fetch robots.txt
	// 2. Parse it with m.robotsParser
	// 3. Store the rules in m.robotsRules
	// 4. Queue any sitemaps found in robots.txt

	return nil
}

// discoverSitemaps discovers and queues sitemap URLs.
func (m *Mirror) discoverSitemaps() error {
	sitemaps := m.sitemapParser.DiscoverSitemaps()

	for _, sitemapURL := range sitemaps {
		if m.verbose {
			log.Printf("Queueing sitemap: %s", sitemapURL)
		}
		m.AddURL(sitemapURL, PriorityHigh, 0, "")
	}

	return nil
}

// detectContentType attempts to detect the content type of a file.
func detectContentType(path string, content []byte) string {
	// First try by extension
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	switch ext {
	case "html", "htm":
		return "text/html"
	case "css":
		return "text/css"
	case "js", "mjs":
		return "application/javascript"
	case "json":
		return "application/json"
	case "xml":
		return "application/xml"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "svg":
		return "image/svg+xml"
	case "webp":
		return "image/webp"
	case "woff":
		return "font/woff"
	case "woff2":
		return "font/woff2"
	case "ttf":
		return "font/ttf"
	case "otf":
		return "font/otf"
	}

	// Fall back to content sniffing for first 512 bytes
	if len(content) > 0 {
		// Simple heuristics
		contentStr := string(content)
		if strings.Contains(contentStr, "<!DOCTYPE") || strings.Contains(contentStr, "<html") {
			return "text/html"
		}
		if strings.HasPrefix(contentStr, "<?xml") {
			return "application/xml"
		}
		if strings.HasPrefix(strings.TrimSpace(contentStr), "{") || strings.HasPrefix(strings.TrimSpace(contentStr), "[") {
			return "application/json"
		}
	}

	return "application/octet-stream"
}
