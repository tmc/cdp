package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScriptFormatDocPointsToCanonicalReference(t *testing.T) {
	doc := readDocFile(t, "CDP_SCRIPT_FORMAT.md")

	want := "skills/writing-cdp-scripts/references/script-format.md"
	if !strings.Contains(doc, want) {
		t.Fatalf("CDP_SCRIPT_FORMAT.md should point to %q", want)
	}
}

func TestCanonicalScriptFormatDocDoesNotAdvertiseUnsupportedFeatures(t *testing.T) {
	doc := readDocFile(t, filepath.Join("..", "..", "skills", "writing-cdp-scripts", "references", "script-format.md"))

	disallowed := []string{
		"#!/usr/bin/env cdp script",
		"meta.yaml",
		"metadata.yaml",
		"imports:",
		"wait until",
		"assert status",
		"assert no errors",
		"assert url contains",
		"capture network to",
		"mock api",
		"throttle <profile>",
		"save <var> to <file>",
		"select <selector>",
		"scroll to <selector>",
		"include <file>",
		"devtools",
		"breakpoint",
		"--matrix",
		"--parallel",
		"--distributed",
	}

	for _, s := range disallowed {
		if strings.Contains(doc, s) {
			t.Errorf("canonical script format doc still mentions unsupported feature %q", s)
		}
	}
}

func TestSkillScriptFormatReferenceMatchesCurrentWaitSyntax(t *testing.T) {
	doc := readDocFile(t, filepath.Join("..", "..", "skills", "writing-cdp-scripts", "references", "script-format.md"))

	disallowed := []string{
		"wait for h1",
	}

	for _, s := range disallowed {
		if strings.Contains(doc, s) {
			t.Errorf("skill reference still mentions unsupported syntax %q", s)
		}
	}
}

func readDocFile(t *testing.T, name string) string {
	t.Helper()

	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}
