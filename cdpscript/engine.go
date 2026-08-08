// Package cdpscript provides a CDP script engine based on rsc.io/script.
package cdpscript

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/har"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/tmc/cdp/internal/browser"
	"github.com/tmc/cdp/internal/cdpinput"
	"github.com/tmc/cdp/internal/discovery"
	"github.com/tmc/cdp/internal/htmltomd"
	"github.com/tmc/cdp/internal/recorder"
	"github.com/tmc/cdp/internal/termmd"
	"golang.org/x/tools/txtar"
	"rsc.io/script"
)

const defaultScriptTimeout = 30 * time.Second

type archiveData struct {
	Comment string
	Main    string
	Files   []txtar.File
}

// Engine executes CDP scripts using the rsc.io/script framework.
type Engine struct {
	engine    *script.Engine
	browser   *browser.Browser
	verbose   bool
	outputDir string
	env       []string
	headless  bool
	timeout   time.Duration
	stdout    io.Writer
	stderr    io.Writer

	// Remote tab connection
	remoteTabID     string
	remotePort      int
	externalBrowser bool

	// Mouse gesture state, so "mouse move" can carry the button while it is
	// held and "mouse up" can default to the last position.
	mousePos     cdpinput.ViewportPoint
	mouseDown    bool
	mouseTracked bool

	// Sourced commands (dynamically loaded from source command)
	sourcedCmds map[string]script.Cmd

	// Element refs from last snapshot (for @e1 style refs)
	refMap *browser.RefMap

	// HAR recorder for capturing network activity with tags
	recorder *recorder.Recorder

	dialogMu        sync.Mutex
	dialogListening bool
	dialogAction    *dialogAction

	downloadMu  sync.Mutex
	downloadDir string
}

// New creates a new CDP script engine.
func New(opts ...Option) *Engine {
	e := &Engine{
		sourcedCmds: make(map[string]script.Cmd),
		stdout:      os.Stdout,
		stderr:      os.Stderr,
	}
	for _, opt := range opts {
		opt(e)
	}

	// Create script engine with CDP commands
	e.engine = &script.Engine{
		Cmds:  e.commands(),
		Conds: e.conditions(),
	}

	return e
}

// Option configures the Engine.
type Option func(*Engine)

// WithVerbose enables verbose output.
func WithVerbose(v bool) Option {
	return func(e *Engine) { e.verbose = v }
}

// WithOutputDir sets the output directory.
func WithOutputDir(dir string) Option {
	return func(e *Engine) { e.outputDir = dir }
}

// WithHeadless controls whether the launched browser runs headless.
func WithHeadless(v bool) Option {
	return func(e *Engine) { e.headless = v }
}

// WithTimeout sets the default timeout for browser startup and selector waits.
func WithTimeout(d time.Duration) Option {
	return func(e *Engine) { e.timeout = d }
}

// WithEnv adds initial environment variables for script execution.
// Each entry must have the form "key=value".
func WithEnv(env ...string) Option {
	return func(e *Engine) { e.env = append(e.env, env...) }
}

// WithRemoteTab configures the engine to connect to an existing browser tab.
func WithRemoteTab(tabID string, port int) Option {
	return func(e *Engine) {
		e.remoteTabID = tabID
		e.remotePort = port
	}
}

// WithRecorder sets the recorder for HAR capture with tagging support.
func WithRecorder(rec *recorder.Recorder) Option {
	return func(e *Engine) {
		e.recorder = rec
	}
}

// WithBrowser executes against an existing browser context.
// The engine does not close a browser supplied this way.
func WithBrowser(br *browser.Browser) Option {
	return func(e *Engine) {
		e.browser = br
		e.externalBrowser = true
	}
}

// WithStdout sets the writer used by commands that produce stdout.
func WithStdout(w io.Writer) Option {
	return func(e *Engine) {
		if w != nil {
			e.stdout = w
		}
	}
}

// WithStderr sets the writer used by commands that produce diagnostics.
func WithStderr(w io.Writer) Option {
	return func(e *Engine) {
		if w != nil {
			e.stderr = w
		}
	}
}

func readArchive(path string) (*archiveData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read script: %w", err)
	}
	return parseArchive(data)
}

func readArchiveReader(r io.Reader) (*archiveData, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read script: %w", err)
	}
	return parseArchive(data)
}

func parseArchive(data []byte) (*archiveData, error) {
	archive := txtar.Parse(data)
	out := &archiveData{
		Comment: cleanArchiveComment(string(archive.Comment)),
	}
	for _, f := range archive.Files {
		if f.Name == "main.cdp" {
			out.Main = string(f.Data)
			continue
		}
		out.Files = append(out.Files, f)
	}
	if out.Main == "" {
		return nil, fmt.Errorf("no main.cdp found in archive")
	}
	return out, nil
}

// HelpText returns the script-scoped help text for a txtar-backed script.
func HelpText(path string) (string, error) {
	archive, err := readArchive(path)
	if err != nil {
		return "", err
	}

	name := filepath.Base(path)
	usage := archiveUsage(archive.Comment, name)

	var b strings.Builder
	b.WriteString(name)
	b.WriteString("\n\n")
	if archive.Comment != "" {
		b.WriteString(archive.Comment)
		b.WriteString("\n\n")
	}
	b.WriteString(usage)
	b.WriteString("\n")
	return b.String(), nil
}

func archiveUsage(comment, name string) string {
	lines := strings.Split(comment, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(trimmed), "usage:") {
			continue
		}
		usage := strings.TrimSpace(trimmed[len("usage:"):])
		if usage != "" {
			return "Usage: " + usage
		}
		for _, next := range lines[i+1:] {
			next = strings.TrimSpace(next)
			if next != "" {
				return "Usage: " + next
			}
		}
		break
	}
	return "Usage: " + name + " [args...]"
}

