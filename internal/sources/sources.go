// Package sources captures JavaScript and CSS sources (including sourcemapped
// originals) from a Chrome DevTools Protocol session.
package sources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/css"
	"github.com/chromedp/cdproto/debugger"
	"github.com/chromedp/chromedp"
	"github.com/tmc/cdp/internal/scrub"
)

// ScriptInfo holds metadata and source for a parsed script.
type ScriptInfo struct {
	ScriptID     cdp.ScriptID
	URL          string
	SourceMapURL string
	Source       string
	IsModule     bool
	Length       int64
	Hash         string
}

// StyleInfo holds metadata and source for a stylesheet.
type StyleInfo struct {
	StyleSheetID cdp.StyleSheetID
	URL          string
	SourceMapURL string
	Source       string
}

// fetchItem is sent to the background goroutine for incremental capture.
type fetchItem struct {
	scriptID    cdp.ScriptID
	styleSheetID cdp.StyleSheetID
	url         string
	sourceMapURL string
	isStyle     bool
}

// Collector captures JavaScript and CSS sources from a browser session.
type Collector struct {
	mu             sync.Mutex
	scripts        map[cdp.ScriptID]*ScriptInfo
	styles         map[cdp.StyleSheetID]*StyleInfo
	written        map[string]bool // URLs already written to disk
	outputDir      string
	verbose        bool
	scrubber       *scrub.Scrubber
	httpClient     *http.Client
	sourcemapCache map[string]string // URL -> content
	ctx            context.Context   // browser context for CDP calls
	fetchCh        chan fetchItem    // channel for incremental capture
	done           chan struct{}     // closed when background goroutine exits
	incremental    bool             // whether incremental mode is active
}

// New creates a source collector that writes to outputDir.
func New(outputDir string, verbose bool) *Collector {
	return &Collector{
		scripts:        make(map[cdp.ScriptID]*ScriptInfo),
		styles:         make(map[cdp.StyleSheetID]*StyleInfo),
		written:        make(map[string]bool),
		outputDir:      outputDir,
		verbose:        verbose,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		sourcemapCache: make(map[string]string),
	}
}

// Enable activates the Debugger and CSS domains so the browser emits
// scriptParsed and styleSheetAdded events. It also starts a background
// goroutine for incremental source capture (fetch + write as events arrive).
func (c *Collector) Enable(ctx context.Context) error {
	var innerCtx context.Context
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		innerCtx = ctx
		if _, err := debugger.Enable().Do(ctx); err != nil {
			return fmt.Errorf("enable debugger: %w", err)
		}
		if err := css.Enable().Do(ctx); err != nil {
			return fmt.Errorf("enable css: %w", err)
		}
		return nil
	})); err != nil {
		return err
	}
	c.ctx = innerCtx
	c.fetchCh = make(chan fetchItem, 256)
	c.done = make(chan struct{})
	c.incremental = true
	go c.backgroundFetcher()
	return nil
}

// Close stops the background fetcher goroutine. Safe to call multiple times.
func (c *Collector) Close() {
	c.mu.Lock()
	if c.incremental && c.fetchCh != nil {
		close(c.fetchCh)
		c.incremental = false
	}
	c.mu.Unlock()
	if c.done != nil {
		<-c.done
	}
}

// backgroundFetcher reads from fetchCh, fetches sources, and writes to disk.
func (c *Collector) backgroundFetcher() {
	defer close(c.done)
	for item := range c.fetchCh {
		if skipURL(item.url) {
			continue
		}
		if item.isStyle {
			c.fetchAndWriteStyle(item)
		} else {
			c.fetchAndWriteScript(item)
		}
	}
}

func (c *Collector) fetchAndWriteScript(item fetchItem) {
	src, _, err := debugger.GetScriptSource(item.scriptID).Do(c.ctx)
	if err != nil {
		if c.verbose {
			log.Printf("sources: incremental get script %s: %v", item.url, err)
		}
		return
	}

	c.mu.Lock()
	if s, ok := c.scripts[item.scriptID]; ok {
		s.Source = src
	}
	c.mu.Unlock()

	c.writeSourceEntry(item.url, src, item.sourceMapURL)
}

