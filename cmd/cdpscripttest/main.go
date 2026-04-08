// Command cdpscripttest runs CDP txtar browser automation scripts.
//
// Usage:
//
//	cdpscripttest [flags] script.txt ...
//	cdpscripttest --url https://example.com testdata/example-com.txt
//	cdpscripttest --url http://localhost:8090 --headful testdata/login.txt
//	cdpscripttest --interactive testdata/login.txt
//	cdpscripttest --debug-port 9222 testdata/live.txt
//
// Each script is a txtar archive whose Comment section is the script body
// and whose file sections are fixtures extracted before the script runs.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/tmc/cdp/cdpscripttest"
	"golang.org/x/term"
	"golang.org/x/tools/txtar"
)

func main() {
	os.Exit(run())
}

func run() int {
	fs := flag.NewFlagSet("cdpscripttest", flag.ContinueOnError)
	baseURL := fs.String("url", "http://localhost:8090", "base URL for navigate commands")
	headful := fs.Bool("headful", false, "run Chrome with a visible window")
	interactive := fs.Bool("interactive", false, "drop to cdp> REPL after each script")
	iFlag := fs.Bool("i", false, "alias for --interactive")
	artifacts := fs.String("artifacts", "", "directory to persist screenshots (default: screenshots/ next to each script)")
	updateGolden := fs.Bool("update-golden", false, "overwrite baseline screenshots instead of comparing (golden update mode)")
	timeout := fs.Duration("timeout", 2*time.Minute, "per-script timeout")
	watch := fs.Bool("watch", false, "re-run scripts on file changes")
	noColor := fs.Bool("no-color", false, "disable ANSI color output")
	verbose := fs.Bool("v", false, "verbose: show full stdout content (useful for text/html commands)")
	browserPath := fs.String("browser", "", "path to Chrome/Brave/Chromium binary (auto-detected if empty)")
	debugPort := fs.Int("debug-port", 0, "connect to an existing Chrome debug port instead of launching a new browser")
	windowSize := fs.String("window-size", "", "browser window size as WxH (e.g. 1280x900); defaults to terminal size")
	inlineImages := fs.Bool("images", false, "display screenshots inline in the terminal (auto-detects iTerm2/Kitty protocol)")
	emitReport := fs.Bool("emit-cdp-report", false, "generate report.md in the artifact directory")
	combinedReport := fs.Bool("emit-cdp-report-combined", false, "write all reports into one combined file (implies --emit-cdp-report)")

	if err := fs.Parse(os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if *iFlag {
		*interactive = true
	}

	isColorEnabled := !*noColor && isatty(os.Stdout) && os.Getenv("NO_COLOR") == ""
	imgProto := imageProtocol(*inlineImages)
	out := &printer{w: os.Stdout, color: isColorEnabled, imageProto: imgProto, verbose: *verbose}

	args := fs.Args()
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "cdpscripttest: no scripts specified\n")
		fs.Usage()
		return 2
	}

	files, err := cdpscripttest.ExpandGlobs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cdpscripttest: %v\n", err)
		return 2
	}

	e := cdpscripttest.NewCLIEngine()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Resolve browser window dimensions.
	w, h := resolveWindowSize(*windowSize)

	var allocCtx context.Context
	var allocCancel context.CancelFunc

	if *debugPort > 0 {
		// Connect to an already-running Chrome instance via debug port.
		remoteURL := fmt.Sprintf("http://localhost:%d", *debugPort)
		allocCtx, allocCancel = chromedp.NewRemoteAllocator(ctx, remoteURL)
		out.info(fmt.Sprintf("connecting to existing browser at %s", remoteURL))
	} else {
		// Launch a new browser process.
		path := *browserPath
		if path == "" {
			path = findBrowser()
		}

		opts := append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("headless", !*headful),
			chromedp.WindowSize(w, h),
		)
		if path != "" {
			opts = append(opts, chromedp.ExecPath(path))
			out.info(fmt.Sprintf("using browser: %s", path))
		}
		allocCtx, allocCancel = chromedp.NewExecAllocator(ctx, opts...)
	}
	defer allocCancel()

	if *combinedReport {
		*emitReport = true
	}

	if *watch {
		return runWatch(ctx, allocCtx, e, files, *baseURL, *artifacts, *timeout, *updateGolden, *emitReport, *combinedReport, out)
	}
	return runOnce(ctx, allocCtx, e, files, *baseURL, *artifacts, *timeout, *updateGolden, *interactive, *emitReport, *combinedReport, out)
}

