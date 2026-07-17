package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/tmc/cdp/internal/coverage"
	"github.com/tmc/cdp/internal/sourcemap"
	"github.com/tmc/cdp/internal/sources"
	"github.com/tmc/cdp/internal/tooldef"
	"golang.org/x/term"
)

const interactiveHistoryLimit = 1000

var errLineInterrupted = errors.New("line interrupted")

// InteractiveMode represents an interactive CDP session
type InteractiveMode struct {
	browserCtx        context.Context // browser-level context for creating/listing tabs
	ctx               context.Context // active tab context for executing commands
	cancel            context.CancelFunc
	launched          bool // true if we launched the browser (should close on exit)
	cfg               fullCaptureConfig
	registry          *CommandRegistry
	help              *HelpSystem
	history           []string
	verbose           bool
	baseOutputDir     string                // root output dir from --output-dir
	contextStack      []string              // stack of context names for push/pop
	recorder          recorderWithOutputDir // optional recorder for output dir switching
	attachRecorder    func(context.Context) // re-attaches traffic capture to a tab context
	toolsDir          string                // directory for .cdp tool definitions
	sourceCollector   *sources.Collector
	coverageCollector *coverage.Collector
	syntheticMaps     *sourcemapManager
}

func (im *InteractiveMode) getCoverageStore() coverage.Store {
	if im.coverageCollector == nil {
		return nil
	}
	return im.coverageCollector
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

// SetAttachRecorder registers a function that re-attaches traffic capture
// (network/fetch listeners) to a given tab context. It is invoked when the
// active tab changes so full capture follows tab switches, mirroring how the
// source collector re-attaches.
func (im *InteractiveMode) SetAttachRecorder(attach func(context.Context)) {
	im.attachRecorder = attach
}

// attachRecorderToTab re-attaches traffic capture to ctx if a recorder attach
// function has been registered.
func (im *InteractiveMode) attachRecorderToTab(ctx context.Context) {
	if im.attachRecorder != nil {
		im.attachRecorder(ctx)
	}
}

// SetSourceCollector sets the source collector for source browsing commands.
// Also auto-loads any .map files from disk into the sourcemap store.
func (im *InteractiveMode) SetSourceCollector(sc *sources.Collector) {
	im.sourceCollector = sc
	if sc != nil {
		if n := im.ensureSourcemaps().loadFromDisk(sc.OutputDir()); n > 0 {
			fmt.Printf("Loaded %d sourcemap(s) from %s\n", n, sc.OutputDir())
		}
	}
	im.registerSourceCommands()
}

func (im *InteractiveMode) attachSourceCollector(ctx context.Context) {
	if im.sourceCollector == nil {
		return
	}
	chromedp.ListenTarget(ctx, im.sourceCollector.Listener(ctx))
	if err := im.sourceCollector.AttachToTarget(ctx); err != nil && im.verbose {
		log.Printf("Warning: source capture target attach: %v", err)
	}
}

func interactiveHistoryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cdp", "history"), nil
}

func (im *InteractiveMode) loadHistory() {
	path, err := interactiveHistoryPath()
	if err != nil {
		if im.verbose {
			log.Printf("Warning: history path unavailable: %v", err)
		}
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) && im.verbose {
			log.Printf("Warning: failed to read history: %v", err)
		}
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			im.history = append(im.history, line)
		}
	}
	if len(im.history) > interactiveHistoryLimit {
		im.history = append([]string(nil), im.history[len(im.history)-interactiveHistoryLimit:]...)
	}
}

