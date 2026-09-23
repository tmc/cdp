package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/tmc/cdp/internal/chromedp"
	"github.com/tmc/cdp/internal/testutil"
)

func TestExtractExtensionID(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"service worker", "chrome-extension://abcdef123456/background.js", "abcdef123456"},
		{"popup page", "chrome-extension://xyz789/popup.html", "xyz789"},
		{"bare id with slash", "chrome-extension://onlyid/", "onlyid"},
		{"no path", "chrome-extension://onlyid", "onlyid"},
		{"regular http", "https://example.com", ""},
		{"empty", "", ""},
		{"about blank", "about:blank", ""},
		{"chrome url", "chrome://extensions/", ""},
		{"nested path", "chrome-extension://ext123/pages/options.html", "ext123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractExtensionID(tt.url)
			if got != tt.want {
				t.Errorf("extractExtensionID(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestValidStorageArea(t *testing.T) {
	tests := []struct {
		area string
		want bool
	}{
		{"local", true},
		{"sync", true},
		{"session", true},
		{"managed", true},
		{"", false},
		{"invalid", false},
		{"LOCAL", false},
	}
	for _, tt := range tests {
		t.Run(tt.area, func(t *testing.T) {
			got := validStorageArea(tt.area)
			if got != tt.want {
				t.Errorf("validStorageArea(%q) = %v, want %v", tt.area, got, tt.want)
			}
		})
	}
}

func TestStrFromMap(t *testing.T) {
	m := map[string]any{
		"name":    "Test Extension",
		"version": "1.0",
		"count":   42,
	}
	tests := []struct {
		key  string
		want string
	}{
		{"name", "Test Extension"},
		{"version", "1.0"},
		{"count", ""},   // not a string
		{"missing", ""}, // not present
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := strFromMap(m, tt.key)
			if got != tt.want {
				t.Errorf("strFromMap(m, %q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestExtensionTarget(t *testing.T) {
	const id = "abcdefghijklmnopabcdefghijklmnop"
	sw := &target.Info{TargetID: "sw", Type: "service_worker", URL: "chrome-extension://" + id + "/background.js"}
	page := &target.Info{TargetID: "page", Type: "page", URL: "chrome-extension://" + id + "/options.html"}
	devtools := &target.Info{TargetID: "devtools", Type: "iframe", URL: "chrome-extension://" + id + "/devtools.html"}
	other := &target.Info{TargetID: "other", Type: "page", URL: "https://example.com/"}
	tests := []struct {
		name    string
		targets []*target.Info
		want    target.ID
		wantOK  bool
	}{
		{"service worker preferred", []*target.Info{page, sw}, "sw", true},
		{"extension page", []*target.Info{other, page}, "page", true},
		{"devtools_page iframe", []*target.Info{other, devtools}, "devtools", true},
		{"none", []*target.Info{other}, "", false},
		{"empty", nil, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := extensionTarget(tt.targets, id)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("extensionTarget = %q, %v; want %q, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestNoExtensionTargetError(t *testing.T) {
	tests := []struct {
		name                    string
		installed, devtoolsOnly bool
		want                    string
	}{
		{"not installed", false, false, "no target found for extension x"},
		{"installed", true, false, "extension x is installed but has no inspectable target"},
		{"devtools only", true, true, "extension x has no inspectable target: DevTools-only extensions (devtools_page, no background service worker) run inside DevTools"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := noExtensionTargetError("x", tt.installed, tt.devtoolsOnly)
			if !strings.HasPrefix(err.Error(), tt.want) {
				t.Errorf("noExtensionTargetError = %q, want prefix %q", err, tt.want)
			}
		})
	}
}

func TestDeveloperPrivateExtensionPath(t *testing.T) {
	const info = `[{"id":"aaa","path":"/ext/a","name":"A"},{"id":"bbb","path":""}]`
	tests := []struct {
		name, json, id string
		wantPath       string
		wantInstalled  bool
	}{
		{"found", info, "aaa", "/ext/a", true},
		{"found without path", info, "bbb", "", true},
		{"missing", info, "ccc", "", false},
		{"bad json", "null", "aaa", "", false},
		{"not json", "undefined", "aaa", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, installed := developerPrivateExtensionPath(tt.json, tt.id)
			if path != tt.wantPath || installed != tt.wantInstalled {
				t.Errorf("developerPrivateExtensionPath = %q, %v; want %q, %v", path, installed, tt.wantPath, tt.wantInstalled)
			}
		})
	}
}

func TestManifestDevToolsOnly(t *testing.T) {
	tests := []struct {
		name, manifest string
		want           bool
	}{
		{"devtools only", `{"manifest_version":3,"devtools_page":"devtools.html"}`, true},
		{"devtools and service worker", `{"devtools_page":"devtools.html","background":{"service_worker":"bg.js"}}`, false},
		{"devtools and mv2 scripts", `{"devtools_page":"devtools.html","background":{"scripts":["bg.js"]}}`, false},
		{"service worker only", `{"background":{"service_worker":"bg.js"}}`, false},
		{"neither", `{"name":"x"}`, false},
		{"invalid", `{`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := manifestDevToolsOnly([]byte(tt.manifest)); got != tt.want {
				t.Errorf("manifestDevToolsOnly(%s) = %v, want %v", tt.manifest, got, tt.want)
			}
		})
	}
}

// TestDevToolsOnlyExtensionNoTarget loads a devtools_page-only extension in
// headless mode, where no DevTools window exists, and checks that
// extension_evaluate and extension_console explain the missing target.
func TestDevToolsOnlyExtensionNoTarget(t *testing.T) {
	skipIfNoBrowser(t)
	dir := t.TempDir()
	files := map[string]string{
		"manifest.json": `{"manifest_version":3,"name":"cdp devtools-only fixture","version":"1","devtools_page":"devtools.html"}`,
		"devtools.html": `<!doctype html><script src="devtools.js"></script>`,
		"devtools.js":   `console.log("devtools page loaded")`,
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(testutil.FindChrome()),
		chromedp.UserDataDir(t.TempDir()),
		chromedp.Flag("disable-extensions", false),
		chromedp.Flag("load-extension", dir),
	)
	alloc, stop := chromedp.NewExecAllocator(context.Background(), opts...)
	t.Cleanup(stop)
	ctx, stop := chromedp.NewContext(alloc)
	t.Cleanup(stop)
	ctx, stop = context.WithTimeout(ctx, 60*time.Second)
	t.Cleanup(stop)
	if err := chromedp.Run(ctx, chromedp.Navigate("data:text/html,<title>fixture</title>")); err != nil {
		t.Fatal(err)
	}

	infoJSON, err := runOnExtensionsPage(ctx, `chrome.developerPrivate.getExtensionsInfo()`)
	if err != nil {
		t.Fatal(err)
	}
	var infos []struct{ ID, Name string }
	if err := json.Unmarshal([]byte(infoJSON), &infos); err != nil {
		t.Fatal(err)
	}
	var id string
	for _, info := range infos {
		if info.Name == "cdp devtools-only fixture" {
			id = info.ID
		}
	}
	if id == "" {
		t.Skip("browser did not load the unpacked fixture extension")
	}

	s := &mcpSession{refs: newRefRegistry(), ctx: ctx, browserCtx: ctx}
	cs := scriptToolsClient(t, s, mcpConfig{})
	want := "extension " + id + " has no inspectable target: DevTools-only extensions"
	for tool, args := range map[string]map[string]any{
		"extension_evaluate": {"id": id, "expression": "1"},
		"extension_console":  {"id": id},
	} {
		text, _, isErr := callText(t, ctx, cs, tool, args)
		if !isErr || !strings.Contains(text, want) {
			t.Errorf("%s = %q, isError=%v; want error containing %q", tool, text, isErr, want)
		}
	}
}