// resolveWindowSize parses an optional "WxH" string; if empty it uses the
// current terminal dimensions scaled up to a reasonable browser size.
func resolveWindowSize(s string) (w, h int) {
	if s != "" {
		fmt.Sscanf(s, "%dx%d", &w, &h)
		if w > 0 && h > 0 {
			return
		}
	}
	// Use terminal dimensions if available, scaled by a factor to account
	// for the fact that terminal cells are taller than they are wide.
	tw, th, err := term.GetSize(int(os.Stdout.Fd()))
	if err == nil && tw > 0 && th > 0 {
		// Approximate: 1 terminal col ≈ 8px, 1 terminal row ≈ 16px.
		w = tw * 8
		h = th * 16
		// Clamp to sensible browser bounds.
		if w < 800 {
			w = 800
		}
		if w > 2560 {
			w = 2560
		}
		if h < 600 {
			h = 600
		}
		if h > 1600 {
			h = 1600
		}
		return
	}
	return 1280, 900 // safe default
}

// findBrowser returns the path to the first usable Chrome-family browser found
// on this system. CDP_BROWSER env var overrides auto-discovery. Returns empty
// string if none is found (chromedp will try its own default discovery).
func findBrowser() string {
	if p := os.Getenv("CDP_BROWSER"); p != "" {
		return p
	}
	candidates := browserCandidates()
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
		if path, err := exec.LookPath(p); err == nil {
			return path
		}
	}
	return ""
}

// browserCandidates returns a prioritized list of browser executable paths
// for the current OS. Brave is preferred per project conventions.
func browserCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"brave",
			"google-chrome",
			"chromium",
		}
	case "linux":
		return []string{
			"brave-browser",
			"brave",
			"google-chrome",
			"google-chrome-stable",
			"chromium-browser",
			"chromium",
			"/usr/bin/brave-browser",
			"/usr/bin/google-chrome",
			"/usr/bin/chromium-browser",
		}
	case "windows":
		return []string{
			`C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`,
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		}
	default:
		return nil
	}
}

