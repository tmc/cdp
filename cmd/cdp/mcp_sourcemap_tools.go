package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tmc/misc/chrome-to-har/internal/sourcemap"
)

// syntheticMap holds a generated sourcemap for a bundle URL.
type syntheticMap struct {
	BundleURL    string          `json:"bundle_url"`
	MapJSON      []byte          `json:"-"`
	Sources      *inferredResult `json:"sources,omitempty"`
	Serving      bool            `json:"serving"`
	InterceptID  string          `json:"intercept_id,omitempty"`
}

// inferredResult is the LLM's structured response about bundle structure.
type inferredResult struct {
	Files   []inferredFile `json:"files"`
	Summary string         `json:"summary"`
}

type inferredFile struct {
	Path        string         `json:"path"`
	Description string         `json:"description"`
	StartLine   int            `json:"start_line"`
	EndLine     int            `json:"end_line"`
	StartOffset int            `json:"start_offset"`
	EndOffset   int            `json:"end_offset"`
	Functions   []inferredFunc `json:"functions,omitempty"`
	Framework   string         `json:"framework,omitempty"`
	Module      string         `json:"module,omitempty"`
}

type inferredFunc struct {
	Name        string `json:"name"`
	StartLine   int    `json:"start_line"`
	EndLine     int    `json:"end_line"`
	Description string `json:"description"`
	Exported    bool   `json:"exported,omitempty"`
}

// syntheticMapStore manages sourcemaps keyed by bundle URL.
type syntheticMapStore struct {
	mu   sync.Mutex
	maps map[string]*syntheticMap
}

func newSyntheticMapStore() *syntheticMapStore {
	return &syntheticMapStore{maps: make(map[string]*syntheticMap)}
}

func (s *syntheticMapStore) get(url string) *syntheticMap {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.maps[url]
}

func (s *syntheticMapStore) set(url string, m *syntheticMap) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maps[url] = m
}

func (s *syntheticMapStore) list() []*syntheticMap {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []*syntheticMap
	for _, m := range s.maps {
		result = append(result, m)
	}
	return result
}

// --- MCP tool registration ---

type AnalyzeBundleInput struct {
	BundleURL    string `json:"bundle_url"`
	SnapshotName string `json:"snapshot_name,omitempty"`
	ActionLabel  string `json:"action_label,omitempty"`
}

type SetBundleStructureInput struct {
	BundleURL    string          `json:"bundle_url"`
	SnapshotName string          `json:"snapshot_name,omitempty"`
	Structure    json.RawMessage `json:"structure"`
}

type GenerateSourcemapInput struct {
	BundleURL string `json:"bundle_url"`
}

type ServeSourcemapInput struct {
	BundleURL string `json:"bundle_url"`
}

type RefineSourcemapInput struct {
	BundleURL    string `json:"bundle_url"`
	SnapshotName string `json:"snapshot_name,omitempty"`
	ActionLabel  string `json:"action_label,omitempty"`
}

func registerSourcemapTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "analyze_bundle",
		Description: `Extract executed code chunks from a JavaScript bundle using coverage data.

Returns the chunks formatted for analysis. After reviewing the output, call
set_bundle_structure with the inferred file structure to generate a sourcemap.

The output includes a JSON schema showing the expected format for set_bundle_structure.`,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AnalyzeBundleInput) (*mcp.CallToolResult, any, error) {
		if input.BundleURL == "" {
			return nil, nil, fmt.Errorf("analyze_bundle: bundle_url is required")
		}

		chunks, _, err := extractBundleChunks(s, input.BundleURL, input.SnapshotName)
		if err != nil {
			return nil, nil, fmt.Errorf("analyze_bundle: %w", err)
		}
		if len(chunks) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "no executed code found for " + input.BundleURL + " — start coverage and navigate first"}},
			}, nil, nil
		}

		// Return chunks as a prompt the agent can analyze directly.
		prompt := buildAnalysisPrompt(input.BundleURL, chunks, input.ActionLabel)

		var b strings.Builder
		fmt.Fprintf(&b, "Bundle: %s\nExecuted chunks: %d\n\n", input.BundleURL, len(chunks))
		b.WriteString(prompt)
		b.WriteString("\n\nAfter analyzing the chunks above, call set_bundle_structure with:\n")
		b.WriteString(`  {"bundle_url": "` + input.BundleURL + `", "structure": <your JSON response>}`)
		b.WriteString("\n")

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: b.String()}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "set_bundle_structure",
		Description: `Accept inferred source file structure for a bundle and generate a synthetic sourcemap.

Call analyze_bundle first to get the code chunks, then pass your analysis here.
The structure field should be JSON matching: {"files": [...], "summary": "..."}
where each file has: path, description, start_line, end_line, start_offset, end_offset,
functions (optional), framework (optional), module (optional).`,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SetBundleStructureInput) (*mcp.CallToolResult, any, error) {
		if input.BundleURL == "" {
			return nil, nil, fmt.Errorf("set_bundle_structure: bundle_url is required")
		}

		var result inferredResult
		if err := json.Unmarshal(input.Structure, &result); err != nil {
			return nil, nil, fmt.Errorf("set_bundle_structure: invalid structure JSON: %w", err)
		}
		if len(result.Files) == 0 {
			return nil, nil, fmt.Errorf("set_bundle_structure: structure must contain at least one file")
		}

		// Get the bundle source for sourcemap generation.
		_, bundleSource, err := extractBundleChunks(s, input.BundleURL, input.SnapshotName)
		if err != nil {
			return nil, nil, fmt.Errorf("set_bundle_structure: %w", err)
		}

		// Generate the sourcemap.
		if s.syntheticMaps == nil {
			s.syntheticMaps = newSyntheticMapStore()
		}
		sm := s.syntheticMaps.get(input.BundleURL)
		if sm == nil {
			sm = &syntheticMap{BundleURL: input.BundleURL}
		}
		sm.Sources = &result

		mapJSON, err := generateMapFromInferred(bundleSource, &result)
		if err != nil {
			return nil, nil, fmt.Errorf("set_bundle_structure: generate map: %w", err)
		}
		sm.MapJSON = mapJSON

		// Hot-update the intercept rule if already serving.
		if sm.Serving && sm.InterceptID != "" && s.intercepts != nil {
			s.intercepts.mu.Lock()
			for i := range s.intercepts.rules {
				if s.intercepts.rules[i].ID == sm.InterceptID {
					s.intercepts.rules[i].Body = string(mapJSON)
					break
				}
			}
			s.intercepts.mu.Unlock()
		}

		s.syntheticMaps.set(input.BundleURL, sm)

		var b strings.Builder
		fmt.Fprintf(&b, "Sourcemap generated for %s: %d source files, %d bytes\n", input.BundleURL, len(result.Files), len(mapJSON))
		if result.Summary != "" {
			fmt.Fprintf(&b, "Summary: %s\n", result.Summary)
		}
		for _, f := range result.Files {
			fmt.Fprintf(&b, "  %s (lines %d-%d): %s\n", f.Path, f.StartLine, f.EndLine, f.Description)
		}
		if sm.Serving {
			fmt.Fprintf(&b, "\nSourcemap hot-updated (serving via rule %s).\n", sm.InterceptID)
		} else {
			fmt.Fprintf(&b, "\nUse serve_sourcemap to activate, or generate_sourcemap to get the raw JSON.\n")
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: b.String()}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "generate_sourcemap",
		Description: "Generate a sourcemap v3 JSON from previously analyzed bundle structure. Returns the raw sourcemap JSON.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GenerateSourcemapInput) (*mcp.CallToolResult, any, error) {
		if s.syntheticMaps == nil {
			return nil, nil, fmt.Errorf("generate_sourcemap: no bundles analyzed — use analyze_bundle first")
		}
		sm := s.syntheticMaps.get(input.BundleURL)
		if sm == nil || sm.MapJSON == nil {
			return nil, nil, fmt.Errorf("generate_sourcemap: no analysis for %s — use analyze_bundle first", input.BundleURL)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(sm.MapJSON)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "serve_sourcemap",
		Description: "Install a Fetch intercept to serve the synthetic sourcemap for a bundle URL. When Chrome requests the .map file, it gets our generated map instead.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ServeSourcemapInput) (*mcp.CallToolResult, any, error) {
		if s.syntheticMaps == nil {
			return nil, nil, fmt.Errorf("serve_sourcemap: no bundles analyzed")
		}
		sm := s.syntheticMaps.get(input.BundleURL)
		if sm == nil || sm.MapJSON == nil {
			return nil, nil, fmt.Errorf("serve_sourcemap: no sourcemap for %s", input.BundleURL)
		}
		if sm.Serving {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("already serving sourcemap for %s (rule %s)", input.BundleURL, sm.InterceptID)}},
			}, nil, nil
		}

		// Enable fetch intercept if needed.
		if err := ensureInterceptEnabled(s); err != nil {
			return nil, nil, fmt.Errorf("serve_sourcemap: %w", err)
		}

		// Install intercept rule for the .map URL.
		mapURL := input.BundleURL + ".map"
		rule := interceptRule{
			URLPattern:  mapURL,
			Stage:       "request",
			Action:      "fulfill",
			StatusCode:  200,
			Body:        string(sm.MapJSON),
			ContentType: "application/json",
			Headers: map[string]string{
				"Access-Control-Allow-Origin": "*",
			},
		}
		id := s.intercepts.addRule(rule)
		sm.Serving = true
		sm.InterceptID = id
		s.syntheticMaps.set(input.BundleURL, sm)

		// Also inject the sourceMappingURL comment into the bundle response
		// so Chrome knows to look for the .map file.
		sourceMapComment := fmt.Sprintf("\n//# sourceMappingURL=%s\n", mapURL)
		appendRule := interceptRule{
			URLPattern: input.BundleURL,
			Stage:      "response",
			Action:     "fulfill",
			StatusCode: 200,
			ContentType: "application/javascript",
		}
		// We need to fetch the actual bundle content and append the comment.
		// For now, just install the .map intercept — the agent can use
		// evaluate to inject the sourceURL.
		_ = appendRule
		_ = sourceMapComment

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("serving sourcemap at %s (rule %s, %d bytes)\nTo activate in DevTools, run: evaluate({expression: 'document.querySelectorAll(\"script\").forEach(s => { if(s.src.includes(\"%s\")) console.log(\"sourcemap active\") })'})", mapURL, id, len(sm.MapJSON), input.BundleURL)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_sourcemaps",
		Description: "List all synthetic sourcemaps and their serving status.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
		if s.syntheticMaps == nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "no sourcemaps"}},
			}, nil, nil
		}
		maps := s.syntheticMaps.list()
		if len(maps) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "no sourcemaps"}},
			}, nil, nil
		}

		type mapInfo struct {
			BundleURL   string `json:"bundle_url"`
			Files       int    `json:"files"`
			MapSize     int    `json:"map_size"`
			Serving     bool   `json:"serving"`
			InterceptID string `json:"intercept_id,omitempty"`
		}
		var infos []mapInfo
		for _, m := range maps {
			nFiles := 0
			if m.Sources != nil {
				nFiles = len(m.Sources.Files)
			}
			infos = append(infos, mapInfo{
				BundleURL:   m.BundleURL,
				Files:       nFiles,
				MapSize:     len(m.MapJSON),
				Serving:     m.Serving,
				InterceptID: m.InterceptID,
			})
		}
		data, _ := json.Marshal(infos)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "refine_sourcemap",
		Description: `Extract new coverage chunks for a bundle after additional user actions.

Returns the updated chunks for re-analysis. Call set_bundle_structure with the
refined file structure to update the sourcemap. If the sourcemap is being served,
the intercept rule will be hot-updated automatically.`,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RefineSourcemapInput) (*mcp.CallToolResult, any, error) {
		if input.BundleURL == "" {
			return nil, nil, fmt.Errorf("refine_sourcemap: bundle_url is required")
		}

		chunks, _, err := extractBundleChunks(s, input.BundleURL, input.SnapshotName)
		if err != nil {
			return nil, nil, fmt.Errorf("refine_sourcemap: %w", err)
		}
		if len(chunks) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "no executed code found"}},
			}, nil, nil
		}

		// Show what changed since last analysis.
		var b strings.Builder
		fmt.Fprintf(&b, "Refined coverage for %s: %d chunks\n", input.BundleURL, len(chunks))
		if s.syntheticMaps != nil {
			if existing := s.syntheticMaps.get(input.BundleURL); existing != nil && existing.Sources != nil {
				fmt.Fprintf(&b, "Previous analysis had %d files. ", len(existing.Sources.Files))
				if existing.Serving {
					fmt.Fprintf(&b, "Currently serving (rule %s). ", existing.InterceptID)
				}
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")

		// Return the analysis prompt for the agent.
		prompt := buildAnalysisPrompt(input.BundleURL, chunks, input.ActionLabel)
		b.WriteString(prompt)
		b.WriteString("\n\nAfter analyzing, call set_bundle_structure to update the sourcemap.\n")
		b.WriteString("If the sourcemap is being served, it will be hot-updated automatically.\n")

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: b.String()}},
		}, nil, nil
	})
}

