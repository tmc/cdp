package cdpscripttest

import (
	"bufio"
	"bytes"
	"context"
	"errors"
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
	reportpkg "github.com/tmc/cdp/cdpscripttest/report"
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
// When set, Test run on testdata/login.txt saves screenshots in
// testdata/artifacts/login/. No path argument needed.
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

var flagReportDir = flag.String("cdp-report-dir", "", "write reports to this directory")

var flagEmitReportHTML = flag.Bool("emit-cdp-report-html", false, "write HTML alongside Markdown reports")

// flagCombinedReport writes a combined index.md (and index.html with
// -emit-cdp-report-html) at the artifact root, linking every script not
// marked "# report:detail". The index is rewritten as scripts complete.
// Implies -emit-cdp-report.
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
	runCapture(t, e, s, filename, r)
}

// runCapture executes the script and returns the captured engine log.
func runCapture(t testing.TB, e *Engine, s *State, filename string, r io.Reader) string {
	t.Helper()

	var captured string
	err := func() (err error) {
		logBuf := new(strings.Builder)
		logBuf.WriteString("\n")

		var logW io.Writer = logBuf

		t.Helper()
		cov, err := startCoverage(s)
		if err != nil {
			return err
		}
		defer func() {
			t.Helper()
			if path, frames, stopErr := s.stopScreenRecordingIfActive(); stopErr != nil {
				s.Logf("screenrecord stop: %v\n", stopErr)
				if err == nil {
					err = stopErr
				}
			} else if path != "" {
				s.Logf("%s\nframes: %d\n", path, frames)
			}
			if closeErr := s.CloseAndWait(logBuf); err == nil {
				err = closeErr
			}
			if covErr := finishCoverage(cov, s); err == nil {
				err = covErr
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

	switch {
	case err == nil || errors.Is(err, ErrStop):
		// stop ends the script early; that is not a failure.
	case errors.Is(err, ErrSkip):
		t.Skip(err)
	default:
		t.Errorf("FAIL: %v", err)
	}
	return captured
}

// Test discovers txtar scripts matching pattern and runs each as a parallel
// subtest. Each subtest gets its own browser and a fresh temporary working
// directory.
//
// allocCtx must be a chromedp allocator context, from chromedp.NewExecAllocator.
// Do not pass a browser context that has already been run: the subtests would
// share its browser, and Page.startScreencast delivers frames only for the
// foreground target, so parallel screenrecord scripts would capture nothing.
//
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
		// has a location (e.g. testdata/artifacts/).
		artRoot = filepath.Join(filepath.Dir(files[0]), "artifacts")
	}

	if *flagReportDir != "" {
		artRoot = *flagReportDir
	}
	emitReport := *flagEmitReport || *flagReportDir != "" || *flagEmitReportHTML || *flagCombinedReport

	// The report writer receives the complete manifest before parallel subtests
	// begin. All report flags use this writer, so the test and CLI entry points
	// produce the same report tree.
	var reportWriter *reportpkg.Writer
	if emitReport {
		scripts, err := ReportManifest(files)
		if err != nil {
			t.Fatal(err)
		}
		w, err := reportpkg.NewWriter(reportpkg.Options{
			Dir:      artRoot,
			HTML:     *flagEmitReportHTML,
			Combined: *flagCombinedReport,
		}, scripts)
		if err != nil {
			t.Logf("create report writer: %v", err)
		} else {
			reportWriter = w
			t.Cleanup(func() {
				if err := w.Close(); err != nil {
					t.Logf("close report writer: %v", err)
				}
			})
		}
	}

	var combinedMu sync.Mutex

	for _, file := range files {
		name := ScriptName(file)
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Each subtest gets its own browser.
			tabCtx, tabCancel := chromedp.NewContext(allocCtx)
			t.Cleanup(tabCancel)

			workdir := t.TempDir()
			var artDir string
			if emitArtifacts {
				// Derive from script location: testdata/x.txt -> testdata/artifacts/x/
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

			captured := runCapture(t, e, s, file, bytes.NewReader(a.Comment))

			if reportWriter != nil {
				combinedMu.Lock()
				err := reportWriter.Update(reportpkg.Script{
					Name:        name,
					Source:      a.Comment,
					Log:         captured,
					ArtifactDir: artDir,
					Failed:      t.Failed(),
				})
				combinedMu.Unlock()
				if err != nil {
					t.Logf("report generation failed: %v", err)
				} else {
					t.Attr("cdp.report", filepath.Join(artDir, "report.md"))
				}
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
