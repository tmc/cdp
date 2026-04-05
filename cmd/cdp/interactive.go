package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/tmc/misc/chrome-to-har/internal/coverage"
	"github.com/tmc/misc/chrome-to-har/internal/sourcemap"
	"github.com/tmc/misc/chrome-to-har/internal/sources"
	"github.com/tmc/misc/chrome-to-har/internal/tooldef"
)

// InteractiveMode represents an interactive CDP session
type InteractiveMode struct {
	browserCtx   context.Context // browser-level context for creating/listing tabs
	ctx          context.Context // active tab context for executing commands
	cancel       context.CancelFunc
	launched     bool // true if we launched the browser (should close on exit)
	cfg          fullCaptureConfig
	registry     *CommandRegistry
	help         *HelpSystem
	history      []string
	verbose      bool
	baseOutputDir     string                    // root output dir from --output-dir
	contextStack      []string                  // stack of context names for push/pop
	recorder          recorderWithOutputDir     // optional recorder for output dir switching
	toolsDir          string                    // directory for .cdp tool definitions
	sourceCollector   *sources.Collector
	coverageCollector *coverage.Collector
	syntheticMaps     *syntheticMapStore
}

// recorderWithOutputDir is the subset of recorder.Recorder needed for context switching.
type recorderWithOutputDir interface {
	SetOutputDir(dir string)
	SetTag(tag string)
	AddNote(ctx context.Context, description string) error
}

// NewInteractiveMode creates a new interactive session.
// If toolsDir is non-empty, .cdp tool definitions are loaded from it.
func NewInteractiveMode(ctx context.Context, cancel context.CancelFunc, launched bool, cfg fullCaptureConfig, toolsDir string) *InteractiveMode {
	registry := NewCommandRegistry()
	im := &InteractiveMode{
		browserCtx:    ctx,
		ctx:           ctx,
		cancel:        cancel,
		launched:      launched,
		cfg:           cfg,
		registry:      registry,
		help:          NewHelpSystem(registry),
		history:       make([]string, 0),
		verbose:       cfg.Verbose,
		baseOutputDir: cfg.OutputDir,
		toolsDir:      toolsDir,
	}
	im.registerDefineCommand()
	im.registerCoverageCommands()
	im.registerSourcemapCommands()
	if toolsDir != "" {
		im.loadTools(toolsDir)
	}
	return im
}

// SetRecorder sets the recorder for output dir switching with push/pop context.
func (im *InteractiveMode) SetRecorder(rec recorderWithOutputDir, baseOutputDir string) {
	im.recorder = rec
	im.baseOutputDir = baseOutputDir
}

// SetSourceCollector sets the source collector for source browsing commands.
func (im *InteractiveMode) SetSourceCollector(sc *sources.Collector) {
	im.sourceCollector = sc
	im.registerSourceCommands()
}

// registerSourceCommands adds source browsing commands to the registry.
func (im *InteractiveMode) registerSourceCommands() {
	im.registry.RegisterCommand(&Command{
		Name:        "sources",
		Category:    "Sources",
		Description: "List captured JavaScript and CSS sources",
		Usage:       "sources [js|css]",
		Examples:    []string{"sources", "sources js", "sources css"},
		Aliases:     []string{"list-sources"},
		Handler: func(ctx context.Context, args []string) error {
			if im.sourceCollector == nil {
				return fmt.Errorf("source capture not enabled (use --save-sources)")
			}
			typeFilter := ""
			if len(args) > 0 {
				typeFilter = args[0]
			}
			if typeFilter == "" || typeFilter == "js" {
				scripts := im.sourceCollector.Scripts()
				if len(scripts) > 0 {
					fmt.Printf("JavaScript sources (%d):\n", len(scripts))
					for _, sc := range scripts {
						sm := ""
						if sc.SourceMapURL != "" {
							sm = " [sourcemap]"
						}
						fmt.Printf("  %6d bytes  %s%s\n", len(sc.Source), sc.URL, sm)
					}
				}
			}
			if typeFilter == "" || typeFilter == "css" {
				styles := im.sourceCollector.Styles()
				if len(styles) > 0 {
					fmt.Printf("CSS sources (%d):\n", len(styles))
					for _, st := range styles {
						sm := ""
						if st.SourceMapURL != "" {
							sm = " [sourcemap]"
						}
						fmt.Printf("  %6d bytes  %s%s\n", len(st.Source), st.URL, sm)
					}
				}
			}
			return nil
		},
	})

	im.registry.RegisterCommand(&Command{
		Name:        "read-source",
		Category:    "Sources",
		Description: "Read a captured source file by URL",
		Usage:       "read-source <url> [start-end]",
		Examples:    []string{"read-source https://example.com/app.js", "read-source https://example.com/app.js 10-20"},
		Handler: func(ctx context.Context, args []string) error {
			if im.sourceCollector == nil {
				return fmt.Errorf("source capture not enabled (use --save-sources)")
			}
			if len(args) < 1 {
				return fmt.Errorf("URL required")
			}
			src, err := findSourceInCollector(im.sourceCollector, args[0])
			if err != nil {
				return err
			}
			if len(args) > 1 {
				text, err := extractLines(src, args[1])
				if err != nil {
					return err
				}
				fmt.Print(text)
			} else {
				fmt.Println(src)
			}
			return nil
		},
	})

	im.registry.RegisterCommand(&Command{
		Name:        "search-source",
		Category:    "Sources",
		Description: "Search across captured sources for a pattern",
		Usage:       "search-source <pattern>",
		Examples:    []string{"search-source apiKey", "search-source 'fetch.*api'"},
		Aliases:     []string{"grep-source"},
		Handler: func(ctx context.Context, args []string) error {
			if im.sourceCollector == nil {
				return fmt.Errorf("source capture not enabled (use --save-sources)")
			}
			if len(args) < 1 {
				return fmt.Errorf("pattern required")
			}
			pattern := strings.Join(args, " ")
			re, reErr := regexp.Compile(pattern)
			match := func(line string) bool {
				if reErr == nil {
					return re.MatchString(line)
				}
				return strings.Contains(line, pattern)
			}

			type srcItem struct {
				url, source string
			}
			var items []srcItem
			for _, sc := range im.sourceCollector.Scripts() {
				if sc.Source != "" {
					items = append(items, srcItem{sc.URL, sc.Source})
				}
			}
			for _, st := range im.sourceCollector.Styles() {
				if st.Source != "" {
					items = append(items, srcItem{st.URL, st.Source})
				}
			}

			found := 0
			for _, item := range items {
				lines := strings.Split(item.source, "\n")
				for i, line := range lines {
					if !match(line) {
						continue
					}
					fmt.Printf("%s:%d: %s\n", item.url, i+1, strings.TrimSpace(line))
					found++
					if found >= 50 {
						fmt.Println("(truncated at 50 matches)")
						return nil
					}
				}
			}
			if found == 0 {
				fmt.Println("No matches.")
			} else {
				fmt.Printf("%d match(es)\n", found)
			}
			return nil
		},
	})
}

