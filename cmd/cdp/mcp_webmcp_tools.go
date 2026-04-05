package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/chromedp/cdproto/webmcp"
	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// webMCPCollector tracks tools registered by web pages via navigator.modelContext.
type webMCPCollector struct {
	mu          sync.Mutex
	tools       map[string]*webmcp.Tool // name -> tool
	invocations []webMCPInvocation
	maxEntries  int
	jsMode      bool // true when CDP domain unavailable, using JS API fallback
}

type webMCPInvocation struct {
	InvocationID string `json:"invocation_id"`
	ToolName     string `json:"tool_name"`
	FrameID      string `json:"frame_id"`
	Input        string `json:"input"`
	Status       string `json:"status,omitempty"`
	Output       string `json:"output,omitempty"`
	ErrorText    string `json:"error_text,omitempty"`
	StartTime    string `json:"start_time"`
	EndTime      string `json:"end_time,omitempty"`
}

func newWebMCPCollector() *webMCPCollector {
	return &webMCPCollector{
		tools:      make(map[string]*webmcp.Tool),
		maxEntries: 500,
	}
}

func (w *webMCPCollector) handleEvent(ev any) {
	w.mu.Lock()
	defer w.mu.Unlock()

	switch e := ev.(type) {
	case *webmcp.EventToolsAdded:
		for _, t := range e.Tools {
			w.tools[t.Name] = t
		}
	case *webmcp.EventToolsRemoved:
		for _, t := range e.Tools {
			delete(w.tools, t.Name)
		}
	case *webmcp.EventToolInvoked:
		inv := webMCPInvocation{
			InvocationID: e.InvocationID,
			ToolName:     e.ToolName,
			FrameID:      string(e.FrameID),
			Input:        e.Input,
			StartTime:    time.Now().UTC().Format(time.RFC3339),
		}
		if len(w.invocations) >= w.maxEntries {
			w.invocations = w.invocations[1:]
		}
		w.invocations = append(w.invocations, inv)
	case *webmcp.EventToolResponded:
		// Update the matching invocation with the response.
		for i := len(w.invocations) - 1; i >= 0; i-- {
			if w.invocations[i].InvocationID == e.InvocationID {
				w.invocations[i].Status = string(e.Status)
				w.invocations[i].Output = string(e.Output)
				w.invocations[i].ErrorText = e.ErrorText
				w.invocations[i].EndTime = time.Now().UTC().Format(time.RFC3339)
				break
			}
		}
	}
}

func (w *webMCPCollector) listTools() []*webmcp.Tool {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]*webmcp.Tool, 0, len(w.tools))
	for _, t := range w.tools {
		out = append(out, t)
	}
	return out
}

func (w *webMCPCollector) getTool(name string) *webmcp.Tool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.tools[name]
}

func (w *webMCPCollector) listInvocations(last int) []webMCPInvocation {
	w.mu.Lock()
	defer w.mu.Unlock()
	if last <= 0 || last > len(w.invocations) {
		last = len(w.invocations)
	}
	out := make([]webMCPInvocation, last)
	copy(out, w.invocations[len(w.invocations)-last:])
	return out
}

// enableWebMCP enables the WebMCP domain and starts listening for events.
// Falls back to JS API mode if the CDP domain is unavailable (-32601).
func enableWebMCP(ctx context.Context) (*webMCPCollector, error) {
	collector := newWebMCPCollector()

	// Try the CDP domain first with a timeout. We use a goroutine + channel
	// instead of context.WithTimeout because cancelling a chromedp-derived
	// context kills the browser target.
	type result struct{ err error }
	ch := make(chan result, 1)
	go func() {
		err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			return webmcp.Enable().Do(ctx)
		}))
		ch <- result{err}
	}()

	var cdpErr error
	select {
	case r := <-ch:
		cdpErr = r.err
	case <-time.After(3 * time.Second):
		cdpErr = fmt.Errorf("timed out after 3s")
	}

	if cdpErr == nil {
		// CDP domain available — listen for events.
		chromedp.ListenTarget(ctx, collector.handleEvent)
		return collector, nil
	}

	// Check if navigator.modelContext JS API is available as fallback.
	// Use a timeout to avoid hanging if the browser tab is unresponsive.
	var hasAPI bool
	jsCh := make(chan error, 1)
	go func() {
		jsCh <- chromedp.Run(ctx, chromedp.Evaluate(
			`typeof navigator.modelContext !== 'undefined'`, &hasAPI,
		))
	}()
	select {
	case jsErr := <-jsCh:
		if jsErr != nil || !hasAPI {
			return nil, fmt.Errorf("enable WebMCP domain: %w (JS API also unavailable)", cdpErr)
		}
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("enable WebMCP domain: %w (JS API check timed out)", cdpErr)
	}

	log.Printf("WebMCP CDP domain unavailable (%v), using JS API fallback", cdpErr)
	collector.jsMode = true
	return collector, nil
}

