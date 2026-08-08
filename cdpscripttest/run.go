package cdpscripttest

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/chromedp/chromedp"
	"github.com/tmc/cdp/cdpscripttest/report"
	"golang.org/x/tools/txtar"
)

// ScriptResult is the result of running a single txtar script file.
type ScriptResult struct {
	File string
	Err  error
	Log  string
}

// RunResult groups results from RunFiles.
type RunResult struct {
	Results []ScriptResult
}

// Passed reports how many scripts passed.
func (r RunResult) Passed() int {
	n := 0
	for _, sr := range r.Results {
		if sr.Err == nil {
			n++
		}
	}
	return n
}

// Failed reports how many scripts failed.
func (r RunResult) Failed() int {
	return len(r.Results) - r.Passed()
}

// RunOptions configures RunFiles.
type RunOptions struct {
	// BaseURL is prepended to navigate command paths.
	BaseURL string

	// ArtifactDir is where screenshots are saved. If empty, defaults to
	// a "screenshots" subdirectory inside each script's temp workdir.
	ArtifactDir string

	// AllocatorOpts are passed to chromedp.NewExecAllocator.
	// If nil, chromedp.DefaultExecAllocatorOptions is used with headless=true.
	AllocatorOpts []chromedp.ExecAllocatorOption

	// Env is the initial environment for scripts. nil uses os.Environ().
	Env []string

	// OnResult is called after each script completes.
	OnResult func(ScriptResult)

	// EmitReport generates a report.md in the artifact directory.
	EmitReport bool

	// Report enables detailed and optional combined reports.
	Report *report.Options
}

// RunFiles runs each txtar script file in files using the given engine and
// options. It returns a RunResult with per-file outcomes.
//
// RunFiles manages the allocator lifecycle internally.
func RunFiles(ctx context.Context, e *Engine, files []string, opts RunOptions) (RunResult, error) {
	var reporter *report.Writer
	sources := make(map[string][]byte)
	if opts.Report != nil {
		scripts := make([]report.Script, 0, len(files))
		for _, file := range files {
			source, err := scriptSource(file)
			if err != nil {
				return RunResult{}, err
			}
			name := strings.TrimSuffix(filepath.Base(file), ".txt")
			sources[file] = source
			scripts = append(scripts, report.Script{
				Name:        name,
				Source:      source,
				ArtifactDir: filepath.Join(opts.Report.Dir, name),
			})
		}
		var err error
		reporter, err = report.NewWriter(*opts.Report, scripts)
		if err != nil {
			return RunResult{}, fmt.Errorf("create report writer: %w", err)
		}
	}

	allocOpts := opts.AllocatorOpts
	if allocOpts == nil {
		allocOpts = append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("headless", true),
		)
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, allocOpts...)
	defer allocCancel()

	var result RunResult
	var reportErr error

	for _, file := range files {
		sr := runFile(allocCtx, e, file, opts)
		result.Results = append(result.Results, sr)
		if reporter != nil {
			name := strings.TrimSuffix(filepath.Base(file), ".txt")
			if err := reporter.Update(report.Script{
				Name:        name,
				Source:      sources[file],
				Log:         sr.Log,
				ArtifactDir: filepath.Join(opts.Report.Dir, name),
				Failed:      sr.Err != nil,
			}); err != nil {
				if reportErr == nil {
					reportErr = fmt.Errorf("update report for %q: %w", file, err)
				}
			}
		}
		if opts.OnResult != nil {
			opts.OnResult(sr)
		}
	}
	if reporter != nil {
		if err := reporter.Close(); err != nil && reportErr == nil {
			reportErr = fmt.Errorf("close report: %w", err)
		}
	}
	if reportErr != nil {
		return result, reportErr
	}

	return result, nil
}

