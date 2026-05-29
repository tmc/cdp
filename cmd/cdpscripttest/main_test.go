package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunArgsHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runArgs([]string{"--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runArgs --help = %d, want 0", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("unexpected stdout:\n%s", stdout.String())
	}
	if want := "Usage of cdpscripttest:"; !strings.Contains(stderr.String(), want) {
		t.Fatalf("stderr missing %q:\n%s", want, stderr.String())
	}
}

func TestRunArgsMissingScripts(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runArgs(nil, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("runArgs without scripts = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("unexpected stdout:\n%s", stdout.String())
	}
	for _, want := range []string{
		"cdpscripttest: no scripts specified",
		"Usage of cdpscripttest:",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q:\n%s", want, stderr.String())
		}
	}
}

func TestRunArgsBadFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runArgs([]string{"--bad-flag"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("runArgs bad flag = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("unexpected stdout:\n%s", stdout.String())
	}
	if want := "flag provided but not defined"; !strings.Contains(stderr.String(), want) {
		t.Fatalf("stderr missing %q:\n%s", want, stderr.String())
	}
}

func TestRunArgsBadGlob(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runArgs([]string{"["}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("runArgs bad glob = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("unexpected stdout:\n%s", stdout.String())
	}
	if want := "cdpscripttest: glob"; !strings.Contains(stderr.String(), want) {
		t.Fatalf("stderr missing %q:\n%s", want, stderr.String())
	}
}
