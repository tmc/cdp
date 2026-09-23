package cdpscripttest

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tmc/cdp/cdpscripttest/report"
	"golang.org/x/tools/txtar"
)

// ScriptName returns the name used for a script file in subtest names,
// artifact directories, and reports: its base name without a .txt or .txtar
// extension.
func ScriptName(file string) string {
	name := filepath.Base(file)
	for _, ext := range []string{".txtar", ".txt"} {
		if n, ok := strings.CutSuffix(name, ext); ok {
			return n
		}
	}
	return name
}

// ReportManifest returns the report manifest for the txtar script files,
// for passing to report.NewWriter. Each entry has the file's ScriptName,
// its script source, and Detail set from its report directive
// (see ExtractReportLevel).
func ReportManifest(files []string) ([]report.Script, error) {
	scripts := make([]report.Script, 0, len(files))
	for _, file := range files {
		a, err := txtar.ParseFile(file)
		if err != nil {
			return nil, fmt.Errorf("parse txtar %q: %w", file, err)
		}
		scripts = append(scripts, report.Script{
			Name:   ScriptName(file),
			Source: a.Comment,
			Detail: ExtractReportLevel(a.Comment) == ReportDetail,
		})
	}
	return scripts, nil
}

// ReportLevel indicates how a script should appear in reports.
type ReportLevel int

const (
	// ReportOverview includes the script in the combined overview report.
	ReportOverview ReportLevel = iota
	// ReportDetail excludes the script from the combined report; it still
	// gets its own per-script report.md.
	ReportDetail
)

// ExtractReportLevel scans script source for a report directive comment:
//
//	# report:detail   — exclude from combined report
//	# report:overview — include in combined report (default)
//
// Directives can appear anywhere in the leading comment block.
func ExtractReportLevel(source []byte) ReportLevel {
	for _, line := range strings.Split(string(source), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "#") {
			break
		}
		comment := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
		switch comment {
		case "report:detail":
			return ReportDetail
		case "report:overview":
			return ReportOverview
		}
	}
	return ReportOverview
}
