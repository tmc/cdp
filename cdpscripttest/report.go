package cdpscripttest

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// reportSection groups commands under a comment heading.
type reportSection struct {
	Comment  string          // comment text (# stripped, timing stripped)
	Timing   string          // e.g. "0.567s"
	Commands []reportCommand // commands in this section
}

// reportCommand is a single command line from the log.
type reportCommand struct {
	Line          string          // the command line (> prefix stripped)
	Stdout        []string        // captured stdout lines
	Matched       string          // "matched: ..." line if present
	CondNotMet    bool            // true if [condition not met] followed
	Screenshots   []screenshotRef // screenshot paths found in stdout
	CompareResult string          // "baseline created", "diff: 8.14%", etc.
}

// screenshotRef is a screenshot file with optional companions.
type screenshotRef struct {
	Path        string // absolute path to screenshot (baseline for compare)
	CurrentPath string // path to .current.png (latest capture from compare)
	Unblurred   string // path to -unblurred variant (probed on disk)
	FailPath    string // path to .fail.png (probed on disk)
	DiffPath    string // path to .diff.png (probed on disk)
}

// ParseLog parses the rsc.io/script engine log into structured sections.
// The log format:
//
//	# comment (timing)   — section header
//	> command args       — command echo
//	[stdout]\nlines      — captured stdout
//	[stderr]\nlines      — captured stderr
//	matched: ...         — internal
//	[condition ...]      — internal
func ParseLog(log string) []reportSection {
	lines := strings.Split(log, "\n")

	var sections []reportSection
	var cur *reportSection

	// ensure returns a pointer to the current section, creating a default one if needed.
	ensure := func() *reportSection {
		if cur == nil {
			sections = append(sections, reportSection{})
			cur = &sections[len(sections)-1]
		}
		return cur
	}

	inStdout := false
	inStderr := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		switch {
		case trimmed == "":
			inStdout = false
			inStderr = false

		case strings.HasPrefix(trimmed, "#"):
			inStdout = false
			inStderr = false
			comment := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
			var timing string
			if i := strings.LastIndex(comment, "("); i > 0 && strings.HasSuffix(comment, "s)") {
				timing = comment[i+1 : len(comment)-1]
				comment = strings.TrimSpace(comment[:i])
			}
			// Merge consecutive comment lines into one section.
			// The engine echoes all # lines from the script; consecutive
			// ones (like multi-line preambles) should combine rather than
			// creating separate sections.
			if cur != nil && len(cur.Commands) == 0 {
				// Previous section has no commands yet — merge.
				if cur.Comment != "" {
					cur.Comment += "\n"
				}
				cur.Comment += comment
				if timing != "" {
					cur.Timing = timing
				}
			} else {
				sections = append(sections, reportSection{
					Comment: comment,
					Timing:  timing,
				})
				cur = &sections[len(sections)-1]
			}

		case strings.HasPrefix(trimmed, ">"):
			inStdout = false
			inStderr = false
			cmd := strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))
			sec := ensure()
			sec.Commands = append(sec.Commands, reportCommand{Line: cmd})

		case trimmed == "[stdout]":
			inStdout = true
			inStderr = false

		case trimmed == "[stderr]":
			inStderr = true
			inStdout = false

		case trimmed == "[condition not met]":
			inStdout = false
			inStderr = false
			sec := ensure()
			if n := len(sec.Commands); n > 0 {
				sec.Commands[n-1].CondNotMet = true
			}

		case strings.HasPrefix(trimmed, "matched:"):
			inStdout = false
			inStderr = false
			sec := ensure()
			if n := len(sec.Commands); n > 0 {
				sec.Commands[n-1].Matched = trimmed
			}

		case strings.HasPrefix(trimmed, "[condition"):
			inStdout = false
			inStderr = false

		case inStdout:
			sec := ensure()
			if n := len(sec.Commands); n > 0 {
				sec.Commands[n-1].Stdout = append(sec.Commands[n-1].Stdout, trimmed)
			}

		case inStderr:
			// stderr lines are not captured in report commands

		default:
			inStdout = false
			inStderr = false
		}
	}

	// Classify screenshots in each command.
	for i := range sections {
		for j := range sections[i].Commands {
			classifyScreenshots(&sections[i].Commands[j])
		}
	}

	return sections
}

