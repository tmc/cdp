package cdpscript

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	cdpbrowser "github.com/chromedp/cdproto/browser"
	"github.com/tmc/cdp/internal/chromedp"
	"rsc.io/script"
)

func (e *Engine) cmdDownloadDir() script.Cmd {
	return simpleCmd("set download directory", "dir", func(s *script.State, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("download-dir requires directory")
		}
		if err := e.ensureBrowser(s.Context()); err != nil {
			return err
		}
		dir := e.artifactPath(args[0])
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create download dir: %w", err)
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			return fmt.Errorf("download dir: %w", err)
		}
		if e.verbose {
			fmt.Fprintf(e.stderr, "[download-dir] %s\n", abs)
		}
		if err := chromedp.Run(e.browser.Context(),
			cdpbrowser.SetDownloadBehavior(cdpbrowser.SetDownloadBehaviorBehaviorAllow).
				WithDownloadPath(abs).
				WithEventsEnabled(true),
		); err != nil {
			return fmt.Errorf("set download dir: %w", err)
		}
		e.downloadMu.Lock()
		e.downloadDir = abs
		e.downloadMu.Unlock()
		s.Setenv("DOWNLOAD_DIR", abs)
		return nil
	})
}

func (e *Engine) cmdWaitDownload() script.Cmd {
	return simpleCmd("wait for downloaded file", "filename [timeout]", func(s *script.State, args []string) error {
		if len(args) < 1 || len(args) > 2 {
			return fmt.Errorf("wait-download requires filename and optional timeout")
		}
		timeout := e.scriptTimeout()
		if len(args) == 2 {
			d, err := time.ParseDuration(args[1])
			if err != nil {
				return fmt.Errorf("wait-download timeout: %w", err)
			}
			timeout = d
		}
		path, err := e.downloadPath(args[0])
		if err != nil {
			return err
		}
		if err := waitForFile(s.Context(), path, timeout); err != nil {
			return err
		}
		s.Setenv("DOWNLOADED", path)
		if e.verbose {
			fmt.Fprintf(e.stderr, "[wait-download] %s\n", path)
		}
		return nil
	})
}

func (e *Engine) artifactPath(path string) string {
	if e.outputDir != "" && !filepath.IsAbs(path) {
		return filepath.Join(e.outputDir, path)
	}
	return path
}

func (e *Engine) downloadPath(name string) (string, error) {
	if filepath.IsAbs(name) {
		return name, nil
	}
	e.downloadMu.Lock()
	dir := e.downloadDir
	e.downloadMu.Unlock()
	if dir == "" {
		return "", fmt.Errorf("download-dir must be set before wait-download")
	}
	return filepath.Join(dir, name), nil
}

// waitForFile waits for path to exist as a regular file. It returns early if
// ctx is cancelled, so a cancelled script run does not have to wait out the
// remaining timeout.
func waitForFile(ctx context.Context, path string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = defaultScriptTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			return nil
		}
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("wait-download %q: %w", path, err)
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("wait-download %q: timed out after %v", path, timeout)
			}
			return fmt.Errorf("wait-download %q: %w", path, ctx.Err())
		case <-tick.C:
		}
	}
}
