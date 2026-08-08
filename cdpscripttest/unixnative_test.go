//go:build cdp

package cdpscripttest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tmc/cdp/cdpscripttest"
	"github.com/tmc/cdp/internal/browser"
)

func TestCDPScriptUnixNativeFixtures(t *testing.T) {
	t.Run("cdpscript-argv.txtar", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		t.Cleanup(cancel)

		err := cdpscripttest.RunCDPScript(ctx, "testdata/cdpscript/cdpscript-argv.txtar", cdpscripttest.CDPScriptRunOptions{
			Args:     []string{"hello", "world"},
			Headless: true,
			Timeout:  20 * time.Second,
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("cdpscript-help-default.txtar", func(t *testing.T) {
		bin := filepath.Join(t.TempDir(), "cdpscript")
		build := exec.Command("go", "build", "-o", bin, "../cmd/cdpscript")
		build.Dir = "."
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("go build cdpscript: %v\n%s", err, out)
		}

		cmd := exec.Command(bin, "testdata/cdpscript/cdpscript-help-default.txtar", "--help")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("cdpscript --help: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
		}
		for _, want := range []string{
			"cdpscript-help-default.txtar",
			"Usage: cdpscript-help-default.txtar [args...]",
		} {
			if !strings.Contains(stdout.String(), want) {
				t.Fatalf("stdout missing %q:\n%s", want, stdout.String())
			}
		}
		if strings.Contains(stdout.String(), "-headless") || strings.Contains(stdout.String(), "-timeout") {
			t.Fatalf("stdout contains Go flag help:\n%s", stdout.String())
		}
	})

	t.Run("cdpscript-pdf-output.txtar", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		t.Cleanup(cancel)

		outDir := t.TempDir()
		err := cdpscripttest.RunCDPScript(ctx, "testdata/cdpscript/cdpscript-pdf-output.txtar", cdpscripttest.CDPScriptRunOptions{
			OutputDir: outDir,
			Headless:  true,
			Timeout:   20 * time.Second,
		})
		if err != nil {
			t.Fatal(err)
		}

		data, err := os.ReadFile(filepath.Join(outDir, "page.pdf"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(data, []byte("%PDF-")) {
			t.Fatalf("page.pdf has prefix %q, want PDF header", data[:min(len(data), 8)])
		}
	})

	t.Run("cdpscript-screenshot-output.txtar", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		t.Cleanup(cancel)

		outDir := t.TempDir()
		err := cdpscripttest.RunCDPScript(ctx, "testdata/cdpscript/cdpscript-screenshot-output.txtar", cdpscripttest.CDPScriptRunOptions{
			OutputDir: outDir,
			Headless:  true,
			Timeout:   20 * time.Second,
		})
		if err != nil {
			t.Fatal(err)
		}

		data, err := os.ReadFile(filepath.Join(outDir, "page.png"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
			t.Fatalf("page.png has prefix %q, want PNG header", data[:min(len(data), 8)])
		}
	})

	t.Run("cdpscript-input-primitives.txtar", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		t.Cleanup(cancel)

		err := cdpscripttest.RunCDPScript(ctx, "testdata/cdpscript/cdpscript-input-primitives.txtar", cdpscripttest.CDPScriptRunOptions{
			Headless: true,
			Timeout:  20 * time.Second,
			Env: []string{
				"FIXTURE_BASE_URL=" + startTestServer(t),
			},
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("cdpscript-observe-act-verify.txtar", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		t.Cleanup(cancel)

		outDir := t.TempDir()
		err := cdpscripttest.RunCDPScript(ctx, "testdata/cdpscript/cdpscript-observe-act-verify.txtar", cdpscripttest.CDPScriptRunOptions{
			OutputDir: outDir,
			Headless:  true,
			Timeout:   20 * time.Second,
			Env: []string{
				"FIXTURE_BASE_URL=" + startTestServer(t),
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		before := readPNG(t, filepath.Join(outDir, "before.png"))
		after := readPNG(t, filepath.Join(outDir, "after.png"))
		if bytes.Equal(before, after) {
			t.Fatal("before.png and after.png are identical; observe-act-verify did not capture a visible change")
		}
	})

	t.Run("cdpscript-attach-observe-act-verify.txtar", func(t *testing.T) {
		port, tabID := startRemoteCDPChrome(t)
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		t.Cleanup(cancel)

		outDir := t.TempDir()
		err := cdpscripttest.RunCDPScript(ctx, "testdata/cdpscript/cdpscript-attach-observe-act-verify.txtar", cdpscripttest.CDPScriptRunOptions{
			OutputDir: outDir,
			Timeout:   20 * time.Second,
			TabID:     tabID,
			Port:      port,
			Env: []string{
				"FIXTURE_BASE_URL=" + startTestServer(t),
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		before := readPNG(t, filepath.Join(outDir, "before.png"))
		after := readPNG(t, filepath.Join(outDir, "after.png"))
		if bytes.Equal(before, after) {
			t.Fatal("before.png and after.png are identical; attached observe-act-verify did not capture a visible change")
		}
	})

	t.Run("cdpscript-har-output.txtar", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		t.Cleanup(cancel)

		outDir := t.TempDir()
		err := cdpscripttest.RunCDPScript(ctx, "testdata/cdpscript/cdpscript-har-output.txtar", cdpscripttest.CDPScriptRunOptions{
			OutputDir: outDir,
			Headless:  true,
			Timeout:   20 * time.Second,
			Env: []string{
				"FIXTURE_BASE_URL=" + startTestServer(t),
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		data, err := os.ReadFile(filepath.Join(outDir, "session.har"))
		if err != nil {
			t.Fatal(err)
		}
		var got struct {
			Log struct {
				Entries []struct {
					Comment string `json:"comment"`
					Request struct {
						URL string `json:"url"`
					} `json:"request"`
				} `json:"entries"`
				Annotations []struct {
					Type        string `json:"type"`
					Description string `json:"description"`
				} `json:"_annotations"`
				TagRanges []struct {
					Tag string `json:"tag"`
				} `json:"_tagRanges"`
			} `json:"log"`
		}
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("decode HAR: %v\n%s", err, data)
		}

		var sawPage, sawFetch, sawTaggedEntry bool
		for _, entry := range got.Log.Entries {
			if strings.Contains(entry.Request.URL, "/interaction/pages/network-har.html") {
				sawPage = true
			}
			if strings.Contains(entry.Request.URL, "/interaction/pages/har-data.json") {
				sawFetch = true
			}
			if entry.Comment == "tag:page-load" {
				sawTaggedEntry = true
			}
		}
		if !sawPage || !sawFetch || !sawTaggedEntry {
			t.Fatalf("HAR entries missing page=%v fetch=%v tagged=%v: %#v", sawPage, sawFetch, sawTaggedEntry, got.Log.Entries)
		}

		var sawDOMAnnotation, sawTagRange bool
		for _, annotation := range got.Log.Annotations {
			if annotation.Type == "dom" && annotation.Description == "Loaded fixture DOM" {
				sawDOMAnnotation = true
			}
		}
		for _, tagRange := range got.Log.TagRanges {
			if tagRange.Tag == "page-load" {
				sawTagRange = true
			}
		}
		if !sawDOMAnnotation || !sawTagRange {
			t.Fatalf("HAR metadata missing dom annotation=%v tag range=%v", sawDOMAnnotation, sawTagRange)
		}
	})
}

func readPNG(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("%s has prefix %q, want PNG header", path, data[:min(len(data), 8)])
	}
	return data
}

func startRemoteCDPChrome(t *testing.T) (port int, tabID string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("remote Chrome tests are unreliable on Windows")
	}
	chromePath := findChromePath()
	if chromePath == "" {
		t.Skip("no Chrome-compatible browser found")
	}
	port = freePort(t)
	userDataDir := t.TempDir()
	args := []string{
		"--headless",
		"--disable-gpu",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-dev-shm-usage",
		"--remote-debugging-address=127.0.0.1",
		fmt.Sprintf("--remote-debugging-port=%d", port),
		fmt.Sprintf("--user-data-dir=%s", userDataDir),
		"about:blank",
	}
	cmd := exec.Command(chromePath, args...)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start remote Chrome: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	client := &http.Client{Timeout: 500 * time.Millisecond}
	versionURL := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)
	for i := 0; i < 60; i++ {
		resp, err := client.Get(versionURL)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if i == 59 {
			t.Fatalf("remote Chrome did not expose DevTools at %s", versionURL)
		}
		time.Sleep(100 * time.Millisecond)
	}

	for i := 0; i < 30; i++ {
		tabs, err := browser.ListTabs("127.0.0.1", port)
		if err == nil {
			for _, tab := range tabs {
				if tab.Type == "page" && tab.ID != "" {
					return port, tab.ID
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("remote Chrome did not expose a page tab")
	return 0, ""
}

func freePort(t *testing.T) int {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}
