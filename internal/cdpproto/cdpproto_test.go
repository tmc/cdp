package cdpproto

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestValid(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"Page.startScreencast", true},
		{"Page.printToPDF", true},
		{"Network.requestWillBeSent", true}, // an event, not a command
		{"Page.startScreenRecording", false},
		{"Runtime.getExecutionContexts", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := Valid(tt.name); got != tt.want {
			t.Errorf("Valid(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestIsMethodName(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"Page.startScreencast", true},
		{"IO.read", true},
		{"page.startScreencast", false}, // domain must be capitalized
		{"Page.StartScreencast", false}, // member must not be
		{"Page.start.screencast", false},
		{"example.com", false},
		{"Page.", false},
		{".start", false},
		{"Page", false},
		{"Content-Type", false},
	}
	for _, tt := range tests {
		if got := isMethodName(tt.in); got != tt.want {
			t.Errorf("isMethodName(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

// TestSnapshotIsCurrent checks that methods.txt matches the cdproto the module
// depends on. It fails after a dependency bump that was not followed by go
// generate, which is the moment to review what the new protocol surface means
// for the tools, docs, and skills.
func TestSnapshotIsCurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the generator")
	}
	out := filepath.Join(t.TempDir(), "methods.txt")
	cmd := exec.Command("go", "run", "./gen", "-o", out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go run ./gen: %v\n%s", err, b)
	}
	want, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("methods.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("methods.txt is stale; run: go generate ./internal/cdpproto")
	}
}

// TestNoUnknownProtocolMethods checks every protocol method named by a string
// literal in this module against the protocol itself.
//
// A call routed through the generated cdproto bindings cannot name a method
// that does not exist, because the binding would not compile. A call that
// passes the name as a string can, and it builds and ships and then fails at
// run time with -32601 against every browser. This is the check that turns
// that into a test failure.
func TestNoUnknownProtocolMethods(t *testing.T) {
	root := moduleRoot(t)
	var bad []Ref
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		switch d.Name() {
		case ".git", "testdata", "node_modules", "extension_bundle":
			return fs.SkipDir
		}
		refs, err := Refs(path)
		if err != nil {
			return nil // not a Go package, or does not parse on its own
		}
		for _, r := range refs {
			if !Valid(r.Name) && !knownDead[r.Name] {
				rel, _ := filepath.Rel(root, path)
				r.File = filepath.Join(rel, r.File)
				bad = append(bad, r)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range bad {
		t.Errorf("%s:%d: %q is not a DevTools protocol method or event\n"+
			"\tIf the protocol added it, bump cdproto and run: go generate ./internal/cdpproto\n"+
			"\tIf it never existed, the call fails at run time with -32601; delete it.",
			r.File, r.Line, r.Name)
	}
}

// knownDead quarantines calls that name a method the protocol does not have.
// Every one of them fails at run time with -32601 against every browser; they
// are listed here only so that this test can enforce the rule going forward
// instead of failing on debt it did not create. The fix is to delete the call,
// not to add a line here.
//
// Removing an entry should make the test pass, because the call is gone.
var knownDead = map[string]bool{
	// Chrome has no video-encoding command. Recording is Page.startScreencast
	// plus local encoding of the frames.
	"Page.startScreenRecording": true,
	"Page.stopScreenRecording":  true,

	// Removed from the protocol; type profiling was dropped from V8.
	"Profiler.startTypeProfile": true,
	"Profiler.stopTypeProfile":  true,
	"Profiler.takeTypeProfile":  true,

	// Never existed. Runtime reports contexts by event, not by query.
	"Runtime.getExecutionContexts": true,

	// Removed; Debugger.scriptParsed now carries resolvedBreakpoints.
	"Debugger.breakpointResolved": true,
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}").Output()
	if err != nil {
		t.Fatalf("locate module root: %v", err)
	}
	return strings.TrimSpace(string(out))
}
