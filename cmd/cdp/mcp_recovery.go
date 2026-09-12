package main

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var errMCPToolPanic = errors.New("internal tool error")

// addMCPTool registers h and turns a handler panic into a tool error.
func addMCPTool[In, Out any](server *mcp.Server, tool *mcp.Tool, h mcp.ToolHandlerFor[In, Out]) {
	mcp.AddTool(server, tool, func(ctx context.Context, req *mcp.CallToolRequest, input In) (result *mcp.CallToolResult, output Out, err error) {
		defer func() {
			if recover() != nil {
				err = errMCPToolPanic
			}
		}()
		return h(ctx, req, input)
	})
}

// addMCPRawTool is addMCPTool for handlers that provide their own result.
func addMCPRawTool(server *mcp.Server, tool *mcp.Tool, h mcp.ToolHandler) {
	server.AddTool(tool, func(ctx context.Context, req *mcp.CallToolRequest) (result *mcp.CallToolResult, err error) {
		defer func() {
			if recover() != nil {
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
