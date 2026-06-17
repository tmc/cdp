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
		"assert no errors",
		"assert url contains",
		"capture network to",
		"mock api",
		"throttle <profile>",
		"save <var> to <file>",
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

func TestScriptFormatDocumentsUnixToolContract(t *testing.T) {
	doc := readDocFile(t, filepath.Join("..", "..", "skills", "writing-cdp-scripts", "references", "script-format.md"))

	want := []string{
		"## Unix Tool Contract",
		"${ARG1}",
		"${ARGC}",
		"cdpscript script.txtar --help",
		"--tab <target-id> --port <port>",
		"| 2 | Usage error |",
		"| 3 | Assertion failed |",
		"| 130 | Interrupted |",
	}

	for _, s := range want {
		if !strings.Contains(doc, s) {
			t.Errorf("script format reference missing Unix contract text %q", s)
		}
	}
}

func TestOperatingCDPBasicsDocumentsAttachPath(t *testing.T) {
	doc := readDocFile(t, filepath.Join("..", "..", "skills", "operating-cdp-cli", "references", "basics.md"))

	want := []string{
		"cdp attach",
		"--remote-host",
		"--remote-port",
		"--tab <target-id>",
		"cdp run --tab <target-id> --port 9222 script.txtar",
		"cdpscript --tab <target-id> --port 9222 script.txtar",
	}

	for _, s := range want {
		if !strings.Contains(doc, s) {
			t.Errorf("operating CLI basics missing %q", s)
		}
	}
	if strings.Contains(doc, "cdp --port 9222") {
		t.Errorf("operating CLI basics still documents unsupported top-level --port attach syntax")
	}
}

func TestOperatingCDPBasicsDocumentsProfileBoundary(t *testing.T) {
	doc := readDocFile(t, filepath.Join("..", "..", "skills", "operating-cdp-cli", "references", "basics.md"))

	want := []string{
		"--list-profiles",
		`--use-profile "Default"`,
		"--cookie-domains",
		"copies the named browser profile into a temporary working directory",
		"does not attach to an already-running browser",
		"use `cdp attach`",
	}

	for _, s := range want {
		if !strings.Contains(doc, s) {
			t.Errorf("operating CLI basics missing profile boundary text %q", s)
		}
	}
}

func TestShadowDOMSkillDocumentsCoordinateBoundary(t *testing.T) {
	doc := readDocFile(t, filepath.Join("..", "..", "skills", "working-with-shadow-dom", "references", "shadow-dom.md"))

	want := []string{
		"Open roots can be inspected with JavaScript",
		"Closed roots are intentionally not exposed",
		"click coord:x,y",
		"coordinate-compositor-surfaces.txtar",
	}

	for _, s := range want {
		if !strings.Contains(doc, s) {
			t.Errorf("shadow DOM reference missing %q", s)
		}
	}
}

func TestReadmeDocumentsAutomationStack(t *testing.T) {
	doc := readDocFile(t, filepath.Join("..", "..", "README.md"))

	want := []string{
		"`cmd/cdp`: live CDP operation",
		"`cdpscript`: executable txtar scripts",
		"`cdpscripttest`: `rsc.io/script`-style browser fixtures",
		"cdp --remote-host localhost --remote-port 9222 --tab <target-id> --shell",
		"cdpscript --tab <target-id> --port 9222 script.txtar",
		"cdpscripttest.RunCDPScript",
		"docs/planning/cdp-best-in-class-checklist.md",
	}

	for _, s := range want {
		if !strings.Contains(doc, s) {
			t.Errorf("README missing %q", s)
		}
	}
	if strings.Contains(doc, "cdp --remote-port 9222 --tab") {
		t.Errorf("README still documents selected-tab attach without --remote-host")
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
