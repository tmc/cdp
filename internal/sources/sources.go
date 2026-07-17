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
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/tmc/cdp/internal/scrub"
	"github.com/tmc/cdp/internal/sitegroup"
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
//
// ctx is the target session that emitted the event. ScriptIDs and
// StyleSheetIDs are session-scoped: id 42 in tab A's session refers to a
// different script than id 42 in tab B's session. The fetcher must use
// the same session that observed the event, not whatever session is
// "currently active" at dequeue time.
type fetchItem struct {
	ctx          context.Context
	scriptID     cdp.ScriptID
	styleSheetID cdp.StyleSheetID
	url          string
	sourceMapURL string
	isStyle      bool
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
	fetchContext   context.Context
	fetchCancel    context.CancelFunc
	incremental    bool // whether incremental mode is active
	pageMu         sync.RWMutex
	pageDomain     string // registrable domain of the current top-level page
	groupByPage    bool
}

const sourceFetchTimeout = 5 * time.Second

const sourceShutdownTimeout = sourceFetchTimeout + time.Second

// New creates a source collector that writes to outputDir.
func New(outputDir string, verbose bool) *Collector {
	return &Collector{
		scripts:   make(map[cdp.ScriptID]*ScriptInfo),
		styles:    make(map[cdp.StyleSheetID]*StyleInfo),
		written:   make(map[string]bool),
		outputDir: outputDir,
		verbose:   verbose,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		sourcemapCache: make(map[string]string),
		groupByPage:    true,
	}
}

// SetGroupByPage selects whether source paths use the navigated page's
// registrable domain. The default is enabled.
func (c *Collector) SetGroupByPage(enabled bool) {
	c.pageMu.Lock()
	c.groupByPage = enabled
	c.pageMu.Unlock()
}

// Enable activates the Debugger and CSS domains so the browser emits
// scriptParsed and styleSheetAdded events. It also starts a background
// goroutine for incremental source capture (fetch + write as events arrive).
//
// Callers must register HandleEvent via chromedp.ListenTarget on the
// long-lived browser context BEFORE calling Enable; otherwise the burst
// of scriptParsed events that fires for already-parsed scripts (the
// only signal an attach-mode session ever sees for an idle, fully-loaded
// page) will be missed.
func (c *Collector) Enable(ctx context.Context) error {
	// Create fetchCh and flip incremental BEFORE issuing Debugger.enable,
	// so the replay burst that fires synchronously in the chromedp event
	// loop while we're still blocked in Run is queued, not dropped. (The
	// listener checks c.incremental under c.mu and skips the send when
	// false; an unbuffered or absent channel meant every replayed
	// scriptParsed event was lost for attach-mode pages.)
	c.mu.Lock()
	c.fetchCh = make(chan fetchItem, 256)
	c.done = make(chan struct{})
	c.fetchContext, c.fetchCancel = context.WithCancel(context.Background())
	c.incremental = true
	c.mu.Unlock()
	go c.backgroundFetcher()

	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		if _, err := debugger.Enable().Do(ctx); err != nil {
			return fmt.Errorf("enable debugger: %w", err)
		}
		if err := css.Enable().Do(ctx); err != nil {
			return fmt.Errorf("enable css: %w", err)
		}
		return nil
	})); err != nil {
		// Roll back so Close() doesn't double-close and so a retry by the
		// caller starts from a clean slate.
		c.mu.Lock()
		c.incremental = false
		close(c.fetchCh)
		c.fetchCh = nil
		if c.fetchCancel != nil {
			c.fetchCancel()
			c.fetchCancel = nil
		}
		c.fetchContext = nil
		c.mu.Unlock()
		return err
	}
	c.mu.Lock()
	// Keep the long-lived browser context, not the short-lived action
	// context passed to the function above. The fetcher runs after Enable
	// returns and must create a fresh action context for each CDP call.
	c.ctx = ctx
	c.mu.Unlock()
	return nil
}

