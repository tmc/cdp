//go:build cdp

package cdpscripttest_test

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tmc/cdp/cdpscripttest"
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
}
