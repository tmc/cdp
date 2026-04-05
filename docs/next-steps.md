# cdp / ndp — Next Steps

Status: 74 cdp MCP tools, 0 ndp MCP tools (2026-04-05).

## Done (2026-04-04/05)

- ~~Sourcemap pipeline E2E~~ — validated on react.dev
- ~~DevTools live sourcemap~~ — serve_sourcemap installs bundle response intercept + Page.reload
- ~~WebMCP bridge~~ — 4 tools: enable_webmcp, list_web_tools, invoke_web_tool, web_tool_invocations
- ~~DevTools coverage extension~~ — MV3 extension at extension/coverage/ with coverage HTTP API
- ~~Coverage JSON tags~~ — all coverage structs have json tags, Source excluded from API
- ~~coverageProvider interface~~ — coverage API decoupled from mcpSession for ndp reuse

## Workstreams

### 1. chrome-devtools-mcp Parity (~7 tools, ~500 lines)

Close the gaps vs Google's official chrome-devtools-mcp (28 tools). See [/tmp/collab-B140.md] for full analysis. E794 aligned on approach.

| Tool | Priority | Effort | File |
|------|----------|--------|------|
| `set_throttling` (network + CPU) | High | ~60 lines | mcp_emulation_tools.go |
| `close_tab` | Medium | ~30 lines | mcp_tools.go |
| `analyze_trace` (CWV: LCP/INP/CLS) | Medium | ~200 lines | mcp_trace_tools.go + internal/traceinsight/ |
| `trace_insight` (per-metric drill-down) | Medium | ~80 lines | mcp_trace_tools.go |
| `drag` | Low | ~50 lines | mcp_input_tools.go |
| `fill_form` | Low | ~60 lines | mcp_tools.go |
| Extend get_har_entries/get_console | Low | ~20 lines | existing files |

Key decision: extract CWV from raw trace events directly (LargestContentfulPaint::Candidate, EventTiming, LayoutShift) instead of importing Chrome DevTools Frontend's TraceEngine.

### 2. Node.js & Electron MCP Tools (~15 ndp tools)

Add `ndp --mcp` server for V8 debugging. See [node-electron-mcp.md](node-electron-mcp.md) for full design.

**Phase 1**: ndp MCP server — evaluate, coverage, sources, profiler, console, list_targets
**Phase 2**: Share coverage infrastructure via coverageProvider interface (already prepped)
**Phase 3**: Sourcemap pipeline for Node (bundled server code, compiled TS)
**Phase 4**: Electron two-server pattern (ndp for main, cdp for renderer)

### 3. ToolRegistry Abstraction

Single `ToolDef` struct → auto-generates CLI + cdpscript + MCP surfaces. Architectural cleanup that reduces the 74-tool maintenance surface from 3x duplication to 1x. Not urgent; blocked on getting tool surface stable first.

### 4. Polish & Harden

- `-short` skip guards on browser integration tests
- CI-friendly config (SKIP_BROWSER_TESTS env var)
- Fix: navigate tool type mismatch, context/cancel leaks in switch_tab/new_tab
- Document all tools with usage examples
- Verify external .map fetch on -remote-host path

## Planning Documents Index

| Doc | Scope |
|-----|-------|
| [next-steps.md](next-steps.md) | This file — master task tracker |
| [node-electron-mcp.md](node-electron-mcp.md) | ndp MCP server design (Node/Electron) |
| [advanced-capabilities-vision.md](advanced-capabilities-vision.md) | 8 forward-looking capabilities (sourcemaps, coverage-guided exploration, runtime deps, change impact, progressive understanding, live annotation, cdpscripttest, DevTools extension) |
| [agent-browser-gap-analysis.md](agent-browser-gap-analysis.md) | Feature parity vs vercel-labs/agent-browser (mostly closed) |
| [planning/roadmap.md](planning/roadmap.md) | ndp roadmap: workspace persistence, TUI, domain integration |
| [planning/visionary_roadmap.md](planning/visionary_roadmap.md) | ndp vision: AI co-pilot, time travel, multiplayer debugging, DAP bridge |
| [chrome-devtools-mcp-parity.md](chrome-devtools-mcp-parity.md) | chrome-devtools-mcp parity plan (7 gap tools) |
