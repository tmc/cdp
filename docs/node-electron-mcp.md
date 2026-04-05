# Node.js & Electron MCP Tools via ndp

## Context

cdp has 74 MCP tools for browser automation. We want the same coverage/source/profiling capabilities for Node.js and Electron apps. The project already has `cmd/ndp/` (~10K lines) with a working V8 Inspector client, profiler (with precise coverage), debugger, and REPL — but no MCP integration.

The coverage API was just refactored to use a `coverageProvider` interface, making it shareable across cdp and ndp.

**Key constraint**: chromedp can't connect to Node.js `--inspect` because Node lacks the CDP Target domain. ndp's `V8InspectorClient` uses raw WebSocket and handles this correctly. We should reuse it, not fight chromedp.

## Approach: Add MCP server to ndp

Two MCP servers for two target types: `cdp --mcp` for browsers, `ndp --mcp` for Node/Electron. An LLM client connects to both simultaneously. This follows the Unix philosophy and avoids a massive refactor of cdp's 74 tools.

### Phase 1: ndp MCP Server (~15 tools)

**New files:**

1. **`cmd/ndp/mcp.go`** — Session struct + server setup
   - `ndpSession` struct holding `*V8InspectorClient`, `*V8Runtime`, `*V8Debugger`, `*V8Profiler`, console buffer, script cache
   - Implement `coverageProvider` interface so coverage API can be shared
   - `runMCP(cfg)` — connect to Node via V8InspectorClient, enable domains, register tools, serve on stdio
   - Event handlers for `Debugger.scriptParsed`, `Runtime.consoleAPICalled`, `Runtime.exceptionThrown`

2. **`cmd/ndp/mcp_tools.go`** — V8-compatible MCP tool registration
   - **Runtime**: `evaluate`, `evaluate_async`
   - **Coverage**: `start_coverage`, `stop_coverage`, `get_coverage`, `coverage_snapshot`
   - **Sources**: `list_sources`, `read_source`, `search_sources`
   - **Profiler**: `start_cpu_profile`, `stop_cpu_profile`, `take_heap_snapshot`
   - **Console**: `get_console`, `get_errors`
   - **Connection**: `list_targets`

3. **`cmd/ndp/mcp_test.go`** — Unit tests with mock V8InspectorClient responses

**Modified files:**

4. **`cmd/ndp/main.go`** — Add `--mcp` flag + `--node-port` (default 9229), wire `runMCP`
5. **`cmd/ndp/v8_inspector_client.go`** — Export `Scripts()` accessor if not already public

### Phase 2: Share coverage infrastructure

The `coverageProvider` interface already exists in `coverage_api.go`. ndp's `ndpSession` implements it, enabling:
- Same HTTP API (`--api-port`) for DevTools extension
- Same JSON struct tags, same lcov export
- Coverage data from Node uses same `internal/coverage` types

**Convert ndp's V8 coverage results** to `internal/coverage` types:
- `V8Profiler.TakePreciseCoverage()` returns `[]ScriptCoverage` → map to `coverage.Snapshot`
- This is a thin conversion layer in `cmd/ndp/mcp_tools.go`

### Phase 3: Sourcemap pipeline for Node

Once coverage works, the synthetic sourcemap pipeline transfers directly:
- `analyze_bundle` → extract V8 byte-range chunks from bundled Node code (webpack'd servers, compiled TS)
- `set_bundle_structure` → agent infers file structure
- No `serve_sourcemap` (no browser DevTools), but write .map files to disk
- lcov output uses sourcemapped paths

This reuses `internal/sourcemap/` extractor and generator unchanged.

### Phase 4: Electron support

Electron exposes two target types:
- **Main process**: Node.js V8 inspector → `ndp --mcp --node-port <port>`
- **Renderer process**: Full browser CDP → `cdp --mcp --remote-port <port>`

Both run simultaneously. Agent connects to both MCP servers. No special Electron mode needed — just document the two-server pattern.

## What NOT to do

- Don't add `--node` to cdp — 74 tools call `chromedp.Run` directly, the refactor would be massive
- Don't build a unified target adapter yet — wait for ToolRegistry abstraction
- Don't rewrite ndp's V8 code — it works

## Critical files

| File | Role |
|------|------|
| `cmd/ndp/v8_inspector_client.go` | WebSocket connection, SendCommand, event handling |
| `cmd/ndp/v8_profiler.go` | StartPreciseCoverage, TakePreciseCoverage, CPU/heap profiling |
| `cmd/ndp/v8_debugger.go` | GetScriptSource, breakpoints, stepping |
| `cmd/ndp/v8_runtime.go` | Evaluate, console events |
| `cmd/ndp/main.go` | Entry point, Cobra commands |
| `cmd/cdp/coverage_api.go` | `coverageProvider` interface (already refactored) |
| `cmd/cdp/mcp.go` | Reference for MCP session pattern |
| `internal/coverage/coverage.go` | Shared coverage types with JSON tags |

## Verification

1. `go build ./cmd/ndp/` — builds clean
2. `go test ./cmd/ndp/ -short` — all tests pass
3. Start Node: `node --inspect=9229 -e "setInterval(() => console.log('tick'), 1000)"`
4. Start ndp MCP: `ndp --mcp --node-port 9229`
5. Test via MCP client: `list_sources` → shows node internals + script
6. Test: `start_coverage` → `evaluate {expression: "1+1"}` → `get_coverage` → shows coverage data
7. Test: `get_console` → shows "tick" messages
8. Test: `curl http://127.0.0.1:9224/api/coverage/snapshots` (if --api-port set)
