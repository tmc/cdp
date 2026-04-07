package cdpscripttest

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestParseLog(t *testing.T) {
	log := `
# Navigate and wait (0.567s)
> navigate /blur-dashboard.html
> waitVisible '#dashboard'
> sleep 500ms

# Baseline capture (0.038s)
> screenshot-compare --threshold 15 --blur '#users' '#dashboard' dashboard.png
[stdout]
/tmp/artifacts/dashboard.png
baseline created

> stdout 'baseline created'
[stdout]
baseline created

matched: baseline created
`
	sections := ParseLog(log)

	if len(sections) != 2 {
		t.Fatalf("got %d sections, want 2", len(sections))
	}

	// Section 0: Navigate and wait
	s0 := sections[0]
	if s0.Comment != "Navigate and wait" {
		t.Errorf("section 0 comment = %q, want %q", s0.Comment, "Navigate and wait")
	}
	if s0.Timing != "0.567s" {
		t.Errorf("section 0 timing = %q, want %q", s0.Timing, "0.567s")
	}
	if len(s0.Commands) != 3 {
		t.Errorf("section 0 commands = %d, want 3", len(s0.Commands))
	}

	// Section 1: Baseline capture
	s1 := sections[1]
	if s1.Comment != "Baseline capture" {
		t.Errorf("section 1 comment = %q, want %q", s1.Comment, "Baseline capture")
	}
	if s1.Timing != "0.038s" {
		t.Errorf("section 1 timing = %q, want %q", s1.Timing, "0.038s")
	}
	if len(s1.Commands) < 1 {
		t.Fatalf("section 1 commands = %d, want >= 1", len(s1.Commands))
	}
	cmd := s1.Commands[0]
	if cmd.CompareResult != "baseline created" {
		t.Errorf("compare result = %q, want %q", cmd.CompareResult, "baseline created")
	}
}

func TestParseLogMergesConsecutiveComments(t *testing.T) {
	log := `
# Line one of preamble.
# Line two of preamble.
# Line three with timing. (0.5s)
> navigate /page
> sleep 500ms

# Second section (1.0s)
> click '#btn'
`
	sections := ParseLog(log)
	if len(sections) != 2 {
		t.Fatalf("got %d sections, want 2", len(sections))
	}

	s0 := sections[0]
	want := "Line one of preamble.\nLine two of preamble.\nLine three with timing."
	if s0.Comment != want {
		t.Errorf("section 0 comment = %q, want %q", s0.Comment, want)
	}
	if s0.Timing != "0.5s" {
		t.Errorf("section 0 timing = %q, want %q", s0.Timing, "0.5s")
	}
	if len(s0.Commands) != 2 {
		t.Errorf("section 0 commands = %d, want 2", len(s0.Commands))
	}
}