// classifyScreenshots inspects stdout lines for .png paths and probes disk
// for -unblurred and .fail.png companions. It also extracts compare results.
func classifyScreenshots(cmd *reportCommand) {
	for _, line := range cmd.Stdout {
		if strings.HasSuffix(line, ".png") {
			ref := screenshotRef{Path: line}
			// Probe for -unblurred companion.
			ext := filepath.Ext(line)
			base := strings.TrimSuffix(line, ext)
			unblurred := base + "-unblurred" + ext
			if _, err := os.Stat(unblurred); err == nil {
				ref.Unblurred = unblurred
			}
			// Probe for .current.png companion (screenshot-compare saves this).
			currentPath := base + ".current" + ext
			if _, err := os.Stat(currentPath); err == nil {
				ref.CurrentPath = currentPath
			}
			// Probe for .diff.png companion.
			diffPath := base + ".diff" + ext
			if _, err := os.Stat(diffPath); err == nil {
				ref.DiffPath = diffPath
			}
			// Probe for .fail.png companion.
			failPath := line + ".fail.png"
			if _, err := os.Stat(failPath); err == nil {
				ref.FailPath = failPath
			}
			cmd.Screenshots = append(cmd.Screenshots, ref)
		}
		// Extract compare results from stdout.
		switch {
		case line == "baseline created":
			cmd.CompareResult = "baseline created"
		case line == "baseline updated":
			cmd.CompareResult = "baseline updated"
		case strings.HasPrefix(line, "diff:"):
			cmd.CompareResult = line
		}
	}
}

// ExtractPreamble reads leading # comment lines from script source as
// narrative prose. Stops at the first non-comment, non-blank line.
func ExtractPreamble(source []byte) string {
	var lines []string
	for _, line := range strings.Split(string(source), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if len(lines) > 0 {
				break
			}
			continue
		}
		if !strings.HasPrefix(trimmed, "#") {
			break
		}
		lines = append(lines, strings.TrimSpace(strings.TrimPrefix(trimmed, "#")))
	}
	return strings.Join(lines, "\n")
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

// isAssertionCmd reports whether cmd is a scripttest assertion (stdout, stderr,
// cmp, etc.) that should be excluded from the report's command blocks.
func isAssertionCmd(cmd string) bool {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return false
	}
	switch fields[0] {
	case "stdout", "stderr", "stdin", "cmp", "cmpenv", "grep":
		return true
	}
	if fields[0] == "!" && len(fields) > 1 {
		return true
	}
	return false
}

// RenderReport writes a GFM markdown report for a single script to w.
func RenderReport(w io.Writer, name, preamble string, sections []reportSection, reportDir string) {
	fmt.Fprintf(w, "# %s\n\n", name)

	if preamble != "" {
		for _, line := range strings.Split(preamble, "\n") {
			fmt.Fprintf(w, "> %s\n", line)
		}
		fmt.Fprintln(w)
	}

	for _, sec := range sections {
		if preamble != "" {
			sec.Comment = stripPreambleLines(sec.Comment, preamble)
		}
		// Skip sections that became empty after preamble stripping
		// and have no commands.
		if sec.Comment == "" && len(sec.Commands) == 0 {
			continue
		}
		renderSection(w, sec, reportDir)
	}
}

