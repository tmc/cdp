package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tmc/cdp/cdpscript"
)

func TestScriptCmdHelpAfterScriptPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.cdpscript")
	data := `#!/usr/bin/env cdpscript
# demo
#
# Demonstrate script help.
#
# usage: demo.cdpscript TARGET

-- main.cdp --
log demo
`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := newScriptCmd()
	var stdout, stderr bytes.Buffer
	cmd.stdout = &stdout
	cmd.stderr = &stderr
	cmd.fs.SetOutput(cmd.stderr)

	err := cmd.run([]string{path, "--help"})
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("run error = %v, want flag.ErrHelp", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr:\n%s", stderr.String())
	}
	for _, want := range []string{
		"demo.cdpscript",
		"demo",
		"Demonstrate script help.",
		"Usage: demo.cdpscript TARGET",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestScriptExitCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "help", err: flag.ErrHelp, want: 0},
		{name: "usage", err: cdpscript.ErrUsage, want: 2},
		{name: "assertion", err: cdpscript.ErrAssertionFailed, want: 3},
		{name: "interrupt", err: context.Canceled, want: 130},
		{name: "general", err: errors.New("boom"), want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scriptExitCode(tt.err); got != tt.want {
				t.Fatalf("scriptExitCode(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}