func (im *InteractiveMode) saveHistory() {
	path, err := interactiveHistoryPath()
	if err != nil {
		if im.verbose {
			log.Printf("Warning: history path unavailable: %v", err)
		}
		return
	}
	history := im.history
	if len(history) > interactiveHistoryLimit {
		history = history[len(history)-interactiveHistoryLimit:]
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		if im.verbose {
			log.Printf("Warning: failed to create history dir: %v", err)
		}
		return
	}
	data := strings.Join(history, "\n")
	if data != "" {
		data += "\n"
	}
	if err := os.WriteFile(path, []byte(data), 0600); err != nil && im.verbose {
		log.Printf("Warning: failed to write history: %v", err)
	}
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

type shellReader interface {
	ReadCommand(prompt string, cont func(string) bool) (string, error)
	Close() error
}

type scannerShellReader struct {
	scanner *bufio.Scanner
	out     io.Writer
	prompt  bool
}

func newScannerShellReader(in io.Reader, out io.Writer, prompt bool) *scannerShellReader {
	return &scannerShellReader{
		scanner: bufio.NewScanner(in),
		out:     out,
		prompt:  prompt,
	}
}

func (r *scannerShellReader) ReadCommand(prompt string, cont func(string) bool) (string, error) {
	var lines []string
	for {
		if r.prompt {
			if len(lines) == 0 {
				fmt.Fprint(r.out, prompt)
			} else {
				fmt.Fprint(r.out, ".... ")
			}
		}
		if !r.scanner.Scan() {
			if err := r.scanner.Err(); err != nil {
				return "", err
			}
			if len(lines) > 0 {
				return strings.Join(lines, "\n"), nil
			}
			return "", io.EOF
		}
		lines = append(lines, r.scanner.Text())
		cmd := strings.Join(lines, "\n")
		if cont == nil || !cont(cmd) {
			return cmd, nil
		}
	}
}

func (r *scannerShellReader) Close() error { return nil }

type terminalShellReader struct {
	in       *os.File
	out      io.Writer
	oldState *term.State
	history  *[]string
	complete func(string) []string
}

func newShellReader(in *os.File, out io.Writer, history *[]string, complete func(string) []string) (shellReader, error) {
	if !term.IsTerminal(int(in.Fd())) {
		// Even when stdin isn't a TTY (piped input, fresh pane with no parent
		// shell, etc.) print the prompt as long as the user can see it on a
		// TTY stdout. Without this, the shell appears hung after the welcome
		// banner.
		showPrompt := false
		if f, ok := out.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
			showPrompt = true
		}
		return newScannerShellReader(in, out, showPrompt), nil
	}
	return &terminalShellReader{
		in:       in,
		out:      out,
		history:  history,
		complete: complete,
	}, nil
}

func (r *terminalShellReader) Close() error {
	return r.restoreTerminal()
}

func (r *terminalShellReader) makeRaw() error {
	if r.oldState != nil {
		return nil
	}
	oldState, err := term.MakeRaw(int(r.in.Fd()))
	if err != nil {
		return err
	}
	r.oldState = oldState
	return nil
}

func (r *terminalShellReader) restoreTerminal() error {
	if r.oldState == nil {
		return nil
	}
	err := term.Restore(int(r.in.Fd()), r.oldState)
	r.oldState = nil
	return err
}

func (r *terminalShellReader) ReadCommand(prompt string, cont func(string) bool) (string, error) {
	var lines []string
	for {
		p := prompt
		if len(lines) > 0 {
			p = ".... "
		}
		line, err := r.readPhysicalLine(p)
		if err != nil {
			return "", err
		}
		lines = append(lines, line)
		cmd := strings.Join(lines, "\n")
		if cont == nil || !cont(cmd) {
			return cmd, nil
		}
	}
}

