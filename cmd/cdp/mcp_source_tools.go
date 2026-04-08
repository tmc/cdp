package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tmc/cdp/internal/coverage"
)

// --- Source browsing tools ---

type ListSourcesInput struct {
	Type string `json:"type,omitempty"`
}

type sourceEntry struct {
	URL          string `json:"url"`
	Type         string `json:"type"`
	Size         int    `json:"size"`
	HasSourceMap bool   `json:"has_source_map"`
}

type ReadSourceInput struct {
	URL   string `json:"url"`
	Lines string `json:"lines,omitempty"`
}

type SearchSourceInput struct {
	Pattern    string `json:"pattern"`
	Type       string `json:"type,omitempty"`
	MaxResults int    `json:"max_results,omitempty"`
}

type searchMatch struct {
	URL     string `json:"url"`
	Line    int    `json:"line"`
	Context string `json:"context"`
}

func registerSourceBrowsingTools(server *mcp.Server, s *mcpSession) {
	// list_sources
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_sources",
		Description: "List all captured JavaScript and CSS sources",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListSourcesInput) (*mcp.CallToolResult, any, error) {
		if s.sourceCollector == nil {
			return nil, nil, fmt.Errorf("list_sources: source capture not enabled")
		}
		var entries []sourceEntry
		if input.Type == "" || input.Type == "js" {
			for _, sc := range s.sourceCollector.Scripts() {
				entries = append(entries, sourceEntry{
					URL:          sc.URL,
					Type:         "js",
					Size:         len(sc.Source),
					HasSourceMap: sc.SourceMapURL != "",
				})
			}
		}
		if input.Type == "" || input.Type == "css" {
			for _, st := range s.sourceCollector.Styles() {
				entries = append(entries, sourceEntry{
					URL:          st.URL,
					Type:         "css",
					Size:         len(st.Source),
					HasSourceMap: st.SourceMapURL != "",
				})
			}
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].URL < entries[j].URL })
		data, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return nil, nil, fmt.Errorf("list_sources: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	// read_source
	mcp.AddTool(server, &mcp.Tool{
		Name:        "read_source",
		Description: "Read the content of a captured source file by URL",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ReadSourceInput) (*mcp.CallToolResult, any, error) {
		if s.sourceCollector == nil {
			return nil, nil, fmt.Errorf("read_source: source capture not enabled")
		}
		src, err := findSourceByURL(s, input.URL)
		if err != nil {
			return nil, nil, fmt.Errorf("read_source: %w", err)
		}
		text := src
		if input.Lines != "" {
			text, err = extractLines(src, input.Lines)
			if err != nil {
				return nil, nil, fmt.Errorf("read_source: %w", err)
			}
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: text}},
		}, nil, nil
	})

	// search_source
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_source",
		Description: "Search across all captured sources for a text pattern or regex",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SearchSourceInput) (*mcp.CallToolResult, any, error) {
		if s.sourceCollector == nil {
			return nil, nil, fmt.Errorf("search_source: source capture not enabled")
		}
		maxResults := input.MaxResults
		if maxResults <= 0 {
			maxResults = 20
		}
		// Try regex first; fall back to literal.
		re, reErr := regexp.Compile(input.Pattern)
		match := func(line string) bool {
			if reErr == nil {
				return re.MatchString(line)
			}
			return strings.Contains(line, input.Pattern)
		}

		type srcItem struct {
			url, source, typ string
		}
		var items []srcItem
		if input.Type == "" || input.Type == "js" {
			for _, sc := range s.sourceCollector.Scripts() {
				if sc.Source != "" {
					items = append(items, srcItem{sc.URL, sc.Source, "js"})
				}
			}
		}
		if input.Type == "" || input.Type == "css" {
			for _, st := range s.sourceCollector.Styles() {
				if st.Source != "" {
					items = append(items, srcItem{st.URL, st.Source, "css"})
				}
			}
		}

		var matches []searchMatch
		for _, item := range items {
			lines := strings.Split(item.source, "\n")
			for i, line := range lines {
				if !match(line) {
					continue
				}
				var ctxLines []string
				if i > 0 {
					ctxLines = append(ctxLines, lines[i-1])
				}
				ctxLines = append(ctxLines, line)
				if i+1 < len(lines) {
					ctxLines = append(ctxLines, lines[i+1])
				}
				matches = append(matches, searchMatch{
					URL:     item.url,
					Line:    i + 1,
					Context: strings.Join(ctxLines, "\n"),
				})
				if len(matches) >= maxResults {
					break
				}
			}
			if len(matches) >= maxResults {
				break
			}
		}
		data, err := json.MarshalIndent(matches, "", "  ")
		if err != nil {
			return nil, nil, fmt.Errorf("search_source: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})
}

// findSourceByURL looks up a source by URL across scripts and styles.
func findSourceByURL(s *mcpSession, u string) (string, error) {
	for _, sc := range s.sourceCollector.Scripts() {
		if sc.URL == u {
			if sc.Source == "" {
				return "", fmt.Errorf("source not yet captured for %s", u)
			}
			return sc.Source, nil
		}
	}
	for _, st := range s.sourceCollector.Styles() {
		if st.URL == u {
			if st.Source == "" {
				return "", fmt.Errorf("source not yet captured for %s", u)
			}
			return st.Source, nil
		}
	}
	return "", fmt.Errorf("no source found for URL %s", u)
}

// extractLines returns a subset of lines from src given a range like "10-20".
func extractLines(src, lineRange string) (string, error) {
	parts := strings.SplitN(lineRange, "-", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid line range %q, expected start-end", lineRange)
	}
	start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return "", fmt.Errorf("invalid start line: %w", err)
	}
	end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return "", fmt.Errorf("invalid end line: %w", err)
	}
	lines := strings.Split(src, "\n")
	if start < 1 {
		start = 1
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start > end {
		return "", fmt.Errorf("start line %d > end line %d", start, end)
	}
	var b strings.Builder
	for i := start - 1; i < end; i++ {
		fmt.Fprintf(&b, "%d\t%s\n", i+1, lines[i])
	}
	return b.String(), nil
}

// --- Coverage tools ---

type StartCoverageInput struct{}
type StopCoverageInput struct{}

type GetCoverageInput struct {
	Name string `json:"name,omitempty"`
}

type GetCoverageDeltaInput struct {
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}

func registerCoverageTools(server *mcp.Server, s *mcpSession) {
	// start_coverage
	mcp.AddTool(server, &mcp.Tool{
		Name:        "start_coverage",
		Description: "Start collecting JavaScript code coverage for the current page",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input StartCoverageInput) (*mcp.CallToolResult, any, error) {
		// Stop any previous collector for clean restart (survives page reloads).
		if s.coverageCollector != nil && s.coverageCollector.Running() {
			s.coverageCollector.Stop()
		}
		c := coverage.New(false)
		if err := c.Start(s.activeCtx()); err != nil {
			return nil, nil, fmt.Errorf("start_coverage: %w", err)
		}
		s.coverageCollector = c
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Coverage collection started."}},
		}, nil, nil
	})

	// stop_coverage
	mcp.AddTool(server, &mcp.Tool{
		Name:        "stop_coverage",
		Description: "Stop coverage collection and return final summary",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input StopCoverageInput) (*mcp.CallToolResult, any, error) {
		if s.coverageCollector == nil {
			return nil, nil, fmt.Errorf("stop_coverage: not running")
		}
		if err := s.coverageCollector.Stop(); err != nil {
			return nil, nil, fmt.Errorf("stop_coverage: %w", err)
		}
		// Keep the collector around so the coverage API can still serve snapshots.
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Coverage collection stopped."}},
		}, nil, nil
	})

	// get_coverage
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_coverage",
		Description: "Take a coverage snapshot and return per-file summary sorted by coverage (least covered first)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetCoverageInput) (*mcp.CallToolResult, any, error) {
		if s.coverageCollector == nil {
			return nil, nil, fmt.Errorf("get_coverage: not running")
		}
		snap, err := s.coverageCollector.TakeSnapshot(input.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("get_coverage: %w", err)
		}
		summary := snap.Summary()
		type entry struct {
			url     string
			summary coverage.FileSummary
		}
		entries := make([]entry, 0, len(summary))
		for url, fs := range summary {
			entries = append(entries, entry{url, fs})
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].summary.CoveragePercent < entries[j].summary.CoveragePercent
		})
		var b strings.Builder
		fmt.Fprintf(&b, "Coverage snapshot: %s\n", snap.Name)
		fmt.Fprintf(&b, "Files: %d\n\n", len(entries))
		for _, e := range entries {
			fmt.Fprintf(&b, "  %5.1f%%  %4d/%4d lines  %s\n",
				e.summary.CoveragePercent, e.summary.CoveredLines, e.summary.TotalLines, e.url)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: b.String()}},
		}, nil, nil
	})

	// get_coverage_delta
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_coverage_delta",
		Description: "Get coverage diff between two snapshots showing newly covered lines",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetCoverageDeltaInput) (*mcp.CallToolResult, any, error) {
		if s.coverageCollector == nil {
			return nil, nil, fmt.Errorf("get_coverage_delta: not running")
		}
		snapshots := s.coverageCollector.Snapshots()
		if len(snapshots) < 2 {
			return nil, nil, fmt.Errorf("get_coverage_delta: need at least 2 snapshots")
		}
		before := snapshots[len(snapshots)-2]
		after := snapshots[len(snapshots)-1]
		if input.Before != "" || input.After != "" {
			for _, snap := range snapshots {
				if snap.Name == input.Before {
					before = snap
				}
				if snap.Name == input.After {
					after = snap
				}
			}
		}
		delta := s.coverageCollector.ComputeDelta(before, after)
		var b strings.Builder
		fmt.Fprintf(&b, "Coverage delta: %s → %s\n\n", before.Name, after.Name)
		for url, sd := range delta.Scripts {
			if len(sd.NewlyCovered) == 0 {
				continue
			}
			pctDelta := 0.0
			if sd.TotalLines > 0 {
				pctDelta = float64(sd.CoveredAfter-sd.CoveredBefore) / float64(sd.TotalLines) * 100
			}
			fmt.Fprintf(&b, "%s  (+%.1f%%, %d new lines)\n", url, pctDelta, len(sd.NewlyCovered))
			lines := make([]int, 0, len(sd.NewlyCovered))
			for ln := range sd.NewlyCovered {
				lines = append(lines, ln)
			}
			sort.Ints(lines)
			for _, ln := range lines {
				fmt.Fprintf(&b, "  L%d\n", ln)
			}
			b.WriteString("\n")
		}
		if b.Len() == 0 {
			b.WriteString("No new coverage between snapshots.")
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: b.String()}},
		}, nil, nil
	})

	// compare_coverage — detailed per-file, per-function comparison
	type CompareCoverageInput struct {
		Before string `json:"before"`
		After  string `json:"after"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "compare_coverage",
		Description: "Compare two named coverage snapshots with per-file, per-function, and line-range detail",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CompareCoverageInput) (*mcp.CallToolResult, any, error) {
		if s.coverageCollector == nil {
			return nil, nil, fmt.Errorf("compare_coverage: not running")
		}
		snapshots := s.coverageCollector.Snapshots()
		var before, after *coverage.Snapshot
		for _, snap := range snapshots {
			if snap.Name == input.Before {
				before = snap
			}
			if snap.Name == input.After {
				after = snap
			}
		}
		if before == nil {
			return nil, nil, fmt.Errorf("compare_coverage: snapshot %q not found", input.Before)
		}
		if after == nil {
			return nil, nil, fmt.Errorf("compare_coverage: snapshot %q not found", input.After)
		}
		text := formatDetailedComparison(s.coverageCollector, before, after)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: text}},
		}, nil, nil
	})

	// list_snapshots
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_snapshots",
		Description: "List all coverage snapshots taken during this session",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
		if s.coverageCollector == nil {
			return nil, nil, fmt.Errorf("list_snapshots: coverage not running")
		}
		snapshots := s.coverageCollector.Snapshots()
		if len(snapshots) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "No snapshots taken yet."}},
			}, nil, nil
		}
		var b strings.Builder
		fmt.Fprintf(&b, "Snapshots: %d\n\n", len(snapshots))
		for i, snap := range snapshots {
			summary := snap.Summary()
			totalCov := 0
			totalLines := 0
			for _, fs := range summary {
				totalCov += fs.CoveredLines
				totalLines += fs.TotalLines
			}
			pct := 0.0
			if totalLines > 0 {
				pct = float64(totalCov) / float64(totalLines) * 100
			}
			fmt.Fprintf(&b, "  %d. %s  (%s, %d files, %.1f%% covered)\n",
				i+1, snap.Name, snap.Timestamp.Format("15:04:05"), len(summary), pct)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: b.String()}},
		}, nil, nil
	})
}

// formatDetailedComparison produces a detailed per-file, per-function comparison.
func formatDetailedComparison(cc *coverage.Collector, before, after *coverage.Snapshot) string {
	delta := cc.ComputeDelta(before, after)
	var b strings.Builder

	// Agent-friendly summary header.
	totalNew := 0
	filesChanged := 0
	type fileDelta struct {
		url      string
		newLines int
		funcs    []string
	}
	var topFiles []fileDelta
	for url, sd := range delta.Scripts {
		if len(sd.NewlyCovered) == 0 {
			continue
		}
		filesChanged++
		totalNew += len(sd.NewlyCovered)
		fd := fileDelta{url: url, newLines: len(sd.NewlyCovered)}
		if afterCov, ok := after.Scripts[url]; ok {
			beforeCov := before.Scripts[url]
			for _, fn := range afterCov.Functions {
				if fn.HitCount == 0 {
					continue
				}
				beforeHit := 0
				if beforeCov != nil {
					for _, bfn := range beforeCov.Functions {
						if bfn.Name == fn.Name && bfn.StartLine == fn.StartLine {
							beforeHit = bfn.HitCount
							break
						}
					}
				}
				if beforeHit == 0 && fn.HitCount > 0 && fn.Name != "" {
					fd.funcs = append(fd.funcs, fn.Name)
				}
			}
		}
		topFiles = append(topFiles, fd)
	}
	sort.Slice(topFiles, func(i, j int) bool { return topFiles[i].newLines > topFiles[j].newLines })

	fmt.Fprintf(&b, "Summary: %s → %s exercised %d new lines across %d files.\n",
		before.Name, after.Name, totalNew, filesChanged)
	for i, fd := range topFiles {
		if i >= 5 {
			break
		}
		line := fmt.Sprintf("  %s (+%d lines", fd.url, fd.newLines)
		if len(fd.funcs) > 0 {
			line += ", functions: " + strings.Join(fd.funcs, ", ")
		}
		line += ")"
		fmt.Fprintf(&b, "%s\n", line)
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "Detailed comparison: %s → %s\n", before.Name, after.Name)
	fmt.Fprintf(&b, "========================================\n\n")

	for url, sd := range delta.Scripts {
		pctBefore := 0.0
		pctAfter := 0.0
		if sd.TotalLines > 0 {
			pctBefore = float64(sd.CoveredBefore) / float64(sd.TotalLines) * 100
			pctAfter = float64(sd.CoveredAfter) / float64(sd.TotalLines) * 100
		}
		pctDelta := pctAfter - pctBefore
		sign := "+"
		if pctDelta < 0 {
			sign = ""
		}
		fmt.Fprintf(&b, "%s\n", url)
		fmt.Fprintf(&b, "  Coverage: %.1f%% → %.1f%% (%s%.1f%%)\n", pctBefore, pctAfter, sign, pctDelta)
		fmt.Fprintf(&b, "  Lines: %d/%d covered (was %d)\n", sd.CoveredAfter, sd.TotalLines, sd.CoveredBefore)

		// Per-function detail from the after snapshot.
		afterCov, ok := after.Scripts[url]
		if ok && len(afterCov.Functions) > 0 {
			beforeCov := before.Scripts[url]
			fmt.Fprintf(&b, "  Functions:\n")
			for _, fn := range afterCov.Functions {
				name := fn.Name
				if name == "" {
					name = "(anonymous)"
				}
				beforeHit := 0
				if beforeCov != nil {
					for _, bfn := range beforeCov.Functions {
						if bfn.Name == fn.Name && bfn.StartLine == fn.StartLine {
							beforeHit = bfn.HitCount
							break
						}
					}
				}
				if fn.HitCount != beforeHit {
					fmt.Fprintf(&b, "    %s (L%d-%d): %d → %d hits\n",
						name, fn.StartLine, fn.EndLine, beforeHit, fn.HitCount)
				}
			}
		}

		// Line ranges for newly covered lines.
		if len(sd.NewlyCovered) > 0 {
			lines := make([]int, 0, len(sd.NewlyCovered))
			for ln := range sd.NewlyCovered {
				lines = append(lines, ln)
			}
			sort.Ints(lines)
			ranges := compactLineRanges(lines)
			fmt.Fprintf(&b, "  Newly covered: %s\n", ranges)
		}
		b.WriteString("\n")
	}

	if len(delta.Scripts) == 0 {
		b.WriteString("No coverage changes between snapshots.\n")
	}
	return b.String()
}

// compactLineRanges turns [1,2,3,5,7,8,9] into "1-3, 5, 7-9".
func compactLineRanges(lines []int) string {
	if len(lines) == 0 {
		return ""
	}
	var parts []string
	start := lines[0]
	end := lines[0]
	for i := 1; i < len(lines); i++ {
		if lines[i] == end+1 {
			end = lines[i]
		} else {
			parts = append(parts, formatRange(start, end))
			start = lines[i]
			end = lines[i]
		}
	}
	parts = append(parts, formatRange(start, end))
	return strings.Join(parts, ", ")
}

func formatRange(start, end int) string {
	if start == end {
		return strconv.Itoa(start)
	}
	return fmt.Sprintf("%d-%d", start, end)
}