func TestExtractPreamble(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "multi-line preamble",
			source: "# Dashboard screenshot comparison.\n# Uses blur on dynamic metrics.\n\nnavigate /page\n",
			want:   "Dashboard screenshot comparison.\nUses blur on dynamic metrics.",
		},
		{
			name:   "no preamble",
			source: "navigate /page\nwaitVisible '#app'\n",
			want:   "",
		},
		{
			name:   "preamble with blank lines before",
			source: "\n\n# Hello\nnavigate /page\n",
			want:   "Hello",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractPreamble([]byte(tt.source))
			if got != tt.want {
				t.Errorf("ExtractPreamble() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderReport(t *testing.T) {
	sections := []reportSection{
		{
			Comment: "Navigate and wait",
			Timing:  "0.567s",
			Commands: []reportCommand{
				{Line: "navigate /blur-dashboard.html"},
				{Line: "waitVisible '#dashboard'"},
				{Line: "sleep 500ms"},
			},
		},
		{
			Comment: "Baseline capture",
			Timing:  "0.038s",
			Commands: []reportCommand{
				{
					Line:          "screenshot-compare --threshold 15 '#dashboard' dashboard.png",
					CompareResult: "baseline created",
					Screenshots: []screenshotRef{
						{Path: "/tmp/art/dashboard.png"},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	RenderReport(&buf, "blur-dashboard", "Dashboard test with blur.", sections, "/tmp/art")
	out := buf.String()

	for _, want := range []string{
		"# blur-dashboard",
		"> Dashboard test with blur.",
		"## Navigate and wait (0.567s)",
		"navigate /blur-dashboard.html",
		"## Baseline capture (0.038s)",
		"**Result:** baseline created",
		"![dashboard.png](dashboard.png)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}

	// Assertion commands should be excluded.
	if strings.Contains(out, "stdout") {
		t.Errorf("output should not contain assertion commands")
	}
}

func TestReportStreamer(t *testing.T) {
	log := "# Navigate and wait (0.567s)\n" +
		"> navigate /page\n" +
		"> waitVisible '#app'\n" +
		"\n" +
		"# Capture screenshot (0.038s)\n" +
		"> screenshot dashboard.png\n" +
		"[stdout]\n" +
		"/tmp/art/dashboard.png\n" +
		"\n" +
		"> stdout 'dashboard'\n" +
		"[stdout]\n" +
		"/tmp/art/dashboard.png\n" +
		"\n" +
		"matched: /tmp/art/dashboard.png\n"

	source := []byte("# Test preamble.\n\nnavigate /page\n")

	var inner bytes.Buffer
	var report bytes.Buffer
	streamer := NewReportStreamer(&inner, &report, "test-script", "/tmp/art", source)

	// Simulate writing the log in chunks (as the engine would).
	for i := 0; i < len(log); i += 17 {
		end := i + 17
		if end > len(log) {
			end = len(log)
		}
		_, err := streamer.Write([]byte(log[i:end]))
		if err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	streamer.Flush()

	out := report.String()

	// The report should have been streamed progressively.
	for _, want := range []string{
		"# test-script",
		"> Test preamble.",
		"## Navigate and wait (0.567s)",
		"navigate /page",
		"## Capture screenshot (0.038s)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("streaming output missing %q\nfull output:\n%s", want, out)
		}
	}

	// Inner writer should have received the full log.
	if !strings.Contains(inner.String(), "navigate /page") {
		t.Error("inner writer missing log content")
	}

	// Assertion commands should be excluded from report.
	lines := strings.Split(out, "\n")
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "stdout") {
			t.Errorf("streaming output should not contain assertion commands, got: %q", trimmed)
		}
	}
}

func TestReportStreamerSectionBoundary(t *testing.T) {
	// Verify that section 1 is emitted when section 2 starts (streaming).
	log := "# Section one (1.0s)\n> cmd1\n\n# Section two (2.0s)\n> cmd2\n"

	var report bytes.Buffer
	streamer := NewReportStreamer(&bytes.Buffer{}, &report, "boundary", "/tmp", nil)
	streamer.Write([]byte(log))
	// Before Flush, section one should already be emitted (triggered by section two boundary).
	out := report.String()
	if !strings.Contains(out, "## Section one (1.0s)") {
		t.Errorf("section one should be emitted before Flush, got:\n%s", out)
	}
	if !strings.Contains(out, "cmd1") {
		t.Errorf("section one commands should be emitted before Flush")
	}

	// Section two is still being accumulated — only emitted on Flush.
	streamer.Flush()
	out = report.String()
	if !strings.Contains(out, "## Section two (2.0s)") {
		t.Errorf("section two should be emitted after Flush, got:\n%s", out)
	}
}

func TestIsAssertionCmd(t *testing.T) {
	tests := []struct {
		cmd  string
		want bool
	}{
		{"stdout 'hello'", true},
		{"stderr 'error'", true},
		{"cmp golden.txt", true},
		{"! stdout 'bad'", true},
		{"navigate /page", false},
		{"screenshot dashboard.png", false},
	}
	for _, tt := range tests {
		if got := isAssertionCmd(tt.cmd); got != tt.want {
			t.Errorf("isAssertionCmd(%q) = %v, want %v", tt.cmd, got, tt.want)
		}
	}
}

func TestExtractReportLevel(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   ReportLevel
	}{
		{"default is overview", "navigate /page\n", ReportOverview},
		{"explicit overview", "# report:overview\n# My test.\nnavigate /page\n", ReportOverview},
		{"detail", "# report:detail\n# Low-level test.\nnavigate /page\n", ReportDetail},
		{"detail in preamble", "# My test.\n# report:detail\nnavigate /page\n", ReportDetail},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractReportLevel([]byte(tt.source))
			if got != tt.want {
				t.Errorf("ExtractReportLevel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateCombinedReport(t *testing.T) {
	scripts := []ScriptReport{
		{
			Name:        "script-b",
			Source:      []byte("# Test B preamble.\nnavigate /b\n"),
			Log:         "# Navigate (0.2s)\n> navigate /b\n",
			ArtifactDir: "/tmp/art/script-b",
			Failed:      true,
		},
		{
			Name:        "script-a",
			Source:      []byte("# Test A preamble.\nnavigate /a\n"),
			Log:         "# Navigate (0.1s)\n> navigate /a\n",
			ArtifactDir: "/tmp/art/script-a",
		},
	}

	dir := t.TempDir()
	reportPath := dir + "/report.md"
	if err := GenerateCombinedReport(reportPath, scripts); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)

	for _, want := range []string{
		"# CDP Script Test Report",
		"2 scripts: 1 passed, 1 failed",
		"## Contents",
		"PASS [script-a](#script-a) — Test A preamble.",
		"FAIL [script-b](#script-b) — Test B preamble.",
		"<details",
		"<summary>PASS <strong>script-a</strong>",
		"<summary>FAIL <strong>script-b</strong>",
		"</details>",
		"navigate /a",
		"navigate /b",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("combined report missing %q\nfull:\n%s", want, out)
		}
	}

	// Failed tests should have open attribute.
	if !strings.Contains(out, `<details id="script-b" open>`) {
		t.Errorf("failed test should have open attribute\nfull:\n%s", out)
	}
	// Passing tests should not.
	if strings.Contains(out, `<details id="script-a" open>`) {
		t.Errorf("passing test should not have open attribute")
	}

	// Scripts should be sorted alphabetically (script-a before script-b).
	aIdx := strings.Index(out, "script-a</strong>")
	bIdx := strings.Index(out, "script-b</strong>")
	if aIdx > bIdx {
		t.Errorf("expected script-a before script-b in sorted output")
	}
}