func runOnce(ctx context.Context, allocCtx context.Context, e *cdpscripttest.Engine,
	files []string, baseURL, artifactDir string, timeout time.Duration,
	updateGolden, interactive, emitReport, combinedReport bool, out *printer) int {

	exitCode := 0

	// Live-updating combined report.
	var combinedWriter *cdpscripttest.CombinedReportWriter
	if combinedReport && artifactDir != "" {
		names := make([]string, 0, len(files))
		sources := make(map[string][]byte, len(files))
		for _, file := range files {
			name := strings.TrimSuffix(filepath.Base(file), ".txt")
			a, err := txtar.ParseFile(file)
			if err != nil {
				continue
			}
			if cdpscripttest.ExtractReportLevel(a.Comment) == cdpscripttest.ReportOverview {
				names = append(names, name)
				sources[name] = a.Comment
			}
		}
		reportPath := filepath.Join(artifactDir, "report.md")
		w, err := cdpscripttest.NewCombinedReportWriter(reportPath, names, sources)
		if err != nil {
			out.info(fmt.Sprintf("combined report: %v", err))
		} else {
			combinedWriter = w
			out.info(fmt.Sprintf("combined report: %s", reportPath))
		}
	}

	for _, file := range files {
		out.fileHeader(file)

		tctx, tcancel := context.WithTimeout(allocCtx, timeout)
		tabCtx, tabCancel := chromedp.NewContext(tctx)

		workdir, _ := os.MkdirTemp("", "cdpscripttest-*")

		// Default artifact dir: screenshots/ next to the script file.
		artDir := artifactDir
		if artDir == "" {
			artDir = filepath.Join(filepath.Dir(file), "screenshots")
		}

		name := strings.TrimSuffix(filepath.Base(file), ".txt")

		s, err := cdpscripttest.NewStateWithArtifactDir(tabCtx, workdir, baseURL, artDir, nil)
		if err == nil {
			if updateGolden {
				s.SetUpdateGolden(true)
			}
		}
		if err != nil {
			out.failure(fmt.Sprintf("state init: %v", err))
			tabCancel()
			tcancel()
			os.RemoveAll(workdir)
			exitCode = 1
			continue
		}

		a, err := txtar.ParseFile(file)
		if err != nil {
			out.failure(fmt.Sprintf("parse %s: %v", file, err))
			tabCancel()
			tcancel()
			os.RemoveAll(workdir)
			exitCode = 1
			continue
		}

		_ = s.ExtractFiles(a)

		logBuf := new(strings.Builder)
		logBuf.WriteString("\n")

		runErr := e.Execute(s.State, file, bufio.NewReader(bytes.NewReader(a.Comment)), logBuf)

		log := strings.TrimSuffix(logBuf.String(), "\n")
		if log != "" {
			out.scriptLog(log)
		}

		if runErr != nil && errors.Is(runErr, cdpscripttest.ErrSkip) {
			out.skip(filepath.Base(file), runErr.Error())
		} else if runErr != nil && errors.Is(runErr, cdpscripttest.ErrStop) {
			out.pass(filepath.Base(file)) // stop is a clean early exit, not a failure
		} else if runErr != nil {
			out.failure(runErr.Error())
			exitCode = 1
			if interactive {
				out.info("dropping to interactive shell (Ctrl-D or 'exit' to quit)")
				repl(s, e, out)
			}
		} else {
			out.pass(filepath.Base(file))
			if interactive {
				out.info("script passed — interactive mode (Ctrl-D or 'exit' to quit)")
				repl(s, e, out)
			}
		}

		// Close state after REPL is done (if interactive) or immediately.
		if closeErr := s.CloseAndWait(logBuf); runErr == nil && closeErr != nil {
			out.failure(fmt.Sprintf("close: %v", closeErr))
			exitCode = 1
		}

		if emitReport && artDir != "" {
			scriptArtDir := filepath.Join(artDir, name)
			reportPath := filepath.Join(scriptArtDir, "report.md")
			if err := cdpscripttest.GenerateReport(reportPath, name, a.Comment, logBuf.String()); err != nil {
				out.info(fmt.Sprintf("report generation failed: %v", err))
			} else {
				out.info(fmt.Sprintf("report: %s", reportPath))
			}
		}

		// Update combined report (skip detail-only scripts).
		if combinedWriter != nil && cdpscripttest.ExtractReportLevel(a.Comment) == cdpscripttest.ReportOverview {
			scriptFailed := runErr != nil && !errors.Is(runErr, cdpscripttest.ErrSkip) && !errors.Is(runErr, cdpscripttest.ErrStop)
			if err := combinedWriter.Update(cdpscripttest.ScriptReport{
				Name:        name,
				Source:      a.Comment,
				Log:         logBuf.String(),
				ArtifactDir: filepath.Join(artDir, name),
				Failed:      scriptFailed,
			}); err != nil {
				out.info(fmt.Sprintf("combined report update: %v", err))
			}
		}

		tabCancel()
		tcancel()
		os.RemoveAll(workdir)
	}

	return exitCode
}

func runWatch(ctx context.Context, allocCtx context.Context, e *cdpscripttest.Engine,
	files []string, baseURL, artifactDir string, timeout time.Duration, updateGolden, emitReport, combinedReport bool, out *printer) int {

	mtimes := make(map[string]time.Time)
	updateMtimes := func() {
		for _, f := range files {
			if fi, err := os.Stat(f); err == nil {
				mtimes[f] = fi.ModTime()
			}
		}
	}
	updateMtimes()

	fmt.Fprintf(os.Stderr, "watching %d file(s), Ctrl-C to stop\n", len(files))
	runOnce(ctx, allocCtx, e, files, baseURL, artifactDir, timeout, updateGolden, false, emitReport, combinedReport, out)

	for {
		select {
		case <-ctx.Done():
			return 0
		case <-time.After(time.Second):
		}

		var changed []string
		for _, f := range files {
			if fi, err := os.Stat(f); err == nil && fi.ModTime() != mtimes[f] {
				changed = append(changed, f)
				mtimes[f] = fi.ModTime()
			}
		}
		if len(changed) > 0 {
			fmt.Print("\033[H\033[2J")
			out.info(fmt.Sprintf("re-running %d changed file(s)...", len(changed)))
			runOnce(ctx, allocCtx, e, changed, baseURL, artifactDir, timeout, updateGolden, false, emitReport, combinedReport, out)
		}
	}
}

