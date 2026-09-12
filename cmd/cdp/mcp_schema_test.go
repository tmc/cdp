package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const mcpToolSchemaPath = "../../docs/mcp-tools-schema.json"

func TestMCPToolSchema(t *testing.T) {
	got, err := mcpToolSchema(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')

	path := filepath.Clean(mcpToolSchemaPath)
	if os.Getenv("UPDATE_MCP_TOOL_SCHEMA") == "1" {
		if err := os.WriteFile(path, got, 0644); err != nil {
			t.Fatal(err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("MCP tool schema is missing; run go generate ./cmd/cdp")
	}
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("MCP tool schema is stale; run go generate ./cmd/cdp")
	}
}

func mcpToolSchema(ctx context.Context) ([]byte, error) {
	server := mcp.NewServer(&mcp.Implementation{Name: "cdp", Version: "schema"}, nil)
	registerMCPTools(server, &mcpSession{refs: newRefRegistry()}, mcpConfig{EnableInspect: true})

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect schema server: %w", err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "cdp-schema", Version: "1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect schema client: %w", err)
	}
	defer clientSession.Close()

	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list MCP tools: %w", err)
	}
	sort.Slice(result.Tools, func(i, j int) bool {
		return result.Tools[i].Name < result.Tools[j].Name
	})
	return json.MarshalIndent(struct {
		Tools []*mcp.Tool `json:"tools"`
	}{result.Tools}, "", "  ")
}
