package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ndpSession holds V8 state shared across MCP tool handlers.
type ndpSession struct {
	mu       sync.Mutex
	client   *V8InspectorClient
	runtime  *V8Runtime
	debugger *V8Debugger
	profiler *V8Profiler

	// Console capture
	consoleMsgs []consoleMsg
	consoleErrs []consoleErr

	// Coverage state
	coverageRunning bool
}

type consoleMsg struct {
	Type      string `json:"type"`
	Text      string `json:"text"`
	Timestamp string `json:"timestamp"`
}

type consoleErr struct {
	Text      string `json:"text"`
	URL       string `json:"url,omitempty"`
	Line      int    `json:"line,omitempty"`
	Column    int    `json:"column,omitempty"`
	Timestamp string `json:"timestamp"`
}

type mcpConfig struct {
	NodePort string
	Verbose  bool
}

func runMCP(cfg mcpConfig) error {
	log.SetOutput(os.Stderr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Connect to Node.js inspector.
	client := NewV8InspectorClient("localhost", cfg.NodePort, cfg.Verbose)

	if err := client.ConnectByPort(ctx, cfg.NodePort); err != nil {
		return fmt.Errorf("connect to Node.js on port %s: %w", cfg.NodePort, err)
	}
	log.Printf("Connected to Node.js on port %s", cfg.NodePort)

	rt := NewV8Runtime(client)
	dbg := NewV8Debugger(client)
	prof := NewV8Profiler(client)

	// Enable domains.
	if err := rt.EnableRuntime(); err != nil {
		log.Printf("warning: enable runtime: %v", err)
	}
	if err := dbg.EnableDebugger(); err != nil {
		log.Printf("warning: enable debugger: %v", err)
	}

	session := &ndpSession{
		client:   client,
		runtime:  rt,
		debugger: dbg,
		profiler: prof,
	}

	// Listen for console and exception events.
	client.OnEvent("Runtime.consoleAPICalled", func(params map[string]interface{}) {
		session.mu.Lock()
		defer session.mu.Unlock()
		msg := consoleMsg{
			Timestamp: time.Now().Format(time.RFC3339Nano),
		}
		if t, ok := params["type"].(string); ok {
			msg.Type = t
		}
		if args, ok := params["args"].([]interface{}); ok {
			var parts []string
			for _, a := range args {
				if m, ok := a.(map[string]interface{}); ok {
					if v, ok := m["value"]; ok {
						parts = append(parts, fmt.Sprintf("%v", v))
					} else if desc, ok := m["description"].(string); ok {
						parts = append(parts, desc)
					} else if typ, ok := m["type"].(string); ok {
						parts = append(parts, typ)
					}
				}
			}
			for i, p := range parts {
				if i > 0 {
					msg.Text += " "
				}
				msg.Text += p
			}
		}
		if len(session.consoleMsgs) >= 1000 {
			session.consoleMsgs = session.consoleMsgs[1:]
		}
		session.consoleMsgs = append(session.consoleMsgs, msg)
	})

	client.OnEvent("Runtime.exceptionThrown", func(params map[string]interface{}) {
		session.mu.Lock()
		defer session.mu.Unlock()
		e := consoleErr{
			Timestamp: time.Now().Format(time.RFC3339Nano),
		}
		if details, ok := params["exceptionDetails"].(map[string]interface{}); ok {
			if text, ok := details["text"].(string); ok {
				e.Text = text
			}
			if url, ok := details["url"].(string); ok {
				e.URL = url
			}
			if line, ok := details["lineNumber"].(float64); ok {
				e.Line = int(line)
			}
			if col, ok := details["columnNumber"].(float64); ok {
				e.Column = int(col)
			}
			// Try to get better description from exception object.
			if exc, ok := details["exception"].(map[string]interface{}); ok {
				if desc, ok := exc["description"].(string); ok {
					e.Text = desc
				}
			}
		}
		if len(session.consoleErrs) >= 1000 {
			session.consoleErrs = session.consoleErrs[1:]
		}
		session.consoleErrs = append(session.consoleErrs, e)
	})

	// Create MCP server.
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "ndp",
		Version: "0.1.0",
	}, &mcp.ServerOptions{})

	registerNDPTools(server, session)

	log.Printf("ndp MCP server ready")
	transport := &mcp.StdioTransport{}
	return server.Run(ctx, transport)
}

// formatResult converts an EvaluationResult to a display string.
func formatResult(res *EvaluationResult) string {
	if res == nil {
		return "undefined"
	}
	if res.Exception != nil {
		return fmt.Sprintf("Error: %s", res.Exception.Text)
	}
	if res.Result == nil {
		return "undefined"
	}
	data, err := json.Marshal(res.Result.Value)
	if err != nil {
		if res.Result.Description != "" {
			return res.Result.Description
		}
		return res.Result.Type
	}
	return string(data)
}