// repl runs an interactive REPL reusing the live browser context.
// The state must not have been closed before calling repl.
func repl(s *cdpscripttest.State, e *cdpscripttest.Engine, out *printer) {
	r := bufio.NewReader(os.Stdin)
	for {
		out.prompt()
		line, err := r.ReadString('\n')
		if err != nil {
			fmt.Fprintln(os.Stdout)
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "exit" || line == "quit" {
			return
		}

		logBuf := new(strings.Builder)
		runErr := e.Execute(s.State, "<repl>", bufio.NewReader(strings.NewReader(line+"\n")), logBuf)

		log := strings.TrimSuffix(logBuf.String(), "\n")
		if log != "" {
			// Use same state-machine parsing as scriptLog to extract output.
			inStdout := false
			for _, l := range strings.Split(log, "\n") {
				t := strings.TrimSpace(l)
				switch {
				case t == "[stdout]":
					inStdout = true
				case strings.HasPrefix(t, ">"), t == "[stderr]", t == "":
					inStdout = false
				case strings.HasPrefix(t, "matched:"), strings.HasPrefix(t, "[condition"):
					inStdout = false
				case inStdout:
					out.replOutput(t)
				}
			}
		}
		if runErr != nil {
			out.failure(runErr.Error())
		}
	}
}

func isatty(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

const (
	ansiReset = "\033[0m"
	ansiBold  = "\033[1m"
	ansiDim   = "\033[2m"
	ansiRed   = "\033[31m"
	ansiGreen = "\033[32m"
	ansiGray  = "\033[90m"
)

// imgProtoType selects the inline image rendering protocol.
type imgProtoType int

const (
	imgProtoNone   imgProtoType = iota
	imgProtoITerm2              // ESC]1337;File=inline=1:<b64>ST
	imgProtoKitty               // ESC_G...ESC\
)

type printer struct {
	w          io.Writer
	color      bool
	imageProto imgProtoType
	verbose    bool
}

func (p *printer) c(code, s string) string {
	if !p.color {
		return s
	}
	return code + s + ansiReset
}

func (p *printer) fileHeader(file string) {
	fmt.Fprintln(p.w, p.c(ansiBold, "=== "+file))
}

// scriptLog parses the rsc.io/script engine log and prints a clean view.
//
// Engine log format:
//
//	# comment (timing)   — section header
//	> command args       — command echo
//	[stdout]\nlines      — captured stdout (followed by > stdout/stderr assertion)
//	[stderr]\nlines      — captured stderr
//	matched: ...         — internal noise, skip
//	[condition ...]      — internal noise, skip
//
// Rendering rules (Russ Cox style — minimal, functional):
//   - # comments: dim gray, timing stripped
//   - CDP commands (navigate, click, …): dim — they're the script, not results
//   - stdout/stderr assertion commands: skipped — noise, output already visible
//   - stdout content: plain, indented 4 spaces
//   - stderr content: dim red, indented
//   - screenshots (.png paths): kitty inline if enabled, else plain path
func (p *printer) scriptLog(log string) {
	lines := strings.Split(log, "\n")
	inStdout := false
	inStderr := false

	// assertionCmds are scripttest built-ins that verify output — skip them.
	isAssertion := func(cmd string) bool {
		switch strings.Fields(cmd)[0] {
		case "stdout", "stderr", "stdin", "cmp", "cmpenv", "grep", "! stdout", "! stderr":
			return true
		}
		return strings.HasPrefix(cmd, "! ")
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		switch {
		case trimmed == "":
			inStdout = false
			inStderr = false

		case strings.HasPrefix(trimmed, "#"):
			inStdout = false
			inStderr = false
			comment := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
			// Strip trailing timing like "(0.271s)"
			if i := strings.LastIndex(comment, "("); i > 0 && strings.HasSuffix(comment, "s)") {
				comment = strings.TrimSpace(comment[:i])
			}
			if comment != "" {
				fmt.Fprintln(p.w, p.c(ansiGray, "    # "+comment))
			}

		case strings.HasPrefix(trimmed, ">"):
			inStdout = false
			inStderr = false
			cmd := strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))
			if isAssertion(cmd) {
				// Suppress stdout/stderr assertion commands — output already shown.
				break
			}
			fmt.Fprintln(p.w, p.c(ansiDim, "    "+cmd))

		case trimmed == "[stdout]":
			inStdout = true
			inStderr = false

		case trimmed == "[stderr]":
			inStderr = true
			inStdout = false

		case trimmed == "[condition not met]",
			strings.HasPrefix(trimmed, "matched:"),
			strings.HasPrefix(trimmed, "[condition"):
			inStdout = false
			inStderr = false

		case inStdout:
			if p.imageProto != imgProtoNone && strings.HasSuffix(trimmed, ".png") {
				p.displayInlineImage(trimmed)
			} else {
				display := trimmed
				if !p.verbose && len(display) > 120 {
					display = display[:117] + "..."
				}
				fmt.Fprintln(p.w, "        "+display)
			}

		case inStderr:
			fmt.Fprintln(p.w, p.c(ansiRed, "        "+trimmed))

		default:
			inStdout = false
			inStderr = false
		}
	}
}

