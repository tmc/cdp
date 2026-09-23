package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateCDPScriptInputRejectsUnknownCommand(t *testing.T) {
	err := validateCDPScriptInput(cdpscriptInput{Script: "missing-command\n"}, "inline.cdp", "missing-command\n")
	if err == nil {
		t.Fatal("validateCDPScriptInput succeeded, want error")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("error = %v, want unknown command", err)
	}
}

func TestValidateCDPScriptInputAcceptsTxtar(t *testing.T) {
	text := `-- main.cdp --
log ok
`
	if err := validateCDPScriptInput(cdpscriptInput{Script: text}, "inline.txtar", text); err != nil {
		t.Fatal(err)
	}
}

func TestCDPScriptInputTextRequiresOneSource(t *testing.T) {
	_, _, err := cdpscriptInputText(cdpscriptInput{})
	if err == nil {
		t.Fatal("empty input succeeded, want error")
	}

	_, _, err = cdpscriptInputText(cdpscriptInput{Path: "x", Script: "log x\n"})
	if err == nil {
		t.Fatal("path and script input succeeded, want error")
	}
}

func TestCDPScriptInputTextReadsPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tool.cdp")
	if err := os.WriteFile(path, []byte("log file\n"), 0o666); err != nil {
		t.Fatal(err)
	}

	text, name, err := cdpscriptInputText(cdpscriptInput{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if text != "log file\n" {
		t.Fatalf("text = %q", text)
	}
	if name != "tool.cdp" {
		t.Fatalf("name = %q, want tool.cdp", name)
	}
}

func TestScriptFormat(t *testing.T) {
	tests := []struct {
		name   string
		format string
		file   string
		text   string
		want   string
	}{
		{name: "explicit cdp", format: "cdp", file: "x.txtar", text: "-- main.cdp --\n", want: "cdp"},
		{name: "explicit txtar", format: "txtar", file: "x.cdp", text: "log x\n", want: "txtar"},
		{name: "txtar suffix", file: "x.txtar", text: "log x\n", want: "txtar"},
		{name: "txtar marker", file: "x.cdp", text: "\n-- main.cdp --\nlog x\n", want: "txtar"},
		{name: "txtar marker first line", file: "inline.cdp", text: "-- main.cdp --\nlog x\n", want: "txtar"},
		{name: "plain", file: "x.cdp", text: "log x\n", want: "cdp"},
		{name: "dashes without marker", file: "x.cdp", text: "log x\n-- not a marker\n", want: "cdp"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scriptFormat(tt.format, tt.file, tt.text); got != tt.want {
				t.Fatalf("scriptFormat() = %q, want %q", got, tt.want)
			}
		})
	}
}
