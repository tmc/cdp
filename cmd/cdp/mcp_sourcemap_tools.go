package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tmc/cdp/internal/chromedp"
	"github.com/tmc/cdp/internal/sourcemap"
)

type inferredResult = sourcemap.Structure
type inferredFile = sourcemap.File
type inferredFunc = sourcemap.Function

// sourcemapDiskPath returns the on-disk path for a bundle URL's .map file.
// The map is saved next to its bundle when sources saved one there.
func sourcemapDiskPath(sourcesDir, bundleURL string) string {
	return sourcemap.DiskPath(sourcesDir, bundleURL)
}

// writeSourcemapToDisk writes a .map file alongside the saved source.
// Returns the path written, or empty string if sourcesDir is not configured.
func writeSourcemapToDisk(sourcesDir, bundleURL string, mapJSON []byte) string {
	path, err := sourcemap.WriteMap(sourcesDir, bundleURL, mapJSON)
	if err != nil {
		log.Printf("sourcemap: %v", err)
		return ""
	}
	return path
}

// writeStructureSidecar writes the inferred file structure as a JSON sidecar
// next to the .map file, so it can be reloaded on startup.
func writeStructureSidecar(mapPath string, sources *inferredResult) {
	if err := sourcemap.WriteStructureSidecar(mapPath, sources); err != nil {
		log.Printf("sourcemap: %v", err)
	}
}

func appendAnalysisLog(mapPath, bundleURL, contextName string, result *inferredResult, bundleSource string, isRefinement bool) {
	if err := sourcemap.AppendAnalysisLog(mapPath, bundleURL, contextName, result, bundleSource, isRefinement); err != nil {
		log.Printf("analysis log: %v", err)
	}
}

// activateSourcemap makes Chrome DevTools aware of a synthetic sourcemap by:
//  1. Installing a response-stage Fetch intercept on the bundle URL that appends
//     a //# sourceMappingURL comment and a SourceMap HTTP header.
//  2. Triggering Page.reload() so scripts re-fetch through the intercept.
//  3. DevTools sees the comment/header, requests the .map URL, our existing
//     request-stage intercept serves the synthetic sourcemap.
//
// Returns a human-readable status message.
func activateSourcemap(s *mcpSession, bundleURL, mapURL string) string {
	var messages []string

	// Install response intercept on the bundle URL to append sourceMappingURL.
	if s.intercepts != nil {
		bundleRule := interceptRule{
			URLPattern: bundleURL,
			Stage:      "response",
			Action:     "append-sourcemap",
			Body:       "\n//# sourceMappingURL=" + mapURL + "\n",
			Headers: map[string]string{
				"SourceMap": mapURL,
			},
		}
		bundleRuleID := s.intercepts.addRule(bundleRule)
		messages = append(messages, fmt.Sprintf("bundle intercept installed (rule %s)", bundleRuleID))
	}

	// Trigger page reload so scripts re-fetch through our intercepts.
	actx := s.activeCtx()
	if actx == nil {
		messages = append(messages, "reload skipped: no active browser context")
	} else if err := chromedp.Run(actx, chromedp.Reload()); err != nil {
		messages = append(messages, fmt.Sprintf("reload failed: %v", err))
	} else {
		messages = append(messages, "page reloaded — DevTools Sources panel should show inferred original files")
	}

	return strings.Join(messages, "; ")
}

// --- MCP tool registration ---

type AnalyzeBundleInput struct {
	BundleURL    string `json:"bundle_url"`
	SnapshotName string `json:"snapshot_name,omitempty"`
	ActionLabel  string `json:"action_label,omitempty"`
}

