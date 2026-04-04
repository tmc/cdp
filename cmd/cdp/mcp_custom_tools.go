package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tmc/misc/chrome-to-har/internal/tooldef"
)

// builtinToolNames is the set of MCP tool names registered by the cdp server.
// Custom tools that collide with these names are prefixed with "custom_".
var builtinToolNames = map[string]bool{
	"navigate": true, "navigate_back": true, "navigate_forward": true, "reload": true,
	"screenshot": true, "get_page_content": true, "page_snapshot": true,
	"click": true, "type_text": true, "wait_for": true,
	"evaluate": true,
	"list_tabs": true, "switch_tab": true, "new_tab": true,
	"push_context": true, "pop_context": true,
	"get_har_entries": true, "get_cookies": true, "set_cookie": true,
	"save_sources": true, "list_sources": true, "read_source": true, "search_source": true,
	"start_coverage": true, "stop_coverage": true, "get_coverage": true,
	"get_coverage_delta": true, "compare_coverage": true, "list_snapshots": true,
	"define_tool": true,
}

// loadAndRegisterCustomTools scans toolsDir for .cdp files and registers each
// as an MCP tool backed by the cdpscript executor.
func loadAndRegisterCustomTools(server *mcp.Server, session *mcpSession, toolsDir string) error {
	defs, err := tooldef.LoadDir(toolsDir)
	if err != nil {
		return fmt.Errorf("load tools dir: %w", err)
	}
	for _, def := range defs {
		if builtinToolNames[def.Name] {
			log.Printf("warning: custom tool %q collides with built-in, registering as custom_%s", def.Name, def.Name)
			def.Name = "custom_" + def.Name
		}
		registerCustomTool(server, session, def)
		log.Printf("loaded custom tool: %s (%s)", def.Name, def.SourcePath)
	}
	return nil
}

// registerCustomTool registers a single ToolDef as an MCP tool.
func registerCustomTool(server *mcp.Server, session *mcpSession, def *tooldef.ToolDef) {
	schema := def.InputSchema()
	tool := &mcp.Tool{
		Name:        def.Name,
		Description: def.Description,
		InputSchema: schema,
	}
	if def.ReadOnly {
		tool.Annotations = &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		}
	}

	server.AddTool(tool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		env, err := parseArguments(req.Params.Arguments)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("error parsing arguments: %v", err)}},
				IsError: true,
			}, nil
		}
		result, err := executeToolScript(session, def.Script, env)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("error: %v", err)}},
				IsError: true,
			}, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: result}},
		}, nil
	})
}

// parseArguments unmarshals the raw JSON arguments into a string map.
func parseArguments(raw json.RawMessage) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("unmarshal arguments: %w", err)
	}
	env := make(map[string]string, len(m))
	for k, v := range m {
		env[k] = fmt.Sprint(v)
	}
	return env, nil
}

// executeToolScript runs a cdpscript body against the MCP session's browser.
// Variable references ($name) in the script are expanded from env.
func executeToolScript(session *mcpSession, scriptBody string, env map[string]string) (string, error) {
	expanded := expandVars(scriptBody, env)

	var stdout strings.Builder
	for _, line := range strings.Split(expanded, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		result, err := execScriptCommand(session, line)
		if err != nil {
			return stdout.String(), fmt.Errorf("line %q: %w", line, err)
		}
		if result != "" {
			stdout.WriteString(result)
			stdout.WriteString("\n")
		}
	}
	return strings.TrimSpace(stdout.String()), nil
}

// expandVars replaces $name references with values from env.
func expandVars(s string, env map[string]string) string {
	return os.Expand(s, func(key string) string {
		if v, ok := env[key]; ok {
			return v
		}
		return ""
	})
}

// scriptCommandHandler handles a single cdpscript command.
type scriptCommandHandler func(ctx context.Context, args string) (string, error)