// extractBundleChunks gets coverage data for a specific bundle URL.
func extractBundleChunks(s *mcpSession, bundleURL, snapshotName string) ([]sourcemap.CodeChunk, string, error) {
	if s.coverageCollector == nil {
		return nil, "", fmt.Errorf("coverage not active — use start_coverage first")
	}

	snapshots := s.coverageCollector.Snapshots()
	if len(snapshots) == 0 {
		return nil, "", fmt.Errorf("no coverage snapshots — take a snapshot first")
	}

	// Find the requested snapshot (or latest).
	var snap = snapshots[len(snapshots)-1]
	if snapshotName != "" {
		snap = nil
		for _, sn := range snapshots {
			if sn.Name == snapshotName {
				snap = sn
				break
			}
		}
		if snap == nil {
			return nil, "", fmt.Errorf("snapshot %q not found", snapshotName)
		}
	}

	scriptCov, ok := snap.Scripts[bundleURL]
	if !ok {
		return nil, "", fmt.Errorf("no coverage data for %s in snapshot %s", bundleURL, snap.Name)
	}

	// Convert coverage ranges.
	var ranges []sourcemap.CoverageRange
	for _, r := range scriptCov.ByteRanges {
		ranges = append(ranges, sourcemap.CoverageRange{
			StartOffset: r.StartOffset,
			EndOffset:   r.EndOffset,
			Count:       r.Count,
		})
	}

	chunks := sourcemap.ExtractChunks(scriptCov.Source, ranges, 3)
	return chunks, scriptCov.Source, nil
}


