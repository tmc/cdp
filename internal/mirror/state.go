package mirror

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"
)

// NewCrawlState creates a new crawl state for the given base URL and options.
func NewCrawlState(baseURL string, opts *MirrorOptions) (*CrawlState, error) {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	if opts == nil {
		opts = DefaultMirrorOptions()
	}

	state := &CrawlState{
		BaseURL:   parsedURL,
		Visited:   make(map[string]bool),
		Queue:     NewURLQueue(),
		Assets:    make(map[string]*Asset),
		Routes:    make(map[string]*Route),
		MaxDepth:  opts.MaxDepth,
		OutputDir: opts.DirectoryPrefix,
		Options:   opts,
		Stats: &CrawlStats{
			StartTime: time.Now(),
		},
	}

	// Add the base URL to the queue
	state.Queue.Push(baseURL, PriorityHigh, 0, "")

	return state, nil
}

// MarkVisited marks a URL as visited.
func (s *CrawlState) MarkVisited(urlStr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Visited[urlStr] = true
}

// IsVisited checks if a URL has been visited.
func (s *CrawlState) IsVisited(urlStr string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Visited[urlStr]
}

// ShouldVisit determines if a URL should be visited based on options and state.
func (s *CrawlState) ShouldVisit(urlStr string, depth int) bool {
	// Check if already visited
	if s.IsVisited(urlStr) {
		return false
	}

	// Check max depth
	if s.MaxDepth > 0 && depth > s.MaxDepth {
		return false
	}

	// Parse URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	// Check if it's the same host (unless SpanHosts is enabled)
	if !s.Options.SpanHosts && parsedURL.Host != s.BaseURL.Host {
		return false
	}

	// Check NoParent option
	if s.Options.NoParent && !strings.HasPrefix(parsedURL.Path, s.BaseURL.Path) {
		return false
	}

	// Check domain filters
	if len(s.Options.AcceptDomains) > 0 {
		accepted := false
		for _, domain := range s.Options.AcceptDomains {
			if strings.Contains(parsedURL.Host, domain) {
				accepted = true
				break
			}
		}
		if !accepted {
			return false
		}
	}

	if len(s.Options.RejectDomains) > 0 {
		for _, domain := range s.Options.RejectDomains {
			if strings.Contains(parsedURL.Host, domain) {
				return false
			}
		}
	}

	// Check directory filters
	if len(s.Options.IncludeDirs) > 0 {
		included := false
		for _, dir := range s.Options.IncludeDirs {
			if strings.HasPrefix(parsedURL.Path, dir) {
				included = true
				break
			}
		}
		if !included {
			return false
		}
	}

	if len(s.Options.ExcludeDirs) > 0 {
		for _, dir := range s.Options.ExcludeDirs {
			if strings.HasPrefix(parsedURL.Path, dir) {
				return false
			}
		}
	}

	// Check extension filters
	ext := path.Ext(parsedURL.Path)
	if ext != "" {
		ext = strings.TrimPrefix(ext, ".")

		if len(s.Options.AcceptExtensions) > 0 {
			accepted := false
			for _, acceptExt := range s.Options.AcceptExtensions {
				if ext == acceptExt {
					accepted = true
					break
				}
			}
			if !accepted {
				return false
			}
		}

		if len(s.Options.RejectExtensions) > 0 {
			for _, rejectExt := range s.Options.RejectExtensions {
				if ext == rejectExt {
					return false
				}
			}
		}
	}

	return true
}

// AddAsset registers an asset in the registry.
func (s *CrawlState) AddAsset(urlStr, localPath, contentType string, size int64) *Asset {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if asset already exists
	if existing, ok := s.Assets[urlStr]; ok {
		return existing
	}

	asset := &Asset{
		URL:         urlStr,
		LocalPath:   localPath,
		ContentType: contentType,
		Size:        size,
		Downloaded:  false,
	}

	s.Assets[urlStr] = asset
	return asset
}

// GetAsset retrieves an asset from the registry.
func (s *CrawlState) GetAsset(urlStr string) (*Asset, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	asset, ok := s.Assets[urlStr]
	return asset, ok
}

// MarkAssetDownloaded marks an asset as downloaded with metadata.
func (s *CrawlState) MarkAssetDownloaded(urlStr string, lastModified time.Time, etag, hash string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if asset, ok := s.Assets[urlStr]; ok {
		asset.Downloaded = true
		asset.LastModified = lastModified
		asset.ETag = etag
		asset.Hash = hash
		s.Stats.IncrementAssetsDownloaded()
		s.Stats.AddBytes(asset.Size)
	}
}

// FindAssetByHash finds an asset by its content hash (for deduplication).
func (s *CrawlState) FindAssetByHash(hash string) *Asset {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, asset := range s.Assets {
		if asset.Hash == hash {
			return asset
		}
	}
	return nil
}

// AddRoute registers a discovered SPA route.
func (s *CrawlState) AddRoute(routePath, component, method, parent string, depth int) *Route {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if route already exists
	if existing, ok := s.Routes[routePath]; ok {
		return existing
	}

	route := &Route{
		Path:      routePath,
		Component: component,
		Method:    method,
		Parent:    parent,
		Depth:     depth,
		Assets:    make([]string, 0),
		Visited:   false,
	}

	s.Routes[routePath] = route
	s.Stats.IncrementRoutesDiscovered()
	return route
}

// GetRoute retrieves a route by path.
func (s *CrawlState) GetRoute(routePath string) (*Route, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	route, ok := s.Routes[routePath]
	return route, ok
}

// MarkRouteVisited marks a route as visited.
func (s *CrawlState) MarkRouteVisited(routePath string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if route, ok := s.Routes[routePath]; ok {
		route.Visited = true
		s.Stats.IncrementRoutesCrawled()
	}
}

// AddRouteAsset adds an asset URL to a route's asset list.
func (s *CrawlState) AddRouteAsset(routePath, assetURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if route, ok := s.Routes[routePath]; ok {
		route.Assets = append(route.Assets, assetURL)
	}
}

// HashContent computes a SHA256 hash of the content for deduplication.
func HashContent(content []byte) string {
	hash := sha256.Sum256(content)
	return fmt.Sprintf("%x", hash)
}

// NormalizeURL normalizes a URL for comparison and storage.
func NormalizeURL(urlStr string) (string, error) {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}

	// Remove fragment
	parsed.Fragment = ""

	// Sort query parameters for consistent comparison
	query := parsed.Query()
	parsed.RawQuery = query.Encode()

	return parsed.String(), nil
}

// GetVisitedCount returns the number of visited URLs.
func (s *CrawlState) GetVisitedCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.Visited)
}

// GetAssetCount returns the number of registered assets.
func (s *CrawlState) GetAssetCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.Assets)
}

// GetRouteCount returns the number of discovered routes.
func (s *CrawlState) GetRouteCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.Routes)
}

// Finalize marks the crawl as complete.
func (s *CrawlState) Finalize() {
	s.Stats.UpdateStats(func() {
		s.Stats.EndTime = time.Now()
	})
}
