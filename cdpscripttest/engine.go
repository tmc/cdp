package cdpscripttest

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"golang.org/x/tools/txtar"
	"rsc.io/script"
)

// flagUpdateGolden is a test flag for enabling golden file update mode.
// When set, screenshot-compare overwrites baselines instead of comparing.
// The UPDATE_GOLDEN env var takes precedence if both are set.
//
// Usage: go test -update-golden -tags cdp ./...
var flagUpdateGolden = flag.Bool("update-golden", false, "update golden baseline screenshots instead of comparing")

// flagArtifacts is a test flag for setting the artifact root directory.
// When set, screenshots are saved to <dir>/<script-name>/ bypassing
// t.ArtifactDir() entirely. The CDPSCRIPTTEST_ARTIFACTS env var takes
// precedence if both are set.
//
// Usage: go test -cdp-artifacts=./out -tags cdp ./...
var flagArtifacts = flag.String("cdp-artifacts", "", "artifact root directory (bypasses t.ArtifactDir)")

// flagEmitArtifacts places screenshots alongside the script files.
// When set, testdata/cdp/fleet-view.txt produces screenshots in
// testdata/cdp/artifacts/fleet-view/. No path argument needed.
//
// Usage: go test -emit-artifacts -tags cdp ./...
var flagEmitArtifacts = flag.Bool("emit-artifacts", false, "save screenshots to <script-dir>/artifacts/<script-name>/")

// flagSkipBlur disables --blur processing so screenshots show raw content.
// Useful for inspecting what the page actually looks like before blur is applied.
//
// Usage: go test -cdp-skip-blur -tags cdp ./...
var flagSkipBlur = flag.Bool("cdp-skip-blur", false, "disable --blur processing (show raw content)")

// flagEmitUnblurred saves an additional unblurred copy alongside each blurred
// screenshot. The unblurred file is named with an -unblurred suffix, e.g.
// dashboard.png → dashboard-unblurred.png.
//
// Usage: go test -cdp-emit-unblurred -tags cdp ./...
var flagEmitUnblurred = flag.Bool("cdp-emit-unblurred", false, "save unblurred copy alongside blurred screenshots")

// flagEmitReport generates a report.md in the artifact directory after each
// script runs. The report is a GFM markdown file with inline screenshots.
//
// Usage: go test -emit-cdp-report -tags cdp ./...
var flagEmitReport = flag.Bool("emit-cdp-report", false, "generate report.md in the artifact directory")

// flagCombinedReport writes all script reports into one combined report.md
// at the artifact root. Each script gets a top-level heading. The file builds
// up incrementally as scripts complete. Implies -emit-cdp-report.
//
// Usage: go test -emit-cdp-report-combined -tags cdp ./...
var flagCombinedReport = flag.Bool("emit-cdp-report-combined", false, "write all reports into one combined file")

// Engine is a script.Engine pre-loaded with CDP commands and conditions.
type Engine struct {
	*script.Engine
}

// NewEngine returns an Engine with DefaultCmds and DefaultConds.
// Use inside go test only — DefaultConds calls testing.Short() which panics
// outside a test binary. Use NewCLIEngine for standalone CLI use.
func NewEngine() *Engine {
	return &Engine{
		Engine: &script.Engine{
			Cmds:  DefaultCmds(),
			Conds: DefaultConds(),
		},
	}
}

// NewCLIEngine returns an Engine with DefaultCmds and CLIConds, safe for use
// outside of go test (e.g. the cdpscripttest CLI binary).
func NewCLIEngine() *Engine {
	return &Engine{
		Engine: &script.Engine{
			Cmds:  DefaultCmds(),
			Conds: CLIConds(),
		},
	}
}

// Execute preprocesses r to normalize prefix-condition syntax before delegating
// to the underlying script.Engine.Execute.
//
// rsc.io/script requires colon-separated prefix conditions: [stdout:pattern].
// This wrapper also accepts the more natural space form [stdout pattern] by
// rewriting it to [stdout:pattern] for any condition key registered as a
// PrefixCondition in the engine.
func (e *Engine) Execute(s *script.State, file string, script *bufio.Reader, log io.Writer) error {
	src, err := io.ReadAll(script)
	if err != nil {
		return err
	}
	src = e.normalizeConds(src)
	return e.Engine.Execute(s, file, bufio.NewReader(bytes.NewReader(src)), log)
}

// normalizeConds rewrites [condname suffix] → [condname:suffix] for every
// prefix condition registered in the engine, so scripts may use either syntax.
func (e *Engine) normalizeConds(src []byte) []byte {
	// Build a pattern that matches [<key> <suffix>] for each prefix condition.
	for key, cond := range e.Conds {
		if !cond.Usage().Prefix {
			continue
		}
		// Match: [ key <whitespace> <non-]>+ ] — replace with [ key:<suffix> ]
		re := regexp.MustCompile(`\[` + regexp.QuoteMeta(key) + `\s+([^\]]+)\]`)
		src = re.ReplaceAll(src, []byte("["+key+":$1]"))
	}
	return src
}

