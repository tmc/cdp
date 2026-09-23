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

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tmc/cdp/cdpscript"
	"github.com/tmc/cdp/internal/tooldef"
)

// loadAndRegisterCustomTools scans toolsDir for .cdp files and registers each
// as an MCP tool backed by the cdpscript executor. Tools whose names collide
// with session.builtinTools are skipped.
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
		if session.builtinTools[def.Name] {
			log.Printf("warning: custom tool %q skipped: name %q is a built-in tool", def.SourcePath, def.Name)
			continue
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
		result, err := executeToolScript(ctx, session, def.Script, env)
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

// executeToolScript runs a cdpscript body against the MCP session's active
// tab. It waits for browser setup and stops when reqCtx is done.
func executeToolScript(reqCtx context.Context, session *mcpSession, scriptBody string, env map[string]string) (string, error) {
	actx, err := session.activeContext(reqCtx)
	if err != nil {
		return "", err
	}
	ctx, cancel := requestToolCtx(reqCtx, actx, 5*time.Minute)
	defer cancel()
	session.mu.Lock()
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
		if session.builtinTools[toolName] {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("tool name %q is a built-in tool", toolName)}},
				IsError: true,
			}, nil, nil
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
