package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- Dialog handling ---

// dialogInfo represents a pending or past JS dialog.
type dialogInfo struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	URL     string `json:"url,omitempty"`
	Default string `json:"default_prompt,omitempty"`
	Handled bool   `json:"handled"`
	Action  string `json:"action,omitempty"`
}

// dialogCollector accumulates JS dialog events.
type dialogCollector struct {
	mu      sync.Mutex
	pending *dialogInfo // most recent unhandled dialog
	history []dialogInfo
}

func newDialogCollector() *dialogCollector {
	return &dialogCollector{}
}

func (d *dialogCollector) handleEvent(ev any) {
	e, ok := ev.(*page.EventJavascriptDialogOpening)
	if !ok {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	info := dialogInfo{
		Type:    string(e.Type),
		Message: e.Message,
		URL:     e.URL,
		Default: e.DefaultPrompt,
	}
	d.pending = &info
	d.history = append(d.history, info)
}

func (d *dialogCollector) getPending() *dialogInfo {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.pending
}

func (d *dialogCollector) markHandled(action string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pending != nil {
		d.pending.Handled = true
		d.pending.Action = action
		d.pending = nil
	}
}

func (d *dialogCollector) getHistory() []dialogInfo {
	d.mu.Lock()
	defer d.mu.Unlock()
	result := make([]dialogInfo, len(d.history))
	copy(result, d.history)
	return result
}

// --- MCP tool registration ---

type HandleDialogInput struct {
	Accept     bool   `json:"accept"`
	PromptText string `json:"prompt_text,omitempty"`
}

func registerDialogTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "handle_dialog",
		Description: "Handle a pending JavaScript dialog (alert, confirm, prompt, beforeunload). Accept or dismiss, optionally providing prompt text.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input HandleDialogInput) (*mcp.CallToolResult, any, error) {
		if s.dialogs == nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "dialog handling not active"}},
			}, nil, nil
		}

		pending := s.dialogs.getPending()
		if pending == nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "no pending dialog"}},
			}, nil, nil
		}

		actx := s.activeCtx()
		cmd := page.HandleJavaScriptDialog(input.Accept)
		if input.PromptText != "" {
			cmd = cmd.WithPromptText(input.PromptText)
		}

		if err := chromedp.Run(actx, chromedp.ActionFunc(func(ctx context.Context) error {
			return cmd.Do(ctx)
		})); err != nil {
			return nil, nil, fmt.Errorf("handle_dialog: %w", err)
		}

		action := "dismissed"
		if input.Accept {
			action = "accepted"
		}
		s.dialogs.markHandled(action)

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("%s %s dialog: %s", action, pending.Type, pending.Message)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_dialogs",
		Description: "Get the history of JavaScript dialogs (alert, confirm, prompt) that have appeared, and any currently pending dialog.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
		if s.dialogs == nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "dialog handling not active"}},
			}, nil, nil
		}

		type output struct {
			Pending *dialogInfo  `json:"pending,omitempty"`
			History []dialogInfo `json:"history"`
		}
		out := output{
			Pending: s.dialogs.getPending(),
			History: s.dialogs.getHistory(),
		}
		data, err := json.Marshal(out)
		if err != nil {
			return nil, nil, fmt.Errorf("get_dialogs: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})
}

// enableDialogCapture starts listening for JS dialog events.
func enableDialogCapture(browserCtx context.Context) *dialogCollector {
	dc := newDialogCollector()
	chromedp.ListenTarget(browserCtx, dc.handleEvent)
	return dc
}