func (c *Collector) fetchAndWriteStyle(item fetchItem) {
	text, err := css.GetStyleSheetText(item.styleSheetID).Do(c.ctx)
	if err != nil {
		if c.verbose {
			log.Printf("sources: incremental get stylesheet %s: %v", item.url, err)
		}
		return
	}

	c.mu.Lock()
	if s, ok := c.styles[item.styleSheetID]; ok {
		s.Source = text
	}
	c.mu.Unlock()

	c.writeSourceEntry(item.url, text, item.sourceMapURL)
}

// writeSourceEntry writes a single source file to disk (with scrubbing and sourcemaps).
func (c *Collector) writeSourceEntry(sourceURL, source, sourceMapURL string) {
	if source == "" {
		return
	}
	origin, relPath := splitURL(sourceURL)
	if origin == "" {
		return
	}

	c.mu.Lock()
	if c.written[sourceURL] {
		c.mu.Unlock()
		return
	}
	c.written[sourceURL] = true
	c.mu.Unlock()

	src := source
	if c.scrubber != nil && c.scrubber.Enabled() {
		src, _ = c.scrubber.ScrubText(src)
	}
	if err := writeFile(filepath.Join(c.outputDir, origin, "_compiled", relPath), src); err != nil {
		if c.verbose {
			log.Printf("sources: write %s: %v", sourceURL, err)
		}
		return
	}
	if c.verbose {
		log.Printf("sources: wrote %s (%d bytes)", sourceURL, len(src))
	}
	if sourceMapURL != "" {
		if _, err := c.writeSourceMap(origin, sourceURL, sourceMapURL, src); err != nil && c.verbose {
			log.Printf("sources: sourcemap for %s: %v", sourceURL, err)
		}
	}
}

// HandleEvent should be registered via chromedp.ListenTarget to receive CDP events.
func (c *Collector) HandleEvent(ev interface{}) {
	switch ev := ev.(type) {
	case *debugger.EventScriptParsed:
		c.mu.Lock()
		c.scripts[ev.ScriptID] = &ScriptInfo{
			ScriptID:     ev.ScriptID,
			URL:          ev.URL,
			SourceMapURL: ev.SourceMapURL,
			IsModule:     ev.IsModule,
			Length:       ev.Length,
			Hash:         ev.Hash,
		}
		incr := c.incremental
		c.mu.Unlock()
		if incr {
			select {
			case c.fetchCh <- fetchItem{
				scriptID:     ev.ScriptID,
				url:          ev.URL,
				sourceMapURL: ev.SourceMapURL,
			}:
			default:
				// Channel full, will be picked up by CaptureAll.
			}
		}
	case *css.EventStyleSheetAdded:
		h := ev.Header
		c.mu.Lock()
		c.styles[h.StyleSheetID] = &StyleInfo{
			StyleSheetID: h.StyleSheetID,
			URL:          h.SourceURL,
			SourceMapURL: h.SourceMapURL,
		}
		incr := c.incremental
		c.mu.Unlock()
		if incr {
			select {
			case c.fetchCh <- fetchItem{
				styleSheetID: h.StyleSheetID,
				url:          h.SourceURL,
				sourceMapURL: h.SourceMapURL,
				isStyle:      true,
			}:
			default:
			}
		}
	}
}

