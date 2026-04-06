package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// walkObjectJS is the JS function that recursively walks an object,
// producing a typed tree with cycle detection and depth limiting.
const walkObjectJS = `(expr, maxDepth, maxKeys, sampleValues) => {
	const seen = new WeakSet();
	function walk(obj, depth) {
		if (obj === null) return {_type: 'null'};
		if (obj === undefined) return {_type: 'undefined'};
		const t = typeof obj;
		if (t === 'boolean' || t === 'number' || t === 'bigint') {
			const r = {_type: t};
			if (sampleValues) r._value = String(obj);
			return r;
		}
		if (t === 'string') {
			const r = {_type: 'string', _length: obj.length};
			if (sampleValues) r._value = obj.length > 100 ? obj.slice(0, 100) + '...' : obj;
			return r;
		}
		if (t === 'symbol') return {_type: 'symbol', _value: obj.toString()};
		if (t === 'function') return {_type: 'function', _length: obj.length, _name: obj.name || '(anonymous)'};
		if (t !== 'object') return {_type: t};

		// Object or array.
		if (seen.has(obj)) return {_type: 'circular'};
		seen.add(obj);

		if (Array.isArray(obj)) {
			const r = {_type: 'array', _length: obj.length};
			if (depth < maxDepth && obj.length > 0) {
				r._items = [];
				const n = Math.min(obj.length, maxKeys);
				for (let i = 0; i < n; i++) {
					r._items.push(walk(obj[i], depth + 1));
				}
				if (obj.length > n) r._truncated = obj.length - n;
			}
			return r;
		}

		const keys = Object.keys(obj);
		const r = {_type: 'object', _keys: keys.length};
		if (obj.constructor && obj.constructor.name !== 'Object') {
			r._class = obj.constructor.name;
		}
		if (depth >= maxDepth) return r;

		const n = Math.min(keys.length, maxKeys);
		for (let i = 0; i < n; i++) {
			try {
				r[keys[i]] = walk(obj[keys[i]], depth + 1);
			} catch(e) {
				r[keys[i]] = {_type: 'error', _value: e.message};
			}
		}
		if (keys.length > n) r._truncated = keys.length - n;
		return r;
	}
	try {
		const target = eval(expr);
		return walk(target, 0);
	} catch(e) {
		return {_type: 'error', _value: e.message};
	}
}
`

type WalkObjectInput struct {
	Expression   string `json:"expression"`              // JS expression to evaluate
	Depth        int    `json:"depth,omitempty"`          // max recursion depth (default 2)
	MaxKeys      int    `json:"max_keys,omitempty"`       // max keys per object (default 20)
	SampleValues bool   `json:"sample_values,omitempty"`  // include primitive values
}

func registerWalkTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "walk_object",
		Description: `Recursively explore a JS object's structure. Returns a typed tree with key counts, function arities, string lengths, and optional sampled values. Handles cycles via WeakSet. Use depth (default 2) and max_keys (default 20) to control output size.`,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input WalkObjectInput) (*mcp.CallToolResult, any, error) {
		depth := input.Depth
		if depth <= 0 {
			depth = 2
		}
		maxKeys := input.MaxKeys
		if maxKeys <= 0 {
			maxKeys = 20
		}

		// Build a self-invoking wrapper that calls the walk function.
		js := fmt.Sprintf("(%s)(%q, %d, %d, %v)",
			walkObjectJS, input.Expression, depth, maxKeys, input.SampleValues)

		var result any
		if err := chromedp.Run(s.activeCtx(), chromedp.Evaluate(js, &result)); err != nil {
			return nil, nil, fmt.Errorf("walk_object: %w", err)
		}

		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return nil, nil, fmt.Errorf("walk_object: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})
}
