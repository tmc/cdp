package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tmc/misc/chrome-to-har/internal/sourcemap"
)

// --- Sourcemap types ---

type inferredFile struct {
	Path        string         `json:"path"`
	Description string         `json:"description,omitempty"`
	StartLine   int            `json:"start_line"`
	EndLine     int            `json:"end_line"`
	StartOffset int            `json:"start_offset"`
	EndOffset   int            `json:"end_offset"`
	Functions   []inferredFunc `json:"functions,omitempty"`
	Framework   string         `json:"framework,omitempty"`
	Module      string         `json:"module,omitempty"`
}

type inferredFunc struct {
	Name      string `json:"name"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

type inferredResult struct {
	Files   []inferredFile `json:"files"`
	Summary string         `json:"summary,omitempty"`
}

type bundleState struct {
	ScriptID  string          `json:"script_id"`
	URL       string          `json:"url"`
	Source    string          `json:"-"`
	Structure *inferredResult `json:"structure,omitempty"`
	MapJSON   []byte          `json:"map_json,omitempty"`
	MapPath   string          `json:"map_path,omitempty"`
}

type bundleStore struct {
	mu      sync.Mutex
	bundles map[string]*bundleState // keyed by URL or scriptID
}

func newBundleStore() *bundleStore {
	return &bundleStore{bundles: make(map[string]*bundleState)}
}

func (bs *bundleStore) get(key string) *bundleState {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	return bs.bundles[key]
}

func (bs *bundleStore) set(key string, b *bundleState) {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	bs.bundles[key] = b
}

// --- Input types ---

type AnalyzeBundleInput struct {
	ScriptID string `json:"script_id,omitempty"`
	URL      string `json:"url,omitempty"`
}

type SetBundleStructureInput struct {
	URL       string `json:"url"`
	Structure string `json:"structure"` // JSON string
}

type GenerateSourcemapInput struct {
	URL string `json:"url"`
}

type SaveSourcesWithMapsInput struct {
	OutputDir string `json:"output_dir,omitempty"`
}

func registerSourcemapTools(server *mcp.Server, s *ndpSession) {
	if s.bundles == nil {
		s.bundles = newBundleStore()
	}

	mcp.AddTool(server, &mcp.Tool{
		Name: "analyze_bundle",
		Description: `Analyze a bundled JavaScript source to identify file boundaries.
Provide script_id or url (from list_sources). Returns code chunks with byte ranges
and a prompt for you to identify the original files in the bundle.`,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AnalyzeBundleInput) (*mcp.CallToolResult, any, error) {
		scriptID, url, err := resolveScript(s, input.ScriptID, input.URL)
		if err != nil {
			return nil, nil, fmt.Errorf("analyze_bundle: %w", err)
		}

		source, err := s.debugger.GetScriptSource(scriptID)
		if err != nil {
			return nil, nil, fmt.Errorf("analyze_bundle: get source: %w", err)
		}

		// Extract chunks from coverage data if available.
		var chunks []sourcemap.CodeChunk
		if s.coverageCollector != nil && s.coverageCollector.Running() {
			snaps := s.coverageCollector.Snapshots()
			if len(snaps) > 0 {
				snap := snaps[len(snaps)-1]
				if cov, ok := snap.Scripts[url]; ok {
					var ranges []sourcemap.CoverageRange
					for _, br := range cov.ByteRanges {
						ranges = append(ranges, sourcemap.CoverageRange{
							StartOffset: br.StartOffset,
							EndOffset:   br.EndOffset,
							Count:       br.Count,
						})
					}
					chunks = sourcemap.ExtractChunks(source, ranges, 2)
				}
			}
		}

		// Fall back to function-based splitting if no coverage chunks.
		if len(chunks) == 0 {
			v8Cov, _ := s.profiler.TakePreciseCoverage()
			for _, sc := range v8Cov {
				if sc.ScriptID != scriptID {
					continue
				}
				var funcs []sourcemap.FunctionRange
				for _, fn := range sc.Functions {
					if len(fn.Ranges) == 0 {
						continue
					}
					funcs = append(funcs, sourcemap.FunctionRange{
						Name:        fn.FunctionName,
						StartOffset: fn.Ranges[0].StartOffset,
						EndOffset:   fn.Ranges[0].EndOffset,
					})
				}
				chunks = sourcemap.SplitFunctions(source, funcs)
				break
			}
		}

		// Store state for set_bundle_structure.
		bs := &bundleState{
			ScriptID: scriptID,
			URL:      url,
			Source:   source,
		}
		s.bundles.set(url, bs)

		// Build analysis prompt.
		totalLines := sourcemap.CountLinesInString(source)
		var sb strings.Builder
		fmt.Fprintf(&sb, "Bundle: %s (%d lines, %d bytes)\n", url, totalLines, len(source))
		fmt.Fprintf(&sb, "Chunks: %d\n\n", len(chunks))

		for i, c := range chunks {
			preview := c.Code
			if len(preview) > 500 {
				preview = preview[:500] + "..."
			}
			fmt.Fprintf(&sb, "--- Chunk %d (lines %d-%d, bytes %d-%d, hits=%d) ---\n",
				i+1, c.StartLine, c.EndLine, c.StartOffset, c.EndOffset, c.HitCount)
			fmt.Fprintf(&sb, "%s\n\n", preview)
		}

		sb.WriteString("Identify the original source files in this bundle. For each file, provide:\n")
		sb.WriteString(`{"files": [{"path": "src/...", "start_line": N, "end_line": N, "start_offset": N, "end_offset": N}]}`)
		sb.WriteString("\nThen call set_bundle_structure with the URL and structure JSON.")

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: sb.String()}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_bundle_structure",
		Description: "Accept inferred source file structure for a bundle and generate a synthetic sourcemap. Structure is a JSON string with {files: [{path, start_line, end_line, start_offset, end_offset}]}.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SetBundleStructureInput) (*mcp.CallToolResult, any, error) {
		var result inferredResult
		if err := json.Unmarshal([]byte(input.Structure), &result); err != nil {
			return nil, nil, fmt.Errorf("set_bundle_structure: parse structure: %w", err)
		}

		bs := s.bundles.get(input.URL)
		if bs == nil {
			return nil, nil, fmt.Errorf("set_bundle_structure: call analyze_bundle first for %q", input.URL)
		}
		if bs.Source == "" {
			src, err := s.debugger.GetScriptSource(bs.ScriptID)
			if err != nil {
				return nil, nil, fmt.Errorf("set_bundle_structure: get source: %w", err)
			}
			bs.Source = src
		}

		mapJSON, err := generateMapFromInferred(bs.Source, &result)
		if err != nil {
			return nil, nil, fmt.Errorf("set_bundle_structure: generate map: %w", err)
		}

		bs.Structure = &result
		bs.MapJSON = mapJSON

		// Write to disk.
		path, err := writeMapToDisk(input.URL, mapJSON)
		if err != nil {
			return nil, nil, fmt.Errorf("set_bundle_structure: write: %w", err)
		}
		bs.MapPath = path
		s.bundles.set(input.URL, bs)

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(
				"sourcemap generated: %d files, written to %s", len(result.Files), path)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "generate_sourcemap",
		Description: "Return the raw sourcemap v3 JSON for a previously analyzed bundle.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GenerateSourcemapInput) (*mcp.CallToolResult, any, error) {
		bs := s.bundles.get(input.URL)
		if bs == nil || len(bs.MapJSON) == 0 {
			return nil, nil, fmt.Errorf("generate_sourcemap: no sourcemap for %q — call analyze_bundle + set_bundle_structure first", input.URL)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(bs.MapJSON)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "save_sources_with_maps",
		Description: `Save all loaded scripts to disk, splitting bundled code using stored sourcemaps.
Output goes to output_dir (default: ~/.cdp/ndp-sources/). Bundled scripts are split into
individual files per the bundle structure.`,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SaveSourcesWithMapsInput) (*mcp.CallToolResult, any, error) {
		outDir := input.OutputDir
		if outDir == "" {
			home, _ := os.UserHomeDir()
			outDir = filepath.Join(home, ".cdp", "ndp-sources")
		}
		if err := os.MkdirAll(outDir, 0755); err != nil {
			return nil, nil, fmt.Errorf("save_sources: mkdir: %w", err)
		}

		scripts := s.client.Scripts()
		saved, split := 0, 0

		for _, sc := range scripts {
			if sc.URL == "" || strings.HasPrefix(sc.URL, "node:") {
				continue
			}

			source, err := s.debugger.GetScriptSource(sc.ScriptID)
			if err != nil {
				continue
			}

			// Check if we have a bundle structure.
			bs := s.bundles.get(sc.URL)
			if bs != nil && bs.Structure != nil && len(bs.Structure.Files) > 0 {
				// Split into individual files.
				for _, f := range bs.Structure.Files {
					fpath := filepath.Join(outDir, f.Path)
					if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
						continue
					}
					// Extract by byte offset if available, else by line.
					var content string
					if f.StartOffset > 0 || f.EndOffset > 0 {
						end := f.EndOffset
						if end > len(source) {
							end = len(source)
						}
						if f.StartOffset < end {
							content = source[f.StartOffset:end]
						}
					} else {
						content = extractByLines(source, f.StartLine, f.EndLine)
					}
					os.WriteFile(fpath, []byte(content), 0644)
					split++
				}
				// Also write the sourcemap.
				if len(bs.MapJSON) > 0 {
					mapPath := filepath.Join(outDir, sanitizePath(sc.URL)+".map")
					os.MkdirAll(filepath.Dir(mapPath), 0755)
					os.WriteFile(mapPath, bs.MapJSON, 0644)
				}
			} else {
				// Write as single file.
				fpath := filepath.Join(outDir, sanitizePath(sc.URL))
				os.MkdirAll(filepath.Dir(fpath), 0755)
				os.WriteFile(fpath, []byte(source), 0644)
			}
			saved++
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(
				"saved %d scripts (%d split from bundles) to %s", saved, split, outDir)}},
		}, nil, nil
	})
}

// resolveScript finds a script by ID or URL.
func resolveScript(s *ndpSession, scriptID, url string) (string, string, error) {
	if scriptID != "" {
		scripts := s.client.Scripts()
		if sc, ok := scripts[scriptID]; ok {
			return scriptID, sc.URL, nil
		}
		return scriptID, scriptID, nil
	}
	if url != "" {
		for _, sc := range s.client.Scripts() {
			if sc.URL == url || strings.Contains(sc.URL, url) {
				return sc.ScriptID, sc.URL, nil
			}
		}
		return "", "", fmt.Errorf("no script matching URL %q", url)
	}
	return "", "", fmt.Errorf("provide script_id or url")
}

// generateMapFromInferred creates a sourcemap v3 from the inferred structure.
func generateMapFromInferred(bundleSource string, result *inferredResult) ([]byte, error) {
	var sources, sourcesContent []string
	var allMappings []sourcemap.Mapping

	for i, f := range result.Files {
		sources = append(sources, f.Path)

		var content string
		if f.StartOffset > 0 || f.EndOffset > 0 {
			end := f.EndOffset
			if end > len(bundleSource) {
				end = len(bundleSource)
			}
			if f.StartOffset < end {
				content = bundleSource[f.StartOffset:end]
			}
		} else {
			content = extractByLines(bundleSource, f.StartLine, f.EndLine)
		}
		sourcesContent = append(sourcesContent, content)

		lineCount := sourcemap.CountLinesInString(content)
		generatedOffset := f.StartLine - 1 // 0-based
		if generatedOffset < 0 {
			generatedOffset = 0
		}
		mappings := sourcemap.BuildChunkMappings(i, generatedOffset, sourcemap.CodeChunk{
			StartLine: f.StartLine,
			EndLine:   f.EndLine,
			StartCol:  0,
		})
		if len(mappings) == 0 {
			mappings = sourcemap.BuildIdentityMappings(i, lineCount)
		}
		allMappings = append(allMappings, mappings...)
	}

	return sourcemap.GenerateV3("bundle.js", sources, sourcesContent, allMappings, nil)
}

// extractByLines extracts lines [start, end] (1-based, inclusive) from source.
func extractByLines(source string, start, end int) string {
	lines := strings.Split(source, "\n")
	if start < 1 {
		start = 1
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start > end {
		return ""
	}
	return strings.Join(lines[start-1:end], "\n")
}

// writeMapToDisk writes a sourcemap to ~/.cdp/sourcemaps/.
func writeMapToDisk(bundleURL string, mapJSON []byte) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".cdp", "sourcemaps")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(bundleURL)))[:12]
	name := sanitizePath(bundleURL)
	if len(name) > 60 {
		name = name[:60]
	}
	path := filepath.Join(dir, name+"-"+hash+".map")
	return path, os.WriteFile(path, mapJSON, 0644)
}

// sanitizePath converts a URL to a safe filesystem path component.
func sanitizePath(url string) string {
	r := strings.NewReplacer("://", "/", "?", "_", "#", "_", ":", "_")
	return r.Replace(url)
}
