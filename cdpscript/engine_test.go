package cdpscript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestHelpText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.cdpscript")
	data := `#!/usr/bin/env cdpscript
-- meta.yaml --
name: hello-world
description: Say hello.

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
		"hello-world",
		"Say hello.",
		"Usage: hello-world [args...]",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("help text missing %q:\n%s", want, text)
		}
	}
}
