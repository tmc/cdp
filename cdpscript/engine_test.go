package cdpscript

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rsc.io/script"
)

func TestAppendArgEnv(t *testing.T) {
	got := appendArgEnv([]string{"BASE_URL=https://example.com"}, []string{"one", "two"})
	want := []string{
		"BASE_URL=https://example.com",
		"ARG1=one",
		"ARG2=two",
		"ARGC=2",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("appendArgEnv mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestExecuteReaderRunsPureScriptWithoutBrowser(t *testing.T) {
	var stdout bytes.Buffer
	engine := New(WithStdout(&stdout))
	script := `-- main.cdp --
log hello ${ARG1}
`
	if err := engine.ExecuteReader(context.Background(), "stdin", strings.NewReader(script), []string{"reader"}); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "hello reader\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestValidateReaderRejectsUnknownCommand(t *testing.T) {
	script := `-- main.cdp --
not-a-command
`
	err := ValidateReader("bad.txtar", strings.NewReader(script))
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("ValidateReader error = %v, want unknown command", err)
	}
}

func TestValidateReaderAcceptsSourceAsAlias(t *testing.T) {
	script := `-- main.cdp --
source -as helper ./helper.cdp
helper
`
	if err := ValidateReader("source.txtar", strings.NewReader(script)); err != nil {
		t.Fatal(err)
	}
}

func TestHelpText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.cdpscript")
	data := `#!/usr/bin/env cdpscript
# hello-world
#
# Say hello.
#
# usage: hello.cdpscript NAME

-- main.cdp --
log hello
`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	text, err := HelpText(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"hello.cdpscript",
		"hello-world",
		"Say hello.",
		"Usage: hello.cdpscript NAME",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("help text missing %q:\n%s", want, text)
		}
	}
}

func TestHelpTextDefaultUsage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.cdpscript")
	data := `#!/usr/bin/env cdpscript
# hello-world

-- main.cdp --
log hello
`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	text, err := HelpText(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Usage: hello.cdpscript [args...]"; !strings.Contains(text, want) {
		t.Fatalf("help text missing %q:\n%s", want, text)
	}
}

func TestCleanArchiveComment(t *testing.T) {
	comment := "#!/usr/bin/env cdpscript\n# Title\n#\n# Details\nplain line\n"
	got := cleanArchiveComment(comment)
	want := "Title\n\nDetails\nplain line"
	if got != want {
		t.Fatalf("cleanArchiveComment = %q, want %q", got, want)
	}
}

func TestCanonicalScriptFormatDocumentsAllCommands(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "skills", "writing-cdp-scripts", "references", "script-format.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)

	for name := range New().commands() {
		if !docHasCommandLine(doc, name) {
			t.Errorf("script-format.md does not document command %q", name)
		}
	}
}

func docHasCommandLine(doc, command string) bool {
	for _, line := range strings.Split(doc, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == command {
			return true
		}
	}
	return false
}

func TestResolveUploadFile(t *testing.T) {
	workdir := t.TempDir()
	cwd := t.TempDir()

	writeFile := func(path string) {
		t.Helper()
		if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(filepath.Join(workdir, "embedded.txt"))
	writeFile(filepath.Join(cwd, "argument.txt"))

	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldwd); err != nil {
			t.Fatal(err)
		}
	})

	s, err := script.NewState(context.Background(), workdir, nil)
	if err != nil {
		t.Fatal(err)
	}

	got, err := resolveUploadFile(s, "embedded.txt")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(workdir, "embedded.txt"); !sameFile(t, got, want) {
		t.Fatalf("resolveUploadFile embedded = %q, want %q", got, want)
	}

	got, err = resolveUploadFile(s, "argument.txt")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(cwd, "argument.txt"); !sameFile(t, got, want) {
		t.Fatalf("resolveUploadFile argument = %q, want %q", got, want)
	}
}

func sameFile(t *testing.T, a, b string) bool {
	t.Helper()
	ai, err := os.Stat(a)
	if err != nil {
		t.Fatal(err)
	}
	bi, err := os.Stat(b)
	if err != nil {
		t.Fatal(err)
	}
	return os.SameFile(ai, bi)
}

func TestParseViewportArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantWidth  int
		wantHeight int
		wantErr    bool
	}{
		{name: "valid", args: []string{"390", "640"}, wantWidth: 390, wantHeight: 640},
		{name: "missing height", args: []string{"390"}, wantErr: true},
		{name: "extra arg", args: []string{"390", "640", "mobile"}, wantErr: true},
		{name: "zero width", args: []string{"0", "640"}, wantErr: true},
		{name: "negative height", args: []string{"390", "-1"}, wantErr: true},
		{name: "bad width", args: []string{"wide", "640"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotWidth, gotHeight, err := parseViewportArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("parseViewportArgs succeeded unexpectedly")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if gotWidth != tt.wantWidth || gotHeight != tt.wantHeight {
				t.Fatalf("parseViewportArgs = %d, %d, want %d, %d", gotWidth, gotHeight, tt.wantWidth, tt.wantHeight)
			}
		})
	}
}