func cleanArchiveComment(comment string) string {
	var out []string
	for _, line := range strings.Split(comment, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#!") {
			continue
		}
		if trimmed == "#" {
			out = append(out, "")
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			out = append(out, strings.TrimPrefix(trimmed, "# "))
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			out = append(out, strings.TrimPrefix(trimmed, "#"))
			continue
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func appendArgEnv(env []string, argv []string) []string {
	for i, arg := range argv {
		env = append(env, fmt.Sprintf("ARG%d=%s", i+1, arg))
	}
	env = append(env, fmt.Sprintf("ARGC=%d", len(argv)))
	return env
}

// ExecuteTxtar runs a script from a txtar archive.
func (e *Engine) ExecuteTxtar(ctx context.Context, path string, argv []string) error {
	archive, err := readArchive(path)
	if err != nil {
		return err
	}
	return e.executeArchive(ctx, filepath.Base(path), archive, argv)
}

// ExecuteReader runs a script from a txtar archive read from r.
func (e *Engine) ExecuteReader(ctx context.Context, name string, r io.Reader, argv []string) error {
	archive, err := readArchiveReader(r)
	if err != nil {
		return err
	}
	return e.executeArchive(ctx, name, archive, argv)
}

// ExecuteScript runs a plain .cdp script body.
func (e *Engine) ExecuteScript(ctx context.Context, name, body string, argv []string) error {
	archive := &archiveData{Main: body}
	return e.executeArchive(ctx, name, archive, argv)
}

func (e *Engine) executeArchive(ctx context.Context, name string, archive *archiveData, argv []string) error {
	defer e.cleanup()
	// Build initial environment
	env := []string{}
	env = append(env, e.env...)
	env = appendArgEnv(env, argv)

	// Create script state
	workDir, err := os.MkdirTemp("", "cdpscript-*")
	if err != nil {
		return fmt.Errorf("failed to create workdir: %w", err)
	}
	defer os.RemoveAll(workDir)

	// Write embedded files to workdir so commands like jsfile can access them
	for _, f := range archive.Files {
		filePath := filepath.Join(workDir, f.Name)
		// Create parent directories if needed
		if dir := filepath.Dir(filePath); dir != workDir {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory for %s: %w", f.Name, err)
			}
		}
		if err := os.WriteFile(filePath, f.Data, 0644); err != nil {
			return fmt.Errorf("failed to write embedded file %s: %w", f.Name, err)
		}
		if e.verbose {
			fmt.Fprintf(e.stderr, "[engine] Extracted embedded file: %s (%d bytes)\n", f.Name, len(f.Data))
		}
	}

	state, err := script.NewState(ctx, workDir, env)
	if err != nil {
		return fmt.Errorf("failed to create state: %w", err)
	}

	// Execute
	var logWriter io.Writer = io.Discard
	if e.verbose {
		logWriter = e.stderr
	}

	if name == "" {
		name = "main.cdp"
	}
	return e.engine.Execute(state, name, bufio.NewReader(strings.NewReader(archive.Main)), logWriter)
}

func (e *Engine) ensureBrowser(ctx context.Context) error {
	if e.browser != nil {
		return nil
	}
	if err := e.initBrowser(ctx); err != nil {
		return fmt.Errorf("failed to init browser: %w", err)
	}
	return nil
}

// initBrowser initializes the browser instance.
func (e *Engine) initBrowser(ctx context.Context) error {
	// If a remote tab is specified, connect to it instead of launching a new browser
	if e.remoteTabID != "" {
		return e.connectToRemoteTab(ctx)
	}

	// Detect browser path
	chromePath := e.detectBrowserPath()
	if chromePath == "" {
		return fmt.Errorf("no Chromium-based browser found")
	}
	if e.verbose {
		fmt.Fprintf(e.stderr, "[engine] Using browser: %s\n", chromePath)
	}

	// Build browser options
	browserOpts := []browser.Option{
		browser.WithHeadless(e.headless),
		browser.WithTimeout(int(e.scriptTimeout().Seconds())),
		browser.WithChromePath(chromePath),
	}
	if e.verbose {
		browserOpts = append(browserOpts, browser.WithVerbose(true))
	}

	// Create and launch browser
	br, err := browser.New(ctx, nil, browserOpts...)
	if err != nil {
		return fmt.Errorf("failed to create browser: %w", err)
	}

	if err := br.Launch(ctx); err != nil {
		return fmt.Errorf("failed to launch browser: %w", err)
	}

	e.browser = br
	return nil
}

// connectToRemoteTab connects to an existing browser tab.
func (e *Engine) connectToRemoteTab(ctx context.Context) error {
	port := e.remotePort
	if port == 0 {
		port = 9222
	}

	if e.verbose {
		fmt.Fprintf(e.stderr, "[engine] Connecting to remote tab %s on port %d\n", e.remoteTabID, port)
	}

	// Create browser with remote options
	browserOpts := []browser.Option{
		browser.WithRemoteChrome("localhost", port),
		browser.WithRemoteTab(e.remoteTabID),
	}
	if e.verbose {
		browserOpts = append(browserOpts, browser.WithVerbose(true))
	}

	br, err := browser.New(ctx, nil, browserOpts...)
	if err != nil {
		return fmt.Errorf("failed to create browser: %w", err)
	}

	if err := br.Launch(ctx); err != nil {
		return fmt.Errorf("failed to connect to tab: %w", err)
	}

	e.browser = br
	return nil
}

func (e *Engine) detectBrowserPath() string {
	return discovery.FindBestBrowser()
}

func (e *Engine) scriptTimeout() time.Duration {
	if e.timeout > 0 {
		return e.timeout
	}
	return defaultScriptTimeout
}

func (e *Engine) cleanup() {
	if e.externalBrowser {
		return
	}
	if e.browser != nil {
		e.browser.Close()
		e.browser = nil
	}
	if e.recorder != nil {
		e.recorder.Close()
	}
}

// commands returns the CDP commands for the script engine.
func (e *Engine) commands() map[string]script.Cmd {
	cmds := map[string]script.Cmd{
		// Navigation
		"goto":    e.cmdGoto(),
		"back":    e.cmdBack(),
		"forward": e.cmdForward(),
		"reload":  e.cmdReload(),

		// Waiting
		"wait": e.cmdWait(),

		// Interaction
		"click":     e.cmdClick(),
		"dblclick":  e.cmdDblclick(),
		"fill":      e.cmdFill(),
		"type":      e.cmdType(),
		"drag":      e.cmdDrag(),
		"mouse":     e.cmdMouse(),
		"set-range": e.cmdSetRange(),
		"hover":     e.cmdHover(),
		"press":     e.cmdPress(),
		"scroll":    e.cmdScroll(),
		"select":    e.cmdSelect(),
		"upload":    e.cmdUpload(),

		// Dialogs
		"dialog": e.cmdDialog(),

		// Emulation
		"viewport": e.cmdViewport(),

		// JavaScript
		"js":     e.cmdJS(),
		"jsfile": e.cmdJSFile(),

		// Extraction & inspection
		"extract": e.cmdExtract(),
		"title":   e.cmdTitle(),
		"url":     e.cmdURL(),
		"render":  e.cmdRender(),

		// Assertions
		"assert": e.cmdAssert(),

		// Output
		"screenshot":    e.cmdScreenshot(),
		"pdf":           e.cmdPDF(),
		"log":           e.cmdLog(),
		"download-dir":  e.cmdDownloadDir(),
		"wait-download": e.cmdWaitDownload(),

		// Network
		"block":  e.cmdBlock(),
		"cookie": e.cmdCookie(),

		// Scripting
		"source": e.cmdSource(),

		// Accessibility & refs
		"snapshot": e.cmdSnapshot(),

		// HAR recording & tagging
		"tag":     e.cmdTag(),
		"har":     e.cmdHAR(),
		"note":    e.cmdNote(),
		"capture": e.cmdCapture(),
	}

	// Add any dynamically loaded commands from sourced scripts
	for name, cmd := range e.sourcedCmds {
		cmds[name] = cmd
	}

	return cmds
}

// conditions returns the conditions for the script engine.
func (e *Engine) conditions() map[string]script.Cond {
	return map[string]script.Cond{
		"headless": script.BoolCondition("running headless", e.headless),
		"has-tab":  script.BoolCondition("connected to existing tab", e.remoteTabID != ""),
	}
}

// CommandNames returns the canonical cdpscript command names.
func CommandNames() []string {
	cmds := New().commands()
	names := make([]string, 0, len(cmds))
	for name := range cmds {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ValidateReader checks that r contains a txtar cdpscript archive and that its
// main.cdp uses known commands. It does not start a browser or execute actions.
func ValidateReader(name string, r io.Reader) error {
	archive, err := readArchiveReader(r)
	if err != nil {
		return err
	}
	return ValidateScript(name, archive.Main)
}

// ValidateScript checks a plain .cdp script body for known commands and basic
// rsc.io/script syntax. It does not start a browser or execute actions.
func ValidateScript(name, body string) error {
	if name == "" {
		name = "main.cdp"
	}
	e := New()
	cmds := make(map[string]script.Cmd, len(e.engine.Cmds))
	for name, cmd := range e.engine.Cmds {
		usage := *cmd.Usage()
		cmds[name] = script.Command(usage, func(s *script.State, args ...string) (script.WaitFunc, error) {
			return nil, nil
		})
	}
	for _, name := range sourcedAliases(body) {
		cmds[name] = script.Command(script.CmdUsage{
			Summary: "sourced command from script",
			Args:    "[args...]",
		}, func(s *script.State, args ...string) (script.WaitFunc, error) {
			return nil, nil
		})
	}
	ve := &script.Engine{Cmds: cmds, Conds: e.conditions()}
	state, err := script.NewState(context.Background(), os.TempDir(), nil)
	if err != nil {
		return err
	}
	if err := validateCommands(name, body, cmds); err != nil {
		return err
	}
	return ve.Execute(state, name, bufio.NewReader(strings.NewReader(body)), io.Discard)
}

func sourcedAliases(body string) []string {
	var aliases []string
	for _, line := range strings.Split(body, "\n") {
		fields := commandFields(line)
		if len(fields) == 0 || fields[0] != "source" {
			continue
		}
		for i := 1; i < len(fields)-1; i++ {
			if fields[i] == "-as" {
				aliases = append(aliases, fields[i+1])
				break
			}
		}
	}
	return aliases
}

func validateCommands(name, body string, cmds map[string]script.Cmd) error {
	for i, line := range strings.Split(body, "\n") {
		fields := commandFields(line)
		if len(fields) == 0 {
			continue
		}
		if _, ok := cmds[fields[0]]; !ok {
			return fmt.Errorf("%s:%d: unknown command %q", name, i+1, fields[0])
		}
	}
	return nil
}

func commandFields(line string) []string {
	fields := strings.Fields(strings.TrimSpace(line))
	for len(fields) > 0 {
		word := fields[0]
		switch {
		case word == "!" || word == "?":
			fields = fields[1:]
		case strings.HasPrefix(word, "[") && strings.HasSuffix(word, "]"):
			fields = fields[1:]
		case strings.HasPrefix(word, "#"):
			return nil
		default:
			return fields
		}
	}
	return nil
}

// Helper to create a simple command
func simpleCmd(summary, args string, run func(s *script.State, args []string) error) script.Cmd {
	return script.Command(
		script.CmdUsage{Summary: summary, Args: args},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			return nil, run(s, args)
		},
	)
}

func (e *Engine) cmdGoto() script.Cmd {
	return simpleCmd("navigate to URL", "url", func(s *script.State, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("goto requires a URL")
		}
		if err := e.ensureBrowser(s.Context()); err != nil {
			return err
		}
		url := args[0]
		if e.verbose {
			fmt.Fprintf(e.stderr, "[goto] %s\n", url)
		}
		return e.browser.Navigate(url)
	})
}

func (e *Engine) cmdWait() script.Cmd {
	return simpleCmd("wait for duration or selector", "[duration|selector]", func(s *script.State, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("wait requires duration or selector")
		}
		arg := strings.Join(args, " ")

		// Try parsing as duration first
		if d, err := time.ParseDuration(arg); err == nil {
			if e.verbose {
				fmt.Fprintf(e.stderr, "[wait] %v\n", d)
			}
			time.Sleep(d)
			return nil
		}

		if err := e.ensureBrowser(s.Context()); err != nil {
			return err
		}
		// Otherwise treat as selector
		if e.verbose {
			fmt.Fprintf(e.stderr, "[wait] for selector: %s\n", arg)
		}
		return e.browser.WaitForSelector(arg, e.scriptTimeout())
	})
}