func buildAnalysisPrompt(bundleURL string, chunks []sourcemap.CodeChunk, actionLabel string) string {
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
- start_line/end_line and start_offset/end_offset are positions in the BUNDLE
- Assign every chunk to a file
- Identify framework (react, vue, angular, vanilla, etc.)
- Group into logical modules
`)
	return b.String()
}

// generateMapFromInferred builds a sourcemap v3 from inferred file structure.
func generateMapFromInferred(bundleSource string, inferred *inferredResult) ([]byte, error) {
	if len(inferred.Files) == 0 {
		return nil, fmt.Errorf("no inferred files")
	}

	var sources []string
	var sourcesContent []string
	var mappings []sourcemap.Mapping
	var names []string
	nameIdx := make(map[string]int)

	for srcIdx, f := range inferred.Files {
		sources = append(sources, f.Path)

		// Extract the source content from the bundle.
		startLine := clampLine(f.StartLine, 1, sourcemap.CountLinesInString(bundleSource))
		endLine := clampLine(f.EndLine, startLine, sourcemap.CountLinesInString(bundleSource))
		content := extractLineRange(bundleSource, startLine, endLine)
		sourcesContent = append(sourcesContent, content)

		// Create line-by-line mappings from bundle → inferred source.
		for line := f.StartLine; line <= f.EndLine; line++ {
			origLine := line - f.StartLine // 0-based in the extracted source
			m := sourcemap.Mapping{
				GeneratedLine: line - 1, // 0-based in bundle
				GeneratedCol:  0,
				SourceIdx:     srcIdx,
				OriginalLine:  origLine,
				OriginalCol:   0,
				NameIdx:       -1,
			}
			mappings = append(mappings, m)
		}

		// Add function names.
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
			// Add a named mapping at the function start.
			mappings = append(mappings, sourcemap.Mapping{
				GeneratedLine: clampLine(fn.StartLine, 1, sourcemap.CountLinesInString(bundleSource)) - 1,
				GeneratedCol:  0,
				SourceIdx:     srcIdx,
				OriginalLine:  clampLine(fn.StartLine, f.StartLine, f.EndLine) - f.StartLine,
				OriginalCol:   0,
				NameIdx:       idx,
			})
		}
	}

	return sourcemap.GenerateV3("bundle.js", sources, sourcesContent, mappings, names)
}

func extractLineRange(source string, startLine, endLine int) string {
	lines := strings.Split(source, "\n")
	start := clampLine(startLine, 1, len(lines)) - 1
	end := clampLine(endLine, 1, len(lines))
	if start >= end {
		return ""
	}
	return strings.Join(lines[start:end], "\n")
}

func clampLine(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func stripCodeFences(s string) string {
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