func TestSelectOptionScriptEscapesInputs(t *testing.T) {
	got := selectOptionScript(`#plan"`, `Pro "Team"`)
	for _, want := range []string{
		`const selector = "#plan\"";`,
		`const choice = "Pro \"Team\"";`,
		`HTMLSelectElement`,
		`dispatchEvent(new Event("change"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("selectOptionScript missing %q:\n%s", want, got)
		}
	}
}

func TestParseScrollArgs(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantDir      string
		wantDistance int
		wantSelector string
		wantErr      bool
	}{
		{name: "default", wantDir: "down", wantDistance: 500},
		{name: "direction", args: []string{"up"}, wantDir: "up", wantDistance: 500},
		{name: "distance", args: []string{"down", "900"}, wantDir: "down", wantDistance: 900},
		{name: "selector", args: []string{"#target"}, wantSelector: "#target"},
		{name: "compound selector", args: []string{"main", ".target"}, wantSelector: "main .target"},
		{name: "bad distance", args: []string{"down", "far"}, wantErr: true},
		{name: "zero distance", args: []string{"down", "0"}, wantErr: true},
		{name: "extra distance", args: []string{"down", "100", "extra"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseScrollArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("parseScrollArgs succeeded unexpectedly")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.direction != tt.wantDir || got.distance != tt.wantDistance || got.selector != tt.wantSelector {
				t.Fatalf("parseScrollArgs = %#v, want direction=%q distance=%d selector=%q", got, tt.wantDir, tt.wantDistance, tt.wantSelector)
			}
		})
	}
}

func TestScrollSelectorScriptEscapesSelector(t *testing.T) {
	got := scrollSelectorScript(`#target"`)
	for _, want := range []string{
		`document.querySelector("#target\"")`,
		`scrollIntoView`,
		`block: "center"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("scrollSelectorScript missing %q:\n%s", want, got)
		}
	}
}

func TestParseDragArgs(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantSteps int
		wantErr   bool
	}{
		{name: "default steps", args: []string{"#source", "#target"}, wantSteps: 10},
		{name: "custom steps", args: []string{"#source", "#target", "4"}, wantSteps: 4},
		{name: "missing target", args: []string{"#source"}, wantErr: true},
		{name: "too many", args: []string{"#source", "#target", "4", "extra"}, wantErr: true},
		{name: "bad steps", args: []string{"#source", "#target", "many"}, wantErr: true},
		{name: "zero steps", args: []string{"#source", "#target", "0"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDragArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("parseDragArgs succeeded unexpectedly")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.source != tt.args[0] || got.target != tt.args[1] || got.steps != tt.wantSteps {
				t.Fatalf("parseDragArgs = %#v, want source=%q target=%q steps=%d", got, tt.args[0], tt.args[1], tt.wantSteps)
			}
		})
	}
}

func TestDragPointScriptEscapesSelector(t *testing.T) {
	got := dragPointScript(`#source"`)
	for _, want := range []string{
		`document.querySelector("#source\"")`,
		`scrollIntoView`,
		`getBoundingClientRect`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("dragPointScript missing %q:\n%s", want, got)
		}
	}
}

func TestParseDialogArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantAccept bool
		wantPrompt string
		wantErr    bool
	}{
		{name: "accept", args: []string{"accept"}, wantAccept: true},
		{name: "accept prompt", args: []string{"accept", "hello", "world"}, wantAccept: true, wantPrompt: "hello world"},
		{name: "dismiss", args: []string{"dismiss"}},
		{name: "missing", wantErr: true},
		{name: "unknown", args: []string{"close"}, wantErr: true},
		{name: "dismiss prompt", args: []string{"dismiss", "unused"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDialogArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("parseDialogArgs succeeded unexpectedly")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.accept != tt.wantAccept || got.promptText != tt.wantPrompt {
				t.Fatalf("parseDialogArgs = %#v, want accept=%v prompt=%q", got, tt.wantAccept, tt.wantPrompt)
			}
		})
	}
}

func TestDownloadPaths(t *testing.T) {
	outputDir := t.TempDir()
	e := &Engine{outputDir: outputDir}

	if got, want := e.artifactPath("downloads"), filepath.Join(outputDir, "downloads"); got != want {
		t.Fatalf("artifactPath = %q, want %q", got, want)
	}

	if _, err := e.downloadPath("file.txt"); err == nil {
		t.Fatal("downloadPath succeeded before download dir was set")
	}

	e.downloadDir = filepath.Join(outputDir, "downloads")
	got, err := e.downloadPath("file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(outputDir, "downloads", "file.txt"); got != want {
		t.Fatalf("downloadPath = %q, want %q", got, want)
	}
}

func TestScreenshotQualityMatchesExtension(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{name: "page.png", want: 100},
		{name: "page.PNG", want: 100},
		{name: "page.jpg", want: 90},
		{name: "page.jpeg", want: 90},
		{name: "page", want: 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := screenshotQuality(tt.name); got != tt.want {
				t.Fatalf("screenshotQuality(%q) = %d, want %d", tt.name, got, tt.want)
			}
		})
	}
}

func TestWaitForFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "download.txt")
	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(20 * time.Millisecond)
		if err := os.WriteFile(path, []byte("ok"), 0o644); err != nil {
			t.Error(err)
		}
	}()

	if err := waitForFile(context.Background(), path, time.Second); err != nil {
		t.Fatal(err)
	}
	<-done
}

// TestWaitForFileCancel checks that a cancelled run aborts the wait instead of
// sitting out the remaining timeout.
func TestWaitForFileCancel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "never-arrives.txt")
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := waitForFile(ctx, path, time.Minute)
	if err == nil {
		t.Fatal("waitForFile returned nil for a file that never appeared")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForFile error = %v, want one wrapping context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("waitForFile waited %v after cancellation; want a prompt return", elapsed)
	}
}
