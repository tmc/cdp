package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// binPath is the cdpscripttest binary built by TestMain, or "" when the build
// was skipped or failed.
var binPath string

func TestMain(m *testing.M) {
	if dir, err := os.MkdirTemp("", "cdpscripttest-exec"); err == nil {
		bin := filepath.Join(dir, "cdpscripttest")
		if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err == nil {
			binPath = bin
		} else {
			// Leave binPath empty; TestExec reports the failure with context.
			os.Stderr.WriteString("build cdpscripttest: " + err.Error() + "\n" + string(out))
		}
		defer os.RemoveAll(dir)
	}
	os.Exit(m.Run())
}

// TestExec runs the built binary. The cases are the ones that finish without a
// browser, so the test stays offline: argument handling, script expansion, and
// the exit codes those paths produce.
func TestExec(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary")
	}
	if binPath == "" {
		t.Fatal("cdpscripttest binary was not built; see stderr above")
	}

	// A script the browser never gets to run: expansion must still find it.
	dir := t.TempDir()
	scripts := filepath.Join(dir, "scripts")
	if err := os.MkdirAll(scripts, 0o777); err != nil {
		t.Fatal(err)
	}
	script := "echo hello\nstdout hello\n"
	if err := os.WriteFile(filepath.Join(scripts, "a.txt"), []byte(script), 0o666); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		args []string
		want int
		out  string // substring of combined output
	}{
		{"help", []string{"--help"}, 0, "Usage of cdpscripttest:"},
		{"no scripts", nil, 2, "no scripts specified"},
		{"missing tree", []string{"nope/..."}, 2, `walk "nope"`},
		{
			// Expansion reaches the fixture, then the browser launch fails.
			// Exit 1 distinguishes a failing script from a usage error.
			"browser missing",
			[]string{"-browser", filepath.Join(dir, "no-such-browser"), scripts + "/..."},
			1,
			"a.txt",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(binPath, tt.args...)
			cmd.Dir = dir
			out, err := cmd.CombinedOutput()
			code := 0
			if err != nil {
				ee, ok := err.(*exec.ExitError)
				if !ok {
					t.Fatalf("run: %v\n%s", err, out)
				}
				code = ee.ExitCode()
			}
			if code != tt.want {
				t.Errorf("exit = %d, want %d\n%s", code, tt.want, out)
			}
			if !strings.Contains(string(out), tt.out) {
				t.Errorf("output missing %q:\n%s", tt.out, out)
			}
		})
	}
}
