package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tmc/cdp/cdpscript"
	"github.com/tmc/cdp/internal/tooldef"
)

// builtinToolNames is the set of MCP tool names registered by the cdp server.
// Custom tools that collide with these names are prefixed with "custom_".
var builtinToolNames = map[string]bool{
	"navigate": true, "navigate_back": true, "navigate_forward": true, "reload": true,
	"screenshot": true, "get_page_content": true, "page_snapshot": true,
	"click": true, "type_text": true, "wait_for": true,
	"evaluate": true, "raw_cdp": true,
	"find_element": true, "get_element": true, "check_element": true,
	"press_key": true, "hover": true, "focus": true,
	"get_console": true, "get_errors": true,
	"list_frames": true, "switch_frame": true,
	"scroll":        true,
	"handle_dialog": true, "get_dialogs": true,
	"upload_file": true,
	"get_storage": true, "set_storage": true, "clear_storage": true,
	"set_viewport": true, "set_user_agent": true, "set_offline": true,
	"set_geolocation": true, "set_extra_headers": true,
	"intercept_request": true, "intercept_response": true,
	"remove_intercept": true, "list_intercepts": true,
	"save_pdf":    true,
	"start_trace": true, "stop_trace": true, "analyze_trace": true,
	"save_state": true, "load_state": true,
	"snapshot_dom": true, "dom_diff": true, "list_dom_snapshots": true,
	"analyze_bundle": true, "generate_sourcemap": true, "serve_sourcemap": true,
	"list_sourcemaps": true, "refine_sourcemap": true,
	"list_tabs": true, "switch_tab": true, "new_tab": true,
	"push_context": true, "pop_context": true,
	"get_har_entries": true, "get_cookies": true, "set_cookie": true,
	"save_sources": true, "list_sources": true, "read_source": true, "search_source": true,
	"start_coverage": true, "stop_coverage": true, "get_coverage": true,
	"get_coverage_delta": true, "compare_coverage": true, "list_snapshots": true,
	"define_tool":   true,
	"run_cdpscript": true, "validate_script": true, "list_examples": true,
}

// loadAndRegisterCustomTools scans toolsDir for .cdp files and registers each
// as an MCP tool backed by the cdpscript executor.
func loadAndRegisterCustomTools(server *mcp.Server, session *mcpSession, toolsDir string) error {
	defs, err := tooldef.LoadDir(toolsDir)
	if err != nil {
		return fmt.Errorf("load tools dir: %w", err)
	}
	for _, def := range defs {
		if err := cdpscript.ValidateScript(def.SourcePath, def.Script); err != nil {
			log.Printf("warning: custom tool %q skipped: %v", def.SourcePath, err)
			continue
		}
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

	addMCPRawTool(server, tool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
func executeToolScript(session *mcpSession, scriptBody string, env map[string]string) (string, error) {
	session.mu.Lock()
	ctx := session.ctx
	outputDir := session.contextOutputDir()
	session.mu.Unlock()

	stdout, stderr, err := runCDPScriptBody(ctx, scriptBody, env, outputDir)
	if err != nil && stderr != "" {
		return stdout, fmt.Errorf("%w\n%s", err, strings.TrimSpace(stderr))
	}
	return stdout, err
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

	addMCPTool(server, &mcp.Tool{
		Name:        "define_tool",
		Description: "Define a new custom cdpscript tool from a script body. Each input string has the format: 'name type \"description\"'.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input defineInput) (*mcp.CallToolResult, any, error) {
		if input.Name == "" || input.Script == "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "name and script are required"}},
				IsError: true,
			}, nil, nil
		}

		if err := validPathSegment("tool name", input.Name); err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
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

		content := tooldef.Generate(def)
		parsed, err := tooldef.Parse(content, "define_tool")
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("invalid tool definition: %v", err)}},
				IsError: true,
			}, nil, nil
		}
		if err := cdpscript.ValidateScript(parsed.Name, parsed.Script); err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("invalid cdpscript: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		// Write the .cdp file only after parse and dry-run validation.
		path := filepath.Join(toolsDir, toolName+".cdp")
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