type SetBundleStructureInput struct {
	BundleURL    string `json:"bundle_url"`
	SnapshotName string `json:"snapshot_name,omitempty"`
	Structure    string `json:"structure"`
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
	addMCPTool(server, &mcp.Tool{
		Name: "analyze_bundle",
		Description: `Analyze a JavaScript bundle using coverage data to infer its original source structure.

If the MCP client supports sampling, the analysis happens automatically via CreateMessage.
Otherwise, returns extracted code chunks for you to analyze manually, then call
set_bundle_structure with the inferred file structure.`,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AnalyzeBundleInput) (*mcp.CallToolResult, any, error) {
		if input.BundleURL == "" {
			return nil, nil, fmt.Errorf("analyze_bundle: bundle_url is required")
		}

		data, err := extractBundleCoverage(s, input.BundleURL, input.SnapshotName)
		if err != nil {
			return nil, nil, fmt.Errorf("analyze_bundle: %w", err)
		}
		if len(data.Chunks) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "no executed code found for " + input.BundleURL + " — start coverage and navigate first"}},
			}, nil, nil
		}

		// Try MCP sampling first — works with clients that support CreateMessage.
		result, samplingErr := sampleBundleAnalysis(ctx, req.Session, input.BundleURL, data.Chunks, input.ActionLabel)
		if samplingErr == nil && result != nil {
			mapJSON, err := sourcemap.GenerateFromStructure(data.Source, result)
			if err != nil {
				return nil, nil, fmt.Errorf("analyze_bundle: generate map: %w", err)
			}
			s.ensureSourcemaps().update(input.BundleURL, func(sm *syntheticMap) {
				sm.Sources = result
				sm.MapJSON = mapJSON
			})

			var b strings.Builder
			fmt.Fprintf(&b, "Analyzed %s: %d inferred source files\n", input.BundleURL, len(result.Files))
			if result.Summary != "" {
				fmt.Fprintf(&b, "Summary: %s\n", result.Summary)
			}
			for _, f := range result.Files {
				fmt.Fprintf(&b, "  %s (lines %d-%d): %s\n", f.Path, f.StartLine, f.EndLine, f.Description)
			}
			fmt.Fprintf(&b, "\nSourcemap generated (%d bytes). Use serve_sourcemap to activate.\n", len(mapJSON))
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: b.String()}},
			}, nil, nil
		}

		// Sampling not available — return function-level coverage for agent analysis.
		prompt := sourcemap.FunctionAnalysisPrompt(input.BundleURL, sourcemap.FunctionPromptData{
			Source:    data.Source,
			Functions: data.Functions,
		}, input.ActionLabel)

		var b strings.Builder
		fmt.Fprintf(&b, "Bundle: %s (%d bytes, %d functions, %d executed chunks)\n",
			input.BundleURL, len(data.Source), len(data.Functions), len(data.Chunks))
		fmt.Fprintf(&b, "(MCP sampling unavailable: %v)\n\n", samplingErr)
		b.WriteString(prompt)
		b.WriteString("\n\nAfter analyzing, call set_bundle_structure with:\n")
		b.WriteString(`  {"bundle_url": "` + input.BundleURL + `", "structure": <your JSON response>}`)
		b.WriteString("\n")

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: b.String()}},
		}, nil, nil
	})

	addMCPTool(server, &mcp.Tool{
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
		structJSON := sourcemap.StripCodeFences(input.Structure)
		if err := json.Unmarshal([]byte(structJSON), &result); err != nil {
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

		maps := s.ensureSourcemaps()
		sm := maps.get(input.BundleURL)
		serving := sm != nil && sm.Serving
		interceptID := ""
		if sm != nil {
			interceptID = sm.InterceptID
		}
		isRefinement := sm != nil && sm.Sources != nil && len(sm.Sources.Files) > 0

		mapJSON, err := sourcemap.GenerateFromStructure(bundleSource, &result)
		if err != nil {
			return nil, nil, fmt.Errorf("set_bundle_structure: generate map: %w", err)
		}

		// Write to disk alongside saved sources.
		sourcesDir := ""
		if s.sourceCollector != nil {
			sourcesDir = s.sourceCollector.OutputDir()
		}
		mapPath := ""
		if path := writeSourcemapToDisk(sourcesDir, input.BundleURL, mapJSON); path != "" {
			mapPath = path
			writeStructureSidecar(path, &result)
			appendAnalysisLog(path, input.BundleURL, s.contextPath(), &result, bundleSource, isRefinement)
		}

		sm = maps.update(input.BundleURL, func(sm *syntheticMap) {
			sm.Sources = &result
			sm.MapJSON = mapJSON
			if mapPath != "" {
				sm.MapPath = mapPath
			}
		})

		// Hot-update the intercept rule if already serving.
		if serving && interceptID != "" && s.intercepts != nil {
			s.intercepts.mu.Lock()
			for i := range s.intercepts.rules {
				if s.intercepts.rules[i].ID == interceptID {
					s.intercepts.rules[i].Body = string(mapJSON)
					break
				}
			}
			s.intercepts.mu.Unlock()
		}

		var b strings.Builder
		fmt.Fprintf(&b, "Sourcemap generated for %s: %d source files, %d bytes\n", input.BundleURL, len(result.Files), len(mapJSON))
		if result.Summary != "" {
			fmt.Fprintf(&b, "Summary: %s\n", result.Summary)
		}
		for _, f := range result.Files {
			fmt.Fprintf(&b, "  %s (lines %d-%d): %s\n", f.Path, f.StartLine, f.EndLine, f.Description)
		}
		if sm.MapPath != "" {
			fmt.Fprintf(&b, "\nWritten to %s\n", sm.MapPath)
		}
		if serving {
			fmt.Fprintf(&b, "Sourcemap hot-updated (serving via rule %s).\n", interceptID)
		} else {
			fmt.Fprintf(&b, "Use serve_sourcemap to activate, or generate_sourcemap to get the raw JSON.\n")
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: b.String()}},
		}, nil, nil
	})

	addMCPTool(server, &mcp.Tool{
		Name:        "generate_sourcemap",
		Description: "Generate a sourcemap v3 JSON from previously analyzed bundle structure. Returns the raw sourcemap JSON.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GenerateSourcemapInput) (*mcp.CallToolResult, any, error) {
		synth := s.sourcemaps()
		if synth == nil {
			return nil, nil, fmt.Errorf("generate_sourcemap: no bundles analyzed — use analyze_bundle first")
		}
		sm := synth.get(input.BundleURL)
		if sm == nil || sm.MapJSON == nil {
			return nil, nil, fmt.Errorf("generate_sourcemap: no analysis for %s — use analyze_bundle first", input.BundleURL)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(sm.MapJSON)}},
		}, nil, nil
	})

	addMCPTool(server, &mcp.Tool{
		Name:        "serve_sourcemap",
		Description: "Install a Fetch intercept to serve the synthetic sourcemap for a bundle URL. When Chrome requests the .map file, it gets our generated map instead.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ServeSourcemapInput) (*mcp.CallToolResult, any, error) {
		synth := s.sourcemaps()
		if synth == nil {
			return nil, nil, fmt.Errorf("serve_sourcemap: no bundles analyzed")
		}
		sm := synth.get(input.BundleURL)
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
		sm = synth.update(input.BundleURL, func(sm *syntheticMap) {
			sm.Serving = true
			sm.InterceptID = id
		})

		// Activate: install bundle response intercept + reload page.
		activateMsg := activateSourcemap(s, input.BundleURL, mapURL)

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("serving sourcemap at %s (rule %s, %d bytes)\n%s", mapURL, id, len(sm.MapJSON), activateMsg)}},
		}, nil, nil
	})

	addMCPTool(server, &mcp.Tool{
		Name:        "list_sourcemaps",
		Description: "List all synthetic sourcemaps and their serving status.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
		synth := s.sourcemaps()
		if synth == nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "no sourcemaps"}},
			}, nil, nil
		}
		maps := synth.list()
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
			MapPath     string `json:"map_path,omitempty"`
			LogEntries  int    `json:"log_entries"`
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
				MapPath:     m.MapPath,
				LogEntries:  sourcemap.CountAnalysisLogEntries(m.MapPath),
			})
		}
		data, _ := json.Marshal(infos)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	addMCPTool(server, &mcp.Tool{
		Name: "refine_sourcemap",
		Description: `Re-analyze a bundle with additional coverage data (e.g. after more user actions) and update the sourcemap.

If MCP sampling is available, the analysis and update happen automatically.
Otherwise, returns new chunks for you to re-analyze, then call set_bundle_structure.`,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RefineSourcemapInput) (*mcp.CallToolResult, any, error) {
		if input.BundleURL == "" {
			return nil, nil, fmt.Errorf("refine_sourcemap: bundle_url is required")
		}

		chunks, bundleSource, err := extractBundleChunks(s, input.BundleURL, input.SnapshotName)
		if err != nil {
			return nil, nil, fmt.Errorf("refine_sourcemap: %w", err)
		}
		if len(chunks) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "no executed code found"}},
			}, nil, nil
		}

		// Try MCP sampling first.
		result, samplingErr := sampleBundleAnalysis(ctx, req.Session, input.BundleURL, chunks, input.ActionLabel)
		if samplingErr == nil && result != nil {
			mapJSON, err := sourcemap.GenerateFromStructure(bundleSource, result)
			if err != nil {
				return nil, nil, fmt.Errorf("refine_sourcemap: generate map: %w", err)
			}
			maps := s.ensureSourcemaps()
			sm := maps.get(input.BundleURL)
			serving := sm != nil && sm.Serving
			interceptID := ""
			if sm != nil {
				interceptID = sm.InterceptID
			}

			// Hot-update if serving.
			if serving && interceptID != "" && s.intercepts != nil {
				s.intercepts.mu.Lock()
				for i := range s.intercepts.rules {
					if s.intercepts.rules[i].ID == interceptID {
						s.intercepts.rules[i].Body = string(mapJSON)
						break
					}
				}
				s.intercepts.mu.Unlock()
			}
			maps.update(input.BundleURL, func(sm *syntheticMap) {
				sm.Sources = result
				sm.MapJSON = mapJSON
			})

			status := "generated"
			if serving {
				status = "hot-updated"
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Refined sourcemap %s for %s: %d files, %d bytes\n%s", status, input.BundleURL, len(result.Files), len(mapJSON), result.Summary)}},
			}, nil, nil
		}

		// Sampling not available — return chunks for agent analysis.
		var b strings.Builder
		fmt.Fprintf(&b, "Refined coverage for %s: %d chunks\n", input.BundleURL, len(chunks))
		fmt.Fprintf(&b, "(MCP sampling unavailable: %v)\n", samplingErr)
		if synth := s.sourcemaps(); synth != nil {
			if existing := synth.get(input.BundleURL); existing != nil && existing.Sources != nil {
				fmt.Fprintf(&b, "Previous analysis had %d files. ", len(existing.Sources.Files))
				if existing.Serving {
					fmt.Fprintf(&b, "Currently serving (rule %s). ", existing.InterceptID)
				}
				b.WriteString("\n")
				// Include prior reasoning from analysis log.
				if existing.MapPath != "" {
					if entries, err := sourcemap.ReadAnalysisLog(existing.MapPath); err == nil && len(entries) > 0 {
						last := entries[len(entries)-1]
						fmt.Fprintf(&b, "\nPrior analysis (%s):\n", last.Timestamp)
						for _, f := range last.Files {
							fmt.Fprintf(&b, "  %s (bytes %d-%d): %s\n", f.Path, f.StartOffset, f.EndOffset, f.Reasoning)
						}
						b.WriteString("\n")
					}
				}
			}
		}
		b.WriteString("\n")

		prompt := sourcemap.ChunkAnalysisPrompt(input.BundleURL, chunks, input.ActionLabel)
		b.WriteString(prompt)
		b.WriteString("\n\nAfter analyzing, call set_bundle_structure to update the sourcemap.\n")
		b.WriteString("If the sourcemap is being served, it will be hot-updated automatically.\n")

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: b.String()}},
		}, nil, nil
	})

	type GetAnalysisLogInput struct {
		BundleURL string `json:"bundle_url"`
		Last      int    `json:"last,omitempty"`
	}

	addMCPTool(server, &mcp.Tool{
		Name:        "get_analysis_log",
		Description: "Read the analysis log for a bundle — shows prior reasoning behind sourcemap naming decisions across sessions.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetAnalysisLogInput) (*mcp.CallToolResult, any, error) {
		synth := s.sourcemaps()
		if synth == nil {
			return nil, nil, fmt.Errorf("get_analysis_log: no sourcemaps")
		}
		sm := synth.get(input.BundleURL)
		if sm == nil || sm.MapPath == "" {
			return nil, nil, fmt.Errorf("get_analysis_log: no on-disk sourcemap for %s", input.BundleURL)
		}
		entries, err := sourcemap.ReadAnalysisLog(sm.MapPath)
		if err != nil {
			return nil, nil, fmt.Errorf("get_analysis_log: %w", err)
		}
		if len(entries) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "no analysis log entries for " + input.BundleURL}},
			}, nil, nil
		}
		if input.Last > 0 && input.Last < len(entries) {
			entries = entries[len(entries)-input.Last:]
		}
		data, _ := json.MarshalIndent(entries, "", "  ")
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})
}

