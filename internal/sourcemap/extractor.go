// Package sourcemap extracts code chunks from coverage byte ranges and
// generates sourcemap v3 files for synthetic source attribution.
package sourcemap

import (
	"sort"
	"strings"
)

// CoverageRange is a byte-offset range with an execution count.
// Matches the shape from internal/coverage but avoids the import cycle.
type CoverageRange struct {
	StartOffset int
	EndOffset   int
	Count       int
}

// CodeChunk is a contiguous region of code extracted from a bundle.
type CodeChunk struct {
	// Byte offsets in the original bundle source.
	StartOffset int
	EndOffset   int

	// The extracted code text.
	Code string

	// Line/column positions (1-based) in the bundle.
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int

	// Context: a few lines before and after the chunk for orientation.
	ContextBefore string
	ContextAfter  string

	// Hit count from coverage (sum of range counts).
	HitCount int
}

// ExtractChunks groups coverage byte ranges, merges overlapping ones,
// extracts the code text, and computes line/col positions.
// contextLines controls how many lines of surrounding context to include.
func ExtractChunks(source string, ranges []CoverageRange, contextLines int) []CodeChunk {
	if len(ranges) == 0 || source == "" {
		return nil
	}

	// Filter to executed ranges (count > 0) and sort by start offset.
	var executed []CoverageRange
	for _, r := range ranges {
		if r.Count > 0 && r.StartOffset < r.EndOffset {
			executed = append(executed, r)
		}
	}
	if len(executed) == 0 {
		return nil
	}
	sort.Slice(executed, func(i, j int) bool {
		return executed[i].StartOffset < executed[j].StartOffset
	})

	// Merge overlapping/adjacent ranges.
	merged := mergeRanges(executed)

	// Build line offset index for efficient offset→line/col lookup.
	lineOffsets := buildLineOffsets(source)

	var chunks []CodeChunk
	for _, r := range merged {
		start := clamp(r.StartOffset, 0, len(source))
		end := clamp(r.EndOffset, 0, len(source))
		if start >= end {
			continue
		}

		sl, sc := offsetToLineCol(lineOffsets, start)
		el, ec := offsetToLineCol(lineOffsets, end)

		chunk := CodeChunk{
			StartOffset: start,
			EndOffset:   end,
			Code:        source[start:end],
			StartLine:   sl,
			StartCol:    sc,
			EndLine:     el,
			EndCol:      ec,
			HitCount:    r.Count,
		}

		if contextLines > 0 {
			chunk.ContextBefore = extractContext(source, lineOffsets, sl, contextLines, true)
			chunk.ContextAfter = extractContext(source, lineOffsets, el, contextLines, false)
		}

		chunks = append(chunks, chunk)
	}

	return chunks
}

// mergeRanges merges overlapping or adjacent coverage ranges.
// Input must be sorted by StartOffset.
func mergeRanges(ranges []CoverageRange) []CoverageRange {
	if len(ranges) == 0 {
		return nil
	}
	merged := []CoverageRange{ranges[0]}
	for _, r := range ranges[1:] {
		last := &merged[len(merged)-1]
		if r.StartOffset <= last.EndOffset {
			// Overlapping or adjacent — extend.
			if r.EndOffset > last.EndOffset {
				last.EndOffset = r.EndOffset
			}
			last.Count += r.Count
		} else {
			merged = append(merged, r)
		}
	}
	return merged
}

// buildLineOffsets returns the byte offset of the start of each line.
// lineOffsets[0] = 0 (line 1 starts at byte 0).
func buildLineOffsets(source string) []int {
	offsets := []int{0}
	for i := 0; i < len(source); i++ {
		if source[i] == '\n' {
			offsets = append(offsets, i+1)
		}
	}
	return offsets
}

// offsetToLineCol converts a byte offset to 1-based line and column
// using a precomputed line offset index.
func offsetToLineCol(lineOffsets []int, offset int) (line, col int) {
	// Binary search for the line.
	line = sort.SearchInts(lineOffsets, offset+1)
	// line is now the index of the first lineOffset > offset,
	// so the actual line is that index (1-based).
	if line > len(lineOffsets) {
		line = len(lineOffsets)
	}
	col = offset - lineOffsets[line-1] + 1
	return line, col
}

// extractContext returns contextLines lines before (if before=true) or
// after (if before=false) the given line.
func extractContext(source string, lineOffsets []int, line, count int, before bool) string {
	totalLines := len(lineOffsets)
	var startLine, endLine int
	if before {
		startLine = line - count
		endLine = line - 1
	} else {
		startLine = line + 1
		endLine = line + count
	}
	startLine = clamp(startLine, 1, totalLines)
	endLine = clamp(endLine, 1, totalLines)
	if startLine > endLine {
		return ""
	}

	startOff := lineOffsets[startLine-1]
	var endOff int
	if endLine < totalLines {
		endOff = lineOffsets[endLine] // end of endLine (start of next line)
	} else {
		endOff = len(source)
	}
	return source[startOff:endOff]
}

// SplitFunctions takes a bundle source and its function coverage data,
// returning one CodeChunk per function (only executed functions).
func SplitFunctions(source string, functions []FunctionRange) []CodeChunk {
	if source == "" || len(functions) == 0 {
		return nil
	}
	lineOffsets := buildLineOffsets(source)
	var chunks []CodeChunk
	for _, fn := range functions {
		if fn.HitCount == 0 {
			continue
		}
		start := clamp(fn.StartOffset, 0, len(source))
		end := clamp(fn.EndOffset, 0, len(source))
		if start >= end {
			continue
		}
		sl, sc := offsetToLineCol(lineOffsets, start)
		el, ec := offsetToLineCol(lineOffsets, end)
		chunks = append(chunks, CodeChunk{
			StartOffset: start,
			EndOffset:   end,
			Code:        source[start:end],
			StartLine:   sl,
			StartCol:    sc,
			EndLine:     el,
			EndCol:      ec,
			HitCount:    fn.HitCount,
		})
	}
	return chunks
}

// FunctionRange describes a function's byte range in a bundle.
type FunctionRange struct {
	Name        string
	StartOffset int
	EndOffset   int
	HitCount    int
}

// LineMap builds a mapping from bundle lines to extracted chunk indices.
// Returns a slice indexed by 1-based line number; value is the chunk index
// (0-based) or -1 if the line is not in any chunk.
func LineMap(totalLines int, chunks []CodeChunk) []int {
	m := make([]int, totalLines+1)
	for i := range m {
		m[i] = -1
	}
	for idx, c := range chunks {
		for line := c.StartLine; line <= c.EndLine; line++ {
			if line >= 0 && line < len(m) {
				m[line] = idx
			}
		}
	}
	return m
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// CountLinesInString returns the number of newline-terminated lines.
func CountLinesInString(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}
