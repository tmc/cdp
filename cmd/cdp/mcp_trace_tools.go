package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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

type AnalyzeTraceInput struct {
	Path string `json:"path,omitempty"`
}

type coreWebVitals struct {
	LCPMS float64 `json:"lcp_ms"`
	INPMS float64 `json:"inp_ms"`
	CLS   float64 `json:"cls"`
}

type traceEvent struct {
	Name string          `json:"name"`
	TS   float64         `json:"ts"`
	Dur  float64         `json:"dur"`
	Args traceEventArgs  `json:"args"`
	Raw  json.RawMessage `json:"-"`
}

type traceEventArgs struct {
	Data map[string]any `json:"data"`
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

	mcp.AddTool(server, &mcp.Tool{
		Name:        "analyze_trace",
		Description: "Analyze a Chrome trace JSON file and return Core Web Vitals: LCP, INP, and CLS. Use path \"-\" to read from stdin.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AnalyzeTraceInput) (*mcp.CallToolResult, any, error) {
		var r io.Reader
		switch input.Path {
		case "":
			return nil, nil, fmt.Errorf("analyze_trace: path required")
		case "-":
			r = os.Stdin
		default:
			f, err := os.Open(input.Path)
			if err != nil {
				return nil, nil, fmt.Errorf("analyze_trace: open: %w", err)
			}
			defer f.Close()
			r = f
		}

		vitals, err := analyzeTrace(r)
		if err != nil {
			return nil, nil, fmt.Errorf("analyze_trace: %w", err)
		}
		data, err := json.Marshal(vitals)
		if err != nil {
			return nil, nil, fmt.Errorf("analyze_trace: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
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

func analyzeTrace(r io.Reader) (coreWebVitals, error) {
	dec := json.NewDecoder(r)
	tok, err := dec.Token()
	if err != nil {
		return coreWebVitals{}, err
	}
	d, ok := tok.(json.Delim)
	if !ok {
		return coreWebVitals{}, fmt.Errorf("trace must be JSON object or array")
	}
	switch d {
	case '{':
		for dec.More() {
			tok, err := dec.Token()
			if err != nil {
				return coreWebVitals{}, err
			}
			key, ok := tok.(string)
			if !ok {
				return coreWebVitals{}, fmt.Errorf("object key is not a string")
			}
			if key != "traceEvents" {
				var skip any
				if err := dec.Decode(&skip); err != nil {
					return coreWebVitals{}, err
				}
				continue
			}
			return analyzeTraceEvents(dec)
		}
		return coreWebVitals{}, fmt.Errorf("traceEvents not found")
	case '[':
		return analyzeTraceEventsOpen(dec)
	default:
		return coreWebVitals{}, fmt.Errorf("trace must be JSON object or array")
	}
}

func analyzeTraceEvents(dec *json.Decoder) (coreWebVitals, error) {
	tok, err := dec.Token()
	if err != nil {
		return coreWebVitals{}, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '[' {
		return coreWebVitals{}, fmt.Errorf("traceEvents must be an array")
	}
	return analyzeTraceEventsOpen(dec)
}

func analyzeTraceEventsOpen(dec *json.Decoder) (coreWebVitals, error) {
	var vitals coreWebVitals
	var navStart float64
	var lcpTS float64

	for dec.More() {
		var ev traceEvent
		if err := dec.Decode(&ev); err != nil {
			return coreWebVitals{}, err
		}
		switch {
		case isNavigationStart(ev.Name):
			if navStart == 0 || ev.TS < navStart {
				navStart = ev.TS
			}
		case strings.Contains(ev.Name, "LargestContentfulPaint::Candidate"):
			if ev.TS >= lcpTS {
				lcpTS = ev.TS
			}
		case ev.Name == "EventTiming":
			d := eventDurationMS(ev)
			if d > vitals.INPMS {
				vitals.INPMS = d
			}
		case ev.Name == "LayoutShift":
			if !boolField(ev.Args.Data, "had_recent_input") {
				vitals.CLS += numberField(ev.Args.Data, "score")
			}
		}
	}
	if _, err := dec.Token(); err != nil {
		return coreWebVitals{}, err
	}
	if lcpTS != 0 {
		if navStart != 0 && lcpTS >= navStart {
			vitals.LCPMS = (lcpTS - navStart) / 1000
		} else {
			vitals.LCPMS = lcpTS / 1000
		}
	}
	return vitals, nil
}

func isNavigationStart(name string) bool {
	return name == "navigationStart" || name == "NavigationStart" || strings.HasSuffix(name, "::navigationStart")
}

func eventDurationMS(ev traceEvent) float64 {
	if d := numberField(ev.Args.Data, "duration"); d != 0 {
		return d
	}
	return ev.Dur / 1000
}

func numberField(m map[string]any, key string) float64 {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case float64:
		return v
	case json.Number:
		f, _ := v.Float64()
		return f
	default:
		return 0
	}
}

func boolField(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	v, _ := m[key].(bool)
	return v
}