// stripPreambleLines removes lines from comment that appear in the preamble,
// returning whatever remains (typically the last line with timing info).
func stripPreambleLines(comment, preamble string) string {
	preambleLines := make(map[string]bool)
	for _, line := range strings.Split(preamble, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			preambleLines[line] = true
		}
	}
	var kept []string
	for _, line := range strings.Split(comment, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !preambleLines[line] {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

// renderSection writes a single report section as GFM markdown.
func renderSection(w io.Writer, sec reportSection, reportDir string) {
	if sec.Comment != "" {
		heading := sec.Comment
		if sec.Timing != "" {
			heading += " (" + sec.Timing + ")"
		}
		fmt.Fprintf(w, "## %s\n\n", heading)
	}

	// Group non-assertion commands into fenced code blocks.
	var cmdBlock []string
	flush := func() {
		if len(cmdBlock) == 0 {
			return
		}
		fmt.Fprintln(w, "```")
		for _, c := range cmdBlock {
			fmt.Fprintln(w, c)
		}
		fmt.Fprintln(w, "```")
		fmt.Fprintln(w)
		cmdBlock = nil
	}

	for _, cmd := range sec.Commands {
		if isAssertionCmd(cmd.Line) {
			continue
		}

		cmdBlock = append(cmdBlock, cmd.Line)

		// If this command has screenshots or compare results, flush
		// the code block and render them inline.
		if len(cmd.Screenshots) > 0 || cmd.CompareResult != "" {
			flush()

			if cmd.CompareResult != "" {
				fmt.Fprintf(w, "**Result:** %s\n\n", cmd.CompareResult)
			}

			for _, ref := range cmd.Screenshots {
				imgName := filepath.Base(ref.Path)
				relPath := relImage(reportDir, ref.Path)

				if ref.Unblurred != "" {
					// Side-by-side table for blurred/unblurred (+ diff if available).
					unblurredName := filepath.Base(ref.Unblurred)
					unblurredRel := relImage(reportDir, ref.Unblurred)
					if ref.DiffPath != "" {
						diffName := filepath.Base(ref.DiffPath)
						diffRel := relImage(reportDir, ref.DiffPath)
						fmt.Fprintln(w, "| Blurred | Unblurred | Diff |")
						fmt.Fprintln(w, "|---------|-----------|------|")
						fmt.Fprintf(w, "| ![%s](%s) | ![%s](%s) | ![%s](%s) |\n",
							imgName, relPath, unblurredName, unblurredRel, diffName, diffRel)
					} else {
						fmt.Fprintln(w, "| Blurred | Unblurred |")
						fmt.Fprintln(w, "|---------|-----------|")
						fmt.Fprintf(w, "| ![%s](%s) | ![%s](%s) |\n",
							imgName, relPath, unblurredName, unblurredRel)
					}
					fmt.Fprintln(w)
				} else if ref.FailPath != "" {
					// Baseline vs failed current (+ diff if available).
					failName := filepath.Base(ref.FailPath)
					failRel := relImage(reportDir, ref.FailPath)
					if ref.DiffPath != "" {
						diffName := filepath.Base(ref.DiffPath)
						diffRel := relImage(reportDir, ref.DiffPath)
						fmt.Fprintln(w, "| Baseline | Current | Diff |")
						fmt.Fprintln(w, "|----------|---------|------|")
						fmt.Fprintf(w, "| ![%s](%s) | ![%s](%s) | ![%s](%s) |\n",
							imgName, relPath, failName, failRel, diffName, diffRel)
					} else {
						fmt.Fprintln(w, "| Baseline | Current |")
						fmt.Fprintln(w, "|----------|---------|")
						fmt.Fprintf(w, "| ![%s](%s) | ![%s](%s) |\n",
							imgName, relPath, failName, failRel)
					}
					fmt.Fprintln(w)
				} else if ref.CurrentPath != "" {
					// Passing compare: baseline vs current (+ diff if available).
					currentName := filepath.Base(ref.CurrentPath)
					currentRel := relImage(reportDir, ref.CurrentPath)
					if ref.DiffPath != "" {
						diffName := filepath.Base(ref.DiffPath)
						diffRel := relImage(reportDir, ref.DiffPath)
						fmt.Fprintln(w, "| Baseline | Current | Diff |")
						fmt.Fprintln(w, "|----------|---------|------|")
						fmt.Fprintf(w, "| ![%s](%s) | ![%s](%s) | ![%s](%s) |\n",
							imgName, relPath, currentName, currentRel, diffName, diffRel)
					} else {
						fmt.Fprintln(w, "| Baseline | Current |")
						fmt.Fprintln(w, "|----------|---------|")
						fmt.Fprintf(w, "| ![%s](%s) | ![%s](%s) |\n",
							imgName, relPath, currentName, currentRel)
					}
					fmt.Fprintln(w)
				} else if ref.DiffPath != "" {
					// Screenshot with diff only (no unblurred/fail/current).
					diffName := filepath.Base(ref.DiffPath)
					diffRel := relImage(reportDir, ref.DiffPath)
					fmt.Fprintln(w, "| Screenshot | Diff |")
					fmt.Fprintln(w, "|------------|------|")
					fmt.Fprintf(w, "| ![%s](%s) | ![%s](%s) |\n",
						imgName, relPath, diffName, diffRel)
					fmt.Fprintln(w)
				} else {
					// Single screenshot, no companions.
					fmt.Fprintf(w, "![%s](%s)\n\n", imgName, relPath)
				}
			}
		}
	}
	flush()
}

// relImage computes a relative path from reportDir to imgPath for GFM embedding.
func relImage(reportDir, imgPath string) string {
	rel, err := filepath.Rel(reportDir, imgPath)
	if err != nil {
		return imgPath
	}
	return rel
}

// GenerateReport writes a GFM markdown report to reportPath from the script
// execution log.
func GenerateReport(reportPath, scriptName string, scriptSource []byte, log string) error {
	sections := ParseLog(log)
	preamble := ExtractPreamble(scriptSource)
	reportDir := filepath.Dir(reportPath)

	if err := os.MkdirAll(reportDir, 0o777); err != nil {
		return fmt.Errorf("report mkdir: %w", err)
	}

	f, err := os.Create(reportPath)
	if err != nil {
		return fmt.Errorf("report create: %w", err)
	}
	defer f.Close()

	RenderReport(f, scriptName, preamble, sections, reportDir)
	return nil
}

// ScriptReport holds the data needed to render one script's report. Collect
// these from parallel subtests and pass them to GenerateCombinedReport.
type ScriptReport struct {
	Name        string // script name (used as heading)
	Source      []byte // raw script source (for preamble)
	Log         string // engine log output
	ArtifactDir string // per-script artifact directory
	Failed      bool   // true if the script failed
}

// scriptEntry is a TOC entry for the combined report. It tracks metadata
// from the script source (available upfront) and the result (available
// after the script finishes).
type scriptEntry struct {
	Name    string // script name
	Source  []byte // raw script source
	Summary string // first line of preamble
	Report  *ScriptReport
}

// CombinedReportWriter manages a live-updating combined report file.
// It writes the full file on each update so a live-reloading viewer
// can show progress as tests complete.
type CombinedReportWriter struct {
	path    string
	entries []scriptEntry // sorted by name
}

// NewCombinedReportWriter creates a combined report at reportPath with a TOC
// listing all script names as pending. The names slice should contain all
// script names that will be reported; sources maps name to raw script source.
func NewCombinedReportWriter(reportPath string, names []string, sources map[string][]byte) (*CombinedReportWriter, error) {
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o777); err != nil {
		return nil, fmt.Errorf("combined report mkdir: %w", err)
	}

	entries := make([]scriptEntry, len(names))
	for i, name := range names {
		src := sources[name]
		summary := strings.SplitN(ExtractPreamble(src), "\n", 2)[0]
		entries[i] = scriptEntry{Name: name, Source: src, Summary: summary}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	w := &CombinedReportWriter{path: reportPath, entries: entries}
	if err := w.flush(); err != nil {
		return nil, err
	}
	return w, nil
}

// Update records a completed script and rewrites the report file.
func (w *CombinedReportWriter) Update(sr ScriptReport) error {
	for i := range w.entries {
		if w.entries[i].Name == sr.Name {
			w.entries[i].Report = &sr
			break
		}
	}
	return w.flush()
}

// flush rewrites the entire report file from current state.
func (w *CombinedReportWriter) flush() error {
	f, err := os.Create(w.path)
	if err != nil {
		return fmt.Errorf("combined report create: %w", err)
	}
	defer f.Close()

	reportDir := filepath.Dir(w.path)

	// Count status.
	var passed, failed, pending int
	for _, e := range w.entries {
		switch {
		case e.Report == nil:
			pending++
		case e.Report.Failed:
			failed++
		default:
			passed++
		}
	}

	// Header + TOC.
	fmt.Fprintln(f, "# CDP Script Test Report")
	fmt.Fprintln(f)
	if pending > 0 {
		fmt.Fprintf(f, "%d scripts: %d passed, %d failed, %d pending\n\n", len(w.entries), passed, failed, pending)
	} else {
		fmt.Fprintf(f, "%d scripts: %d passed, %d failed\n\n", len(w.entries), passed, failed)
	}
	fmt.Fprintln(f, "## Contents")
	fmt.Fprintln(f)
	for _, e := range w.entries {
		anchor := slugify(e.Name)
		status := "PEND"
		if e.Report != nil {
			status = "PASS"
			if e.Report.Failed {
				status = "FAIL"
			}
		}
		if e.Summary != "" {
			fmt.Fprintf(f, "- %s [%s](#%s) — %s\n", status, e.Name, anchor, e.Summary)
		} else {
			fmt.Fprintf(f, "- %s [%s](#%s)\n", status, e.Name, anchor)
		}
	}
	fmt.Fprintln(f)
	fmt.Fprintln(f, "---")
	fmt.Fprintln(f)

	// Script details (only completed scripts get content).
	for _, e := range w.entries {
		if e.Report == nil {
			continue
		}
		sr := e.Report
		sections := ParseLog(sr.Log)
		preamble := ExtractPreamble(sr.Source)

		openAttr := ""
		if sr.Failed {
			openAttr = " open"
		}
		fmt.Fprintf(f, "<details id=\"%s\"%s>\n", slugify(sr.Name), openAttr)
		status := "PASS"
		if sr.Failed {
			status = "FAIL"
		}
		if e.Summary != "" {
			fmt.Fprintf(f, "<summary>%s <strong>%s</strong> — %s</summary>\n\n", status, sr.Name, e.Summary)
		} else {
			fmt.Fprintf(f, "<summary>%s <strong>%s</strong></summary>\n\n", status, sr.Name)
		}

		RenderReport(f, sr.Name, preamble, sections, reportDir)

		fmt.Fprintln(f, "</details>")
		fmt.Fprintln(f)
	}
	return nil
}

// GenerateCombinedReport writes a single GFM markdown file combining all
// script reports. This is a convenience wrapper for non-live use cases.
func GenerateCombinedReport(reportPath string, scripts []ScriptReport) error {
	names := make([]string, len(scripts))
	sources := make(map[string][]byte, len(scripts))
	for i, sr := range scripts {
		names[i] = sr.Name
		sources[sr.Name] = sr.Source
	}
	w, err := NewCombinedReportWriter(reportPath, names, sources)
	if err != nil {
		return err
	}
	for _, sr := range scripts {
		if err := w.Update(sr); err != nil {
			return err
		}
	}
	return nil
}

// slugify converts a name to a GFM-compatible anchor slug.
func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		if r == ' ' || r == '_' {
			return '-'
		}
		return -1
	}, s)
	return s
}

