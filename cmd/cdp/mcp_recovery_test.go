package main

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPToolPanicIsToolError(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	addMCPTool(server, &mcp.Tool{Name: "panic"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, struct{}, error) {
		panic("test panic")
	})
	addMCPTool(server, &mcp.Tool{Name: "ok"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, struct {
		Status string `json:"status"`
	}, error) {
		return nil, struct {
			Status string `json:"status"`
		}{Status: "ok"}, nil
	})

	client := mcpTestClient(t, server)
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "panic"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("panic result IsError = false: %+v", result)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(text.Text, "panic") {
		t.Fatalf("panic result content = %#v, want tool name", result.Content)
	}

	result, err = client.CallTool(t.Context(), &mcp.CallToolParams{Name: "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("follow-up result IsError = true: %+v", result)
	}
}

func TestMCPRawToolPanicIsToolError(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	addMCPRawTool(server, &mcp.Tool{Name: "panic", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		panic("test panic")
	})

	client := mcpTestClient(t, server)
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "panic"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("panic result IsError = false: %+v", result)
	}
}

func mcpTestClient(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	clientSession, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { clientSession.Close() })
	return clientSession
}