// AttachToTarget enables Debugger and CSS on a new target context, so the
// scriptParsed and styleSheetAdded events for that target's already-parsed
// scripts are replayed. Use this after switching to a new target via
// chromedp.NewContext(parent, chromedp.WithTargetID(...)) to receive events
// from the new target's session.
//
// Callers must register a per-target listener via
// chromedp.ListenTarget(ctx, c.Listener(ctx)) on the new target ctx BEFORE
// calling AttachToTarget. chromedp listeners are bound to a specific
// target's session, and the replay burst from Debugger.enable is
// single-shot. The Listener closure carries the ctx so the background
// fetcher uses the right session for GetScriptSource (ScriptIDs are
// per-session, not globally unique).
func (c *Collector) AttachToTarget(ctx context.Context) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		if _, err := debugger.Enable().Do(ctx); err != nil {
			return fmt.Errorf("enable debugger: %w", err)
		}
		if err := css.Enable().Do(ctx); err != nil {
			return fmt.Errorf("enable css: %w", err)
		}
		return nil
	}))
}

// Close stops the background fetcher goroutine. Safe to call multiple times.
func (c *Collector) Close() {
	c.mu.Lock()
	cancel := c.fetchCancel
	done := c.done
	if c.incremental && c.fetchCh != nil {
		close(c.fetchCh)
		c.incremental = false
	}
	c.mu.Unlock()

	if done != nil {
		timer := time.NewTimer(sourceShutdownTimeout)
		select {
		case <-done:
			timer.Stop()
		case <-timer.C:
			if cancel != nil {
				cancel()
			}
			<-done
		}
	}

	c.mu.Lock()
	c.fetchCancel = nil
	c.fetchContext = nil
	c.mu.Unlock()
	if cancel != nil {
		// Release the context even when the fetcher drained normally.
		cancel()
	}
}

func (c *Collector) operationContext(base, cancelFetch context.Context) (context.Context, func()) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(base), sourceFetchTimeout)
	if cancelFetch == nil {
		return ctx, cancel
	}
	stop := context.AfterFunc(cancelFetch, cancel)
	return ctx, func() {
		stop()
		cancel()
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
	ctx := item.ctx
	if ctx == nil {
		c.mu.Lock()
		ctx = c.ctx
		c.mu.Unlock()
	}
	c.mu.Lock()
	fetchContext := c.fetchContext
	c.mu.Unlock()
	opCtx, done := c.operationContext(ctx, fetchContext)
	defer done()
	var src string
	err := chromedp.Run(opCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		src, _, err = debugger.GetScriptSource(item.scriptID).Do(ctx)
		return err
	}))
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
	ctx := item.ctx
	if ctx == nil {
		c.mu.Lock()
		ctx = c.ctx
		c.mu.Unlock()
	}
	c.mu.Lock()
	fetchContext := c.fetchContext
	c.mu.Unlock()
	opCtx, done := c.operationContext(ctx, fetchContext)
	defer done()
	var text string
	err := chromedp.Run(opCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		text, err = css.GetStyleSheetText(item.styleSheetID).Do(ctx)
		return err
	}))
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
	if err := writeFile(c.sourcePath(origin, "_compiled", relPath), src); err != nil {
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

// debugEvents is set from CDP_SOURCES_DEBUG=1 at process start. When true,
// the collector logs every relevant CDP event it receives. Writes go
// directly to os.Stderr (not the log package) so the line can't be
// silently dropped by a log redirect.
var debugEvents = os.Getenv("CDP_SOURCES_DEBUG") == "1"

func debugLogf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "sources: "+format+"\n", args...)
}

// Listener returns an event-handler closure bound to ctx. Pass the result
// to chromedp.ListenTarget. The closure tags every queued fetchItem with
// ctx so the background fetcher uses that target's session for
// Debugger.GetScriptSource and CSS.GetStyleSheetText (both are scoped to
// the session that emitted the event — ScriptIDs/StyleSheetIDs are not
// globally unique).
//
// Use a separate Listener per target context. After a tab switch, register
// a fresh Listener on the new ctx; the prior listener stays bound to its
// original session and continues to handle late events from that target.
func (c *Collector) Listener(ctx context.Context) func(ev any) {
	return func(ev any) {
		c.dispatch(ctx, ev)
	}
}

// HandleEvent is a back-compat shim that uses the collector's Enable-time
// ctx. Prefer Listener(ctx) for new code so per-target session routing is
// preserved across tab switches.
func (c *Collector) HandleEvent(ev any) {
	c.mu.Lock()
	ctx := c.ctx
	c.mu.Unlock()
	c.dispatch(ctx, ev)
}

