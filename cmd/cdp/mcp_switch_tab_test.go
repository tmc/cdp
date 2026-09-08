package main

import (
	"context"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tmc/cdp/internal/chromedp"
)

func TestSwitchTabKeepsEventLoop(t *testing.T) {
	controller, _, _, ids := targetLifecycleBrowser(t)
	s := &mcpSession{ctx: controller, browserCtx: controller}
	defer func() {
		if s.tabCancel != nil {
			s.tabCancel()
		}
	}()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	registerTabTools(server, s)
	a, b := mcp.NewInMemoryTransports()
	ss, err := server.Connect(t.Context(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := client.Connect(t.Context(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	for _, id := range ids {
		result, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "switch_tab", Arguments: map[string]any{"id": string(id)}})
		if err != nil || result.IsError {
			t.Fatalf("switch_tab: result=%+v err=%v", result, err)
		}
		ctx, cancel := context.WithTimeout(s.activeCtx(), 2*time.Second)
		var value int
		err = chromedp.Run(ctx, chromedp.Evaluate("21 * 2", &value))
		cancel()
		if err != nil || value != 42 {
			t.Fatalf("evaluation after switch: value=%d err=%v", value, err)
		}
	}
	s.tabCancel()
	requireTargets(t, controller, ids, true)
}