func (e *Engine) cmdClick() script.Cmd {
	return simpleCmd("click element", "selector|@ref", func(s *script.State, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("click requires a selector or ref")
		}
		if err := e.ensureBrowser(s.Context()); err != nil {
			return err
		}
		target := strings.Join(args, " ")
		if e.verbose {
			fmt.Fprintf(e.stderr, "[click] %s\n", target)
		}
		page := e.browser.GetCurrentPage()
		if page == nil {
			return fmt.Errorf("no active page")
		}

		if p, ok, err := cdpinput.ParseCoordSelector(target); ok || err != nil {
			if err != nil {
				return err
			}
			return chromedp.Run(e.browser.Context(), chromedp.ActionFunc(func(ctx context.Context) error {
				return cdpinput.ClickAt(ctx, p)
			}))
		}

		// Check if target is a ref
		if role, name, nth, isRef := e.resolveRef(target); isRef {
			if e.verbose {
				fmt.Fprintf(e.stderr, "[click] resolved ref to role=%s name=%q nth=%d\n", role, name, nth)
			}
			return page.ClickByRole(role, name, nth)
		}

		return page.Click(target)
	})
}

func (e *Engine) cmdFill() script.Cmd {
	return simpleCmd("fill input field", "selector|@ref value", func(s *script.State, args []string) error {
		if len(args) < 2 {
			return fmt.Errorf("fill requires selector/ref and value")
		}
		if err := e.ensureBrowser(s.Context()); err != nil {
			return err
		}
		target := args[0]
		value := strings.Join(args[1:], " ")
		if e.verbose {
			fmt.Fprintf(e.stderr, "[fill] %s = %s\n", target, value)
		}
		page := e.browser.GetCurrentPage()
		if page == nil {
			return fmt.Errorf("no active page")
		}

		// Check if target is a ref
		if role, name, nth, isRef := e.resolveRef(target); isRef {
			if e.verbose {
				fmt.Fprintf(e.stderr, "[fill] resolved ref to role=%s name=%q nth=%d\n", role, name, nth)
			}
			return page.TypeByRole(role, name, value, nth)
		}

		return page.Type(target, value)
	})
}

