package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- Storage tools (localStorage / sessionStorage) ---

type GetStorageInput struct {
	Type string `json:"type"`
	Key  string `json:"key,omitempty"`
}

type SetStorageInput struct {
	Type  string `json:"type"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

type ClearStorageInput struct {
	Type string `json:"type"`
	Key  string `json:"key,omitempty"`
}

func registerStorageTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_storage",
		Description: `Get localStorage or sessionStorage. Type must be "local" or "session". If key is omitted, returns all key-value pairs.`,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetStorageInput) (*mcp.CallToolResult, any, error) {
		storageObj, err := storageType(input.Type)
		if err != nil {
			return nil, nil, fmt.Errorf("get_storage: %w", err)
		}

		actx := s.activeCtx()
		var result any

		if input.Key != "" {
			js := fmt.Sprintf(`%s.getItem(%q)`, storageObj, input.Key)
			if err := chromedp.Run(actx, chromedp.Evaluate(js, &result)); err != nil {
				return nil, nil, fmt.Errorf("get_storage: %w", err)
			}
			if result == nil {
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("key %q not found", input.Key)}},
				}, nil, nil
			}
		} else {
			js := fmt.Sprintf(`(function() {
				var s = %s, r = {};
				for (var i = 0; i < s.length; i++) {
					var k = s.key(i);
					r[k] = s.getItem(k);
				}
				return r;
			})()`, storageObj)
			if err := chromedp.Run(actx, chromedp.Evaluate(js, &result)); err != nil {
				return nil, nil, fmt.Errorf("get_storage: %w", err)
			}
		}

		data, err := json.Marshal(result)
		if err != nil {
			return nil, nil, fmt.Errorf("get_storage: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_storage",
		Description: `Set a value in localStorage or sessionStorage. Type must be "local" or "session".`,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SetStorageInput) (*mcp.CallToolResult, any, error) {
		storageObj, err := storageType(input.Type)
		if err != nil {
			return nil, nil, fmt.Errorf("set_storage: %w", err)
		}

		actx := s.activeCtx()
		js := fmt.Sprintf(`%s.setItem(%q, %q)`, storageObj, input.Key, input.Value)
		if err := chromedp.Run(actx, chromedp.Evaluate(js, nil)); err != nil {
			return nil, nil, fmt.Errorf("set_storage: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("set %s[%q]", input.Type, input.Key)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "clear_storage",
		Description: `Clear localStorage or sessionStorage. Type must be "local" or "session". If key is provided, removes only that key; otherwise clears all.`,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ClearStorageInput) (*mcp.CallToolResult, any, error) {
		storageObj, err := storageType(input.Type)
		if err != nil {
			return nil, nil, fmt.Errorf("clear_storage: %w", err)
		}

		actx := s.activeCtx()
		var js string
		if input.Key != "" {
			js = fmt.Sprintf(`%s.removeItem(%q)`, storageObj, input.Key)
		} else {
			js = fmt.Sprintf(`%s.clear()`, storageObj)
		}
		if err := chromedp.Run(actx, chromedp.Evaluate(js, nil)); err != nil {
			return nil, nil, fmt.Errorf("clear_storage: %w", err)
		}

		if input.Key != "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("removed %s[%q]", input.Type, input.Key)}},
			}, nil, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("cleared %sStorage", input.Type)}},
		}, nil, nil
	})
}

func storageType(t string) (string, error) {
	switch t {
	case "local", "localStorage":
		return "localStorage", nil
	case "session", "sessionStorage":
		return "sessionStorage", nil
	default:
		return "", fmt.Errorf("type must be %q or %q, got %q", "local", "session", t)
	}
}