// builtinScriptCommands returns the map of built-in cdpscript commands.
func builtinScriptCommands(session *mcpSession) map[string]scriptCommandHandler {
	return map[string]scriptCommandHandler{
		"goto": func(ctx context.Context, args string) (string, error) {
			if args == "" {
				return "", fmt.Errorf("goto requires a URL")
			}
			if err := chromedp.Run(ctx, chromedp.Navigate(args)); err != nil {
				return "", fmt.Errorf("goto: %w", err)
			}
			return "", nil
		},
		"click": func(ctx context.Context, args string) (string, error) {
			if args == "" {
				return "", fmt.Errorf("click requires a selector")
			}
			if err := chromedp.Run(ctx, chromedp.Click(args, chromedp.ByQuery)); err != nil {
				return "", fmt.Errorf("click: %w", err)
			}
			return "", nil
		},
		"fill": fillHandler,
		"type": fillHandler,
		"wait": func(ctx context.Context, args string) (string, error) {
			if args == "" {
				return "", fmt.Errorf("wait requires a selector or duration (e.g. 2s)")
			}
			// Duration overload: if args parses as a duration, sleep instead.
			if d, err := time.ParseDuration(args); err == nil {
				time.Sleep(d)
				return "", nil
			}
			if err := chromedp.Run(ctx, chromedp.WaitVisible(args, chromedp.ByQuery)); err != nil {
				return "", fmt.Errorf("wait: %w", err)
			}
			return "", nil
		},
		"js": func(ctx context.Context, args string) (string, error) {
			if args == "" {
				return "", fmt.Errorf("js requires an expression")
			}
			var result any
			if err := chromedp.Run(ctx, chromedp.Evaluate(args, &result)); err != nil {
				return "", fmt.Errorf("js: %w", err)
			}
			return formatResult(result), nil
		},
		"title": func(ctx context.Context, args string) (string, error) {
			var title string
			if err := chromedp.Run(ctx, chromedp.Title(&title)); err != nil {
				return "", fmt.Errorf("title: %w", err)
			}
			return title, nil
		},
		"url": func(ctx context.Context, args string) (string, error) {
			var loc string
			if err := chromedp.Run(ctx, chromedp.Location(&loc)); err != nil {
				return "", fmt.Errorf("url: %w", err)
			}
			return loc, nil
		},
		"extract": func(ctx context.Context, args string) (string, error) {
			if args == "" {
				return "", fmt.Errorf("extract requires a selector")
			}
			var text string
			if err := chromedp.Run(ctx, chromedp.Text(args, &text, chromedp.ByQuery)); err != nil {
				return "", fmt.Errorf("extract: %w", err)
			}
			return text, nil
		},
		"screenshot": func(ctx context.Context, args string) (string, error) {
			var buf []byte
			if err := chromedp.Run(ctx, chromedp.FullScreenshot(&buf, 100)); err != nil {
				return "", fmt.Errorf("screenshot: %w", err)
			}
			return "(screenshot captured)", nil
		},
		"reload": func(ctx context.Context, args string) (string, error) {
			if err := chromedp.Run(ctx, chromedp.Reload()); err != nil {
				return "", fmt.Errorf("reload: %w", err)
			}
			return "", nil
		},
		"back": func(ctx context.Context, args string) (string, error) {
			if err := chromedp.Run(ctx, chromedp.NavigateBack()); err != nil {
				return "", fmt.Errorf("back: %w", err)
			}
			return "", nil
		},
		"forward": func(ctx context.Context, args string) (string, error) {
			if err := chromedp.Run(ctx, chromedp.NavigateForward()); err != nil {
				return "", fmt.Errorf("forward: %w", err)
			}
			return "", nil
		},
	}
}

