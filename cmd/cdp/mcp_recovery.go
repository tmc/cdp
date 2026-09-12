package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var errMCPToolPanic = errors.New("internal tool error")

// addMCPTool registers h and turns a handler panic into a tool error.
// It cannot recover a panic from a goroutine started by h.
func addMCPTool[In, Out any](server *mcp.Server, tool *mcp.Tool, h mcp.ToolHandlerFor[In, Out]) {
	mcp.AddTool(server, tool, func(ctx context.Context, req *mcp.CallToolRequest, input In) (result *mcp.CallToolResult, output Out, err error) {
		defer func() {
			if v := recover(); v != nil {
				logMCPToolPanic(tool.Name, v)
				err = fmt.Errorf("%w: %s", errMCPToolPanic, tool.Name)
			}
		}()
		return h(ctx, req, input)
	})
}

// addMCPRawTool is addMCPTool for handlers that provide their own result.
func addMCPRawTool(server *mcp.Server, tool *mcp.Tool, h mcp.ToolHandler) {
	server.AddTool(tool, func(ctx context.Context, req *mcp.CallToolRequest) (result *mcp.CallToolResult, err error) {
		defer func() {
			if v := recover(); v != nil {
				logMCPToolPanic(tool.Name, v)
				result = &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: errMCPToolPanic.Error()}},
					IsError: true,
				}
				err = nil
			}
		}()
		return h(ctx, req)
	})
}

func logMCPToolPanic(name string, v any) {
	fmt.Fprintf(os.Stderr, "cdp: tool %s panicked: %v\n%s", name, v, debug.Stack())
}