func (e *Engine) cmdType() script.Cmd {
	// type is an alias for fill with the same ref support
	return e.cmdFill()
}

func (e *Engine) cmdScreenshot() script.Cmd {
	return simpleCmd("take screenshot", "filename", func(s *script.State, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("screenshot requires filename")
		}
		if err := e.ensureBrowser(s.Context()); err != nil {
			return err
		}
		filename := args[0]
		if e.outputDir != "" && !filepath.IsAbs(filename) {
			filename = filepath.Join(e.outputDir, filename)
		}

		if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		if e.verbose {
			fmt.Fprintf(e.stderr, "[screenshot] %s\n", filename)
		}

		var data []byte
		if err := chromedp.Run(e.browser.Context(), chromedp.FullScreenshot(&data, screenshotQuality(filename))); err != nil {
			return fmt.Errorf("failed to take screenshot: %w", err)
		}
		if err := os.WriteFile(filename, data, 0644); err != nil {
			return fmt.Errorf("failed to write screenshot: %w", err)
		}
		fmt.Fprintf(e.stderr, "Saved screenshot to %s (%d bytes)\n", filename, len(data))
		return nil
	})
}

func screenshotQuality(filename string) int {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".jpg", ".jpeg":
		return 90
	default:
		return 100
	}
}

func (e *Engine) cmdJS() script.Cmd {
	return simpleCmd("execute JavaScript", "code", func(s *script.State, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("js requires code")
		}
		if err := e.ensureBrowser(s.Context()); err != nil {
			return err
		}
		code := strings.Join(args, " ")
		if e.verbose {
			fmt.Fprintf(e.stderr, "[js] %s\n", code)
		}
		_, err := e.browser.ExecuteScript(code)
		return err
	})
}

func (e *Engine) cmdJSFile() script.Cmd {
	return simpleCmd("execute JavaScript from file", "filename", func(s *script.State, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("jsfile requires filename")
		}
		if err := e.ensureBrowser(s.Context()); err != nil {
			return err
		}
		filename := args[0]
		// If path is relative, resolve it relative to the script's workdir
		if !filepath.IsAbs(filename) {
			filename = filepath.Join(s.Getwd(), filename)
		}
		data, err := os.ReadFile(filename)
		if err != nil {
			return fmt.Errorf("failed to read JS file: %w", err)
		}
		code := string(data)
		if e.verbose {
			fmt.Fprintf(e.stderr, "[jsfile] %s (%d bytes)\n", filename, len(data))
		}
		result, err := e.browser.ExecuteScript(code)
		if err != nil {
			return err
		}
		// If the result is meaningful, print it
		if result != nil {
			fmt.Fprintf(e.stdout, "%v\n", result)
		}
		return nil
	})
}

func (e *Engine) cmdLog() script.Cmd {
	return simpleCmd("log message", "message", func(s *script.State, args []string) error {
		msg := strings.Join(args, " ")
		fmt.Fprintln(e.stdout, msg)
		return nil
	})
}

func (e *Engine) cmdPDF() script.Cmd {
	return simpleCmd("save page as PDF", "filename", func(s *script.State, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("pdf requires filename")
		}
		if err := e.ensureBrowser(s.Context()); err != nil {
			return err
		}
		filename := args[0]
		if e.outputDir != "" && !filepath.IsAbs(filename) {
			filename = filepath.Join(e.outputDir, filename)
		}

		if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		if e.verbose {
			fmt.Fprintf(e.stderr, "[pdf] %s\n", filename)
		}

		var data []byte
		if err := chromedp.Run(e.browser.Context(), chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			data, _, err = page.PrintToPDF().Do(ctx)
			return err
		})); err != nil {
			return fmt.Errorf("failed to generate PDF: %w", err)
		}

		return os.WriteFile(filename, data, 0644)
	})
}

func (e *Engine) cmdExtract() script.Cmd {
	return simpleCmd("extract text from element", "selector", func(s *script.State, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("extract requires selector")
		}
		if err := e.ensureBrowser(s.Context()); err != nil {
			return err
		}
		selector := strings.Join(args, " ")
		page := e.browser.GetCurrentPage()
		if page == nil {
			return fmt.Errorf("no active page")
		}
		text, err := page.GetText(selector)
		if err != nil {
			return err
		}
		s.Setenv("EXTRACTED", text)
		fmt.Fprintln(e.stdout, text)
		return nil
	})
}