// registerCoverageCommands adds coverage commands to the registry.
// Called during NewInteractiveMode.
func (im *InteractiveMode) registerCoverageCommands() {
	im.registry.RegisterCommand(&Command{
		Name:        "coverage",
		Category:    "Coverage",
		Description: "Manage code coverage collection",
		Usage:       "coverage <start|snapshot|delta|compare|report|stop> [args]",
		Examples: []string{
			"coverage start",
			"coverage snapshot before-login",
			"coverage snapshot after-login",
			"coverage delta",
			"coverage compare before-login after-login",
			"coverage report",
			"coverage stop",
		},
		Handler: func(ctx context.Context, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("subcommand required: start, snapshot, delta, compare, report, stop")
			}
			switch args[0] {
			case "start":
				return im.coverageStart(ctx)
			case "snapshot":
				name := ""
				if len(args) > 1 {
					name = args[1]
				}
				return im.coverageSnapshot(name)
			case "delta":
				return im.coverageDelta()
			case "compare":
				if len(args) < 3 {
					return fmt.Errorf("usage: coverage compare <snap1> <snap2>")
				}
				return im.coverageCompare(args[1], args[2])
			case "report":
				return im.coverageReport()
			case "stop":
				return im.coverageStop()
			default:
				return fmt.Errorf("unknown subcommand %q: use start, snapshot, delta, compare, report, stop", args[0])
			}
		},
	})
}

func (im *InteractiveMode) coverageStart(ctx context.Context) error {
	if im.coverageCollector != nil {
		return fmt.Errorf("coverage already running")
	}
	c := coverage.New(im.verbose)
	if err := c.Start(ctx); err != nil {
		return fmt.Errorf("start coverage: %w", err)
	}
	im.coverageCollector = c
	fmt.Println("Coverage collection started.")
	return nil
}

func (im *InteractiveMode) coverageSnapshot(name string) error {
	if im.coverageCollector == nil {
		return fmt.Errorf("coverage not running (use: coverage start)")
	}
	snap, err := im.coverageCollector.TakeSnapshot(name)
	if err != nil {
		return fmt.Errorf("take snapshot: %w", err)
	}
	summary := snap.Summary()
	fmt.Printf("Snapshot %q: %d files\n", snap.Name, len(summary))
	for url, fs := range summary {
		fmt.Printf("  %5.1f%%  %4d/%4d lines  %s\n", fs.CoveragePercent, fs.CoveredLines, fs.TotalLines, url)
	}
	return nil
}

func (im *InteractiveMode) coverageDelta() error {
	if im.coverageCollector == nil {
		return fmt.Errorf("coverage not running (use: coverage start)")
	}
	snapshots := im.coverageCollector.Snapshots()
	if len(snapshots) < 2 {
		return fmt.Errorf("need at least 2 snapshots for delta")
	}
	before := snapshots[len(snapshots)-2]
	after := snapshots[len(snapshots)-1]
	delta := im.coverageCollector.ComputeDelta(before, after)
	fmt.Printf("Coverage delta: %s → %s\n\n", before.Name, after.Name)
	any := false
	for url, sd := range delta.Scripts {
		if len(sd.NewlyCovered) == 0 {
			continue
		}
		any = true
		pctDelta := 0.0
		if sd.TotalLines > 0 {
			pctDelta = float64(sd.CoveredAfter-sd.CoveredBefore) / float64(sd.TotalLines) * 100
		}
		fmt.Printf("%s  (+%.1f%%, %d new lines)\n", url, pctDelta, len(sd.NewlyCovered))
	}
	if !any {
		fmt.Println("No new coverage between snapshots.")
	}
	return nil
}

func (im *InteractiveMode) coverageCompare(snap1, snap2 string) error {
	if im.coverageCollector == nil {
		return fmt.Errorf("coverage not running (use: coverage start)")
	}
	snapshots := im.coverageCollector.Snapshots()
	var before, after *coverage.Snapshot
	for _, snap := range snapshots {
		if snap.Name == snap1 {
			before = snap
		}
		if snap.Name == snap2 {
			after = snap
		}
	}
	if before == nil {
		return fmt.Errorf("snapshot %q not found", snap1)
	}
	if after == nil {
		return fmt.Errorf("snapshot %q not found", snap2)
	}
	text := formatDetailedComparison(im.coverageCollector, before, after)
	fmt.Print(text)
	return nil
}

