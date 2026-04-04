package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/chromedp/cdproto/tracing"
	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- Tracing tools ---

// traceCollector accumulates trace data from EventDataCollected.
type traceCollector struct {
	mu      sync.Mutex
	events  []json.RawMessage
	running bool
}

func newTraceCollector() *traceCollector {
	return &traceCollector{}
}

func (tc *traceCollector) handleEvent(ev any) {
	switch e := ev.(type) {
	case *tracing.EventDataCollected:
		tc.mu.Lock()
		for _, v := range e.Value {
			tc.events = append(tc.events, json.RawMessage(v))
		}
		tc.mu.Unlock()
	case *tracing.EventTracingComplete:
		tc.mu.Lock()
		tc.running = false
		tc.mu.Unlock()
	}
}

func (tc *traceCollector) isRunning() bool {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return tc.running
}

func (tc *traceCollector) getEvents() []json.RawMessage {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	result := make([]json.RawMessage, len(tc.events))
	copy(result, tc.events)
	return result
}

func (tc *traceCollector) reset() {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.events = nil
	tc.running = true
}

type StartTraceInput struct {
	Categories string `json:"categories,omitempty"`
}

type StopTraceInput struct {
	Path string `json:"path,omitempty"`
}

func registerTraceTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "start_trace",
		Description: `Start Chrome tracing. Optional categories (comma-separated, e.g. "devtools.timeline,v8.execute"). Default captures timeline, network, and rendering events.`,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input StartTraceInput) (*mcp.CallToolResult, any, error) {
		if s.traces == nil {
			s.traces = newTraceCollector()
			chromedp.ListenTarget(s.activeCtx(), s.traces.handleEvent)
		}
		if s.traces.isRunning() {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "tracing already running"}},
			}, nil, nil
		}

		s.traces.reset()

		categories := input.Categories
		if categories == "" {
			categories = "devtools.timeline,v8.execute,disabled-by-default-devtools.timeline,disabled-by-default-v8.cpu_profiler"
		}

		actx := s.activeCtx()
		if err := chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error {
			return tracing.Start().WithTraceConfig(&tracing.TraceConfig{
				IncludedCategories: splitCategories(categories),
			}).Do(ctx)
		})); err != nil {
			return nil, nil, fmt.Errorf("start_trace: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("tracing started (categories: %s)", categories)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "stop_trace",
		Description: "Stop Chrome tracing and save the trace file. Provide a path to write the trace JSON, or it writes to the output directory.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input StopTraceInput) (*mcp.CallToolResult, any, error) {
		if s.traces == nil || !s.traces.isRunning() {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "tracing not running"}},
			}, nil, nil
		}

		actx := s.activeCtx()
		if err := chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error {
			return tracing.End().Do(ctx)
		})); err != nil {
			return nil, nil, fmt.Errorf("stop_trace: %w", err)
		}

		// Wait briefly for EventTracingComplete + data collection.
		// The events are collected via the listener.
		events := s.traces.getEvents()

		// Build Chrome trace format.
		traceData := map[string]any{
			"traceEvents": events,
			"metadata":    map[string]string{"source": "cdp-mcp-server"},
		}
		data, err := json.Marshal(traceData)
		if err != nil {
			return nil, nil, fmt.Errorf("stop_trace: marshal: %w", err)
		}

		path := input.Path
		if path == "" && s.outputDir != "" {
			path = filepath.Join(s.outputDir, "trace.json")
		}
		if path == "" {
			path = "trace.json"
		}
		if !filepath.IsAbs(path) && s.outputDir != "" {
			path = filepath.Join(s.outputDir, path)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, nil, fmt.Errorf("stop_trace: create dir: %w", err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			return nil, nil, fmt.Errorf("stop_trace: write: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("trace saved to %s (%d events, %d bytes)", path, len(events), len(data))}},
		}, nil, nil
	})
}

func splitCategories(s string) []string {
	var result []string
	for _, c := range splitComma(s) {
		c = trimSpace(c)
		if c != "" {
			result = append(result, c)
		}
	}
	return result
}

func splitComma(s string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

func trimSpace(s string) string {
	for len(s) > 0 && s[0] == ' ' {
		s = s[1:]
	}
	for len(s) > 0 && s[len(s)-1] == ' ' {
		s = s[:len(s)-1]
	}
	return s
}
