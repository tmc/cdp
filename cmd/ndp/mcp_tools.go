package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- Input types ---

type EvaluateInput struct {
	Expression   string `json:"expression"`
	AwaitPromise bool   `json:"await_promise,omitempty"`
}

type ListSourcesInput struct {
	Filter string `json:"filter,omitempty"` // substring match on URL
}

type ReadSourceInput struct {
	ScriptID string `json:"script_id,omitempty"`
	URL      string `json:"url,omitempty"`
}

type SearchSourcesInput struct {
	Query         string `json:"query"`
	CaseSensitive bool   `json:"case_sensitive,omitempty"`
	IsRegex       bool   `json:"is_regex,omitempty"`
}

type GetConsoleInput struct {
	Limit int  `json:"limit,omitempty"`
	Clear bool `json:"clear,omitempty"`
}

type GetErrorsInput struct {
	Limit int  `json:"limit,omitempty"`
	Clear bool `json:"clear,omitempty"`
}

type StartCoverageInput struct{}
type StopCoverageInput struct{}
type GetCoverageInput struct{}
type CoverageSnapshotInput struct {
	Name string `json:"name,omitempty"`
}

type StartCPUProfileInput struct {
	SamplingInterval int `json:"sampling_interval,omitempty"`
}

type StopCPUProfileInput struct{}

type TakeHeapSnapshotInput struct{}

