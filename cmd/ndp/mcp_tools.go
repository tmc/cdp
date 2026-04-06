package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ndpWalkObjectJS is the JS function that recursively walks an object,
// producing a typed tree with cycle detection and depth limiting.
// This is the same logic as cdp's walkObjectJS but kept as a separate
// constant since the two binaries don't share code.
const ndpWalkObjectJS = `(expr, maxDepth, maxKeys, sampleValues) => {
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
		if (seen.has(obj)) return {_type: 'circular'};
		seen.add(obj);
		if (Array.isArray(obj)) {
			const r = {_type: 'array', _length: obj.length};
			if (depth < maxDepth && obj.length > 0) {
				r._items = [];
				const n = Math.min(obj.length, maxKeys);
				for (let i = 0; i < n; i++) r._items.push(walk(obj[i], depth + 1));
				if (obj.length > n) r._truncated = obj.length - n;
			}
			return r;
		}
		const keys = Object.keys(obj);
		const r = {_type: 'object', _keys: keys.length};
		if (obj.constructor && obj.constructor.name !== 'Object') r._class = obj.constructor.name;
		if (depth >= maxDepth) return r;
		const n = Math.min(keys.length, maxKeys);
		for (let i = 0; i < n; i++) {
			try { r[keys[i]] = walk(obj[keys[i]], depth + 1); }
			catch(e) { r[keys[i]] = {_type: 'error', _value: e.message}; }
		}
		if (keys.length > n) r._truncated = keys.length - n;
		return r;
	}
	try { return walk(eval(expr), 0); }
	catch(e) { return {_type: 'error', _value: e.message}; }
}
`

// --- Input types ---

type EvaluateInput struct {
	Expression   string `json:"expression"`
	AwaitPromise bool   `json:"await_promise,omitempty"`
}

type ListSourcesInput struct {
	Filter string `json:"filter,omitempty"` // substring match on URL
}

type ReadSourceInput struct {
	ScriptID string `json:"script_id,omitempty"`
	URL      string `json:"url,omitempty"`
}

type SearchSourcesInput struct {
	Query         string `json:"query"`
	CaseSensitive bool   `json:"case_sensitive,omitempty"`
	IsRegex       bool   `json:"is_regex,omitempty"`
}

type GetConsoleInput struct {
	Limit int  `json:"limit,omitempty"`
	Clear bool `json:"clear,omitempty"`
}

type GetErrorsInput struct {
	Limit int  `json:"limit,omitempty"`
	Clear bool `json:"clear,omitempty"`
}

type StartCoverageInput struct{}
type StopCoverageInput struct{}
type GetCoverageInput struct{}
type CoverageSnapshotInput struct {
	Name string `json:"name,omitempty"`
}

type WalkObjectInput struct {
	Expression   string `json:"expression"`             // JS expression to evaluate
	Depth        int    `json:"depth,omitempty"`         // max recursion depth (default 2)
	MaxKeys      int    `json:"max_keys,omitempty"`      // max keys per object (default 20)
	SampleValues bool   `json:"sample_values,omitempty"` // include primitive values
}

type StartCPUProfileInput struct {
	SamplingInterval int `json:"sampling_interval,omitempty"`
}

type StopCPUProfileInput struct{}

type TakeHeapSnapshotInput struct{}

