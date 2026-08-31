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

func TestReportOptions(t *testing.T) {
	tests := []struct {
		name                            string
		emit, html, combined            bool
		dir, artifacts, want            string
		wantHTML, wantCombined, wantNil bool
	}{
		{"disabled", false, false, false, "", "", "", false, false, true},
		{"explicit", true, true, true, "reports", "artifacts", "reports", true, true, false},
		{"artifacts", true, false, false, "", "artifacts", "artifacts", false, false, false},
		{"default", true, false, false, "", "", "testdata/screenshots", false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reportOptions(tt.emit, tt.html, tt.combined, tt.dir, tt.artifacts, []string{"testdata/example.txt"})
			if tt.wantNil {
				if got != nil {
					t.Fatalf("reportOptions() = %#v, want nil", got)
				}
				return
			}
			if got == nil || got.Dir != tt.want || got.HTML != tt.wantHTML || got.Combined != tt.wantCombined {
				t.Fatalf("reportOptions() = %#v", got)
			}
		})
	}
}