func (c *Collector) dispatch(ctx context.Context, ev any) {
	if e, ok := ev.(*page.EventFrameNavigated); ok && e.Frame != nil && e.Frame.ParentID == "" {
		c.pageMu.Lock()
		c.pageDomain = sourcePageDomain(e.Frame.URL)
		c.pageMu.Unlock()
		return
	}

	switch ev := ev.(type) {
	case *debugger.EventScriptParsed:
		if debugEvents {
			debugLogf("scriptParsed id=%s url=%s len=%d", ev.ScriptID, ev.URL, ev.Length)
		}
		c.mu.Lock()
		c.scripts[ev.ScriptID] = &ScriptInfo{
			ScriptID:     ev.ScriptID,
			URL:          ev.URL,
			SourceMapURL: ev.SourceMapURL,
			IsModule:     ev.IsModule,
			Length:       ev.Length,
			Hash:         ev.Hash,
		}
		// Send while holding mu: Close closes fetchCh under the same lock,
		// so the send can never hit a just-closed channel. The send is
		// non-blocking, so holding the lock cannot stall event dispatch.
		if c.incremental && c.fetchCh != nil {
			select {
			case c.fetchCh <- fetchItem{
				ctx:          ctx,
				scriptID:     ev.ScriptID,
				url:          ev.URL,
				sourceMapURL: ev.SourceMapURL,
			}:
			default:
				// Channel full, will be picked up by CaptureAll.
			}
		}
		c.mu.Unlock()
	case *css.EventStyleSheetAdded:
		h := ev.Header
		if debugEvents {
			debugLogf("styleSheetAdded id=%s url=%s", h.StyleSheetID, h.SourceURL)
		}
		c.mu.Lock()
		c.styles[h.StyleSheetID] = &StyleInfo{
			StyleSheetID: h.StyleSheetID,
			URL:          h.SourceURL,
			SourceMapURL: h.SourceMapURL,
		}
		if c.incremental && c.fetchCh != nil {
			select {
			case c.fetchCh <- fetchItem{
				ctx:          ctx,
				styleSheetID: h.StyleSheetID,
				url:          h.SourceURL,
				sourceMapURL: h.SourceMapURL,
				isStyle:      true,
			}:
			default:
			}
		}
		c.mu.Unlock()
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
			opCtx, done := c.operationContext(ctx, nil)
			src, _, err := debugger.GetScriptSource(s.ScriptID).Do(opCtx)
			done()
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
			opCtx, done := c.operationContext(ctx, nil)
			text, err := css.GetStyleSheetText(s.StyleSheetID).Do(opCtx)
			done()
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
// Layout: outputDir/page-domain/sources/origin/_compiled/path for served
// files, and outputDir/page-domain/sources/origin/... for sourcemapped
// originals.
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
		record(writeFile(c.sourcePath(origin, "_compiled", relPath), src))
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
		smPath := c.sourcePath(origin, "_compiled", mapRelPath)
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
		dest := c.sourcePath(origin, "", clean)
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

func sourcePageDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "unknown_domain"
	}
	return sitegroup.RegistrableDomain(u.Hostname())
}

func (c *Collector) sourcePath(origin string, parts ...string) string {
	c.pageMu.RLock()
	page := c.pageDomain
	groupByPage := c.groupByPage
	c.pageMu.RUnlock()
	if !groupByPage {
		return filepath.Join(append([]string{c.outputDir, "sources", origin}, parts...)...)
	}
	if page == "" {
		page = "unknown_domain"
	}
	path := filepath.Join(c.outputDir, page, "sources", origin)
	for _, part := range parts {
		if part != "" {
			path = filepath.Join(path, part)
		}
	}
	return path
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
	if err != nil {
		return "", ""
	}
	origin = u.Host
	if origin == "" {
		// Hostless URLs like file:///... still need an output bucket.
		// Fall back to the scheme so all file:// sources group under
		// outputDir/file/_compiled/... and follow the same layout as
		// http(s) origins.
		origin = u.Scheme
	}
	if origin == "" {
		return "", ""
	}
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