// Run runs the script from filename (reading from r) using the provided State.
// It is analogous to scripttest.Run but understands CDP State.
//
// Because NewState already bakes the *State into the context passed to
// script.NewState, CDP commands can retrieve it via cdpState(s) at any time.
func Run(t testing.TB, e *Engine, s *State, filename string, r io.Reader) {
	t.Helper()
	runCapture(t, e, s, filename, r, nil)
}

// runCaptureOpts configures how runCapture behaves.
type runCaptureOpts struct {
	// ReportWriter, when non-nil, receives streaming GFM report output
	// as the script executes. Typically set to t.Output().
	ReportWriter io.Writer

	// ReportName is the script name used in the report heading.
	ReportName string

	// ReportDir is the artifact directory for relative image paths.
	ReportDir string

	// ScriptSource is the raw script for preamble extraction.
	ScriptSource []byte
}

// runCapture executes the script and returns the captured engine log.
func runCapture(t testing.TB, e *Engine, s *State, filename string, r io.Reader, opts *runCaptureOpts) string {
	t.Helper()

	var captured string
	err := func() (err error) {
		logBuf := new(strings.Builder)
		logBuf.WriteString("\n")

		// If streaming, the engine writes to the streamer which tees
		// to logBuf and the report writer (t.Output()).
		var logW io.Writer = logBuf
		var streamer *reportStreamer
		if opts != nil && opts.ReportWriter != nil {
			streamer = NewReportStreamer(logBuf, opts.ReportWriter, opts.ReportName, opts.ReportDir, opts.ScriptSource)
			logW = streamer
		}

		t.Helper()
		defer func() {
			t.Helper()
			if closeErr := s.CloseAndWait(logBuf); err == nil {
				err = closeErr
			}
			if streamer != nil {
				streamer.Flush()
			}
			captured = logBuf.String()
			if logBuf.Len() > 0 {
				t.Log(strings.TrimSuffix(logBuf.String(), "\n"))
			}
		}()

		if testing.Verbose() {
			wait, err := script.Env().Run(s.State)
			if err != nil {
				t.Fatal(err)
			}
			if wait != nil {
				stdout, stderr, err := wait(s.State)
				if err != nil {
					t.Fatalf("env: %v\n%s", err, stderr)
				}
				if len(stdout) > 0 {
					s.Logf("%s\n", stdout)
				}
			}
		}

		// Check if the context is already dead before starting the script.
		// This produces a clearer error than "file:0: context deadline exceeded".
		if err := s.cdpCtx.Err(); err != nil {
			return fmt.Errorf("%s: browser context expired before script started: %w", filename, err)
		}

		return e.Execute(s.State, filename, bufio.NewReader(r), logW)
	}()

	if err != nil {
		t.Errorf("FAIL: %v", err)
	}
	return captured
}