func runFile(allocCtx context.Context, e *Engine, file string, opts RunOptions) ScriptResult {
	tabCtx, tabCancel := chromedp.NewContext(allocCtx)
	defer tabCancel()

	workdir, err := os.MkdirTemp("", "cdpscripttest-*")
	if err != nil {
		return ScriptResult{File: file, Err: fmt.Errorf("mkdirtemp: %w", err)}
	}
	defer os.RemoveAll(workdir)

	artifactDir := opts.ArtifactDir
	if opts.Report != nil {
		artifactDir = filepath.Join(opts.Report.Dir, strings.TrimSuffix(filepath.Base(file), ".txt"))
	}
	s, err := NewStateWithArtifactDir(tabCtx, workdir, opts.BaseURL, artifactDir, opts.Env)
	if err != nil {
		return ScriptResult{File: file, Err: err}
	}

	a, err := txtar.ParseFile(file)
	if err != nil {
		return ScriptResult{File: file, Err: fmt.Errorf("parse txtar: %w", err)}
	}

	if err := s.ExtractFiles(a); err != nil {
		return ScriptResult{File: file, Err: fmt.Errorf("extract files: %w", err)}
	}

	logBuf := new(strings.Builder)
	logBuf.WriteString("\n")

	runErr := func() (err error) {
		cov, err := startCoverage(s)
		if err != nil {
			return err
		}
		defer func() {
			if path, frames, stopErr := s.stopScreenRecordingIfActive(); stopErr != nil {
				logBuf.WriteString(fmt.Sprintf("screenrecord stop: %v\n", stopErr))
				if err == nil {
					err = stopErr
				}
			} else if path != "" {
				logBuf.WriteString(fmt.Sprintf("%s\nframes: %d\n", path, frames))
			}
			if closeErr := s.CloseAndWait(logBuf); err == nil {
				err = closeErr
			}
			if covErr := finishCoverage(cov, s); err == nil {
				err = covErr
			}
		}()
		return e.Execute(s.State, file, bufio.NewReader(bytes.NewReader(a.Comment)), logBuf)
	}()

	log := strings.TrimSuffix(logBuf.String(), "\n")

	if opts.EmitReport && opts.ArtifactDir != "" {
		name := strings.TrimSuffix(filepath.Base(file), ".txt")
		reportPath := filepath.Join(opts.ArtifactDir, "report.md")
		_ = GenerateReport(reportPath, name, a.Comment, logBuf.String())
	}

	return ScriptResult{
		File: file,
		Err:  runErr,
		Log:  log,
	}
}

func scriptSource(file string) ([]byte, error) {
	a, err := txtar.ParseFile(file)
	if err != nil {
		return nil, fmt.Errorf("parse txtar %q: %w", file, err)
	}
	return a.Comment, nil
}

// ExpandGlobs expands patterns into script file paths. Supports:
//   - exact paths: testdata/foo.txt
//   - shell globs: testdata/*.txt
//   - Go-style recursive: testdata/... (all *.txt under testdata/)
func ExpandGlobs(patterns []string) ([]string, error) {
	var files []string
	for _, p := range patterns {
		// Treat bare directories as recursive: testdata/ → testdata/...
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			p = strings.TrimRight(p, "/") + "/..."
		}
		if strings.HasSuffix(p, "/...") || p == "..." {
			root := strings.TrimSuffix(p, "/...")
			if p == "..." {
				root = "."
			}
			if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !d.IsDir() && strings.HasSuffix(path, ".txt") {
					files = append(files, path)
				}
				return nil
			}); err != nil {
				return nil, fmt.Errorf("walk %q: %w", root, err)
			}
			continue
		}
		matches, err := filepath.Glob(p)
		if err != nil {
			return nil, fmt.Errorf("glob %q: %w", p, err)
		}
		if len(matches) == 0 {
			files = append(files, p)
		} else {
			files = append(files, matches...)
		}
	}
	return files, nil
}

// discardWriter implements io.Writer by discarding all writes.
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

var _ io.Writer = discardWriter{}
