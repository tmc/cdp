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
	hasTesting  bool // true when navigator.modelContextTesting is available
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

	// Check for JS API fallback with a timeout to avoid hanging.
	type jsProbe struct {
		hasContext bool
		hasTesting bool
		err        error
	}
	jsCh := make(chan jsProbe, 1)
	go func() {
		var p jsProbe
		p.err = chromedp.Run(ctx, chromedp.Evaluate(
			`typeof navigator.modelContext !== 'undefined'`, &p.hasContext,
		))
		if p.err == nil {
			chromedp.Run(ctx, chromedp.Evaluate(
				`typeof navigator.modelContextTesting !== 'undefined' && typeof navigator.modelContextTesting.listTools === 'function'`,
				&p.hasTesting,
			))
		}
		jsCh <- p
	}()

	var probe jsProbe
	select {
	case probe = <-jsCh:
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("enable WebMCP domain: %w (JS API check timed out)", cdpErr)
	}
	if probe.err != nil || !probe.hasContext {
		return nil, fmt.Errorf("enable WebMCP domain: %w (JS API also unavailable)", cdpErr)
	}

	collector.jsMode = true
	collector.hasTesting = probe.hasTesting
	if probe.hasTesting {
		log.Printf("WebMCP CDP domain unavailable (%v), using modelContextTesting API", cdpErr)
	} else {
		log.Printf("WebMCP CDP domain unavailable (%v), using modelContext API (registration only)", cdpErr)
	}
	return collector, nil
}

// discoverToolsViaJS uses the JS API to discover registered tools.
// Priority: modelContextTesting.listTools() > modelContext.getTools() > method enumeration.
func discoverToolsViaJS(ctx context.Context) (string, error) {
	// Try modelContextTesting.listTools() first (sync, no promises).
	var hasTesting bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(
		`typeof navigator.modelContextTesting !== 'undefined' && typeof navigator.modelContextTesting.listTools === 'function'`,
		&hasTesting,
	)); err == nil && hasTesting {
		var result string
		if err := chromedp.Run(ctx, chromedp.Evaluate(
			`JSON.stringify(navigator.modelContextTesting.listTools().map(t => ({name: t.name, description: t.description || '', input_schema: t.inputSchema || null})))`,
			&result,
		)); err != nil {
			return "", fmt.Errorf("discover tools via JS: modelContextTesting.listTools: %w", err)
		}
		return result, nil
	}

	// Fallback: modelContext.getTools() (async, needs .then()).
	var hasGetTools bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(
		`typeof navigator.modelContext !== 'undefined' && typeof navigator.modelContext.getTools === 'function'`,
		&hasGetTools,
	)); err == nil && hasGetTools {
		var result string
		if err := chromedp.Run(ctx, chromedp.Evaluate(
			`navigator.modelContext.getTools().then(tools => JSON.stringify(tools.map(t => ({name: t.name, description: t.description || '', input_schema: t.inputSchema || null}))))`,
			&result, chromedp.EvalAsValue,
		)); err != nil {
			return "", fmt.Errorf("discover tools via JS: getTools: %w", err)
		}
		return result, nil
	}

	// No discovery method — report available methods.
	var result string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`JSON.stringify({
  error: "no tool discovery method available",
  available_methods: typeof navigator.modelContext !== 'undefined' ? Object.getOwnPropertyNames(Object.getPrototypeOf(navigator.modelContext)).filter(m => m !== 'constructor') : [],
  note: "Enable 'WebMCP for testing' flag for tool discovery via modelContextTesting.listTools()."
})`, &result)); err != nil {
		return "", fmt.Errorf("discover tools via JS: %w", err)
	}
	return result, nil
}

// invokeWebToolJS calls a tool via the JS API.
// Priority: modelContextTesting.executeTool() > modelContext.getTools().execute().
func invokeWebToolJS(ctx context.Context, name, inputJSON string) (string, error) {
	// Try modelContextTesting.executeTool() first (sync).
	var hasTesting bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(
		`typeof navigator.modelContextTesting !== 'undefined' && typeof navigator.modelContextTesting.executeTool === 'function'`,
		&hasTesting,
	)); err == nil && hasTesting {
		var result string
		expr := fmt.Sprintf(`navigator.modelContextTesting.executeTool(%q, %s)`, name, inputJSON)
		if err := chromedp.Run(ctx, chromedp.Evaluate(expr, &result)); err != nil {
			return "", fmt.Errorf("invoke web tool %q via modelContextTesting: %w", name, err)
		}
		return result, nil
	}

	// Fallback: modelContext.getTools() + execute (async, .then() chain).
	var hasGetTools bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(
		`typeof navigator.modelContext !== 'undefined' && typeof navigator.modelContext.getTools === 'function'`,
		&hasGetTools,
	)); err == nil && hasGetTools {
		expr := fmt.Sprintf(`navigator.modelContext.getTools().then(tools => {
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

	return "", fmt.Errorf("invoke web tool %q: no invocation method available (need modelContextTesting or modelContext.getTools)", name)
}

// invokeWebTool calls a page-registered MCP tool via Runtime.evaluate.
// The WebMCP CDP domain is observation-only; invocation requires JS injection.
// Delegates to invokeWebToolJS which handles API detection.
func invokeWebTool(ctx context.Context, name, inputJSON string) (string, error) {
	return invokeWebToolJS(ctx, name, inputJSON)
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
				if s.webMCP.hasTesting {
					mode = "JS API (modelContextTesting)"
				}
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
			msg := "WebMCP enabled (JS API fallback — CDP domain unavailable)."
			if collector.hasTesting {
				msg += " modelContextTesting API available — full tool discovery and invocation supported."
			} else {
				msg += " modelContext only — registration-side API (registerTool/unregisterTool). Enable 'WebMCP for testing' flag for tool discovery."
			}
			msg += " Note: passive invocation observation not available in JS mode."
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: msg}},
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
