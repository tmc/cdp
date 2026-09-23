package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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

	tools, err := listMCPTools(ctx, server)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(struct {
		Tools []*mcp.Tool `json:"tools"`
	}{tools}, "", "  ")
}
