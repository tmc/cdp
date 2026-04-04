package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	harrecorder "github.com/tmc/misc/chrome-to-har/internal/recorder"
	"github.com/tmc/misc/chrome-to-har/internal/sources"
)

// mcpSession holds browser state shared across MCP tool handlers.
type mcpSession struct {
	mu              sync.Mutex
	browserCtx      context.Context    // browser-level context
	ctx             context.Context    // active tab context
	tabCancel       context.CancelFunc // cancels the current tab context (nil for initial tab)
	cancel          context.CancelFunc // cancels the browser context
	recorder        *harrecorder.Recorder
	sourceCollector *sources.Collector
	outputDir       string
	contextStack    []string
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
	}
	return dir
}

// popContext pops the current context and updates the output directory.
// Returns the new output directory path and an error if the stack is empty.
func (s *mcpSession) popContext() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.contextStack) == 0 {
		return "", fmt.Errorf("no context to pop")
	}
	s.contextStack = s.contextStack[:len(s.contextStack)-1]
	dir := s.contextOutputDir()
	if s.recorder != nil {
		s.recorder.SetOutputDir(dir)
	}
	return dir, nil
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
	defer browserCancel()

	// Optionally create a recorder for HAR capture.
	var rec *harrecorder.Recorder
	if cfg.OutputDir != "" {
		if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
			return fmt.Errorf("create output dir: %w", err)
		}
		rec, err = harrecorder.New(
			harrecorder.WithVerbose(cfg.Verbose),
			harrecorder.WithStreaming(true),
			harrecorder.WithOutputDir(cfg.OutputDir),
		)
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
	var sourceCollector *sources.Collector
	if cfg.SaveSources {
		sourcesDir := filepath.Join(cfg.OutputDir, "sources")
		if cfg.OutputDir == "" {
			sourcesDir = "sources"
		}
		sourceCollector = sources.New(sourcesDir, cfg.Verbose)
		if err := sourceCollector.Enable(browserCtx); err != nil {
			log.Printf("warning: failed to enable source capture: %v", err)
			sourceCollector = nil
		} else {
			chromedp.ListenTarget(browserCtx, sourceCollector.HandleEvent)
			defer func() {
				if err := sourceCollector.CaptureAll(browserCtx); err != nil && cfg.Verbose {
					log.Printf("warning: source capture errors: %v", err)
				}
				if err := sourceCollector.WriteToDisk(); err != nil {
					log.Printf("warning: failed to write sources: %v", err)
				}
			}()
		}
	}

	session := &mcpSession{
		browserCtx:      browserCtx,
		ctx:             browserCtx,
		cancel:          browserCancel,
		recorder:        rec,
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
