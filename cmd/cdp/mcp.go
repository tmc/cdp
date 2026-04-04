package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tmc/misc/chrome-to-har/internal/coverage"
	harrecorder "github.com/tmc/misc/chrome-to-har/internal/recorder"
	"github.com/tmc/misc/chrome-to-har/internal/scrub"
	"github.com/tmc/misc/chrome-to-har/internal/sources"
)

// mcpSession holds browser state shared across MCP tool handlers.
type mcpSession struct {
	mu                sync.Mutex
	browserCtx        context.Context    // browser-level context
	ctx               context.Context    // active tab context
	tabCancel         context.CancelFunc // cancels the current tab context (nil for initial tab)
	cancel            context.CancelFunc // cancels the browser context
	recorder          *harrecorder.Recorder
	sourceCollector   *sources.Collector
	coverageCollector *coverage.Collector
	refs              *refRegistry
	console           *consoleCollector
	dialogs           *dialogCollector
	intercepts        *interceptor
	traces            *traceCollector
	domSnapshots      *domSnapshotStore
	syntheticMaps     *syntheticMapStore
	activeFrameID     cdp.FrameID
	outputDir         string
	contextStack      []string
}

// activeCtx returns the current active tab context.
func (s *mcpSession) activeCtx() context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ctx
}

// setActiveCtx sets the active tab context, canceling the previous tab context if any.
func (s *mcpSession) setActiveCtx(ctx context.Context, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tabCancel != nil {
		s.tabCancel()
	}
	s.ctx = ctx
	s.tabCancel = cancel
}

// contextOutputDir returns the output directory for the current context stack.
func (s *mcpSession) contextOutputDir() string {
	dir := s.outputDir
	for _, name := range s.contextStack {
		dir = filepath.Join(dir, name)
	}
	return dir
}

// pushContext pushes a named context and updates the output directory.
// Automatically: starts a HAR tag range, adds a note annotation,
// and takes a coverage start snapshot (if active).
// Returns the new output directory path.
func (s *mcpSession) pushContext(name string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.contextStack = append(s.contextStack, name)
	dir := s.contextOutputDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("error creating context dir: %v", err)
		return dir
	}
	if s.recorder != nil {
		s.recorder.SetOutputDir(dir)
		s.recorder.SetTag(name)
		if err := s.recorder.AddNote(s.ctx, fmt.Sprintf("context: %s started", name)); err != nil {
			log.Printf("context: add note: %v", err)
		}
	}
	if s.coverageCollector != nil && s.coverageCollector.Running() {
		snapName := name + "-start"
		if _, err := s.coverageCollector.TakeSnapshot(snapName); err != nil {
			log.Printf("coverage: auto-snapshot %s: %v", snapName, err)
		}
	}
	return dir
}

// popContext pops the current context and updates the output directory.
// Automatically: ends the HAR tag range, adds a note annotation,
// takes a coverage end snapshot with delta and lcov output (if active).
// Returns the new output directory path and an error if the stack is empty.
func (s *mcpSession) popContext() (string, error) {
	s.mu.Lock()
	if len(s.contextStack) == 0 {
		s.mu.Unlock()
		return "", fmt.Errorf("no context to pop")
	}
	name := s.contextStack[len(s.contextStack)-1]
	contextDir := s.contextOutputDir()
	s.contextStack = s.contextStack[:len(s.contextStack)-1]
	dir := s.contextOutputDir()
	if s.recorder != nil {
		s.recorder.SetOutputDir(dir)
		// Restore parent context's tag or clear.
		parentTag := ""
		if len(s.contextStack) > 0 {
			parentTag = s.contextStack[len(s.contextStack)-1]
		}
		if err := s.recorder.AddNote(s.ctx, fmt.Sprintf("context: %s ended", name)); err != nil {
			log.Printf("context: add note: %v", err)
		}
		s.recorder.SetTag(parentTag)
	}
	cc := s.coverageCollector
	s.mu.Unlock()

	if cc != nil && cc.Running() {
		snapName := name + "-end"
		endSnap, err := cc.TakeSnapshot(snapName)
		if err != nil {
			log.Printf("coverage: auto-snapshot %s: %v", snapName, err)
		} else {
			s.writeCoverageLcov(contextDir, name, endSnap)
		}
	}
	return dir, nil
}

// writeCoverageLcov writes delta and cumulative lcov files for a context pop.
func (s *mcpSession) writeCoverageLcov(contextDir, name string, endSnap *coverage.Snapshot) {
	covDir := filepath.Join(contextDir, "coverage")
	if err := os.MkdirAll(covDir, 0755); err != nil {
		log.Printf("coverage: create dir: %v", err)
		return
	}

	// Find the matching start snapshot.
	snapshots := s.coverageCollector.Snapshots()
	startName := name + "-start"
	var startSnap *coverage.Snapshot
	for _, snap := range snapshots {
		if snap.Name == startName {
			startSnap = snap
		}
	}

	if startSnap != nil {
		delta := s.coverageCollector.ComputeDelta(startSnap, endSnap)
		lcov := coverage.DeltaToLcov(delta)
		path := filepath.Join(covDir, name+"-delta.lcov")
		if err := os.WriteFile(path, []byte(lcov), 0644); err != nil {
			log.Printf("coverage: write delta lcov: %v", err)
		}
	}

	// Write cumulative lcov from the end snapshot.
	cumLcov := coverage.SnapshotToLcov(endSnap)
	cumPath := filepath.Join(covDir, "cumulative.lcov")
	if err := os.WriteFile(cumPath, []byte(cumLcov), 0644); err != nil {
		log.Printf("coverage: write cumulative lcov: %v", err)
	}
}