// reportStreamer is an io.Writer that wraps the engine's log writer and
// simultaneously streams a GFM markdown report to a second writer as the
// script executes. Each section is emitted as soon as a new section boundary
// (a # comment line) is encountered, so the report builds up in real time.
//
// Use NewReportStreamer to create one, and call Flush after the script finishes
// to emit the final section.
type reportStreamer struct {
	inner     io.Writer // original log destination (strings.Builder)
	report    io.Writer // streaming report destination (t.Output())
	reportDir string    // base dir for relative image paths
	partial   []byte    // incomplete line buffer

	// header state: written once at the start
	headerDone bool
	name       string
	preamble   string

	// current section being accumulated
	cur      *reportSection
	inStdout bool
	inStderr bool
}

// NewReportStreamer returns an io.Writer that passes all bytes through to inner
// (preserving the engine log) and simultaneously parses them to stream a GFM
// report to report. scriptSource is used to extract the preamble.
//
// Call Flush after the script engine finishes to emit the last section and
// any trailing content.
func NewReportStreamer(inner, report io.Writer, name, reportDir string, scriptSource []byte) *reportStreamer {
	return &reportStreamer{
		inner:     inner,
		report:    report,
		reportDir: reportDir,
		name:      name,
		preamble:  ExtractPreamble(scriptSource),
	}
}