func (r *terminalShellReader) readPhysicalLine(prompt string) (string, error) {
	fmt.Fprint(r.out, prompt)
	if err := r.makeRaw(); err != nil {
		return "", err
	}
	defer r.restoreTerminal()
	var buf []rune
	pos := 0
	histIndex := len(*r.history)
	var saved []rune

	for {
		b, err := r.readByte()
		if err != nil {
			return "", err
		}

		switch b {
		case '\r', '\n':
			fmt.Fprint(r.out, "\r\n")
			return string(buf), nil
		case 0x03: // Ctrl-C
			fmt.Fprint(r.out, "^C\r\n")
			return "", errLineInterrupted
		case 0x04: // Ctrl-D
			if len(buf) == 0 {
				fmt.Fprint(r.out, "\r\n")
				return "", io.EOF
			}
		case 0x01: // Ctrl-A
			pos = 0
			r.redrawLine(prompt, buf, pos)
		case 0x05: // Ctrl-E
			pos = len(buf)
			r.redrawLine(prompt, buf, pos)
		case 0x09: // Tab
			buf, pos = r.completeLine(prompt, buf, pos)
		case 0x7f, 0x08: // Backspace
			if pos > 0 {
				buf = append(buf[:pos-1], buf[pos:]...)
				pos--
				r.redrawLine(prompt, buf, pos)
			}
		case 0x1b: // Escape sequence
			next, err := r.readByte()
			if err != nil {
				return "", err
			}
			if next != '[' {
				continue
			}
			key, err := r.readByte()
			if err != nil {
				return "", err
			}
			switch key {
			case 'A': // Up
				if histIndex > 0 {
					if histIndex == len(*r.history) {
						saved = append([]rune(nil), buf...)
					}
					histIndex--
					buf = []rune((*r.history)[histIndex])
					pos = len(buf)
					r.redrawLine(prompt, buf, pos)
				}
			case 'B': // Down
				if histIndex < len(*r.history) {
					histIndex++
					if histIndex == len(*r.history) {
						buf = append([]rune(nil), saved...)
					} else {
						buf = []rune((*r.history)[histIndex])
					}
					pos = len(buf)
					r.redrawLine(prompt, buf, pos)
				}
			case 'C': // Right
				if pos < len(buf) {
					pos++
					r.redrawLine(prompt, buf, pos)
				}
			case 'D': // Left
				if pos > 0 {
					pos--
					r.redrawLine(prompt, buf, pos)
				}
			case '3': // Delete: ESC [ 3 ~
				if tilde, err := r.readByte(); err == nil && tilde == '~' && pos < len(buf) {
					buf = append(buf[:pos], buf[pos+1:]...)
					r.redrawLine(prompt, buf, pos)
				}
			}
		default:
			if b >= 0x20 {
				buf = append(buf, 0)
				copy(buf[pos+1:], buf[pos:])
				buf[pos] = rune(b)
				pos++
				r.redrawLine(prompt, buf, pos)
			}
		}
	}
}

func (r *terminalShellReader) readByte() (byte, error) {
	var buf [1]byte
	n, err := r.in.Read(buf[:])
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, io.EOF
	}
	return buf[0], nil
}

func (r *terminalShellReader) redrawLine(prompt string, buf []rune, pos int) {
	line := string(buf)
	fmt.Fprintf(r.out, "\r\033[2K%s%s", prompt, line)
	if back := len(buf) - pos; back > 0 {
		fmt.Fprintf(r.out, "\033[%dD", back)
	}
}

func (r *terminalShellReader) completeLine(prompt string, buf []rune, pos int) ([]rune, int) {
	if r.complete == nil {
		return buf, pos
	}
	prefix := currentWord(buf, pos)
	if prefix == "" {
		return buf, pos
	}
	matches := r.complete(prefix)
	if len(matches) == 0 {
		return buf, pos
	}
	common := longestCommonPrefix(matches)
	if len(common) > len(prefix) {
		insert := []rune(common[len(prefix):])
		buf = append(buf, make([]rune, len(insert))...)
		copy(buf[pos+len(insert):], buf[pos:])
		copy(buf[pos:], insert)
		pos += len(insert)
		r.redrawLine(prompt, buf, pos)
		return buf, pos
	}
	fmt.Fprint(r.out, "\r\n")
	for _, match := range matches {
		fmt.Fprintf(r.out, "  %s\r\n", match)
	}
	r.redrawLine(prompt, buf, pos)
	return buf, pos
}

func currentWord(buf []rune, pos int) string {
	start := pos
	for start > 0 && !isShellSpace(buf[start-1]) {
		start--
	}
	return string(buf[start:pos])
}

func isShellSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

func longestCommonPrefix(values []string) string {
	if len(values) == 0 {
		return ""
	}
	prefix := values[0]
	for _, v := range values[1:] {
		for !strings.HasPrefix(v, prefix) {
			if prefix == "" {
				return ""
			}
			prefix = prefix[:len(prefix)-1]
		}
	}
	return prefix
}

func rawCDPNeedsContinuation(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	fields := strings.Fields(line)
	if len(fields) == 0 || !strings.Contains(fields[0], ".") || !strings.Contains(line, "{") {
		return false
	}
	depth := 0
	inString := false
	escape := false
	for _, r := range line {
		if inString {
			if escape {
				escape = false
				continue
			}
			switch r {
			case '\\':
				escape = true
			case '"':
				inString = false
			}
			continue
		}
		switch r {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			if depth > 0 {
				depth--
			}
		}
	}
	return depth > 0 || inString
}

