package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tmc/cdp/internal/coverage"
	harrecorder "github.com/tmc/cdp/internal/recorder"
	"github.com/tmc/cdp/internal/scrub"
	"github.com/tmc/cdp/internal/sources"
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
	webMCP            *webMCPCollector
	networkLog        *networkCollector
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

func (s *mcpSession) getCoverageStore() coverage.Store {
	if s.coverageCollector == nil {
		return nil
	}
	return s.coverageCollector
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
	Headless           bool
	Verbose            bool
	OutputDir          string
	URL                string
	DebugPort          int
	DebugPortExplicit  bool // true when --debug-port was explicitly set on the command line
	ConnectExisting    bool // true when --connect-existing was set
	ToolsDir           string
	SaveSources        bool
	NoScrub            bool
	APIPort            int
	LoadExtensions     string
	EnableInspect      bool
}

// runMCP starts the MCP server with browser session tools on stdio.
func runMCP(cfg mcpConfig) error {
	// Ensure all log output goes to stderr; stdout is the MCP transport.
	log.SetOutput(os.Stderr)

	// Default --save-sources to true in MCP mode when --output-dir is set,
	// since disk persistence is needed for the sourcemap pipeline.
	if cfg.OutputDir != "" && !cfg.SaveSources {
		cfg.SaveSources = true
		log.Printf("MCP mode: enabling --save-sources (output-dir=%s)", cfg.OutputDir)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Extract bundled extensions to ~/.cdp/extensions/.
	extBase, err := extractBundledExtensions()
	if err != nil {
		log.Printf("warning: extract bundled extensions: %v", err)
	} else {
		coveragePath := filepath.Join(extBase, "coverage")
		if _, err := os.Stat(filepath.Join(coveragePath, "manifest.json")); err == nil {
			if cfg.LoadExtensions != "" {
				cfg.LoadExtensions += "," + coveragePath
			} else {
				cfg.LoadExtensions = coveragePath
			}
			log.Printf("bundled coverage extension: %s", coveragePath)
		}
	}

	// Create session immediately so the MCP server can start responding
	// to initialize before the browser is ready. Browser setup runs in
	// the background; tools that need it will block until ready.
	session := &mcpSession{
		refs:      newRefRegistry(),
		outputDir: cfg.OutputDir,
	}

	// Set up secret scrubber (default on, --no-scrub to disable).
	var scrubber *scrub.Scrubber
	if !cfg.NoScrub {
		scrubber = scrub.New()
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "cdp",
		Version: "0.1.0",
	}, nil)

	registerMCPTools(server, session, cfg)

	if cfg.ToolsDir != "" {
		if err := loadAndRegisterCustomTools(server, session, cfg.ToolsDir); err != nil {
			log.Printf("warning: loading custom tools: %v", err)
		}
		if err := registerDefineToolMeta(server, session, cfg.ToolsDir); err != nil {
			return fmt.Errorf("register define_tool: %w", err)
		}
	}

	// Start coverage API server for the DevTools extension.
	// In MCP mode, auto-assign a free port if none specified.
	apiPort := cfg.APIPort
	if apiPort == 0 {
		if ln, err := net.Listen("tcp", "127.0.0.1:0"); err == nil {
			apiPort = ln.Addr().(*net.TCPAddr).Port
			ln.Close()
		}
	}
	if apiPort > 0 {
		log.Printf("Coverage API server on port %d", apiPort)
		go startCoverageAPI(apiPort, session)
	}

	// Set up browser in a goroutine so the MCP server can respond to
	// initialize immediately. The browser is typically ready within a
	// few seconds, well before the first tool call arrives.
	browserReady := make(chan struct{})
	go func() {
		defer close(browserReady)

		fcfg := fullCaptureConfig{
			Verbose:           cfg.Verbose,
			Headless:          cfg.Headless,
			DebugPort:         cfg.DebugPort,
			DebugPortExplicit: cfg.DebugPortExplicit,
			ConnectExisting:   cfg.ConnectExisting,
			OutputDir:         cfg.OutputDir,
			LoadExtensions:    cfg.LoadExtensions,
		}
		browserCtx, browserCancel, _, err := setupChromeForEnhanced(ctx, fcfg)
		if err != nil {
			log.Printf("error: setup browser: %v", err)
			return
		}

		// Optionally create a recorder for HAR capture.
		var rec *harrecorder.Recorder
		if cfg.OutputDir != "" {
			if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
				log.Printf("error: create output dir: %v", err)
				browserCancel()
				return
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
				log.Printf("error: create recorder: %v", err)
				browserCancel()
				return
			}
		}

		// Navigate to initial URL if provided.
		if cfg.URL != "" && cfg.URL != "about:blank" {
			if err := chromedp.Run(browserCtx, chromedp.Navigate(cfg.URL)); err != nil {
				log.Printf("error: navigate to %s: %v", cfg.URL, err)
			}
		}

		// Set up source capture if requested.
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

		// Populate the session now that the browser is ready.
		session.mu.Lock()
		session.browserCtx = browserCtx
		session.ctx = browserCtx
		session.cancel = browserCancel
		session.recorder = rec
		session.console = enableConsoleCapture(browserCtx)
		session.dialogs = enableDialogCapture(browserCtx)
		session.sourceCollector = sourceCollector
		session.mu.Unlock()

		// Auto-load sourcemaps from disk if --save-sources is active.
		if sourceCollector != nil {
			store := newSyntheticMapStore()
			if n := loadSourcemapsFromDisk(sourceCollector.OutputDir(), store); n > 0 {
				session.mu.Lock()
				session.syntheticMaps = store
				session.mu.Unlock()
				log.Printf("loaded %d sourcemap(s) from %s", n, sourceCollector.OutputDir())
			}
		}

		log.Printf("browser ready")

		// Block until context is cancelled to keep cleanup in scope.
		<-ctx.Done()

		if sourceCollector != nil {
			sourceCollector.Close()
			if err := sourceCollector.CaptureAll(browserCtx); err != nil && cfg.Verbose {
				log.Printf("warning: source capture errors: %v", err)
			}
			if err := sourceCollector.WriteToDisk(); err != nil {
				log.Printf("warning: failed to write sources: %v", err)
			}
		}
		browserCancel()
	}()

	return server.Run(ctx, &mcp.StdioTransport{})
}