// CaptureAll fetches source content for all recorded scripts and stylesheets.
func (c *Collector) CaptureAll(ctx context.Context) error {
	c.mu.Lock()
	scripts := make([]*ScriptInfo, 0, len(c.scripts))
	for _, s := range c.scripts {
		scripts = append(scripts, s)
	}
	styles := make([]*StyleInfo, 0, len(c.styles))
	for _, s := range c.styles {
		styles = append(styles, s)
	}
	c.mu.Unlock()

	var firstErr error
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		for _, s := range scripts {
			if skipURL(s.URL) {
				continue
			}
			src, _, err := debugger.GetScriptSource(s.ScriptID).Do(ctx)
			if err != nil {
				if c.verbose {
					log.Printf("sources: get script %s (%s): %v", s.ScriptID, s.URL, err)
				}
				if firstErr == nil {
					firstErr = fmt.Errorf("get script source %s: %w", s.URL, err)
				}
				continue
			}
			s.Source = src
		}

		for _, s := range styles {
			if skipURL(s.URL) {
				continue
			}
			text, err := css.GetStyleSheetText(s.StyleSheetID).Do(ctx)
			if err != nil {
				if c.verbose {
					log.Printf("sources: get stylesheet %s (%s): %v", s.StyleSheetID, s.URL, err)
				}
				if firstErr == nil {
					firstErr = fmt.Errorf("get stylesheet source %s: %w", s.URL, err)
				}
				continue
			}
			s.Source = text
		}
		return firstErr
	}))
}

// WriteToDisk writes all captured sources to the output directory.
// Layout: outputDir/origin/_compiled/path for served files,
// outputDir/origin/... for sourcemapped originals.
func (c *Collector) WriteToDisk() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	type entry struct {
		url, source, sourceMapURL string
	}
	var entries []entry
	for _, s := range c.scripts {
		entries = append(entries, entry{s.URL, s.Source, s.SourceMapURL})
	}
	for _, s := range c.styles {
		entries = append(entries, entry{s.URL, s.Source, s.SourceMapURL})
	}

	var wrote int
	var firstErr error
	record := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	var totalRedactions int
	for _, e := range entries {
		if skipURL(e.url) || e.source == "" {
			continue
		}
		if c.written[e.url] {
			continue // already written incrementally
		}
		origin, relPath := splitURL(e.url)
		if origin == "" {
			continue
		}
		src := e.source
		if c.scrubber != nil && c.scrubber.Enabled() {
			var n int
			src, n = c.scrubber.ScrubText(src)
			totalRedactions += n
		}
		record(writeFile(filepath.Join(c.outputDir, origin, "_compiled", relPath), src))
		c.written[e.url] = true
		wrote++
		if e.sourceMapURL != "" {
			n, err := c.writeSourceMap(origin, e.url, e.sourceMapURL, src)
			record(err)
			wrote += n
		}
	}
	if totalRedactions > 0 && c.verbose {
		log.Printf("sources: scrubbed %d secret(s) across files", totalRedactions)
	}
	if c.verbose {
		log.Printf("sources: wrote %d files to %s", wrote, c.outputDir)
	}
	return firstErr
}

// SetScrubber sets a scrubber for redacting secrets before writing to disk.
func (c *Collector) SetScrubber(s *scrub.Scrubber) {
	c.scrubber = s
}

// OutputDir returns the configured output directory.
func (c *Collector) OutputDir() string {
	return c.outputDir
}

// Scripts returns all captured script info. Safe for concurrent use.
func (c *Collector) Scripts() []*ScriptInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*ScriptInfo, 0, len(c.scripts))
	for _, s := range c.scripts {
		out = append(out, s)
	}
	return out
}

// Styles returns all captured style info. Safe for concurrent use.
func (c *Collector) Styles() []*StyleInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*StyleInfo, 0, len(c.styles))
	for _, s := range c.styles {
		out = append(out, s)
	}
	return out
}

// writeSourceMap resolves and writes sourcemapped original files.
// It returns the number of files written.
func (c *Collector) writeSourceMap(origin, sourceURL, sourceMapURL, compiledSource string) (int, error) {
	mapContent, mapRelPath, err := c.fetchSourceMap(sourceURL, sourceMapURL)
	if err != nil {
		if c.verbose {
			log.Printf("sources: fetch sourcemap for %s: %v", sourceURL, err)
		}
		return 0, nil // non-fatal
	}

	// Write the sourcemap file itself.
	if mapRelPath != "" {
		smPath := filepath.Join(c.outputDir, origin, "_compiled", mapRelPath)
		if err := writeFile(smPath, mapContent); err != nil {
			return 0, fmt.Errorf("write sourcemap: %w", err)
		}
	}

	originals, err := resolveSourceMap(sourceMapURL, mapContent)
	if err != nil {
		if c.verbose {
			log.Printf("sources: parse sourcemap for %s: %v", sourceURL, err)
		}
		return 0, nil
	}

	var wrote int
	var firstErr error
	for relPath, content := range originals {
		if content == "" {
			continue
		}
		// Clean the path to avoid directory traversal.
		clean := filepath.Clean(relPath)
		if strings.HasPrefix(clean, "..") {
			continue
		}
		dest := filepath.Join(c.outputDir, origin, clean)
		if err := writeFile(dest, content); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		wrote++
	}
	return wrote, firstErr
}