func (im *InteractiveMode) coverageReport() error {
	if im.coverageCollector == nil {
		return fmt.Errorf("coverage not running (use: coverage start)")
	}
	snapshots := im.coverageCollector.Snapshots()
	if len(snapshots) == 0 {
		fmt.Println("No snapshots taken yet.")
		return nil
	}
	fmt.Printf("Coverage report: %d snapshots\n\n", len(snapshots))
	for i, snap := range snapshots {
		summary := snap.Summary()
		totalCov := 0
		totalLines := 0
		for _, fs := range summary {
			totalCov += fs.CoveredLines
			totalLines += fs.TotalLines
		}
		pct := 0.0
		if totalLines > 0 {
			pct = float64(totalCov) / float64(totalLines) * 100
		}
		fmt.Printf("  %d. %-20s  %s  %d files  %d/%d lines (%.1f%%)\n",
			i+1, snap.Name, snap.Timestamp.Format("15:04:05"),
			len(summary), totalCov, totalLines, pct)

		// Show delta from previous snapshot if available.
		if i > 0 {
			delta := im.coverageCollector.ComputeDelta(snapshots[i-1], snap)
			newLines := 0
			for _, sd := range delta.Scripts {
				newLines += len(sd.NewlyCovered)
			}
			if newLines > 0 {
				fmt.Printf("       ↳ +%d newly covered lines since %s\n", newLines, snapshots[i-1].Name)
			}
		}
	}
	return nil
}

func (im *InteractiveMode) coverageStop() error {
	if im.coverageCollector == nil {
		return fmt.Errorf("coverage not running")
	}
	if err := im.coverageCollector.Stop(); err != nil {
		return fmt.Errorf("stop coverage: %w", err)
	}
	im.coverageCollector = nil
	fmt.Println("Coverage collection stopped.")
	return nil
}

// findSourceInCollector looks up a source by URL across scripts and styles.
func findSourceInCollector(sc *sources.Collector, u string) (string, error) {
	for _, s := range sc.Scripts() {
		if s.URL == u {
			if s.Source == "" {
				return "", fmt.Errorf("source not yet captured for %s", u)
			}
			return s.Source, nil
		}
	}
	for _, s := range sc.Styles() {
		if s.URL == u {
			if s.Source == "" {
				return "", fmt.Errorf("source not yet captured for %s", u)
			}
			return s.Source, nil
		}
	}
	return "", fmt.Errorf("no source found for URL %s", u)
}

// Run starts the interactive session
func (im *InteractiveMode) Run() error {
	im.showWelcome()

	scanner := bufio.NewScanner(os.Stdin)

	// captureSourcesOnExit stops the background fetcher, does a final sweep,
	// and writes any remaining sources to disk.
	captureSourcesOnExit := func() {
		if im.sourceCollector == nil {
			return
		}
		im.sourceCollector.Close() // drain background goroutine
		if err := chromedp.Run(im.ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			return im.sourceCollector.CaptureAll(ctx)
		})); err != nil && im.verbose {
			log.Printf("Warning: source capture errors: %v", err)
		}
		if err := im.sourceCollector.WriteToDisk(); err != nil {
			log.Printf("Warning: failed to write sources: %v", err)
		}
	}

	for {
		fmt.Print("cdp> ")

		if !scanner.Scan() {
			break
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Add to history
		im.history = append(im.history, line)

		// Handle special commands
		if im.handleSpecialCommand(line) {
			continue
		}

		// Check for exit
		if line == "exit" || line == "quit" || line == "q" {
			captureSourcesOnExit()
			if im.launched && im.cancel != nil {
				im.cancel()
				fmt.Println("Browser closed.")
			}
			fmt.Println("Goodbye!")
			break
		}

		// Execute command
		if err := im.executeCommand(line); err != nil {
			fmt.Printf("Error: %v\n", err)
		}
	}

	// Capture sources before closing browser.
	captureSourcesOnExit()

	// Close the browser on exit (EOF, Ctrl-D) if we launched it.
	if im.launched && im.cancel != nil {
		im.cancel()
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading input: %w", err)
	}

	return nil
}

// showWelcome displays the welcome message
func (im *InteractiveMode) showWelcome() {
	fmt.Println("\n╭─────────────────────────────────────────────────────────╮")
	fmt.Println("│      Welcome to CDP Interactive Mode                    │")
	fmt.Println("│      Chrome DevTools Protocol Command Line Interface    │")
	fmt.Println("╰─────────────────────────────────────────────────────────╯")
	fmt.Println()
	fmt.Println("Type 'help' for available commands or 'quick' for quick reference")
	fmt.Println("Type 'exit' or 'quit' to leave")
	fmt.Println()
}