// Test discovers txtar scripts matching pattern and runs each as a parallel
// subtest. Each test gets an isolated browser tab (chromedp context) and a
// fresh temporary working directory.
//
// allocCtx must be a chromedp allocator context (from chromedp.NewExecAllocator).
// baseURL is prepended to paths in navigate commands.
// env is the initial environment; nil uses os.Environ().
func Test(t *testing.T, e *Engine, allocCtx context.Context, baseURL, pattern string, env []string) {
	t.Helper()

	gracePeriod := 100 * time.Millisecond
	if deadline, ok := t.Deadline(); ok {
		timeout := time.Until(deadline)
		if gp := timeout / 20; gp > gracePeriod {
			gracePeriod = gp
		}
		timeout -= 2 * gracePeriod
		var cancel context.CancelFunc
		allocCtx, cancel = context.WithTimeout(allocCtx, timeout)
		t.Cleanup(cancel)
	}

	files, _ := filepath.Glob(pattern)
	if len(files) == 0 {
		t.Fatal("no testdata matched: " + pattern)
	}

	// Determine the artifact root directory. CDPSCRIPTTEST_ARTIFACTS env var
	// takes precedence, then -cdp-artifacts flag, then -emit-artifacts (which
	// derives the root from the script file's directory), then t.ArtifactDir().
	// Each script gets a subdirectory named after its filename (sans .txt).
	artRoot := os.Getenv("CDPSCRIPTTEST_ARTIFACTS")
	if artRoot == "" && *flagArtifacts != "" {
		artRoot = *flagArtifacts
	}
	emitArtifacts := *flagEmitArtifacts && artRoot == ""
	if artRoot == "" && !emitArtifacts {
		artRoot = artifactDirForTB(t)
	}
	if emitArtifacts && artRoot == "" {
		// Derive artRoot from the script directory so the combined report
		// has a location (e.g. testdata/cdp/artifacts/).
		artRoot = filepath.Join(filepath.Dir(files[0]), "artifacts")
	}

	emitReport := *flagEmitReport || *flagCombinedReport

	// Combined report: live-updating file rewritten as each script finishes.
	var combinedMu sync.Mutex
	var combinedWriter *CombinedReportWriter
	wantCombined := *flagCombinedReport && artRoot != ""
	if wantCombined {
		// Build the full manifest from script files.
		names := make([]string, len(files))
		sources := make(map[string][]byte, len(files))
		for i, file := range files {
			name := strings.TrimSuffix(filepath.Base(file), ".txt")
			names[i] = name
			if a, err := txtar.ParseFile(file); err == nil {
				if ExtractReportLevel(a.Comment) == ReportOverview {
					sources[name] = a.Comment
				}
			}
		}
		// Filter to overview-only scripts.
		var overviewNames []string
		for _, name := range names {
			if _, ok := sources[name]; ok {
				overviewNames = append(overviewNames, name)
			}
		}
		reportPath := filepath.Join(artRoot, "report.md")
		w, err := NewCombinedReportWriter(reportPath, overviewNames, sources)
		if err != nil {
			t.Logf("combined report: %v", err)
		} else {
			combinedWriter = w
		}
	}

	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".txt")
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Each subtest gets its own browser tab.
			tabCtx, tabCancel := chromedp.NewContext(allocCtx)
			t.Cleanup(tabCancel)

			workdir := t.TempDir()
			var artDir string
			if emitArtifacts {
				// Derive from script location: testdata/cdp/x.txt → testdata/cdp/artifacts/x/
				artDir = filepath.Join(filepath.Dir(file), "artifacts", name)
			} else {
				artDir = filepath.Join(artRoot, name)
			}
			s, err := NewStateWithArtifactDir(tabCtx, workdir, baseURL, artDir, env)
			if err != nil {
				t.Fatal(err)
			}
			// Wire -update-golden flag when UPDATE_GOLDEN env var was not set.
			if !s.UpdateGolden() && *flagUpdateGolden {
				s.SetUpdateGolden(true)
			}
			if *flagSkipBlur {
				s.skipBlur = true
			}
			if *flagEmitUnblurred {
				s.emitUnblurred = true
			}

			// Unpack txtar archive: script = Comment, fixture files = Files.
			a, err := txtar.ParseFile(file)
			if err != nil {
				t.Fatal(err)
			}
			initDirs(t, s)
			if err := s.ExtractFiles(a); err != nil {
				t.Fatal(err)
			}

			t.Log(time.Now().UTC().Format(time.RFC3339))
			if work, ok := s.LookupEnv("WORK"); ok {
				t.Logf("$WORK=%s", work)
			}

			var opts *runCaptureOpts
			if emitReport {
				opts = &runCaptureOpts{
					ReportWriter: t.Output(),
					ReportName:   name,
					ReportDir:    artDir,
					ScriptSource: a.Comment,
				}
			}

			captured := runCapture(t, e, s, file, bytes.NewReader(a.Comment), opts)

			if emitReport {
				reportPath := filepath.Join(artDir, "report.md")
				if err := GenerateReport(reportPath, name, a.Comment, captured); err != nil {
					t.Logf("report generation failed: %v", err)
				} else {
					t.Attr("cdp.report", reportPath)
				}
			}

			// Update combined report (skip detail-only scripts).
			if combinedWriter != nil && ExtractReportLevel(a.Comment) == ReportOverview {
				combinedMu.Lock()
				if err := combinedWriter.Update(ScriptReport{
					Name:        name,
					Source:      a.Comment,
					Log:         captured,
					ArtifactDir: artDir,
					Failed:      t.Failed(),
				}); err != nil {
					t.Logf("combined report update: %v", err)
				}
				combinedMu.Unlock()
			}
		})
	}
}

// initDirs sets up standard environment variables (WORK, TMPDIR) in the state,
// matching what scripttest does.
func initDirs(t testing.TB, s *State) {
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	work := s.Getwd()
	must(s.Setenv("WORK", work))
	tmp := filepath.Join(work, "tmp")
	must(os.MkdirAll(tmp, 0o777))
	must(s.Setenv(tempEnvName(), tmp))
}

func tempEnvName() string {
	switch runtime.GOOS {
	case "windows":
		return "TMP"
	case "plan9":
		return "TMPDIR"
	default:
		return "TMPDIR"
	}
}