func fillHandler(ctx context.Context, args string) (string, error) {
	sel, text, ok := splitFirstArg(args)
	if !ok {
		return "", fmt.Errorf("fill requires selector and text")
	}
	if err := chromedp.Run(ctx, chromedp.SendKeys(sel, text, chromedp.ByQuery)); err != nil {
		return "", fmt.Errorf("fill: %w", err)
	}
	return "", nil
}

// execScriptCommand dispatches a single cdpscript command line against the browser.
func execScriptCommand(session *mcpSession, line string) (string, error) {
	cmd, args := splitCommand(line)
	ctx := session.activeCtx()

	commands := builtinScriptCommands(session)
	if handler, ok := commands[cmd]; ok {
		return handler(ctx, args)
	}
	return "", fmt.Errorf("unknown command: %s", cmd)
}

// splitCommand splits a line into the command name and the rest.
func splitCommand(line string) (cmd, args string) {
	cmd, args, _ = strings.Cut(line, " ")
	return cmd, strings.TrimSpace(args)
}

// splitFirstArg splits args into the first whitespace-delimited token and the remainder.
// Used for commands like "fill <selector> <text>".
func splitFirstArg(args string) (first, rest string, ok bool) {
	first, rest, ok = strings.Cut(strings.TrimSpace(args), " ")
	rest = strings.TrimSpace(rest)
	return first, rest, ok
}

// formatResult converts an arbitrary JS evaluation result to a string.
func formatResult(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprint(val)
		}
		return string(b)
	}
}

// registerDefineToolMeta registers the define_tool meta-tool for dynamic tool creation.
func registerDefineToolMeta(server *mcp.Server, session *mcpSession, toolsDir string) error {
	type defineInput struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Script      string   `json:"script"`
		Inputs      []string `json:"inputs,omitempty"`
		ReadOnly    bool     `json:"readonly,omitempty"`
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "define_tool",
		Description: "Define a new custom cdpscript tool from a script body. Each input string has the format: 'name type \"description\"'.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input defineInput) (*mcp.CallToolResult, any, error) {
		if input.Name == "" || input.Script == "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "name and script are required"}},
				IsError: true,
			}, nil, nil
		}

		toolName := input.Name
		if builtinToolNames[toolName] {
			toolName = "custom_" + toolName
			log.Printf("warning: define_tool %q collides with built-in, using %s", input.Name, toolName)
		}

		def := &tooldef.ToolDef{
			Name:        toolName,
			Description: input.Description,
			Script:      input.Script,
			ReadOnly:    input.ReadOnly,
		}

		for _, raw := range input.Inputs {
			tokens := strings.Fields(raw)
			if len(tokens) < 2 {
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("invalid input spec %q: need at least name and type", raw)}},
					IsError: true,
				}, nil, nil
			}
			inp := tooldef.InputDef{
				Name: tokens[0],
				Type: tokens[1],
			}
			// Extract quoted description if present in the raw string.
			if idx := strings.IndexByte(raw, '"'); idx >= 0 {
				end := strings.IndexByte(raw[idx+1:], '"')
				if end >= 0 {
					inp.Description = raw[idx+1 : idx+1+end]
				}
			}
			for _, tok := range tokens[2:] {
				if tok == "optional" {
					inp.Optional = true
				}
			}
			def.Inputs = append(def.Inputs, inp)
		}

		// Write the .cdp file.
		path := filepath.Join(toolsDir, input.Name+".cdp")
		content := tooldef.Generate(def)
		if err := os.MkdirAll(toolsDir, 0755); err != nil {
			return nil, nil, fmt.Errorf("create tools dir: %w", err)
		}
		if err := os.WriteFile(path, content, 0644); err != nil {
			return nil, nil, fmt.Errorf("write tool file: %w", err)
		}
		def.SourcePath = path

		// Register the new tool on the server.
		registerCustomTool(server, session, def)
		log.Printf("defined custom tool: %s (%s)", def.Name, path)

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("tool %q defined and registered (%s)", def.Name, path)}},
		}, nil, nil
	})
	return nil
}