// handleSpecialCommand handles special non-CDP commands
func (im *InteractiveMode) handleSpecialCommand(line string) bool {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return false
	}

	cmd := parts[0]
	args := parts[1:]

	switch cmd {
	case "help", "h", "?":
		im.help.ShowHelp(args)
		return true

	case "list", "ls":
		im.help.ListCommands()
		return true

	case "search", "find":
		if len(args) > 0 {
			im.help.SearchCommands(strings.Join(args, " "))
		} else {
			fmt.Println("Usage: search <term>")
		}
		return true

	case "quick", "qr", "ref":
		im.help.ShowQuickReference()
		return true

	case "history", "hist":
		im.showHistory()
		return true

	case "clear", "cls":
		im.clearScreen()
		return true

	case "verbose":
		im.verbose = !im.verbose
		fmt.Printf("Verbose mode: %v\n", im.verbose)
		return true

	case "version", "ver":
		fmt.Println("CDP Tool v1.0.0")
		return true

	case "refresh-profile", "rp":
		if im.cfg.UseProfile == "" {
			fmt.Println("No profile configured. Use --use-profile when launching.")
			return true
		}
		fmt.Printf("Re-copying profile '%s' and reconnecting...\n", im.cfg.UseProfile)
		if err := im.reconnect(); err != nil {
			fmt.Printf("Error: %v\n", err)
		}
		return true

	case "reconnect", "rc":
		fmt.Println("Reconnecting to browser...")
		if err := im.reconnect(); err != nil {
			fmt.Printf("Error: %v\n", err)
		}
		return true

	case "tabs", "list-tabs", "lt":
		im.listTabs()
		return true

	case "newtab", "nt":
		url := "about:blank"
		if len(args) > 0 {
			url = args[0]
		}
		im.newTab(url)
		return true

	case "tab", "t":
		if len(args) == 0 {
			fmt.Println("Usage: tab <index|id>")
			return true
		}
		im.switchTab(args[0])
		return true

	case "push-context", "push":
		if len(args) == 0 {
			fmt.Println("Usage: push-context <name>")
			return true
		}
		im.pushContext(args[0])
		return true

	case "pop-context", "pop":
		im.popContext()
		return true

	case "context":
		im.showContext()
		return true

	default:
		return false
	}
}

// reconnect attempts to re-establish the browser connection.
func (im *InteractiveMode) reconnect() error {
	fmt.Println("Reconnecting to browser...")
	if im.cancel != nil {
		im.cancel()
	}
	ctx, cancel, launched, err := setupChromeForEnhanced(context.Background(), im.cfg)
	if err != nil {
		return fmt.Errorf("reconnect failed: %w", err)
	}
	im.browserCtx = ctx
	im.ctx = ctx
	im.cancel = cancel
	im.launched = launched
	fmt.Println("Reconnected.")
	return nil
}

// listTabs lists all open browser tabs.
func (im *InteractiveMode) listTabs() {
	targets, err := chromedp.Targets(im.browserCtx)
	if err != nil {
		fmt.Printf("Error listing tabs: %v\n", err)
		return
	}

	// Get the active target ID for marking.
	activeTarget := chromedp.FromContext(im.ctx).Target
	var activeID string
	if activeTarget != nil {
		activeID = string(activeTarget.TargetID)
	}

	fmt.Println("Open tabs:")
	idx := 0
	for _, t := range targets {
		if t.Type != "page" {
			continue
		}
		marker := "  "
		if string(t.TargetID) == activeID {
			marker = "* "
		}
		title := t.Title
		if title == "" {
			title = "(untitled)"
		}
		fmt.Printf("  %s[%d] %s — %s\n", marker, idx, title, t.URL)
		idx++
	}
	if idx == 0 {
		fmt.Println("  (no tabs)")
	}
}

// newTab creates a new browser tab and switches to it.
func (im *InteractiveMode) newTab(url string) {
	tabCtx, _ := chromedp.NewContext(im.browserCtx)
	if err := chromedp.Run(tabCtx, chromedp.Navigate(url)); err != nil {
		fmt.Printf("Error creating tab: %v\n", err)
		return
	}
	im.ctx = tabCtx
	fmt.Printf("New tab: %s\n", url)
}

// switchTab switches the active context to an existing tab by index or target ID.
func (im *InteractiveMode) switchTab(selector string) {
	targets, err := chromedp.Targets(im.browserCtx)
	if err != nil {
		fmt.Printf("Error listing tabs: %v\n", err)
		return
	}

	// Filter to page targets only.
	var pages []*target.Info
	for _, t := range targets {
		if t.Type == "page" {
			pages = append(pages, t)
		}
	}

	// Try as numeric index first.
	var targetInfo *target.Info
	if idx, err := strconv.Atoi(selector); err == nil {
		if idx >= 0 && idx < len(pages) {
			targetInfo = pages[idx]
		} else {
			fmt.Printf("Tab index %d out of range (0-%d)\n", idx, len(pages)-1)
			return
		}
	} else {
		// Try as target ID prefix.
		for _, t := range pages {
			if strings.HasPrefix(string(t.TargetID), selector) {
				targetInfo = t
				break
			}
		}
	}

	if targetInfo == nil {
		fmt.Printf("No tab matching '%s'\n", selector)
		return
	}

	tabCtx, _ := chromedp.NewContext(im.browserCtx, chromedp.WithTargetID(targetInfo.TargetID))
	// Run a no-op to attach to the target.
	if err := chromedp.Run(tabCtx); err != nil {
		fmt.Printf("Error switching to tab: %v\n", err)
		return
	}
	im.ctx = tabCtx

	title := targetInfo.Title
	if title == "" {
		title = "(untitled)"
	}
	fmt.Printf("Switched to: %s — %s\n", title, targetInfo.URL)
}

// contextOutputDir returns the output directory for the current context stack.
func (im *InteractiveMode) contextOutputDir() string {
	dir := im.baseOutputDir
	for _, name := range im.contextStack {
		dir = filepath.Join(dir, name)
	}
	return dir
}