func registerNDPTools(server *mcp.Server, s *ndpSession) {
	// --- Runtime tools ---

	mcp.AddTool(server, &mcp.Tool{
		Name:        "evaluate",
		Description: "Evaluate a JavaScript expression in the Node.js runtime. Set await_promise to true for async expressions.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input EvaluateInput) (*mcp.CallToolResult, any, error) {
		opts := &EvaluateOptions{
			IncludeCommandLineAPI: true,
			ReturnByValue:        true,
			AwaitPromise:         input.AwaitPromise,
		}
		result, err := s.runtime.Evaluate(input.Expression, opts)
		if err != nil {
			return nil, nil, fmt.Errorf("evaluate: %w", err)
		}
		text := formatResult(result)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: text}},
		}, nil, nil
	})

	// --- Source tools ---

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_sources",
		Description: "List all loaded JavaScript sources (scripts). Optionally filter by URL substring.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListSourcesInput) (*mcp.CallToolResult, any, error) {
		scripts := s.client.Scripts()
		var result []*V8Script
		for _, sc := range scripts {
			if input.Filter != "" && !strings.Contains(sc.URL, input.Filter) {
				continue
			}
			result = append(result, sc)
		}
		data, err := json.Marshal(result)
		if err != nil {
			return nil, nil, fmt.Errorf("list_sources: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "read_source",
		Description: "Read the source code of a loaded script by script ID or URL.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ReadSourceInput) (*mcp.CallToolResult, any, error) {
		scriptID := input.ScriptID
		if scriptID == "" && input.URL != "" {
			// Find script by URL.
			for _, sc := range s.client.Scripts() {
				if sc.URL == input.URL || strings.Contains(sc.URL, input.URL) {
					scriptID = sc.ScriptID
					break
				}
			}
			if scriptID == "" {
				return nil, nil, fmt.Errorf("read_source: no script matching URL %q", input.URL)
			}
		}
		if scriptID == "" {
			return nil, nil, fmt.Errorf("read_source: provide script_id or url")
		}
		source, err := s.debugger.GetScriptSource(scriptID)
		if err != nil {
			return nil, nil, fmt.Errorf("read_source: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: source}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_sources",
		Description: "Search across all loaded scripts for a query string or regex.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SearchSourcesInput) (*mcp.CallToolResult, any, error) {
		type searchHit struct {
			ScriptID string                   `json:"script_id"`
			URL      string                   `json:"url"`
			Matches  []map[string]interface{} `json:"matches"`
		}
		var results []searchHit
		for _, sc := range s.client.Scripts() {
			matches, err := s.debugger.SearchInContent(sc.ScriptID, input.Query, input.CaseSensitive, input.IsRegex)
			if err != nil {
				continue
			}
			if len(matches) > 0 {
				results = append(results, searchHit{
					ScriptID: sc.ScriptID,
					URL:      sc.URL,
					Matches:  matches,
				})
			}
		}
		data, err := json.Marshal(results)
		if err != nil {
			return nil, nil, fmt.Errorf("search_sources: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	// --- Console tools ---

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_console",
		Description: "Get captured console messages from the Node.js process.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetConsoleInput) (*mcp.CallToolResult, any, error) {
		s.mu.Lock()
		msgs := make([]consoleMsg, len(s.consoleMsgs))
		copy(msgs, s.consoleMsgs)
		if input.Clear {
			s.consoleMsgs = nil
		}
		s.mu.Unlock()

		if input.Limit > 0 && len(msgs) > input.Limit {
			msgs = msgs[len(msgs)-input.Limit:]
		}
		if len(msgs) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "no console messages"}},
			}, nil, nil
		}
		data, err := json.Marshal(msgs)
		if err != nil {
			return nil, nil, fmt.Errorf("get_console: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_errors",
		Description: "Get captured JavaScript exceptions from the Node.js process.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetErrorsInput) (*mcp.CallToolResult, any, error) {
		s.mu.Lock()
		errs := make([]consoleErr, len(s.consoleErrs))
		copy(errs, s.consoleErrs)
		if input.Clear {
			s.consoleErrs = nil
		}
		s.mu.Unlock()

		if input.Limit > 0 && len(errs) > input.Limit {
			errs = errs[len(errs)-input.Limit:]
		}
		if len(errs) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "no errors"}},
			}, nil, nil
		}
		data, err := json.Marshal(errs)
		if err != nil {
			return nil, nil, fmt.Errorf("get_errors: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	// --- Coverage tools ---

	mcp.AddTool(server, &mcp.Tool{
		Name:        "start_coverage",
		Description: "Start collecting JavaScript code coverage in the Node.js process.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input StartCoverageInput) (*mcp.CallToolResult, any, error) {
		cc := s.coverageCollector
		if cc.Running() {
			cc.Stop()
		}
		if err := cc.Start(); err != nil {
			return nil, nil, fmt.Errorf("start_coverage: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "coverage collection started"}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "stop_coverage",
		Description: "Stop collecting JavaScript code coverage.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input StopCoverageInput) (*mcp.CallToolResult, any, error) {
		cc := s.coverageCollector
		if !cc.Running() {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "coverage not running"}},
			}, nil, nil
		}
		if err := cc.Stop(); err != nil {
			return nil, nil, fmt.Errorf("stop_coverage: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "coverage collection stopped"}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_coverage",
		Description: "Get current code coverage data. Returns per-file line coverage with hit counts.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetCoverageInput) (*mcp.CallToolResult, any, error) {
		cc := s.coverageCollector
		if !cc.Running() {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "coverage not running — call start_coverage first"}},
			}, nil, nil
		}
		snap, err := cc.TakeSnapshot("latest")
		if err != nil {
			return nil, nil, fmt.Errorf("get_coverage: %w", err)
		}
		data, err := json.Marshal(snap.Summary())
		if err != nil {
			return nil, nil, fmt.Errorf("get_coverage: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "coverage_snapshot",
		Description: "Take a named coverage snapshot for later comparison. Coverage must be running.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CoverageSnapshotInput) (*mcp.CallToolResult, any, error) {
		cc := s.coverageCollector
		if !cc.Running() {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "coverage not running — call start_coverage first"}},
			}, nil, nil
		}
		name := input.Name
		if name == "" {
			name = fmt.Sprintf("snap-%d", len(cc.Snapshots())+1)
		}
		snap, err := cc.TakeSnapshot(name)
		if err != nil {
			return nil, nil, fmt.Errorf("coverage_snapshot: %w", err)
		}
		summary := snap.Summary()
		data, err := json.Marshal(summary)
		if err != nil {
			return nil, nil, fmt.Errorf("coverage_snapshot: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("snapshot %q: %d files\n%s", name, len(summary), string(data))}},
		}, nil, nil
	})

	// --- Profiler tools ---

	mcp.AddTool(server, &mcp.Tool{
		Name:        "start_cpu_profile",
		Description: "Start CPU profiling. Optionally set sampling_interval in microseconds.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input StartCPUProfileInput) (*mcp.CallToolResult, any, error) {
		if err := s.profiler.EnableProfiler(); err != nil {
			return nil, nil, fmt.Errorf("start_cpu_profile: enable: %w", err)
		}
		if input.SamplingInterval > 0 {
			s.profiler.SetSamplingInterval(input.SamplingInterval)
		}
		if err := s.profiler.StartCPUProfiling("mcp", 0); err != nil {
			return nil, nil, fmt.Errorf("start_cpu_profile: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "CPU profiling started"}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "stop_cpu_profile",
		Description: "Stop CPU profiling and return the profile data.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input StopCPUProfileInput) (*mcp.CallToolResult, any, error) {
		profile, err := s.profiler.StopCPUProfiling()
		if err != nil {
			return nil, nil, fmt.Errorf("stop_cpu_profile: %w", err)
		}
		data, err := json.Marshal(profile)
		if err != nil {
			return nil, nil, fmt.Errorf("stop_cpu_profile: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "take_heap_snapshot",
		Description: "Take a heap snapshot for memory analysis.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input TakeHeapSnapshotInput) (*mcp.CallToolResult, any, error) {
		if err := s.profiler.EnableHeapProfiler(); err != nil {
			return nil, nil, fmt.Errorf("take_heap_snapshot: enable: %w", err)
		}
		if err := s.profiler.TakeHeapSnapshot(false); err != nil {
			return nil, nil, fmt.Errorf("take_heap_snapshot: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "heap snapshot taken"}},
		}, nil, nil
	})

	// --- Connection tools ---

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_targets",
		Description: "List available Node.js debugging targets.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
		targets, err := s.client.DiscoverTargets(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list_targets: %w", err)
		}
		data, err := json.Marshal(targets)
		if err != nil {
			return nil, nil, fmt.Errorf("list_targets: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})
}
