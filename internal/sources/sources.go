// Package sources captures JavaScript and CSS sources (including sourcemapped
// originals) from a Chrome DevTools Protocol session.
package sources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/css"
	"github.com/chromedp/cdproto/debugger"
	"github.com/tmc/misc/chrome-to-har/internal/scrub"
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

// Collector captures JavaScript and CSS sources from a browser session.
type Collector struct {
	mu        sync.Mutex
	scripts   map[cdp.ScriptID]*ScriptInfo
	styles    map[cdp.StyleSheetID]*StyleInfo
	outputDir string
	verbose   bool
	scrubber  *scrub.Scrubber
}

// New creates a source collector that writes to outputDir.
func New(outputDir string, verbose bool) *Collector {
	return &Collector{
		scripts:   make(map[cdp.ScriptID]*ScriptInfo),
		styles:    make(map[cdp.StyleSheetID]*StyleInfo),
		outputDir: outputDir,
		verbose:   verbose,
	}
}

// Enable activates the Debugger and CSS domains so the browser emits
// scriptParsed and styleSheetAdded events.
func (c *Collector) Enable(ctx context.Context) error {
	if _, err := debugger.Enable().Do(ctx); err != nil {
		return fmt.Errorf("enable debugger: %w", err)
	}
	if err := css.Enable().Do(ctx); err != nil {
		return fmt.Errorf("enable css: %w", err)
	}
	return nil
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
		c.mu.Unlock()
	case *css.EventStyleSheetAdded:
		h := ev.Header
		c.mu.Lock()
		c.styles[h.StyleSheetID] = &StyleInfo{
			StyleSheetID: h.StyleSheetID,
			URL:          h.SourceURL,
			SourceMapURL: h.SourceMapURL,
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

// fetchSourceMap fetches sourcemap content. Handles inline data URIs;
// external URL sourcemaps are not yet supported.
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
	return "", relPath, fmt.Errorf("external sourcemap fetch not implemented: %s", absURL)
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
