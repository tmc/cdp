package sourcemap

import (
	"fmt"
	"strings"
)

// Structure is an inferred source-file layout for a JavaScript bundle.
type Structure struct {
	Files   []File `json:"files"`
	Summary string `json:"summary"`
}

// File is one inferred original source file inside a bundled script.
type File struct {
	Path        string     `json:"path"`
	Description string     `json:"description"`
	StartLine   int        `json:"start_line"`
	EndLine     int        `json:"end_line"`
	StartOffset int        `json:"start_offset"`
	EndOffset   int        `json:"end_offset"`
	Functions   []Function `json:"functions,omitempty"`
	Framework   string     `json:"framework,omitempty"`
	Module      string     `json:"module,omitempty"`
}

// Function is one inferred original function inside a source file.
type Function struct {
	Name        string `json:"name"`
	StartLine   int    `json:"start_line"`
	EndLine     int    `json:"end_line"`
	Description string `json:"description"`
	Exported    bool   `json:"exported,omitempty"`
}

// GenerateFromStructure builds a sourcemap v3 from an inferred file structure.
//
// For single-line minified bundles, byte offsets are used as generated column
// positions. Line-level mappings are not precise enough when the whole bundle
// is on one line.
func GenerateFromStructure(bundleSource string, structure *Structure) ([]byte, error) {
	if structure == nil || len(structure.Files) == 0 {
		return nil, fmt.Errorf("no inferred files")
	}

	totalLines := CountLinesInString(bundleSource)
	useByteOffsets := totalLines <= 3 && len(bundleSource) > 1000

	var sources []string
	var sourcesContent []string
	var mappings []Mapping
	var names []string
	nameIdx := make(map[string]int)

	for srcIdx, f := range structure.Files {
		sources = append(sources, f.Path)

		if useByteOffsets {
			startOff := clampStructure(f.StartOffset, 0, len(bundleSource))
			endOff := clampStructure(f.EndOffset, startOff, len(bundleSource))
			if endOff <= startOff && f.EndOffset == 0 {
				startOff = 0
				endOff = len(bundleSource)
			}
			sourcesContent = append(sourcesContent, bundleSource[startOff:endOff])

			mappings = append(mappings, Mapping{
				GeneratedLine: 0,
				GeneratedCol:  startOff,
				SourceIdx:     srcIdx,
				OriginalLine:  0,
				OriginalCol:   0,
				NameIdx:       -1,
			})
		} else {
			startLine := clampStructure(f.StartLine, 1, totalLines)
			endLine := clampStructure(f.EndLine, startLine, totalLines)
			sourcesContent = append(sourcesContent, extractLineRange(bundleSource, startLine, endLine))

			for line := startLine; line <= endLine; line++ {
				mappings = append(mappings, Mapping{
					GeneratedLine: line - 1,
					GeneratedCol:  0,
					SourceIdx:     srcIdx,
					OriginalLine:  line - startLine,
					OriginalCol:   0,
					NameIdx:       -1,
				})
			}
		}

		for _, fn := range f.Functions {
			if fn.Name == "" {
				continue
			}
			idx, ok := nameIdx[fn.Name]
			if !ok {
				idx = len(names)
				names = append(names, fn.Name)
				nameIdx[fn.Name] = idx
			}

			if useByteOffsets {
				fnOff := clampStructure(f.StartOffset, 0, len(bundleSource))
				if fn.StartLine > f.StartLine {
					fileRange := f.EndOffset - f.StartOffset
					lineRange := f.EndLine - f.StartLine
					if lineRange > 0 {
						fnOff = f.StartOffset + (fn.StartLine-f.StartLine)*fileRange/lineRange
					}
				}
				mappings = append(mappings, Mapping{
					GeneratedLine: 0,
					GeneratedCol:  clampStructure(fnOff, 0, len(bundleSource)),
					SourceIdx:     srcIdx,
					OriginalLine:  0,
					OriginalCol:   0,
					NameIdx:       idx,
				})
			} else {
				mappings = append(mappings, Mapping{
					GeneratedLine: clampStructure(fn.StartLine, 1, totalLines) - 1,
					GeneratedCol:  0,
					SourceIdx:     srcIdx,
					OriginalLine:  clampStructure(fn.StartLine, f.StartLine, f.EndLine) - f.StartLine,
					OriginalCol:   0,
					NameIdx:       idx,
				})
			}
		}
	}

	return GenerateV3("bundle.js", sources, sourcesContent, mappings, names)
}

func extractLineRange(source string, startLine, endLine int) string {
	lines := strings.Split(source, "\n")
	start := clampStructure(startLine, 1, len(lines)) - 1
	end := clampStructure(endLine, 1, len(lines))
	if start >= end {
		return ""
	}
	return strings.Join(lines[start:end], "\n")
}

func clampStructure(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