// imageProtocol detects the best inline image protocol available.
// Returns imgProtoNone when enabled is false.
func imageProtocol(enabled bool) imgProtoType {
	if !enabled {
		return imgProtoNone
	}
	if os.Getenv("KITTY_WINDOW_ID") != "" {
		return imgProtoKitty
	}
	if os.Getenv("TERM_PROGRAM") == "iTerm.app" {
		return imgProtoITerm2
	}
	// Fall back to iTerm2 protocol for other terminals that support it
	// (WezTerm, VSCode, etc.) — it's more widely supported than Kitty.
	return imgProtoITerm2
}

// displayInlineImage writes a PNG to the terminal using the detected protocol.
func (p *printer) displayInlineImage(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(p.w, p.c(ansiGray, "        "+path+" (unreadable)"))
		return
	}
	fmt.Fprintln(p.w, "        "+path)
	switch p.imageProto {
	case imgProtoITerm2:
		writeITerm2Image(p.w, data)
	case imgProtoKitty:
		writeKittyImage(p.w, data)
	}
}

func (p *printer) pass(name string) {
	fmt.Fprintln(p.w, p.c(ansiGreen, "ok  "+name))
}

func (p *printer) skip(name, msg string) {
	fmt.Fprintln(p.w, p.c(ansiGray, "skip "+name+" ("+msg+")"))
}

func (p *printer) failure(msg string) {
	fmt.Fprintln(p.w, p.c(ansiRed, "FAIL "+msg))
}

func (p *printer) info(msg string) {
	fmt.Fprintln(p.w, p.c(ansiGray, "    "+msg))
}

func (p *printer) replOutput(line string) {
	if !p.verbose && len(line) > 120 {
		line = line[:117] + "..."
	}
	fmt.Fprintln(p.w, line)
}

func (p *printer) prompt() {
	fmt.Fprint(p.w, p.c(ansiBold, "cdp> "))
}

// writeITerm2Image displays data as an inline image using the iTerm2 protocol.
// Supported by iTerm2, WezTerm, VSCode terminal, and others.
// Width is capped at 40 columns to keep images readable without dominating the output.
func writeITerm2Image(w io.Writer, data []byte) {
	b64 := base64.StdEncoding.EncodeToString(data)
	fmt.Fprintf(w, "\033]1337;File=inline=1;size=%d;width=40;preserveAspectRatio=1:%s\a\n", len(data), b64)
}

// writeKittyImage encodes data as a Kitty terminal image. It chunks the
// base64-encoded payload into 4096-byte APC sequences as required by the
// protocol. The image is displayed at a scaled-down size (columns=40).
func writeKittyImage(w io.Writer, data []byte) {
	import64 := func(b []byte) string {
		const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
		out := make([]byte, 0, (len(b)+2)/3*4)
		for i := 0; i < len(b); i += 3 {
			n := len(b) - i
			if n > 3 {
				n = 3
			}
			var v uint32
			for j := 0; j < n; j++ {
				v |= uint32(b[i+j]) << (16 - 8*j)
			}
			out = append(out,
				chars[v>>18&0x3f],
				chars[v>>12&0x3f],
			)
			if n >= 2 {
				out = append(out, chars[v>>6&0x3f])
			} else {
				out = append(out, '=')
			}
			if n >= 3 {
				out = append(out, chars[v&0x3f])
			} else {
				out = append(out, '=')
			}
		}
		return string(out)
	}

	b64 := import64(data)
	const chunkSize = 4096
	for i := 0; i < len(b64); i += chunkSize {
		end := i + chunkSize
		if end > len(b64) {
			end = len(b64)
		}
		chunk := b64[i:end]
		more := 1
		if end >= len(b64) {
			more = 0
		}
		var payload string
		if i == 0 {
			// First chunk: include format/action headers.
			// f=100=PNG, a=T=transmit+display, m=more, c=columns.
			payload = fmt.Sprintf("a=T,f=100,m=%d,c=40,q=2;%s", more, chunk)
		} else {
			payload = fmt.Sprintf("m=%d,q=2;%s", more, chunk)
		}
		fmt.Fprintf(w, "\033_G%s\033\\", payload)
	}
	fmt.Fprintln(w)
}

// Ensure unused import doesn't linger.