func registerNDPTools(server *mcp.Server, s *ndpSession) {
	// --- Runtime tools ---

	mcp.AddTool(server, &mcp.Tool{
		Name:        "evaluate",
		Description: "Evaluate a JavaScript expression in the Node.js runtime. Set await_promise to true for async expressions.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input EvaluateInput) (*mcp.CallToolResult, any, error) {
		opts := &EvaluateOptions{
			IncludeCommandLineAPI: true,
			ReturnByValue:        true,
			AwaitPromise:         input.AwaitPromise,
		}
		result, err := s.runtime.Evaluate(input.Expression, opts)
		if err != nil {
			return nil, nil, fmt.Errorf("evaluate: %w", err)
		}
		text := formatResult(result)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: text}},
		}, nil, nil
	})

	// --- Source tools ---

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_sources",
		Description: "List all loaded JavaScript sources (scripts). Optionally filter by URL substring.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListSourcesInput) (*mcp.CallToolResult, any, error) {
		scripts := s.client.Scripts()
		var result []*V8Script
		for _, sc := range scripts {
			if input.Filter != "" && !strings.Contains(sc.URL, input.Filter) {
				continue
			}
			result = append(result, sc)
		}
		data, err := json.Marshal(result)
		if err != nil {
			return nil, nil, fmt.Errorf("list_sources: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "read_source",
		Description: "Read the source code of a loaded script by script ID or URL.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ReadSourceInput) (*mcp.CallToolResult, any, error) {
		scriptID := input.ScriptID
		if scriptID == "" && input.URL != "" {
			// Find script by URL.
			for _, sc := range s.client.Scripts() {
				if sc.URL == input.URL || strings.Contains(sc.URL, input.URL) {
					scriptID = sc.ScriptID
					break
				}
			}
			if scriptID == "" {
				return nil, nil, fmt.Errorf("read_source: no script matching URL %q", input.URL)
			}
		}
		if scriptID == "" {
			return nil, nil, fmt.Errorf("read_source: provide script_id or url")
		}
		source, err := s.debugger.GetScriptSource(scriptID)
		if err != nil {
			return nil, nil, fmt.Errorf("read_source: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: source}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_sources",
		Description: "Search across all loaded scripts for a query string or regex.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SearchSourcesInput) (*mcp.CallToolResult, any, error) {
		type searchHit struct {
			ScriptID string                   `json:"script_id"`
			URL      string                   `json:"url"`
			Matches  []map[string]interface{} `json:"matches"`
		}
		var results []searchHit
		for _, sc := range s.client.Scripts() {
			matches, err := s.debugger.SearchInContent(sc.ScriptID, input.Query, input.CaseSensitive, input.IsRegex)
			if err != nil {
				continue
			}
			if len(matches) > 0 {
				results = append(results, searchHit{
					ScriptID: sc.ScriptID,
					URL:      sc.URL,
					Matches:  matches,
				})
			}
		}
		data, err := json.Marshal(results)
		if err != nil {
			return nil, nil, fmt.Errorf("search_sources: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	// --- Console tools ---

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_console",
		Description: "Get captured console messages from the Node.js process.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetConsoleInput) (*mcp.CallToolResult, any, error) {
		s.mu.Lock()
		msgs := make([]consoleMsg, len(s.consoleMsgs))
		copy(msgs, s.consoleMsgs)
		if input.Clear {
			s.consoleMsgs = nil
		}
		s.mu.Unlock()

		if input.Limit > 0 && len(msgs) > input.Limit {
			msgs = msgs[len(msgs)-input.Limit:]
		}
		if len(msgs) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "no console messages"}},
			}, nil, nil
		}
		data, err := json.Marshal(msgs)
		if err != nil {
			return nil, nil, fmt.Errorf("get_console: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_errors",
		Description: "Get captured JavaScript exceptions from the Node.js process.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetErrorsInput) (*mcp.CallToolResult, any, error) {
		s.mu.Lock()
		errs := make([]consoleErr, len(s.consoleErrs))
		copy(errs, s.consoleErrs)
		if input.Clear {
			s.consoleErrs = nil
		}
		s.mu.Unlock()

		if input.Limit > 0 && len(errs) > input.Limit {
			errs = errs[len(errs)-input.Limit:]
		}
		if len(errs) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "no errors"}},
			}, nil, nil
		}
		data, err := json.Marshal(errs)
		if err != nil {
			return nil, nil, fmt.Errorf("get_errors: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	// --- Coverage tools ---

	mcp.AddTool(server, &mcp.Tool{
		Name:        "start_coverage",
		Description: "Start collecting JavaScript code coverage in the Node.js process.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input StartCoverageInput) (*mcp.CallToolResult, any, error) {
		cc := s.coverageCollector
		if cc.Running() {
			cc.Stop()
		}
		if err := cc.Start(); err != nil {
			return nil, nil, fmt.Errorf("start_coverage: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "coverage collection started"}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "stop_coverage",
		Description: "Stop collecting JavaScript code coverage.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input StopCoverageInput) (*mcp.CallToolResult, any, error) {
		cc := s.coverageCollector
		if !cc.Running() {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "coverage not running"}},
			}, nil, nil
		}
		if err := cc.Stop(); err != nil {
			return nil, nil, fmt.Errorf("stop_coverage: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "coverage collection stopped"}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_coverage",
		Description: "Get current code coverage data. Returns per-file line coverage with hit counts.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetCoverageInput) (*mcp.CallToolResult, any, error) {
		cc := s.coverageCollector
		if !cc.Running() {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "coverage not running — call start_coverage first"}},
			}, nil, nil
		}
		snap, err := cc.TakeSnapshot("latest")
		if err != nil {
			return nil, nil, fmt.Errorf("get_coverage: %w", err)
		}
		data, err := json.Marshal(snap.Summary())
		if err != nil {
			return nil, nil, fmt.Errorf("get_coverage: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "coverage_snapshot",
		Description: "Take a named coverage snapshot for later comparison. Coverage must be running.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CoverageSnapshotInput) (*mcp.CallToolResult, any, error) {
		cc := s.coverageCollector
		if !cc.Running() {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "coverage not running — call start_coverage first"}},
			}, nil, nil
		}
		name := input.Name
		if name == "" {
			name = fmt.Sprintf("snap-%d", len(cc.Snapshots())+1)
		}
		snap, err := cc.TakeSnapshot(name)
		if err != nil {
			return nil, nil, fmt.Errorf("coverage_snapshot: %w", err)
		}
		summary := snap.Summary()
		data, err := json.Marshal(summary)
		if err != nil {
			return nil, nil, fmt.Errorf("coverage_snapshot: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("snapshot %q: %d files\n%s", name, len(summary), string(data))}},
		}, nil, nil
	})

	// --- Profiler tools ---

	mcp.AddTool(server, &mcp.Tool{
		Name:        "start_cpu_profile",
		Description: "Start CPU profiling. Optionally set sampling_interval in microseconds.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input StartCPUProfileInput) (*mcp.CallToolResult, any, error) {
		if err := s.profiler.EnableProfiler(); err != nil {
			return nil, nil, fmt.Errorf("start_cpu_profile: enable: %w", err)
		}
		if input.SamplingInterval > 0 {
			s.profiler.SetSamplingInterval(input.SamplingInterval)
		}
		if err := s.profiler.StartCPUProfiling("mcp", 0); err != nil {
			return nil, nil, fmt.Errorf("start_cpu_profile: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "CPU profiling started"}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "stop_cpu_profile",
		Description: "Stop CPU profiling and return the profile data.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input StopCPUProfileInput) (*mcp.CallToolResult, any, error) {
		profile, err := s.profiler.StopCPUProfiling()
		if err != nil {
			return nil, nil, fmt.Errorf("stop_cpu_profile: %w", err)
		}
		data, err := json.Marshal(profile)
		if err != nil {
			return nil, nil, fmt.Errorf("stop_cpu_profile: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "take_heap_snapshot",
		Description: "Take a heap snapshot for memory analysis.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input TakeHeapSnapshotInput) (*mcp.CallToolResult, any, error) {
		if err := s.profiler.EnableHeapProfiler(); err != nil {
			return nil, nil, fmt.Errorf("take_heap_snapshot: enable: %w", err)
		}
		if err := s.profiler.TakeHeapSnapshot(false); err != nil {
			return nil, nil, fmt.Errorf("take_heap_snapshot: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "heap snapshot taken"}},
		}, nil, nil
	})

	// --- Connection tools ---

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_targets",
		Description: "List available Node.js debugging targets.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
		targets, err := s.client.DiscoverTargets(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list_targets: %w", err)
		}
		data, err := json.Marshal(targets)
		if err != nil {
			return nil, nil, fmt.Errorf("list_targets: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	// --- Electron detection ---

	mcp.AddTool(server, &mcp.Tool{
		Name:        "detect_electron",
		Description: "Check if the connected process is an Electron app. Works in both main process (full info) and renderer (nodeIntegration disabled, uses User-Agent fallback).",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
		info := detectElectron(s)
		data, err := json.Marshal(info)
		if err != nil {
			return nil, nil, fmt.Errorf("detect_electron: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	// --- Object inspection ---

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

		js := fmt.Sprintf("(%s)(%q, %d, %d, %v)",
			ndpWalkObjectJS, input.Expression, depth, maxKeys, input.SampleValues)

		opts := &EvaluateOptions{
			IncludeCommandLineAPI: true,
			ReturnByValue:        true,
		}
		result, err := s.runtime.Evaluate(js, opts)
		if err != nil {
			return nil, nil, fmt.Errorf("walk_object: %w", err)
		}

		text := formatResult(result)
		// Try to pretty-print if it's valid JSON.
		var parsed any
		if json.Unmarshal([]byte(text), &parsed) == nil {
			if pretty, err := json.MarshalIndent(parsed, "", "  "); err == nil {
				text = string(pretty)
			}
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: text}},
		}, nil, nil
	})
}

// electronInfo holds the result of Electron detection.
type electronInfo struct {
	IsElectron      bool   `json:"is_electron"`
	ElectronVersion string `json:"electron_version,omitempty"`
	ChromeVersion   string `json:"chrome_version,omitempty"`
	NodeVersion     string `json:"node_version,omitempty"`
	ProcessType     string `json:"process_type,omitempty"`
	AppName         string `json:"app_name,omitempty"`
	AppPath         string `json:"app_path,omitempty"`
	DetectionMethod string `json:"detection_method,omitempty"`
	Note            string `json:"note,omitempty"`
}

var electronUARegexp = regexp.MustCompile(`Electron/(\S+)`)

// detectElectron tries multiple strategies to identify an Electron process.
func detectElectron(s *ndpSession) *electronInfo {
	// Strategy 1: evaluate process.versions (works when nodeIntegration is on).
	expr := `JSON.stringify({
		electron: typeof process !== 'undefined' && process.versions ? (process.versions.electron || null) : null,
		chrome: typeof process !== 'undefined' && process.versions ? (process.versions.chrome || null) : null,
		node: typeof process !== 'undefined' && process.versions ? (process.versions.node || null) : null,
		type: typeof process !== 'undefined' ? (process.type || null) : null,
		appName: (function() { try { return require('electron').app.getName() } catch(e) { return null } })(),
		appPath: (function() { try { return require('electron').app.getAppPath() } catch(e) { return null } })()
	})`
	result, err := s.runtime.Evaluate(expr, &EvaluateOptions{ReturnByValue: true})
	if err == nil && result != nil && result.Exception == nil && result.Result != nil {
		text := formatResult(result)
		var parsed map[string]interface{}
		if json.Unmarshal([]byte(text), &parsed) == nil {
			if v, _ := parsed["electron"].(string); v != "" {
				info := &electronInfo{
					IsElectron:      true,
					ElectronVersion: v,
					DetectionMethod: "process.versions",
				}
				if s, _ := parsed["chrome"].(string); s != "" {
					info.ChromeVersion = s
				}
				if s, _ := parsed["node"].(string); s != "" {
					info.NodeVersion = s
				}
				if s, _ := parsed["type"].(string); s != "" {
					info.ProcessType = s
				}
				if s, _ := parsed["appName"].(string); s != "" {
					info.AppName = s
				}
				if s, _ := parsed["appPath"].(string); s != "" {
					info.AppPath = s
				}
				return info
			}
		}
	}

	// Strategy 2: check /json/version User-Agent for "Electron/X.X.X".
	if info := detectElectronFromUA(s.client); info != nil {
		return info
	}

	// Strategy 3: check if any loaded script URL uses app:// protocol.
	for _, sc := range s.client.Scripts() {
		if strings.HasPrefix(sc.URL, "app://") {
			return &electronInfo{
				IsElectron:      true,
				ProcessType:     "renderer (nodeIntegration disabled)",
				DetectionMethod: "app:// protocol",
				Note:            "detected via app:// URL scheme; main process APIs unavailable",
			}
		}
	}

	return &electronInfo{IsElectron: false, DetectionMethod: "none", Note: "not an Electron process"}
}

// detectElectronFromUA fetches /json/version and checks the User-Agent.
func detectElectronFromUA(client *V8InspectorClient) *electronInfo {
	url := fmt.Sprintf("http://%s:%s/json/version", client.host, client.port)
	resp, err := http.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var versionInfo map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&versionInfo); err != nil {
		return nil
	}

	// Check "Browser" field (e.g. "Chrome/120.0.6099.291 Electron/28.1.0").
	browser := versionInfo["Browser"]
	if m := electronUARegexp.FindStringSubmatch(browser); len(m) > 1 {
		return &electronInfo{
			IsElectron:      true,
			ElectronVersion: m[1],
			ProcessType:     "renderer (nodeIntegration disabled)",
			DetectionMethod: "User-Agent",
			Note:            "detected via /json/version User-Agent; main process APIs unavailable",
		}
	}

	// Check "User-Agent" field as well.
	ua := versionInfo["User-Agent"]
	if m := electronUARegexp.FindStringSubmatch(ua); len(m) > 1 {
		return &electronInfo{
			IsElectron:      true,
			ElectronVersion: m[1],
			ProcessType:     "renderer (nodeIntegration disabled)",
			DetectionMethod: "User-Agent",
			Note:            "detected via /json/version User-Agent; main process APIs unavailable",
		}
	}

	return nil
}