// contextPath returns a display string for the current context stack.
func (s *mcpSession) contextPath() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.contextStack) == 0 {
		return "(root)"
	}
	return strings.Join(s.contextStack, "/")
}

// mcpConfig holds configuration for the MCP server mode.
type mcpConfig struct {
	Headless    bool
	Verbose     bool
	OutputDir   string
	URL         string
	DebugPort   int
	ToolsDir    string
	SaveSources bool
	NoScrub     bool
}

// runMCP starts the MCP server with browser session tools on stdio.
func runMCP(cfg mcpConfig) error {
	// Ensure all log output goes to stderr; stdout is the MCP transport.
	log.SetOutput(os.Stderr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up Chrome browser context.
	fcfg := fullCaptureConfig{
		Verbose:   cfg.Verbose,
		DebugPort: cfg.DebugPort,
		OutputDir: cfg.OutputDir,
	}
	browserCtx, browserCancel, _, err := setupChromeForEnhanced(ctx, fcfg)
	if err != nil {
		return fmt.Errorf("setup browser: %w", err)
	}

	// Set up secret scrubber (default on, --no-scrub to disable).
	var scrubber *scrub.Scrubber
	if !cfg.NoScrub {
		scrubber = scrub.New()
	}

	// Optionally create a recorder for HAR capture.
	var rec *harrecorder.Recorder
	if cfg.OutputDir != "" {
		if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
			return fmt.Errorf("create output dir: %w", err)
		}
		opts := []harrecorder.Option{
			harrecorder.WithVerbose(cfg.Verbose),
			harrecorder.WithStreaming(true),
			harrecorder.WithOutputDir(cfg.OutputDir),
		}
		if scrubber != nil {
			opts = append(opts, harrecorder.WithScrubber(scrubber))
		}
		rec, err = harrecorder.New(opts...)
		if err != nil {
			return fmt.Errorf("create recorder: %w", err)
		}
	}

	// Navigate to initial URL if provided.
	if cfg.URL != "" && cfg.URL != "about:blank" {
		if err := chromedp.Run(browserCtx, chromedp.Navigate(cfg.URL)); err != nil {
			return fmt.Errorf("navigate to %s: %w", cfg.URL, err)
		}
	}

	// Set up source capture if requested.
	// Set up defers so source capture runs before browserCancel (LIFO).
	var sourceCollector *sources.Collector
	if cfg.SaveSources {
		sourcesDir := filepath.Join(cfg.OutputDir, "sources")
		if cfg.OutputDir == "" {
			sourcesDir = "sources"
		}
		sourceCollector = sources.New(sourcesDir, cfg.Verbose)
		if scrubber != nil {
			sourceCollector.SetScrubber(scrubber)
		}
		if err := sourceCollector.Enable(browserCtx); err != nil {
			log.Printf("warning: failed to enable source capture: %v", err)
			sourceCollector = nil
		} else {
			chromedp.ListenTarget(browserCtx, sourceCollector.HandleEvent)
		}
	}
	defer browserCancel()
	if sourceCollector != nil {
		defer func() {
			sourceCollector.Close() // drain background goroutine
			if err := sourceCollector.CaptureAll(browserCtx); err != nil && cfg.Verbose {
				log.Printf("warning: source capture errors: %v", err)
			}
			if err := sourceCollector.WriteToDisk(); err != nil {
				log.Printf("warning: failed to write sources: %v", err)
			}
		}()
	}

	session := &mcpSession{
		browserCtx:      browserCtx,
		ctx:             browserCtx,
		cancel:          browserCancel,
		recorder:        rec,
		refs:            newRefRegistry(),
		console:         enableConsoleCapture(browserCtx),
		dialogs:         enableDialogCapture(browserCtx),
		outputDir:       cfg.OutputDir,
		sourceCollector: sourceCollector,
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "cdp",
		Version: "0.1.0",
	}, nil)

	registerMCPTools(server, session)

	if cfg.ToolsDir != "" {
		if err := loadAndRegisterCustomTools(server, session, cfg.ToolsDir); err != nil {
			log.Printf("warning: loading custom tools: %v", err)
		}
		if err := registerDefineToolMeta(server, session, cfg.ToolsDir); err != nil {
			return fmt.Errorf("register define_tool: %w", err)
		}
	}

	return server.Run(ctx, &mcp.StdioTransport{})
}
