package sourcemap

import (
	"fmt"
	"strings"
)

// FunctionCoverage is the function-level coverage data used for bundle
// analysis prompts.
type FunctionCoverage struct {
	Name      string
	StartLine int
	EndLine   int
	HitCount  int
	Ranges    []CoverageRange
}

// FunctionPromptData is the bundle source and V8 function coverage used to
// infer original source structure.
type FunctionPromptData struct {
	Source    string
	Functions []FunctionCoverage
}

// FunctionAnalysisPrompt creates a prompt using V8 per-function coverage data.
func FunctionAnalysisPrompt(bundleURL string, data FunctionPromptData, actionLabel string) string {
	var b strings.Builder
	b.WriteString("Analyze this JavaScript bundle to infer original source files.\n")
	b.WriteString("V8 has identified individual functions with precise byte ranges.\n\n")
	fmt.Fprintf(&b, "Bundle URL: %s\n", bundleURL)
	fmt.Fprintf(&b, "Bundle size: %d bytes\n", len(data.Source))
	if actionLabel != "" {
		fmt.Fprintf(&b, "Action context: %s\n", actionLabel)
	}

	executedFns := 0
	for _, fn := range data.Functions {
		if fn.HitCount > 0 {
			executedFns++
		}
	}
	fmt.Fprintf(&b, "Total functions: %d (%d executed)\n\n", len(data.Functions), executedFns)

	b.WriteString("=== EXECUTED FUNCTIONS ===\n\n")
	shown := 0
	for _, fn := range data.Functions {
		if fn.HitCount == 0 {
			continue
		}
		if shown >= 50 {
			fmt.Fprintf(&b, "... and %d more executed functions (truncated)\n", executedFns-50)
			break
		}
		shown++

		name := fn.Name
		if name == "" {
			name = "(anonymous)"
		}

		funcSource := ""
		if len(fn.Ranges) > 0 {
			start := fn.Ranges[0].StartOffset
			end := fn.Ranges[0].EndOffset
			if start >= 0 && end <= len(data.Source) && start < end {
				funcSource = data.Source[start:end]
			}
		}

		fmt.Fprintf(&b, "--- Function: %s (lines %d-%d, %d hits) ---\n",
			name, fn.StartLine, fn.EndLine, fn.HitCount)
		if len(fn.Ranges) > 0 {
			fmt.Fprintf(&b, "    byte range: %d-%d\n", fn.Ranges[0].StartOffset, fn.Ranges[0].EndOffset)
		}
		if funcSource != "" {
			if len(funcSource) > 1500 {
				funcSource = funcSource[:1500] + "\n// ... truncated"
			}
			b.WriteString(funcSource)
			b.WriteString("\n\n")
		}
	}

	unexecuted := 0
	for _, fn := range data.Functions {
		if fn.HitCount == 0 {
			unexecuted++
		}
	}
	if unexecuted > 0 {
		fmt.Fprintf(&b, "\n=== NON-EXECUTED FUNCTIONS (%d) ===\n", unexecuted)
		shown = 0
		for _, fn := range data.Functions {
			if fn.HitCount != 0 {
				continue
			}
			if shown >= 20 {
				fmt.Fprintf(&b, "... and %d more\n", unexecuted-20)
				break
			}
			shown++
			name := fn.Name
			if name == "" {
				name = "(anonymous)"
			}
			if len(fn.Ranges) > 0 {
				fmt.Fprintf(&b, "  %s (bytes %d-%d)\n", name, fn.Ranges[0].StartOffset, fn.Ranges[0].EndOffset)
			} else {
				fmt.Fprintf(&b, "  %s\n", name)
			}
		}
	}

	b.WriteString(`

Respond with JSON:
{
  "files": [
    {
      "path": "src/router.js",
      "description": "Client-side router",
      "start_offset": 0,
      "end_offset": 5000,
      "start_line": 1,
      "end_line": 1,
      "functions": [
        {"name": "navigate", "start_line": 1, "end_line": 1, "description": "Navigate to path", "exported": true}
      ],
      "framework": "next",
      "module": "routing"
    }
  ],
  "summary": "Brief description"
}

Rules:
- Group functions into inferred source files based on naming, call patterns, and co-activation
- start_offset/end_offset are BYTE positions in the bundle (critical for minified single-line code)
- For webpack/turbopack: module IDs in require() calls hint at module boundaries
- Export names (Object.defineProperty patterns) often match original file/function names
- Cluster co-activated functions (similar hit counts) into the same module
`)
	return b.String()
}

// ChunkAnalysisPrompt creates a prompt from executed code chunks.
func ChunkAnalysisPrompt(bundleURL string, chunks []CodeChunk, actionLabel string) string {
	var b strings.Builder
	b.WriteString("Analyze this bundled/minified JavaScript to infer original source files.\n\n")
	fmt.Fprintf(&b, "Bundle URL: %s\n", bundleURL)
	if actionLabel != "" {
		fmt.Fprintf(&b, "Action that triggered this code: %s\n", actionLabel)
	}
	fmt.Fprintf(&b, "Executed chunks: %d\n\n", len(chunks))

	for i, c := range chunks {
		if i >= 30 {
			fmt.Fprintf(&b, "\n... and %d more chunks (truncated)\n", len(chunks)-30)
			break
		}
		fmt.Fprintf(&b, "=== Chunk %d (bytes %d-%d, lines %d-%d, hits %d) ===\n",
			i+1, c.StartOffset, c.EndOffset, c.StartLine, c.EndLine, c.HitCount)
		code := c.Code
		if len(code) > 2000 {
			code = code[:2000] + "\n// ... truncated"
		}
		b.WriteString(code)
		b.WriteString("\n\n")
	}

	b.WriteString(`Respond with JSON:
{
  "files": [
    {
      "path": "src/components/Login.tsx",
      "description": "Login form component",
      "start_line": 1,
      "end_line": 45,
      "start_offset": 0,
      "end_offset": 1234,
      "functions": [
        {"name": "handleSubmit", "start_line": 10, "end_line": 25, "description": "Form handler", "exported": false}
      ],
      "framework": "react",
      "module": "auth"
    }
  ],
  "summary": "Brief description of the bundle contents"
}

Rules:
- Infer realistic paths based on code patterns (src/..., lib/..., etc.)
- start_offset/end_offset are BYTE positions in the bundle (critical for minified single-line bundles)
- start_line/end_line are line positions (may all be line 1 for minified code — use byte offsets instead)
- Assign every chunk to a file
- Identify framework (react, vue, angular, vanilla, etc.)
- Group into logical modules
- For webpack bundles: look for moduleId:(e,t,r)=>{...} patterns and use module IDs as grouping
`)
	return b.String()
}

// StripCodeFences removes markdown code fences around a JSON response.
func StripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```json") {
		s = s[7:]
	} else if strings.HasPrefix(s, "```") {
		s = s[3:]
	}
	if strings.HasSuffix(s, "```") {
		s = s[:len(s)-3]
	}
	return strings.TrimSpace(s)
}