// Write passes p to the inner writer, then parses complete lines for report
// streaming. Partial lines are buffered until a newline arrives.
func (rs *reportStreamer) Write(p []byte) (int, error) {
	n, err := rs.inner.Write(p)

	// Buffer incoming bytes, process complete lines.
	rs.partial = append(rs.partial, p[:n]...)
	for {
		idx := indexOf(rs.partial, '\n')
		if idx < 0 {
			break
		}
		line := string(rs.partial[:idx])
		rs.partial = rs.partial[idx+1:]
		rs.processLine(line)
	}

	return n, err
}

// Flush emits the final section and any buffered partial line.
func (rs *reportStreamer) Flush() {
	// Process any remaining partial line.
	if len(rs.partial) > 0 {
		rs.processLine(string(rs.partial))
		rs.partial = nil
	}
	rs.emitSection()
}

// processLine handles a single complete log line.
func (rs *reportStreamer) processLine(line string) {
	if !rs.headerDone {
		rs.headerDone = true
		fmt.Fprintf(rs.report, "# %s\n\n", rs.name)
		if rs.preamble != "" {
			for _, pl := range strings.Split(rs.preamble, "\n") {
				fmt.Fprintf(rs.report, "> %s\n", pl)
			}
			fmt.Fprintln(rs.report)
		}
	}

	trimmed := strings.TrimSpace(line)

	switch {
	case trimmed == "":
		rs.inStdout = false
		rs.inStderr = false

	case strings.HasPrefix(trimmed, "#"):
		rs.inStdout = false
		rs.inStderr = false
		comment := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
		var timing string
		if i := strings.LastIndex(comment, "("); i > 0 && strings.HasSuffix(comment, "s)") {
			timing = comment[i+1 : len(comment)-1]
			comment = strings.TrimSpace(comment[:i])
		}
		// Merge consecutive comment lines into one section.
		if rs.cur != nil && len(rs.cur.Commands) == 0 {
			if rs.cur.Comment != "" {
				rs.cur.Comment += "\n"
			}
			rs.cur.Comment += comment
			if timing != "" {
				rs.cur.Timing = timing
			}
		} else {
			// New section boundary: emit the previous section.
			rs.emitSection()
			rs.cur = &reportSection{
				Comment: comment,
				Timing:  timing,
			}
		}

	case strings.HasPrefix(trimmed, ">"):
		rs.inStdout = false
		rs.inStderr = false
		cmd := strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))
		rs.ensureSection()
		rs.cur.Commands = append(rs.cur.Commands, reportCommand{Line: cmd})

	case trimmed == "[stdout]":
		rs.inStdout = true
		rs.inStderr = false

	case trimmed == "[stderr]":
		rs.inStderr = true
		rs.inStdout = false

	case trimmed == "[condition not met]":
		rs.inStdout = false
		rs.inStderr = false
		rs.ensureSection()
		if n := len(rs.cur.Commands); n > 0 {
			rs.cur.Commands[n-1].CondNotMet = true
		}

	case strings.HasPrefix(trimmed, "matched:"):
		rs.inStdout = false
		rs.inStderr = false
		rs.ensureSection()
		if n := len(rs.cur.Commands); n > 0 {
			rs.cur.Commands[n-1].Matched = trimmed
		}

	case strings.HasPrefix(trimmed, "[condition"):
		rs.inStdout = false
		rs.inStderr = false

	case rs.inStdout:
		rs.ensureSection()
		if n := len(rs.cur.Commands); n > 0 {
			rs.cur.Commands[n-1].Stdout = append(rs.cur.Commands[n-1].Stdout, trimmed)
		}

	case rs.inStderr:
		// stderr not captured in report

	default:
		rs.inStdout = false
		rs.inStderr = false
	}
}

func (rs *reportStreamer) ensureSection() {
	if rs.cur == nil {
		rs.cur = &reportSection{}
	}
}

// emitSection classifies screenshots in the current section, renders it
// to the report writer, then clears the current section.
func (rs *reportStreamer) emitSection() {
	if rs.cur == nil {
		return
	}
	if rs.preamble != "" {
		rs.cur.Comment = stripPreambleLines(rs.cur.Comment, rs.preamble)
	}
	// Skip sections that became empty after preamble stripping.
	if rs.cur.Comment == "" && len(rs.cur.Commands) == 0 {
		rs.cur = nil
		return
	}
	for i := range rs.cur.Commands {
		classifyScreenshots(&rs.cur.Commands[i])
	}
	renderSection(rs.report, *rs.cur, rs.reportDir)
	rs.cur = nil
}

func indexOf(b []byte, c byte) int {
	for i, v := range b {
		if v == c {
			return i
		}
	}
	return -1
}