// Run starts the interactive session
func (im *InteractiveMode) Run() error {
	if im.cfg.APIPort > 0 {
		go startCoverageAPI(im.cfg.APIPort, im)
	}
	im.showWelcome()

	im.loadHistory()
	input, err := newShellReader(os.Stdin, os.Stdout, &im.history, im.TabComplete)
	if err != nil {
		return fmt.Errorf("initializing terminal input: %w", err)
	}
	inputClosed := false
	closeInput := func() {
		if !inputClosed {
			_ = input.Close()
			inputClosed = true
		}
	}
	defer closeInput()
	defer im.saveHistory()

	// captureSourcesOnExit stops the background fetcher, does a final sweep,
	// and writes any remaining sources to disk.
	capturedSources := false
	captureSourcesOnExit := func() {
		if capturedSources {
			return
		}
		capturedSources = true
		if im.sourceCollector == nil {
			return
		}
		im.sourceCollector.Close() // drain background goroutine
		captureDone := make(chan error, 1)
		go func() {
			captureDone <- chromedp.Run(im.ctx, chromedp.ActionFunc(func(ctx context.Context) error {
				return im.sourceCollector.CaptureAll(ctx)
			}))
		}()
		captureComplete := false
		select {
		case err := <-captureDone:
			captureComplete = true
			if err != nil && im.verbose {
				log.Printf("Warning: source capture errors: %v", err)
			}
		case <-time.After(30 * time.Second):
			if im.verbose {
				log.Printf("Warning: source capture timed out; writing incrementally captured sources")
			}
		}
		if captureComplete {
			if err := im.sourceCollector.WriteToDisk(); err != nil {
				log.Printf("Warning: failed to write sources to %s: %v", im.sourceCollector.OutputDir(), err)
			}
		}
	}

	for {
		line, err := input.ReadCommand("cdp> ", rawCDPNeedsContinuation)
		if errors.Is(err, io.EOF) {
			break
		}
		if errors.Is(err, errLineInterrupted) {
			continue
		}
		if err != nil {
			return fmt.Errorf("error reading input: %w", err)
		}

		line = strings.TrimSpace(line)
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
			closeInput()
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
	if im.cfg.OutputDir != "" {
		if abs, err := filepath.Abs(im.cfg.OutputDir); err == nil {
			fmt.Printf("Output dir: %s\n", abs)
		} else {
			fmt.Printf("Output dir: %s\n", im.cfg.OutputDir)
		}
	}
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
	im.attachSourceCollector(ctx)
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
	im.attachSourceCollector(tabCtx)
	im.attachRecorderToTab(tabCtx)
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
	im.attachSourceCollector(tabCtx)
	im.attachRecorderToTab(tabCtx)
	im.ctx = tabCtx

	title := targetInfo.Title
	if title == "" {
		title = "(untitled)"
	}
	fmt.Printf("Switched to: %s — %s\n", title, targetInfo.URL)
}

// contextOutputDir returns the output directory for the current context stack.
func (im *InteractiveMode) contextOutputDir() string {
	return contextStackOutputDir(im.baseOutputDir, im.contextStack)
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
	fmt.Printf("Context: %s — %s\n", contextStackDisplay(im.contextStack), dir)
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
		im.recorder.SetTag(contextStackParentTag(im.contextStack))
	}
	fmt.Printf("Context: %s — %s\n", contextStackDisplay(im.contextStack), dir)
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
	fmt.Printf("Context: %s — %s\n", contextStackDisplay(im.contextStack), im.contextOutputDir())
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
		var nav *navigationProgress
		if cmd.Category == "Navigation" {
			nav = newNavigationProgress(im.cfg.Progress, navigationURL(cmd, args), im.cfg.NavigationTimeout)
			nav.listen(im.ctx)
			nav.start()
		}
		run := func() error {
			ctx, cancel := im.commandContext(cmd)
			defer cancel()
			if cmd.Name == "navigate" {
				return im.navigate(ctx, args, nav)
			}
			return cmd.Handler(ctx, args)
		}
		err := run()
		if nav != nil {
			nav.finish(err)
			err = nav.wrapError(err)
		}
		if isDisconnected(err) {
			if reconnErr := im.reconnect(); reconnErr != nil {
				return reconnErr
			}
			// Retry the command once after reconnecting.
			return run()
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

func navigationURL(cmd *Command, args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return cmd.Name
}

type navigationProgress struct {
	progress *startupProgress
	url      string
	timeout  int
	started  time.Time

	mu        sync.Mutex
	active    bool
	stage     string
	domReady  chan struct{}
	loadReady chan struct{}
	idleReady chan struct{}
	domOnce   sync.Once
	loadOnce  sync.Once
	idleOnce  sync.Once
	pending   map[network.RequestID]struct{}
	idleTimer *time.Timer
}

func newNavigationProgress(progress *startupProgress, url string, timeout int) *navigationProgress {
	return &navigationProgress{progress: progress, url: url, timeout: timeout, stage: "request not sent"}
}

func (n *navigationProgress) listen(ctx context.Context) {
	if n == nil {
		return
	}
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *network.EventRequestWillBeSent:
			n.mu.Lock()
			if n.active {
				n.pending[e.RequestID] = struct{}{}
				n.resetIdleLocked()
			}
			n.mu.Unlock()
			if e.Request != nil && sameNavigationURL(e.Request.URL, n.url) {
				n.setStage("request sent")
			}
		case *network.EventResponseReceived:
			if e.Response != nil && sameNavigationURL(e.Response.URL, n.url) {
				n.setStage("response received")
			}
		case *page.EventDomContentEventFired:
			n.setStage("DOM content loaded")
			n.domOnce.Do(func() { close(n.domReady) })
		case *page.EventLoadEventFired:
			n.setStage("load event")
			n.loadOnce.Do(func() { close(n.loadReady) })
		case *network.EventLoadingFinished:
			n.requestDone(e.RequestID)
		case *network.EventLoadingFailed:
			n.requestDone(e.RequestID)
		}
	})
}