// pushContext pushes a named context, directing HAR/HARL writes to a subdirectory.
// Automatically: starts a HAR tag range, adds a note annotation,
// and takes a coverage start snapshot (if active).
func (im *InteractiveMode) pushContext(name string) {
	if im.baseOutputDir == "" {
		fmt.Println("No --output-dir configured; push-context has no effect.")
		return
	}
	im.contextStack = append(im.contextStack, name)
	dir := im.contextOutputDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf("Error creating context dir: %v\n", err)
		return
	}
	if im.recorder != nil {
		im.recorder.SetOutputDir(dir)
		im.recorder.SetTag(name)
		if err := im.recorder.AddNote(im.ctx, fmt.Sprintf("context: %s started", name)); err != nil && im.verbose {
			log.Printf("context: add note: %v", err)
		}
	}
	if im.coverageCollector != nil {
		snapName := name + "-start"
		if _, err := im.coverageCollector.TakeSnapshot(snapName); err != nil {
			fmt.Printf("Coverage auto-snapshot %s: %v\n", snapName, err)
		} else {
			fmt.Printf("Coverage: auto-snapshot %s\n", snapName)
		}
	}
	fmt.Printf("Context: %s — %s\n", strings.Join(im.contextStack, "/"), dir)
}

// popContext pops the current context, returning to the parent directory.
// Automatically: ends the HAR tag range, adds a note annotation,
// takes a coverage end snapshot with delta and lcov output (if active).
func (im *InteractiveMode) popContext() {
	if len(im.contextStack) == 0 {
		fmt.Println("No context to pop.")
		return
	}
	name := im.contextStack[len(im.contextStack)-1]
	contextDir := im.contextOutputDir()

	if im.coverageCollector != nil {
		snapName := name + "-end"
		endSnap, err := im.coverageCollector.TakeSnapshot(snapName)
		if err != nil {
			fmt.Printf("Coverage auto-snapshot %s: %v\n", snapName, err)
		} else {
			fmt.Printf("Coverage: auto-snapshot %s\n", snapName)
			im.writeCoverageLcov(contextDir, name, endSnap)
		}
	}

	im.contextStack = im.contextStack[:len(im.contextStack)-1]
	dir := im.contextOutputDir()
	if im.recorder != nil {
		if err := im.recorder.AddNote(im.ctx, fmt.Sprintf("context: %s ended", name)); err != nil && im.verbose {
			log.Printf("context: add note: %v", err)
		}
		im.recorder.SetOutputDir(dir)
		// Restore parent context's tag or clear.
		parentTag := ""
		if len(im.contextStack) > 0 {
			parentTag = im.contextStack[len(im.contextStack)-1]
		}
		im.recorder.SetTag(parentTag)
	}
	if len(im.contextStack) == 0 {
		fmt.Printf("Context: (root) — %s\n", dir)
	} else {
		fmt.Printf("Context: %s — %s\n", strings.Join(im.contextStack, "/"), dir)
	}
}

// writeCoverageLcov writes delta and cumulative lcov files for a context pop.
func (im *InteractiveMode) writeCoverageLcov(contextDir, name string, endSnap *coverage.Snapshot) {
	covDir := filepath.Join(contextDir, "coverage")
	if err := os.MkdirAll(covDir, 0755); err != nil {
		fmt.Printf("Coverage: create dir: %v\n", err)
		return
	}

	snapshots := im.coverageCollector.Snapshots()
	startName := name + "-start"
	var startSnap *coverage.Snapshot
	for _, snap := range snapshots {
		if snap.Name == startName {
			startSnap = snap
		}
	}

	if startSnap != nil {
		delta := im.coverageCollector.ComputeDelta(startSnap, endSnap)
		lcov := coverage.DeltaToLcov(delta)
		path := filepath.Join(covDir, name+"-delta.lcov")
		if err := os.WriteFile(path, []byte(lcov), 0644); err != nil {
			fmt.Printf("Coverage: write delta lcov: %v\n", err)
		} else {
			fmt.Printf("Coverage: wrote %s\n", path)
		}
	}

	cumLcov := coverage.SnapshotToLcov(endSnap)
	cumPath := filepath.Join(covDir, "cumulative.lcov")
	if err := os.WriteFile(cumPath, []byte(cumLcov), 0644); err != nil {
		fmt.Printf("Coverage: write cumulative lcov: %v\n", err)
	} else {
		fmt.Printf("Coverage: wrote %s\n", cumPath)
	}
}

// showContext shows the current context stack and output directory.
func (im *InteractiveMode) showContext() {
	if im.baseOutputDir == "" {
		fmt.Println("No --output-dir configured.")
		return
	}
	if len(im.contextStack) == 0 {
		fmt.Printf("Context: (root) — %s\n", im.baseOutputDir)
	} else {
		fmt.Printf("Context: %s — %s\n", strings.Join(im.contextStack, "/"), im.contextOutputDir())
	}
}

// isDisconnected reports whether an error indicates the browser connection is lost.
func isDisconnected(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "context canceled") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "use of closed network connection")
}

// executeCommand executes a CDP command
func (im *InteractiveMode) executeCommand(line string) error {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return nil
	}

	cmdName := parts[0]
	args := parts[1:]

	// Check if it's a registered command
	if cmd, found := im.registry.GetCommand(cmdName); found {
		if im.verbose {
			fmt.Printf("Executing: %s\n", cmd.Name)
		}
		err := cmd.Handler(im.ctx, args)
		if isDisconnected(err) {
			if reconnErr := im.reconnect(); reconnErr != nil {
				return reconnErr
			}
			// Retry the command once after reconnecting.
			return cmd.Handler(im.ctx, args)
		}
		return err
	}

	// Try to execute as raw CDP command
	if strings.Contains(cmdName, ".") {
		return im.executeRawCDP(line)
	}

	// Try to get completions
	completions := im.help.GetCompletions(cmdName)
	if len(completions) > 0 {
		fmt.Printf("Unknown command '%s'. Did you mean:\n", cmdName)
		for _, c := range completions {
			fmt.Printf("  • %s\n", c)
		}
		return nil
	}

	return fmt.Errorf("unknown command: %s", cmdName)
}

