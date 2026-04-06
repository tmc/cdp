package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- Electron IPC sniffer tools ---

// startIPCLogJS is the JS that monkey-patches known Electron IPC bridges
// to log all outgoing and incoming messages.
const startIPCLogJS = `(() => {
	if (window.__ipcLog) return {already_active: true, count: window.__ipcLog.length};
	window.__ipcLog = [];
	const ts = () => new Date().toISOString();
	const push = (dir, ch, args) => {
		window.__ipcLog.push({timestamp: ts(), direction: dir, channel: ch, args: JSON.parse(JSON.stringify(args, (k,v) => {
			if (typeof v === 'function') return '[function]';
			if (v instanceof Error) return v.message;
			if (typeof v === 'object' && v !== null && v.constructor && v.constructor.name !== 'Object' && v.constructor.name !== 'Array')
				return '[' + v.constructor.name + ']';
			return v;
		}))});
	};

	// Patch contextBridge-exposed objects (electronBridge, vscode, etc.)
	const bridges = [
		{obj: window.electronBridge, name: 'electronBridge'},
		{obj: window.vscode, name: 'vscode'},
		{obj: window.electron, name: 'electron'},
	];
	let patched = 0;
	for (const {obj, name} of bridges) {
		if (!obj) continue;
		for (const key of Object.keys(obj)) {
			if (typeof obj[key] === 'function') {
				const orig = obj[key].bind(obj);
				obj[key] = function(...args) {
					push('out', name + '.' + key, args);
					return orig(...args);
				};
				patched++;
			}
		}
	}

	// Patch ipcRenderer if exposed (nodeIntegration or preload leak).
	const ipc = window.require && (() => { try { return window.require('electron').ipcRenderer } catch(e) { return null } })();
	if (ipc) {
		const origSend = ipc.send.bind(ipc);
		const origInvoke = ipc.invoke.bind(ipc);
		ipc.send = function(ch, ...args) { push('out', ch, args); return origSend(ch, ...args); };
		ipc.invoke = function(ch, ...args) { push('out', ch, args); return origInvoke(ch, ...args); };
		const origOn = ipc.on.bind(ipc);
		ipc.on = function(ch, fn) {
			return origOn(ch, (ev, ...args) => { push('in', ch, args); return fn(ev, ...args); });
		};
		patched++;
	}

	// Listen for postMessage-based IPC.
	window.addEventListener('message', (ev) => {
		if (ev.source === window) {
			push('in', 'window.postMessage', [ev.data]);
		}
	});

	return {started: true, patched: patched};
})()
`

// getIPCLogJS returns and optionally clears the captured IPC log.
// %s is replaced with channel filter or empty string, %v with clear bool.
const getIPCLogJSTmpl = `(() => {
	if (!window.__ipcLog) return {error: 'not started'};
	let entries = window.__ipcLog;
	const filter = %q;
	if (filter) {
		entries = entries.filter(e => e.channel.includes(filter));
	}
	if (%v) {
		window.__ipcLog = [];
	}
	return entries;
})()
`

type StartIPCLogInput struct{}

type GetIPCLogInput struct {
	Channel string `json:"channel,omitempty"` // filter by channel substring
	Clear   bool   `json:"clear,omitempty"`   // clear log after reading
	Limit   int    `json:"limit,omitempty"`   // max entries to return
}

func registerIPCTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "start_ipc_log",
		Description: `Start capturing Electron IPC messages. Monkey-patches known bridges (electronBridge, vscode, electron) and postMessage listener. Call get_ipc_log to read captured messages.`,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input StartIPCLogInput) (*mcp.CallToolResult, any, error) {
		var result any
		if err := chromedp.Run(s.activeCtx(), chromedp.Evaluate(startIPCLogJS, &result)); err != nil {
			return nil, nil, fmt.Errorf("start_ipc_log: %w", err)
		}
		data, _ := json.Marshal(result)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "get_ipc_log",
		Description: `Get captured IPC messages from start_ipc_log. Optional channel filter (substring match). Set clear=true to reset the log after reading.`,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetIPCLogInput) (*mcp.CallToolResult, any, error) {
		js := fmt.Sprintf(getIPCLogJSTmpl, input.Channel, input.Clear)
		var result any
		if err := chromedp.Run(s.activeCtx(), chromedp.Evaluate(js, &result)); err != nil {
			return nil, nil, fmt.Errorf("get_ipc_log: %w", err)
		}

		// Apply limit if needed.
		if input.Limit > 0 {
			if arr, ok := result.([]any); ok && len(arr) > input.Limit {
				result = arr[len(arr)-input.Limit:]
			}
		}

		data, err := json.Marshal(result)
		if err != nil {
			return nil, nil, fmt.Errorf("get_ipc_log: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})
}