func sameNavigationURL(a, b string) bool {
	ua, errA := url.Parse(a)
	ub, errB := url.Parse(b)
	if errA != nil || errB != nil {
		return a == b
	}
	if ua.Scheme != ub.Scheme || ua.Host != ub.Host || ua.RawQuery != ub.RawQuery {
		return false
	}
	pa, pb := strings.TrimSuffix(ua.Path, "/"), strings.TrimSuffix(ub.Path, "/")
	return pa == pb
}

func (n *navigationProgress) start() {
	if n == nil {
		return
	}
	n.mu.Lock()
	n.started = time.Now()
	n.active = true
	n.domReady = make(chan struct{})
	n.loadReady = make(chan struct{})
	n.idleReady = make(chan struct{})
	n.pending = make(map[network.RequestID]struct{})
	n.mu.Unlock()
	n.write("navigating " + n.url + " (requesting)")
}

func (n *navigationProgress) beginIdle() {
	if n == nil {
		return
	}
	n.mu.Lock()
	n.resetIdleLocked()
	n.mu.Unlock()
}

func (n *navigationProgress) requestDone(id network.RequestID) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.active {
		return
	}
	delete(n.pending, id)
	n.resetIdleLocked()
}

func (n *navigationProgress) resetIdleLocked() {
	if n.idleTimer != nil {
		n.idleTimer.Stop()
	}
	if len(n.pending) != 0 {
		return
	}
	n.idleTimer = time.AfterFunc(500*time.Millisecond, func() {
		n.idleOnce.Do(func() { close(n.idleReady) })
	})
}