// discoverToolsViaJS uses the JS API to discover registered tools.
// Returns a JSON array of tool info, or an error.
func discoverToolsViaJS(ctx context.Context) (string, error) {
	// Check if getTools is available (sync check).
	var hasGetTools bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(
		`typeof navigator.modelContext !== 'undefined' && typeof navigator.modelContext.getTools === 'function'`,
		&hasGetTools,
	)); err != nil {
		return "", fmt.Errorf("discover tools via JS: %w", err)
	}

	if hasGetTools {
		// Use getTools() — requires await, so use a promise callback pattern.
		var result string
		err := chromedp.Run(ctx, chromedp.Evaluate(
			`navigator.modelContext.getTools().then(tools => JSON.stringify(tools.map(t => ({name: t.name, description: t.description || '', input_schema: t.inputSchema || null}))))`,
			&result, chromedp.EvalAsValue,
		))
		if err != nil {
			return "", fmt.Errorf("discover tools via JS: getTools: %w", err)
		}
		return result, nil
	}

	// No getTools — report available methods (sync, no async needed).
	var result string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`JSON.stringify({
  error: "getTools() not available",
  available_methods: Object.getOwnPropertyNames(Object.getPrototypeOf(navigator.modelContext)).filter(m => m !== 'constructor'),
  note: "This browser version supports registerTool/unregisterTool but not tool discovery."
})`, &result)); err != nil {
		return "", fmt.Errorf("discover tools via JS: %w", err)
	}
	return result, nil
}

// invokeWebToolJS calls a tool via the JS API, with a shape that works
// regardless of whether getTools()/execute() or a direct invoke pattern is used.
func invokeWebToolJS(ctx context.Context, name, inputJSON string) (string, error) {
	// Use .then() chains instead of async IIFE to avoid Brave EvaluateAsDevTools bug.
	expr := fmt.Sprintf(`navigator.modelContext.getTools().then(tools => {
  const tool = tools.find(t => t.name === %q);
  if (!tool) throw new Error("tool not found: %s");
  return tool.execute(%s);
}).then(result => JSON.stringify(result))`, name, name, inputJSON)

	var hasGetTools bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(
		`typeof navigator.modelContext !== 'undefined' && typeof navigator.modelContext.getTools === 'function'`,
		&hasGetTools,
	)); err != nil || !hasGetTools {
		// Try invokeTool as fallback.
		invokeExpr := fmt.Sprintf(
			`navigator.modelContext.invokeTool(%q, %s).then(r => JSON.stringify(r))`,
			name, inputJSON,
		)
		var result string
		if err2 := chromedp.Run(ctx, chromedp.Evaluate(invokeExpr, &result, chromedp.EvalAsValue)); err2 != nil {
			return "", fmt.Errorf("invoke web tool %q: no invocation method available", name)
		}
		return result, nil
	}

	var result string
	if err := chromedp.Run(ctx, chromedp.Evaluate(expr, &result, chromedp.EvalAsValue)); err != nil {
		return "", fmt.Errorf("invoke web tool %q: %w", name, err)
	}
	return result, nil
}

// invokeWebTool calls a page-registered MCP tool via Runtime.evaluate.
// The WebMCP CDP domain is observation-only; invocation requires JS injection.
func invokeWebTool(ctx context.Context, name, inputJSON string) (string, error) {
	expr := fmt.Sprintf(
		`navigator.modelContext.getTools().then(tools => {
  const tool = tools.find(t => t.name === %q);
  if (!tool) throw new Error("tool not found: %s");
  return tool.execute(%s);
}).then(result => JSON.stringify(result))`, name, name, inputJSON)

	var result string
	if err := chromedp.Run(ctx, chromedp.Evaluate(expr, &result, chromedp.EvalAsValue)); err != nil {
		return "", fmt.Errorf("invoke web tool %q: %w", name, err)
	}
	return result, nil
}

// --- MCP tool registration ---

type ListWebToolsInput struct {
	FrameID string `json:"frame_id,omitempty"`
}

type InvokeWebToolInput struct {
	Name  string `json:"name"`
	Input string `json:"input,omitempty"`
}

type WebToolInvocationsInput struct {
	Last int `json:"last,omitempty"`
}

func registerWebMCPTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "list_web_tools",
		Description: `List MCP tools registered by the current web page via navigator.modelContext.

Requires the WebMCP domain to be enabled (call enable_webmcp first).
Returns tool names, descriptions, input schemas, and annotations.`,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListWebToolsInput) (*mcp.CallToolResult, any, error) {
		if s.webMCP == nil {
			return nil, nil, fmt.Errorf("WebMCP not enabled — call enable_webmcp first")
		}

		// JS mode: discover tools via the page's JS API.
		if s.webMCP.jsMode {
			actx := s.activeCtx()
			result, err := discoverToolsViaJS(actx)
			if err != nil {
				return nil, nil, fmt.Errorf("list_web_tools: %w", err)
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("(JS API mode)\n%s", result)}},
			}, nil, nil
		}

		tools := s.webMCP.listTools()
		if len(tools) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "no web tools registered on this page"}},
			}, nil, nil
		}

		type toolInfo struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"input_schema,omitempty"`
			ReadOnly    bool            `json:"read_only,omitempty"`
			Autosubmit  bool            `json:"autosubmit,omitempty"`
			FrameID     string          `json:"frame_id"`
			NodeID      int64           `json:"node_id,omitempty"`
		}
		var infos []toolInfo
		for _, t := range tools {
			if input.FrameID != "" && string(t.FrameID) != input.FrameID {
				continue
			}
			info := toolInfo{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: json.RawMessage(t.InputSchema),
				FrameID:     string(t.FrameID),
			}
			if t.Annotations != nil {
				info.ReadOnly = t.Annotations.ReadOnly
				info.Autosubmit = t.Annotations.Autosubmit
			}
			if t.BackendNodeID > 0 {
				info.NodeID = int64(t.BackendNodeID)
			}
			infos = append(infos, info)
		}

		data, _ := json.MarshalIndent(infos, "", "  ")
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("%d web tool(s):\n%s", len(infos), string(data))}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "invoke_web_tool",
		Description: `Invoke a web-page-registered MCP tool by name.

The tool must have been discovered via list_web_tools. Input should be
a JSON string matching the tool's input schema. The invocation happens
via Runtime.evaluate calling navigator.modelContext on the page.`,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input InvokeWebToolInput) (*mcp.CallToolResult, any, error) {
		if s.webMCP == nil {
			return nil, nil, fmt.Errorf("WebMCP not enabled — call enable_webmcp first")
		}
		if input.Name == "" {
			return nil, nil, fmt.Errorf("invoke_web_tool: name is required")
		}

		inputJSON := input.Input
		if inputJSON == "" {
			inputJSON = "{}"
		}

		actx := s.activeCtx()

		// In JS mode, invoke directly — we can't check getTool since
		// the CDP domain isn't providing tool discovery.
		if s.webMCP.jsMode {
			result, err := invokeWebToolJS(actx, input.Name, inputJSON)
			if err != nil {
				return nil, nil, fmt.Errorf("invoke_web_tool: %w", err)
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: result}},
			}, nil, nil
		}

		tool := s.webMCP.getTool(input.Name)
		if tool == nil {
			return nil, nil, fmt.Errorf("invoke_web_tool: tool %q not found — check list_web_tools", input.Name)
		}

		result, err := invokeWebTool(actx, input.Name, inputJSON)
		if err != nil {
			return nil, nil, fmt.Errorf("invoke_web_tool: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: result}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "enable_webmcp",
		Description: `Enable the WebMCP domain to discover page-registered MCP tools.

Once enabled, the browser reports toolsAdded/toolsRemoved events as
pages register tools via navigator.modelContext.registerTool().
Use list_web_tools to see discovered tools, invoke_web_tool to call them.`,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
		if s.webMCP != nil {
			mode := "CDP"
			if s.webMCP.jsMode {
				mode = "JS API"
			}
			tools := s.webMCP.listTools()
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("WebMCP already enabled (%s mode, %d tools registered)", mode, len(tools))}},
			}, nil, nil
		}

		actx := s.activeCtx()
		collector, err := enableWebMCP(actx)
		if err != nil {
			return nil, nil, fmt.Errorf("enable_webmcp: %w", err)
		}
		s.webMCP = collector

		if collector.jsMode {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "WebMCP enabled (JS API fallback — CDP domain unavailable). Use list_web_tools to discover tools, invoke_web_tool to call them. Note: passive invocation observation is not available in JS mode."}},
			}, nil, nil
		}

		tools := collector.listTools()
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("WebMCP enabled — %d tool(s) discovered", len(tools))}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "web_tool_invocations",
		Description: "List recent tool invocations observed via the WebMCP domain. Shows tool name, input, status, output, and timing.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input WebToolInvocationsInput) (*mcp.CallToolResult, any, error) {
		if s.webMCP == nil {
			return nil, nil, fmt.Errorf("WebMCP not enabled — call enable_webmcp first")
		}
		if s.webMCP.jsMode {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "invocation observation not available in JS API mode (requires CDP WebMCP domain for passive event monitoring)"}},
			}, nil, nil
		}
		last := input.Last
		if last <= 0 {
			last = 50
		}
		invocations := s.webMCP.listInvocations(last)
		if len(invocations) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "no invocations observed"}},
			}, nil, nil
		}
		data, _ := json.MarshalIndent(invocations, "", "  ")
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("%d invocation(s):\n%s", len(invocations), string(data))}},
		}, nil, nil
	})
}
