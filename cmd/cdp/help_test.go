package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// captureStdout runs fn with os.Stdout redirected to a pipe and returns what
// fn wrote. The REPL help methods print directly to stdout, so this is the
// only way to assert on their rendered output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// TestHelpDocumentsRawCDP checks that the interactive help mentions raw
// Domain.method dispatch, which is otherwise undiscoverable from the shell.
func TestHelpDocumentsRawCDP(t *testing.T) {
	help := NewHelpSystem(NewCommandRegistry())

	tests := []struct {
		name     string
		render   func()
		contains []string
	}{
		{
			name:   "general_help",
			render: help.showGeneralHelp,
			contains: []string{
				"Raw CDP Protocol",
				"Domain.method",
				"Page.addScriptToEvaluateOnNewDocument",
			},
		},
		{
			name:   "quick_reference",
			render: help.ShowQuickReference,
			contains: []string{
				"Raw CDP",
				"Domain.method",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := captureStdout(t, tt.render)
			for _, want := range tt.contains {
				if !strings.Contains(out, want) {
					t.Errorf("%s output missing %q\noutput:\n%s", tt.name, want, out)
				}
			}
		})
	}
}