func (n *navigationProgress) wait(ctx context.Context, mode string) error {
	var ready <-chan struct{}
	switch mode {
	case "load":
		ready = n.loadReady
	case "networkidle":
		ready = n.idleReady
	default:
		ready = n.domReady
	}
	select {
	case <-ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (n *navigationProgress) setStage(stage string) {
	n.mu.Lock()
	if !n.active || n.stage == stage {
		n.mu.Unlock()
		return
	}
	n.stage = stage
	n.mu.Unlock()
	n.write("navigating " + n.url + " (" + stage + ")")
}

func (n *navigationProgress) finish(err error) {
	if n == nil {
		return
	}
	n.mu.Lock()
	n.active = false
	elapsed := time.Since(n.started)
	n.mu.Unlock()
	if err == nil {
		n.writeFinal(fmt.Sprintf("✓ loaded %s (%s)", n.url, elapsed.Round(time.Millisecond)))
		return
	}
	n.writeFinal(fmt.Sprintf("navigation failed %s (%s)", n.url, elapsed.Round(time.Millisecond)))
}

func (n *navigationProgress) stageName() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.stage
}

func (n *navigationProgress) elapsed() time.Duration {
	n.mu.Lock()
	defer n.mu.Unlock()
	return time.Since(n.started)
}

func (n *navigationProgress) write(message string) {
	if n == nil || n.progress == nil || !n.progress.enabled {
		return
	}
	n.progress.status(message, false)
}

func (n *navigationProgress) writeFinal(message string) {
	if n == nil || n.progress == nil || !n.progress.enabled {
		return
	}
	n.progress.status(message, true)
}

func (n *navigationProgress) wrapError(err error) error {
	if err == nil {
		return nil
	}
	elapsed := n.elapsed().Round(time.Millisecond)
	stage := n.stageName()
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("navigate %s: timed out after %s (timeout %ds, last stage: %s): %w", n.url, elapsed, n.timeout, stage, err)
	}
	return fmt.Errorf("navigate %s failed after %s (last stage: %s): %w", n.url, elapsed, stage, err)
}

func (im *InteractiveMode) commandContext(cmd *Command) (context.Context, context.CancelFunc) {
	if cmd.Category != "Navigation" || im.cfg.NavigationTimeout == 0 {
		return im.ctx, func() {}
	}
	if im.cfg.NavigationTimeout < 0 {
		return im.ctx, func() {}
	}
	// A timeout derived directly from a chromedp context can close its target
	// when the child is cancelled. Preserve the executor and values while
	// severing cancellation from the browser-owning context.
	return context.WithTimeout(context.WithoutCancel(im.ctx), time.Duration(im.cfg.NavigationTimeout)*time.Second)
}

func (im *InteractiveMode) navigate(ctx context.Context, args []string, nav *navigationProgress) error {
	if len(args) < 1 {
		return errors.New("URL required")
	}
	cdpContext := chromedp.FromContext(im.ctx)
	if cdpContext == nil || cdpContext.Target == nil {
		return errors.New("browser target unavailable")
	}
	_, _, errorText, _, err := page.Navigate(args[0]).Do(cdp.WithExecutor(ctx, cdpContext.Target))
	if err != nil {
		return err
	}
	if errorText != "" {
		return fmt.Errorf("page load error %s", errorText)
	}
	if nav == nil {
		return nil
	}
	nav.beginIdle()
	return nav.wait(ctx, im.cfg.WaitMode)
}