// bundleCoverageData holds coverage data for a single bundle script.
type bundleCoverageData struct {
	Source    string
	Chunks    []sourcemap.CodeChunk
	Functions []sourcemap.FunctionCoverage
}

// extractBundleChunks gets coverage data for a specific bundle URL.
func extractBundleChunks(s *mcpSession, bundleURL, snapshotName string) ([]sourcemap.CodeChunk, string, error) {
	data, err := extractBundleCoverage(s, bundleURL, snapshotName)
	if err != nil {
		return nil, "", err
	}
	return data.Chunks, data.Source, nil
}

// extractBundleCoverage gets full coverage data including per-function entries.
func extractBundleCoverage(s *mcpSession, bundleURL, snapshotName string) (*bundleCoverageData, error) {
	if s.coverageCollector == nil {
		return nil, fmt.Errorf("coverage not active — use start_coverage first")
	}

	snapshots := s.coverageCollector.Snapshots()
	if len(snapshots) == 0 {
		return nil, fmt.Errorf("no coverage snapshots — take a snapshot first")
	}

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
			return nil, fmt.Errorf("snapshot %q not found", snapshotName)
		}
	}

	scriptCov, ok := snap.Scripts[bundleURL]
	if !ok {
		return nil, fmt.Errorf("no coverage data for %s in snapshot %s", bundleURL, snap.Name)
	}

	var ranges []sourcemap.CoverageRange
	for _, r := range scriptCov.ByteRanges {
		ranges = append(ranges, sourcemap.CoverageRange{
			StartOffset: r.StartOffset,
			EndOffset:   r.EndOffset,
			Count:       r.Count,
		})
	}

	var functions []sourcemap.FunctionCoverage
	for _, fn := range scriptCov.Functions {
		var fnRanges []sourcemap.CoverageRange
		for _, r := range fn.Ranges {
			fnRanges = append(fnRanges, sourcemap.CoverageRange{
				StartOffset: r.StartOffset,
				EndOffset:   r.EndOffset,
				Count:       r.Count,
			})
		}
		functions = append(functions, sourcemap.FunctionCoverage{
			Name:      fn.Name,
			StartLine: fn.StartLine,
			EndLine:   fn.EndLine,
			HitCount:  fn.HitCount,
			Ranges:    fnRanges,
		})
	}

	chunks := sourcemap.ExtractChunks(scriptCov.Source, ranges, 3)
	return &bundleCoverageData{
		Source:    scriptCov.Source,
		Chunks:    chunks,
		Functions: functions,
	}, nil
}