func (e *Engine) cmdHover() script.Cmd {
	return simpleCmd("hover over element", "selector", func(s *script.State, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("hover requires selector")
		}
		if err := e.ensureBrowser(s.Context()); err != nil {
			return err
		}
		selector := strings.Join(args, " ")
		page := e.browser.GetCurrentPage()
		if page == nil {
			return fmt.Errorf("no active page")
		}
		return page.Hover(selector)
	})
}

func (e *Engine) cmdPress() script.Cmd {
	return simpleCmd("press keyboard key", "key", func(s *script.State, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("press requires key")
		}
		if err := e.ensureBrowser(s.Context()); err != nil {
			return err
		}
		key := args[0]
		page := e.browser.GetCurrentPage()
		if page == nil {
			return fmt.Errorf("no active page")
		}
		return page.Press(key)
	})
}

func (e *Engine) cmdBack() script.Cmd {
	return simpleCmd("go back in history", "", func(s *script.State, args []string) error {
		if err := e.ensureBrowser(s.Context()); err != nil {
			return err
		}
		return e.navigateHistory(-1)
	})
}

func (e *Engine) cmdForward() script.Cmd {
	return simpleCmd("go forward in history", "", func(s *script.State, args []string) error {
		if err := e.ensureBrowser(s.Context()); err != nil {
			return err
		}
		return e.navigateHistory(1)
	})
}

func (e *Engine) cmdReload() script.Cmd {
	return simpleCmd("reload page", "", func(s *script.State, args []string) error {
		if err := e.ensureBrowser(s.Context()); err != nil {
			return err
		}
		return chromedp.Run(e.browser.Context(), chromedp.Reload())
	})
}

func (e *Engine) navigateHistory(offset int64) error {
	return chromedp.Run(e.browser.Context(), chromedp.ActionFunc(func(ctx context.Context) error {
		current, entries, err := page.GetNavigationHistory().Do(ctx)
		if err != nil {
			return fmt.Errorf("reading navigation history: %w", err)
		}

		target := current + offset
		if target < 0 || target >= int64(len(entries)) {
			return fmt.Errorf("no history entry at offset %d", offset)
		}

		if e.verbose {
			fmt.Fprintf(e.stderr, "[history] current=%d target=%d url=%s\n", current, target, entries[target].URL)
		}
		return page.NavigateToHistoryEntry(entries[target].ID).Do(ctx)
	}))
}

func (e *Engine) cmdSource() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "source a CDP script file",
			Args:    "[-x] [-as name] path",
			Detail: []string{
				"Loads and executes a .cdp script file.",
				"  -x       trace execution (show each command before running)",
				"  -as name register script as a new command with given name",
			},
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			// Parse flags
			trace := false
			asName := ""
			var scriptPath string

			i := 0
			for i < len(args) {
				switch args[i] {
				case "-x":
					trace = true
					i++
				case "-as":
					if i+1 >= len(args) {
						return nil, fmt.Errorf("source: -as requires a name argument")
					}
					asName = args[i+1]
					i += 2
				default:
					scriptPath = args[i]
					i++
				}
			}

			if scriptPath == "" {
				return nil, fmt.Errorf("source: path required")
			}

			// Read the script file
			data, err := os.ReadFile(scriptPath)
			if err != nil {
				return nil, fmt.Errorf("source: failed to read %s: %w", scriptPath, err)
			}

			scriptContent := string(data)

			// If -as is specified, register as a command instead of executing
			if asName != "" {
				e.registerSourcedCommand(asName, scriptContent, trace)
				if e.verbose {
					fmt.Fprintf(e.stderr, "[source] Registered command: %s\n", asName)
				}
				// Rebuild the engine commands to include the new one
				e.engine.Cmds = e.commands()
				return nil, nil
			}

			// Execute the script inline
			return nil, e.executeSourcedScript(s, scriptContent, trace)
		},
	)
}

// registerSourcedCommand registers a sourced script as a callable command.
func (e *Engine) registerSourcedCommand(name, scriptContent string, trace bool) {
	e.sourcedCmds[name] = script.Command(
		script.CmdUsage{
			Summary: fmt.Sprintf("sourced command from script"),
			Args:    "[args...]",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			// Set arguments as environment variables
			for i, arg := range args {
				s.Setenv(fmt.Sprintf("ARG%d", i+1), arg)
			}
			s.Setenv("ARGC", fmt.Sprintf("%d", len(args)))

			return nil, e.executeSourcedScript(s, scriptContent, trace)
		},
	)
}

func (e *Engine) cmdTitle() script.Cmd {
	return simpleCmd("get page title", "", func(s *script.State, args []string) error {
		if err := e.ensureBrowser(s.Context()); err != nil {
			return err
		}
		var title string
		if err := chromedp.Run(e.browser.Context(), chromedp.Title(&title)); err != nil {
			return fmt.Errorf("failed to get title: %w", err)
		}
		s.Setenv("TITLE", title)
		fmt.Fprintln(e.stdout, title)
		return nil
	})
}

func (e *Engine) cmdURL() script.Cmd {
	return simpleCmd("get current URL", "", func(s *script.State, args []string) error {
		if err := e.ensureBrowser(s.Context()); err != nil {
			return err
		}
		var url string
		if err := chromedp.Run(e.browser.Context(), chromedp.Location(&url)); err != nil {
			return fmt.Errorf("failed to get URL: %w", err)
		}
		s.Setenv("URL", url)
		fmt.Fprintln(e.stdout, url)
		return nil
	})
}

func (e *Engine) cmdRender() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "render page or element as markdown",
			Args:    "[--term] [selector]",
			Detail: []string{
				"Gets the outerHTML of the page (or a CSS selector) and converts it to markdown.",
				"  --term     render through terminal formatter instead of raw markdown",
				"  selector   CSS selector to scope rendering (default: body)",
				"",
				"Sets $RENDERED with the markdown output.",
			},
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			termRender := false
			selector := "body"

			for i := 0; i < len(args); i++ {
				switch args[i] {
				case "--term":
					termRender = true
				default:
					selector = strings.Join(args[i:], " ")
					i = len(args) // consume remaining
				}
			}

			if err := e.ensureBrowser(s.Context()); err != nil {
				return nil, err
			}
			if e.verbose {
				fmt.Fprintf(e.stderr, "[render] selector=%s term=%v\n", selector, termRender)
			}

			var html string
			if err := chromedp.Run(e.browser.Context(), chromedp.OuterHTML(selector, &html)); err != nil {
				return nil, fmt.Errorf("getting HTML: %w", err)
			}

			markdown, err := htmltomd.Convert(html)
			if err != nil {
				return nil, fmt.Errorf("converting to markdown: %w", err)
			}

			output := strings.TrimSpace(markdown)
			if termRender {
				rendered, err := termmd.RenderMarkdown(output)
				if err != nil {
					return nil, fmt.Errorf("rendering for terminal: %w", err)
				}
				output = rendered
			}

			s.Setenv("RENDERED", output)
			fmt.Fprintln(e.stdout, output)
			return nil, nil
		},
	)
}