// fetchSourceMap fetches sourcemap content. Handles inline data URIs
// and external URLs (resolved against the source script URL).
func (c *Collector) fetchSourceMap(sourceURL, sourceMapURL string) (content, relPath string, err error) {
	if strings.HasPrefix(sourceMapURL, "data:") {
		data, err := decodeDataURI(sourceMapURL)
		if err != nil {
			return "", "", fmt.Errorf("decode data uri: %w", err)
		}
		return data, "", nil
	}
	absURL := resolveURL(sourceURL, sourceMapURL)
	_, relPath = splitURL(absURL)

	// Check cache.
	c.mu.Lock()
	if cached, ok := c.sourcemapCache[absURL]; ok {
		c.mu.Unlock()
		return cached, relPath, nil
	}
	c.mu.Unlock()

	// Fetch via HTTP.
	resp, err := c.httpClient.Get(absURL)
	if err != nil {
		return "", relPath, fmt.Errorf("fetch sourcemap %s: %w", absURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", relPath, fmt.Errorf("fetch sourcemap %s: %s", absURL, resp.Status)
	}

	// Limit read to 50MB to avoid unbounded memory use.
	const maxSize = 50 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSize))
	if err != nil {
		return "", relPath, fmt.Errorf("read sourcemap %s: %w", absURL, err)
	}

	data := string(body)
	c.mu.Lock()
	c.sourcemapCache[absURL] = data
	c.mu.Unlock()

	if c.verbose {
		log.Printf("sources: fetched sourcemap %s (%d bytes)", absURL, len(body))
	}
	return data, relPath, nil
}

// resolveSourceMap extracts original source files from a sourcemap's
// sources/sourcesContent arrays.
func resolveSourceMap(mapURL, mapContent string) (map[string]string, error) {
	var sm struct {
		Sources        []string `json:"sources"`
		SourcesContent []string `json:"sourcesContent"`
	}
	if err := json.Unmarshal([]byte(mapContent), &sm); err != nil {
		return nil, fmt.Errorf("parse sourcemap: %w", err)
	}
	result := make(map[string]string, len(sm.Sources))
	for i, src := range sm.Sources {
		if i < len(sm.SourcesContent) {
			result[src] = sm.SourcesContent[i]
		}
	}
	return result, nil
}

func skipURL(u string) bool {
	return u == "" || strings.HasPrefix(u, "data:") || strings.HasPrefix(u, "blob:")
}

func splitURL(raw string) (origin, relPath string) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", ""
	}
	origin = u.Host
	relPath = strings.TrimPrefix(u.Path, "/")
	if relPath == "" {
		relPath = "index"
	}
	return origin, relPath
}

func resolveURL(base, ref string) string {
	baseURL, err := url.Parse(base)
	if err != nil {
		return ref
	}
	refURL, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return baseURL.ResolveReference(refURL).String()
}

func decodeDataURI(uri string) (string, error) {
	// Format: data:[mediatype][;base64],<data>
	idx := strings.Index(uri, ",")
	if idx < 0 {
		return "", fmt.Errorf("invalid data uri")
	}
	header := uri[:idx]
	data := uri[idx+1:]
	if strings.Contains(header, ";base64") {
		decoded, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return "", fmt.Errorf("base64 decode: %w", err)
		}
		return string(decoded), nil
	}
	return data, nil
}

func writeFile(fpath, content string) error {
	dir := filepath.Dir(fpath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	return os.WriteFile(fpath, []byte(content), 0644)
}