// sampleBundleAnalysis uses MCP sampling to ask the connected LLM to analyze
// code chunks. Returns an error if the client doesn't support sampling.
func sampleBundleAnalysis(ctx context.Context, session *mcp.ServerSession, bundleURL string, chunks []sourcemap.CodeChunk, actionLabel string) (*inferredResult, error) {
	if session == nil {
		return nil, fmt.Errorf("no MCP session")
	}

	prompt := sourcemap.ChunkAnalysisPrompt(bundleURL, chunks, actionLabel)

	result, err := session.CreateMessage(ctx, &mcp.CreateMessageParams{
		Messages: []*mcp.SamplingMessage{
			{
				Content: &mcp.TextContent{Text: prompt},
				Role:    "user",
			},
		},
		SystemPrompt: "You are a JavaScript bundle analyzer. Respond with valid JSON only, no markdown fences.",
		MaxTokens:    8192,
		Temperature:  0.2,
	})
	if err != nil {
		return nil, fmt.Errorf("sampling: %w", err)
	}

	text := ""
	if tc, ok := result.Content.(*mcp.TextContent); ok {
		text = tc.Text
	}
	if text == "" {
		return nil, fmt.Errorf("empty sampling response")
	}

	text = sourcemap.StripCodeFences(text)

	var inferred inferredResult
	if err := json.Unmarshal([]byte(text), &inferred); err != nil {
		return nil, fmt.Errorf("parse sampling response: %w\nraw: %.500s", err, text)
	}
	return &inferred, nil
}