func (e *Engine) cmdAssert() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "assert condition on page",
			Args:    "exists|text|visible|status|response|header args...",
			Detail: []string{
				"Assert conditions on the page:",
				"  assert exists selector             - element exists in DOM",
				"  assert text selector text          - element contains text",
				"  assert visible selector            - element is visible",
				"  assert status url-substr code      - last matching response has status",
				"  assert response url-substr text    - last matching response body contains text",
				"  assert header url-substr name text - last matching response header contains text",
			},
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) < 2 {
				return nil, fmt.Errorf("assert requires condition and selector")
			}
			condition := args[0]
			selector := args[1]

			switch condition {
			case "exists":
				if err := e.ensureBrowser(s.Context()); err != nil {
					return nil, err
				}
				var count int
				err := chromedp.Run(e.browser.Context(),
					chromedp.Evaluate(fmt.Sprintf(`document.querySelectorAll(%q).length`, selector), &count),
				)
				if err != nil {
					return nil, fmt.Errorf("failed to check existence: %w", err)
				}
				if count == 0 {
					return nil, fmt.Errorf("%w: no elements found for selector %q", ErrAssertionFailed, selector)
				}
				if e.verbose {
					fmt.Fprintf(e.stderr, "[assert] exists %s: found %d elements\n", selector, count)
				}

			case "text":
				if err := e.ensureBrowser(s.Context()); err != nil {
					return nil, err
				}
				if len(args) < 3 {
					return nil, fmt.Errorf("assert text requires expected text")
				}
				expected := strings.Join(args[2:], " ")
				var text string
				err := chromedp.Run(e.browser.Context(), chromedp.Text(selector, &text))
				if err != nil {
					return nil, fmt.Errorf("failed to get text: %w", err)
				}
				if !strings.Contains(text, expected) {
					return nil, fmt.Errorf("%w: text %q does not contain %q", ErrAssertionFailed, text, expected)
				}
				if e.verbose {
					fmt.Fprintf(e.stderr, "[assert] text %s contains %q\n", selector, expected)
				}

			case "visible":
				if err := e.ensureBrowser(s.Context()); err != nil {
					return nil, err
				}
				var visible bool
				err := chromedp.Run(e.browser.Context(),
					chromedp.Evaluate(fmt.Sprintf(`
						(function() {
							const el = document.querySelector(%q);
							if (!el) return false;
							const style = window.getComputedStyle(el);
							return style.display !== 'none' && style.visibility !== 'hidden' && style.opacity !== '0';
						})()
					`, selector), &visible),
				)
				if err != nil {
					return nil, fmt.Errorf("failed to check visibility: %w", err)
				}
				if !visible {
					return nil, fmt.Errorf("%w: element %q is not visible", ErrAssertionFailed, selector)
				}
				if e.verbose {
					fmt.Fprintf(e.stderr, "[assert] visible %s: true\n", selector)
				}

			case "status":
				if len(args) != 3 {
					return nil, fmt.Errorf("assert status requires url substring and status code")
				}
				return nil, e.assertStatus(args[1], args[2])

			case "response":
				if len(args) < 3 {
					return nil, fmt.Errorf("assert response requires url substring and expected text")
				}
				return nil, e.assertResponseContains(args[1], strings.Join(args[2:], " "))

			case "header":
				if len(args) < 4 {
					return nil, fmt.Errorf("assert header requires url substring, header name, and expected text")
				}
				return nil, e.assertHeaderContains(args[1], args[2], strings.Join(args[3:], " "))

			default:
				return nil, fmt.Errorf("unknown assertion type: %s (use exists, text, visible, status, response, or header)", condition)
			}

			return nil, nil
		},
	)
}

func (e *Engine) assertStatus(pattern, want string) error {
	entry, err := e.findRecordedEntry(pattern)
	if err != nil {
		return err
	}
	got := fmt.Sprint(entry.Response.Status)
	if got != want {
		return fmt.Errorf("%w: response %q status = %s, want %s", ErrAssertionFailed, entry.Request.URL, got, want)
	}
	return nil
}

func (e *Engine) assertResponseContains(pattern, want string) error {
	entry, err := e.findRecordedEntry(pattern)
	if err != nil {
		return err
	}
	text := entry.Response.Content.Text
	if entry.Response.Content.Encoding == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(text)
		if err == nil {
			text = string(decoded)
		}
	}
	if !strings.Contains(text, want) {
		return fmt.Errorf("%w: response %q body does not contain %q", ErrAssertionFailed, entry.Request.URL, want)
	}
	return nil
}

func (e *Engine) assertHeaderContains(pattern, name, want string) error {
	entry, err := e.findRecordedEntry(pattern)
	if err != nil {
		return err
	}
	for _, h := range entry.Response.Headers {
		if strings.EqualFold(h.Name, name) {
			if strings.Contains(h.Value, want) {
				return nil
			}
			return fmt.Errorf("%w: response %q header %s = %q, want it to contain %q", ErrAssertionFailed, entry.Request.URL, h.Name, h.Value, want)
		}
	}
	return fmt.Errorf("%w: response %q missing header %q", ErrAssertionFailed, entry.Request.URL, name)
}

func (e *Engine) findRecordedEntry(pattern string) (*har.Entry, error) {
	if e.recorder == nil {
		return nil, fmt.Errorf("no HAR recording active (use 'tag' command first)")
	}
	h, err := e.recorder.HAR()
	if err != nil {
		return nil, err
	}
	for i := len(h.Log.Entries) - 1; i >= 0; i-- {
		entry := h.Log.Entries[i]
		if entry.Request != nil && strings.Contains(entry.Request.URL, pattern) {
			return entry, nil
		}
	}
	return nil, fmt.Errorf("%w: no recorded response matches %q", ErrAssertionFailed, pattern)
}