// executeRawCDP executes a raw CDP command
func (im *InteractiveMode) executeRawCDP(command string) error {
	method, params, err := parseRawCDPCommand(command)
	if err != nil {
		return err
	}
	if im.verbose {
		data, _ := json.Marshal(params)
		fmt.Printf("Raw CDP: %s %s\n", method, data)
	}

	result, err := runRawCDP(im.ctx, method, params, "target")
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal CDP result: %w", err)
	}
	fmt.Println(string(data))
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
			stdout, stderr, err := runCDPScriptBody(im.ctx, d.Script, env, im.contextOutputDir())
			if stdout != "" {
				fmt.Println(stdout)
			}
			if stderr != "" && im.verbose {
				fmt.Fprint(os.Stderr, stderr)
			}
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
		Usage:       "sourcemap <analyze|set-structure|generate|serve|list|log> [args]",
		Examples: []string{
			"sourcemap analyze http://example.com/bundle.js",
			"sourcemap analyze http://example.com/bundle.js after-login",
			`sourcemap set-structure http://example.com/bundle.js '{"files":[...],"summary":"..."}'`,
			"sourcemap generate http://example.com/bundle.js",
			"sourcemap list",
			"sourcemap log http://example.com/bundle.js",
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
			case "log":
				if len(args) < 2 {
					return fmt.Errorf("usage: sourcemap log <bundle_url>")
				}
				return im.sourcemapLog(args[1])
			default:
				return fmt.Errorf("unknown subcommand %q: use analyze, set-structure, generate, serve, list, log", args[0])
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

	maps := im.ensureSourcemaps()
	sm := maps.get(bundleURL)
	isRefinement := sm != nil && sm.Sources != nil && len(sm.Sources.Files) > 0

	mapJSON, err := sourcemap.GenerateFromStructure(scriptCov.Source, &result)
	if err != nil {
		return fmt.Errorf("generate sourcemap: %w", err)
	}

	// Write to disk alongside saved sources.
	sourcesDir := ""
	if im.sourceCollector != nil {
		sourcesDir = im.sourceCollector.OutputDir()
	}
	mapPath := ""
	if path := writeSourcemapToDisk(sourcesDir, bundleURL, mapJSON); path != "" {
		mapPath = path
		writeStructureSidecar(path, &result)
		ctxName := contextStackDisplay(im.contextStack)
		if ctxName == "(root)" {
			ctxName = ""
		}
		appendAnalysisLog(path, bundleURL, ctxName, &result, scriptCov.Source, isRefinement)
	}
	sm = maps.update(bundleURL, func(sm *syntheticMap) {
		sm.Sources = &result
		sm.MapJSON = mapJSON
		if mapPath != "" {
			sm.MapPath = mapPath
		}
	})

	fmt.Printf("Sourcemap generated: %d files, %d bytes\n", len(result.Files), len(mapJSON))
	for _, f := range result.Files {
		fmt.Printf("  %s (bytes %d-%d): %s\n", f.Path, f.StartOffset, f.EndOffset, f.Description)
	}
	if sm.MapPath != "" {
		fmt.Printf("\nWritten to %s\n", sm.MapPath)
	}
	fmt.Println("Use 'sourcemap generate' to view the raw JSON or 'sourcemap serve' to activate.")
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
		logCount := sourcemap.CountAnalysisLogEntries(sm.MapPath)
		logInfo := ""
		if logCount > 0 {
			logInfo = fmt.Sprintf(", %d log entries", logCount)
		}
		fmt.Printf("  %s: %d files, %d bytes [%s%s]\n", sm.BundleURL, nFiles, len(sm.MapJSON), status, logInfo)
		if sm.MapPath != "" {
			fmt.Printf("    → %s\n", sm.MapPath)
		}
	}
	return nil
}

func (im *InteractiveMode) sourcemapLog(bundleURL string) error {
	if im.syntheticMaps == nil {
		return fmt.Errorf("no sourcemaps")
	}
	sm := im.syntheticMaps.get(bundleURL)
	if sm == nil {
		// Try substring match.
		for _, m := range im.syntheticMaps.list() {
			if strings.Contains(m.BundleURL, bundleURL) {
				sm = m
				break
			}
		}
	}
	if sm == nil || sm.MapPath == "" {
		return fmt.Errorf("no on-disk sourcemap for %s", bundleURL)
	}

	entries, err := sourcemap.ReadAnalysisLog(sm.MapPath)
	if err != nil {
		return fmt.Errorf("read log: %w", err)
	}
	if len(entries) == 0 {
		fmt.Println("No analysis log entries.")
		return nil
	}

	fmt.Printf("Analysis log for %s (%d entries):\n\n", sm.BundleURL, len(entries))
	for i, e := range entries {
		kind := "new"
		if e.IsRefinement {
			kind = "refinement"
		}
		ctx := ""
		if e.Context != "" {
			ctx = fmt.Sprintf(" [context: %s]", e.Context)
		}
		fmt.Printf("--- Entry %d (%s, %s%s) ---\n", i+1, e.Timestamp, kind, ctx)
		if e.Summary != "" {
			fmt.Printf("  Summary: %s\n", e.Summary)
		}
		for _, f := range e.Files {
			fmt.Printf("  %s (bytes %d-%d)\n", f.Path, f.StartOffset, f.EndOffset)
			fmt.Printf("    Reasoning: %s\n", f.Reasoning)
			if f.Snippet != "" {
				fmt.Printf("    Snippet: %s\n", f.Snippet)
			}
			for _, fn := range f.Functions {
				fmt.Printf("    fn %s (L%d-%d): %s\n", fn.Name, fn.StartLine, fn.EndLine, fn.Reasoning)
			}
		}
		fmt.Println()
	}
	return nil
}
