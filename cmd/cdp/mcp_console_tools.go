package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// consoleEntry represents a captured console message.
type consoleEntry struct {
	Type      string   `json:"type"`
	Text      string   `json:"text"`
	Timestamp string   `json:"timestamp"`
	Stack     string   `json:"stack,omitempty"`
	Args      []string `json:"args,omitempty"`
}

// errorEntry represents a captured JS exception.
type errorEntry struct {
	Text      string `json:"text"`
	Timestamp string `json:"timestamp"`
	URL       string `json:"url,omitempty"`
	Line      int64  `json:"line,omitempty"`
	Column    int64  `json:"column,omitempty"`
	Stack     string `json:"stack,omitempty"`
}

// consoleCollector captures console messages and exceptions.
type consoleCollector struct {
	mu         sync.Mutex
	messages   []consoleEntry
	errors     []errorEntry
	maxEntries int
}

func newConsoleCollector() *consoleCollector {
	return &consoleCollector{
		maxEntries: 1000,
	}
}

// handleEvent processes CDP events for console messages and exceptions.
func (c *consoleCollector) handleEvent(ev any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch e := ev.(type) {
	case *runtime.EventConsoleAPICalled:
		var args []string
		var textParts []string
		for _, arg := range e.Args {
			s := remoteObjectToString(arg)
			args = append(args, s)
			textParts = append(textParts, s)
		}
		ts := ""
		if e.Timestamp != nil {
			ts = time.Time(*e.Timestamp).Format(time.RFC3339Nano)
		}
		var stack string
		if e.StackTrace != nil {
			stack = formatStackTrace(e.StackTrace)
		}
		entry := consoleEntry{
			Type:      string(e.Type),
			Text:      strings.Join(textParts, " "),
			Timestamp: ts,
			Stack:     stack,
			Args:      args,
		}
		if len(c.messages) >= c.maxEntries {
			c.messages = c.messages[1:]
		}
		c.messages = append(c.messages, entry)

	case *runtime.EventExceptionThrown:
		if e.ExceptionDetails == nil {
			return
		}
		d := e.ExceptionDetails
		text := d.Text
		if d.Exception != nil {
			if desc := d.Exception.Description; desc != "" {
				text = desc
			}
		}
		ts := ""
		if e.Timestamp != nil {
			ts = time.Time(*e.Timestamp).Format(time.RFC3339Nano)
		}
		var stack string
		if d.StackTrace != nil {
			stack = formatStackTrace(d.StackTrace)
		}
		entry := errorEntry{
			Text:      text,
			Timestamp: ts,
			URL:       d.URL,
			Line:      d.LineNumber,
			Column:    d.ColumnNumber,
			Stack:     stack,
		}
		if len(c.errors) >= c.maxEntries {
			c.errors = c.errors[1:]
		}
		c.errors = append(c.errors, entry)
	}
}

func (c *consoleCollector) getMessages(typeFilter string, limit int) []consoleEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	var result []consoleEntry
	for _, m := range c.messages {
		if typeFilter != "" && m.Type != typeFilter {
			continue
		}
		result = append(result, m)
	}
	if limit > 0 && len(result) > limit {
		result = result[len(result)-limit:]
	}
	return result
}

func (c *consoleCollector) getErrors(limit int) []errorEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]errorEntry, len(c.errors))
	copy(result, c.errors)
	if limit > 0 && len(result) > limit {
		result = result[len(result)-limit:]
	}
	return result
}

func (c *consoleCollector) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = nil
	c.errors = nil
}

func remoteObjectToString(obj *runtime.RemoteObject) string {
	if obj.Value != nil {
		var s string
		if err := json.Unmarshal(obj.Value, &s); err == nil {
			return s
		}
		return string(obj.Value)
	}
	if obj.Description != "" {
		return obj.Description
	}
	return string(obj.Type)
}

func formatStackTrace(st *runtime.StackTrace) string {
	var b strings.Builder
	for _, f := range st.CallFrames {
		fmt.Fprintf(&b, "  at %s (%s:%d:%d)\n", f.FunctionName, f.URL, f.LineNumber, f.ColumnNumber)
	}
	return b.String()
}

// --- MCP tool registration ---

type GetConsoleInput struct {
	Type  string `json:"type,omitempty"`
	Limit int    `json:"limit,omitempty"`
	Index int    `json:"index,omitempty"`
	Clear bool   `json:"clear,omitempty"`
}

type GetErrorsInput struct {
	Limit int  `json:"limit,omitempty"`
	Clear bool `json:"clear,omitempty"`
}

func registerConsoleTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_console",
		Description: "Get captured console messages. Filter by type (log, warn, error, info, debug). Use index (1-based) to get a single message. Returns most recent entries.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetConsoleInput) (*mcp.CallToolResult, any, error) {
		if s.console == nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "console capture not active"}},
			}, nil, nil
		}
		messages := s.console.getMessages(input.Type, 0)
		if input.Index > 0 {
			if input.Index > len(messages) {
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("index %d out of range (have %d messages)", input.Index, len(messages))}},
				}, nil, nil
			}
			messages = messages[input.Index-1 : input.Index]
		} else if input.Limit > 0 && len(messages) > input.Limit {
			messages = messages[len(messages)-input.Limit:]
		}
		if input.Clear {
			s.console.clear()
		}
		if len(messages) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "no console messages"}},
			}, nil, nil
		}
		data, err := json.Marshal(messages)
		if err != nil {
			return nil, nil, fmt.Errorf("get_console: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_errors",
		Description: "Get captured JavaScript exceptions and errors. Returns most recent entries.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetErrorsInput) (*mcp.CallToolResult, any, error) {
		if s.console == nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "console capture not active"}},
			}, nil, nil
		}
		errors := s.console.getErrors(input.Limit)
		if input.Clear {
			s.console.clear()
		}
		if len(errors) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "no errors"}},
			}, nil, nil
		}
		data, err := json.Marshal(errors)
		if err != nil {
			return nil, nil, fmt.Errorf("get_errors: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})
}

// enableConsoleCapture starts capturing console messages on the browser context.
func enableConsoleCapture(browserCtx context.Context) *consoleCollector {
	cc := newConsoleCollector()
	chromedp.ListenTarget(browserCtx, cc.handleEvent)
	// Runtime events are enabled by default in chromedp.
	return cc
}