func (e *Engine) cmdCookie() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "get, set, or clear browser cookies",
			Args:    "get [name] | set name value [domain] [path] | clear [name]",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) < 1 {
				return nil, fmt.Errorf("cookie requires get, set, or clear")
			}
			if err := e.ensureBrowser(s.Context()); err != nil {
				return nil, err
			}
			switch args[0] {
			case "get":
				return nil, e.cookieGet(args[1:])
			case "set":
				return nil, e.cookieSet(args[1:])
			case "clear":
				return nil, e.cookieClear(args[1:])
			default:
				return nil, fmt.Errorf("cookie: unknown action %q", args[0])
			}
		},
	)
}

func (e *Engine) cookieGet(args []string) error {
	cookies, err := network.GetCookies().Do(e.browser.Context())
	if err != nil {
		return fmt.Errorf("cookie get: %w", err)
	}
	if len(args) == 0 {
		data, err := json.MarshalIndent(cookies, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(e.stdout, string(data))
		return nil
	}
	for _, c := range cookies {
		if c.Name == args[0] {
			fmt.Fprintln(e.stdout, c.Value)
			return nil
		}
	}
	return fmt.Errorf("%w: cookie %q not found", ErrAssertionFailed, args[0])
}

func (e *Engine) cookieSet(args []string) error {
	if len(args) < 2 || len(args) > 4 {
		return fmt.Errorf("cookie set requires name value [domain] [path]")
	}
	var loc string
	_ = chromedp.Run(e.browser.Context(), chromedp.Location(&loc))
	cmd := network.SetCookie(args[0], args[1]).WithURL(loc)
	if len(args) >= 3 {
		cmd = cmd.WithDomain(args[2])
	}
	if len(args) == 4 {
		cmd = cmd.WithPath(args[3])
	}
	if err := cmd.Do(e.browser.Context()); err != nil {
		return fmt.Errorf("cookie set: %w", err)
	}
	return nil
}

func (e *Engine) cookieClear(args []string) error {
	switch len(args) {
	case 0:
		return network.ClearBrowserCookies().Do(e.browser.Context())
	case 1:
		var loc string
		_ = chromedp.Run(e.browser.Context(), chromedp.Location(&loc))
		return network.DeleteCookies(args[0]).WithURL(loc).Do(e.browser.Context())
	default:
		return fmt.Errorf("cookie clear accepts at most one cookie name")
	}
}

func (e *Engine) cmdBlock() script.Cmd {
	return simpleCmd("block URLs matching pattern", "pattern", func(s *script.State, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("block requires a URL pattern")
		}
		if err := e.ensureBrowser(s.Context()); err != nil {
			return err
		}
		pattern := args[0]
		if e.verbose {
			fmt.Fprintf(e.stderr, "[block] %s\n", pattern)
		}
		// Use the browser's blocking mechanism
		return e.browser.BlockURLPattern(pattern)
	})
}

// executeSourcedScript executes script content with optional tracing.
func (e *Engine) executeSourcedScript(s *script.State, content string, trace bool) error {
	// Process line by line
	lines := strings.Split(content, "\n")

	for lineNo, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Expand environment variables in the line
		line = s.ExpandEnv(line, false)

		// Trace output if enabled
		if trace {
			fmt.Fprintf(e.stderr, "+ %s\n", line)
		}

		// Parse the line into command and args
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		cmdName := parts[0]
		cmdArgs := parts[1:]

		// Look up the command
		cmd, ok := e.engine.Cmds[cmdName]
		if !ok {
			return fmt.Errorf("source: line %d: unknown command: %s", lineNo+1, cmdName)
		}

		// Execute it
		waitFn, err := cmd.Run(s, cmdArgs...)
		if err != nil {
			return fmt.Errorf("source: line %d: %s: %w", lineNo+1, cmdName, err)
		}
		if waitFn != nil {
			if _, _, err := waitFn(s); err != nil {
				return fmt.Errorf("source: line %d: %s (wait): %w", lineNo+1, cmdName, err)
			}
		}
	}

	return nil
}

func (e *Engine) cmdSnapshot() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "get accessibility snapshot with element refs",
			Args:    "[-i] [--depth N] [--compact] [--selector CSS]",
			Detail: []string{
				"Returns an accessibility tree with refs that can be used to interact with elements.",
				"  -i, --interactive  only include interactive elements (buttons, links, inputs)",
				"  --depth N          limit tree depth (0 = unlimited)",
				"  --compact          remove structural elements without meaningful content",
				"  --selector CSS     scope snapshot to elements matching CSS selector",
				"",
				"Example output:",
				"  - heading \"Example\" [ref=e1] [level=1]",
				"  - button \"Submit\" [ref=e2]",
				"  - textbox \"Email\" [ref=e3]",
				"",
				"Use refs with click/fill/type commands: click @e2",
			},
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if err := e.ensureBrowser(s.Context()); err != nil {
				return nil, err
			}
			opts := browser.SnapshotOptions{}

			// Parse flags
			for i := 0; i < len(args); i++ {
				switch args[i] {
				case "-i", "--interactive":
					opts.Interactive = true
				case "--compact":
					opts.Compact = true
				case "--depth":
					if i+1 >= len(args) {
						return nil, fmt.Errorf("snapshot: --depth requires a number")
					}
					i++
					var depth int
					if _, err := fmt.Sscanf(args[i], "%d", &depth); err != nil {
						return nil, fmt.Errorf("snapshot: invalid depth: %s", args[i])
					}
					opts.MaxDepth = depth
				case "--selector":
					if i+1 >= len(args) {
						return nil, fmt.Errorf("snapshot: --selector requires a CSS selector")
					}
					i++
					opts.Selector = args[i]
				default:
					return nil, fmt.Errorf("snapshot: unknown flag: %s", args[i])
				}
			}

			page := e.browser.GetCurrentPage()
			if page == nil {
				return nil, fmt.Errorf("no active page")
			}

			snapshot, err := page.GetAccessibilitySnapshot(opts)
			if err != nil {
				return nil, fmt.Errorf("getting snapshot: %w", err)
			}

			// Store refs for later use
			e.refMap = snapshot.Refs

			// Print the tree
			fmt.Fprintln(e.stdout, snapshot.Tree)

			// Print stats if verbose
			if e.verbose {
				stats := browser.GetSnapshotStats(snapshot.Tree, snapshot.Refs)
				fmt.Fprintf(e.stderr, "[snapshot] %d refs, %d interactive, %d lines\n",
					stats["refs"], stats["interactive"], stats["lines"])
			}

			// Set environment variable with ref count
			s.Setenv("SNAPSHOT_REFS", fmt.Sprintf("%d", len(snapshot.Refs.Refs)))

			return nil, nil
		},
	)
}