// executeRawCDP executes a raw CDP command
func (im *InteractiveMode) executeRawCDP(command string) error {
	// Parse Domain.method {params}
	parts := strings.SplitN(command, " ", 2)
	if len(parts) == 0 {
		return fmt.Errorf("empty command")
	}

	method := parts[0]
	if !strings.Contains(method, ".") {
		return fmt.Errorf("invalid CDP format: expected 'Domain.method'")
	}

	// Parse parameters
	params := "{}"
	if len(parts) > 1 {
		params = strings.TrimSpace(parts[1])
	}

	if im.verbose {
		fmt.Printf("Raw CDP: %s %s\n", method, params)
	}

	// Execute using chromedp (simplified - would need full CDP implementation)
	fmt.Printf("Executing CDP: %s with params: %s\n", method, params)
	fmt.Println("(Note: Raw CDP execution requires full implementation)")

	return nil
}

// showHistory displays command history
func (im *InteractiveMode) showHistory() {
	if len(im.history) == 0 {
		fmt.Println("No command history")
		return
	}

	fmt.Println("\nCommand History:")
	fmt.Println("────────────────")
	for i, cmd := range im.history {
		fmt.Printf("%3d: %s\n", i+1, cmd)
	}
	fmt.Println()
}

// clearScreen clears the terminal screen
func (im *InteractiveMode) clearScreen() {
	// ANSI escape code to clear screen
	fmt.Print("\033[2J\033[H")
	im.showWelcome()
}

// TabComplete provides tab completion for commands
func (im *InteractiveMode) TabComplete(partial string) []string {
	return im.help.GetCompletions(partial)
}

