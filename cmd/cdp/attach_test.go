package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/tmc/cdp/internal/discovery"
)

func TestDiscoverAttachReportTargets(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(attachTestHandler(`[
		{"id":"page-1","type":"page","title":"Example","url":"https://example.com/"},
		{"id":"worker-1","type":"service_worker","title":"Worker","url":"https://example.com/sw.js"}
	]`))
	defer srv.Close()

	host, port := attachServerHostPort(t, srv.URL)
	report := discoverAttachReport(host, port)
	if len(report.Errors) != 0 {
		t.Fatalf("errors = %v", report.Errors)
	}
	if len(report.Targets) != 1 {
		t.Fatalf("len(targets) = %d, want 1", len(report.Targets))
	}
	target := report.Targets[0]
	if target.ID != "page-1" {
		t.Fatalf("target id = %q, want page-1", target.ID)
	}
	want := fmt.Sprintf("cdp --remote-host %s --remote-port %d --tab page-1 --shell", host, port)
	if target.Command != want {
		t.Fatalf("command = %q, want %q", target.Command, want)
	}
}

func TestDiscoverAttachReportNoPageTargets(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(attachTestHandler(`[
		{"id":"worker-1","type":"service_worker","title":"Worker","url":"https://example.com/sw.js"}
	]`))
	defer srv.Close()

	host, port := attachServerHostPort(t, srv.URL)
	report := discoverAttachReport(host, port)
	if len(report.Targets) != 0 {
		t.Fatalf("len(targets) = %d, want 0", len(report.Targets))
	}
	if len(report.Instructions) == 0 {
		t.Fatal("instructions are empty")
	}
	if got := report.Instructions[len(report.Instructions)-1]; got != fmt.Sprintf("cdp attach --port %d", port) {
		t.Fatalf("last instruction = %q", got)
	}
}

func TestAttachCommandJSON(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(attachTestHandler(`[
		{"id":"page-1","type":"page","title":"Example","url":"https://example.com/"}
	]`))
	defer srv.Close()

	host, port := attachServerHostPort(t, srv.URL)
	cmd := newAttachCmd()
	var out bytes.Buffer
	cmd.stdout = &out
	cmd.stderr = &bytes.Buffer{}
	if err := cmd.run([]string{"--host", host, "--port", strconv.Itoa(port), "--format", "json"}); err != nil {
		t.Fatal(err)
	}

	var report attachReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if len(report.Targets) != 1 || report.Targets[0].ID != "page-1" {
		t.Fatalf("targets = %+v", report.Targets)
	}
}

func TestPrintAttachReportDiagnostics(t *testing.T) {
	t.Parallel()

	report := attachReport{
		Host:         "localhost",
		Errors:       []string{"localhost:9222: devtools endpoint not reachable"},
		Instructions: []string{"cdp attach --port 9222"},
	}
	var out bytes.Buffer
	printAttachReport(&out, report)
	text := out.String()
	for _, want := range []string{
		"No attachable CDP targets found",
		"Probe diagnostics:",
		"devtools endpoint not reachable",
		"cdp attach --port 9222",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestLaunchInstructionsFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		goos       string
		candidates []discovery.BrowserCandidate
		want       []string
	}{
		{
			name: "darwin discovered browser",
			goos: "darwin",
			candidates: []discovery.BrowserCandidate{
				{Name: "Brave Browser", Path: "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser"},
			},
			want: []string{
				`'/Applications/Brave Browser.app/Contents/MacOS/Brave Browser' --remote-debugging-port=9333 --user-data-dir="$(mktemp -d)"`,
				"cdp attach --port 9333",
			},
		},
		{
			name: "windows discovered browser",
			goos: "windows",
			candidates: []discovery.BrowserCandidate{
				{Name: "Google Chrome", Path: `C:\Program Files\Google\Chrome\Application\chrome.exe`},
			},
			want: []string{
				`"C:\Program Files\Google\Chrome\Application\chrome.exe" --remote-debugging-port=9333 --user-data-dir="%TEMP%\cdp-debug-profile-%RANDOM%"`,
				"cdp attach --port 9333",
			},
		},
		{
			name:       "linux fallback",
			goos:       "linux",
			candidates: nil,
			want: []string{
				`brave-browser --remote-debugging-port=9333 --user-data-dir="$(mktemp -d)"`,
				`google-chrome --remote-debugging-port=9333 --user-data-dir="$(mktemp -d)"`,
				"cdp attach --port 9333",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := launchInstructionsFor(tt.goos, tt.candidates, 9333)
			if len(got) != len(tt.want) {
				t.Fatalf("len(instructions) = %d, want %d: %#v", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("instruction[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestAttachFailureErrorIncludesInstructions(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	host, port := attachServerHostPort(t, srv.URL)
	err := attachFailureError(host, port, fmt.Errorf("dial failed"))
	text := err.Error()
	if !strings.Contains(text, `"instructions"`) {
		t.Fatalf("error missing instructions: %s", text)
	}
	if !strings.Contains(text, fmt.Sprintf(`"port":%d`, port)) {
		t.Fatalf("error missing port: %s", text)
	}
}

func attachTestHandler(tabs string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json/version":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"Browser":"Chrome/Test","Protocol-Version":"1.3"}`)
		case "/json", "/json/list":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, tabs)
		default:
			http.NotFound(w, r)
		}
	})
}

func attachServerHostPort(t *testing.T, rawurl string) (string, int) {
	t.Helper()
	u, err := url.Parse(rawurl)
	if err != nil {
		t.Fatal(err)
	}
	host, portString, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		t.Fatal(err)
	}
	return host, port
}
