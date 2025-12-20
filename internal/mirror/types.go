package mirror

import (
	"net/url"
	"sync"
	"time"
)

// URLPriority represents the priority of a URL in the queue.
type URLPriority int

const (
	PriorityLow    URLPriority = 0
	PriorityNormal URLPriority = 1
	PriorityHigh   URLPriority = 2
)

// QueueItem represents an item in the URL queue.
type QueueItem struct {
	URL       string
	Priority  URLPriority
	Depth     int
	Parent    string
	Timestamp time.Time
}

// Asset represents a downloaded asset (image, CSS, JS, etc.).
type Asset struct {
	URL          string
	LocalPath    string
	ContentType  string
	Size         int64
	Downloaded   bool
	LastModified time.Time
	ETag         string
	Hash         string // SHA256 hash for deduplication
}

// Route represents a discovered SPA route.
type Route struct {
	Path      string
	Component string
	Method    string // navigate, click, direct
	Parent    string
	Depth     int
	Assets    []string
	Visited   bool
}

// CrawlStats tracks mirroring statistics.
type CrawlStats struct {
	mu                sync.RWMutex
	RoutesDiscovered  int
	RoutesCrawled     int
	AssetsDownloaded  int
	TotalBytes        int64
	StartTime         time.Time
	EndTime           time.Time
	ErrorCount        int
	SkippedCount      int
	DuplicateCount    int
}

// CrawlState maintains the state of the mirroring operation.
type CrawlState struct {
	mu          sync.RWMutex
	BaseURL     *url.URL
	Visited     map[string]bool      // URLs already processed
	Queue       *URLQueue            // URLs to visit
	Assets      map[string]*Asset    // Downloaded assets by URL
	Routes      map[string]*Route    // Discovered SPA routes by path
	MaxDepth    int                  // Maximum recursion depth
	Stats       *CrawlStats          // Statistics
	OutputDir   string               // Output directory for mirror
	Options     *MirrorOptions       // Mirroring options
}

// MirrorOptions configures the mirroring behavior.
type MirrorOptions struct {
	// Depth control
	MaxDepth        int
	NoParent        bool
	SpanHosts       bool

	// Content filtering
	AcceptExtensions []string
	RejectExtensions []string
	AcceptDomains    []string
	RejectDomains    []string
	IncludeDirs      []string
	ExcludeDirs      []string
	AcceptRegex      string
	RejectRegex      string

	// Download control
	Wait           time.Duration
	NoClobber      bool
	Timestamping   bool
	Continue       bool
	LimitRate      int64
	Quota          int64

	// Output control
	DirectoryPrefix string
	NoDirectories   bool
	ForceDirectories bool
	CutDirs         int
	NoHostDirectories bool

	// SPA-specific
	SPAMode           string // auto, react, vue, angular, none
	DiscoverRoutes    bool
	ClickLinks        bool
	MaxClickDepth     int
	WaitForStable     bool
	StabilityTimeout  time.Duration

	// Content capture
	CaptureAPICalls   bool
	CaptureWebSockets bool
	SaveState         string
	Screenshots       bool
	ExtractData       string

	// Link rewriting
	ConvertLinks      bool
	ConvertSPARoutes  bool
	RewriteAPICalls   bool
	GenerateIndex     bool
	BaseHref          string

	// Performance
	MaxConcurrent     int
	MemoryLimit       int64
	DiskCache         int64
	TimeoutMultiplier float64

	// User agent and robots.txt
	UserAgent         string
	RespectRobotsTxt  bool

	// Verbosity
	Verbose           bool
}

// DefaultMirrorOptions returns default mirroring options.
func DefaultMirrorOptions() *MirrorOptions {
	return &MirrorOptions{
		MaxDepth:          0, // Infinite by default
		NoParent:          true,
		SpanHosts:         false,
		Wait:              0,
		NoClobber:         false,
		Timestamping:      false,
		Continue:          false,
		LimitRate:         0,
		Quota:             0,
		DirectoryPrefix:   ".",
		NoDirectories:     false,
		ForceDirectories:  false,
		CutDirs:           0,
		NoHostDirectories: false,
		SPAMode:           "auto",
		DiscoverRoutes:    true,
		ClickLinks:        false,
		MaxClickDepth:     2,
		WaitForStable:     true,
		StabilityTimeout:  30 * time.Second,
		CaptureAPICalls:   false,
		CaptureWebSockets: false,
		Screenshots:       false,
		ConvertLinks:      true,
		ConvertSPARoutes:  false,
		RewriteAPICalls:   false,
		GenerateIndex:     false,
		MaxConcurrent:     10,
		MemoryLimit:       0,
		DiskCache:         0,
		TimeoutMultiplier: 1.0,
		UserAgent:         "Mozilla/5.0 (compatible; Churl/1.0)",
		RespectRobotsTxt:  true,
		Verbose:           false,
	}
}

// UpdateStats safely updates crawl statistics.
func (s *CrawlStats) UpdateStats(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn()
}

// GetStats safely retrieves crawl statistics.
func (s *CrawlStats) GetStats() (discovered, crawled, downloaded int, totalBytes int64, errors, skipped, duplicates int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.RoutesDiscovered, s.RoutesCrawled, s.AssetsDownloaded, s.TotalBytes, s.ErrorCount, s.SkippedCount, s.DuplicateCount
}

// Duration returns the duration of the crawl.
func (s *CrawlStats) Duration() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.EndTime.IsZero() {
		return time.Since(s.StartTime)
	}
	return s.EndTime.Sub(s.StartTime)
}

// IncrementRouteDiscovered increments the routes discovered counter.
func (s *CrawlStats) IncrementRoutesDiscovered() {
	s.UpdateStats(func() {
		s.RoutesDiscovered++
	})
}

// IncrementRoutesCrawled increments the routes crawled counter.
func (s *CrawlStats) IncrementRoutesCrawled() {
	s.UpdateStats(func() {
		s.RoutesCrawled++
	})
}

// IncrementAssetsDownloaded increments the assets downloaded counter.
func (s *CrawlStats) IncrementAssetsDownloaded() {
	s.UpdateStats(func() {
		s.AssetsDownloaded++
	})
}

// AddBytes adds to the total bytes downloaded.
func (s *CrawlStats) AddBytes(n int64) {
	s.UpdateStats(func() {
		s.TotalBytes += n
	})
}

// IncrementErrors increments the error counter.
func (s *CrawlStats) IncrementErrors() {
	s.UpdateStats(func() {
		s.ErrorCount++
	})
}

// IncrementSkipped increments the skipped counter.
func (s *CrawlStats) IncrementSkipped() {
	s.UpdateStats(func() {
		s.SkippedCount++
	})
}

// IncrementDuplicates increments the duplicate counter.
func (s *CrawlStats) IncrementDuplicates() {
	s.UpdateStats(func() {
		s.DuplicateCount++
	})
}