// ExecuteScript executes a script file
func (im *InteractiveMode) ExecuteScript(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("opening script file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	fmt.Printf("Executing script: %s\n", filename)
	fmt.Println("────────────────────────")

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		fmt.Printf("[%d] %s\n", lineNum, line)

		// Execute command
		if err := im.executeCommand(line); err != nil {
			return fmt.Errorf("line %d: %w", lineNum, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading script: %w", err)
	}

	fmt.Println("\nScript execution completed")
	return nil
}

// BatchExecute executes multiple commands in batch
func (im *InteractiveMode) BatchExecute(commands []string) error {
	fmt.Println("Executing batch commands:")
	fmt.Println("─────────────────────────")

	for i, cmd := range commands {
		fmt.Printf("[%d/%d] %s\n", i+1, len(commands), cmd)

		if err := im.executeCommand(cmd); err != nil {
			return fmt.Errorf("command %d: %w", i+1, err)
		}
	}

	fmt.Println("\nBatch execution completed")
	return nil
}

// SaveSession saves the current session to a file
func (im *InteractiveMode) SaveSession(filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("creating session file: %w", err)
	}
	defer file.Close()

	fmt.Fprintf(file, "# CDP Session - %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(file, "# Commands: %d\n\n", len(im.history))

	for _, cmd := range im.history {
		fmt.Fprintf(file, "%s\n", cmd)
	}

	fmt.Printf("Session saved to: %s (%d commands)\n", filename, len(im.history))
	return nil
}

// LoadSession loads and executes a saved session
func (im *InteractiveMode) LoadSession(filename string) error {
	return im.ExecuteScript(filename)
}

// loadTools loads .cdp tool definitions from dir and registers them as commands.
func (im *InteractiveMode) loadTools(dir string) {
	defs, err := tooldef.LoadDir(dir)
	if err != nil {
		log.Printf("warning: loading tools from %s: %v", dir, err)
		return
	}
	for _, def := range defs {
		im.registerToolCommand(def)
	}
	if len(defs) > 0 && im.verbose {
		log.Printf("[tools] loaded %d tool(s) from %s", len(defs), dir)
	}
}

// registerToolCommand registers a ToolDef as a shell command.
func (im *InteractiveMode) registerToolCommand(def *tooldef.ToolDef) {
	d := def // capture for closure
	usage := d.Name
	for _, inp := range d.Inputs {
		if inp.Optional {
			usage += " [" + inp.Name + "]"
		} else {
			usage += " <" + inp.Name + ">"
		}
	}
	im.registry.RegisterCommand(&Command{
		Name:        d.Name,
		Category:    "Tools",
		Description: d.Description,
		Usage:       usage,
		Handler: func(ctx context.Context, args []string) error {
			env := make(map[string]string)
			for i, inp := range d.Inputs {
				if i < len(args) {
					env[inp.Name] = args[i]
				}
			}
			_, err := executeToolLines(ctx, d.Script, env, func(c context.Context, line string) error {
				return im.executeCommand(line)
			})
			return err
		},
	})
	if im.verbose {
		log.Printf("[tools] registered: %s", d.Name)
	}
}

// registerDefineCommand adds the "define" command for creating tools interactively.
func (im *InteractiveMode) registerDefineCommand() {
	im.registry.RegisterCommand(&Command{
		Name:        "define",
		Category:    "Tools",
		Description: "Define a new tool from a one-liner script",
		Usage:       `define <name> "<description>" -- <script lines separated by ;>`,
		Examples: []string{
			`define check_login "Verify user is logged in" -- goto $url; wait #dashboard; title`,
		},
		Handler: func(ctx context.Context, args []string) error {
			return im.handleDefine(args)
		},
	})
}

// handleDefine implements the define command.
//
//	define <name> "<description>" -- <script;lines;separated;by;semicolons>
func (im *InteractiveMode) handleDefine(args []string) error {
	if len(args) < 4 {
		return fmt.Errorf("usage: define <name> \"<description>\" -- <script>")
	}

	name := args[0]

	// Find "--" separator.
	sepIdx := -1
	for i, a := range args {
		if a == "--" {
			sepIdx = i
			break
		}
	}
	if sepIdx < 2 {
		return fmt.Errorf("usage: define <name> \"<description>\" -- <script>")
	}

	description := strings.Join(args[1:sepIdx], " ")
	// Strip surrounding quotes if present.
	description = strings.Trim(description, `"`)

	scriptParts := args[sepIdx+1:]
	if len(scriptParts) == 0 {
		return fmt.Errorf("empty script body")
	}
	scriptBody := strings.Join(scriptParts, " ")
	// Semicolons separate lines.
	scriptBody = strings.ReplaceAll(scriptBody, ";", "\n")

	def := &tooldef.ToolDef{
		Name:        name,
		Description: description,
		Script:      strings.TrimSpace(scriptBody),
	}

	// Write to toolsDir if configured.
	if im.toolsDir != "" {
		path := filepath.Join(im.toolsDir, name+".cdp")
		if err := os.MkdirAll(im.toolsDir, 0755); err != nil {
			return fmt.Errorf("creating tools dir: %w", err)
		}
		if err := os.WriteFile(path, tooldef.Generate(def), 0644); err != nil {
			return fmt.Errorf("writing tool file: %w", err)
		}
		def.SourcePath = path
		fmt.Printf("Saved to %s\n", path)
	}

	// Register immediately.
	im.registerToolCommand(def)
	fmt.Printf("Tool '%s' defined and registered.\n", name)
	return nil
}

// registerSourcemapCommands adds sourcemap commands to the registry.
func (im *InteractiveMode) registerSourcemapCommands() {
	im.registry.RegisterCommand(&Command{
		Name:        "sourcemap",
		Category:    "Sourcemap",
		Description: "Synthetic sourcemap generation from coverage data",
		Usage:       "sourcemap <analyze|set-structure|generate|serve|list> [args]",
		Examples: []string{
			"sourcemap analyze http://example.com/bundle.js",
			"sourcemap analyze http://example.com/bundle.js after-login",
			`sourcemap set-structure http://example.com/bundle.js '{"files":[...],"summary":"..."}'`,
			"sourcemap generate http://example.com/bundle.js",
			"sourcemap list",
		},
		Aliases: []string{"smap"},
		Handler: func(ctx context.Context, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("subcommand required: analyze, set-structure, generate, serve, list")
			}
			switch args[0] {
			case "analyze":
				if len(args) < 2 {
					return fmt.Errorf("usage: sourcemap analyze <bundle_url> [snapshot_name]")
				}
				snap := ""
				if len(args) > 2 {
					snap = args[2]
				}
				return im.sourcemapAnalyze(args[1], snap)
			case "set-structure":
				if len(args) < 3 {
					return fmt.Errorf("usage: sourcemap set-structure <bundle_url> '<json>'")
				}
				return im.sourcemapSetStructure(args[1], strings.Join(args[2:], " "))
			case "generate":
				if len(args) < 2 {
					return fmt.Errorf("usage: sourcemap generate <bundle_url>")
				}
				return im.sourcemapGenerate(args[1])
			case "serve":
				if len(args) < 2 {
					return fmt.Errorf("usage: sourcemap serve <bundle_url>")
				}
				return im.sourcemapServe(args[1])
			case "list":
				return im.sourcemapList()
			default:
				return fmt.Errorf("unknown subcommand %q: use analyze, set-structure, generate, serve, list", args[0])
			}
		},
	})
}

func (im *InteractiveMode) sourcemapAnalyze(bundleURL, snapshotName string) error {
	if im.coverageCollector == nil {
		return fmt.Errorf("coverage not active — run 'coverage start' first")
	}

	snapshots := im.coverageCollector.Snapshots()
	if len(snapshots) == 0 {
		return fmt.Errorf("no coverage snapshots — run 'coverage snapshot' first")
	}

	snap := snapshots[len(snapshots)-1]
	if snapshotName != "" {
		snap = nil
		for _, sn := range snapshots {
			if sn.Name == snapshotName {
				snap = sn
				break
			}
		}
		if snap == nil {
			return fmt.Errorf("snapshot %q not found", snapshotName)
		}
	}

	scriptCov, ok := snap.Scripts[bundleURL]
	if !ok {
		// Try substring match.
		for url, sc := range snap.Scripts {
			if strings.Contains(url, bundleURL) {
				scriptCov = sc
				bundleURL = url
				break
			}
		}
		if scriptCov == nil {
			fmt.Printf("No coverage data for %s. Available scripts:\n", bundleURL)
			for url := range snap.Scripts {
				fmt.Printf("  %s\n", url)
			}
			return nil
		}
	}

	fmt.Printf("Bundle: %s (%d bytes)\n", bundleURL, len(scriptCov.Source))
	fmt.Printf("Functions: %d\n", len(scriptCov.Functions))

	executed := 0
	for _, fn := range scriptCov.Functions {
		if fn.HitCount > 0 {
			executed++
		}
	}
	fmt.Printf("Executed: %d\n\n", executed)

	for i, fn := range scriptCov.Functions {
		if fn.HitCount == 0 {
			continue
		}
		if i >= 50 {
			fmt.Printf("... and more\n")
			break
		}
		name := fn.Name
		if name == "" {
			name = "(anonymous)"
		}
		byteRange := ""
		if len(fn.Ranges) > 0 {
			byteRange = fmt.Sprintf(" [bytes %d-%d]", fn.Ranges[0].StartOffset, fn.Ranges[0].EndOffset)
		}
		fmt.Printf("  %s (lines %d-%d, %d hits)%s\n", name, fn.StartLine, fn.EndLine, fn.HitCount, byteRange)
	}

	// Also show chunk summary.
	var ranges []sourcemap.CoverageRange
	for _, r := range scriptCov.ByteRanges {
		ranges = append(ranges, sourcemap.CoverageRange{
			StartOffset: r.StartOffset,
			EndOffset:   r.EndOffset,
			Count:       r.Count,
		})
	}
	chunks := sourcemap.ExtractChunks(scriptCov.Source, ranges, 0)
	fmt.Printf("\nExtracted %d code chunks from byte-range coverage.\n", len(chunks))
	fmt.Println("Use 'sourcemap set-structure' to provide the inferred file structure.")
	return nil
}

func (im *InteractiveMode) sourcemapSetStructure(bundleURL, jsonStr string) error {
	if im.coverageCollector == nil {
		return fmt.Errorf("coverage not active")
	}

	var result inferredResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if len(result.Files) == 0 {
		return fmt.Errorf("structure must contain at least one file")
	}

	// Get bundle source.
	snapshots := im.coverageCollector.Snapshots()
	if len(snapshots) == 0 {
		return fmt.Errorf("no snapshots")
	}
	snap := snapshots[len(snapshots)-1]
	scriptCov, ok := snap.Scripts[bundleURL]
	if !ok {
		for url, sc := range snap.Scripts {
			if strings.Contains(url, bundleURL) {
				scriptCov = sc
				bundleURL = url
				break
			}
		}
		if scriptCov == nil {
			return fmt.Errorf("no coverage data for %s", bundleURL)
		}
	}

	if im.syntheticMaps == nil {
		im.syntheticMaps = newSyntheticMapStore()
	}

	sm := im.syntheticMaps.get(bundleURL)
	if sm == nil {
		sm = &syntheticMap{BundleURL: bundleURL}
	}
	sm.Sources = &result

	mapJSON, err := generateMapFromInferred(scriptCov.Source, &result)
	if err != nil {
		return fmt.Errorf("generate sourcemap: %w", err)
	}
	sm.MapJSON = mapJSON
	im.syntheticMaps.set(bundleURL, sm)

	fmt.Printf("Sourcemap generated: %d files, %d bytes\n", len(result.Files), len(mapJSON))
	for _, f := range result.Files {
		fmt.Printf("  %s (bytes %d-%d): %s\n", f.Path, f.StartOffset, f.EndOffset, f.Description)
	}
	fmt.Println("\nUse 'sourcemap generate' to view the raw JSON or 'sourcemap serve' to activate.")
	return nil
}

func (im *InteractiveMode) sourcemapGenerate(bundleURL string) error {
	if im.syntheticMaps == nil {
		return fmt.Errorf("no sourcemaps — use 'sourcemap set-structure' first")
	}
	sm := im.syntheticMaps.get(bundleURL)
	if sm == nil || sm.MapJSON == nil {
		return fmt.Errorf("no sourcemap for %s", bundleURL)
	}
	fmt.Println(string(sm.MapJSON))
	return nil
}

func (im *InteractiveMode) sourcemapServe(bundleURL string) error {
	if im.syntheticMaps == nil {
		return fmt.Errorf("no sourcemaps")
	}
	sm := im.syntheticMaps.get(bundleURL)
	if sm == nil || sm.MapJSON == nil {
		return fmt.Errorf("no sourcemap for %s — use 'sourcemap set-structure' first", bundleURL)
	}
	if sm.Serving {
		fmt.Printf("Already serving sourcemap for %s (rule %s)\n", bundleURL, sm.InterceptID)
		return nil
	}
	// Note: actual Fetch intercept requires the MCP interceptor plumbing.
	// In interactive mode, print the map URL and instructions.
	mapURL := bundleURL + ".map"
	fmt.Printf("Sourcemap ready at %s (%d bytes)\n", mapURL, len(sm.MapJSON))
	fmt.Println("To activate in DevTools, paste in console:")
	fmt.Printf("  document.querySelectorAll('script').forEach(s => { if(s.src.includes('%s')) console.log('found bundle') })\n", bundleURL)
	fmt.Println("\nNote: Full Fetch intercept serving requires MCP mode (cdp --mcp).")
	return nil
}

func (im *InteractiveMode) sourcemapList() error {
	if im.syntheticMaps == nil || len(im.syntheticMaps.list()) == 0 {
		fmt.Println("No sourcemaps.")
		return nil
	}
	for _, sm := range im.syntheticMaps.list() {
		nFiles := 0
		if sm.Sources != nil {
			nFiles = len(sm.Sources.Files)
		}
		status := "generated"
		if sm.Serving {
			status = fmt.Sprintf("serving (rule %s)", sm.InterceptID)
		}
		fmt.Printf("  %s: %d files, %d bytes [%s]\n", sm.BundleURL, nFiles, len(sm.MapJSON), status)
	}
	return nil
}