// resolveRef resolves a ref (like "@e1" or "e1") to role, name, nth for interaction.
// Returns empty strings if not a ref.
func (e *Engine) resolveRef(target string) (role, name string, nth int, isRef bool) {
	ref := browser.ParseRef(target)
	if ref == "" {
		return "", "", 0, false
	}

	if e.refMap == nil || e.refMap.Refs == nil {
		return "", "", 0, false
	}

	entry, ok := e.refMap.Refs[ref]
	if !ok {
		return "", "", 0, false
	}

	return entry.Role, entry.Name, entry.Nth, true
}

func (e *Engine) cmdTag() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "set tag for network activity",
			Args:    "[tag-name]",
			Detail: []string{
				"Tags subsequent network requests until a new tag is set or cleared.",
				"Tags are included in HAR output for filtering and organization.",
				"",
				"Examples:",
				"  tag login-flow    # Tag requests as 'login-flow'",
				"  tag               # Clear current tag",
				"  tag dashboard     # Tag requests as 'dashboard'",
			},
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if err := e.ensureBrowser(s.Context()); err != nil {
				return nil, err
			}
			if e.recorder == nil {
				// Create a recorder if one doesn't exist
				rec, err := recorder.New(recorder.WithVerbose(e.verbose))
				if err != nil {
					return nil, fmt.Errorf("creating recorder: %w", err)
				}
				e.recorder = rec

				// Enable network events first
				if err := chromedp.Run(e.browser.Context(), network.Enable()); err != nil {
					return nil, fmt.Errorf("enabling network events: %w", err)
				}

				// Start recording network events
				chromedp.ListenTarget(e.browser.Context(), e.recorder.HandleNetworkEvent(e.browser.Context()))
				if e.verbose {
					fmt.Fprintf(e.stderr, "[tag] Started HAR recording\n")
				}
			}

			tag := ""
			if len(args) > 0 {
				tag = strings.Join(args, "-")
			}

			e.recorder.SetTag(tag)
			s.Setenv("CURRENT_TAG", tag)

			if e.verbose {
				if tag != "" {
					fmt.Fprintf(e.stderr, "[tag] Set to: %s\n", tag)
				} else {
					fmt.Fprintf(e.stderr, "[tag] Cleared\n")
				}
			}

			return nil, nil
		},
	)
}

func (e *Engine) cmdHAR() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "write HAR file",
			Args:    "filename",
			Detail: []string{
				"Writes captured network activity to a HAR file.",
				"Includes tags, annotations, and tag ranges.",
				"",
				"Example:",
				"  har output.har",
			},
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) < 1 {
				return nil, fmt.Errorf("har requires filename")
			}

			if e.recorder == nil {
				return nil, fmt.Errorf("no HAR recording active (use 'tag' command first)")
			}

			filename := args[0]
			if e.outputDir != "" && !filepath.IsAbs(filename) {
				filename = filepath.Join(e.outputDir, filename)
			}

			if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
				return nil, fmt.Errorf("creating directory: %w", err)
			}

			if err := e.recorder.WriteHAR(filename); err != nil {
				return nil, fmt.Errorf("writing HAR: %w", err)
			}

			if e.verbose {
				fmt.Fprintf(e.stderr, "[har] Written to %s\n", filename)
			}
			fmt.Fprintf(e.stderr, "HAR saved to %s\n", filename)

			return nil, nil
		},
	)
}

func (e *Engine) cmdNote() script.Cmd {
	return simpleCmd("add note to HAR", "description", func(s *script.State, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("note requires description")
		}
		if err := e.ensureBrowser(s.Context()); err != nil {
			return err
		}

		if e.recorder == nil {
			return fmt.Errorf("no HAR recording active (use 'tag' command first)")
		}

		description := strings.Join(args, " ")
		if err := e.recorder.AddNote(e.browser.Context(), description); err != nil {
			return fmt.Errorf("adding note: %w", err)
		}

		if e.verbose {
			fmt.Fprintf(e.stderr, "[note] Added: %s\n", description)
		}

		return nil
	})
}

func (e *Engine) cmdCapture() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "capture screenshot or DOM to HAR",
			Args:    "screenshot|dom [description]",
			Detail: []string{
				"Captures a screenshot or DOM snapshot and adds to HAR annotations.",
				"",
				"Examples:",
				"  capture screenshot Login page loaded",
				"  capture dom Before form submission",
			},
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) < 1 {
				return nil, fmt.Errorf("capture requires type (screenshot or dom)")
			}
			if err := e.ensureBrowser(s.Context()); err != nil {
				return nil, err
			}

			if e.recorder == nil {
				return nil, fmt.Errorf("no HAR recording active (use 'tag' command first)")
			}

			captureType := strings.ToLower(args[0])
			description := ""
			if len(args) > 1 {
				description = strings.Join(args[1:], " ")
			}

			switch captureType {
			case "screenshot":
				if err := e.recorder.AddScreenshot(e.browser.Context(), description); err != nil {
					return nil, fmt.Errorf("capturing screenshot: %w", err)
				}
				if e.verbose {
					fmt.Fprintf(e.stderr, "[capture] Screenshot: %s\n", description)
				}

			case "dom":
				if err := e.recorder.AddDOMSnapshot(e.browser.Context(), description); err != nil {
					return nil, fmt.Errorf("capturing DOM: %w", err)
				}
				if e.verbose {
					fmt.Fprintf(e.stderr, "[capture] DOM: %s\n", description)
				}

			default:
				return nil, fmt.Errorf("unknown capture type: %s (use screenshot or dom)", captureType)
			}

			return nil, nil
		},
	)
}
